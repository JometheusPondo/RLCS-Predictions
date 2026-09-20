package scoring

import (
	"fmt"
	"testing"

	"github.com/jometheuspondo/rlcs-predictions/internal/models"
)

func TestChampionBonuses(t *testing.T) {
	winner := models.PickA
	final := models.Match{TeamA: "FUT", TeamB: "NRG", Status: models.StatusCompleted, Winner: &winner,
		Round: models.Round{Stage: models.StageBracket, SortOrder: models.SortOrderFinal}}
	participants := make([]models.Participant, 14)
	for i := range participants {
		participants[i].ID = fmt.Sprint(i)
	}
	participants[0].WinnerPicks = []models.WinnerPick{{TeamName: "NRG"}, {TeamName: "FUT"}}
	participants[1].WinnerPicks = []models.WinnerPick{{TeamName: "FUT"}, {TeamName: "NRG"}}
	for _, id := range []string{models.AdminID, models.OwnerID, "the-coin", "chat"} {
		participants = append(participants, models.Participant{ID: id, WinnerPicks: []models.WinnerPick{{TeamName: "FUT"}}})
	}
	bonuses := ChampionBonuses([]models.Match{final}, participants)
	if len(bonuses) != 1 || bonuses["0"] != 6 {
		t.Fatalf("one of 14 eligible people should earn 6: %v", bonuses)
	}
	final.Status = models.StatusLive
	if len(ChampionBonuses([]models.Match{final}, participants)) != 0 {
		t.Fatal("bonus awarded before the final completed")
	}
	final.Status, final.Round.Stage = models.StatusCompleted, models.Stage1v1
	if len(ChampionBonuses([]models.Match{final}, participants)) != 0 {
		t.Fatal("side event awarded a tournament champion bonus")
	}
}
