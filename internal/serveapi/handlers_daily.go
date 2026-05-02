package serveapi

import (
	"errors"
	"net/http"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/artaeon/granit/internal/daily"
)

func parseDailyParam(s string) (time.Time, error) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch s {
	case "today":
		return today, nil
	case "yesterday":
		return today.AddDate(0, 0, -1), nil
	case "tomorrow":
		return today.AddDate(0, 0, 1), nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, errors.New("invalid date")
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location()), nil
}

// dailyConfigFor adapts the global config to a date-overridable form.
// granit's daily.GetDailyPath uses today; for other dates we compute the
// path manually using the configured folder.
func (s *Server) dailyConfigFor() daily.DailyConfig {
	cfg := s.cfg.Daily
	if cfg.Template == "" {
		cfg.Template = daily.DefaultConfig().Template
	}
	return cfg
}

func (s *Server) handleGetDaily(w http.ResponseWriter, r *http.Request) {
	dateParam := chi.URLParam(r, "date")
	date, err := parseDailyParam(dateParam)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	cfg := s.dailyConfigFor()
	folder := cfg.Folder
	filename := date.Format("2006-01-02") + ".md"
	rel := filename
	if folder != "" {
		rel = filepath.ToSlash(filepath.Join(folder, filename))
	}

	// If today, lean on granit's EnsureDaily (which uses today internally).
	now := time.Now()
	created := false
	if date.Year() == now.Year() && date.Month() == now.Month() && date.Day() == now.Day() {
		_, c, err := daily.EnsureDaily(s.cfg.Vault.Root, cfg)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		created = c
	}

	if created {
		s.rescanMu.Lock()
		_ = s.cfg.Vault.ScanFast()
		// EnsureDaily seeded the new daily with the user's recurring
		// tasks; without an inline reload the TaskStore stays
		// unaware until the file watcher catches up. Race window is
		// small but caused the "no tasks" bug on first launch of the
		// day — the dashboard fetches /api/v1/tasks before the
		// watcher has fired. Reload here closes that gap.
		_ = s.cfg.TaskStore.Reload()
		s.rescanMu.Unlock()
		w.Header().Set("X-Created", "true")
	}

	n := s.cfg.Vault.GetNote(rel)
	if n == nil {
		writeError(w, http.StatusNotFound, "daily note not found")
		return
	}
	s.cfg.Vault.EnsureLoaded(rel)
	w.Header().Set("ETag", s.etagFor(n.ModTime, n.Size))
	writeJSON(w, http.StatusOK, noteFull{
		Path:        n.RelPath,
		Title:       n.Title,
		ModTime:     n.ModTime,
		Size:        n.Size,
		Frontmatter: n.Frontmatter,
		Body:        stripFrontmatterBody(n.Content),
		Links:       n.Links,
		Tags:        tagsFor(n),
	})
}
