package tui

import (
	"strings"
	"testing"
)

func TestParseCells(t *testing.T) {
	cells := parseCells("| Name | Age | City |")
	if len(cells) != 3 {
		t.Fatalf("expected 3 cells, got %d", len(cells))
	}
	if cells[0] != "Name" || cells[1] != "Age" || cells[2] != "City" {
		t.Errorf("unexpected cells: %v", cells)
	}
}

func TestParseCells_NoLeadingPipe(t *testing.T) {
	cells := parseCells("Name | Age")
	if len(cells) != 2 {
		t.Fatalf("expected 2 cells, got %d", len(cells))
	}
}

func TestParseCells_Empty(t *testing.T) {
	cells := parseCells("|  |  |")
	if len(cells) != 2 {
		t.Fatalf("expected 2 cells, got %d", len(cells))
	}
}

func TestParseAlignment_Left(t *testing.T) {
	if parseAlignment("---") != alignLeft {
		t.Error("--- should be left aligned")
	}
	if parseAlignment(":---") != alignLeft {
		t.Error(":--- should be left aligned")
	}
}

func TestParseAlignment_Right(t *testing.T) {
	if parseAlignment("---:") != alignRight {
		t.Error("---: should be right aligned")
	}
}

func TestParseAlignment_Center(t *testing.T) {
	if parseAlignment(":---:") != alignCenter {
		t.Error(":---: should be center aligned")
	}
}

func TestNormalizeRow_Pad(t *testing.T) {
	row := normalizeRow([]string{"a"}, 3)
	if len(row) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(row))
	}
	if row[0] != "a" || row[1] != "" || row[2] != "" {
		t.Errorf("unexpected row: %v", row)
	}
}

func TestNormalizeRow_Truncate(t *testing.T) {
	row := normalizeRow([]string{"a", "b", "c", "d"}, 2)
	if len(row) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(row))
	}
}

func TestTableEditor_OpenAndGetMarkdown(t *testing.T) {
	te := NewTableEditor()
	content := []string{
		"| Name | Age |",
		"|------|-----|",
		"| Alice | 30 |",
		"| Bob | 25 |",
	}
	te.Open(content, 0)

	if !te.IsActive() {
		t.Error("expected active after Open")
	}

	md := te.GetMarkdown()
	if !strings.Contains(md, "Alice") {
		t.Error("markdown should contain Alice")
	}
	if !strings.Contains(md, "Bob") {
		t.Error("markdown should contain Bob")
	}
	if !strings.Contains(md, "---") {
		t.Error("markdown should contain separator")
	}
}

func TestTableEditor_OpenNew(t *testing.T) {
	te := NewTableEditor()
	te.OpenNew(5)

	if !te.IsActive() {
		t.Error("expected active after OpenNew")
	}
	if len(te.headers) != 3 {
		t.Errorf("expected 3 default headers, got %d", len(te.headers))
	}
	if len(te.rows) != 2 {
		t.Errorf("expected 2 default rows, got %d", len(te.rows))
	}
}

func TestTableEditor_GetMarkdown_Empty(t *testing.T) {
	te := NewTableEditor()
	if te.GetMarkdown() != "" {
		t.Error("empty table should return empty markdown")
	}
}

func TestTableEditor_GetResult_NotReady(t *testing.T) {
	te := NewTableEditor()
	_, _, _, ok := te.GetResult()
	if ok {
		t.Error("expected no result when not active")
	}
}
