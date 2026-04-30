package tui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSheetView_LoadCSVRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.csv")
	csv := "name,age,score\nAlice,30,99.5\nBob,25,87.2\nCarol,40,75\n"
	if err := os.WriteFile(path, []byte(csv), 0o644); err != nil {
		t.Fatal(err)
	}
	sv := NewSheetView()
	sv.SetSize(120, 40)
	if err := sv.Open(path); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got := sv.numRows(); got != 4 {
		t.Errorf("rows: got %d want 4", got)
	}
	if got := sv.numCols(); got != 3 {
		t.Errorf("cols: got %d want 3", got)
	}
	if got := sv.cell(1, 0); got != "Alice" {
		t.Errorf("cell(1,0): got %q want Alice", got)
	}
	// Numeric column kind detection (with header row).
	if k := sv.colKind(1); k != sheetColNumber {
		t.Errorf("col 1 kind: got %d want sheetColNumber", k)
	}
	if k := sv.colKind(2); k != sheetColNumber {
		t.Errorf("col 2 kind: got %d want sheetColNumber", k)
	}
	// Stats
	values := sv.numericColumnValues(1)
	if len(values) != 3 {
		t.Errorf("numeric values len: got %d want 3", len(values))
	}
	// Save and reload
	sv.setCell(1, 1, "31")
	if !sv.modified {
		t.Errorf("setCell did not flag modified")
	}
	if err := sv.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if sv.modified {
		t.Errorf("Save did not clear modified")
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "Alice,31,99.5") {
		t.Errorf("saved CSV missing edit: %q", data)
	}
}

func TestSheetView_TSVDelimiter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.tsv")
	tsv := "a\tb\tc\n1\t2\t3\n"
	if err := os.WriteFile(path, []byte(tsv), 0o644); err != nil {
		t.Fatal(err)
	}
	sv := NewSheetView()
	sv.SetSize(80, 24)
	if err := sv.Open(path); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if sv.numCols() != 3 {
		t.Errorf("tsv cols: got %d want 3", sv.numCols())
	}
}

func TestSheetView_PickerScansVault(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{"a.csv", "sub/b.xlsx", "sub/c.tsv", "ignore.txt"} {
		full := filepath.Join(dir, p)
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		_ = os.WriteFile(full, []byte("x"), 0o644)
	}
	sv := NewSheetView()
	sv.SetSize(80, 24)
	sv.OpenPicker(dir)
	if got := len(sv.pickerFiles); got != 3 {
		t.Errorf("picker files: got %d want 3", got)
	}
}

func TestSheetView_A1Address(t *testing.T) {
	cases := []struct {
		row, col int
		want     string
	}{
		{0, 0, "A1"},
		{0, 25, "Z1"},
		{0, 26, "AA1"},
		{4, 27, "AB5"},
	}
	for _, tc := range cases {
		if got := a1Address(tc.row, tc.col); got != tc.want {
			t.Errorf("a1(%d,%d)=%q want %q", tc.row, tc.col, got, tc.want)
		}
	}
	r, c, ok := parseA1("AB5")
	if !ok || r != 4 || c != 27 {
		t.Errorf("parseA1(AB5) = %d,%d,%v want 4,27,true", r, c, ok)
	}
	if _, _, ok := parseA1("bad"); ok {
		t.Errorf("parseA1(bad) should fail")
	}
}

