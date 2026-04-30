package tui

import (
	"strings"
	"testing"
)

func mkEditor(lines ...string) *Editor {
	e := &Editor{}
	e.content = lines
	if len(e.content) == 0 {
		e.content = []string{""}
	}
	return e
}

func TestEditor_ReplaceRange_SameLine(t *testing.T) {
	e := mkEditor("hello world")
	e.ReplaceRange(0, 6, 0, 11, "there")
	if got := e.content[0]; got != "hello there" {
		t.Fatalf("got %q", got)
	}
	if e.cursor != 0 || e.col != 11 {
		t.Fatalf("cursor wrong: line=%d col=%d", e.cursor, e.col)
	}
	if e.selectionActive {
		t.Fatal("selection should be cleared")
	}
}

func TestEditor_ReplaceRange_AcrossLines(t *testing.T) {
	e := mkEditor("first line", "second", "third line")
	// Replace from "f"(0,0) through "second"(1,6) with "REPLACED".
	e.ReplaceRange(0, 0, 1, 6, "REPLACED")
	want := []string{"REPLACED", "third line"}
	if len(e.content) != 2 || e.content[0] != want[0] || e.content[1] != want[1] {
		t.Fatalf("got %v", e.content)
	}
	if e.cursor != 0 || e.col != len("REPLACED") {
		t.Fatalf("cursor wrong: line=%d col=%d", e.cursor, e.col)
	}
}

func TestEditor_ReplaceRange_InsertNewlineSplits(t *testing.T) {
	e := mkEditor("ab")
	e.ReplaceRange(0, 1, 0, 1, "X\nY")
	if len(e.content) != 2 || e.content[0] != "aX" || e.content[1] != "Yb" {
		t.Fatalf("got %v", e.content)
	}
	if e.cursor != 1 || e.col != 1 {
		t.Fatalf("cursor wrong: line=%d col=%d", e.cursor, e.col)
	}
}

func TestEditor_ReplaceRange_ClampsOutOfBounds(t *testing.T) {
	e := mkEditor("abc")
	// endLine and endCol way past the actual content.
	e.ReplaceRange(0, 0, 99, 999, "Z")
	if e.content[0] != "Z" || len(e.content) != 1 {
		t.Fatalf("expected clamping to single line 'Z', got %v", e.content)
	}
}

func TestEditor_ReplaceRange_NormalizesCRLF(t *testing.T) {
	e := mkEditor("x")
	e.ReplaceRange(0, 1, 0, 1, "a\r\nb")
	if len(e.content) != 2 || e.content[0] != "xa" || e.content[1] != "b" {
		t.Fatalf("got %v", e.content)
	}
}

func TestEditor_ReplaceRange_ReversedRangeIsNormalized(t *testing.T) {
	e := mkEditor("abcdef")
	// (0,4)→(0,2) reversed; expect same result as (0,2)→(0,4).
	e.ReplaceRange(0, 4, 0, 2, "X")
	if e.content[0] != "abXef" {
		t.Fatalf("got %q", e.content[0])
	}
}

func TestEditor_ReplaceRange_NoOpEmptyRangeEmptyText(t *testing.T) {
	e := mkEditor("hello")
	before := e.content[0]
	e.ReplaceRange(0, 2, 0, 2, "")
	if e.content[0] != before {
		t.Fatalf("expected no-op, got %q", e.content[0])
	}
}

func TestEditor_ReplaceRange_PreservesBeforeAndAfter(t *testing.T) {
	e := mkEditor("alpha", "beta", "gamma", "delta")
	e.ReplaceRange(1, 0, 2, 5, "MID")
	want := []string{"alpha", "MID", "delta"}
	if strings.Join(e.content, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v", e.content)
	}
}

func TestEditor_CurrentLineRange_ReturnsFullLine(t *testing.T) {
	e := mkEditor("hello world")
	e.cursor = 0
	e.col = 5
	sl, sc, el, ec := e.CurrentLineRange()
	if sl != 0 || sc != 0 || el != 0 || ec != len("hello world") {
		t.Fatalf("got (%d,%d)-(%d,%d)", sl, sc, el, ec)
	}
}
