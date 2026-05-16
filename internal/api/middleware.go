package api

import (
	"log/slog"
	"mime"
	"net/http"
	"runtime/debug"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// loggingMiddleware emits one INFO line per request with method, path, status,
// bytes written, duration, and chi's request id. Uses chi's response-wrapper to
// capture the status code (the bare ResponseWriter doesn't expose it).
func loggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			logger.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", chimw.GetReqID(r.Context()),
			)
		})
	}
}

// recoverMiddleware catches handler panics and returns a clean 500. Logs the
// panic value + stack at ERROR. If headers have already been sent (rare —
// requires a partial write before the panic), the client gets whatever was on
// the wire; otherwise we write a normal error envelope.
func recoverMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				// http.ErrAbortHandler is a sentinel that signals an intentional
				// abort — don't log it as an error or rewrite the response.
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				logger.Error("handler panic",
					"panic", rec,
					"method", r.Method,
					"path", r.URL.Path,
					"stack", string(debug.Stack()),
				)
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"internal server error","code":"internal_error"}`))
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// contentTypeMiddleware enforces Content-Type: application/json on requests
// that carry a body. POST/PUT/PATCH with Content-Length > 0 (or a chunked
// transfer encoding) must declare JSON — otherwise 415. Bodyless POSTs (like
// the dev /sync/now trigger) pass through.
func contentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
		default:
			next.ServeHTTP(w, r)
			return
		}

		hasBody := r.ContentLength > 0 || r.Header.Get("Transfer-Encoding") != ""
		if !hasBody {
			next.ServeHTTP(w, r)
			return
		}

		mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mt != "application/json" {
			writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type",
				"Content-Type must be application/json")
			return
		}
		next.ServeHTTP(w, r)
	})
}
