package serveapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
	"github.com/artaeon/granit/internal/config"
)

// Habits are derived from `## Habits` sections in daily notes. Each checkbox
// line `- [ ] habit name` / `- [x] habit name` becomes a habit entry for that
// day. The web view aggregates them across the last N days, computes streaks,
// and surfaces a per-habit dot grid.
//
// This means habits stay 100% in markdown (no separate sidecar) — toggling
// one is just toggling the checkbox of the underlying task. Granit's TaskStore
// already manages those tasks, so granit's TUI and the web view stay coherent.

type habitDay struct {
	Date string `json:"date"`
	Done bool   `json:"done"`
}

type habitInfo struct {
	Name          string     `json:"name"`
	Days          []habitDay `json:"days"`
	CurrentStreak int        `json:"currentStreak"`
	LongestStreak int        `json:"longestStreak"`
	Last7Pct      int        `json:"last7Pct"`
	Last30Pct     int        `json:"last30Pct"`
	DoneToday     bool       `json:"doneToday"`
	NotePathToday string     `json:"notePathToday,omitempty"`
	TaskIDToday   string     `json:"taskIdToday,omitempty"`
}

var (
	habitCheckboxRe = regexp.MustCompile(`^\s*[-*+]\s+\[([ xX])\]\s+(.+)$`)
	dailyNameRe     = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\.md$`)
)

// dailyDate extracts a YYYY-MM-DD date from a note's relative path (basename
// is YYYY-MM-DD.md). Returns false for non-daily notes.
func dailyDate(relPath string) (string, bool) {
	base := filepath.Base(relPath)
	m := dailyNameRe.FindStringSubmatch(base)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// parseHabitsSection extracts checkbox-line habits from any heading whose
// text equals "Habits" (case-insensitive, any heading level).
func parseHabitsSection(content string) map[string]bool {
	out := map[string]bool{}
	lines := strings.Split(content, "\n")
	in := false
	for _, line := range lines {
		trim := strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(strings.TrimSpace(trim), "#") {
			text := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(trim), "#"))
			if strings.EqualFold(text, "Habits") {
				in = true
				continue
			}
			in = false
			continue
		}
		if !in {
			continue
		}
		m := habitCheckboxRe.FindStringSubmatch(trim)
		if m == nil {
			continue
		}
		done := m[1] == "x" || m[1] == "X"
		name := strings.TrimSpace(m[2])
		// strip inline metadata granit/we may have added (`due:`, `!N`, `#tag`)
		name = stripTaskMeta(name)
		if name == "" {
			continue
		}
		// Last write wins per day (in case of duplicates).
		out[name] = done
	}
	return out
}

