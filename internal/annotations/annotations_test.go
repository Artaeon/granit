package annotations

import (
	"strings"
	"testing"
)

func TestRoundTripAdd(t *testing.T) {
	dir := t.TempDir()
	a, err := Add(dir, Annotation{
		NotePath:   "Notes/Idea.md",
		LineNum:    7,
		AnchorText: "The wise man knows what he does not know.",
		Text:       "Compare with Socrates.",
		Color:      "yellow",
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == "" || a.CreatedAt == "" || a.UpdatedAt == "" {
		t.Errorf("expected ID + timestamps populated: %+v", a)
	}
	got, err := ListForNote(dir, "Notes/Idea.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != a.ID {
		t.Errorf("ListForNote round trip failed: %+v", got)
	}
}

func TestAddValidation(t *testing.T) {
	dir := t.TempDir()
	cases := []Annotation{
		{NotePath: "", LineNum: 1, Text: "x"},                    // missing notePath
		{NotePath: "n.md", LineNum: 0, Text: "x"},                 // line 0
		{NotePath: "n.md", LineNum: 1, Text: ""},                  // empty text
		{NotePath: "n.md", LineNum: 1, Text: "   "},               // whitespace text
	}
	for i, a := range cases {
		if _, err := Add(dir, a); err == nil {
			t.Errorf("case %d: expected error for %+v", i, a)
		}
	}
}

func TestPatchUpdates(t *testing.T) {
	dir := t.TempDir()
	a, _ := Add(dir, Annotation{NotePath: "n.md", LineNum: 3, Text: "first"})
	patched, err := Patch(dir, a.ID, func(x *Annotation) {
		x.Text = "second"
		x.Color = "blue"
	})
	if err != nil {
		t.Fatal(err)
	}
	if patched.Text != "second" || patched.Color != "blue" {
		t.Errorf("patch didn't apply: %+v", patched)
	}
	if patched.UpdatedAt == a.UpdatedAt {
		t.Errorf("UpdatedAt should advance on patch")
	}
}

func TestPatchUnknownID(t *testing.T) {
	dir := t.TempDir()
	_, err := Patch(dir, "missing", func(*Annotation) {})
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestDeleteIdempotent(t *testing.T) {
	dir := t.TempDir()
	a, _ := Add(dir, Annotation{NotePath: "n.md", LineNum: 1, Text: "x"})
	if err := Delete(dir, a.ID); err != nil {
		t.Fatal(err)
	}
	// Second delete is a no-op, not an error.
	if err := Delete(dir, a.ID); err != nil {
		t.Errorf("second Delete should be no-op, got %v", err)
	}
	if err := Delete(dir, "never-existed"); err != nil {
		t.Errorf("Delete of unknown ID should be no-op, got %v", err)
	}
}

func TestRewriteNotePath(t *testing.T) {
	dir := t.TempDir()
	Add(dir, Annotation{NotePath: "old.md", LineNum: 1, Text: "a"})
	Add(dir, Annotation{NotePath: "old.md", LineNum: 2, Text: "b"})
	Add(dir, Annotation{NotePath: "other.md", LineNum: 1, Text: "c"})
	n, err := RewriteNotePath(dir, "old.md", "new.md")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("expected 2 rewrites, got %d", n)
	}
	got, _ := ListForNote(dir, "new.md")
	if len(got) != 2 {
		t.Errorf("expected 2 annotations under new.md, got %d", len(got))
	}
	got, _ = ListForNote(dir, "old.md")
	if len(got) != 0 {
		t.Errorf("expected no annotations under old.md, got %d", len(got))
	}
	// Unrelated note untouched.
	got, _ = ListForNote(dir, "other.md")
	if len(got) != 1 {
		t.Errorf("other.md should still have its annotation: %+v", got)
	}
}

func TestAnchorClipping(t *testing.T) {
	dir := t.TempDir()
	long := strings.Repeat("x", 200)
	a, err := Add(dir, Annotation{NotePath: "n.md", LineNum: 1, AnchorText: long, Text: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(a.AnchorText) > AnchorPreviewLen {
		t.Errorf("anchor not clipped: len=%d", len(a.AnchorText))
	}
}

func TestSortStableAcrossSaves(t *testing.T) {
	dir := t.TempDir()
	// Insert in mixed order — output must come back ordered.
	Add(dir, Annotation{NotePath: "b.md", LineNum: 5, Text: "z"})
	Add(dir, Annotation{NotePath: "a.md", LineNum: 10, Text: "y"})
	Add(dir, Annotation{NotePath: "a.md", LineNum: 2, Text: "x"})
	s, _ := LoadAll(dir)
	if len(s.Annotations) != 3 {
		t.Fatalf("expected 3, got %d", len(s.Annotations))
	}
	// Should be: a.md:2, a.md:10, b.md:5
	if s.Annotations[0].NotePath != "a.md" || s.Annotations[0].LineNum != 2 {
		t.Errorf("unexpected first row: %+v", s.Annotations[0])
	}
	if s.Annotations[2].NotePath != "b.md" {
		t.Errorf("unexpected last row: %+v", s.Annotations[2])
	}
}
