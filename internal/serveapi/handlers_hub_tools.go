// Package serveapi — handlers for /api/v1/hub/tools.
//
// The hub's tools half is a CRUD over .granit/hub-tools.json. A
// tool card carries an icon, name, optional description, and an
// ordered list of copy-paste setup commands the user pastes into
// a terminal ("brew install neovim", "kubectl config use-context
// prod", etc). Mirrors the hub-link handler shape; lives in its
// own file because the storage is separate and the CRUD has its
// own quirks (sanitise commands on every write, broadcast a
// dedicated WS event so a tab on the tools section can refresh
// without re-fetching the links).
package serveapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/artaeon/granit/internal/hub"
	"github.com/artaeon/granit/internal/wshub"
)

const hubToolsStatePath = ".granit/hub-tools.json"

// bcastHubTools fires the dedicated tools-changed event so a tab
// scrolled to the tools section can refresh without also reloading
// the (potentially long) link list. Mirrors bcastHub.
func (s *Server) bcastHubTools() {
	if s.hub == nil {
		return
	}
	s.hub.Broadcast(wshub.Event{Type: "hub.tools.changed", Path: hubToolsStatePath})
}

func (s *Server) handleListHubTools(w http.ResponseWriter, r *http.Request) {
	tools, err := hub.LoadAllTools(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tools == nil {
		tools = []hub.Tool{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": tools, "total": len(tools)})
}

func (s *Server) handleCreateHubTool(w http.ResponseWriter, r *http.Request) {
	var t hub.Tool
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	t.Description = strings.TrimSpace(t.Description)
	t.Icon = strings.TrimSpace(t.Icon)
	t.Color = strings.TrimSpace(t.Color)
	t.Commands = hub.SanitizeCommands(t.Commands)
	t.Tags = hub.SanitizeTags(t.Tags)
	if t.ID == "" {
		t.ID = hub.NewID()
	}
	now := hub.Now()
	t.CreatedAt = now
	t.UpdatedAt = now

	tools, err := hub.LoadAllTools(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tools = append(tools, t)
	if err := hub.SaveAllTools(s.cfg.Vault.Root, tools); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastHubTools()
	writeJSON(w, http.StatusCreated, t)
}

// handlePatchHubTool — partial update. Unmarshals into a sparse
// map so the client can rename a tool, replace its commands, or
// edit a single field without sending the whole record. Mirrors
// the link patch shape.
func (s *Server) handlePatchHubTool(w http.ResponseWriter, r *http.Request) {
	id := urlParam(r, "id")
	var patch map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	tools, err := hub.LoadAllTools(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	idx := -1
	for i, t := range tools {
		if t.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		writeError(w, http.StatusNotFound, "tool not found")
		return
	}
	t := tools[idx]
	apply := func(key string, into interface{}) {
		if raw, ok := patch[key]; ok {
			_ = json.Unmarshal(raw, into)
		}
	}
	apply("name", &t.Name)
	apply("description", &t.Description)
	apply("icon", &t.Icon)
	apply("color", &t.Color)
	apply("tags", &t.Tags)
	apply("commands", &t.Commands)
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	t.Description = strings.TrimSpace(t.Description)
	t.Icon = strings.TrimSpace(t.Icon)
	t.Color = strings.TrimSpace(t.Color)
	t.Commands = hub.SanitizeCommands(t.Commands)
	t.Tags = hub.SanitizeTags(t.Tags)
	t.UpdatedAt = hub.Now()
	tools[idx] = t
	if err := hub.SaveAllTools(s.cfg.Vault.Root, tools); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastHubTools()
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleDeleteHubTool(w http.ResponseWriter, r *http.Request) {
	id := urlParam(r, "id")
	tools, err := hub.LoadAllTools(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := tools[:0]
	found := false
	for _, t := range tools {
		if t.ID == id {
			found = true
			continue
		}
		out = append(out, t)
	}
	if !found {
		writeError(w, http.StatusNotFound, "tool not found")
		return
	}
	if err := hub.SaveAllTools(s.cfg.Vault.Root, out); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastHubTools()
	w.WriteHeader(http.StatusNoContent)
}

// handleReorderHubTools rewrites SortOrder on the given IDs in
// the supplied order. Tools not in the list keep their existing
// SortOrder so a partial reorder (drag one card to the top of a
// long list) doesn't trash the rest of the catalogue.
func (s *Server) handleReorderHubTools(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(body.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "ids required")
		return
	}
	if err := hub.ReorderTools(s.cfg.Vault.Root, body.IDs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastHubTools()
	w.WriteHeader(http.StatusNoContent)
}
