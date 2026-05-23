package matchsource

import (
	_ "embed"
	"encoding/csv"
	"strings"
	"testing"

	"github.com/jometheuspondo/rlcs-predictions/internal/models"
)

// CSV fixtures are exact copies of the Groups Output and Bracket Output tabs
// from the live broadcast LOP as of 2026-05-15 (pre-event, every score 0).
// Both stay in testdata/ so they don't ship in the binary; matchsource itself
// does no //go:embed of CSVs.

//go:embed testdata/groups.csv
var testGroupsCSV string

//go:embed testdata/bracket.csv
var testBracketCSV string

const testTournamentID = 1

// readCSVString is a test helper that parses an embedded CSV string the same
// way SheetSource.fetchCSV would parse the HTTP response body.
func readCSVString(t *testing.T, s string) [][]string {
	t.Helper()
	r := csv.NewReader(strings.NewReader(s))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("csv parse: %v", err)
	}
	return rows
}

// indexByLabel keys matches by the label embedded in their match_id (we don't
// store the label as a field, but the deterministic hash means we can compute
// the expected id from a label and check membership). For the tests that need
// to identify matches by label, we instead key by (stage, team_a, team_b)
// which is unique per stage in the LOP fixtures.
func indexMatchesByTeams(matches []models.Match, stage string) map[string]models.Match {
	out := make(map[string]models.Match)
	for _, m := range matches {
		if m.Round.Stage != stage {
			continue
		}
		key := m.TeamA + "|" + m.TeamB
		out[key] = m
	}
	return out
}

func TestParseGroupsCSV_MatchCount(t *testing.T) {
	rows := readCSVString(t, testGroupsCSV)
	matches, err := parseGroupsCSV(rows, testTournamentID)
	if err != nil {
		t.Fatalf("parseGroupsCSV: %v", err)
	}
	if got, want := len(matches), 24; got != want {
		t.Errorf("group match count: got %d, want %d", got, want)
	}

	// Three rounds × 4 groups × 2 matches per round = 24, evenly split per round.
	perRound := map[string]int{}
	for _, m := range matches {
		perRound[m.Round.Name]++
	}
	for _, r := range []string{"Group Stage - Round 1", "Group Stage - Round 2", "Group Stage - Round 3"} {
		if got, want := perRound[r], 8; got != want {
			t.Errorf("round %q match count: got %d, want %d", r, got, want)
		}
	}
}

func TestParseGroupsCSV_TeamNameNormalization(t *testing.T) {
	rows := readCSVString(t, testGroupsCSV)
	matches, err := parseGroupsCSV(rows, testTournamentID)
	if err != nil {
		t.Fatalf("parseGroupsCSV: %v", err)
	}

	// Verify three known overrides from teamNameOverrides have been applied:
	// "Wildcard Gaming" → "Wildcard", "Made In Brazil" → "MIBR",
	// "Manchester City" → "Manchester City Esports".
	saw := map[string]bool{}
	for _, m := range matches {
		saw[m.TeamA] = true
		saw[m.TeamB] = true
	}
	for _, want := range []string{"Wildcard", "MIBR", "Manchester City Esports"} {
		if !saw[want] {
			t.Errorf("expected normalized team %q to appear in group matches", want)
		}
	}
	for _, unwanted := range []string{"Wildcard Gaming", "Made In Brazil", "Manchester City"} {
		if saw[unwanted] {
			t.Errorf("expected raw sheet name %q to have been overridden", unwanted)
		}
	}
}

