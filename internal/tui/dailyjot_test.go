package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// jotEntry — isTask / isDone / taskText
// ---------------------------------------------------------------------------

func TestJotEntry_IsTask(t *testing.T) {
	tests := []struct {
		text   string
		isTask bool
		isDone bool
		task   string
	}{
		{"plain text", false, false, "plain text"},
		{"[ ] buy milk", true, false, "buy milk"},
		{"[x] buy milk", true, true, "buy milk"},
		{"[x]no space", false, false, "[x]no space"},
		{"[ ]no space", false, false, "[ ]no space"},
		{"", false, false, ""},
		{"[ ] ", true, false, ""},
		{"[x] ", true, true, ""},
		{"[ ] [ ] nested", true, false, "[ ] nested"},
	}
	for _, tt := range tests {
		e := jotEntry{Time: "10:00", Text: tt.text}
		if got := e.isTask(); got != tt.isTask {
			t.Errorf("isTask(%q) = %v, want %v", tt.text, got, tt.isTask)
		}
		if got := e.isDone(); got != tt.isDone {
			t.Errorf("isDone(%q) = %v, want %v", tt.text, got, tt.isDone)
		}
		if got := e.taskText(); got != tt.task {
			t.Errorf("taskText(%q) = %q, want %q", tt.text, got, tt.task)
		}
	}
}

// ---------------------------------------------------------------------------
// NewDailyJot — initial state
// ---------------------------------------------------------------------------

func TestNewDailyJot_InitialState(t *testing.T) {
	dj := NewDailyJot()
	if dj.active {
		t.Error("expected active=false")
	}
	if dj.mode != jotInput {
		t.Errorf("expected mode=jotInput, got %d", dj.mode)
	}
	if len(dj.days) != 0 {
		t.Errorf("expected 0 days, got %d", len(dj.days))
	}
}

// ---------------------------------------------------------------------------
// File I/O — loadDay / saveDay round-trip
// ---------------------------------------------------------------------------

func setupTempVault(t *testing.T) (string, *DailyJot) {
	t.Helper()
	tmp := t.TempDir()
	dj := &DailyJot{
		vaultRoot:  tmp,
		jotsFolder: "Jots",
	}
	return tmp, dj
}

func writeJotFile(t *testing.T, dir, date, content string) {
	t.Helper()
	jotsDir := filepath.Join(dir, "Jots")
	if err := os.MkdirAll(jotsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jotsDir, date+".md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDay_ValidFile(t *testing.T) {
	tmp, dj := setupTempVault(t)
	writeJotFile(t, tmp, "2026-03-18", `---
date: 2026-03-18
type: jot
tags: [jot]
---
- 09:15 — First entry
- 14:30 — Second entry with [[link]] and #tag
`)
	day := dj.loadDay("2026-03-18")
	if day.Date != "2026-03-18" {
		t.Errorf("date = %q, want 2026-03-18", day.Date)
	}
	if len(day.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(day.Entries))
	}
	if day.Entries[0].Time != "09:15" || day.Entries[0].Text != "First entry" {
		t.Errorf("entry[0] = %+v", day.Entries[0])
	}
	if day.Entries[1].Time != "14:30" || day.Entries[1].Text != "Second entry with [[link]] and #tag" {
		t.Errorf("entry[1] = %+v", day.Entries[1])
	}
}

func TestLoadDay_MissingFile(t *testing.T) {
	_, dj := setupTempVault(t)
	day := dj.loadDay("2099-01-01")
	if len(day.Entries) != 0 {
		t.Errorf("expected 0 entries for missing file, got %d", len(day.Entries))
	}
}

func TestLoadDay_FrontmatterStateMachine(t *testing.T) {
	tmp, dj := setupTempVault(t)
	// File with a horizontal rule (---) in content after frontmatter
	writeJotFile(t, tmp, "2026-03-18", `---
date: 2026-03-18
type: jot
---
- 09:00 — Before rule
---
- 10:00 — After rule
`)
	day := dj.loadDay("2026-03-18")
	if len(day.Entries) != 2 {
		t.Fatalf("expected 2 entries (both sides of ---), got %d", len(day.Entries))
	}
	if day.Entries[0].Text != "Before rule" {
		t.Errorf("entry[0].Text = %q, want 'Before rule'", day.Entries[0].Text)
	}
	if day.Entries[1].Text != "After rule" {
		t.Errorf("entry[1].Text = %q, want 'After rule'", day.Entries[1].Text)
	}
}

func TestLoadDay_NoFrontmatter(t *testing.T) {
	tmp, dj := setupTempVault(t)
	// File without frontmatter at all
	writeJotFile(t, tmp, "2026-03-18", `- 09:00 — No frontmatter entry
`)
	day := dj.loadDay("2026-03-18")
	if len(day.Entries) != 1 {
		t.Fatalf("expected 1 entry without frontmatter, got %d", len(day.Entries))
	}
}

func TestLoadDay_MalformedLines(t *testing.T) {
	tmp, dj := setupTempVault(t)
	writeJotFile(t, tmp, "2026-03-18", `---
date: 2026-03-18
---
- 09:15 — Good entry
- not a time entry
- AB:CD — Letters not digits
- 25:00 — Invalid hour
- 10:61 — Invalid minute
- 10:30 — Valid entry
Just a random line
- 99:99 — Out of range
`)
	day := dj.loadDay("2026-03-18")
	if len(day.Entries) != 2 {
		t.Errorf("expected 2 valid entries, got %d", len(day.Entries))
		for i, e := range day.Entries {
			t.Logf("  entry[%d] = %s %s", i, e.Time, e.Text)
		}
	}
}

func TestLoadDay_TaskEntries(t *testing.T) {
	tmp, dj := setupTempVault(t)
	writeJotFile(t, tmp, "2026-03-18", `---
date: 2026-03-18
---
- 09:00 — [ ] incomplete task
- 10:00 — [x] done task
- 11:00 — normal jot
`)
	day := dj.loadDay("2026-03-18")
	if len(day.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(day.Entries))
	}
	if !day.Entries[0].isTask() || day.Entries[0].isDone() {
		t.Error("entry[0] should be incomplete task")
	}
	if !day.Entries[1].isTask() || !day.Entries[1].isDone() {
		t.Error("entry[1] should be done task")
	}
	if day.Entries[2].isTask() {
		t.Error("entry[2] should not be a task")
	}
}

