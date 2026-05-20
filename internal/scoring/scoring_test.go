package scoring

import (
	"testing"

	"github.com/jometheuspondo/rlcs-predictions/internal/models"
)

func ip(n int) *int       { return &n }
func sp(s string) *string { return &s }

// match builds a completed match with the given stage, winner side, and game
// scores. status defaults to completed.
func completedMatch(id, stage, winner string, scoreA, scoreB int) models.Match {
	return models.Match{
		ID:         id,
		Round:      models.Round{Stage: stage},
		Status:     models.StatusCompleted,
		Winner:     sp(winner),
		TeamAScore: ip(scoreA),
		TeamBScore: ip(scoreB),
	}
}

func TestComputeScores_CorrectNonUnderdog(t *testing.T) {
	// One match, team A wins. Six humans all pick A → 6 humans on the side →
	// above the underdog cutoff → not an underdog pick → 2 points each.
	m := completedMatch("m1", models.StageGroup, models.PickA, 3, 1)
	var preds []PredictionRow
	for _, who := range []string{"a", "b", "c", "d", "e", "f"} {
		preds = append(preds, PredictionRow{ParticipantID: who, MatchID: "m1", Pick: models.PickA})
	}

	scores := ComputeScores([]models.Match{m}, preds)
	for _, who := range []string{"a", "b", "c", "d", "e", "f"} {
		if scores[who] != PointsCorrect {
			t.Errorf("%s: got %d, want %d (correct, not underdog)", who, scores[who], PointsCorrect)
		}
	}
}

func TestComputeScores_UnderdogBoundary(t *testing.T) {
	m := completedMatch("m1", models.StageGroup, models.PickA, 3, 0)

	// 4 humans pick A → 4 humans on the side → 4 <= 4 → underdog → 4 points.
	four := []PredictionRow{}
	for _, who := range []string{"a", "b", "c", "d"} {
		four = append(four, PredictionRow{ParticipantID: who, MatchID: "m1", Pick: models.PickA})
	}
	scores := ComputeScores([]models.Match{m}, four)
	if scores["a"] != PointsCorrectUnderdog {
		t.Errorf("4 humans on side: got %d, want %d (underdog)", scores["a"], PointsCorrectUnderdog)
	}

	// 5 humans pick A → 5 humans on the side → 5 > 4 → not an underdog.
	five := append(four, PredictionRow{ParticipantID: "e", MatchID: "m1", Pick: models.PickA})
	scores = ComputeScores([]models.Match{m}, five)
	if scores["a"] != PointsCorrect {
		t.Errorf("5 humans on side: got %d, want %d (not underdog)", scores["a"], PointsCorrect)
	}
}

func TestComputeScores_WrongWentDistance(t *testing.T) {
	// Group Bo5 that went 3-2 = 5 games = the distance. Wrong pick → 1 point.
	groupDistance := completedMatch("g", models.StageGroup, models.PickA, 3, 2)
	// Bracket Bo7 that went 4-3 = 7 games = the distance.
	bracketDistance := completedMatch("b", models.StageBracket, models.PickB, 3, 4)
	// Group Bo5 sweep 3-0 — did NOT go the distance.
	groupSweep := completedMatch("s", models.StageGroup, models.PickA, 3, 0)

	preds := []PredictionRow{
		{ParticipantID: "p", MatchID: "g", Pick: models.PickB}, // wrong, distance → 1
		{ParticipantID: "p", MatchID: "b", Pick: models.PickA}, // wrong, distance → 1
		{ParticipantID: "p", MatchID: "s", Pick: models.PickB}, // wrong, sweep → 0
	}
	scores := ComputeScores([]models.Match{groupDistance, bracketDistance, groupSweep}, preds)
	if scores["p"] != 2*PointsWentDistance+PointsWrong {
		t.Errorf("got %d, want %d (1 + 1 + 0)", scores["p"], 2*PointsWentDistance+PointsWrong)
	}
}

func TestComputeScores_IgnoresNonCompletedAndUnknownMatches(t *testing.T) {
	live := models.Match{
		ID: "live", Round: models.Round{Stage: models.StageGroup},
		Status: models.StatusLive, TeamAScore: ip(1), TeamBScore: ip(0),
	}
	preds := []PredictionRow{
		{ParticipantID: "p", MatchID: "live", Pick: models.PickA},  // match not completed
		{ParticipantID: "p", MatchID: "ghost", Pick: models.PickA}, // match not in set
	}
	scores := ComputeScores([]models.Match{live}, preds)
	if scores["p"] != 0 {
		t.Errorf("got %d, want 0 (no completed matches)", scores["p"])
	}
}

func TestComputeScores_SumsAcrossMatches(t *testing.T) {
	// p1: correct underdog on m1 (alone) + wrong-distance on m2 = 4 + 1 = 5.
	// p2: correct non-underdog on m1 would need 5 others; keep it simple —
	//     p2 wrong sweep on m1 (0) + correct underdog on m2 = 0 + 4 = 4.
	m1 := completedMatch("m1", models.StageGroup, models.PickA, 3, 0)
	m2 := completedMatch("m2", models.StageBracket, models.PickB, 2, 4)

	preds := []PredictionRow{
		{ParticipantID: "p1", MatchID: "m1", Pick: models.PickA}, // correct, 0 others → underdog → 4
		{ParticipantID: "p1", MatchID: "m2", Pick: models.PickA}, // wrong, 2+4=6 games = distance → 1
		{ParticipantID: "p2", MatchID: "m1", Pick: models.PickB}, // wrong, 3-0 sweep → 0
		{ParticipantID: "p2", MatchID: "m2", Pick: models.PickB}, // correct, 0 others → underdog → 4
	}
	scores := ComputeScores([]models.Match{m1, m2}, preds)
	if scores["p1"] != 5 {
		t.Errorf("p1: got %d, want 5", scores["p1"])
	}
	if scores["p2"] != 4 {
		t.Errorf("p2: got %d, want 4", scores["p2"])
	}
}

