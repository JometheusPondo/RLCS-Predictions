package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jometheuspondo/rlcs-predictions/internal/models"
)

var ErrEventReadOnly = errors.New("past events are read-only")
var ErrTeamsUnresolved = errors.New("both teams must be confirmed before predicting")

// BackupBeforeEventMigration creates a consistent SQLite backup once, before upgrading an existing installation.
func (db *DB) BackupBeforeEventMigration(ctx context.Context, path string) (string, error) {
	var exists int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'`).Scan(&exists); err != nil {
		return "", err
	}
	if exists == 0 {
		return "", nil
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE filename = '005_event_history.sql'`).Scan(&exists); err != nil {
		return "", err
	}
	if exists != 0 {
		return "", nil
	}

	backup := fmt.Sprintf("%s.pre-worlds-%s.bak", path, time.Now().UTC().Format("20060102T150405.000000000"))
	if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, backup); err != nil {
		return "", fmt.Errorf("backup before event migration: %w", err)
	}
	return backup, nil
}

type EventStore struct {
	*DB
	tournamentID  int
	ownerOverride bool
}

// ForTournament returns an independent query scope; it never mutates the shared database handle.
func (db *DB) ForTournament(id int) *EventStore {
	return &EventStore{DB: db, tournamentID: id}
}

func (db *DB) ListTournaments(ctx context.Context) ([]models.Tournament, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, liquipedia_page, name, is_active, last_synced_at, timezone FROM tournaments ORDER BY is_active DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.Tournament, 0)
	for rows.Next() {
		var event models.Tournament
		if err := rows.Scan(&event.ID, &event.LiquipediaPage, &event.Name, &event.IsActive, &event.LastSyncedAt, &event.Timezone); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func (db *DB) GetTournament(ctx context.Context, id int) (*models.Tournament, error) {
	var event models.Tournament
	err := db.QueryRowContext(ctx, `SELECT id, liquipedia_page, name, is_active, last_synced_at, timezone FROM tournaments WHERE id = ?`, id).
		Scan(&event.ID, &event.LiquipediaPage, &event.Name, &event.IsActive, &event.LastSyncedAt, &event.Timezone)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &event, err
}

// ActivateTournament switches the live event atomically without deleting any historical rows.
func (db *DB) ActivateTournament(ctx context.Context, page, name, timezone string) (*models.Tournament, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err = tx.ExecContext(ctx, `UPDATE tournaments SET is_active = 0`); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO tournaments (liquipedia_page, name, timezone, is_active) VALUES (?, ?, ?, 1)
        ON CONFLICT(liquipedia_page) DO UPDATE SET name = excluded.name, timezone = excluded.timezone, is_active = 1`, page, name, timezone); err != nil {
		return nil, err
	}

	var id int
	if err = tx.QueryRowContext(ctx, `SELECT id FROM tournaments WHERE liquipedia_page = ?`, page).Scan(&id); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return db.GetTournament(ctx, id)
}

func (db *EventStore) requireActive(ctx context.Context) error {
	event, err := db.GetTournament(ctx, db.tournamentID)
	if err != nil {
		return err
	}
	if !event.IsActive && !db.ownerOverride {
		return ErrEventReadOnly
	}
	return nil
}