func TestSaveDay_RoundTrip(t *testing.T) {
	tmp, dj := setupTempVault(t)
	day := jotDay{
		Date:  "2026-03-18",
		Label: "Today",
		Entries: []jotEntry{
			{Time: "09:15", Text: "First"},
			{Time: "14:30", Text: "Second with [[link]]"},
			{Time: "16:00", Text: "[ ] task entry"},
		},
	}
	if err := dj.saveDay(day); err != nil {
		t.Fatal(err)
	}

	loaded := dj.loadDay("2026-03-18")
	if len(loaded.Entries) != 3 {
		t.Fatalf("round-trip: got %d entries, want 3", len(loaded.Entries))
	}
	for i, e := range loaded.Entries {
		if e.Time != day.Entries[i].Time || e.Text != day.Entries[i].Text {
			t.Errorf("entry[%d] mismatch: got %+v, want %+v", i, e, day.Entries[i])
		}
	}

	// Verify file exists with correct content
	content, err := os.ReadFile(filepath.Join(tmp, "Jots", "2026-03-18.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "---\ndate: 2026-03-18\n") {
		t.Error("file missing frontmatter")
	}
}

func TestSaveDay_AtomicWrite(t *testing.T) {
	tmp, dj := setupTempVault(t)
	day := jotDay{
		Date:    "2026-03-18",
		Entries: []jotEntry{{Time: "09:00", Text: "test"}},
	}
	if err := dj.saveDay(day); err != nil {
		t.Fatal(err)
	}
	// Verify no temp files left behind
	entries, _ := os.ReadDir(filepath.Join(tmp, "Jots"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".jot-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestSaveDay_CreatesDirectory(t *testing.T) {
	tmp, dj := setupTempVault(t)
	day := jotDay{
		Date:    "2026-03-18",
		Entries: []jotEntry{{Time: "09:00", Text: "test"}},
	}
	// Jots directory doesn't exist yet
	if err := dj.saveDay(day); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "Jots", "2026-03-18.md")); err != nil {
		t.Errorf("expected file to exist: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Navigation — totalEntries / flatIndex
// ---------------------------------------------------------------------------

func TestTotalEntries(t *testing.T) {
	dj := DailyJot{
		days: []jotDay{
			{Entries: []jotEntry{{Time: "09:00", Text: "a"}, {Time: "10:00", Text: "b"}}},
			{Entries: []jotEntry{{Time: "11:00", Text: "c"}}},
			{Entries: nil},
		},
	}
	if got := dj.totalEntries(); got != 3 {
		t.Errorf("totalEntries() = %d, want 3", got)
	}
}

func TestTotalEntries_Empty(t *testing.T) {
	dj := DailyJot{}
	if got := dj.totalEntries(); got != 0 {
		t.Errorf("totalEntries() = %d, want 0", got)
	}
}

func TestFlatIndex(t *testing.T) {
	// Day 0 has entries [a, b, c] (displayed in reverse: c, b, a)
	// Day 1 has entries [d, e] (displayed in reverse: e, d)
	// Flat: cursor 0→c, 1→b, 2→a, 3→e, 4→d
	dj := DailyJot{
		days: []jotDay{
			{Entries: []jotEntry{
				{Time: "09:00", Text: "a"},
				{Time: "10:00", Text: "b"},
				{Time: "11:00", Text: "c"},
			}},
			{Entries: []jotEntry{
				{Time: "12:00", Text: "d"},
				{Time: "13:00", Text: "e"},
			}},
		},
	}

	tests := []struct {
		cursor   int
		dayIdx   int
		entryIdx int
		ok       bool
	}{
		{0, 0, 2, true},  // newest in day 0 (c)
		{1, 0, 1, true},  // middle (b)
		{2, 0, 0, true},  // oldest (a)
		{3, 1, 1, true},  // newest in day 1 (e)
		{4, 1, 0, true},  // oldest in day 1 (d)
		{5, 0, 0, false}, // out of bounds
		{-1, 0, 0, false},
	}
	for _, tt := range tests {
		dayIdx, entryIdx, ok := dj.flatIndex(tt.cursor)
		if ok != tt.ok {
			t.Errorf("flatIndex(%d) ok=%v, want %v", tt.cursor, ok, tt.ok)
			continue
		}
		if ok && (dayIdx != tt.dayIdx || entryIdx != tt.entryIdx) {
			t.Errorf("flatIndex(%d) = (%d,%d), want (%d,%d)", tt.cursor, dayIdx, entryIdx, tt.dayIdx, tt.entryIdx)
		}
	}
}

func TestFlatIndex_EmptyDays(t *testing.T) {
	dj := DailyJot{
		days: []jotDay{
			{Entries: nil},
			{Entries: []jotEntry{{Time: "09:00", Text: "a"}}},
		},
	}
	// Day 0 is empty, so cursor 0 should map to day 1's entry
	dayIdx, entryIdx, ok := dj.flatIndex(0)
	if !ok || dayIdx != 1 || entryIdx != 0 {
		t.Errorf("flatIndex(0) = (%d,%d,%v), want (1,0,true)", dayIdx, entryIdx, ok)
	}
}

// ---------------------------------------------------------------------------
// Mutations — appendJot / deleteJot / updateJot / toggleTask
// ---------------------------------------------------------------------------

func TestAppendJot(t *testing.T) {
	tmp, dj := setupTempVault(t)
	_ = tmp
	dj.days = []jotDay{{
		Date:  "2026-03-19", // must match today for the test
		Label: "Today",
	}}

	// Force today's date by pre-seeding the day
	dj.appendJot("test jot")
	// appendJot checks time.Now() so we just verify an entry was added
	total := dj.totalEntries()
	if total < 1 {
		t.Error("expected at least 1 entry after appendJot")
	}
}

func TestDeleteJot_BoundsCheck(t *testing.T) {
	_, dj := setupTempVault(t)
	dj.days = []jotDay{{
		Date:    "2026-03-18",
		Entries: []jotEntry{{Time: "09:00", Text: "a"}, {Time: "10:00", Text: "b"}},
	}}

	// Should not panic on out-of-bounds
	dj.deleteJot(-1, 0)
	dj.deleteJot(0, -1)
	dj.deleteJot(5, 0)
	dj.deleteJot(0, 5)

	if len(dj.days[0].Entries) != 2 {
		t.Errorf("expected 2 entries unchanged, got %d", len(dj.days[0].Entries))
	}

	// Valid delete
	dj.deleteJot(0, 0)
	if len(dj.days[0].Entries) != 1 {
		t.Errorf("expected 1 entry after delete, got %d", len(dj.days[0].Entries))
	}
	if dj.days[0].Entries[0].Text != "b" {
		t.Errorf("wrong entry remaining: %q", dj.days[0].Entries[0].Text)
	}
}

func TestUpdateJot_BoundsCheck(t *testing.T) {
	_, dj := setupTempVault(t)
	dj.days = []jotDay{{
		Date:    "2026-03-18",
		Entries: []jotEntry{{Time: "09:00", Text: "original"}},
	}}

	// Should not panic
	dj.updateJot(-1, 0, "nope")
	dj.updateJot(0, -1, "nope")
	dj.updateJot(5, 0, "nope")
	dj.updateJot(0, 5, "nope")

	if dj.days[0].Entries[0].Text != "original" {
		t.Error("out-of-bounds updateJot should not modify entries")
	}

	dj.updateJot(0, 0, "updated")
	if dj.days[0].Entries[0].Text != "updated" {
		t.Errorf("expected 'updated', got %q", dj.days[0].Entries[0].Text)
	}
}

func TestToggleTask(t *testing.T) {
	_, dj := setupTempVault(t)
	dj.days = []jotDay{{
		Date: "2026-03-18",
		Entries: []jotEntry{
			{Time: "09:00", Text: "plain jot"},
			{Time: "10:00", Text: "[ ] open task"},
			{Time: "11:00", Text: "[x] done task"},
		},
	}}

	// plain → task
	dj.toggleTask(0, 0)
	if dj.days[0].Entries[0].Text != "[ ] plain jot" {
		t.Errorf("expected '[ ] plain jot', got %q", dj.days[0].Entries[0].Text)
	}

	// open task → done
	dj.toggleTask(0, 1)
	if dj.days[0].Entries[1].Text != "[x] open task" {
		t.Errorf("expected '[x] open task', got %q", dj.days[0].Entries[1].Text)
	}

	// done task → open
	dj.toggleTask(0, 2)
	if dj.days[0].Entries[2].Text != "[ ] done task" {
		t.Errorf("expected '[ ] done task', got %q", dj.days[0].Entries[2].Text)
	}

	// Out-of-bounds should not panic
	dj.toggleTask(-1, 0)
	dj.toggleTask(0, -1)
	dj.toggleTask(99, 0)
}

// ---------------------------------------------------------------------------
// Carry-over
// ---------------------------------------------------------------------------

func TestCarryOverTasks(t *testing.T) {
	_, dj := setupTempVault(t)
	dj.days = []jotDay{
		{Date: "2026-03-19", Label: "Today", Entries: nil},
		{Date: "2026-03-18", Label: "Yesterday", Entries: []jotEntry{
			{Time: "09:00", Text: "[ ] incomplete task"},
			{Time: "10:00", Text: "[x] done task"},
			{Time: "11:00", Text: "plain jot"},
			{Time: "12:00", Text: "[ ] another incomplete"},
		}},
	}

	dj.carryOverTasks()

	if dj.carryOverCount != 2 {
		t.Errorf("carryOverCount = %d, want 2", dj.carryOverCount)
	}
	if len(dj.days[0].Entries) != 2 {
		t.Fatalf("expected 2 carried entries, got %d", len(dj.days[0].Entries))
	}
	if dj.days[0].Entries[0].Text != "[ ] incomplete task" {
		t.Errorf("entry[0] = %q, want '[ ] incomplete task'", dj.days[0].Entries[0].Text)
	}
	if dj.days[0].Entries[1].Text != "[ ] another incomplete" {
		t.Errorf("entry[1] = %q, want '[ ] another incomplete'", dj.days[0].Entries[1].Text)
	}
}

func TestCarryOverTasks_NoDuplicates(t *testing.T) {
	_, dj := setupTempVault(t)
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	dj.days = []jotDay{
		{Date: today, Label: "Today", Entries: []jotEntry{
			{Time: "08:00", Text: "[ ] already here"},
		}},
		{Date: yesterday, Label: "Yesterday", Entries: []jotEntry{
			{Time: "09:00", Text: "[ ] already here"},
		}},
	}

	dj.carryOverTasks()
	if dj.carryOverCount != 0 {
		t.Errorf("carryOverCount = %d, want 0 (no duplicates)", dj.carryOverCount)
	}
	if len(dj.days[0].Entries) != 1 {
		t.Errorf("expected 1 entry (no duplicate), got %d", len(dj.days[0].Entries))
	}
}

func TestCarryOverTasks_SingleDay(t *testing.T) {
	_, dj := setupTempVault(t)
	dj.days = []jotDay{
		{Date: "2026-03-19", Entries: []jotEntry{
			{Time: "09:00", Text: "[ ] task"},
		}},
	}
	// Should not panic with only one day
	dj.carryOverTasks()
	if len(dj.days[0].Entries) != 1 {
		t.Errorf("expected 1 entry unchanged, got %d", len(dj.days[0].Entries))
	}
}

// ---------------------------------------------------------------------------
// Filter — buildFilterIndex
// ---------------------------------------------------------------------------

func TestBuildFilterIndex(t *testing.T) {
	dj := DailyJot{
		days: []jotDay{
			{Entries: []jotEntry{
				{Time: "09:00", Text: "apple pie"},
				{Time: "10:00", Text: "banana split"},
				{Time: "11:00", Text: "apple sauce"},
			}},
			{Entries: []jotEntry{
				{Time: "12:00", Text: "orange juice"},
				{Time: "13:00", Text: "apple cider"},
			}},
		},
	}

	dj.filterQuery = "apple"
	dj.buildFilterIndex()

	// Flat order (reverse-chron within each day):
	// cursor 0 = day0/entry2 (apple sauce), cursor 1 = day0/entry0 (apple pie)
	// cursor 2 = day0/entry0... wait, let me think.
	// Day 0 entries iterated: i=2 (apple sauce, pos=0), i=1 (banana split, pos=1), i=0 (apple pie, pos=2)
	// Day 1 entries iterated: i=1 (apple cider, pos=3), i=0 (orange juice, pos=4)
	// Matches: pos 0 (apple sauce), pos 2 (apple pie), pos 3 (apple cider)
	if len(dj.filteredIdxs) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(dj.filteredIdxs))
	}
	if dj.filteredIdxs[0] != 0 || dj.filteredIdxs[1] != 2 || dj.filteredIdxs[2] != 3 {
		t.Errorf("filteredIdxs = %v, want [0, 2, 3]", dj.filteredIdxs)
	}
}

func TestBuildFilterIndex_EmptyQuery(t *testing.T) {
	dj := DailyJot{
		filterQuery: "",
		days:        []jotDay{{Entries: []jotEntry{{Time: "09:00", Text: "test"}}}},
	}
	dj.buildFilterIndex()
	if len(dj.filteredIdxs) != 0 {
		t.Errorf("empty query should return no indices, got %d", len(dj.filteredIdxs))
	}
}

func TestBuildFilterIndex_CaseInsensitive(t *testing.T) {
	dj := DailyJot{
		filterQuery: "HELLO",
		days:        []jotDay{{Entries: []jotEntry{{Time: "09:00", Text: "hello world"}}}},
	}
	dj.buildFilterIndex()
	if len(dj.filteredIdxs) != 1 {
		t.Errorf("case-insensitive filter should find 1, got %d", len(dj.filteredIdxs))
	}
}

func TestBuildFilterIndex_CursorClamp(t *testing.T) {
	dj := DailyJot{
		filterQuery: "xyz",
		cursor:      5,
		days:        []jotDay{{Entries: []jotEntry{{Time: "09:00", Text: "abc"}}}},
	}
	dj.buildFilterIndex()
	if dj.cursor != 0 {
		t.Errorf("cursor should be clamped to 0, got %d", dj.cursor)
	}
}

// ---------------------------------------------------------------------------
// Text cursor
// ---------------------------------------------------------------------------

func TestDailyJot_InsertChar(t *testing.T) {
	dj := &DailyJot{mode: jotInput}
	dj.insertChar('a')
	dj.insertChar('b')
	dj.insertChar('c')
	if string(dj.inputRunes) != "abc" {
		t.Errorf("expected 'abc', got %q", string(dj.inputRunes))
	}
	if dj.inputCursor != 3 {
		t.Errorf("cursor = %d, want 3", dj.inputCursor)
	}

	// Insert in middle
	dj.inputCursor = 1
	dj.insertChar('X')
	if string(dj.inputRunes) != "aXbc" {
		t.Errorf("expected 'aXbc', got %q", string(dj.inputRunes))
	}
	if dj.inputCursor != 2 {
		t.Errorf("cursor = %d, want 2", dj.inputCursor)
	}
}

func TestDeleteCharBack(t *testing.T) {
	dj := &DailyJot{
		mode:        jotInput,
		inputRunes:  []rune("hello"),
		inputCursor: 5,
	}
	dj.deleteCharBack()
	if string(dj.inputRunes) != "hell" {
		t.Errorf("expected 'hell', got %q", string(dj.inputRunes))
	}

	// Delete from middle
	dj.inputCursor = 2
	dj.deleteCharBack()
	if string(dj.inputRunes) != "hll" {
		t.Errorf("expected 'hll', got %q", string(dj.inputRunes))
	}

	// Delete at position 0 — noop
	dj.inputCursor = 0
	dj.deleteCharBack()
	if string(dj.inputRunes) != "hll" {
		t.Errorf("expected 'hll' unchanged, got %q", string(dj.inputRunes))
	}
}

func TestInsertChar_Unicode(t *testing.T) {
	dj := &DailyJot{mode: jotInput}
	dj.insertChar('é')
	dj.insertChar('🎉')
	dj.insertChar('日')
	if string(dj.inputRunes) != "é🎉日" {
		t.Errorf("expected 'é🎉日', got %q", string(dj.inputRunes))
	}
	if dj.inputCursor != 3 {
		t.Errorf("cursor = %d, want 3", dj.inputCursor)
	}
}

// ---------------------------------------------------------------------------
// Update — input mode
// ---------------------------------------------------------------------------

func jotKeyMsg(key string) tea.Msg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

func jotKeySpecial(t tea.KeyType) tea.Msg {
	return tea.KeyMsg{Type: t}
}

func TestUpdateInput_EscCloses(t *testing.T) {
	dj := DailyJot{OverlayBase: OverlayBase{active: true}, mode: jotInput}
	dj, _ = dj.Update(jotKeySpecial(tea.KeyEsc))
	if dj.active {
		t.Error("esc should close overlay")
	}
}

func TestUpdateInput_EnterAppendsJot(t *testing.T) {
	tmp, djp := setupTempVault(t)
	_ = tmp
	djp.active = true
	djp.mode = jotInput
	djp.inputRunes = []rune("test jot")
	djp.inputCursor = 8
	dj := *djp

	dj, _ = dj.Update(jotKeySpecial(tea.KeyEnter))
	if len(dj.inputRunes) != 0 {
		t.Error("input should be cleared after enter")
	}
	if dj.inputCursor != 0 {
		t.Error("cursor should be reset after enter")
	}
}

func TestUpdateInput_EnterEmptyNoop(t *testing.T) {
	dj := DailyJot{OverlayBase: OverlayBase{active: true}, mode: jotInput, inputRunes: nil}
	before := dj.totalEntries()
	dj, _ = dj.Update(jotKeySpecial(tea.KeyEnter))
	if dj.totalEntries() != before {
		t.Error("empty enter should not add entry")
	}
}

func TestUpdateInput_TaskShorthand(t *testing.T) {
	tmp, djp := setupTempVault(t)
	_ = tmp
	djp.active = true
	djp.mode = jotInput
	djp.inputRunes = []rune("[] buy milk")
	djp.inputCursor = 11
	dj := *djp

	dj, _ = dj.Update(jotKeySpecial(tea.KeyEnter))
	if dj.totalEntries() == 0 {
		t.Fatal("expected entry after enter")
	}
	// The most recently added entry should have "[ ] " prefix
	lastEntry := dj.days[0].Entries[len(dj.days[0].Entries)-1]
	if !lastEntry.isTask() {
		t.Errorf("expected task, got %q", lastEntry.Text)
	}
	if lastEntry.taskText() != "buy milk" {
		t.Errorf("expected 'buy milk', got %q", lastEntry.taskText())
	}
}

func TestUpdateInput_DownToBrowse(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true},
		mode:        jotInput,
		days:        []jotDay{{Entries: []jotEntry{{Time: "09:00", Text: "a"}}}},
	}
	dj, _ = dj.Update(jotKeySpecial(tea.KeyDown))
	if dj.mode != jotBrowse {
		t.Errorf("expected jotBrowse, got %d", dj.mode)
	}
	if dj.cursor != 0 {
		t.Errorf("cursor = %d, want 0", dj.cursor)
	}
}

