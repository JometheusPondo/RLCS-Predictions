package db

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/jometheuspondo/rlcs-predictions/internal/models"
)

func TestMigrationPreservesMajorAndSeparatesWorlds(t *testing.T) {
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "events.db")
	database, err := Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if _, err = database.ExecContext(ctx, `CREATE TABLE schema_migrations (filename TEXT PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT (datetime('now')))`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"001_init.sql", "002_auth_and_winner_picks.sql", "003_blast_admin_account.sql", "004_sheet_source_fields.sql"} {
		if err = database.applyMigration(ctx, name); err != nil {
			t.Fatal(err)
		}
	}
	for _, query := range []string{
		`INSERT INTO participants (id, display_name, password) VALUES ('talent', 'Talent', 'unchanged'), ('the-coin','The Coin','coin'), ('chat','Chat','chat')`,
		`INSERT INTO tournaments (id, liquipedia_page, name) VALUES (1, 'Rocket_League_Championship_Series/2026/Paris_Major', 'Paris Major')`,
		`INSERT INTO rounds (id,tournament_id,stage,sort_order,name) VALUES (1,1,'bracket',1000,'Final')`,
		`INSERT INTO matches (id,round_id,team_a,team_b,team_a_score,team_b_score,winner,status,scheduled_at) VALUES ('major-final',1,'Old Team A','Old Team B',4,3,'A','completed','2026-05-24T16:00:00Z')`,
		`INSERT INTO predictions (participant_id,match_id,pick) VALUES ('talent','major-final','A')`,
		`INSERT INTO winner_pick_history (participant_id,team_name,picked_at) VALUES ('talent','Old Team A','2026-05-19 12:00:00')`,
	} {
		if _, err = database.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}

	backup, err := database.BackupBeforeEventMigration(ctx, databasePath)
	if err != nil || backup == "" {
		t.Fatalf("backup failed: %s %v", backup, err)
	}
	backupDB, err := sql.Open("sqlite", backup+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	var savedPredictions int
	if err = backupDB.QueryRowContext(ctx, `SELECT count(*) FROM predictions`).Scan(&savedPredictions); err != nil || savedPredictions != 1 {
		t.Fatal("backup did not preserve predictions")
	}
	backupDB.Close()

	if err = database.Migrate(ctx, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	worlds, err := database.ActivateTournament(ctx, "worlds", "World Championship 2026", "America/Chicago")
	if err != nil {
		t.Fatal(err)
	}
	majorStore, worldsStore := database.ForTournament(1), database.ForTournament(worlds.ID)
	major, err := majorStore.GetParticipantWithPredictions(ctx, "talent")
	if err != nil {
		t.Fatal(err)
	}
	if major.Score != 4 || major.CorrectCount != 1 || len(major.Predictions) != 1 || len(major.WinnerPicks) != 1 || major.WinnerPicks[0].TeamName != "Old Team A" {
		t.Fatalf("Major history changed: %+v", major)
	}
	current, err := worldsStore.GetParticipantWithPredictions(ctx, "talent")
	if err != nil {
		t.Fatal(err)
	}
	if current.Score != 0 || len(current.Predictions) != 0 || len(current.WinnerPicks) != 0 {
		t.Fatalf("history leaked into Worlds: %+v", current)
	}
	password, err := database.GetParticipantPassword(ctx, "talent")
	if err != nil || password != "unchanged" {
		t.Fatal("account was reset")
	}

	round, err := database.UpsertRound(ctx, worlds.ID, models.StageGroup, "Groups", 100)
	if err != nil {
		t.Fatal(err)
	}
	date, start := "2026-09-15", "2026-09-15T16:00:00Z"
	match := models.Match{ID: "worlds-match", TeamA: "New Team A", TeamB: "New Team B", Status: models.StatusUpcoming, BestOf: 5, Round: models.Round{Stage: models.StageGroup}}
	if err = database.UpsertMatch(ctx, &match, round); err != nil {
		t.Fatal(err)
	}
	if err = worldsStore.AddWinnerPick(ctx, "talent", "New Team A"); err != nil {
		t.Fatalf("Major lock affected Worlds champion choice: %v", err)
	}
	match.Status, match.EventDate, match.ScheduledAt = models.StatusLive, &date, &start
	if err = database.UpsertMatch(ctx, &match, round); err != nil {
		t.Fatal(err)
	}
	if err = worldsStore.SetPrediction(ctx, "talent", match.ID, "A"); !errors.Is(err, ErrPredictionsLocked) {
		t.Fatalf("normal account bypassed lock: %v", err)
	}
	for _, id := range []string{"the-coin", "chat"} {
		if err = worldsStore.SetPrediction(ctx, id, match.ID, "A"); err != nil {
			t.Fatalf("%s lost exemption: %v", id, err)
		}
		if err = worldsStore.DeletePrediction(ctx, id, match.ID); err != nil {
			t.Fatal(err)
		}
		if err = majorStore.SetPrediction(ctx, id, "major-final", "B"); !errors.Is(err, ErrEventReadOnly) {
			t.Fatalf("%s can edit archive: %v", id, err)
		}
		if err = majorStore.AddWinnerPick(ctx, id, "Old Team B"); !errors.Is(err, ErrEventReadOnly) {
			t.Fatal("archive champion pick is writable")
		}
	}
	if err = worldsStore.SetPrediction(ctx, "the-coin", "major-final", "A"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-event write allowed: %v", err)
	}
	match.Status = models.StatusCompleted
	if err = database.UpsertMatch(ctx, &match, round); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"the-coin", "chat"} {
		if err = worldsStore.SetPrediction(ctx, id, match.ID, "B"); err != nil {
			t.Fatalf("%s cannot correct a completed pick: %v", id, err)
		}
	}
	teams, err := worldsStore.ListTeamNames(ctx)
	if err != nil || len(teams) != 2 || teams[0] != "New Team A" {
		t.Fatalf("team list mixed events: %v %v", teams, err)
	}

	if err = database.Migrate(ctx, slog.Default()); err != nil {
		t.Fatal(err)
	}
	if backup, err = database.BackupBeforeEventMigration(ctx, databasePath); err != nil || backup != "" {
		t.Fatal("restart repeated backup")
	}
	again, err := database.ActivateTournament(ctx, "worlds", "World Championship 2026", "America/Chicago")
	if err != nil || again.ID != worlds.ID {
		t.Fatal("restart created another event")
	}
	events, err := database.ListTournaments(ctx)
	if err != nil || len(events) != 2 || !events[0].IsActive || events[1].IsActive {
		t.Fatal("active event state invalid")
	}
	var count int
	if err = database.QueryRowContext(ctx, `SELECT count(*) FROM predictions WHERE match_id = 'major-final'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("Major prediction was lost")
	}
}
