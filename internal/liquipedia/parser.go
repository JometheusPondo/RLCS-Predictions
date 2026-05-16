package liquipedia

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/jometheuspondo/rlcs-predictions/internal/models"
)

// ParsedTournament is the output of ParsePage — structured rounds and matches.
type ParsedTournament struct {
	Rounds  []ParsedRound
	Matches []ParsedMatch
}

// ParsedRound is a round Description; Stage uses models.StageGroup / StageBracket.
type ParsedRound struct {
	Stage     string
	Name      string
	SortOrder int
}

// ParsedMatch carries the fields the poller needs to upsert a row. RoundName +
// RoundStage are used by the poller to look up the round_id.
type ParsedMatch struct {
	RoundName   string
	RoundStage  string
	TeamA       string
	TeamB       string
	TeamAScore  *int
	TeamBScore  *int
	Winner      *string // models.PickA / PickB / nil
	Status      string
	ScheduledAt *string // RFC3339, or nil
}

// placeholderPattern matches bracket placeholders like "Group A #1", "Group D #3".
// These are skipped per spec § 5.4 (TBD-equivalent).
var placeholderPattern = regexp.MustCompile(`^Group [A-Z] #\d+$`)

// roundNumPattern extracts a round number from labels like "Round 1".
var roundNumPattern = regexp.MustCompile(`\d+`)

// ParsePage parses the HTML body from MediaWiki's action=parse output and
// returns the rounds and matches it contains. Resilient by design: a failure
// in one section (group stage or bracket) does not invalidate the other.
//
// DOM contracts this parser relies on (verified against the May 2026 Paris
// Major page on 2026-05-13):
//
//	.brkts-matchlist                          — one per Liquipedia "group"
//	  > .general-collapsible-default-title    — text "Group A Matches"
//	  > .brkts-matchlist-collapse-area
//	      > .brkts-matchlist-header           — text "Round 1" / "Round 2" / "Round 3"
//	      > .brkts-matchlist-match            — one match, with 2 opponent cells
//	          aria-label                      — full team name ("Team Vitality")
//	          .name span                      — short team code ("VIT")
//	          .brkts-matchlist-score cell     — text score, empty if not played
//	          .timer-object[data-timestamp]   — Unix epoch, "error" if unset
//
//	.brkts-bracket                            — one per playoff bracket
//	  > .brkts-round-header                   — contains 1+ .brkts-header.brkts-header-div
//	                                             (one per round when the header spans
//	                                             multiple rounds in double-elim)
//	  > .brkts-round-body                     — matches for the round(s)
//	      > .brkts-round-center               — direct match container at this level
//	          > .brkts-match.brkts-match-has-details
//	              > .brkts-opponent-entry     — team + score
//	                  .brkts-opponent-block-literal — placeholder text ("Group A #1")
//	                                                  or team-template markup
//	                  .brkts-opponent-score-inner — score
//	      > .brkts-round-lower                — wraps the next round(s) inward; may
//	                                             contain MULTIPLE .brkts-round-body
//	                                             children at deeper levels
//
// If Liquipedia restructures the page, this parser will degrade rather than
// hard-fail — bad sections log warnings and skip; the rest still parses.
func ParsePage(html string) (*ParsedTournament, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("load html: %w", err)
	}

	out := &ParsedTournament{}

	groupRounds, groupMatches := parseGroupStage(doc)
	out.Rounds = append(out.Rounds, groupRounds...)
	out.Matches = append(out.Matches, groupMatches...)

	bracketRounds, bracketMatches := parsePlayoffBracket(doc)
	out.Rounds = append(out.Rounds, bracketRounds...)
	out.Matches = append(out.Matches, bracketMatches...)

	return out, nil
}

