package simulation

import (
	"testing"

	"github.com/jometheuspondo/rlcs-predictions/internal/models"
	"github.com/jometheuspondo/rlcs-predictions/internal/scoring"
)

func ip(n int) *int       { return &n }
func sp(s string) *string { return &s }

// basicFixture: one completed group match M0 (A won 3-1 — not the distance)
// and one upcoming group match M1, both on 2026-05-20. Three participants.
//
//	M0 picks: alice→A, bob→A, carol→B   → baseline alice 4, bob 4, carol 0
//	M1 picks: alice→A, bob→B, carol→A
//
// Hand-computed expected swings: alice {best 0, worst 1}, bob {best 1, worst 0},
// carol {best 0, worst 0}.
func basicFixture() ([]models.Match, []scoring.PredictionRow, []models.Participant) {
	m0 := models.Match{
		ID:          "M0",
		Round:       models.Round{Stage: models.StageGroup},
		Status:      models.StatusCompleted,
		Winner:      sp(models.PickA),
		TeamAScore:  ip(3),
		TeamBScore:  ip(1),
		ScheduledAt: sp("2026-05-20T12:00:00Z"),
	}
	m1 := models.Match{
		ID:          "M1",
		Round:       models.Round{Stage: models.StageGroup},
		Status:      models.StatusUpcoming,
		ScheduledAt: sp("2026-05-20T15:00:00Z"),
	}
	preds := []scoring.PredictionRow{
		{ParticipantID: "alice", MatchID: "M0", Pick: models.PickA},
		{ParticipantID: "bob", MatchID: "M0", Pick: models.PickA},
		{ParticipantID: "carol", MatchID: "M0", Pick: models.PickB},
		{ParticipantID: "alice", MatchID: "M1", Pick: models.PickA},
		{ParticipantID: "bob", MatchID: "M1", Pick: models.PickB},
		{ParticipantID: "carol", MatchID: "M1", Pick: models.PickA},
	}
	participants := []models.Participant{
		{ID: "alice", DisplayName: "alice"},
		{ID: "bob", DisplayName: "bob"},
		{ID: "carol", DisplayName: "carol"},
	}
	return []models.Match{m0, m1}, preds, participants
}

func resultsByID(results []Result) map[string]Result {
	out := make(map[string]Result, len(results))
	for _, r := range results {
		out[r.ParticipantID] = r
	}
	return out
}

func TestCompute_BasicDeltas(t *testing.T) {
	matches, preds, participants := basicFixture()
	day, results := Compute(matches, preds, participants)

	if day != "2026-05-20" {
		t.Errorf("day: got %q, want 2026-05-20", day)
	}

	want := map[string]Result{
		"alice": {ParticipantID: "alice", BestCaseDelta: 0, WorstCaseDelta: 1},
		"bob":   {ParticipantID: "bob", BestCaseDelta: 1, WorstCaseDelta: 0},
		"carol": {ParticipantID: "carol", BestCaseDelta: 0, WorstCaseDelta: 0},
	}
	got := resultsByID(results)
	for id, w := range want {
		g, ok := got[id]
		if !ok {
			t.Errorf("%s: missing from results", id)
			continue
		}
		if g.BestCaseDelta != w.BestCaseDelta || g.WorstCaseDelta != w.WorstCaseDelta {
			t.Errorf("%s: got best=%d worst=%d, want best=%d worst=%d",
				id, g.BestCaseDelta, g.WorstCaseDelta, w.BestCaseDelta, w.WorstCaseDelta)
		}
	}
}

func TestCompute_DeltasNeverNegative(t *testing.T) {
	// Under model A, a participant weakly out-scores the field in their
	// all-correct world and is weakly out-scored in their all-wrong world, so
	// neither delta can be negative in its stated direction.
	matches, preds, participants := basicFixture()
	_, results := Compute(matches, preds, participants)
	for _, r := range results {
		if r.BestCaseDelta < 0 {
			t.Errorf("%s: best-case delta %d is negative", r.ParticipantID, r.BestCaseDelta)
		}
		if r.WorstCaseDelta < 0 {
			t.Errorf("%s: worst-case delta %d is negative", r.ParticipantID, r.WorstCaseDelta)
		}
	}
}

