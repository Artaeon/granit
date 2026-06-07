package serveapi

// Daily Routine AI — Phase 2.
//
// Two endpoints:
//
//   POST /api/v1/calendar/routine-proposal — streams an SSE proposal for
//        the day. Body: {"date":"YYYY-MM-DD"} (defaults to today). The
//        stream emits two event kinds:
//          event: proposal — data is the partial / final JSON object
//                            (see routineProposal below)
//          event: done     — data is {"ok":true}
//          event: error    — data is {"message":"…"}
//
//   POST /api/v1/calendar/apply-routine — applies a (possibly user-edited)
//        proposal. Body: {"date":"YYYY-MM-DD","dailyPlan":"…","eventOps":[…]}.
//        Returns {"applied":N,"failed":[…]} — partial-safe: a mid-batch op
//        failure does NOT abort the rest; the failed op IDs / indices are
//        reported back so the UI can surface which rows didn't land.
//
// Constraints:
//   - Only native granit events (events.json) are mutated. ICS files
//     under <vault>/Calendars/ are externally-synced mirrors and stay
//     read-only.
//   - The daily-plan rewrite reuses upsertNamedSection from
//     handlers_morning.go so the section parser stays in one place.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/deadlines"
	"github.com/artaeon/granit/internal/goals"
	"github.com/artaeon/granit/internal/granitmeta"
	"github.com/artaeon/granit/internal/habits"
	"github.com/artaeon/granit/internal/sabbath"
)

// routineProposalRequest is the optional body for the streaming endpoint.
// Empty body → today. Bad date → 400. We accept YYYY-MM-DD only.
type routineProposalRequest struct {
	Date string `json:"date"`
}

// routineEventOp is one event mutation the AI proposes. Op is one of
// "create" / "update" / "delete". The relevant fields vary by op:
//   - create: Event is required (title + date + start/end times).
//   - update: EventID + Patch are required.
//   - delete: EventID is required.
//
// Kept as a single struct (rather than an interface or three types) so the
// JSON shape matches the wire format the AI emits + the frontend posts
// back; the apply path branches on Op.
type routineEventOp struct {
	Op      string            `json:"op"`
	Event   *granitmeta.Event `json:"event,omitempty"`
	EventID string            `json:"eventId,omitempty"`
	Patch   map[string]any    `json:"patch,omitempty"`
}

// routineProposal is the JSON shape the SSE stream emits + the apply
// endpoint expects. Match this exactly in the frontend's TS types.
type routineProposal struct {
	Rationale string           `json:"rationale"`
	DailyPlan string           `json:"dailyPlan"`
	EventOps  []routineEventOp `json:"eventOps"`
}

// handleCalendarRoutineProposal streams a routine proposal for the given
// date. Stub for commit 1: returns a hardcoded fake proposal as a single
// SSE event so the route + wire shape can be exercised end-to-end before
// wiring the snapshot + AI call.
func (s *Server) handleCalendarRoutineProposal(w http.ResponseWriter, r *http.Request) {
	var body routineProposalRequest
	if r.Body != nil && r.ContentLength != 0 {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	date := strings.TrimSpace(body.Date)
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	if !eventDateRe.MatchString(date) {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported by transport")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	send := func(event, data string) {
		if event != "" {
			_, _ = fmt.Fprintf(w, "event: %s\n", event)
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	// Sabbath gate — same posture as the calendar agent.
	if sabbath.IsActiveNow(s.cfg.Vault.Root) {
		send("error", mustJSON(map[string]string{"message": "AI features are paused during Sabbath — exit Sabbath mode to use them"}))
		return
	}

	// Snapshot the day. Cheap; reads .granit/*.json + the daily note
	// + the live task store. We include it in the stub response so the
	// frontend can render the context summary even before the AI call
	// is wired in. The next commit replaces the stub rationale with a
	// real model response.
	snap := s.buildRoutineSnapshot(date)
	rationale := fmt.Sprintf(
		"Stub proposal — %d events, %d tasks, %d active goals, %d habits, %d deadlines in scope. Real AI call lands in a follow-up commit.",
		len(snap.Events), len(snap.Tasks), len(snap.Goals), len(snap.Habits), len(snap.Deadlines),
	)
	stub := routineProposal{
		Rationale: rationale,
		DailyPlan: "## Daily Plan — " + date + "\n\n_(stub — no real plan yet)_\n",
		EventOps:  []routineEventOp{},
	}
	send("proposal", mustJSON(stub))
	send("done", `{"ok":true}`)
}

// routineSnapshot is the AI prompt's context section. Trimmed shapes
// (we drop everything the AI doesn't need) so the prompt stays bounded
// regardless of vault size.
type routineSnapshot struct {
	Date             string             `json:"date"`
	Events           []routineEventLite `json:"events"`
	Tasks            []routineTaskLite  `json:"tasks"`
	Goals            []routineGoalLite  `json:"goals"`
	Habits           []routineHabitLite `json:"habits"`
	Deadlines        []routineDeadlineLite `json:"deadlines"`
	CurrentDailyPlan string             `json:"currentDailyPlan,omitempty"`
}

// Trimmed shapes for the snapshot. We deliberately keep these small —
// the AI only needs the fields it's going to reason about, and a fat
// JSON payload would eat context budget on a vault with hundreds of
// goals or tasks.
type routineEventLite struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	StartTime string `json:"startTime,omitempty"`
	EndTime   string `json:"endTime,omitempty"`
	ProjectID string `json:"projectId,omitempty"`
	Kind      string `json:"kind,omitempty"`
}

type routineTaskLite struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Priority int    `json:"priority,omitempty"`
	DueDate  string `json:"dueDate,omitempty"`
	Project  string `json:"project,omitempty"`
}

type routineGoalLite struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	TargetDate string `json:"targetDate,omitempty"`
	Project    string `json:"project,omitempty"`
}