func TestUpdateInput_DownNoEntries(t *testing.T) {
	dj := DailyJot{OverlayBase: OverlayBase{active: true}, mode: jotInput}
	dj, _ = dj.Update(jotKeySpecial(tea.KeyDown))
	if dj.mode != jotInput {
		t.Error("down with no entries should stay in input mode")
	}
}

func TestUpdateInput_CursorMovement(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true},
		mode:        jotInput,
		inputRunes:  []rune("hello"),
		inputCursor: 3,
	}

	// Left
	dj, _ = dj.Update(jotKeySpecial(tea.KeyLeft))
	if dj.inputCursor != 2 {
		t.Errorf("left: cursor = %d, want 2", dj.inputCursor)
	}

	// Right
	dj, _ = dj.Update(jotKeySpecial(tea.KeyRight))
	if dj.inputCursor != 3 {
		t.Errorf("right: cursor = %d, want 3", dj.inputCursor)
	}

	// Home
	dj, _ = dj.Update(jotKeyMsg("ctrl+a"))
	if dj.inputCursor != 0 {
		t.Errorf("home: cursor = %d, want 0", dj.inputCursor)
	}

	// End
	dj, _ = dj.Update(jotKeyMsg("ctrl+e"))
	if dj.inputCursor != 5 {
		t.Errorf("end: cursor = %d, want 5", dj.inputCursor)
	}
}

