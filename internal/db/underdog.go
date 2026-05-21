package db

import (
	"context"

	"github.com/jometheuspondo/rlcs-predictions/internal/models"
	"github.com/jometheuspondo/rlcs-predictions/internal/scoring"
)

// =============================================================================
// Underdog indicator
// =============================================================================

// ListMatchesWithUnderdog returns ListMatches with the computed Match.Underdog
// field populated on every match whose predictions have LOCKED.
//
// Underdog names the side fewer humans picked (see scoring.UnderdogSide). It is
// deliberately left nil on matches that are not yet locked: revealing the
// crowd's lean while a pick can still be changed would let people chase the
// underdog bonus, so the indicator stays hidden until the pick distribution is
// frozen. Match.Locked is that freeze point — the day's lock time for group
// matches, the match start for bracket matches — so gating on it covers both.
//
// This is the only caller of matchUnderdogs; ListMatches stays free of the
// extra pick-count query for the callers (write checks, the simulation) that
// don't need it.
func (db *DB) ListMatchesWithUnderdog(ctx context.Context) ([]models.Match, error) {
	matches, err := db.ListMatches(ctx)
	if err != nil {
		return nil, err
	}
	underdogs, err := db.matchUnderdogs(ctx)
	if err != nil {
		return nil, err
	}
	for i := range matches {
		if !matches[i].Locked {
			continue // reveal only once predictions are frozen
		}
		if info, ok := underdogs[matches[i].ID]; ok {
			matches[i].Underdog = &info
		}
	}
	return matches, nil
}

// matchUnderdogs returns match_id → UnderdogInfo (the underdog side and the
// number of humans who picked it) for every match that has a single underdog
// side. The underdog is decided by scoring.UnderdogSide from the per-match
// human pick counts.
//
// "Human" picks exclude blast_admin and the benchmark accounts (The Coin,
// Chat): scoringPredictions already drops the admin and flags the benchmarks,
// so skipping Benchmark rows here gives exactly the humans-only basis the
// scoring underdog tally uses — the indicator and the +4 bonus can't disagree.
//
// The returned map is NOT gated by lock state; ListMatchesWithUnderdog applies
// that gate. A match absent from the map has no underdog side (a tie, or no
// minority side at or below the cutoff).
func (db *DB) matchUnderdogs(ctx context.Context) (map[string]models.UnderdogInfo, error) {
	preds, err := db.scoringPredictions(ctx)
	if err != nil {
		return nil, err
	}

	// Human pick counts per match, indexed [matchID] -> {A, B}.
	type sideCounts struct{ a, b int }
	counts := make(map[string]*sideCounts)
	for _, p := range preds {
		if p.Benchmark {
			continue // The Coin / Chat are not humans for the underdog tally
		}
		c := counts[p.MatchID]
		if c == nil {
			c = &sideCounts{}
			counts[p.MatchID] = c
		}
		switch p.Pick {
		case models.PickA:
			c.a++
		case models.PickB:
			c.b++
		}
	}

	out := make(map[string]models.UnderdogInfo)
	for matchID, c := range counts {
		side, ok := scoring.UnderdogSide(c.a, c.b)
		if !ok {
			continue
		}
		picks := c.a
		if side == models.PickB {
			picks = c.b
		}
		out[matchID] = models.UnderdogInfo{Side: side, Picks: picks}
	}
	return out, nil
}
