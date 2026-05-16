package liquipedia

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jometheuspondo/rlcs-predictions/internal/db"
	"github.com/jometheuspondo/rlcs-predictions/internal/models"
)

// Poller drives Liquipedia syncs from a background goroutine. It runs an
// initial sync immediately on Run(), then ticks every Interval. Errors don't
// propagate — they're captured in LastError() and the next tick retries.
type Poller struct {
	client     *Client
	db         *db.DB
	tournament *models.Tournament
	interval   time.Duration
	logger     *slog.Logger

	mu        sync.RWMutex
	lastError error
}

// NewPoller wires up the dependencies. The tournament is taken by pointer
// because LastSyncedAt updates we read elsewhere (e.g. /api/sync/status) want
// to see the latest value.
func NewPoller(
	client *Client,
	database *db.DB,
	tournament *models.Tournament,
	interval time.Duration,
	logger *slog.Logger,
) *Poller {
	if logger == nil {
		logger = slog.Default()
	}
	return &Poller{
		client:     client,
		db:         database,
		tournament: tournament,
		interval:   interval,
		logger:     logger,
	}
}

// Run blocks until ctx is cancelled. It does an initial sync immediately, then
// syncs every Interval. Call as `go poller.Run(ctx)`.
func (p *Poller) Run(ctx context.Context) {
	p.logger.Info("poller starting",
		"page", p.tournament.LiquipediaPage,
		"interval", p.interval,
	)

	p.sync(ctx)

	t := time.NewTicker(p.interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("poller stopping")
			return
		case <-t.C:
			p.sync(ctx)
		}
	}
}

// sync runs one full fetch-parse-upsert cycle. A timeout protects against a
// hung HTTP request blocking the ticker. All errors are captured on the poller;
// they do not propagate up.
func (p *Poller) sync(parentCtx context.Context) {
	ctx, cancel := context.WithTimeout(parentCtx, 60*time.Second)
	defer cancel()

	page := p.tournament.LiquipediaPage
	p.logger.Info("sync starting", "page", page)

	html, err := p.client.FetchParsedPage(ctx, page)
	if err != nil {
		p.recordError(fmt.Errorf("fetch: %w", err))
		p.logger.Error("sync fetch failed", "err", err)
		return
	}

	parsed, err := ParsePage(html)
	if err != nil {
		p.recordError(fmt.Errorf("parse: %w", err))
		p.logger.Error("sync parse failed", "err", err)
		return
	}

	p.logger.Debug("parsed",
		"rounds", len(parsed.Rounds),
		"matches", len(parsed.Matches),
	)

	// Upsert rounds; capture id-by-name so matches can resolve their round_id.
	roundIDs := make(map[string]int, len(parsed.Rounds))
	for _, r := range parsed.Rounds {
		id, err := p.db.UpsertRound(ctx, p.tournament.ID, r.Stage, r.Name, r.SortOrder)
		if err != nil {
			p.logger.Warn("upsert round failed", "name", r.Name, "err", err)
			continue
		}
		roundIDs[r.Name] = id
	}

	added, updated, skipped := 0, 0, 0
	for _, pm := range parsed.Matches {
		roundID, ok := roundIDs[pm.RoundName]
		if !ok {
			// Round upsert failed earlier; can't link this match.
			p.logger.Warn("match has unknown round",
				"round", pm.RoundName,
				"team_a", pm.TeamA, "team_b", pm.TeamB,
			)
			skipped++
			continue
		}

		m := &models.Match{
			ID:          computeMatchID(p.tournament.ID, pm.RoundStage, pm.RoundName, pm.TeamA, pm.TeamB),
			TeamA:       pm.TeamA,
			TeamB:       pm.TeamB,
			TeamAScore:  pm.TeamAScore,
			TeamBScore:  pm.TeamBScore,
			Winner:      pm.Winner,
			Status:      pm.Status,
			ScheduledAt: pm.ScheduledAt,
		}

		existing, getErr := p.db.GetMatch(ctx, m.ID)
		isNew := errors.Is(getErr, db.ErrNotFound) || existing == nil

		if err := p.db.UpsertMatch(ctx, m, roundID); err != nil {
			p.logger.Warn("upsert match failed",
				"id", m.ID,
				"team_a", m.TeamA, "team_b", m.TeamB,
				"err", err,
			)
			continue
		}
		if isNew {
			added++
		} else {
			updated++
		}
	}

	syncedAt := time.Now().UTC().Format(time.RFC3339)
	if err := p.db.UpdateLastSyncedAt(ctx, p.tournament.ID, syncedAt); err != nil {
		p.logger.Warn("update last_synced_at failed", "err", err)
	} else {
		// Keep the in-memory struct in sync so callers reading it (like
		// /api/sync/status) see the new timestamp without a DB round-trip.
		p.tournament.LastSyncedAt = &syncedAt
	}

	p.recordError(nil)
	p.logger.Info("sync complete",
		"rounds", len(parsed.Rounds),
		"matches_parsed", len(parsed.Matches),
		"added", added,
		"updated", updated,
		"skipped", skipped,
	)
}

func (p *Poller) recordError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastError = err
}

// LastError returns the most recent sync error (nil if the last sync succeeded).
// Exposed so /api/sync/status in Phase 4 can fold this into its response.
func (p *Poller) LastError() error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lastError
}

// computeMatchID returns the stable 16-char hex match ID per spec § 5.4.
// Team names are sorted lexically so the same physical match always hashes to
// the same id regardless of which side Liquipedia renders first.
//
// The id is a function of (tournament, stage, round name, sorted team pair).
// Changes to the round name or team names produce a different id — meaning
// some Liquipedia edits may cause "new" rows. That's an accepted limitation.
func computeMatchID(tournamentID int, stage, roundName, teamA, teamB string) string {
	a, b := teamA, teamB
	if a > b {
		a, b = b, a
	}
	h := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%s|%s|%s", tournamentID, stage, roundName, a, b)))
	return hex.EncodeToString(h[:8])
}

// DevSyncHandler returns a handler for POST /api/sync/now. Dev-only — main
// should register it conditionally on cfg.DevMode. Calls sync synchronously so
// the caller sees the result.
func (p *Poller) DevSyncHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Slightly longer than sync's internal 60s timeout to allow the inner
		// timeout to fire first and produce a precise error.
		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()

		p.sync(ctx)

		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{"triggered": true}
		if err := p.LastError(); err != nil {
			resp["last_error"] = err.Error()
			w.WriteHeader(http.StatusInternalServerError)
		}
		_ = json.NewEncoder(w).Encode(resp)
	}
}