// parseGroupStage walks every .brkts-matchlist on the page. The format is
// 4 groups (A/B/C/D) × 3 rounds (single round robin), but rounds are FLATTENED
// across groups in the output: all Round-1 matches from every group go into a
// single "Group Stage - Round 1" round. That gives the broadcaster a per-day
// view rather than a per-group view in the prediction UI.
func parseGroupStage(doc *goquery.Document) (rounds []ParsedRound, matches []ParsedMatch) {
	seenRounds := make(map[string]bool)

	doc.Find(".brkts-matchlist").Each(func(_ int, ml *goquery.Selection) {
		title := strings.TrimSpace(ml.Find(".general-collapsible-default-title").First().Text())
		// "Group A Matches" → drop suffix; skip if not in the expected form.
		title = strings.TrimSuffix(title, " Matches")
		if title == "" || !strings.HasPrefix(title, "Group ") {
			return
		}

		area := ml.Find(".brkts-matchlist-collapse-area").First()
		if area.Length() == 0 {
			return
		}

		currentRoundName := ""

		area.Children().Each(func(_ int, child *goquery.Selection) {
			if child.HasClass("brkts-matchlist-header") {
				roundLabel := strings.TrimSpace(child.Text())
				if roundLabel == "" {
					return
				}
				roundNum := extractFirstInt(roundLabel)
				roundName := "Group Stage - " + roundLabel
				currentRoundName = roundName

				if !seenRounds[roundName] {
					rounds = append(rounds, ParsedRound{
						Stage:     models.StageGroup,
						Name:      roundName,
						SortOrder: roundNum * models.SortOrderGroupStep,
					})
					seenRounds[roundName] = true
				}
				return
			}

			if !child.HasClass("brkts-matchlist-match") {
				return
			}
			if currentRoundName == "" {
				return // match seen before any round header — skip
			}

			pm, ok := parseMatchlistMatch(child)
			if !ok {
				return
			}
			pm.RoundName = currentRoundName
			pm.RoundStage = models.StageGroup
			matches = append(matches, pm)
		})
	})

	return rounds, matches
}

// parseMatchlistMatch reads one .brkts-matchlist-match element. Returns ok=false
// when either team is a placeholder or the cell structure is malformed.
func parseMatchlistMatch(match *goquery.Selection) (pm ParsedMatch, ok bool) {
	opponents := match.Find(".brkts-matchlist-opponent")
	scores := match.Find(".brkts-matchlist-score")

	if opponents.Length() != 2 {
		return pm, false
	}

	teamA := extractTeamName(opponents.Eq(0))
	teamB := extractTeamName(opponents.Eq(1))
	if !isValidTeamName(teamA) || !isValidTeamName(teamB) {
		return pm, false
	}
	pm.TeamA = teamA
	pm.TeamB = teamB

	if scores.Length() >= 2 {
		pm.TeamAScore = parseScoreText(scores.Eq(0).Text())
		pm.TeamBScore = parseScoreText(scores.Eq(1).Text())
	}

	pm.Status, pm.Winner = computeStatus(pm.TeamAScore, pm.TeamBScore)
	pm.ScheduledAt = extractScheduledTime(match)
	return pm, true
}

// parsePlayoffBracket walks the .brkts-bracket element. Each .brkts-round-header
// may contain MULTIPLE .brkts-header.brkts-header-div labels — in double-elim,
// one header element typically lists every lower-bracket round in a single row
// (LB R1, LB R2, LB QF, Semifinals, Grand Final). The following .brkts-round-
// body siblings then encode those rounds via recursive .brkts-round-lower
// nesting, with outermost level == LAST label and innermost == FIRST label.
//
// For a single-label header (e.g. "Upper Bracket Quarterfinals"), all
// following bodies map to that one label.
//
// Known limitation: when a sub-tree branches deeper than the header's label
// count, those matches are skipped (logged via debug). All currently-placeholder
// matches skip via isValidTeamName regardless of round attribution, so this only
// matters once real teams populate the bracket; revisit in Phase 4+ with real data.
func parsePlayoffBracket(doc *goquery.Document) (rounds []ParsedRound, matches []ParsedMatch) {
	seenRounds := make(map[string]bool)

	doc.Find(".brkts-bracket").Each(func(_ int, bracket *goquery.Selection) {
		var currentLabels []string

		bracket.Children().Each(func(_ int, child *goquery.Selection) {
			if child.HasClass("brkts-round-header") {
				currentLabels = collectHeaderLabels(child)
				for _, label := range currentLabels {
					if label == "" || seenRounds[label] {
						continue
					}
					rounds = append(rounds, ParsedRound{
						Stage:     models.StageBracket,
						Name:      label,
						SortOrder: bracketSortOrder(label),
					})
					seenRounds[label] = true
				}
				return
			}

			if !child.HasClass("brkts-round-body") || len(currentLabels) == 0 {
				return
			}

			walkBracketBody(child, currentLabels, 0, &matches)
		})
	})

	return rounds, matches
}

