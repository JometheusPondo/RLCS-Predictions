package db

import (
	"context"
	"errors"

	"github.com/jometheuspondo/rlcs-predictions/internal/models"
)

var ErrPicksHidden = errors.New("match predictions are hidden until lock")

type MatchPicker struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Pick        string `json:"pick"`
}

// ListMatchPickers reveals only a locked match's picks within the selected event.
func (db *EventStore) ListMatchPickers(ctx context.Context, matchID string) ([]MatchPicker, error) {
	matches, err := db.ListMatches(ctx)
	if err != nil {
		return nil, err
	}
	found := false
	for _, match := range matches {
		if match.ID == matchID {
			if !match.Locked {
				return nil, ErrPicksHidden
			}
			found = true
			break
		}
	}
	if !found {
		return nil, ErrNotFound
	}

	rows, err := db.QueryContext(ctx, `
		SELECT p.id, p.display_name, pr.pick
		FROM predictions pr JOIN participants p ON p.id = pr.participant_id
		JOIN matches m ON m.id = pr.match_id JOIN rounds r ON r.id = m.round_id
		WHERE m.id = ? AND r.tournament_id = ? AND p.id != ?
		ORDER BY p.display_name COLLATE NOCASE, p.id`, matchID, db.tournamentID, models.AdminID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	pickers := make([]MatchPicker, 0)
	for rows.Next() {
		var picker MatchPicker
		if err := rows.Scan(&picker.ID, &picker.DisplayName, &picker.Pick); err != nil {
			return nil, err
		}
		pickers = append(pickers, picker)
	}
	return pickers, rows.Err()
}
