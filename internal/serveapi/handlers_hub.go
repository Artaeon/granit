// Package serveapi — handlers for /api/v1/hub.
//
// The hub is granit's "single login, find everything I need"
// surface — a small CRUD over .granit/hub.json. Wraps the
// internal/hub package with list / create / get / patch / delete.
//
// Credentials are stored unencrypted on disk (file system perms
// are the only protection); the UI carries the "use a real
// password manager for sensitive secrets" caveat. We deliberately
// don't add encryption ceremony at this layer — adding crypto
// without a real key-management story is security theatre and
// would mislead the user about the actual protection model.
package serveapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/artaeon/granit/internal/hub"
	"github.com/artaeon/granit/internal/wshub"
	"github.com/go-chi/chi/v5"
)

const hubStatePath = ".granit/hub.json"

func (s *Server) bcastHub() {
	if s.hub == nil {
		return
	}
	s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: hubStatePath})
}

func (s *Server) handleListHubItems(w http.ResponseWriter, r *http.Request) {
	items, err := hub.LoadAll(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []hub.Item{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) handleCreateHubItem(w http.ResponseWriter, r *http.Request) {
	var item hub.Item
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	item.Title = strings.TrimSpace(item.Title)
	if item.Title == "" {
		writeError(w, http.StatusBadRequest, "title required")
		return
	}
	if item.ID == "" {
		item.ID = hub.NewID()
	}
	now := hub.Now()
	item.CreatedAt = now
	item.UpdatedAt = now

	items, err := hub.LoadAll(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items = append(items, item)
	if err := hub.SaveAll(s.cfg.Vault.Root, items); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastHub()
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) handlePatchHubItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	// Unmarshal into a partial map so we can apply only the keys the
	// client sent, leaving everything else intact. Mirrors the
	// patch shape the deadlines / events handlers use.
	var patch map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	items, err := hub.LoadAll(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	idx := -1
	for i, it := range items {
		if it.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	it := items[idx]
	apply := func(key string, into interface{}) {
		if raw, ok := patch[key]; ok {
			_ = json.Unmarshal(raw, into)
		}
	}
	apply("title", &it.Title)
	apply("url", &it.URL)
	apply("category", &it.Category)
	apply("icon", &it.Icon)
	apply("notes", &it.Notes)
	apply("username", &it.Username)
	apply("password", &it.Password)
	apply("favorite", &it.Favorite)
	it.Title = strings.TrimSpace(it.Title)
	if it.Title == "" {
		writeError(w, http.StatusBadRequest, "title required")
		return
	}
	it.UpdatedAt = hub.Now()
	items[idx] = it
	if err := hub.SaveAll(s.cfg.Vault.Root, items); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastHub()
	writeJSON(w, http.StatusOK, it)
}

func (s *Server) handleDeleteHubItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	items, err := hub.LoadAll(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := items[:0]
	found := false
	for _, it := range items {
		if it.ID == id {
			found = true
			continue
		}
		out = append(out, it)
	}
	if !found {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	if err := hub.SaveAll(s.cfg.Vault.Root, out); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastHub()
	w.WriteHeader(http.StatusNoContent)
}
