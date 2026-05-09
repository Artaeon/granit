package serveapi

import (
	"strings"
	"testing"
)

func TestAnchorAtLine(t *testing.T) {
	body := "first line\nsecond line\n\nfourth, with leading spaces  "
	cases := []struct {
		line int
		want string
	}{
		{1, "first line"},
		{2, "second line"},
		{3, ""},
		{4, "fourth, with leading spaces"}, // trimmed
		{0, ""},
		{99, ""},
	}
	for _, c := range cases {
		if got := anchorAtLine(body, c.line); got != c.want {
			t.Errorf("anchorAtLine(%d) = %q, want %q", c.line, got, c.want)
		}
	}
}

func TestAnchorAtLineCaps60(t *testing.T) {
	long := strings.Repeat("x", 200)
	got := anchorAtLine(long, 1)
	if len(got) != 60 {
		t.Errorf("len = %d, want 60", len(got))
	}
}

func TestNumberLinesShort(t *testing.T) {
	got := numberLines("a\nb\nc", 100)
	want := "1: a\n2: b\n3: c\n"
	if got != want {
		t.Errorf("numberLines short = %q, want %q", got, want)
	}
}

func TestNumberLinesTruncates(t *testing.T) {
	// 10 lines, cap at 4: should produce 2 head + ellipsis + 2 tail.
	body := strings.Join([]string{
		"L1", "L2", "L3", "L4", "L5",
		"L6", "L7", "L8", "L9", "L10",
	}, "\n")
	got := numberLines(body, 4)
	// Head = first 2 lines, tail = last 2 lines.
	if !strings.Contains(got, "1: L1\n") {
		t.Errorf("head missing: %s", got)
	}
	if !strings.Contains(got, "2: L2\n") {
		t.Errorf("head missing line 2: %s", got)
	}
	// Tail starts at line 9 (10 - 4/2 + 1 = 9).
	if !strings.Contains(got, "9: L9\n") {
		t.Errorf("tail missing line 9: %s", got)
	}
	if !strings.Contains(got, "10: L10\n") {
		t.Errorf("tail missing line 10: %s", got)
	}
	if !strings.Contains(got, "elided") {
		t.Errorf("ellipsis row missing: %s", got)
	}
	// Lines 3-8 must NOT appear in the body part (they're elided).
	for _, n := range []string{"3: L3", "5: L5", "8: L8"} {
		if strings.Contains(got, n) {
			t.Errorf("elided line leaked: %q in %s", n, got)
		}
	}
}
