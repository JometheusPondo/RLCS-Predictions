package matchsource

import (
	"fmt"

	"github.com/jometheuspondo/rlcs-predictions/internal/models"
)

// Groups Output sheet layout (verified 2026-05-15 against the live broadcast LOP):
//
//   Row 3:  Group A header (col 7 = "GROUP A")
//   Row 5:  Group A standings header ("Place", "Team", ..., "Round 1" @col 10, "Round 2" @col 16, "Round 3" @col 22)
//   Row 6..9:  Group A 4 standings rows
//
//   Row 11: Group B header
//   Row 13: Group B standings header
//   Row 14..17: Group B data
//
//   Row 19: Group C header / 21: standings hdr / 22..25: data
//   Row 27: Group D header / 29: standings hdr / 30..33: data
//
// Within each 6-column round block (R1 cols 10..15, R2 cols 16..21, R3 cols 22..27):
//   offset 0: match label (single letter, A..X across all 24 group matches)
//   offset 1: team name (full English)
//   offset 2: br_id (1..16)
//   offset 3: score (integer)
//   offset 4: slot string ("Day 1 2A")
//   offset 5: spacer
//
// Standings rows arrive in groups of four; within each group's 4 rows in one
// round column block:
//   rows 0+1 = match 1 (top + bottom team)
//   rows 2+3 = match 2 (top + bottom team)
// The label cell is populated only on the TOP row of each pair; the bottom
// row shows the team's own results-row entry (team_name, br_id, score) with
// blank label.
//
// 4 groups × 3 rounds × 2 matches = 24 group matches total.

// groupBlock describes one group's layout in the CSV.
type groupBlock struct {
	letter      string // "A", "B", "C", "D"
	dataStartRow int   // first standings data row (0-indexed)
}

// roundColumn describes one round's column layout within a group block.
type roundColumn struct {
	roundNum int    // 1, 2, 3
	colStart int    // first column of the 6-col block
	roundName string // "Group Stage - Round 1"
}

// groupBlocks and roundColumns are computed once at parse time from the
// verified layout. The standings data rows are at fixed offsets after the
// section header — if Google Sheets re-flows the layout, this will fail
// loudly rather than silently mis-attribute matches.
var groupBlocks = []groupBlock{
	{letter: "A", dataStartRow: 6},
	{letter: "B", dataStartRow: 14},
	{letter: "C", dataStartRow: 22},
	{letter: "D", dataStartRow: 30},
}

var roundColumns = []roundColumn{
	{roundNum: 1, colStart: 10, roundName: "Group Stage - Round 1"},
	{roundNum: 2, colStart: 16, roundName: "Group Stage - Round 2"},
	{roundNum: 3, colStart: 22, roundName: "Group Stage - Round 3"},
}

// parseGroupsCSV walks the four group blocks and emits all 24 group matches.
//
// Round metadata is embedded into every match (Round.SortOrder follows the
// SortOrderGroupStep convention: round_num * 100). The syncer will derive
// unique rounds from the match list before upserting.
func parseGroupsCSV(rows [][]string, tournamentID int) ([]models.Match, error) {
	if len(rows) < 34 {
		return nil, fmt.Errorf("groups csv too short: %d rows (need at least 34)", len(rows))
	}

	out := make([]models.Match, 0, 24)

	for _, gb := range groupBlocks {
		for _, rc := range roundColumns {
			// Match 1: top row at dataStartRow, bottom row at dataStartRow+1
			m1, ok := parseGroupMatch(rows, gb, rc, 0, tournamentID)
			if ok {
				out = append(out, m1)
			}
			// Match 2: top row at dataStartRow+2, bottom row at dataStartRow+3
			m2, ok := parseGroupMatch(rows, gb, rc, 2, tournamentID)
			if ok {
				out = append(out, m2)
			}
		}
	}

	return out, nil
}

// parseGroupMatch reads one match from the standings grid. matchRowOffset is
// 0 for the first match in a group/round block, 2 for the second. Returns
// ok=false if the label cell is empty (defensive — every group match in the
// LOP has a label).
func parseGroupMatch(rows [][]string, gb groupBlock, rc roundColumn, matchRowOffset int, tournamentID int) (models.Match, bool) {
	topRow := gb.dataStartRow + matchRowOffset
	botRow := topRow + 1

	label := cellAt(rows, topRow, rc.colStart)
	if label == "" {
		return models.Match{}, false
	}

	teamATop := cellAt(rows, topRow, rc.colStart+1)
	brIDTop := cellAt(rows, topRow, rc.colStart+2)
	scoreA := parseInt0(cellAt(rows, topRow, rc.colStart+3))
	slotRaw := cellAt(rows, topRow, rc.colStart+4)

	teamBBot := cellAt(rows, botRow, rc.colStart+1)
	brIDBot := cellAt(rows, botRow, rc.colStart+2)
	scoreB := parseInt0(cellAt(rows, botRow, rc.colStart+3))

	// Group rows always carry real br_ids (1..16); if either is bogus, log
	// would be excessive — just skip the match. Defensive against future
	// sheet edits that might empty a cell.
	if !isResolvedTeamCell(brIDTop) || !isResolvedTeamCell(brIDBot) {
		return models.Match{}, false
	}

	teamA := normalizeTeamName(teamATop)
	teamB := normalizeTeamName(teamBBot)

	status, winner := computeSheetStatus(scoreA, scoreB, models.StageGroup)

	var scoreAPtr, scoreBPtr *int
	// Match-source contract: nil scores mean "not played yet". Sheet stores
	// 0 by default; mirror Liquipedia by emitting nil for the 0-0 upcoming
	// state and concrete pointers once play starts.
	if status != models.StatusUpcoming {
		a, b := scoreA, scoreB
		scoreAPtr = &a
		scoreBPtr = &b
	}

	m := models.Match{
		ID: sheetMatchID(tournamentID, models.StageGroup, label),
		Round: models.Round{
			Stage:     models.StageGroup,
			Name:      rc.roundName,
			SortOrder: rc.roundNum * models.SortOrderGroupStep,
		},
		TeamA:       teamA,
		TeamB:       teamB,
		TeamAScore:  scoreAPtr,
		TeamBScore:  scoreBPtr,
		Winner:      winner,
		Status:      status,
		ScheduledAt: scheduledAtFromSlot(slotRaw),
		Slot:        slotPosFromSlot(slotRaw),
		// Group rows never carry placeholders — teams are seeded at draw.
	}
	return m, true
}