type routineHabitLite struct {
	Name      string `json:"name"`
	Frequency string `json:"frequency,omitempty"`
	Time      string `json:"time,omitempty"`
	Done      bool   `json:"done"`
}

type routineDeadlineLite struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Date       string `json:"date"`
	Importance string `json:"importance,omitempty"`
	Project    string `json:"project,omitempty"`
}

// buildRoutineSnapshot gathers the AI prompt's context section for the
// given date. Read-only — never mutates state. Cheap on the server:
// hits the live task store + vault index + the .granit sidecars.
//
// Scope rules:
//   - Events: native (events.json) events whose Date matches the target
//     date exactly. Recurring expansion is deliberately omitted — the
//     proposal is for ONE day and the AI should see the literal records
//     it can mutate. (Recurring overrides on that date are a follow-up.)
//   - Tasks: open tasks scheduled for the date OR due on/before it.
//     Cap at 40 to keep the prompt bounded.
//   - Goals: active goals (LoadActive), capped at 20.
//   - Habits: every non-archived habit + a Done flag computed from the
//     target day's daily-note checkbox state (mirrors handleDailyContext).
//   - Deadlines: deadlines within the same ISO week as the target date,
//     status=active.
//   - CurrentDailyPlan: the body of the existing "## Daily Plan" section
//     in the target day's daily note, when one exists. Empty otherwise.
func (s *Server) buildRoutineSnapshot(date string) routineSnapshot {
	snap := routineSnapshot{Date: date}

	target, err := time.Parse("2006-01-02", date)
	if err != nil {
		// Caller already validated date format — defensive fallback.
		return snap
	}

	// Events — native only, on the exact date. ICS sources are
	// excluded by construction (we only read events.json).
	if all, err := granitmeta.ReadEvents(s.cfg.Vault.Root); err == nil {
		for _, ev := range all {
			if ev.Date != date {
				continue
			}
			snap.Events = append(snap.Events, routineEventLite{
				ID:        ev.ID,
				Title:     ev.Title,
				StartTime: ev.StartTime,
				EndTime:   ev.EndTime,
				ProjectID: ev.ProjectID,
				Kind:      ev.Kind,
			})
		}
	}

	// Tasks — open, scoped to the day or earlier-due.
	const maxTasks = 40
	for _, t := range s.cfg.TaskStore.All() {
		if t.Done {
			continue
		}
		scheduledForDay := false
		if t.ScheduledStart != nil {
			ss := *t.ScheduledStart
			scheduledForDay = ss.Year() == target.Year() && ss.YearDay() == target.YearDay()
		}
		dueByDay := t.DueDate != "" && t.DueDate <= date
		if !scheduledForDay && !dueByDay {
			continue
		}
		snap.Tasks = append(snap.Tasks, routineTaskLite{
			ID:       t.ID,
			Text:     t.Text,
			Priority: t.Priority,
			DueDate:  t.DueDate,
			Project:  t.Project,
		})
		if len(snap.Tasks) >= maxTasks {
			break
		}
	}

	// Goals — active only, capped.
	const maxGoals = 20
	for _, g := range goals.LoadActive(s.cfg.Vault.Root) {
		snap.Goals = append(snap.Goals, routineGoalLite{
			ID:         g.ID,
			Title:      g.Title,
			TargetDate: g.TargetDate,
			Project:    g.Project,
		})
		if len(snap.Goals) >= maxGoals {
			break
		}
	}

	// Habits — non-archived. The "done today" flag mirrors the
	// hasCheckedHabit logic used by handleDailyContext so the AI sees
	// the same view of progress the user does.
	dailyBody := s.readDailyBody(date)
	hData := habits.Load(s.cfg.Vault.Root)
	for _, h := range hData.Habits {
		if hData.Archived[h.Name] {
			continue
		}
		snap.Habits = append(snap.Habits, routineHabitLite{
			Name:      h.Name,
			Frequency: hData.Frequencies[h.Name],
			Time:      hData.Times[h.Name],
			Done:      hasCheckedHabit(dailyBody, h.Name),
		})
	}

	// Deadlines — within the same ISO week as the target date,
	// active only. The week is anchored on Monday to match the rest
	// of granit's week-window semantics.
	year, week := target.ISOWeek()
	for _, d := range deadlines.LoadAll(s.cfg.Vault.Root) {
		if d.Status != "" && d.Status != "active" {
			continue
		}
		if !deadlines.ValidateDate(d.Date) {
			continue
		}
		dt, _ := time.Parse("2006-01-02", d.Date)
		dy, dw := dt.ISOWeek()
		if dy != year || dw != week {
			continue
		}
		snap.Deadlines = append(snap.Deadlines, routineDeadlineLite{
			ID:         d.ID,
			Title:      d.Title,
			Date:       d.Date,
			Importance: d.Importance,
			Project:    d.ProjectName,
		})
	}

	snap.CurrentDailyPlan = extractDailyPlanSection(dailyBody)
	return snap
}

