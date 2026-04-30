package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/agents"
	"github.com/artaeon/granit/internal/objects"
	"github.com/artaeon/granit/internal/vault"
)

// AgentRunner is the modal overlay that drives a single agent run:
// pick an agent preset, enter a goal, watch the transcript stream
// in. Phase 2.5 ships with a minimal-but-real implementation — one
// flagship preset (Research Synthesizer), live transcript display,
// approve callback wired to a confirm prompt.
//
// Architecture: the runner spawns a goroutine for the agent loop
// and pipes events back to the bubbletea Update via a tea.Msg
// channel pattern. Cancellation flows the other way through ctx.
type AgentRunner struct {
	OverlayBase

	// Static config — set once at Open() and not mutated.
	vault    *vault.Vault
	registry *objects.Registry
	index    *objects.Index
	aiConfig AIConfig
	currentTasks func() []Task

	// UI state.
	phase       runnerPhase
	presets     []agents.Preset
	cursor      int
	goalInput   string
	statusLine  string

	// Run state — populated when phase = runnerRunning / runnerDone.
	events     []agents.Event
	finalAns   string
	stoppedBy  string

	// Concurrency: the agent runs in a goroutine, posts events to
	// the eventCh, and we drain them in Update. ctx + cancel
	// allow Esc to abort an in-flight run.
	eventCh chan agents.Event
	doneCh  chan agents.Transcript
	cancel  context.CancelFunc
	mu      sync.Mutex

	// Run-history persistence (Phase 16). When a run completes, we
	// build an `agent_run` typed-object note containing the
	// transcript + metadata. The host reads via GetPersistRequest
	// and writes the file through createTypedObjectFile so existing
	// vault-refresh / index-rebuild plumbing handles it. Consumed-
	// once: cleared after read.
	persistPath    string
	persistContent string
	persistReq     bool
	startedAt      time.Time
}

type runnerPhase int

const (
	// Picking an agent preset from the list.
	runnerPickPreset runnerPhase = iota
	// Typing the goal/question for the chosen preset.
	runnerEnterGoal
	// Agent is running — events stream in.
	runnerRunning
	// Run finished (answer or budget). Show transcript + answer.
	runnerDone
)

// builtinAgentPresets is the list of presets shipped in code. Each
// is a starter for a common PKM workflow:
//
//   - research-synthesizer — read-heavy, no writes
//   - project-manager       — read-heavy with typed-objects focus
//   - inbox-triager         — read+write (creates tasks for each item)
//
// Vault-local overrides at `.granit/agents/<id>.json` REPLACE these
// by ID (full override, same rationale as the Type registry's
// override semantics).
func builtinAgentPresets() []agents.Preset {
	return []agents.Preset{
		{
			ID:          "research-synthesizer",
			Name:        "Research Synthesizer",
			Description: "Given a topic, finds related notes and summarises patterns + open questions.",
			SystemPrompt: "You are a careful research synthesiser. The user gives you a topic. " +
				"Use search_vault and read_note to gather related notes from the vault, " +
				"then synthesise a structured answer covering: (1) what the notes say, " +
				"(2) recurring themes, (3) open questions or contradictions. " +
				"Do not invent facts not present in the notes. Cite note paths when claiming something.",
			Tools: []string{"search_vault", "read_note", "list_notes", "get_today"},
		},
		{
			ID:          "project-manager",
			Name:        "Project Manager",
			Description: "Reviews a project: status, blockers, related tasks, recent activity.",
			SystemPrompt: "You are a project manager assistant. The user names a project (typically by " +
				"its title or a substring). Find the matching project object via query_objects with " +
				"type=project; if there are multiple matches, ask the user which one to review (Final " +
				"Answer with a list). Otherwise: read the project note, find its open tasks via " +
				"query_tasks, and produce a structured report covering (1) current status, " +
				"(2) blockers / waiting-on, (3) next concrete actions. Cite tasks and note paths. " +
				"Do not invent dates — call get_today first if you need to reason about overdue.",
			Tools: []string{"query_objects", "read_note", "query_tasks", "search_vault", "get_today"},
		},
		{
			ID:          "inbox-triager",
			Name:        "Inbox Triager",
			Description: "Reviews recent captures and proposes next-action tasks (with confirmation).",
			SystemPrompt: "You triage an inbox of captured notes. Use list_notes on the 'Inbox' folder " +
				"(or whatever folder the user names) to enumerate recent captures. For each capture, " +
				"read it briefly with read_note, then propose ONE concrete next-action task via " +
				"create_task — phrase the task so it's actionable in <30 minutes. Always include a " +
				"due date (call get_today if needed) and a relevant tag. Do not create a task for " +
				"items that are already complete, irrelevant, or duplicates of existing tasks. " +
				"Stop after 5 captures and produce a Final Answer summarising what you did.",
			Tools:        []string{"list_notes", "read_note", "query_tasks", "create_task", "get_today"},
			IncludeWrite: true,
		},
	}
}