func TestUpdateInput_CtrlU(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true},
		mode:        jotInput,
		inputRunes:  []rune("hello world"),
		inputCursor: 5,
	}
	dj, _ = dj.Update(jotKeyMsg("ctrl+u"))
	if string(dj.inputRunes) != " world" {
		t.Errorf("ctrl+u: got %q, want ' world'", string(dj.inputRunes))
	}
	if dj.inputCursor != 0 {
		t.Errorf("ctrl+u: cursor = %d, want 0", dj.inputCursor)
	}
}

func TestUpdateInput_CtrlK(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true},
		mode:        jotInput,
		inputRunes:  []rune("hello world"),
		inputCursor: 5,
	}
	dj, _ = dj.Update(jotKeyMsg("ctrl+k"))
	if string(dj.inputRunes) != "hello" {
		t.Errorf("ctrl+k: got %q, want 'hello'", string(dj.inputRunes))
	}
}

// ---------------------------------------------------------------------------
// Update — browse mode
// ---------------------------------------------------------------------------

func TestUpdateBrowse_Navigation(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true, width: 100, height: 60},
		mode:        jotBrowse,
		cursor:      0,
		days: []jotDay{
			{Entries: []jotEntry{
				{Time: "09:00", Text: "a"},
				{Time: "10:00", Text: "b"},
				{Time: "11:00", Text: "c"},
			}},
		},
	}

	// j moves down
	dj, _ = dj.Update(jotKeyMsg("j"))
	if dj.cursor != 1 {
		t.Errorf("j: cursor = %d, want 1", dj.cursor)
	}

	// k moves up
	dj, _ = dj.Update(jotKeyMsg("k"))
	if dj.cursor != 0 {
		t.Errorf("k: cursor = %d, want 0", dj.cursor)
	}

	// k at top returns to input
	dj, _ = dj.Update(jotKeyMsg("k"))
	if dj.mode != jotInput {
		t.Error("k at top should return to input mode")
	}
}

func TestUpdateBrowse_DownBoundary(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true, width: 100, height: 60},
		mode:        jotBrowse,
		cursor:      1,
		days:        []jotDay{{Entries: []jotEntry{{Time: "09:00", Text: "a"}, {Time: "10:00", Text: "b"}}}},
	}
	dj, _ = dj.Update(jotKeyMsg("j"))
	if dj.cursor != 1 {
		t.Errorf("j at bottom: cursor = %d, want 1 (no change)", dj.cursor)
	}
}

func TestUpdateBrowse_EscCloses(t *testing.T) {
	dj := DailyJot{OverlayBase: OverlayBase{active: true}, mode: jotBrowse}
	dj, _ = dj.Update(jotKeySpecial(tea.KeyEsc))
	if dj.active {
		t.Error("esc should close")
	}
}

func TestUpdateBrowse_IReturnsToInput(t *testing.T) {
	dj := DailyJot{OverlayBase: OverlayBase{active: true}, mode: jotBrowse, pendingDelete: true, statusMsg: "test"}
	dj, _ = dj.Update(jotKeyMsg("i"))
	if dj.mode != jotInput {
		t.Error("i should switch to input mode")
	}
	if dj.pendingDelete {
		t.Error("pendingDelete should be cleared")
	}
	if dj.statusMsg != "" {
		t.Error("statusMsg should be cleared")
	}
}

func TestUpdateBrowse_SlashToFilter(t *testing.T) {
	dj := DailyJot{OverlayBase: OverlayBase{active: true}, mode: jotBrowse, cursor: 5, pendingDelete: true}
	dj, _ = dj.Update(jotKeyMsg("/"))
	if dj.mode != jotFilter {
		t.Error("/ should switch to filter mode")
	}
	if dj.cursor != 0 {
		t.Error("cursor should reset to 0")
	}
	if dj.pendingDelete {
		t.Error("pendingDelete should be cleared")
	}
}

func TestUpdateBrowse_SpaceToggleTask(t *testing.T) {
	_, djp := setupTempVault(t)
	djp.active = true
	djp.mode = jotBrowse
	djp.cursor = 0
	djp.days = []jotDay{{
		Date:    "2026-03-18",
		Entries: []jotEntry{{Time: "09:00", Text: "plain"}},
	}}
	dj := *djp

	dj, _ = dj.Update(jotKeyMsg(" "))
	if dj.days[0].Entries[0].Text != "[ ] plain" {
		t.Errorf("space should convert to task, got %q", dj.days[0].Entries[0].Text)
	}
}

func TestUpdateBrowse_SpacePastDayNoop(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true, width: 100, height: 60},
		mode:        jotBrowse,
		cursor:      1, // maps to day 1
		days: []jotDay{
			{Entries: nil},
			{Date: "2026-03-17", Entries: []jotEntry{{Time: "09:00", Text: "old"}}},
		},
	}
	dj, _ = dj.Update(jotKeyMsg(" "))
	if dj.days[1].Entries[0].Text != "old" {
		t.Error("space on past day should be noop")
	}
}

func TestUpdateBrowse_EnterToEdit(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true},
		mode:        jotBrowse,
		cursor:      0,
		days: []jotDay{{
			Date:    "2026-03-18",
			Entries: []jotEntry{{Time: "09:00", Text: "edit me"}},
		}},
	}
	dj, _ = dj.Update(jotKeySpecial(tea.KeyEnter))
	if dj.mode != jotEdit {
		t.Errorf("enter should switch to edit mode, got %d", dj.mode)
	}
	if string(dj.editRunes) != "edit me" {
		t.Errorf("editRunes = %q, want 'edit me'", string(dj.editRunes))
	}
	if dj.editCursor != 7 {
		t.Errorf("editCursor = %d, want 7", dj.editCursor)
	}
}

func TestUpdateBrowse_DeleteConfirmation(t *testing.T) {
	_, djp := setupTempVault(t)
	djp.active = true
	djp.mode = jotBrowse
	djp.cursor = 0
	djp.days = []jotDay{{
		Date: "2026-03-18",
		Entries: []jotEntry{
			{Time: "09:00", Text: "a"},
			{Time: "10:00", Text: "b"},
		},
	}}
	dj := *djp

	// First d — pending
	dj, _ = dj.Update(jotKeyMsg("d"))
	if !dj.pendingDelete {
		t.Error("first d should set pendingDelete")
	}
	if dj.statusMsg == "" {
		t.Error("should have status message")
	}

	// Second d — confirm delete
	dj, _ = dj.Update(jotKeyMsg("d"))
	if dj.pendingDelete {
		t.Error("pendingDelete should be cleared after confirm")
	}
	if dj.totalEntries() != 1 {
		t.Errorf("expected 1 entry after delete, got %d", dj.totalEntries())
	}
}

func TestUpdateBrowse_DeleteCancelledByOtherKey(t *testing.T) {
	dj := DailyJot{
		OverlayBase:   OverlayBase{active: true},
		mode:          jotBrowse,
		pendingDelete: true,
		statusMsg:     "Press d again to delete",
		cursor:        0,
		days:          []jotDay{{Date: "2026-03-18", Entries: []jotEntry{{Time: "09:00", Text: "a"}}}},
	}

	dj, _ = dj.Update(jotKeyMsg("j"))
	if dj.pendingDelete {
		t.Error("non-d key should cancel pendingDelete")
	}
	if dj.statusMsg != "" {
		t.Error("statusMsg should be cleared")
	}
}

// ---------------------------------------------------------------------------
// Update — edit mode
// ---------------------------------------------------------------------------

func TestUpdateEdit_EscCancels(t *testing.T) {
	dj := DailyJot{OverlayBase: OverlayBase{active: true}, mode: jotEdit}
	dj, _ = dj.Update(jotKeySpecial(tea.KeyEsc))
	if dj.mode != jotBrowse {
		t.Error("esc in edit should return to browse")
	}
}

func TestUpdateEdit_EnterSaves(t *testing.T) {
	_, djp := setupTempVault(t)
	djp.active = true
	djp.mode = jotEdit
	djp.editDayIdx = 0
	djp.editEntryIdx = 0
	djp.editRunes = []rune("updated text")
	djp.editCursor = 12
	djp.days = []jotDay{{
		Date:    "2026-03-18",
		Entries: []jotEntry{{Time: "09:00", Text: "original"}},
	}}
	dj := *djp

	dj, _ = dj.Update(jotKeySpecial(tea.KeyEnter))
	if dj.mode != jotBrowse {
		t.Error("enter should return to browse")
	}
	if dj.days[0].Entries[0].Text != "updated text" {
		t.Errorf("expected 'updated text', got %q", dj.days[0].Entries[0].Text)
	}
}

