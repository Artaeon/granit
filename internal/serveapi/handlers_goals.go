// Package serveapi — handlers for /api/v1/goals.
//
// Every read and write goes through internal/goals (single source of truth
// for the .granit/goals.json schema). The TUI was already on internal/goals
// after commit 62187f8 — these handlers bring the web to parity so a PATCH
// from a browser can no longer drop fields the TUI wrote (the granitmeta
// truncation bug we already fixed for goals; this just extends CRUD past
// the read-only listing we shipped previously).
//
// All write paths emit a `state.changed` WS event with
// `path: ".granit/goals.json"` so the web `/goals` page (and anything else
// listening) refetches without polling.
package serveapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/goals"
	"github.com/artaeon/granit/internal/wshub"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
)

const goalsStatePath = ".granit/goals.json"

// broadcastGoalsChanged tells WS subscribers that goals.json moved.
// Centralised so we never forget the path string on a new write site.
func (s *Server) broadcastGoalsChanged() {
	if s.hub == nil {
		return
	}
	s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: goalsStatePath})
}

// handleListGoals returns the full Goal schema (including notes,
// review_frequency, last_reviewed, review_log, color, completed_at,
// and per-milestone due_date / completed_at). Earlier the handler went
// through granitmeta.Goal which silently dropped those fields — a web
// PATCH would round-trip and erase data the TUI had written. The
// internal/goals package preserves every field.
func (s *Server) handleListGoals(w http.ResponseWriter, r *http.Request) {
	all := goals.LoadAll(s.cfg.Vault.Root)
	if all == nil {
		all = []goals.Goal{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"goals": all, "total": len(all)})
}

