// Package serveapi — handlers for /api/v1/ventures.
//
// A venture is the umbrella entity above projects + goals. Project.Venture
// and Goal.Venture stay as free-text strings (so existing data round-trips
// unchanged); this endpoint manages the optional enrichment record that
// adds description, mission, color, etc. to a venture name.
//
// Decoration on list/get: each response includes project_count + goal_count
// derived from the two existing JSON sidecars so the UI doesn't have to
// fetch + cross-reference three endpoints to render the rollup.
package serveapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/goals"
	"github.com/artaeon/granit/internal/granitmeta"
	"github.com/artaeon/granit/internal/ventures"
	"github.com/artaeon/granit/internal/wshub"
	"github.com/go-chi/chi/v5"
)

const venturesStatePath = ".granit/ventures.json"

// ventureView decorates the on-disk record with computed counts so the
// /ventures page can render rollup totals without two extra fetches.
type ventureView struct {
	ventures.Venture
	ProjectCount int `json:"project_count"`
	GoalCount    int `json:"goal_count"`
}

// broadcastVenturesChanged tells WS subscribers ventures.json moved.
// Centralised so a future write site can't forget the path string.
func (s *Server) broadcastVenturesChanged() {
	if s.hub == nil {
		return
	}
	s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: venturesStatePath})
}

// countLinks returns (projects, goals) referencing the given venture
// name. Case-insensitive on the name to match ventures.Find — a project
// with venture="acme" should count toward a Venture record named
// "Acme". Empty venture strings on either side are skipped.
func countLinks(name string, projects []granitmeta.Project, allGoals []goals.Goal) (int, int) {
	target := strings.ToLower(strings.TrimSpace(name))
	if target == "" {
		return 0, 0
	}
	pCount := 0
	for _, p := range projects {
		if strings.EqualFold(p.Venture, target) {
			pCount++
		}
	}
	gCount := 0
	for _, g := range allGoals {
		if strings.EqualFold(g.Venture, target) {
			gCount++
		}
	}
	return pCount, gCount
}

// decorateVenture pulls the link counts. Reads project + goal sidecars
// once per response; cheap relative to the cost of a venture page render.
func (s *Server) decorateVenture(v ventures.Venture) ventureView {
	projects, _ := granitmeta.ReadProjects(s.cfg.Vault.Root)
	all := goals.LoadAll(s.cfg.Vault.Root)
	p, g := countLinks(v.Name, projects, all)
	return ventureView{Venture: v, ProjectCount: p, GoalCount: g}
}

func (s *Server) handleListVentures(w http.ResponseWriter, r *http.Request) {
	list := ventures.LoadAll(s.cfg.Vault.Root)
	if list == nil {
		list = []ventures.Venture{}
	}
	// Read project + goal sidecars once and reuse for every venture's
	// link counts — N venture lookups against in-memory slices instead
	// of N filesystem reads.
	projects, _ := granitmeta.ReadProjects(s.cfg.Vault.Root)
	all := goals.LoadAll(s.cfg.Vault.Root)
	out := make([]ventureView, len(list))
	for i, v := range list {
		p, g := countLinks(v.Name, projects, all)
		out[i] = ventureView{Venture: v, ProjectCount: p, GoalCount: g}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ventures": out, "total": len(out)})
}

func (s *Server) handleGetVenture(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	list := ventures.LoadAll(s.cfg.Vault.Root)
	v := ventures.Find(list, name)
	if v == nil {
		writeError(w, http.StatusNotFound, "venture not found")
		return
	}
	writeJSON(w, http.StatusOK, s.decorateVenture(*v))
}

// handleCreateVenture accepts the full Venture schema and appends it.
// Name uniqueness is enforced (case-insensitive) — projects/goals key by
// name and a duplicate would silently merge their rollups.
func (s *Server) handleCreateVenture(w http.ResponseWriter, r *http.Request) {
	var v ventures.Venture
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := v.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	v.Name = strings.TrimSpace(v.Name)
	list := ventures.LoadAll(s.cfg.Vault.Root)
	if existing := ventures.Find(list, v.Name); existing != nil {
		writeError(w, http.StatusConflict, "venture name already exists")
		return
	}
	now := time.Now().Format("2006-01-02")
	if v.CreatedAt == "" {
		v.CreatedAt = now
	}
	v.UpdatedAt = now
	if v.Status == "" {
		v.Status = ventures.StatusActive
	}
	list = append(list, v)
	if err := ventures.SaveAll(s.cfg.Vault.Root, list); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastVenturesChanged()
	writeJSON(w, http.StatusCreated, s.decorateVenture(v))
}

// handlePatchVenture applies a partial update — any field present in the
// JSON body overwrites the stored value. Tags is replaced wholesale (not
// merged) so the client always sends the canonical list. Renaming is
// allowed but must not collide; project.venture / goal.venture refs to
// the OLD name will silently stop matching, which is the same trade-off
// the project rename path makes (no transitive repointing).
func (s *Server) handlePatchVenture(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var patch map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	list := ventures.LoadAll(s.cfg.Vault.Root)
	idx := -1
	for i := range list {
		if strings.EqualFold(list[i].Name, name) {
			idx = i
			break
		}
	}
	if idx == -1 {
		writeError(w, http.StatusNotFound, "venture not found")
		return
	}
	v := list[idx]
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
		func() error { return apply("description", &v.Description) },
		func() error { return apply("mission", &v.Mission) },
		func() error { return apply("color", &v.Color) },
		func() error { return apply("status", &v.Status) },
		func() error { return apply("url", &v.URL) },
		func() error { return apply("tags", &v.Tags) },
	} {
		if err := step(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if raw, ok := patch["name"]; ok {
		var newName string
		if err := json.Unmarshal(raw, &newName); err == nil {
			newName = strings.TrimSpace(newName)
			if newName != "" && !strings.EqualFold(newName, v.Name) {
				for i := range list {
					if i != idx && strings.EqualFold(list[i].Name, newName) {
						writeError(w, http.StatusConflict, "venture name already exists")
						return
					}
				}
				v.Name = newName
			}
		}
	}
	v.Touch()
	list[idx] = v
	if err := ventures.SaveAll(s.cfg.Vault.Root, list); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastVenturesChanged()
	writeJSON(w, http.StatusOK, s.decorateVenture(v))
}

func (s *Server) handleDeleteVenture(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	list := ventures.LoadAll(s.cfg.Vault.Root)
	out := make([]ventures.Venture, 0, len(list))
	found := false
	for _, v := range list {
		if strings.EqualFold(v.Name, name) {
			found = true
			continue
		}
		out = append(out, v)
	}
	if !found {
		writeError(w, http.StatusNotFound, "venture not found")
		return
	}
	if err := ventures.SaveAll(s.cfg.Vault.Root, out); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastVenturesChanged()
	w.WriteHeader(http.StatusNoContent)
}