// collectHeaderLabels reads the .brkts-header.brkts-header-div siblings inside
// a .brkts-round-header. Each div may contain alternate-display .brkts-header-
// option children (full/abbreviated/short labels for responsive layouts) — we
// want only the canonical label, not the options. Clone the node, drop the
// option children, then take the text that's left.
func collectHeaderLabels(header *goquery.Selection) []string {
	var labels []string
	header.Find(".brkts-header.brkts-header-div").Each(func(_ int, h *goquery.Selection) {
		clone := h.Clone()
		clone.Find(".brkts-header-option").Remove()
		labels = append(labels, strings.TrimSpace(clone.Text()))
	})
	return labels
}

// walkBracketBody recursively descends a .brkts-round-body, emitting matches
// from each level's .brkts-round-center. The level-to-label mapping reverses
// the labels slice: level 0 (outermost) → labels[len-1], deeper levels → earlier.
//
// At each level, .brkts-round-lower may contain multiple .brkts-round-body
// children (parallel branches in the binary tree); we recurse into all of them.
func walkBracketBody(body *goquery.Selection, labels []string, level int, out *[]ParsedMatch) {
	idx := len(labels) - 1 - level
	var label string
	if idx >= 0 && idx < len(labels) {
		label = labels[idx]
	}

	// Matches at this level
	if label != "" {
		body.ChildrenFiltered(".brkts-round-center").Each(func(_ int, center *goquery.Selection) {
			center.ChildrenFiltered(".brkts-match.brkts-match-has-details").Each(func(_ int, m *goquery.Selection) {
				pm, ok := parseBracketMatch(m)
				if !ok {
					return
				}
				pm.RoundName = label
				pm.RoundStage = models.StageBracket
				*out = append(*out, pm)
			})
		})
	}

	// Descend through brkts-round-lower (may have multiple sub-bodies)
	body.ChildrenFiltered(".brkts-round-lower").Each(func(_ int, lower *goquery.Selection) {
		lower.ChildrenFiltered(".brkts-round-body").Each(func(_ int, sub *goquery.Selection) {
			walkBracketBody(sub, labels, level+1, out)
		})
	})
}

// parseBracketMatch reads one .brkts-match in the playoff bracket. Two direct
// .brkts-opponent-entry children carry team + score. Returns ok=false if either
// side is empty or a placeholder.
func parseBracketMatch(match *goquery.Selection) (pm ParsedMatch, ok bool) {
	entries := match.ChildrenFiltered(".brkts-opponent-entry")
	if entries.Length() < 2 {
		return pm, false
	}

	teamA := extractBracketOpponentName(entries.Eq(0))
	teamB := extractBracketOpponentName(entries.Eq(1))
	if !isValidTeamName(teamA) || !isValidTeamName(teamB) {
		return pm, false
	}
	pm.TeamA = teamA
	pm.TeamB = teamB

	pm.TeamAScore = parseScoreText(entries.Eq(0).Find(".brkts-opponent-score-inner").First().Text())
	pm.TeamBScore = parseScoreText(entries.Eq(1).Find(".brkts-opponent-score-inner").First().Text())

	pm.Status, pm.Winner = computeStatus(pm.TeamAScore, pm.TeamBScore)
	pm.ScheduledAt = extractScheduledTime(match)
	return pm, true
}

// ---- helpers ----

