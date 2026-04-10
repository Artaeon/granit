package tui

import (
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// fieldType describes how a frontmatter field is displayed and edited.
type fieldType int

const (
	ftString  fieldType = iota
	ftTags              // []interface{} rendered as comma-separated pills
	ftDate              // date string with YYYY-MM-DD validation
	ftBool              // true/false toggled with Enter
	ftNumber            // numeric input
)

// fmField holds one parsed frontmatter key-value pair.
type fmField struct {
	key      string
	value    string      // string representation for editing
	listVals []string    // only for ftTags
	boolVal  bool        // only for ftBool
	kind     fieldType
}

// FrontmatterEditor provides a structured overlay for editing YAML frontmatter.
type FrontmatterEditor struct {
	active  bool
	fields  []fmField
	cursor  int
	scroll  int
	width   int
	height  int

	editing    bool   // currently editing a field value
	editBuf    string // edit buffer for the current field
	addingKey  bool   // currently entering a new field name
	addKeyBuf  string

	confirmDel  bool // awaiting delete confirmation
	presetMenu  bool // preset menu is open
	presetIdx   int

	result    string // the generated YAML frontmatter
	consumed  bool   // whether GetResult was already called
	hasResult bool   // a result is ready to be consumed
}

// NewFrontmatterEditor creates a new (inactive) frontmatter editor.
func NewFrontmatterEditor() FrontmatterEditor {
	return FrontmatterEditor{}
}

// IsActive returns whether the overlay is currently visible.
func (fe *FrontmatterEditor) IsActive() bool {
	return fe.active
}

// SetSize updates the available dimensions.
func (fe *FrontmatterEditor) SetSize(w, h int) {
	fe.width = w
	fe.height = h
}

// Close hides the overlay.
func (fe *FrontmatterEditor) Close() {
	fe.active = false
	fe.editing = false
	fe.addingKey = false
	fe.confirmDel = false
	fe.presetMenu = false
}

// Open parses frontmatter from the given note content and opens the overlay.
func (fe *FrontmatterEditor) Open(content string) {
	fe.active = true
	fe.cursor = 0
	fe.scroll = 0
	fe.editing = false
	fe.addingKey = false
	fe.confirmDel = false
	fe.presetMenu = false
	fe.hasResult = false
	fe.consumed = false
	fe.result = ""
	fe.editBuf = ""
	fe.addKeyBuf = ""
	fe.presetIdx = 0
	fe.fields = nil

	fe.parseFrontmatter(content)
}

// GetResult returns the generated YAML frontmatter block (with --- markers)
// using a consumed-once pattern. Returns ("", false) if no result is ready.
func (fe *FrontmatterEditor) GetResult() (string, bool) {
	if fe.hasResult && !fe.consumed {
		fe.consumed = true
		return fe.result, true
	}
	return "", false
}

// ---------------------------------------------------------------------------
// Parsing
// ---------------------------------------------------------------------------

func (fe *FrontmatterEditor) parseFrontmatter(content string) {
	if !strings.HasPrefix(content, "---") {
		return
	}
	end := strings.Index(content[3:], "---")
	if end == -1 {
		return
	}

	block := content[3 : 3+end]
	lines := strings.Split(strings.TrimSpace(block), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if key == "" {
			continue
		}
		fe.fields = append(fe.fields, fe.classifyField(key, val))
	}
}

// classifyField determines the field type from the key name and value shape.
func (fe *FrontmatterEditor) classifyField(key, val string) fmField {
	lk := strings.ToLower(key)

	// Array values: [item1, item2, ...]
	if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
		inner := val[1 : len(val)-1]
		items := splitTrimCSV(inner)
		return fmField{key: key, value: val, listVals: items, kind: ftTags}
	}

	// Keys commonly holding tags/aliases
	if lk == "tags" || lk == "aliases" || lk == "keywords" {
		items := splitTrimCSV(val)
		return fmField{key: key, value: val, listVals: items, kind: ftTags}
	}

	// Boolean
	lv := strings.ToLower(val)
	if lv == "true" || lv == "false" {
		return fmField{key: key, value: val, boolVal: lv == "true", kind: ftBool}
	}

	// Date (YYYY-MM-DD)
	if lk == "date" || lk == "created" || lk == "updated" || lk == "modified" || lk == "due" {
		return fmField{key: key, value: val, kind: ftDate}
	}
	if fmIsDateStr(val) {
		return fmField{key: key, value: val, kind: ftDate}
	}

	// Number
	if fmIsNumeric(val) {
		return fmField{key: key, value: val, kind: ftNumber}
	}

	return fmField{key: key, value: val, kind: ftString}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (fe FrontmatterEditor) Update(msg tea.KeyMsg) (FrontmatterEditor, tea.Cmd) {
	if !fe.active {
		return fe, nil
	}

	// --- Preset menu ---
	if fe.presetMenu {
		return fe.updatePresetMenu(msg)
	}

	// --- Delete confirmation ---
	if fe.confirmDel {
		return fe.updateConfirmDel(msg)
	}

	// --- Adding new key ---
	if fe.addingKey {
		return fe.updateAddingKey(msg)
	}

	// --- Editing field value ---
	if fe.editing {
		return fe.updateEditing(msg)
	}

	// --- Normal navigation ---
	return fe.updateNormal(msg)
}

func (fe FrontmatterEditor) updateNormal(msg tea.KeyMsg) (FrontmatterEditor, tea.Cmd) {
	visH := fe.visibleHeight()

	switch msg.String() {
	case "esc":
		fe.active = false
	case "up", "k":
		if fe.cursor > 0 {
			fe.cursor--
			if fe.cursor < fe.scroll {
				fe.scroll = fe.cursor
			}
		}
	case "down", "j":
		if fe.cursor < len(fe.fields)-1 {
			fe.cursor++
			if fe.cursor >= fe.scroll+visH {
				fe.scroll = fe.cursor - visH + 1
			}
		}
	case "enter":
		if len(fe.fields) > 0 && fe.cursor < len(fe.fields) {
			f := &fe.fields[fe.cursor]
			switch f.kind {
			case ftBool:
				f.boolVal = !f.boolVal
				if f.boolVal {
					f.value = "true"
				} else {
					f.value = "false"
				}
			default:
				fe.editing = true
				if f.kind == ftTags {
					fe.editBuf = strings.Join(f.listVals, ", ")
				} else {
					fe.editBuf = f.value
				}
			}
		}
	case "a":
		fe.addingKey = true
		fe.addKeyBuf = ""
	case "d":
		if len(fe.fields) > 0 && fe.cursor < len(fe.fields) {
			fe.confirmDel = true
		}
	case "p":
		fe.presetMenu = true
		fe.presetIdx = 0
	case "ctrl+s":
		fe.result = fe.buildYAML()
		fe.hasResult = true
		fe.consumed = false
		fe.active = false
	}
	return fe, nil
}

func (fe FrontmatterEditor) updateEditing(msg tea.KeyMsg) (FrontmatterEditor, tea.Cmd) {
	switch msg.String() {
	case "esc":
		fe.editing = false
		fe.editBuf = ""
	case "enter":
		if fe.cursor < len(fe.fields) {
			f := &fe.fields[fe.cursor]
			switch f.kind {
			case ftTags:
				items := splitTrimCSV(fe.editBuf)
				f.listVals = items
				f.value = "[" + strings.Join(items, ", ") + "]"
			case ftDate:
				// Validate date format
				if fmIsDateStr(fe.editBuf) || fe.editBuf == "" {
					f.value = fe.editBuf
				}
				// invalid dates are silently rejected (keep old value)
			case ftNumber:
				if fmIsNumeric(fe.editBuf) || fe.editBuf == "" {
					f.value = fe.editBuf
				}
			default:
				f.value = fe.editBuf
			}
		}
		fe.editing = false
		fe.editBuf = ""
	case "backspace":
		if len(fe.editBuf) > 0 {
			fe.editBuf = TrimLastRune(fe.editBuf)
		}
	default:
		ch := msg.String()
		if len(ch) == 1 && ch[0] >= 32 {
			fe.editBuf += ch
		}
	}
	return fe, nil
}

func (fe FrontmatterEditor) updateAddingKey(msg tea.KeyMsg) (FrontmatterEditor, tea.Cmd) {
	switch msg.String() {
	case "esc":
		fe.addingKey = false
		fe.addKeyBuf = ""
	case "enter":
		name := strings.TrimSpace(fe.addKeyBuf)
		if name != "" && !fe.hasField(name) {
			fe.fields = append(fe.fields, fmField{key: name, value: "", kind: ftString})
			fe.cursor = len(fe.fields) - 1
			fe.ensureCursorVisible()
		}
		fe.addingKey = false
		fe.addKeyBuf = ""
	case "backspace":
		if len(fe.addKeyBuf) > 0 {
			fe.addKeyBuf = TrimLastRune(fe.addKeyBuf)
		}
	default:
		ch := msg.String()
		if len(ch) == 1 && ch[0] >= 32 {
			fe.addKeyBuf += ch
		}
	}
	return fe, nil
}

func (fe FrontmatterEditor) updateConfirmDel(msg tea.KeyMsg) (FrontmatterEditor, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		if fe.cursor < len(fe.fields) {
			fe.fields = append(fe.fields[:fe.cursor], fe.fields[fe.cursor+1:]...)
			if fe.cursor >= len(fe.fields) && fe.cursor > 0 {
				fe.cursor--
			}
		}
		fe.confirmDel = false
	default:
		fe.confirmDel = false
	}
	return fe, nil
}

