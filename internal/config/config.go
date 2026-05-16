// Package config loads runtime configuration from environment variables.
// Each field has a documented default. See .env.example at the project root.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

// MatchSource values for the MATCH_SOURCE env var.
const (
	MatchSourceLiquipedia = "liquipedia"
	MatchSourceSheet      = "sheet"
)

// Config bundles all runtime configuration. Loaded once at startup.
type Config struct {
	Port         string
	DatabasePath string
	LogLevel     slog.Level

	// MatchSource is the active source for tournament data. Either
	// "liquipedia" (HTML scrape via MediaWiki API) or "sheet" (CSV export
	// of two Google Sheets tabs). Default "liquipedia" keeps existing
	// installations on the original behavior.
	//
	// Switching mid-tournament orphans existing predictions: the two
	// sources produce different match_id hashes, so the predictions table's
	// foreign keys to matches.id no longer resolve. One-way switch.
	MatchSource string

	// Liquipedia-specific. Read regardless of MatchSource so a misconfigured
	// switch back to liquipedia still works.
	LiquipediaPage         string
	LiquipediaPollInterval time.Duration
	LiquipediaUserAgent    string

	// Sheet-specific. Required when MatchSource == "sheet". The spreadsheet
	// must be shared "Anyone with the link can view" — no auth is performed.
	SheetSpreadsheetID string
	SheetGroupsGID     string
	SheetBracketGID    string
	SheetPollInterval  time.Duration

	// DevMode gates dev-only routes like POST /api/sync/now. Off by default;
	// set DEV_MODE=true in .env when working locally.
	DevMode bool
}

// Load reads environment variables and applies defaults. If a `.env` file
// exists in the working directory, its KEY=VALUE pairs are loaded into the
// process environment first (existing env vars take precedence, so a CLI
// override beats .env beats defaults). Returns an error only for parse
// failures (durations, etc.) or an invalid MATCH_SOURCE; missing values fall
// back to defaults.
func Load() (*Config, error) {
	loadDotEnvIfPresent()

	cfg := &Config{
		Port:                getEnv("PORT", "8080"),
		DatabasePath:        getEnv("DATABASE_PATH", "./data/rlcs.db"),
		LiquipediaPage:      getEnv("LIQUIPEDIA_PAGE", "Rocket_League_Championship_Series/2026/Paris_Major"),
		LiquipediaUserAgent: getEnv("LIQUIPEDIA_USER_AGENT", "RLCSPredictions/0.1 (https://github.com/jometheuspondo/rlcs-predictions; contact: replace-me@example.com)"),

		// Defaults for the Paris Major broadcast LOP spreadsheet (verified
		// against the public copy on 2026-05-15). If the operator points at
		// a different sheet, all three IDs must be overridden together.
		SheetSpreadsheetID: getEnv("SHEET_SPREADSHEET_ID", "1Eo3OEO8CY048BTz8QFmWG-xczH6UOecjFCFZCJQuLUs"),
		SheetGroupsGID:     getEnv("SHEET_GROUPS_GID", "10266191"),
		SheetBracketGID:    getEnv("SHEET_BRACKET_GID", "936433744"),
	}

	interval, err := time.ParseDuration(getEnv("LIQUIPEDIA_POLL_INTERVAL", "5m"))
	if err != nil {
		return nil, fmt.Errorf("LIQUIPEDIA_POLL_INTERVAL: %w", err)
	}
	cfg.LiquipediaPollInterval = interval

	// Sheet polling is cheap (a single HTTPS GET to docs.google.com per tab,
	// no rate limit), so a shorter default than Liquipedia is reasonable.
	// Operators can override either independently.
	sheetInterval, err := time.ParseDuration(getEnv("SHEET_POLL_INTERVAL", "2m"))
	if err != nil {
		return nil, fmt.Errorf("SHEET_POLL_INTERVAL: %w", err)
	}
	cfg.SheetPollInterval = sheetInterval

	cfg.LogLevel = parseLogLevel(getEnv("LOG_LEVEL", "info"))
	cfg.DevMode = strings.EqualFold(getEnv("DEV_MODE", "false"), "true")

	// MATCH_SOURCE default is liquipedia for safety — an existing install
	// upgrading without setting the env var keeps its current behavior.
	cfg.MatchSource = strings.ToLower(strings.TrimSpace(getEnv("MATCH_SOURCE", MatchSourceLiquipedia)))
	switch cfg.MatchSource {
	case MatchSourceLiquipedia, MatchSourceSheet:
		// ok
	default:
		return nil, fmt.Errorf("MATCH_SOURCE: %q is not a valid source (want %q or %q)",
			cfg.MatchSource, MatchSourceLiquipedia, MatchSourceSheet)
	}

	return cfg, nil
}

// HasPlaceholderUserAgent returns true if the User-Agent still contains the
// "replace-me" sentinel. Callers should warn loudly at startup — Liquipedia
// bans on bogus contact info per spec § 5.1.
func (c *Config) HasPlaceholderUserAgent() bool {
	return strings.Contains(c.LiquipediaUserAgent, "replace-me")
}

// ActivePollInterval returns the poll interval for the active match source.
// Each source has its own interval because Liquipedia enforces a strict rate
// gate while Google Sheets does not.
func (c *Config) ActivePollInterval() time.Duration {
	if c.MatchSource == MatchSourceSheet {
		return c.SheetPollInterval
	}
	return c.LiquipediaPollInterval
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// loadDotEnvIfPresent reads .env from cwd (if present) and populates any
// missing env vars. Existing env vars are never overwritten so a real shell
// export still wins. Silent if .env is missing. Lines are KEY=VALUE; `#`
// starts a comment; blank lines skipped; surrounding quotes on VALUE stripped.
func loadDotEnvIfPresent() {
	data, err := os.ReadFile(".env")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		if n := len(val); n >= 2 {
			if (val[0] == '"' && val[n-1] == '"') || (val[0] == '\'' && val[n-1] == '\'') {
				val = val[1 : n-1]
			}
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}
