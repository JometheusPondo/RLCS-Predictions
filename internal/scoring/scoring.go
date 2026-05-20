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
// "Underdog pick" means AT MOST UnderdogMaxHumanPicks humans picked that same
// side on that same match. Because predictions lock when a match starts, the
// pick distribution is frozen by the time a match completes — so the underdog
// determination is stable, and computing scores on every leaderboard read is
// deterministic.
//
// The blast_admin account is not a participant; the DB layer excludes its
// predictions before they reach this package, so they neither earn points nor
// affect anyone's underdog count.
//
// Benchmark rows (PredictionRow.Benchmark == true — the manually-entered "The
// Coin" and "Chat" accounts) are a softer exclusion: they ARE scored like any
// other participant, but they do NOT contribute to the underdog tally, so they
// cannot tip a human's pick into or out of underdog territory. The underdog
// test — at most UnderdogMaxHumanPicks humans on the side — is the same for
// every row, so a benchmark earns the underdog bonus on exactly the sides a
// human would.
package scoring

import "github.com/jometheuspondo/rlcs-predictions/internal/models"

// Point values. Named so the rules read as prose at the call site.
const (
	PointsCorrectUnderdog = 4
	PointsCorrect         = 2
	PointsWentDistance    = 1
	PointsWrong           = 0
)

// UnderdogMaxHumanPicks is the underdog cutoff: a pick is an underdog pick when
// AT MOST this many humans picked the same side on the same match. Benchmark
// accounts are not humans and never count toward this total (see the package
// doc); they are still scored against it. At 4, a side chosen by 1–4 humans is
// an underdog side; 5 or more humans and it is not.
const UnderdogMaxHumanPicks = 4

// UnderdogSide reports which side of a match is "the underdog" for display
// purposes, given the human pick counts on side A and side B. It is a pure
// helper, separate from ComputeScores: the leaderboard scores every pick, but
// a match card highlights at most ONE underdog team.
//
// The underdog is the side with STRICTLY FEWER human picks, returned only when
// that side is at or below UnderdogMaxHumanPicks — i.e. only when it actually
// earns the underdog bonus. So:
//
//   - a clear minority side at/under the cutoff  → that side, ok=true
//   - the two sides tied                         → ok=false (no single minority)
//   - the smaller side still above the cutoff    → ok=false (no underdog)
//
// It therefore never reports two underdog sides, and reports none when the
// match has no bonus-eligible minority. Benchmark and admin picks must already
// be excluded from picksA / picksB by the caller — same humans-only basis the
// scoring tally uses.
func UnderdogSide(picksA, picksB int) (side string, ok bool) {
	if picksA == picksB {
		return "", false
	}
	minSide, minCount := models.PickA, picksA
	if picksB < picksA {
		minSide, minCount = models.PickB, picksB
	}
	if minCount > UnderdogMaxHumanPicks {
		return "", false
	}
	return minSide, true
}

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

	// Benchmark marks a non-standard account (e.g. "The Coin", "Chat") that is
	// scored normally but is NOT counted toward the cross-participant underdog
	// tally. The zero value (false) is a normal participant, so existing
	// PredictionRow literals need no change.
	Benchmark bool
}

// ParticipantStats is one participant's scoring output: total points and the
// count of correctly-predicted matches.
type ParticipantStats struct {
	Score   int
	Correct int
}

// ComputeStats returns per-participant score and correct-pick count from one
// pass over the predictions. Only completed matches contribute. A pick is
// correct when it matches the winner; bonuses don't affect the correct count,
// and a wrong-but-distance pick still counts as wrong.
func ComputeStats(matches []models.Match, preds []PredictionRow) map[string]ParticipantStats {
	byID := make(map[string]models.Match, len(matches))
	for _, m := range matches {
		byID[m.ID] = m
	}

	// Pick distribution: how many predictions chose each (match, side).
	// Key is matchID + "|" + pick. Benchmark rows are skipped — they must not
	// inflate the tally that decides whether a human's pick is an underdog.
	pickCount := make(map[string]int)
	for _, p := range preds {
		if p.Benchmark {
			continue
		}
		pickCount[p.MatchID+"|"+p.Pick]++
	}

	out := make(map[string]ParticipantStats)
	for _, p := range preds {
		m, ok := byID[p.MatchID]
		if !ok || m.Status != models.StatusCompleted {
			continue
		}
		s := out[p.ParticipantID]
		s.Score += pointsFor(m, p, pickCount)
		if m.Winner != nil && *m.Winner == p.Pick {
			s.Correct++
		}
		out[p.ParticipantID] = s
	}
	return out
}

// ComputeScores returns participant_id to total score. Thin wrapper over
// ComputeStats; kept for callers and tests that only need the score map.
func ComputeScores(matches []models.Match, preds []PredictionRow) map[string]int {
	stats := ComputeStats(matches, preds)
	scores := make(map[string]int, len(stats))
	for id, s := range stats {
		scores[id] = s.Score
	}
	return scores
}

// pointsFor scores a single prediction against its (completed) match.
func pointsFor(m models.Match, p PredictionRow, pickCount map[string]int) int {
	correct := m.Winner != nil && *m.Winner == p.Pick

	if correct {
		// pickCount holds only human (non-benchmark) picks. A pick is an
		// underdog pick when at most UnderdogMaxHumanPicks humans chose that
		// side — the same test for every row, human or benchmark.
		if pickCount[p.MatchID+"|"+p.Pick] <= UnderdogMaxHumanPicks {
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
