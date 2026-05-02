package agentruntime

import (
	"context"
	"fmt"

	"github.com/artaeon/granit/internal/agents"
)

// Runner is the high-level entrypoint a caller (HTTP handler, scheduled
// job, future CLI subcommand) uses to fire a single agent run. Nothing
// stateful — Run is safe to call concurrently as long as the bridge's
// vault store can tolerate concurrent reads (granit's TaskStore + Vault
// already do via their own locks).
type Runner struct {
	llm    agents.LLM
	bridge *Bridge

	// Approve gates write-tool invocations. The TUI shows a confirm
	// modal; the web server can either auto-approve (preset's
	// IncludeWrite is opt-in metadata) or surface to the user via WS.
	// nil means "allow all" — only safe when no write-tool preset will
	// ever be run; the agents package enforces this.
	Approve agents.ApproveCallback

	// MaxSteps caps ReAct iterations. Zero falls through to the
	// agents-package default (8).
	MaxSteps int
}

// New constructs a runner. llm and bridge are required; pass an
// auto-approve func if you want write-tool presets to run unattended
// (scheduled bots, server-side plan-my-day, …).
func New(llm agents.LLM, bridge *Bridge) *Runner {
	return &Runner{llm: llm, bridge: bridge}
}

// Run executes preset against goal. Events are forwarded to onEvent (may
// be nil) so the caller can stream them to a UI in real time. The
// returned Transcript contains the full run history regardless of what
// onEvent did with the live stream.
//
// On error from the LLM or a tool, Run still returns a transcript with
// the failure recorded — callers should always persist it for forensic
// value, not branch on err being nil.
func (r *Runner) Run(ctx context.Context, preset agents.Preset, goal string, onEvent agents.EventHandler) (*agents.Transcript, error) {
	if r.llm == nil {
		return nil, fmt.Errorf("agentruntime: nil LLM")
	}
	if r.bridge == nil {
		return nil, fmt.Errorf("agentruntime: nil bridge")
	}

	registry, err := agents.BuildRegistryForPreset(preset, r.allReadTools(), r.allWriteTools())
	if err != nil {
		return nil, fmt.Errorf("build registry: %w", err)
	}

	approve := r.Approve
	if approve == nil && registry.HasWriteTools() {
		// Auto-approve when the preset opted into writes. The
		// preset's IncludeWrite flag is the user's "yes, this
		// agent may write" — re-asking on every step would be noise.
		approve = func(string, string) bool { return true }
	}

	agent, err := agents.New(registry, r.llm, agents.Options{
		MaxSteps:     r.MaxSteps,
		SystemPrompt: preset.SystemPrompt,
		Approve:      approve,
		OnEvent:      onEvent,
	})
	if err != nil {
		return nil, fmt.Errorf("new agent: %w", err)
	}
	return agent.Run(ctx, goal)
}

// allReadTools returns the full read-tool catalog wired to our bridge.
// BuildRegistryForPreset filters this down to only the tools the preset
// listed (or every read tool if Tools is empty).
func (r *Runner) allReadTools() []agents.Tool {
	return []agents.Tool{
		agents.ReadNote(r.bridge),
		agents.ListNotes(r.bridge),
		agents.SearchVault(r.bridge),
		agents.QueryObjects(r.bridge),
		agents.QueryTasks(r.bridge),
		agents.GetToday(),
	}
}

// allWriteTools returns the full write-tool catalog. Only registered
// when the preset's IncludeWrite is true (BuildRegistryForPreset
// enforces this); read-only presets never see these tools in their
// system prompt.
func (r *Runner) allWriteTools() []agents.Tool {
	return []agents.Tool{
		agents.WriteNote(r.bridge, r.bridge),
		agents.CreateTask(r.bridge),
		agents.CreateObject(r.bridge, r.bridge),
	}
}
