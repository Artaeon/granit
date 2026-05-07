package serveapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/agentruntime"
	"github.com/artaeon/granit/internal/aiaudit"
	"github.com/artaeon/granit/internal/aicontext"
	"github.com/artaeon/granit/internal/aiprefs"
	"github.com/artaeon/granit/internal/airedact"
	"github.com/artaeon/granit/internal/config"
)

// runAIFeature is the shared pipeline every Tier 1 AI feature runs
// through. Centralised so the four-step ritual (consent check →
// build snapshot → redact → chat → audit) stays uniform across
// every new feature granit grows. Features that need a different
// shape (e.g. streaming, tool calls) will diverge — but every
// non-streaming "snapshot in, markdown out" feature lives here.
//
// Returns the assistant's reply text or an error. Audit log
// recorded as a side effect; redaction applied per prefs.
func (s *Server) runAIFeature(ctx context.Context, feature aiprefs.Feature, systemPrompt, userPrompt string) (string, error) {
	prefs, _ := aiprefs.Load(s.cfg.Vault.Root)
	cfg, ok := prefs.Features[feature]
	if !ok || !cfg.Enabled {
		return "", fmt.Errorf("feature %q is disabled in AI preferences", feature)
	}
	// Apply redaction if enabled. We keep stats so the audit log
	// surfaces "12 emails + 3 phones redacted" without storing the
	// originals.
	finalPrompt := userPrompt
	var stats []airedact.Stat
	if prefs.RedactionEnabled {
		finalPrompt, stats = airedact.RedactWithStats(userPrompt, airedact.DefaultRules())
	}
	cfgFile := config.LoadForVault(s.cfg.Vault.Root)
	llm, err := agentruntime.NewLLM(cfgFile)
	if err != nil {
		return "", err
	}
	if hint := preflightLLM(llm); hint != "" {
		return "", fmt.Errorf("%s", hint)
	}
	chatter, ok := llm.(agentruntime.Chatter)
	if !ok {
		return "", fmt.Errorf("configured LLM does not support chat")
	}
	messages := []agentruntime.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: finalPrompt},
	}
	out, err := chatter.Chat(ctx, messages)
	// Audit. Error or success, log.
	if s.aiAudit != nil {
		entry := aiaudit.Entry{
			Feature:           string(feature),
			Provider:          cfgFile.AIProvider,
			ResponseSizeBytes: len(out),
		}
		if err != nil {
			entry.Error = err.Error()
		}
		// Convert redaction stats to the audit shape.
		if len(stats) > 0 {
			entry.RedactionsByRule = make([]aiaudit.Stat, len(stats))
			for i, s := range stats {
				entry.RedactionsByRule[i] = aiaudit.Stat{Name: s.Name, Count: s.Count}
			}
		}
		_, _ = s.aiAudit.Append(entry, finalPrompt)
	}
	return out, err
}

// ─── Daily Briefing ───────────────────────────────────────────────

const dailyBriefingSystemPrompt = `You are the user's personal daily briefer in Granit, a single-tenant knowledge / calendar / tasks tool.
You will receive a JSON snapshot describing today: events, urgent open tasks, recent notes, active goals, upcoming deadlines.
Write a short, direct morning briefing in markdown:
  - Open with one sentence framing the day (busy / open / focused, etc.)
  - List today's events compactly (HH:MM · Title)
  - List the 3-5 most important tasks, with one-line rationale each
  - Surface ONE deadline only if it's within 7 days
  - End with a single short sentence of encouragement, without saccharine
Keep total length under 200 words. No headers above level 2. Plain markdown the user can paste into their daily note.`

func (s *Server) handleAIDailyBriefing(w http.ResponseWriter, r *http.Request) {
	snap := s.aiContext.Build(aicontext.BuildOpts{})
	body, _ := json.Marshal(snap)
	out, err := s.runAIFeature(r.Context(), aiprefs.FeatureDailyBriefing,
		dailyBriefingSystemPrompt,
		"Today's snapshot:\n\n```json\n"+string(body)+"\n```")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"markdown": out})
}

// ─── Weekly Review Draft ──────────────────────────────────────────

