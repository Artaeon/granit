// Package agents implements granit's multi-step agent runtime: a
// ReAct-style execution loop where an LLM picks tools from a
// registered catalog, observes their output, and iterates until it
// produces a final answer or hits a budget.
//
// Compared to the existing single-shot bots (internal/tui/bots.go),
// agents differ along three axes:
//
//  1. Tools — agents call back into granit (read notes, query the
//     typed-objects index, search the vault, create tasks). Bots
//     are pure prompt → completion.
//
//  2. Multi-step — an agent runs a Thought / Action / Observation
//     loop until it has enough to answer. Bots are one prompt, one
//     response.
//
//  3. State — each loop iteration sees the full transcript of prior
//     steps. Bots are stateless.
//
// Architecture mirrors Anthropic's "tool use" idea but uses a plain
// text protocol (Thought:/Action:/Observation:) so it works on any
// LLM, including small local Ollama models that don't speak
// structured function-calling. A future phase can layer
// JSON-tool-calling on top for capable models.
//
// Public surface for callers (TUI overlay):
//
//   r := agents.NewRegistry()
//   r.Register(agents.ReadNote(...))         // read-only tools
//   r.Register(agents.WriteNote(..., approve)) // gated by callback
//   ag := agents.New(r, llmCaller, agents.Options{MaxSteps: 8})
//   transcript, err := ag.Run(ctx, "Find books I rated <3 and summarise patterns")
//
// Tools have stable IDs and self-describing schemas — the agent
// renders them into the system prompt so the LLM can pick correctly
// without us hardcoding tool names in templates.
package agents

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ToolKind classifies a tool by side-effect class. Read tools never
// touch disk in a way the user wouldn't expect from a search; Write
// tools may modify vault files and so must run through a
// confirmation callback in interactive mode. Misclassification is a
// programming error — we lean on the kind to gate Write at the
// runtime layer.
type ToolKind string

const (
	// KindRead tools observe vault state without modifying it. Safe
	// to call without user confirmation.
	KindRead ToolKind = "read"
	// KindWrite tools mutate disk. The runtime requires an
	// ApproveCallback for any session that runs Write tools; in
	// interactive use that surfaces a confirmation prompt, in
	// CLI/CI use it can be auto-approved.
	KindWrite ToolKind = "write"
)

// ToolParam describes a single argument a Tool accepts. The runtime
// renders these into the system prompt so the LLM knows what to
// supply, and validates the LLM's output against them before
// invoking Run.
type ToolParam struct {
	// Name is the argument key (LLM provides it as a YAML-style
	// `key: value` line in the Action block). Lowercase only;
	// snake_case for multi-word.
	Name string
	// Description is the natural-language hint shown to the LLM.
	// Keep it under ~80 chars — LLMs latch onto the first
	// sentence and ignore the rest.
	Description string
	// Required marks args the LLM MUST supply. Missing required
	// args fail validation before Run is called; the LLM gets the
	// error as an observation and can retry.
	Required bool
}

// ToolCall is the parsed Action block from the LLM's output: the
// name of the tool plus a flat string→string map of arguments.
// Higher-level tools that need richer arg shapes (lists, nested
// maps) parse the string values themselves — we keep the wire
// format flat to minimise LLM parsing failures.
type ToolCall struct {
	Tool string
	Args map[string]string
}

// ToolResult is what a Tool's Run returns. Output goes back to the
// LLM as the next Observation; Err short-circuits the run when
// non-nil and is rendered as an error observation so the LLM can
// recover (e.g. retry with a different argument).
type ToolResult struct {
	// Output is the natural-language string the LLM sees as the
	// next Observation. Should be concise — LLM context is
	// finite. For tools that return large data (note bodies,
	// search hits) the runtime truncates and tells the LLM how
	// to fetch more.
	Output string
	// Err is set when the tool genuinely failed (file not found,
	// permission denied) AND the LLM should be told. Programmer
	// errors (nil receiver, bad assertion) should panic, not
	// return Err.
	Err error
}

