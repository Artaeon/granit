package serveapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/artaeon/granit/internal/daily"
	"github.com/artaeon/granit/internal/tasks"
)

type taskView struct {
	ID              string     `json:"id"`
	NotePath        string     `json:"notePath"`
	LineNum         int        `json:"lineNum"`
	Text            string     `json:"text"`
	Done            bool       `json:"done"`
	Priority        int        `json:"priority"`
	Tags            []string   `json:"tags,omitempty"`
	DueDate         string     `json:"dueDate,omitempty"`
	ScheduledStart  *time.Time `json:"scheduledStart,omitempty"`
	DurationMinutes int        `json:"durationMinutes,omitempty"`
	ProjectID       string     `json:"projectId,omitempty"`
	CreatedAt       *time.Time `json:"createdAt,omitempty"`
	CompletedAt     *time.Time `json:"completedAt,omitempty"`
	Triage           string     `json:"triage,omitempty"`
	SnoozedUntil     string     `json:"snoozedUntil,omitempty"`
	EstimatedMinutes int        `json:"estimatedMinutes,omitempty"`
	DependsOn        []string   `json:"dependsOn,omitempty"`
	UpdatedAt        *time.Time `json:"updatedAt,omitempty"`
	Indent           int        `json:"indent,omitempty"`
	ParentLine       int        `json:"parentLine,omitempty"`
	Recurrence       string     `json:"recurrence,omitempty"`
	Notes            string     `json:"notes,omitempty"`
	GranitID         string     `json:"granitId,omitempty"`
	GranitOrigin     string     `json:"granitOrigin,omitempty"`
}

// priorityStoreToAPI maps the parser's internal Priority field (where
// 4=Highest, 3=High, 2=Med, 1=Low — matches the emoji order) to the
// web's API convention where 1=Highest, 2=Med, 3=Low. The web's
// TaskCard renders P1 in red (most urgent) so the inversion is what
// users expect. Out-of-range values pass through as 0 so the
// `priority > 0` UI guard hides them.
func priorityStoreToAPI(p int) int {
	switch p {
	case 4:
		return 1
	case 3:
		return 2
	case 2:
		return 3
	}
	return 0
}

func taskToView(t tasks.Task) taskView {
	v := taskView{
		ID:               t.ID,
		GranitID:         t.ID,
		NotePath:         t.NotePath,
		LineNum:          t.LineNum,
		Text:             t.Text,
		Done:             t.Done,
		Priority:         priorityStoreToAPI(t.Priority),
		Tags:             t.Tags,
		DueDate:          t.DueDate,
		ProjectID:        t.ProjectID,
		Triage:           string(t.Triage),
		SnoozedUntil:     t.SnoozedUntil,
		EstimatedMinutes: t.EstimatedMinutes,
		DependsOn:        t.DependsOn,
		Indent:           t.Indent,
		ParentLine:       t.ParentLine,
		Recurrence:       t.Recurrence,
		Notes:            t.Notes,
	}
	if t.Origin != "" {
		v.GranitOrigin = string(t.Origin)
	}
	if t.ScheduledStart != nil {
		v.ScheduledStart = t.ScheduledStart
	}
	if t.Duration > 0 {
		v.DurationMinutes = int(t.Duration / time.Minute)
	}
	if !t.CreatedAt.IsZero() {
		ca := t.CreatedAt
		v.CreatedAt = &ca
	}
	if t.CompletedAt != nil {
		v.CompletedAt = t.CompletedAt
	}
	return v
}

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := q.Get("status")
	tag := q.Get("tag")
	due := q.Get("due_on")
	dueBefore := q.Get("due_before")
	notePath := q.Get("note")
	triage := q.Get("triage")

	all := s.cfg.TaskStore.All()
	out := make([]taskView, 0, len(all))
	for _, t := range all {
		if status == "open" && t.Done {
			continue
		}
		if status == "done" && !t.Done {
			continue
		}
		if tag != "" {
			has := false
			for _, x := range t.Tags {
				if x == tag {
					has = true
					break
				}
			}
			if !has {
				continue
			}
		}
		if due != "" && t.DueDate != due {
			continue
		}
		if dueBefore != "" && (t.DueDate == "" || t.DueDate >= dueBefore) {
			continue
		}
		if notePath != "" && t.NotePath != notePath {
			continue
		}
		if triage != "" && string(t.Triage) != triage {
			continue
		}
		out = append(out, taskToView(t))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Done != out[j].Done {
			return !out[i].Done
		}
		if out[i].DueDate != out[j].DueDate {
			if out[i].DueDate == "" {
				return false
			}
			if out[j].DueDate == "" {
				return true
			}
			return out[i].DueDate < out[j].DueDate
		}
		if out[i].NotePath != out[j].NotePath {
			return out[i].NotePath < out[j].NotePath
		}
		return out[i].LineNum < out[j].LineNum
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{"tasks": out, "total": len(out)})
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, ok := s.cfg.TaskStore.GetByID(id)
	if !ok {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	writeJSON(w, http.StatusOK, taskToView(t))
}

