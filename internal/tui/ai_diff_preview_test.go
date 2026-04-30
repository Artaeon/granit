package tui

import (
	"strings"
	"testing"
)

func TestAIDiffPreview_OpenAndReset(t *testing.T) {
	p := NewAIDiffPreview()
	if p.IsActive() {
		t.Fatal("zero value should be inactive")
	}
	p.Open(aiActionRewrite, "old text", "new text", 0, 0, 0, 8, true)
	if !p.IsActive() {
		t.Fatal("Open should activate")
	}
	if p.Output() != "new text" {
		t.Errorf("output: got %q", p.Output())
	}
	sl, sc, el, ec := p.Range()
	if sl != 0 || sc != 0 || el != 0 || ec != 8 {
		t.Errorf("range: (%d,%d)-(%d,%d)", sl, sc, el, ec)
	}
	p.Reset()
	if p.IsActive() {
		t.Fatal("Reset should deactivate")
	}
}

func TestAIDiffPreview_OpenWithEmptyOutputIsNoop(t *testing.T) {
	p := NewAIDiffPreview()
	p.Open(aiActionRewrite, "old", "", 0, 0, 0, 3, true)
	if p.IsActive() {
		t.Fatal("empty output must not activate the preview")
	}
}

func TestAIDiffPreview_ViewContainsBeforeAfter(t *testing.T) {
	p := NewAIDiffPreview()
	p.SetSize(80, 24)
	p.Open(aiActionImprove, "raw text here", "polished text here", 0, 0, 0, 13, true)
	view := p.View()
	if !strings.Contains(view, "BEFORE") {
		t.Errorf("view missing BEFORE header: %q", view)
	}
	if !strings.Contains(view, "AFTER") {
		t.Errorf("view missing AFTER header: %q", view)
	}
	if !strings.Contains(view, "raw text here") {
		t.Errorf("view missing original text")
	}
	if !strings.Contains(view, "polished text here") {
		t.Errorf("view missing proposed text")
	}
	if !strings.Contains(view, "y / Enter") {
		t.Errorf("view missing accept hint")
	}
}

func TestExtractEditorRange_SameLine(t *testing.T) {
	e := mkEditor("hello world")
	got := extractEditorRange(e, 0, 6, 0, 11)
	if got != "world" {
		t.Errorf("got %q, want %q", got, "world")
	}
}

func TestExtractEditorRange_AcrossLines(t *testing.T) {
	e := mkEditor("first line", "second", "third line")
	got := extractEditorRange(e, 0, 6, 2, 5)
	want := "line\nsecond\nthird"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractEditorRange_ClampsOutOfBounds(t *testing.T) {
	e := mkEditor("abc")
	got := extractEditorRange(e, 0, 0, 99, 999)
	if got != "abc" {
		t.Errorf("got %q, want %q", got, "abc")
	}
}

func TestExtractEditorRange_ReversedNormalised(t *testing.T) {
	e := mkEditor("abcdef")
	got := extractEditorRange(e, 0, 4, 0, 1)
	if got != "bcd" {
		t.Errorf("got %q, want %q", got, "bcd")
	}
}