func TestUpdateEdit_EnterStaleIndex(t *testing.T) {
	_, djp := setupTempVault(t)
	djp.active = true
	djp.mode = jotEdit
	djp.editDayIdx = 0
	djp.editEntryIdx = 5 // stale — out of bounds
	djp.editRunes = []rune("updated")
	djp.editCursor = 7
	djp.days = []jotDay{{
		Date:    "2026-03-18",
		Entries: []jotEntry{{Time: "09:00", Text: "original"}},
	}}
	dj := *djp

	dj, _ = dj.Update(jotKeySpecial(tea.KeyEnter))
	if dj.mode != jotBrowse {
		t.Error("should return to browse even with stale index")
	}
	if dj.days[0].Entries[0].Text != "original" {
		t.Error("stale index should not modify any entry")
	}
}

func TestUpdateEdit_CursorMovement(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true},
		mode:        jotEdit,
		editRunes:   []rune("hello"),
		editCursor:  3,
	}

	dj, _ = dj.Update(jotKeySpecial(tea.KeyLeft))
	if dj.editCursor != 2 {
		t.Errorf("left: editCursor = %d, want 2", dj.editCursor)
	}

	dj, _ = dj.Update(jotKeySpecial(tea.KeyRight))
	if dj.editCursor != 3 {
		t.Errorf("right: editCursor = %d, want 3", dj.editCursor)
	}

	dj, _ = dj.Update(jotKeyMsg("ctrl+a"))
	if dj.editCursor != 0 {
		t.Errorf("home: editCursor = %d, want 0", dj.editCursor)
	}

	dj, _ = dj.Update(jotKeyMsg("ctrl+e"))
	if dj.editCursor != 5 {
		t.Errorf("end: editCursor = %d, want 5", dj.editCursor)
	}
}

// ---------------------------------------------------------------------------
// Update — filter mode
// ---------------------------------------------------------------------------

func TestUpdateFilter_EscReturnsToBrowse(t *testing.T) {
	dj := DailyJot{OverlayBase: OverlayBase{active: true}, mode: jotFilter, cursor: 3}
	dj, _ = dj.Update(jotKeySpecial(tea.KeyEsc))
	if dj.mode != jotBrowse {
		t.Error("esc should return to browse")
	}
	if dj.cursor != 0 {
		t.Error("cursor should reset to 0")
	}
}

func TestUpdateFilter_EnterJumps(t *testing.T) {
	dj := DailyJot{
		OverlayBase:  OverlayBase{active: true, width: 100, height: 60},
		mode:         jotFilter,
		cursor:       0,
		filteredIdxs: []int{5, 10},
		days: []jotDay{
			{Entries: make([]jotEntry, 6)},
			{Entries: make([]jotEntry, 5)},
		},
	}
	dj, _ = dj.Update(jotKeySpecial(tea.KeyEnter))
	if dj.mode != jotBrowse {
		t.Error("enter should switch to browse")
	}
	if dj.cursor != 5 {
		t.Errorf("cursor = %d, want 5 (first filtered match)", dj.cursor)
	}
}

func TestUpdateFilter_EnterNoMatches(t *testing.T) {
	dj := DailyJot{OverlayBase: OverlayBase{active: true}, mode: jotFilter, filteredIdxs: nil}
	dj, _ = dj.Update(jotKeySpecial(tea.KeyEnter))
	if dj.mode != jotBrowse {
		t.Error("enter with no matches should switch to browse")
	}
	if dj.cursor != 0 {
		t.Error("cursor should be 0")
	}
}

func TestUpdateFilter_DownUp(t *testing.T) {
	dj := DailyJot{
		OverlayBase:  OverlayBase{active: true},
		mode:         jotFilter,
		cursor:       0,
		filteredIdxs: []int{1, 3, 5},
	}

	dj, _ = dj.Update(jotKeySpecial(tea.KeyDown))
	if dj.cursor != 1 {
		t.Errorf("down: cursor = %d, want 1", dj.cursor)
	}

	dj, _ = dj.Update(jotKeySpecial(tea.KeyUp))
	if dj.cursor != 0 {
		t.Errorf("up: cursor = %d, want 0", dj.cursor)
	}

	// Up at 0 stays at 0
	dj, _ = dj.Update(jotKeySpecial(tea.KeyUp))
	if dj.cursor != 0 {
		t.Error("up at 0 should stay at 0")
	}
}

func TestUpdateFilter_DownBoundary(t *testing.T) {
	dj := DailyJot{
		OverlayBase:  OverlayBase{active: true},
		mode:         jotFilter,
		cursor:       1,
		filteredIdxs: []int{1, 3},
	}
	dj, _ = dj.Update(jotKeySpecial(tea.KeyDown))
	if dj.cursor != 1 {
		t.Error("down at end should not change cursor")
	}
}

// ---------------------------------------------------------------------------
// Update — link completion
// ---------------------------------------------------------------------------

func TestUpdateLinkCompletion_EscExits(t *testing.T) {
	dj := DailyJot{OverlayBase: OverlayBase{active: true}, mode: jotInput, linkMode: true}
	dj, _ = dj.Update(jotKeySpecial(tea.KeyEsc))
	if dj.linkMode {
		t.Error("esc should exit link mode")
	}
}

func TestUpdateLinkCompletion_TabSelects(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true},
		mode:        jotInput,
		linkMode:    true,
		linkMatches: []string{"Project Notes", "Personal"},
		linkCursor:  0,
		inputRunes:  []rune("see [["),
		inputCursor: 6,
	}
	dj, _ = dj.Update(jotKeySpecial(tea.KeyTab))
	if dj.linkMode {
		t.Error("tab should exit link mode")
	}
	result := string(dj.inputRunes)
	if result != "see [[Project Notes]]" {
		t.Errorf("expected 'see [[Project Notes]]', got %q", result)
	}
}

func TestUpdateLinkCompletion_DownUp(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true},
		mode:        jotInput,
		linkMode:    true,
		linkMatches: []string{"A", "B", "C"},
		linkCursor:  0,
	}

	dj, _ = dj.Update(jotKeySpecial(tea.KeyDown))
	if dj.linkCursor != 1 {
		t.Errorf("down: linkCursor = %d, want 1", dj.linkCursor)
	}

	dj, _ = dj.Update(jotKeySpecial(tea.KeyUp))
	if dj.linkCursor != 0 {
		t.Errorf("up: linkCursor = %d, want 0", dj.linkCursor)
	}
}

func TestUpdateLinkCompletion_BackspaceExitsOnEmptyQuery(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true},
		mode:        jotInput,
		linkMode:    true,
		linkQuery:   "",
		linkMatches: []string{"A"},
		inputRunes:  []rune("[["),
		inputCursor: 2,
	}
	dj, _ = dj.Update(jotKeySpecial(tea.KeyBackspace))
	if dj.linkMode {
		t.Error("backspace with empty query should exit link mode")
	}
}

func TestUpdateLinkCompletion_BackspaceShortensQuery(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true},
		mode:        jotInput,
		linkMode:    true,
		linkQuery:   "pro",
		linkMatches: []string{"Project"},
		linkCursor:  0,
		noteNames:   []string{"Project.md", "Personal.md"},
		inputRunes:  []rune("[[pro"),
		inputCursor: 5,
	}
	dj, _ = dj.Update(jotKeySpecial(tea.KeyBackspace))
	if dj.linkQuery != "pr" {
		t.Errorf("linkQuery = %q, want 'pr'", dj.linkQuery)
	}
	if !dj.linkMode {
		t.Error("should still be in link mode")
	}
}

// ---------------------------------------------------------------------------
// Update — non-active passthrough
// ---------------------------------------------------------------------------

func TestUpdate_InactiveNoop(t *testing.T) {
	dj := DailyJot{}
	dj, _ = dj.Update(jotKeySpecial(tea.KeyEnter))
	if dj.active {
		t.Error("inactive jot should ignore messages")
	}
}

func TestUpdate_NonKeyMsgNoop(t *testing.T) {
	dj := DailyJot{OverlayBase: OverlayBase{active: true}, mode: jotInput}
	dj, _ = dj.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	// Should not panic or change state
	if dj.mode != jotInput {
		t.Error("non-key msg should not change mode")
	}
}

// ---------------------------------------------------------------------------
// Open / Close / SetSize
// ---------------------------------------------------------------------------

func TestDailyJot_OpenClose(t *testing.T) {
	tmp, dj := setupTempVault(t)
	dj.SetSize(100, 60)
	dj.Open(tmp, "Jots", []string{"note1.md", "note2.md"}, 14)

	if !dj.active {
		t.Error("Open should activate")
	}
	if dj.mode != jotInput {
		t.Error("Open should set input mode")
	}
	if len(dj.noteNames) != 2 {
		t.Errorf("expected 2 noteNames, got %d", len(dj.noteNames))
	}
	if dj.width != 100 || dj.height != 60 {
		t.Errorf("size = %dx%d, want 100x60", dj.width, dj.height)
	}

	// Today should always be present
	if len(dj.days) == 0 {
		t.Fatal("expected at least today in days")
	}

	dj.Close()
	if dj.active {
		t.Error("Close should deactivate")
	}
}

