package matchsource

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jometheuspondo/rlcs-predictions/internal/models"
)

// SheetSource fetches tournament match data from public Google Sheets tabs via
// the docs.google.com CSV export endpoint. No auth — the sheet must be
// Viewer-shared with "anyone with the link".
//
// Three tabs are consumed:
//
//   - Groups Output: 4 group blocks × 3 rounds × 2 matches = 24 group matches.
//     Each match cell carries label (A–X), team_name, br_id (1–16), score,
//     and slot string ("Day 1 2A").
//
//   - Bracket Output: 5 round columns = 13 bracket matches (labels A–M). Same
//     cell shape, but until teams resolve, team_name is a placeholder like
//     "Group A First" / "Winner of C" and br_id is "#N/A" or a placeholder echo.
//
//   - Overall Schedule (optional): a flat row-per-match table with published
//     CET start times. SheetSource overlays these onto the match list so the
//     locking layer can day-lock from real times. A failure to fetch or parse
//     this tab is non-fatal — matches simply keep day-granularity ScheduledAt.
//
// Match IDs are hash(tournament_id | stage | label) — stable across the
// placeholder-to-real-team transition. NOTE: this is a different id space from
// the Liquipedia adapter's hash, so switching MATCH_SOURCE mid-tournament will
// orphan existing predictions (they're keyed by match_id). One-way switch.
type SheetSource struct {
	httpClient    *http.Client
	spreadsheetID string
	groupsGID     string
	bracketGID    string
	scheduleGID   string // optional; empty disables the start-time overlay
	tournamentID  int
	logger        *slog.Logger
}

// SheetSourceOptions configures a SheetSource. SpreadsheetID, GroupsGID, and
// BracketGID are required. ScheduleGID is optional — when empty, matches keep
// day-granularity start times. HTTPClient and Logger default if unset.
type SheetSourceOptions struct {
	SpreadsheetID string
	GroupsGID     string
	BracketGID    string
	ScheduleGID   string
	TournamentID  int
	HTTPClient    *http.Client
	Logger        *slog.Logger
}

// NewSheetSource constructs a SheetSource. Returns an error if any required
// ID is empty.
func NewSheetSource(opts SheetSourceOptions) (*SheetSource, error) {
	if opts.SpreadsheetID == "" {
		return nil, fmt.Errorf("sheetsource: SpreadsheetID required")
	}
	if opts.GroupsGID == "" {
		return nil, fmt.Errorf("sheetsource: GroupsGID required")
	}
	if opts.BracketGID == "" {
		return nil, fmt.Errorf("sheetsource: BracketGID required")
	}

	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &SheetSource{
		httpClient:    hc,
		spreadsheetID: opts.SpreadsheetID,
		groupsGID:     opts.GroupsGID,
		bracketGID:    opts.BracketGID,
		scheduleGID:   opts.ScheduleGID,
		tournamentID:  opts.TournamentID,
		logger:        logger,
	}, nil
}

// FetchMatches fetches the Groups and Bracket tabs, merges the parsed matches,
// then overlays real start times from the Overall Schedule tab.
//
// A failure on the Groups or Bracket tab fails the whole call — partial data
// would leave the UI inconsistent. A failure on the Schedule tab is logged and
// ignored: matches keep day-granularity ScheduledAt, and the locking layer
// falls back to per-match locking for any day without real times.
func (s *SheetSource) FetchMatches(ctx context.Context) ([]models.Match, error) {
	groupsCSV, err := s.fetchCSV(ctx, s.groupsGID)
	if err != nil {
		return nil, fmt.Errorf("groups fetch: %w", err)
	}
	groupMatches, err := parseGroupsCSV(groupsCSV, s.tournamentID)
	if err != nil {
		return nil, fmt.Errorf("groups parse: %w", err)
	}

	bracketCSV, err := s.fetchCSV(ctx, s.bracketGID)
	if err != nil {
		return nil, fmt.Errorf("bracket fetch: %w", err)
	}
	bracketMatches, err := parseBracketCSV(bracketCSV, s.tournamentID)
	if err != nil {
		return nil, fmt.Errorf("bracket parse: %w", err)
	}

	out := make([]models.Match, 0, len(groupMatches)+len(bracketMatches))
	out = append(out, groupMatches...)
	out = append(out, bracketMatches...)

	s.applyScheduleOverlay(ctx, out)

	return out, nil
}

