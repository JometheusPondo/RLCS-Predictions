package matchsource

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/jometheuspondo/rlcs-predictions/internal/models"
)

const WorldsSpreadsheetID = "1BuyYGV59e_fR8fUkdgIRiFheBUaokulpj7pRtE39f-c"

type WorldsSource struct {
	sheet *SheetSource
}

func NewWorldsSource(tournamentID int, spreadsheetID string, logger *slog.Logger) (*WorldsSource, error) {
	sheet, err := NewSheetSource(SheetSourceOptions{
		SpreadsheetID: spreadsheetID, TournamentID: tournamentID,
		GroupsGID: "10266191", BracketGID: "1847119516", ScheduleGID: "1663218581", Logger: logger,
	})
	if err != nil {
		return nil, err
	}
	return &WorldsSource{sheet: sheet}, nil
}

// FetchMatches validates every event tab before returning a complete Worlds update.
func (s *WorldsSource) FetchMatches(ctx context.Context) ([]models.Match, error) {
	gids := []string{"1174796019", s.sheet.groupsGID, s.sheet.bracketGID, "381597648", s.sheet.scheduleGID}
	tabs := make([][][]string, len(gids))
	for i, gid := range gids {
		rows, err := s.sheet.fetchCSV(ctx, gid)
		if err != nil {
			return nil, fmt.Errorf("Worlds tab %s: %w", gid, err)
		}
		tabs[i] = rows
	}
	return parseWorlds(tabs[0], tabs[1], tabs[2], tabs[3], tabs[4], s.sheet.tournamentID)
}

func parseWorlds(playIn, groups, bracket, sideEvents, schedule [][]string, tournamentID int) ([]models.Match, error) {
	matches := make([]models.Match, 0, 53)
	for row := range playIn {
		for _, col := range []int{3, 9} {
			label := strings.TrimSpace(cellAt(playIn, row, col))
			if !containsLabel("AA AB AC AD AE AF AI AJ AK AL", label) {
				continue
			}
			round := "3v3 Play-In — Upper Quarterfinals"
			order := 10
			if row >= 23 {
				round = "3v3 Play-In — Lower Quarterfinals"
				order = 30
			}
			if col == 9 {
				round = strings.ReplaceAll(round, "Quarterfinals", "Semifinals")
				order += 10
			}
			m, err := worldsCellMatch(playIn, row, col, tournamentID, models.StagePlayIn, round, order, 5)
			if err != nil {
				return nil, err
			}
			matches = append(matches, m)
		}
	}

	for _, group := range groupBlocks {
		for _, round := range roundColumns {
			for _, offset := range []int{0, 2} {
				row := group.dataStartRow + offset
				label := strings.TrimSpace(cellAt(groups, row, round.colStart))
				if len(label) != 1 || label[0] < 'A' || label[0] > 'X' {
					return nil, fmt.Errorf("Worlds group layout: invalid match label %q", label)
				}
				name := fmt.Sprintf("3v3 Group %s — Round %d", group.letter, round.roundNum)
				m, err := worldsCellMatch(groups, row, round.colStart, tournamentID, models.StageGroup, name, round.roundNum*100, 5)
				if err != nil {
					return nil, err
				}
				matches = append(matches, m)
			}
		}
	}

	for _, round := range bracketRounds {
		for row := range bracket {
			label := strings.TrimSpace(cellAt(bracket, row, round.colLabel))
			if !containsLabel("A B C D E F G H I J K L M", label) {
				continue
			}
			m, err := worldsCellMatch(bracket, row, round.colLabel, tournamentID, models.StageBracket, "3v3 "+round.roundName, round.sortOrder, 7)
			if err != nil {
				return nil, err
			}
			matches = append(matches, m)
		}
	}

	stage := ""
	for row := range sideEvents {
		if heading := strings.TrimSpace(cellAt(sideEvents, row, 2)); heading == models.Stage1v1 || heading == models.Stage2v2 {
			stage = heading
		}
		if stage == "" {
			continue
		}
		for _, col := range []int{3, 9} {
			label := strings.TrimSpace(cellAt(sideEvents, row, col))
			if !containsLabel("A B C", label) {
				continue
			}
			name, order := stage+" Semifinals", 1300
			if col == 9 {
				name, order = stage+" Grand Final", 1400
			}
			m, err := worldsCellMatch(sideEvents, row, col, tournamentID, stage, name, order, 7)
			if err != nil {
				return nil, err
			}
			matches = append(matches, m)
		}
	}

	counts := make(map[string]int)
	seen := make(map[string]bool)
	for _, m := range matches {
		if seen[m.ID] {
			return nil, fmt.Errorf("Worlds duplicate match %s", m.ID)
		}
		seen[m.ID] = true
		counts[m.Round.Stage]++
	}
	for stage, expected := range map[string]int{models.StagePlayIn: 10, models.StageGroup: 24, models.StageBracket: 13, models.Stage1v1: 3, models.Stage2v2: 3} {
		if counts[stage] != expected {
			return nil, fmt.Errorf("Worlds %s: found %d matches, expected %d", stage, counts[stage], expected)
		}
	}
	if err := overlayWorldsSchedule(matches, schedule, tournamentID); err != nil {
		return nil, err
	}
	return matches, nil
}

func containsLabel(labels, label string) bool {
	if label == "" {
		return false
	}
	for _, candidate := range strings.Fields(labels) {
		if candidate == label {
			return true
		}
	}
	return false
}

