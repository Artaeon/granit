package agentruntime

import (
	"context"
	"fmt"
	"sync"

	"github.com/artaeon/granit/internal/agents"
)

// Runner is the high-level entrypoint a caller (HTTP handler, scheduled
// job, future CLI subcommand) uses to fire a single agent run. Nothing
// stateful between Run calls — concurrent calls each get their own
// CostTracker so usage is attributed correctly.
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
	// agents-package default (8). Higher values let deep-research
	// presets explore more thoroughly at higher cost.
	MaxSteps int

	// BudgetMicroCents caps cumulative cost across the run in
	// micro-cents (1/1_000_000 of a cent). Zero means "no budget" —
	// MaxSteps is the only ceiling. When exceeded mid-run, the
	// runtime cancels the context so the agent stops at its next
	// iteration boundary, and the transcript records "stopped by
	// budget".
	//
	// Only enforced when the LLM implements Metered (OpenAI does;
	// Ollama doesn't, since it's free). Without metering this field
	// is silently ignored.
	BudgetMicroCents int64
}

// CostTracker accumulates usage across an agent run. Returned from
// Run so the caller can render a final cost line in the transcript
// note + the WS completion frame. Thread-safe even though typical
// runs are single-goroutine — the OnEvent callback may be invoked
// re-entrantly in some agent-loop bug scenarios, and we'd rather not
// race.
type CostTracker struct {
	mu               sync.Mutex
	PromptTokens     int
	CompletionTokens int
	MicroCents       int64 // -1 means "no pricing for the model"
	Model            string
	// BudgetHit flips true the moment the runtime cancels the run for
	// hitting BudgetMicroCents. Callers read it after Run returns to
	// distinguish a budget stop ("status=budget", warning UI) from a
	// generic context.Canceled (which would otherwise look like a real
	// error to the HTTP layer). Snapshot copies it so it survives a
	// later read.
	BudgetHit bool
}

func (c *CostTracker) record(u Usage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.PromptTokens += u.PromptTokens
	c.CompletionTokens += u.CompletionTokens
	c.Model = u.Model
	if cents := CostMicroCents(u); cents >= 0 {
		// First call sets the base; subsequent calls add. -1
		// (unknown model) doesn't poison the running total — we
		// just accumulate the calls we can price.
		if c.MicroCents < 0 {
			c.MicroCents = 0
		}
		c.MicroCents += cents
	}
}

// Snapshot returns a copy under the lock so the caller can read it
// safely while the run is still going. The Cost field is the running
// total in micro-cents; -1 when the model isn't priced.
func (c *CostTracker) Snapshot() CostTracker {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CostTracker{
		PromptTokens:     c.PromptTokens,
		CompletionTokens: c.CompletionTokens,
		MicroCents:       c.MicroCents,
		Model:            c.Model,
		BudgetHit:        c.BudgetHit,
	}
}

func (c *CostTracker) markBudgetHit() {
	c.mu.Lock()
	c.BudgetHit = true
	c.mu.Unlock()
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
// Budget enforcement: after every LLM call (which the agent emits as
// a ResponseReceived event), the runtime polls Metered.LastUsage(),
// accumulates cost into the CostTracker, and if BudgetMicroCents > 0
// and the running total exceeds it, cancels the run's context. The
// agent stops cleanly at the next iteration boundary; the transcript
// records the budget hit.
func (r *Runner) Run(ctx context.Context, preset agents.Preset, goal string, onEvent agents.EventHandler) (*agents.Transcript, *CostTracker, error) {
	if r.llm == nil {
		return nil, nil, fmt.Errorf("agentruntime: nil LLM")
	}
	if r.bridge == nil {
		return nil, nil, fmt.Errorf("agentruntime: nil bridge")
	}

	registry, err := agents.BuildRegistryForPreset(preset, r.allReadTools(), r.allWriteTools())
	if err != nil {
		return nil, nil, fmt.Errorf("build registry: %w", err)
	}

	approve := r.Approve
	if approve == nil && registry.HasWriteTools() {
		// Auto-approve when the preset opted into writes. The
		// preset's IncludeWrite flag is the user's "yes, this
		// agent may write" — re-asking on every step would be noise.
		approve = func(string, string) bool { return true }
	}

	tracker := &CostTracker{MicroCents: -1}
	metered, _ := r.llm.(Metered)

	// Wrap the run context so the budget gate can cancel mid-loop.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// budgetEmitted flips once we surface the synthetic "budget exceeded"
	// event. Without it, every in-flight Thought arriving after we cancel
	// (but before the agent loop notices ctx.Done()) would re-evaluate the
	// over-budget condition and re-emit the same error frame to the WS
	// stream — clients would see the message N times.
	var budgetEmitted bool

	wrappedEvent := func(ev agents.Event) {
		// Attribute cost on every Thought event. The agent's loop is
		// "send prompt → get response → parse Thought + Action → run
		// tool → next iteration", so a Thought arriving means a model
		// call just completed. We poll the LLM's LastUsage right
		// after — the metered impl updated lastUsage before returning
		// from Complete, so the values are fresh.
		if metered != nil && ev.Kind == agents.EventThought {
			u := metered.LastUsage()
			if u.PromptTokens > 0 || u.CompletionTokens > 0 {
				tracker.record(u)
				snap := tracker.Snapshot()
				if r.BudgetMicroCents > 0 && snap.MicroCents > r.BudgetMicroCents && !budgetEmitted {
					// Budget exceeded: cancel the context so the
					// agent stops on its next iteration boundary.
					// Surface ONE synthetic error event — subsequent
					// Thoughts queued before cancel-propagation are
					// suppressed by the budgetEmitted flag.
					budgetEmitted = true
					tracker.markBudgetHit()
					cancel()
					if onEvent != nil {
						onEvent(agents.Event{
							Step: ev.Step,
							Kind: agents.EventError,
							Text: fmt.Sprintf("budget exceeded (%s spent, %s limit) — stopping",
								FormatCents(snap.MicroCents), FormatCents(r.BudgetMicroCents)),
						})
					}
					return
				}
			}
		}
		if onEvent != nil {
			onEvent(ev)
		}
	}

	// Resolve the effective step cap. Precedence: explicit Runner override
	// > preset hint > agents-package default (8). The preset hint matters
	// for callers (e.g. scheduled jobs, raw API consumers) that don't know
	// to ask for more steps on a research-style preset; without this the
	// preset's MaxSteps field would be silently ignored.
	maxSteps := r.MaxSteps
	if maxSteps == 0 {
		maxSteps = preset.MaxSteps
	}

	agent, err := agents.New(registry, r.llm, agents.Options{
		MaxSteps:     maxSteps,
		SystemPrompt: preset.SystemPrompt,
		Approve:      approve,
		OnEvent:      wrappedEvent,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("new agent: %w", err)
	}
	tr, err := agent.Run(runCtx, goal)
	return tr, tracker, err
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
