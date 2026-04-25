package tui

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// jotEntry represents a single bullet in a daily jot file.
// Text starting with "[ ] " or "[x] " is a task.
type jotEntry struct {
	Time string // "15:04"
	Text string // raw text with [[links]], #tags, and optional [ ]/[x] prefix
}

// isTask returns true if the entry is a task (starts with [ ] or [x] ).
func (e jotEntry) isTask() bool {
	return strings.HasPrefix(e.Text, "[ ] ") || strings.HasPrefix(e.Text, "[x] ")
}

// isDone returns true if the entry is a completed task.
func (e jotEntry) isDone() bool {
	return strings.HasPrefix(e.Text, "[x] ")
}

// taskText returns the text after the checkbox prefix, or the full text if not a task.
func (e jotEntry) taskText() string {
	if strings.HasPrefix(e.Text, "[ ] ") || strings.HasPrefix(e.Text, "[x] ") {
		return e.Text[4:]
	}
	return e.Text
}

type jotDay struct {
	Date    string     // "2006-01-02"
	Label   string     // "Today", "Yesterday", or "March 16, 2026"
	Entries []jotEntry
}

type jotMode int

const (
	jotInput  jotMode = iota // typing new jot (default)
	jotBrowse                // scrolling/selecting entries
	jotEdit                  // editing existing entry
	jotFilter                // filtering entries by search query
	jotTags                  // browsing tags
)

type tagInfo struct {
	Tag   string
	Count int
}

type DailyJot struct {
	OverlayBase
	vaultRoot  string
	jotsFolder string
	daysBack   int
	mode       jotMode

	days []jotDay // index 0 = today

	// Text cursor — shared by input and edit buffers
	inputRunes  []rune
	inputCursor int // cursor position within inputRunes
	editRunes   []rune
	editCursor  int
	editDayIdx   int
	editEntryIdx int

	cursor int // flat index across all entries (or filtered entries)
	scroll int // view scroll offset

	// Filter
	filterQuery   string
	filterRunes   []rune
	filterCursor  int
	filteredIdxs  []int // flat indices of matching entries

	// Delete confirmation
	pendingDelete bool

	// Carry-over
	carryOverCount int // number of tasks carried over on open

	// Tag aggregation
	tags         []tagInfo // sorted by count descending
	tagCursor    int
	tagScroll    int
	tagFiltered  []int  // flat indices matching selected tag
	selectedTag  string // active tag filter (empty = showing tag list)

	// Promotion
	promotedNote string // set after promotion, cleared by getter

	noteNames   []string // for [[link]] completion
	linkMode    bool
	linkQuery   string
	linkMatches []string
	linkCursor  int

	statusMsg string
}

func NewDailyJot() DailyJot {
	return DailyJot{}
}

func (dj *DailyJot) Open(vaultRoot, jotsFolder string, noteNames []string, daysBack int) {
	dj.Activate()
	dj.vaultRoot = vaultRoot
	dj.jotsFolder = jotsFolder
	if jotsFolder == "" {
		dj.jotsFolder = "Jots"
	}
	dj.daysBack = daysBack
	if daysBack <= 0 {
		dj.daysBack = 14
	}
	dj.noteNames = noteNames
	dj.mode = jotInput
	dj.inputRunes = nil
	dj.inputCursor = 0
	dj.editRunes = nil
	dj.editCursor = 0
	dj.cursor = 0
	dj.scroll = 0
	dj.statusMsg = ""
	dj.linkMode = false
	dj.linkQuery = ""
	dj.linkMatches = nil
	dj.linkCursor = 0
	dj.pendingDelete = false
	dj.filterQuery = ""
	dj.filterRunes = nil
	dj.filterCursor = 0
	dj.filteredIdxs = nil
	dj.carryOverCount = 0
	dj.tags = nil
	dj.tagCursor = 0
	dj.tagScroll = 0
	dj.tagFiltered = nil
	dj.selectedTag = ""
	dj.promotedNote = ""
	dj.loadDays(dj.daysBack)
	dj.carryOverTasks()
}

// GetPromotedNote returns and clears the path of a newly promoted note.
func (dj *DailyJot) GetPromotedNote() string {
	note := dj.promotedNote
	dj.promotedNote = ""
	return note
}

// --- Text cursor helpers ---

func (dj *DailyJot) activeRunes() []rune {
	switch dj.mode {
	case jotEdit:
		return dj.editRunes
	case jotFilter:
		return dj.filterRunes
	default:
		return dj.inputRunes
	}
}

func (dj *DailyJot) activeCursor() int {
	switch dj.mode {
	case jotEdit:
		return dj.editCursor
	case jotFilter:
		return dj.filterCursor
	default:
		return dj.inputCursor
	}
}

func (dj *DailyJot) setActiveRunes(r []rune) {
	switch dj.mode {
	case jotEdit:
		dj.editRunes = r
	case jotFilter:
		dj.filterRunes = r
		dj.filterQuery = string(r)
		dj.buildFilterIndex()
	default:
		dj.inputRunes = r
	}
}

func (dj *DailyJot) setActiveCursor(pos int) {
	switch dj.mode {
	case jotEdit:
		dj.editCursor = pos
	case jotFilter:
		dj.filterCursor = pos
	default:
		dj.inputCursor = pos
	}
}

func (dj *DailyJot) insertChar(ch rune) {
	r := dj.activeRunes()
	c := dj.activeCursor()
	newR := make([]rune, len(r)+1)
	copy(newR, r[:c])
	newR[c] = ch
	copy(newR[c+1:], r[c:])
	dj.setActiveRunes(newR)
	dj.setActiveCursor(c + 1)
}

func (dj *DailyJot) deleteCharBack() {
	r := dj.activeRunes()
	c := dj.activeCursor()
	if c > 0 {
		newR := make([]rune, len(r)-1)
		copy(newR, r[:c-1])
		copy(newR[c-1:], r[c:])
		dj.setActiveRunes(newR)
		dj.setActiveCursor(c - 1)
	}
}

func (dj *DailyJot) activeString() string {
	return string(dj.activeRunes())
}

// --- File I/O ---

const jotSeparator = " — "

func (dj *DailyJot) loadDays(n int) {
	dj.days = nil
	now := time.Now()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")

	for i := 0; i < n; i++ {
		date := now.AddDate(0, 0, -i)
		dateStr := date.Format("2006-01-02")
		day := dj.loadDay(dateStr)

		switch dateStr {
		case today:
			day.Label = "Today — " + date.Format("January 2, 2006")
		case yesterday:
			day.Label = "Yesterday — " + date.Format("January 2, 2006")
		default:
			day.Label = date.Format("January 2, 2006")
		}

		if len(day.Entries) > 0 || dateStr == today {
			dj.days = append(dj.days, day)
		}
	}
}