// applyScheduleOverlay best-effort fetches the Overall Schedule tab and
// replaces day-granularity ScheduledAt values with real published start times
// where available. Non-fatal: any failure is logged and the matches are left
// with their day-granularity times.
func (s *SheetSource) applyScheduleOverlay(ctx context.Context, matches []models.Match) {
	if s.scheduleGID == "" {
		return
	}

	scheduleCSV, err := s.fetchCSV(ctx, s.scheduleGID)
	if err != nil {
		s.logger.Warn("schedule tab fetch failed; using day-granularity start times", "err", err)
		return
	}

	startByID, err := parseScheduleCSV(scheduleCSV, s.tournamentID)
	if err != nil {
		s.logger.Warn("schedule tab parse failed; using day-granularity start times", "err", err)
		return
	}

	applied := 0
	for i := range matches {
		if start, ok := startByID[matches[i].ID]; ok {
			start := start // capture for &
			matches[i].ScheduledAt = &start
			applied++
		}
	}
	s.logger.Debug("applied published start times from schedule tab",
		"matched", applied, "total", len(matches))
}

// fetchCSV pulls one tab as CSV from the docs.google.com export endpoint.
// Returns the parsed CSV rows (variable row length — google leaves trailing
// empty cells truncated in some rows).
func (s *SheetSource) fetchCSV(ctx context.Context, gid string) ([][]string, error) {
	url := fmt.Sprintf(
		"https://docs.google.com/spreadsheets/d/%s/export?format=csv&gid=%s",
		s.spreadsheetID, gid,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("docs.google.com returned %d: %s", resp.StatusCode, string(body))
	}

	r := csv.NewReader(resp.Body)
	r.FieldsPerRecord = -1 // allow variable-width rows
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("csv read: %w", err)
	}
	return rows, nil
}

// teamNameOverrides maps sheet-display team names to canonical DB names.
// Empty (or missing) entries pass through the sheet name unchanged. Edit
// here when the broadcast LOP renames a team — these are the only names that
// need an explicit bridge.
//
// Three are documented upstream as known to drift between sheet and DB
// conventions; the rest of the 16-team field passes through verbatim. If a
// new mismatch appears mid-tournament, log evidence and add it here.
var teamNameOverrides = map[string]string{
	"Wildcard Gaming": "Wildcard",
	"Made In Brazil":  "MIBR",
	"Manchester City": "Manchester City Esports",
}

// normalizeTeamName runs the sheet name through the override map. Trims
// whitespace; passes through unmodified if not in the map.
func normalizeTeamName(sheetName string) string {
	n := strings.TrimSpace(sheetName)
	if mapped, ok := teamNameOverrides[n]; ok {
		return mapped
	}
	return n
}

// isResolvedTeamCell reports whether a (team_name, br_id) cell pair represents
// a real (resolved) team rather than a bracket placeholder. The rule: br_id
// must parse to an integer in [1, 16]. Placeholders carry br_id "#N/A" (for
// "Group X Y") or a string echo ("Winner of A").
func isResolvedTeamCell(brID string) bool {
	n, err := strconv.Atoi(strings.TrimSpace(brID))
	if err != nil {
		return false
	}
	return n >= 1 && n <= 16
}

// looksLikePlaceholderTeamName reports whether a team-name cell holds the
// sheet's "team not resolved yet" display text rather than an actual team
// name. Used as a fallback by the bracket parser when br_id is unresolved
// but the name cell already holds a real team — a sheet-template glitch
// where the team-name formula resolved while the br_id helper formula did
// not.
//
// Known placeholder shapes from the bracket template: stage-1 standings
// like "Group A First" / "Group B Second" / "Group D Third"; bracket
// references like "Winner of A" / "Loser of B"; formula-error values like
// "#N/A" or "#REF!"; the empty string.
func looksLikePlaceholderTeamName(name string) bool {
	s := strings.TrimSpace(name)
	if s == "" || strings.HasPrefix(s, "#") {
		return true
	}
	lower := strings.ToLower(s)
	return strings.HasPrefix(lower, "winner of ") ||
		strings.HasPrefix(lower, "loser of ") ||
		strings.HasPrefix(lower, "group ")
}