// Preset fields that can be added with the 'p' key.
var fmPresets = []struct {
	key   string
	value string
	kind  fieldType
}{
	{"title", "", ftString},
	{"date", time.Now().Format("2006-01-02"), ftDate},
	{"tags", "[]", ftTags},
	{"type", "note", ftString},
	{"aliases", "[]", ftTags},
}

func (fe FrontmatterEditor) updatePresetMenu(msg tea.KeyMsg) (FrontmatterEditor, tea.Cmd) {
	switch msg.String() {
	case "esc":
		fe.presetMenu = false
	case "up", "k":
		if fe.presetIdx > 0 {
			fe.presetIdx--
		}
	case "down", "j":
		if fe.presetIdx < len(fmPresets)-1 {
			fe.presetIdx++
		}
	case "enter":
		p := fmPresets[fe.presetIdx]
		if !fe.hasField(p.key) {
			f := fmField{key: p.key, value: p.value, kind: p.kind}
			if p.kind == ftTags {
				f.listVals = nil
			}
			if p.kind == ftBool {
				f.boolVal = false
			}
			fe.fields = append(fe.fields, f)
			fe.cursor = len(fe.fields) - 1
			fe.ensureCursorVisible()
		}
		fe.presetMenu = false
	}
	return fe, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (fe FrontmatterEditor) View() string {
	width := fe.width / 2
	if width < 55 {
		width = 55
	}
	if width > 80 {
		width = 80
	}
	innerWidth := width - 6

	var b strings.Builder

	// Title
	titleSt := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleSt.Render("  Frontmatter Editor"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat(ThemeSeparator, innerWidth-4)))
	b.WriteString("\n\n")

	// --- Preset menu (sub-overlay) ---
	if fe.presetMenu {
		b.WriteString(fe.viewPresetMenu(innerWidth))
		b.WriteString("\n\n")
		b.WriteString(DimStyle.Render("  Enter: add  Esc: cancel"))
		return fe.wrapBorder(width, b.String())
	}

	// --- Add-key input ---
	if fe.addingKey {
		promptSt := lipgloss.NewStyle().Foreground(peach).Bold(true)
		b.WriteString(promptSt.Render("  New field name: "))
		inputBg := lipgloss.NewStyle().Background(surface0).Foreground(text)
		cursor := lipgloss.NewStyle().Foreground(mauve).Render("|")
		b.WriteString(inputBg.Render(" " + fe.addKeyBuf + cursor))
		b.WriteString("\n\n")
		b.WriteString(DimStyle.Render("  Enter: add  Esc: cancel"))
		return fe.wrapBorder(width, b.String())
	}

	// --- Delete confirmation ---
	if fe.confirmDel && fe.cursor < len(fe.fields) {
		warnSt := lipgloss.NewStyle().Foreground(red).Bold(true)
		b.WriteString(warnSt.Render("  Delete field \"" + fe.fields[fe.cursor].key + "\"?"))
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  Press y to confirm, any other key to cancel"))
		return fe.wrapBorder(width, b.String())
	}

	// --- Field list ---
	if len(fe.fields) == 0 {
		b.WriteString(DimStyle.Render("  No frontmatter fields"))
		b.WriteString("\n")
	} else {
		visH := fe.visibleHeight()
		end := fe.scroll + visH
		if end > len(fe.fields) {
			end = len(fe.fields)
		}

		keySt := lipgloss.NewStyle().Foreground(blue).Bold(true)

		for i := fe.scroll; i < end; i++ {
			f := fe.fields[i]
			isSel := i == fe.cursor

			// Type indicator
			typeTag := fe.typeLabel(f.kind)

			// Key
			keyStr := keySt.Render(f.key)

			// Value rendering depends on type
			var valStr string
			if fe.editing && isSel {
				// Show edit buffer with cursor
				editCursor := lipgloss.NewStyle().Foreground(mauve).Render("|")
				editBg := lipgloss.NewStyle().Background(surface0).Foreground(text)
				valStr = editBg.Render(" " + fe.editBuf + editCursor + " ")
			} else {
				valStr = fe.renderFieldValue(f, innerWidth-20)
			}

			line := "  " + typeTag + " " + keyStr + ": " + valStr

			if isSel {
				accentBar := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(ThemeAccentBar)
				selBg := lipgloss.NewStyle().Background(surface0).Width(innerWidth)
				b.WriteString(selBg.Render(accentBar + line[1:])) // replace leading space with accent bar
			} else {
				b.WriteString(line)
			}
			if i < end-1 {
				b.WriteString("\n")
			}
		}

		// Scroll indicator
		if len(fe.fields) > visH {
			b.WriteString("\n")
			moreSt := lipgloss.NewStyle().Foreground(surface2).Italic(true)
			total := len(fe.fields)
			b.WriteString(moreSt.Render("  " + fmItoa(fe.scroll+1) + "-" + fmItoa(end) + " of " + fmItoa(total)))
		}
	}

	// Help bar
	b.WriteString("\n\n")
	helpParts := []string{
		"Enter: edit/toggle",
		"a: add field",
		"d: delete",
		"p: presets",
		"Ctrl+S: save",
		"Esc: cancel",
	}
	b.WriteString(DimStyle.Render("  " + strings.Join(helpParts, "  ")))

	return fe.wrapBorder(width, b.String())
}

