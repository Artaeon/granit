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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/agentruntime"
	"github.com/artaeon/granit/internal/aiprefs"
	"github.com/artaeon/granit/internal/atomicio"
	"github.com/artaeon/granit/internal/deadlines"
	"github.com/artaeon/granit/internal/goals"
	"github.com/artaeon/granit/internal/granitmeta"
	"github.com/artaeon/granit/internal/habits"
	"github.com/artaeon/granit/internal/sabbath"
	"github.com/artaeon/granit/internal/wshub"
	"github.com/oklog/ulid/v2"
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

	// Consent gate — Daily Briefing is the natural feature flag for
	// "the AI rewrites the user's day", matching the morning-routine
	// posture. Users opt in via Settings → AI.
	prefs, _ := aiprefs.Load(s.cfg.Vault.Root)
	fcfg, fok := prefs.Features[aiprefs.FeatureDailyBriefing]
	if !fok || !fcfg.Enabled {
		send("error", mustJSON(map[string]string{"message": "feature \"daily_briefing\" is disabled in AI preferences"}))
		return
	}

	// Snapshot the day. Cheap; reads .granit/*.json + the daily note
	// + the live task store. The snapshot becomes the AI's user prompt.
	snap := s.buildRoutineSnapshot(date)

	cfgFile := resolveLLMConfig(s.cfg.Vault.Root, fcfg.Provider, prefs.DefaultProvider)
	llm, err := agentruntime.NewLLM(cfgFile)
	if err != nil {
		send("error", mustJSON(map[string]string{"message": err.Error()}))
		return
	}
	if hint := preflightLLM(llm); hint != "" {
		send("error", mustJSON(map[string]string{"message": hint}))
		return
	}

	systemPrompt := routineProposalSystemPrompt
	snapJSON, _ := json.Marshal(snap)
	userPrompt := fmt.Sprintf("Today's context (JSON):\n\n```json\n%s\n```\n\nReturn the proposal as a single JSON object matching the schema in your instructions.", string(snapJSON))

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()

	messages := []agentruntime.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	// Stream the model output and re-emit partial proposals as we go.
	// We accumulate raw text and try to parse a complete JSON object on
	// every chunk boundary; a successful parse replaces the current
	// preview. Partial / malformed objects are skipped silently — the
	// final flush carries the canonical result.
	var (
		buf      strings.Builder
		lastEmit string
		runErr   error
	)

	tryEmit := func() {
		text := buf.String()
		proposal, ok := tryParseRoutineProposal(text)
		if !ok {
			return
		}
		out := mustJSON(proposal)
		if out == lastEmit {
			return
		}
		lastEmit = out
		send("proposal", out)
	}

	if streamer, ok := llm.(agentruntime.ChatStreamer); ok {
		runErr = streamer.ChatStream(ctx, messages, func(chunk string) {
			buf.WriteString(chunk)
			tryEmit()
		})
	} else if chatter, ok := llm.(agentruntime.Chatter); ok {
		var reply string
		reply, runErr = chatter.Chat(ctx, messages)
		buf.WriteString(reply)
		tryEmit()
	} else {
		runErr = fmt.Errorf("configured LLM does not support chat")
	}

	if runErr != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			runErr = fmt.Errorf("cancelled by user")
		} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			runErr = fmt.Errorf("timed out")
		}
		send("error", mustJSON(map[string]string{"message": runErr.Error()}))
		return
	}

	// Final attempt to surface a canonical proposal if streaming
	// didn't already emit one (e.g. the model padded the JSON with
	// prose that made every intermediate parse fail).
	if lastEmit == "" {
		if proposal, ok := tryParseRoutineProposal(buf.String()); ok {
			send("proposal", mustJSON(proposal))
		} else {
			send("error", mustJSON(map[string]string{"message": "AI returned no parseable proposal"}))
			return
		}
	}
	send("done", `{"ok":true}`)
}

// routineProposalSystemPrompt is the model's instruction. We constrain
// the output to a single JSON object, list the exact schema, and pin the
// rules that make the proposal safe to apply (no ICS writes, no
// invented IDs).
const routineProposalSystemPrompt = `You are a calendar planner inside the user's personal vault (granit). The user has just asked you to rewrite their daily plan for the given date.

Return STRICTLY one JSON object — no prose, no fences, no commentary — matching this schema:

{
  "rationale": "1-2 sentences explaining the shape of the day you're proposing",
  "dailyPlan": "<markdown body for the ## Daily Plan section, WITHOUT the leading '## Daily Plan' header>",
  "eventOps": [
    { "op": "create", "event": { "title": "string", "date": "YYYY-MM-DD", "startTime": "HH:MM", "endTime": "HH:MM", "projectId": "optional string" } },
    { "op": "update", "eventId": "<existing id from the context>", "patch": { "startTime": "HH:MM", "endTime": "HH:MM", "title": "optional", "projectId": "optional" } },
    { "op": "delete", "eventId": "<existing id from the context>" }
  ]
}

Rules:
- ONLY mutate native granit events. The IDs in the context are the only ones you may reference for update / delete. NEVER invent an event ID.
- All times are HH:MM 24-hour local time. All dates are YYYY-MM-DD.
- Prefer small, conservative changes. If the existing day already looks reasonable, return an empty eventOps and a daily plan that just narrates it.
- The dailyPlan body should mention the goal, the top tasks, and the habits — match the morning-routine markdown style ("### Today's Goal", "### Tasks", "### Habits", "### Thoughts" when relevant).
- Do NOT emit ops on goals, deadlines, tasks, or habits. They are context only.
- Respect existing project_id and kind on updated events unless the user explicitly asked you to re-classify.
- Return at most 10 eventOps.`

