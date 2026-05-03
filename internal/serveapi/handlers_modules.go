package serveapi

import (
	"encoding/json"
	"net/http"

	"github.com/artaeon/granit/internal/modules"
	"github.com/artaeon/granit/internal/wshub"
)

// moduleEntry is the wire shape returned by GET /api/v1/modules.
// Values mirror modules.Module declarations + the runtime enable flag.
type moduleEntry struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Enabled     bool     `json:"enabled"`
	DependsOn   []string `json:"dependsOn,omitempty"`
}

type modulesResponse struct {
	Modules []moduleEntry `json:"modules"`
	// CoreIDs surfaces module IDs the UI should render as always-on
	// with a lock icon. They aren't toggleable so they're not in the
	// Modules slice — keeping them on a separate field avoids the
	// settings UI accidentally rendering a disabled checkbox for them.
	CoreIDs []coreEntry `json:"coreIds"`
}

type coreEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type putModulesRequest struct {
	Enabled map[string]bool `json:"enabled"`
}

func (s *Server) handleListModules(w http.ResponseWriter, r *http.Request) {
	reg := s.modulesRegistry()
	out := modulesResponse{
		Modules: make([]moduleEntry, 0),
		CoreIDs: make([]coreEntry, 0, len(modules.CoreIDs)),
	}
	for _, m := range reg.All() {
		out.Modules = append(out.Modules, moduleEntry{
			ID:          m.ID(),
			Name:        m.Name(),
			Description: m.Description(),
			Category:    m.Category(),
			Enabled:     reg.Enabled(m.ID()),
			DependsOn:   m.DependsOn(),
		})
	}
	for _, id := range modules.CoreIDs {
		out.CoreIDs = append(out.CoreIDs, coreEntry{ID: id, Name: modules.CoreNames[id]})
	}
	writeJSON(w, http.StatusOK, out)
}

// handlePutModules atomically applies a desired enable-state batch.
// The body is a partial map — modules not mentioned keep their current
// state. Core IDs in the body are silently ignored: they're always-on
// by definition and accepting a "disable" toggle for them would let a
// stale UI accidentally hide settings.
func (s *Server) handlePutModules(w http.ResponseWriter, r *http.Request) {
	var req putModulesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(req.Enabled) == 0 {
		writeError(w, http.StatusBadRequest, "enabled map is empty")
		return
	}
	// Strip any core-ID entries before passing to the registry.
	core := map[string]bool{}
	for _, id := range modules.CoreIDs {
		core[id] = true
	}
	clean := make(map[string]bool, len(req.Enabled))
	for id, on := range req.Enabled {
		if core[id] {
			continue
		}
		clean[id] = on
	}
	reg := s.modulesRegistry()
	if err := reg.SetEnabledBatch(clean); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := reg.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Tell connected web clients to refresh their module store. Same
	// hub the file watcher publishes through, so the existing
	// onWsEvent listeners pick this up without a special transport.
	if s.hub != nil {
		s.hub.Broadcast(wshub.Event{Type: "modules.changed"})
	}
	// Echo the post-write list so the caller doesn't need a follow-up
	// GET to refresh its cache.
	s.handleListModules(w, r)
}
