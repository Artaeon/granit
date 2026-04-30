package agents

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Options tunes the agent loop. Sensible defaults keep cost/latency
// bounded even when the LLM tries to run away with a long plan.
type Options struct {
	// MaxSteps caps the Thought→Action→Observation iterations.
	// Default 8 — enough for "search → read → synthesise" flows
	// without letting a hallucinating model loop forever.
	MaxSteps int

	// SystemPrompt is appended after the built-in tool catalog.
	// Use it to give an agent a persona ("You are a Research
	// Synthesizer..."). Empty falls back to a generic helper
	// preamble.
	SystemPrompt string

	// Approve is called before every KindWrite tool runs. Returns
	// true to permit, false to skip with an observation telling
	// the LLM the user declined. Required when the registry
	// contains any Write tools — Run returns an error otherwise
	// to fail-fast on unsafe configs.
	Approve ApproveCallback

	// OnEvent receives every state transition during the run —
	// useful for the TUI to stream the transcript live. Optional;
	// nil disables event emission. Implementations MUST be cheap
	// and non-blocking (fired synchronously from the agent
	// goroutine).
	OnEvent EventHandler
}

// ApproveCallback is invoked before a Write tool runs, with the
// tool name and rendered argument summary. Return true to permit
// the call. The runtime renders its own observation either way
// — the callback's only job is the user-facing "do you want this?"
// gate.
type ApproveCallback func(toolName string, argsSummary string) bool

// Event represents a single transition in an agent run. The TUI
// renders these as a streamed transcript; tests inspect them to
// assert on the loop's behaviour.
type Event struct {
	Step int       // 1-indexed iteration count
	Kind EventKind
	Text string    // human-readable event payload
}

// EventKind enumerates transcript event types. Order roughly
// matches the loop progression (PromptSent → ResponseReceived → ...
// → ToolResult → next iteration), but the TUI shouldn't assume any
// particular sequence.
type EventKind string

const (
	EventGoal        EventKind = "goal"        // The initial user goal
	EventThought     EventKind = "thought"     // LLM reasoning text
	EventToolCall    EventKind = "tool_call"   // Tool name + args
	EventToolResult  EventKind = "tool_result" // Tool output (truncated)
	EventDeclined    EventKind = "declined"    // Write tool denied by Approve
	EventFinalAnswer EventKind = "final"       // The agent's answer
	EventError       EventKind = "error"       // Recoverable parse/validation error
	EventBudgetHit   EventKind = "budget"      // Loop hit MaxSteps
)

// EventHandler receives one Event per transition. Implementations
// should be fast (the agent emits synchronously) and side-effect
// safe (the same event may be replayed during error recovery in
// future versions).
type EventHandler func(Event)

// Transcript is the full record of a single agent Run. The agent
// returns this after the loop terminates so callers can render the
// reasoning trail, audit tool usage, or replay for debugging.
type Transcript struct {
	Goal        string
	Steps       []Step
	FinalAnswer string
	StartedAt   time.Time
	EndedAt     time.Time
	StoppedBy   string // "answer" | "budget" | "error" | "cancelled"
}

// Step is one Thought → Action → Observation triple. Tests assert
// on these to confirm the agent took the expected route.
type Step struct {
	Number      int
	Thought     string
	ToolCall    *ToolCall   // nil when this step produced a final answer
	ToolResult  *ToolResult // nil when ToolCall was rejected
	FinalAnswer string      // non-empty when this step answered
}

// Agent runs a Goal through a ReAct loop using the registered tools
// and an LLM. Construct with New, then call Run.
type Agent struct {
	registry *Registry
	llm      LLM
	opts     Options
}

// New builds an agent. Returns an error when the configuration is
// unsafe (Write tools registered without an Approve callback) — the
// failure happens once at construction so a misconfigured caller
// gets feedback before any LLM tokens are spent.
func New(r *Registry, llm LLM, opts Options) (*Agent, error) {
	if r == nil {
		return nil, errors.New("agents: registry is required")
	}
	if llm == nil {
		return nil, errors.New("agents: llm is required")
	}
	if r.HasWriteTools() && opts.Approve == nil {
		return nil, errors.New("agents: write tools registered but Options.Approve is nil — pass an approval callback or remove the write tools")
	}
	if opts.MaxSteps <= 0 {
		opts.MaxSteps = 8
	}
	return &Agent{registry: r, llm: llm, opts: opts}, nil
}

