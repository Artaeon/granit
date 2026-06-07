package serveapi

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/artaeon/granit/internal/atomicio"
)

// Workspace layouts (the "named split-tree layouts" the web shell
// presents in its workspace pills) used to live in browser
// localStorage. That meant a setup carefully tuned on one device
// never followed the user to another one — the first pain point a
// multi-device user hits.
//
// This pair of handlers persists the full workspace store to
// <vault>/.granit/workspaces.json as an opaque JSON blob. The
// backend does NOT inspect the shape — schema migrations and
// per-workspace validation live in the frontend store (see
// /web/src/lib/workspace/workspaceStore.svelte.ts), so a future
// schema bump doesn't force a coordinated server release.
//
// The web's existing localStorage path stays as a fallback when the
// user is offline / unauthenticated — see the controller's
// applyVaultPayload + push flow for the resolution rule.
//
// First pass: opaque pass-through of the persisted shape. Typed
// per-pane state + per-pane filter persistence land in a later pass
// of Phase 3.

const workspacesEmptyPayload = `{"workspaces":[]}`

// workspacesPath returns the absolute path to the per-vault sidecar.
// atomicio.WriteState handles MkdirAll on the write side; a missing
// file on read is the documented "no synced state yet" signal.
func workspacesPath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "workspaces.json")
}

// handleGetWorkspaces returns the raw JSON bytes from the sidecar, or
// the empty default ({"workspaces":[]}) when the file is missing or
// unreadable. Returns the payload verbatim — the frontend owns the
// schema, so we don't decode/re-encode (preserves field ordering,
// avoids dropping unknown keys a newer client wrote).
func (s *Server) handleGetWorkspaces(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(workspacesPath(s.cfg.Vault.Root))
	if err != nil {
		// Missing / unreadable both fall back to the empty default;
		// a fresh vault has no sidecar yet, and a corrupt one is
		// better surfaced as "empty" than a 500 that blocks the
		// shell from booting.
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(workspacesEmptyPayload))
		return
	}
	// Validate it's at least syntactically JSON before serving — a
	// half-written file (shouldn't happen given the atomic write but
	// belt + braces) shouldn't poison the client.
	if !json.Valid(data) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(workspacesEmptyPayload))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// handlePutWorkspaces writes the request body verbatim to the
// sidecar after a JSON-validity check. Atomic via atomicio.WriteState
// (write to a .tmp sibling, fsync, rename) — concurrent reads always
// see either the previous full payload or the new one, never a
// partial file. Returns the saved payload so callers can confirm
// without a follow-up GET.
//
// No shape validation: the frontend is the schema owner. A malformed
// shape is still legal JSON the next client read will reject; an
// unparseable body is the only thing we 400 on.
func (s *Server) handlePutWorkspaces(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}
	if len(body) == 0 {
		writeError(w, http.StatusBadRequest, "empty body")
		return
	}
	if !json.Valid(body) {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := atomicio.WriteState(workspacesPath(s.cfg.Vault.Root), body); err != nil {
		writeError(w, http.StatusInternalServerError, "write workspaces: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
