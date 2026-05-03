package serveapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/artaeon/granit/internal/config"
	"github.com/artaeon/granit/internal/icswriter"
	"github.com/artaeon/granit/internal/wshub"
)

// calendarNameOK validates the user-supplied filename for a new local
// calendar. Restrictive on purpose — we're going to put this directly
// in the path under <vault>/calendars/<name>.ics and we don't want
// path traversal, hidden files, or platform-illegal characters.
func calendarNameOK(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	if name == "." || name == ".." {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_' || r == ' ':
		default:
			return false
		}
	}
	return true
}

// calSourceView decorates an icsSource with a stable id (the filename, lowercased)
// and the enabled flag derived from the merged config so the UI can render
// a checkbox per row without recomputing it client-side.
type calSourceView struct {
	ID       string `json:"id"`       // stable identifier — the filename
	Source   string `json:"source"`   // display name
	Path     string `json:"path"`     // absolute path on disk
	Folder   string `json:"folder"`   // parent dir, vault-relative ("" for vault root)
	Enabled  bool   `json:"enabled"`  // true if not in disabled_calendars
	Writable bool   `json:"writable"` // true if granit can edit (under vault/calendars/)
}

// handleListCalendarSources lists every .ics file the API can see, with
// the enabled flag matching how the TUI would render it. The web UI uses
// this to draw per-source toggles in the calendar sidebar.
func (s *Server) handleListCalendarSources(w http.ResponseWriter, r *http.Request) {
	cfg := config.LoadForVault(s.cfg.Vault.Root)
	sources := icsListSources(s.cfg.Vault.Root)
	out := make([]calSourceView, len(sources))
	for i, src := range sources {
		rel, _ := filepath.Rel(s.cfg.Vault.Root, src.Path)
		folder := filepath.Dir(rel)
		if folder == "." {
			folder = ""
		}
		out[i] = calSourceView{
			ID:       strings.ToLower(src.Source),
			Source:   src.Source,
			Path:     src.Path,
			Folder:   folder,
			Enabled:  !isICSDisabled(src, cfg.DisabledCalendars),
			Writable: src.Writable,
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sources":  out,
		"disabled": cfg.DisabledCalendars,
		"total":    len(out),
	})
}

// handlePatchCalendarSources accepts {"disabled": ["a", "b"]} and writes
// it back to the vault config so the change persists across both web and
// TUI sessions. We replace the list wholesale — the client always sends
// the canonical set rather than adds/removes.
func (s *Server) handlePatchCalendarSources(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Disabled []string `json:"disabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	cfg := config.LoadForVault(s.cfg.Vault.Root)
	cfg.DisabledCalendars = body.Disabled
	if err := cfg.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Echo back the now-current view so the client doesn't need a follow-up GET.
	s.handleListCalendarSources(w, r)
}

// handleCreateCalendar creates an empty writable .ics under
// <vault>/calendars/<name>.ics. The file is born with a stub VCALENDAR
// (PRODID + NAME + X-WR-CALNAME) so it's valid 5545 from the moment of
// creation — events can be added immediately via the events endpoints.
//
// 409 on collision so a typo doesn't silently overwrite an existing
// calendar (the user re-runs with a different name).
func (s *Server) handleCreateCalendar(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	name := strings.TrimSpace(body.Name)
	if !calendarNameOK(name) {
		writeError(w, http.StatusBadRequest, "invalid name (a-z, 0-9, -, _, space; max 64 chars)")
		return
	}
	dir := filepath.Join(s.cfg.Vault.Root, "calendars")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	path := filepath.Join(dir, name+".ics")
	if _, err := os.Stat(path); err == nil {
		writeError(w, http.StatusConflict, "calendar already exists")
		return
	} else if !errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	display := strings.TrimSpace(body.DisplayName)
	if display == "" {
		display = name
	}
	meta := icswriter.CalendarMeta{
		ProdID:      "-//granit//calendar 1.0//EN",
		Name:        name,
		DisplayName: display,
	}
	if err := icswriter.WriteFile(path, meta, nil); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: filepath.ToSlash(filepath.Join("calendars", name+".ics"))})
	writeJSON(w, http.StatusCreated, calSourceView{
		ID:       strings.ToLower(name + ".ics"),
		Source:   name + ".ics",
		Path:     path,
		Folder:   "calendars",
		Enabled:  true,
		Writable: true,
	})
}
