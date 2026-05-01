package serveapi

import (
	"embed"
	"io/fs"
	"net/http"
)

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