func stripTaskMeta(s string) string {
	for _, re := range []*regexp.Regexp{rePriorityMarker, reDueMarker, taskInlineTagRe, taskTimeMarkerRe, taskDateEmojiRe, taskEmojiPrioRe} {
		s = re.ReplaceAllString(s, " ")
	}
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

var (
	taskInlineTagRe  = regexp.MustCompile(`(^|\s)#[\p{L}\p{N}_/-]+(\s|$)`)
	// ⏰ HH:MM (optional :SS, optional -HH:MM[:SS]) — granit time blocks
	taskTimeMarkerRe = regexp.MustCompile(`\x{23F0}\s*\d{1,2}:\d{2}(?::\d{2})?(?:\s*-\s*\d{1,2}:\d{2}(?::\d{2})?)?`)
	// 📅 YYYY-MM-DD — emoji due-date marker
	taskDateEmojiRe = regexp.MustCompile(`\x{1F4C5}\s*\d{4}-\d{2}-\d{2}`)
	// 🔺⏫🔼🔽⏬ — granit emoji priorities
	taskEmojiPrioRe = regexp.MustCompile(`[\x{1F53A}\x{23EB}\x{1F53C}\x{1F53D}\x{23EC}]`)
)

func (s *Server) handleListHabits(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	today := now.Format("2006-01-02")
	windowStart := now.AddDate(0, 0, -89) // 90 days
	windowStartStr := windowStart.Format("2006-01-02")

	// dateString → habitName → done
	per := map[string]map[string]bool{}

	for _, n := range s.cfg.Vault.SnapshotNotes() {
		date, ok := dailyDate(n.RelPath)
		if !ok {
			continue
		}
		if date < windowStartStr || date > today {
			continue
		}
		// Ensure content loaded
		if !s.cfg.Vault.EnsureLoaded(n.RelPath) {
			continue
		}
		hs := parseHabitsSection(n.Content)
		if len(hs) == 0 {
			continue
		}
		per[date] = hs
	}

	// Collect habit names: anything seen at least once in the window.
	names := map[string]bool{}
	for _, m := range per {
		for k := range m {
			names[k] = true
		}
	}

	// Today's daily note (for toggle target)
	cfg := s.dailyConfigFor()
	todayPath := today + ".md"
	if cfg.Folder != "" {
		todayPath = filepath.ToSlash(filepath.Join(cfg.Folder, todayPath))
	}

	// Map today's habit lines → task IDs (so the frontend can PATCH /tasks/{id})
	todayTaskID := map[string]string{}
	for _, t := range s.cfg.TaskStore.All() {
		if t.NotePath != todayPath {
			continue
		}
		clean := stripTaskMeta(t.Text)
		if names[clean] {
			todayTaskID[clean] = t.ID
		}
	}

	out := make([]habitInfo, 0, len(names))
	for name := range names {
		info := habitInfo{Name: name}
		// Build day list back to windowStart, oldest → newest
		for d := windowStart; !d.After(now); d = d.AddDate(0, 0, 1) {
			ds := d.Format("2006-01-02")
			done := false
			if hs, ok := per[ds]; ok {
				if v, present := hs[name]; present {
					done = v
				}
			}
			info.Days = append(info.Days, habitDay{Date: ds, Done: done})
		}
		// Streaks
		info.CurrentStreak, info.LongestStreak = computeStreaks(info.Days)
		// Last-7 / Last-30 percentages
		info.Last7Pct = pctDone(info.Days, 7)
		info.Last30Pct = pctDone(info.Days, 30)
		// Today's state + linkable target
		if hs, ok := per[today]; ok {
			info.DoneToday = hs[name]
		}
		info.NotePathToday = todayPath
		if id := todayTaskID[name]; id != "" {
			info.TaskIDToday = id
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool {
		// Sort: undone-today first, then by current streak desc, then alpha.
		if out[i].DoneToday != out[j].DoneToday {
			return !out[i].DoneToday
		}
		if out[i].CurrentStreak != out[j].CurrentStreak {
			return out[i].CurrentStreak > out[j].CurrentStreak
		}
		return out[i].Name < out[j].Name
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"habits": out,
		"total":  len(out),
		"today":  today,
		"days":   90,
	})
}

func computeStreaks(days []habitDay) (current, longest int) {
	// Current streak: longest run of consecutive Done at the END of the slice
	// (allowing today to be undone — count up from yesterday backward).
	if len(days) == 0 {
		return 0, 0
	}
	// If today is done, count from today; else from before today.
	end := len(days) - 1
	if !days[end].Done {
		end--
	}
	for i := end; i >= 0 && days[i].Done; i-- {
		current++
	}
	// Longest streak across the window
	run := 0
	for _, d := range days {
		if d.Done {
			run++
			if run > longest {
				longest = run
			}
		} else {
			run = 0
		}
	}
	return current, longest
}

func pctDone(days []habitDay, n int) int {
	if len(days) == 0 || n <= 0 {
		return 0
	}
	if n > len(days) {
		n = len(days)
	}
	tail := days[len(days)-n:]
	done := 0
	for _, d := range tail {
		if d.Done {
			done++
		}
	}
	return int(float64(done) / float64(n) * 100)
}

// ---- retro-toggle ----

type habitToggleBody struct {
	Name string `json:"name"`         // habit name (matches the task text after "- [ ] ")
	Date string `json:"date"`         // YYYY-MM-DD; "" or absent = today
	Done bool   `json:"done"`         // target state
}

// handleToggleHabit lets the user mark a habit done/undone for ANY day,
// not just today. Edits (or creates) the daily note for the given date,
// finds an existing line under ## Habits matching the name and toggles
// its checkbox, or appends a new line if none exists.
//
// The previous toggle path on the web could only patch the current
// day's task by ID, leaving the heatmap dots for past days read-only.
// This endpoint backs the click-to-toggle interaction on the heatmap.
//
// Trade-off: we don't lean on EnsureDaily here because that always
// targets today. For past dates we just write the file directly with
// a minimal frontmatter; for future dates the same flow happens to
// work and matches what users expect (mark a habit done in advance,
// e.g. logging a planned workout).
func (s *Server) handleToggleHabit(w http.ResponseWriter, r *http.Request) {
	var body habitToggleBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	date := strings.TrimSpace(body.Date)
	if date == "" {
		date = time.Now().Format("2006-01-02")
	} else if _, err := time.Parse("2006-01-02", date); err != nil {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}

	// Compute the daily-note path the same way the rest of the server does.
	cfg := config.LoadForVault(s.cfg.Vault.Root)
	folder := strings.TrimSpace(cfg.DailyNotesFolder)
	rel := date + ".md"
	if folder != "" {
		rel = filepath.ToSlash(filepath.Join(folder, rel))
	}
	abs := filepath.Join(s.cfg.Vault.Root, rel)

	// Read existing content (may be missing — we'll seed below).
	var content string
	if data, err := os.ReadFile(abs); err == nil {
		content = string(data)
	}

	box := "[ ]"
	if body.Done {
		box = "[x]"
	}
	target := strings.ToLower(name)

	// Try to find an existing checkbox line under ## Habits whose text
	// matches (case-insensitive). If found, flip its state. If not,
	// append a new line under (or alongside) the section.
	lines := strings.Split(content, "\n")
	inHabits := false
	habitsLine := -1
	updated := false
	for i, ln := range lines {
		trim := strings.TrimSpace(ln)
		// Match ANY heading level for "Habits" — same shape as the
		// reader's parseHabitsSection. The previous "## " prefix-only
		// check meant a daily with `### Habits` was invisible to the
		// toggle; it would create a brand-new `## Habits` section
		// alongside, orphaning the existing data into a phantom
		// duplicate. Critical for users whose older dailies use a
		// different heading level.
		if strings.HasPrefix(trim, "#") {
			text := strings.TrimSpace(strings.TrimLeft(trim, "#"))
			inHabits = strings.EqualFold(text, "Habits")
			if inHabits {
				habitsLine = i
			}
			continue
		}
		if !inHabits {
			continue
		}
		m := habitCheckboxRe.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		// m[2] is the task text — strip granit markers (priority/due/
		// time emojis, !N, due:, recurrence tags, # tags) before
		// comparing. Imperfect but matches user intuition: "Daily
		// Workout !1 #habit" still matches "daily workout".
		text := stripHabitMarkers(m[2])
		if strings.EqualFold(strings.TrimSpace(text), strings.TrimSpace(name)) || strings.Contains(strings.ToLower(text), target) {
			lines[i] = strings.Replace(ln, "["+m[1]+"]", box, 1)
			updated = true
			break
		}
	}

	if !updated {
		newLine := "- " + box + " " + name
		if habitsLine >= 0 {
			// Insert right after the heading.
			before := lines[:habitsLine+1]
			after := lines[habitsLine+1:]
			lines = append(append(before, newLine), after...)
		} else {
			// No ## Habits section — append one. Keep the existing
			// content intact + add a trailing section.
			if content != "" && !strings.HasSuffix(content, "\n") {
				lines = append(lines, "")
			}
			if content == "" {
				// Brand new file — give it the same minimal frontmatter
				// our daily template uses so other consumers recognize it.
				lines = []string{
					"---",
					"date: " + date,
					"type: daily",
					"---",
					"",
					"## Habits",
					newLine,
					"",
				}
			} else {
				lines = append(lines, "", "## Habits", newLine, "")
			}
		}
	}

	out := strings.Join(lines, "\n")
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("mkdir: %v", err))
		return
	}
	if err := atomicio.WriteNote(abs, out); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("write: %v", err))
		return
	}
	// Force a fresh scan + task-store reload so subsequent GETs see
	// the updated state without waiting for the watcher debounce.
	s.rescanMu.Lock()
	_ = s.cfg.Vault.ScanFast()
	_ = s.cfg.TaskStore.Reload()
	s.rescanMu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"name": name,
		"date": date,
		"done": body.Done,
		"path": rel,
	})
}

// stripHabitMarkers removes granit's task-line markers so a habit line
// like "Daily workout !1 due:2026-05-03 #habit ⏰ 06:00" still matches
// the plain "Daily workout" name. Identical regex set used by the
// habit aggregator above.
func stripHabitMarkers(text string) string {
	t := text
	t = taskTimeMarkerRe.ReplaceAllString(t, "")
	t = taskDateEmojiRe.ReplaceAllString(t, "")
	t = taskEmojiPrioRe.ReplaceAllString(t, "")
	t = regexp.MustCompile(`(?:^|\s)!([1-3])(?:\s|$)`).ReplaceAllString(t, " ")
	t = regexp.MustCompile(`due:\d{4}-\d{2}-\d{2}`).ReplaceAllString(t, "")
	t = regexp.MustCompile(`#[A-Za-z0-9_/-]+`).ReplaceAllString(t, "")
	return strings.TrimSpace(t)
}