func TestComputeScores_BenchmarkExcludedFromUnderdogTally(t *testing.T) {
	// Four humans pick A → 4 humans on the side → 4 <= 4 → underdog → 4 points
	// each. A benchmark account ("The Coin") also picks A. If the benchmark
	// counted toward the tally the side would show 5 humans and drop out of
	// underdog territory — it must NOT count.
	m := completedMatch("m1", models.StageGroup, models.PickA, 3, 0)

	preds := []PredictionRow{}
	for _, who := range []string{"h1", "h2", "h3", "h4"} {
		preds = append(preds, PredictionRow{ParticipantID: who, MatchID: "m1", Pick: models.PickA})
	}
	preds = append(preds, PredictionRow{
		ParticipantID: "the-coin", MatchID: "m1", Pick: models.PickA, Benchmark: true,
	})

	scores := ComputeScores([]models.Match{m}, preds)
	for _, who := range []string{"h1", "h2", "h3", "h4"} {
		if scores[who] != PointsCorrectUnderdog {
			t.Errorf("%s: got %d, want %d — benchmark pick inflated the underdog tally",
				who, scores[who], PointsCorrectUnderdog)
		}
	}
}

func TestComputeScores_BenchmarkScoredAgainstHumanTally(t *testing.T) {
	// A benchmark account is scored against the human-only tally, exactly like
	// a participant. On m1, three humans plus the benchmark pick A → 3 humans
	// on the side → underdog → the benchmark earns the 4-point bonus. On m2,
	// five humans plus the benchmark pick A → 5 humans → above the cutoff →
	// the benchmark gets the plain correct score, no bonus.
	m1 := completedMatch("m1", models.StageGroup, models.PickA, 3, 0)
	m2 := completedMatch("m2", models.StageGroup, models.PickA, 3, 0)

	preds := []PredictionRow{
		{ParticipantID: "chat", MatchID: "m1", Pick: models.PickA, Benchmark: true},
		{ParticipantID: "chat", MatchID: "m2", Pick: models.PickA, Benchmark: true},
	}
	for _, who := range []string{"h1", "h2", "h3"} {
		preds = append(preds, PredictionRow{ParticipantID: who, MatchID: "m1", Pick: models.PickA})
	}
	for _, who := range []string{"h1", "h2", "h3", "h4", "h5"} {
		preds = append(preds, PredictionRow{ParticipantID: who, MatchID: "m2", Pick: models.PickA})
	}

	scores := ComputeScores([]models.Match{m1, m2}, preds)
	// m1 underdog bonus (4) + m2 plain correct (2) = 6.
	if scores["chat"] != PointsCorrectUnderdog+PointsCorrect {
		t.Errorf("chat (benchmark): got %d, want %d (4 underdog + 2 correct)",
			scores["chat"], PointsCorrectUnderdog+PointsCorrect)
	}
}

func TestUnderdogSide(t *testing.T) {
	cases := []struct {
		name     string
		a, b     int
		wantSide string
		wantOK   bool
	}{
		{"A is the minority, under cutoff", 2, 7, models.PickA, true},
		{"B is the minority, under cutoff", 9, 3, models.PickB, true},
		{"A minority at the cutoff", 4, 9, models.PickA, true},
		{"A minority just over the cutoff", 5, 9, "", false},
		{"both sides over the cutoff", 7, 6, "", false},
		{"tie under the cutoff", 3, 3, "", false},
		{"tie over the cutoff", 8, 8, "", false},
		{"one side empty", 0, 5, models.PickA, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			side, ok := UnderdogSide(c.a, c.b)
			if side != c.wantSide || ok != c.wantOK {
				t.Errorf("UnderdogSide(%d, %d) = (%q, %t), want (%q, %t)",
					c.a, c.b, side, ok, c.wantSide, c.wantOK)
			}
		})
	}
}

func TestComputeStats_CorrectCount(t *testing.T) {
	// Three completed matches. p1 picks correctly on all three. p2 picks
	// correctly on m1, wrong on m2 (a Bo5 going the distance, so p2 gets the
	// 1-point distance bonus but the pick is still wrong), and wrong on m3
	// (no distance bonus). Correct should count picks that matched the
	// winner, not points earned, so p1.Correct=3 and p2.Correct=1.
	m1 := completedMatch("m1", models.StageGroup, models.PickA, 3, 1)
	m2 := completedMatch("m2", models.StageGroup, models.PickB, 2, 3)
	m3 := completedMatch("m3", models.StageGroup, models.PickA, 3, 0)

	preds := []PredictionRow{
		{ParticipantID: "p1", MatchID: "m1", Pick: models.PickA},
		{ParticipantID: "p1", MatchID: "m2", Pick: models.PickB},
		{ParticipantID: "p1", MatchID: "m3", Pick: models.PickA},
		{ParticipantID: "p2", MatchID: "m1", Pick: models.PickA},
		{ParticipantID: "p2", MatchID: "m2", Pick: models.PickA},
		{ParticipantID: "p2", MatchID: "m3", Pick: models.PickB},
	}

	stats := ComputeStats([]models.Match{m1, m2, m3}, preds)

	if stats["p1"].Correct != 3 {
		t.Errorf("p1.Correct: got %d, want 3", stats["p1"].Correct)
	}
	if stats["p2"].Correct != 1 {
		t.Errorf("p2.Correct: got %d, want 1", stats["p2"].Correct)
	}
}