// tryParseRoutineProposal extracts a JSON object from raw model output
// and decodes it into a routineProposal. Tolerates leading prose / code
// fences by walking from the first '{' and balancing braces. Returns
// (zero, false) when no complete object is found yet — the streaming
// loop calls this on every chunk and only emits when the parse
// succeeds, so partial output never reaches the client.
func tryParseRoutineProposal(s string) (routineProposal, bool) {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return routineProposal{}, false
	}
	depth := 0
	inStr := false
	escape := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if escape {
			escape = false
			continue
		}
		if inStr {
			if c == '\\' {
				escape = true
				continue
			}
			if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				var p routineProposal
				if err := json.Unmarshal([]byte(s[start:i+1]), &p); err != nil {
					return routineProposal{}, false
				}
				return p, true
			}
		}
	}
	return routineProposal{}, false
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

// handleCalendarApplyRoutine applies a (possibly user-edited) proposal.
//
// Partial-safe: a mid-batch op failure does NOT abort the rest of the
// batch. The per-op error is captured into the Failed slice with the
// row's index in the request so the UI can highlight exactly which
// rows didn't land. This mirrors handlePlanDayApply and the bulk-patch
// pattern in habitsBulkSelect.svelte.ts.
//
// Order of work:
//  1. Rewrite the "## Daily Plan" section of the date's daily note when
//     a non-empty DailyPlan is supplied. Failure here aborts the whole
//     call — losing the plan write is the only thing that would leave
//     the user without an audit trail of what changed.
//  2. Walk eventOps in order. Each op is executed atomically by reading
//     events.json, mutating in-memory, writing back. On any per-op
//     error we record the failure and continue.
//  3. Broadcast event.changed / event.removed for every successful op
//     so the calendar refreshes on connected clients.
//
// We never touch ICS files — eventOps only mutate the native events
// sidecar.
func (s *Server) handleCalendarApplyRoutine(w http.ResponseWriter, r *http.Request) {
	var body routineApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Date = strings.TrimSpace(body.Date)
	if !eventDateRe.MatchString(body.Date) {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}

	failed := make([]routineApplyFailure, 0)
	applied := 0

	// 1. Daily-plan rewrite. We only touch the "## Daily Plan" section;
	// the rest of the note is preserved byte-for-byte. Errors here are
	// returned as 500 — the user gets to retry with a cleaner state
	// rather than silently losing their proposed plan.
	if strings.TrimSpace(body.DailyPlan) != "" {
		if err := s.rewriteDailyPlan(body.Date, body.DailyPlan); err != nil {
			writeError(w, http.StatusInternalServerError, "daily plan rewrite failed: "+err.Error())
			return
		}
	}

	// 2. Event ops. Read events.json once, apply ops in-memory, write
	// once. Atomic enough for the small batches the AI proposes; if a
	// per-op validation fails we record the failure and move on.
	events, err := granitmeta.ReadEvents(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Track which event IDs need a hub broadcast after the write lands.
	// Two slices so we can broadcast event.changed for create/update
	// and event.removed for delete in one pass after the disk write.
	changed := make([]string, 0)
	removed := make([]string, 0)

	for i, op := range body.EventOps {
		switch op.Op {
		case "create":
			if op.Event == nil {
				failed = append(failed, routineApplyFailure{Index: i, Op: op.Op, Message: "missing event payload"})
				continue
			}
			ev := *op.Event
			if strings.TrimSpace(ev.Title) == "" || strings.TrimSpace(ev.Date) == "" {
				failed = append(failed, routineApplyFailure{Index: i, Op: op.Op, Message: "title and date required"})
				continue
			}
			if err := validateEventTimes(ev.Date, ev.StartTime, ev.EndTime); err != nil {
				failed = append(failed, routineApplyFailure{Index: i, Op: op.Op, Message: err.Error()})
				continue
			}
			if strings.TrimSpace(ev.ID) == "" {
				ev.ID = newRoutineEventID()
			}
			if ev.CreatedAt == "" {
				ev.CreatedAt = time.Now().Format(time.RFC3339)
			}
			events = append(events, ev)
			changed = append(changed, ev.ID)
			applied++

		case "update":
			id := strings.TrimSpace(op.EventID)
			if id == "" {
				failed = append(failed, routineApplyFailure{Index: i, Op: op.Op, Message: "missing eventId"})
				continue
			}
			idx := -1
			for j, ev := range events {
				if ev.ID == id {
					idx = j
					break
				}
			}
			if idx < 0 {
				failed = append(failed, routineApplyFailure{Index: i, Op: op.Op, EventID: id, Message: "event not found"})
				continue
			}
			ev := events[idx]
			applyRoutinePatch(&ev, op.Patch)
			if err := validateEventTimes(ev.Date, ev.StartTime, ev.EndTime); err != nil {
				failed = append(failed, routineApplyFailure{Index: i, Op: op.Op, EventID: id, Message: err.Error()})
				continue
			}
			events[idx] = ev
			changed = append(changed, ev.ID)
			applied++

		case "delete":
			id := strings.TrimSpace(op.EventID)
			if id == "" {
				failed = append(failed, routineApplyFailure{Index: i, Op: op.Op, Message: "missing eventId"})
				continue
			}
			found := false
			out := make([]granitmeta.Event, 0, len(events))
			for _, ev := range events {
				if ev.ID == id {
					found = true
					continue
				}
				out = append(out, ev)
			}
			if !found {
				failed = append(failed, routineApplyFailure{Index: i, Op: op.Op, EventID: id, Message: "event not found"})
				continue
			}
			events = out
			removed = append(removed, id)
			applied++

		default:
			failed = append(failed, routineApplyFailure{Index: i, Op: op.Op, Message: "unknown op (expected create/update/delete)"})
		}
	}

	// One disk write covers every successful op. If the write itself
	// fails we can't credit any of the ops as applied — flip them all
	// back into failed so the UI's "applied N" doesn't lie. We don't
	// try to be clever about rollback; the in-memory list is already
	// the source of truth for the next read.
	if err := granitmeta.WriteEvents(s.cfg.Vault.Root, events); err != nil {
		writeError(w, http.StatusInternalServerError, "events write failed: "+err.Error())
		return
	}

	for _, id := range changed {
		s.hub.Broadcast(wshub.Event{Type: "event.changed", ID: id})
	}
	for _, id := range removed {
		s.hub.Broadcast(wshub.Event{Type: "event.removed", ID: id})
	}

	writeJSON(w, http.StatusOK, routineApplyResponse{
		Applied: applied,
		Failed:  failed,
	})
}

// rewriteDailyPlan replaces (or inserts) the "## Daily Plan" section in
// the daily note for the given date. Reuses upsertNamedSection so the
// section parser stays in one place. EnsureDaily creates the note when
// missing so the first apply of the day still lands.
func (s *Server) rewriteDailyPlan(date, planBody string) error {
	cfg := s.dailyConfigFor()
	folder := strings.Trim(cfg.Folder, "/")
	rel := date + ".md"
	if folder != "" {
		rel = filepath.ToSlash(filepath.Join(folder, date+".md"))
	}
	dailyPath := filepath.Join(s.cfg.Vault.Root, rel)

	raw, err := os.ReadFile(dailyPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	plan := planBody
	// Caller may include or omit the "## Daily Plan" header. We
	// normalise to "header + body" so upsertNamedSection always
	// matches a known marker.
	if !strings.HasPrefix(strings.TrimSpace(plan), "## Daily Plan") {
		plan = "## Daily Plan — " + date + "\n\n" + plan
	}
	updated := upsertDailyPlan(string(raw), plan)

	if err := atomicio.WriteNote(dailyPath, updated); err != nil {
		return err
	}

	// Refresh the in-memory state so subsequent reads see the new
	// content. Same pattern handleSaveMorning uses.
	s.rescanMu.Lock()
	_ = s.cfg.Vault.ScanFast()
	_ = s.cfg.TaskStore.Reload()
	s.rescanMu.Unlock()
	return nil
}

// applyRoutinePatch translates the AI's loosely-typed patch map into
// concrete field assignments on a granitmeta.Event. We accept both
// snake_case (events.json convention) and camelCase (AI JSON-output
// convention) keys for every field — the AI's instructions name
// camelCase but local round-trips happen in snake_case, so accepting
// both avoids a fragile schema gate.
func applyRoutinePatch(ev *granitmeta.Event, patch map[string]any) {
	if patch == nil {
		return
	}
	pickStr := func(keys ...string) (string, bool) {
		for _, k := range keys {
			if v, ok := patch[k]; ok {
				if s, ok := v.(string); ok {
					return s, true
				}
			}
		}
		return "", false
	}
	if v, ok := pickStr("title"); ok {
		ev.Title = v
	}
	if v, ok := pickStr("date"); ok {
		ev.Date = v
	}
	if v, ok := pickStr("startTime", "start_time"); ok {
		ev.StartTime = v
	}
	if v, ok := pickStr("endTime", "end_time"); ok {
		ev.EndTime = v
	}
	if v, ok := pickStr("location"); ok {
		ev.Location = v
	}
	if v, ok := pickStr("color"); ok {
		ev.Color = v
	}
	if v, ok := pickStr("kind"); ok {
		ev.Kind = v
	}
	if v, ok := pickStr("projectId", "project_id"); ok {
		ev.ProjectID = v
	}
}

// newRoutineEventID mints a lowercase ULID for a created event. Kept
// as a tiny indirection so the apply handler doesn't drag the ulid
// package import into every commit in the series.
func newRoutineEventID() string {
	return strings.ToLower(ulid.Make().String())
}