func (fe FrontmatterEditor) renderFieldValue(f fmField, maxW int) string {
	switch f.kind {
	case ftBool:
		if f.boolVal {
			return lipgloss.NewStyle().Foreground(green).Bold(true).Render("true")
		}
		return lipgloss.NewStyle().Foreground(red).Render("false")

	case ftTags:
		if len(f.listVals) == 0 {
			return DimStyle.Render("(empty)")
		}
		var pills []string
		pillSt := lipgloss.NewStyle().
			Foreground(crust).
			Background(blue).
			Padding(0, 1)
		for _, tag := range f.listVals {
			if tag != "" {
				pills = append(pills, pillSt.Render(tag))
			}
		}
		rendered := strings.Join(pills, " ")
		return rendered

	case ftDate:
		if f.value == "" {
			return DimStyle.Render("(no date)")
		}
		dateSt := lipgloss.NewStyle().Foreground(peach)
		return dateSt.Render(f.value)

	case ftNumber:
		numSt := lipgloss.NewStyle().Foreground(yellow)
		if f.value == "" {
			return DimStyle.Render("(empty)")
		}
		return numSt.Render(f.value)

	default: // ftString
		if f.value == "" {
			return DimStyle.Render("(empty)")
		}
		v := TruncateDisplay(f.value, maxW)
		return lipgloss.NewStyle().Foreground(text).Render(v)
	}
}

