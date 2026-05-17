// Package api builds the HTTP layer: router, handlers, and middleware. It
// depends on the db package for storage and the syncer.Poller for the
// /api/sync/status endpoint's last_error field.
package api

import (
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/jometheuspondo/rlcs-predictions/internal/db"
	"github.com/jometheuspondo/rlcs-predictions/internal/models"
	"github.com/jometheuspondo/rlcs-predictions/internal/syncer"
)

// Deps bundles the dependencies the API layer needs from main. Tournament is
// taken by pointer so handlers see live LastSyncedAt updates from the poller.
type Deps struct {
	DB         *db.DB
	Tournament *models.Tournament
	Poller     *syncer.Poller
	Logger     *slog.Logger
	DevMode    bool

	// DistFS is the embedded frontend. When non-nil, a catch-all route serves
	// it (with index.html SPA fallback). nil disables frontend serving — handy
	// for API-only test setups.
	DistFS fs.FS
}

// NewRouter returns the configured http.Handler. /api/* is the JSON API;
// everything else is the embedded SPA. In dev mode, POST /api/sync/now is
// registered for manual sync triggers.
func NewRouter(d Deps) http.Handler {
	s := &server{deps: d}

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(loggingMiddleware(d.Logger))
	r.Use(recoverMiddleware(d.Logger))
	r.Use(contentTypeMiddleware)

	r.Route("/api", func(r chi.Router) {
		// JSON (not chi's plain-text default) for unmatched API paths/methods,
		// so clients get a consistent error envelope everywhere under /api.
		r.NotFound(apiNotFound)
		r.MethodNotAllowed(apiMethodNotAllowed)

		// Permissive auth: attaches the requester id when a valid bearer token
		// is present, otherwise the request proceeds anonymously. Per-endpoint
		// auth requirements are enforced in the handlers.
		r.Use(authMiddleware(d.DB, d.Logger))

		r.Get("/health", s.health)
		r.Get("/matches", s.listMatches)
		r.Get("/simulation", s.getSimulation)
		r.Get("/sync/status", s.syncStatus)

		r.Post("/login", s.login)

		r.Get("/participants", s.listParticipants)
		// Self-registration is disabled for now: accounts are provisioned by
		// the operator directly. The createParticipant handler is intentionally
		// left in place — re-register this route (and re-add the create UI in
		// LandingPage.tsx) when an approval flow is built.
		//   r.Post("/participants", s.createParticipant)
		r.Get("/participants/{id}", s.getParticipant)
		r.Put("/participants/{id}/winner", s.setWinnerPick)
		r.Put("/participants/{id}/predictions/{match_id}", s.setPrediction)
		r.Delete("/participants/{id}/predictions/{match_id}", s.deletePrediction)

		if d.DevMode {
			r.Post("/sync/now", d.Poller.DevSyncHandler())
		}
	})

	// Embedded SPA — must be registered after /api so the API wins its prefix.
	if d.DistFS != nil {
		r.Handle("/*", spaHandler(d.DistFS))
	}

	return r
}

func apiNotFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
}

func apiMethodNotAllowed(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed for this endpoint")
}