func TestSheetView_NumericParsing(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"42", 42, true},
		{"$1,234.50", 1234.5, true},
		{"50%", 0.5, true},
		{"hello", 0, false},
		{"", 0, false},
		// Trailing currency (DE/AT format)
		{"852.00 €", 852, true},
		{"-99 €", -99, true},
		{"1,234.56 €", 1234.56, true},
		// EU number format (dot=thousands, comma=decimal)
		{"1.234,56 €", 1234.56, true},
		{"5,5", 5.5, true},      // EU decimal
		{"1,234", 1234, true},   // US thousands (3 trailing digits)
		{"5,55", 5.55, true},    // EU decimal (2 trailing digits)
		// Pound and yen
		{"£100", 100, true},
		{"¥10,000", 10000, true},
	}
	for _, tc := range cases {
		got, ok := parseCellNumeric(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("parse(%q)=%v,%v want %v,%v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestSheetView_DetectCurrencyTrailing(t *testing.T) {
	cases := []struct {
		raw          string
		wantSym      string
		wantTrailing bool
	}{
		{"$100", "$", false},
		{"100 $", "$", true},
		{"€500.50", "€", false},
		{"500.50 €", "€", true},
		{"£42", "£", false},
		{"42 £", "£", true},
		{"plain", "$", false}, // default
	}
	for _, tc := range cases {
		sym, trailing := detectCurrencySymbol(tc.raw)
		if sym != tc.wantSym || trailing != tc.wantTrailing {
			t.Errorf("detect(%q)=(%q,%v) want (%q,%v)", tc.raw, sym, trailing, tc.wantSym, tc.wantTrailing)
		}
	}
}

func TestSheetView_FormatTrailingCurrency(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"852.00 €", "852.00 €"},
		{"1234.5 €", "1,234.50 €"},
		{"-99 €", "-99.00 €"},
		{"$1234", "$1,234.00"},
	}
	for _, tc := range cases {
		got := formatCellDisplay(tc.raw, sheetColCurrency)
		if got != tc.want {
			t.Errorf("format(%q)=%q want %q", tc.raw, got, tc.want)
		}
	}
}

func TestSheetView_SortByColumn(t *testing.T) {
	sv := NewSheetView()
	sv.SetSize(120, 24)
	sv.sheets = []sheetData{{name: "S1", rows: [][]string{
		{"name", "score"},
		{"Alice", "30"},
		{"Bob", "10"},
		{"Carol", "20"},
	}}}
	sv.Activate()
	sv.recomputeColumnKinds()
	// Sort by score ascending
	sv.col = 1
	sv.sortCol = 1
	sv.sortDir = 1
	sv.applySort()
	if sv.cell(1, 0) != "Bob" || sv.cell(2, 0) != "Carol" || sv.cell(3, 0) != "Alice" {
		t.Errorf("asc sort wrong: %q %q %q", sv.cell(1, 0), sv.cell(2, 0), sv.cell(3, 0))
	}
	// Descending
	sv.sortDir = -1
	sv.applySort()
	if sv.cell(1, 0) != "Alice" || sv.cell(3, 0) != "Bob" {
		t.Errorf("desc sort wrong: %q %q", sv.cell(1, 0), sv.cell(3, 0))
	}
}

func TestSheetView_SectionHeaderDetection(t *testing.T) {
	sv := NewSheetView()
	sv.SetSize(80, 24)
	sv.sheets = []sheetData{{name: "S1", rows: [][]string{
		{"a", "b", "c"},
		{"Section 1", "", ""}, // section banner
		{"x", "1", "2"},
		{"x", "", "5"}, // not a section (col c has value)
	}}}
	sv.Activate()
	sv.headerIsLabel = true
	if !sv.isSectionHeaderRow(1) {
		t.Errorf("row 1 should be section header")
	}
	if sv.isSectionHeaderRow(0) {
		t.Errorf("row 0 (header) should NOT be section header")
	}
	if sv.isSectionHeaderRow(2) {
		t.Errorf("row 2 should NOT be section header")
	}
	if sv.isSectionHeaderRow(3) {
		t.Errorf("row 3 should NOT be section header")
	}
}

func TestSheetView_InsertDeleteRowCol(t *testing.T) {
	sv := NewSheetView()
	sv.SetSize(80, 24)
	sv.sheets = []sheetData{{name: "Sheet1", rows: [][]string{{"a", "b"}, {"c", "d"}}}}
	sv.Activate()
	sv.recomputeColumnKinds()
	sv.recomputeColWidths()
	sv.insertRow(1)
	if sv.numRows() != 3 {
		t.Errorf("insertRow rows: %d want 3", sv.numRows())
	}
	if sv.cell(1, 0) != "" {
		t.Errorf("inserted row should be empty, got %q", sv.cell(1, 0))
	}
	sv.deleteRow(0)
	if sv.numRows() != 2 || sv.cell(0, 0) != "" {
		t.Errorf("deleteRow: rows=%d cell0=%q", sv.numRows(), sv.cell(0, 0))
	}
	sv.insertCol(2)
	if sv.numCols() != 3 {
		t.Errorf("insertCol cols: %d want 3", sv.numCols())
	}
	sv.deleteCol(0)
	if sv.numCols() != 2 {
		t.Errorf("deleteCol cols: %d want 2", sv.numCols())
	}
}