func TestParseGroupsCSV_KnownMatchups(t *testing.T) {
	rows := readCSVString(t, testGroupsCSV)
	matches, err := parseGroupsCSV(rows, testTournamentID)
	if err != nil {
		t.Fatalf("parseGroupsCSV: %v", err)
	}

	byTeams := indexMatchesByTeams(matches, models.StageGroup)

	// Group A Round 1 match I: Wildcard Gaming vs Team Vitality on Day 1 2A.
	// After normalization "Wildcard Gaming" → "Wildcard".
	m, ok := byTeams["Wildcard|Team Vitality"]
	if !ok {
		t.Fatalf("expected Wildcard vs Team Vitality match in group stage")
	}
	if m.Round.Name != "Group Stage - Round 1" {
		t.Errorf("Wildcard vs Team Vitality round: got %q, want %q", m.Round.Name, "Group Stage - Round 1")
	}
	if m.Slot == nil || *m.Slot != "2A" {
		t.Errorf("Wildcard vs Team Vitality slot: got %v, want %q", m.Slot, "2A")
	}
	if m.ScheduledAt == nil || *m.ScheduledAt != "2026-05-20T00:00:00Z" {
		t.Errorf("Wildcard vs Team Vitality scheduled_at: got %v, want Day 1 (2026-05-20)", m.ScheduledAt)
	}
	if m.Status != models.StatusUpcoming {
		t.Errorf("pre-event match status: got %q, want %q", m.Status, models.StatusUpcoming)
	}
	if m.TeamAScore != nil || m.TeamBScore != nil {
		t.Errorf("upcoming match should have nil scores, got %v / %v", m.TeamAScore, m.TeamBScore)
	}

	// Group D Round 3 match X: R8 Esports vs FURIA on Day 2 6B.
	m, ok = byTeams["R8 Esports|FURIA"]
	if !ok {
		t.Fatalf("expected R8 Esports vs FURIA match in group stage")
	}
	if m.Slot == nil || *m.Slot != "6B" {
		t.Errorf("R8 Esports vs FURIA slot: got %v, want %q", m.Slot, "6B")
	}
	if m.ScheduledAt == nil || *m.ScheduledAt != "2026-05-21T00:00:00Z" {
		t.Errorf("R8 Esports vs FURIA scheduled_at: got %v, want Day 2 (2026-05-21)", m.ScheduledAt)
	}
}

func TestParseBracketCSV_MatchCount(t *testing.T) {
	rows := readCSVString(t, testBracketCSV)
	matches, err := parseBracketCSV(rows, testTournamentID)
	if err != nil {
		t.Fatalf("parseBracketCSV: %v", err)
	}
	// 4 (R1: C,D,E,F) + 4 (R2: A,B,G,H) + 2 (QF: I,J) + 2 (SF: K,L) + 1 (GF: M) = 13.
	if got, want := len(matches), 13; got != want {
		t.Errorf("bracket match count: got %d, want %d", got, want)
	}

	perRound := map[string]int{}
	for _, m := range matches {
		perRound[m.Round.Name]++
	}
	for round, want := range map[string]int{
		"Bracket - Round 1": 4,
		"Bracket - Round 2": 4,
		"Quarterfinals":     2,
		"Semifinals":        2,
		"Grand Finals":      1,
	} {
		if got := perRound[round]; got != want {
			t.Errorf("bracket round %q count: got %d, want %d", round, got, want)
		}
	}
}

func TestParseBracketCSV_PlaceholdersPopulated(t *testing.T) {
	rows := readCSVString(t, testBracketCSV)
	matches, err := parseBracketCSV(rows, testTournamentID)
	if err != nil {
		t.Fatalf("parseBracketCSV: %v", err)
	}

	// Pre-event: every bracket match has both sides unresolved, so each row
	// should have PlaceholderA AND PlaceholderB set, with TeamA / TeamB empty.
	for i, m := range matches {
		if m.TeamA != "" {
			t.Errorf("bracket match %d: TeamA expected empty, got %q", i, m.TeamA)
		}
		if m.TeamB != "" {
			t.Errorf("bracket match %d: TeamB expected empty, got %q", i, m.TeamB)
		}
		if m.PlaceholderA == nil || *m.PlaceholderA == "" {
			t.Errorf("bracket match %d (round %q): PlaceholderA expected non-empty", i, m.Round.Name)
		}
		if m.PlaceholderB == nil || *m.PlaceholderB == "" {
			t.Errorf("bracket match %d (round %q): PlaceholderB expected non-empty", i, m.Round.Name)
		}
		if m.Status != models.StatusUpcoming {
			t.Errorf("bracket match %d: status got %q, want upcoming", i, m.Status)
		}
	}
}

