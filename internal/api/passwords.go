package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/jometheuspondo/rlcs-predictions/internal/db"
	"github.com/jometheuspondo/rlcs-predictions/internal/models"
)

type resetPasswordReq struct {
	ParticipantID string `json:"participant_id"`
	NewPassword   string `json:"new_password"`
}

// resetPassword supports unauthenticated recovery for this honor-system site.
func (s *server) resetPassword(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var req resetPasswordReq
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body must be valid JSON")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body must contain one JSON object")
		return
	}

	req.ParticipantID = strings.TrimSpace(req.ParticipantID)
	if req.ParticipantID == models.OwnerID {
		writeError(w, http.StatusForbidden, "reset_disabled", "password reset is disabled for this account")
		return
	}
	if req.ParticipantID == "" {
		writeError(w, http.StatusBadRequest, "invalid_participant_id", "select an account first")
		return
	}
	if strings.TrimSpace(req.NewPassword) == "" || len(req.NewPassword) > 1024 {
		writeError(w, http.StatusBadRequest, "invalid_password", "enter a non-blank password of at most 1024 bytes")
		return
	}

	err := s.deps.DB.ResetParticipantPassword(r.Context(), req.ParticipantID, req.NewPassword)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "participant_not_found", "account not found")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
