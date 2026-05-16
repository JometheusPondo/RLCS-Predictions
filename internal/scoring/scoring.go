// Package scoring computes participant scores from their predictions on
// completed matches. It is a pure package — no DB, no IO — so the rules are
// trivially unit-testable.
//
// Point values for one prediction on a completed match:
//
//	correct, underdog pick          → 4
//	correct                         → 2
//	incorrect, series went distance → 1
//	incorrect, did not go distance  → 0
//
// "Went the distance" means the series used every game: a Bo5 group-stage
// match at 3-2 (5 games), a Bo7 bracket match at 4-3 (7 games).
//
// "Underdog pick" means STRICTLY FEWER than UnderdogMaxOthers *other*
// participants picked that same side on that same match. Because predictions
// lock when a match starts, the pick distribution is frozen by the time a
// match completes — so the underdog determination is stable, and computing
// scores on every leaderboard read is deterministic.
//
// The blast_admin account is not a participant; the DB layer excludes its
// predictions before they reach this package, so they neither earn points nor
// affect anyone's underdog count.
package scoring

import "github.com/jometheuspondo/rlcs-predictions/internal/models"

// Point values. Named so the rules read as prose at the call site.
const (
	PointsCorrectUnderdog = 4
	PointsCorrect         = 2
	PointsWentDistance    = 1
	PointsWrong           = 0
)

// UnderdogMaxOthers is the underdog cutoff: a pick is an underdog pick when
// STRICTLY FEWER than this many OTHER participants picked the same side on the
// same match. At 5, a pick shared with 0–4 others is an underdog; sharing it
// with 5 or more others is not.
const UnderdogMaxOthers = 5

// gamesToWin is the wins needed to take a series in each stage: group stage is
// Bo5 (first to 3), bracket is Bo7 (first to 4).
func gamesToWin(stage string) int {
	if stage == models.StageBracket {
		return 4
	}
	return 3
}

// PredictionRow is one participant's pick on one match — the input unit for
// scoring. The DB layer produces these, already filtered to exclude the
// blast_admin account.
type PredictionRow struct {
	ParticipantID string
	MatchID       string
	Pick          string // models.PickA / models.PickB
}

// ComputeScores returns participant_id → total score, given every match and
// every (real) participant's predictions. Only completed matches contribute;
// a prediction whose match isn't in `matches` is ignored.
func ComputeScores(matches []models.Match, preds []PredictionRow) map[string]int {
	byID := make(map[string]models.Match, len(matches))
	for _, m := range matches {
		byID[m.ID] = m
	}

	// Pick distribution: how many predictions chose each (match, side).
	// Key is matchID + "|" + pick.
	pickCount := make(map[string]int)
	for _, p := range preds {
		pickCount[p.MatchID+"|"+p.Pick]++
	}

	scores := make(map[string]int)
	for _, p := range preds {
		m, ok := byID[p.MatchID]
		if !ok || m.Status != models.StatusCompleted {
			continue
		}
		scores[p.ParticipantID] += pointsFor(m, p, pickCount)
	}
	return scores
}

// pointsFor scores a single prediction against its (completed) match.
func pointsFor(m models.Match, p PredictionRow, pickCount map[string]int) int {
	correct := m.Winner != nil && *m.Winner == p.Pick

	if correct {
		others := pickCount[p.MatchID+"|"+p.Pick] - 1 // minus self
		if others < UnderdogMaxOthers {
			return PointsCorrectUnderdog
		}
		return PointsCorrect
	}

	if wentTheDistance(m) {
		return PointsWentDistance
	}
	return PointsWrong
}

// wentTheDistance reports whether the series used every game — total games
// equals 2*gamesToWin-1 (5 for a Bo5, 7 for a Bo7). Nil scores count as 0.
func wentTheDistance(m models.Match) bool {
	a, b := 0, 0
	if m.TeamAScore != nil {
		a = *m.TeamAScore
	}
	if m.TeamBScore != nil {
		b = *m.TeamBScore
	}
	return a+b == 2*gamesToWin(m.Round.Stage)-1
}
