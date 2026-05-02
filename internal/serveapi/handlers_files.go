package serveapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

// handleGetFile serves a non-markdown file from the vault by path. Used
// by the markdown preview to embed images via `![[image.png]]`. Markdown
// files have their own endpoint at /api/v1/notes/{path}; this one is
// for binaries (images, PDFs, audio) the preview wants to display
// inline.
//
// Path safety: the wildcard captures anything after /files/. We refuse
// absolute paths, any `..` component (even ones that would cancel
// out), and require the cleaned absolute path to be inside the vault
// root. Same paranoid containment used by tasks.resolveInVault — every
// vault file-system access in the server goes through this shape.
func (s *Server) handleGetFile(w http.ResponseWriter, r *http.Request) {
	rel := chi.URLParam(r, "*")
	if rel == "" {
		writeError(w, http.StatusBadRequest, "path required")
		return
	}
	if filepath.IsAbs(rel) {
		writeError(w, http.StatusBadRequest, "absolute path rejected")
		return
	}
	for _, part := range strings.FieldsFunc(rel, func(c rune) bool { return c == '/' || c == '\\' }) {
		if part == ".." {
			writeError(w, http.StatusBadRequest, "path traversal rejected")
			return
		}
	}
	root := s.cfg.Vault.Root
	abs := filepath.Clean(filepath.Join(root, rel))
	rootClean := filepath.Clean(root)
	if abs != rootClean && !strings.HasPrefix(abs, rootClean+string(filepath.Separator)) {
		writeError(w, http.StatusBadRequest, "path escapes vault")
		return
	}
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	// Block markdown — those have their own JSON endpoint with
	// frontmatter parsing. Serving them as raw octet-streams here
	// would be confusing.
	if strings.HasSuffix(strings.ToLower(rel), ".md") {
		writeError(w, http.StatusBadRequest, "use /api/v1/notes/ for markdown")
		return
	}
	// Cache hint for static assets — the SW will already cache
	// /api/v1/* GETs, but a strong validator helps when SW is bypassed.
	w.Header().Set("Cache-Control", "private, max-age=300")
	http.ServeFile(w, r, abs)
}
