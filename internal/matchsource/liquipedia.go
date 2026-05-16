package matchsource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/jometheuspondo/rlcs-predictions/internal/liquipedia"
	"github.com/jometheuspondo/rlcs-predictions/internal/models"
)

// LiquipediaSource adapts the existing liquipedia.Client + ParsePage path to
// the MatchSource interface. It owns no behavior of its own beyond converting
// liquipedia.ParsedMatch values into models.Match values with a stable
// match_id computed the same way the old liquipedia.Poller did.
//
// The internal/liquipedia package itself is NOT modified by this file —
// LiquipediaSource consumes its existing public API.
type LiquipediaSource struct {
	client       *liquipedia.Client
	pageSlug     string
	tournamentID int
}

// NewLiquipediaSource constructs a source that pulls from a Liquipedia page
// slug. The caller is responsible for the rate-limited client (see
// liquipedia.NewClient) and for passing the tournament_id that's used to
// salt the match-id hash.
func NewLiquipediaSource(client *liquipedia.Client, pageSlug string, tournamentID int) *LiquipediaSource {
	return &LiquipediaSource{
		client:       client,
		pageSlug:     pageSlug,
		tournamentID: tournamentID,
	}
}

// FetchMatches fetches the Liquipedia page, parses it, and converts each
// ParsedMatch into a models.Match. Round metadata is embedded into each
// match via a name→ParsedRound lookup; if a match references an unknown
// round name (shouldn't happen — the parser emits both from the same DOM
// walk), the match is dropped with no error.
//
// The match-id scheme matches the pre-interface Liquipedia poller:
//
//	sha256(tournament_id | stage | round_name | sorted team pair)[:8]
//
// So switching MATCH_SOURCE from liquipedia to liquipedia (same source) is a
// no-op; switching to sheet uses a DIFFERENT id space (see SheetSource).
func (s *LiquipediaSource) FetchMatches(ctx context.Context) ([]models.Match, error) {
	html, err := s.client.FetchParsedPage(ctx, s.pageSlug)
	if err != nil {
		return nil, fmt.Errorf("liquipedia fetch: %w", err)
	}

	parsed, err := liquipedia.ParsePage(html)
	if err != nil {
		return nil, fmt.Errorf("liquipedia parse: %w", err)
	}

	// Build (stage, name) → Round lookup so each ParsedMatch can carry its
	// round's sort_order without the poller needing the rounds slice
	// separately. Stage is part of the key because round names CAN collide
	// across stages (a future tournament might use "Round 1" in both group
	// and bracket).
	type roundKey struct{ stage, name string }
	roundIdx := make(map[roundKey]liquipedia.ParsedRound, len(parsed.Rounds))
	for _, r := range parsed.Rounds {
		roundIdx[roundKey{r.Stage, r.Name}] = r
	}

	out := make([]models.Match, 0, len(parsed.Matches))
	for _, pm := range parsed.Matches {
		r, ok := roundIdx[roundKey{pm.RoundStage, pm.RoundName}]
		if !ok {
			// Match emitted a round name the parser didn't also register —
			// drop quietly; the poller logs at the source level.
			continue
		}

		m := models.Match{
			ID: liquipediaMatchID(s.tournamentID, pm.RoundStage, pm.RoundName, pm.TeamA, pm.TeamB),
			Round: models.Round{
				Stage:     r.Stage,
				Name:      r.Name,
				SortOrder: r.SortOrder,
			},
			TeamA:       pm.TeamA,
			TeamB:       pm.TeamB,
			TeamAScore:  pm.TeamAScore,
			TeamBScore:  pm.TeamBScore,
			Winner:      pm.Winner,
			Status:      pm.Status,
			ScheduledAt: pm.ScheduledAt,
			// Liquipedia rows never carry placeholders or slots — leave nil.
		}
		out = append(out, m)
	}

	return out, nil
}

// liquipediaMatchID reproduces the hash scheme from the pre-interface
// liquipedia.Poller exactly: tournament_id | stage | round | sorted team pair.
// Sorting the pair makes the id stable regardless of which side the parser
// rendered first.
func liquipediaMatchID(tournamentID int, stage, roundName, teamA, teamB string) string {
	a, b := teamA, teamB
	if a > b {
		a, b = b, a
	}
	h := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%s|%s|%s", tournamentID, stage, roundName, a, b)))
	return hex.EncodeToString(h[:8])
}
