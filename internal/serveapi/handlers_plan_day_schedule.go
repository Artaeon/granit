package serveapi

import (
	"context"
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

type planScheduleResponse struct {
	RunID     string                  `json:"runId"`
	Scheduled []planScheduleScheduled `json:"scheduled"`
	Unmatched []string                `json:"unmatched"`
}

func (s *Server) handlePlanDaySchedule(w http.ResponseWriter, r *http.Request) {
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

	bridge := agentruntime.NewBridge(s.cfg.Vault, s.cfg.TaskStore, nil, nil)
	runner := agentruntime.New(llm, bridge)

	runID := strings.ToLower(ulid.Make().String())

	// 3-min hard timeout — plan-my-day is a short preset. Anything
	// longer is almost certainly a stuck tool call and we'd rather
	// fail with a clean error than hang the request.
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()

	_, _, runErr := runner.Run(ctx, preset, "", nil)
	if runErr != nil && ctx.Err() == nil {
		// Run errored before timeout — surface it.
		writeError(w, http.StatusInternalServerError, "agent run failed: "+runErr.Error())
		return
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

	scheduled := make([]planScheduleScheduled, 0, len(blocks))
	unmatched := make([]string, 0)

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
		if err := s.cfg.TaskStore.Schedule(t.ID, start, dur); err != nil {
			// Schedule failure is rare (sidecar write IO) but
			// non-fatal — keep going for the rest of the blocks. Log
			// to the response by treating as unmatched so the UI's
			// "scheduled N of M" math stays honest.
			unmatched = append(unmatched, b.Text)
			continue
		}
		// Also flip triage to scheduled so the kanban board reflects.
		_ = s.cfg.TaskStore.Triage(t.ID, tasks.TriageScheduled)
		scheduled = append(scheduled, planScheduleScheduled{
			TaskID: t.ID,
			Start:  start.Format(time.RFC3339),
		})
	}

	writeJSON(w, http.StatusOK, planScheduleResponse{
		RunID:     runID,
		Scheduled: scheduled,
		Unmatched: unmatched,
	})
}