func (dj DailyJot) loadDay(date string) jotDay {
	day := jotDay{Date: date}
	filePath := filepath.Join(dj.vaultRoot, dj.jotsFolder, date+".md")

	f, err := os.Open(filePath)
	if err != nil {
		return day
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// State machine: 0=before frontmatter, 1=inside frontmatter, 2=past frontmatter
	fmState := 0
	for scanner.Scan() {
		line := scanner.Text()

		if line == "---" {
			if fmState < 2 {
				fmState++
				continue
			}
			// Past frontmatter: "---" is a horizontal rule, not frontmatter
		}
		if fmState == 1 {
			continue
		}

		if !strings.HasPrefix(line, "- ") {
			continue
		}
		rest := line[2:]
		if len(rest) < 5 || rest[2] != ':' {
			continue
		}
		timeStr := rest[:5]
		if timeStr[0] < '0' || timeStr[0] > '9' || timeStr[1] < '0' || timeStr[1] > '9' ||
			timeStr[3] < '0' || timeStr[3] > '9' || timeStr[4] < '0' || timeStr[4] > '9' {
			continue
		}
		// Validate HH (00-23) and MM (00-59)
		hh := (timeStr[0]-'0')*10 + (timeStr[1] - '0')
		mm := (timeStr[3]-'0')*10 + (timeStr[4] - '0')
		if hh > 23 || mm > 59 {
			continue
		}
		after := rest[5:]
		if !strings.HasPrefix(after, jotSeparator) {
			continue
		}
		day.Entries = append(day.Entries, jotEntry{
			Time: timeStr,
			Text: after[len(jotSeparator):],
		})
	}

	return day
}

func (dj DailyJot) saveDay(day jotDay) error {
	dir := filepath.Join(dj.vaultRoot, dj.jotsFolder)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	filePath := filepath.Join(dir, day.Date+".md")
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("date: " + day.Date + "\n")
	b.WriteString("type: jot\n")
	b.WriteString("tags: [jot]\n")
	b.WriteString("---\n")

	for _, e := range day.Entries {
		b.WriteString(fmt.Sprintf("- %s%s%s\n", e.Time, jotSeparator, e.Text))
	}

	// Atomic write: temp file + rename to prevent corruption on crash
	tmp, err := os.CreateTemp(dir, ".jot-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if _, err := tmp.WriteString(b.String()); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, filePath)
}

// --- Carry-over: bring yesterday's incomplete tasks to today ---

func (dj *DailyJot) carryOverTasks() {
	if len(dj.days) < 2 {
		return
	}
	now := time.Now()
	todayStr := now.Format("2006-01-02")

	// Ensure today exists (prepend may reallocate the slice)
	if dj.days[0].Date != todayStr {
		newDay := jotDay{
			Date:  todayStr,
			Label: "Today — " + now.Format("January 2, 2006"),
		}
		dj.days = append([]jotDay{newDay}, dj.days...)
	}

	// Use indices (not pointers) since the slice may have been reallocated above
	yesterdayIdx := -1
	for i := 1; i < len(dj.days); i++ {
		if len(dj.days[i].Entries) > 0 {
			yesterdayIdx = i
			break
		}
	}
	if yesterdayIdx < 0 {
		return
	}

	// Build set of today's task texts to avoid duplicates
	todayTexts := make(map[string]bool)
	for _, e := range dj.days[0].Entries {
		if e.isTask() {
			todayTexts[e.taskText()] = true
		}
	}

	carried := 0
	for _, e := range dj.days[yesterdayIdx].Entries {
		if e.isTask() && !e.isDone() && !todayTexts[e.taskText()] {
			dj.days[0].Entries = append(dj.days[0].Entries, jotEntry{
				Time: now.Format("15:04"),
				Text: e.Text, // preserves "[ ] " prefix
			})
			carried++
		}
	}

	if carried > 0 {
		dj.carryOverCount = carried
		_ = dj.saveDay(dj.days[0])
	}
}

// --- Mutations ---

func (dj *DailyJot) appendJot(text string) {
	now := time.Now()
	todayStr := now.Format("2006-01-02")

	if len(dj.days) == 0 || dj.days[0].Date != todayStr {
		newDay := jotDay{
			Date:  todayStr,
			Label: "Today — " + now.Format("January 2, 2006"),
		}
		dj.days = append([]jotDay{newDay}, dj.days...)
	}

	dj.days[0].Entries = append(dj.days[0].Entries, jotEntry{
		Time: now.Format("15:04"),
		Text: text,
	})

	if err := dj.saveDay(dj.days[0]); err != nil {
		dj.statusMsg = "Error: " + err.Error()
	} else {
		dj.statusMsg = ""
	}
}

func (dj *DailyJot) deleteJot(dayIdx, entryIdx int) {
	if dayIdx < 0 || dayIdx >= len(dj.days) {
		return
	}
	day := &dj.days[dayIdx]
	if entryIdx < 0 || entryIdx >= len(day.Entries) {
		return
	}
	day.Entries = append(day.Entries[:entryIdx], day.Entries[entryIdx+1:]...)
	if err := dj.saveDay(*day); err != nil {
		dj.statusMsg = "Error: " + err.Error()
	}
}

func (dj *DailyJot) updateJot(dayIdx, entryIdx int, text string) {
	if dayIdx < 0 || dayIdx >= len(dj.days) || entryIdx < 0 || entryIdx >= len(dj.days[dayIdx].Entries) {
		return
	}
	dj.days[dayIdx].Entries[entryIdx].Text = text
	if err := dj.saveDay(dj.days[dayIdx]); err != nil {
		dj.statusMsg = "Error: " + err.Error()
	}
}

// toggleTask converts a jot to a task or toggles task completion.
func (dj *DailyJot) toggleTask(dayIdx, entryIdx int) {
	if dayIdx < 0 || dayIdx >= len(dj.days) || entryIdx < 0 || entryIdx >= len(dj.days[dayIdx].Entries) {
		return
	}
	e := &dj.days[dayIdx].Entries[entryIdx]
	if e.isDone() {
		e.Text = "[ ] " + e.taskText()
	} else if e.isTask() {
		e.Text = "[x] " + e.taskText()
	} else {
		e.Text = "[ ] " + e.Text
	}
	if err := dj.saveDay(dj.days[dayIdx]); err != nil {
		dj.statusMsg = "Error: " + err.Error()
	}
}

// --- Navigation helpers ---

func (dj DailyJot) totalEntries() int {
	total := 0
	for _, d := range dj.days {
		total += len(d.Entries)
	}
	return total
}

func (dj DailyJot) flatIndex(cursor int) (dayIdx, entryIdx int, ok bool) {
	pos := 0
	for di, d := range dj.days {
		for i := len(d.Entries) - 1; i >= 0; i-- {
			if pos == cursor {
				return di, i, true
			}
			pos++
		}
	}
	return 0, 0, false
}

func (dj *DailyJot) adjustScroll() {
	maxVisible := dj.maxVisibleEntries()
	if dj.cursor < dj.scroll {
		dj.scroll = dj.cursor
	}
	if dj.cursor >= dj.scroll+maxVisible {
		dj.scroll = dj.cursor - maxVisible + 1
	}
}

// maxVisibleEntries returns the number of entry slots available, accounting for
// day headers (2 lines each) that consume space in the visible window.
func (dj DailyJot) maxVisibleEntries() int {
	rawMax := dj.height/2 - 8
	if rawMax < 6 {
		rawMax = 6
	}
	// Count how many day headers are visible in the current scroll window.
	// Each header consumes 2 lines of the maxEntryLines budget.
	headers := 0
	pos := 0
	for _, d := range dj.days {
		if len(d.Entries) == 0 {
			continue
		}
		dayEnd := pos + len(d.Entries)
		if dayEnd > dj.scroll && pos < dj.scroll+rawMax {
			headers++
		}
		pos += len(d.Entries)
	}
	adjusted := rawMax - headers*2
	if adjusted < 3 {
		adjusted = 3
	}
	return adjusted
}

// --- Filter ---

func (dj *DailyJot) adjustFilterScroll() {
	maxVisible := dj.height/2 - 8
	if maxVisible < 6 {
		maxVisible = 6
	}
	if dj.cursor < dj.scroll {
		dj.scroll = dj.cursor
	}
	if dj.cursor >= dj.scroll+maxVisible {
		dj.scroll = dj.cursor - maxVisible + 1
	}
}

func (dj *DailyJot) buildFilterIndex() {
	dj.filteredIdxs = nil
	if dj.filterQuery == "" {
		return
	}
	query := strings.ToLower(dj.filterQuery)
	pos := 0
	for _, d := range dj.days {
		for i := len(d.Entries) - 1; i >= 0; i-- {
			if strings.Contains(strings.ToLower(d.Entries[i].Text), query) {
				dj.filteredIdxs = append(dj.filteredIdxs, pos)
			}
			pos++
		}
	}
	if dj.cursor >= len(dj.filteredIdxs) {
		dj.cursor = maxInt(0, len(dj.filteredIdxs)-1)
	}
}

// --- Tag aggregation ---

func (dj *DailyJot) buildTagIndex() {
	counts := make(map[string]int)
	for _, d := range dj.days {
		for _, e := range d.Entries {
			// Use tagRe but skip tags inside wikilinks
			for _, loc := range tagRe.FindAllStringIndex(e.Text, -1) {
				// Check if this tag is inside a [[ ]]
				inside := false
				for _, wl := range wikilinkRe.FindAllStringIndex(e.Text, -1) {
					if loc[0] >= wl[0] && loc[1] <= wl[1] {
						inside = true
						break
					}
				}
				if !inside {
					tag := e.Text[loc[0]:loc[1]]
					counts[tag]++
				}
			}
		}
	}
	dj.tags = nil
	for tag, count := range counts {
		dj.tags = append(dj.tags, tagInfo{Tag: tag, Count: count})
	}
	// Sort by count descending, then alphabetically
	for i := 1; i < len(dj.tags); i++ {
		for j := i; j > 0; j-- {
			if dj.tags[j].Count > dj.tags[j-1].Count ||
				(dj.tags[j].Count == dj.tags[j-1].Count && dj.tags[j].Tag < dj.tags[j-1].Tag) {
				dj.tags[j], dj.tags[j-1] = dj.tags[j-1], dj.tags[j]
			} else {
				break
			}
		}
	}
}

func (dj *DailyJot) buildTagFilterIndex(tag string) {
	dj.tagFiltered = nil
	pos := 0
	for _, d := range dj.days {
		for i := len(d.Entries) - 1; i >= 0; i-- {
			if strings.Contains(d.Entries[i].Text, tag) {
				dj.tagFiltered = append(dj.tagFiltered, pos)
			}
			pos++
		}
	}
}

func (dj *DailyJot) adjustTagScroll() {
	maxVisible := dj.height/2 - 8
	if maxVisible < 6 {
		maxVisible = 6
	}
	if dj.tagCursor < dj.tagScroll {
		dj.tagScroll = dj.tagCursor
	}
	if dj.tagCursor >= dj.tagScroll+maxVisible {
		dj.tagScroll = dj.tagCursor - maxVisible + 1
	}
}

// --- Promotion ---

func (dj *DailyJot) promoteJot(dayIdx, entryIdx int) {
	if dayIdx < 0 || dayIdx >= len(dj.days) || entryIdx < 0 || entryIdx >= len(dj.days[dayIdx].Entries) {
		return
	}
	e := dj.days[dayIdx].Entries[entryIdx]
	title := e.taskText()
	if title == "" {
		return
	}

	slug := slugify(title)
	if slug == "" {
		slug = "promoted-jot"
	}

	// Find unique filename
	notePath := filepath.Join(dj.vaultRoot, slug+".md")
	for i := 2; fileExists(notePath); i++ {
		notePath = filepath.Join(dj.vaultRoot, fmt.Sprintf("%s-%d.md", slug, i))
	}
	relPath := filepath.Base(notePath)
	noteTitle := strings.TrimSuffix(relPath, ".md")

	// Extract tags from jot text
	var tags []string
	for _, match := range tagRe.FindAllString(e.Text, -1) {
		tags = append(tags, strings.TrimPrefix(match, "#"))
	}

	// Write the note
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("date: " + dj.days[dayIdx].Date + "\n")
	if len(tags) > 0 {
		b.WriteString("tags: [" + strings.Join(tags, ", ") + "]\n")
	}
	b.WriteString("---\n\n")
	b.WriteString("# " + title + "\n\n")
	b.WriteString("Promoted from Daily Jot (" + dj.days[dayIdx].Date + " " + e.Time + ")\n")

	// Atomic write so a partial promote leaves the original jot untouched.
	if err := atomicWriteNote(notePath, b.String()); err != nil {
		dj.statusMsg = "Error: " + err.Error()
		return
	}

	// Add backlink to the original jot
	dj.days[dayIdx].Entries[entryIdx].Text += " → [[" + noteTitle + "]]"
	if err := dj.saveDay(dj.days[dayIdx]); err != nil {
		dj.statusMsg = "Error: " + err.Error()
		return
	}

	dj.promotedNote = relPath
	dj.statusMsg = "Promoted → " + noteTitle
}

func slugify(text string) string {
	// Strip wikilinks
	text = wikilinkRe.ReplaceAllString(text, "$1")
	// Strip checkbox prefixes
	text = strings.TrimPrefix(text, "[ ] ")
	text = strings.TrimPrefix(text, "[x] ")

	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(text) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash && b.Len() > 0 {
			b.WriteRune('-')
			prevDash = true
		}
		if b.Len() >= 50 {
			break
		}
	}
	result := strings.TrimRight(b.String(), "-")
	return result
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// --- Update ---

func (dj DailyJot) Update(msg tea.Msg) (DailyJot, tea.Cmd) {
	if !dj.active {
		return dj, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Cancel pending delete on any key that isn't 'd'
		if dj.pendingDelete && msg.String() != "d" {
			dj.pendingDelete = false
			dj.statusMsg = ""
		}

		if dj.linkMode {
			return dj.updateLinkCompletion(msg)
		}

		switch dj.mode {
		case jotInput:
			return dj.updateInput(msg)
		case jotBrowse:
			return dj.updateBrowse(msg)
		case jotEdit:
			return dj.updateEdit(msg)
		case jotFilter:
			return dj.updateFilter(msg)
		case jotTags:
			return dj.updateTags(msg)
		}
	}
	return dj, nil
}

func (dj DailyJot) updateInput(msg tea.KeyMsg) (DailyJot, tea.Cmd) {
	switch msg.String() {
	case "esc":
		dj.active = false
		return dj, nil
	case "enter":
		text := strings.TrimSpace(string(dj.inputRunes))
		if text != "" {
			// "[] text" shorthand creates a task
			if strings.HasPrefix(text, "[] ") {
				text = "[ ] " + text[3:]
			}
			dj.appendJot(text)
			dj.inputRunes = nil
			dj.inputCursor = 0
		}
		return dj, nil
	case "backspace":
		dj.deleteCharBack()
		return dj, nil
	case "left":
		if dj.inputCursor > 0 {
			dj.inputCursor--
		}
		return dj, nil
	case "right":
		if dj.inputCursor < len(dj.inputRunes) {
			dj.inputCursor++
		}
		return dj, nil
	case "home", "ctrl+a":
		dj.inputCursor = 0
		return dj, nil
	case "end", "ctrl+e":
		dj.inputCursor = len(dj.inputRunes)
		return dj, nil
	case "ctrl+u":
		dj.inputRunes = dj.inputRunes[dj.inputCursor:]
		dj.inputCursor = 0
		return dj, nil
	case "ctrl+k":
		dj.inputRunes = dj.inputRunes[:dj.inputCursor]
		return dj, nil
	case "down", "tab":
		if dj.totalEntries() > 0 {
			dj.mode = jotBrowse
			dj.cursor = 0
			dj.scroll = 0
		}
		return dj, nil
	default:
		char := msg.String()
		if len(char) == 1 && char[0] >= 32 {
			dj.insertChar(rune(char[0]))
			s := string(dj.inputRunes)
			if strings.HasSuffix(s, "[[") {
				dj.linkMode = true
				dj.linkQuery = ""
				dj.linkMatches = dj.filterNoteNames("")
				dj.linkCursor = 0
			}
		}
		return dj, nil
	}
}

func (dj DailyJot) updateBrowse(msg tea.KeyMsg) (DailyJot, tea.Cmd) {
	total := dj.totalEntries()
	switch msg.String() {
	case "esc":
		dj.active = false
		return dj, nil
	case "j", "down":
		if total > 0 && dj.cursor < total-1 {
			dj.cursor++
			dj.adjustScroll()
		}
		return dj, nil
	case "k", "up":
		if dj.cursor > 0 {
			dj.cursor--
			dj.adjustScroll()
		} else {
			dj.mode = jotInput
		}
		return dj, nil
	case "n", "i":
		dj.mode = jotInput
		dj.pendingDelete = false
		dj.statusMsg = ""
		return dj, nil
	case "/":
		dj.mode = jotFilter
		dj.filterRunes = nil
		dj.filterCursor = 0
		dj.filterQuery = ""
		dj.filteredIdxs = nil
		dj.cursor = 0
		dj.scroll = 0
		dj.pendingDelete = false
		dj.statusMsg = ""
		return dj, nil
	case " ":
		// Toggle task state (today only)
		if dayIdx, entryIdx, ok := dj.flatIndex(dj.cursor); ok && dayIdx == 0 {
			dj.toggleTask(dayIdx, entryIdx)
		}
		return dj, nil
	case "t":
		// Convert jot to task (if not already)
		if dayIdx, entryIdx, ok := dj.flatIndex(dj.cursor); ok && dayIdx == 0 {
			e := dj.days[dayIdx].Entries[entryIdx]
			if !e.isTask() {
				dj.toggleTask(dayIdx, entryIdx)
			}
		}
		return dj, nil
	case "e", "enter":
		if dayIdx, entryIdx, ok := dj.flatIndex(dj.cursor); ok && dayIdx == 0 {
			dj.mode = jotEdit
			dj.editDayIdx = dayIdx
			dj.editEntryIdx = entryIdx
			txt := dj.days[dayIdx].Entries[entryIdx].Text
			dj.editRunes = []rune(txt)
			dj.editCursor = len(dj.editRunes)
			dj.pendingDelete = false
			dj.statusMsg = ""
		}
		return dj, nil
	case "d":
		if dayIdx, _, ok := dj.flatIndex(dj.cursor); ok && dayIdx == 0 {
			if dj.pendingDelete {
				// Second press — confirm delete
				_, entryIdx, _ := dj.flatIndex(dj.cursor)
				dj.deleteJot(dayIdx, entryIdx)
				if dj.cursor >= dj.totalEntries() && dj.cursor > 0 {
					dj.cursor--
				}
				dj.adjustScroll()
				dj.pendingDelete = false
				dj.statusMsg = ""
			} else {
				dj.pendingDelete = true
				dj.statusMsg = "Press d again to delete"
			}
		}
		return dj, nil
	case "#":
		dj.buildTagIndex()
		if len(dj.tags) == 0 {
			dj.statusMsg = "No tags found"
			return dj, nil
		}
		dj.mode = jotTags
		dj.tagCursor = 0
		dj.tagScroll = 0
		dj.selectedTag = ""
		dj.tagFiltered = nil
		dj.pendingDelete = false
		dj.statusMsg = ""
		return dj, nil
	case "p":
		if dayIdx, entryIdx, ok := dj.flatIndex(dj.cursor); ok && dayIdx == 0 {
			dj.promoteJot(dayIdx, entryIdx)
		}
		return dj, nil
	}
	return dj, nil
}

func (dj DailyJot) updateTags(msg tea.KeyMsg) (DailyJot, tea.Cmd) {
	if dj.selectedTag != "" {
		// Browsing entries for a selected tag
		return dj.updateTagFiltered(msg)
	}
	// Browsing tag list
	switch msg.String() {
	case "esc":
		dj.mode = jotBrowse
		return dj, nil
	case "j", "down":
		if dj.tagCursor < len(dj.tags)-1 {
			dj.tagCursor++
			dj.adjustTagScroll()
		}
		return dj, nil
	case "k", "up":
		if dj.tagCursor > 0 {
			dj.tagCursor--
			dj.adjustTagScroll()
		}
		return dj, nil
	case "enter":
		if dj.tagCursor < len(dj.tags) {
			dj.selectedTag = dj.tags[dj.tagCursor].Tag
			dj.buildTagFilterIndex(dj.selectedTag)
			dj.cursor = 0
			dj.scroll = 0
		}
		return dj, nil
	}
	return dj, nil
}

func (dj DailyJot) updateTagFiltered(msg tea.KeyMsg) (DailyJot, tea.Cmd) {
	switch msg.String() {
	case "esc", "backspace":
		dj.selectedTag = ""
		dj.tagFiltered = nil
		return dj, nil
	case "j", "down":
		if len(dj.tagFiltered) > 0 && dj.cursor < len(dj.tagFiltered)-1 {
			dj.cursor++
			dj.adjustFilterScroll()
		}
		return dj, nil
	case "k", "up":
		if dj.cursor > 0 {
			dj.cursor--
			dj.adjustFilterScroll()
		}
		return dj, nil
	case "enter":
		// Jump to this entry in browse mode
		if len(dj.tagFiltered) > 0 && dj.cursor < len(dj.tagFiltered) {
			realIdx := dj.tagFiltered[dj.cursor]
			dj.mode = jotBrowse
			dj.cursor = realIdx
			dj.selectedTag = ""
			dj.tagFiltered = nil
			dj.adjustScroll()
		}
		return dj, nil
	}
	return dj, nil
}

func (dj DailyJot) updateEdit(msg tea.KeyMsg) (DailyJot, tea.Cmd) {
	switch msg.String() {
	case "esc":
		dj.mode = jotBrowse
		return dj, nil
	case "enter":
		text := strings.TrimSpace(string(dj.editRunes))
		if text != "" {
			// Guard against stale indices (entries may have changed externally)
			if dj.editDayIdx < len(dj.days) && dj.editEntryIdx < len(dj.days[dj.editDayIdx].Entries) {
				dj.updateJot(dj.editDayIdx, dj.editEntryIdx, text)
			}
		}
		dj.mode = jotBrowse
		return dj, nil
	case "backspace":
		dj.deleteCharBack()
		return dj, nil
	case "left":
		if dj.editCursor > 0 {
			dj.editCursor--
		}
		return dj, nil
	case "right":
		if dj.editCursor < len(dj.editRunes) {
			dj.editCursor++
		}
		return dj, nil
	case "home", "ctrl+a":
		dj.editCursor = 0
		return dj, nil
	case "end", "ctrl+e":
		dj.editCursor = len(dj.editRunes)
		return dj, nil
	case "ctrl+u":
		dj.editRunes = dj.editRunes[dj.editCursor:]
		dj.editCursor = 0
		return dj, nil
	case "ctrl+k":
		dj.editRunes = dj.editRunes[:dj.editCursor]
		return dj, nil
	default:
		char := msg.String()
		if len(char) == 1 && char[0] >= 32 {
			dj.insertChar(rune(char[0]))
			s := string(dj.editRunes)
			if strings.HasSuffix(s, "[[") {
				dj.linkMode = true
				dj.linkQuery = ""
				dj.linkMatches = dj.filterNoteNames("")
				dj.linkCursor = 0
			}
		}
		return dj, nil
	}
}

func (dj DailyJot) updateFilter(msg tea.KeyMsg) (DailyJot, tea.Cmd) {
	switch msg.String() {
	case "esc":
		dj.mode = jotBrowse
		dj.cursor = 0
		dj.scroll = 0
		return dj, nil
	case "enter":
		// Jump to selected filtered entry in browse mode
		if len(dj.filteredIdxs) > 0 && dj.cursor < len(dj.filteredIdxs) {
			realIdx := dj.filteredIdxs[dj.cursor]
			dj.mode = jotBrowse
			dj.cursor = realIdx
			dj.adjustScroll()
		} else {
			dj.mode = jotBrowse
			dj.cursor = 0
		}
		return dj, nil
	case "backspace":
		dj.deleteCharBack()
		dj.cursor = 0
		dj.scroll = 0
		return dj, nil
	case "down", "ctrl+n":
		if len(dj.filteredIdxs) > 0 && dj.cursor < len(dj.filteredIdxs)-1 {
			dj.cursor++
			dj.adjustFilterScroll()
		}
		return dj, nil
	case "up", "ctrl+p":
		if dj.cursor > 0 {
			dj.cursor--
			dj.adjustFilterScroll()
		}
		return dj, nil
	default:
		char := msg.String()
		if len(char) == 1 && char[0] >= 32 {
			dj.insertChar(rune(char[0]))
			dj.cursor = 0
			dj.scroll = 0
		}
		return dj, nil
	}
}

func (dj DailyJot) updateLinkCompletion(msg tea.KeyMsg) (DailyJot, tea.Cmd) {
	switch msg.String() {
	case "esc":
		dj.linkMode = false
		return dj, nil
	case "tab", "enter":
		if len(dj.linkMatches) > 0 && dj.linkCursor < len(dj.linkMatches) {
			selected := dj.linkMatches[dj.linkCursor]
			buf := dj.activeString()
			idx := strings.LastIndex(buf, "[[")
			if idx >= 0 {
				completed := []rune(buf[:idx] + "[[" + selected + "]]")
				dj.setActiveRunes(completed)
				dj.setActiveCursor(len(completed))
			}
		}
		dj.linkMode = false
		return dj, nil
	case "down", "ctrl+n":
		if len(dj.linkMatches) > 0 && dj.linkCursor < len(dj.linkMatches)-1 {
			dj.linkCursor++
		}
		return dj, nil
	case "up", "ctrl+p":
		if dj.linkCursor > 0 {
			dj.linkCursor--
		}
		return dj, nil
	case "backspace":
		if dj.linkQuery == "" {
			dj.linkMode = false
			dj.deleteCharBack()
		} else {
			qr := []rune(dj.linkQuery)
			dj.linkQuery = string(qr[:len(qr)-1])
			dj.linkMatches = dj.filterNoteNames(dj.linkQuery)
			dj.linkCursor = 0
			dj.deleteCharBack()
		}
		return dj, nil
	default:
		char := msg.String()
		if len(char) == 1 && char[0] >= 32 {
			dj.linkQuery += char
			dj.linkMatches = dj.filterNoteNames(dj.linkQuery)
			dj.linkCursor = 0
			dj.insertChar(rune(char[0]))
		}
		return dj, nil
	}
}

// --- Helpers ---

func (dj DailyJot) filterNoteNames(query string) []string {
	query = strings.ToLower(query)
	var matches []string
	for _, name := range dj.noteNames {
		base := strings.TrimSuffix(filepath.Base(name), ".md")
		if query == "" || strings.Contains(strings.ToLower(base), query) {
			matches = append(matches, base)
		}
		if len(matches) >= 10 {
			break
		}
	}
	return matches
}

// --- Rendering ---

var wikilinkRe = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
var tagRe = regexp.MustCompile(`#([a-zA-Z][a-zA-Z0-9_/-]*)`)

func renderStyledText(s string, dim bool) string {
	tealStyle := lipgloss.NewStyle().Foreground(teal)
	blueStyle := lipgloss.NewStyle().Foreground(blue)
	baseColor := text
	if dim {
		baseColor = subtext0
	}
	textStyle := lipgloss.NewStyle().Foreground(baseColor)

	type styledSpan struct {
		start, end int
		rendered   string
	}
	var spans []styledSpan

	for _, loc := range wikilinkRe.FindAllStringIndex(s, -1) {
		spans = append(spans, styledSpan{loc[0], loc[1], tealStyle.Render(s[loc[0]:loc[1]])})
	}
	for _, loc := range tagRe.FindAllStringIndex(s, -1) {
		overlaps := false
		for _, sp := range spans {
			if loc[0] < sp.end && loc[1] > sp.start {
				overlaps = true
				break
			}
		}
		if !overlaps {
			spans = append(spans, styledSpan{loc[0], loc[1], blueStyle.Render(s[loc[0]:loc[1]])})
		}
	}

	if len(spans) == 0 {
		return textStyle.Render(s)
	}

	for i := 1; i < len(spans); i++ {
		for j := i; j > 0 && spans[j].start < spans[j-1].start; j-- {
			spans[j], spans[j-1] = spans[j-1], spans[j]
		}
	}

	var b strings.Builder
	pos := 0
	for _, sp := range spans {
		if sp.start > pos {
			b.WriteString(textStyle.Render(s[pos:sp.start]))
		}
		b.WriteString(sp.rendered)
		pos = sp.end
	}
	if pos < len(s) {
		b.WriteString(textStyle.Render(s[pos:]))
	}
	return b.String()
}

// renderInputLine renders a rune buffer with a visible cursor at the given position.
func renderInputLine(runes []rune, cursorPos, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	inputStyle := lipgloss.NewStyle().Foreground(text)
	cursorStyle := lipgloss.NewStyle().Background(text).Foreground(mantle)

	// Calculate visible window around cursor
	start := 0
	if cursorPos > maxWidth-1 {
		start = cursorPos - maxWidth + 1
	}
	end := start + maxWidth
	if end > len(runes) {
		end = len(runes)
	}

	var b strings.Builder
	for i := start; i < end; i++ {
		if i == cursorPos {
			b.WriteString(cursorStyle.Render(string(runes[i])))
		} else {
			b.WriteString(inputStyle.Render(string(runes[i])))
		}
	}
	// Cursor at end of text
	if cursorPos >= len(runes) {
		if cursorPos >= start && cursorPos < start+maxWidth {
			b.WriteString(cursorStyle.Render(" "))
		}
	}
	return b.String()
}

func (dj DailyJot) renderEntryText(entry jotEntry, maxWidth int, dim bool) string {
	displayText := entry.taskText()
	truncated := TruncateDisplay(displayText, maxWidth)

	var prefix string
	if entry.isDone() {
		doneStyle := lipgloss.NewStyle().Foreground(green)
		prefix = doneStyle.Render("[x] ")
		// Strikethrough effect: dim the text
		return prefix + lipgloss.NewStyle().Foreground(overlay0).Strikethrough(true).Render(truncated)
	} else if entry.isTask() {
		taskStyle := lipgloss.NewStyle().Foreground(yellow)
		prefix = taskStyle.Render("[ ] ")
		return prefix + renderStyledText(truncated, dim)
	}
	return renderStyledText(truncated, dim)
}

func (dj DailyJot) View() string {
	// Tab mode fills the editor pane; overlay mode keeps the
	// historical 60–90 char clamp for centered-popup look.
	var width int
	if dj.IsTabMode() {
		width = dj.width - 2
		if width < 60 {
			width = 60
		}
	} else {
		width = dj.width * 2 / 3
		if width > 90 {
			width = 90
		}
		if width < 60 {
			width = 60
		}
	}
	if dj.width > 0 && dj.width < width+6 {
		width = dj.width - 6
		if width < 40 {
			width = 40
		}
	}

	innerWidth := width - 6

	var b strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleStyle.Render("  " + IconViewChar + " Daily Jot"))

	// Jot count + task stats for today
	if len(dj.days) > 0 && len(dj.days[0].Entries) > 0 {
		today := dj.days[0]
		tasks, done := 0, 0
		for _, e := range today.Entries {
			if e.isTask() {
				tasks++
				if e.isDone() {
					done++
				}
			}
		}
		countStyle := lipgloss.NewStyle().Foreground(overlay0)
		if tasks > 0 {
			b.WriteString(countStyle.Render(fmt.Sprintf("  (%d jots, %d/%d tasks)", len(today.Entries), done, tasks)))
		} else {
			b.WriteString(countStyle.Render(fmt.Sprintf("  (%d today)", len(today.Entries))))
		}
	}
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")

	// Carry-over notice
	if dj.carryOverCount > 0 {
		coStyle := lipgloss.NewStyle().Foreground(yellow).Italic(true)
		b.WriteString("  " + coStyle.Render(fmt.Sprintf("↳ %d incomplete task(s) carried from yesterday", dj.carryOverCount)))
		b.WriteString("\n")
	}

	// Input / filter line
	inputWidth := innerWidth - 6
	if inputWidth < 10 {
		inputWidth = 10
	}

	if dj.mode == jotFilter {
		// Filter mode
		filterPrompt := lipgloss.NewStyle().Foreground(yellow).Bold(true)
		b.WriteString("  " + filterPrompt.Render("/ "))
		b.WriteString(renderInputLine(dj.filterRunes, dj.filterCursor, inputWidth))
		b.WriteString("\n")
	} else if dj.mode == jotInput {
		promptStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
		b.WriteString("  " + promptStyle.Render("> "))
		b.WriteString(renderInputLine(dj.inputRunes, dj.inputCursor, inputWidth))
		b.WriteString("\n")
	} else {
		dimPrompt := lipgloss.NewStyle().Foreground(overlay0)
		b.WriteString("  " + dimPrompt.Render("> "))
		placeholder := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
		b.WriteString(placeholder.Render("type a jot and press Enter..."))
		b.WriteString("\n")
	}

	// Link completion popup
	if dj.linkMode && len(dj.linkMatches) > 0 {
		b.WriteString(dj.renderLinkPopup(innerWidth))
	}

	b.WriteString(DimStyle.Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")

	// Status message
	if dj.statusMsg != "" {
		msgColor := red
		if dj.pendingDelete {
			msgColor = yellow
		}
		msgStyle := lipgloss.NewStyle().Foreground(msgColor)
		b.WriteString("  " + msgStyle.Render(dj.statusMsg) + "\n")
	}

	// Calculate available height for entries
	maxEntryLines := dj.height/2 - 8
	if maxEntryLines < 6 {
		maxEntryLines = 6
	}

	// Render entries
	if dj.mode == jotTags {
		b.WriteString(dj.renderTagView(innerWidth, maxEntryLines))
	} else if dj.mode == jotFilter {
		b.WriteString(dj.renderFilteredEntries(innerWidth, maxEntryLines))
	} else {
		b.WriteString(dj.renderAllEntries(innerWidth, maxEntryLines))
	}

	b.WriteString(DimStyle.Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")

	// Help bar
	switch dj.mode {
	case jotInput:
		b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
			{"Enter", "add"}, {"[]", "task"}, {"[[", "link"}, {"↓", "browse"}, {"Esc", "close"},
		}))
	case jotBrowse:
		b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
			{"Spc", "toggle"}, {"t", "task"}, {"e", "edit"}, {"d", "del"}, {"p", "promote"}, {"#", "tags"}, {"/", "filter"}, {"Esc", "close"},
		}))
	case jotEdit:
		b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
			{"Enter", "save"}, {"←→", "move"}, {"Esc", "cancel"},
		}))
	case jotFilter:
		b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
			{"Enter", "go to"}, {"↑↓", "select"}, {"Esc", "back"},
		}))
	case jotTags:
		if dj.selectedTag != "" {
			b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
				{"Enter", "go to"}, {"↑↓", "select"}, {"Esc", "back"},
			}))
		} else {
			b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
				{"Enter", "filter"}, {"↑↓", "select"}, {"Esc", "back"},
			}))
		}
	}

	if dj.IsTabMode() {
		return b.String()
	}
	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (dj DailyJot) renderAllEntries(innerWidth, maxEntryLines int) string {
	var b strings.Builder

	flatIdx := 0
	linesRendered := 0
	total := dj.totalEntries()

	for dayNum, day := range dj.days {
		if len(day.Entries) == 0 {
			continue
		}

		dayStart := flatIdx
		dayEnd := flatIdx + len(day.Entries)
		visible := dayEnd > dj.scroll && dayStart < dj.scroll+maxEntryLines

		if visible && linesRendered+2 < maxEntryLines {
			// Only render header if at least one entry can fit after it (+2 for header lines, +1 for entry)
			headerStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
			b.WriteString("\n")
			b.WriteString("  " + headerStyle.Render(day.Label))
			b.WriteString("\n")
			linesRendered += 2
		}

		for i := len(day.Entries) - 1; i >= 0; i-- {
			entry := day.Entries[i]
			currentFlatIdx := flatIdx + (len(day.Entries) - 1 - i)

			if currentFlatIdx < dj.scroll {
				continue
			}
			if linesRendered >= maxEntryLines {
				break
			}

			isPast := dayNum > 0
			isSelected := (dj.mode == jotBrowse || dj.mode == jotEdit) && currentFlatIdx == dj.cursor
			entryTextWidth := innerWidth - 14 // room for bullet + time + task prefix

			if dj.mode == jotEdit && isSelected {
				editTimeStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
				b.WriteString("  " + editTimeStyle.Render("  "+entry.Time+"  "))
				b.WriteString(renderInputLine(dj.editRunes, dj.editCursor, entryTextWidth))
			} else if isSelected {
				selTime := lipgloss.NewStyle().Foreground(peach)
				b.WriteString("  " + selTime.Render("▸ "+entry.Time+"  "))
				b.WriteString(dj.renderEntryText(entry, entryTextWidth, false))
			} else {
				timeStyle := lipgloss.NewStyle().Foreground(overlay0)
				bullet := "•"
				if isPast {
					bullet = "·"
				}
				b.WriteString("  " + timeStyle.Render("  "+bullet+" "+entry.Time+"  "))
				b.WriteString(dj.renderEntryText(entry, entryTextWidth, isPast))
			}
			b.WriteString("\n")
			linesRendered++
		}

		flatIdx += len(day.Entries)
	}

	if total == 0 {
		emptyStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
		b.WriteString("\n  " + emptyStyle.Render("No jots yet. Type something above and press Enter."))
		b.WriteString("\n")
	}

	// Scroll indicator
	if total > maxEntryLines {
		b.WriteString("\n")
		posStyle := lipgloss.NewStyle().Foreground(overlay0)
		b.WriteString(posStyle.Render(fmt.Sprintf("  %d–%d of %d", dj.scroll+1, minInt(dj.scroll+maxEntryLines, total), total)))
		b.WriteString("\n")
	} else {
		b.WriteString("\n")
	}

	return b.String()
}