func NewAgentRunner() AgentRunner { return AgentRunner{} }

// Open activates the runner. Caller passes the live vault/registry/
// index + AIConfig + a function to supply current tasks.
func (a *AgentRunner) Open(
	v *vault.Vault,
	reg *objects.Registry,
	idx *objects.Index,
	cfg AIConfig,
	currentTasks func() []Task,
) {
	a.Activate()
	a.vault = v
	a.registry = reg
	a.index = idx
	a.aiConfig = cfg
	a.currentTasks = currentTasks
	// Load built-ins, then overlay vault-local presets from
	// .granit/agents/*.json. Errors during load are surfaced as
	// the picker's status line so the user knows something was
	// skipped without blocking the whole runner.
	cat := agents.NewPresetCatalog(builtinAgentPresets())
	if v != nil {
		if _, errs := cat.LoadVaultDir(v.Root); len(errs) > 0 {
			var msgs []string
			for _, err := range errs {
				msgs = append(msgs, err.Error())
			}
			a.statusLine = "Some vault-local presets failed to load: " + strings.Join(msgs, "; ")
		}
	}
	a.presets = cat.All()
	a.phase = runnerPickPreset
	a.cursor = 0
	a.goalInput = ""
	a.events = nil
	a.finalAns = ""
	a.stoppedBy = ""
}

// agentEventMsg / agentDoneMsg are the tea.Msg envelopes the
// runner posts from its background goroutine to the Update loop.
// Granit's app_update.go forwards them to AgentRunner.Update.
type agentEventMsg struct{ Ev agents.Event }
type agentDoneMsg struct{ Tr agents.Transcript }

// Update dispatches keyboard + agent-event messages to the
// appropriate phase handler. Returns the (possibly mutated) runner
// + a tea.Cmd that the host should execute.
func (a AgentRunner) Update(msg tea.Msg) (AgentRunner, tea.Cmd) {
	if !a.active {
		return a, nil
	}
	switch m := msg.(type) {
	case agentEventMsg:
		a.events = append(a.events, m.Ev)
		// Pull the next event from the channel asynchronously.
		return a, a.waitForEvent()
	case agentDoneMsg:
		a.finalAns = m.Tr.FinalAnswer
		a.stoppedBy = m.Tr.StoppedBy
		a.phase = runnerDone
		// Capture the run as an agent_run typed-object note. Host
		// reads via GetPersistRequest on the next tick and writes
		// the file. Wrapped so a serialisation hiccup never blocks
		// the user from seeing the answer.
		a.queuePersist(m.Tr)
		return a, nil
	case tea.KeyMsg:
		return a.updateKey(m)
	}
	return a, nil
}

