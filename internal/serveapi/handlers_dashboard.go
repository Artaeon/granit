package serveapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

const dashboardFileRel = ".granit/everything-dashboard.json"
const dashboardVersion = 1

type dashboardWidget struct {
	ID      string                 `json:"id"`
	Type    string                 `json:"type"`
	Enabled bool                   `json:"enabled"`
	Config  map[string]interface{} `json:"config,omitempty"`
}

// dashboardLayout is one named preset (focus / morning / shutdown /
// custom). The user can save the current widget arrangement as a
// preset and switch between them; switching copies the layout's
// Widgets into the top-level Widgets so the active arrangement is
// always denormalised at the top of the config — older client builds
// that don't know about layouts keep working unchanged.
type dashboardLayout struct {
	Name    string            `json:"name"`
	Widgets []dashboardWidget `json:"widgets"`
}

type dashboardConfig struct {
	Version int               `json:"version"`
	// Widgets holds the active arrangement. Source of truth for what
	// the dashboard renders. Keeping it at the top level means a
	// pre-layouts client just reads Widgets and ignores Layouts.
	Widgets []dashboardWidget `json:"widgets"`
	// Active is the name of the currently-applied layout (must match
	// one of Layouts[i].Name) or "" when no preset is active. Empty
	// means the user is on an ad-hoc arrangement that hasn't been
	// saved as a preset — switching to a preset later overwrites
	// Widgets but leaves Layouts intact.
	Active string `json:"active,omitempty"`
	// Layouts is the catalogue of saved presets. Empty for users
	// who haven't created any (the legacy single-layout case).
	Layouts []dashboardLayout `json:"layouts,omitempty"`
}

func defaultDashboard() dashboardConfig {
	return dashboardConfig{
		Version: dashboardVersion,
		Widgets: []dashboardWidget{
			{ID: "w-greeting", Type: "greeting", Enabled: true},
			// at-a-glance sits second in the default config so a fresh
			// install reads "shape of today" right under the greeting.
			// Existing users keep their saved layout — this only
			// affects users with no dashboard config on disk yet.
			{ID: "w-at-a-glance", Type: "at-a-glance", Enabled: true},
			{ID: "w-now", Type: "now", Enabled: true},
			{ID: "w-streaks", Type: "streaks", Enabled: true},
			{ID: "w-scripture", Type: "scripture", Enabled: true},
			{ID: "w-pinned", Type: "pinned", Enabled: true},
			{ID: "w-daily", Type: "daily-note", Enabled: true},
			{ID: "w-habits", Type: "habits", Enabled: true},
			{ID: "w-pomodoro", Type: "pomodoro", Enabled: false},
			{ID: "w-quick-capture", Type: "quick-capture", Enabled: true},
			{ID: "w-today-tasks", Type: "today-tasks", Enabled: true},
			{ID: "w-scheduled", Type: "scheduled-today", Enabled: true},
			{ID: "w-goals", Type: "goals-progress", Enabled: true},
			{ID: "w-recent", Type: "recent-notes", Enabled: true},
			{ID: "w-projects", Type: "projects-active", Enabled: true},
			{ID: "w-install", Type: "install", Enabled: true},
			{ID: "w-inbox", Type: "inbox", Enabled: false},
			{ID: "w-week", Type: "calendar-week", Enabled: false},
		},
	}
}

// widgetTypeToModuleID maps a dashboard widget Type to the module ID
// that gates it. A widget without an entry stays visible regardless of
// module state — that's the right default for "always-on" widgets like
// greeting / now / quick-capture / daily-note / pinned that aren't
// owned by any toggleable module.
var widgetTypeToModuleID = map[string]string{
	"goals-progress":  "goals",
	"projects-active": "projects",
	"habits":          "habit_tracker",
	"streaks":         "habit_tracker",
	"top-deadlines":   "deadlines",
	"top-goals":       "goals",
	"quick-links":     "hub",
	"scripture":       "scripture",
	"today-focus":     "morning",
	"ventures":        "ventures",
	"prayer":          "prayer",
}

func (s *Server) handleGetDashboard(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.readDashboard()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Filter widgets whose owning module has been disabled. We don't
	// remove the widget from the user's saved config — they may
	// re-enable the module later and expect the widget to come back
	// with their previous Enabled flag. Just flip the response copy's
	// Enabled to false so the client renders nothing for it.
	reg := s.modulesRegistry()
	if reg != nil {
		filtered := make([]dashboardWidget, 0, len(cfg.Widgets))
		for _, w := range cfg.Widgets {
			if mid, ok := widgetTypeToModuleID[w.Type]; ok && !reg.Enabled(mid) {
				w.Enabled = false
			}
			filtered = append(filtered, w)
		}
		cfg.Widgets = filtered
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handlePutDashboard(w http.ResponseWriter, r *http.Request) {
	var cfg dashboardConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if cfg.Version == 0 {
		cfg.Version = dashboardVersion
	}
	if cfg.Widgets == nil {
		cfg.Widgets = []dashboardWidget{}
	}
	if err := s.writeDashboard(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) readDashboard() (dashboardConfig, error) {
	path := filepath.Join(s.cfg.Vault.Root, dashboardFileRel)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultDashboard(), nil
		}
		return dashboardConfig{}, err
	}
	var cfg dashboardConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return dashboardConfig{}, err
	}
	if cfg.Version == 0 {
		cfg.Version = dashboardVersion
	}
	have := map[string]bool{}
	for _, w := range cfg.Widgets {
		have[w.ID] = true
	}
	for _, dw := range defaultDashboard().Widgets {
		if !have[dw.ID] {
			dw.Enabled = false
			cfg.Widgets = append(cfg.Widgets, dw)
		}
	}
	return cfg, nil
}