func (dj DailyJot) renderFilteredEntries(innerWidth, maxEntryLines int) string {
	var b strings.Builder

	if len(dj.filteredIdxs) == 0 {
		emptyStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
		if dj.filterQuery == "" {
			b.WriteString("\n  " + emptyStyle.Render("Type to search jots..."))
		} else {
			b.WriteString("\n  " + emptyStyle.Render("No matching jots"))
		}
		b.WriteString("\n\n")
		return b.String()
	}

	matchStyle := lipgloss.NewStyle().Foreground(overlay0)
	b.WriteString("\n")
	b.WriteString(matchStyle.Render(fmt.Sprintf("  %d match(es)", len(dj.filteredIdxs))))
	b.WriteString("\n")

	linesRendered := 0
	for ci, flatIdx := range dj.filteredIdxs {
		if ci < dj.scroll {
			continue
		}
		if linesRendered >= maxEntryLines {
			break
		}

		if dayIdx, entryIdx, ok := dj.flatIndex(flatIdx); ok {
			entry := dj.days[dayIdx].Entries[entryIdx]
			isSelected := ci == dj.cursor
			entryTextWidth := innerWidth - 14

			timeStyle := lipgloss.NewStyle().Foreground(overlay0)
			dateTag := dj.days[dayIdx].Date[5:] + " " // "MM-DD "

			if isSelected {
				selTime := lipgloss.NewStyle().Foreground(peach)
				b.WriteString("  " + selTime.Render("▸ "+dateTag+entry.Time+"  "))
				b.WriteString(dj.renderEntryText(entry, entryTextWidth, false))
			} else {
				b.WriteString("  " + timeStyle.Render("  "+dateTag+entry.Time+"  "))
				b.WriteString(dj.renderEntryText(entry, entryTextWidth, false))
			}
			b.WriteString("\n")
			linesRendered++
		}
	}

	// Scroll indicator for filtered results
	if len(dj.filteredIdxs) > maxEntryLines {
		posStyle := lipgloss.NewStyle().Foreground(overlay0)
		b.WriteString(posStyle.Render(fmt.Sprintf("  %d–%d of %d", dj.scroll+1, minInt(dj.scroll+maxEntryLines, len(dj.filteredIdxs)), len(dj.filteredIdxs))))
		b.WriteString("\n")
	} else {
		b.WriteString("\n")
	}
	return b.String()
}

