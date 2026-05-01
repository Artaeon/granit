package serveapi

import (
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
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