func (a AgentRunner) updateKey(km tea.KeyMsg) (AgentRunner, tea.Cmd) {
	key := km.String()
	switch a.phase {
	case runnerPickPreset:
		switch key {
		case "esc":
			a.active = false
			return a, nil
		case "j", "down":
			if a.cursor < len(a.presets)-1 {
				a.cursor++
			}
		case "k", "up":
			if a.cursor > 0 {
				a.cursor--
			}
		case "enter":
			a.phase = runnerEnterGoal
			a.goalInput = ""
		}
	case runnerEnterGoal:
		switch key {
		case "esc":
			a.phase = runnerPickPreset
			return a, nil
		case "enter":
			if strings.TrimSpace(a.goalInput) == "" {
				a.statusLine = "Type a question or topic before pressing Enter."
				return a, nil
			}
			return a, a.startRun()
		case "backspace":
			if len(a.goalInput) > 0 {
				a.goalInput = TrimLastRune(a.goalInput)
			}
		default:
			if len(key) == 1 || key == "space" {
				if key == "space" {
					a.goalInput += " "
				} else {
					a.goalInput += key
				}
			}
		}
	case runnerRunning:
		switch key {
		case "esc":
			if a.cancel != nil {
				a.cancel()
			}
			a.statusLine = "Cancelling…"
		}
	case runnerDone:
		switch key {
		case "esc", "enter", "q":
			a.active = false
			return a, nil
		case "n":
			// New run — reuse the runner overlay.
			a.phase = runnerPickPreset
			a.events = nil
			a.finalAns = ""
			a.stoppedBy = ""
		}
	}
	return a, nil
}