func TestSheetView_IsSpreadsheetExt(t *testing.T) {
	cases := map[string]bool{
		"a.csv":  true,
		"a.tsv":  true,
		"a.xlsx": true,
		"a.xlsm": true,
		"a.md":   false,
		"a.txt":  false,
		"a":      false,
	}
	for in, want := range cases {
		if got := IsSpreadsheetExt(in); got != want {
			t.Errorf("IsSpreadsheetExt(%q)=%v want %v", in, got, want)
		}
	}
}

func TestSheetView_XLSXRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.xlsx")
	// Create initial file via SheetView so we exercise saveXLSX
	// from a non-existent file too.
	sv := NewSheetView()
	sv.SetSize(120, 40)
	if err := sv.Open(path); err != nil {
		t.Fatalf("Open new xlsx: %v", err)
	}
	sv.setCell(0, 0, "Item")
	sv.setCell(0, 1, "Price")
	sv.setCell(1, 0, "Coffee")
	sv.setCell(1, 1, "4.5")
	sv.setCell(2, 0, "Tea")
	sv.setCell(2, 1, "3")
	if err := sv.Save(); err != nil {
		t.Fatalf("Save xlsx: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("xlsx not written: %v", err)
	}

	// Reopen and verify content survived the round-trip.
	sv2 := NewSheetView()
	sv2.SetSize(120, 40)
	if err := sv2.Open(path); err != nil {
		t.Fatalf("Reopen xlsx: %v", err)
	}
	if sv2.cell(0, 0) != "Item" || sv2.cell(2, 1) != "3" {
		t.Errorf("xlsx round-trip lost data: %q / %q", sv2.cell(0, 0), sv2.cell(2, 1))
	}
}

func TestSheetView_FindNext(t *testing.T) {
	sv := NewSheetView()
	sv.SetSize(80, 24)
	sv.sheets = []sheetData{{name: "Sheet1", rows: [][]string{
		{"name", "city"},
		{"Alice", "NYC"},
		{"Bob", "LA"},
		{"Carol", "NYC"},
	}}}
	sv.Activate()
	sv.recomputeColumnKinds()
	sv.recomputeColWidths()
	sv.findNext("nyc")
	if sv.row != 1 || sv.col != 1 {
		t.Errorf("first NYC: row=%d col=%d want 1,1", sv.row, sv.col)
	}
	sv.findNext("nyc")
	if sv.row != 3 || sv.col != 1 {
		t.Errorf("second NYC: row=%d col=%d want 3,1", sv.row, sv.col)
	}
}

func TestSheetView_AddThousands(t *testing.T) {
	cases := []struct {
		v        float64
		decimals int
		want     string
	}{
		{0, 0, "0"},
		{42, 0, "42"},
		{999, 0, "999"},
		{1000, 0, "1,000"},
		{12345, 0, "12,345"},
		{1234567, 2, "1,234,567.00"},
		{-12345.5, 2, "-12,345.50"},
		{1234.5, 1, "1,234.5"},
	}
	for _, tc := range cases {
		if got := addThousands(tc.v, tc.decimals); got != tc.want {
			t.Errorf("addThousands(%v,%d)=%q want %q", tc.v, tc.decimals, got, tc.want)
		}
	}
}

func TestSheetView_FormatCellDisplay(t *testing.T) {
	cases := []struct {
		raw  string
		kind sheetCol
		want string
	}{
		{"1234.5", sheetColNumber, "1,234.5"},
		{"62.99", sheetColNumber, "62.99"},
		{"5000.00", sheetColNumber, "5,000.00"},
		{"1000", sheetColNumber, "1,000"},
		{"$1234.50", sheetColCurrency, "$1,234.50"},
		{"-500", sheetColCurrency, "-$500.00"},
		{"50%", sheetColPercent, "50%"},
		{"hello", sheetColText, "hello"},
		{"", sheetColNumber, ""},
	}
	for _, tc := range cases {
		got := formatCellDisplay(tc.raw, tc.kind)
		if got != tc.want {
			t.Errorf("format(%q,%d)=%q want %q", tc.raw, tc.kind, got, tc.want)
		}
	}
}

