package db

import (
	"context"

	"github.com/jometheuspondo/rlcs-predictions/internal/models"
)

// championParticipants includes accounts with no winner pick and attaches only
// this event's history, ordered so the latest pick is last.
func (db *EventStore) championParticipants(ctx context.Context) ([]models.Participant, error) {
	history, err := db.allWinnerPicks(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT id FROM participants WHERE id NOT IN (?, ?)`, models.AdminID, models.OwnerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var participants []models.Participant
	for rows.Next() {
		var p models.Participant
		if err := rows.Scan(&p.ID); err != nil {
			return nil, err
		}
		p.WinnerPicks = history[p.ID]
		participants = append(participants, p)
	}
	return participants, rows.Err()
}