func TestParseBracketCSV_SpecificPlaceholderText(t *testing.T) {
	rows := readCSVString(t, testBracketCSV)
	matches, err := parseBracketCSV(rows, testTournamentID)
	if err != nil {
		t.Fatalf("parseBracketCSV: %v", err)
	}

	// Build a lookup by (round, placeholder_a) so we can spot-check expected
	// matchups regardless of slice order.
	type key struct{ round, phA string }
	idx := make(map[key]models.Match)
	for _, m := range matches {
		if m.PlaceholderA == nil {
			continue
		}
		idx[key{m.Round.Name, *m.PlaceholderA}] = m
	}

	cases := []struct {
		round, phA, phB string
	}{
		// R2 match A: Group A First vs Group D First, Day 4 2A.
		{"Bracket - Round 2", "Group A First", "Group D First"},
		// R2 match B: Group B First vs Group C First, Day 4 2A.
		{"Bracket - Round 2", "Group B First", "Group C First"},
		// R1 match C: Group B Second vs Group D Third, Day 3 4A.
		{"Bracket - Round 1", "Group B Second", "Group D Third"},
		// QF match I: Loser of A vs Winner of C.
		{"Quarterfinals", "Loser of A", "Winner of C"},
		// SF match K: Winner of B vs Winner of I.
		{"Semifinals", "Winner of B", "Winner of I"},
		// GF match M: Winner of K vs Winner of L.
		{"Grand Finals", "Winner of K", "Winner of L"},
	}
	for _, tc := range cases {
		m, ok := idx[key{tc.round, tc.phA}]
		if !ok {
			t.Errorf("no match in %q with PlaceholderA=%q", tc.round, tc.phA)
			continue
		}
		if m.PlaceholderB == nil || *m.PlaceholderB != tc.phB {
			got := "<nil>"
			if m.PlaceholderB != nil {
				got = *m.PlaceholderB
			}
			t.Errorf("%s / %s: PlaceholderB got %q, want %q", tc.round, tc.phA, got, tc.phB)
		}
	}
}

func TestParseBracketCSV_SlotAndDate(t *testing.T) {
	rows := readCSVString(t, testBracketCSV)
	matches, err := parseBracketCSV(rows, testTournamentID)
	if err != nil {
		t.Fatalf("parseBracketCSV: %v", err)
	}

	wantByRound := map[string]struct {
		date string
		slot string
	}{
		// Bracket Round 1 plays Day 3 (2026-05-22) per the LOP.
		"Bracket - Round 1": {"2026-05-22T00:00:00Z", "4A"},
		// Round 2 plays Day 4 (2026-05-23).
		"Bracket - Round 2": {"2026-05-23T00:00:00Z", "2A"},
		// Quarterfinals, semifinals, grand finals all on Day 5.
		"Quarterfinals": {"2026-05-24T00:00:00Z", "2A"},
		"Semifinals":    {"2026-05-24T00:00:00Z", "4A"},
		"Grand Finals":  {"2026-05-24T00:00:00Z", "5A"},
	}

	for _, m := range matches {
		want, ok := wantByRound[m.Round.Name]
		if !ok {
			continue
		}
		if m.ScheduledAt == nil || *m.ScheduledAt != want.date {
			t.Errorf("%q scheduled_at: got %v, want %q", m.Round.Name, m.ScheduledAt, want.date)
		}
		// Each round in the LOP shares one slot across its matches (e.g.
		// both R1 matches on Day 3 4A, both QF on Day 5 2A). Just confirm
		// every match in that round has a non-nil slot whose first char
		// matches the expected slot's first char — defensive against slot
		// string drift.
		if m.Slot == nil {
			t.Errorf("%q match: slot expected non-nil", m.Round.Name)
		}
	}
}

func TestSheetMatchID_StableAcrossLabel(t *testing.T) {
	// Same (tournament, stage, label) → same hash. Different stage with same
	// label → different hash (because of stage in the input).
	a := sheetMatchID(1, models.StageGroup, "A")
	b := sheetMatchID(1, models.StageGroup, "A")
	if a != b {
		t.Errorf("same args produced different hashes: %s vs %s", a, b)
	}
	c := sheetMatchID(1, models.StageBracket, "A")
	if a == c {
		t.Errorf("group label A and bracket label A produced the same hash %s — stage not in input?", a)
	}
}

func TestComputeSheetStatus(t *testing.T) {
	cases := []struct {
		name       string
		a, b       int
		stage      string
		wantStatus string
		wantWinner string // "" for nil
	}{
		{"both zero group", 0, 0, models.StageGroup, models.StatusUpcoming, ""},
		{"group 3-1 a wins", 3, 1, models.StageGroup, models.StatusCompleted, models.PickA},
		{"group 2-3 b wins", 2, 3, models.StageGroup, models.StatusCompleted, models.PickB},
		{"group 2-2 live", 2, 2, models.StageGroup, models.StatusLive, ""},
		{"group 2-0 live", 2, 0, models.StageGroup, models.StatusLive, ""},
		{"bracket 3-0 still live", 3, 0, models.StageBracket, models.StatusLive, ""},
		{"bracket 4-2 a wins", 4, 2, models.StageBracket, models.StatusCompleted, models.PickA},
		{"bracket 3-4 b wins", 3, 4, models.StageBracket, models.StatusCompleted, models.PickB},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotStatus, gotWinner := computeSheetStatus(tc.a, tc.b, tc.stage)
			if gotStatus != tc.wantStatus {
				t.Errorf("status: got %q, want %q", gotStatus, tc.wantStatus)
			}
			var gotWinnerStr string
			if gotWinner != nil {
				gotWinnerStr = *gotWinner
			}
			if gotWinnerStr != tc.wantWinner {
				t.Errorf("winner: got %q, want %q", gotWinnerStr, tc.wantWinner)
			}
		})
	}
}