func worldsCellMatch(rows [][]string, row, col, tournamentID int, stage, round string, order, bestOf int) (models.Match, error) {
	label := strings.TrimSpace(cellAt(rows, row, col))
	slot := strings.TrimSpace(cellAt(rows, row, col+4))
	date, err := worldsDate(slot)
	if err != nil {
		return models.Match{}, fmt.Errorf("%s match %s: %w", stage, label, err)
	}

	a, pa := worldsTeam(cellAt(rows, row, col+1))
	b, pb := worldsTeam(cellAt(rows, row+1, col+1))
	scoreA, err := worldsScore(cellAt(rows, row, col+3))
	if err != nil {
		return models.Match{}, err
	}
	scoreB, err := worldsScore(cellAt(rows, row+1, col+3))
	if err != nil {
		return models.Match{}, err
	}
	threshold := bestOf/2 + 1
	if scoreA > threshold || scoreB > threshold || (scoreA == threshold && scoreB == threshold) {
		return models.Match{}, fmt.Errorf("invalid score %d-%d for Bo%d", scoreA, scoreB, bestOf)
	}
	if (pa != nil || pb != nil) && (scoreA > 0 || scoreB > 0) {
		return models.Match{}, fmt.Errorf("%s %s has scores but unresolved teams", stage, label)
	}

	midnight := date + "T00:00:00Z"
	m := models.Match{
		ID:    sheetMatchID(tournamentID, stage, label),
		Round: models.Round{Stage: stage, Name: round, SortOrder: order},
		TeamA: a, TeamB: b, PlaceholderA: pa, PlaceholderB: pb,
		BestOf: bestOf, EventDate: &date, ScheduledAt: &midnight,
		Slot: slotPosFromSlot(slot), Status: models.StatusUpcoming,
	}
	if scoreA > 0 || scoreB > 0 {
		m.TeamAScore, m.TeamBScore = &scoreA, &scoreB
		m.Status = models.StatusLive
		if scoreA == threshold {
			m.Status = models.StatusCompleted
			m.Winner = ptrStr(models.PickA)
		}
		if scoreB == threshold {
			m.Status = models.StatusCompleted
			m.Winner = ptrStr(models.PickB)
		}
	}
	return m, nil
}

func worldsTeam(name string) (string, *string) {
	name = strings.TrimSpace(name)
	if name == "" || strings.HasPrefix(name, "#") || name == "-" || strings.EqualFold(name, "TBD") {
		return "", ptrStr("Team to be confirmed")
	}
	if looksLikePlaceholderTeamName(name) || strings.HasPrefix(name, "GSL Play-In ") {
		return "", ptrStr(name)
	}
	return normalizeTeamName(name), nil
}

func worldsScore(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	score, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || score < 0 {
		return 0, fmt.Errorf("invalid Worlds score %q", raw)
	}
	return score, nil
}

func worldsDate(slot string) (string, error) {
	day, _ := parseSlotString(slot)
	n, err := strconv.Atoi(strings.TrimPrefix(day, "Day "))
	if err != nil || n < 1 || n > 6 {
		return "", fmt.Errorf("invalid Worlds day %q", slot)
	}
	return time.Date(2026, time.September, 14+n, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), nil
}

// overlayWorldsSchedule reads the rightmost published schedule block, in venue time.
func overlayWorldsSchedule(matches []models.Match, rows [][]string, tournamentID int) error {
	if len(rows) == 0 {
		return fmt.Errorf("Worlds schedule is empty")
	}
	base, startCol := -1, -1
	for col, header := range rows[0] {
		if strings.TrimSpace(header) == "Match Letter" {
			base = col
		}
		if strings.TrimSpace(header) == "Scheduled Start (CT)" {
			startCol = col
		}
	}
	if base < 0 || startCol != base+9 {
		return fmt.Errorf("Worlds schedule headers changed")
	}
	zone, err := time.LoadLocation("America/Chicago")
	if err != nil {
		return err
	}

	byID := make(map[string]*models.Match)
	for i := range matches {
		byID[matches[i].ID] = &matches[i]
	}
	timedDays := make(map[string]bool)
	for _, row := range rows[1:] {
		label, stageText := cellInRow(row, base), cellInRow(row, base+2)
		stage := worldsScheduleStage(stageText)
		if stage == "" || strings.TrimSpace(label) == "" {
			continue
		}
		m := byID[sheetMatchID(tournamentID, stage, label)]
		if m == nil {
			return fmt.Errorf("schedule references unknown %s match %s", stage, label)
		}
		date, err := worldsDate(cellInRow(row, base+1))
		if err != nil {
			return err
		}
		if date != *m.EventDate {
			return fmt.Errorf("schedule day disagrees for %s %s", stage, label)
		}

		raw := strings.TrimSpace(cellInRow(row, startCol))
		if raw == "" {
			continue
		}
		var start time.Time
		for _, layout := range []string{"2006-01-02 3:04 PM", "2006-01-02 3:04:05 PM"} {
			start, err = time.ParseInLocation(layout, date+" "+strings.ToUpper(raw), zone)
			if err == nil {
				break
			}
		}
		if err != nil {
			return fmt.Errorf("invalid scheduled start %q: %w", raw, err)
		}
		stamp := start.UTC().Format(time.RFC3339)
		m.ScheduledAt = &stamp
		timedDays[date] = true
	}
	for _, m := range matches {
		if !timedDays[*m.EventDate] {
			return fmt.Errorf("no published start time for %s", *m.EventDate)
		}
	}
	return nil
}

func worldsScheduleStage(text string) string {
	switch {
	case strings.HasPrefix(text, "Play-In"):
		return models.StagePlayIn
	case strings.HasPrefix(text, "Group"):
		return models.StageGroup
	case strings.HasPrefix(text, "Bracket"):
		return models.StageBracket
	case strings.HasPrefix(text, "1v1"):
		return models.Stage1v1
	case strings.HasPrefix(text, "2v2"):
		return models.Stage2v2
	default:
		return ""
	}
}
