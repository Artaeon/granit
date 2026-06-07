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
	"github.com/artaeon/granit/internal/habits"
	"github.com/artaeon/granit/internal/wshub"
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
	// StackAfter is the name of the habit this one is anchored to —
	// "after I do <StackAfter>, I do this habit." Behavioural-
	// science staple for building a new habit on top of an existing
	// completed action. Empty when no anchor is configured. Read
	// from the .granit/habits-stacks.json sidecar via the habits
	// package — same source the TUI uses, so cross-surface edits
	// stay in sync.
	StackAfter string `json:"stackAfter,omitempty"`
	// Sidecar-stored metadata. All fields are read from the same
	// .granit/habits-*.json files the TUI writes — the package owns
	// load/save, this handler just surfaces them so the web UI can
	// render and edit them without going through markdown.
	Category     string   `json:"category,omitempty"`
	ReminderTime string   `json:"reminderTime,omitempty"` // HH:MM 24h
	Frequency    string   `json:"frequency,omitempty"`    // "daily" | "weekdays" | "weekends" | "3x-week" | "mon,wed,fri"
	Archived     bool     `json:"archived,omitempty"`
	Tags         []string `json:"tags,omitempty"`
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
	// Plain-text markers used by stripHabitMarkers below. Promoted out
	// of the function body so we don't pay regexp.MustCompile per
	// habit aggregation call (which happens once per task line, across
	// every daily note, on every /habits request).
	taskBangPrioRe = regexp.MustCompile(`(?:^|\s)!([1-3])(?:\s|$)`)
	taskDueTextRe  = regexp.MustCompile(`due:\d{4}-\d{2}-\d{2}`)
	taskHashTagRe  = regexp.MustCompile(`#[A-Za-z0-9_/-]+`)
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
		// mtime-keyed cache (habits_cache.go) skips both EnsureLoaded
		// and parseHabitsSection when the daily note hasn't changed
		// since the previous /habits request.
		hs, found := s.parseHabitsSectionCached(n)
		if !found {
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

	// Sidecar data — the same source the TUI uses. Reading the whole
	// habits.Data once here keeps every `## Habits` surface (web list
	// + TUI heatmap + future widgets) agreeing on what's anchored,
	// categorised, scheduled, etc. without N round-trips to disk.
	hdata := habits.Load(s.cfg.Vault.Root)

	out := make([]habitInfo, 0, len(names))
	for name := range names {
		info := habitInfo{Name: name}
		if anchor, ok := hdata.Stacks[name]; ok && anchor != "" {
			info.StackAfter = anchor
		}
		if cat, ok := hdata.Categories[name]; ok && cat != "" {
			info.Category = cat
		}
		if t, ok := hdata.Times[name]; ok && t != "" {
			info.ReminderTime = t
		}
		if f, ok := hdata.Frequencies[name]; ok && f != "" {
			info.Frequency = f
		}
		if hdata.Archived[name] {
			info.Archived = true
		}
		if tags, ok := hdata.Tags[name]; ok && len(tags) > 0 {
			info.Tags = tags
		}
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
	//
	// CRITICAL: Vault.ScanFast() leaves Note.Content empty for notes
	// whose modtime hasn't changed since the last scan, AND on first
	// discovery of a brand-new file it indexes the path but doesn't
	// load body bytes. TaskStore.Reload() then parses an empty body
	// for the daily note we just wrote, so the new habit's checkbox
	// line is invisible to the task index — taskIdToday stays empty
	// and the web's toggle button greys out forever ("can't track new
	// habits"). EnsureLoaded forces a content read on `rel` before the
	// reload so the new task is actually visible. Same pitfall the
	// `Granit ScanFast/EnsureLoaded contract` memory note warns about.
	s.rescanMu.Lock()
	_ = s.cfg.Vault.ScanFast()
	_ = s.cfg.Vault.EnsureLoaded(rel)
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
	t = taskBangPrioRe.ReplaceAllString(t, " ")
	t = taskDueTextRe.ReplaceAllString(t, "")
	t = taskHashTagRe.ReplaceAllString(t, "")
	return strings.TrimSpace(t)
}

// ---- delete + rename ----
//
// Habits have no separate record — the canonical state is the
// checkbox lines under `## Habits` in daily notes. "Deleting" a habit
// means removing those lines across every daily where they appear;
// "renaming" means rewriting the visible text of those lines (keep
// the checkbox state + any markers intact). Both walk every daily
// note in the vault (not the 90-day list window — the user wants the
// habit gone or renamed everywhere, including past dailies).
//
// Matching: case-insensitive exact match on stripHabitMarkers(line)
// vs the supplied name. Not the Contains() fallback the toggle path
// uses — destructive ops must be precise so "run" doesn't also nuke
// "running club".

// rewriteHabitInDailies walks every daily note (basename YYYY-MM-DD.md)
// and applies fn(stripped, line, checkbox) to each ## Habits checkbox
// line whose stripped text matches `name`. fn returns ("", true) to
// drop the line; (newLine, true) to replace it; ("", false) to leave
// it alone. Returns the number of files modified.
func (s *Server) rewriteHabitInDailies(name string, fn func(stripped, line, checkbox string) (replacement string, modify bool)) (int, error) {
	if strings.TrimSpace(name) == "" {
		return 0, fmt.Errorf("name required")
	}
	target := strings.TrimSpace(name)
	touched := 0
	var touchedRels []string
	for _, n := range s.cfg.Vault.SnapshotNotes() {
		if _, ok := dailyDate(n.RelPath); !ok {
			continue
		}
		if !s.cfg.Vault.EnsureLoaded(n.RelPath) {
			continue
		}
		lines := strings.Split(n.Content, "\n")
		inHabits := false
		fileChanged := false
		out := lines[:0:0]
		for _, ln := range lines {
			trim := strings.TrimSpace(ln)
			if strings.HasPrefix(trim, "#") {
				text := strings.TrimSpace(strings.TrimLeft(trim, "#"))
				inHabits = strings.EqualFold(text, "Habits")
				out = append(out, ln)
				continue
			}
			if !inHabits {
				out = append(out, ln)
				continue
			}
			m := habitCheckboxRe.FindStringSubmatch(ln)
			if m == nil {
				out = append(out, ln)
				continue
			}
			stripped := stripHabitMarkers(m[2])
			if !strings.EqualFold(strings.TrimSpace(stripped), target) {
				out = append(out, ln)
				continue
			}
			replacement, modify := fn(stripped, ln, m[1])
			if !modify {
				out = append(out, ln)
				continue
			}
			fileChanged = true
			if replacement == "" {
				// Drop the line entirely (delete habit).
				continue
			}
			out = append(out, replacement)
		}
		if !fileChanged {
			continue
		}
		abs := filepath.Join(s.cfg.Vault.Root, n.RelPath)
		if err := atomicio.WriteNote(abs, strings.Join(out, "\n")); err != nil {
			return touched, fmt.Errorf("write %s: %w", n.RelPath, err)
		}
		touched++
		touchedRels = append(touchedRels, n.RelPath)
	}
	if touched > 0 {
		// Force a content reload on every rewritten file before
		// TaskStore.Reload — ScanFast won't reload body bytes by
		// modtime alone (same pitfall handled in handleToggleHabit).
		s.rescanMu.Lock()
		_ = s.cfg.Vault.ScanFast()
		for _, rel := range touchedRels {
			_ = s.cfg.Vault.EnsureLoaded(rel)
		}
		_ = s.cfg.TaskStore.Reload()
		s.rescanMu.Unlock()
	}
	return touched, nil
}

func (s *Server) handleDeleteHabit(w http.ResponseWriter, r *http.Request) {
	name := urlParam(r, "name")
	touched, err := s.rewriteHabitInDailies(name, func(_, _, _ string) (string, bool) {
		return "", true
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":         name,
		"filesTouched": touched,
	})
}

// habitPatchBody is the PATCH /habits/{name} body. Every field is a
// pointer so we can distinguish "omitted" (don't touch the sidecar)
// from "explicitly cleared" (write the cleared value). new_name keeps
// its historical name — this endpoint started life as rename-only and
// still drives the rename rewrite when supplied.
type habitPatchBody struct {
	NewName      *string   `json:"new_name,omitempty"`
	Category     *string   `json:"category,omitempty"`
	ReminderTime *string   `json:"reminderTime,omitempty"`
	Frequency    *string   `json:"frequency,omitempty"`
	Archived     *bool     `json:"archived,omitempty"`
	Tags         *[]string `json:"tags,omitempty"`
}

var (
	// HH:MM 24h — 00:00 through 23:59. Loose on leading zero in the
	// hour digit so "9:00" passes; the frontend may emit it that way.
	reminderTimeRe = regexp.MustCompile(`^([01]?\d|2[0-3]):[0-5]\d$`)
	// Comma-separated list of three-letter weekday abbreviations
	// (case-insensitive). Used as the fallback frequency pattern when
	// the value isn't one of the named cadences.
	weekdayListRe = regexp.MustCompile(`^(?i)(mon|tue|wed|thu|fri|sat|sun)(\s*,\s*(mon|tue|wed|thu|fri|sat|sun))*$`)
)

// validFrequency accepts the four named cadences plus a comma-separated
// weekday list. Empty string is also valid — it means "clear the entry".
func validFrequency(f string) bool {
	if f == "" {
		return true
	}
	switch f {
	case string(habits.FrequencyDaily), string(habits.FrequencyWeekdays),
		string(habits.FrequencyWeekends), string(habits.Frequency3xWeek):
		return true
	}
	return weekdayListRe.MatchString(f)
}

func (s *Server) handleRenameHabit(w http.ResponseWriter, r *http.Request) {
	name := urlParam(r, "name")
	if strings.TrimSpace(name) == "" {
		writeError(w, http.StatusBadRequest, "habit name required")
		return
	}
	var body habitPatchBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	// ---- validate every supplied field up-front so we never half-apply.
	var reminder string
	if body.ReminderTime != nil {
		reminder = strings.TrimSpace(*body.ReminderTime)
		if reminder != "" && !reminderTimeRe.MatchString(reminder) {
			writeError(w, http.StatusBadRequest, "reminderTime must be HH:MM or empty")
			return
		}
	}
	var freq string
	if body.Frequency != nil {
		freq = strings.TrimSpace(*body.Frequency)
		if !validFrequency(freq) {
			writeError(w, http.StatusBadRequest, "frequency must be one of daily|weekdays|weekends|3x-week or a comma-separated weekday list")
			return
		}
	}
	var cleanTags []string
	if body.Tags != nil {
		// Trim each, drop empties. We don't enforce uniqueness — the
		// UI does, and a duplicate in storage is harmless.
		for _, t := range *body.Tags {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			cleanTags = append(cleanTags, t)
		}
	}

	// ---- rename first; sidecar updates key by the post-rename name.
	effectiveName := name
	newNameOut := ""
	touched := 0
	if body.NewName != nil {
		nn := strings.TrimSpace(*body.NewName)
		if nn == "" {
			writeError(w, http.StatusBadRequest, "new_name must be non-empty when supplied")
			return
		}
		newNameOut = nn
		if !strings.EqualFold(nn, strings.TrimSpace(name)) {
			n, err := s.rewriteHabitInDailies(name, func(_, line, _ string) (string, bool) {
				m := habitCheckboxRe.FindStringSubmatch(line)
				if m == nil {
					return "", false
				}
				idx := strings.Index(line, m[2])
				if idx < 0 {
					return "", false
				}
				oldText := m[2]
				stripped := stripHabitMarkers(oldText)
				tail := oldText
				if pos := strings.Index(strings.ToLower(oldText), strings.ToLower(stripped)); pos >= 0 && stripped != "" {
					tail = oldText[:pos] + nn + oldText[pos+len(stripped):]
				} else {
					tail = nn
				}
				return line[:idx] + tail + line[idx+len(m[2]):], true
			})
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			touched = n
			effectiveName = nn
		}
	}

	// ---- sidecar writes. Load once, mutate, save each that changed.
	// Track which sidecars were touched so we can fire WS events
	// without spamming every habit-* path on every patch.
	d := habits.Load(s.cfg.Vault.Root)

	// If we renamed, move existing sidecar keys to the new name so
	// the per-habit metadata follows the rename. Only sidecars that
	// actually had a value for the old name get marked dirty — no
	// point rewriting an empty JSON file when there was nothing to
	// move.
	dirty := map[string]bool{}
	if effectiveName != name {
		if moveStringKey(d.Categories, name, effectiveName) {
			dirty["categories"] = true
		}
		if moveStringKey(d.Times, name, effectiveName) {
			dirty["times"] = true
		}
		if moveStringKey(d.Frequencies, name, effectiveName) {
			dirty["frequencies"] = true
		}
		if moveStringKey(d.Stacks, name, effectiveName) {
			dirty["stacks"] = true
		}
		if v, ok := d.Archived[name]; ok {
			delete(d.Archived, name)
			d.Archived[effectiveName] = v
			dirty["archived"] = true
		}
		if v, ok := d.Tags[name]; ok {
			delete(d.Tags, name)
			d.Tags[effectiveName] = v
			dirty["tags"] = true
		}
	}

	if body.Category != nil {
		c := strings.TrimSpace(*body.Category)
		if c == "" {
			delete(d.Categories, effectiveName)
		} else {
			d.Categories[effectiveName] = c
		}
		dirty["categories"] = true
	}
	if body.ReminderTime != nil {
		if reminder == "" {
			delete(d.Times, effectiveName)
		} else {
			d.Times[effectiveName] = reminder
		}
		dirty["times"] = true
	}
	if body.Frequency != nil {
		if freq == "" {
			delete(d.Frequencies, effectiveName)
		} else {
			d.Frequencies[effectiveName] = freq
		}
		dirty["frequencies"] = true
	}
	if body.Archived != nil {
		if *body.Archived {
			d.Archived[effectiveName] = true
		} else {
			delete(d.Archived, effectiveName)
		}
		dirty["archived"] = true
	}
	if body.Tags != nil {
		if len(cleanTags) == 0 {
			delete(d.Tags, effectiveName)
		} else {
			d.Tags[effectiveName] = cleanTags
		}
		dirty["tags"] = true
	}
	if dirty["categories"] {
		if err := habits.SaveCategories(s.cfg.Vault.Root, d.Categories); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: ".granit/habits-categories.json"})
	}
	if dirty["times"] {
		if err := habits.SaveTimes(s.cfg.Vault.Root, d.Times); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: ".granit/habits-times.json"})
	}
	if dirty["frequencies"] {
		if err := habits.SaveFrequencies(s.cfg.Vault.Root, d.Frequencies); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: ".granit/habits-frequency.json"})
	}
	if dirty["archived"] {
		if err := habits.SaveArchived(s.cfg.Vault.Root, d.Archived); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: ".granit/habits-archived.json"})
	}
	if dirty["tags"] {
		if err := habits.SaveTags(s.cfg.Vault.Root, d.Tags); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: ".granit/habits-tags.json"})
	}
	if dirty["stacks"] {
		if err := habits.SaveStacks(s.cfg.Vault.Root, d.Stacks); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: ".granit/habits-stacks.json"})
	}

	// Build the response. Keep the historical {name, newName,
	// filesTouched} shape for the rename path so existing callers
	// don't break; add the freshly-effective per-habit metadata
	// alongside so the client can render without a follow-up GET.
	resp := map[string]any{
		"name":         name,
		"newName":      newNameOut,
		"filesTouched": touched,
		"habit": map[string]any{
			"name":         effectiveName,
			"category":     d.Categories[effectiveName],
			"reminderTime": d.Times[effectiveName],
			"frequency":    d.Frequencies[effectiveName],
			"archived":     d.Archived[effectiveName],
			"tags":         d.Tags[effectiveName],
			"stackAfter":   d.Stacks[effectiveName],
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

// moveStringKey moves m[from] to m[to] if present. Returns true when
// a move actually happened — lets callers skip sidecar writes for
// maps that didn't change.
func moveStringKey(m map[string]string, from, to string) bool {
	v, ok := m[from]
	if !ok {
		return false
	}
	delete(m, from)
	m[to] = v
	return true
}

// handleSetHabitStack writes the stack-anchor sidecar entry for one
// habit. Empty / missing `after` clears the anchor. We don't
// validate that `after` refers to an existing habit — referential
// integrity is the user's responsibility (and a stale reference
// renders as "after <unknown>" which is the right signal to
// either rename or clear it). PUT semantics: the value sent is
// the new state, period.
type habitStackBody struct {
	After string `json:"after"`
}

func (s *Server) handleSetHabitStack(w http.ResponseWriter, r *http.Request) {
	name := urlParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "habit name required")
		return
	}
	var body habitStackBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	// Read existing sidecar so we don't clobber other entries —
	// we're patching one key in a map.
	d := habits.Load(s.cfg.Vault.Root)
	after := strings.TrimSpace(body.After)
	if after == "" {
		delete(d.Stacks, name)
	} else {
		// Self-reference would create a degenerate "after me, I do
		// me" loop. Reject so the UI doesn't have to.
		if after == name {
			writeError(w, http.StatusBadRequest, "stack anchor cannot reference the habit itself")
			return
		}
		d.Stacks[name] = after
	}
	if err := habits.SaveStacks(s.cfg.Vault.Root, d.Stacks); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Broadcast so other tabs / surfaces refresh. The path matches
	// the SidecarPaths() entry the package documents.
	s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: ".granit/habits-stacks.json"})
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "after": after})
}
