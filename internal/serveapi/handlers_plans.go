package serveapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/aiprefs"
	"github.com/artaeon/granit/internal/goals"
	"github.com/artaeon/granit/internal/granitmeta"
	"github.com/artaeon/granit/internal/ventures"
)

// ─── Weekly Plan Extraction ──────────────────────────────────────
//
// The user writes a freeform note for the week. We send it to the LLM
// together with a structured snapshot of what already exists in the
// vault — ventures, projects, goals, open tasks, this week's calendar
// — and ask it to PROPOSE tasks/milestones to create, MATCHED against
// the existing entities. The user reviews the proposals in the
// /plans/week page and commits the ones they want.
//
// The handler does no writes — extraction is read-only. Commit lives
// in handlePlanCommit (next handler over). This split keeps the model
// inside a safe "propose only" box: nothing changes until the human
// approves.
//
// Context the model sees:
//   - All ventures (name, mission, status)
//   - All projects (name, venture, kind, status, next_action, due_date)
//   - All active goals (title, venture/project, target_date, current
//     milestones) — completed/archived goals are filtered out
//   - Up to 80 open tasks (text, project, due_date, scheduled_time)
//   - This week's calendar events (title, start, end) so the model
//     can avoid piling tasks onto an already-packed day
//
// Output is strict JSON matching planExtractionResponse below. The
// system prompt forbids fences and commentary; we defensively strip
// any anyway and surface a `warning` if the response was unparseable.

const planExtractSystemPrompt = `You are extracting a structured weekly plan from a user's freeform planning note.

INPUT
You receive:
  - PLAN_TEXT: the user's freeform brain-dump for this week
  - VAULT: a JSON snapshot of the user's existing ventures, projects, active goals, open tasks, and this week's calendar events

OUTPUT
Return STRICT JSON — no markdown fences, no preamble, no trailing commentary. Schema:
{
  "items": [
    {
      "kind": "task" | "milestone",
      "label": "<short imperative phrase, max ~80 chars>",
      "venture_name": "<exact venture name from VAULT.ventures, or empty>",
      "project_name": "<exact project name from VAULT.projects, or empty>",
      "goal_id": "<exact goal id from VAULT.goals, or empty>",
      "due_date": "YYYY-MM-DD or empty",
      "source_line": "<the snippet from PLAN_TEXT that produced this item>",
      "match_type": "exact" | "fuzzy" | "new" | "personal",
      "match_confidence": 0-100,
      "rationale": "<under 15 words, why this routing>"
    }
  ],
  "unmatched": ["<source lines you couldn't confidently route>"]
}

RULES
1. PRESERVE THE USER'S WORDS. Don't paraphrase the task text unless the phrasing is genuinely unclear. The user wrote what they meant.
2. MATCH STRICTLY. venture_name and project_name MUST be exact strings from VAULT (case-sensitive). If you're not sure, leave them empty and put the line in "unmatched" instead of inventing names.
3. FUZZY-MATCH is allowed when the user clearly meant an existing entity. E.g. user writes "abm letters" → existing project "25 Ärzte Briefe vorbereiten" → match_type "fuzzy", match_confidence 70. Use the EXACT existing name in project_name.
4. PROPOSE NEW ENTITIES SPARINGLY. match_type "new" means you're proposing a task that doesn't fit any existing project. The user must explicitly accept new entities. Don't invent ventures or projects; only propose new TASKS attached to existing ones (or to no parent).
5. PERSONAL items (sermon prep, weekly review, family things) → match_type "personal", venture/project empty.
6. DEDUPLICATE against VAULT.open_tasks. If a plan line obviously matches an already-open task ("ship plan-day-1" when that task is already open under MealTime), SKIP it — don't duplicate.
7. RESPECT THE CALENDAR. If the plan says "Wednesday" or "by Friday" and the calendar shows that day is already heavy, mention it in the rationale ("Wed already has 4 events — light day on Thu may be safer").
8. DUE DATES. Only set due_date when the source line gives a clear signal ("by Friday", "Wednesday", a specific date). Don't invent. For day names, resolve to the actual YYYY-MM-DD using TODAY in VAULT.
9. ONE ITEM PER ACTIONABLE LINE. A plan line with three sub-bullets becomes three items, each carrying the sub-bullet text. Don't fold multiple actions into one item.
10. milestones are weekly-scoped commitments that belong to an existing GOAL. Only emit kind="milestone" when the source line clearly maps to a goal in VAULT.

Be decisive. Confidence under 60 should go to unmatched, not low-confidence items.`

// planVaultSnapshot is the JSON the model sees as VAULT.
type planVaultSnapshot struct {
	Today    string             `json:"today"`
	WeekISO  string             `json:"week_iso"`
	Ventures []planVentureBrief `json:"ventures"`
	Projects []planProjectBrief `json:"projects"`
	Goals    []planGoalBrief    `json:"goals"`
	OpenTasks []planTaskBrief   `json:"open_tasks"`
	WeekCalendar []planEventBrief `json:"week_calendar"`
}