func (fe FrontmatterEditor) typeLabel(k fieldType) string {
	switch k {
	case ftBool:
		return lipgloss.NewStyle().Foreground(green).Render("[b]")
	case ftTags:
		return lipgloss.NewStyle().Foreground(blue).Render("[t]")
	case ftDate:
		return lipgloss.NewStyle().Foreground(peach).Render("[d]")
	case ftNumber:
		return lipgloss.NewStyle().Foreground(yellow).Render("[n]")
	default:
		return lipgloss.NewStyle().Foreground(overlay0).Render("[s]")
	}
}

func (fe FrontmatterEditor) viewPresetMenu(innerWidth int) string {
	var b strings.Builder
	headerSt := lipgloss.NewStyle().Foreground(peach).Bold(true)
	b.WriteString(headerSt.Render("  Add common field:"))
	b.WriteString("\n\n")

	for i, p := range fmPresets {
		exists := fe.hasField(p.key)
		label := p.key
		if exists {
			label += " (exists)"
		}

		if i == fe.presetIdx {
			accentBar := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(ThemeAccentBar)
			nameSt := lipgloss.NewStyle().Foreground(mauve).Bold(true)
			if exists {
				nameSt = nameSt.Strikethrough(true)
			}
			b.WriteString("  " + accentBar + " " + nameSt.Render(label))
		} else {
			st := lipgloss.NewStyle().Foreground(text)
			if exists {
				st = DimStyle.Copy().Strikethrough(true)
			}
			b.WriteString("    " + st.Render(label))
		}
		if i < len(fmPresets)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func (fe FrontmatterEditor) wrapBorder(width int, content string) string {
	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(content)
}

// ---------------------------------------------------------------------------
// YAML generation
// ---------------------------------------------------------------------------

func (fe FrontmatterEditor) buildYAML() string {
	if len(fe.fields) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("---\n")
	for _, f := range fe.fields {
		b.WriteString(f.key)
		b.WriteString(": ")
		switch f.kind {
		case ftTags:
			b.WriteString("[")
			b.WriteString(strings.Join(f.listVals, ", "))
			b.WriteString("]")
		case ftBool:
			if f.boolVal {
				b.WriteString("true")
			} else {
				b.WriteString("false")
			}
		default:
			b.WriteString(f.value)
		}
		b.WriteString("\n")
	}
	b.WriteString("---")
	return b.String()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (fe FrontmatterEditor) hasField(key string) bool {
	lk := strings.ToLower(key)
	for _, f := range fe.fields {
		if strings.ToLower(f.key) == lk {
			return true
		}
	}
	return false
}

func (fe *FrontmatterEditor) ensureCursorVisible() {
	visH := fe.visibleHeight()
	if fe.cursor >= fe.scroll+visH {
		fe.scroll = fe.cursor - visH + 1
	}
	if fe.cursor < fe.scroll {
		fe.scroll = fe.cursor
	}
}

func (fe FrontmatterEditor) visibleHeight() int {
	h := fe.height - 10
	if h < 3 {
		h = 3
	}
	return h
}

// splitTrimCSV splits a comma-separated string and trims whitespace from each item.
func splitTrimCSV(s string) []string {
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			result = append(result, t)
		}
	}
	return result
}

// fmIsDateStr returns true if s looks like YYYY-MM-DD.
func fmIsDateStr(s string) bool {
	if len(s) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// fmIsNumeric returns true if s is a valid integer or float.
func fmIsNumeric(s string) bool {
	if s == "" {
		return false
	}
	dotSeen := false
	for i, ch := range s {
		if ch == '-' && i == 0 {
			continue
		}
		if ch == '.' && !dotSeen {
			dotSeen = true
			continue
		}
		if !unicode.IsDigit(ch) {
			return false
		}
	}
	return true
}

// fmItoa converts an int to string without importing strconv.
func fmItoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}

