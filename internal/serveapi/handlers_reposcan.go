package serveapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"

	"github.com/artaeon/granit/internal/reposcan"
)

// POST /api/v1/reposcan
//
// Body: {"path": "<absolute local path>"}
//
// The local single-tenant deployment lets us trust the bearer-
// authenticated user with read access to anything under their
// home directory or the vault root. Both get added to the
// allowedRoots list reposcan enforces — paths anywhere else
// (e.g. /etc, /var/log) are refused with 403.
//
// Read-only: never writes to the repo, never invokes hooks.
// `git log` is bounded to 5s wall-clock to defend against
// misconfigured NFS / slow repos.

type repoScanRequest struct {
	Path string `json:"path"`
}

func (s *Server) handleScanRepo(w http.ResponseWriter, r *http.Request) {
	var body repoScanRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.Path == "" {
		writeError(w, http.StatusBadRequest, "path required")
		return
	}
	// Allowed roots: the user's home directory + the vault root.
	// The vault root is always set; home may be missing in an
	// unusual setup (root user without HOME), which still leaves
	// the vault as a valid scan target. Both being available is
	// the common case.
	allowed := []string{s.cfg.Vault.Root}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		allowed = append(allowed, home)
	}
	ctx, err := reposcan.ScanRepo(body.Path, allowed)
	if err != nil {
		switch {
		case errors.Is(err, reposcan.ErrPathTraversal):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, reposcan.ErrOutsideAllowed):
			// 403 — the path resolves cleanly but it's not under a
			// root we trust. Distinct from 404 so the UI can
			// surface a clear "you can only scan your home dir /
			// vault" hint.
			writeError(w, http.StatusForbidden, err.Error())
		case errors.Is(err, reposcan.ErrSymlinkRoot):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, reposcan.ErrNotADirectory):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, os.ErrNotExist):
			writeError(w, http.StatusNotFound, "path not found")
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, ctx)
}
