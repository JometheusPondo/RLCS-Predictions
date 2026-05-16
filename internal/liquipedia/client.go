// Package liquipedia provides a rate-limited HTTP client and an HTML parser
// for scraping tournament data from Liquipedia's MediaWiki API.
package liquipedia

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MinRequestInterval enforces the 1-request-per-2-seconds gate from spec § 5.1.
// Liquipedia bans on policy violations; this is non-negotiable.
const MinRequestInterval = 2 * time.Second

// DefaultBaseURL is the Liquipedia Rocket League wiki API base.
const DefaultBaseURL = "https://liquipedia.net/rocketleague"

// Client is a rate-limited HTTP client for Liquipedia's action=parse API.
//
// IMPORTANT — gzip handling: this client deliberately does NOT set the
// Accept-Encoding header. Liquipedia's varnish proxy returns 406 on requests
// without gzip ("Gzip encoding is required for API requests"). Go's default
// http.Transport sets Accept-Encoding: gzip automatically AND auto-
// decompresses the response body — but only if the user code hasn't touched
// that header. Setting it manually disables Go's auto-decompression and yields
// raw gzipped bytes. So: leave Accept-Encoding alone, let Go handle it.
type Client struct {
	httpClient *http.Client
	userAgent  string
	baseURL    string

	// saveResponsePath, if non-empty, persists each parsed HTML to disk for
	// debugging (spec § 5.3). Set to "tmp/last_response.html" in dev.
	saveResponsePath string

	logger *slog.Logger

	mu              sync.Mutex
	lastRequestTime time.Time
}

// ClientOptions configures a Client. UserAgent is required.
type ClientOptions struct {
	UserAgent        string
	BaseURL          string       // optional; defaults to DefaultBaseURL
	HTTPClient       *http.Client // optional; defaults to a 30s-timeout client
	SaveResponsePath string       // optional; e.g. "tmp/last_response.html"
	Logger           *slog.Logger // optional; defaults to slog.Default()
}

// NewClient returns a Client with the given options. Returns an error if
// UserAgent is empty (mandatory per Liquipedia's terms of use).
func NewClient(opts ClientOptions) (*Client, error) {
	if opts.UserAgent == "" {
		return nil, errors.New("liquipedia: UserAgent is required")
	}

	c := &Client{
		userAgent:        opts.UserAgent,
		baseURL:          opts.BaseURL,
		saveResponsePath: opts.SaveResponsePath,
		logger:           opts.Logger,
	}
	if c.baseURL == "" {
		c.baseURL = DefaultBaseURL
	}
	if c.logger == nil {
		c.logger = slog.Default()
	}
	if opts.HTTPClient != nil {
		c.httpClient = opts.HTTPClient
	} else {
		c.httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return c, nil
}

// parseAPIResponse models the MediaWiki action=parse&format=json envelope:
//
//	{ "parse": { "title": "...", "pageid": N, "text": { "*": "<html>" } } }
//
// Errors come back as { "error": { "code": "...", "info": "..." } }.
type parseAPIResponse struct {
	Parse *struct {
		Title  string `json:"title"`
		PageID int    `json:"pageid"`
		Text   *struct {
			HTML string `json:"*"`
		} `json:"text"`
	} `json:"parse"`
	Error *struct {
		Code string `json:"code"`
		Info string `json:"info"`
	} `json:"error"`
}

// FetchParsedPage fetches the parsed wikitext HTML for a page slug via
// MediaWiki's action=parse&format=json&prop=text endpoint. The rate gate is
// enforced before the request is sent.
//
// pageSlug is the slug only, e.g.
// "Rocket_League_Championship_Series/2026/Paris_Major".
func (c *Client) FetchParsedPage(ctx context.Context, pageSlug string) (string, error) {
	if err := c.wait(ctx); err != nil {
		return "", err
	}

	apiURL := fmt.Sprintf(
		"%s/api.php?action=parse&page=%s&format=json&prop=text",
		c.baseURL,
		url.QueryEscape(pageSlug),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")
	// DO NOT set Accept-Encoding here — see Client comment for why.

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("liquipedia returned %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	var parsed parseAPIResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("decode json: %w", err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("api error %s: %s", parsed.Error.Code, parsed.Error.Info)
	}
	if parsed.Parse == nil || parsed.Parse.Text == nil {
		return "", errors.New("liquipedia response missing parse.text")
	}

	html := parsed.Parse.Text.HTML

	if c.saveResponsePath != "" {
		if err := c.saveResponse(html); err != nil {
			c.logger.Warn("save response failed", "path", c.saveResponsePath, "err", err)
			// don't fail the fetch on a save error
		}
	}

	return html, nil
}

// wait blocks until the rate gate allows another request, or returns ctx.Err()
// if cancellation comes first. The mutex serializes callers so concurrent
// requests respect the gate.
func (c *Client) wait(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delay := MinRequestInterval - time.Since(c.lastRequestTime)
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	c.lastRequestTime = time.Now()
	return nil
}

func (c *Client) saveResponse(html string) error {
	if dir := filepath.Dir(c.saveResponsePath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(c.saveResponsePath, []byte(html), 0o644)
}
