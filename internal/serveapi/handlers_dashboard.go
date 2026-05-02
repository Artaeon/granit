package serveapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
)

const dashboardFileRel = ".granit/everything-dashboard.json"
const dashboardVersion = 1

type dashboardWidget struct {
	ID      string                 `json:"id"`
	Type    string                 `json:"type"`
	Enabled bool                   `json:"enabled"`
	Config  map[string]interface{} `json:"config,omitempty"`
}

type dashboardConfig struct {
	Version int               `json:"version"`
	Widgets []dashboardWidget `json:"widgets"`
}

func defaultDashboard() dashboardConfig {
	return dashboardConfig{
		Version: dashboardVersion,
		Widgets: []dashboardWidget{
			{ID: "w-greeting", Type: "greeting", Enabled: true},
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

func (s *Server) handleGetDashboard(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.readDashboard()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
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
