package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/jometheuspondo/rlcs-predictions/internal/db"
	"github.com/jometheuspondo/rlcs-predictions/internal/models"
)

type eventContextKey struct{}

// selectEvent resolves the requested event before any event data is read or written.
func (s *server) selectEvent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := s.deps.Tournament.ID
		if raw := r.URL.Query().Get("event"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed <= 0 {
				writeError(w, http.StatusBadRequest, "invalid_event", "invalid event")
				return
			}
			id = parsed
		}

		event, err := s.deps.DB.GetTournament(r.Context(), id)
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "event_not_found", "event not found")
			return
		}
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		if !event.IsActive && !isOwner(r) && (r.Method == http.MethodPut || r.Method == http.MethodDelete) {
			writeError(w, http.StatusForbidden, "event_read_only", "past events are read-only")
			return
		}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), eventContextKey{}, event)))
	})
}

func (s *server) event(r *http.Request) *models.Tournament {
	return r.Context().Value(eventContextKey{}).(*models.Tournament)
}

func (s *server) eventDB(r *http.Request) *db.EventStore {
	store := s.deps.DB.ForTournament(s.event(r).ID)
	if isOwner(r) {
		return store.WithOwnerOverride()
	}
	return store
}

func (s *server) listEvents(w http.ResponseWriter, r *http.Request) {
	events, err := s.deps.DB.ListTournaments(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (s *server) listTeams(w http.ResponseWriter, r *http.Request) {
	teams, err := s.eventDB(r).ListTeamNames(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, teams)
}