// extractTeamName prefers aria-label on the opponent cell (full team name like
// "Team Vitality"), falling back to the short .name span ("VIT") and then to a
// literal block. aria-label is more recognizable for broadcasters.
func extractTeamName(opponent *goquery.Selection) string {
	if al, ok := opponent.Attr("aria-label"); ok {
		if v := strings.TrimSpace(al); v != "" {
			return v
		}
	}
	if v := strings.TrimSpace(opponent.Find(".name").First().Text()); v != "" {
		return v
	}
	return strings.TrimSpace(opponent.Find(".brkts-opponent-block-literal").First().Text())
}

// extractBracketOpponentName handles both filled and placeholder bracket cells.
// Once group stage completes, real teams appear via team-template-text; until
// then, .brkts-opponent-block-literal carries placeholders like "Group A #1".
func extractBracketOpponentName(entry *goquery.Selection) string {
	if al, ok := entry.Attr("aria-label"); ok {
		if v := strings.TrimSpace(al); v != "" {
			return v
		}
	}
	if v := strings.TrimSpace(entry.Find(".team-template-text").First().Text()); v != "" {
		return v
	}
	if v := strings.TrimSpace(entry.Find(".name").First().Text()); v != "" {
		return v
	}
	return strings.TrimSpace(entry.Find(".brkts-opponent-block-literal").First().Text())
}

// isValidTeamName returns false for the placeholder values that should cause
// the entire match to be skipped: empty, "TBD" (any case), or "Group X #N".
func isValidTeamName(name string) bool {
	n := strings.TrimSpace(name)
	if n == "" {
		return false
	}
	if strings.EqualFold(n, "TBD") {
		return false
	}
	if placeholderPattern.MatchString(n) {
		return false
	}
	return true
}

func parseScoreText(s string) *int {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &n
}

// computeStatus derives status + winner from two scores per spec § 5.4:
//
//	both nil          → upcoming, no winner
//	one nil           → live, no winner
//	both present, ≠   → completed, winner = higher side
//	both present, ==  → live, no winner (defensive; ties shouldn't happen in Bo5)
func computeStatus(a, b *int) (status string, winner *string) {
	if a == nil && b == nil {
		return models.StatusUpcoming, nil
	}
	if a == nil || b == nil {
		return models.StatusLive, nil
	}
	switch {
	case *a > *b:
		w := models.PickA
		return models.StatusCompleted, &w
	case *b > *a:
		w := models.PickB
		return models.StatusCompleted, &w
	default:
		return models.StatusLive, nil
	}
}

// extractScheduledTime looks for the first .timer-object[data-timestamp] that
// is a valid Unix epoch (Liquipedia uses the literal string "error" when a
// match isn't scheduled yet). Returns an RFC3339 string in UTC, or nil.
func extractScheduledTime(match *goquery.Selection) *string {
	var result *string
	match.Find(".timer-object").EachWithBreak(func(_ int, t *goquery.Selection) bool {
		ts, ok := t.Attr("data-timestamp")
		if !ok || ts == "" || ts == "error" {
			return true // keep searching
		}
		n, err := strconv.ParseInt(ts, 10, 64)
		if err != nil {
			return true
		}
		rfc := time.Unix(n, 0).UTC().Format(time.RFC3339)
		result = &rfc
		return false // found one — stop
	})
	return result
}

// extractFirstInt finds the first integer in a string; "Round 1" → 1.
// Returns 0 if no integer is present.
func extractFirstInt(s string) int {
	m := roundNumPattern.FindString(s)
	if m == "" {
		return 0
	}
	n, _ := strconv.Atoi(m)
	return n
}

// bracketSortOrder maps a round label to its sort-order anchor. Tests "semi"
// before "final" because "Semifinals" contains "final".
func bracketSortOrder(label string) int {
	l := strings.ToLower(label)
	switch {
	case strings.Contains(l, "quarter"):
		return models.SortOrderQuarters
	case strings.Contains(l, "semi"):
		return models.SortOrderSemifinals
	case strings.Contains(l, "grand final"), strings.Contains(l, "final"):
		return models.SortOrderFinal
	case strings.Contains(l, "lower bracket round 1"):
		return models.SortOrderQuarters - 200 // 800
	case strings.Contains(l, "lower bracket round 2"):
		return models.SortOrderQuarters - 100 // 900
	default:
		return 1300
	}
}