func TestSheetView_GridRenderContainsValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.csv")
	csv := "name,price\nApple,1.50\nBanana,2.25\n"
	if err := os.WriteFile(path, []byte(csv), 0o644); err != nil {
		t.Fatal(err)
	}
	sv := NewSheetView()
	sv.SetSize(120, 24)
	if err := sv.Open(path); err != nil {
		t.Fatal(err)
	}
	out := sv.View()
	for _, want := range []string{"Apple", "Banana", "data.csv", "A1"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q", want)
		}
	}
}

func TestSheetView_TemplatesAvailable(t *testing.T) {
	tpls := allSheetTemplates()
	if len(tpls) < 5 {
		t.Errorf("expected ≥5 templates, got %d", len(tpls))
	}
	// First template must be Blank so users always have a fallback.
	if tpls[0].ID != "blank" {
		t.Errorf("first template should be blank, got %q", tpls[0].ID)
	}
	// Every template must have at least 1 row of seed data.
	for _, tpl := range tpls {
		if len(tpl.Rows) == 0 {
			t.Errorf("template %q has no rows", tpl.ID)
		}
		if tpl.Suggested == "" {
			t.Errorf("template %q has no suggested filename", tpl.ID)
		}
	}
}

func TestSheetView_TemplateExpandsPlaceholders(t *testing.T) {
	now := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)
	tpl := sheetTemplate{
		Suggested: "Budget-{month}",
		Rows: [][]string{
			{"Date", "{date}"},
			{"Year", "{year}"},
		},
	}
	if got := tpl.expandFilename(now); got != "Budget-2026-04" {
		t.Errorf("filename: got %q want Budget-2026-04", got)
	}
	rows := tpl.expandedRows(now)
	if rows[0][1] != "2026-04-27" {
		t.Errorf("expanded date: got %q", rows[0][1])
	}
	if rows[1][1] != "2026" {
		t.Errorf("expanded year: got %q", rows[1][1])
	}
}

func TestSheetView_OpenWithTemplate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tpl-test.csv")
	sv := NewSheetView()
	sv.SetSize(120, 30)
	tpl := sheetTemplate{
		ID:        "test",
		Name:      "Test Template",
		Suggested: "test",
		Rows: [][]string{
			{"col1", "col2"},
			{"a", "b"},
			{"c", "d"},
		},
	}
	if err := sv.openWithTemplate(path, tpl); err != nil {
		t.Fatalf("openWithTemplate: %v", err)
	}
	if !sv.modified {
		t.Errorf("template open should leave file modified (unsaved)")
	}
	if sv.numRows() != 3 || sv.numCols() != 2 {
		t.Errorf("rows/cols: got %d×%d want 3×2", sv.numRows(), sv.numCols())
	}
	if sv.cell(1, 0) != "a" {
		t.Errorf("seed cell wrong: got %q", sv.cell(1, 0))
	}
	// Save and reload to verify roundtrip.
	if err := sv.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	sv2 := NewSheetView()
	sv2.SetSize(120, 30)
	if err := sv2.Open(path); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if sv2.cell(2, 1) != "d" {
		t.Errorf("reopen mismatch: got %q want d", sv2.cell(2, 1))
	}
}

func TestSheetView_UndoRedo(t *testing.T) {
	sv := NewSheetView()
	sv.SetSize(80, 24)
	sv.sheets = []sheetData{{name: "S1", rows: [][]string{
		{"a", "b"}, {"1", "2"},
	}}}
	sv.Activate()
	sv.recomputeColumnKinds()

	// Edit cell, then undo.
	sv.pushUndo()
	sv.setCell(1, 0, "X")
	if sv.cell(1, 0) != "X" {
		t.Fatalf("setCell didn't take")
	}
	if !sv.undo() {
		t.Errorf("undo should succeed")
	}
	if sv.cell(1, 0) != "1" {
		t.Errorf("undo didn't restore: got %q", sv.cell(1, 0))
	}
	// Redo
	if !sv.redo() {
		t.Errorf("redo should succeed")
	}
	if sv.cell(1, 0) != "X" {
		t.Errorf("redo didn't reapply: got %q", sv.cell(1, 0))
	}
	// Empty redo after fresh edit
	sv.pushUndo()
	sv.setCell(1, 1, "Y")
	if len(sv.redoStack) != 0 {
		t.Errorf("new edit should clear redo stack")
	}
}

