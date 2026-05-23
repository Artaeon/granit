package serveapi

import (
	"strings"
	"testing"
)

// findBacklinkContexts is the load-bearing helper behind the
// per-mention "line + snippet" detail in the editor's backlinks
// panel. The cases below cover the resolution rules from
// internal/vault/index.go that this function mirrors, plus the
// snippet/clip behaviour the panel relies on for readable rows.
func TestFindBacklinkContexts_TitleMatch(t *testing.T) {
	byTitle := map[string]string{"Target": "target.md"}
	byBase := map[string]string{}
	content := "Earlier we noted [[Target]] for the meeting."
	got := findBacklinkContexts(content, "target.md", byTitle, byBase, 5)
	if len(got) != 1 {
		t.Fatalf("want 1 context, got %d (%+v)", len(got), got)
	}
	if got[0].Line != 1 {
		t.Errorf("want line 1, got %d", got[0].Line)
	}
	if got[0].Snippet == "" {
		t.Error("want non-empty snippet")
	}
}

func TestFindBacklinkContexts_BasenameFallback(t *testing.T) {
	// Title map has no entry — basename fallback should resolve.
	byTitle := map[string]string{}
	byBase := map[string]string{"meeting-notes": "folder/meeting-notes.md"}
	content := "see [[meeting-notes]] for context"
	got := findBacklinkContexts(content, "folder/meeting-notes.md", byTitle, byBase, 5)
	if len(got) != 1 {
		t.Fatalf("basename fallback should resolve, got %d", len(got))
	}
}

func TestFindBacklinkContexts_AnchorFragmentIgnored(t *testing.T) {
	byTitle := map[string]string{"Target": "target.md"}
	byBase := map[string]string{}
	content := "we discussed [[Target#agenda]] yesterday"
	got := findBacklinkContexts(content, "target.md", byTitle, byBase, 5)
	if len(got) != 1 {
		t.Fatalf("anchor fragment should not block resolution, got %d", len(got))
	}
}

func TestFindBacklinkContexts_AliasIgnored(t *testing.T) {
	// `[[Target|display]]` — the alias text is for display only; the
	// regex must capture only the target.
	byTitle := map[string]string{"Target": "target.md"}
	byBase := map[string]string{}
	content := "see [[Target|the dashboard]] for status"
	got := findBacklinkContexts(content, "target.md", byTitle, byBase, 5)
	if len(got) != 1 {
		t.Fatalf("alias should not break resolution, got %d", len(got))
	}
}

func TestFindBacklinkContexts_MultipleMentions(t *testing.T) {
	byTitle := map[string]string{"X": "x.md"}
	byBase := map[string]string{}
	content := "line one mentions [[X]]\nthen line two mentions [[X]] again\nand [[X]] one more time"
	got := findBacklinkContexts(content, "x.md", byTitle, byBase, 5)
	if len(got) != 3 {
		t.Fatalf("want 3 mentions, got %d", len(got))
	}
	wantLines := []int{1, 2, 3}
	for i, c := range got {
		if c.Line != wantLines[i] {
			t.Errorf("context %d: want line %d, got %d", i, wantLines[i], c.Line)
		}
	}
}

func TestFindBacklinkContexts_CapEnforced(t *testing.T) {
	byTitle := map[string]string{"X": "x.md"}
	byBase := map[string]string{}
	// 10 mentions, cap at 3
	content := "[[X]] [[X]] [[X]] [[X]] [[X]] [[X]] [[X]] [[X]] [[X]] [[X]]"
	got := findBacklinkContexts(content, "x.md", byTitle, byBase, 3)
	if len(got) != 3 {
		t.Fatalf("cap should limit to 3, got %d", len(got))
	}
}

func TestFindBacklinkContexts_NonTargetSkipped(t *testing.T) {
	// Source mentions both `[[A]]` (our target) and `[[B]]` (unrelated).
	// Only the A match should appear in contexts for target A.
	byTitle := map[string]string{"A": "a.md", "B": "b.md"}
	byBase := map[string]string{}
	content := "see [[A]] and also [[B]] for context"
	got := findBacklinkContexts(content, "a.md", byTitle, byBase, 5)
	if len(got) != 1 {
		t.Fatalf("only A mention should match for target a.md, got %d", len(got))
	}
}

func TestFindBacklinkContexts_EmptyInputs(t *testing.T) {
	byTitle := map[string]string{"X": "x.md"}
	byBase := map[string]string{}
	// Empty content
	if got := findBacklinkContexts("", "x.md", byTitle, byBase, 5); got != nil {
		t.Errorf("empty content should return nil, got %+v", got)
	}
	// Empty target
	if got := findBacklinkContexts("[[X]]", "", byTitle, byBase, 5); got != nil {
		t.Errorf("empty target should return nil, got %+v", got)
	}
	// Cap zero or negative
	if got := findBacklinkContexts("[[X]]", "x.md", byTitle, byBase, 0); got != nil {
		t.Errorf("cap=0 should return nil, got %+v", got)
	}
}

func TestMakeContextSnippet_PrependsEllipsisWhenClipped(t *testing.T) {
	content := "a very long preamble here before the [[link]] and a tail after"
	// match starts at "[[link]]"
	matchStart := 37
	matchEnd := 45
	s := makeContextSnippet(content, matchStart, matchEnd, 10)
	// "…" is a 3-byte UTF-8 rune; HasPrefix gives byte-correct check.
	if !strings.HasPrefix(s, "…") {
		t.Errorf("expected leading ellipsis, got %q", s)
	}
}

func TestMakeContextSnippet_CollapsesNewlines(t *testing.T) {
	content := "line one\nline two with [[link]]\nline three"
	matchStart := 23
	matchEnd := 31
	s := makeContextSnippet(content, matchStart, matchEnd, 40)
	// Snippet should not contain any literal newlines — the panel
	// renders this on one line.
	for _, r := range s {
		if r == '\n' {
			t.Errorf("snippet should collapse newlines, got %q", s)
			break
		}
	}
}
