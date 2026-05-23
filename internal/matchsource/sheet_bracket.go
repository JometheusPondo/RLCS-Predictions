package matchsource

import (
	"fmt"

	"github.com/jometheuspondo/rlcs-predictions/internal/models"
)

// Bracket Output sheet layout (verified 2026-05-15 against the live broadcast LOP):
//
//   Row 3: round headers in cols ["Round 1"@0, "Round 2"@?, "Quarterfinals"@?, ...]
//          (the header text isn't used by the parser — we use fixed column anchors)
//
// Round column blocks (label column → 5 cols wide: label, team_name, br_id, score, slot):
//   Round 1:        label@3,  team@4,  br@5,  score@6,  slot@7
//   Round 2:        label@9,  team@10, br@11, score@12, slot@13
//   Quarterfinals:  label@15, team@16, br@17, score@18, slot@19
//   Semifinals:     label@21, team@22, br@23, score@24, slot@25
//   Grand Finals:   label@27, team@28, br@29, score@30, slot@31
//
// Within each round column, each MATCH spans 2 rows:
//   top row: label cell populated + top team's name/br_id/score/slot
//   bot row: label cell BLANK + bottom team's name/br_id/score (slot omitted)
//
// Total: R1=4 matches (C,D,E,F), R2=4 (A,B,G,H), QF=2 (I,J), SF=2 (K,L),
//        GF=1 (M) = 13 bracket matches.
//
// Until a team resolves, its team_name carries placeholder text ("Group A
// First", "Winner of C", "Loser of A") and br_id is "#N/A" or echoes the
// placeholder string. isResolvedTeamCell() returns false for those; the
// match is emitted with TeamA/B="" and PlaceholderA/B set to the display text.

// bracketRound describes one round's column anchors and round metadata.
type bracketRound struct {
	colLabel int    // column index of the label cell
	roundName string // models.Round.Name to emit
	sortOrder int    // models.Round.SortOrder
}

// bracketRounds anchors each round to its column offset and round metadata.
// SortOrder for R1/R2 places them BEFORE Quarterfinals — the broadcast LOP
// runs them on Day 3 and Day 4 respectively, ahead of the Day 5 finals.
// Values picked to leave gaps above the group stage (max 300) and below
// SortOrderQuarters (1000).
var bracketRounds = []bracketRound{
	{colLabel: 3, roundName: "Bracket - Round 1", sortOrder: 800},
	{colLabel: 9, roundName: "Bracket - Round 2", sortOrder: 900},
	{colLabel: 15, roundName: "Quarterfinals", sortOrder: models.SortOrderQuarters},
	{colLabel: 21, roundName: "Semifinals", sortOrder: models.SortOrderSemifinals},
	{colLabel: 27, roundName: "Grand Finals", sortOrder: models.SortOrderFinal},
}

// parseBracketCSV walks every row of the bracket CSV. For each row, in each
// round column block, if the label cell is non-empty it marks a match-top;
// the bottom team is read from the next row.
func parseBracketCSV(rows [][]string, tournamentID int) ([]models.Match, error) {
	if len(rows) < 6 {
		return nil, fmt.Errorf("bracket csv too short: %d rows (need at least 6)", len(rows))
	}

	out := make([]models.Match, 0, 13)

	for _, br := range bracketRounds {
		// Walk every row; rows beyond the last are caught by cellAt's bounds check.
		for rowIdx := 0; rowIdx < len(rows); rowIdx++ {
			label := cellAt(rows, rowIdx, br.colLabel)
			if label == "" {
				continue
			}
			// Bottom team is on the next row, same column block.
			m, ok := parseBracketMatch(rows, br, rowIdx, label, tournamentID)
			if ok {
				out = append(out, m)
			}
		}
	}

	return out, nil
}

// parseBracketMatch reads a single bracket match. topRow has the label and
// the top team; topRow+1 has the bottom team (label-blank).
func parseBracketMatch(rows [][]string, br bracketRound, topRow int, label string, tournamentID int) (models.Match, bool) {
	cTeam := br.colLabel + 1
	cBrID := br.colLabel + 2
	cScore := br.colLabel + 3
	cSlot := br.colLabel + 4

	teamATop := cellAt(rows, topRow, cTeam)
	brIDTop := cellAt(rows, topRow, cBrID)
	scoreA := parseInt0(cellAt(rows, topRow, cScore))
	slotRaw := cellAt(rows, topRow, cSlot)

	botRow := topRow + 1
	teamBBot := cellAt(rows, botRow, cTeam)
	brIDBot := cellAt(rows, botRow, cBrID)
	scoreB := parseInt0(cellAt(rows, botRow, cScore))

	// Both sides empty is a layout glitch — skip.
	if teamATop == "" && teamBBot == "" {
		return models.Match{}, false
	}

	// Resolve each side independently. Primary check is br_id (the integer
	// helper column 1-16). The team-name cell is a fallback for the case
	// where the sheet's br_id formula hasn't resolved but the team-name
	// formula has: if the name doesn't match a known placeholder pattern,
	// trust it. team_a / team_b stay empty only for actual placeholder
	// sides; placeholder display text goes into PlaceholderA / PlaceholderB.
	var teamA, teamB string
	var placeholderA, placeholderB *string

	if isResolvedTeamCell(brIDTop) || !looksLikePlaceholderTeamName(teamATop) {
		teamA = normalizeTeamName(teamATop)
	} else {
		placeholderA = ptrStr(teamATop)
	}
	if isResolvedTeamCell(brIDBot) || !looksLikePlaceholderTeamName(teamBBot) {
		teamB = normalizeTeamName(teamBBot)
	} else {
		placeholderB = ptrStr(teamBBot)
	}

	// Status calc uses 0 for empty/unresolved scores — both placeholders =>
	// both 0 => upcoming, which is correct.
	status, winner := computeSheetStatus(scoreA, scoreB, models.StageBracket)

	var scoreAPtr, scoreBPtr *int
	if status != models.StatusUpcoming {
		a, b := scoreA, scoreB
		scoreAPtr = &a
		scoreBPtr = &b
	}

	m := models.Match{
		ID: sheetMatchID(tournamentID, models.StageBracket, label),
		Round: models.Round{
			Stage:     models.StageBracket,
			Name:      br.roundName,
			SortOrder: br.sortOrder,
		},
		TeamA:        teamA,
		TeamB:        teamB,
		TeamAScore:   scoreAPtr,
		TeamBScore:   scoreBPtr,
		Winner:       winner,
		Status:       status,
		ScheduledAt:  scheduledAtFromSlot(slotRaw),
		PlaceholderA: placeholderA,
		PlaceholderB: placeholderB,
		Slot:         slotPosFromSlot(slotRaw),
	}
	return m, true
}