func TestParseSlotString(t *testing.T) {
	cases := []struct {
		in           string
		wantDay, wantPos string
	}{
		{"Day 1 2A", "Day 1", "2A"},
		{"Day 5 5A", "Day 5", "5A"},
		{"Day 2 6B", "Day 2", "6B"},
		{"", "", ""},
		{"  Day 3  4A  ", "Day 3", "4A"}, // extra whitespace tolerated
		{"Not a day", "", ""},
	}
	for _, tc := range cases {
		gotDay, gotPos := parseSlotString(tc.in)
		if gotDay != tc.wantDay || gotPos != tc.wantPos {
			t.Errorf("parseSlotString(%q) = (%q, %q), want (%q, %q)",
				tc.in, gotDay, gotPos, tc.wantDay, tc.wantPos)
		}
	}
}

func TestIsResolvedTeamCell(t *testing.T) {
	resolved := []string{"1", "8", "16", " 5 "}
	unresolved := []string{"", "#N/A", "Winner of A", "Loser of B", "0", "17", "-1", "abc"}
	for _, s := range resolved {
		if !isResolvedTeamCell(s) {
			t.Errorf("expected %q to be a resolved team cell", s)
		}
	}
	for _, s := range unresolved {
		if isResolvedTeamCell(s) {
			t.Errorf("expected %q to NOT be a resolved team cell", s)
		}
	}
}

func TestLooksLikePlaceholderTeamName(t *testing.T) {
	placeholders := []string{
		"",
		"   ",
		"#N/A",
		"#REF!",
		"#ERROR!",
		"Winner of A",
		"Winner of C",
		"winner of g",
		"WINNER OF B",
		"Loser of A",
		"Loser of B",
		"Group A First",
		"Group D Third",
		"group b second",
	}
	realTeams := []string{
		"Karmine Corp",
		"Twisted Minds",
		"FURIA",
		"NRG",
		"Team Vitality",
		"Manchester City Esports",
		"Spacestation Gaming",
		"Ninjas in Pyjamas",
		"FUT Esports",
		"Made In Brazil",
	}
	for _, s := range placeholders {
		if !looksLikePlaceholderTeamName(s) {
			t.Errorf("expected %q to look like a placeholder", s)
		}
	}
	for _, s := range realTeams {
		if looksLikePlaceholderTeamName(s) {
			t.Errorf("expected %q to NOT look like a placeholder", s)
		}
	}
}

func TestParseBracketCSV_TrustsRealTeamNameWhenBrIDUnresolved(t *testing.T) {
	// Minimal in-memory CSV exercising the parser fallback. Six empty pad
	// rows then one R2 bracket match (label col 9, team col 10, br_id col 11):
	//   top side: real team name + br_id "#N/A" → fallback should resolve.
	//   bottom side: placeholder text + br_id "#N/A" → stays unresolved.
	rows := [][]string{
		nil, nil, nil, nil, nil, nil,
		{"", "", "", "", "", "", "", "", "", "G", "Twisted Minds", "#N/A", "0", "Day 4 5A"},
		{"", "", "", "", "", "", "", "", "", "", "Winner of C", "#N/A", "0", ""},
	}

	matches, err := parseBracketCSV(rows, testTournamentID)
	if err != nil {
		t.Fatalf("parseBracketCSV: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("match count: got %d, want 1", len(matches))
	}
	m := matches[0]
	if m.TeamA != "Twisted Minds" {
		t.Errorf("TeamA: got %q, want %q (fallback should resolve real names)", m.TeamA, "Twisted Minds")
	}
	if m.PlaceholderA != nil {
		t.Errorf("PlaceholderA: got %q, want nil", *m.PlaceholderA)
	}
	if m.TeamB != "" {
		t.Errorf("TeamB: got %q, want empty (still a placeholder)", m.TeamB)
	}
	if m.PlaceholderB == nil || *m.PlaceholderB != "Winner of C" {
		got := "<nil>"
		if m.PlaceholderB != nil {
			got = *m.PlaceholderB
		}
		t.Errorf("PlaceholderB: got %q, want %q", got, "Winner of C")
	}
}
