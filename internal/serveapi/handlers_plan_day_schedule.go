package serveapi

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/agentruntime"
	"github.com/artaeon/granit/internal/agents"
	"github.com/artaeon/granit/internal/config"
	"github.com/artaeon/granit/internal/tasks"
	"github.com/oklog/ulid/v2"
)

// POST /api/v1/agents/plan-day-schedule
//
// Synchronous wrapper around the plan-my-day preset that ALSO parses
// the resulting `## Plan` block in today's daily note and writes
// scheduledStart + durationMinutes back to each task whose text fuzzy-
// matches a plan line.
//
// Trade-off: parsing the agent's `## Plan` text is the cleaner approach
// over adding a `schedule_task` write tool to the LLM toolbelt. Reasons:
//
//   - Deterministic: regex extracts every block; we never miss one
//     because the LLM forgot to call a tool. Easier to debug.
//   - Same surface: the user reads the same `## Plan` text in their
//     daily note either way — so post-processing the natural output
//     keeps the LLM's text the source of truth.
//   - Unmatched lines surface as a count ("Scheduled 4 of 6") rather
//     than silently dropping. With a write tool, the LLM might invent
//     task IDs, plan items it never actually scheduled, etc.
//
// The cost: the regex has to track the en-dash / em-dash / hyphen
// variants the LLM emits non-deterministically. We accept all three.

// planLineRE matches `- HH:MM–HH:MM — text` blocks. Group order:
//
//	1: start hour
//	2: start minute
//	3: end hour
//	4: end minute
//	5: trailing text (the task / focus theme)
//
// Hyphen variants accepted between times: -, –, —. Same between time and
// text. We deliberately keep the regex liberal — false-positive matches
// in body text are filtered by the `## Plan` section scope.
var planLineRE = regexp.MustCompile(`^\s*-\s*(\d{2}):(\d{2})\s*[-–—]\s*(\d{2}):(\d{2})\s*[-–—]\s*(.+?)\s*$`)

type planBlock struct {
	StartH, StartM int
	EndH, EndM     int
	Text           string
}

// parsePlanSection extracts plan blocks from `## Plan` (case-insensitive)
// in a daily note's body. The section runs until the next `##` heading
// or EOF. Blank lines and non-matching lines are skipped silently.
func parsePlanSection(body string) []planBlock {
	lines := strings.Split(body, "\n")
	var (
		blocks []planBlock
		inPlan bool
	)
	for _, ln := range lines {
		// New `## ` heading: enter Plan section if it's "Plan", exit if
		// it's anything else. Title-only match — sometimes the LLM
		// emits "## Plan" or "## Plan — Friday" or "## Today's Plan".
		// We accept "Plan" as a prefix word in the heading.
		trimmed := strings.TrimSpace(ln)
		if strings.HasPrefix(trimmed, "## ") || trimmed == "##" {
			head := strings.TrimSpace(strings.TrimPrefix(trimmed, "##"))
			head = strings.ToLower(head)
			// "plan" alone, "plan — …", "today's plan", etc.
			if head == "plan" ||
				strings.HasPrefix(head, "plan ") ||
				strings.HasPrefix(head, "plan—") ||
				strings.HasPrefix(head, "plan–") ||
				strings.HasPrefix(head, "plan-") ||
				strings.Contains(head, "today's plan") ||
				strings.Contains(head, "daily plan") {
				inPlan = true
				continue
			}
			inPlan = false
			continue
		}
		if !inPlan {
			continue
		}
		m := planLineRE.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		sh := mustAtoi(m[1])
		sm := mustAtoi(m[2])
		eh := mustAtoi(m[3])
		em := mustAtoi(m[4])
		txt := strings.TrimSpace(m[5])
		if txt == "" {
			continue
		}
		blocks = append(blocks, planBlock{
			StartH: sh, StartM: sm, EndH: eh, EndM: em, Text: txt,
		})
	}
	return blocks
}

