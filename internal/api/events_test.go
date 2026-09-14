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

func TestEventAPIIsolationAndArchiveWriteProtection(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err = database.Migrate(ctx, logger); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO participants(id,display_name) VALUES ('talent','Talent'),('the-coin','The Coin'),('chat','Chat')`); err != nil {
		t.Fatal(err)
	}

	major, err := database.ActivateTournament(ctx, "major", "Major", "Europe/Paris")
	if err != nil {
		t.Fatal(err)
	}
	round, err := database.UpsertRound(ctx, major.ID, models.StageGroup, "Group", 100)
	if err != nil {
		t.Fatal(err)
	}
	match := models.Match{ID: "major", TeamA: "Old A", TeamB: "Old B", Status: models.StatusUpcoming, BestOf: 5}
	if err = database.UpsertMatch(ctx, &match, round); err != nil {
		t.Fatal(err)
	}
	if err = database.ForTournament(major.ID).SetPrediction(ctx, "talent", "major", "A"); err != nil {
		t.Fatal(err)
	}
	worlds, err := database.ActivateTournament(ctx, "worlds", "Worlds", "America/Chicago")
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Deps{DB: database, Tournament: worlds, Logger: logger})

	request := func(method, path, token, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	for _, token := range []string{"talent", "the-coin", "chat", models.AdminID} {
		for _, path := range []string{"/api/participants/" + token + "/predictions/major?event=1", "/api/participants/" + token + "/winner?event=1"} {
			response := request(http.MethodPut, path, token, `{"pick":"B","team_name":"Old B"}`)
			if response.Code != http.StatusForbidden {
				t.Fatalf("archive writable by %s: %d %s", token, response.Code, response.Body.String())
			}
		}
		if response := request(http.MethodDelete, "/api/participants/"+token+"/predictions/major?event=1", token, ""); response.Code != http.StatusForbidden {
			t.Fatal("archive deletion allowed")
		}
	}
	if response := request(http.MethodPut, "/api/participants/the-coin/predictions/major?event=2", "the-coin", `{"pick":"B"}`); response.Code != http.StatusNotFound {
		t.Fatalf("cross-event match accepted: %s", response.Body.String())
	}

	response := request(http.MethodGet, "/api/participants/talent?event=1", "", "")
	var archive models.ParticipantWithPredictions
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &archive) != nil || len(archive.Predictions) != 1 {
		t.Fatalf("archive not readable: %s", response.Body.String())
	}
	response = request(http.MethodGet, "/api/participants/talent?event=2", "", "")
	var current models.ParticipantWithPredictions
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &current) != nil || len(current.Predictions) != 0 {
		t.Fatalf("Worlds leaked history: %s", response.Body.String())
	}
	if response = request(http.MethodGet, "/api/matches", "", ""); response.Body.String() != "[]\n" {
		t.Fatalf("default event not Worlds: %s", response.Body.String())
	}
	if response = request(http.MethodGet, "/api/matches?event=999", "", ""); response.Code != http.StatusNotFound {
		t.Fatal("unknown event did not 404")
	}
	if response = request(http.MethodGet, "/api/matches?event=invalid", "", ""); response.Code != http.StatusBadRequest {
		t.Fatal("malformed event accepted")
	}
}
