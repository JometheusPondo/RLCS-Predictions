package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jometheuspondo/rlcs-predictions/internal/db"
	"github.com/jometheuspondo/rlcs-predictions/internal/models"
)

// ctxKey is an unexported context-key type, so keys from this package can't
// collide with keys set by other packages or middleware.
type ctxKey int

const requesterIDKey ctxKey = iota

// authMiddleware reads an "Authorization: Bearer <participant-id>" header. The
// token IS the participant id — there's no session table; this is an
// honor-system tool (see the conversation spec). If the token maps to a real
// participant, that id is attached to the request context.
//
// Crucially, this middleware is permissive: a missing or invalid token does
// NOT reject the request. It proceeds as "anonymous", because some endpoints
// (the landing page's participant list, /login itself) must work without auth.
// Per-endpoint auth requirements are enforced in the handlers via requesterID().
func authMiddleware(database *db.DB, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := bearerToken(r)
			if id != "" {
				ok, err := database.ParticipantExists(r.Context(), id)
				if err != nil {
					// Treat a DB error as "anonymous" rather than failing the
					// request — the handler's own auth check will reject it if
					// the endpoint actually requires auth.
					logger.Error("auth: participant lookup failed", "err", err)
				} else if ok {
					ctx := context.WithValue(r.Context(), requesterIDKey, id)
					r = r.WithContext(ctx)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// bearerToken extracts the token from an "Authorization: Bearer <token>"
// header, or "" if the header is absent or malformed.
func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}

// requesterID returns the authenticated participant id for this request, or ""
// when the request is anonymous.
func requesterID(r *http.Request) string {
	id, _ := r.Context().Value(requesterIDKey).(string)
	return id
}

// isAdmin reports whether the request is authenticated as the backstage
// blast_admin account.
func isAdmin(r *http.Request) bool {
	return requesterID(r) == models.AdminID
}

// canSeeAllPredictions reports whether the requester may see the target
// participant's in-progress picks: true when viewing your own profile, or when
// you are blast_admin. Everyone else sees only completed-match predictions.
func canSeeAllPredictions(r *http.Request, targetID string) bool {
	return requesterID(r) == targetID || isAdmin(r)
}
