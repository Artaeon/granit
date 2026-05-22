package serveapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/granitmeta"
	"github.com/artaeon/granit/internal/tasks"
	"github.com/artaeon/granit/internal/wshub"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
)

// Date / time validators for event payloads. The handlers used to
// trust the client's strings verbatim, so a malformed payload could
// land an event with start_time="9 PM" or date="May 6" — strings the
// calendar feed couldn't parse, so the event silently disappeared
// from the grid. Validating at the boundary keeps the storage shape
// invariant and lets the API surface a clear 400 instead of a ghost
// event.
var (
	eventDateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	eventTimeRe = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
)

// validateEventTimes ensures date is YYYY-MM-DD, optional times are
// 24-hour HH:MM, and end (when present) is strictly after start.
// Empty start_time / end_time is allowed (the event is then "all-
// day-ish" — granitmeta accepts both shapes).
func validateEventTimes(date, start, end string) error {
	if !eventDateRe.MatchString(date) {
		return fmt.Errorf("date must be YYYY-MM-DD")
	}
	if start != "" && !eventTimeRe.MatchString(start) {
		return fmt.Errorf("start_time must be HH:MM (24-hour)")
	}
	if end != "" && !eventTimeRe.MatchString(end) {
		return fmt.Errorf("end_time must be HH:MM (24-hour)")
	}
	if start != "" && end != "" {
		if end <= start {
			return fmt.Errorf("end_time must be after start_time")
		}
	}
	return nil
}

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
	name := urlParam(r, "name")
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
	s.webhook.notify("project.changed")
	writeJSON(w, http.StatusCreated, decorateProject(p, s.cfg.TaskStore.All()))
}

// handlePatchProject applies a partial update — any field present in the
// JSON body overwrites the stored value. Goals/Notes/Tags are replaced
// wholesale (not merged) so the client always sends the canonical list.
func (s *Server) handlePatchProject(w http.ResponseWriter, r *http.Request) {
	name := urlParam(r, "name")
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
		func() error { return apply("kind", &p.Kind) },
		func() error { return apply("venture", &p.Venture) },
		func() error { return apply("repo_url", &p.RepoURL) },
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
	s.webhook.notify("project.changed")
	writeJSON(w, http.StatusOK, decorateProject(p, s.cfg.TaskStore.All()))
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	name := urlParam(r, "name")
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
	s.webhook.notify("project.removed")
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
	if err := validateEventTimes(ev.Date, ev.StartTime, ev.EndTime); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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
	apply("rrule", &ev.RRule)
	apply("ex_dates", &ev.ExDates)
	apply("project_id", &ev.ProjectID)
	apply("overrides", &ev.Overrides)
	apply("kind", &ev.Kind)
	// Validate AFTER apply so a partial patch (e.g. just start_time)
	// gets validated against the merged record. Catches "user shifted
	// the start past the end" without forcing them to also patch end.
	if err := validateEventTimes(ev.Date, ev.StartTime, ev.EndTime); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	events[idx] = ev
	if err := granitmeta.WriteEvents(s.cfg.Vault.Root, events); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.hub.Broadcast(wshub.Event{Type: "event.changed", ID: ev.ID})
	writeJSON(w, http.StatusOK, ev)
}

// handleOverrideEventOccurrence sets a per-occurrence override on a
// recurring event so a single instance can move/rename without
// disturbing the rest of the series. The user grabs Tuesday 9am and
// drags it to Wednesday noon — without overrides we'd have to
// rewrite the SERIES base (shifting every Tuesday). With overrides,
// we annotate THIS Tuesday's anchor with a per-instance patch and
// the expander surfaces the patched version on render.
//
// Body: { "key": "<exdate-shape key>", "override": {...} }. The key
// identifies which occurrence (mirrors EXDATE form). An override with
// every field empty effectively clears the override (we drop it from
// the map). To cancel an occurrence outright, the existing /skip
// endpoint adds an EXDATE — overrides and skips are siblings, not
// nested.
func (s *Server) handleOverrideEventOccurrence(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Key      string                  `json:"key"`
		Override granitmeta.EventOverride `json:"override"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Key = strings.TrimSpace(body.Key)
	if body.Key == "" {
		writeError(w, http.StatusBadRequest, "key required")
		return
	}
	// Validate optional time fields — same shape rules as the regular
	// patch path. Date is YYYY-MM-DD; StartTime / EndTime are HH:MM.
	if body.Override.Date != "" && !eventDateRe.MatchString(body.Override.Date) {
		writeError(w, http.StatusBadRequest, "override date must be YYYY-MM-DD")
		return
	}
	if body.Override.StartTime != "" && !eventTimeRe.MatchString(body.Override.StartTime) {
		writeError(w, http.StatusBadRequest, "override start_time must be HH:MM (24-hour)")
		return
	}
	if body.Override.EndTime != "" && !eventTimeRe.MatchString(body.Override.EndTime) {
		writeError(w, http.StatusBadRequest, "override end_time must be HH:MM (24-hour)")
		return
	}
	if body.Override.StartTime != "" && body.Override.EndTime != "" &&
		body.Override.EndTime <= body.Override.StartTime {
		writeError(w, http.StatusBadRequest, "end_time must be after start_time")
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
	if events[idx].RRule == "" {
		writeError(w, http.StatusBadRequest, "event is not recurring")
		return
	}
	// Empty override clears the entry. An override that's purely
	// title/location/etc with no shift is allowed (the user might
	// only want to rename "this Tuesday's standup" without moving it).
	if body.Override == (granitmeta.EventOverride{}) {
		if events[idx].Overrides != nil {
			delete(events[idx].Overrides, body.Key)
			if len(events[idx].Overrides) == 0 {
				events[idx].Overrides = nil
			}
		}
	} else {
		if events[idx].Overrides == nil {
			events[idx].Overrides = map[string]granitmeta.EventOverride{}
		}
		events[idx].Overrides[body.Key] = body.Override
	}
	if err := granitmeta.WriteEvents(s.cfg.Vault.Root, events); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.hub.Broadcast(wshub.Event{Type: "event.changed", ID: events[idx].ID})
	writeJSON(w, http.StatusOK, events[idx])
}

// handleSkipEventOccurrence appends a date to an event's ExDates
// list so the recurrence expander filters that single occurrence
// from the rendered calendar — the user's "cancel just this week's
// meeting" action without disrupting the rest of the series. The
// date must be YYYY-MM-DD (all-day) or YYYY-MM-DDTHH:MM:SS (timed)
// to match the format the expander compares against. No-op when the
// date is already in the list. Errors when the event isn't recurring
// (no RRule) or doesn't exist.
func (s *Server) handleSkipEventOccurrence(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Date string `json:"date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Date = strings.TrimSpace(body.Date)
	if body.Date == "" {
		writeError(w, http.StatusBadRequest, "date required")
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
	if events[idx].RRule == "" {
		writeError(w, http.StatusBadRequest, "event is not recurring")
		return
	}
	already := false
	for _, x := range events[idx].ExDates {
		if x == body.Date {
			already = true
			break
		}
	}
	if !already {
		events[idx].ExDates = append(events[idx].ExDates, body.Date)
		if err := granitmeta.WriteEvents(s.cfg.Vault.Root, events); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.hub.Broadcast(wshub.Event{Type: "event.changed", ID: events[idx].ID})
	}
	writeJSON(w, http.StatusOK, events[idx])
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

// Goals — see handlers_goals.go for the full surface (list/create/patch/
// delete + milestones + reviews). Kept in a separate file so this one
// stays under the size budget for a project-and-event handler.

