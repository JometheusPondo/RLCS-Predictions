package api

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// spaHandler serves the embedded frontend. Static files (index.html, hashed
// JS/CSS, etc.) are served when the request path matches an embedded file;
// every other path falls back to index.html so React Router can render the
// client-side route (/profile/:id, /leaderboard, …).
//
// Only requests that don't match /api/* reach here — the API route group is
// registered first and matches its own prefix.
func spaHandler(distFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(distFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if reqPath == "" {
			reqPath = "index.html"
		}

		// If the path maps to a real embedded file, serve it as-is.
		if f, err := distFS.Open(reqPath); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// No matching file → serve index.html with a 200 so the SPA boots.
		// Rewriting the path is how you make http.FileServer return index.html
		// instead of a 404 for an unknown route.
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
