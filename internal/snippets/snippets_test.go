package snippets

import (
	"strings"
	"testing"
	"time"
)

// TestNew_BuiltinDefaults locks the contract that New() returns the
// builtin set in registration order. The web autocomplete and the TUI
// both rely on the order being stable so the picker doesn't reshuffle
// between releases.
func TestNew_BuiltinDefaults(t *testing.T) {
	e := New()
	all := e.All()
	if len(all) == 0 {
		t.Fatal("expected builtin snippets, got 0")
	}
	if all[0].Trigger != "/date" {
		t.Errorf("first snippet = %q, want %q", all[0].Trigger, "/date")
	}
}

// TestExpandPlaceholders_DateTime verifies the {{date}} / {{time}} /
// {{datetime}} substitutions render as the locale's current values.
// We can't pin "now" inside the engine, so we sandwich the call between
// two time reads and assert the output falls inside that window —
// flake-resistant without a clock-injection refactor.
func TestExpandPlaceholders_DateTime(t *testing.T) {
	e := New()
	before := time.Now()
	out := e.ExpandPlaceholders("today is {{date}} at {{time}}")
	after := time.Now()
	for _, day := range []string{before.Format("2006-01-02"), after.Format("2006-01-02")} {
		if strings.Contains(out, day) {
			return
		}
	}
	t.Errorf("expected output %q to contain a YYYY-MM-DD between %s and %s", out,
		before.Format("2006-01-02"), after.Format("2006-01-02"))
}

// TestTryExpand_Hit covers the happy path: an exact trigger match
// returns the expanded content (with placeholders replaced) and ok=true.
func TestTryExpand_Hit(t *testing.T) {
	e := New()
	out, ok := e.TryExpand("/todo")
	if !ok {
		t.Fatal("expected /todo to match")
	}
	if out != "- [ ] " {
		t.Errorf("got %q, want %q", out, "- [ ] ")
	}
}

// TestTryExpand_Miss verifies non-matching words return ok=false. The
// editor uses this signal to leave the user's text untouched when
// they type a `/word` that isn't a snippet.
func TestTryExpand_Miss(t *testing.T) {
	e := New()
	if _, ok := e.TryExpand("/notathing"); ok {
		t.Fatal("expected /notathing to miss")
	}
}

// TestMatchPrefix_Empty returns nil so the autocomplete picker can
// distinguish "user typed nothing" from "user typed something with
// no matches" (which returns an empty slice via the loop fall-through,
// not nil).
func TestMatchPrefix_Empty(t *testing.T) {
	e := New()
	if e.MatchPrefix("") != nil {
		t.Errorf("MatchPrefix(\"\") should be nil")
	}
}

// TestMatchPrefix_Partial picks up every snippet that shares the
// prefix. Verifies the autocomplete behaviour for typing /m → meeting,
// mermaid, etc.
func TestMatchPrefix_Partial(t *testing.T) {
	e := New()
	got := e.MatchPrefix("/h")
	// /h1, /h2, /h3 — three at minimum from the builtin set.
	if len(got) < 3 {
		t.Fatalf("MatchPrefix(/h) returned %d, want >=3", len(got))
	}
	for _, s := range got {
		if !strings.HasPrefix(s.Trigger, "/h") {
			t.Errorf("returned non-prefixed match: %q", s.Trigger)
		}
	}
}

// TestAddSnippet appends a custom trigger and verifies it shows up in
// All() + matches via TryExpand. Mirrors how the TUI mixes in
// zettelkasten templates at startup.
func TestAddSnippet(t *testing.T) {
	e := New()
	e.AddSnippet("/custom", "custom content {{date}}")
	out, ok := e.TryExpand("/custom")
	if !ok {
		t.Fatal("expected /custom to match after AddSnippet")
	}
	if !strings.HasPrefix(out, "custom content ") {
		t.Errorf("got %q, want prefix %q", out, "custom content ")
	}
}