const weeklyReviewSystemPrompt = `You are drafting a weekly review for the user in Granit.
You will receive a JSON snapshot of recent notes, open tasks, active goals, deadlines, and the recent dailies.
Write a balanced markdown draft using these section headings:
  ## Wins
  ## Setbacks
  ## What I learned
  ## Next week's one thing
Three to five bullets per section. Honest, specific, no padding. The user will edit before saving.
Total length under 400 words. Plain markdown.`

func (s *Server) handleAIWeeklyReview(w http.ResponseWriter, r *http.Request) {
	snap := s.aiContext.Build(aicontext.BuildOpts{
		MaxRecentNotes: 25,
		DailyHistory:   7,
	})
	body, _ := json.Marshal(snap)
	out, err := s.runAIFeature(r.Context(), aiprefs.FeatureWeeklyReview,
		weeklyReviewSystemPrompt,
		"Snapshot for this week:\n\n```json\n"+string(body)+"\n```")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"markdown": out})
}

// ─── Inbox Triage ─────────────────────────────────────────────────

const inboxTriageSystemPrompt = `You will receive a JSON list of untriaged tasks.
For EACH task, suggest:
  - priority: 1 (urgent + important) | 2 (important) | 3 (later) | 0 (drop)
  - schedule: "today" | "tomorrow" | "this_week" | "next_week" | "no_date"
  - one-sentence rationale (under 15 words)
Return a JSON array (NOT prose, NOT markdown — bare JSON), shape:
[{"id": "<task id>", "priority": 2, "schedule": "this_week", "rationale": "..."}, ...]
Be decisive. Drop tasks that look stale or duplicative. Keep schedule reasonable.`

type triageProposal struct {
	ID        string `json:"id"`
	Priority  int    `json:"priority"`
	Schedule  string `json:"schedule"`
	Rationale string `json:"rationale"`
}

func (s *Server) handleAIInboxTriage(w http.ResponseWriter, r *http.Request) {
	if s.cfg.TaskStore == nil {
		writeError(w, http.StatusInternalServerError, "task store not configured")
		return
	}
	all := s.cfg.TaskStore.All()
	type seed struct {
		ID       string   `json:"id"`
		Title    string   `json:"title"`
		DueDate  string   `json:"due_date,omitempty"`
		Priority int      `json:"priority,omitempty"`
		Tags     []string `json:"tags,omitempty"`
		NotePath string   `json:"note_path,omitempty"`
	}
	now := time.Now()
	seeds := make([]seed, 0)
	for _, t := range all {
		if t.Done {
			continue
		}
		// Untriaged = inbox triage state OR no triage state set
		if t.Triage != "" && t.Triage != "inbox" {
			continue
		}
		if t.SnoozedUntil != "" {
			if su, err := time.Parse(time.RFC3339, t.SnoozedUntil); err == nil && su.After(now) {
				continue
			}
		}
		seeds = append(seeds, seed{
			ID: t.ID, Title: t.Text, DueDate: t.DueDate,
			Priority: t.Priority, Tags: t.Tags, NotePath: t.NotePath,
		})
		if len(seeds) >= 30 {
			break // cap so the prompt + response stay bounded
		}
	}
	if len(seeds) == 0 {
		writeJSON(w, http.StatusOK, map[string]interface{}{"proposals": []triageProposal{}})
		return
	}
	body, _ := json.Marshal(seeds)
	out, err := s.runAIFeature(r.Context(), aiprefs.FeatureInboxTriage,
		inboxTriageSystemPrompt,
		"Untriaged tasks:\n\n```json\n"+string(body)+"\n```")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Try to parse the response as JSON. Models occasionally wrap
	// in fences; strip them defensively. If parsing fails, return
	// the raw text so the UI can show the user what came back.
	cleaned := strings.TrimSpace(out)
	if strings.HasPrefix(cleaned, "```") {
		// Drop opening fence + optional language tag.
		cleaned = strings.TrimPrefix(cleaned, "```json")
		cleaned = strings.TrimPrefix(cleaned, "```")
		cleaned = strings.TrimSuffix(cleaned, "```")
		cleaned = strings.TrimSpace(cleaned)
	}
	var proposals []triageProposal
	if err := json.Unmarshal([]byte(cleaned), &proposals); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"proposals": []triageProposal{},
			"raw":       out,
			"warning":   "Model didn't return parseable JSON; showing raw response.",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"proposals": proposals})
}

