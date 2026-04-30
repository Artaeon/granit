package tui

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/xuri/excelize/v2"
)

// SheetView is the CSV/XLSX viewer-and-editor surface. Renders as
// a feature tab in the editor pane (Obsidian-tab style) so it
// composes with note tabs, TaskManager, Calendar, etc.
//
// Power-user UX: vim-like navigation (hjkl, gg/G, 0/$), single-key
// row/column ops, in-place cell edit, ASCII chart panel for any
// numeric column, column-stats footer (sum/avg/min/max/count).
//
// Supports CSV (encoding/csv) and XLSX (xuri/excelize) on the
// load and save paths; XLSX preserves its multi-sheet structure
// and exposes Tab/Shift+Tab to swap between sheets.

type sheetMode int

const (
	sheetModeNormal sheetMode = iota
	sheetModeEdit             // editing the active cell
	sheetModeFind             // typing a find query
	sheetModeGoto             // typing A1-style cell address
	sheetModeChart            // chart side-panel open (still in normal nav)
	sheetModePicker           // picking a file to open (no sheet loaded)
)

type sheetChartType int

const (
	sheetChartBar sheetChartType = iota
	sheetChartLine
	sheetChartHistogram
)

func (t sheetChartType) String() string {
	switch t {
	case sheetChartLine:
		return "Line"
	case sheetChartHistogram:
		return "Histogram"
	default:
		return "Bar"
	}
}

type sheetCol int

const (
	sheetColText sheetCol = iota
	sheetColNumber
	sheetColDate
	sheetColCurrency
	sheetColPercent
)

type sheetData struct {
	name    string
	rows    [][]string
	colKind []sheetCol
}

type SheetView struct {
	OverlayBase

	filePath  string // absolute path on disk
	fileType  string // "csv" or "xlsx"
	delimiter rune   // for CSV (comma by default; tab if .tsv)

	sheets      []sheetData
	activeSheet int

	// Cursor & viewport
	row, col           int // 0-based active cell within the active sheet
	rowOff, colOff     int // top-left corner of viewport (row/col indices)
	colWidths          []int

	// Modes
	mode       sheetMode
	editBuf    []rune
	editCursor int
	findBuf    []rune
	gotoBuf    []rune

	// Chart panel
	showChart bool
	chartType sheetChartType
	chartCol  int

	// Persistence
	modified  bool
	saveErr   error
	statusMsg string
	statusAt  time.Time

	// Selection (single rectangular range)
	selActive                  bool
	selStartRow, selStartCol   int
	selEndRow, selEndCol       int

	// Header row treats row 0 as labels for column kind detection
	// and stats. User can toggle with 'H'.
	headerIsLabel bool

	// showEmpty controls whether unused cells render a dim "·"
	// placeholder (true) or appear blank (false). Defaults off
	// because dense sheets with many empty columns become unreadable
	// when every cell has a placeholder dot. Toggle with 'e'.
	showEmpty bool

	// gridLines toggles thin │ separators between columns. Defaults
	// on for the spreadsheet feel; toggle with 'l'.
	gridLines bool

	// freezeHeader pins row 0 to the top of the viewport when scrolled
	// down (only when headerIsLabel). Defaults on.
	freezeHeader bool

	// Sort state: column index and direction (-1 desc, 0 none, 1 asc).
	// Applied as a stable sort over the data rows (header excluded
	// when headerIsLabel). Toggle through none → asc → desc with 's'.
	sortCol int
	sortDir int

	// helpVisible toggles the full keyboard reference overlay
	// (rendered on top of the grid). Bound to '?' in normal mode.
	helpVisible bool
	helpScroll  int

	// Undo / redo stacks. Each snapshot is a deep-copy of the
	// active sheet's rows + cursor + colWidths, captured BEFORE
	// every mutation (cell edit, row/col insert/delete, sort,
	// paste). Bounded at 100 to cap memory.
	undoStack []sheetSnapshot
	redoStack []sheetSnapshot

	// findMatchCount is the count of cells matching the last
	// /-search query. Surfaced in the status bar so the user
	// can tell how many hits exist without flipping through
	// them all with 'n'.
	findMatchCount int

	// selActive2 is the visual-selection range. When true, the
	// grid renders cells inside [selStartRow..selEndRow] ×
	// [selStartCol..selEndCol] with a distinct background, and
	// 'y' yanks the range as TSV.
	selVisualMode bool

	// pendingClose is set when the user presses 'q' or Ctrl+Q
	// inside the sheet so the host can drop the feature tab.
	pendingClose bool

	// confirmAction is non-empty when the surface is waiting on
	// a y/n keypress for a destructive action (e.g. "delete
	// column with N values?"). The string is the action ID; the
	// next 'y' executes, anything else cancels.
	confirmAction string
	confirmTarget int // row or column index the action operates on
	confirmText   string

	// Picker state — used when the surface is opened without
	// a target file (CmdSheetView from the palette). Lets the
	// user pick from CSV/XLSX files in the vault, or hit 'n'
	// to create a new one.
	pickerVaultRoot string
	pickerFiles     []sheetPickerEntry
	pickerCursor    int
	pickerScroll    int
	pickerNewName   []rune
	pickerNewMode   bool

	// pickerTemplateMode and pickerTemplateCursor drive the
	// template selection screen shown when the user presses 'n'
	// from the file list. Templates are picked first, then the
	// filename input appears with a sensible default derived
	// from the template (Budget-2026-04, Tasks-2026-04-27, etc.).
	pickerTemplateMode   bool
	pickerTemplateCursor int
	pickerTemplate       *sheetTemplate
}

type sheetPickerEntry struct {
	relPath string
	absPath string
	size    int64
	modTime time.Time
}

// NewSheetView returns a zero-value SheetView. Open() loads a
// file and switches the surface to active.
func NewSheetView() SheetView {
	return SheetView{
		delimiter:     ',',
		headerIsLabel: true,
		showEmpty:     false,
		gridLines:     true,
		freezeHeader:  true,
		sortCol:       -1,
		sortDir:       0,
	}
}

// Open loads the file at path (CSV or XLSX) and activates the
// surface. If the file does not exist a single empty sheet is
// created so the user can save it on Ctrl+S.
func (s *SheetView) Open(path string) error {
	s.Activate()
	s.filePath = path
	s.row = 0
	s.col = 0
	s.rowOff = 0
	s.colOff = 0
	s.activeSheet = 0
	s.mode = sheetModeNormal
	s.modified = false
	s.saveErr = nil
	s.selActive = false
	s.statusMsg = ""

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".tsv":
		s.fileType = "csv"
		s.delimiter = '\t'
	case ".xlsx", ".xlsm":
		s.fileType = "xlsx"
	default:
		s.fileType = "csv"
		s.delimiter = ','
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		s.sheets = []sheetData{{
			name: "Sheet1",
			rows: [][]string{
				{"A", "B", "C"},
				{"", "", ""},
			},
		}}
		s.recomputeColumnKinds()
		s.recomputeColWidths()
		s.statusMsg = "New file: " + filepath.Base(path)
		s.statusAt = time.Now()
		return nil
	}

	switch s.fileType {
	case "xlsx":
		if err := s.loadXLSX(path); err != nil {
			return err
		}
	default:
		if err := s.loadCSV(path); err != nil {
			return err
		}
	}
	s.recomputeColumnKinds()
	s.recomputeColWidths()
	s.statusMsg = fmt.Sprintf("Loaded %s (%d×%d)  ·  press ? for keyboard help",
		filepath.Base(path), s.numRows(), s.numCols())
	s.statusAt = time.Now()
	return nil
}

// Close resets the surface to inactive and drops the parsed
// data so a subsequent Open re-reads from disk.
func (s *SheetView) Close() {
	s.OverlayBase.Close()
	s.sheets = nil
	s.colWidths = nil
	s.editBuf = nil
	s.findBuf = nil
	s.gotoBuf = nil
	s.modified = false
	s.saveErr = nil
	s.pendingClose = false
	s.pickerFiles = nil
	s.pickerNewName = nil
	s.pickerNewMode = false
	s.pickerTemplateMode = false
	s.pickerTemplateCursor = 0
	s.pickerTemplate = nil
}

// OpenPicker activates the surface in picker mode — scans the
// vault for CSV / TSV / XLSX files and lets the user choose one
// (or press 'n' to create a new one). Used by the command
// palette entry "Open Spreadsheet" when no specific file is
// targeted.
func (s *SheetView) OpenPicker(vaultRoot string) {
	s.Activate()
	s.mode = sheetModePicker
	s.pickerVaultRoot = vaultRoot
	s.pickerCursor = 0
	s.pickerScroll = 0
	s.pickerNewName = nil
	s.pickerNewMode = false
	s.filePath = ""
	s.sheets = nil
	s.scanPickerFiles()
}

func (s *SheetView) scanPickerFiles() {
	s.pickerFiles = nil
	if s.pickerVaultRoot == "" {
		return
	}
	exts := map[string]bool{
		".csv": true, ".tsv": true, ".xlsx": true, ".xlsm": true,
	}
	_ = filepath.Walk(s.pickerVaultRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == ".granit" || base == ".granit-trash" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !exts[ext] {
			return nil
		}
		abs, _ := filepath.Abs(path)
		rel, _ := filepath.Rel(s.pickerVaultRoot, abs)
		s.pickerFiles = append(s.pickerFiles, sheetPickerEntry{
			relPath: rel,
			absPath: abs,
			size:    info.Size(),
			modTime: info.ModTime(),
		})
		return nil
	})
	// Most recently modified first — what the user usually wants.
	sort.Slice(s.pickerFiles, func(i, j int) bool {
		return s.pickerFiles[i].modTime.After(s.pickerFiles[j].modTime)
	})
}

// FilePath returns the absolute path the surface is editing,
// or empty if nothing is loaded.
func (s *SheetView) FilePath() string { return s.filePath }

// Modified reports whether there are unsaved changes.
func (s *SheetView) Modified() bool { return s.modified }

// ConsumePendingClose returns true if the user requested a tab
// close from inside the sheet (e.g. pressed 'q'). Resets the flag.
func (s *SheetView) ConsumePendingClose() bool {
	if !s.pendingClose {
		return false
	}
	s.pendingClose = false
	return true
}

// IsSpreadsheetExt reports whether path has a CSV/TSV/XLSX
// extension recognised by the sheet viewer. Used by the
// file-tree dispatch to decide whether to open the sheet
// surface instead of the markdown editor.
func IsSpreadsheetExt(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".csv", ".tsv", ".xlsx", ".xlsm":
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// File IO
// ---------------------------------------------------------------------------

func (s *SheetView) loadCSV(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	r := csv.NewReader(f)
	r.Comma = s.delimiter
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	rows, err := r.ReadAll()
	if err != nil {
		return err
	}
	maxCols := 0
	for _, row := range rows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}
	for i := range rows {
		for len(rows[i]) < maxCols {
			rows[i] = append(rows[i], "")
		}
	}
	if len(rows) == 0 {
		rows = [][]string{{""}}
	}
	s.sheets = []sheetData{{name: "Sheet1", rows: rows}}
	return nil
}

func (s *SheetView) loadXLSX(path string) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	names := f.GetSheetList()
	if len(names) == 0 {
		s.sheets = []sheetData{{name: "Sheet1", rows: [][]string{{""}}}}
		return nil
	}
	out := make([]sheetData, 0, len(names))
	for _, name := range names {
		rows, err := f.GetRows(name)
		if err != nil {
			return fmt.Errorf("read sheet %q: %w", name, err)
		}
		maxCols := 0
		for _, r := range rows {
			if len(r) > maxCols {
				maxCols = len(r)
			}
		}
		if maxCols == 0 {
			maxCols = 1
		}
		for i := range rows {
			for len(rows[i]) < maxCols {
				rows[i] = append(rows[i], "")
			}
		}
		if len(rows) == 0 {
			rows = [][]string{make([]string, maxCols)}
		}
		out = append(out, sheetData{name: name, rows: rows})
	}
	s.sheets = out
	return nil
}

// Save writes back to filePath in its original format. Returns
// nil on success and stashes the error in saveErr otherwise so
// the host can read it via ConsumeSaveError().
func (s *SheetView) Save() error {
	if s.filePath == "" {
		return fmt.Errorf("no file path")
	}
	switch s.fileType {
	case "xlsx":
		if err := s.saveXLSX(); err != nil {
			s.saveErr = err
			return err
		}
	default:
		if err := s.saveCSV(); err != nil {
			s.saveErr = err
			return err
		}
	}
	s.modified = false
	s.statusMsg = "Saved " + filepath.Base(s.filePath)
	s.statusAt = time.Now()
	return nil
}