func mustAtoi(s string) int {
	// We've already matched \d{2} in the regex, so this can't fail
	// unless someone deletes the regex constraint. Still, return 0 as
	// a safe fallback rather than panicking from a server handler.
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// fuzzyMatch finds the best matching task for a plan line. Strategy:
// case-insensitive substring; prefer the LONGEST overlap (so "ship the
// auth refresh" matches a task "Ship the auth refresh" over a task
// "Ship something"). Returns nil if no task contains a 3+ char overlap
// (less is too noisy — "x" or "by" matches dozens of tasks).
func fuzzyMatch(line string, candidates []tasks.Task) *tasks.Task {
	needle := strings.ToLower(strings.TrimSpace(line))
	if len(needle) < 3 {
		return nil
	}
	var best *tasks.Task
	bestScore := 0
	for i := range candidates {
		t := &candidates[i]
		hay := strings.ToLower(t.Text)
		// Try the full line first — perfect substring is a strong signal.
		score := substringOverlap(hay, needle)
		if score > bestScore {
			bestScore = score
			best = t
		}
	}
	// Require at least 3 chars of overlap. Below that we'd match
	// stop-words or coincidental letter pairs.
	if bestScore < 3 {
		return nil
	}
	return best
}

// substringOverlap returns the length of the longest contiguous substring
// of needle that appears in hay. We don't need a true LCS — substring
// is good enough and ~10x faster. Walks the needle from longest prefix
// down to length 3 and short-circuits on first hit.
func substringOverlap(hay, needle string) int {
	if needle == "" || hay == "" {
		return 0
	}
	// Fast path: full needle is in hay.
	if strings.Contains(hay, needle) {
		return len(needle)
	}
	// Window-shrink: try every prefix from the full needle down. This
	// favours matches that anchor at the start of the plan line, which
	// is where the meaningful text usually is (the LLM tends to put
	// the verb-phrase first: "Ship the auth refresh").
	for n := len(needle) - 1; n >= 3; n-- {
		// Try every substring of needle of length n; first hit wins.
		// O(n²) in the worst case but n is bounded by the line length
		// (~60 chars) so it's effectively constant.
		for i := 0; i+n <= len(needle); i++ {
			sub := needle[i : i+n]
			if strings.Contains(hay, sub) {
				return n
			}
		}
	}
	return 0
}

type planScheduleScheduled struct {
	TaskID string `json:"taskId"`
	Start  string `json:"start"`
}

// planProposal is one editable row in the new preview UI. The drawer
// shows a list of these and POSTs the user's edited subset back to
// /agents/plan-day-apply for actual scheduling.
//
// PlanLine is the raw markdown the LLM emitted (e.g.
// "- 09:00–09:30 — review PR"); the UI shows it in a tooltip so the
// user can see *why* this slot exists.
//
// Reason is the matched plan-line text only (no time prefix) — what
// fuzzyMatch matched against. Useful for debugging surprising matches.
type planProposal struct {
	TaskID           string `json:"taskId"`
	TaskText         string `json:"taskText"`
	Start            string `json:"start"`
	DurationMinutes  int    `json:"durationMinutes"`
	PlanLine         string `json:"planLine"`
	Reason           string `json:"reason"`
}

type planScheduleResponse struct {
	RunID     string                  `json:"runId"`
	Scheduled []planScheduleScheduled `json:"scheduled"`
	Unmatched []string                `json:"unmatched"`
	// Proposals is populated for both dry-run and apply modes so the UI
	// can always render the same row shape. In non-dry-run mode, every
	// entry in Scheduled has a corresponding Proposal at the same index
	// (proposals carry the human-readable text + plan line; scheduled
	// is the persisted record). In dry-run mode, Scheduled is empty.
	Proposals []planProposal `json:"proposals"`
	DryRun    bool           `json:"dryRun"`
}

// planScheduleRequest is the optional body for POST /agents/plan-day-schedule.
// An empty body still works (legacy callers) — DryRun defaults to false
// so the behaviour matches the original "fire-and-forget" flow.
type planScheduleRequest struct {
	DryRun bool `json:"dry_run"`
}

func (s *Server) handlePlanDaySchedule(w http.ResponseWriter, r *http.Request) {
	// Decode optional body. Empty body = legacy behaviour (immediate
	// scheduling). Bad JSON is tolerated as empty body — we don't want
	// to break the existing TaskBacklog button if the web ever sends
	// a stale shape during a deploy.
	var req planScheduleRequest
	if r.Body != nil && r.ContentLength != 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	cat := agents.NewPresetCatalog(agents.BuiltinPresets())
	_, _ = cat.LoadVaultDir(s.cfg.Vault.Root)
	preset, ok := cat.ByID("plan-my-day")
	if !ok {
		writeError(w, http.StatusNotFound, "plan-my-day preset missing")
		return
	}

	cfg := config.LoadForVault(s.cfg.Vault.Root)
	llm, err := agentruntime.NewLLM(cfg)
	if err != nil {
		// User-fixable misconfig (no API key, bad model). 400 not 500.
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Pre-flight ping: same shape as /agents/run. Without this, a bad
	// model / unreachable Ollama / wrong key makes the user wait the
	// full 3-minute timeout for an empty result. preflightLLM returns
	// classifiable misconfigs (404, refused, bad key) within ~5s so
	// the UI gets an actionable 400 message instead of a silent 200.
	if hint := preflightLLM(llm); hint != "" {
		writeError(w, http.StatusBadRequest, hint)
		return
	}

	bridge := agentruntime.NewBridge(s.cfg.Vault, s.cfg.TaskStore, nil, nil)
	runner := agentruntime.New(llm, bridge)

	runID := strings.ToLower(ulid.Make().String())
	startedAt := time.Now()

	// 3-min hard timeout — plan-my-day is a short preset. Anything
	// longer is almost certainly a stuck tool call and we'd rather
	// fail with a clean error than hang the request.
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()

	tr, cost, runErr := runner.Run(ctx, preset, "", nil)
	if runErr != nil && ctx.Err() == nil {
		// Run errored before timeout — persist a transcript (so the
		// user can audit the failure) and surface a 500.
		if tr != nil {
			budgetHit := false
			if cost != nil {
				budgetHit = cost.Snapshot().BudgetHit
			}
			_, _ = persistAgentRun(s, preset, *tr, "", startedAt, runErr, cfg, cost, budgetHit)
		}
		writeError(w, http.StatusInternalServerError, "agent run failed: "+runErr.Error())
		return
	}
	// Persist a transcript for the successful run (or the timeout case
	// — partial transcript is still useful audit trail). Mirrors what
	// /agents/run does so the user can review what the AI did from the
	// /agents page either way. We don't WS-broadcast here; the HTTP
	// caller is already awaiting our response.
	if tr != nil {
		budgetHit := false
		if cost != nil {
			budgetHit = cost.Snapshot().BudgetHit
		}
		_, _ = persistAgentRun(s, preset, *tr, "", startedAt, runErr, cfg, cost, budgetHit)
	}
	// On timeout we still try to parse — the agent may have written the
	// plan before the deadline hit. If not, we'll just return empty
	// scheduled/unmatched arrays.

	// Locate today's daily note and pull its body.
	dailyCfg := s.dailyConfigFor()
	folder := strings.Trim(dailyCfg.Folder, "/")
	today := time.Now().Format("2006-01-02")
	rel := today + ".md"
	if folder != "" {
		rel = filepath.ToSlash(filepath.Join(folder, today+".md"))
	}
	n := s.cfg.Vault.GetNote(rel)
	if n == nil {
		// No daily note yet. The agent should have created it but if
		// the LLM bailed before write_note, we end up here. Return an
		// empty result rather than 500 — the UI shows "0 scheduled".
		writeJSON(w, http.StatusOK, planScheduleResponse{
			RunID:     runID,
			Scheduled: []planScheduleScheduled{},
			Unmatched: []string{},
		})
		return
	}
	s.cfg.Vault.EnsureLoaded(rel)
	// Read fresh from disk if EnsureLoaded didn't pick up the agent's
	// write yet — atomic rename can race the watcher's debounce.
	abs := filepath.Join(s.cfg.Vault.Root, rel)
	body := n.Content
	if raw, err := os.ReadFile(abs); err == nil && len(raw) > 0 {
		body = string(raw)
	}

	blocks := parsePlanSection(body)

	// Today's open task pool: unfinished tasks that are unscheduled OR
	// scheduled today. Anything scheduled on a different day stays put.
	startOfDay := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, time.Local)
	endOfDay := startOfDay.Add(24 * time.Hour)
	candidates := s.cfg.TaskStore.Filter(func(t tasks.Task) bool {
		if t.Done {
			return false
		}
		if t.ScheduledStart == nil {
			return true
		}
		// Scheduled today is fair game (the LLM might rearrange).
		ss := *t.ScheduledStart
		return !ss.Before(startOfDay) && ss.Before(endOfDay)
	})

	scheduled, unmatched, proposals := buildPlanProposals(
		blocks, candidates, startOfDay, req.DryRun, s.cfg.TaskStore,
	)

	writeJSON(w, http.StatusOK, planScheduleResponse{
		RunID:     runID,
		Scheduled: scheduled,
		Unmatched: unmatched,
		Proposals: proposals,
		DryRun:    req.DryRun,
	})
}

// planScheduler is the narrow contract buildPlanProposals needs from a
// task store. Lets tests pass a nil store for the dry-run path without
// dragging the full TaskStore machinery into a focused unit test.
type planScheduler interface {
	Schedule(id string, start time.Time, dur time.Duration) error
	Triage(id string, state tasks.TriageState) error
}

// buildPlanProposals walks the parsed plan blocks, fuzzy-matches each to
// a task in candidates, and either persists the schedule (dryRun=false)
// or just records the proposal (dryRun=true). Centralising this loop
// lets us unit-test the dry-run contract: "DryRun MUST NOT call Schedule".
//
// store may be nil only when dryRun=true. Passing nil in commit mode is
// a programmer error and would panic — the test helper avoids this.
func buildPlanProposals(
	blocks []planBlock,
	candidates []tasks.Task,
	startOfDay time.Time,
	dryRun bool,
	store planScheduler,
) (
	scheduled []planScheduleScheduled,
	unmatched []string,
	proposals []planProposal,
) {
	scheduled = make([]planScheduleScheduled, 0, len(blocks))
	unmatched = make([]string, 0)
	proposals = make([]planProposal, 0, len(blocks))

	for _, b := range blocks {
		t := fuzzyMatch(b.Text, candidates)
		if t == nil {
			unmatched = append(unmatched, b.Text)
			continue
		}
		start := time.Date(
			startOfDay.Year(), startOfDay.Month(), startOfDay.Day(),
			b.StartH, b.StartM, 0, 0, time.Local,
		)
		end := time.Date(
			startOfDay.Year(), startOfDay.Month(), startOfDay.Day(),
			b.EndH, b.EndM, 0, 0, time.Local,
		)
		dur := end.Sub(start)
		// Sanity: plan blocks crossing midnight or with negative duration
		// (LLM hallucination, e.g. 18:00–17:30) get clamped to a 30-min
		// default rather than dropped — better to schedule something at
		// the right HH:MM than to discard a perfectly-good match.
		if dur <= 0 {
			dur = 30 * time.Minute
		}

		// Build the proposal row for the UI either way. The LLM's text
		// is preserved verbatim so the user can see what the model
		// actually wrote — handy when fuzzyMatch latches onto an
		// unexpected task (rare but happens with short verbs).
		proposals = append(proposals, planProposal{
			TaskID:          t.ID,
			TaskText:        t.Text,
			Start:           start.Format(time.RFC3339),
			DurationMinutes: int(dur / time.Minute),
			PlanLine:        formatPlanLine(b),
			Reason:          b.Text,
		})

		if dryRun {
			// No write. The UI will collect edited proposals and POST
			// /agents/plan-day-apply with the user's chosen subset.
			continue
		}

		if err := store.Schedule(t.ID, start, dur); err != nil {
			// Schedule failure is rare (sidecar write IO) but
			// non-fatal — keep going for the rest of the blocks. Log
			// to the response by treating as unmatched so the UI's
			// "scheduled N of M" math stays honest.
			unmatched = append(unmatched, b.Text)
			continue
		}
		// Also flip triage to scheduled so the kanban board reflects.
		_ = store.Triage(t.ID, tasks.TriageScheduled)
		scheduled = append(scheduled, planScheduleScheduled{
			TaskID: t.ID,
			Start:  start.Format(time.RFC3339),
		})
	}
	return scheduled, unmatched, proposals
}

// formatPlanLine renders a planBlock back into the markdown line shape
// the LLM emits, so the drawer's tooltip ("AI suggested this because:
// <line>") shows the same text the user would see if they opened the
// daily note. Uses an en-dash to match the canonical preset output.
func formatPlanLine(b planBlock) string {
	return "- " +
		twoDigit(b.StartH) + ":" + twoDigit(b.StartM) +
		"–" +
		twoDigit(b.EndH) + ":" + twoDigit(b.EndM) +
		" — " + b.Text
}

func twoDigit(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

// planApplyProposal is one row from the user's edited subset. The web
// drawer collects these from the dry-run response, lets the user tweak
// time/duration/keep-skip, and POSTs the survivors here.
//
// Start is RFC3339; DurationMinutes is the user's chosen duration (the
// drawer's duration picker snaps to 15/30/45/60/90).
type planApplyProposal struct {
	TaskID          string `json:"taskId"`
	Start           string `json:"start"`
	DurationMinutes int    `json:"durationMinutes"`
}

type planApplyRequest struct {
	Proposals []planApplyProposal `json:"proposals"`
}

type planApplyResponse struct {
	Scheduled []planScheduleScheduled `json:"scheduled"`
	// Errors holds task IDs that couldn't be scheduled (sidecar write
	// failure, missing task, malformed start). UI shows these as
	// "couldn't apply N proposals".
	Errors []string `json:"errors"`
}

// handlePlanDayApply commits a subset of proposals from a prior dry-run
// to the task store. Idempotent against the same subset (TaskStore.Schedule
// overwrites scheduledStart/durationMinutes), so a click-double-click
// won't double-schedule.
//
// Why a separate endpoint instead of folding into plan-day-schedule:
// the dry-run is expensive (LLM call, ~5–30s) and we want apply to be
// fast (just sidecar writes, ~50ms). Splitting also means the apply
// path doesn't need an LLM at all — relevant when the user hits Apply
// after the model has gone offline mid-session.
func (s *Server) handlePlanDayApply(w http.ResponseWriter, r *http.Request) {
	var body planApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(body.Proposals) == 0 {
		// Empty apply is harmless but pointless — return an empty list
		// rather than an error so the UI can call Apply on a fully-
		// rejected proposal set without surfacing an error toast.
		writeJSON(w, http.StatusOK, planApplyResponse{
			Scheduled: []planScheduleScheduled{},
			Errors:    []string{},
		})
		return
	}

	scheduled := make([]planScheduleScheduled, 0, len(body.Proposals))
	errs := make([]string, 0)

	for _, p := range body.Proposals {
		if p.TaskID == "" {
			errs = append(errs, "(missing taskId)")
			continue
		}
		start, err := time.Parse(time.RFC3339, p.Start)
		if err != nil {
			errs = append(errs, p.TaskID)
			continue
		}
		dur := time.Duration(p.DurationMinutes) * time.Minute
		if dur <= 0 {
			dur = 30 * time.Minute
		}
		if err := s.cfg.TaskStore.Schedule(p.TaskID, start, dur); err != nil {
			errs = append(errs, p.TaskID)
			continue
		}
		_ = s.cfg.TaskStore.Triage(p.TaskID, tasks.TriageScheduled)
		scheduled = append(scheduled, planScheduleScheduled{
			TaskID: p.TaskID,
			Start:  start.Format(time.RFC3339),
		})
	}

	writeJSON(w, http.StatusOK, planApplyResponse{
		Scheduled: scheduled,
		Errors:    errs,
	})
}