func (dj DailyJot) renderTagView(innerWidth, maxEntryLines int) string {
	var b strings.Builder

	if dj.selectedTag != "" {
		// Show entries filtered by selected tag
		headerStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
		b.WriteString("\n")
		b.WriteString("  " + headerStyle.Render(dj.selectedTag))
		countStyle := lipgloss.NewStyle().Foreground(overlay0)
		b.WriteString(countStyle.Render(fmt.Sprintf("  (%d)", len(dj.tagFiltered))))
		b.WriteString("\n")

		linesRendered := 0
		for ci, flatIdx := range dj.tagFiltered {
			if ci < dj.scroll {
				continue
			}
			if linesRendered >= maxEntryLines {
				break
			}
			if dayIdx, entryIdx, ok := dj.flatIndex(flatIdx); ok {
				entry := dj.days[dayIdx].Entries[entryIdx]
				isSelected := ci == dj.cursor
				entryTextWidth := innerWidth - 14

				dateTag := dj.days[dayIdx].Date[5:] + " "
				if isSelected {
					selTime := lipgloss.NewStyle().Foreground(peach)
					b.WriteString("  " + selTime.Render("▸ "+dateTag+entry.Time+"  "))
					b.WriteString(dj.renderEntryText(entry, entryTextWidth, false))
				} else {
					timeStyle := lipgloss.NewStyle().Foreground(overlay0)
					b.WriteString("  " + timeStyle.Render("  "+dateTag+entry.Time+"  "))
					b.WriteString(dj.renderEntryText(entry, entryTextWidth, dayIdx > 0))
				}
				b.WriteString("\n")
				linesRendered++
			}
		}

		if len(dj.tagFiltered) > maxEntryLines {
			posStyle := lipgloss.NewStyle().Foreground(overlay0)
			b.WriteString(posStyle.Render(fmt.Sprintf("  %d–%d of %d", dj.scroll+1, minInt(dj.scroll+maxEntryLines, len(dj.tagFiltered)), len(dj.tagFiltered))))
			b.WriteString("\n")
		} else {
			b.WriteString("\n")
		}
		return b.String()
	}

	// Show tag list
	titleStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	b.WriteString("\n")
	b.WriteString("  " + titleStyle.Render("Tags"))
	countStyle := lipgloss.NewStyle().Foreground(overlay0)
	b.WriteString(countStyle.Render(fmt.Sprintf("  (%d)", len(dj.tags))))
	b.WriteString("\n")

	linesRendered := 0
	for i, tag := range dj.tags {
		if i < dj.tagScroll {
			continue
		}
		if linesRendered >= maxEntryLines {
			break
		}

		tagStyle := lipgloss.NewStyle().Foreground(blue)
		numStyle := lipgloss.NewStyle().Foreground(overlay0)
		isSelected := i == dj.tagCursor

		if isSelected {
			selStyle := lipgloss.NewStyle().Foreground(peach)
			b.WriteString("  " + selStyle.Render("▸ "+tag.Tag))
			b.WriteString(numStyle.Render(fmt.Sprintf("  (%d)", tag.Count)))
		} else {
			b.WriteString("    " + tagStyle.Render(tag.Tag))
			b.WriteString(numStyle.Render(fmt.Sprintf("  (%d)", tag.Count)))
		}
		b.WriteString("\n")
		linesRendered++
	}

	if len(dj.tags) > maxEntryLines {
		posStyle := lipgloss.NewStyle().Foreground(overlay0)
		b.WriteString(posStyle.Render(fmt.Sprintf("  %d–%d of %d", dj.tagScroll+1, minInt(dj.tagScroll+maxEntryLines, len(dj.tags)), len(dj.tags))))
		b.WriteString("\n")
	} else {
		b.WriteString("\n")
	}

	return b.String()
}

