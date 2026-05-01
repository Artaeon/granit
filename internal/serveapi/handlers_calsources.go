package serveapi

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/artaeon/granit/internal/config"
)

// calSourceView decorates an icsSource with a stable id (the filename, lowercased)
// and the enabled flag derived from the merged config so the UI can render
// a checkbox per row without recomputing it client-side.
type calSourceView struct {
	ID      string `json:"id"`      // stable identifier — the filename
	Source  string `json:"source"`  // display name
	Path    string `json:"path"`    // absolute path on disk
	Folder  string `json:"folder"`  // parent dir, vault-relative ("" for vault root)
	Enabled bool   `json:"enabled"` // true if not in disabled_calendars
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
			ID:      strings.ToLower(src.Source),
			Source:  src.Source,
			Path:    src.Path,
			Folder:  folder,
			Enabled: !isICSDisabled(src, cfg.DisabledCalendars),
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
