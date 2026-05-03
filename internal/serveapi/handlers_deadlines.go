package serveapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/deadlines"
	"github.com/artaeon/granit/internal/wshub"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
)

// statePathDeadlines is the vault-relative path the WS broadcaster
// reports — the web's pages match on this exact string when subscribing
// to state.changed, so it's centralised here as a single literal.
const statePathDeadlines = ".granit/deadlines.json"

// broadcastDeadlinesChanged emits the WS event the web pages listen
// for (the calendar / dedicated deadlines route both refetch on this
// path). Centralised so every write site stays in lockstep.
func (s *Server) broadcastDeadlinesChanged() {
	s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: statePathDeadlines})
}

// handleListDeadlines returns the full deadline schema, sorted via
// SortForDisplay (active+missed first by date asc, then met+cancelled).
func (s *Server) handleListDeadlines(w http.ResponseWriter, r *http.Request) {
	all := deadlines.LoadAll(s.cfg.Vault.Root)
	out := deadlines.SortForDisplay(all)
	if out == nil {
		out = []deadlines.Deadline{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"deadlines": out,
		"total":     len(out),
	})
}

func (s *Server) handleGetDeadline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	all := deadlines.LoadAll(s.cfg.Vault.Root)
	d, idx := deadlines.FindByID(all, id)
	if idx == -1 {
		writeError(w, http.StatusNotFound, "deadline not found")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleCreateDeadline(w http.ResponseWriter, r *http.Request) {
	var d deadlines.Deadline
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(d.Title) == "" {
		writeError(w, http.StatusBadRequest, "title required")
		return
	}
	if !deadlines.ValidateDate(d.Date) {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	if d.ID == "" {
		d.ID = strings.ToLower(ulid.Make().String())
	}
	d.Importance = deadlines.NormalizeImportance(d.Importance)
	d.Status = deadlines.NormalizeStatus(d.Status)
	now := time.Now().UTC()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	d.UpdatedAt = now
	all := deadlines.LoadAll(s.cfg.Vault.Root)
	for _, existing := range all {
		if existing.ID == d.ID {
			writeError(w, http.StatusConflict, "deadline id already exists")
			return
		}
	}
	all = append(all, d)
	if err := deadlines.SaveAll(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastDeadlinesChanged()
	writeJSON(w, http.StatusCreated, d)
}

// handlePatchDeadline applies a partial update. A malformed field
// returns 400 with the offending key — silently ignoring bad shapes
// would let a buggy client write garbage and never know.
func (s *Server) handlePatchDeadline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var patch map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	all := deadlines.LoadAll(s.cfg.Vault.Root)
	d, idx := deadlines.FindByID(all, id)
	if idx == -1 {
		writeError(w, http.StatusNotFound, "deadline not found")
		return
	}
	apply := func(field string, into interface{}) error {
		raw, ok := patch[field]
		if !ok {
			return nil
		}
		if err := json.Unmarshal(raw, into); err != nil {
			return fmt.Errorf("field %q: %w", field, err)
		}
		return nil
	}
	for _, step := range []func() error{
		func() error { return apply("title", &d.Title) },
		func() error { return apply("date", &d.Date) },
		func() error { return apply("description", &d.Description) },
		func() error { return apply("goal_id", &d.GoalID) },
		func() error { return apply("project", &d.ProjectName) },
		func() error { return apply("task_ids", &d.TaskIDs) },
		func() error { return apply("importance", &d.Importance) },
		func() error { return apply("status", &d.Status) },
	} {
		if err := step(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if strings.TrimSpace(d.Title) == "" {
		writeError(w, http.StatusBadRequest, "title required")
		return
	}
	if !deadlines.ValidateDate(d.Date) {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	d.Importance = deadlines.NormalizeImportance(d.Importance)
	d.Status = deadlines.NormalizeStatus(d.Status)
	d.UpdatedAt = time.Now().UTC()
	all[idx] = d
	if err := deadlines.SaveAll(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastDeadlinesChanged()
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleDeleteDeadline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	all := deadlines.LoadAll(s.cfg.Vault.Root)
	out := make([]deadlines.Deadline, 0, len(all))
	found := false
	for _, d := range all {
		if d.ID == id {
			found = true
			continue
		}
		out = append(out, d)
	}
	if !found {
		writeError(w, http.StatusNotFound, "deadline not found")
		return
	}
	if err := deadlines.SaveAll(s.cfg.Vault.Root, out); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastDeadlinesChanged()
	w.WriteHeader(http.StatusNoContent)
}
