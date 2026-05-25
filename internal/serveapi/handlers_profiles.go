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

// handleActivateProfile sets the active profile pointer AND applies
// the profile's EnabledModules to the modules registry. An empty
// EnabledModules slice means "Classic semantics — enable everything
// the registry knows"; an explicit list narrows the enabled set to
// exactly those IDs.
//
// Core modules (notes / tasks / calendar / settings — anything in
// modules.CoreIDs) are skipped entirely: they're always-on by
// definition and a profile can't disable them without breaking the
// app.
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
	// Apply the profile's module set. Profiles use empty-slice to mean
	// "enable everything" (Classic); a non-empty slice narrows to
	// exactly those IDs.
	active := preg.Active()
	mreg := s.modulesRegistry()
	desired := desiredModulesFor(active, mreg)
	if len(desired) > 0 {
		if err := mreg.SetEnabledBatch(desired); err != nil {
			// Don't roll back the active pointer — the user successfully
			// switched, just the module-side application stumbled. Better
			// to leave them on the new profile with their old modules
			// than fail-fast and confuse them.
			s.cfg.Logger.Warn("profiles: applying modules failed", "profile", id, "err", err)
		} else if err := mreg.Save(); err != nil {
			s.cfg.Logger.Warn("profiles: saving modules failed", "profile", id, "err", err)
		}
	}
	// Fan out so the web SPA refreshes its profile + modules stores
	// without polling.
	if s.hub != nil {
		s.hub.Broadcast(wshub.Event{Type: "profile.changed"})
		s.hub.Broadcast(wshub.Event{Type: "modules.changed"})
	}
	// Echo the post-change list so the client doesn't need a follow-up
	// GET to refresh.
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
