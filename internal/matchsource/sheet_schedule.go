package matchsource

import (
	"fmt"
	"strings"
	"time"

	"github.com/jometheuspondo/rlcs-predictions/internal/models"
)

// The Overall Schedule tab (gid 1663218581) is a flat table — one row per
// match — carrying the broadcast's published start times. SheetSource overlays
// these onto the matches parsed from the Groups/Bracket tabs so the locking
// layer can day-lock from real times.
//
// Verified column layout (2026-05-16 against the live LOP):
//
//	0  Match Letter        "D", "I", "M", "SM" …
//	1  Day/Stream          "Day 1 1A"
//	2  Date/Stage          "Group B Round 1" / "Bracket Lower R1" / "SHOWMATCH"
//	3  B/O
//	4  Team 1
//	7  Team 2
//	9  Scheduled Start (CET)   "2:00 PM" — may be blank for not-yet-timed days
//	10 Target Start, 11 Act Start, 12 Start Delta, 13 Match Finish, 14 Notes
const (
	schedColMatchLetter = 0
	schedColDayStream   = 1
	schedColDateStage   = 2
	schedColStart       = 9
)

// cestZone is the fixed UTC+2 offset for Central European Summer Time. The
// RLCS Paris Major runs 2026-05-20..24, entirely within CEST, so a fixed
// offset is correct and avoids a tzdata dependency — the distroless runtime
// image ships no zoneinfo database, so time.LoadLocation would fail there.
var cestZone = time.FixedZone("CEST", 2*60*60)

// parseScheduleCSV reads the Overall Schedule tab and returns
// matchID → RFC3339 UTC scheduled-start string, for every row whose
// "Scheduled Start (CET)" cell is filled in. Rows with a blank start time,
// the header row, and the SHOWMATCH row are omitted; the caller keeps
// day-granularity ScheduledAt for matches not present in the result.
//
// Keys are recomputed via sheetMatchID(tournamentID, stage, matchLetter) so
// they line up exactly with the ids the Groups/Bracket parsers produce.
func parseScheduleCSV(rows [][]string, tournamentID int) (map[string]string, error) {
	out := make(map[string]string)

	for _, r := range rows {
		letter := strings.TrimSpace(cellInRow(r, schedColMatchLetter))
		dayStream := strings.TrimSpace(cellInRow(r, schedColDayStream))
		dateStage := strings.TrimSpace(cellInRow(r, schedColDateStage))
		startRaw := strings.TrimSpace(cellInRow(r, schedColStart))

		// Skip the header row, blank rows, and rows with no published start.
		if letter == "" || letter == "Match Letter" || startRaw == "" {
			continue
		}

		stage := scheduleStage(dateStage)
		if stage == "" {
			continue // SHOWMATCH or anything not a real prediction match
		}

		day, _ := parseSlotString(dayStream) // "Day 1 1A" → "Day 1"
		dateRFC3339, ok := dayDates[day]
		if !ok {
			continue
		}

		startUTC, err := cetClockToUTC(dateRFC3339, startRaw)
		if err != nil {
			// Unparseable time cell — skip it; that match keeps day granularity.
			continue
		}

		out[sheetMatchID(tournamentID, stage, letter)] = startUTC
	}

	return out, nil
}

// scheduleStage maps the Overall Schedule "Date/Stage" text to a models stage.
// Group rows contain "Group", bracket rows contain "Bracket"; anything else
// (notably the SHOWMATCH row) returns "" so the caller skips it.
func scheduleStage(dateStage string) string {
	switch {
	case strings.Contains(dateStage, "Group"):
		return models.StageGroup
	case strings.Contains(dateStage, "Bracket"):
		return models.StageBracket
	default:
		return ""
	}
}

// cetClockToUTC combines a date (a dayDates RFC3339 value, e.g.
// "2026-05-22T00:00:00Z") with a CET wall-clock time ("2:00 PM") and returns
// the instant as an RFC3339 UTC string. CET in late May is CEST (UTC+2).
func cetClockToUTC(dateRFC3339, clock string) (string, error) {
	d, err := time.Parse(time.RFC3339, dateRFC3339)
	if err != nil {
		return "", fmt.Errorf("bad date %q: %w", dateRFC3339, err)
	}
	// Go's reference layout for "h:mm AM/PM" is "3:04 PM". Upper-casing makes
	// the AM/PM marker match regardless of how the sheet cased it.
	tod, err := time.Parse("3:04 PM", strings.ToUpper(clock))
	if err != nil {
		return "", fmt.Errorf("bad clock %q: %w", clock, err)
	}
	local := time.Date(d.Year(), d.Month(), d.Day(),
		tod.Hour(), tod.Minute(), 0, 0, cestZone)
	return local.UTC().Format(time.RFC3339), nil
}

// cellInRow returns the cell at col, or "" if the row is too short. The
// Overall Schedule tab is parsed row-by-row, so this row-scoped accessor is
// the natural fit (vs. cellAt, which indexes the whole grid).
func cellInRow(row []string, col int) string {
	if col < 0 || col >= len(row) {
		return ""
	}
	return row[col]
}