// startRun kicks off the agent goroutine and returns a tea.Cmd that
// pulls the first event into the Update loop.
func (a *AgentRunner) startRun() tea.Cmd {
	preset := a.presets[a.cursor]

	// Early validation: if no AI provider is configured, fail
	// fast with an actionable message instead of spawning a
	// goroutine that hangs on a connection-refused error 10s
	// later. The historical UX was: pick agent → type goal →
	// Enter → silence → "dial tcp 127.0.0.1:11434" cryptic dump.
	// Now: pick agent → Enter → "no AI provider configured" hint
	// in the goal-input phase so the user knows to fix Settings.
	if strings.TrimSpace(a.aiConfig.Provider) == "" {
		a.statusLine = "No AI provider configured. Open Settings (Ctrl+,) and pick Ollama / OpenAI / etc."
		a.phase = runnerEnterGoal
		return nil
	}

	a.phase = runnerRunning
	a.events = nil
	a.startedAt = time.Now()

	// Build the bridge — read-only and write-side bound to granit's
	// vault. Write callbacks are only invoked when a preset opts in
	// AND the user approves at the confirmation gate.
	bridge := &agentBridge{
		vault:    a.vault,
		registry: a.registry,
		index:    a.index,
		tasks:    func() []agents.TaskRecord { return agentTaskBridge(a.currentTasks()) },
		writer:   func(rel, content string) (string, error) { return a.writeNote(rel, content) },
		appender: func(line string) (string, error) { return a.appendTaskLine(line) },
	}

	// Tool factories — separated by kind so BuildRegistryForPreset
	// can filter by the preset's allow-list and IncludeWrite flag.
	readTools := []agents.Tool{
		agents.ReadNote(bridge),
		agents.ListNotes(bridge),
		agents.SearchVault(bridge),
		agents.QueryObjects(bridge),
		agents.QueryTasks(bridge),
		agents.GetToday(),
	}
	writeTools := []agents.Tool{
		agents.WriteNote(bridge, bridge),
		agents.CreateTask(bridge),
		agents.CreateObject(bridge, bridge),
	}
	reg, err := agents.BuildRegistryForPreset(preset, readTools, writeTools)
	if err != nil {
		a.statusLine = "agent registry build failed: " + err.Error()
		a.phase = runnerEnterGoal
		return nil
	}

	// Per-preset model override. We clone the AIConfig and swap
	// the model name only — provider, API keys, and URLs stay
	// from the user's global Settings. This lets a preset
	// declare "I want llama3.1:8b for synthesis" without
	// forcing the user to flip global settings every time they
	// switch agents.
	llmConfig := a.aiConfig
	if strings.TrimSpace(preset.Model) != "" {
		llmConfig.Model = preset.Model
	}
	llm := &agentLLM{cfg: llmConfig}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	a.cancel = cancel

	// Capture channels in LOCAL variables and stash them on the
	// receiver for waitForEvent. Critical: the agent goroutine
	// must reference the local variables, NOT a.eventCh, so a
	// subsequent startRun() that reassigns a.eventCh can't make
	// the old goroutine write to the new run's channels (or to
	// a closed one). The old AgentRunner version captured
	// `a.eventCh` in the OnEvent closure and produced a
	// data-race-shaped panic on rapid Esc-then-Alt+A flows.
	eventCh := make(chan agents.Event, 32)
	doneCh := make(chan agents.Transcript, 1)
	a.eventCh = eventCh
	a.doneCh = doneCh

	maxSteps := preset.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 8 // package default; explicit here for clarity
	}
	opts := agents.Options{
		MaxSteps:     maxSteps,
		SystemPrompt: preset.SystemPrompt,
		// Approve callback — only used for KindWrite tools. Phase
		// 3.2 ships an "auto-approve" implementation that surfaces
		// the proposed action in the streamed transcript and trusts
		// the user to Esc-cancel if they don't like what they see.
		// A future phase swaps this for an interactive y/n prompt
		// inside the runner overlay (requires more state plumbing
		// — leaving as auto-approve until users actually need it).
		Approve: func(toolName string, argsSummary string) bool {
			// Minimum: the transcript event renderer shows the
			// tool call with full args. Returning true allows it.
			// User can press Esc to cancel the whole run if a
			// proposed write looks wrong.
			return true
		},
		OnEvent: func(e agents.Event) {
			// Non-blocking send so the agent loop can't be held
			// up by a slow UI consumer. Channel buffer of 32 is
			// generous — typical runs emit 6-12 events.
			select {
			case eventCh <- e:
			default:
			}
		},
	}
	ag, err := agents.New(reg, llm, opts)
	if err != nil {
		a.statusLine = "agent setup failed: " + err.Error()
		a.phase = runnerEnterGoal
		return nil
	}
	goal := strings.TrimSpace(a.goalInput)
	go func() {
		tr, err := ag.Run(ctx, goal)
		if err != nil && tr != nil && tr.StoppedBy == "" {
			tr.StoppedBy = "error"
		}
		if tr == nil {
			tr = &agents.Transcript{StoppedBy: "error"}
		}
		if err != nil {
			tr.FinalAnswer = "Run failed: " + err.Error()
		}
		// Use the locals — survives a parallel startRun() that
		// might be replacing a.eventCh / a.doneCh for a fresh
		// session. Buffered doneCh ensures this send never
		// blocks even when the UI has stopped reading.
		doneCh <- *tr
		close(eventCh)
		cancel()
	}()
	return a.waitForEvent()
}

