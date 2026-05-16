package locking

import (
	"testing"
	"time"

	"github.com/jometheuspondo/rlcs-predictions/internal/models"
)

func sp(s string) *string { return &s }

// mkMatch builds a match with a given scheduled timestamp and status.
// scheduledAt "" means no scheduled time at all.
func mkMatch(id, scheduledAt, status string) models.Match {
	m := models.Match{ID: id, Status: status}
	if scheduledAt != "" {
		m.ScheduledAt = sp(scheduledAt)
	}
	return m
}

// The fixture mirrors the real tournament shape: four group/bracket days with
// published 2 PM CEST starts (12:00 UTC), and a final day at midnight (start
// time not yet published — which doesn't matter, the final day is per-match).
func fixtureMatches() []models.Match {
	return []models.Match{
		mkMatch("d1", "2026-05-20T12:00:00Z", models.StatusUpcoming),
		mkMatch("d2", "2026-05-21T12:00:00Z", models.StatusUpcoming),
		mkMatch("d3", "2026-05-22T00:00:00Z", models.StatusUpcoming), // bracket day, start unpublished
		mkMatch("d4", "2026-05-23T00:00:00Z", models.StatusUpcoming), // bracket day, start unpublished
		mkMatch("d5", "2026-05-24T00:00:00Z", models.StatusUpcoming), // final day
	}
}

func TestBuildSchedule_FinalDayAndDayLocks(t *testing.T) {
	s := BuildSchedule(fixtureMatches())

	if s.finalDay != "2026-05-24" {
		t.Errorf("finalDay: got %q, want 2026-05-24", s.finalDay)
	}
	// Days 1 and 2 have real (non-midnight) starts → present in dayLock.
	for _, d := range []string{"2026-05-20", "2026-05-21"} {
		if _, ok := s.dayLock[d]; !ok {
			t.Errorf("expected dayLock entry for %s", d)
		}
	}
	// Days 3 and 4 only have midnight (date-only) timestamps → absent.
	for _, d := range []string{"2026-05-22", "2026-05-23"} {
		if _, ok := s.dayLock[d]; ok {
			t.Errorf("did not expect a dayLock entry for %s (start unpublished)", d)
		}
	}
}

func TestBuildSchedule_EarliestStartWins(t *testing.T) {
	// Three matches on the same day at 12:00, 13:00, 14:00 → lock at 12:00.
	s := BuildSchedule([]models.Match{
		mkMatch("late", "2026-05-20T14:00:00Z", models.StatusUpcoming),
		mkMatch("first", "2026-05-20T12:00:00Z", models.StatusUpcoming),
		mkMatch("mid", "2026-05-20T13:00:00Z", models.StatusUpcoming),
		mkMatch("final", "2026-05-21T00:00:00Z", models.StatusUpcoming),
	})
	want := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	if got := s.dayLock["2026-05-20"]; !got.Equal(want) {
		t.Errorf("dayLock: got %v, want %v", got, want)
	}
}

func TestIsLocked_CompletedAlwaysLocked(t *testing.T) {
	s := BuildSchedule(fixtureMatches())
	m := mkMatch("x", "2026-05-20T12:00:00Z", models.StatusCompleted)
	if !s.IsLocked(m, time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC)) {
		t.Error("a completed match must be locked even before its day")
	}
}

func TestIsLocked_ScheduledDay(t *testing.T) {
	s := BuildSchedule(fixtureMatches())
	day1 := mkMatch("d1", "2026-05-20T12:00:00Z", models.StatusUpcoming)

	before := time.Date(2026, 5, 20, 11, 59, 0, 0, time.UTC)
	if s.IsLocked(day1, before) {
		t.Error("day 1 must be open one minute before the 12:00 lock time")
	}

	atLock := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	if !s.IsLocked(day1, atLock) {
		t.Error("day 1 must be locked exactly at the lock time")
	}

	after := time.Date(2026, 5, 20, 15, 0, 0, 0, time.UTC)
	if !s.IsLocked(day1, after) {
		t.Error("day 1 must be locked after the lock time")
	}
}

func TestIsLocked_ScheduledDayLocksFutureDaysIndependently(t *testing.T) {
	s := BuildSchedule(fixtureMatches())
	day2 := mkMatch("d2", "2026-05-21T12:00:00Z", models.StatusUpcoming)

	// It is past day 1's lock time but before day 2's — day 2 stays open.
	duringDay1 := time.Date(2026, 5, 20, 15, 0, 0, 0, time.UTC)
	if s.IsLocked(day2, duringDay1) {
		t.Error("day 2 must stay open while only day 1 has started")
	}
}

func TestIsLocked_FinalDayIsPerMatch(t *testing.T) {
	s := BuildSchedule(fixtureMatches())
	now := time.Date(2026, 5, 24, 20, 0, 0, 0, time.UTC) // well into the final day

	upcoming := mkMatch("gf", "2026-05-24T00:00:00Z", models.StatusUpcoming)
	if s.IsLocked(upcoming, now) {
		t.Error("final-day match must stay open until it starts")
	}

	live := mkMatch("gf", "2026-05-24T00:00:00Z", models.StatusLive)
	if !s.IsLocked(live, now) {
		t.Error("final-day match must lock once it has started (live)")
	}
}

func TestIsLocked_UnpublishedDayFallsBackToPerMatch(t *testing.T) {
	s := BuildSchedule(fixtureMatches())
	now := time.Date(2026, 5, 22, 18, 0, 0, 0, time.UTC)

	// Day 3 has no published start time → per-match locking.
	upcoming := mkMatch("d3", "2026-05-22T00:00:00Z", models.StatusUpcoming)
	if s.IsLocked(upcoming, now) {
		t.Error("day 3 (start unpublished) must stay open while its match is upcoming")
	}
	live := mkMatch("d3", "2026-05-22T00:00:00Z", models.StatusLive)
	if !s.IsLocked(live, now) {
		t.Error("day 3 (start unpublished) match must lock once it has started")
	}
}

func TestIsLocked_UnpublishedDayBecomesDayLockedWhenStartAppears(t *testing.T) {
	// Same fixture, but day 3 now has a published 3 PM CEST start (13:00 UTC).
	matches := fixtureMatches()
	matches[2] = mkMatch("d3", "2026-05-22T13:00:00Z", models.StatusUpcoming)
	s := BuildSchedule(matches)

	day3 := matches[2]
	before := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	if s.IsLocked(day3, before) {
		t.Error("day 3 must be open before its now-published lock time")
	}
	after := time.Date(2026, 5, 22, 13, 30, 0, 0, time.UTC)
	if !s.IsLocked(day3, after) {
		t.Error("day 3 must day-lock once a start time is published and reached")
	}
}