// ---------------------------------------------------------------------------
// filterNoteNames
// ---------------------------------------------------------------------------

func TestFilterNoteNames(t *testing.T) {
	dj := DailyJot{
		noteNames: []string{
			"Notes/Project Alpha.md",
			"Notes/Project Beta.md",
			"Notes/Personal.md",
			"Notes/Cooking.md",
		},
	}

	// Empty query returns all (up to 10)
	matches := dj.filterNoteNames("")
	if len(matches) != 4 {
		t.Errorf("empty query: got %d matches, want 4", len(matches))
	}

	// Filtered
	matches = dj.filterNoteNames("project")
	if len(matches) != 2 {
		t.Errorf("'project' query: got %d matches, want 2", len(matches))
	}

	// Case insensitive
	matches = dj.filterNoteNames("PERSONAL")
	if len(matches) != 1 {
		t.Errorf("'PERSONAL' query: got %d matches, want 1", len(matches))
	}

	// No match
	matches = dj.filterNoteNames("zzz")
	if len(matches) != 0 {
		t.Errorf("'zzz' query: got %d matches, want 0", len(matches))
	}
}

func TestFilterNoteNames_Limit(t *testing.T) {
	dj := DailyJot{}
	for i := 0; i < 20; i++ {
		dj.noteNames = append(dj.noteNames, "note.md")
	}
	matches := dj.filterNoteNames("")
	if len(matches) != 10 {
		t.Errorf("should cap at 10, got %d", len(matches))
	}
}

// ---------------------------------------------------------------------------
// maxVisibleEntries
// ---------------------------------------------------------------------------

func TestMaxVisibleEntries(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{height: 60},
		days: []jotDay{
			{Entries: []jotEntry{{}, {}, {}}},
			{Entries: []jotEntry{{}, {}}},
		},
	}
	mv := dj.maxVisibleEntries()
	if mv < 3 {
		t.Errorf("maxVisibleEntries = %d, expected >= 3", mv)
	}
}

func TestMaxVisibleEntries_SmallHeight(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{height: 10},
		days:        []jotDay{{Entries: []jotEntry{{}}}},
	}
	mv := dj.maxVisibleEntries()
	if mv < 3 {
		t.Errorf("maxVisibleEntries = %d, minimum should be 3", mv)
	}
}

func TestMaxVisibleEntries_ZeroHeight(t *testing.T) {
	dj := DailyJot{}
	mv := dj.maxVisibleEntries()
	if mv < 3 {
		t.Errorf("maxVisibleEntries = %d, minimum should be 3", mv)
	}
}

// ---------------------------------------------------------------------------
// adjustScroll
// ---------------------------------------------------------------------------

func TestDailyJot_AdjustScroll(t *testing.T) {
	dj := &DailyJot{
		OverlayBase: OverlayBase{height: 60},
		scroll:      0,
		cursor:      0,
		days:        []jotDay{{Entries: make([]jotEntry, 30)}},
	}

	// Cursor way past scroll window
	dj.cursor = 25
	dj.adjustScroll()
	if dj.scroll <= 0 {
		t.Error("scroll should have increased")
	}

	// Cursor before scroll
	dj.scroll = 10
	dj.cursor = 5
	dj.adjustScroll()
	if dj.scroll != 5 {
		t.Errorf("scroll = %d, want 5", dj.scroll)
	}
}

// ---------------------------------------------------------------------------
// renderStyledText
// ---------------------------------------------------------------------------

func TestRenderStyledText_Plain(t *testing.T) {
	result := renderStyledText("just plain text", false)
	if result == "" {
		t.Error("should not return empty")
	}
}

func TestRenderStyledText_WithLinks(t *testing.T) {
	result := renderStyledText("see [[My Note]] for details", false)
	if !strings.Contains(result, "My Note") {
		t.Error("should contain link text")
	}
}

func TestRenderStyledText_WithTags(t *testing.T) {
	result := renderStyledText("tagged #project and #work", false)
	if !strings.Contains(result, "project") {
		t.Error("should contain tag text")
	}
}

func TestRenderStyledText_TagInsideLink(t *testing.T) {
	// Tag inside wikilink should not be double-styled
	result := renderStyledText("see [[#heading]] here", false)
	if result == "" {
		t.Error("should not return empty")
	}
}

// ---------------------------------------------------------------------------
// renderInputLine
// ---------------------------------------------------------------------------

func TestRenderInputLine_Empty(t *testing.T) {
	result := renderInputLine(nil, 0, 40)
	if result == "" {
		t.Error("should render cursor for empty input")
	}
}

func TestRenderInputLine_ZeroWidth(t *testing.T) {
	result := renderInputLine([]rune("hello"), 0, 0)
	if result != "" {
		t.Errorf("zero width should return empty, got %q", result)
	}
}

func TestRenderInputLine_NegativeWidth(t *testing.T) {
	result := renderInputLine([]rune("hello"), 0, -5)
	if result != "" {
		t.Errorf("negative width should return empty, got %q", result)
	}
}

func TestRenderInputLine_CursorAtEnd(t *testing.T) {
	result := renderInputLine([]rune("hello"), 5, 40)
	if result == "" {
		t.Error("should render text with cursor at end")
	}
}

// ---------------------------------------------------------------------------
// View — smoke tests (don't crash)
// ---------------------------------------------------------------------------

func TestView_EmptyState(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true, width: 100, height: 40},
		mode:        jotInput,
	}
	v := dj.View()
	if v == "" {
		t.Error("View should not be empty")
	}
	if !strings.Contains(v, "Daily Jot") {
		t.Error("View should contain title")
	}
}

func TestView_WithEntries(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true, width: 100, height: 40},
		mode:        jotBrowse,
		cursor:      0,
		days: []jotDay{
			{
				Date:  "2026-03-19",
				Label: "Today — March 19, 2026",
				Entries: []jotEntry{
					{Time: "09:00", Text: "test entry"},
					{Time: "10:00", Text: "[ ] task"},
					{Time: "11:00", Text: "[x] done"},
				},
			},
		},
	}
	v := dj.View()
	if !strings.Contains(v, "Today") {
		t.Error("View should contain day header")
	}
}

func TestView_FilterMode(t *testing.T) {
	dj := DailyJot{
		OverlayBase:  OverlayBase{active: true, width: 100, height: 40},
		mode:         jotFilter,
		filterQuery:  "test",
		filterRunes:  []rune("test"),
		filteredIdxs: []int{0},
		days: []jotDay{{
			Date:    "2026-03-19",
			Label:   "Today",
			Entries: []jotEntry{{Time: "09:00", Text: "test entry"}},
		}},
	}
	v := dj.View()
	if !strings.Contains(v, "match") {
		t.Error("filter view should show match count")
	}
}

func TestView_EditMode(t *testing.T) {
	dj := DailyJot{
		OverlayBase:  OverlayBase{active: true, width: 100, height: 40},
		mode:         jotEdit,
		cursor:       0,
		editDayIdx:   0,
		editEntryIdx: 0,
		editRunes:    []rune("editing"),
		editCursor:   7,
		days: []jotDay{{
			Date:    "2026-03-19",
			Label:   "Today",
			Entries: []jotEntry{{Time: "09:00", Text: "original"}},
		}},
	}
	v := dj.View()
	if !strings.Contains(v, "save") {
		t.Error("edit view should show save hint")
	}
}

func TestView_VerySmallTerminal(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true, width: 20, height: 10},
		mode:        jotInput,
	}
	v := dj.View()
	if v == "" {
		t.Error("should still render on small terminal")
	}
}

func TestView_WithStatusMsg(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true, width: 100, height: 40},
		mode:        jotBrowse,
		statusMsg:   "Press d again to delete",
		days: []jotDay{{
			Date:    "2026-03-19",
			Label:   "Today",
			Entries: []jotEntry{{Time: "09:00", Text: "a"}},
		}},
	}
	v := dj.View()
	if !strings.Contains(v, "Press d again") {
		t.Error("should display status message")
	}
}

func TestView_LinkCompletionPopup(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true, width: 100, height: 40},
		mode:        jotInput,
		linkMode:    true,
		linkMatches: []string{"Note A", "Note B"},
		linkCursor:  0,
	}
	v := dj.View()
	if !strings.Contains(v, "Note A") {
		t.Error("should render link popup")
	}
}

func TestView_CarryOverNotice(t *testing.T) {
	dj := DailyJot{
		OverlayBase:    OverlayBase{active: true, width: 100, height: 40},
		mode:           jotInput,
		carryOverCount: 3,
	}
	v := dj.View()
	if !strings.Contains(v, "3 incomplete") {
		t.Error("should show carry-over notice")
	}
}

