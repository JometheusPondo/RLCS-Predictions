package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jometheuspondo/rlcs-predictions/internal/db"
)

// matchPickers exposes the locked match's participant lists without changing profile visibility.
func (s *server) matchPickers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	pickers, err := s.eventDB(r).ListMatchPickers(r.Context(), chi.URLParam(r, "match_id"))
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "match not found")
		return
	}
	if errors.Is(err, db.ErrPicksHidden) {
		writeError(w, http.StatusForbidden, "picks_hidden", "predictions are hidden until match lock")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, pickers)
}