// Run drives the ReAct loop until the LLM produces a final answer,
// the loop hits MaxSteps, or ctx is cancelled. Returns the full
// Transcript regardless of how the loop ended — callers inspect
// .StoppedBy and .FinalAnswer to decide what to render.
func (a *Agent) Run(ctx context.Context, goal string) (*Transcript, error) {
	tr := &Transcript{
		Goal:      goal,
		StartedAt: time.Now(),
	}
	a.emit(Event{Step: 0, Kind: EventGoal, Text: goal})

	systemPrompt := a.buildSystemPrompt()

	for step := 1; step <= a.opts.MaxSteps; step++ {
		if err := ctx.Err(); err != nil {
			tr.StoppedBy = "cancelled"
			tr.EndedAt = time.Now()
			return tr, err
		}

		prompt := a.buildIterationPrompt(systemPrompt, goal, tr.Steps)
		resp, err := a.llm.Complete(ctx, prompt)
		if err != nil {
			tr.StoppedBy = "error"
			tr.EndedAt = time.Now()
			return tr, fmt.Errorf("llm: %w", err)
		}
		thought, action, finalAns := parseLLMResponse(resp)
		s := Step{Number: step, Thought: thought}

		if finalAns != "" {
			s.FinalAnswer = finalAns
			tr.Steps = append(tr.Steps, s)
			tr.FinalAnswer = finalAns
			tr.StoppedBy = "answer"
			a.emit(Event{Step: step, Kind: EventThought, Text: thought})
			a.emit(Event{Step: step, Kind: EventFinalAnswer, Text: finalAns})
			tr.EndedAt = time.Now()
			return tr, nil
		}

		a.emit(Event{Step: step, Kind: EventThought, Text: thought})

		if action == nil {
			// LLM didn't produce a valid Action OR a Final Answer.
			// Tell it explicitly so the next iteration can recover.
			obsText := "Your last response did not contain a valid Action: block or a Final Answer: line. Please respond with one or the other."
			s.ToolResult = &ToolResult{Output: obsText}
			tr.Steps = append(tr.Steps, s)
			a.emit(Event{Step: step, Kind: EventError, Text: obsText})
			continue
		}

		// Validate the parsed call against the registry.
		if probs := a.registry.Validate(*action); len(probs) > 0 {
			obsText := strings.Join(probs, "; ")
			s.ToolCall = action
			s.ToolResult = &ToolResult{Output: obsText}
			tr.Steps = append(tr.Steps, s)
			a.emit(Event{Step: step, Kind: EventToolCall, Text: renderToolCall(*action)})
			a.emit(Event{Step: step, Kind: EventError, Text: obsText})
			continue
		}

		tool, _ := a.registry.ToolFor(action.Tool)
		s.ToolCall = action
		a.emit(Event{Step: step, Kind: EventToolCall, Text: renderToolCall(*action)})

		// Confirmation gate for Write tools.
		if tool.Kind() == KindWrite && a.opts.Approve != nil {
			if !a.opts.Approve(action.Tool, renderArgs(action.Args)) {
				obsText := fmt.Sprintf("User declined to run %s. Try a different approach or stop with a Final Answer.", action.Tool)
				s.ToolResult = &ToolResult{Output: obsText}
				tr.Steps = append(tr.Steps, s)
				a.emit(Event{Step: step, Kind: EventDeclined, Text: obsText})
				continue
			}
		}

		result := tool.Run(ctx, action.Args)
		s.ToolResult = &result
		tr.Steps = append(tr.Steps, s)
		if result.Err != nil {
			a.emit(Event{Step: step, Kind: EventError, Text: result.Err.Error()})
		} else {
			a.emit(Event{Step: step, Kind: EventToolResult, Text: result.Output})
		}
	}

	tr.StoppedBy = "budget"
	tr.EndedAt = time.Now()
	a.emit(Event{Step: a.opts.MaxSteps, Kind: EventBudgetHit, Text: fmt.Sprintf("Reached step limit (%d) without a Final Answer.", a.opts.MaxSteps)})
	return tr, nil
}

