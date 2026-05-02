package serveapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/config"
)

// dailyContext bundles two pieces of "what's loose today?" data the
// daily note's top-of-page band shows the user without leaving the
// editor:
//
//   - Carryover: yesterday's open tasks (status=open, due ≤ yesterday).
//     The user can mark them done from the daily note instead of
//     navigating to /tasks first.
//   - Habits: the user's daily-recurring habit list from config.json,
//     each annotated with whether today's daily note already has a
//     "[x]" checkbox for that habit text. Cheap to compute (we already
//     have the daily loaded in the vault index).
//
// The web fetches /api/v1/daily/context once on note-load and once on
// every WS note.changed event for today — keeps the band live without
// re-rendering the whole editor.
type carryoverItem struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Priority int    `json:"priority,omitempty"`
	DueDate  string `json:"dueDate,omitempty"`
	NotePath string `json:"notePath"`
}

type habitItem struct {
	Text string `json:"text"` // raw config string, e.g. "Workout"
	Done bool   `json:"done"` // whether today's daily already has it checked
}

type dailyContextResponse struct {
	Date      string          `json:"date"`      // today's date, YYYY-MM-DD
	Carryover []carryoverItem `json:"carryover"` // yesterday's open tasks
	Habits    []habitItem     `json:"habits"`    // from config.DailyRecurringTasks
}

// handleDailyContext returns today's carryover + habits. Read-only; the
// daily note's quick-add bar is the write path. Cheap on the server —
// pulls from the live task store + config + vault index.
func (s *Server) handleDailyContext(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")

	all := s.cfg.TaskStore.All()

	// Carryover: tasks that are still open and were either due before
	// today, or scheduled for yesterday. We deliberately don't include
	// "tasks created yesterday in yesterday's note" — that ends up
	// pulling unrelated jots forward and creates noise. The user's
	// signal for "this should still happen" is having a due date
	// before today.
	carry := make([]carryoverItem, 0, 8)
	for _, t := range all {
		if t.Done {
			continue
		}
		if t.DueDate == "" || t.DueDate >= today {
			continue
		}
		// Skip tasks that already live in today's daily — they're
		// already in front of the user; surfacing them again as
		// "carryover" is double-counting.
		if strings.Contains(t.NotePath, today) {
			continue
		}
		carry = append(carry, carryoverItem{
			ID:       t.ID,
			Text:     t.Text,
			Priority: t.Priority,
			DueDate:  t.DueDate,
			NotePath: t.NotePath,
		})
		if len(carry) >= 20 {
			break
		}
	}
	_ = yesterday // reserved for future "items scheduled yesterday" rule

	// Habits: from the global config's DailyRecurringTasks list. We
	// flag each as done if today's daily note already contains a
	// matching `[x]` line (case-sensitive substring match — same
	// algorithm the TUI uses to render habit ticks).
	cfg := config.LoadForVault(s.cfg.Vault.Root)
	habits := make([]habitItem, 0, len(cfg.DailyRecurringTasks))

	dailyCfg := s.dailyConfigFor()
	dailyPath := dailyCfg.Folder + "/" + today + ".md"
	if dailyCfg.Folder == "" {
		dailyPath = today + ".md"
	}
	body := ""
	if n := s.cfg.Vault.GetNote(dailyPath); n != nil {
		s.cfg.Vault.EnsureLoaded(dailyPath)
		if n2 := s.cfg.Vault.GetNote(dailyPath); n2 != nil {
			body = n2.Content
		}
	}

	for _, h := range cfg.DailyRecurringTasks {
		text := strings.TrimSpace(h)
		if text == "" {
			continue
		}
		habits = append(habits, habitItem{Text: text, Done: hasCheckedHabit(body, text)})
	}

	writeJSON(w, http.StatusOK, dailyContextResponse{
		Date:      today,
		Carryover: carry,
		Habits:    habits,
	})
}

// hasCheckedHabit returns true if the daily-note body has a "[x]"
// markdown checkbox followed by the habit text. Forgiving on
// whitespace and bullet style so a hand-edited line still counts.
func hasCheckedHabit(body, habit string) bool {
	if body == "" || habit == "" {
		return false
	}
	low := strings.ToLower(habit)
	for _, line := range strings.Split(body, "\n") {
		l := strings.TrimSpace(line)
		// Match "- [x] foo", "* [x] foo", "+ [x] foo", "- [X] foo".
		if (!strings.HasPrefix(l, "- [x]") && !strings.HasPrefix(l, "- [X]") &&
			!strings.HasPrefix(l, "* [x]") && !strings.HasPrefix(l, "* [X]") &&
			!strings.HasPrefix(l, "+ [x]") && !strings.HasPrefix(l, "+ [X]")) {
			continue
		}
		// Strip the "- [x] " prefix and compare case-insensitively.
		rest := strings.TrimSpace(l[5:])
		if strings.Contains(strings.ToLower(rest), low) {
			return true
		}
	}
	return false
}
