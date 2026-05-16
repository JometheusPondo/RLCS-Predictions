// Package matchsource defines a uniform interface for producing the match
// list that drives the prediction site, and ships two implementations:
//
//   - LiquipediaSource: wraps the existing liquipedia package's HTML fetch +
//     parse. No behavior change vs the pre-interface code path.
//
//   - SheetSource: fetches two public Google Sheets tabs (Groups Output and
//     Bracket Output) as CSV via the docs.google.com export endpoint and
//     turns them into models.Match values. Placeholder bracket rows
//     (unresolved teams) are emitted with empty team_a / team_b strings and
//     display text in PlaceholderA / PlaceholderB.
//
// The active source is selected at startup via the MATCH_SOURCE env var
// (config.Config.MatchSource), and handed to syncer.Poller.
package matchsource

import (
	"context"

	"github.com/jometheuspondo/rlcs-predictions/internal/models"
)

// MatchSource is the interface that syncer.Poller consumes. A FetchMatches
// call should return the COMPLETE current view of the tournament — every
// round, every match, in whatever state the source currently sees them. The
// poller is responsible for diffing against the DB via UpsertMatch.
//
// The returned matches carry their round's full metadata embedded in
// Match.Round; the poller uses (Round.Stage, Round.Name, Round.SortOrder) to
// upsert the rounds table before upserting matches. Multiple matches sharing
// the same round produce one rounds row.
//
// Implementations should NOT mutate the database directly. They should not
// retain state between calls beyond what's needed for their own HTTP client
// (rate limiters, cookies). All sync timing is owned by the poller.
type MatchSource interface {
	// FetchMatches returns the full current match list. Errors should describe
	// the failure mode (fetch vs parse vs network) for log triage; the poller
	// captures the error in LastError and the next tick retries.
	FetchMatches(ctx context.Context) ([]models.Match, error)
}
