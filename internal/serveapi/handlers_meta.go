package serveapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/goals"
	"github.com/artaeon/granit/internal/granitmeta"
	"github.com/artaeon/granit/internal/tasks"
	"github.com/artaeon/granit/internal/wshub"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
)

// projectView decorates the on-disk project with computed fields the web
// UI needs (progress + task counts) so the client doesn't need to recompute.
type projectView struct {
	granitmeta.Project
	Progress   float64 `json:"progress"`
	TasksDone  int     `json:"tasksDone"`
	TasksTotal int     `json:"tasksTotal"`
}

func projectMatches(p granitmeta.Project, t tasks.Task) bool {
	if p.Folder != "" && t.NotePath != "" {
		if strings.HasPrefix(t.NotePath, strings.TrimRight(p.Folder, "/")+"/") {
			return true
		}
	}
	if t.Project == p.Name {
		return true
	}
	return false
}

func computeProjectProgress(p granitmeta.Project) float64 {
	totalM, doneM := 0, 0
	for _, g := range p.Goals {
		totalM += len(g.Milestones)
		for _, m := range g.Milestones {
			if m.Done {
				doneM++
			}
		}
	}
	if totalM > 0 {
		return float64(doneM) / float64(totalM)
	}
	if len(p.Goals) > 0 {
		done := 0
		for _, g := range p.Goals {
			if g.Done {
				done++
			}
		}
		return float64(done) / float64(len(p.Goals))
	}
	return 0
}

