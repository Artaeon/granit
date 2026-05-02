package serveapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/agentruntime"
	"github.com/artaeon/granit/internal/agents"
	"github.com/artaeon/granit/internal/atomicio"
	"github.com/artaeon/granit/internal/config"
	"github.com/artaeon/granit/internal/wshub"
	"github.com/oklog/ulid/v2"
)

// runAgentBody is the POST /agents/run request shape. Goal can be empty
// for presets that get all their context from the system prompt (e.g.
// plan-my-day reads "today" without an explicit goal).
type runAgentBody struct {
	Preset string `json:"preset"`
	Goal   string `json:"goal"`
}

// handleRunAgent kicks off an agent run on the server. The request
// returns immediately with the run ID; events stream back via the
// existing WebSocket connection so the page that started the run can
// follow along live, and any other connected device sees the same
// stream (handy for "kick off plan-my-day from your phone, watch it
// finish on your laptop").
//
// Trade-off note: the run is fire-and-forget from HTTP's perspective.
// We don't keep an in-memory map of active runs the client can poll —
// the WS stream is the source of truth, and the persisted agent_run
// note is the post-run record. Keeps the server stateless.
func (s *Server) handleRunAgent(w http.ResponseWriter, r *http.Request) {
	var body runAgentBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Preset = strings.TrimSpace(body.Preset)
	body.Goal = strings.TrimSpace(body.Goal)
	if body.Preset == "" {
		writeError(w, http.StatusBadRequest, "preset required")
		return
	}

	cat := agents.NewPresetCatalog(agents.BuiltinPresets())
	_, _ = cat.LoadVaultDir(s.cfg.Vault.Root)
	preset, ok := cat.ByID(body.Preset)
	if !ok {
		writeError(w, http.StatusNotFound, "preset not found")
		return
	}

	cfg := config.LoadForVault(s.cfg.Vault.Root)
	llm, err := agentruntime.NewLLM(cfg)
	if err != nil {
		// Surface the misconfiguration as 400 not 500 — the user has
		// to fix their config.json, no fault of the server.
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	bridge := agentruntime.NewBridge(s.cfg.Vault, s.cfg.TaskStore, nil, nil)
	runner := agentruntime.New(llm, bridge)

	runID := strings.ToLower(ulid.Make().String())
	startedAt := time.Now()

	// Run on a background goroutine so the HTTP request returns now.
	// We capture the request's context only for the JSON decode; the
	// actual run uses a new ctx that survives the HTTP handler return.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		// Stream every event as a WS frame the web can render live.
		onEvent := func(ev agents.Event) {
			s.hub.Broadcast(wshub.Event{
				Type: "agent.event",
				ID:   runID,
				Data: map[string]any{
					"step": ev.Step,
					"kind": string(ev.Kind),
					"text": ev.Text,
				},
			})
		}

		tr, runErr := runner.Run(ctx, preset, body.Goal, onEvent)

		// Persist the transcript as an agent_run note so it shows up
		// in /agents the same way TUI runs do.
		if tr != nil {
			path, content := buildAgentRunNote(preset, *tr, body.Goal, startedAt, runErr, cfg)
			if path != "" {
				_ = atomicio.WriteNote(s.cfg.Vault.Root+"/"+path, content)
			}
			// Notify clients the run finished. Path lets the UI link
			// directly to the transcript note.
			final := ""
			status := "ok"
			if runErr != nil {
				status = "error"
				final = runErr.Error()
			} else {
				final = strings.TrimSpace(tr.FinalAnswer)
				switch tr.StoppedBy {
				case "budget":
					status = "budget"
				case "answer", "":
					status = "ok"
				default:
					status = tr.StoppedBy
				}
			}
			s.hub.Broadcast(wshub.Event{
				Type: "agent.complete",
				ID:   runID,
				Path: path,
				Data: map[string]any{
					"status":      status,
					"finalAnswer": final,
					"steps":       len(tr.Steps),
				},
			})
		} else if runErr != nil {
			// runner.Run returned without a transcript — most likely
			// a config error before the loop started. Surface to clients.
			s.hub.Broadcast(wshub.Event{
				Type: "agent.complete",
				ID:   runID,
				Data: map[string]any{"status": "error", "finalAnswer": runErr.Error()},
			})
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]any{
		"runId":  runID,
		"preset": preset.ID,
	})
}