func (dj DailyJot) renderLinkPopup(innerWidth int) string {
	var b strings.Builder
	maxShow := 6
	total := len(dj.linkMatches)
	if total < maxShow {
		maxShow = total
	}

	// Scroll the popup window to keep linkCursor visible
	start := 0
	if dj.linkCursor >= maxShow {
		start = dj.linkCursor - maxShow + 1
	}
	end := start + maxShow
	if end > total {
		end = total
		start = end - maxShow
		if start < 0 {
			start = 0
		}
	}

	popupBg := lipgloss.NewStyle().
		Foreground(teal).
		Background(surface0).
		Padding(0, 1)

	selBg := lipgloss.NewStyle().
		Foreground(mantle).
		Background(teal).
		Bold(true).
		Padding(0, 1)

	for i := start; i < end; i++ {
		name := dj.linkMatches[i]
		if lipgloss.Width(name) > innerWidth-8 {
			name = TruncateDisplay(name, innerWidth-8)
		}
		b.WriteString("    ")
		if i == dj.linkCursor {
			b.WriteString(selBg.Render("▸ " + name))
		} else {
			b.WriteString(popupBg.Render("  " + name))
		}
		b.WriteString("\n")
	}
	if end < total {
		moreStyle := lipgloss.NewStyle().Foreground(overlay0)
		b.WriteString("    " + moreStyle.Render(fmt.Sprintf("  … %d more", total-end)))
		b.WriteString("\n")
	}
	return b.String()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