// Tool is the interface every agent-callable operation implements.
// Implementations live in tools_read.go / tools_write.go alongside
// each other so the read/write split is visible at file-tree level.
type Tool interface {
	// Name returns the stable identifier the LLM uses in Action
	// blocks (e.g. "read_note"). Must be lowercase snake_case
	// and unique within the registry.
	Name() string
	// Description is the natural-language summary shown to the
	// LLM in the system prompt. Keep it action-first ("Read the
	// content of a markdown note") so picking is unambiguous.
	Description() string
	// Kind controls confirmation gating. See KindRead/KindWrite.
	Kind() ToolKind
	// Params declares the arguments the tool accepts. Returned
	// list is the canonical order — used for system-prompt
	// rendering and for error messages.
	Params() []ToolParam
	// Run executes the tool. ctx carries cancellation; args have
	// already been validated against Params by the runtime, so a
	// missing-required-arg case never reaches here. Implementations
	// should respect ctx.Done() for any I/O that could block.
	Run(ctx context.Context, args map[string]string) ToolResult
}

// Registry holds the active set of tools available to an agent. A
// fresh registry is empty — caller is expected to install tools
// explicitly with Register, so a session that shouldn't touch the
// disk doesn't accidentally have write tools registered.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry returns an empty registry. Tools are installed via
// Register; ToolFor / All / Describe are read-only afterward.
func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}}
}

// Register installs a tool by Name(). Returns an error on duplicate
// registration so two different tools don't silently collide on the
// same name (a confusing class of bug — the LLM would call one but
// the other would run).
func (r *Registry) Register(t Tool) error {
	if t == nil {
		return errors.New("agents: cannot register nil tool")
	}
	name := t.Name()
	if name == "" {
		return errors.New("agents: tool name is required")
	}
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("agents: tool %q already registered", name)
	}
	r.tools[name] = t
	return nil
}

// MustRegister wraps Register and panics on error. Used in tests
// and in code paths where a duplicate registration is a programmer
// error (e.g. construction of the default registry from built-in
// tool factory functions).
func (r *Registry) MustRegister(t Tool) {
	if err := r.Register(t); err != nil {
		panic(err)
	}
}

// ToolFor returns the tool with the given name, or (nil, false) when
// no such tool exists. Used by the runtime when parsing an Action
// block — an unknown name surfaces to the LLM as an observation
// listing the valid tool names so it can correct itself.
func (r *Registry) ToolFor(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// All returns every registered tool sorted by Name() for stable
// rendering in the system prompt.
func (r *Registry) All() []Tool {
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// Names returns just the registered tool names sorted alphabetically.
// Used for the "available tools: a, b, c" line in error
// observations when the LLM picks a non-existent tool.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.tools))
	for name := range r.tools {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// HasWriteTools reports whether the registry contains any KindWrite
// tools. The runtime checks this once at session start and rejects
// the session if write tools are registered without an
// ApproveCallback — fail-fast on the contract instead of letting a
// bad config silently grant the LLM full disk access.
func (r *Registry) HasWriteTools() bool {
	for _, t := range r.tools {
		if t.Kind() == KindWrite {
			return true
		}
	}
	return false
}

// Describe renders the registry into the natural-language tool block
// that goes into the system prompt. Format chosen to be parseable by
// small Ollama models (which routinely fail at JSON-Schema-style
// descriptions): one tool per paragraph, plain headings.
//
//	## TOOL_NAME — kind
//	Description.
//	Args:
//	  arg1 (required) — description
//	  arg2 — description
func (r *Registry) Describe() string {
	if len(r.tools) == 0 {
		return "(no tools available)"
	}
	var b strings.Builder
	for i, t := range r.All() {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "## %s — %s\n%s", t.Name(), t.Kind(), t.Description())
		params := t.Params()
		if len(params) > 0 {
			b.WriteString("\nArgs:")
			for _, p := range params {
				req := ""
				if p.Required {
					req = " (required)"
				}
				fmt.Fprintf(&b, "\n  %s%s — %s", p.Name, req, p.Description)
			}
		} else {
			b.WriteString("\nArgs: (none)")
		}
	}
	return b.String()
}

// Validate confirms a ToolCall references a known tool and supplies
// every required argument. Returns a list of human-readable error
// strings (one per problem) so the LLM gets a single observation
// describing all the issues to fix. Empty return = valid.
func (r *Registry) Validate(call ToolCall) []string {
	var problems []string
	tool, ok := r.tools[call.Tool]
	if !ok {
		return []string{
			fmt.Sprintf("unknown tool %q. Available: %s",
				call.Tool, strings.Join(r.Names(), ", ")),
		}
	}
	for _, p := range tool.Params() {
		if !p.Required {
			continue
		}
		v, present := call.Args[p.Name]
		if !present || strings.TrimSpace(v) == "" {
			problems = append(problems,
				fmt.Sprintf("missing required arg %q for tool %s", p.Name, call.Tool))
		}
	}
	return problems
}
