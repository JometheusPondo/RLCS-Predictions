// Package locking applies the same per-match rule to every tournament stage.
// Sheet importers mark a match live after its first nonzero game score, and
// completed after a winning score. Scheduled times never lock predictions.
package locking

import (
	"github.com/jometheuspondo/rlcs-predictions/internal/models"
	"time"
)

// Schedule retains the existing query interface; locking no longer depends on
// the other matches or the calendar.
type Schedule struct{}

func BuildSchedule(_ []models.Match) Schedule { return Schedule{} }

// IsLocked locks only the match whose first game result has been imported.
func (Schedule) IsLocked(m models.Match, _ time.Time) bool {
	return m.Status != models.StatusUpcoming
}
