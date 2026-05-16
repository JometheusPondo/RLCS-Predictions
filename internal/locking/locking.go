// Package locking decides whether a match's predictions are locked. It is a
// pure package — given the match set and the current time, it computes
// everything; there is no config and no IO.
//
// Two regimes:
//
//   - Scheduled days (every day except the last): all of a day's predictions
//     lock at one moment — the start of the day's first match. Locking one
//     match of the day locks them all; future days stay open. The lock time
//     is data-driven: it's the earliest real start time among that day's
//     matches, as published in the broadcast schedule.
//
//   - Per-match locking (the final day, and any earlier day whose start times
//     haven't been published yet): a match locks the moment it starts — its
//     status leaves "upcoming" on the first game update. The day is never
//     locked as a whole.
//
// A day falls back to per-match locking until at least one of its matches has
// a real published start time. So before the broadcast schedule fills in the
// bracket days, those days lock per-match; once a start time appears in the
// sheet, the whole day starts day-locking automatically on the next sync — no
// redeploy, no config change.
//
// "Real start time" vs "date only": the SheetSource sets a match's
// ScheduledAt to the day at 00:00:00 UTC when no wall-clock start has been
// published, and to the true start otherwise. No RLCS match starts at 00:00
// UTC (that's 02:00 CEST), so midnight is a safe "time unknown" sentinel.
package locking

import (
	"time"

	"github.com/jometheuspondo/rlcs-predictions/internal/models"
)

// dateLayout is the YYYY-MM-DD form used to key a day. Zero-padded, so plain
// string comparison orders dates correctly.
const dateLayout = "2006-01-02"

// Schedule is the precomputed locking context for a match set: which calendar
// date is the final day (per-match locking), and the lock time for each other
// day that has a published start. Build it once per match set with
// BuildSchedule, then call IsLocked per match.
type Schedule struct {
	finalDay string               // YYYY-MM-DD of the last match day; "" if unknown
	dayLock  map[string]time.Time // YYYY-MM-DD → that day's lock time (earliest real start)
}

// BuildSchedule derives the locking context from every match in the
// tournament. The final day is the latest match date; each earlier day's lock
// time is the earliest real (non-midnight) start among its matches. Days with
// no real start times yet are simply absent from dayLock and fall back to
// per-match locking.
func BuildSchedule(matches []models.Match) Schedule {
	s := Schedule{dayLock: make(map[string]time.Time)}
	for _, m := range matches {
		t, ok := parseScheduled(m)
		if !ok {
			continue
		}
		date := t.Format(dateLayout)
		if date > s.finalDay {
			s.finalDay = date
		}
		if hasRealStart(t) {
			if cur, exists := s.dayLock[date]; !exists || t.Before(cur) {
				s.dayLock[date] = t
			}
		}
	}
	return s
}

// IsLocked reports whether predictions on m are locked as of now.
//
//   - A completed match is always locked.
//   - Final day, or a match with no date: per-match — locked once the match
//     has started (status is no longer "upcoming").
//   - A scheduled day with a known lock time: locked once now is at or past
//     that lock time.
//   - A scheduled day whose start times aren't published yet: per-match.
func (s Schedule) IsLocked(m models.Match, now time.Time) bool {
	if m.Status == models.StatusCompleted {
		return true
	}

	t, ok := parseScheduled(m)
	if !ok {
		return startedPerMatch(m)
	}
	date := t.Format(dateLayout)

	if date == s.finalDay {
		return startedPerMatch(m)
	}
	if lockAt, scheduled := s.dayLock[date]; scheduled {
		return !now.Before(lockAt)
	}
	return startedPerMatch(m)
}

// startedPerMatch is the per-match rule: a match is locked once it leaves the
// "upcoming" state — i.e. once it has received its first game update.
func startedPerMatch(m models.Match) bool {
	return m.Status != models.StatusUpcoming
}

// parseScheduled parses a match's ScheduledAt into a UTC time. ok is false
// when the match has no scheduled time at all.
func parseScheduled(m models.Match) (time.Time, bool) {
	if m.ScheduledAt == nil || *m.ScheduledAt == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, *m.ScheduledAt)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

// hasRealStart reports whether t carries a real wall-clock start rather than
// just a date. See the package doc on the midnight-UTC sentinel.
func hasRealStart(t time.Time) bool {
	return !(t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0)
}
