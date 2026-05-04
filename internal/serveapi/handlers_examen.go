package serveapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
	"github.com/artaeon/granit/internal/daily"
)

// Examen — evening companion to the morning routine. Two-question
// Ignatian rhythm: where did I see God today, where did I miss?
// Optional gratitude + tomorrow's-prep fields round it out. Saves
// as a `## Examen` block in today's daily note (or whatever date
// the user explicitly targets), upserting in place so re-running
// the wizard later in the same evening replaces rather than appends.
//
// We deliberately store inside the daily note rather than as a sidecar
// JSON because:
//   - the user's own daily-note search ("what was I praying about
//     last Thursday?") finds it without an extra index;
//   - the section travels with their vault repo so a clone on
//     another machine has the history without re-syncing JSON;
//   - the markdown is the natural home for narrative text — JSON
//     adds structure the user has to escape into when they really
//     just want to write a paragraph.

// ExamenSaveBody is the wizard payload. Date is YYYY-MM-DD (defaults
// to today when omitted). All four prose fields are optional —
// missing fields are omitted from the rendered section so an
// abbreviated examen ("just gratitude tonight") doesn't leave empty
// headers behind.
type ExamenSaveBody struct {
	Date      string `json:"date"`
	SawGod    string `json:"saw_god"`
	Missed    string `json:"missed"`
	Gratitude string `json:"gratitude"`
	Tomorrow  string `json:"tomorrow"`
}

// handleSaveExamen composes the `## Examen` section and writes it to
// the daily note for the given Date (or today when blank). Returns
// the relative path of the daily note so the client can offer an
// "open today's daily note" link from the success state.
func (s *Server) handleSaveExamen(w http.ResponseWriter, r *http.Request) {
	var b ExamenSaveBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	// Resolve the target daily note. EnsureDaily creates today's note
	// if missing; for past dates we resolve by date and fall back to
	// the standard daily path so the file exists before we write.
	cfg := s.dailyConfigFor()
	dailyPath, when, err := resolveExamenDaily(s.cfg.Vault.Root, cfg, b.Date)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("daily: %v", err))
		return
	}

	rawBytes, err := os.ReadFile(dailyPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	section := buildExamen(b, when)
	updated := upsertNamedSection(string(rawBytes), "## Examen", section)
	if err := atomicio.WriteNote(dailyPath, updated); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Refresh in-memory state so subsequent reads see the new content.
	s.rescanMu.Lock()
	_ = s.cfg.Vault.ScanFast()
	_ = s.cfg.TaskStore.Reload()
	s.rescanMu.Unlock()

	rel, err := filepath.Rel(s.cfg.Vault.Root, dailyPath)
	if err != nil {
		rel = dailyPath
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"path":  filepath.ToSlash(rel),
		"saved": true,
	})
}

// resolveExamenDaily picks the daily-note path + the time used in the
// section header. Today defaults to EnsureDaily (which creates the
// file if it doesn't exist); a past date resolves to the conventional
// path under daily.Folder() and we accept that the file may already
// exist (the typical case: the user is examining today and the
// morning routine already created the file).
func resolveExamenDaily(vaultRoot string, cfg daily.DailyConfig, dateISO string) (string, time.Time, error) {
	now := time.Now()
	if strings.TrimSpace(dateISO) == "" {
		path, _, err := daily.EnsureDaily(vaultRoot, cfg)
		return path, now, err
	}
	t, err := time.Parse("2006-01-02", dateISO)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("date must be YYYY-MM-DD: %w", err)
	}
	// Build the conventional daily path. We don't call EnsureDaily for
	// arbitrary historical dates because creating empty files for a
	// random past date is surprising — for the common case (today's
	// examen) the file already exists from the morning routine.
	folder := strings.TrimRight(cfg.Folder, "/")
	if folder == "" {
		folder = "Daily"
	}
	rel := folder + "/" + t.Format("2006-01-02") + ".md"
	abs := filepath.Join(vaultRoot, rel)
	if _, statErr := os.Stat(abs); statErr != nil {
		// File doesn't exist — create an empty stub so the upsert
		// has something to write into. Better UX than failing the
		// save with "daily not found".
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return "", time.Time{}, err
		}
		if err := os.WriteFile(abs, []byte{}, 0o644); err != nil {
			return "", time.Time{}, err
		}
	}
	return abs, t, nil
}

// buildExamen formats the section. Empty fields are skipped so a
// partial examen ("just thanksgiving tonight") doesn't leave hollow
// "### Where I saw God\n\n" blocks behind.
func buildExamen(b ExamenSaveBody, when time.Time) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Examen — %s\n\n", when.Format("Monday, January 2, 2006")))

	type field struct {
		header string
		body   string
	}
	fields := []field{
		{"### Where I saw God", strings.TrimSpace(b.SawGod)},
		{"### Where I missed", strings.TrimSpace(b.Missed)},
		{"### Gratitude", strings.TrimSpace(b.Gratitude)},
		{"### For tomorrow", strings.TrimSpace(b.Tomorrow)},
	}
	any := false
	for _, f := range fields {
		if f.body == "" {
			continue
		}
		any = true
		sb.WriteString(f.header)
		sb.WriteString("\n\n")
		sb.WriteString(f.body)
		sb.WriteString("\n\n")
	}
	if !any {
		// Pure-empty payload — write a minimal stub so the section
		// exists with the timestamp; the user can edit the file
		// directly. Better than silently writing nothing.
		sb.WriteString("_(no entries this evening)_\n\n")
	}
	return sb.String()
}
