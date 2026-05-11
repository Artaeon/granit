package serveapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/artaeon/granit/internal/aimemory"
	"github.com/artaeon/granit/internal/wshub"
	"github.com/go-chi/chi/v5"
)

// AI memory CRUD — long-term facts the chat overlay injects into
// every thread's system prelude. Vault-scoped: <vault>/.granit/
// ai-memory.json. No auth distinction (same bearer as everything
// else in granit); we already operate single-tenant + the data
// is on the user's own box.

type aiMemoryItemBody struct {
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

func (s *Server) handleListAIMemory(w http.ResponseWriter, r *http.Request) {
	facts, err := aimemory.Snapshot(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if facts == nil {
		facts = []aimemory.Fact{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"facts": facts,
		"total": len(facts),
	})
}

func (s *Server) handleAddAIMemory(w http.ResponseWriter, r *http.Request) {
	var body aiMemoryItemBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(body.Content) == "" {
		writeError(w, http.StatusBadRequest, "content required")
		return
	}
	f, err := aimemory.Add(s.cfg.Vault.Root, body.Content, body.Tags)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.broadcastAIMemoryChanged()
	writeJSON(w, http.StatusCreated, f)
}

func (s *Server) handlePatchAIMemory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id required")
		return
	}
	var body aiMemoryItemBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	f, err := aimemory.Patch(s.cfg.Vault.Root, id, body.Content, body.Tags)
	if err != nil {
		if errors.Is(err, aimemory.ErrNotFound) {
			writeError(w, http.StatusNotFound, "fact not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.broadcastAIMemoryChanged()
	writeJSON(w, http.StatusOK, f)
}

func (s *Server) handleDeleteAIMemory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id required")
		return
	}
	if err := aimemory.Delete(s.cfg.Vault.Root, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastAIMemoryChanged()
	w.WriteHeader(http.StatusNoContent)
}

// broadcastAIMemoryChanged emits a state.changed event so any open
// chat overlay can re-fetch and re-inject on the next turn. Uses
// the existing state.changed shape so a future "settings panel
// showing memory" just listens like any other vault state.
func (s *Server) broadcastAIMemoryChanged() {
	if s.hub == nil {
		return
	}
	s.hub.Broadcast(wshub.Event{
		Type: "state.changed",
		Path: ".granit/ai-memory.json",
	})
}
