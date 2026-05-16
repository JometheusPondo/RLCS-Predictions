// Package app embeds the built frontend so the server ships as a single
// binary with no runtime file dependencies (spec § 8).
//
// This root package exists solely to host the //go:embed directive: embed
// cannot traverse upward out of a package directory, so a file in cmd/server/
// could never reach web/dist. cmd/server imports this package.
package app

import (
	"embed"
	"io/fs"
)

//go:embed all:web/dist
var distFS embed.FS

// DistFS returns the built frontend as an fs.FS rooted at the dist directory
// (the "web/dist" prefix is stripped). Before `make frontend` has run, web/dist
// holds only a .gitkeep placeholder — the server still starts and the API
// works, but the SPA handler will 404 until a real build populates it.
func DistFS() (fs.FS, error) {
	return fs.Sub(distFS, "web/dist")
}