func TestView_ScrollIndicator(t *testing.T) {
	entries := make([]jotEntry, 50)
	for i := range entries {
		entries[i] = jotEntry{Time: "09:00", Text: "entry"}
	}
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true, width: 100, height: 40},
		mode:        jotBrowse,
		scroll:      5,
		days:        []jotDay{{Date: "2026-03-19", Label: "Today", Entries: entries}},
	}
	v := dj.View()
	if !strings.Contains(v, "of 50") {
		t.Error("should show scroll indicator for long lists")
	}
}

// ---------------------------------------------------------------------------
// Link popup scrolling — cursor past maxShow should still be visible
// ---------------------------------------------------------------------------

func TestRenderLinkPopup_ScrollsWithCursor(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true, width: 100, height: 40},
		mode:        jotInput,
		linkMode:    true,
		linkMatches: []string{
			"Note A", "Note B", "Note C", "Note D",
			"Note E", "Note F", "Note G", "Note H",
		},
		linkCursor: 7, // past the maxShow=6 window
	}
	v := dj.View()
	// Note H (index 7) should be visible since cursor is there
	if !strings.Contains(v, "Note H") {
		t.Error("link popup should scroll to show cursor at index 7")
	}
}

func TestRenderLinkPopup_CursorInMiddle(t *testing.T) {
	dj := DailyJot{
		linkMatches: []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J"},
		linkCursor:  3, // within first 6, should show A-F
	}
	popup := dj.renderLinkPopup(80)
	if !strings.Contains(popup, "A") {
		t.Error("should show first items when cursor is within window")
	}
	if !strings.Contains(popup, "D") {
		t.Error("should show cursor item D")
	}
}

func TestRenderLinkPopup_CursorAtEnd(t *testing.T) {
	dj := DailyJot{
		linkMatches: []string{"A", "B", "C", "D", "E", "F", "G", "H"},
		linkCursor:  7, // at end
	}
	popup := dj.renderLinkPopup(80)
	if !strings.Contains(popup, "H") {
		t.Error("should show last item H when cursor is at end")
	}
	// "more" indicator should not show since we're at the end
	if strings.Contains(popup, "more") {
		t.Error("should not show 'more' when displaying last items")
	}
}

// ---------------------------------------------------------------------------
// Filter mode scrolling
// ---------------------------------------------------------------------------

func TestUpdateFilter_ScrollsWithCursor(t *testing.T) {
	idxs := make([]int, 20)
	for i := range idxs {
		idxs[i] = i
	}
	dj := DailyJot{
		OverlayBase:  OverlayBase{active: true, height: 30}, // maxVisible = 30/2 - 8 = 7
		mode:         jotFilter,
		cursor:       0,
		scroll:       0,
		filteredIdxs: idxs,
	}

	// Navigate down past visible area
	for i := 0; i < 10; i++ {
		dj, _ = dj.Update(jotKeySpecial(tea.KeyDown))
	}
	if dj.cursor != 10 {
		t.Errorf("cursor = %d, want 10", dj.cursor)
	}
	if dj.scroll <= 0 {
		t.Error("scroll should have advanced to keep cursor visible")
	}
	// cursor should be within [scroll, scroll+maxVisible)
	maxVisible := dj.height/2 - 8
	if maxVisible < 6 {
		maxVisible = 6
	}
	if dj.cursor < dj.scroll || dj.cursor >= dj.scroll+maxVisible {
		t.Errorf("cursor %d outside scroll window [%d, %d)", dj.cursor, dj.scroll, dj.scroll+maxVisible)
	}
}

func TestUpdateFilter_BackspaceResetsScroll(t *testing.T) {
	dj := DailyJot{
		OverlayBase:  OverlayBase{active: true},
		mode:         jotFilter,
		scroll:       5,
		cursor:       8,
		filterRunes:  []rune("test"),
		filterCursor: 4,
		filteredIdxs: []int{1, 2, 3, 4, 5, 6, 7, 8, 9},
		days:         []jotDay{{Entries: make([]jotEntry, 20)}},
	}
	dj, _ = dj.Update(jotKeySpecial(tea.KeyBackspace))
	if dj.scroll != 0 {
		t.Errorf("backspace should reset scroll to 0, got %d", dj.scroll)
	}
	if dj.cursor != 0 {
		t.Errorf("backspace should reset cursor to 0, got %d", dj.cursor)
	}
}

func TestView_FilterScrollIndicator(t *testing.T) {
	entries := make([]jotEntry, 30)
	for i := range entries {
		entries[i] = jotEntry{Time: "09:00", Text: "match"}
	}
	idxs := make([]int, 30)
	for i := range idxs {
		idxs[i] = i
	}
	dj := DailyJot{
		OverlayBase:  OverlayBase{active: true, width: 100, height: 30},
		mode:         jotFilter,
		filterQuery:  "match",
		filterRunes:  []rune("match"),
		filteredIdxs: idxs,
		scroll:       5,
		days:         []jotDay{{Date: "2026-03-19", Label: "Today", Entries: entries}},
	}
	v := dj.View()
	if !strings.Contains(v, "of 30") {
		t.Error("filter view should show scroll indicator for many results")
	}
}

// ---------------------------------------------------------------------------
// Dead code removal verification — backspace in input mode
// ---------------------------------------------------------------------------

func TestUpdateInput_BackspaceDoesNotCheckLinkMode(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true},
		mode:        jotInput,
		inputRunes:  []rune("abc"),
		inputCursor: 3,
	}
	dj, _ = dj.Update(jotKeySpecial(tea.KeyBackspace))
	if string(dj.inputRunes) != "ab" {
		t.Errorf("expected 'ab', got %q", string(dj.inputRunes))
	}
}

// ---------------------------------------------------------------------------
// Config — Open with configurable params
// ---------------------------------------------------------------------------

func TestOpen_CustomConfig(t *testing.T) {
	tmp, dj := setupTempVault(t)
	dj.SetSize(100, 60)
	dj.Open(tmp, "MyJots", []string{}, 7)
	if dj.jotsFolder != "MyJots" {
		t.Errorf("jotsFolder = %q, want 'MyJots'", dj.jotsFolder)
	}
	if dj.daysBack != 7 {
		t.Errorf("daysBack = %d, want 7", dj.daysBack)
	}
}

func TestOpen_EmptyFolderDefault(t *testing.T) {
	tmp, dj := setupTempVault(t)
	dj.Open(tmp, "", []string{}, 0)
	if dj.jotsFolder != "Jots" {
		t.Errorf("empty folder should default to 'Jots', got %q", dj.jotsFolder)
	}
	if dj.daysBack != 14 {
		t.Errorf("zero daysBack should default to 14, got %d", dj.daysBack)
	}
}

// ---------------------------------------------------------------------------
// Tag Aggregation
// ---------------------------------------------------------------------------

func TestBuildTagIndex(t *testing.T) {
	dj := &DailyJot{
		days: []jotDay{
			{Entries: []jotEntry{
				{Time: "09:00", Text: "idea about #project"},
				{Time: "10:00", Text: "#project meeting #work"},
				{Time: "11:00", Text: "see [[#heading]] in note"},
			}},
			{Entries: []jotEntry{
				{Time: "12:00", Text: "#project kickoff"},
				{Time: "13:00", Text: "#work review"},
			}},
		},
	}
	dj.buildTagIndex()

	if len(dj.tags) == 0 {
		t.Fatal("expected tags to be found")
	}
	// #project appears 3 times, #work appears 2 times
	// #heading inside [[ ]] should be excluded
	tagMap := make(map[string]int)
	for _, ti := range dj.tags {
		tagMap[ti.Tag] = ti.Count
	}
	if tagMap["#project"] != 3 {
		t.Errorf("#project count = %d, want 3", tagMap["#project"])
	}
	if tagMap["#work"] != 2 {
		t.Errorf("#work count = %d, want 2", tagMap["#work"])
	}
	if _, ok := tagMap["#heading"]; ok {
		t.Error("#heading inside wikilink should be excluded")
	}
	// First tag should be highest count
	if dj.tags[0].Tag != "#project" {
		t.Errorf("first tag = %q, want #project (highest count)", dj.tags[0].Tag)
	}
}

func TestBuildTagIndex_NoTags(t *testing.T) {
	dj := &DailyJot{
		days: []jotDay{
			{Entries: []jotEntry{{Time: "09:00", Text: "no tags here"}}},
		},
	}
	dj.buildTagIndex()
	if len(dj.tags) != 0 {
		t.Errorf("expected 0 tags, got %d", len(dj.tags))
	}
}

func TestBuildTagFilterIndex(t *testing.T) {
	dj := &DailyJot{
		days: []jotDay{
			{Entries: []jotEntry{
				{Time: "09:00", Text: "no tag"},
				{Time: "10:00", Text: "#work stuff"},
				{Time: "11:00", Text: "more #work"},
			}},
		},
	}
	dj.buildTagFilterIndex("#work")
	if len(dj.tagFiltered) != 2 {
		t.Errorf("expected 2 filtered entries, got %d", len(dj.tagFiltered))
	}
}

