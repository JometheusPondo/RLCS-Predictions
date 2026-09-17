package locking

import (
	"github.com/jometheuspondo/rlcs-predictions/internal/models"
	"testing"
	"time"
)

func TestAllStagesLockPerMatch(t *testing.T) {
	start := "2026-09-15T16:00:00Z"
	now := time.Date(2026, 9, 20, 22, 0, 0, 0, time.UTC)
	for _, stage := range []string{models.StagePlayIn, models.StageGroup, models.StageBracket, models.Stage1v1, models.Stage2v2} {
		t.Run(stage, func(t *testing.T) {
			upcoming := models.Match{ID: "upcoming", Status: models.StatusUpcoming, ScheduledAt: &start, Round: models.Round{Stage: stage}}
			live := upcoming
			live.ID, live.Status = "live", models.StatusLive
			completed := upcoming
			completed.ID, completed.Status = "completed", models.StatusCompleted
			schedule := BuildSchedule([]models.Match{upcoming, live, completed})
			if schedule.IsLocked(upcoming, now) {
				t.Fatal("scheduled time or another match's result locked an upcoming match")
			}
			if !schedule.IsLocked(live, now) || !schedule.IsLocked(completed, now) {
				t.Fatal("a match with reported game results remained editable")
			}
		})
	}
}