func (s *Server) writeDashboard(cfg dashboardConfig) error {
	dir := filepath.Join(s.cfg.Vault.Root, ".granit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(s.cfg.Vault.Root, dashboardFileRel)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ----- Layout presets -----
//
// Presets let the user save the current dashboard arrangement under a
// name and switch between named layouts (e.g. focus / morning /
// shutdown). The active layout's widgets are mirrored into the
// top-level Widgets field so a client that doesn't know about layouts
// keeps working — they just see one Widgets list and don't care.
//
// Routes (registered in server.go):
//   GET    /api/v1/dashboard/layouts            → list saved layouts
//   POST   /api/v1/dashboard/layouts            → {name} save current Widgets as preset
//   DELETE /api/v1/dashboard/layouts/{name}     → drop preset
//   POST   /api/v1/dashboard/layouts/{name}/activate → switch active

// findLayout returns the index of the named layout (case-insensitive)
// or -1 when not found. Case-insensitive because preset names are
// user-typed and "Focus" / "focus" should be treated as the same
// thing for switch / replace semantics.
func findLayout(layouts []dashboardLayout, name string) int {
	for i, l := range layouts {
		if strings.EqualFold(l.Name, name) {
			return i
		}
	}
	return -1
}

// cloneWidgets returns a deep-enough copy that mutating the result
// won't reach back into the source slice. Widget.Config is a map of
// arbitrary JSON, so we shallow-copy each entry — the values are
// expected to be JSON primitives by contract, no nested pointers.
func cloneWidgets(in []dashboardWidget) []dashboardWidget {
	out := make([]dashboardWidget, len(in))
	for i, w := range in {
		nw := w
		if w.Config != nil {
			nw.Config = make(map[string]interface{}, len(w.Config))
			for k, v := range w.Config {
				nw.Config[k] = v
			}
		}
		out[i] = nw
	}
	return out
}

func (s *Server) handleListDashboardLayouts(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.readDashboard()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cfg.Layouts == nil {
		cfg.Layouts = []dashboardLayout{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"layouts": cfg.Layouts,
		"active":  cfg.Active,
	})
}

// handleSaveDashboardLayout snapshots the current Widgets under a name.
// If a preset with that name already exists it's overwritten — that's
// the natural "update preset" UX; the user can rename via delete +
// re-save if they want a fresh copy. The active pointer also flips
// to the saved name so the user lands on the layout they just saved.
func (s *Server) handleSaveDashboardLayout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	cfg, err := s.readDashboard()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	idx := findLayout(cfg.Layouts, name)
	layout := dashboardLayout{Name: name, Widgets: cloneWidgets(cfg.Widgets)}
	if idx == -1 {
		cfg.Layouts = append(cfg.Layouts, layout)
	} else {
		// Preserve original casing of the existing name to avoid a
		// rename masquerading as an update — if the user wants to
		// rename, they delete + recreate.
		layout.Name = cfg.Layouts[idx].Name
		cfg.Layouts[idx] = layout
	}
	cfg.Active = layout.Name
	if err := s.writeDashboard(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handleDeleteDashboardLayout(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	cfg, err := s.readDashboard()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	idx := findLayout(cfg.Layouts, name)
	if idx == -1 {
		writeError(w, http.StatusNotFound, "layout not found")
		return
	}
	cfg.Layouts = append(cfg.Layouts[:idx], cfg.Layouts[idx+1:]...)
	// If we just deleted the active preset, drop the pointer — the
	// current top-level Widgets stays intact (an unnamed arrangement)
	// so deleting an active preset doesn't blow up the dashboard.
	if strings.EqualFold(cfg.Active, name) {
		cfg.Active = ""
	}
	if err := s.writeDashboard(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// handleActivateDashboardLayout switches the active layout — copies
// the named preset's widgets into the top-level Widgets and updates
// Active. Idempotent: activating the already-active preset is a no-op.
func (s *Server) handleActivateDashboardLayout(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	cfg, err := s.readDashboard()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	idx := findLayout(cfg.Layouts, name)
	if idx == -1 {
		writeError(w, http.StatusNotFound, "layout not found")
		return
	}
	cfg.Widgets = cloneWidgets(cfg.Layouts[idx].Widgets)
	cfg.Active = cfg.Layouts[idx].Name
	if err := s.writeDashboard(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}
