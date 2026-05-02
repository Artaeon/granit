package serveapi

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
)

// init registers MIME types that Go's stdlib doesn't always know about
// on every host. Without these, the server binary deployed on a minimal
// system (Alpine, scratch container) would serve the manifest as
// text/plain — Chrome refuses that as a PWA manifest, so the install
// prompt never fires. Pinning here keeps the server self-contained.
func init() {
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
	_ = mime.AddExtensionType(".svg", "image/svg+xml")
	_ = mime.AddExtensionType(".woff2", "font/woff2")
}

//go:embed all:dist
var distFS embed.FS

// Assets returns an http.FileSystem rooted at the built SvelteKit frontend,
// or a fallback if no build is present (so the binary still serves the API
// without a frontend bundle).
func Assets() http.FileSystem {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return placeholder{}
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return placeholder{}
	}
	return http.FS(sub)
}

type placeholder struct{}

func (placeholder) Open(name string) (http.File, error) {
	return nil, fs.ErrNotExist
}
