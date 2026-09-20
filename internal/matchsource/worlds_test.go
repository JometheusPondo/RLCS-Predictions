package matchsource

import (
	"os"
	"strings"
	"testing"

	"github.com/jometheuspondo/rlcs-predictions/internal/models"
)

func worldsFixtures(t *testing.T) [][][]string {
	t.Helper()
	tabs := make([][][]string, 0, 5)
	for _, name := range []string{"playin", "groups", "bracket", "side_events", "schedule"} {
		data, err := os.ReadFile("testdata/worlds_" + name + ".csv")
		if err != nil {
			t.Fatal(err)
		}
		tabs = append(tabs, readCSVString(t, string(data)))
	}
	return tabs
}

func parseWorldsFixtures(t *testing.T, tabs [][][]string, id int) []models.Match {
	t.Helper()
	matches, err := parseWorlds(tabs[0], tabs[1], tabs[2], tabs[3], tabs[4], id)
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

func TestWorldsAllFormatsAndVenueDates(t *testing.T) {
	matches := parseWorldsFixtures(t, worldsFixtures(t), 2)
	if len(matches) != 53 {
		t.Fatalf("got %d matches, want 53", len(matches))
	}
	counts := make(map[string]int)
	days := make(map[string]int)
	byID := make(map[string]models.Match)
	for _, m := range matches {
		counts[m.Round.Stage]++
		days[*m.EventDate]++
		byID[m.ID] = m
		if m.Status != models.StatusUpcoming {
			t.Errorf("unexpected status: %+v", m)
		}
		if strings.Contains(m.TeamA+m.TeamB, "#") {
			t.Error("formula error leaked into team names")
		}
	}
	if len(byID) != 53 || len(days) != 6 {
		t.Fatalf("identity/day collision: %d IDs, %d days", len(byID), len(days))
	}
	for stage, count := range map[string]int{"play_in": 10, "group": 24, "bracket": 13, "1v1": 3, "2v2": 3} {
		if counts[stage] != count {
			t.Errorf("%s: got %d", stage, counts[stage])
		}
	}

	first := byID[sheetMatchID(2, models.StagePlayIn, "AA")]
	if *first.ScheduledAt != "2026-09-15T16:00:00Z" || first.BestOf != 5 {
		t.Fatalf("wrong CDT conversion or series: %+v", first)
	}
	highID := byID[sheetMatchID(2, models.StageGroup, "I")]
	if highID.TeamA != "Karmine Corp" {
		t.Errorf("team id 20 was discarded: %+v", highID)
	}
	singles := byID[sheetMatchID(2, models.Stage1v1, "A")]
	if singles.TeamA != "kv1" || singles.BestOf != 7 || *singles.EventDate != "2026-09-16" {
		t.Errorf("wrong 1v1 match: %+v", singles)
	}
	final := byID[sheetMatchID(2, models.StageBracket, "M")]
	if *final.EventDate != "2026-09-20" || *final.ScheduledAt != "2026-09-20T21:00:00Z" {
		t.Errorf("wrong final: %+v", final)
	}
}

func TestWorldsIdentitySurvivesResolutionAndSeparatesEvents(t *testing.T) {
	tabs := worldsFixtures(t)
	before := parseWorldsFixtures(t, tabs, 2)
	tabs[1][6][11], tabs[1][7][11] = "Karmine Corp", "Team Falcons"
	after := parseWorldsFixtures(t, tabs, 2)
	other := parseWorldsFixtures(t, tabs, 3)
	for i := range before {
		if before[i].ID != after[i].ID {
			t.Fatal("team resolution changed a match ID")
		}
		if before[i].ID == other[i].ID {
			t.Fatal("events share a match ID")
		}
	}
	for _, m := range after {
		if m.ID == sheetMatchID(2, models.StageGroup, "A") && (m.TeamA != "Karmine Corp" || m.PlaceholderA != nil) {
			t.Fatal("resolved team still a placeholder")
		}
	}
}

func TestWorldsRejectsBrokenScheduleAndMissingMatches(t *testing.T) {
	for _, damage := range []func([][][]string){
		func(tabs [][][]string) { tabs[4][1][21] = "not a time" },
		func(tabs [][][]string) { tabs[4][1][13] = "Day 6 1A" },
		func(tabs [][][]string) { tabs[1][6][10] = "" },
		func(tabs [][][]string) { tabs[0][8][6] = "#REF!" },
	} {
		tabs := worldsFixtures(t)
		damage(tabs)
		if _, err := parseWorlds(tabs[0], tabs[1], tabs[2], tabs[3], tabs[4], 2); err == nil {
			t.Fatal("accepted broken source data")
		}
	}
}

func TestWorldsScheduleOverwrittenMatchLetterHeader(t *testing.T) {
	tabs := worldsFixtures(t)
	tabs[4][0][12] = "f" // Live sheet on September 20.
	// The left-hand block must not replace the authoritative right-hand times.
	tabs[4][1][9] = "1:00 PM"
	matches := parseWorldsFixtures(t, tabs, 2)
	for _, m := range matches {
		if m.ID == sheetMatchID(2, models.StagePlayIn, "AA") {
			if m.ScheduledAt == nil || *m.ScheduledAt != "2026-09-15T16:00:00Z" {
				t.Fatalf("did not retain right-hand schedule: %+v", m)
			}
			return
		}
	}
	t.Fatal("missing first play-in match")
}

func TestWorldsSideEventCompletionUsesBestOfSeven(t *testing.T) {
	tabs := worldsFixtures(t)
	tabs[3][22][6], tabs[3][23][6] = "3", "2"
	matches := parseWorldsFixtures(t, tabs, 2)
	for _, m := range matches {
		if m.ID == sheetMatchID(2, models.Stage1v1, "A") && m.Status != models.StatusLive {
			t.Fatal("1v1 ended at three wins")
		}
	}
	tabs[3][22][6] = "4"
	matches = parseWorldsFixtures(t, tabs, 2)
	for _, m := range matches {
		if m.ID == sheetMatchID(2, models.Stage1v1, "A") && (m.Status != models.StatusCompleted || m.Winner == nil || *m.Winner != models.PickA) {
			t.Fatal("1v1 did not end at four wins")
		}
	}
}
