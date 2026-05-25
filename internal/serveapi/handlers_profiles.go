package serveapi

import (
	"net/http"

	"github.com/artaeon/granit/internal/modules"
	"github.com/artaeon/granit/internal/profiles"
	"github.com/artaeon/granit/internal/wshub"
)

// HTTP layer for the profile registry. Phase 1 web surface: list +
// activate. The "apply" semantics on activate touch the modules
// registry — switching profiles flips which features are enabled,
// matching what the TUI's profile-apply step does. Dashboard / layout
// application stays TUI-side for now; the web reads its own
// dashboard config independently.
//
// Routes (registered in server.go):
//   GET    /api/v1/profiles                    → list all + active ID
//   POST   /api/v1/profiles/{id}/activate      → set active + apply modules

// profileEntry mirrors profiles.Profile for the wire, dropping the
// internal-only fields and dehydrating the Dashboard spec into a
// boolean (has-or-no). The web doesn't currently apply dashboard
// layouts from profiles — that machinery lives in the TUI.
type profileEntry struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Description    string   `json:"description,omitempty"`
	BuiltIn        bool     `json:"builtIn"`
	EnabledModules []string `json:"enabledModules,omitempty"`
	DefaultLayout  string   `json:"defaultLayout,omitempty"`
	HasDashboard   bool     `json:"hasDashboard,omitempty"`
}

type profilesResponse struct {
	Profiles []profileEntry `json:"profiles"`
	ActiveID string         `json:"activeId"`
}

func toProfileEntry(p *profiles.Profile) profileEntry {
	return profileEntry{
		ID:             p.ID,
		Name:           p.Name,
		Description:    p.Description,
		BuiltIn:        p.BuiltIn,
		EnabledModules: p.EnabledModules,
		DefaultLayout:  p.DefaultLayout,
		HasDashboard:   len(p.Dashboard.Cells) > 0,
	}
}

func (s *Server) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	reg := s.profilesRegistry()
	all := reg.All()
	out := profilesResponse{
		Profiles: make([]profileEntry, 0, len(all)),
		ActiveID: reg.ActiveID(),
	}
	for _, p := range all {
		out.Profiles = append(out.Profiles, toProfileEntry(p))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleActivateProfile sets the active profile pointer. Phase 1 web
// stops here — modules are NOT touched. The built-in profile manifests
// (Classic / Daily Operator / Researcher / Builder / Student / Writer
// / Founder) carry TUI-shaped EnabledModules lists (`task_manager`,
// `goals_mode`, `daily_jot`, `project_mode`, …) that don't match the
// IDs the web modules registry knows about (`goals`, `projects`,
// `vision`, `prayer`, `roots`, etc.). Naively applying them via
// SetEnabledBatch disables nearly every web module — the bug the user
// hit on 2026-05-25 when their sidebar collapsed to a handful of
// items after a profile switch.
//
// Until a Phase 2 web-aware profile shape exists (separate web-module
// list, or a TUI→web ID mapping), the safe behaviour is: the active
// pointer flips, downstream layout/dashboard hints can be read by
// future surfaces, and module toggles stay where the user set them
// via /settings/features.
func (s *Server) handleActivateProfile(w http.ResponseWriter, r *http.Request) {
	id := urlParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "profile id required")
		return
	}
	preg := s.profilesRegistry()
	if err := preg.SetActive(id); err != nil {
		// SetActive returns ErrUnknownProfile for bad IDs. Map to 404
		// so the client can distinguish "you sent a typo" from "the
		// disk write failed".
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	// Surface profile entries that reference unknown module IDs in
	// the server log. Module-apply is deferred to Phase 2, but the
	// known-vs-unknown audit still helps diagnose profile authoring
	// mistakes — a TUI-targeted profile is expected to have unknowns
	// when run against the web registry.
	active := preg.Active()
	mreg := s.modulesRegistry()
	if unknown := unknownModuleIDs(active, mreg); len(unknown) > 0 {
		s.cfg.Logger.Debug("profiles: profile lists module IDs unknown to the web registry — these would be ignored if module-apply were enabled",
			"profile", id, "unknown", unknown)
	}
	if s.hub != nil {
		s.hub.Broadcast(wshub.Event{Type: "profile.changed"})
	}
	s.handleListProfiles(w, r)
}

// desiredModulesFor builds the enabled-batch map for a profile's
// EnabledModules list. Empty list means "Classic semantics" → return
// nil so the caller skips the batch entirely (every module keeps its
// current state). A non-empty list means "exactly these IDs enabled,
// everything else off" — except core IDs which we never include since
// they're always-on by definition.
func desiredModulesFor(p *profiles.Profile, mreg *modules.Registry) map[string]bool {
	if len(p.EnabledModules) == 0 {
		return nil
	}
	core := map[string]bool{}
	for _, id := range modules.CoreIDs {
		core[id] = true
	}
	want := map[string]bool{}
	for _, id := range p.EnabledModules {
		want[id] = true
	}
	out := map[string]bool{}
	for _, m := range mreg.All() {
		id := m.ID()
		if core[id] {
			continue
		}
		out[id] = want[id]
	}
	return out
}

// unknownModuleIDs returns the subset of p.EnabledModules that the
// modules registry doesn't know about. desiredModulesFor silently
// drops these (the iteration only visits registry-known IDs), so
// without this surfacing a profile typo would be invisible. Returns
// an empty slice when everything checks out.
func unknownModuleIDs(p *profiles.Profile, mreg *modules.Registry) []string {
	if len(p.EnabledModules) == 0 {
		return nil
	}
	known := map[string]bool{}
	for _, m := range mreg.All() {
		known[m.ID()] = true
	}
	// Core IDs aren't registered as toggleable modules but are also
	// not "unknown" — a profile listing them isn't a typo, just a
	// no-op since they're always-on.
	for _, id := range modules.CoreIDs {
		known[id] = true
	}
	var unknown []string
	for _, id := range p.EnabledModules {
		if !known[id] {
			unknown = append(unknown, id)
		}
	}
	return unknown
}
