package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/jometheuspondo/rlcs-predictions/internal/db"
	"github.com/jometheuspondo/rlcs-predictions/internal/models"
)

// server holds the dependencies; handlers are methods so they share state via
// the receiver rather than closures over package globals.
type server struct {
	deps Deps
}

// errorResponse is the shape every 4xx/5xx returns. Both fields populated:
// Code is the snake_case tag for the frontend to switch on; Error is a
// human-readable message for logs or fallback display.
type errorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

// =============================================================================
// Health + sync
// =============================================================================

func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// syncStatus folds the poller's in-memory LastError() into the persisted
// last_synced_at. Per Phase 3 design, last_error is NOT persisted in the DB.
func (s *server) syncStatus(w http.ResponseWriter, _ *http.Request) {
	status := models.SyncStatus{
		LastSyncedAt: s.deps.Tournament.LastSyncedAt,
	}
	if e := s.deps.Poller.LastError(); e != nil {
		msg := e.Error()
		status.LastError = &msg
	}
	writeJSON(w, http.StatusOK, status)
}

// =============================================================================
// Matches
// =============================================================================

func (s *server) listMatches(w http.ResponseWriter, r *http.Request) {
	matches, err := s.deps.DB.ListMatches(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, matches)
}

// =============================================================================
// Participants
// =============================================================================

func (s *server) listParticipants(w http.ResponseWriter, r *http.Request) {
	ps, err := s.deps.DB.ListParticipants(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ps)
}

type createParticipantReq struct {
	DisplayName string `json:"display_name"`
}

// createParticipant slugifies the display_name into a URL-friendly id and
// inserts the row. Two participants with display_names that slugify to the
// same id collide — spec § 6 returns 409 in that case rather than auto-
// disambiguating; the user picks a different display_name.
func (s *server) createParticipant(w http.ResponseWriter, r *http.Request) {
	var req createParticipantReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body must be valid JSON")
		return
	}

	name, err := validateDisplayName(req.DisplayName)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_display_name", err.Error())
		return
	}

	id := slugify(name)
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_display_name",
			"display_name must contain at least one alphanumeric character")
		return
	}

	p, err := s.deps.DB.CreateParticipant(r.Context(), id, name)
	if errors.Is(err, db.ErrIDConflict) {
		writeError(w, http.StatusConflict, "id_collision",
			"a participant with that display name already exists")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/participants/"+id)
	writeJSON(w, http.StatusCreated, p)
}

func (s *server) getParticipant(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, err := s.deps.DB.GetParticipantWithPredictions(r.Context(), id)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "participant not found")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	// Permission filter: viewing your own profile (or being blast_admin) shows
	// every prediction; anyone else sees only predictions on completed matches.
	// In-progress picks stay private. Winner-pick history is always public.
	if !canSeeAllPredictions(r, id) {
		filtered, err := s.filterToCompleted(r.Context(), p.Predictions)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		p.Predictions = filtered
	}

	writeJSON(w, http.StatusOK, p)
}

// filterToCompleted returns only the predictions whose match has completed.
// Used to hide a participant's in-progress picks from other users.
func (s *server) filterToCompleted(ctx context.Context, preds []models.Prediction) ([]models.Prediction, error) {
	matches, err := s.deps.DB.ListMatches(ctx)
	if err != nil {
		return nil, err
	}
	completed := make(map[string]bool, len(matches))
	for _, m := range matches {
		if m.Status == models.StatusCompleted {
			completed[m.ID] = true
		}
	}
	out := make([]models.Prediction, 0, len(preds))
	for _, p := range preds {
		if completed[p.MatchID] {
			out = append(out, p)
		}
	}
	return out, nil
}

// =============================================================================
// Auth
// =============================================================================

type loginReq struct {
	ParticipantID string `json:"participant_id"`
	Password      string `json:"password"`
}

type loginResp struct {
	Token string `json:"token"`
}

// login validates a participant_id + password pair. On success it returns a
// bearer token, which is simply the participant id (honor-system tool, no
// session table). Failures return a generic 401 that doesn't distinguish
// "no such participant" from "wrong password".
func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body must be valid JSON")
		return
	}
	req.ParticipantID = strings.TrimSpace(req.ParticipantID)
	if req.ParticipantID == "" {
		writeError(w, http.StatusBadRequest, "invalid_participant_id", "participant_id is required")
		return
	}

	stored, err := s.deps.DB.GetParticipantPassword(r.Context(), req.ParticipantID)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "incorrect participant or password")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	// An empty stored password (NULL in the DB — predates the auth migration or
	// was never assigned) means the account can't be logged into. Constant-time
	// compare is cheap insurance against a password-timing oracle.
	if stored == "" || subtle.ConstantTimeCompare([]byte(stored), []byte(req.Password)) != 1 {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "incorrect participant or password")
		return
	}

	writeJSON(w, http.StatusOK, loginResp{Token: req.ParticipantID})
}

// =============================================================================
// Winner picks
// =============================================================================

type setWinnerPickReq struct {
	TeamName string `json:"team_name"`
}