// dayDates maps the sheet's "Day N" prefix to a calendar date for the match.
// The intra-day position ("2A", "5B") is kept separately in models.Match.Slot.
//
// Dates are RFC3339 at midnight UTC. This is the FALLBACK granularity: when
// the Overall Schedule tab has a published wall-clock start for a match,
// applyScheduleOverlay replaces ScheduledAt with that real time. A match left
// at 00:00:00 UTC therefore means "start time not yet published" — the
// locking layer treats midnight UTC as that sentinel.
var dayDates = map[string]string{
	"Day 1": "2026-05-20T00:00:00Z",
	"Day 2": "2026-05-21T00:00:00Z",
	"Day 3": "2026-05-22T00:00:00Z",
	"Day 4": "2026-05-23T00:00:00Z",
	"Day 5": "2026-05-24T00:00:00Z",
}

// parseSlotString splits a slot string like "Day 1 2A" into ("Day 1", "2A").
// Returns ("", "") if the format doesn't match. Whitespace-flexible so an
// extra space ("Day 1  2A") still parses.
func parseSlotString(s string) (day, pos string) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "Day ") {
		return "", ""
	}
	parts := strings.Fields(s)
	if len(parts) < 3 {
		return "", ""
	}
	// "Day", "N", "XY" → day="Day N", pos="XY"
	return parts[0] + " " + parts[1], parts[2]
}

// scheduledAtFromSlot resolves the date portion of a slot string to an
// RFC3339 timestamp via dayDates. Returns nil if the slot string doesn't
// match a known day prefix.
func scheduledAtFromSlot(slot string) *string {
	day, _ := parseSlotString(slot)
	if day == "" {
		return nil
	}
	ts, ok := dayDates[day]
	if !ok {
		return nil
	}
	return &ts
}

// slotPosFromSlot returns just the intra-day position ("2A") from a slot
// string, or nil if absent. Stored on models.Match.Slot so the broadcaster
// can sort matches by their on-day order.
func slotPosFromSlot(slot string) *string {
	_, pos := parseSlotString(slot)
	if pos == "" {
		return nil
	}
	return &pos
}

// winThreshold returns the score that completes a match for a given stage.
// Group stage is Bo5 (first to 3); bracket is Bo7 (first to 4).
func winThreshold(stage string) int {
	if stage == models.StageBracket {
		return 4
	}
	return 3
}

// computeSheetStatus derives status + winner from the two scores. Unlike
// the Liquipedia rule (where nil vs 0 is meaningful), sheet scores are
// always present as integers, so completion is detected by max(a,b)
// reaching the stage's win threshold.
//
//	both 0           → upcoming
//	max >= threshold → completed, winner = higher
//	otherwise        → live (Bo started but neither side has clinched)
func computeSheetStatus(scoreA, scoreB int, stage string) (status string, winner *string) {
	if scoreA == 0 && scoreB == 0 {
		return models.StatusUpcoming, nil
	}
	thresh := winThreshold(stage)
	switch {
	case scoreA >= thresh && scoreA > scoreB:
		w := models.PickA
		return models.StatusCompleted, &w
	case scoreB >= thresh && scoreB > scoreA:
		w := models.PickB
		return models.StatusCompleted, &w
	default:
		return models.StatusLive, nil
	}
}

// sheetMatchID hashes (tournament_id, stage, label) into the 16-char hex id.
// Stable across the placeholder→resolved-team transition because the label
// (A–X for groups, A–M for bracket) doesn't change.
func sheetMatchID(tournamentID int, stage, label string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%s", tournamentID, stage, label)))
	return hex.EncodeToString(h[:8])
}

// cellAt returns the cell at (row, col) or "" if the row is too short. Google
// Sheets export trims trailing empty columns per row, so out-of-range access
// is normal, not an error.
func cellAt(rows [][]string, row, col int) string {
	if row < 0 || row >= len(rows) {
		return ""
	}
	r := rows[row]
	if col < 0 || col >= len(r) {
		return ""
	}
	return r[col]
}

// parseInt0 parses s as an int, returning 0 if blank or unparseable. Used
// for sheet scores where "0" is the default-empty value.
func parseInt0(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}

// ptrStr returns &s if s is non-empty after trim, else nil. Helper for the
// pointer-string fields on models.Match.
func ptrStr(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}