func decorateProject(p granitmeta.Project, allTasks []tasks.Task) projectView {
	pv := projectView{Project: p, Progress: computeProjectProgress(p)}
	for _, t := range allTasks {
		if projectMatches(p, t) {
			pv.TasksTotal++
			if t.Done {
				pv.TasksDone++
			}
		}
	}
	if len(p.Goals) == 0 && pv.TasksTotal > 0 {
		// No goals tracked — fall back to task progress for the bar.
		pv.Progress = float64(pv.TasksDone) / float64(pv.TasksTotal)
	}
	return pv
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := granitmeta.ReadProjects(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	allTasks := s.cfg.TaskStore.All()
	out := make([]projectView, len(projects))
	for i, p := range projects {
		out[i] = decorateProject(p, allTasks)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"projects": out, "total": len(out)})
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	projects, err := granitmeta.ReadProjects(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, p := range projects {
		if p.Name == name {
			writeJSON(w, http.StatusOK, decorateProject(p, s.cfg.TaskStore.All()))
			return
		}
	}
	writeError(w, http.StatusNotFound, "project not found")
}

// handleCreateProject accepts the full Project schema and appends it.
// Name uniqueness is enforced — TUI keys projects by name.
func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var p granitmeta.Project
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(p.Name) == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	projects, err := granitmeta.ReadProjects(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, existing := range projects {
		if existing.Name == p.Name {
			writeError(w, http.StatusConflict, "project name already exists")
			return
		}
	}
	now := time.Now().Format("2006-01-02")
	if p.CreatedAt == "" {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	if p.Status == "" {
		p.Status = "active"
	}
	projects = append(projects, p)
	if err := granitmeta.WriteProjects(s.cfg.Vault.Root, projects); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.hub.Broadcast(wshub.Event{Type: "project.changed", ID: p.Name})
	writeJSON(w, http.StatusCreated, decorateProject(p, s.cfg.TaskStore.All()))
}

// handlePatchProject applies a partial update — any field present in the
// JSON body overwrites the stored value. Goals/Notes/Tags are replaced
// wholesale (not merged) so the client always sends the canonical list.
func (s *Server) handlePatchProject(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var patch map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	projects, err := granitmeta.ReadProjects(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	idx := -1
	for i, p := range projects {
		if p.Name == name {
			idx = i
			break
		}
	}
	if idx == -1 {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	p := projects[idx]
	// First malformed field wins — return 400 with the offending key
	// so the client knows which JSON it sent was bad. Earlier we
	// silently swallowed unmarshal errors which let bugs through
	// (a typo'd shape would 200 with the field unchanged and no
	// signal to the user).
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
		func() error { return apply("description", &p.Description) },
		func() error { return apply("folder", &p.Folder) },
		func() error { return apply("tags", &p.Tags) },
		func() error { return apply("status", &p.Status) },
		func() error { return apply("color", &p.Color) },
		func() error { return apply("category", &p.Category) },
		func() error { return apply("notes", &p.Notes) },
		func() error { return apply("task_filter", &p.TaskFilter) },
		func() error { return apply("goals", &p.Goals) },
		func() error { return apply("next_action", &p.NextAction) },
		func() error { return apply("priority", &p.Priority) },
		func() error { return apply("due_date", &p.DueDate) },
		func() error { return apply("time_spent", &p.TimeSpent) },
	} {
		if err := step(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	// Renaming is allowed but must not collide.
	if raw, ok := patch["name"]; ok {
		var newName string
		if err := json.Unmarshal(raw, &newName); err == nil && newName != "" && newName != p.Name {
			for i, existing := range projects {
				if i != idx && existing.Name == newName {
					writeError(w, http.StatusConflict, "project name already exists")
					return
				}
			}
			p.Name = newName
		}
	}
	p.UpdatedAt = time.Now().Format("2006-01-02")
	projects[idx] = p
	if err := granitmeta.WriteProjects(s.cfg.Vault.Root, projects); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.hub.Broadcast(wshub.Event{Type: "project.changed", ID: p.Name})
	writeJSON(w, http.StatusOK, decorateProject(p, s.cfg.TaskStore.All()))
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	projects, err := granitmeta.ReadProjects(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]granitmeta.Project, 0, len(projects))
	found := false
	for _, p := range projects {
		if p.Name == name {
			found = true
			continue
		}
		out = append(out, p)
	}
	if !found {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err := granitmeta.WriteProjects(s.cfg.Vault.Root, out); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.hub.Broadcast(wshub.Event{Type: "project.removed", ID: name})
	w.WriteHeader(http.StatusNoContent)
}

// ----- Events (events.json) -----

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	events, err := granitmeta.ReadEvents(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"events": events, "total": len(events)})
}

func (s *Server) handleCreateEvent(w http.ResponseWriter, r *http.Request) {
	var ev granitmeta.Event
	if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(ev.Title) == "" || strings.TrimSpace(ev.Date) == "" {
		writeError(w, http.StatusBadRequest, "title and date required")
		return
	}
	if ev.ID == "" {
		ev.ID = strings.ToLower(ulid.Make().String())
	}
	if ev.CreatedAt == "" {
		ev.CreatedAt = time.Now().Format(time.RFC3339)
	}
	events, err := granitmeta.ReadEvents(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	events = append(events, ev)
	if err := granitmeta.WriteEvents(s.cfg.Vault.Root, events); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.hub.Broadcast(wshub.Event{Type: "event.changed", ID: ev.ID})
	writeJSON(w, http.StatusCreated, ev)
}

func (s *Server) handlePatchEvent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var patch map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	events, err := granitmeta.ReadEvents(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	idx := -1
	for i, ev := range events {
		if ev.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		writeError(w, http.StatusNotFound, "event not found")
		return
	}
	ev := events[idx]
	apply := func(field string, into interface{}) {
		if raw, ok := patch[field]; ok {
			_ = json.Unmarshal(raw, into)
		}
	}
	apply("title", &ev.Title)
	apply("date", &ev.Date)
	apply("start_time", &ev.StartTime)
	apply("end_time", &ev.EndTime)
	apply("location", &ev.Location)
	apply("color", &ev.Color)
	events[idx] = ev
	if err := granitmeta.WriteEvents(s.cfg.Vault.Root, events); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.hub.Broadcast(wshub.Event{Type: "event.changed", ID: ev.ID})
	writeJSON(w, http.StatusOK, ev)
}

func (s *Server) handleDeleteEvent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	events, err := granitmeta.ReadEvents(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]granitmeta.Event, 0, len(events))
	found := false
	for _, ev := range events {
		if ev.ID == id {
			found = true
			continue
		}
		out = append(out, ev)
	}
	if !found {
		writeError(w, http.StatusNotFound, "event not found")
		return
	}
	if err := granitmeta.WriteEvents(s.cfg.Vault.Root, out); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.hub.Broadcast(wshub.Event{Type: "event.removed", ID: id})
	w.WriteHeader(http.StatusNoContent)
}

// ----- Goals -----

// handleListGoals returns the full Goal schema (including notes,
// review_frequency, last_reviewed, review_log, color, completed_at,
// and per-milestone due_date / completed_at). Earlier the handler went
// through granitmeta.Goal which silently dropped those fields — a web
// PATCH would round-trip and erase data the TUI had written. The new
// internal/goals package preserves every field.
func (s *Server) handleListGoals(w http.ResponseWriter, r *http.Request) {
	all := goals.LoadAll(s.cfg.Vault.Root)
	if all == nil {
		all = []goals.Goal{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"goals": all, "total": len(all)})
}

