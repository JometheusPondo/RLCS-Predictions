package db

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jometheuspondo/rlcs-predictions/internal/locking"
	"github.com/jometheuspondo/rlcs-predictions/internal/models"
	"github.com/jometheuspondo/rlcs-predictions/internal/scoring"
	"github.com/jometheuspondo/rlcs-predictions/internal/simulation"
)

// Sentinel errors returned by query functions. Callers (API handlers, the
// poller) match on these to decide HTTP status codes and log levels.
var (
	// ErrNotFound is returned when a lookup targets a row that doesn't exist.
	ErrNotFound = errors.New("not found")

	// ErrIDConflict is returned by CreateParticipant when the derived id already exists.
	ErrIDConflict = errors.New("id conflict")

	// ErrPredictionsLocked is returned when a prediction write/delete targets a
	// match whose predictions are locked — the match has completed, or the
	// day's lock time has passed, or (on the final day) the match has started.
	// Server-side enforcement; the frontend also gates this via Match.Locked.
	ErrPredictionsLocked = errors.New("predictions are locked for this match")
)

// =============================================================================
// Participants
// =============================================================================

// ListParticipants returns every participant with their computed score and
// winner-pick history, sorted score DESC then name ASC.
//
// This includes the blast_admin backstage account — it must be selectable in
// the landing-page dropdown so the operator can log in. Excluding it from the
// leaderboard is the leaderboard's job (it filters AdminID out client-side),
// and the scoring layer excludes its predictions, so its score is always 0.
//
// Scores are computed in Go (see internal/scoring) rather than in SQL: the
// rules — a 4-way branch plus an underdog cross-participant pick count — don't
// fit a readable inline query.
func (db *DB) ListParticipants(ctx context.Context) ([]models.Participant, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, display_name FROM participants`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.Participant, 0)
	for rows.Next() {
		var p models.Participant
		if err := rows.Scan(&p.ID, &p.DisplayName); err != nil {
			return nil, err
		}
		p.WinnerPicks = []models.WinnerPick{}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	scores, err := db.computeAllScores(ctx)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Score = scores[out[i].ID]
	}

	// Attach winner-pick history. One query for all of it, grouped in Go —
	// avoids an N+1 without needing a dynamic IN clause.
	picks, err := db.allWinnerPicks(ctx)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if ps, ok := picks[out[i].ID]; ok {
			out[i].WinnerPicks = ps
		}
	}

	// Leaderboard order: score desc, then display name asc.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].DisplayName < out[j].DisplayName
	})

	return out, nil
}

// allWinnerPicks returns every participant's winner-pick history, keyed by
// participant id, each slice ordered oldest-first.
func (db *DB) allWinnerPicks(ctx context.Context) (map[string][]models.WinnerPick, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT participant_id, team_name, picked_at
		FROM winner_pick_history
		ORDER BY participant_id, picked_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string][]models.WinnerPick)
	for rows.Next() {
		var (
			pid  string
			pick models.WinnerPick
		)
		if err := rows.Scan(&pid, &pick.TeamName, &pick.PickedAt); err != nil {
			return nil, err
		}
		out[pid] = append(out[pid], pick)
	}
	return out, rows.Err()
}

// winnerPicksFor returns a single participant's winner-pick history, oldest first.
func (db *DB) winnerPicksFor(ctx context.Context, participantID string) ([]models.WinnerPick, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT team_name, picked_at
		FROM winner_pick_history
		WHERE participant_id = ?
		ORDER BY picked_at ASC
	`, participantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.WinnerPick, 0)
	for rows.Next() {
		var pick models.WinnerPick
		if err := rows.Scan(&pick.TeamName, &pick.PickedAt); err != nil {
			return nil, err
		}
		out = append(out, pick)
	}
	return out, rows.Err()
}

