package tui

import (
	"strings"
	"testing"

	"github.com/artaeon/granit/internal/agents"
)

// renderAgentEvent stays stable as we rename event kinds and add new
// ones. The TUI's transcript view depends on this output shape — a
// regression here would render junk in the runner's live transcript.
func TestRenderAgentEvent_KindFormatting(t *testing.T) {
	cases := []struct {
		ev   agents.Event
		want string
	}{
		{agents.Event{Step: 0, Kind: agents.EventGoal, Text: "find X"}, "Goal:"},
		{agents.Event{Step: 1, Kind: agents.EventThought, Text: "thinking"}, "thought"},
		{agents.Event{Step: 2, Kind: agents.EventToolCall, Text: `read_note(path="x.md")`}, "→"},
		{agents.Event{Step: 2, Kind: agents.EventToolResult, Text: "got it"}, "←"},
		{agents.Event{Step: 3, Kind: agents.EventDeclined, Text: "no"}, "declined"},
		{agents.Event{Step: 4, Kind: agents.EventError, Text: "boom"}, "error"},
		{agents.Event{Step: 5, Kind: agents.EventBudgetHit, Text: "out of steps"}, "out of steps"},
		{agents.Event{Step: 6, Kind: agents.EventFinalAnswer, Text: ""}, "answer"},
	}
	for _, c := range cases {
		got := renderAgentEvent(c.ev)
		if !strings.Contains(got, c.want) {
			t.Errorf("event %s: rendered %q does not contain %q", c.ev.Kind, got, c.want)
		}
	}
}

// truncateAgentLine collapses internal newlines and caps length so
// no single overflowing observation can blow out the live overlay.
func TestTruncateAgentLine(t *testing.T) {
	cases := []struct {
		in     string
		max    int
		want   string
	}{
		{"short", 80, "short"},
		{"a\nb\nc", 80, "a b c"}, // newlines collapsed
		{strings.Repeat("x", 100), 20, strings.Repeat("x", 19) + "…"},
	}
	for _, c := range cases {
		got := truncateAgentLine(c.in, c.max)
		if got != c.want {
			t.Errorf("input=%q max=%d: got %q, want %q", c.in, c.max, got, c.want)
		}
	}
}

// translateProviderError converts low-level provider errors into
// actionable user-facing messages. Each branch points at the
// concrete fix (start Ollama, pull the model, fix API key) so the
// agent transcript is self-explanatory.
func TestTranslateProviderError(t *testing.T) {
	cases := []struct {
		name      string
		cfg       AIConfig
		err       error
		wantSub   string
	}{
		{
			name:    "ollama connection refused → start daemon hint",
			cfg:     AIConfig{Provider: "ollama"},
			err:     stubErr("Post http://127.0.0.1:11434: dial tcp: connection refused"),
			wantSub: "ollama serve",
		},
		{
			name:    "default provider unset → ollama hint",
			cfg:     AIConfig{Provider: ""},
			err:     stubErr("connection refused"),
			wantSub: "ollama serve",
		},
		{
			name:    "model not pulled → ollama pull hint",
			cfg:     AIConfig{Provider: "ollama", Model: "llama3.1:8b"},
			err:     stubErr(`model "llama3.1:8b" not found`),
			wantSub: "ollama pull llama3.1:8b",
		},
		{
			name:    "openai 401 → API key hint",
			cfg:     AIConfig{Provider: "openai", Model: "gpt-4o-mini"},
			err:     stubErr("status 401 Unauthorized"),
			wantSub: "API key",
		},
		{
			name:    "timeout → bigger-model hint",
			cfg:     AIConfig{Provider: "ollama", Model: "huge:70b"},
			err:     stubErr("context deadline exceeded"),
			wantSub: "smaller model",
		},
		{
			name:    "unrecognised passes through original error",
			cfg:     AIConfig{Provider: "ollama"},
			err:     stubErr("some weird error"),
			wantSub: "some weird error",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := translateProviderError(c.cfg, c.err)
			if got == nil {
				t.Fatal("expected non-nil error")
			}
			if !strings.Contains(got.Error(), c.wantSub) {
				t.Errorf("error %q does not contain %q", got, c.wantSub)
			}
		})
	}
}