// ─── Deadline Detect ──────────────────────────────────────────────
//
// Sister feature to inbox-triage. Triage proposes "this_week / next_week"
// scheduling buckets; deadline-detect goes a step further and proposes a
// HARD due_date for tasks whose title or note carries a clear deadline
// signal ("by Friday", "before launch", "submit by 2026-06-01"). Tasks
// without a clear signal return no_signal so the user isn't pressured
// into committing to an artificial date.

const deadlineDetectSystemPrompt = `You will receive a JSON list of open tasks that have NO due_date set.
For EACH task, decide whether the title (or context if shown) implies a clear deadline.
Return a JSON array (NOT prose) of proposals:
[{"id": "<task id>", "due_date": "YYYY-MM-DD" | "", "rationale": "..."}]
Rules:
  - Use today's date "%s" as the reference for relative phrases (e.g. "Friday", "next week").
  - "due_date": "" means NO clear signal — do not invent dates. Honest "no signal" is better than guessing.
  - Strong signals: explicit dates, "by/before/until <day>", deadline-shaped verbs ("submit", "file", "renew").
  - Weak signals (e.g. "soon", "asap"): leave due_date blank.
  - Rationale: under 12 words, name the phrase that triggered the date.
Be conservative. The user trusts proposals; spurious dates erode that trust.`

type deadlineProposal struct {
	ID        string `json:"id"`
	DueDate   string `json:"due_date"`
	Rationale string `json:"rationale"`
}

func (s *Server) handleAIDeadlineDetect(w http.ResponseWriter, r *http.Request) {
	if s.cfg.TaskStore == nil {
		writeError(w, http.StatusInternalServerError, "task store not configured")
		return
	}
	all := s.cfg.TaskStore.All()
	type seed struct {
		ID       string   `json:"id"`
		Title    string   `json:"title"`
		Priority int      `json:"priority,omitempty"`
		Tags     []string `json:"tags,omitempty"`
		NotePath string   `json:"note_path,omitempty"`
	}
	seeds := make([]seed, 0)
	for _, t := range all {
		if t.Done || t.DueDate != "" {
			continue
		}
		seeds = append(seeds, seed{
			ID: t.ID, Title: t.Text, Priority: t.Priority,
			Tags: t.Tags, NotePath: t.NotePath,
		})
		if len(seeds) >= 30 {
			break
		}
	}
	if len(seeds) == 0 {
		writeJSON(w, http.StatusOK, map[string]interface{}{"proposals": []deadlineProposal{}})
		return
	}
	body, _ := json.Marshal(seeds)
	today := time.Now().Format("2006-01-02")
	out, err := s.runAIFeature(r.Context(), aiprefs.FeatureDeadlineDetect,
		fmt.Sprintf(deadlineDetectSystemPrompt, today),
		"Open tasks without due dates:\n\n```json\n"+string(body)+"\n```")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cleaned := strings.TrimSpace(out)
	if strings.HasPrefix(cleaned, "```") {
		cleaned = strings.TrimPrefix(cleaned, "```json")
		cleaned = strings.TrimPrefix(cleaned, "```")
		cleaned = strings.TrimSuffix(cleaned, "```")
		cleaned = strings.TrimSpace(cleaned)
	}
	var proposals []deadlineProposal
	if err := json.Unmarshal([]byte(cleaned), &proposals); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"proposals": []deadlineProposal{},
			"raw":       out,
			"warning":   "Model didn't return parseable JSON; showing raw response.",
		})
		return
	}
	// Filter out blanks before returning so the UI doesn't render a
	// dozen "no signal" rows. The model is told to leave blanks for
	// no-signal cases — we honor that here rather than UI-side.
	kept := proposals[:0]
	for _, p := range proposals {
		if p.DueDate == "" {
			continue
		}
		kept = append(kept, p)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"proposals": kept})
}
