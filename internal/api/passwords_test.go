package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jometheuspondo/rlcs-predictions/internal/db"
	"github.com/jometheuspondo/rlcs-predictions/internal/models"
)

func TestPasswordReset(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(filepath.Join(t.TempDir(), "passwords.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := database.Migrate(ctx, logger); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO participants(id,display_name,password) VALUES ('talent','Talent','old'),('other','Other','unchanged')`); err != nil {
		t.Fatal(err)
	}

	event, err := database.ActivateTournament(ctx, "worlds", "Worlds", "America/Chicago")
	if err != nil {
		t.Fatal(err)
	}
	round, err := database.UpsertRound(ctx, event.ID, models.StageGroup, "Group", 100)
	if err != nil {
		t.Fatal(err)
	}
	match := models.Match{ID: "match", TeamA: "A", TeamB: "B", Status: models.StatusUpcoming, BestOf: 5}
	if err := database.UpsertMatch(ctx, &match, round); err != nil {
		t.Fatal(err)
	}
	if err := database.ForTournament(event.ID).SetPrediction(ctx, "talent", "match", "A"); err != nil {
		t.Fatal(err)
	}
	if err := database.ForTournament(event.ID).AddWinnerPick(ctx, "talent", "A"); err != nil {
		t.Fatal(err)
	}

	router := NewRouter(Deps{DB: database, Tournament: event, Logger: logger})
	request := func(path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	resetBody := func(id, password string) string {
		body, err := json.Marshal(resetPasswordReq{ParticipantID: id, NewPassword: password})
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}

	newPassword := "  anything '雪'  "
	response := request("/api/reset-password", resetBody("talent", newPassword))
	if response.Code != http.StatusNoContent || response.Body.Len() != 0 {
		t.Fatalf("reset failed: %d %s", response.Code, response.Body.String())
	}
	stored, err := database.GetParticipantPassword(ctx, "talent")
	if err != nil || stored != newPassword {
		t.Fatalf("password was not stored verbatim: %v", err)
	}
	other, err := database.GetParticipantPassword(ctx, "other")
	if err != nil || other != "unchanged" {
		t.Fatal("reset changed another account")
	}

	for _, table := range []string{"predictions", "winner_pick_history"} {
		var count int
		if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE participant_id = ?", "talent").Scan(&count); err != nil || count != 1 {
			t.Fatalf("reset altered %s: %v", table, err)
		}
	}
	for _, test := range []struct {
		password string
		status   int
	}{{"old", http.StatusUnauthorized}, {newPassword, http.StatusOK}} {
		body, err := json.Marshal(loginReq{ParticipantID: "talent", Password: test.password})
		if err != nil {
			t.Fatal(err)
		}
		if response := request("/api/login", string(body)); response.Code != test.status {
			t.Fatalf("login after reset: %d", response.Code)
		}
	}

	for _, test := range []struct {
		name   string
		body   string
		status int
	}{
		{"empty", resetBody("talent", ""), http.StatusBadRequest},
		{"whitespace", resetBody("talent", " \t "), http.StatusBadRequest},
		{"too long", resetBody("talent", strings.Repeat("x", 1025)), http.StatusBadRequest},
		{"missing account", resetBody("", "new"), http.StatusBadRequest},
		{"unknown account", resetBody("missing", "new"), http.StatusNotFound},
		{"invalid JSON", "{", http.StatusBadRequest},
		{"extra object", resetBody("talent", "new") + "{}", http.StatusBadRequest},
		{"unknown field", `{"participant_id":"talent","new_password":"new","password":"old"}`, http.StatusBadRequest},
		{"oversize body", resetBody("talent", strings.Repeat("x", 8192)), http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			if response := request("/api/reset-password", test.body); response.Code != test.status {
				t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
			}
			stored, err := database.GetParticipantPassword(ctx, "talent")
			if err != nil || stored != newPassword {
				t.Fatal("invalid reset changed the password")
			}
		})
	}
}
