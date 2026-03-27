package tui

import (
	"strings"
	"testing"
)

// ── TruncateDisplay ─────────────────────────────────────────────

func TestTruncateDisplay_Normal(t *testing.T) {
	got := TruncateDisplay("Hello, world!", 5)
	if len(got) == 0 {
		t.Fatal("expected non-empty result")
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipsis suffix, got %q", got)
	}
}

func TestTruncateDisplay_ShortString(t *testing.T) {
	got := TruncateDisplay("Hi", 10)
	if got != "Hi" {
		t.Fatalf("expected %q unchanged, got %q", "Hi", got)
	}
}

func TestTruncateDisplay_EmptyString(t *testing.T) {
	got := TruncateDisplay("", 10)
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestTruncateDisplay_MaxWidthZero(t *testing.T) {
	got := TruncateDisplay("Hello", 0)
	if got != "" {
		t.Fatalf("expected empty string for maxWidth=0, got %q", got)
	}
}

func TestTruncateDisplay_MaxWidthOne(t *testing.T) {
	got := TruncateDisplay("Hello, world!", 1)
	if got != "…" {
		t.Fatalf("expected single ellipsis for maxWidth=1, got %q", got)
	}
}

func TestTruncateDisplay_Unicode(t *testing.T) {
	// Emoji characters are typically 2 cells wide.
	got := TruncateDisplay("🎉🎊🎈🎆", 5)
	if got == "" {
		t.Fatal("expected non-empty result for emoji string")
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipsis suffix for truncated emoji string, got %q", got)
	}
}

func TestTruncateDisplay_ExactFit(t *testing.T) {
	got := TruncateDisplay("abcde", 5)
	if got != "abcde" {
		t.Fatalf("expected %q (exact fit), got %q", "abcde", got)
	}
}

// ── RenderHelpBar ───────────────────────────────────────────────

func TestRenderHelpBar_WithBindings(t *testing.T) {
	bindings := []struct{ Key, Desc string }{
		{"q", "quit"},
		{"?", "help"},
	}
	got := RenderHelpBar(bindings)
	if got == "" {
		t.Fatal("expected non-empty help bar")
	}
	// The rendered output contains ANSI codes, but the raw text should
	// include our key and description strings.
	if !strings.Contains(got, "q") {
		t.Error("expected output to contain key 'q'")
	}
	if !strings.Contains(got, "quit") {
		t.Error("expected output to contain desc 'quit'")
	}
	if !strings.Contains(got, "?") {
		t.Error("expected output to contain key '?'")
	}
	if !strings.Contains(got, "help") {
		t.Error("expected output to contain desc 'help'")
	}
}

func TestRenderHelpBar_EmptyBindings(t *testing.T) {
	got := RenderHelpBar(nil)
	// With no bindings, we get an empty styled string. It should not panic
	// and should be effectively empty (may contain ANSI reset codes).
	if strings.Contains(got, "quit") {
		t.Error("expected no content with empty bindings")
	}
}