// setWinnerPick appends a tournament-winner pick to the participant's history.
// Auth: the participant themselves, or blast_admin. The team must be one that
// actually competes in the tournament. Returns the updated participant.
func (s *server) setWinnerPick(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if requesterID(r) != id && !isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "you can only change your own winner pick")
		return
	}

	var req setWinnerPickReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body must be valid JSON")
		return
	}
	req.TeamName = strings.TrimSpace(req.TeamName)
	if req.TeamName == "" {
		writeError(w, http.StatusBadRequest, "invalid_team", "team_name is required")
		return
	}

	// The auth middleware validated the *requester*; an admin could target any
	// id, so confirm the target participant exists.
	exists, err := s.deps.DB.ParticipantExists(r.Context(), id)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "not_found", "participant not found")
		return
	}

	// Validate team_name against the tournament's actual teams.
	teams, err := s.deps.DB.ListTeamNames(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	valid := false
	for _, t := range teams {
		if t == req.TeamName {
			valid = true
			break
		}
	}
	if !valid {
		writeError(w, http.StatusBadRequest, "invalid_team", "team_name is not a competing team")
		return
	}

	if err := s.deps.DB.AddWinnerPick(r.Context(), id, req.TeamName); err != nil {
		s.serverError(w, r, err)
		return
	}

	// Return the updated participant. This path is self-or-admin only, so
	// returning unfiltered predictions leaks nothing.
	p, err := s.deps.DB.GetParticipantWithPredictions(r.Context(), id)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// =============================================================================
// Predictions
// =============================================================================

type setPredictionReq struct {
	Pick string `json:"pick"`
}

// setPrediction reads match_id from the URL path (per spec § 6 PUT route)
// rather than the body. Returns the saved prediction so the frontend can use
// it for optimistic-update reconciliation. Auth: self or blast_admin only.
func (s *server) setPrediction(w http.ResponseWriter, r *http.Request) {
	pid := chi.URLParam(r, "id")
	mid := chi.URLParam(r, "match_id")

	if requesterID(r) != pid && !isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "you can only change your own predictions")
		return
	}

	var req setPredictionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body must be valid JSON")
		return
	}

	if req.Pick != models.PickA && req.Pick != models.PickB {
		writeError(w, http.StatusBadRequest, "invalid_pick", "pick must be 'A' or 'B'")
		return
	}

	err := s.deps.DB.SetPrediction(r.Context(), pid, mid, req.Pick)
	switch {
	case errors.Is(err, db.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "participant or match not found")
	case errors.Is(err, db.ErrMatchCompleted):
		writeError(w, http.StatusBadRequest, "match_completed", "match is completed; predictions are locked")
	case err != nil:
		s.serverError(w, r, err)
	default:
		writeJSON(w, http.StatusOK, models.Prediction{MatchID: mid, Pick: req.Pick})
	}
}

func (s *server) deletePrediction(w http.ResponseWriter, r *http.Request) {
	pid := chi.URLParam(r, "id")
	mid := chi.URLParam(r, "match_id")

	if requesterID(r) != pid && !isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "you can only change your own predictions")
		return
	}

	err := s.deps.DB.DeletePrediction(r.Context(), pid, mid)
	switch {
	case errors.Is(err, db.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "prediction, participant, or match not found")
	case errors.Is(err, db.ErrMatchCompleted):
		writeError(w, http.StatusBadRequest, "match_completed", "match is completed; predictions are locked")
	case err != nil:
		s.serverError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// =============================================================================
// Helpers
// =============================================================================

// serverError logs the underlying error and returns a generic 500 to the client
// (no internal details leaked over the wire).
func (s *server) serverError(w http.ResponseWriter, r *http.Request, err error) {
	s.deps.Logger.Error("handler error",
		"method", r.Method,
		"path", r.URL.Path,
		"err", err,
	)
	writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Error: message, Code: code})
}

// =============================================================================
// Validation
// =============================================================================

const (
	displayNameMin = 2  // rune count, after trim
	displayNameMax = 40 // rune count, after trim
)

// validateDisplayName trims, then enforces length bounds and rejects control
// characters. Returns the canonical (trimmed) form to be stored.
func validateDisplayName(s string) (string, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "", errors.New("display_name is required")
	}
	n := utf8.RuneCountInString(trimmed)
	if n < displayNameMin {
		return "", fmt.Errorf("display_name must be at least %d characters", displayNameMin)
	}
	if n > displayNameMax {
		return "", fmt.Errorf("display_name must be at most %d characters", displayNameMax)
	}
	for _, r := range trimmed {
		if unicode.IsControl(r) {
			return "", errors.New("display_name contains control characters")
		}
	}
	return trimmed, nil
}

// =============================================================================
// ID generation (slugified per spec § 6)
// =============================================================================

// slugify converts a display_name to a URL-friendly id: lowercase ASCII
// alphanumerics + hyphens, consecutive hyphens collapsed, leading/trailing
// hyphens stripped. Returns "" if the input contains no alphanumerics —
// callers treat empty as invalid_display_name.
//
// Examples:
//
//	"Jome"          → "jome"
//	"Jome Pondo"    → "jome-pondo"
//	"Jome  Pondo!"  → "jome-pondo"
//	"@@@"           → ""
func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	lastWasHyphen := true // start true so leading non-alnum runs are skipped
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastWasHyphen = false
			continue
		}
		if !lastWasHyphen {
			b.WriteByte('-')
			lastWasHyphen = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}