// readDailyBody returns the raw markdown of the daily note for the
// given date, or "" when no note exists yet. Uses the same
// dailyConfigFor resolver the other handlers use so the daily folder
// override stays consistent.
func (s *Server) readDailyBody(date string) string {
	dailyCfg := s.dailyConfigFor()
	folder := strings.Trim(dailyCfg.Folder, "/")
	rel := date + ".md"
	if folder != "" {
		rel = filepath.ToSlash(filepath.Join(folder, date+".md"))
	}
	if n := s.cfg.Vault.GetNote(rel); n != nil {
		s.cfg.Vault.EnsureLoaded(rel)
		if n2 := s.cfg.Vault.GetNote(rel); n2 != nil {
			return n2.Content
		}
	}
	return ""
}

// extractDailyPlanSection pulls the body of an existing "## Daily Plan"
// section from a daily note. Returns "" when the section is absent.
// Same parsing rules as upsertNamedSection: marker line followed by
// content until the next H2 or EOF.
func extractDailyPlanSection(body string) string {
	if body == "" {
		return ""
	}
	const marker = "## Daily Plan"
	idx := strings.Index(body, marker)
	if idx < 0 {
		return ""
	}
	rest := body[idx:]
	end := -1
	for i := 0; i < len(rest); {
		nl := strings.IndexByte(rest[i:], '\n')
		var line string
		if nl < 0 {
			line = rest[i:]
			i = len(rest)
		} else {
			line = rest[i : i+nl+1]
			i += nl + 1
		}
		if i-len(line) == 0 {
			continue // skip the marker line itself
		}
		if strings.HasPrefix(strings.TrimRight(line, "\n"), "## ") &&
			!strings.HasPrefix(strings.TrimRight(line, "\n"), "### ") {
			end = i - len(line)
			break
		}
	}
	if end < 0 {
		return rest
	}
	return rest[:end]
}


// routineApplyRequest is the body for /api/v1/calendar/apply-routine.
// Date scopes the dailyPlan rewrite to that day's daily note. eventOps
// is the user's possibly-edited subset of the proposed ops.
type routineApplyRequest struct {
	Date      string           `json:"date"`
	DailyPlan string           `json:"dailyPlan"`
	EventOps  []routineEventOp `json:"eventOps"`
}

// routineApplyFailure records one failed op for the partial-safe response.
// Index is the position in the request's eventOps array (so the UI can
// highlight the row that didn't land); Message is the underlying error.
type routineApplyFailure struct {
	Index   int    `json:"index"`
	Op      string `json:"op,omitempty"`
	EventID string `json:"eventId,omitempty"`
	Message string `json:"message"`
}

type routineApplyResponse struct {
	Applied int                   `json:"applied"`
	Failed  []routineApplyFailure `json:"failed"`
}

// handleCalendarApplyRoutine applies a proposal. Stub for commit 1 —
// validates the body shape + returns an empty applied/failed response so
// the frontend wiring has something to call. Real apply lands in a later
// commit.
func (s *Server) handleCalendarApplyRoutine(w http.ResponseWriter, r *http.Request) {
	var body routineApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !eventDateRe.MatchString(strings.TrimSpace(body.Date)) {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	writeJSON(w, http.StatusOK, routineApplyResponse{
		Applied: 0,
		Failed:  []routineApplyFailure{},
	})
}