// stubErr is a tiny shim so the test cases can declare error values
// inline without pulling in errors.New per case.
type stubErr string

func (e stubErr) Error() string { return string(e) }

// agentTaskBridge maps internal Task records to the agents-package
// view, stripping TUI-only fields and giving the agent only what it
// needs for filtering. Confirms tags are copied (not aliased) so
// later mutations to the source slice don't corrupt the agent's view.
func TestAgentTaskBridge_DefensiveTagCopy(t *testing.T) {
	src := []Task{
		{Text: "T1", NotePath: "Tasks.md", Done: false, DueDate: "2026-04-30",
			Priority: 3, Tags: []string{"work", "urgent"}},
	}
	out := agentTaskBridge(src)
	if len(out) != 1 || out[0].Text != "T1" {
		t.Fatalf("bridge dropped data: %+v", out)
	}
	if len(out[0].Tags) != 2 {
		t.Fatalf("tags lost: %+v", out[0].Tags)
	}
	// Mutate source — bridged copy should be unaffected.
	src[0].Tags[0] = "MUTATED"
	if out[0].Tags[0] == "MUTATED" {
		t.Error("tags should be copied, not aliased — source mutation leaked into bridged view")
	}
}

// parseFlatFrontmatter handles the everyday vault frontmatter:
// simple key: value, surrounding quotes, comments, and tab/space
// continuation skipping.
func TestParseFlatFrontmatter(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want map[string]string
	}{
		{
			name: "simple",
			in:   "---\ntype: person\nname: Alice\n---\nbody",
			want: map[string]string{"type": "person", "name": "Alice"},
		},
		{
			name: "no frontmatter",
			in:   "# Just a body\nno YAML here",
			want: map[string]string{},
		},
		{
			name: "quoted values",
			in:   "---\ntype: book\ntitle: \"Atomic Habits\"\n---\n",
			want: map[string]string{"type": "book", "title": "Atomic Habits"},
		},
		{
			name: "inline comment stripped",
			in:   "---\ntype: book\nstatus: read  # finished Feb\n---\n",
			want: map[string]string{"type": "book", "status": "read"},
		},
		{
			name: "tab-indented continuation skipped",
			in:   "---\ntype: book\ntags:\n\t- t1\n\t- t2\nauthor: Jane\n---\n",
			want: map[string]string{"type": "book", "author": "Jane"},
		},
		{
			name: "space-indented continuation skipped",
			in:   "---\ntype: book\ntags:\n  - t1\n  - t2\nauthor: Jane\n---\n",
			want: map[string]string{"type": "book", "author": "Jane"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseFlatFrontmatter(c.in)
			if len(got) != len(c.want) {
				t.Errorf("len: got %d %+v, want %d %+v", len(got), got, len(c.want), c.want)
				return
			}
			for k, v := range c.want {
				if got[k] != v {
					t.Errorf("key %q: got %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

// noteTitleFallback prefers the first H1; falls back to filename-
// without-extension. Used by the index builder when frontmatter
// title: is absent.
func TestNoteTitleFallback(t *testing.T) {
	cases := []struct {
		path, body, want string
	}{
		{"foo.md", "# Heading\nbody", "Heading"},
		{"sub/dir/foo.md", "no h1", "foo"},
		{"foo.md", "## not h1\n# real h1\n", "real h1"},
		{"x.md", "  #   spaced   \n", "spaced"},
	}
	for _, c := range cases {
		if got := noteTitleFallback(c.path, c.body); got != c.want {
			t.Errorf("path=%q body=%q: got %q, want %q", c.path, c.body, got, c.want)
		}
	}
}