// GetParticipantPassword returns the stored plaintext password for a
// participant. Returns ErrNotFound if no such participant. A NULL password
// (participant predates the auth migration, or none was assigned) comes back
// as an empty string — callers treat empty as "cannot authenticate".
func (db *DB) GetParticipantPassword(ctx context.Context, id string) (string, error) {
	var pw sql.NullString
	err := db.QueryRowContext(ctx,
		`SELECT password FROM participants WHERE id = ?`, id,
	).Scan(&pw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return pw.String, nil
}

// ParticipantExists reports whether a participant id is present. Used by the
// auth middleware to validate a bearer token without loading the whole row.
func (db *DB) ParticipantExists(ctx context.Context, id string) (bool, error) {
	var one int
	err := db.QueryRowContext(ctx,
		`SELECT 1 FROM participants WHERE id = ?`, id,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// CreateParticipant inserts a new participant with the given id and display name.
// Returns ErrIDConflict if the id is already taken. New participants have a NULL
// password until an operator assigns one and an empty winner-pick history.
func (db *DB) CreateParticipant(ctx context.Context, id, displayName string) (*models.Participant, error) {
	_, err := db.ExecContext(ctx,
		`INSERT INTO participants (id, display_name) VALUES (?, ?)`,
		id, displayName,
	)
	if err != nil {
		// modernc.org/sqlite returns the constraint failure as a string in err.Error().
		// Matching on the message is brittle but acceptable for a single, well-known constraint.
		if strings.Contains(err.Error(), "UNIQUE constraint failed") ||
			strings.Contains(err.Error(), "constraint failed: PRIMARY KEY") {
			return nil, ErrIDConflict
		}
		return nil, err
	}
	return &models.Participant{
		ID:          id,
		DisplayName: displayName,
		Score:       0,
		WinnerPicks: []models.WinnerPick{},
	}, nil
}

// GetParticipantWithPredictions returns a participant with their computed
// score, every prediction they've made, and their winner-pick history.
// Returns ErrNotFound if no such participant.
//
// This returns ALL predictions unconditionally — permission filtering (hiding
// in-progress picks from other users) is the handler's job, since only the
// handler knows who is asking. blast_admin can be fetched here by id even
// though it's excluded from ListParticipants.
func (db *DB) GetParticipantWithPredictions(ctx context.Context, id string) (*models.ParticipantWithPredictions, error) {
	var pwp models.ParticipantWithPredictions
	err := db.QueryRowContext(ctx,
		`SELECT id, display_name FROM participants WHERE id = ?`, id,
	).Scan(&pwp.ID, &pwp.DisplayName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx,
		`SELECT match_id, pick FROM predictions WHERE participant_id = ? ORDER BY match_id`,
		id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	pwp.Predictions = make([]models.Prediction, 0)
	for rows.Next() {
		var p models.Prediction
		if err := rows.Scan(&p.MatchID, &p.Pick); err != nil {
			return nil, err
		}
		pwp.Predictions = append(pwp.Predictions, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Score needs the global picture (the underdog rule counts across all
	// participants), so it's the same computation as the leaderboard.
	scores, err := db.computeAllScores(ctx)
	if err != nil {
		return nil, err
	}
	pwp.Score = scores[id]

	picks, err := db.winnerPicksFor(ctx, id)
	if err != nil {
		return nil, err
	}
	pwp.WinnerPicks = picks

	return &pwp, nil
}

// =============================================================================
// Scoring
// =============================================================================

// computeAllScores returns participant_id → total score, applying the scoring
// rules in internal/scoring. The underdog rule needs the full cross-
// participant pick distribution, so even a single participant's score is
// derived from the global data set.
func (db *DB) computeAllScores(ctx context.Context) (map[string]int, error) {
	matches, err := db.ListMatches(ctx)
	if err != nil {
		return nil, err
	}
	preds, err := db.scoringPredictions(ctx)
	if err != nil {
		return nil, err
	}
	return scoring.ComputeScores(matches, preds), nil
}

// scoringPredictions returns every prediction as a scoring.PredictionRow,
// EXCLUDING the blast_admin account. blast_admin is not a participant: its
// predictions must neither earn points nor count toward anyone's underdog
// tally, so they are filtered out before scoring ever sees them.
func (db *DB) scoringPredictions(ctx context.Context) ([]scoring.PredictionRow, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT participant_id, match_id, pick FROM predictions WHERE participant_id != ?`,
		models.AdminID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]scoring.PredictionRow, 0)
	for rows.Next() {
		var r scoring.PredictionRow
		if err := rows.Scan(&r.ParticipantID, &r.MatchID, &r.Pick); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// =============================================================================
// Simulation
// =============================================================================

// SimulateProjection runs the best-case / worst-case standings projection for
// the current day (see internal/simulation). Returns the projected day
// (YYYY-MM-DD, "" when the tournament has no unfinished matches) and one
// result per real participant.
//
// It reuses the same inputs as scoring — every match, every non-admin
// prediction, the participant list — so the projection and the live
// leaderboard are always derived from one consistent snapshot.
func (db *DB) SimulateProjection(ctx context.Context) (day string, results []simulation.Result, err error) {
	matches, err := db.ListMatches(ctx)
	if err != nil {
		return "", nil, err
	}
	preds, err := db.scoringPredictions(ctx)
	if err != nil {
		return "", nil, err
	}
	participants, err := db.ListParticipants(ctx)
	if err != nil {
		return "", nil, err
	}
	day, results = simulation.Compute(matches, preds, participants)
	return day, results, nil
}

// =============================================================================
// Matches
// =============================================================================

// matchSelectColumns is the canonical column list for SELECTing matches with
// their joined round. Kept as a const so ListMatches and GetMatch can't drift.
// Order MUST match the Scan target order in scanMatch.
const matchSelectColumns = `
	m.id, m.team_a, m.team_b, m.team_a_score, m.team_b_score,
	m.winner, m.status, m.scheduled_at,
	m.placeholder_a, m.placeholder_b, m.slot,
	r.id, r.stage, r.sort_order, r.name
`

// rowScanner accepts both *sql.Row and *sql.Rows so scanMatch works for
// QueryRowContext and QueryContext.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanMatch reads one row from a SELECT that uses matchSelectColumns. It does
// NOT set Match.Locked — that is a computed field, populated by ListMatches.
func scanMatch(s rowScanner, m *models.Match) error {
	return s.Scan(
		&m.ID, &m.TeamA, &m.TeamB, &m.TeamAScore, &m.TeamBScore,
		&m.Winner, &m.Status, &m.ScheduledAt,
		&m.PlaceholderA, &m.PlaceholderB, &m.Slot,
		&m.Round.ID, &m.Round.Stage, &m.Round.SortOrder, &m.Round.Name,
	)
}

// ListMatches returns every match joined with its round, sorted by
// round.sort_order ASC then match id ASC, with the computed Locked flag set
// on each.
//
// Locking is data-driven (see internal/locking): the lock schedule — each
// day's lock time and which date is the final, per-match day — is derived
// from the match set itself, so ListMatches is the single source of truth for
// Match.Locked.
func (db *DB) ListMatches(ctx context.Context) ([]models.Match, error) {
	q := `
		SELECT ` + matchSelectColumns + `
		FROM matches m
		JOIN rounds r ON r.id = m.round_id
		ORDER BY r.sort_order ASC, m.id ASC
	`
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.Match, 0)
	for rows.Next() {
		var m models.Match
		if err := scanMatch(rows, &m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sched := locking.BuildSchedule(out)
	now := time.Now()
	for i := range out {
		out[i].Locked = sched.IsLocked(out[i], now)
	}
	return out, nil
}

// GetMatch returns a single match by id with its round joined.
//
// NOTE: the returned match's Locked field is always false — a meaningful lock
// state needs the whole tournament (day windows, final-day detection), which
// GetMatch does not load. Use ListMatches when Locked matters; GetMatch exists
// for callers (the syncer) that don't care about it.
func (db *DB) GetMatch(ctx context.Context, id string) (*models.Match, error) {
	var m models.Match
	q := `
		SELECT ` + matchSelectColumns + `
		FROM matches m
		JOIN rounds r ON r.id = m.round_id
		WHERE m.id = ?
	`
	err := scanMatch(db.QueryRowContext(ctx, q, id), &m)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// UpsertMatch is used by the match-source poller. On first sight inserts the
// row; on subsequent syncs updates the mutable fields. Match id is the stable
// hash from spec § 5.4 (Liquipedia) or a (tournament, stage, label) hash
// (SheetSource), so it's never updated.
//
// placeholder_a / placeholder_b / slot ARE updated on conflict — SheetSource
// rewrites placeholder display text every sync, and once real teams resolve
// it writes the team names into team_a / team_b and clears the placeholders.
func (db *DB) UpsertMatch(ctx context.Context, m *models.Match, roundID int) error {
	const q = `
		INSERT INTO matches
			(id, round_id, team_a, team_b, team_a_score, team_b_score,
			 winner, status, scheduled_at, placeholder_a, placeholder_b, slot,
			 updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(id) DO UPDATE SET
			team_a         = excluded.team_a,
			team_b         = excluded.team_b,
			team_a_score   = excluded.team_a_score,
			team_b_score   = excluded.team_b_score,
			winner         = excluded.winner,
			status         = excluded.status,
			scheduled_at   = excluded.scheduled_at,
			placeholder_a  = excluded.placeholder_a,
			placeholder_b  = excluded.placeholder_b,
			slot           = excluded.slot,
			updated_at     = datetime('now')
	`
	_, err := db.ExecContext(ctx, q,
		m.ID, roundID, m.TeamA, m.TeamB, m.TeamAScore, m.TeamBScore,
		m.Winner, m.Status, m.ScheduledAt,
		m.PlaceholderA, m.PlaceholderB, m.Slot,
	)
	return err
}

// =============================================================================
// Rounds
// =============================================================================

// UpsertRound is used by the match-source poller. Inserts the round if absent
// and returns its id; if it already exists, refreshes sort_order in case the
// spec's numbering changed and returns the existing id.
func (db *DB) UpsertRound(ctx context.Context, tournamentID int, stage, name string, sortOrder int) (int, error) {
	if _, err := db.ExecContext(ctx, `
		INSERT INTO rounds (tournament_id, stage, sort_order, name)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(tournament_id, stage, name) DO UPDATE SET sort_order = excluded.sort_order
	`, tournamentID, stage, sortOrder, name); err != nil {
		return 0, err
	}

	var id int
	err := db.QueryRowContext(ctx, `
		SELECT id FROM rounds WHERE tournament_id = ? AND stage = ? AND name = ?
	`, tournamentID, stage, name).Scan(&id)
	return id, err
}

// =============================================================================
// Tournaments
// =============================================================================

// GetOrCreateActiveTournament returns the tournament for the given Liquipedia
// page, creating it if it doesn't yet exist. Called at startup to seed the
// active tournament row.
func (db *DB) GetOrCreateActiveTournament(ctx context.Context, liquipediaPage, name string) (*models.Tournament, error) {
	var (
		t        models.Tournament
		isActive int
	)
	err := db.QueryRowContext(ctx, `
		SELECT id, liquipedia_page, name, is_active, last_synced_at
		FROM tournaments
		WHERE liquipedia_page = ?
	`, liquipediaPage).Scan(&t.ID, &t.LiquipediaPage, &t.Name, &isActive, &t.LastSyncedAt)

	if errors.Is(err, sql.ErrNoRows) {
		result, err := db.ExecContext(ctx, `
			INSERT INTO tournaments (liquipedia_page, name, is_active)
			VALUES (?, ?, 1)
		`, liquipediaPage, name)
		if err != nil {
			return nil, err
		}
		id, err := result.LastInsertId()
		if err != nil {
			return nil, err
		}
		return &models.Tournament{
			ID: int(id), LiquipediaPage: liquipediaPage, Name: name, IsActive: true,
		}, nil
	}
	if err != nil {
		return nil, err
	}
	t.IsActive = isActive == 1
	return &t, nil
}

// UpdateLastSyncedAt records a successful sync. syncedAt should be RFC3339.
func (db *DB) UpdateLastSyncedAt(ctx context.Context, tournamentID int, syncedAt string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE tournaments SET last_synced_at = ? WHERE id = ?`,
		syncedAt, tournamentID,
	)
	return err
}

// =============================================================================
// Predictions
// =============================================================================

// SetPrediction upserts a prediction. Validates that the match exists and that
// its predictions are not locked (see checkPredictionWriteable). Returns
// ErrNotFound if either the participant or the match is missing, or
// ErrPredictionsLocked if the match's predictions have locked.
func (db *DB) SetPrediction(ctx context.Context, participantID, matchID, pick string) error {
	if err := db.checkPredictionWriteable(ctx, participantID, matchID); err != nil {
		return err
	}

	_, err := db.ExecContext(ctx, `
		INSERT INTO predictions (participant_id, match_id, pick)
		VALUES (?, ?, ?)
		ON CONFLICT(participant_id, match_id) DO UPDATE SET
			pick       = excluded.pick,
			updated_at = datetime('now')
	`, participantID, matchID, pick)
	return err
}

// DeletePrediction removes a prediction. Same validation rules as SetPrediction.
// Returns ErrNotFound if no prediction existed for that (participant, match) pair.
func (db *DB) DeletePrediction(ctx context.Context, participantID, matchID string) error {
	if err := db.checkPredictionWriteable(ctx, participantID, matchID); err != nil {
		return err
	}

	result, err := db.ExecContext(ctx,
		`DELETE FROM predictions WHERE participant_id = ? AND match_id = ?`,
		participantID, matchID,
	)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// checkPredictionWriteable validates that both the match and participant
// exist, and that the match's predictions are not locked.
//
// The lock decision is taken straight from ListMatches, which computes
// Match.Locked — so server-side enforcement and the Locked flag the frontend
// sees can never disagree.
func (db *DB) checkPredictionWriteable(ctx context.Context, participantID, matchID string) error {
	matches, err := db.ListMatches(ctx)
	if err != nil {
		return err
	}
	var match *models.Match
	for i := range matches {
		if matches[i].ID == matchID {
			match = &matches[i]
			break
		}
	}
	if match == nil {
		return ErrNotFound
	}
	if match.Locked {
		return ErrPredictionsLocked
	}

	var exists int
	err = db.QueryRowContext(ctx, `SELECT 1 FROM participants WHERE id = ?`, participantID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// =============================================================================
// Winner picks
// =============================================================================

// AddWinnerPick appends a tournament-winner pick to a participant's history.
// The history is append-only — the participant's current pick is the most
// recent row. The caller is responsible for auth and for validating teamName
// against the tournament's actual teams (see ListTeamNames).
func (db *DB) AddWinnerPick(ctx context.Context, participantID, teamName string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO winner_pick_history (participant_id, team_name) VALUES (?, ?)`,
		participantID, teamName,
	)
	return err
}

// ListTeamNames returns the distinct set of team names appearing in any match,
// sorted alphabetically. Populates the winner-pick dropdown and validates
// winner-pick submissions. Empty until the first sync has populated matches.
//
// Excludes placeholder rows (bracket matches whose teams haven't resolved):
// SheetSource writes empty strings into team_a / team_b on those rows and
// puts display text in placeholder_a / placeholder_b — neither belongs in
// the winner-pick dropdown.
func (db *DB) ListTeamNames(ctx context.Context) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT team_name FROM (
			SELECT team_a AS team_name FROM matches
			WHERE team_a != '' AND placeholder_a IS NULL
			UNION
			SELECT team_b AS team_name FROM matches
			WHERE team_b != '' AND placeholder_b IS NULL
		)
		ORDER BY team_name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// =============================================================================
// Sync status
// =============================================================================

// GetSyncStatus returns the last_synced_at timestamp from the active tournament.
// last_error is not persisted in this phase — it lives in the poller's memory and
// the API handler will fold it in at request time (Phase 4).
func (db *DB) GetSyncStatus(ctx context.Context) (lastSyncedAt *string, err error) {
	err = db.QueryRowContext(ctx, `
		SELECT last_synced_at FROM tournaments
		WHERE is_active = 1
		ORDER BY id DESC
		LIMIT 1
	`).Scan(&lastSyncedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return lastSyncedAt, err
}
