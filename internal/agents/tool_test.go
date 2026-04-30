package agents

import (
	"context"
	"strings"
	"testing"
)

// stubTool is the minimal Tool implementation used across tests —
// captures the args it received and returns a canned output. Faster
// than reaching for a real read_note in tests that only care about
// the registry/runtime contract.
type stubTool struct {
	name    string
	desc    string
	kind    ToolKind
	params  []ToolParam
	output  string
	err     error
	called  bool
	gotArgs map[string]string
}

func (s *stubTool) Name() string         { return s.name }
func (s *stubTool) Description() string  { return s.desc }
func (s *stubTool) Kind() ToolKind       { return s.kind }
func (s *stubTool) Params() []ToolParam  { return s.params }
func (s *stubTool) Run(_ context.Context, args map[string]string) ToolResult {
	s.called = true
	s.gotArgs = args
	return ToolResult{Output: s.output, Err: s.err}
}

// Register rejects nil tools, empty names, and duplicate names. Each
// rejection branch is independently necessary — bare assertions ensure
// none silently slips back in.
func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(nil); err == nil {
		t.Error("nil tool should be rejected")
	}
	if err := r.Register(&stubTool{name: ""}); err == nil {
		t.Error("empty name should be rejected")
	}
	if err := r.Register(&stubTool{name: "x", kind: KindRead}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(&stubTool{name: "x", kind: KindRead}); err == nil {
		t.Error("duplicate name should be rejected")
	}
}

// MustRegister panics on error, useful in setup code where a
// duplicate is a programmer mistake. Recovered here to confirm the
// panic actually fires (otherwise a silent failure would leave a
// half-initialised registry).
func TestRegistry_MustRegisterPanics(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(&stubTool{name: "a", kind: KindRead})
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustRegister should panic on duplicate")
		}
	}()
	r.MustRegister(&stubTool{name: "a", kind: KindRead})
}

// ToolFor / Names / All cooperate: ToolFor finds by name, Names lists
// alphabetically, All returns the same set sorted.
func TestRegistry_Lookup(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(&stubTool{name: "zeta", kind: KindRead})
	r.MustRegister(&stubTool{name: "alpha", kind: KindRead})
	r.MustRegister(&stubTool{name: "mu", kind: KindRead})

	if _, ok := r.ToolFor("alpha"); !ok {
		t.Error("ToolFor(alpha) should find it")
	}
	if _, ok := r.ToolFor("nope"); ok {
		t.Error("ToolFor(nope) should miss")
	}
	names := r.Names()
	if len(names) != 3 || names[0] != "alpha" || names[2] != "zeta" {
		t.Errorf("Names not sorted alphabetically: %v", names)
	}
	all := r.All()
	if len(all) != 3 || all[0].Name() != "alpha" {
		t.Errorf("All not sorted: %+v", all)
	}
}

// HasWriteTools is the safety gate: a registry with no Write tools
// returns false, one with at least one returns true. The runtime
// checks this to require an approve callback when needed.
func TestRegistry_HasWriteTools(t *testing.T) {
	r := NewRegistry()
	if r.HasWriteTools() {
		t.Error("empty registry: HasWriteTools should be false")
	}
	r.MustRegister(&stubTool{name: "ro", kind: KindRead})
	if r.HasWriteTools() {
		t.Error("read-only registry: HasWriteTools should be false")
	}
	r.MustRegister(&stubTool{name: "rw", kind: KindWrite})
	if !r.HasWriteTools() {
		t.Error("registry with a write tool: HasWriteTools should be true")
	}
}

// Describe renders to a stable text block keyed off tool name +
// declared params. Format is checked structurally (contains key
// markers) rather than verbatim to keep it forward-compatible with
// formatting tweaks.
func TestRegistry_Describe(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(&stubTool{
		name: "read_note", desc: "Read a markdown note.",
		kind: KindRead,
		params: []ToolParam{
			{Name: "path", Description: "Vault-relative path", Required: true},
			{Name: "max_chars", Description: "Truncate threshold"},
		},
	})
	out := r.Describe()
	for _, want := range []string{
		"## read_note — read",
		"Read a markdown note.",
		"Args:",
		"path (required) — Vault-relative path",
		"max_chars — Truncate threshold",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Describe missing %q\n--- output ---\n%s", want, out)
		}
	}
}

// Describe handles tools with no params cleanly — the LLM should see
// "Args: (none)" so it doesn't try to invent ones.
func TestRegistry_DescribeNoParams(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(&stubTool{name: "today", desc: "Returns today's date.", kind: KindRead})
	out := r.Describe()
	if !strings.Contains(out, "Args: (none)") {
		t.Errorf("expected 'Args: (none)' for paramless tool: %s", out)
	}
}

// Validate flags unknown tools with the available-names list so the
// LLM can self-correct on the next iteration.
func TestRegistry_ValidateUnknownTool(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(&stubTool{name: "alpha", kind: KindRead})
	probs := r.Validate(ToolCall{Tool: "beta"})
	if len(probs) != 1 || !strings.Contains(probs[0], "unknown tool") {
		t.Errorf("expected unknown-tool error, got %v", probs)
	}
	if !strings.Contains(probs[0], "alpha") {
		t.Errorf("error should list available names: %v", probs)
	}
}

// Validate flags every missing required arg in one observation, so
// the LLM can fix all of them in one retry instead of round-tripping
// for each individually.
func TestRegistry_ValidateMissingRequired(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(&stubTool{
		name: "x", kind: KindRead,
		params: []ToolParam{
			{Name: "a", Required: true},
			{Name: "b", Required: true},
			{Name: "c"}, // optional
		},
	})
	probs := r.Validate(ToolCall{Tool: "x", Args: map[string]string{}})
	if len(probs) != 2 {
		t.Fatalf("expected 2 missing-required errors, got %d: %v", len(probs), probs)
	}
}

// Validate accepts a fully-populated call.
func TestRegistry_ValidateOK(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(&stubTool{
		name: "x", kind: KindRead,
		params: []ToolParam{{Name: "a", Required: true}},
	})
	probs := r.Validate(ToolCall{Tool: "x", Args: map[string]string{"a": "value"}})
	if len(probs) != 0 {
		t.Errorf("valid call rejected: %v", probs)
	}
}