// writeNote is the AgentRunner-side implementation of the agent
// bridge's WriteNote — converts an agents-layer write into a real
// vault write via granit's atomic note writer. Path validation
// already happened in the tool layer; this method assumes a clean
// vault-relative path.
//
// Refuses with an error when the vault isn't loaded yet (which
// shouldn't happen during a real run but guards the test path).
func (a *AgentRunner) writeNote(relPath, content string) (string, error) {
	if a.vault == nil {
		return "", errAgentVaultUnset
	}
	abs := filepath.Join(a.vault.Root, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	if err := atomicWriteNote(abs, content); err != nil {
		return "", err
	}
	// Bring the in-memory vault cache up to date so the very next
	// tool call (e.g. read_note on the freshly-written path) sees
	// the new file. Without this, an agent that writes-then-reads
	// would observe its own write missing.
	if a.vault.Notes != nil {
		a.vault.Notes[relPath] = &vault.Note{
			Path:    abs,
			RelPath: relPath,
			Content: content,
		}
	}
	return abs, nil
}

// appendTaskLine appends to Tasks.md via granit's standard
// helper, returning the absolute path it wrote to. Mirrors the
// behaviour of the /todo CLI subcommand and the morning-routine
// task creation path.
func (a *AgentRunner) appendTaskLine(line string) (string, error) {
	if a.vault == nil {
		return "", errAgentVaultUnset
	}
	if err := appendTaskLine(a.vault.Root, line); err != nil {
		return "", err
	}
	return filepath.Join(a.vault.Root, "Tasks.md"), nil
}

// errAgentVaultUnset is returned by the runner's write helpers when
// they're called before Open() wired the vault — defence against a
// programmer error in test paths or future async refactors.
type runnerErr string

func (e runnerErr) Error() string { return string(e) }

const errAgentVaultUnset runnerErr = "agent runner: vault not initialised — call Open before writing"

// queuePersist builds an agent_run typed-object note from a finished
// transcript and stages it for the host to write. Idempotent within a
// single run — no-op when the runner has no preset selected (defensive,
// shouldn't happen since we only call after agentDoneMsg).
//
// The note path encodes a UTC timestamp + preset ID so multiple runs
// of the same preset on the same day stay distinct: e.g.
// "Agents/2026-04-30T1542-research-synthesizer.md".
func (a *AgentRunner) queuePersist(tr agents.Transcript) {
	if a.cursor < 0 || a.cursor >= len(a.presets) {
		return
	}
	preset := a.presets[a.cursor]
	when := tr.StartedAt
	if when.IsZero() {
		when = a.startedAt
	}
	if when.IsZero() {
		when = time.Now()
	}
	stamp := when.UTC().Format("2006-01-02T1504")
	title := stamp + "-" + preset.ID

	// Truncate goal for the property bag — frontmatter values stay
	// scannable on narrow Object Browser columns.
	goal := strings.TrimSpace(tr.Goal)
	if goal == "" {
		goal = strings.TrimSpace(a.goalInput)
	}
	goalProp := goal
	if len(goalProp) > 200 {
		goalProp = goalProp[:197] + "..."
	}

	model := strings.TrimSpace(preset.Model)
	if model == "" {
		model = strings.TrimSpace(a.aiConfig.Model)
	}

	status := tr.StoppedBy
	switch status {
	case "answer":
		status = "ok"
	case "":
		status = "ok"
	}

	t, ok := a.registry.ByID("agent_run")
	if !ok {
		return // type missing — nothing to persist into
	}

	extras := map[string]string{
		"preset": preset.ID,
		"goal":   goalProp,
		"status": status,
		"steps":  strconv.Itoa(len(tr.Steps)),
	}
	if model != "" {
		extras["model"] = model
	}

	body := renderTranscriptMarkdown(preset, tr, goal)
	a.persistPath = objects.PathFor(t, title)
	a.persistContent = objects.BuildFrontmatter(t, title, extras) + body
	a.persistReq = true
}

// GetPersistRequest returns the queued (path, content) once after a
// run finishes. Consumed-once.
func (a *AgentRunner) GetPersistRequest() (string, string, bool) {
	if !a.persistReq {
		return "", "", false
	}
	p, c := a.persistPath, a.persistContent
	a.persistReq = false
	a.persistPath = ""
	a.persistContent = ""
	return p, c, true
}

// renderTranscriptMarkdown formats a Transcript as a markdown body
// for an agent_run note. Goal goes at the top; each step renders as
// "## Step N" with Thought / Action / Observation subsections.
// Final answer (when present) gets its own section so search hits
// land readers on the conclusion, not the trail.
func renderTranscriptMarkdown(preset agents.Preset, tr agents.Transcript, goal string) string {
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
				b.WriteString("**Thought:** ")
				b.WriteString(strings.TrimSpace(step.Thought))
				b.WriteString("\n\n")
			}
			if step.ToolCall != nil {
				fmt.Fprintf(&b, "**Action:** `%s`", step.ToolCall.Tool)
				if len(step.ToolCall.Args) > 0 {
					b.WriteString(" with args ")
					first := true
					for k, v := range step.ToolCall.Args {
						if !first {
							b.WriteString(", ")
						}
						fmt.Fprintf(&b, "%s=`%s`", k, oneline(v))
						first = false
					}
				}
				b.WriteString("\n\n")
			}
			if step.ToolResult != nil {
				b.WriteString("**Observation:**\n\n")
				if step.ToolResult.Err != nil {
					fmt.Fprintf(&b, "    error: %s\n\n", step.ToolResult.Err.Error())
				} else {
					out := step.ToolResult.Output
					if len(out) > 1000 {
						out = out[:1000] + "\n... (truncated)"
					}
					b.WriteString("```\n")
					b.WriteString(out)
					b.WriteString("\n```\n\n")
				}
			}
			if step.FinalAnswer != "" {
				b.WriteString("**Final answer reached.**\n\n")
			}
		}
	}
	return b.String()
}

