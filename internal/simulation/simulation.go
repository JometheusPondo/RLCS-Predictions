// Package simulation runs the best-case / worst-case standings projection the
// broadcast crew asked for: once a day's predictions are locked in, show each
// participant how far they could climb or fall by the end of that day.
//
// For a participant P, two scenarios are simulated over the current day's
// not-yet-finished matches:
//
//   - Best case:  every match P picked resolves the way P picked it.
//   - Worst case: every match P picked resolves the opposite way.
//
// Each scenario is a fully-determined set of results, so the WHOLE field is
// re-scored under it (every participant's locked picks resolved against the
// same outcomes) and the standings are rebuilt. P's projected position is read
// off that scenario leaderboard.
//
// Only the matches P actually picked are resolved in P's scenario — matches P
// sat out are left unplayed. A consequence worth knowing: on any match, two
// people who picked the same side score identically (the underdog bonus
// depends on the side and the locked field, not on who picked it), so in P's
// all-correct world P weakly out-scores the entire field on the day. P can
// therefore only climb or hold in the best case, and only fall or hold in the
// worst case — both deltas are always >= 0 in their stated direction.
//
// Reported numbers (both always >= 0):
//
//   - BestCaseDelta:  positions GAINED if every pick P made that day hits.
//   - WorstCaseDelta: positions LOST if every pick P made that day misses.
//
// A delta of 0 is meaningful: the participant is capped (already top/bottom of
// the board, or their slate is pure consensus so a good/bad day lifts or sinks
// everyone together). Wide deltas mean a contrarian, high-variance slate.
//
// v1 simplifications (intentional):
//   - "the day" is the earliest date that still has an unfinished match.
//   - The "wrong but went the distance" consolation point is not modeled, so
//     a missed pick scores 0 and the worst case is a clean floor.
package simulation

import (
	"sort"

	"github.com/jometheuspondo/rlcs-predictions/internal/models"
	"github.com/jometheuspondo/rlcs-predictions/internal/scoring"
)

// dateLen is the length of a YYYY-MM-DD date prefix.
const dateLen = len("2006-01-02")

// Result is one participant's projected day swing. Both deltas are >= 0:
// BestCaseDelta is a climb, WorstCaseDelta is a fall.
type Result struct {
	ParticipantID  string `json:"participant_id"`
	BestCaseDelta  int    `json:"best_case"`
	WorstCaseDelta int    `json:"worst_case"`
}

// Compute runs the projection for every real participant.
//
// Inputs: every match, every (non-admin) prediction row, and the participant
// list (used only for leaderboard membership and the display-name tiebreak —
// scores are recomputed here so the baseline and scenario standings always
// come from the same scoring pass). Returns the simulation day (YYYY-MM-DD, or
// "" when no unfinished matches remain) and one Result per participant.
// blast_admin is skipped — it never appears on the leaderboard.
func Compute(matches []models.Match, preds []scoring.PredictionRow, participants []models.Participant) (day string, results []Result) {
	day = currentDay(matches)

	real := make([]models.Participant, 0, len(participants))
	for _, p := range participants {
		if p.ID != models.AdminID {
			real = append(real, p)
		}
	}

	dayMatches := incompleteMatchesOn(matches, day)
	picksByParticipant := indexPicks(preds)

	// Baseline standings — the real, no-hypothetical scores.
	currentPos := positions(real, scoring.ComputeScores(matches, preds))

	results = make([]Result, 0, len(real))
	for _, p := range real {
		picks := picksByParticipant[p.ID]

		bestPos := positions(real, scenarioScores(matches, dayMatches, preds, picks, true))
		worstPos := positions(real, scenarioScores(matches, dayMatches, preds, picks, false))

		results = append(results, Result{
			ParticipantID:  p.ID,
			BestCaseDelta:  currentPos[p.ID] - bestPos[p.ID],
			WorstCaseDelta: worstPos[p.ID] - currentPos[p.ID],
		})
	}
	return day, results
}