func (s *SheetView) saveCSV() error {
	if len(s.sheets) == 0 {
		return fmt.Errorf("no data")
	}
	f, err := os.Create(s.filePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	w := csv.NewWriter(f)
	w.Comma = s.delimiter
	if err := w.WriteAll(s.sheets[0].rows); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

func (s *SheetView) saveXLSX() error {
	// Re-open the original file when possible so we preserve
	// styling/formulas the parser can't round-trip; fall back
	// to a fresh workbook if it doesn't exist.
	var f *excelize.File
	var err error
	if _, statErr := os.Stat(s.filePath); statErr == nil {
		f, err = excelize.OpenFile(s.filePath)
		if err != nil {
			return err
		}
	} else {
		f = excelize.NewFile()
		// New workbooks come with a default "Sheet1" — drop it
		// so the loop below recreates the sheets we actually have.
		_ = f.DeleteSheet("Sheet1")
	}
	defer func() { _ = f.Close() }()

	existing := map[string]bool{}
	for _, n := range f.GetSheetList() {
		existing[n] = true
	}
	// Track which existing sheets are still referenced; drop
	// ones the user removed. (We don't expose sheet add/delete
	// in v1, so this is mostly a safety net.)
	keep := map[string]bool{}
	for _, sh := range s.sheets {
		keep[sh.name] = true
		if !existing[sh.name] {
			if _, err := f.NewSheet(sh.name); err != nil {
				return err
			}
		} else {
			// Wipe existing rows in the sheet so deletions
			// round-trip correctly. excelize doesn't expose a
			// "clear sheet" call, so we delete and recreate.
			_ = f.DeleteSheet(sh.name)
			if _, err := f.NewSheet(sh.name); err != nil {
				return err
			}
		}
		for r, row := range sh.rows {
			for c, val := range row {
				cell, err := excelize.CoordinatesToCellName(c+1, r+1)
				if err != nil {
					return err
				}
				if val == "" {
					continue
				}
				// Preserve numeric typing where possible so
				// downstream consumers (Excel, LibreOffice)
				// keep using SUM/AVG correctly.
				if n, err := strconv.ParseFloat(val, 64); err == nil && !strings.ContainsAny(val, " ,'") {
					if err := f.SetCellValue(sh.name, cell, n); err != nil {
						return err
					}
				} else {
					if err := f.SetCellValue(sh.name, cell, val); err != nil {
						return err
					}
				}
			}
		}
	}
	for n := range existing {
		if !keep[n] {
			_ = f.DeleteSheet(n)
		}
	}
	if len(s.sheets) > 0 {
		if idx, err := f.GetSheetIndex(s.sheets[0].name); err == nil && idx >= 0 {
			f.SetActiveSheet(idx)
		}
	}
	return f.SaveAs(s.filePath)
}

// ConsumeSaveError returns and clears the most recent save error.
func (s *SheetView) ConsumeSaveError() error {
	err := s.saveErr
	s.saveErr = nil
	return err
}

// ---------------------------------------------------------------------------
// Sheet helpers
// ---------------------------------------------------------------------------

func (s *SheetView) sheet() *sheetData {
	if s.activeSheet < 0 || s.activeSheet >= len(s.sheets) {
		return nil
	}
	return &s.sheets[s.activeSheet]
}

func (s *SheetView) numRows() int {
	if sh := s.sheet(); sh != nil {
		return len(sh.rows)
	}
	return 0
}

func (s *SheetView) numCols() int {
	sh := s.sheet()
	if sh == nil || len(sh.rows) == 0 {
		return 0
	}
	return len(sh.rows[0])
}

func (s *SheetView) cell(r, c int) string {
	sh := s.sheet()
	if sh == nil || r < 0 || r >= len(sh.rows) {
		return ""
	}
	if c < 0 || c >= len(sh.rows[r]) {
		return ""
	}
	return sh.rows[r][c]
}

func (s *SheetView) setCell(r, c int, v string) {
	sh := s.sheet()
	if sh == nil {
		return
	}
	for len(sh.rows) <= r {
		width := s.numCols()
		if width == 0 {
			width = 1
		}
		sh.rows = append(sh.rows, make([]string, width))
	}
	for len(sh.rows[r]) <= c {
		sh.rows[r] = append(sh.rows[r], "")
	}
	if sh.rows[r][c] != v {
		sh.rows[r][c] = v
		s.modified = true
	}
}

// recomputeColumnKinds inspects sample data per column and
// classifies it so charts and stats know which columns are
// numeric. Heuristic: ≥70% of non-empty cells parse as numbers
// → number; mostly YYYY-MM-DD → date; trailing % → percent.
func (s *SheetView) recomputeColumnKinds() {
	sh := s.sheet()
	if sh == nil {
		return
	}
	cols := s.numCols()
	sh.colKind = make([]sheetCol, cols)
	startRow := 0
	if s.headerIsLabel && len(sh.rows) > 1 {
		startRow = 1
	}
	for c := 0; c < cols; c++ {
		var nonEmpty, numeric, dateLike, percent, currency int
		for r := startRow; r < len(sh.rows); r++ {
			v := strings.TrimSpace(sh.rows[r][c])
			if v == "" {
				continue
			}
			nonEmpty++
			// Currency: has $/€/£/¥ either at start or end
			hasCurrencySymbol := false
			for _, sym := range []string{"$", "€", "£", "¥"} {
				if strings.Contains(v, sym) {
					hasCurrencySymbol = true
					break
				}
			}
			if _, ok := parseCellNumeric(v); ok {
				numeric++
				switch {
				case strings.HasSuffix(v, "%"):
					percent++
				case hasCurrencySymbol:
					currency++
				}
				continue
			}
			if _, err := time.Parse("2006-01-02", v); err == nil {
				dateLike++
				continue
			}
			if _, err := time.Parse("2006-01-02 15:04:05", v); err == nil {
				dateLike++
			}
		}
		switch {
		case nonEmpty == 0:
			sh.colKind[c] = sheetColText
		case currency*100 >= nonEmpty*70:
			sh.colKind[c] = sheetColCurrency
		case percent*100 >= nonEmpty*70:
			sh.colKind[c] = sheetColPercent
		case numeric*100 >= nonEmpty*70:
			sh.colKind[c] = sheetColNumber
		case dateLike*100 >= nonEmpty*70:
			sh.colKind[c] = sheetColDate
		default:
			sh.colKind[c] = sheetColText
		}
	}
}

func (s *SheetView) colKind(c int) sheetCol {
	sh := s.sheet()
	if sh == nil || c < 0 || c >= len(sh.colKind) {
		return sheetColText
	}
	return sh.colKind[c]
}

// autoFitColumn resizes one column to its widest cell, capped
// at 60 cols (vs 28 in the global recomputeColWidths) so the
// user can see long strings without an extra modal dance. The
// cap remains so a 5000-char paste can't trash the grid.
func (s *SheetView) autoFitColumn(c int) {
	if c < 0 || c >= s.numCols() {
		return
	}
	w := 4
	for r := 0; r < s.numRows(); r++ {
		cw := utf8.RuneCountInString(s.cell(r, c))
		if cw > w {
			w = cw
		}
		if w >= 60 {
			w = 60
			break
		}
	}
	if c < len(s.colWidths) {
		s.colWidths[c] = w
	}
}

// recomputeColWidths sizes each column to fit its content,
// clamped to [4, 28] so a single 200-char cell can't blow out
// the grid. Header row counts toward width. Considers the
// FORMATTED display width (with comma thousand-separators
// and currency suffixes) so numeric cells don't get truncated
// just because the formatted form is one or two chars wider
// than the raw input.
func (s *SheetView) recomputeColWidths() {
	cols := s.numCols()
	s.colWidths = make([]int, cols)
	for c := 0; c < cols; c++ {
		w := 4
		kind := s.colKind(c)
		for r := 0; r < s.numRows(); r++ {
			raw := s.cell(r, c)
			// Compute the on-screen width — formatted for cells
			// that the renderer will format, raw for header /
			// section / plain text.
			disp := raw
			if !(s.headerIsLabel && r == 0) && !s.isSectionHeaderRow(r) {
				if f := formatCellDisplay(raw, kind); f != "" {
					disp = f
				}
			}
			cw := utf8.RuneCountInString(disp)
			if cw > w {
				w = cw
			}
			if w >= 28 {
				w = 28
				break
			}
		}
		s.colWidths[c] = w
	}
}

// parseCellNumeric attempts to read a numeric value from a cell,
// honouring leading and trailing currency symbols ($, €, £, ¥),
// percent suffix, and both US (1,234.56) and EU (1.234,56) digit-
// grouping conventions. Returns ok=false when the cell isn't numeric.
//
// Examples that parse:
//
//	"42"          → 42
//	"$1,234.50"   → 1234.5
//	"852.00 €"    → 852       (trailing symbol — common in DE/AT)
//	"1.234,56 €"  → 1234.56   (EU grouping with comma decimal)
//	"50%"         → 0.5
//	"-€99.00"     → -99
func parseCellNumeric(v string) (float64, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	// Strip currency symbols anywhere in the string (most spreadsheets
	// put them at the start in en_US locales and at the end in DE/AT
	// locales). We don't differentiate which symbol was used here —
	// formatCellDisplay re-applies the original symbol from the raw
	// cell at render time.
	for _, sym := range []string{"$", "€", "£", "¥"} {
		v = strings.ReplaceAll(v, sym, "")
	}
	v = strings.TrimSpace(v)
	hasPercent := strings.HasSuffix(v, "%")
	if hasPercent {
		v = strings.TrimSuffix(v, "%")
		v = strings.TrimSpace(v)
	}
	negative := strings.HasPrefix(v, "-")
	if negative {
		v = strings.TrimPrefix(v, "-")
	}
	// Detect EU vs US format. Rule of thumb: if the LAST separator
	// in the string is a comma it's the decimal point (EU); otherwise
	// the dot is decimal (US). A string with only one of the two
	// separators is ambiguous — we assume the present one is the
	// thousands separator unless it's followed by exactly 1-2 digits,
	// in which case it's a decimal.
	hasDot := strings.Contains(v, ".")
	hasComma := strings.Contains(v, ",")
	switch {
	case hasDot && hasComma:
		if strings.LastIndex(v, ",") > strings.LastIndex(v, ".") {
			// EU: 1.234,56 — strip dots, swap comma to dot
			v = strings.ReplaceAll(v, ".", "")
			v = strings.Replace(v, ",", ".", 1)
		} else {
			// US: 1,234.56
			v = strings.ReplaceAll(v, ",", "")
		}
	case hasComma:
		// "1,234" → 1234 (thousands) OR "5,5" → 5.5 (EU decimal).
		// Heuristic: comma followed by 1-2 trailing digits = decimal.
		idx := strings.LastIndex(v, ",")
		tail := len(v) - idx - 1
		if tail == 1 || tail == 2 {
			v = strings.Replace(v, ",", ".", 1)
		} else {
			v = strings.ReplaceAll(v, ",", "")
		}
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, false
	}
	if negative {
		n = -n
	}
	if hasPercent {
		n /= 100
	}
	return n, true
}

// ---------------------------------------------------------------------------
// Update — keyboard handling
// ---------------------------------------------------------------------------

// Update handles a tea.Msg and returns the updated SheetView.
// The host sends key messages for the active feature tab; mouse
// and resize messages are not consumed here (they reach the host).
func (s SheetView) Update(msg tea.Msg) (SheetView, tea.Cmd) {
	if !s.IsActive() {
		return s, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		// Help overlay swallows ALL keys while visible — Esc/?/q
		// dismiss it, anything else scrolls or no-ops. Done at the
		// top level so the user can't accidentally trigger row
		// inserts or saves while reading the reference.
		if s.helpVisible {
			return s.updateHelp(key)
		}
		switch s.mode {
		case sheetModePicker:
			return s.updatePicker(key)
		case sheetModeEdit:
			return s.updateEdit(key)
		case sheetModeFind:
			return s.updateFind(key)
		case sheetModeGoto:
			return s.updateGoto(key)
		default:
			return s.updateNormal(key)
		}
	}
	return s, nil
}

func (s SheetView) updateHelp(k tea.KeyMsg) (SheetView, tea.Cmd) {
	switch k.String() {
	case "esc", "q", "?":
		s.helpVisible = false
		s.helpScroll = 0
	case "down", "j":
		s.helpScroll++
	case "up", "k":
		if s.helpScroll > 0 {
			s.helpScroll--
		}
	case "g", "home":
		s.helpScroll = 0
	case "G", "end":
		s.helpScroll = 999 // clamped at render time
	case "pgdown", " ":
		s.helpScroll += 8
	case "pgup":
		s.helpScroll -= 8
		if s.helpScroll < 0 {
			s.helpScroll = 0
		}
	}
	return s, nil
}

func (s SheetView) updatePicker(k tea.KeyMsg) (SheetView, tea.Cmd) {
	// Sub-mode 1: filename input (after a template is picked)
	if s.pickerNewMode {
		return s.updatePickerNewName(k)
	}
	// Sub-mode 2: template selection (after 'n' is pressed)
	if s.pickerTemplateMode {
		return s.updatePickerTemplate(k)
	}
	// Sub-mode 3: file list (default)
	switch k.String() {
	case "up", "k":
		if s.pickerCursor > 0 {
			s.pickerCursor--
		}
	case "down", "j":
		if s.pickerCursor < len(s.pickerFiles)-1 {
			s.pickerCursor++
		}
	case "g", "home":
		s.pickerCursor = 0
	case "G", "end":
		s.pickerCursor = len(s.pickerFiles) - 1
	case "enter":
		if s.pickerCursor >= 0 && s.pickerCursor < len(s.pickerFiles) {
			path := s.pickerFiles[s.pickerCursor].absPath
			if err := s.Open(path); err != nil {
				s.statusMsg = "Open failed: " + err.Error()
				s.statusAt = time.Now()
				s.mode = sheetModePicker
			}
		}
	case "n":
		// Enter template selection screen.
		s.pickerTemplateMode = true
		s.pickerTemplateCursor = 0
	case "r":
		s.scanPickerFiles()
		if s.pickerCursor >= len(s.pickerFiles) {
			s.pickerCursor = len(s.pickerFiles) - 1
		}
	case "?":
		// Help toggle works inside the picker too.
		s.helpVisible = true
	case "q", "esc":
		s.pendingClose = true
	}
	if s.pickerCursor < 0 {
		s.pickerCursor = 0
	}
	return s, nil
}

// updatePickerTemplate handles the template-selection screen
// shown after the user presses 'n' from the file list.
func (s SheetView) updatePickerTemplate(k tea.KeyMsg) (SheetView, tea.Cmd) {
	templates := allSheetTemplates()
	switch k.String() {
	case "esc":
		s.pickerTemplateMode = false
		s.pickerTemplateCursor = 0
	case "up", "k":
		if s.pickerTemplateCursor > 0 {
			s.pickerTemplateCursor--
		}
	case "down", "j":
		if s.pickerTemplateCursor < len(templates)-1 {
			s.pickerTemplateCursor++
		}
	case "g", "home":
		s.pickerTemplateCursor = 0
	case "G", "end":
		s.pickerTemplateCursor = len(templates) - 1
	case "enter":
		if s.pickerTemplateCursor >= 0 && s.pickerTemplateCursor < len(templates) {
			tpl := templates[s.pickerTemplateCursor]
			s.pickerTemplate = &tpl
			// Move on to filename input — pre-fill with template
			// default so the user can just press Enter to accept.
			defaultName := tpl.expandFilename(time.Now()) + ".csv"
			s.pickerNewName = []rune(defaultName)
			s.pickerTemplateMode = false
			s.pickerNewMode = true
		}
	}
	// Direct number-key shortcut: 1-9 immediately picks the
	// corresponding template (matching its display number).
	if k.Type == tea.KeyRunes && len(k.Runes) == 1 {
		r := k.Runes[0]
		if r >= '1' && r <= '9' {
			idx := int(r - '1')
			if idx < len(templates) {
				tpl := templates[idx]
				s.pickerTemplate = &tpl
				defaultName := tpl.expandFilename(time.Now()) + ".csv"
				s.pickerNewName = []rune(defaultName)
				s.pickerTemplateMode = false
				s.pickerNewMode = true
			}
		}
	}
	return s, nil
}

// updatePickerNewName handles filename input after a template
// has been chosen.
func (s SheetView) updatePickerNewName(k tea.KeyMsg) (SheetView, tea.Cmd) {
	switch k.String() {
	case "esc":
		// Step back to template picker (or close it altogether
		// if no template was chosen yet — defensive default).
		s.pickerNewMode = false
		s.pickerNewName = nil
		if s.pickerTemplate != nil {
			s.pickerTemplateMode = true
		}
		return s, nil
	case "enter":
		name := strings.TrimSpace(string(s.pickerNewName))
		if name == "" {
			return s, nil
		}
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".csv" && ext != ".tsv" && ext != ".xlsx" {
			name += ".csv"
		}
		abs := filepath.Join(s.pickerVaultRoot, name)
		s.pickerNewMode = false
		s.pickerNewName = nil

		// Open with template content if a template was picked.
		if s.pickerTemplate != nil && s.pickerTemplate.ID != "blank" {
			if err := s.openWithTemplate(abs, *s.pickerTemplate); err != nil {
				s.statusMsg = "Open failed: " + err.Error()
				s.statusAt = time.Now()
				s.mode = sheetModePicker
				return s, nil
			}
		} else {
			if err := s.Open(abs); err != nil {
				s.statusMsg = "Open failed: " + err.Error()
				s.statusAt = time.Now()
				s.mode = sheetModePicker
				return s, nil
			}
		}
		s.pickerTemplate = nil
		// Save immediately so the file persists and shows up in
		// the picker on next launch.
		_ = s.Save()
		return s, nil
	case "backspace":
		if len(s.pickerNewName) > 0 {
			s.pickerNewName = s.pickerNewName[:len(s.pickerNewName)-1]
		}
		return s, nil
	case "ctrl+u":
		// Clear filename (vim-style).
		s.pickerNewName = nil
		return s, nil
	}
	if k.Type == tea.KeyRunes {
		s.pickerNewName = append(s.pickerNewName, k.Runes...)
	} else if k.Type == tea.KeySpace {
		s.pickerNewName = append(s.pickerNewName, ' ')
	}
	return s, nil
}

// openWithTemplate seeds the sheet view with a template's row
// data instead of an empty file. The file is created on the
// next Save() call (consistent with how blank files work).
func (s *SheetView) openWithTemplate(path string, tpl sheetTemplate) error {
	s.Activate()
	s.filePath = path
	s.row = 0
	s.col = 0
	s.rowOff = 0
	s.colOff = 0
	s.activeSheet = 0
	s.mode = sheetModeNormal
	s.modified = true // template content needs to be persisted
	s.saveErr = nil
	s.selActive = false

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".tsv":
		s.fileType = "csv"
		s.delimiter = '\t'
	case ".xlsx", ".xlsm":
		s.fileType = "xlsx"
	default:
		s.fileType = "csv"
		s.delimiter = ','
	}

	rows := tpl.expandedRows(time.Now())
	s.sheets = []sheetData{{name: "Sheet1", rows: rows}}
	s.recomputeColumnKinds()
	s.recomputeColWidths()
	s.statusMsg = "New " + tpl.Name + " — Ctrl+S to save  ·  ? for help"
	s.statusAt = time.Now()
	return nil
}

func (s SheetView) updateNormal(k tea.KeyMsg) (SheetView, tea.Cmd) {
	key := k.String()
	// Pending y/n confirmation for destructive actions takes
	// priority over normal key handling — without this the
	// next keypress would do something unrelated and the
	// confirmation would silently expire.
	if s.confirmAction != "" {
		switch key {
		case "y", "Y":
			action := s.confirmAction
			target := s.confirmTarget
			s.confirmAction = ""
			s.confirmText = ""
			switch action {
			case "deleteCol":
				s.pushUndo()
				s.deleteCol(target)
				if s.col > s.numCols()-1 && s.col > 0 {
					s.col--
				}
				s.statusMsg = "Column deleted"
				s.statusAt = time.Now()
			case "deleteRow":
				s.pushUndo()
				s.deleteRow(target)
				if s.row > s.numRows()-1 && s.row > 0 {
					s.row--
				}
				s.statusMsg = "Row deleted"
				s.statusAt = time.Now()
			}
			return s, nil
		default:
			s.confirmAction = ""
			s.confirmText = ""
			s.statusMsg = "Cancelled"
			s.statusAt = time.Now()
			return s, nil
		}
	}
	switch key {
	case "left", "h":
		if s.col > 0 {
			s.col--
		}
	case "right", "l":
		if s.col < s.numCols()-1 {
			s.col++
		}
	case "up", "k":
		if s.row > 0 {
			s.row--
		}
	case "down", "j":
		if s.row < s.numRows()-1 {
			s.row++
		}
	case "pgup":
		s.row -= s.viewportRows()
		if s.row < 0 {
			s.row = 0
		}
	case "pgdown":
		s.row += s.viewportRows()
		if s.row > s.numRows()-1 {
			s.row = s.numRows() - 1
		}
	case "home":
		s.col = 0
	case "end":
		s.col = s.numCols() - 1
	case "g":
		s.row = 0
	case "G":
		s.row = s.numRows() - 1
	case "0":
		s.col = 0
	case "$":
		s.col = s.numCols() - 1
	case "ctrl+home":
		s.row, s.col = 0, 0
	case "ctrl+end":
		s.row, s.col = s.numRows()-1, s.numCols()-1
	case "i", "enter", "f2":
		s.pushUndo()
		s.beginEdit(false)
	case "I":
		s.pushUndo()
		s.beginEdit(true) // start with cleared cell
	case "a":
		// edit at end (mirrors vim 'a')
		s.pushUndo()
		s.beginEdit(false)
		s.editCursor = len(s.editBuf)
	case "x", "delete":
		s.pushUndo()
		s.setCell(s.row, s.col, "")
		s.recomputeColumnKinds()
		s.recomputeColWidths()
	case "o":
		s.pushUndo()
		s.insertRow(s.row + 1)
		s.row++
	case "O":
		s.pushUndo()
		s.insertRow(s.row)
	case "d":
		// Confirm when row has lots of content. Two-cell rows
		// are auto-deleted because re-typing them is trivial.
		if filled := s.rowFilledCount(s.row); filled > 2 {
			s.confirmAction = "deleteRow"
			s.confirmTarget = s.row
			s.confirmText = fmt.Sprintf("Delete row %d with %d values? (y/N)", s.row+1, filled)
			s.statusMsg = s.confirmText
			s.statusAt = time.Now()
			break
		}
		s.pushUndo()
		s.deleteRow(s.row)
		if s.row > s.numRows()-1 && s.row > 0 {
			s.row--
		}
	case "D":
		// Destructive — count non-empty cells; ask for y/n
		// confirmation when the column has more than 2 values.
		// Below that threshold the column is cheap to recreate
		// and undo is one keystroke away.
		if filled := s.columnFilledCount(s.col); filled > 2 {
			s.confirmAction = "deleteCol"
			s.confirmTarget = s.col
			s.confirmText = fmt.Sprintf("Delete column %s with %d values? (y/N)",
				colLetters(s.col), filled)
			s.statusMsg = s.confirmText
			s.statusAt = time.Now()
			break
		}
		s.pushUndo()
		s.deleteCol(s.col)
		if s.col > s.numCols()-1 && s.col > 0 {
			s.col--
		}
	case "+":
		s.pushUndo()
		s.insertCol(s.col + 1)
		s.col++
	case "-":
		s.pushUndo()
		s.insertCol(s.col)
	case "T":
		// Total row — append (or remove) a "TOTAL" row at the
		// bottom that auto-sums numeric columns. The row is
		// editable like any other; deleting the bottom row
		// removes it.
		if s.numRows() > 0 && strings.TrimSpace(s.cell(s.numRows()-1, 0)) == "TOTAL" {
			// Looks like a previously-added total row — remove it.
			s.pushUndo()
			s.deleteRow(s.numRows() - 1)
			if s.row >= s.numRows() {
				s.row = s.numRows() - 1
			}
			s.statusMsg = "TOTAL row removed"
		} else {
			s.pushUndo()
			cols := s.numCols()
			row := make([]string, cols)
			row[0] = "TOTAL"
			startRow := 0
			if s.headerIsLabel {
				startRow = 1
			}
			for c := 1; c < cols; c++ {
				sum := 0.0
				any := false
				for r := startRow; r < s.numRows(); r++ {
					if v, ok := parseCellNumeric(s.cell(r, c)); ok {
						sum += v
						any = true
					}
				}
				if any {
					row[c] = strconv.FormatFloat(sum, 'f', svDetectDecimalsForCol(&s, c), 64)
				}
			}
			s.sheet().rows = append(s.sheet().rows, row)
			s.statusMsg = "TOTAL row added — edit or delete (d) to remove"
		}
		s.statusAt = time.Now()
		s.recomputeColumnKinds()
		s.recomputeColWidths()
	case "Y":
		// Fill down — copy the active cell into all empty cells
		// directly below it in the same column, stopping at the
		// first non-empty cell. Saves a ton of typing when
		// labelling categories ("Income", "Income", "Income"…).
		src := s.cell(s.row, s.col)
		if src == "" {
			s.statusMsg = "fill down: source cell is empty"
			s.statusAt = time.Now()
			break
		}
		s.pushUndo()
		filled := 0
		for r := s.row + 1; r < s.numRows(); r++ {
			if strings.TrimSpace(s.cell(r, s.col)) != "" {
				break
			}
			s.setCell(r, s.col, src)
			filled++
		}
		if filled == 0 {
			s.statusMsg = "fill down: no empty cells below"
		} else {
			s.statusMsg = fmt.Sprintf("Filled %d cells with %q", filled, src)
		}
		s.statusAt = time.Now()
		s.recomputeColumnKinds()
		s.recomputeColWidths()
	case "=":
		// Auto-fit current column to its widest cell, ignoring
		// the [4..28] clamp recomputeColWidths normally applies.
		// Useful when a column's content was truncated and you
		// want to see the full value without manually resizing.
		s.autoFitColumn(s.col)
		s.statusMsg = fmt.Sprintf("Auto-fit column %s", colLetters(s.col))
		s.statusAt = time.Now()
	case "u":
		// Vim-style undo
		if s.undo() {
			s.statusMsg = "undo"
		} else {
			s.statusMsg = "nothing to undo"
		}
		s.statusAt = time.Now()
	case "ctrl+r":
		// Vim-style redo
		if s.redo() {
			s.statusMsg = "redo"
		} else {
			s.statusMsg = "nothing to redo"
		}
		s.statusAt = time.Now()
	case "ctrl+s":
		if err := s.Save(); err != nil {
			s.statusMsg = "Save failed: " + err.Error()
			s.statusAt = time.Now()
		}
	case "tab":
		if len(s.sheets) > 1 {
			s.activeSheet = (s.activeSheet + 1) % len(s.sheets)
			s.row, s.col = 0, 0
			s.rowOff, s.colOff = 0, 0
			s.recomputeColumnKinds()
			s.recomputeColWidths()
		}
	case "shift+tab":
		if len(s.sheets) > 1 {
			s.activeSheet--
			if s.activeSheet < 0 {
				s.activeSheet = len(s.sheets) - 1
			}
			s.row, s.col = 0, 0
			s.rowOff, s.colOff = 0, 0
			s.recomputeColumnKinds()
			s.recomputeColWidths()
		}
	case "c":
		// Toggle chart panel for the current column. If the
		// column isn't numeric, find the nearest numeric column
		// to the right so the panel still shows something
		// meaningful instead of collapsing immediately.
		s.showChart = !s.showChart
		if s.showChart {
			s.chartCol = s.col
			if s.colKind(s.chartCol) != sheetColNumber &&
				s.colKind(s.chartCol) != sheetColCurrency &&
				s.colKind(s.chartCol) != sheetColPercent {
				if alt := s.firstNumericCol(); alt >= 0 {
					s.chartCol = alt
				}
			}
		}
	case "C":
		// Cycle chart type when the panel is open.
		if s.showChart {
			s.chartType = (s.chartType + 1) % 3
		}
	case "H":
		// Toggle "treat row 0 as header" for stats/charts.
		s.headerIsLabel = !s.headerIsLabel
		s.recomputeColumnKinds()
	case "e":
		// Toggle empty-cell · placeholder.
		s.showEmpty = !s.showEmpty
		if s.showEmpty {
			s.statusMsg = "showing empty placeholders"
		} else {
			s.statusMsg = "hiding empty placeholders"
		}
		s.statusAt = time.Now()
	case "L":
		// Toggle vertical column separators.
		s.gridLines = !s.gridLines
	case "F":
		// Toggle frozen header row.
		s.freezeHeader = !s.freezeHeader
		if s.freezeHeader {
			s.statusMsg = "header row frozen"
		} else {
			s.statusMsg = "header unfrozen"
		}
		s.statusAt = time.Now()
	case "s":
		// Cycle sort: none → asc → desc → none, on the active column.
		s.pushUndo()
		if s.sortCol != s.col {
			s.sortCol = s.col
			s.sortDir = 1
		} else {
			switch s.sortDir {
			case 1:
				s.sortDir = -1
			case -1:
				s.sortDir = 0
				s.sortCol = -1
			default:
				s.sortDir = 1
			}
		}
		s.applySort()
		switch s.sortDir {
		case 1:
			s.statusMsg = "sorted ↑ by " + colLetters(s.sortCol)
		case -1:
			s.statusMsg = "sorted ↓ by " + colLetters(s.sortCol)
		default:
			s.statusMsg = "sort cleared"
		}
		s.statusAt = time.Now()
	case "/":
		s.mode = sheetModeFind
		s.findBuf = nil
	case ":":
		s.mode = sheetModeGoto
		s.gotoBuf = nil
	case "n":
		// Repeat last find.
		if len(s.findBuf) > 0 {
			s.findNext(string(s.findBuf))
		}
	case "v":
		// Vim-style visual mode — toggle a rectangular cell
		// selection anchored at the current cursor. Move with
		// hjkl/arrows to extend; press 'y' to yank the range,
		// 'x' to clear it, 'Esc' to dismiss.
		if s.selVisualMode {
			s.selVisualMode = false
			s.selActive = false
		} else {
			s.selVisualMode = true
			s.selActive = true
			s.selStartRow, s.selStartCol = s.row, s.col
			s.selEndRow, s.selEndCol = s.row, s.col
			s.statusMsg = "visual mode — y yank · x clear · Esc cancel"
			s.statusAt = time.Now()
		}
	case "y":
		// Yank: rectangular range when visual mode is active,
		// otherwise just the active cell. Selection is rendered
		// as TSV so it pastes cleanly into spreadsheets.
		if s.selActive {
			tsv := s.selectionAsTSV()
			n := strings.Count(tsv, "\t") + strings.Count(tsv, "\n") + 1
			_ = ClipboardCopy(tsv)
			s.statusMsg = fmt.Sprintf("Yanked %d cells", n)
			s.statusAt = time.Now()
			s.selVisualMode = false
			s.selActive = false
		} else {
			v := s.cell(s.row, s.col)
			if v != "" {
				_ = ClipboardCopy(v)
				s.statusMsg = "Yanked: " + v
				s.statusAt = time.Now()
			}
		}
	case "p":
		if data, err := ClipboardPaste(); err == nil && data != "" {
			s.pushUndo()
			// Multi-cell paste: TSV / CSV / newline-delimited.
			s.pasteAt(s.row, s.col, data)
			s.recomputeColumnKinds()
			s.recomputeColWidths()
		}
	case "esc":
		if s.showChart {
			s.showChart = false
		} else if s.selActive {
			s.selActive = false
			s.selVisualMode = false
		} else if s.findMatchCount > 0 {
			s.findMatchCount = 0
		}
	case "?":
		// Open the keyboard reference overlay. Bound to ? in
		// normal mode (matches vim/less convention).
		s.helpVisible = true
		s.helpScroll = 0
	case "q":
		s.pendingClose = true
	case "ctrl+q":
		s.pendingClose = true
	}
	s.clampCursor()
	// Extend visual selection when cursor moves while in visual
	// mode — done AFTER clampCursor so the end stays in-bounds.
	if s.selVisualMode {
		s.selEndRow, s.selEndCol = s.row, s.col
	}
	s.adjustViewport()
	return s, nil
}

// selectionAsTSV serialises the active selection to tab-
// separated values, one row per line, suitable for pasting
// into another spreadsheet (Excel/Numbers/Google Sheets).
func (s *SheetView) selectionAsTSV() string {
	r0, r1 := s.selStartRow, s.selEndRow
	c0, c1 := s.selStartCol, s.selEndCol
	if r0 > r1 {
		r0, r1 = r1, r0
	}
	if c0 > c1 {
		c0, c1 = c1, c0
	}
	var b strings.Builder
	for r := r0; r <= r1; r++ {
		for c := c0; c <= c1; c++ {
			if c > c0 {
				b.WriteByte('\t')
			}
			b.WriteString(s.cell(r, c))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// inSelection reports whether (r, c) sits inside the active
// rectangular selection (regardless of which corner is the
// anchor vs the moving end).
func (s *SheetView) inSelection(r, c int) bool {
	if !s.selActive {
		return false
	}
	r0, r1 := s.selStartRow, s.selEndRow
	c0, c1 := s.selStartCol, s.selEndCol
	if r0 > r1 {
		r0, r1 = r1, r0
	}
	if c0 > c1 {
		c0, c1 = c1, c0
	}
	return r >= r0 && r <= r1 && c >= c0 && c <= c1
}

func (s SheetView) updateEdit(k tea.KeyMsg) (SheetView, tea.Cmd) {
	switch k.String() {
	case "esc":
		s.mode = sheetModeNormal
		s.editBuf = nil
		s.editCursor = 0
		return s, nil
	case "enter":
		s.commitEdit()
		// Move cursor down so rapid data entry feels right.
		if s.row < s.numRows()-1 {
			s.row++
		} else {
			s.insertRow(s.numRows())
			s.row++
		}
		s.adjustViewport()
		return s, nil
	case "tab":
		s.commitEdit()
		if s.col < s.numCols()-1 {
			s.col++
		} else {
			s.insertCol(s.numCols())
			s.col++
		}
		s.adjustViewport()
		return s, nil
	case "shift+tab":
		s.commitEdit()
		if s.col > 0 {
			s.col--
		}
		s.adjustViewport()
		return s, nil
	case "left":
		if s.editCursor > 0 {
			s.editCursor--
		}
		return s, nil
	case "right":
		if s.editCursor < len(s.editBuf) {
			s.editCursor++
		}
		return s, nil
	case "home":
		s.editCursor = 0
		return s, nil
	case "end":
		s.editCursor = len(s.editBuf)
		return s, nil
	case "backspace":
		if s.editCursor > 0 {
			s.editBuf = append(s.editBuf[:s.editCursor-1], s.editBuf[s.editCursor:]...)
			s.editCursor--
		}
		return s, nil
	case "delete":
		if s.editCursor < len(s.editBuf) {
			s.editBuf = append(s.editBuf[:s.editCursor], s.editBuf[s.editCursor+1:]...)
		}
		return s, nil
	}
	if k.Type == tea.KeyRunes {
		s.editBuf = append(s.editBuf[:s.editCursor],
			append(append([]rune{}, k.Runes...), s.editBuf[s.editCursor:]...)...)
		s.editCursor += len(k.Runes)
	} else if k.Type == tea.KeySpace {
		s.editBuf = append(s.editBuf[:s.editCursor],
			append([]rune{' '}, s.editBuf[s.editCursor:]...)...)
		s.editCursor++
	}
	return s, nil
}

func (s SheetView) updateFind(k tea.KeyMsg) (SheetView, tea.Cmd) {
	switch k.String() {
	case "esc":
		s.mode = sheetModeNormal
		s.findBuf = nil
		return s, nil
	case "enter":
		s.mode = sheetModeNormal
		s.findNext(string(s.findBuf))
		return s, nil
	case "backspace":
		if len(s.findBuf) > 0 {
			s.findBuf = s.findBuf[:len(s.findBuf)-1]
		}
		return s, nil
	}
	if k.Type == tea.KeyRunes {
		s.findBuf = append(s.findBuf, k.Runes...)
	} else if k.Type == tea.KeySpace {
		s.findBuf = append(s.findBuf, ' ')
	}
	return s, nil
}

func (s SheetView) updateGoto(k tea.KeyMsg) (SheetView, tea.Cmd) {
	switch k.String() {
	case "esc":
		s.mode = sheetModeNormal
		s.gotoBuf = nil
		return s, nil
	case "enter":
		s.mode = sheetModeNormal
		ref := strings.ToUpper(strings.TrimSpace(string(s.gotoBuf)))
		s.gotoBuf = nil
		if r, c, ok := parseA1(ref); ok {
			if r >= 0 && r < s.numRows() && c >= 0 && c < s.numCols() {
				s.row, s.col = r, c
				s.adjustViewport()
			} else {
				s.statusMsg = "Out of range: " + ref
				s.statusAt = time.Now()
			}
		} else {
			s.statusMsg = "Bad address: " + ref
			s.statusAt = time.Now()
		}
		return s, nil
	case "backspace":
		if len(s.gotoBuf) > 0 {
			s.gotoBuf = s.gotoBuf[:len(s.gotoBuf)-1]
		}
		return s, nil
	}
	if k.Type == tea.KeyRunes {
		s.gotoBuf = append(s.gotoBuf, k.Runes...)
	}
	return s, nil
}

func (s *SheetView) beginEdit(clear bool) {
	if clear {
		s.editBuf = nil
	} else {
		s.editBuf = []rune(s.cell(s.row, s.col))
	}
	s.editCursor = len(s.editBuf)
	s.mode = sheetModeEdit
}

func (s *SheetView) commitEdit() {
	s.setCell(s.row, s.col, string(s.editBuf))
	s.editBuf = nil
	s.editCursor = 0
	s.mode = sheetModeNormal
	s.recomputeColumnKinds()
	s.recomputeColWidths()
}

func (s *SheetView) insertRow(at int) {
	sh := s.sheet()
	if sh == nil {
		return
	}
	width := s.numCols()
	if width == 0 {
		width = 1
	}
	row := make([]string, width)
	if at < 0 {
		at = 0
	}
	if at > len(sh.rows) {
		at = len(sh.rows)
	}
	sh.rows = append(sh.rows, nil)
	copy(sh.rows[at+1:], sh.rows[at:])
	sh.rows[at] = row
	s.modified = true
}

func (s *SheetView) deleteRow(at int) {
	sh := s.sheet()
	if sh == nil || at < 0 || at >= len(sh.rows) {
		return
	}
	if len(sh.rows) <= 1 {
		// Don't allow zero-row sheets — clear the cells instead.
		for c := range sh.rows[0] {
			sh.rows[0][c] = ""
		}
		s.modified = true
		return
	}
	sh.rows = append(sh.rows[:at], sh.rows[at+1:]...)
	s.modified = true
}

func (s *SheetView) insertCol(at int) {
	sh := s.sheet()
	if sh == nil {
		return
	}
	for r := range sh.rows {
		if at < 0 {
			at = 0
		}
		if at > len(sh.rows[r]) {
			at = len(sh.rows[r])
		}
		sh.rows[r] = append(sh.rows[r], "")
		copy(sh.rows[r][at+1:], sh.rows[r][at:])
		sh.rows[r][at] = ""
	}
	s.modified = true
	s.recomputeColumnKinds()
	s.recomputeColWidths()
}

func (s *SheetView) deleteCol(at int) {
	sh := s.sheet()
	if sh == nil || at < 0 {
		return
	}
	for r := range sh.rows {
		if at >= len(sh.rows[r]) {
			continue
		}
		if len(sh.rows[r]) <= 1 {
			sh.rows[r][0] = ""
			continue
		}
		sh.rows[r] = append(sh.rows[r][:at], sh.rows[r][at+1:]...)
	}
	s.modified = true
	s.recomputeColumnKinds()
	s.recomputeColWidths()
}

func (s *SheetView) findNext(q string) {
	if q == "" {
		return
	}
	qLower := strings.ToLower(q)
	rows := s.numRows()
	cols := s.numCols()

	// Count total matches across the whole sheet (cheap — string
	// scan is O(n) and we render the count in the status bar so
	// the user knows whether 'n' will keep cycling).
	count := 0
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			if strings.Contains(strings.ToLower(s.cell(r, c)), qLower) {
				count++
			}
		}
	}
	s.findMatchCount = count
	if count == 0 {
		s.statusMsg = "no match: " + q
		s.statusAt = time.Now()
		return
	}

	startR, startC := s.row, s.col+1
	for r := startR; r < rows; r++ {
		for c := 0; c < cols; c++ {
			if r == startR && c < startC {
				continue
			}
			if strings.Contains(strings.ToLower(s.cell(r, c)), qLower) {
				s.row, s.col = r, c
				s.adjustViewport()
				s.statusMsg = fmt.Sprintf("/%s — %d match%s",
					q, count, svPlural(count))
				s.statusAt = time.Now()
				return
			}
		}
	}
	// Wrap.
	for r := 0; r <= startR; r++ {
		for c := 0; c < cols; c++ {
			if r == startR && c >= startC {
				break
			}
			if strings.Contains(strings.ToLower(s.cell(r, c)), qLower) {
				s.row, s.col = r, c
				s.adjustViewport()
				s.statusMsg = fmt.Sprintf("/%s — wrapped, %d match%s",
					q, count, svPlural(count))
				s.statusAt = time.Now()
				return
			}
		}
	}
}

func svPlural(n int) string {
	if n == 1 {
		return ""
	}
	return "es"
}

// pasteAt writes data into the grid starting at (r, c). data is
// parsed as TSV first (Excel-friendly), falling back to CSV if no
// tabs are present.
func (s *SheetView) pasteAt(r, c int, data string) {
	delim := '\t'
	if !strings.Contains(data, "\t") {
		delim = ','
	}
	rd := csv.NewReader(strings.NewReader(data))
	rd.Comma = delim
	rd.FieldsPerRecord = -1
	rd.LazyQuotes = true
	rows, err := rd.ReadAll()
	if err != nil || len(rows) == 0 {
		// Single value paste.
		s.setCell(r, c, data)
		return
	}
	for ri, row := range rows {
		for ci, val := range row {
			s.setCell(r+ri, c+ci, val)
		}
	}
}

// sheetSnapshot captures enough state to restore a sheet view
// after a mutation. Cursor + colWidths are included so undo
// feels seamless (cursor jumps back to where the change was).
type sheetSnapshot struct {
	sheetIdx int
	rows     [][]string
	row, col int
	rowOff   int
	colOff   int
}

const sheetUndoLimit = 100

// pushUndo snapshots the active sheet's state onto the undo
// stack, clears redo (anything done after an undo invalidates
// the redo branch — vim/most-editor convention), and trims to
// the limit. Call BEFORE every mutation.
func (s *SheetView) pushUndo() {
	sh := s.sheet()
	if sh == nil {
		return
	}
	snap := sheetSnapshot{
		sheetIdx: s.activeSheet,
		rows:     deepCopyRows(sh.rows),
		row:      s.row,
		col:      s.col,
		rowOff:   s.rowOff,
		colOff:   s.colOff,
	}
	s.undoStack = append(s.undoStack, snap)
	if len(s.undoStack) > sheetUndoLimit {
		s.undoStack = s.undoStack[len(s.undoStack)-sheetUndoLimit:]
	}
	s.redoStack = nil
}

// undo pops the most recent snapshot and pushes the current
// state onto the redo stack. No-op when undo stack is empty.
func (s *SheetView) undo() bool {
	if len(s.undoStack) == 0 {
		return false
	}
	last := s.undoStack[len(s.undoStack)-1]
	s.undoStack = s.undoStack[:len(s.undoStack)-1]
	// Save current state on redo BEFORE restoring.
	if sh := s.sheet(); sh != nil {
		s.redoStack = append(s.redoStack, sheetSnapshot{
			sheetIdx: s.activeSheet,
			rows:     deepCopyRows(sh.rows),
			row:      s.row,
			col:      s.col,
			rowOff:   s.rowOff,
			colOff:   s.colOff,
		})
	}
	s.restoreSnapshot(last)
	return true
}

// redo pops the most recent redo snapshot and pushes current
// onto undo. No-op when redo stack is empty.
func (s *SheetView) redo() bool {
	if len(s.redoStack) == 0 {
		return false
	}
	last := s.redoStack[len(s.redoStack)-1]
	s.redoStack = s.redoStack[:len(s.redoStack)-1]
	if sh := s.sheet(); sh != nil {
		s.undoStack = append(s.undoStack, sheetSnapshot{
			sheetIdx: s.activeSheet,
			rows:     deepCopyRows(sh.rows),
			row:      s.row,
			col:      s.col,
			rowOff:   s.rowOff,
			colOff:   s.colOff,
		})
	}
	s.restoreSnapshot(last)
	return true
}

func (s *SheetView) restoreSnapshot(snap sheetSnapshot) {
	if snap.sheetIdx < 0 || snap.sheetIdx >= len(s.sheets) {
		return
	}
	s.activeSheet = snap.sheetIdx
	s.sheets[snap.sheetIdx].rows = snap.rows
	s.row = snap.row
	s.col = snap.col
	s.rowOff = snap.rowOff
	s.colOff = snap.colOff
	s.modified = true
	s.recomputeColumnKinds()
	s.recomputeColWidths()
	s.clampCursor()
	s.adjustViewport()
}

func deepCopyRows(rows [][]string) [][]string {
	out := make([][]string, len(rows))
	for i, r := range rows {
		nr := make([]string, len(r))
		copy(nr, r)
		out[i] = nr
	}
	return out
}

// applySort stable-sorts the active sheet's data rows by the
// configured sort column and direction. The header row (when
// headerIsLabel) is held in place. Numeric columns sort
// numerically; everything else falls back to case-insensitive
// string comparison.
func (s *SheetView) applySort() {
	sh := s.sheet()
	if sh == nil || s.sortDir == 0 || s.sortCol < 0 {
		return
	}
	startRow := 0
	if s.headerIsLabel && len(sh.rows) > 0 {
		startRow = 1
	}
	if startRow >= len(sh.rows) {
		return
	}
	col := s.sortCol
	asc := s.sortDir > 0
	rows := sh.rows[startRow:]
	kind := s.colKind(col)
	numeric := kind == sheetColNumber || kind == sheetColCurrency || kind == sheetColPercent
	sort.SliceStable(rows, func(i, j int) bool {
		var a, b string
		if col < len(rows[i]) {
			a = rows[i][col]
		}
		if col < len(rows[j]) {
			b = rows[j][col]
		}
		if numeric {
			av, aok := parseCellNumeric(a)
			bv, bok := parseCellNumeric(b)
			switch {
			case aok && bok:
				if asc {
					return av < bv
				}
				return av > bv
			case aok:
				return asc
			case bok:
				return !asc
			}
		}
		al := strings.ToLower(a)
		bl := strings.ToLower(b)
		if asc {
			return al < bl
		}
		return al > bl
	})
	s.modified = true
}

// isSectionHeaderRow reports whether row r looks like a section
// banner: column A has content but every other column is empty.
// These get a distinct render style so multi-section sheets feel
// structured rather than a wall of text.
func (s *SheetView) isSectionHeaderRow(r int) bool {
	sh := s.sheet()
	if sh == nil || r < 0 || r >= len(sh.rows) {
		return false
	}
	if r == 0 && s.headerIsLabel {
		return false // proper header row, not a section banner
	}
	row := sh.rows[r]
	if len(row) == 0 {
		return false
	}
	if strings.TrimSpace(row[0]) == "" {
		return false
	}
	for i := 1; i < len(row); i++ {
		if strings.TrimSpace(row[i]) != "" {
			return false
		}
	}
	return true
}

// columnFilledCount returns how many cells in column c have
// non-empty content. Used for the destructive-delete prompt
// threshold so the user is only interrupted on column delete
// when there's real data to lose.
func (s *SheetView) columnFilledCount(c int) int {
	n := 0
	for r := 0; r < s.numRows(); r++ {
		if strings.TrimSpace(s.cell(r, c)) != "" {
			n++
		}
	}
	return n
}

func (s *SheetView) rowFilledCount(r int) int {
	n := 0
	for c := 0; c < s.numCols(); c++ {
		if strings.TrimSpace(s.cell(r, c)) != "" {
			n++
		}
	}
	return n
}

func (s *SheetView) firstNumericCol() int {
	for c := 0; c < s.numCols(); c++ {
		k := s.colKind(c)
		if k == sheetColNumber || k == sheetColCurrency || k == sheetColPercent {
			return c
		}
	}
	return -1
}

func (s *SheetView) clampCursor() {
	if s.numRows() == 0 {
		s.row = 0
	} else if s.row > s.numRows()-1 {
		s.row = s.numRows() - 1
	}
	if s.numCols() == 0 {
		s.col = 0
	} else if s.col > s.numCols()-1 {
		s.col = s.numCols() - 1
	}
	if s.row < 0 {
		s.row = 0
	}
	if s.col < 0 {
		s.col = 0
	}
}

func (s *SheetView) viewportRows() int {
	// Reserve rows for: file bar (1) + formula bar (1) +
	// column ruler (1) + stats (1) + status (1) = 5.
	// Sheet tabs (1) and chart (variable) are conditional.
	h := s.height
	if h <= 0 {
		return 10
	}
	overhead := 5
	if len(s.sheets) > 1 {
		overhead++ // sheet tabs row
	}
	if s.showChart {
		overhead += s.chartHeight() + 1
	}
	if h-overhead < 1 {
		return 1
	}
	return h - overhead
}

func (s *SheetView) chartHeight() int {
	if s.height <= 0 {
		return 8
	}
	want := s.height / 3
	if want < 6 {
		want = 6
	}
	if want > 14 {
		want = 14
	}
	return want
}

func (s *SheetView) adjustViewport() {
	vpRows := s.viewportRows()
	if s.row < s.rowOff {
		s.rowOff = s.row
	}
	if s.row >= s.rowOff+vpRows {
		s.rowOff = s.row - vpRows + 1
	}
	if s.rowOff < 0 {
		s.rowOff = 0
	}
	// Column scroll: ensure active column fits in available width.
	avail := s.gridWidth() - 6 // 6 for row gutter
	if avail < 1 {
		avail = 1
	}
	// If active col is to the left of viewport, scroll left.
	if s.col < s.colOff {
		s.colOff = s.col
	}
	// Find the rightmost column that fits given current colOff.
	for {
		w := 0
		for c := s.colOff; c <= s.col && c < s.numCols(); c++ {
			w += s.colWidths[c] + 1
		}
		if w <= avail || s.colOff >= s.col {
			break
		}
		s.colOff++
	}
	if s.colOff < 0 {
		s.colOff = 0
	}
}

func (s *SheetView) gridWidth() int {
	w := s.width
	if w <= 0 {
		w = 80
	}
	if s.showChart {
		// reserve 28 cols for the chart panel; collapse if too narrow
		if w > 60 {
			w -= 30
		}
	}
	return w
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// Sheet style cache. The vars are package-scoped (so the hot
// render loop doesn't allocate a new lipgloss.Style per cell),
// but they're rebuilt by refreshSheetStyles() at the top of every
// View() call so a theme switch (which rewrites the package-wide
// colour vars in styles.go) takes effect on the next paint
// without an app restart.
//
// Visual hierarchy (lowest → highest contrast):
//   normal cell    → base background
//   zebra cell     → mantle background
//   crosshair tint → surface1 background
//   header row     → surface2 background + bold + bright text
//   section header → mauve foreground bold over surface0
//   active cell    → mauve background + base text + bold
//   editing cell   → yellow background + base text + bold
var (
	svStyleHeaderBar       lipgloss.Style
	svStyleColRuler        lipgloss.Style
	svStyleColRulerActive  lipgloss.Style
	svStyleColRulerCross   lipgloss.Style
	svStyleRowGutter       lipgloss.Style
	svStyleRowGutterActive lipgloss.Style
	svStyleRowGutterCross  lipgloss.Style
	svStyleHeaderRow       lipgloss.Style
	svStyleZebra           lipgloss.Style
	svStyleNormal          lipgloss.Style
	svStyleCellActive      lipgloss.Style
	svStyleCellEditing     lipgloss.Style
	svStyleCellCrosshair   lipgloss.Style
	svStyleEmpty           lipgloss.Style
	svStyleSeparator       lipgloss.Style
)

func init() { refreshSheetStyles() }

// refreshSheetStyles re-derives the sheet view's style cache
// from the live theme palette. Called by View() so theme swaps
// land immediately. Light themes (catppuccin-latte) get the
// inverted contrast for free because the underlying base/mantle/
// surface0 colors flip from dark to light.
func refreshSheetStyles() {
	svStyleHeaderBar = lipgloss.NewStyle().Background(surface0).Foreground(text)
	svStyleColRuler = lipgloss.NewStyle().Background(surface1).Foreground(subtext1).Bold(true)
	svStyleColRulerActive = lipgloss.NewStyle().Background(mauve).Foreground(base).Bold(true)
	svStyleColRulerCross = lipgloss.NewStyle().Background(surface2).Foreground(text).Bold(true)
	svStyleRowGutter = lipgloss.NewStyle().Background(surface1).Foreground(subtext1)
	svStyleRowGutterActive = lipgloss.NewStyle().Background(mauve).Foreground(base).Bold(true)
	svStyleRowGutterCross = lipgloss.NewStyle().Background(surface2).Foreground(text).Bold(true)
	svStyleHeaderRow = lipgloss.NewStyle().Background(surface2).Foreground(text).Bold(true)
	svStyleZebra = lipgloss.NewStyle().Background(mantle)
	svStyleNormal = lipgloss.NewStyle().Background(base)
	svStyleCellActive = lipgloss.NewStyle().Background(mauve).Foreground(base).Bold(true)
	svStyleCellEditing = lipgloss.NewStyle().Background(yellow).Foreground(base).Bold(true)
	svStyleCellCrosshair = lipgloss.NewStyle().Background(surface1)
	svStyleEmpty = lipgloss.NewStyle().Foreground(overlay0)
	svStyleSeparator = lipgloss.NewStyle().Foreground(surface2)
}

func (s SheetView) View() string {
	if !s.IsActive() {
		return ""
	}
	// Snapshot live theme palette into the sheet's style cache so
	// theme switches take effect on the next paint without a
	// restart. Cheap (15 value-type style allocations).
	refreshSheetStyles()
	if s.mode == sheetModePicker {
		return s.renderPicker()
	}
	if s.numCols() == 0 || s.numRows() == 0 {
		return s.renderEmptyState()
	}
	width := s.width
	if width <= 0 {
		width = 80
	}
	height := s.height
	if height <= 0 {
		height = 24
	}

	header := s.renderHeader(width)
	sheetTabs := ""
	if len(s.sheets) > 1 {
		sheetTabs = s.renderSheetTabs(width)
	}
	gridW := s.gridWidth()
	grid := s.renderGrid(gridW)
	stats := s.renderStats(width)
	statusLine := s.renderStatus(width)

	var body string
	if s.showChart {
		chart := s.renderChartPanel(width - gridW - 1)
		// Two columns: grid on left, chart on right.
		body = lipgloss.JoinHorizontal(lipgloss.Top, grid,
			lipgloss.NewStyle().Background(base).Width(1).Render(" "), chart)
	} else {
		body = grid
	}

	pieces := []string{header}
	if sheetTabs != "" {
		pieces = append(pieces, sheetTabs)
	}
	pieces = append(pieces, body, stats, statusLine)
	out := lipgloss.JoinVertical(lipgloss.Left, pieces...)
	// Truncate to height to avoid pushing other UI out of view.
	lines := strings.Split(out, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	rendered := strings.Join(lines, "\n")

	// Help overlay sits on top of everything when visible —
	// rendered last so it composites cleanly over the grid.
	if s.helpVisible {
		rendered = svOverlayCenter(rendered, s.renderHelp(width, height), width, height)
	}
	return rendered
}

// svOverlayCenter composites a centered overlay panel over the
// already-rendered backdrop, replacing the lines underneath
// while preserving everything outside the overlay's footprint.
// Lipgloss's Place works on plain strings; here we just splice
// the overlay rows over the backdrop rows so a partially-drawn
// background (status bar, etc.) stays visible at the edges.
func svOverlayCenter(backdrop, overlay string, width, height int) string {
	bgLines := strings.Split(backdrop, "\n")
	for len(bgLines) < height {
		bgLines = append(bgLines, "")
	}
	ovLines := strings.Split(overlay, "\n")
	ovH := len(ovLines)
	ovW := 0
	for _, l := range ovLines {
		if w := lipgloss.Width(l); w > ovW {
			ovW = w
		}
	}
	top := (height - ovH) / 2
	if top < 0 {
		top = 0
	}
	left := (width - ovW) / 2
	if left < 0 {
		left = 0
	}
	for i, ol := range ovLines {
		row := top + i
		if row < 0 || row >= len(bgLines) {
			continue
		}
		// Splice: pad backdrop to `left`, then overlay, then
		// remaining backdrop tail (if overlay is narrower than
		// backdrop). Use lipgloss.Width to slice ANSI-aware.
		bgLine := bgLines[row]
		bgW := lipgloss.Width(bgLine)
		leftPad := ""
		if left > 0 {
			if bgW >= left {
				leftPad = svAnsiSliceLeft(bgLine, left)
			} else {
				leftPad = bgLine + strings.Repeat(" ", left-bgW)
			}
		}
		bgLines[row] = leftPad + ol
	}
	return strings.Join(bgLines, "\n")
}

// svAnsiSliceLeft returns the first `n` display columns of an
// ANSI-styled string. Naive — sufficient for our overlay use
// where we just need column-accurate left-pad slicing.
func svAnsiSliceLeft(s string, n int) string {
	if n <= 0 {
		return ""
	}
	var b strings.Builder
	cols := 0
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			b.WriteRune(r)
			continue
		}
		if inEsc {
			b.WriteRune(r)
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		if cols >= n {
			break
		}
		b.WriteRune(r)
		cols++
	}
	return b.String()
}

// renderHelp draws the keyboard-reference overlay. Layout:
// two side-by-side columns of grouped keybinds. The whole panel
// scrolls when the window is short — but in a 24-row terminal
// the most-useful sections (Navigation, Editing, Format, View)
// all fit at once.
func (s *SheetView) renderHelp(viewportW, viewportH int) string {
	type group struct {
		title string
		items [][2]string
	}
	leftGroups := []group{
		{"Navigation", [][2]string{
			{"↑↓←→ / hjkl", "move one cell"},
			{"g / G", "first / last row"},
			{"0 / $", "first / last column"},
			{"Ctrl+Home/End", "top-left / corner"},
			{"PgUp / PgDn", "page up / down"},
			{": A1 ↵", "jump to address"},
			{"/ text ↵", "find text"},
			{"n", "find next match"},
		}},
		{"Editing", [][2]string{
			{"i / ↵ / F2", "edit cell"},
			{"a", "edit at end"},
			{"I", "clear & edit"},
			{"x / Del", "clear cell"},
			{"o / O", "row below/above"},
			{"+ / -", "col after/before"},
			{"d / D", "del row / col (confirm)"},
			{"y / p", "yank / paste"},
			{"Y", "fill down"},
			{"T", "toggle TOTAL row"},
			{"v", "visual selection"},
			{"u / Ctrl+R", "undo / redo"},
			{"Ctrl+S", "save file"},
		}},
		{"Edit mode keys", [][2]string{
			{"↵", "save, ↓"},
			{"Tab / ⇧Tab", "save, → / ←"},
			{"Esc", "cancel"},
			{"← → Home End", "move in cell"},
			{"Bksp / Del", "delete char"},
		}},
	}
	rightGroups := []group{
		{"Sort & Filter", [][2]string{
			{"s", "sort none→↑→↓"},
			{"H", "row 0 = header"},
		}},
		{"View & Format", [][2]string{
			{"c", "chart panel"},
			{"C", "cycle chart"},
			{"=", "auto-fit column"},
			{"e", "empty · marks"},
			{"L", "column lines"},
			{"F", "freeze header"},
			{"F6", "light / dark"},
		}},
		{"Sheets (xlsx)", [][2]string{
			{"Tab", "next sheet"},
			{"⇧Tab", "previous sheet"},
		}},
		{"Tabs", [][2]string{
			{"q / Esc", "close sheet"},
			{"Ctrl+W", "close any tab"},
			{"Ctrl+Tab", "cycle tabs"},
			{"Alt+1…9", "jump to tab N"},
		}},
		{"Help", [][2]string{
			{"?", "show / hide"},
			{"↑↓ PgUp/Dn", "scroll"},
			{"Esc / q / ?", "close help"},
		}},
	}

	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(yellow).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(text)
	dimStyle := lipgloss.NewStyle().Foreground(overlay1)

	// Sizing: target ~88 cols wide, two columns inside.
	maxW := 88
	if viewportW-4 < maxW {
		maxW = viewportW - 4
	}
	if maxW < 50 {
		maxW = 50
	}
	innerW := maxW - 4
	colW := innerW / 2

	renderColumn := func(groups []group) []string {
		var out []string
		keyW := 16
		for _, g := range groups {
			out = append(out, titleStyle.Render(g.title))
			out = append(out, dimStyle.Render(strings.Repeat("─", colW-2)))
			for _, kv := range g.items {
				k := padOrTrunc(kv[0], keyW)
				d := kv[1]
				if utf8.RuneCountInString(d) > colW-keyW-3 {
					d = truncRunes(d, colW-keyW-4) + "…"
				}
				out = append(out, keyStyle.Render(k)+"  "+descStyle.Render(d))
			}
			out = append(out, "")
		}
		return out
	}

	leftLines := renderColumn(leftGroups)
	rightLines := renderColumn(rightGroups)
	rows := len(leftLines)
	if len(rightLines) > rows {
		rows = len(rightLines)
	}
	for len(leftLines) < rows {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < rows {
		rightLines = append(rightLines, "")
	}

	// Compose
	var body []string
	body = append(body,
		lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(" 󰓎  Spreadsheet — Keyboard Reference"),
		dimStyle.Render(" Press ? or Esc to close.  ↑↓ to scroll if cropped."),
		"",
	)
	for i := 0; i < rows; i++ {
		l := padOrTrunc(leftLines[i], colW)
		r := rightLines[i]
		body = append(body, l+"  "+r)
	}

	visH := viewportH - 6
	if visH < 6 {
		visH = 6
	}
	if visH > len(body) {
		visH = len(body)
	}
	maxScroll := len(body) - visH
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := s.helpScroll
	if scroll > maxScroll {
		scroll = maxScroll
	}
	visLines := body[scroll : scroll+visH]

	footer := ""
	if maxScroll > 0 {
		footer = "\n" + lipgloss.NewStyle().Foreground(overlay1).Italic(true).
			Render(fmt.Sprintf(" %d/%d  ↓ for more", scroll, maxScroll))
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Background(base).
		Padding(1, 2).
		Width(maxW)
	return box.Render(strings.Join(visLines, "\n") + footer)
}

func (s *SheetView) renderEmptyState() string {
	width := s.width
	if width <= 0 {
		width = 80
	}
	tip := lipgloss.NewStyle().Foreground(subtext0).Padding(1, 2).
		Render("Empty sheet — press " +
			lipgloss.NewStyle().Foreground(yellow).Bold(true).Render("i") +
			" to start typing.")
	return tip
}

// renderHeader draws a two-row top section:
//
//   row 1 (file bar):    │ ▦ budget.xlsx ●     2 sheets · 30×7   │
//   row 2 (formula bar): │ A5 ƒ Monatliche Kostenübersicht  TXT  │
//
// Splitting these avoids the overflow problem the single-row
// version had with long file names + long cell values, and makes
// each piece of info easier to scan.
func (s *SheetView) renderHeader(width int) string {
	return s.renderFileBar(width) + "\n" + s.renderFormulaBar(width)
}

// renderFileBar (row 1): file icon, name, modified glyph, plus a
// right-aligned summary "N sheets · R×C  Φ filtered/sorted".
func (s *SheetView) renderFileBar(width int) string {
	name := filepath.Base(s.filePath)
	if name == "" || name == "." {
		name = "(untitled)"
	}
	icon := svFileIcon(s.fileType)

	modBadge := ""
	if s.modified {
		modBadge = lipgloss.NewStyle().Foreground(yellow).Bold(true).Background(surface0).Render("  ●  unsaved")
	} else {
		modBadge = lipgloss.NewStyle().Foreground(green).Background(surface0).Render("  ✓  saved")
	}

	// Truncate filename if it would push the rest off the line.
	maxName := width - 50
	if maxName < 12 {
		maxName = 12
	}
	if utf8.RuneCountInString(name) > maxName {
		name = truncRunes(name, maxName-1) + "…"
	}

	left := lipgloss.NewStyle().Background(surface0).
		Foreground(mauve).Bold(true).Render(" " + icon + " " + name) + modBadge

	// Right summary: sheets count, dimensions, sort indicator.
	parts := []string{}
	if len(s.sheets) > 1 {
		parts = append(parts, fmt.Sprintf("%d sheets", len(s.sheets)))
	}
	parts = append(parts, fmt.Sprintf("%d×%d", s.numRows(), s.numCols()))
	if s.sortDir != 0 && s.sortCol >= 0 {
		dir := "↑"
		if s.sortDir < 0 {
			dir = "↓"
		}
		parts = append(parts, "sort "+dir+" "+colLetters(s.sortCol))
	}
	rightTxt := " " + strings.Join(parts, " · ") + " "
	right := lipgloss.NewStyle().Background(surface0).Foreground(subtext0).Render(rightTxt)

	pad := width - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 0 {
		pad = 0
	}
	mid := lipgloss.NewStyle().Background(surface0).Render(strings.Repeat(" ", pad))
	return left + mid + right
}

// renderFormulaBar (row 2): A1 pill, formula prefix, cell content,
// kind badge — all on one row but with substantially shorter text
// so it fits any sensible terminal.
func (s *SheetView) renderFormulaBar(width int) string {
	pillBg := mauve
	if s.mode == sheetModeEdit {
		pillBg = yellow
	}
	addressPill := lipgloss.NewStyle().Background(pillBg).Foreground(base).Bold(true).
		Render(" " + a1Address(s.row, s.col) + " ")

	// Sheet-name chip when multi-sheet so the user always sees
	// which worksheet they're on.
	sheetChip := ""
	if len(s.sheets) > 1 && s.activeSheet >= 0 && s.activeSheet < len(s.sheets) {
		sheetChip = lipgloss.NewStyle().Background(surface1).Foreground(subtext1).
			Render(" " + s.sheets[s.activeSheet].name + " ")
	}

	val := s.cell(s.row, s.col)
	if s.mode == sheetModeEdit {
		val = string(s.editBuf[:s.editCursor]) + "▎" + string(s.editBuf[s.editCursor:])
	}
	prefix := "  "
	if s.mode == sheetModeEdit {
		prefix = " ✎ "
	} else if val != "" {
		prefix = " ƒ "
	}

	kindLabel, kindBg := svKindBadge(s.colKind(s.col))
	kindStyled := lipgloss.NewStyle().Background(kindBg).Foreground(base).Bold(true).
		Render(" " + kindLabel + " ")

	leftW := lipgloss.Width(addressPill) + lipgloss.Width(sheetChip) + lipgloss.Width(prefix)
	rightW := lipgloss.Width(kindStyled)
	maxValW := width - leftW - rightW - 1
	if maxValW < 8 {
		maxValW = 8
	}
	if utf8.RuneCountInString(val) > maxValW {
		val = truncRunes(val, maxValW-1) + "…"
	}

	formulaStyled := lipgloss.NewStyle().Background(surface0).Foreground(text).
		Render(prefix + val)
	pad := width - leftW - lipgloss.Width(formulaStyled) + lipgloss.Width(prefix) - rightW
	if pad < 0 {
		pad = 0
	}
	padStr := lipgloss.NewStyle().Background(surface0).Render(strings.Repeat(" ", pad))
	return addressPill + sheetChip + formulaStyled + padStr + kindStyled
}

// svFileIcon picks a glyph based on the file extension. Pure ASCII
// fall-back ensures rendering on non-UTF-8 terminals.
func svFileIcon(fileType string) string {
	switch fileType {
	case "xlsx":
		return "▦"
	default:
		return "▤"
	}
}

// svKindBadge maps a column kind to its on-screen label and the
// background color used for the badge in the header bar.
func svKindBadge(k sheetCol) (string, lipgloss.Color) {
	switch k {
	case sheetColNumber:
		return "123", sky
	case sheetColCurrency:
		return " $ ", green
	case sheetColPercent:
		return " % ", peach
	case sheetColDate:
		return "DAT", lavender
	default:
		return "TXT", overlay1
	}
}

// renderSheetTabs draws Excel-style worksheet tabs. The active
// tab uses a colored top bar (cycling through a small palette so
// each sheet has a stable visual identity) and a "raised" effect
// via background contrast.
func (s *SheetView) renderSheetTabs(width int) string {
	palette := []lipgloss.Color{mauve, sky, green, peach, lavender, pink, teal, yellow}
	var parts []string
	parts = append(parts, lipgloss.NewStyle().Background(mantle).Render(" "))
	for i, sh := range s.sheets {
		bandColor := palette[i%len(palette)]
		if i == s.activeSheet {
			// "Raised" active tab: colored band on top + base background under text
			tab := lipgloss.NewStyle().Background(bandColor).Foreground(base).Bold(true).
				Render(" " + sh.name + " ")
			parts = append(parts, tab)
		} else {
			tab := lipgloss.NewStyle().Background(surface0).Foreground(subtext0).
				Render(" " + sh.name + " ")
			parts = append(parts, tab)
		}
		parts = append(parts, lipgloss.NewStyle().Background(mantle).Render(" "))
	}
	line := strings.Join(parts, "")
	if lipgloss.Width(line) > width {
		line = truncRunes(line, width-1) + "…"
	} else {
		line += lipgloss.NewStyle().Background(mantle).Render(strings.Repeat(" ", width-lipgloss.Width(line)))
	}
	return line
}

// renderGrid produces the main spreadsheet grid. Visual layers
// (bottom → top): base background → zebra alternating → crosshair
// (active row + active column) → frozen header → section banner
// rows → semantic per-kind text color → active cell highlight.
//
// New in this revision:
//   - Frozen header row (row 0 sticks to the top when scrolled)
//   - Section banners auto-detected (only column A populated)
//   - Vertical column separators (toggle with 'l')
//   - Empty cells are blank by default ('e' to show ·)
//   - Brighter active row / column / cell highlights
//   - Sort indicator on the active sort column header
func (s *SheetView) renderGrid(width int) string {
	rows := s.numRows()
	cols := s.numCols()
	vpRows := s.viewportRows()

	// When the header row is frozen, reserve one viewport row for
	// it so it stays glued to the top while the data scrolls.
	frozenHeader := s.headerIsLabel && s.freezeHeader && rows > 0 && s.rowOff > 0
	dataVpRows := vpRows - 1 // 1 row consumed by column ruler
	if frozenHeader {
		dataVpRows-- // reserve another for the frozen header
	}
	if dataVpRows < 1 {
		dataVpRows = 1
	}

	startDataRow := s.rowOff
	endRow := startDataRow + dataVpRows
	if endRow > rows {
		endRow = rows
	}

	gutter := s.rowGutterWidth()
	avail := width - gutter
	if avail < 8 {
		avail = 8
	}
	visibleCols := []int{}
	w := 0
	for c := s.colOff; c < cols; c++ {
		need := s.colWidths[c] + 1
		if w+need > avail && len(visibleCols) > 0 {
			break
		}
		visibleCols = append(visibleCols, c)
		w += need
	}

	var b strings.Builder
	b.WriteString(s.renderColumnRuler(width, gutter, visibleCols))
	b.WriteString("\n")

	// Frozen header row — only when scrolled past row 0.
	if frozenHeader {
		b.WriteString(s.renderRow(0, width, gutter, visibleCols, false, true))
		b.WriteString("\n")
	}

	// Data rows
	usedRows := 0
	for r := startDataRow; r < endRow; r++ {
		isZebra := (r-startDataRow)%2 == 1
		// Skip the regular header row when it's already showing
		// frozen at the top, otherwise it'd appear twice.
		if frozenHeader && r == 0 {
			continue
		}
		b.WriteString(s.renderRow(r, width, gutter, visibleCols, isZebra, false))
		b.WriteString("\n")
		usedRows++
	}

	// Pad to viewport height so the chart panel beside us is the
	// same height as the grid.
	for i := usedRows; i < dataVpRows; i++ {
		b.WriteString(svStyleNormal.Width(width).Render(""))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderColumnRuler draws the A B C… header strip with active
// column highlighted in mauve, with a sort-direction glyph when
// applicable.
func (s *SheetView) renderColumnRuler(width, gutter int, visibleCols []int) string {
	var b strings.Builder
	// Top-left corner — slightly tinted so the ruler corner reads
	// as part of the bar.
	b.WriteString(svStyleColRuler.Width(gutter).Render(""))
	for _, c := range visibleCols {
		label := colLetters(c)
		// Sort indicator suffix
		if s.sortDir != 0 && s.sortCol == c {
			if s.sortDir > 0 {
				label += "↑"
			} else {
				label += "↓"
			}
		}
		cellW := s.colWidths[c] + 1
		switch {
		case c == s.col:
			b.WriteString(padCenter(label, cellW, svStyleColRulerActive))
		default:
			b.WriteString(padCenter(label, cellW, svStyleColRuler))
		}
	}
	used := gutter
	for _, c := range visibleCols {
		used += s.colWidths[c] + 1
	}
	if used < width {
		b.WriteString(svStyleColRuler.Width(width - used).Render(""))
	}
	return b.String()
}

// renderRow draws a single data row. forceHeader=true paints it
// as the frozen header bar (used when row 0 is being repeated at
// the top of the viewport while scrolled down).
func (s *SheetView) renderRow(r, width, gutter int, visibleCols []int, isZebra, forceHeader bool) string {
	isActiveRow := r == s.row && !forceHeader
	isHeader := forceHeader || (s.headerIsLabel && r == 0)
	isSection := !isHeader && s.isSectionHeaderRow(r)

	// Choose the background carrier for the row's empty padding.
	var rowBg lipgloss.Style
	switch {
	case isActiveRow:
		rowBg = svStyleCellCrosshair
	case isHeader:
		rowBg = svStyleHeaderRow
	case isSection:
		rowBg = lipgloss.NewStyle().Background(surface0)
	case isZebra:
		rowBg = svStyleZebra
	default:
		rowBg = svStyleNormal
	}

	var b strings.Builder

	// Row gutter
	gutterText := fmt.Sprintf("%d ", r+1)
	switch {
	case isActiveRow:
		b.WriteString(svStyleRowGutterActive.Width(gutter).Align(lipgloss.Right).Render(gutterText))
	case isHeader:
		b.WriteString(svStyleHeaderRow.Width(gutter).Align(lipgloss.Right).Foreground(mauve).Bold(true).Render(gutterText))
	default:
		b.WriteString(svStyleRowGutter.Width(gutter).Align(lipgloss.Right).Render(gutterText))
	}

	// Cells
	for ci, c := range visibleCols {
		cellW := s.colWidths[c] + 1
		rawVal := s.cell(r, c)
		isActiveCell := isActiveRow && c == s.col
		isCrossCol := c == s.col && !isActiveRow

		var cellFg lipgloss.Color = text
		isEmpty := strings.TrimSpace(rawVal) == ""
		displayVal := rawVal
		align := lipgloss.Left
		kind := s.colKind(c)

		if !isHeader && !isSection {
			displayVal = formatCellDisplay(rawVal, kind)
			// Per-cell coloring — looks at the cell's own format
			// markers first so a mixed-format column still colors
			// each cell correctly.
			cellFg, align = svCellColorAlign(rawVal, kind)
		}

		inSel := s.inSelection(r, c) && !isHeader && !forceHeader

		// Per-cell style on top of the row carrier.
		var cellStyle lipgloss.Style
		switch {
		case isActiveCell && s.mode == sheetModeEdit:
			cellStyle = svStyleCellEditing
			displayVal = string(s.editBuf[:s.editCursor]) + "▎" + string(s.editBuf[s.editCursor:])
		case isActiveCell:
			cellStyle = svStyleCellActive
		case isHeader:
			cellStyle = svStyleHeaderRow
		case isSection:
			cellStyle = lipgloss.NewStyle().Background(surface0).Foreground(mauve).Bold(true)
		case inSel:
			// Visual-mode selection — distinct lavender tint
			// so it reads as different from crosshair and active.
			cellStyle = lipgloss.NewStyle().Background(lavender).Foreground(base).Bold(true)
		case isCrossCol && isZebra:
			cellStyle = svStyleCellCrosshair.Foreground(cellFg)
		case isCrossCol:
			cellStyle = svStyleCellCrosshair.Foreground(cellFg)
		case isActiveRow:
			cellStyle = svStyleCellCrosshair.Foreground(cellFg)
		case isZebra:
			cellStyle = svStyleZebra.Foreground(cellFg)
		default:
			cellStyle = svStyleNormal.Foreground(cellFg)
		}

		// Empty cell rendering — quiet by default, dim · when toggled on.
		if isEmpty && !isActiveCell {
			if s.showEmpty {
				displayVal = "·"
				cellStyle = cellStyle.Foreground(overlay0)
			} else {
				displayVal = ""
			}
		}

		// Section banner: render the row as a single mauve-bold
		// title spanning the full width, regardless of which cell
		// holds the text.
		if isSection {
			if c == 0 {
				banner := " ▸ " + rawVal
				if utf8.RuneCountInString(banner) > width-gutter-1 {
					banner = truncRunes(banner, width-gutter-2) + "…"
				}
				rest := width - gutter - utf8.RuneCountInString(banner)
				if rest < 0 {
					rest = 0
				}
				b.WriteString(cellStyle.Width(width - gutter).Render(banner + strings.Repeat(" ", rest)))
				return b.String() // section banner consumes the whole row
			}
			continue
		}

		// Truncate to fit
		if utf8.RuneCountInString(displayVal) > cellW-1 {
			displayVal = truncRunes(displayVal, cellW-2) + "…"
		}

		content := svPadCell(displayVal, cellW, align)
		b.WriteString(cellStyle.Render(content))

		// Vertical separator between columns (skip after last visible).
		if s.gridLines && ci < len(visibleCols)-1 {
			// Use the row's background so the separator blends.
			sepStyle := rowBg.Foreground(surface2)
			if isActiveRow || isHeader {
				sepStyle = rowBg.Foreground(overlay0)
			}
			// Replace the trailing space of the previous cell with
			// a │ — done inline via overlay would be complex; we
			// just append the bar and reduce the next cell's width.
			// Simpler: strip last space before separator.
			// Implementation: rewrite the buffer's last char.
			// Our svPadCell adds a trailing " " — remove and add │.
			cur := b.String()
			if strings.HasSuffix(cur, " ") {
				b.Reset()
				b.WriteString(cur[:len(cur)-1])
				b.WriteString(sepStyle.Render("│"))
			} else {
				b.WriteString(sepStyle.Render("│"))
			}
		}
	}

	// Pad row tail
	usedRow := gutter
	for _, c := range visibleCols {
		usedRow += s.colWidths[c] + 1
	}
	if usedRow < width {
		b.WriteString(rowBg.Width(width - usedRow).Render(""))
	}
	return b.String()
}

// svPadCell pads a display string to a fixed width while honoring
// the requested alignment, leaving one trailing space so adjacent
// cells get visual separation.
func svPadCell(val string, cellW int, align lipgloss.Position) string {
	w := utf8.RuneCountInString(val)
	if w >= cellW-1 {
		return val + " "
	}
	pad := cellW - 1 - w
	if align == lipgloss.Right {
		return strings.Repeat(" ", pad) + val + " "
	}
	return val + strings.Repeat(" ", pad) + " "
}

// formatCellDisplay returns a humanised display string. The
// column kind is only a hint — the actual format is chosen from
// the cell value itself so multi-table sheets (where one column
// mixes currency / percent / text rows) render every cell with
// the right format regardless of which one "won" the column-
// dominance vote.
func formatCellDisplay(raw string, kind sheetCol) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	// Per-cell format detection — overrides column kind when
	// the cell itself has clear format markers. This is what
	// makes "64.3%" render as "64.3%" even when its column was
	// classified as generic number because of a mixed row above.
	hasPercent := strings.HasSuffix(v, "%")
	hasCurrency := false
	for _, sym := range []string{"$", "€", "£", "¥"} {
		if strings.Contains(v, sym) {
			hasCurrency = true
			break
		}
	}
	switch {
	case hasPercent:
		if n, ok := parseCellNumeric(v); ok {
			n *= 100
			return strconv.FormatFloat(n, 'f', svPercentDecimals(n), 64) + "%"
		}
	case hasCurrency:
		if n, ok := parseCellNumeric(v); ok {
			sign := ""
			if n < 0 {
				sign = "-"
				n = -n
			}
			sym, trailing := detectCurrencySymbol(v)
			amount := addThousands(n, 2)
			if trailing {
				return sign + amount + " " + sym
			}
			return sign + sym + amount
		}
	}
	// Column-kind based fallback (used when cell has no format
	// markers but the column is known to be numeric/date).
	switch kind {
	case sheetColNumber:
		if n, ok := parseCellNumeric(v); ok {
			// Honour the user's typed precision so "5000.00"
			// stays "5,000.00" and "62.99" stays "62.99". The
			// previous heuristic (svBestDecimals from magnitude)
			// produced rogue digits like "62.990" for input "62.99".
			decimals := svDetectDecimals(v)
			return addThousands(n, decimals)
		}
	case sheetColCurrency:
		if n, ok := parseCellNumeric(v); ok {
			sign := ""
			if n < 0 {
				sign = "-"
				n = -n
			}
			return sign + "$" + addThousands(n, 2)
		}
	case sheetColPercent:
		if n, ok := parseCellNumeric(v); ok {
			n *= 100
			return strconv.FormatFloat(n, 'f', svPercentDecimals(n), 64) + "%"
		}
	}
	return raw
}

// svCellColorAlign returns the foreground color and alignment to
// use for a cell, derived from the cell value first and the
// column kind second. Negative numerics flip to red so financial
// data reads at a glance.
func svCellColorAlign(raw string, kind sheetCol) (lipgloss.Color, lipgloss.Position) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return text, lipgloss.Left
	}
	hasPercent := strings.HasSuffix(v, "%")
	hasCurrency := false
	for _, sym := range []string{"$", "€", "£", "¥"} {
		if strings.Contains(v, sym) {
			hasCurrency = true
			break
		}
	}
	switch {
	case hasPercent:
		c := peach
		if n, ok := parseCellNumeric(v); ok && n < 0 {
			c = red
		}
		return c, lipgloss.Right
	case hasCurrency:
		c := green
		if n, ok := parseCellNumeric(v); ok && n < 0 {
			c = red
		}
		return c, lipgloss.Right
	}
	if _, ok := parseCellNumeric(v); ok {
		c := sky
		if n, _ := parseCellNumeric(v); n < 0 {
			c = red
		}
		return c, lipgloss.Right
	}
	if kind == sheetColDate {
		return lavender, lipgloss.Left
	}
	return text, lipgloss.Left
}

// detectCurrencySymbol returns the currency glyph used in the
// cell ("$", "€", "£", "¥") and whether it appears at the end
// rather than the start. Defaults to "$" leading when no symbol
// is present (the column is detected as currency from sibling
// rows).
func detectCurrencySymbol(v string) (sym string, trailing bool) {
	v = strings.TrimSpace(v)
	for _, s := range []string{"€", "£", "¥", "$"} {
		idx := strings.Index(v, s)
		if idx < 0 {
			continue
		}
		// Trailing if symbol is in the right-third of the cell.
		return s, idx > len(v)/2
	}
	return "$", false
}

func svBestDecimals(n float64) int {
	if math.Abs(n) >= 100 {
		return 2
	}
	if math.Abs(n) >= 10 {
		return 3
	}
	return 4
}

// svDetectDecimalsForCol picks a decimal precision that
// matches the column's existing values — so a TOTAL row
// summing "5,000.00" + "200.50" comes out as "5200.50" not
// "5200" or "5200.000000".
func svDetectDecimalsForCol(s *SheetView, c int) int {
	max := 0
	startRow := 0
	if s.headerIsLabel {
		startRow = 1
	}
	for r := startRow; r < s.numRows(); r++ {
		d := svDetectDecimals(s.cell(r, c))
		if d > max {
			max = d
		}
	}
	return max
}

// svDetectDecimals counts the fractional digits in the user's
// raw cell value. Strips currency symbols and trailing % so
// "62.99 €" still reports 2 decimals. Returns 0 when no
// fractional part is present, capped at 6 to keep numbers
// readable in narrow columns.
func svDetectDecimals(raw string) int {
	v := strings.TrimSpace(raw)
	for _, sym := range []string{"$", "€", "£", "¥", "%", " "} {
		v = strings.ReplaceAll(v, sym, "")
	}
	dot := strings.LastIndex(v, ".")
	comma := strings.LastIndex(v, ",")
	sep := dot
	if comma > sep {
		sep = comma
	}
	if sep < 0 {
		return 0
	}
	tail := len(v) - sep - 1
	// "1,234" — comma + 3 trailing digits — is thousands, not
	// decimals. Differentiate by looking for an OPPOSITE-kind
	// separator earlier in the string.
	if tail == 3 {
		other := dot
		if dot == sep {
			other = comma
		}
		if other < 0 {
			return 0
		}
	}
	if tail < 1 || tail > 6 {
		return 0
	}
	return tail
}

func svPercentDecimals(n float64) int {
	if math.Abs(n-math.Trunc(n)) < 1e-9 {
		return 0
	}
	return 1
}

// addThousands renders a float with comma thousand-separators
// and the given number of fractional digits.
func addThousands(v float64, decimals int) string {
	negative := v < 0
	if negative {
		v = -v
	}
	s := strconv.FormatFloat(v, 'f', decimals, 64)
	intPart := s
	frac := ""
	if i := strings.Index(s, "."); i >= 0 {
		intPart = s[:i]
		frac = s[i:]
	}
	// Insert commas every three digits from the right.
	n := len(intPart)
	if n <= 3 {
		if negative {
			return "-" + intPart + frac
		}
		return intPart + frac
	}
	first := n % 3
	var b strings.Builder
	if first > 0 {
		b.WriteString(intPart[:first])
		if n > first {
			b.WriteByte(',')
		}
	}
	for i := first; i < n; i += 3 {
		b.WriteString(intPart[i : i+3])
		if i+3 < n {
			b.WriteByte(',')
		}
	}
	out := b.String() + frac
	if negative {
		return "-" + out
	}
	return out
}

func (s *SheetView) rowGutterWidth() int {
	w := len(strconv.Itoa(s.numRows()))
	if w < 3 {
		w = 3
	}
	return w + 1
}

// renderStats renders the statistics footer. When a visual
// range is selected we compute stats over the SELECTION
// (Excel-style); otherwise we show stats for the active column.
func (s *SheetView) renderStats(width int) string {
	if s.selActive {
		return s.renderSelectionStats(width)
	}
	bg := mantle
	c := s.col
	values := s.numericColumnValues(c)

	colName := svColumnHeaderText(s, c)
	colDisplay := colLetters(c)
	if colName != "" {
		colDisplay = colName + " " +
			lipgloss.NewStyle().Background(bg).Foreground(overlay0).Render("("+colLetters(c)+")")
	}

	if len(values) == 0 {
		// Text column — show value count + unique count
		nonEmpty := 0
		uniq := map[string]struct{}{}
		startRow := 0
		if s.headerIsLabel {
			startRow = 1
		}
		for r := startRow; r < s.numRows(); r++ {
			v := strings.TrimSpace(s.cell(r, c))
			if v == "" {
				continue
			}
			nonEmpty++
			uniq[v] = struct{}{}
		}
		colChip := lipgloss.NewStyle().Background(bg).Foreground(text).Bold(true).Render(" " + colDisplay + " ")
		typeChip := lipgloss.NewStyle().Background(overlay1).Foreground(base).Bold(true).Render(" TXT ")
		stat1 := svStatChip(bg, "n", strconv.Itoa(nonEmpty), text)
		stat2 := svStatChip(bg, "unique", strconv.Itoa(len(uniq)), lavender)
		body := colChip + typeChip + " " + stat1 + "  " + stat2
		pad := width - lipgloss.Width(body)
		if pad > 0 {
			body += lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", pad))
		}
		return body
	}

	sum, mn, mx := 0.0, math.Inf(1), math.Inf(-1)
	for _, v := range values {
		sum += v
		if v < mn {
			mn = v
		}
		if v > mx {
			mx = v
		}
	}
	avg := sum / float64(len(values))
	medianVals := append([]float64(nil), values...)
	sort.Float64s(medianVals)
	med := medianVals[len(medianVals)/2]

	kindLabel, kindBg := svKindBadge(s.colKind(c))
	colChip := lipgloss.NewStyle().Background(bg).Foreground(text).Bold(true).Render(" " + colDisplay + " ")
	typeChip := lipgloss.NewStyle().Background(kindBg).Foreground(base).Bold(true).Render(" " + kindLabel + " ")

	chips := []string{
		svStatChip(bg, "n", strconv.Itoa(len(values)), text),
		svStatChip(bg, "Σ", svFormatStat(sum, s.colKind(c)), green),
		svStatChip(bg, "x̄", svFormatStat(avg, s.colKind(c)), sky),
		svStatChip(bg, "med", svFormatStat(med, s.colKind(c)), lavender),
		svStatChip(bg, "↓", svFormatStat(mn, s.colKind(c)), peach),
		svStatChip(bg, "↑", svFormatStat(mx, s.colKind(c)), green),
	}
	body := colChip + typeChip + " " + strings.Join(chips, "  ")
	if lipgloss.Width(body) > width {
		body = truncRunes(body, width-1) + "…"
	} else {
		pad := width - lipgloss.Width(body)
		if pad > 0 {
			body += lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", pad))
		}
	}
	return body
}

// renderSelectionStats draws the stats footer for the active
// rectangular selection. Reports cell count, non-empty count,
// and (when ≥2 numeric values are in range) sum / avg / min /
// max — matching Excel's bottom-bar behaviour. Useful for "I
// just want to know what these 12 cells add up to" without
// dropping a SUM formula.
func (s *SheetView) renderSelectionStats(width int) string {
	bg := mantle
	r0, r1 := s.selStartRow, s.selEndRow
	c0, c1 := s.selStartCol, s.selEndCol
	if r0 > r1 {
		r0, r1 = r1, r0
	}
	if c0 > c1 {
		c0, c1 = c1, c0
	}
	cellCount := (r1 - r0 + 1) * (c1 - c0 + 1)
	nonEmpty := 0
	var nums []float64
	for r := r0; r <= r1; r++ {
		for c := c0; c <= c1; c++ {
			v := strings.TrimSpace(s.cell(r, c))
			if v != "" {
				nonEmpty++
			}
			if n, ok := parseCellNumeric(v); ok {
				nums = append(nums, n)
			}
		}
	}

	rangeChip := lipgloss.NewStyle().Background(lavender).Foreground(base).Bold(true).
		Render(fmt.Sprintf(" %s:%s ", a1Address(r0, c0), a1Address(r1, c1)))
	dimsChip := lipgloss.NewStyle().Background(bg).Foreground(text).
		Render(fmt.Sprintf(" %d×%d  ", r1-r0+1, c1-c0+1))

	chips := []string{
		svStatChip(bg, "cells", strconv.Itoa(cellCount), text),
		svStatChip(bg, "filled", strconv.Itoa(nonEmpty), lavender),
	}
	if len(nums) >= 1 {
		sum, mn, mx := 0.0, math.Inf(1), math.Inf(-1)
		for _, v := range nums {
			sum += v
			if v < mn {
				mn = v
			}
			if v > mx {
				mx = v
			}
		}
		avg := sum / float64(len(nums))
		// Pick a representative kind from the first cell of the
		// selection so currency/percent stats render in the right
		// units.
		repKind := s.colKind(c0)
		chips = append(chips,
			svStatChip(bg, "n", strconv.Itoa(len(nums)), text),
			svStatChip(bg, "Σ", svFormatStat(sum, repKind), green),
			svStatChip(bg, "x̄", svFormatStat(avg, repKind), sky),
			svStatChip(bg, "↓", svFormatStat(mn, repKind), peach),
			svStatChip(bg, "↑", svFormatStat(mx, repKind), green),
		)
	}
	body := rangeChip + dimsChip + strings.Join(chips, "  ")
	if lipgloss.Width(body) > width {
		body = truncRunes(body, width-1) + "…"
	} else {
		pad := width - lipgloss.Width(body)
		if pad > 0 {
			body += lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", pad))
		}
	}
	return body
}

// svColumnHeaderText returns the header label for column c when
// headerIsLabel is on (i.e. row 0 is the column name). Empty
// otherwise.
func svColumnHeaderText(s *SheetView, c int) string {
	if !s.headerIsLabel {
		return ""
	}
	if s.numRows() == 0 {
		return ""
	}
	v := strings.TrimSpace(s.cell(0, c))
	if v == "" {
		return ""
	}
	if utf8.RuneCountInString(v) > 18 {
		v = truncRunes(v, 17) + "…"
	}
	return v
}

// svStatChip renders a "label=value" pair with the label dimmed
// and the value coloured by the given accent. Used by the stats
// footer to give each metric a consistent shape.
func svStatChip(bg lipgloss.Color, label, val string, accent lipgloss.Color) string {
	lab := lipgloss.NewStyle().Background(bg).Foreground(overlay1).Render(label + " ")
	v := lipgloss.NewStyle().Background(bg).Foreground(accent).Bold(true).Render(val)
	return lab + v
}

// svFormatStat formats a stat number using the column's kind so
// currency stats render with $ and percent stats with %.
func svFormatStat(v float64, kind sheetCol) string {
	switch kind {
	case sheetColCurrency:
		sign := ""
		if v < 0 {
			sign = "-"
			v = -v
		}
		return sign + "$" + addThousands(v, 2)
	case sheetColPercent:
		v *= 100
		return strconv.FormatFloat(v, 'f', svPercentDecimals(v), 64) + "%"
	}
	if math.Abs(v) >= 1e6 {
		return fmtNum(v)
	}
	if math.Abs(v-math.Trunc(v)) < 1e-9 {
		return addThousands(v, 0)
	}
	return addThousands(v, 2)
}

// renderStatus draws the bottom status line:
//
//	│ NORMAL │ A5 12×4 │ Tab=next col · i=edit · /=find …      │
//
// The mode badge is colored per mode (mauve/yellow/teal/blue)
// so the user always knows what input the surface is expecting.
// Transient messages (yank, save, search miss) override the
// hint slot for ~4 seconds, then it falls back to the help text.
func (s *SheetView) renderStatus(width int) string {
	mode, modeBg := svModeBadge(s.mode)
	modeStyled := lipgloss.NewStyle().Background(modeBg).Foreground(base).Bold(true).
		Render(" " + mode + " ")

	// Position chip
	posTxt := fmt.Sprintf(" %s  %d×%d ", a1Address(s.row, s.col), s.numRows(), s.numCols())
	posStyled := lipgloss.NewStyle().Background(surface1).Foreground(text).Render(posTxt)

	// Mod chip
	modChip := ""
	if s.modified {
		modChip = lipgloss.NewStyle().Background(yellow).Foreground(base).Bold(true).Render(" MOD ")
	}

	// Right-side: input prompt or hint
	var rightContent string
	switch s.mode {
	case sheetModeFind:
		rightContent = " " + lipgloss.NewStyle().Background(surface0).Foreground(yellow).Bold(true).
			Render("/"+string(s.findBuf)+"▎")
	case sheetModeGoto:
		rightContent = " " + lipgloss.NewStyle().Background(surface0).Foreground(yellow).Bold(true).
			Render(":"+string(s.gotoBuf)+"▎")
	default:
		rightContent = " " + sheetHelpHint(s) + " "
	}
	rightStyled := lipgloss.NewStyle().Background(surface0).Foreground(subtext1).Render(rightContent)

	leftPart := modeStyled + posStyled + modChip
	pad := width - lipgloss.Width(leftPart) - lipgloss.Width(rightStyled)
	if pad < 0 {
		pad = 0
	}
	mid := lipgloss.NewStyle().Background(surface0).Render(strings.Repeat(" ", pad))

	// Transient toast (Saved / Yanked / search wrapped)
	if s.statusMsg != "" && time.Since(s.statusAt) < 4*time.Second {
		toast := lipgloss.NewStyle().Background(surface0).Foreground(yellow).Bold(true).
			Render(" ⚡ " + s.statusMsg + " ")
		mPad := pad - lipgloss.Width(toast)
		if mPad < 0 {
			mPad = 0
		}
		mid = lipgloss.NewStyle().Background(surface0).Render(strings.Repeat(" ", mPad)) + toast
	}
	return leftPart + mid + rightStyled
}

func svModeBadge(m sheetMode) (string, lipgloss.Color) {
	switch m {
	case sheetModeEdit:
		return "EDIT", yellow
	case sheetModeFind:
		return "FIND", teal
	case sheetModeGoto:
		return "GOTO", blue
	case sheetModePicker:
		return "PICK", lavender
	default:
		return "NORM", mauve
	}
}

// sheetHelpHint returns a context-relevant one-liner for the
// status bar. We pick a SHORT hint per mode/state so the most
// useful next-key always fits. The full reference is one ?
// keypress away — the hint always advertises that fallback.
func sheetHelpHint(s *SheetView) string {
	switch s.mode {
	case sheetModeEdit:
		return "↵ save  ·  Tab next col  ·  Esc cancel  ·  ? help"
	case sheetModeFind:
		return "↵ jump to match  ·  Esc cancel"
	case sheetModeGoto:
		return "↵ jump to A1 cell  ·  Esc cancel"
	}
	if s.helpVisible {
		return "↑↓ scroll  ·  Esc / q / ? close help"
	}
	if s.showChart {
		return "C cycle chart  ·  c close  ·  ←→ change column  ·  ? help"
	}
	if s.selVisualMode {
		return "y yank range  ·  Esc cancel  ·  arrows extend  ·  ? help"
	}
	hint := "i edit  ·  v select  ·  o row  ·  + col  ·  s sort  ·  / find  ·  u undo  ·  c chart  ·  ^S save"
	if len(s.sheets) > 1 {
		hint += "  ·  Tab sheet"
	}
	hint += "  ·  ? help"
	return hint
}

// renderChartPanel draws a side-panel chart of the active
// column's numeric values. Each chart type uses a different
// accent color so the panel stays visually anchored as the user
// cycles through bar / line / histogram. Includes a min/max
// axis strip and a metric mini-summary above the plot.
func (s *SheetView) renderChartPanel(width int) string {
	if width < 16 {
		return ""
	}
	innerW := width - 4
	col := s.chartCol
	values := s.numericColumnValues(col)
	labels := s.columnLabels(col)

	colName := svColumnHeaderText(s, col)
	if colName == "" {
		colName = "Column " + colLetters(col)
	}

	chartAccent := green
	switch s.chartType {
	case sheetChartLine:
		chartAccent = sky
	case sheetChartHistogram:
		chartAccent = peach
	}

	titleBar := lipgloss.NewStyle().
		Background(chartAccent).Foreground(base).Bold(true).
		Width(innerW).Padding(0, 1).
		Render(colName + "  " +
			lipgloss.NewStyle().Background(chartAccent).Foreground(base).Render("· "+s.chartType.String()))

	// Mini stats above the plot
	var subtitle string
	if len(values) > 0 {
		mn, mx := values[0], values[0]
		sum := 0.0
		for _, v := range values {
			sum += v
			if v < mn {
				mn = v
			}
			if v > mx {
				mx = v
			}
		}
		avg := sum / float64(len(values))
		subtitle = lipgloss.NewStyle().Background(surface0).Foreground(subtext0).Width(innerW).Padding(0, 1).
			Render(fmt.Sprintf(" n=%d  Σ=%s  x̄=%s",
				len(values),
				svFormatStat(sum, s.colKind(col)),
				svFormatStat(avg, s.colKind(col))))
	}

	chartH := s.chartHeight() + s.viewportRows() - 4
	if chartH < 4 {
		chartH = 4
	}

	var bodyLines []string
	if len(values) == 0 {
		bodyLines = []string{
			"",
			lipgloss.NewStyle().Foreground(overlay1).Render("  (no numeric data)"),
			"",
			lipgloss.NewStyle().Foreground(subtext0).Render("  Move cursor onto a"),
			lipgloss.NewStyle().Foreground(subtext0).Render("  numeric column, or"),
			lipgloss.NewStyle().Foreground(subtext0).Render("  press 'C' to cycle."),
		}
	} else {
		switch s.chartType {
		case sheetChartLine:
			bodyLines = renderLineChart(values, innerW-8, chartH, chartAccent, s.colKind(col))
		case sheetChartHistogram:
			bodyLines = renderHistogram(values, innerW, chartH, chartAccent)
		default:
			bodyLines = renderBarChart(values, labels, innerW, chartH, chartAccent, s.colKind(col))
		}
	}

	footer := lipgloss.NewStyle().Background(surface0).Foreground(subtext1).
		Width(innerW).Padding(0, 1).
		Render(" c=close  C=" +
			lipgloss.NewStyle().Background(surface0).Foreground(yellow).Bold(true).Render("cycle") +
			"  ←/→ adjusts column ")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(chartAccent).
		Background(base).
		Padding(0, 1)

	inner := lipgloss.JoinVertical(lipgloss.Left,
		titleBar,
		subtitle,
		strings.Join(bodyLines, "\n"),
		footer,
	)
	return box.Render(inner)
}

func (s *SheetView) numericColumnValues(c int) []float64 {
	var out []float64
	startRow := 0
	if s.headerIsLabel {
		startRow = 1
	}
	for r := startRow; r < s.numRows(); r++ {
		if v, ok := parseCellNumeric(s.cell(r, c)); ok {
			out = append(out, v)
		}
	}
	return out
}

func (s *SheetView) columnLabels(c int) []string {
	var out []string
	startRow := 0
	labelCol := 0
	if c == 0 {
		labelCol = -1 // no separate label column
	}
	if s.headerIsLabel {
		startRow = 1
	}
	for r := startRow; r < s.numRows(); r++ {
		if _, ok := parseCellNumeric(s.cell(r, c)); !ok {
			continue
		}
		if labelCol < 0 {
			out = append(out, strconv.Itoa(r+1))
		} else {
			out = append(out, s.cell(r, labelCol))
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Chart renderers — pure ASCII so they survive any terminal
// ---------------------------------------------------------------------------

// renderBarChart draws a horizontal bar chart with intensity
// shading: each bar uses the accent color but with values close
// to the column max getting a brighter brick (█) and lower
// values fading to a half-block (▌). Negatives flip to red.
func renderBarChart(values []float64, labels []string, width, height int, accent lipgloss.Color, kind sheetCol) []string {
	if width < 12 || height < 2 {
		return nil
	}
	mn, mx := math.Inf(1), math.Inf(-1)
	for _, v := range values {
		if v < mn {
			mn = v
		}
		if v > mx {
			mx = v
		}
	}
	if mn == mx {
		mx = mn + 1
	}
	// Limit to first N values that fit.
	maxRows := height
	if len(values) > maxRows {
		values = values[:maxRows]
		if len(labels) > maxRows {
			labels = labels[:maxRows]
		}
	}
	labelWidth := 6
	for _, lab := range labels {
		if utf8.RuneCountInString(lab) > labelWidth {
			labelWidth = utf8.RuneCountInString(lab)
		}
	}
	if labelWidth > 14 {
		labelWidth = 14
	}
	valWidth := 10
	barWidth := width - labelWidth - valWidth - 3
	if barWidth < 6 {
		barWidth = 6
	}
	out := make([]string, 0, len(values))
	for i, v := range values {
		ratio := (v - mn) / (mx - mn)
		if ratio < 0 {
			ratio = 0
		}
		blocks := int(math.Round(ratio * float64(barWidth)))
		// Pick color: red for negatives in numeric kinds
		c := accent
		if (kind == sheetColCurrency || kind == sheetColNumber || kind == sheetColPercent) && v < 0 {
			c = red
		}
		bar := lipgloss.NewStyle().Foreground(c).Render(strings.Repeat("█", blocks)) +
			lipgloss.NewStyle().Foreground(surface2).Render(strings.Repeat("░", barWidth-blocks))
		lab := ""
		if i < len(labels) {
			lab = labels[i]
		}
		lab = padOrTrunc(lab, labelWidth)
		val := padLeft(svFormatStat(v, kind), valWidth)
		line := lipgloss.NewStyle().Foreground(subtext0).Render(lab+" ") +
			bar +
			" " + lipgloss.NewStyle().Foreground(text).Render(val)
		out = append(out, line)
	}
	for len(out) < height {
		out = append(out, "")
	}
	return out
}

// renderLineChart draws a line chart using Braille pixels (each
// cell carries up to 2x4 dots) so the curve looks smooth even
// in a small terminal panel. Includes a left axis with the min
// and max labels, plus a center gridline at the mean.
func renderLineChart(values []float64, width, height int, accent lipgloss.Color, kind sheetCol) []string {
	if width < 8 || height < 3 || len(values) == 0 {
		return nil
	}
	mn, mx := math.Inf(1), math.Inf(-1)
	for _, v := range values {
		if v < mn {
			mn = v
		}
		if v > mx {
			mx = v
		}
	}
	if mn == mx {
		mx = mn + 1
	}
	// Braille canvas: each character is 2 dots wide × 4 dots tall.
	pxW := width * 2
	pxH := height * 4

	// Sample values to pxW pixel columns.
	pixels := make([][]bool, pxH)
	for r := range pixels {
		pixels[r] = make([]bool, pxW)
	}
	mapVal := func(v float64) int {
		ratio := (v - mn) / (mx - mn)
		if ratio < 0 {
			ratio = 0
		}
		return pxH - 1 - int(math.Round(ratio*float64(pxH-1)))
	}
	prevY := -1
	for x := 0; x < pxW; x++ {
		idx := int(float64(x) * float64(len(values)-1) / float64(pxW-1))
		if pxW == 1 {
			idx = 0
		}
		if idx >= len(values) {
			idx = len(values) - 1
		}
		y := mapVal(values[idx])
		pixels[y][x] = true
		// connect with previous pixel
		if prevY >= 0 {
			startY, endY := prevY, y
			if startY > endY {
				startY, endY = endY, startY
			}
			for yy := startY; yy <= endY; yy++ {
				if x-1 >= 0 {
					pixels[yy][x-1] = true
				}
			}
		}
		prevY = y
	}
	// Pack into Braille runes.
	out := make([]string, height)
	axisStyle := lipgloss.NewStyle().Foreground(overlay1)
	for ry := 0; ry < height; ry++ {
		var line strings.Builder
		// Left axis label
		var label string
		switch ry {
		case 0:
			label = padLeft(svFormatStat(mx, kind), 7)
		case height - 1:
			label = padLeft(svFormatStat(mn, kind), 7)
		case height / 2:
			label = padLeft(svFormatStat((mn+mx)/2, kind), 7)
		default:
			label = strings.Repeat(" ", 7)
		}
		line.WriteString(axisStyle.Render(label + " "))
		// Plot
		for cx := 0; cx < width; cx++ {
			var bits int
			// Braille bit positions:
			//   1 4
			//   2 5
			//   3 6
			//   7 8
			brailleBits := [4][2]int{
				{0x01, 0x08},
				{0x02, 0x10},
				{0x04, 0x20},
				{0x40, 0x80},
			}
			x := cx * 2
			y0 := ry * 4
			for dy := 0; dy < 4; dy++ {
				for dx := 0; dx < 2; dx++ {
					yy := y0 + dy
					xx := x + dx
					if yy < pxH && xx < pxW && pixels[yy][xx] {
						bits |= brailleBits[dy][dx]
					}
				}
			}
			r := rune(0x2800 + bits)
			line.WriteString(lipgloss.NewStyle().Foreground(accent).Render(string(r)))
		}
		out[ry] = line.String()
	}
	return out
}

// renderHistogram draws vertical bars for each bin with a count
// label below. Each bar uses a vertical block character chosen
// per partial-block resolution so heights step in eighths.
func renderHistogram(values []float64, width, height int, accent lipgloss.Color) []string {
	if width < 8 || height < 3 || len(values) == 0 {
		return nil
	}
	bins := width / 3
	if bins < 4 {
		bins = 4
	}
	if bins > 24 {
		bins = 24
	}
	mn, mx := math.Inf(1), math.Inf(-1)
	for _, v := range values {
		if v < mn {
			mn = v
		}
		if v > mx {
			mx = v
		}
	}
	if mn == mx {
		mx = mn + 1
	}
	binW := (mx - mn) / float64(bins)
	counts := make([]int, bins)
	for _, v := range values {
		i := int((v - mn) / binW)
		if i >= bins {
			i = bins - 1
		}
		if i < 0 {
			i = 0
		}
		counts[i]++
	}
	maxC := 0
	for _, c := range counts {
		if c > maxC {
			maxC = c
		}
	}
	if maxC == 0 {
		maxC = 1
	}
	colW := 3
	plotH := height - 1
	out := make([]string, height)
	// Eight-step partial blocks for fractional row heights.
	partials := []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇'}
	for r := 0; r < plotH; r++ {
		var b strings.Builder
		for i := 0; i < bins; i++ {
			rowsHi := float64(counts[i]) / float64(maxC) * float64(plotH) * 8
			fullRows := int(rowsHi / 8)
			rem := int(math.Round(rowsHi - float64(fullRows*8)))
			rowFromBottom := plotH - 1 - r
			cellStr := ""
			switch {
			case rowFromBottom < fullRows:
				cellStr = strings.Repeat("█", colW)
			case rowFromBottom == fullRows && rem > 0:
				cellStr = strings.Repeat(string(partials[rem]), colW)
			default:
				cellStr = strings.Repeat(" ", colW)
			}
			b.WriteString(lipgloss.NewStyle().Foreground(accent).Render(cellStr))
		}
		out[r] = b.String()
	}
	// Axis
	var axis strings.Builder
	for i := 0; i < bins; i++ {
		lbl := strconv.Itoa(counts[i])
		if utf8.RuneCountInString(lbl) > colW {
			lbl = "·"
		}
		axis.WriteString(lipgloss.NewStyle().Foreground(overlay1).Render(padOrTrunc(lbl, colW)))
	}
	out[height-1] = axis.String()
	return out
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// renderPicker draws the file picker. Files are shown as
// horizontal cards with an icon, type chip, relative path,
// size, and last-modified date — all colour-coded by file type
// so the user can scan the list at a glance.
func (s *SheetView) renderPicker() string {
	width := s.width
	if width <= 0 {
		width = 80
	}
	height := s.height
	if height <= 0 {
		height = 24
	}

	// Title
	title := lipgloss.NewStyle().
		Background(mauve).Foreground(base).Bold(true).Width(width).Padding(0, 2).
		Render("▦  Spreadsheet — Open or Create")
	subtitle := ""
	switch {
	case s.pickerNewMode:
		subtitle = lipgloss.NewStyle().Background(mantle).Foreground(yellow).Bold(true).Width(width).Padding(0, 2).
			Render("Name your new spreadsheet")
	case s.pickerTemplateMode:
		subtitle = lipgloss.NewStyle().Background(mantle).Foreground(mauve).Bold(true).Width(width).Padding(0, 2).
			Render("Pick a template")
	default:
		subtitle = lipgloss.NewStyle().Background(mantle).Foreground(subtext0).Width(width).Padding(0, 2).
			Render(fmt.Sprintf("%d file(s) in this vault   ↑↓=move   ↵=open   n=new   r=refresh   ?=help   q=close",
				len(s.pickerFiles)))
	}

	var body []string
	if s.pickerTemplateMode {
		body = append(body, s.renderTemplatePickerBody(width, height)...)
	} else if s.pickerNewMode {
		tplLine := ""
		if s.pickerTemplate != nil {
			tplLine = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  Template: ") +
				lipgloss.NewStyle().Foreground(text).Render(s.pickerTemplate.Name) +
				lipgloss.NewStyle().Foreground(overlay1).Render("  ·  "+s.pickerTemplate.Description)
		}
		body = append(body,
			"",
			tplLine,
			"",
			lipgloss.NewStyle().Foreground(text).Render("  Filename:"),
			"",
			lipgloss.NewStyle().Background(surface0).Foreground(yellow).Bold(true).
				Width(width-4).Padding(0, 2).
				Render("  "+string(s.pickerNewName)+"▎"),
			"",
			lipgloss.NewStyle().Foreground(overlay1).Render("  ↵ create file   ·   ^U clear   ·   esc back to templates"),
			lipgloss.NewStyle().Foreground(overlay0).Render("  Default extension: .csv  (use .xlsx for Excel files)"),
		)
	} else if len(s.pickerFiles) == 0 {
		body = append(body,
			"",
			"",
			lipgloss.NewStyle().Foreground(overlay1).Width(width).Align(lipgloss.Center).
				Render("No CSV / TSV / XLSX files found in this vault."),
			"",
			lipgloss.NewStyle().Foreground(text).Width(width).Align(lipgloss.Center).
				Render("Press " +
					lipgloss.NewStyle().Foreground(yellow).Bold(true).Render("n") +
					" to create a new spreadsheet."),
		)
	} else {
		// File card list
		listH := height - 6
		if listH < 4 {
			listH = 4
		}
		if s.pickerCursor < s.pickerScroll {
			s.pickerScroll = s.pickerCursor
		}
		if s.pickerCursor >= s.pickerScroll+listH {
			s.pickerScroll = s.pickerCursor - listH + 1
		}
		end := s.pickerScroll + listH
		if end > len(s.pickerFiles) {
			end = len(s.pickerFiles)
		}
		for i := s.pickerScroll; i < end; i++ {
			body = append(body, s.renderPickerCard(s.pickerFiles[i], i == s.pickerCursor, width))
		}
	}

	var footerText string
	switch {
	case s.pickerTemplateMode:
		footerText = " ↵=pick · 1-9=quick pick · ↑↓=move · esc=back to files "
	case s.pickerNewMode:
		footerText = " ↵=create · ^U=clear · esc=back to templates "
	default:
		footerText = " ↵=open · n=new · r=refresh · ↑↓=move · gG=top/bottom · ?=help · q=close "
	}
	footer := lipgloss.NewStyle().Background(surface0).Foreground(subtext1).Width(width).
		Render(footerText)

	pieces := []string{title, subtitle}
	pieces = append(pieces, body...)
	out := lipgloss.JoinVertical(lipgloss.Left, pieces...)
	pad := height - lipgloss.Height(out) - 1
	if pad > 0 {
		out += strings.Repeat("\n", pad)
	}
	return out + "\n" + footer
}

// renderPickerCard draws one entry in the picker as a colored
// card with file icon, type chip, path, size, and modified date.
func (s *SheetView) renderPickerCard(f sheetPickerEntry, active bool, width int) string {
	ext := strings.ToUpper(strings.TrimPrefix(filepath.Ext(f.relPath), "."))
	icon, typeColor := svPickerIcon(ext)

	bg := base
	if active {
		bg = surface1
	}

	pointer := "  "
	if active {
		pointer = lipgloss.NewStyle().Background(bg).Foreground(mauve).Bold(true).Render(" ▸")
	} else {
		pointer = lipgloss.NewStyle().Background(bg).Render("  ")
	}

	iconStyle := lipgloss.NewStyle().Background(typeColor).Foreground(base).Bold(true).Render(" " + icon + " ")
	typeChip := lipgloss.NewStyle().Background(bg).Foreground(typeColor).Bold(true).Render(" " + ext + " ")

	when := f.modTime.Format("2006-01-02 15:04")
	whenAge := svRelativeAge(f.modTime)
	// "Recent" badge for files modified in the last 24 hours so
	// the user can spot what they just touched.
	recentBadge := ""
	if time.Since(f.modTime) < 24*time.Hour {
		recentBadge = lipgloss.NewStyle().Background(yellow).Foreground(base).Bold(true).
			Render(" NEW ")
	} else if time.Since(f.modTime) < 7*24*time.Hour {
		recentBadge = lipgloss.NewStyle().Background(green).Foreground(base).Bold(true).
			Render(" RECENT ")
	} else {
		recentBadge = lipgloss.NewStyle().Background(bg).Render("        ")
	}

	pathW := width - 60 // pointer + icon + type + badge + size + date + age
	if pathW < 12 {
		pathW = 12
	}
	pathStr := f.relPath
	if utf8.RuneCountInString(pathStr) > pathW {
		pathStr = "…" + truncRunes(pathStr, pathW-1)
	}
	pathStyled := lipgloss.NewStyle().Background(bg).Foreground(text).Bold(active).
		Render(" " + padOrTrunc(pathStr, pathW))

	sizeStyled := lipgloss.NewStyle().Background(bg).Foreground(subtext0).
		Render(padLeft(humanSize(f.size), 8) + "  ")
	dateStyled := lipgloss.NewStyle().Background(bg).Foreground(overlay1).
		Render(when + "  ")
	ageStyled := lipgloss.NewStyle().Background(bg).Foreground(lavender).
		Render(padLeft(whenAge, 9))

	line := pointer + iconStyle + typeChip + recentBadge + pathStyled + sizeStyled + dateStyled + ageStyled
	w := lipgloss.Width(line)
	if w < width {
		line += lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", width-w))
	}
	return line
}

// renderTemplatePickerBody draws the template-selection screen.
// Each template gets a card with a digit shortcut, name, and
// description; the active template's card is highlighted.
func (s *SheetView) renderTemplatePickerBody(width, height int) []string {
	templates := allSheetTemplates()
	listH := height - 6
	if listH < 6 {
		listH = 6
	}
	if s.pickerTemplateCursor < 0 {
		s.pickerTemplateCursor = 0
	}
	if s.pickerTemplateCursor >= len(templates) {
		s.pickerTemplateCursor = len(templates) - 1
	}
	// Visible window
	scroll := 0
	if s.pickerTemplateCursor >= scroll+listH {
		scroll = s.pickerTemplateCursor - listH + 1
	}
	end := scroll + listH
	if end > len(templates) {
		end = len(templates)
	}

	var out []string
	out = append(out, "")
	for i := scroll; i < end; i++ {
		tpl := templates[i]
		active := i == s.pickerTemplateCursor
		bg := base
		if active {
			bg = surface1
		}
		// Digit shortcut chip — grey if > 9.
		chip := "  "
		if i < 9 {
			chip = lipgloss.NewStyle().Background(mauve).Foreground(base).Bold(true).
				Render(fmt.Sprintf(" %d ", i+1))
		} else {
			chip = lipgloss.NewStyle().Background(surface2).Foreground(text).
				Render("   ")
		}
		pointer := lipgloss.NewStyle().Background(bg).Render("  ")
		if active {
			pointer = lipgloss.NewStyle().Background(bg).Foreground(mauve).Bold(true).Render(" ▸")
		}
		// Template "icon" character — distinct per category for
		// quick visual scanning.
		icon, iconColor := svTemplateIcon(tpl.ID)
		iconStyled := lipgloss.NewStyle().Background(iconColor).Foreground(base).Bold(true).
			Render(" " + icon + " ")

		nameStyled := lipgloss.NewStyle().Background(bg).Foreground(text).Bold(active).
			Render(" " + padOrTrunc(tpl.Name, 28))
		descStyled := lipgloss.NewStyle().Background(bg).Foreground(subtext0).
			Render(" " + tpl.Description)

		line := pointer + chip + iconStyled + nameStyled + descStyled
		w := lipgloss.Width(line)
		if w < width {
			line += lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", width-w))
		}
		out = append(out, line)
	}
	return out
}

// svTemplateIcon returns a glyph + accent color per template
// category. Helps scanning the list when there are many.
func svTemplateIcon(id string) (string, lipgloss.Color) {
	switch id {
	case "blank":
		return "▤", overlay1
	case "monthly-budget", "expense-tracker", "invoice":
		return "$", green
	case "task-tracker", "sales-pipeline":
		return "✓", sky
	case "habit-tracker":
		return "●", peach
	case "time-log":
		return "⌚", lavender
	case "workout":
		return "⚡", red
	case "reading-list":
		return "♔", yellow
	default:
		return "▦", mauve
	}
}

func svPickerIcon(ext string) (string, lipgloss.Color) {
	switch ext {
	case "XLSX", "XLSM":
		return "▦", green
	case "CSV":
		return "▤", sky
	case "TSV":
		return "▥", lavender
	default:
		return "·", overlay1
	}
}

// svRelativeAge returns "5m ago", "2d ago", etc.
func svRelativeAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(d.Hours()/24/7))
	default:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/24/30))
	}
}

func humanSize(n int64) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1fM", float64(n)/(1024*1024))
	case n >= 1024:
		return fmt.Sprintf("%.1fK", float64(n)/1024)
	default:
		return fmt.Sprintf("%dB", n)
	}
}

func a1Address(row, col int) string {
	return fmt.Sprintf("%s%d", colLetters(col), row+1)
}

func colLetters(c int) string {
	if c < 0 {
		return ""
	}
	out := ""
	c++
	for c > 0 {
		c--
		out = string(rune('A'+(c%26))) + out
		c /= 26
	}
	return out
}

func parseA1(addr string) (row, col int, ok bool) {
	if addr == "" {
		return 0, 0, false
	}
	letters := ""
	digits := ""
	for _, r := range addr {
		switch {
		case r >= 'A' && r <= 'Z':
			letters += string(r)
		case r >= '0' && r <= '9':
			digits += string(r)
		default:
			return 0, 0, false
		}
	}
	if letters == "" || digits == "" {
		return 0, 0, false
	}
	c := 0
	for _, r := range letters {
		c = c*26 + int(r-'A'+1)
	}
	c--
	r, err := strconv.Atoi(digits)
	if err != nil || r <= 0 {
		return 0, 0, false
	}
	return r - 1, c, true
}

func truncRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

func padOrTrunc(s string, w int) string {
	r := []rune(s)
	if len(r) > w {
		return string(r[:w])
	}
	return s + strings.Repeat(" ", w-len(r))
}

func padLeft(s string, w int) string {
	r := utf8.RuneCountInString(s)
	if r >= w {
		return s
	}
	return strings.Repeat(" ", w-r) + s
}

func padCenter(s string, w int, st lipgloss.Style) string {
	r := utf8.RuneCountInString(s)
	if r >= w {
		return st.Render(truncRunes(s, w))
	}
	left := (w - r) / 2
	right := w - r - left
	return st.Render(strings.Repeat(" ", left) + s + strings.Repeat(" ", right))
}

func fmtNum(v float64) string {
	if math.Abs(v) >= 1e9 {
		return fmt.Sprintf("%.2fB", v/1e9)
	}
	if math.Abs(v) >= 1e6 {
		return fmt.Sprintf("%.2fM", v/1e6)
	}
	if math.Abs(v) >= 1e4 {
		return fmt.Sprintf("%.1fk", v/1e3)
	}
	if v == math.Trunc(v) {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	return strconv.FormatFloat(v, 'f', 2, 64)
}

func maxIntSv(a, b int) int {
	if a > b {
		return a
	}
	return b
}
