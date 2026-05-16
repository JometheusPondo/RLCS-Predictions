// Package syncer drives periodic tournament data syncs from a background
// goroutine. It is source-agnostic: the active match source (Liquipedia or
// SheetSource) is passed in as a matchsource.MatchSource and the syncer
// handles the round-derivation + upsert-loop bookkeeping.
//
// Replaces the old internal/liquipedia.Poller, which is left in place
// untouched per the constraint to leave the liquipedia package alone.
package syncer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jometheuspondo/rlcs-predictions/internal/db"
	"github.com/jometheuspondo/rlcs-predictions/internal/matchsource"
	"github.com/jometheuspondo/rlcs-predictions/internal/models"
)

// Poller runs an initial sync immediately on Run(), then re-syncs every
// Interval. Errors are captured in LastError(); they don't propagate and don't
// stop the loop — the next tick retries from scratch.
type Poller struct {
	source     matchsource.MatchSource
	db         *db.DB
	tournament *models.Tournament
	interval   time.Duration
	logger     *slog.Logger

	mu        sync.RWMutex
	lastError error
}

// NewPoller wires up the dependencies. The tournament is taken by pointer so
// in-memory updates to LastSyncedAt are visible to other callers (e.g. the
// /api/sync/status handler reads from the same struct).
func NewPoller(
	source matchsource.MatchSource,
	database *db.DB,
	tournament *models.Tournament,
	interval time.Duration,
	logger *slog.Logger,
) *Poller {
	if logger == nil {
		logger = slog.Default()
	}
	return &Poller{
		source:     source,
		db:         database,
		tournament: tournament,
		interval:   interval,
		logger:     logger,
	}
}

// Run blocks until ctx is cancelled. It does an initial sync immediately,
// then re-syncs every Interval. Intended to be called as `go poller.Run(ctx)`.
func (p *Poller) Run(ctx context.Context) {
	p.logger.Info("poller starting",
		"tournament", p.tournament.LiquipediaPage,
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

// sync performs one fetch-derive-upsert cycle. A timeout protects against
// a hung match source blocking the ticker. All errors are captured on the
// poller; they don't propagate.
func (p *Poller) sync(parentCtx context.Context) {
	ctx, cancel := context.WithTimeout(parentCtx, 60*time.Second)
	defer cancel()

	p.logger.Info("sync starting")

	matches, err := p.source.FetchMatches(ctx)
	if err != nil {
		p.recordError(fmt.Errorf("fetch: %w", err))
		p.logger.Error("sync fetch failed", "err", err)
		return
	}

	p.logger.Debug("matches fetched", "count", len(matches))

	// Derive unique rounds from the match list before upserting matches.
	// Each match carries its full Round metadata, but multiple matches share
	// the same round — we de-dup by (stage, name) and keep the first
	// occurrence's sort_order.
	type roundKey struct{ stage, name string }
	roundSpec := make(map[roundKey]models.Round)
	for _, m := range matches {
		k := roundKey{m.Round.Stage, m.Round.Name}
		if _, ok := roundSpec[k]; !ok {
			roundSpec[k] = m.Round
		}
	}

	roundIDs := make(map[roundKey]int, len(roundSpec))
	for k, r := range roundSpec {
		id, err := p.db.UpsertRound(ctx, p.tournament.ID, r.Stage, r.Name, r.SortOrder)
		if err != nil {
			p.logger.Warn("upsert round failed", "stage", r.Stage, "name", r.Name, "err", err)
			continue
		}
		roundIDs[k] = id
	}

	added, updated, skipped := 0, 0, 0
	for i := range matches {
		m := &matches[i]
		k := roundKey{m.Round.Stage, m.Round.Name}
		roundID, ok := roundIDs[k]
		if !ok {
			// Round upsert failed; can't link this match to a round_id.
			p.logger.Warn("match has unknown round",
				"stage", m.Round.Stage,
				"round", m.Round.Name,
				"id", m.ID,
				"team_a", m.TeamA, "team_b", m.TeamB,
			)
			skipped++
			continue
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
		p.tournament.LastSyncedAt = &syncedAt
	}

	p.recordError(nil)
	p.logger.Info("sync complete",
		"rounds", len(roundIDs),
		"matches_fetched", len(matches),
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

// LastError returns the most recent sync error, or nil if the last sync
// succeeded. Consumed by GET /api/sync/status.
func (p *Poller) LastError() error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lastError
}

// DevSyncHandler returns the POST /api/sync/now handler. Dev-only; main
// registers it conditionally on cfg.DevMode. Calls sync synchronously so the
// caller sees the result and any captured error.
func (p *Poller) DevSyncHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Slightly longer than sync's internal 60s timeout so the inner
		// timeout fires first and surfaces a precise error.
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
