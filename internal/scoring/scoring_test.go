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
	// One match, team A wins. Six participants all pick A → each has 5 others
	// on the same side → not an underdog pick → 2 points each.
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

	// 5 participants pick A: each has 4 others → 4 < 5 → underdog → 4 points.
	five := []PredictionRow{}
	for _, who := range []string{"a", "b", "c", "d", "e"} {
		five = append(five, PredictionRow{ParticipantID: who, MatchID: "m1", Pick: models.PickA})
	}
	scores := ComputeScores([]models.Match{m}, five)
	if scores["a"] != PointsCorrectUnderdog {
		t.Errorf("5 pickers (4 others): got %d, want %d (underdog)", scores["a"], PointsCorrectUnderdog)
	}

	// 6 participants pick A: each has 5 others → 5 is NOT < 5 → not underdog.
	six := append(five, PredictionRow{ParticipantID: "f", MatchID: "m1", Pick: models.PickA})
	scores = ComputeScores([]models.Match{m}, six)
	if scores["a"] != PointsCorrect {
		t.Errorf("6 pickers (5 others): got %d, want %d (not underdog)", scores["a"], PointsCorrect)
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
		{ParticipantID: "p", MatchID: "live", Pick: models.PickA},    // match not completed
		{ParticipantID: "p", MatchID: "ghost", Pick: models.PickA},   // match not in set
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