// TestCompute_SwingCoversCurrentDayOnly is the regression test for the rule
// that the swing reflects ONLY the current day. A participant who fills out
// predictions for several days at once must still see a swing based purely on
// the current (earliest unfinished) day — picks for later days sit unused
// until those days come around.
func TestCompute_SwingCoversCurrentDayOnly(t *testing.T) {
	// Two upcoming matches on different days. M1 is the current (earliest)
	// day; M2 is the day after.
	m1 := models.Match{
		ID: "M1", Round: models.Round{Stage: models.StageGroup},
		Status: models.StatusUpcoming, ScheduledAt: sp("2026-05-20T14:00:00Z"),
	}
	m2 := models.Match{
		ID: "M2", Round: models.Round{Stage: models.StageGroup},
		Status: models.StatusUpcoming, ScheduledAt: sp("2026-05-21T14:00:00Z"),
	}
	matches := []models.Match{m1, m2}
	participants := []models.Participant{
		{ID: "alice", DisplayName: "alice"},
		{ID: "bob", DisplayName: "bob"},
		{ID: "carol", DisplayName: "carol"},
	}

	// M1 picks (the current day) — these alone should drive the swing.
	m1Picks := []scoring.PredictionRow{
		{ParticipantID: "alice", MatchID: "M1", Pick: models.PickA},
		{ParticipantID: "bob", MatchID: "M1", Pick: models.PickA},
		{ParticipantID: "carol", MatchID: "M1", Pick: models.PickB},
	}
	// Everyone has ALSO pre-filled the next day (M2). This is the "did all the
	// group picks at once" case — M2 picks must be noise as far as the swing
	// is concerned.
	withNextDay := append([]scoring.PredictionRow(nil), m1Picks...)
	withNextDay = append(withNextDay,
		scoring.PredictionRow{ParticipantID: "alice", MatchID: "M2", Pick: models.PickA},
		scoring.PredictionRow{ParticipantID: "bob", MatchID: "M2", Pick: models.PickB},
		scoring.PredictionRow{ParticipantID: "carol", MatchID: "M2", Pick: models.PickA},
	)

	day, withResults := Compute(matches, withNextDay, participants)
	if day != "2026-05-20" {
		t.Errorf("day: got %q, want 2026-05-20 (the earlier, current day)", day)
	}

	// Same M1 picks, no M2 picks at all. The swing must be identical — if it
	// differs, next-day picks are leaking into the current day's projection.
	_, m1OnlyResults := Compute(matches, m1Picks, participants)

	withMap := resultsByID(withResults)
	m1OnlyMap := resultsByID(m1OnlyResults)
	for id, w := range withMap {
		o := m1OnlyMap[id]
		if w.BestCaseDelta != o.BestCaseDelta || w.WorstCaseDelta != o.WorstCaseDelta {
			t.Errorf("%s: swing changed when next-day (M2) picks were added — "+
				"with M2: best=%d worst=%d, M1 only: best=%d worst=%d; "+
				"next-day picks must not affect the current day's swing",
				id, w.BestCaseDelta, w.WorstCaseDelta, o.BestCaseDelta, o.WorstCaseDelta)
		}
	}

	// Concrete check on the current-day-only swing (M1 drives everything).
	want := map[string]Result{
		"alice": {ParticipantID: "alice", BestCaseDelta: 0, WorstCaseDelta: 1},
		"bob":   {ParticipantID: "bob", BestCaseDelta: 0, WorstCaseDelta: 1},
		"carol": {ParticipantID: "carol", BestCaseDelta: 2, WorstCaseDelta: 0},
	}
	for id, w := range want {
		g := withMap[id]
		if g.BestCaseDelta != w.BestCaseDelta || g.WorstCaseDelta != w.WorstCaseDelta {
			t.Errorf("%s: got best=%d worst=%d, want best=%d worst=%d",
				id, g.BestCaseDelta, g.WorstCaseDelta, w.BestCaseDelta, w.WorstCaseDelta)
		}
	}
}

