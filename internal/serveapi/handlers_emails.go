package serveapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/artaeon/granit/internal/email"
)

// handleListEmails returns every email tracked in the vault. The
// frontend filters / groups client-side; the dataset is small
// enough (a few hundred entries for an active user) that paging
// would be premature.
func (s *Server) handleListEmails(w http.ResponseWriter, r *http.Request) {
	items, err := email.LoadAll(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"emails": items,
		"total":  len(items),
	})
}

func (s *Server) handleGetEmail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	items, err := email.LoadAll(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	found := email.Find(items, id)
	if found == nil {
		writeError(w, http.StatusNotFound, "email not found")
		return
	}
	writeJSON(w, http.StatusOK, *found)
}

// handleCreateEmail accepts a partial email body and persists it.
// Server-side it stamps a fresh ULID + CreatedAt + UpdatedAt; any
// of those fields the client sends are ignored to prevent
// fabricated histories.
func (s *Server) handleCreateEmail(w http.ResponseWriter, r *http.Request) {
	var b email.Email
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	// Server stamps these regardless of what the client sent.
	now := time.Now().UTC().Format(time.RFC3339)
	fresh := email.New()
	b.ID = fresh.ID
	b.CreatedAt = fresh.CreatedAt
	b.UpdatedAt = now
	if b.Status == "" {
		b.Status = email.StatusInbox
	}
	if b.Direction == "" {
		b.Direction = email.DirectionIn
	}
	// Trim incoming strings so leading/trailing whitespace from
	// paste-in doesn't haunt the index.
	b.Subject = strings.TrimSpace(b.Subject)
	b.From = strings.TrimSpace(b.From)
	b.Project = strings.TrimSpace(b.Project)
	if err := b.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	items, err := email.LoadAll(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items = append(items, b)
	if err := email.SaveAll(s.cfg.Vault.Root, items); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

// handlePatchEmail merges the provided fields into an existing
// email. Bumps UpdatedAt. ID / CreatedAt are ignored even when
// the client sends them — the audit trail is server-controlled.
func (s *Server) handlePatchEmail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var patch map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	items, err := email.LoadAll(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	found := email.Find(items, id)
	if found == nil {
		writeError(w, http.StatusNotFound, "email not found")
		return
	}
	// Apply field-by-field. We could marshal/unmarshal but the
	// explicit list keeps the server in control of which fields
	// are mutable + what their types are.
	if v, ok := patch["subject"].(string); ok {
		found.Subject = strings.TrimSpace(v)
	}
	if v, ok := patch["direction"].(string); ok {
		found.Direction = email.Direction(v)
	}
	if v, ok := patch["from"].(string); ok {
		found.From = strings.TrimSpace(v)
	}
	if v, ok := patch["to"].([]interface{}); ok {
		found.To = stringSliceFromAny(v)
	}
	if v, ok := patch["cc"].([]interface{}); ok {
		found.Cc = stringSliceFromAny(v)
	}
	if v, ok := patch["body"].(string); ok {
		found.Body = v
	}
	if v, ok := patch["received_at"].(string); ok {
		found.ReceivedAt = v
	}
	if v, ok := patch["sent_at"].(string); ok {
		found.SentAt = v
	}
	if v, ok := patch["status"].(string); ok {
		found.Status = email.Status(v)
	}
	if v, ok := patch["tags"].([]interface{}); ok {
		found.Tags = stringSliceFromAny(v)
	}
	if v, ok := patch["follow_up_date"].(string); ok {
		found.FollowUpDate = v
	}
	if v, ok := patch["person_id"].(string); ok {
		found.PersonID = v
	}
	if v, ok := patch["project"].(string); ok {
		found.Project = strings.TrimSpace(v)
	}
	found.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := found.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := email.SaveAll(s.cfg.Vault.Root, items); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, *found)
}

func (s *Server) handleDeleteEmail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	items, err := email.LoadAll(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := items[:0]
	found := false
	for _, e := range items {
		if e.ID == id {
			found = true
			continue
		}
		out = append(out, e)
	}
	if !found {
		writeError(w, http.StatusNotFound, "email not found")
		return
	}
	if err := email.SaveAll(s.cfg.Vault.Root, out); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// stringSliceFromAny coerces a []interface{} (the shape encoding/
// json gives for arrays in a generic map) into a clean []string.
// Non-string entries are silently dropped — a typed PATCH
// schema would be stricter but this is enough for a personal CRM.
func stringSliceFromAny(in []interface{}) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if s, ok := v.(string); ok {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}
