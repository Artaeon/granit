package serveapi

import (
	"net/http"

	"github.com/artaeon/granit/internal/snippets"
)

// snippetView is the wire shape of a snippet on /api/v1/snippets. We
// don't expose the *Engine directly because (a) Engine has no JSON
// tags by design (the package is UI-agnostic) and (b) the web client
// only needs the fields below.
type snippetView struct {
	Trigger     string `json:"trigger"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

// handleListSnippets returns the full builtin snippet set so the
// CodeMirror editor extension can render the picker. Read-only by
// design today; a future "user snippets in the vault" feature would
// add a write path — but builtins should stay server-owned so a
// granit upgrade can ship new templates without users hand-merging.
//
// Auth-gated like every other /api/v1 route via the auth middleware
// in server.go. Cheap to compute (no I/O) so we recompute per-request
// instead of caching — the engine is a few microseconds and the data
// is intentionally hot-reloadable when we add user snippets later.
func (s *Server) handleListSnippets(w http.ResponseWriter, r *http.Request) {
	e := snippets.New()
	all := e.All()
	out := make([]snippetView, len(all))
	for i, sn := range all {
		out[i] = snippetView{Trigger: sn.Trigger, Description: sn.Description, Content: sn.Content}
	}
	writeJSON(w, http.StatusOK, map[string]any{"snippets": out, "total": len(out)})
}