type patchTaskBody struct {
	Done            *bool   `json:"done,omitempty"`
	Priority        *int    `json:"priority,omitempty"`
	DueDate         *string `json:"dueDate,omitempty"`
	Text            *string `json:"text,omitempty"`
	ScheduledStart  *string `json:"scheduledStart,omitempty"`
	DurationMinutes *int    `json:"durationMinutes,omitempty"`
	ProjectID       *string `json:"projectId,omitempty"`
	Triage          *string `json:"triage,omitempty"`
	SnoozedUntil    *string `json:"snoozedUntil,omitempty"` // YYYY-MM-DDThh:mm or "" to clear
	Recurrence      *string `json:"recurrence,omitempty"`   // line marker, e.g. "daily" / ""
	Notes           *string `json:"notes,omitempty"`        // free-form sidecar metadata
	ClearSchedule   bool    `json:"clearSchedule,omitempty"`
}

func (s *Server) handlePatchTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var b patchTaskBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	store := s.cfg.TaskStore
	if _, ok := store.GetByID(id); !ok {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	// Markdown-line mutations (bundled into a single UpdateLine for atomicity)
	if b.Done != nil || b.Priority != nil || b.DueDate != nil || b.Text != nil || b.SnoozedUntil != nil || b.Recurrence != nil {
		err := store.UpdateLine(id, func(line string) string {
			if b.Done != nil {
				line = transformDone(line, *b.Done)
			}
			if b.Priority != nil {
				line = transformPriority(line, *b.Priority)
			}
			if b.DueDate != nil {
				line = transformDue(line, *b.DueDate)
			}
			if b.SnoozedUntil != nil {
				line = transformSnooze(line, *b.SnoozedUntil)
			}
			if b.Recurrence != nil {
				line = transformRecurrence(line, *b.Recurrence)
			}
			if b.Text != nil {
				line = transformText(line, *b.Text)
			}
			return line
		})
		if err != nil {
			if errors.Is(err, tasks.ErrNotFound) {
				writeError(w, http.StatusNotFound, "task not found")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Sidecar mutations
	if b.ClearSchedule {
		if err := store.UpdateMeta(id, func(t *tasks.Task) {
			t.ScheduledStart = nil
			t.Duration = 0
		}); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if b.ScheduledStart != nil {
		st, err := time.Parse(time.RFC3339, *b.ScheduledStart)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid scheduledStart, expected RFC3339")
			return
		}
		dur := time.Duration(0)
		if b.DurationMinutes != nil {
			dur = time.Duration(*b.DurationMinutes) * time.Minute
		} else if existing, ok := store.GetByID(id); ok && existing.Duration > 0 {
			dur = existing.Duration
		}
		if err := store.Schedule(id, st, dur); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else if b.DurationMinutes != nil && !b.ClearSchedule {
		// duration changed without restating start
		if err := store.UpdateMeta(id, func(t *tasks.Task) {
			t.Duration = time.Duration(*b.DurationMinutes) * time.Minute
		}); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if b.ProjectID != nil {
		if err := store.UpdateMeta(id, func(t *tasks.Task) { t.ProjectID = *b.ProjectID }); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if b.Notes != nil {
		// Notes is sidecar-only — not in the markdown line, so it lives
		// purely in the metadata file. UpdateMeta gives us the atomicity
		// the rest of the metadata writes use.
		if err := store.UpdateMeta(id, func(t *tasks.Task) { t.Notes = *b.Notes }); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if b.Triage != nil {
		if err := store.Triage(id, tasks.TriageState(*b.Triage)); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	t, _ := store.GetByID(id)
	writeJSON(w, http.StatusOK, taskToView(t))
}

type createTaskBody struct {
	NotePath        string   `json:"notePath"`
	Text            string   `json:"text"`
	Priority        int      `json:"priority,omitempty"`
	DueDate         string   `json:"dueDate,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	Section         string   `json:"section,omitempty"`
	ScheduledStart  *string  `json:"scheduledStart,omitempty"`
	DurationMinutes int      `json:"durationMinutes,omitempty"`
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var b createTaskBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(b.Text) == "" {
		writeError(w, http.StatusBadRequest, "text required")
		return
	}
	// Empty notePath = "the user wanted today's daily" — every front-end
	// surface that doesn't supply a path (the dashboard quick-capture
	// widget, future API consumers) means "land it under today's daily."
	// Without this default the task would silently land in Tasks.md at
	// the vault root, where the daily-note Tasks page never looks.
	notePath := strings.TrimSpace(b.NotePath)
	section := b.Section
	if notePath == "" {
		dcfg := s.dailyConfigFor()
		_, _, _ = daily.EnsureDaily(s.cfg.Vault.Root, dcfg)
		filename := time.Now().Format("2006-01-02") + ".md"
		notePath = filename
		if dcfg.Folder != "" {
			notePath = filepath.ToSlash(filepath.Join(dcfg.Folder, filename))
		}
		if section == "" {
			section = "## Tasks"
		}
	}
	textWithMarkers := buildTaskTextLine(b.Text, b.Priority, b.DueDate, b.Tags)
	opts := tasks.CreateOpts{
		File:    notePath,
		Origin:  tasks.Origin("manual"),
		Section: section,
	}
	t, err := s.cfg.TaskStore.Create(textWithMarkers, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if b.ScheduledStart != nil {
		if start, err := time.Parse(time.RFC3339, *b.ScheduledStart); err == nil {
			dur := time.Duration(b.DurationMinutes) * time.Minute
			_ = s.cfg.TaskStore.Schedule(t.ID, start, dur)
			t, _ = s.cfg.TaskStore.GetByID(t.ID)
		}
	}
	writeJSON(w, http.StatusCreated, taskToView(t))
}

// handleDeleteTask removes the task line from its source note and
// tombstones the ID so reconciliation doesn't resurrect it. Mirrors
// the TUI's delete behaviour exactly — same TaskStore.Delete, same
// atomic write path. Returns 204 on success, 404 when the id is
// unknown.
func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id required")
		return
	}
	if err := s.cfg.TaskStore.Delete(id); err != nil {
		if errors.Is(err, tasks.ErrNotFound) {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- line transforms ----

var (
	rePriorityMarker = regexp.MustCompile(`(^|\s)![1-3](\s|$)`)
	reDueMarker      = regexp.MustCompile(`(^|\s)due:\d{4}-\d{2}-\d{2}(\s|$)`)
	reSnoozeMarker   = regexp.MustCompile(`(^|\s)snooze:\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(\s|$)`)
	// granit recognizes recurrence as either an emoji form (🔁 daily) or
	// the hashtag form (#daily). We strip both on rewrite and emit the
	// hashtag form — plain ASCII, easier to type, parser reads it back.
	reRecurEmoji  = regexp.MustCompile(`\x{1F501}\s*(daily|weekly|monthly|3x-week)`)
	reRecurTag    = regexp.MustCompile(`(^|\s)#(daily|weekly|monthly|3x-week)(\s|$)`)
	reCheckbox    = regexp.MustCompile(`^(\s*[-*+]\s+\[)([ xX])(\]\s+)`)
)

func transformDone(line string, done bool) string {
	m := reCheckbox.FindStringSubmatchIndex(line)
	if m == nil {
		return line
	}
	ch := byte(' ')
	if done {
		ch = 'x'
	}
	// FindStringSubmatchIndex returns [full_start, full_end, g1_start, g1_end,
	// g2_start, g2_end, ...]. The checkbox character is the SECOND capture
	// group (the [ xX] inside the brackets), so its bounds are at indices
	// m[4]:m[5]. Previous code used m[2]:m[3] which is group 1 (the
	// prefix "  - [") — replacing that with the char produced "x ] task"
	// instead of "- [x] task" and broke every PATCH done:true call.
	return line[:m[4]] + string(ch) + line[m[5]:]
}

func transformPriority(line string, p int) string {
	clean := rePriorityMarker.ReplaceAllString(line, "$1$2")
	if p < 1 || p > 3 {
		return strings.TrimRight(clean, " ")
	}
	return strings.TrimRight(clean, " ") + " !" + string(rune('0'+p))
}

func transformDue(line string, due string) string {
	clean := reDueMarker.ReplaceAllString(line, "$1$2")
	if due == "" {
		return strings.TrimRight(clean, " ")
	}
	return strings.TrimRight(clean, " ") + " due:" + due
}

// transformSnooze writes (or clears) a `snooze:YYYY-MM-DDThh:mm` marker
// on a checkbox line. The marker is what the parser reads back into
// Task.SnoozedUntil, so writing it round-trips cleanly.
func transformSnooze(line string, until string) string {
	clean := reSnoozeMarker.ReplaceAllString(line, "$1$2")
	if until == "" {
		return strings.TrimRight(clean, " ")
	}
	return strings.TrimRight(clean, " ") + " snooze:" + until
}

// transformRecurrence writes the hashtag form (#daily / #weekly / etc.)
// and strips any pre-existing emoji/hashtag form so we don't end up with
// duplicates. Empty `freq` clears the recurrence entirely.
func transformRecurrence(line string, freq string) string {
	clean := reRecurEmoji.ReplaceAllString(line, "")
	clean = reRecurTag.ReplaceAllString(clean, "$1$3")
	clean = strings.TrimRight(clean, " ")
	if freq == "" {
		return clean
	}
	return clean + " #" + freq
}

func transformText(line string, newText string) string {
	m := reCheckbox.FindStringSubmatchIndex(line)
	if m == nil {
		return line
	}
	prefix := line[:m[5]] // includes "[x] "
	// Preserve markers we don't want to clobber: collect them from the old tail.
	tail := line[m[5]:]
	var preserved []string
	for _, re := range []*regexp.Regexp{rePriorityMarker, reDueMarker} {
		for _, mm := range re.FindAllString(tail, -1) {
			preserved = append(preserved, strings.TrimSpace(mm))
		}
	}
	out := prefix + strings.TrimSpace(newText)
	for _, p := range preserved {
		out += " " + p
	}
	return out
}

func buildTaskTextLine(text string, priority int, due string, tags []string) string {
	parts := []string{strings.TrimSpace(text)}
	if priority >= 1 && priority <= 3 {
		parts = append(parts, "!"+string(rune('0'+priority)))
	}
	if due != "" {
		parts = append(parts, "due:"+due)
	}
	for _, tag := range tags {
		parts = append(parts, "#"+tag)
	}
	return strings.Join(parts, " ")
}