func TestCompute_ParticipantWhoSkippedTheDay(t *testing.T) {
	// sam picked the completed match but none of the day's matches → no
	// exposure → zero swing both ways.
	m0 := models.Match{
		ID: "M0", Round: models.Round{Stage: models.StageGroup},
		Status: models.StatusCompleted, Winner: sp(models.PickA),
		TeamAScore: ip(3), TeamBScore: ip(1),
		ScheduledAt: sp("2026-05-20T12:00:00Z"),
	}
	m1 := models.Match{
		ID: "M1", Round: models.Round{Stage: models.StageGroup},
		Status: models.StatusUpcoming, ScheduledAt: sp("2026-05-20T15:00:00Z"),
	}
	preds := []scoring.PredictionRow{
		{ParticipantID: "pat", MatchID: "M0", Pick: models.PickA},
		{ParticipantID: "sam", MatchID: "M0", Pick: models.PickA},
		{ParticipantID: "pat", MatchID: "M1", Pick: models.PickA},
		// sam makes no M1 prediction.
	}
	participants := []models.Participant{
		{ID: "pat", DisplayName: "pat"},
		{ID: "sam", DisplayName: "sam"},
	}

	_, results := Compute([]models.Match{m0, m1}, preds, participants)
	got := resultsByID(results)
	if got["sam"].BestCaseDelta != 0 || got["sam"].WorstCaseDelta != 0 {
		t.Errorf("sam (no day picks): got best=%d worst=%d, want 0/0",
			got["sam"].BestCaseDelta, got["sam"].WorstCaseDelta)
	}
}

func TestCompute_NoDayWhenAllCompleted(t *testing.T) {
	m := models.Match{
		ID: "M0", Round: models.Round{Stage: models.StageGroup},
		Status: models.StatusCompleted, Winner: sp(models.PickA),
		TeamAScore: ip(3), TeamBScore: ip(0),
		ScheduledAt: sp("2026-05-20T12:00:00Z"),
	}
	preds := []scoring.PredictionRow{
		{ParticipantID: "pat", MatchID: "M0", Pick: models.PickA},
	}
	participants := []models.Participant{{ID: "pat", DisplayName: "pat"}}

	day, results := Compute([]models.Match{m}, preds, participants)
	if day != "" {
		t.Errorf("day: got %q, want \"\" (no unfinished matches)", day)
	}
	for _, r := range results {
		if r.BestCaseDelta != 0 || r.WorstCaseDelta != 0 {
			t.Errorf("%s: got best=%d worst=%d, want 0/0 (nothing to simulate)",
				r.ParticipantID, r.BestCaseDelta, r.WorstCaseDelta)
		}
	}
}

func TestCurrentDay(t *testing.T) {
	done := func(id, date string) models.Match {
		return models.Match{ID: id, Status: models.StatusCompleted, ScheduledAt: sp(date)}
	}
	upcoming := func(id, date string) models.Match {
		return models.Match{ID: id, Status: models.StatusUpcoming, ScheduledAt: sp(date)}
	}

	if got := currentDay([]models.Match{
		done("a", "2026-05-20T12:00:00Z"),
		upcoming("b", "2026-05-21T12:00:00Z"),
	}); got != "2026-05-21" {
		t.Errorf("earliest incomplete: got %q, want 2026-05-21", got)
	}

	if got := currentDay([]models.Match{
		done("a", "2026-05-20T12:00:00Z"),
		done("b", "2026-05-21T12:00:00Z"),
	}); got != "" {
		t.Errorf("all completed: got %q, want \"\"", got)
	}

	if got := currentDay([]models.Match{
		upcoming("a", "2026-05-22T12:00:00Z"),
		upcoming("b", "2026-05-20T12:00:00Z"),
	}); got != "2026-05-20" {
		t.Errorf("earliest of several: got %q, want 2026-05-20", got)
	}
}
