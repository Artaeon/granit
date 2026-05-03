package serveapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/prayer"
	"github.com/artaeon/granit/internal/wshub"
	"github.com/go-chi/chi/v5"
)

const statePathPrayer = ".granit/prayer/intentions.json"

func (s *Server) bcastPrayer() {
	s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: statePathPrayer})
}

func (s *Server) handleListPrayer(w http.ResponseWriter, r *http.Request) {
	out := prayer.SortForDisplay(prayer.LoadAll(s.cfg.Vault.Root))
	if out == nil {
		out = []prayer.Intention{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"intentions": out, "total": len(out)})
}

func (s *Server) handleCreatePrayer(w http.ResponseWriter, r *http.Request) {
	var p prayer.Intention
	if !readJSON(w, r, &p) {
		return
	}
	if strings.TrimSpace(p.Text) == "" {
		writeError(w, http.StatusBadRequest, "text required")
		return
	}
	p.Status = prayer.NormalizeStatus(p.Status)
	if p.ID == "" {
		p.ID = newULID()
	}
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	if p.StartedAt == "" {
		p.StartedAt = time.Now().Format("2006-01-02")
	}
	all := append(prayer.LoadAll(s.cfg.Vault.Root), p)
	if err := prayer.SaveAll(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastPrayer()
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handlePatchPrayer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	all := prayer.LoadAll(s.cfg.Vault.Root)
	p, idx := prayer.FindByID(all, id)
	if idx == -1 {
		writeError(w, http.StatusNotFound, "intention not found")
		return
	}
	var patch map[string]json.RawMessage
	if !readJSON(w, r, &patch) {
		return
	}
	apply := func(k string, dst any) {
		if raw, ok := patch[k]; ok {
			_ = json.Unmarshal(raw, dst)
		}
	}
	apply("text", &p.Text)
	apply("category", &p.Category)
	apply("answer", &p.Answer)
	apply("notes", &p.Notes)
	if raw, ok := patch["status"]; ok {
		var v string
		_ = json.Unmarshal(raw, &v)
		newStatus := prayer.NormalizeStatus(v)
		// Stamp answered_at the first time a transition into "answered"
		// happens. Doesn't override an existing answered_at on
		// re-status (e.g. archived → answered) because the original
		// answer date is the meaningful one.
		if p.Status != string(prayer.StatusAnswered) && newStatus == string(prayer.StatusAnswered) && p.AnsweredAt == "" {
			p.AnsweredAt = time.Now().Format("2006-01-02")
		}
		p.Status = newStatus
	}
	apply("started_at", &p.StartedAt)
	apply("answered_at", &p.AnsweredAt)
	p.UpdatedAt = time.Now().UTC()
	all[idx] = p
	if err := prayer.SaveAll(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastPrayer()
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeletePrayer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	all := prayer.LoadAll(s.cfg.Vault.Root)
	out := all[:0]
	found := false
	for _, p := range all {
		if p.ID == id {
			found = true
			continue
		}
		out = append(out, p)
	}
	if !found {
		writeError(w, http.StatusNotFound, "intention not found")
		return
	}
	if err := prayer.SaveAll(s.cfg.Vault.Root, out); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastPrayer()
	w.WriteHeader(http.StatusNoContent)
}