func TestSheetView_VisualSelectionTSV(t *testing.T) {
	sv := NewSheetView()
	sv.SetSize(80, 24)
	sv.sheets = []sheetData{{name: "S1", rows: [][]string{
		{"a", "b", "c"},
		{"1", "2", "3"},
		{"4", "5", "6"},
	}}}
	sv.Activate()
	sv.selActive = true
	sv.selStartRow, sv.selStartCol = 1, 0
	sv.selEndRow, sv.selEndCol = 2, 1
	got := sv.selectionAsTSV()
	want := "1\t2\n4\t5\n"
	if got != want {
		t.Errorf("selectionAsTSV: got %q want %q", got, want)
	}
	if !sv.inSelection(1, 0) || !sv.inSelection(2, 1) {
		t.Errorf("inSelection corners should be true")
	}
	if sv.inSelection(0, 0) || sv.inSelection(2, 2) {
		t.Errorf("inSelection outside should be false")
	}
}

func TestSheetView_DetectDecimals(t *testing.T) {
	cases := []struct {
		raw  string
		want int
	}{
		{"42", 0},
		{"42.5", 1},
		{"42.50", 2},
		{"5000.00", 2},
		{"62.99 €", 2},
		{"1,234", 0},     // thousands, not decimal
		{"1,234.56", 2},  // US: dot is decimal
		{"1.234,56", 2},  // EU: comma is decimal
		{"5,5", 1},       // EU short decimal
		{"50%", 0},
	}
	for _, tc := range cases {
		if got := svDetectDecimals(tc.raw); got != tc.want {
			t.Errorf("decimals(%q)=%d want %d", tc.raw, got, tc.want)
		}
	}
}

func TestSheetView_AutoFitColumn(t *testing.T) {
	sv := NewSheetView()
	sv.SetSize(120, 24)
	sv.sheets = []sheetData{{name: "S1", rows: [][]string{
		{"short", "this is a very long header value that exceeds 28"},
		{"x", "y"},
	}}}
	sv.Activate()
	sv.recomputeColWidths()
	// Default cap is 28
	if sv.colWidths[1] != 28 {
		t.Errorf("default colWidth[1]=%d want 28 (clamped)", sv.colWidths[1])
	}
	sv.autoFitColumn(1)
	if sv.colWidths[1] <= 28 {
		t.Errorf("autoFit should expand past 28, got %d", sv.colWidths[1])
	}
}

func TestSheetView_FillDown(t *testing.T) {
	sv := NewSheetView()
	sv.SetSize(80, 24)
	sv.sheets = []sheetData{{name: "S1", rows: [][]string{
		{"label", "v"},
		{"X", ""},
		{"", ""},
		{"", ""},
		{"Y", ""}, // stops fill at this row
		{"", ""},
	}}}
	sv.Activate()
	sv.recomputeColumnKinds()
	// Place cursor on row 1 col 0 (X) and fill down
	sv.row, sv.col = 1, 0
	src := sv.cell(sv.row, sv.col)
	sv.pushUndo()
	filled := 0
	for r := sv.row + 1; r < sv.numRows(); r++ {
		if got := sv.cell(r, 0); got != "" {
			break
		}
		sv.setCell(r, 0, src)
		filled++
	}
	if filled != 2 {
		t.Errorf("filled=%d want 2 (rows 2 and 3, stop at row 4)", filled)
	}
	if sv.cell(2, 0) != "X" || sv.cell(3, 0) != "X" {
		t.Errorf("fill not applied: row2=%q row3=%q", sv.cell(2, 0), sv.cell(3, 0))
	}
	if sv.cell(4, 0) != "Y" {
		t.Errorf("fill should NOT overwrite row 4: got %q", sv.cell(4, 0))
	}
}

