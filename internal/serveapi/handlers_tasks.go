package serveapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/artaeon/granit/internal/daily"
	"github.com/artaeon/granit/internal/granitmeta"
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
	GoalID           string     `json:"goalId,omitempty"`
	DeadlineID       string     `json:"deadlineId,omitempty"`
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

// priorityAPIToStore is the inverse — needed by the list-handler's
// `priority=` query filter so the web's "1=highest" query value maps
// to the parser's "4=highest" stored value.
func priorityAPIToStore(p int) int {
	switch p {
	case 1:
		return 4
	case 2:
		return 3
	case 3:
		return 2
	}
	return 0
}

// reExplicitDue catches the two markers our writers actually emit
// (📅 emoji + ASCII due:). Used by taskToView to suppress the
// parser's daily-filename fallback at the API boundary — the
// fallback is right for the TUI's display logic but lies to API
// consumers when the user explicitly cleared a due date with
// PATCH dueDate:"". Without this, "I removed the due date" became
// "due today" on every subsequent GET.
var reExplicitDue = regexp.MustCompile(`(?:\x{1F4C5}\s*\d{4}-\d{2}-\d{2})|(?:(?:^|\s)due:\d{4}-\d{2}-\d{2}(?:\s|$))`)

func taskToView(t tasks.Task) taskView {
	dueDate := t.DueDate
	if !reExplicitDue.MatchString(t.Text) {
		dueDate = ""
	}
	v := taskView{
		ID:               t.ID,
		GranitID:         t.ID,
		NotePath:         t.NotePath,
		LineNum:          t.LineNum,
		Text:             t.Text,
		Done:             t.Done,
		Priority:         priorityStoreToAPI(t.Priority),
		Tags:             t.Tags,
		DueDate:          dueDate,
		ProjectID:        t.ProjectID,
		Triage:           string(t.Triage),
		SnoozedUntil:     t.SnoozedUntil,
		EstimatedMinutes: t.EstimatedMinutes,
		DependsOn:        t.DependsOn,
		Indent:           t.Indent,
		ParentLine:       t.ParentLine,
		Recurrence:       t.Recurrence,
		Notes:            t.Notes,
		GoalID:           t.GoalID,
		DeadlineID:       t.DeadlineID,
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
	// New filters — priority / project / goal / deadline. The audit
	// caught these as silent no-ops: the URL accepted the query
	// parameter and returned the unfiltered set, which made every
	// "Filter by P1" or "Filter by project Hub - MealTime" appear
	// broken on refresh because the page-side filter wasn't
	// re-applied until the client rerendered.
	priorityFilter := 0
	if v := q.Get("priority"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			priorityFilter = priorityAPIToStore(n)
		}
	}
	projectFilter := q.Get("project")
	goalFilter := q.Get("goal")
	deadlineFilter := q.Get("deadline")

	// For the project filter we accept the project's Name (the same
	// identifier the rest of the API uses — `granitmeta.Project.Name`
	// is the load-bearing key on disk). A task is "in" the project
	// when one of: (a) its sidecar ProjectID matches, (b) the note
	// path lives under the project's folder, or (c) one of the
	// project's tags is on the task. This mirrors the web's existing
	// derivation in routes/tasks/+page.svelte so server-side filtering
	// agrees with client-side grouping.
	var matchProj func(t tasks.Task) bool
	if projectFilter != "" {
		var folder string
		var pTags []string
		if all, err := granitmeta.ReadProjects(s.cfg.Vault.Root); err == nil {
			for _, p := range all {
				if p.Name == projectFilter {
					folder = p.Folder
					pTags = p.Tags
					break
				}
			}
		}
		matchProj = func(t tasks.Task) bool {
			if t.ProjectID == projectFilter {
				return true
			}
			if folder != "" && strings.HasPrefix(t.NotePath, folder+"/") {
				return true
			}
			for _, pt := range pTags {
				for _, tt := range t.Tags {
					if pt == tt {
						return true
					}
				}
			}
			return false
		}
	}

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
		if priorityFilter > 0 && t.Priority != priorityFilter {
			continue
		}
		if matchProj != nil && !matchProj(t) {
			continue
		}
		if goalFilter != "" && t.GoalID != goalFilter {
			continue
		}
		if deadlineFilter != "" && t.DeadlineID != deadlineFilter {
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
	// GoalID and DeadlineID are pointer-typed for explicit clear
	// semantics: an absent field leaves the existing link alone,
	// `""` removes the marker, a value writes a new marker. Same
	// pattern as DueDate / SnoozedUntil. Round-trips through the
	// markdown line via transformGoal / transformDeadline.
	GoalID        *string `json:"goalId,omitempty"`
	DeadlineID    *string `json:"deadlineId,omitempty"`
	ClearSchedule bool    `json:"clearSchedule,omitempty"`
}

func (s *Server) handlePatchTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var b patchTaskBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	// Validate before any UpdateLine mutation. transformDue regex
	// only strips strict YYYY-MM-DD, so a malformed input (e.g.
	// "next-friday") would write a literal due:next-friday into the
	// markdown that the parser can't subsequently strip — every
	// later patch would APPEND a second due: token. Same for
	// snooze: catch malformed early and refuse.
	if b.DueDate != nil && *b.DueDate != "" {
		if _, err := time.Parse("2006-01-02", *b.DueDate); err != nil {
			writeError(w, http.StatusBadRequest, "dueDate must be YYYY-MM-DD")
			return
		}
	}
	if b.SnoozedUntil != nil && *b.SnoozedUntil != "" {
		if _, err := time.Parse("2006-01-02T15:04", *b.SnoozedUntil); err != nil {
			writeError(w, http.StatusBadRequest, "snoozedUntil must be YYYY-MM-DDThh:mm")
			return
		}
	}
	// Same defensive validation as DueDate / SnoozedUntil. transformGoal
	// only strips strict goal:G\d+ tokens; a malformed value (e.g.
	// "goal:foo") would leak through and the next patch would append a
	// second marker. Reject early.
	if b.GoalID != nil && *b.GoalID != "" {
		// Goal IDs come from two mints: TUI's Gxxx form and the web's
		// goal-<timestamp> form. Accept any letters/digits/dash/underscore
		// token; reject only payloads that would break the line marker
		// (whitespace, quotes, markdown punctuation).
		if !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(*b.GoalID) {
			writeError(w, http.StatusBadRequest, "goalId must be alphanumeric / dash / underscore")
			return
		}
	}
	if b.DeadlineID != nil && *b.DeadlineID != "" {
		if !regexp.MustCompile(`^[0-9a-z]{26}$`).MatchString(*b.DeadlineID) {
			writeError(w, http.StatusBadRequest, "deadlineId must be a 26-char ULID")
			return
		}
	}
	store := s.cfg.TaskStore
	if _, ok := store.GetByID(id); !ok {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	// Markdown-line mutations (bundled into a single UpdateLine for atomicity)
	if b.Done != nil || b.Priority != nil || b.DueDate != nil || b.Text != nil || b.SnoozedUntil != nil || b.Recurrence != nil || b.GoalID != nil || b.DeadlineID != nil {
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
			// Goal / deadline must run BEFORE transformText so that
			// transformText can preserve them in the same way it
			// preserves priority and due markers.
			if b.GoalID != nil {
				line = transformGoal(line, *b.GoalID)
			}
			if b.DeadlineID != nil {
				line = transformDeadline(line, *b.DeadlineID)
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

	// Force a fresh scan + task-store reload BEFORE responding so the
	// next GET (which the web fires immediately on success) is
	// guaranteed to see the post-patch state. Mirrors the same pattern
	// used in handleSaveMorning / handleToggleHabit / handlePatchDaily.
	//
	// Why: in plan mode the user drops a backlog task on the grid, the
	// frontend awaits this patch and then awaits GET /calendar to
	// repaint the grid. If the file watcher's debounce kicks in
	// between those two requests, ScanFast() can race the in-memory
	// schedule mutation we just applied — clearing it back out — so
	// the calendar feed comes back without the new task_scheduled
	// event and the user has to reload to see their drop. Synchronous
	// rescan here means the response is the post-state, full stop.
	s.rescanMu.Lock()
	_ = s.cfg.Vault.ScanFast()
	_ = s.cfg.TaskStore.Reload()
	s.rescanMu.Unlock()

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
	GoalID          string   `json:"goalId,omitempty"`
	DeadlineID      string   `json:"deadlineId,omitempty"`
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
	if b.DueDate != "" {
		if _, err := time.Parse("2006-01-02", b.DueDate); err != nil {
			writeError(w, http.StatusBadRequest, "dueDate must be YYYY-MM-DD")
			return
		}
	}
	if b.GoalID != "" && !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(b.GoalID) {
		writeError(w, http.StatusBadRequest, "goalId must be alphanumeric / dash / underscore")
		return
	}
	if b.DeadlineID != "" && !regexp.MustCompile(`^[0-9a-z]{26}$`).MatchString(b.DeadlineID) {
		writeError(w, http.StatusBadRequest, "deadlineId must be a 26-char ULID")
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
	if b.GoalID != "" {
		textWithMarkers += " goal:" + b.GoalID
	}
	if b.DeadlineID != "" {
		textWithMarkers += " deadline:" + b.DeadlineID
	}
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
	reGoalLineMarker     = regexp.MustCompile(`(^|\s)goal:[A-Za-z0-9_-]+(\s|$)`)
	reDeadlineLineMarker = regexp.MustCompile(`(^|\s)deadline:[0-9a-z]{26}(\s|$)`)
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

// transformGoal writes (or clears) a `goal:G\d+` marker on a checkbox
// line. Marker shape mirrors the parser's reGoalLink so the round-trip
// is symmetric. Empty `goalID` clears any existing marker.
func transformGoal(line string, goalID string) string {
	clean := reGoalLineMarker.ReplaceAllString(line, "$1$2")
	if goalID == "" {
		return strings.TrimRight(clean, " ")
	}
	return strings.TrimRight(clean, " ") + " goal:" + goalID
}

// transformDeadline writes (or clears) a `deadline:<ulid>` marker on
// a checkbox line. ULIDs come from internal/deadlines (lowercase
// 26-char Crockford alphabet). Empty `deadlineID` clears the marker.
// The marker shape parallels `goal:` — the TUI's parser ignores
// unknown markers gracefully, so a TUI that doesn't yet know about
// deadlines just leaves the token as inert text in the line.
func transformDeadline(line string, deadlineID string) string {
	clean := reDeadlineLineMarker.ReplaceAllString(line, "$1$2")
	if deadlineID == "" {
		return strings.TrimRight(clean, " ")
	}
	return strings.TrimRight(clean, " ") + " deadline:" + deadlineID
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
	for _, re := range []*regexp.Regexp{rePriorityMarker, reDueMarker, reGoalLineMarker, reDeadlineLineMarker} {
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