// emit fires an event on the configured handler if any.
func (a *Agent) emit(e Event) {
	if a.opts.OnEvent != nil {
		a.opts.OnEvent(e)
	}
}

// buildSystemPrompt assembles the persistent preamble: agent persona
// (from Options.SystemPrompt or default), plus the rendered tool
// catalog so the LLM knows what's available.
func (a *Agent) buildSystemPrompt() string {
	persona := a.opts.SystemPrompt
	if strings.TrimSpace(persona) == "" {
		persona = "You are a careful research agent. Use the tools below to gather information from the user's vault, then answer the question. Prefer 1-3 tool calls over many; stop as soon as you have enough."
	}
	return persona + "\n\n" +
		"You operate in a Thought / Action / Observation loop.\n" +
		"On each turn, write either:\n\n" +
		"  Thought: <your reasoning>\n" +
		"  Action: <tool_name>\n" +
		"  Args:\n" +
		"    arg1: value1\n" +
		"    arg2: value2\n\n" +
		"OR, when you have enough information to answer:\n\n" +
		"  Thought: <brief reasoning>\n" +
		"  Final Answer: <your answer to the user>\n\n" +
		"Tools you can call:\n\n" +
		a.registry.Describe()
}

// buildIterationPrompt is the per-step prompt: system preamble +
// goal + the running transcript of previous Thoughts / Actions /
// Observations. Keeps history grow-only; in future we can summarise
// when the prompt blows past a budget.
func (a *Agent) buildIterationPrompt(systemPrompt, goal string, steps []Step) string {
	var b strings.Builder
	b.WriteString(systemPrompt)
	b.WriteString("\n\n---\n\n")
	b.WriteString("User's question: ")
	b.WriteString(goal)
	b.WriteString("\n\n")
	if len(steps) > 0 {
		b.WriteString("Transcript so far:\n\n")
		for _, s := range steps {
			fmt.Fprintf(&b, "Thought: %s\n", strings.TrimSpace(s.Thought))
			if s.ToolCall != nil {
				fmt.Fprintf(&b, "Action: %s\n", s.ToolCall.Tool)
				if len(s.ToolCall.Args) > 0 {
					b.WriteString("Args:\n")
					for k, v := range s.ToolCall.Args {
						fmt.Fprintf(&b, "  %s: %s\n", k, v)
					}
				}
			}
			if s.ToolResult != nil {
				out := s.ToolResult.Output
				if s.ToolResult.Err != nil {
					out = "(error) " + s.ToolResult.Err.Error()
				}
				// Cap each observation in the prompt to keep
				// context bounded across multi-step runs.
				if len(out) > 2000 {
					out = out[:2000] + "\n[...truncated...]"
				}
				fmt.Fprintf(&b, "Observation: %s\n", out)
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("Now produce the next Thought + Action, OR a Thought + Final Answer.\n")
	return b.String()
}

// reArgLine matches an indented "key: value" line inside an Args
// block. Note Go's regexp (RE2) lacks lookaheads, so the rest of
// the parser is line-walking rather than regex-based.
var reArgLine = regexp.MustCompile(`^\s+(\w+):\s*(.+?)\s*$`)

// sectionPrefix returns the lowercase header tag of a line ("thought",
// "action", "args", "final answer") or "" when the line isn't a
// section header. Case-insensitive matching tolerates small-model
// inconsistencies (THOUGHT: / thought: / Thought:).
func sectionPrefix(line string) string {
	t := strings.TrimSpace(line)
	lower := strings.ToLower(t)
	switch {
	case strings.HasPrefix(lower, "thought:"):
		return "thought"
	case strings.HasPrefix(lower, "action:"):
		return "action"
	case strings.HasPrefix(lower, "args:"):
		return "args"
	case strings.HasPrefix(lower, "final answer:"):
		return "final"
	}
	return ""
}

// sectionHeaderText returns the literal header string for a section
// kind. Used by stripSectionHeader so "final" knows to strip
// "Final Answer:" not "final:".
var sectionHeaderText = map[string]string{
	"thought": "thought:",
	"action":  "action:",
	"args":    "args:",
	"final":   "final answer:",
}

// stripSectionHeader strips "Thought:" / "Action:" / "Final Answer:"
// from the beginning of a line, returning the value-portion.
// Case-insensitive on the prefix.
func stripSectionHeader(line, section string) string {
	t := strings.TrimSpace(line)
	prefix, ok := sectionHeaderText[section]
	if !ok {
		return t
	}
	if len(t) >= len(prefix) && strings.EqualFold(t[:len(prefix)], prefix) {
		return strings.TrimSpace(t[len(prefix):])
	}
	return t
}

// parseLLMResponse walks the response line by line, accumulating
// each section into its bucket. Stops capturing a multi-line
// Thought when it hits the next section. Returns (thought, action,
// finalAnswer) — one of action OR finalAnswer is non-empty on
// success; both empty when the response had neither (the runtime
// sends a recovery observation).
//
// Multi-line Thoughts are common — small models often "narrate"
// before producing the Action — and are joined with newlines.
// Final Answer is greedily multi-line: everything from "Final
// Answer:" to end-of-response is the answer.
func parseLLMResponse(resp string) (thought string, action *ToolCall, finalAnswer string) {
	lines := strings.Split(strings.TrimSpace(resp), "\n")
	var (
		thoughtBuf      []string
		finalBuf        []string
		toolName        string
		args            = map[string]string{}
		current         string
	)
	for _, line := range lines {
		if sec := sectionPrefix(line); sec != "" {
			current = sec
			value := stripSectionHeader(line, sec)
			switch sec {
			case "thought":
				if value != "" {
					thoughtBuf = append(thoughtBuf, value)
				}
			case "action":
				toolName = strings.TrimSpace(value)
			case "args":
				// Args has no inline value; the args follow on
				// the next indented lines.
			case "final":
				if value != "" {
					finalBuf = append(finalBuf, value)
				}
			}
			continue
		}
		// Continuation lines belong to whichever section we're in.
		switch current {
		case "thought":
			if t := strings.TrimSpace(line); t != "" {
				thoughtBuf = append(thoughtBuf, t)
			}
		case "args":
			if m := reArgLine.FindStringSubmatch(line); m != nil {
				// Defensive trim: the regex's `\s*$` anchor is
				// supposed to consume trailing whitespace, but
				// some Go regex/Unicode whitespace edge cases
				// leak it through. Cheap belt-and-braces — a
				// stray trailing space on a path arg makes
				// downstream string comparisons fail silently.
				args[m[1]] = strings.TrimSpace(m[2])
			}
		case "final":
			finalBuf = append(finalBuf, line)
		}
	}
	thought = strings.TrimSpace(strings.Join(thoughtBuf, "\n"))
	if len(finalBuf) > 0 {
		return thought, nil, strings.TrimSpace(strings.Join(finalBuf, "\n"))
	}
	if toolName != "" {
		return thought, &ToolCall{Tool: toolName, Args: args}, ""
	}
	return thought, nil, ""
}

// renderToolCall produces a one-line "name(arg1=v1, arg2=v2)" for
// transcript display. Used by the TUI's streamed event view and
// for the audit log.
func renderToolCall(call ToolCall) string {
	if len(call.Args) == 0 {
		return call.Tool + "()"
	}
	return call.Tool + "(" + renderArgs(call.Args) + ")"
}

// renderArgs renders args in stable key order so the same call
// always renders identically (helps test snapshots).
func renderArgs(args map[string]string) string {
	if len(args) == 0 {
		return ""
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sortStrings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s=%q", k, args[k])
	}
	return b.String()
}

// sortStrings sorts in place. Tiny helper to avoid importing sort
// just for one site (keeps the agent.go imports tight).
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