func TestSheetView_ColumnRowFilledCount(t *testing.T) {
	sv := NewSheetView()
	sv.SetSize(80, 24)
	sv.sheets = []sheetData{{name: "S1", rows: [][]string{
		{"a", "b", ""},
		{"x", "", "z"},
		{"", "", ""},
	}}}
	sv.Activate()
	if got := sv.columnFilledCount(0); got != 2 {
		t.Errorf("col 0 filled=%d want 2", got)
	}
	if got := sv.columnFilledCount(2); got != 1 {
		t.Errorf("col 2 filled=%d want 1", got)
	}
	if got := sv.rowFilledCount(0); got != 2 {
		t.Errorf("row 0 filled=%d want 2", got)
	}
	if got := sv.rowFilledCount(1); got != 2 {
		t.Errorf("row 1 filled=%d want 2", got)
	}
	if got := sv.rowFilledCount(2); got != 0 {
		t.Errorf("row 2 filled=%d want 0", got)
	}
}

func TestSheetView_TotalRowSum(t *testing.T) {
	sv := NewSheetView()
	sv.SetSize(120, 24)
	sv.sheets = []sheetData{{name: "S1", rows: [][]string{
		{"item", "qty", "price"},
		{"a", "2", "10.50"},
		{"b", "3", "5.25"},
		{"c", "1", "100.00"},
	}}}
	sv.Activate()
	sv.headerIsLabel = true
	sv.recomputeColumnKinds()

	// Simulate the T-key path: build the total row exactly the
	// way updateNormal does.
	cols := sv.numCols()
	row := make([]string, cols)
	row[0] = "TOTAL"
	for c := 1; c < cols; c++ {
		sum := 0.0
		any := false
		for r := 1; r < sv.numRows(); r++ {
			if v, ok := parseCellNumeric(sv.cell(r, c)); ok {
				sum += v
				any = true
			}
		}
		if any {
			row[c] = strconv.FormatFloat(sum, 'f', svDetectDecimalsForCol(&sv, c), 64)
		}
	}
	sv.sheet().rows = append(sv.sheet().rows, row)

	if sv.cell(4, 1) != "6" {
		t.Errorf("total qty: %q want 6", sv.cell(4, 1))
	}
	if sv.cell(4, 2) != "115.75" {
		t.Errorf("total price: %q want 115.75", sv.cell(4, 2))
	}
}

func TestSheetView_SelectionStatsRender(t *testing.T) {
	sv := NewSheetView()
	sv.SetSize(120, 24)
	sv.sheets = []sheetData{{name: "S1", rows: [][]string{
		{"a", "b"},
		{"1", "10"},
		{"2", "20"},
		{"3", "30"},
	}}}
	sv.Activate()
	sv.recomputeColumnKinds()
	sv.selActive = true
	sv.selStartRow, sv.selStartCol = 1, 1
	sv.selEndRow, sv.selEndCol = 3, 1
	out := sv.renderSelectionStats(120)
	for _, want := range []string{"B2:B4", "cells", "n", "Σ"} {
		if !strings.Contains(out, want) {
			t.Errorf("selection stats missing %q", want)
		}
	}
}

func TestSheetView_HelpOverlay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "h.csv")
	_ = os.WriteFile(path, []byte("a,b\n1,2\n"), 0o644)
	sv := NewSheetView()
	sv.SetSize(120, 30)
	if err := sv.Open(path); err != nil {
		t.Fatal(err)
	}
	if sv.helpVisible {
		t.Errorf("help should start hidden")
	}
	out := sv.View()
	if strings.Contains(out, "Keyboard Reference") {
		t.Errorf("help shouldn't render before ?")
	}
	// Simulate ?
	sv.helpVisible = true
	out2 := sv.View()
	if !strings.Contains(out2, "Keyboard Reference") {
		t.Errorf("help should render when visible")
	}
	for _, want := range []string{"Navigation", "Editing", "Sort & Filter", "View & Format"} {
		if !strings.Contains(out2, want) {
			t.Errorf("help missing section %q", want)
		}
	}
}

func TestSheetView_RenderPickerEmpty(t *testing.T) {
	dir := t.TempDir()
	sv := NewSheetView()
	sv.SetSize(80, 24)
	sv.OpenPicker(dir)
	out := sv.View()
	if !strings.Contains(out, "No CSV") {
		t.Errorf("empty picker should mention 'No CSV', got: %s", out)
	}
}