// currentDay returns the earliest calendar date (YYYY-MM-DD) that still has an
// unfinished match — the day whose swing the projection describes. Returns ""
// when every match is completed.
func currentDay(matches []models.Match) string {
	earliest := ""
	for _, m := range matches {
		if m.Status == models.StatusCompleted {
			continue
		}
		d := matchDate(m)
		if d == "" {
			continue
		}
		if earliest == "" || d < earliest {
			earliest = d
		}
	}
	return earliest
}

// incompleteMatchesOn returns the not-yet-completed matches scheduled on day.
func incompleteMatchesOn(matches []models.Match, day string) []models.Match {
	if day == "" {
		return nil
	}
	out := make([]models.Match, 0)
	for _, m := range matches {
		if m.Status != models.StatusCompleted && matchDate(m) == day {
			out = append(out, m)
		}
	}
	return out
}

// matchDate extracts the YYYY-MM-DD date prefix from a match's ScheduledAt
// (RFC3339). Returns "" when the match has no scheduled time.
func matchDate(m models.Match) string {
	if m.ScheduledAt == nil || len(*m.ScheduledAt) < dateLen {
		return ""
	}
	return (*m.ScheduledAt)[:dateLen]
}

// indexPicks groups prediction rows into participant_id → (match_id → side).
func indexPicks(preds []scoring.PredictionRow) map[string]map[string]string {
	out := make(map[string]map[string]string)
	for _, p := range preds {
		byMatch := out[p.ParticipantID]
		if byMatch == nil {
			byMatch = make(map[string]string)
			out[p.ParticipantID] = byMatch
		}
		byMatch[p.MatchID] = p.Pick
	}
	return out
}

// scenarioScores re-scores the whole field for one participant's scenario.
// dayPicks is that participant's picks (match_id → side). best=true resolves
// each of their day matches their way; best=false resolves them the opposite
// way. Day matches the participant didn't pick are left unresolved.
func scenarioScores(allMatches, dayMatches []models.Match, preds []scoring.PredictionRow, dayPicks map[string]string, best bool) map[string]int {
	// Decide the hypothetical winner for each resolved day match.
	winners := make(map[string]string) // match_id → winning side
	for _, m := range dayMatches {
		pick, picked := dayPicks[m.ID]
		if !picked {
			continue
		}
		if best {
			winners[m.ID] = pick
		} else {
			winners[m.ID] = opposite(pick)
		}
	}

	// Copy the match slice and overlay the hypothetical results. models.Match
	// is a value type; copying then reassigning fields leaves the originals
	// untouched.
	modified := make([]models.Match, len(allMatches))
	copy(modified, allMatches)
	for i := range modified {
		side, resolved := winners[modified[i].ID]
		if !resolved {
			continue
		}
		w := side
		modified[i].Status = models.StatusCompleted
		modified[i].Winner = &w
		// Scores cleared: ComputeScores reads them only to detect a series
		// that "went the distance"; nil means "not the distance", so a missed
		// pick scores 0 — the intended v1 worst-case floor.
		modified[i].TeamAScore = nil
		modified[i].TeamBScore = nil
	}
	return scoring.ComputeScores(modified, preds)
}

// opposite returns the other side.
func opposite(pick string) string {
	if pick == models.PickA {
		return models.PickB
	}
	return models.PickA
}

// positions ranks participants by score (desc) then display name (asc) — the
// same ordering the real leaderboard uses — and returns participant_id →
// 1-based position. A participant absent from scores counts as 0.
func positions(participants []models.Participant, scores map[string]int) map[string]int {
	ordered := make([]models.Participant, len(participants))
	copy(ordered, participants)
	sort.SliceStable(ordered, func(i, j int) bool {
		si, sj := scores[ordered[i].ID], scores[ordered[j].ID]
		if si != sj {
			return si > sj
		}
		return ordered[i].DisplayName < ordered[j].DisplayName
	})
	pos := make(map[string]int, len(ordered))
	for i, p := range ordered {
		pos[p.ID] = i + 1
	}
	return pos
}
