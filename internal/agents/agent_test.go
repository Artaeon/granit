package agents

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// New rejects nil dependencies and unsafe configurations (write
// tools without an approve callback). Each branch is a contract we
// don't want quietly relaxing.
func TestNew_RejectsBadConfig(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(&stubTool{name: "rw", kind: KindWrite})

	if _, err := New(nil, &MockLLM{}, Options{}); err == nil {
		t.Error("nil registry should error")
	}
	if _, err := New(NewRegistry(), nil, Options{}); err == nil {
		t.Error("nil llm should error")
	}
	if _, err := New(r, &MockLLM{}, Options{}); err == nil {
		t.Error("write tools without Approve should error")
	}
	// Same registry + Approve is OK.
	if _, err := New(r, &MockLLM{}, Options{Approve: func(string, string) bool { return true }}); err != nil {
		t.Errorf("write tools + Approve should succeed: %v", err)
	}
}

// New defaults MaxSteps when the caller didn't set it. Without
// this an Options{} would lock the agent to zero iterations and
// it'd never run anything.
func TestNew_DefaultsMaxSteps(t *testing.T) {
	r := NewRegistry()
	a, err := New(r, &MockLLM{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if a.opts.MaxSteps != 8 {
		t.Errorf("default MaxSteps: got %d, want 8", a.opts.MaxSteps)
	}
}

// parseLLMResponse extracts Thought / Action / Args / Final Answer
// from the messy, case-insensitive output small models produce.
// Each case here is a pattern observed in practice with Ollama 0.5b
// — the parser must not be brittle to them.
func TestParseLLMResponse(t *testing.T) {
	cases := []struct {
		name        string
		resp        string
		wantThought string
		wantTool    string
		wantArgs    map[string]string
		wantFinal   string
	}{
		{
			name: "tool call with args",
			resp: `Thought: I should look up Alice.
Action: read_note
Args:
  path: People/Alice.md
  max_chars: 4000`,
			wantThought: "I should look up Alice.",
			wantTool:    "read_note",
			wantArgs:    map[string]string{"path": "People/Alice.md", "max_chars": "4000"},
		},
		{
			name: "final answer",
			resp: `Thought: I have enough to answer.
Final Answer: Alice's email is alice@example.com.`,
			wantThought: "I have enough to answer.",
			wantFinal:   "Alice's email is alice@example.com.",
		},
		{
			name:        "lowercase keywords",
			resp:        "thought: hi\naction: get_today",
			wantThought: "hi",
			wantTool:    "get_today",
			wantArgs:    map[string]string{},
		},
		{
			name: "tool call no args",
			resp: `Thought: get the date
Action: get_today`,
			wantThought: "get the date",
			wantTool:    "get_today",
			wantArgs:    map[string]string{},
		},
		{
			name:        "neither action nor final",
			resp:        "Thought: hmm I'm not sure.",
			wantThought: "hmm I'm not sure.",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			thought, action, final := parseLLMResponse(c.resp)
			if thought != c.wantThought {
				t.Errorf("thought: got %q, want %q", thought, c.wantThought)
			}
			if final != c.wantFinal {
				t.Errorf("final: got %q, want %q", final, c.wantFinal)
			}
			if c.wantTool == "" {
				if action != nil {
					t.Errorf("expected no action, got %+v", action)
				}
				return
			}
			if action == nil {
				t.Fatalf("expected action with tool %q, got nil", c.wantTool)
			}
			if action.Tool != c.wantTool {
				t.Errorf("tool: got %q, want %q", action.Tool, c.wantTool)
			}
			for k, v := range c.wantArgs {
				if action.Args[k] != v {
					t.Errorf("arg %s: got %q, want %q", k, action.Args[k], v)
				}
			}
		})
	}
}

// renderToolCall formats calls in stable key order so transcripts
// diff cleanly across runs.
func TestRenderToolCall_StableOrder(t *testing.T) {
	c := ToolCall{Tool: "x", Args: map[string]string{"b": "2", "a": "1", "c": "3"}}
	got := renderToolCall(c)
	want := `x(a="1", b="2", c="3")`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Run with a tool call followed by a final answer — the canonical
// happy path. Both the transcript and the streamed events should
// reflect the two-step shape.
func TestAgent_Run_HappyPath(t *testing.T) {
	r := NewRegistry()
	tool := &stubTool{name: "search", kind: KindRead, output: "found 1 hit: alpha.md"}
	r.MustRegister(tool)
	llm := &MockLLM{Responses: []string{
		"Thought: search for the topic\nAction: search\nArgs:\n  query: alpha",
		"Thought: I have what I need.\nFinal Answer: The note is alpha.md.",
	}}
	var events []Event
	a, err := New(r, llm, Options{
		MaxSteps: 4,
		OnEvent:  func(e Event) { events = append(events, e) },
	})
	if err != nil {
		t.Fatal(err)
	}
	tr, err := a.Run(context.Background(), "Find the alpha topic")
	if err != nil {
		t.Fatal(err)
	}
	if tr.StoppedBy != "answer" {
		t.Errorf("StoppedBy: got %q, want answer", tr.StoppedBy)
	}
	if !strings.Contains(tr.FinalAnswer, "alpha.md") {
		t.Errorf("FinalAnswer: %q", tr.FinalAnswer)
	}
	if len(tr.Steps) != 2 {
		t.Errorf("Steps: got %d, want 2", len(tr.Steps))
	}
	if !tool.called {
		t.Error("search tool should have been called")
	}
	// Events: goal, thought, tool_call, tool_result, thought, final.
	wantKinds := []EventKind{EventGoal, EventThought, EventToolCall, EventToolResult, EventThought, EventFinalAnswer}
	if len(events) != len(wantKinds) {
		t.Fatalf("event count: got %d, want %d (events=%+v)", len(events), len(wantKinds), events)
	}
	for i, want := range wantKinds {
		if events[i].Kind != want {
			t.Errorf("event[%d]: got %q, want %q", i, events[i].Kind, want)
		}
	}
}

// Validation failures (unknown tool, missing required arg) become
// observations rather than aborting the run — the LLM gets a chance
// to retry.
func TestAgent_Run_RecoversFromBadAction(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(&stubTool{name: "search", kind: KindRead, output: "ok"})
	llm := &MockLLM{Responses: []string{
		"Thought: try a wrong tool\nAction: nonsense",
		"Thought: ok use the right one\nAction: search\nArgs:\n  query: x",
		"Thought: done\nFinal Answer: handled",
	}}
	a, _ := New(r, llm, Options{MaxSteps: 5})
	tr, err := a.Run(context.Background(), "anything")
	if err != nil {
		t.Fatal(err)
	}
	if tr.StoppedBy != "answer" {
		t.Errorf("StoppedBy: got %q", tr.StoppedBy)
	}
	if len(tr.Steps) != 3 {
		t.Errorf("Steps: got %d, want 3", len(tr.Steps))
	}
	// First step had a validation error in its observation.
	if tr.Steps[0].ToolResult == nil ||
		!strings.Contains(tr.Steps[0].ToolResult.Output, "unknown tool") {
		t.Errorf("expected unknown-tool observation, got %+v", tr.Steps[0].ToolResult)
	}
}

// MaxSteps caps the loop. The transcript records StoppedBy="budget"
// and a budget-hit event fires.
func TestAgent_Run_BudgetCap(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(&stubTool{name: "noop", kind: KindRead, output: "tick"})
	// LLM never produces a Final Answer — keeps calling the tool.
	llm := &MockLLM{Responses: []string{
		"Thought: 1\nAction: noop",
		"Thought: 2\nAction: noop",
		"Thought: 3\nAction: noop",
		"Thought: 4\nAction: noop",
	}}
	var hitBudget bool
	a, _ := New(r, llm, Options{
		MaxSteps: 3,
		OnEvent: func(e Event) {
			if e.Kind == EventBudgetHit {
				hitBudget = true
			}
		},
	})
	tr, err := a.Run(context.Background(), "loop forever")
	if err != nil {
		t.Fatal(err)
	}
	if tr.StoppedBy != "budget" {
		t.Errorf("StoppedBy: got %q, want budget", tr.StoppedBy)
	}
	if len(tr.Steps) != 3 {
		t.Errorf("Steps: got %d, want 3", len(tr.Steps))
	}
	if !hitBudget {
		t.Error("expected budget event to fire")
	}
}

// Write tools route through the Approve callback. A decline becomes
// an observation; the agent can recover and try a different
// approach.
func TestAgent_Run_WriteToolApproveGate(t *testing.T) {
	r := NewRegistry()
	wrote := 0
	r.MustRegister(&stubTool{
		name: "save_note", kind: KindWrite,
		params: []ToolParam{{Name: "path", Required: true}},
		output: "ok",
	})
	// The stub records calls via .called; we want a separate
	// counter to distinguish approved-and-ran from approved-and-
	// declined. Simpler: read tool's "called" field on the
	// underlying instance after the run.
	llm := &MockLLM{Responses: []string{
		"Thought: save it\nAction: save_note\nArgs:\n  path: x.md",
		"Thought: declined, skip and answer\nFinal Answer: Done.",
	}}
	a, _ := New(r, llm, Options{
		MaxSteps: 4,
		Approve:  func(_ string, _ string) bool { wrote++; return false },
	})
	tr, err := a.Run(context.Background(), "save x")
	if err != nil {
		t.Fatal(err)
	}
	if wrote != 1 {
		t.Errorf("Approve callback should fire once, got %d", wrote)
	}
	// Step 1 should have a "user declined" observation.
	if tr.Steps[0].ToolResult == nil ||
		!strings.Contains(tr.Steps[0].ToolResult.Output, "declined") {
			t.Errorf("expected declined observation, got %+v", tr.Steps[0].ToolResult)
	}
	if tr.StoppedBy != "answer" {
		t.Errorf("StoppedBy: got %q, want answer", tr.StoppedBy)
	}
}

// LLM errors propagate out of Run with the transcript in
// "error" state, so the TUI can show what happened.
func TestAgent_Run_LLMErrorPropagates(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(&stubTool{name: "noop", kind: KindRead})
	called := false
	llm := LLMFunc(func(_ context.Context, _ string) (string, error) {
		called = true
		return "", errors.New("ollama refused")
	})
	a, _ := New(r, llm, Options{MaxSteps: 4})
	tr, err := a.Run(context.Background(), "anything")
	if err == nil || !strings.Contains(err.Error(), "ollama refused") {
		t.Errorf("expected llm error to propagate, got %v", err)
	}
	if !called {
		t.Error("LLM should have been called once")
	}
	if tr.StoppedBy != "error" {
		t.Errorf("StoppedBy: got %q, want error", tr.StoppedBy)
	}
}

// Cancellation via context kills the loop and reports cancellation
// as the stop reason.
func TestAgent_Run_ContextCancellation(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(&stubTool{name: "noop", kind: KindRead})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before Run even starts
	llm := &MockLLM{Responses: []string{"Thought: ...\nAction: noop"}}
	a, _ := New(r, llm, Options{MaxSteps: 3})
	tr, err := a.Run(ctx, "anything")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if tr.StoppedBy != "cancelled" {
		t.Errorf("StoppedBy: got %q, want cancelled", tr.StoppedBy)
	}
}

// MockLLM.Prompts gives tests a way to assert that the agent's
// prompt construction is shaped correctly without exposing
// buildIterationPrompt directly.
func TestAgent_PromptContainsToolCatalog(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(&stubTool{name: "search", desc: "Find things.", kind: KindRead})
	llm := &MockLLM{Responses: []string{"Thought: x\nFinal Answer: ok"}}
	a, _ := New(r, llm, Options{MaxSteps: 1})
	if _, err := a.Run(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	if len(llm.Prompts) == 0 {
		t.Fatal("expected at least one prompt")
	}
	for _, want := range []string{
		"## search — read",
		"Find things.",
		"Thought / Action / Observation",
		"User's question: demo",
	} {
		if !strings.Contains(llm.Prompts[0], want) {
			t.Errorf("prompt missing %q\n--- prompt ---\n%s", want, llm.Prompts[0])
		}
	}
}