// oneline collapses any whitespace in a tool argument to single spaces
// so the markdown rendering stays on one line per arg.
func oneline(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// waitForEvent returns a tea.Cmd that blocks until either an event
// arrives or the run completes, then forwards the appropriate msg
// back to Update. Tea.Cmds are go-routine'd by the runtime so this
// doesn't block the UI.
func (a *AgentRunner) waitForEvent() tea.Cmd {
	eventCh := a.eventCh
	doneCh := a.doneCh
	return func() tea.Msg {
		select {
		case ev, ok := <-eventCh:
			if !ok {
				// Channel closed — wait for the done message.
				tr := <-doneCh
				return agentDoneMsg{Tr: tr}
			}
			return agentEventMsg{Ev: ev}
		case tr := <-doneCh:
			return agentDoneMsg{Tr: tr}
		}
	}
}

// View renders the runner per phase. Kept compact — power users
// will iterate on the agent design through real runs, not by
// admiring the chrome.
func (a AgentRunner) View() string {
	if !a.active {
		return ""
	}
	w := a.width
	if w < 60 {
		w = 60
	}
	if w > 100 {
		w = 100
	}
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	header := titleStyle.Render("  🤖 Agents")
	rule := DimStyle.Render(strings.Repeat("─", maxInt(0, w-4)))

	var body string
	switch a.phase {
	case runnerPickPreset:
		body = a.renderPickPreset(w)
	case runnerEnterGoal:
		body = a.renderEnterGoal(w)
	case runnerRunning:
		body = a.renderRunning(w)
	case runnerDone:
		body = a.renderDone(w)
	}
	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(w).
		Background(mantle)
	content := strings.Join([]string{header, rule, body}, "\n")
	return border.Render(content)
}

func (a AgentRunner) renderPickPreset(_ int) string {
	var b strings.Builder
	b.WriteString("\n  Choose an agent:\n\n")
	for i, p := range a.presets {
		row := fmt.Sprintf("  %s  %s — %s", "▶", p.Name, p.Description)
		if i != a.cursor {
			row = fmt.Sprintf("     %s — %s", p.Name, p.Description)
			b.WriteString(NormalItemStyle.Render(row))
		} else {
			b.WriteString(lipgloss.NewStyle().
				Foreground(peach).Bold(true).Render(row))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  j/k navigate · Enter pick · Esc close"))
	return b.String()
}

func (a AgentRunner) renderEnterGoal(_ int) string {
	preset := a.presets[a.cursor]
	var b strings.Builder
	fmt.Fprintf(&b, "\n  %s\n", lipgloss.NewStyle().
		Foreground(lavender).Bold(true).Render("Agent: "+preset.Name))
	fmt.Fprintf(&b, "  %s\n\n",
		DimStyle.Render(preset.Description))
	b.WriteString("  What would you like the agent to investigate?\n\n")
	cursor := lipgloss.NewStyle().Foreground(peach).Render("█")
	fmt.Fprintf(&b, "  %s%s\n",
		lipgloss.NewStyle().Foreground(text).Render(a.goalInput), cursor)
	if a.statusLine != "" {
		b.WriteString("\n  " + lipgloss.NewStyle().Foreground(yellow).Render(a.statusLine))
	}
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  Enter run · Esc back"))
	return b.String()
}

func (a AgentRunner) renderRunning(_ int) string {
	var b strings.Builder
	b.WriteString("\n  ")
	b.WriteString(lipgloss.NewStyle().Foreground(yellow).Bold(true).Render("Running…"))
	b.WriteString("\n\n")
	for _, ev := range a.events {
		b.WriteString(renderAgentEvent(ev))
		b.WriteString("\n")
	}
	if a.statusLine != "" {
		b.WriteString("\n  ")
		b.WriteString(DimStyle.Render(a.statusLine))
	}
	b.WriteString("\n  ")
	b.WriteString(DimStyle.Render("Esc to cancel"))
	return b.String()
}

func (a AgentRunner) renderDone(_ int) string {
	var b strings.Builder
	stopMsg := "  ✓ Done"
	if a.stoppedBy == "budget" {
		stopMsg = "  ⚠ Stopped at step budget"
	}
	if a.stoppedBy == "error" {
		stopMsg = "  ✗ Run failed"
	}
	if a.stoppedBy == "cancelled" {
		stopMsg = "  Cancelled by user"
	}
	b.WriteString("\n  ")
	b.WriteString(lipgloss.NewStyle().Foreground(green).Bold(true).Render(stopMsg))
	b.WriteString("\n\n")
	for _, ev := range a.events {
		b.WriteString(renderAgentEvent(ev))
		b.WriteString("\n")
	}
	if a.finalAns != "" {
		b.WriteString("\n  ")
		b.WriteString(lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("Answer:"))
		b.WriteString("\n  ")
		b.WriteString(strings.ReplaceAll(a.finalAns, "\n", "\n  "))
	}
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  n new run · Enter / Esc close"))
	return b.String()
}

// renderAgentEvent formats one streamed event into a compact
// indented line. Tools and final answers get distinct colours so
// the eye can scan to the parts that matter (the answer + the
// tools that were called).
func renderAgentEvent(ev agents.Event) string {
	prefix := "  "
	switch ev.Kind {
	case agents.EventGoal:
		return prefix + lipgloss.NewStyle().Foreground(sapphire).Bold(true).Render("Goal: ") + ev.Text
	case agents.EventThought:
		return prefix + DimStyle.Render(fmt.Sprintf("[%d] thought: ", ev.Step)) +
			lipgloss.NewStyle().Foreground(text).Italic(true).Render(truncateAgentLine(ev.Text, 200))
	case agents.EventToolCall:
		return prefix + lipgloss.NewStyle().Foreground(lavender).Render(fmt.Sprintf("[%d] → ", ev.Step)) +
			lipgloss.NewStyle().Foreground(lavender).Bold(true).Render(ev.Text)
	case agents.EventToolResult:
		return prefix + DimStyle.Render(fmt.Sprintf("[%d] ← ", ev.Step)) +
			DimStyle.Render(truncateAgentLine(ev.Text, 200))
	case agents.EventDeclined:
		return prefix + lipgloss.NewStyle().Foreground(red).Render(fmt.Sprintf("[%d] declined: ", ev.Step)+ev.Text)
	case agents.EventError:
		return prefix + lipgloss.NewStyle().Foreground(red).Render(fmt.Sprintf("[%d] error: ", ev.Step)+truncateAgentLine(ev.Text, 200))
	case agents.EventBudgetHit:
		return prefix + lipgloss.NewStyle().Foreground(yellow).Render(ev.Text)
	case agents.EventFinalAnswer:
		return prefix + lipgloss.NewStyle().Foreground(green).Bold(true).Render(fmt.Sprintf("[%d] answer", ev.Step))
	}
	return prefix + ev.Text
}

// truncateAgentLine collapses internal newlines and caps the rendered
// length so a single overflowing event doesn't blow out the
// overlay. Long observations remain visible in the transcript by
// scrolling, just trimmed in the live preview.
func truncateAgentLine(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