type planVentureBrief struct {
	Name    string `json:"name"`
	Mission string `json:"mission,omitempty"`
	Status  string `json:"status,omitempty"`
}
type planProjectBrief struct {
	Name       string `json:"name"`
	Venture    string `json:"venture,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Status     string `json:"status,omitempty"`
	NextAction string `json:"next_action,omitempty"`
	DueDate    string `json:"due_date,omitempty"`
}
type planGoalBrief struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Venture    string   `json:"venture,omitempty"`
	Project    string   `json:"project,omitempty"`
	TargetDate string   `json:"target_date,omitempty"`
	Milestones []string `json:"open_milestones,omitempty"`
}
type planTaskBrief struct {
	ID            string `json:"id"`
	Text          string `json:"text"`
	Project       string `json:"project,omitempty"`
	DueDate       string `json:"due_date,omitempty"`
	ScheduledTime string `json:"scheduled_time,omitempty"`
}
type planEventBrief struct {
	Title string `json:"title"`
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

// planExtractedItem mirrors the JSON the model returns per item.
type planExtractedItem struct {
	Kind            string `json:"kind"`
	Label           string `json:"label"`
	VentureName     string `json:"venture_name,omitempty"`
	ProjectName     string `json:"project_name,omitempty"`
	GoalID          string `json:"goal_id,omitempty"`
	DueDate         string `json:"due_date,omitempty"`
	SourceLine      string `json:"source_line,omitempty"`
	MatchType       string `json:"match_type,omitempty"`
	MatchConfidence int    `json:"match_confidence,omitempty"`
	Rationale       string `json:"rationale,omitempty"`
}

type planExtractionResponse struct {
	Items     []planExtractedItem `json:"items"`
	Unmatched []string            `json:"unmatched,omitempty"`
	Warning   string              `json:"warning,omitempty"`
	Raw       string              `json:"raw,omitempty"`
}

type planExtractRequest struct {
	PlanText string `json:"plan_text"`
	// WeekISO optional — when provided, helps the model resolve
	// relative dates ("Wednesday") to absolute YYYY-MM-DD. When
	// empty, the handler defaults to the current ISO week.
	WeekISO string `json:"week_iso,omitempty"`
}

func (s *Server) handlePlanExtract(w http.ResponseWriter, r *http.Request) {
	var req planExtractRequest
	if !readJSON(w, r, &req) {
		return
	}
	planText := strings.TrimSpace(req.PlanText)
	if planText == "" {
		writeError(w, http.StatusBadRequest, "plan_text is empty — nothing to extract")
		return
	}
	now := time.Now()
	weekISO := req.WeekISO
	if weekISO == "" {
		y, w := now.ISOWeek()
		weekISO = fmt.Sprintf("%d-W%02d", y, w)
	}

	snap := s.buildPlanSnapshot(now)
	snap.Today = now.Format("2006-01-02")
	snap.WeekISO = weekISO

	body, err := json.Marshal(snap)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to marshal snapshot: "+err.Error())
		return
	}
	userPrompt := "PLAN_TEXT:\n```\n" + planText + "\n```\n\nVAULT:\n```json\n" + string(body) + "\n```"

	out, err := s.runAIFeature(r.Context(), aiprefs.FeaturePlanExtract,
		planExtractSystemPrompt, userPrompt)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Strip optional fences before parsing — defensive against models
	// that ignore the "no fences" instruction.
	cleaned := strings.TrimSpace(out)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var parsed planExtractionResponse
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		writeJSON(w, http.StatusOK, planExtractionResponse{
			Items:   []planExtractedItem{},
			Warning: "Model didn't return parseable JSON. Showing raw response so you can see what came back.",
			Raw:     out,
		})
		return
	}
	// Defensive: ensure items is never null in the response so the
	// client's .filter / .map calls don't crash on an empty plan.
	if parsed.Items == nil {
		parsed.Items = []planExtractedItem{}
	}
	writeJSON(w, http.StatusOK, parsed)
}

// buildPlanSnapshot gathers the read-only vault state the extraction
// prompt sees. Kept tight on size — every list capped, every record
// trimmed to the fields a routing decision needs. The LLM doesn't
// need full task notes or venture descriptions; a name + status +
// link is enough.
func (s *Server) buildPlanSnapshot(now time.Time) planVaultSnapshot {
	snap := planVaultSnapshot{
		Ventures:     []planVentureBrief{},
		Projects:     []planProjectBrief{},
		Goals:        []planGoalBrief{},
		OpenTasks:    []planTaskBrief{},
		WeekCalendar: []planEventBrief{},
	}

	// Ventures — all of them; user typically has < 20.
	for _, v := range ventures.LoadAll(s.cfg.Vault.Root) {
		snap.Ventures = append(snap.Ventures, planVentureBrief{
			Name: v.Name, Mission: v.Mission, Status: string(v.Status),
		})
	}

	// Projects — every project the user has, skipping archived. The
	// model needs to see archived names only if it has to deduplicate
	// against them; current scope is "match active work" so archived
	// stays out.
	if projs, err := granitmeta.ReadProjects(s.cfg.Vault.Root); err == nil {
		for _, p := range projs {
			if strings.EqualFold(p.Status, "archived") {
				continue
			}
			snap.Projects = append(snap.Projects, planProjectBrief{
				Name: p.Name, Venture: p.Venture, Kind: p.Kind,
				Status: p.Status, NextAction: p.NextAction, DueDate: p.DueDate,
			})
		}
	}

	// Goals — active only; include up to 3 open milestone titles per
	// goal so the model can route plan lines like "ship milestone X".
	for _, g := range goals.LoadAll(s.cfg.Vault.Root) {
		st := string(g.Status)
		if st == "completed" || st == "archived" {
			continue
		}
		brief := planGoalBrief{
			ID: g.ID, Title: g.Title, Venture: g.Venture,
			Project: g.Project, TargetDate: g.TargetDate,
		}
		for _, m := range g.Milestones {
			if m.Done {
				continue
			}
			brief.Milestones = append(brief.Milestones, m.Text)
			if len(brief.Milestones) >= 3 {
				break
			}
		}
		snap.Goals = append(snap.Goals, brief)
	}

	// Open tasks — capped at 80; prioritise items with a project or
	// due date over noise. The model uses this list for two things:
	// (a) to deduplicate against existing work, (b) to understand
	// what's already in flight per venture.
	if s.cfg.TaskStore != nil {
		all := s.cfg.TaskStore.All()
		// Pre-filter to open tasks; sort by signal (has project, has
		// due date) so the cap takes the most useful 80.
		open := make([]planTaskBrief, 0, len(all))
		for _, t := range all {
			if t.Done {
				continue
			}
			open = append(open, planTaskBrief{
				ID: t.ID, Text: t.Text, Project: t.Project,
				DueDate: t.DueDate, ScheduledTime: t.ScheduledTime,
			})
		}
		sort.Slice(open, func(i, j int) bool {
			scoreI := taskSignal(open[i])
			scoreJ := taskSignal(open[j])
			return scoreI > scoreJ
		})
		if len(open) > 80 {
			open = open[:80]
		}
		snap.OpenTasks = open
	}

	// This week's calendar — Mon..Sun of the ISO week containing
	// `now`. Title + date + time is enough for the model to know
	// "Wednesday is packed". Events store keeps date/time as
	// separate fields, so we filter by date string and surface them
	// to the model in a shape the prompt can reason about.
	weekStart, weekEnd := isoWeekRange(now)
	if events, err := granitmeta.ReadEvents(s.cfg.Vault.Root); err == nil {
		startISO := weekStart.Format("2006-01-02")
		endISO := weekEnd.Format("2006-01-02")
		for _, e := range events {
			if e.Date == "" || e.Date < startISO || e.Date >= endISO {
				continue
			}
			start := e.Date
			if e.StartTime != "" {
				start += " " + e.StartTime
			}
			end := ""
			if e.EndTime != "" {
				end = e.Date + " " + e.EndTime
			}
			snap.WeekCalendar = append(snap.WeekCalendar, planEventBrief{
				Title: e.Title, Start: start, End: end,
			})
		}
		sort.Slice(snap.WeekCalendar, func(i, j int) bool {
			return snap.WeekCalendar[i].Start < snap.WeekCalendar[j].Start
		})
	}

	return snap
}

// taskSignal scores how "extraction-useful" a task is. Tasks with a
// project AND a due date are highest signal — they tell the model
// where active work lives. Tasks with neither are noise; the cap
// drops them first.
func taskSignal(t planTaskBrief) int {
	s := 0
	if t.Project != "" {
		s += 2
	}
	if t.DueDate != "" {
		s += 2
	}
	if t.ScheduledTime != "" {
		s += 1
	}
	return s
}

// isoWeekRange returns [Monday 00:00, next Monday 00:00) of the ISO
// week containing `at`. Local-time boundaries so events that say
// "Wed 14:00" land on the right calendar day regardless of UTC drift.
func isoWeekRange(at time.Time) (time.Time, time.Time) {
	// Go's weekday: Sunday=0. ISO week starts on Monday.
	wd := int(at.Weekday())
	if wd == 0 {
		wd = 7
	}
	monday := time.Date(at.Year(), at.Month(), at.Day()-(wd-1), 0, 0, 0, 0, at.Location())
	return monday, monday.AddDate(0, 0, 7)
}