// handleCreateGoal mints a new Goal. Title is the only required field;
// status defaults to "active" and id is a ULID when the client doesn't
// supply one. Created/UpdatedAt stamps are RFC3339 (matches the TUI).
func (s *Server) handleCreateGoal(w http.ResponseWriter, r *http.Request) {
	var g goals.Goal
	if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(g.Title) == "" {
		writeError(w, http.StatusBadRequest, "title required")
		return
	}
	if g.ID == "" {
		g.ID = strings.ToLower(ulid.Make().String())
	}
	if g.Status == "" {
		g.Status = goals.StatusActive
	}
	now := time.Now().Format(time.RFC3339)
	if g.CreatedAt == "" {
		g.CreatedAt = now
	}
	g.UpdatedAt = now
	if g.Milestones == nil {
		g.Milestones = []goals.Milestone{}
	}

	all := goals.LoadAll(s.cfg.Vault.Root)
	for _, existing := range all {
		if existing.ID == g.ID {
			writeError(w, http.StatusConflict, "goal id already exists")
			return
		}
	}
	all = append(all, g)
	if err := goals.SaveAll(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastGoalsChanged()
	writeJSON(w, http.StatusCreated, g)
}

// handlePatchGoal applies a partial update. Only the explicit set of
// fields below can be changed via PATCH — id, created_at, milestones,
// review_log are all locked (milestones go through their own endpoints,
// reviews through /review, and the others are immutable).
//
// Each field decodes individually so a malformed shape returns a 400
// pointing at the offending key instead of a silent partial save.
func (s *Server) handlePatchGoal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var patch map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	all := goals.LoadAll(s.cfg.Vault.Root)
	idx := -1
	for i, g := range all {
		if g.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		writeError(w, http.StatusNotFound, "goal not found")
		return
	}
	g := all[idx]
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
		func() error { return apply("title", &g.Title) },
		func() error { return apply("description", &g.Description) },
		func() error { return apply("status", &g.Status) },
		func() error { return apply("category", &g.Category) },
		func() error { return apply("color", &g.Color) },
		func() error { return apply("tags", &g.Tags) },
		func() error { return apply("target_date", &g.TargetDate) },
		func() error { return apply("notes", &g.Notes) },
		func() error { return apply("review_frequency", &g.ReviewFrequency) },
		func() error { return apply("project", &g.Project) },
	} {
		if err := step(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	// Status transitions can imply timestamps. Setting completed stamps
	// completed_at (if not already), clearing it back to active wipes
	// the stamp so the badge mirrors reality.
	if _, statusInPatch := patch["status"]; statusInPatch {
		if g.Status == goals.StatusCompleted && g.CompletedAt == "" {
			g.CompletedAt = time.Now().Format(time.RFC3339)
		}
		if g.Status != goals.StatusCompleted {
			g.CompletedAt = ""
		}
	}
	g.UpdatedAt = time.Now().Format(time.RFC3339)
	all[idx] = g
	if err := goals.SaveAll(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastGoalsChanged()
	writeJSON(w, http.StatusOK, g)
}

// handleDeleteGoal removes the goal from goals.json. Idempotent on
// repeat (404 the second time is informative, not destructive).
func (s *Server) handleDeleteGoal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	all := goals.LoadAll(s.cfg.Vault.Root)
	out := make([]goals.Goal, 0, len(all))
	found := false
	for _, g := range all {
		if g.ID == id {
			found = true
			continue
		}
		out = append(out, g)
	}
	if !found {
		writeError(w, http.StatusNotFound, "goal not found")
		return
	}
	if err := goals.SaveAll(s.cfg.Vault.Root, out); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastGoalsChanged()
	w.WriteHeader(http.StatusNoContent)
}

// ----- Milestones -----

// milestoneInput is the wire shape of a milestone POST/PATCH. Fields are
// pointers so a PATCH can distinguish "not present" (don't touch) from
// "explicitly empty" (clear) — done is the only one that's "naturally"
// distinguishable but pointers keep the API symmetric.
type milestoneInput struct {
	Text    *string `json:"text,omitempty"`
	Done    *bool   `json:"done,omitempty"`
	DueDate *string `json:"due_date,omitempty"`
}

func (s *Server) findGoal(id string) (allGoals []goals.Goal, idx int) {
	allGoals = goals.LoadAll(s.cfg.Vault.Root)
	for i, g := range allGoals {
		if g.ID == id {
			return allGoals, i
		}
	}
	return allGoals, -1
}

// handleAddMilestone appends a new milestone. Text is required; due_date
// is optional. We deliberately don't go through goals.AddMilestone here
// because we want the same broadcast/validate path as the other writes.
func (s *Server) handleAddMilestone(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var in milestoneInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Text == nil || strings.TrimSpace(*in.Text) == "" {
		writeError(w, http.StatusBadRequest, "text required")
		return
	}
	all, idx := s.findGoal(id)
	if idx == -1 {
		writeError(w, http.StatusNotFound, "goal not found")
		return
	}
	m := goals.Milestone{Text: *in.Text}
	if in.DueDate != nil {
		m.DueDate = *in.DueDate
	}
	if in.Done != nil && *in.Done {
		m.Done = true
		m.CompletedAt = time.Now().Format(time.RFC3339)
	}
	all[idx].Milestones = append(all[idx].Milestones, m)
	all[idx].UpdatedAt = time.Now().Format(time.RFC3339)
	if err := goals.SaveAll(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastGoalsChanged()
	writeJSON(w, http.StatusCreated, all[idx])
}

// handlePatchMilestone toggles done / edits text / due_date on the
// milestone at the given index. Toggling done flips completed_at on or
// off so the timestamp matches the boolean — earlier the TUI was the
// only writer that maintained this invariant.
func (s *Server) handlePatchMilestone(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	idxStr := chi.URLParam(r, "idx")
	mi, err := strconv.Atoi(idxStr)
	if err != nil || mi < 0 {
		writeError(w, http.StatusBadRequest, "invalid milestone index")
		return
	}
	var in milestoneInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	all, gi := s.findGoal(id)
	if gi == -1 {
		writeError(w, http.StatusNotFound, "goal not found")
		return
	}
	if mi >= len(all[gi].Milestones) {
		writeError(w, http.StatusNotFound, "milestone not found")
		return
	}
	m := all[gi].Milestones[mi]
	if in.Text != nil {
		m.Text = *in.Text
	}
	if in.DueDate != nil {
		m.DueDate = *in.DueDate
	}
	if in.Done != nil {
		if *in.Done && !m.Done {
			m.CompletedAt = time.Now().Format(time.RFC3339)
		} else if !*in.Done {
			m.CompletedAt = ""
		}
		m.Done = *in.Done
	}
	all[gi].Milestones[mi] = m
	all[gi].UpdatedAt = time.Now().Format(time.RFC3339)
	if err := goals.SaveAll(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastGoalsChanged()
	writeJSON(w, http.StatusOK, all[gi])
}

// handleDeleteMilestone removes the milestone at idx.
func (s *Server) handleDeleteMilestone(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	idxStr := chi.URLParam(r, "idx")
	mi, err := strconv.Atoi(idxStr)
	if err != nil || mi < 0 {
		writeError(w, http.StatusBadRequest, "invalid milestone index")
		return
	}
	all, gi := s.findGoal(id)
	if gi == -1 {
		writeError(w, http.StatusNotFound, "goal not found")
		return
	}
	if mi >= len(all[gi].Milestones) {
		writeError(w, http.StatusNotFound, "milestone not found")
		return
	}
	all[gi].Milestones = append(all[gi].Milestones[:mi], all[gi].Milestones[mi+1:]...)
	all[gi].UpdatedAt = time.Now().Format(time.RFC3339)
	if err := goals.SaveAll(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastGoalsChanged()
	writeJSON(w, http.StatusOK, all[gi])
}

// ----- Reviews -----

type reviewInput struct {
	Note     string `json:"note"`
	Date     string `json:"date,omitempty"`
	Progress *int   `json:"progress,omitempty"`
}

// handleLogReview appends a review entry, refreshes last_reviewed, and
// (if the client didn't pin a progress value) snapshots the current
// milestone-progress percentage. Mirrors what the TUI's submitReview
// helper does so a review entry from either client looks identical.
func (s *Server) handleLogReview(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var in reviewInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	all, gi := s.findGoal(id)
	if gi == -1 {
		writeError(w, http.StatusNotFound, "goal not found")
		return
	}
	date := in.Date
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	progress := all[gi].Progress()
	if in.Progress != nil {
		progress = *in.Progress
	}
	all[gi].ReviewLog = append(all[gi].ReviewLog, goals.Review{
		Date:     date,
		Note:     in.Note,
		Progress: progress,
	})
	all[gi].LastReviewed = date
	all[gi].UpdatedAt = time.Now().Format(time.RFC3339)
	if err := goals.SaveAll(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastGoalsChanged()
	writeJSON(w, http.StatusOK, all[gi])
}