func TestUpdateBrowse_HashToTags(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true},
		mode:        jotBrowse,
		cursor:      0,
		days: []jotDay{{
			Entries: []jotEntry{
				{Time: "09:00", Text: "tagged #project"},
			},
		}},
	}
	dj, _ = dj.Update(jotKeyMsg("#"))
	if dj.mode != jotTags {
		t.Errorf("expected jotTags mode, got %d", dj.mode)
	}
	if len(dj.tags) == 0 {
		t.Error("expected tags to be populated")
	}
}

func TestUpdateBrowse_HashNoTags(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true},
		mode:        jotBrowse,
		days:        []jotDay{{Entries: []jotEntry{{Time: "09:00", Text: "no tags"}}}},
	}
	dj, _ = dj.Update(jotKeyMsg("#"))
	if dj.mode != jotBrowse {
		t.Error("# with no tags should stay in browse mode")
	}
	if dj.statusMsg == "" {
		t.Error("should show 'No tags found' status")
	}
}

func TestUpdateTags_Navigation(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true, width: 100, height: 60},
		mode:        jotTags,
		tagCursor:   0,
		tags:        []tagInfo{{Tag: "#a", Count: 5}, {Tag: "#b", Count: 3}, {Tag: "#c", Count: 1}},
	}

	dj, _ = dj.Update(jotKeyMsg("j"))
	if dj.tagCursor != 1 {
		t.Errorf("j: tagCursor = %d, want 1", dj.tagCursor)
	}

	dj, _ = dj.Update(jotKeyMsg("k"))
	if dj.tagCursor != 0 {
		t.Errorf("k: tagCursor = %d, want 0", dj.tagCursor)
	}
}

func TestUpdateTags_EnterSelectsTag(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true},
		mode:        jotTags,
		tagCursor:   0,
		tags:        []tagInfo{{Tag: "#project", Count: 3}},
		days: []jotDay{{
			Entries: []jotEntry{
				{Time: "09:00", Text: "#project idea"},
				{Time: "10:00", Text: "other"},
			},
		}},
	}
	dj, _ = dj.Update(jotKeySpecial(tea.KeyEnter))
	if dj.selectedTag != "#project" {
		t.Errorf("selectedTag = %q, want '#project'", dj.selectedTag)
	}
	if len(dj.tagFiltered) == 0 {
		t.Error("tagFiltered should be populated")
	}
}

func TestUpdateTags_EscReturnsToBrowse(t *testing.T) {
	dj := DailyJot{OverlayBase: OverlayBase{active: true}, mode: jotTags}
	dj, _ = dj.Update(jotKeySpecial(tea.KeyEsc))
	if dj.mode != jotBrowse {
		t.Error("esc should return to browse")
	}
}

func TestUpdateTagFiltered_EscReturnsToTagList(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true},
		mode:        jotTags,
		selectedTag: "#project",
		tagFiltered: []int{0, 1},
	}
	dj, _ = dj.Update(jotKeySpecial(tea.KeyEsc))
	if dj.selectedTag != "" {
		t.Error("esc should clear selectedTag")
	}
}

func TestUpdateTagFiltered_EnterJumps(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true, width: 100, height: 60},
		mode:        jotTags,
		selectedTag: "#work",
		tagFiltered: []int{3, 7},
		cursor:      1,
		days: []jotDay{
			{Entries: make([]jotEntry, 5)},
			{Entries: make([]jotEntry, 5)},
		},
	}
	dj, _ = dj.Update(jotKeySpecial(tea.KeyEnter))
	if dj.mode != jotBrowse {
		t.Error("enter should switch to browse mode")
	}
	if dj.cursor != 7 {
		t.Errorf("cursor = %d, want 7", dj.cursor)
	}
	if dj.selectedTag != "" {
		t.Error("selectedTag should be cleared")
	}
}

func TestView_TagMode(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true, width: 100, height: 40},
		mode:        jotTags,
		tagCursor:   0,
		tags: []tagInfo{
			{Tag: "#project", Count: 5},
			{Tag: "#work", Count: 3},
		},
	}
	v := dj.View()
	if !strings.Contains(v, "#project") {
		t.Error("tag view should show #project")
	}
	if !strings.Contains(v, "(5)") {
		t.Error("tag view should show count")
	}
}

func TestView_TagFilteredMode(t *testing.T) {
	dj := DailyJot{
		OverlayBase: OverlayBase{active: true, width: 100, height: 40},
		mode:        jotTags,
		selectedTag: "#work",
		tagFiltered: []int{0},
		cursor:      0,
		days: []jotDay{{
			Date:    "2026-03-19",
			Label:   "Today",
			Entries: []jotEntry{{Time: "09:00", Text: "#work meeting"}},
		}},
	}
	v := dj.View()
	if !strings.Contains(v, "#work") {
		t.Error("tag filtered view should show selected tag")
	}
}

// ---------------------------------------------------------------------------
// Promotion
// ---------------------------------------------------------------------------

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Buy milk", "buy-milk"},
		{"Hello World!", "hello-world"},
		{"  spaces  ", "spaces"},
		{"#tag stuff", "tag-stuff"},
		{"[[link]] text", "link-text"},
		{"[ ] task name", "task-name"},
		{"[x] done task", "done-task"},
		{"", ""},
		{"a", "a"},
	}
	for _, tt := range tests {
		got := slugify(tt.input)
		if got != tt.want {
			t.Errorf("slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSlugify_LongInput(t *testing.T) {
	long := strings.Repeat("word ", 20)
	got := slugify(long)
	if len(got) > 55 { // 50 + possible trailing chars
		t.Errorf("slugify should truncate, got len=%d", len(got))
	}
}

func TestPromoteJot(t *testing.T) {
	tmp, dj := setupTempVault(t)
	dj.days = []jotDay{{
		Date: "2026-03-19",
		Entries: []jotEntry{
			{Time: "09:00", Text: "promote this idea #project"},
		},
	}}
	dj.promoteJot(0, 0)

	if dj.promotedNote == "" {
		t.Fatal("promotedNote should be set")
	}
	if dj.statusMsg == "" || !strings.Contains(dj.statusMsg, "Promoted") {
		t.Errorf("statusMsg = %q, expected 'Promoted' message", dj.statusMsg)
	}

	// Check the note file was created
	notePath := filepath.Join(tmp, dj.promotedNote)
	content, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("promoted note not found: %v", err)
	}
	if !strings.Contains(string(content), "promote this idea") {
		t.Error("promoted note should contain original text")
	}
	if !strings.Contains(string(content), "tags: [project]") {
		t.Error("promoted note should extract tags")
	}

	// Check backlink was added to original jot
	if !strings.Contains(dj.days[0].Entries[0].Text, "→ [[") {
		t.Error("original jot should have backlink")
	}
}

func TestPromoteJot_BoundsCheck(t *testing.T) {
	_, dj := setupTempVault(t)
	dj.days = []jotDay{{Date: "2026-03-19", Entries: []jotEntry{{Time: "09:00", Text: "a"}}}}

	// Should not panic
	dj.promoteJot(-1, 0)
	dj.promoteJot(0, -1)
	dj.promoteJot(5, 0)
}

func TestPromoteJot_UniqueFilename(t *testing.T) {
	tmp, dj := setupTempVault(t)
	dj.days = []jotDay{{
		Date: "2026-03-19",
		Entries: []jotEntry{
			{Time: "09:00", Text: "same title"},
			{Time: "10:00", Text: "same title"},
		},
	}}

	dj.promoteJot(0, 0)
	first := dj.promotedNote
	dj.promoteJot(0, 1)
	second := dj.promotedNote

	if first == second {
		t.Errorf("duplicate promotion should create unique filenames, both = %q", first)
	}

	// Both files should exist
	if _, err := os.Stat(filepath.Join(tmp, first)); err != nil {
		t.Errorf("first promoted note not found: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, second)); err != nil {
		t.Errorf("second promoted note not found: %v", err)
	}
}

func TestUpdateBrowse_PromoteKey(t *testing.T) {
	_, djp := setupTempVault(t)
	djp.active = true
	djp.mode = jotBrowse
	djp.cursor = 0
	djp.days = []jotDay{{
		Date:    "2026-03-19",
		Entries: []jotEntry{{Time: "09:00", Text: "promote me"}},
	}}
	dj := *djp

	dj, _ = dj.Update(jotKeyMsg("p"))
	if dj.promotedNote == "" {
		t.Error("p should promote the jot")
	}
	if !strings.Contains(dj.statusMsg, "Promoted") {
		t.Error("should show promotion status")
	}
}

func TestGetPromotedNote(t *testing.T) {
	dj := &DailyJot{promotedNote: "test-note.md"}
	got := dj.GetPromotedNote()
	if got != "test-note.md" {
		t.Errorf("got %q, want 'test-note.md'", got)
	}
	// Should be cleared after getting
	if dj.promotedNote != "" {
		t.Error("promotedNote should be cleared after GetPromotedNote")
	}
	// Second call returns empty
	if dj.GetPromotedNote() != "" {
		t.Error("second GetPromotedNote should return empty")
	}
}
