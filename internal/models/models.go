// Package models defines the domain types shared across the db, api, and scraper layers.
//
// All JSON tags are aligned with the response shapes in spec § 6.
package models

// Match status values. Strings (not a named type) to keep db Scan / json marshal painless.
const (
	StatusUpcoming  = "upcoming"
	StatusLive      = "live"
	StatusCompleted = "completed"
)

// Pick values for a participant's prediction or a completed match's winner.
const (
	PickA = "A"
	PickB = "B"
)

// Stage values for a round. The group stage is a single round robin (4 groups
// of 4); 'group' rather than 'swiss' — the spec's original Swiss assumption
// didn't match the actual Paris Major format.
const (
	StageGroup   = "group"
	StageBracket = "bracket"
)

// Sort-order anchors per spec § 5.4. Group-stage rounds get 100 * round_number;
// bracket rounds occupy the 1000+ range so they always sort after the group stage.
const (
	SortOrderGroupStep   = 100
	SortOrderQuarters    = 1000
	SortOrderSemifinals  = 1100
	SortOrderFinal       = 1200
)

// Participant is the shape returned by GET /api/participants and the embedded
// participant in GET /api/participants/:id. WinnerPicks is the append-only
// history of tournament-winner picks, oldest first; the last element is the
// current pick. Empty slice when the participant has never picked.
type Participant struct {
	ID           string       `json:"id"`
	DisplayName  string       `json:"display_name"`
	Score        int          `json:"score"`
	CorrectCount int          `json:"correct_count"`
	WinnerPicks  []WinnerPick `json:"winner_picks"`
}

// WinnerPick is one entry in a participant's tournament-winner pick history.
type WinnerPick struct {
	TeamName string `json:"team_name"`
	PickedAt string `json:"picked_at"`
}

// Prediction is a single pick. Embedded inside ParticipantWithPredictions for the profile endpoint.
type Prediction struct {
	MatchID string `json:"match_id"`
	Pick    string `json:"pick"`
}

// ParticipantWithPredictions is the shape returned by GET /api/participants/:id.
//
// Predictions visibility is permission-filtered by the handler: a requester
// viewing their own profile (or blast_admin) sees every prediction; anyone
// else sees only predictions on completed matches. WinnerPicks is always
// fully visible regardless of requester — the leaderboard shows it publicly.
type ParticipantWithPredictions struct {
	Participant
	Predictions []Prediction `json:"predictions"`
}

// AdminID is the backstage reference account. It can authenticate and read
// everything, but it is NOT a participant: it is filtered out of the public
// participant list and the leaderboard, it earns no score, and its
// predictions (if any) are excluded from scoring — including the underdog
// pick-count tally.
const AdminID = "blast_admin"

// Tournament is internal-facing; not currently returned by any API endpoint.
type Tournament struct {
	ID             int     `json:"id"`
	LiquipediaPage string  `json:"liquipedia_page"`
	Name           string  `json:"name"`
	IsActive       bool    `json:"is_active"`
	LastSyncedAt   *string `json:"last_synced_at,omitempty"`
}

// Round is embedded in Match. Per spec § 6 the embedded object only exposes
// {name, stage, sort_order}; ID stays internal.
type Round struct {
	ID        int    `json:"-"`
	Stage     string `json:"stage"`
	SortOrder int    `json:"sort_order"`
	Name      string `json:"name"`
}

// UnderdogInfo carries the underdog side of a match and the number of human
// participants who picked it. Embedded as Match.Underdog.
type UnderdogInfo struct {
	Side  string `json:"side"`
	Picks int    `json:"picks"`
}

// Match is the shape returned by GET /api/matches and used internally by the
// match-source layer. Pointer fields are nullable: scores are nil until the
// match plays, winner is nil until it completes, scheduled_at is nil if the
// source hasn't announced a time.
//
// PlaceholderA / PlaceholderB carry display text ("Group A First", "Winner of
// C") for bracket matches whose teams haven't resolved yet. When set, the
// corresponding TeamA / TeamB is the empty string. Liquipedia-sourced rows
// always leave these nil and put a real team name in TeamA / TeamB —
// placeholder matches from Liquipedia get skipped at parse time.
//
// Slot is the intra-day position string from the sheet ("2A", "5B") for
// SheetSource rows; Liquipedia rows leave this nil and use ScheduledAt
// instead.
//
// Locked is a COMPUTED field, not stored in the DB. The db layer sets it on
// every match returned by ListMatches: true means predictions on this match
// are locked (the day's lock time has passed, or — on the final day — the
// match has started, or the match is completed). The frontend uses it to
// gate tappability; the server also enforces it independently on writes.
//
// Underdog is also COMPUTED, not stored. It carries the underdog side and the
// human-pick count for that side, or is nil when the match has no single
// underdog. Set only by ListMatchesWithUnderdog and only on LOCKED matches,
// so the crowd's lean isn't revealed while picks can still change.
type Match struct {
	ID           string        `json:"id"`
	Round        Round         `json:"round"`
	TeamA        string        `json:"team_a"`
	TeamB        string        `json:"team_b"`
	TeamAScore   *int          `json:"team_a_score"`
	TeamBScore   *int          `json:"team_b_score"`
	Winner       *string       `json:"winner"`
	Status       string        `json:"status"`
	ScheduledAt  *string       `json:"scheduled_at"`
	PlaceholderA *string       `json:"placeholder_a"`
	PlaceholderB *string       `json:"placeholder_b"`
	Slot         *string       `json:"slot"`
	Locked       bool          `json:"locked"`
	Underdog     *UnderdogInfo `json:"underdog"`
}

// SyncStatus is the shape returned by GET /api/sync/status.
type SyncStatus struct {
	LastSyncedAt *string `json:"last_synced_at"`
	LastError    *string `json:"last_error"`
}
