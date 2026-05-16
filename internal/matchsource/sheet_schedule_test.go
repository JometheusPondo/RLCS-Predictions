package matchsource

import (
	_ "embed"
	"testing"

	"github.com/jometheuspondo/rlcs-predictions/internal/models"
)

//go:embed testdata/schedule.csv
var testScheduleCSV string

func TestParseScheduleCSV_GroupTimes(t *testing.T) {
	rows := readCSVString(t, testScheduleCSV)
	startByID, err := parseScheduleCSV(rows, testTournamentID)
	if err != nil {
		t.Fatalf("parseScheduleCSV: %v", err)
	}

	// All 24 group matches have published CET start times; no bracket row
	// does, and the SHOWMATCH row is skipped — so exactly 24 entries.
	if got := len(startByID); got != 24 {
		t.Errorf("schedule entry count: got %d, want 24", got)
	}

	// Match D — "Day 1 1A", a Group row — starts 2:00 PM CEST = 12:00 UTC.
	dID := sheetMatchID(testTournamentID, models.StageGroup, "D")
	if got := startByID[dID]; got != "2026-05-20T12:00:00Z" {
		t.Errorf("match D start: got %q, want 2026-05-20T12:00:00Z", got)
	}

	// Match I — "Day 1 2A" — starts 3:00 PM CEST = 13:00 UTC.
	iID := sheetMatchID(testTournamentID, models.StageGroup, "I")
	if got := startByID[iID]; got != "2026-05-20T13:00:00Z" {
		t.Errorf("match I start: got %q, want 2026-05-20T13:00:00Z", got)
	}

	// Match X — "Day 2 6B" — starts 7:00 PM CEST = 17:00 UTC on day 2.
	xID := sheetMatchID(testTournamentID, models.StageGroup, "X")
	if got := startByID[xID]; got != "2026-05-21T17:00:00Z" {
		t.Errorf("match X start: got %q, want 2026-05-21T17:00:00Z", got)
	}
}

func TestParseScheduleCSV_BracketRowsHaveNoTime(t *testing.T) {
	rows := readCSVString(t, testScheduleCSV)
	startByID, err := parseScheduleCSV(rows, testTournamentID)
	if err != nil {
		t.Fatalf("parseScheduleCSV: %v", err)
	}

	// Bracket rows have a blank Scheduled Start cell in the current sheet —
	// they must NOT appear in the result (the match keeps day granularity).
	for _, label := range []string{"A", "I", "M"} {
		id := sheetMatchID(testTournamentID, models.StageBracket, label)
		if _, ok := startByID[id]; ok {
			t.Errorf("bracket match %q should have no schedule entry (start blank)", label)
		}
	}
}

func TestScheduleStage(t *testing.T) {
	cases := map[string]string{
		"Group B Round 1":     models.StageGroup,
		"Bracket Lower R1":    models.StageBracket,
		"Bracket Grand Final": models.StageBracket,
		"SHOWMATCH":           "",
		"":                    "",
	}
	for in, want := range cases {
		if got := scheduleStage(in); got != want {
			t.Errorf("scheduleStage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCetClockToUTC(t *testing.T) {
	cases := []struct {
		date, clock, want string
	}{
		{"2026-05-20T00:00:00Z", "2:00 PM", "2026-05-20T12:00:00Z"},
		{"2026-05-20T00:00:00Z", "7:00 PM", "2026-05-20T17:00:00Z"},
		{"2026-05-22T00:00:00Z", "3:00 PM", "2026-05-22T13:00:00Z"},
	}
	for _, tc := range cases {
		got, err := cetClockToUTC(tc.date, tc.clock)
		if err != nil {
			t.Errorf("cetClockToUTC(%q,%q): %v", tc.date, tc.clock, err)
			continue
		}
		if got != tc.want {
			t.Errorf("cetClockToUTC(%q,%q) = %q, want %q", tc.date, tc.clock, got, tc.want)
		}
	}
}
