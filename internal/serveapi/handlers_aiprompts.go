package serveapi

import (
	"net/http"
	"strings"

	"github.com/artaeon/granit/internal/aiprompts"
	"github.com/artaeon/granit/internal/wshub"
	"github.com/oklog/ulid/v2"
)

const statePathAIPrompts = ".granit/ai-prompts.json"

func (s *Server) bcastAIPrompts() {
	s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: statePathAIPrompts})
}

// handleGetAIPrompts returns the user's saved prompt library.
// Always 200; missing/corrupt file yields an empty list.
func (s *Server) handleGetAIPrompts(w http.ResponseWriter, r *http.Request) {
	lib := aiprompts.Load(s.cfg.Vault.Root)
	writeJSON(w, http.StatusOK, lib)
}

// handlePutAIPrompts is a full upsert — the client sends the whole
// library. Entries missing an ID get a fresh ULID stamped here, so
// the client can POST a brand-new entry with just {label, prompt,
// scope} and trust the server to fill the audit fields.
//
// Validation lives in the aiprompts package; we surface errors as
// 400 so the UI can show the user what was wrong with the request.
func (s *Server) handlePutAIPrompts(w http.ResponseWriter, r *http.Request) {
	var incoming aiprompts.Library
	if !readJSON(w, r, &incoming) {
		return
	}
	for i := range incoming.Entries {
		if strings.TrimSpace(incoming.Entries[i].ID) == "" {
			incoming.Entries[i].ID = strings.ToLower(ulid.Make().String())
		}
	}
	if err := aiprompts.Save(s.cfg.Vault.Root, incoming); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.bcastAIPrompts()
	writeJSON(w, http.StatusOK, aiprompts.Load(s.cfg.Vault.Root))
}
