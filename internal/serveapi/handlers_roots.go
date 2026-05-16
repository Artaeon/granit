package serveapi

import (
	"net/http"
	"strings"

	"github.com/artaeon/granit/internal/roots"
	"github.com/artaeon/granit/internal/wshub"
	"github.com/oklog/ulid/v2"
)

const statePathRoots = ".granit/roots.json"

func (s *Server) bcastRoots() {
	s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: statePathRoots})
}

// rootsResponse decorates the on-disk Roots with the ring labels so
// the client doesn't need to hard-code them. Single source of truth
// for ring naming stays in the Go package; the UI just renders what
// the server sends.
type rootsResponse struct {
	roots.Roots
	RingLabels map[int]string `json:"ring_labels"`
}

func (s *Server) handleGetRoots(w http.ResponseWriter, r *http.Request) {
	rec := roots.Load(s.cfg.Vault.Root)
	if rec.Center == "" {
		rec.Center = roots.DefaultCenter
	}
	writeJSON(w, http.StatusOK, rootsResponse{Roots: rec, RingLabels: roots.RingLabels})
}

// handlePutRoots is a full upsert — the client sends the whole
// record. Same reasoning as vision: a contemplative artifact edited
// via a form-like surface doesn't earn the complexity of patch-merge.
//
// IDs: any incoming node with an empty ID gets a fresh ULID stamped
// here. Lets the client send a brand-new node with just label+ring
// and trust the server to fill the audit fields.
func (s *Server) handlePutRoots(w http.ResponseWriter, r *http.Request) {
	var incoming roots.Roots
	if !readJSON(w, r, &incoming) {
		return
	}
	for i := range incoming.Nodes {
		if strings.TrimSpace(incoming.Nodes[i].ID) == "" {
			incoming.Nodes[i].ID = strings.ToLower(ulid.Make().String())
		}
	}
	if err := roots.Save(s.cfg.Vault.Root, incoming); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.bcastRoots()
	rec := roots.Load(s.cfg.Vault.Root)
	if rec.Center == "" {
		rec.Center = roots.DefaultCenter
	}
	writeJSON(w, http.StatusOK, rootsResponse{Roots: rec, RingLabels: roots.RingLabels})
}
