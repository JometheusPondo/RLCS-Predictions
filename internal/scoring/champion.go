package scoring

import "github.com/jometheuspondo/rlcs-predictions/internal/models"

// ChampionBonuses awards floor(other eligible participants / 2) to each
// correct champion picker. Accounts with no pick count as not picking the
// champion. Historical selections never count more than once.
func ChampionBonuses(matches []models.Match, participants []models.Participant) map[string]int {
	champion := ""
	for _, m := range matches {
		if m.Round.Stage != models.StageBracket || m.Round.SortOrder != models.SortOrderFinal || m.Status != models.StatusCompleted || m.Winner == nil {
			continue
		}
		if *m.Winner == models.PickA {
			champion = m.TeamA
		} else if *m.Winner == models.PickB {
			champion = m.TeamB
		}
	}
	bonuses := make(map[string]int)
	if champion == "" {
		return bonuses
	}
	total := 0
	var correct []string
	for _, p := range participants {
		if p.ID == models.AdminID || p.ID == models.OwnerID || p.ID == "the-coin" || p.ID == "chat" {
			continue
		}
		total++
		if len(p.WinnerPicks) > 0 && p.WinnerPicks[len(p.WinnerPicks)-1].TeamName == champion {
			correct = append(correct, p.ID)
		}
	}
	for _, id := range correct {
		bonuses[id] = (total - len(correct)) / 2
	}
	return bonuses
}

// ComputeScoresWithChampion keeps projections consistent with real standings.
func ComputeScoresWithChampion(matches []models.Match, preds []PredictionRow, participants []models.Participant) map[string]int {
	scores := ComputeScores(matches, preds)
	for id, bonus := range ChampionBonuses(matches, participants) {
		scores[id] += bonus
	}
	return scores
}
