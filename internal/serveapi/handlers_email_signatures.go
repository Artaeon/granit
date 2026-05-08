// Package serveapi — handlers for /api/v1/email-signatures.
//
// CRUD over .granit/email-signatures.json. Tiny wrapper around
// internal/emailsignatures; no rendering on the server (the web
// renders inside an iframe sandbox so user-authored HTML can't
// fire scripts on the preview page).
package serveapi

import (
	"encoding/json"
	"net/http"

	"github.com/artaeon/granit/internal/emailsignatures"
	"github.com/artaeon/granit/internal/wshub"
	"github.com/go-chi/chi/v5"
)

const emailSignaturesStatePath = ".granit/email-signatures.json"

func (s *Server) bcastEmailSignatures() {
	if s.hub == nil {
		return
	}
	s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: emailSignaturesStatePath})
}

func (s *Server) handleListEmailSignatures(w http.ResponseWriter, r *http.Request) {
	list, err := emailsignatures.LoadAll(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"signatures": list, "total": len(list)})
}

func (s *Server) handleGetEmailSignature(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sig, err := emailsignatures.Find(s.cfg.Vault.Root, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sig == nil {
		writeError(w, http.StatusNotFound, "signature not found")
		return
	}
	writeJSON(w, http.StatusOK, sig)
}

func (s *Server) handleCreateEmailSignature(w http.ResponseWriter, r *http.Request) {
	var sig emailsignatures.Signature
	if err := json.NewDecoder(r.Body).Decode(&sig); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	sig.ID = "" // server mints
	out, err := emailsignatures.Upsert(s.cfg.Vault.Root, sig)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastEmailSignatures()
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handlePatchEmailSignature(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	current, err := emailsignatures.Find(s.cfg.Vault.Root, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if current == nil {
		writeError(w, http.StatusNotFound, "signature not found")
		return
	}
	// Patch: decode into a map so missing fields don't clobber.
	// Fields we accept on patch: name, html, plain_text, category,
	// is_default. created_at/updated_at are server-controlled.
	var p map[string]any
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if v, ok := p["name"].(string); ok {
		current.Name = v
	}
	if v, ok := p["html"].(string); ok {
		current.HTML = v
	}
	if v, ok := p["plain_text"].(string); ok {
		current.PlainText = v
	}
	if v, ok := p["category"].(string); ok {
		current.Category = v
	}
	if v, ok := p["is_default"].(bool); ok {
		current.IsDefault = v
	}
	out, err := emailsignatures.Upsert(s.cfg.Vault.Root, *current)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastEmailSignatures()
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteEmailSignature(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := emailsignatures.Delete(s.cfg.Vault.Root, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastEmailSignatures()
	w.WriteHeader(http.StatusNoContent)
}