// buildAgentRunNote renders a TUI-compatible agent_run note: same
// frontmatter shape, same body sections (Goal / Answer / Transcript)
// so /agents/runs picks it up identically to a run made from the TUI.
func buildAgentRunNote(preset agents.Preset, tr agents.Transcript, goal string, startedAt time.Time, runErr error, cfg config.Config) (string, string) {
	stamp := startedAt.UTC().Format("2006-01-02T1504")
	title := stamp + "-" + preset.ID
	path := "Agents/" + title + ".md"

	status := "ok"
	if runErr != nil {
		status = "error"
	} else {
		switch tr.StoppedBy {
		case "budget":
			status = "budget"
		case "answer", "":
			status = "ok"
		default:
			status = tr.StoppedBy
		}
	}

	model := cfg.OpenAIModel
	if cfg.AIProvider == "ollama" || cfg.AIProvider == "local" || cfg.AIProvider == "" {
		model = cfg.OllamaModel
	}

	goalLine := strings.TrimSpace(goal)
	if len(goalLine) > 200 {
		goalLine = goalLine[:197] + "..."
	}

	var fm strings.Builder
	fm.WriteString("---\n")
	fm.WriteString("type: agent_run\n")
	fmt.Fprintf(&fm, "title: %q\n", title)
	fmt.Fprintf(&fm, "preset: %s\n", preset.ID)
	if model != "" {
		fmt.Fprintf(&fm, "model: %s\n", model)
	}
	if goalLine != "" {
		fmt.Fprintf(&fm, "goal: %q\n", goalLine)
	}
	fmt.Fprintf(&fm, "status: %s\n", status)
	fmt.Fprintf(&fm, "started: %s\n", startedAt.Format(time.RFC3339))
	fmt.Fprintf(&fm, "steps: %d\n", len(tr.Steps))
	fm.WriteString("tags: [agent]\n")
	fm.WriteString("---\n\n")

	body := renderTranscriptBody(preset, tr, goal, runErr)
	return path, fm.String() + body
}

// renderTranscriptBody mirrors the TUI's renderTranscriptMarkdown but
// stays in the serveapi package — duplicating ~50 lines is cheaper than
// adding a tui→server import path that breaks the import direction.
func renderTranscriptBody(preset agents.Preset, tr agents.Transcript, goal string, runErr error) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s — agent run\n\n", preset.Name)
	if goal != "" {
		b.WriteString("**Goal:** ")
		b.WriteString(goal)
		b.WriteString("\n\n")
	}
	if !tr.StartedAt.IsZero() && !tr.EndedAt.IsZero() {
		fmt.Fprintf(&b, "**Duration:** %s · **Steps:** %d · **Stopped by:** %s\n\n",
			tr.EndedAt.Sub(tr.StartedAt).Round(100*time.Millisecond),
			len(tr.Steps), tr.StoppedBy)
	}
	if runErr != nil {
		fmt.Fprintf(&b, "**Error:** %s\n\n", runErr.Error())
	}
	if tr.FinalAnswer != "" {
		b.WriteString("## Answer\n\n")
		b.WriteString(strings.TrimSpace(tr.FinalAnswer))
		b.WriteString("\n\n")
	}
	if len(tr.Steps) > 0 {
		b.WriteString("## Transcript\n\n")
		for _, step := range tr.Steps {
			fmt.Fprintf(&b, "### Step %d\n\n", step.Number)
			if strings.TrimSpace(step.Thought) != "" {
				fmt.Fprintf(&b, "**Thought:** %s\n\n", strings.TrimSpace(step.Thought))
			}
			if step.ToolCall != nil {
				fmt.Fprintf(&b, "**Action:** `%s`\n\n", step.ToolCall.Tool)
			}
			if step.ToolResult != nil {
				if step.ToolResult.Err != nil {
					fmt.Fprintf(&b, "**Observation:** error: %s\n\n", step.ToolResult.Err.Error())
				} else {
					out := step.ToolResult.Output
					if len(out) > 1000 {
						out = out[:1000] + "\n... (truncated)"
					}
					fmt.Fprintf(&b, "**Observation:**\n\n```\n%s\n```\n\n", out)
				}
			}
		}
	}
	return b.String()
}
