package tui

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artaeon/granit/internal/vault"
)

func (m *Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+p":
		m.searchMode = false
		return m, nil
	case "enter":
		if len(m.searchResults) > 0 && m.searchCursor < len(m.searchResults) {
			m.loadNote(m.searchResults[m.searchCursor])
			m.setSidebarCursorToFile(m.searchResults[m.searchCursor])
			m.setFocus(focusEditor)
		}
		m.searchMode = false
		return m, nil
	case "up":
		if m.searchCursor > 0 {
			m.searchCursor--
		}
		return m, nil
	case "down":
		if m.searchCursor < len(m.searchResults)-1 {
			m.searchCursor++
		}
		return m, nil
	case "backspace":
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
			m.filterSearch()
		}
		return m, nil
	default:
		char := msg.String()
		if len(char) == 1 && char[0] >= 32 {
			m.searchQuery += char
			m.filterSearch()
		}
		return m, nil
	}
}

func (m *Model) filterSearch() {
	if m.searchQuery == "" {
		m.searchResults = m.vault.SortedPaths()
		m.searchCursor = 0
		return
	}
	query := strings.ToLower(m.searchQuery)
	m.searchResults = nil

	// Rank results: exact filename > prefix > fuzzy filename > content match
	var exactMatches, prefixMatches, fuzzyMatches, contentMatches []string
	seen := make(map[string]bool)

	for _, path := range m.vault.SortedPaths() {
		lowerPath := strings.ToLower(path)
		baseName := strings.ToLower(strings.TrimSuffix(filepath.Base(path), ".md"))

		if baseName == query {
			exactMatches = append(exactMatches, path)
			seen[path] = true
		} else if strings.HasPrefix(baseName, query) {
			prefixMatches = append(prefixMatches, path)
			seen[path] = true
		} else if fuzzyMatch(lowerPath, query) {
			fuzzyMatches = append(fuzzyMatches, path)
			seen[path] = true
		}
	}

	for _, path := range m.vault.SortedPaths() {
		if seen[path] {
			continue
		}
		note := m.vault.GetNote(path)
		if note != nil && strings.Contains(strings.ToLower(note.Content), query) {
			contentMatches = append(contentMatches, path)
		}
	}

	m.searchResults = append(m.searchResults, exactMatches...)
	m.searchResults = append(m.searchResults, prefixMatches...)
	m.searchResults = append(m.searchResults, fuzzyMatches...)
	m.searchResults = append(m.searchResults, contentMatches...)

	if len(m.searchResults) == 0 {
		m.searchCursor = 0
	} else if m.searchCursor >= len(m.searchResults) {
		m.searchCursor = len(m.searchResults) - 1
	}
}

func (m *Model) updateNewNote(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.newNoteMode = false
		m.pendingTemplate = ""
		return m, nil
	case "enter":
		if m.newNoteName != "" {
			name := m.newNoteName
			if !strings.HasSuffix(name, ".md") {
				name += ".md"
			}
			path := filepath.Join(m.vault.Root, name)
			title := strings.TrimSuffix(filepath.Base(name), ".md")
			content := "---\ntitle: " + title + "\ndate: " + time.Now().Format("2006-01-02") + "\ntags: []\n---\n\n# " + title + "\n\n"
			if m.pendingTemplate != "" {
				content = strings.ReplaceAll(m.pendingTemplate, "{{title}}", title)
				m.pendingTemplate = ""
			}

			if err := os.MkdirAll(filepath.Dir(path), 0755); err == nil {
				if err := os.WriteFile(path, []byte(content), 0644); err == nil {
					_ = m.vault.Scan()
					m.index = vault.NewIndex(m.vault)
					m.index.Build()
					m.sidebar.SetFiles(m.vault.SortedPaths())
					m.statusbar.SetNoteCount(m.vault.NoteCount())
					m.loadNote(name)
					m.setSidebarCursorToFile(name)
					m.setFocus(focusEditor)
					m.statusbar.SetMessage("Created " + name)
				}
			}
		}
		m.newNoteMode = false
		return m, tea.Batch(nil, m.clearMessageAfter(2*time.Second))
	case "backspace":
		if len(m.newNoteName) > 0 {
			m.newNoteName = m.newNoteName[:len(m.newNoteName)-1]
		}
		return m, nil
	default:
		char := msg.String()
		if len(char) == 1 && char[0] >= 32 {
			m.newNoteName += char
		}
		return m, nil
	}
}

func (m *Model) updateExtractNote(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.extractMode = false
		return m, nil
	case "enter":
		if m.extractName != "" {
			sel := m.editor.GetSelectedText()
			if sel == "" {
				m.extractMode = false
				m.statusbar.SetMessage("No text selected")
				return m, m.clearMessageAfter(2 * time.Second)
			}

			name := m.extractName
			if !strings.HasSuffix(name, ".md") {
				name += ".md"
			}
			path := filepath.Join(m.vault.Root, name)

			if _, err := os.Stat(path); err == nil {
				m.statusbar.SetMessage("Note already exists: " + name)
				m.extractMode = false
				return m, m.clearMessageAfter(2 * time.Second)
			}

			title := strings.TrimSuffix(filepath.Base(name), ".md")
			content := "---\ntitle: " + title + "\ndate: " + time.Now().Format("2006-01-02") + "\ntags: []\n---\n\n" + sel + "\n"

			if err := os.MkdirAll(filepath.Dir(path), 0755); err == nil {
				if err := os.WriteFile(path, []byte(content), 0644); err == nil {
					m.editor.DeleteSelection()
					m.editor.InsertText("[[" + title + "]]")
					m.saveCurrentNote()()
					_ = m.vault.Scan()
					m.index = vault.NewIndex(m.vault)
					m.index.Build()
					paths := m.vault.SortedPaths()
					m.sidebar.SetFiles(paths)
					m.autocomplete.SetNotes(paths)
					m.statusbar.SetNoteCount(m.vault.NoteCount())
					m.statusbar.SetMessage("Extracted to [[" + title + "]]")
				}
			}
		}
		m.extractMode = false
		return m, m.clearMessageAfter(2 * time.Second)
	case "backspace":
		if len(m.extractName) > 0 {
			m.extractName = m.extractName[:len(m.extractName)-1]
		}
		return m, nil
	default:
		char := msg.String()
		if len(char) == 1 && char[0] >= 32 {
			m.extractName += char
		}
		return m, nil
	}
}

// splitLinkAnchor splits a wikilink target like "note#heading" into the note
// part and the heading anchor (without the '#'). If there is no anchor the
// second return value is "".
func splitLinkAnchor(link string) (string, string) {
	if idx := strings.Index(link, "#"); idx >= 0 {
		return link[:idx], link[idx+1:]
	}
	return link, ""
}

func (m *Model) resolveLink(link string) string {
	// Strip heading anchor before resolving.
	notePart, _ := splitLinkAnchor(link)

	if m.vault.GetNote(notePart) != nil {
		return notePart
	}
	if !strings.HasSuffix(notePart, ".md") {
		withMd := notePart + ".md"
		if m.vault.GetNote(withMd) != nil {
			return withMd
		}
	}
	base := filepath.Base(notePart)
	if !strings.HasSuffix(base, ".md") {
		base += ".md"
	}
	for _, p := range m.vault.SortedPaths() {
		if filepath.Base(p) == base {
			return p
		}
	}
	return ""
}

// navigateToHeading scrolls the editor so that the first heading whose text
// matches anchor (case-insensitive, ignoring leading '#' characters and
// surrounding whitespace) is visible near the top of the viewport.
func (m *Model) navigateToHeading(anchor string) {
	if anchor == "" {
		return
	}
	// Normalise the anchor: Obsidian lowercases and replaces spaces with
	// hyphens in URL-style anchors, so we do a relaxed comparison.
	normalize := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		s = strings.ReplaceAll(s, "-", " ")
		s = strings.ReplaceAll(s, "_", " ")
		return s
	}
	target := normalize(anchor)

	for i, line := range m.editor.content {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Strip leading '#' characters and the mandatory space.
		headingText := strings.TrimLeft(trimmed, "#")
		headingText = strings.TrimSpace(headingText)
		if normalize(headingText) == target {
			m.editor.SetCursorPosition(i, 0)
			m.editor.SetScroll(i)
			return
		}
	}
}

func (m *Model) findFileIndex(relPath string) int {
	for i, f := range m.sidebar.filtered {
		if f == relPath {
			return i
		}
	}
	return -1
}

// setSidebarCursorToFile updates the sidebar cursor to point at relPath.
// If relPath is not found in the filtered list, the cursor is left unchanged.
func (m *Model) setSidebarCursorToFile(relPath string) {
	if idx := m.findFileIndex(relPath); idx >= 0 {
		m.sidebar.cursor = idx
	}
}

func (m *Model) setFocus(f focus) {
	m.focus = f
	m.sidebar.focused = (f == focusSidebar)
	m.sidebar.fileTree.SetFocused(f == focusSidebar)
	m.editor.focused = (f == focusEditor)
	m.backlinks.focused = (f == focusBacklinks)

	switch f {
	case focusSidebar:
		m.statusbar.SetMode("FILES")
	case focusEditor:
		if m.viewMode {
			m.statusbar.SetMode("VIEW")
		} else if m.vimState != nil && m.vimState.IsEnabled() {
			mode := "VIM:" + m.vimState.ModeString()
			if rs := m.vimState.RecordingStatus(); rs != "" {
				mode += " [" + rs + "]"
			}
			m.statusbar.SetMode(mode)
		} else {
			m.statusbar.SetMode("EDIT")
		}
	case focusBacklinks:
		m.statusbar.SetMode("LINKS")
	}
}

func (m *Model) cycleFocus(direction int) {
	newFocus := (int(m.focus) + direction + 3) % 3
	m.setFocus(focus(newFocus))
}

func (m *Model) updateLayout() {
	layout := m.config.Layout
	if layout == "" {
		layout = "default"
	}

	// Adaptive layout: auto-collapse panels for small terminals
	if m.width < 80 {
		// Very narrow — editor only (mobile-friendly)
		layout = "minimal"
	} else if m.width < 120 {
		// Medium — drop multi-panel layouts to 2-panel or less
		switch layout {
		case "default", "research":
			layout = "writer"
		case "reading", "cornell", "preview":
			layout = "minimal"
		case "dashboard", "taskboard", "cockpit", "stacked", "kanban", "widescreen":
			layout = "writer"
		case "focus":
			layout = "minimal"
		case "presenter":
			layout = "minimal"
		}
	} else if m.width < 160 {
		// Not wide enough for 4+ panels — fall back to 3-panel or less
		switch layout {
		case "dashboard", "cockpit", "stacked", "widescreen":
			layout = "default"
		case "taskboard", "kanban":
			layout = "writer"
		}
	}

	showSidebar := LayoutHasSidebar(layout)
	showBacklinks := LayoutHasBacklinks(layout)
	showOutline := LayoutHasOutline(layout)

	sidebarWidth := 0
	backlinksWidth := 0
	outlineWidth := 0
	if showSidebar {
		sidebarWidth = m.width / 5
		if sidebarWidth < 22 {
			sidebarWidth = 22
		}
		if sidebarWidth > 35 {
			sidebarWidth = 35
		}
	}
	if showBacklinks {
		backlinksWidth = m.width / 5
		if backlinksWidth < 22 {
			backlinksWidth = 22
		}
		if backlinksWidth > 30 {
			backlinksWidth = 30
		}
	}
	if showOutline {
		outlineWidth = m.width / 7
		if outlineWidth < 18 {
			outlineWidth = 18
		}
		if outlineWidth > 25 {
			outlineWidth = 25
		}
	}

	panelBorders := 0
	if showSidebar {
		panelBorders += 2
	}
	if showBacklinks {
		panelBorders += 2
	}
	if showOutline {
		panelBorders += 2
	}

	// Ensure panel widths + minimum editor (30) + borders fit in terminal width.
	// The editor border always takes 2 columns.
	minEditorWidth := 30
	totalPanels := sidebarWidth + backlinksWidth + outlineWidth
	available := m.width - panelBorders - 2 // space for editor after borders
	if totalPanels+minEditorWidth > available {
		// Proportionally shrink panels to fit
		budget := available - minEditorWidth
		if budget < 0 {
			budget = 0
		}
		if totalPanels > 0 && budget > 0 {
			ratio := float64(budget) / float64(totalPanels)
			sidebarWidth = int(float64(sidebarWidth) * ratio)
			backlinksWidth = int(float64(backlinksWidth) * ratio)
			outlineWidth = int(float64(outlineWidth) * ratio)
			// Distribute rounding remainder to sidebar (largest panel)
			remainder := budget - sidebarWidth - backlinksWidth - outlineWidth
			sidebarWidth += remainder
		} else {
			// No room for panels at all — hide them
			sidebarWidth = 0
			backlinksWidth = 0
			outlineWidth = 0
		}
		// Hide panels that shrank below a usable minimum
		if outlineWidth > 0 && outlineWidth < 10 {
			outlineWidth = 0
			panelBorders -= 2
		}
		if backlinksWidth > 0 && backlinksWidth < 10 {
			backlinksWidth = 0
			panelBorders -= 2
		}
		if sidebarWidth > 0 && sidebarWidth < 10 {
			sidebarWidth = 0
			panelBorders -= 2
		}
	}

	editorWidth := m.width - sidebarWidth - backlinksWidth - outlineWidth - panelBorders - 2

	// Taskboard and research layouts have an extra right panel
	if layout == "taskboard" || layout == "research" {
		extraPanelWidth := m.width / 4
		if extraPanelWidth < 25 {
			extraPanelWidth = 25
		}
		if extraPanelWidth > 40 {
			extraPanelWidth = 40
		}
		editorWidth = m.width - sidebarWidth - extraPanelWidth - 6
	}
	if editorWidth < 30 {
		editorWidth = 30
	}

	// Content height: terminal minus status bar (2 lines) minus panel borders (2 lines)
	overhead := 4
	if m.breadcrumb != nil && (len(m.breadcrumb.Pinned()) > 0 || m.breadcrumb.CanGoBack()) {
		overhead++ // breadcrumb nav bar between content and status
	}
	contentHeight := m.height - overhead
	if contentHeight < 1 {
		contentHeight = 1
	}

	m.sidebar.SetSize(sidebarWidth, contentHeight)
	m.editor.SetSize(editorWidth, contentHeight)
	m.renderer.SetSize(editorWidth, contentHeight)
	m.backlinks.SetSize(backlinksWidth, contentHeight)
	m.statusbar.SetWidth(m.width)
	m.publisher.SetSize(m.width, m.height)
	m.splitPane.SetSize(m.width, m.height)
	m.luaOverlay.SetSize(m.width, m.height)
	m.flashcards.SetSize(m.width, m.height)
	m.quizMode.SetSize(m.width, m.height)
	m.learnDash.SetSize(m.width, m.height)
	m.aiChat.SetSize(m.width, m.height)
	m.composer.SetSize(m.width, m.height)
	m.knowledgeGraph.SetSize(m.width, m.height)
	m.tableEditor.SetSize(m.width, m.height)
	m.semanticSearch.SetSize(m.width, m.height)
	m.threadWeaver.SetSize(m.width, m.height)
	m.noteChat.SetSize(m.width, m.height)
	m.pomodoro.SetSize(m.width, m.height)
	m.webClipper.SetSize(m.width, m.height)
	if m.toast != nil {
		m.toast.SetWidth(m.width)
	}
	m.kanban.SetSize(m.width, m.height)
	m.encryption.SetSize(m.width, m.height)
	m.backlinkPreview.SetSize(m.width, m.height)
	m.gitHistory.SetSize(m.width, m.height)
	m.workspace.SetSize(m.width, m.height)
	m.timeline.SetSize(m.width, m.height)
	m.vaultSwitch.SetSize(m.width, m.height)
	m.frontmatterEdit.SetSize(m.width, m.height)
	m.research.SetSize(m.width, m.height)
	m.imageManager.SetSize(m.width, m.height)
	m.themeEditor.SetSize(m.width, m.height)
	m.linkAssist.SetSize(m.width, m.height)
	m.taskManager.SetSize(m.width, m.height)
	m.blogPublisher.SetSize(m.width, m.height)
	m.backup.SetSize(m.width, m.height)
	m.tutorial.SetSize(m.width, m.height)
	m.helpOverlay.SetSize(m.width, m.height)
	m.settings.SetSize(m.width, m.height)
	m.graphView.SetSize(m.width, m.height)
	m.tagBrowser.SetSize(m.width, m.height)
	m.commandPalette.SetSize(m.width, m.height)
	m.outline.SetSize(m.width, m.height)
	m.bookmarks.SetSize(m.width, m.height)
	m.findReplace.SetSize(m.width, m.height)
	m.quickSwitch.SetSize(m.width, m.height)
	m.canvas.SetSize(m.width, m.height)
	m.calendar.SetSize(m.width, m.height)
	m.bots.SetSize(m.width, m.height)
	m.focusMode.SetSize(m.width, m.height)
	m.vaultStats.SetSize(m.width, m.height)
	m.templates.SetSize(m.width, m.height)
	m.trash.SetSize(m.width, m.height)
	m.export.SetSize(m.width, m.height)
	m.git.SetSize(m.width, m.height)
	m.plugins.SetSize(m.width, m.height)
	m.contentSearch.SetSize(m.width, m.height)
	m.globalReplace.SetSize(m.width, m.height)
	m.spellcheck.SetSize(m.width, m.height)
	m.focusSession.SetSize(m.width, m.height)
	m.standupGen.SetSize(m.width, m.height)
	m.noteHistory.SetSize(m.width, m.height)
	m.smartConnect.SetSize(m.width, m.height)
	m.writingStats.SetSize(m.width, m.height)
	m.quickCapture.SetSize(m.width, m.height)
	m.dashboard.SetSize(m.width, m.height)
	m.mindMap.SetSize(m.width, m.height)
	m.journalPrompts.SetSize(m.width, m.height)
	m.clipManager.SetSize(m.width, m.height)
	m.dailyPlanner.SetSize(m.width, m.height)
	m.aiScheduler.SetSize(m.width, m.height)
	m.planMyDay.SetSize(m.width, m.height)
	m.recurringTasks.SetSize(m.width, m.height)
	m.notePreview.SetSize(m.width, m.height)
	m.scratchpad.SetSize(m.width, m.height)
	m.projectMode.SetSize(m.width, m.height)
	m.commandCenter.SetSize(m.width, m.height)
	m.nlSearch.SetSize(m.width, m.height)
	m.writingCoach.SetSize(m.width, m.height)
	m.dataview.SetSize(m.width, m.height)
	m.timeTracker.SetSize(m.width, m.height)
	m.knowledgeGaps.SetSize(m.width, m.height)
	m.aiTemplates.SetSize(m.width, m.height)
	m.languageLearning.SetSize(m.width, m.height)
	m.habitTracker.SetSize(m.width, m.height)
	m.vaultRefactor.SetSize(m.width, m.height)
	m.dailyBriefing.SetSize(m.width, m.height)
}

// refreshComponents re-scans the vault and updates all dependent components
// syncPomodoroCompletions consumes completed tasks from the pomodoro timer
// and syncs them back to their source notes (toggling checkboxes).
func (m *Model) syncPomodoroCompletions() {
	completions := m.pomodoro.GetCompletedTasks()
	if len(completions) == 0 {
		return
	}
	for _, tc := range completions {
		if tc.NotePath == "" {
			continue
		}
		if note := m.vault.GetNote(tc.NotePath); note != nil {
			lines := strings.Split(note.Content, "\n")
			if tc.LineNum >= 0 && tc.LineNum < len(lines) {
				line := lines[tc.LineNum]
				hasMarker := strings.Contains(line, "- [ ]") || strings.Contains(line, "- [x]")
				hasText := tc.Text == "" || strings.Contains(line, tc.Text)
				if !hasMarker || !hasText {
					continue
				}
				if tc.Done {
					lines[tc.LineNum] = strings.Replace(lines[tc.LineNum], "- [ ]", "- [x]", 1)
				} else {
					lines[tc.LineNum] = strings.Replace(lines[tc.LineNum], "- [x]", "- [ ]", 1)
				}
				newContent := strings.Join(lines, "\n")
				if err := os.WriteFile(filepath.Join(m.vault.Root, tc.NotePath), []byte(newContent), 0644); err != nil {
					m.statusbar.SetError("Error syncing pomodoro task: " + err.Error())
				}
			}
		}
	}
	m.refreshComponents("")
}

// loadCalendarEvents gathers native events + ICS files and sets them on the calendar.
func (m *Model) loadCalendarEvents() {
	var calEvents []CalendarEvent
	if m.eventStore != nil {
		now := time.Now()
		start := now.AddDate(0, -1, 0).Format("2006-01-02")
		end := now.AddDate(0, 3, 0).Format("2006-01-02")
		calEvents = append(calEvents, m.eventStore.ToCalendarEvents(start, end)...)
	}
	_ = filepath.Walk(m.vault.Root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), ".ics") {
			disabled := false
			for _, dc := range m.config.DisabledCalendars {
				dc = strings.TrimSpace(dc)
				if dc != "" && (strings.Contains(info.Name(), dc) || strings.Contains(path, dc)) {
					disabled = true
					break
				}
			}
			if disabled {
				return nil
			}
			if icsEvts, err := ParseICSFile(path); err == nil {
				calEvents = append(calEvents, icsEvts...)
			}
		}
		return nil
	})
	m.calendar.SetEvents(calEvents)
}

// after any file has been modified by an overlay. If changedPath is non-empty
// and matches the currently open note, the editor is reloaded too.
func (m *Model) refreshComponents(changedPath string) {
	if time.Since(m.lastRefresh) < 500*time.Millisecond {
		return // Skip redundant refresh
	}
	m.lastRefresh = time.Now()
	_ = m.vault.Scan()
	m.index.Build()
	paths := m.vault.SortedPaths()
	m.sidebar.SetFiles(paths)
	m.autocomplete.SetNotes(paths)
	m.statusbar.SetNoteCount(m.vault.NoteCount())

	// Update task cache and counts
	m.cachedTasks = ParseAllTasks(m.vault.Notes)
	m.dueTodayCount = CountTasksDueTodayFromList(m.cachedTasks)
	m.statusbar.SetDueTodayCount(m.dueTodayCount)
	m.statusbar.SetOverdueCount(CountOverdueTasksFromList(m.cachedTasks))

	// Update inbox count
	m.statusbar.SetInboxCount(countInboxItems(m.vault.Root))

	// Update calendar daily notes and note contents
	m.calendar.SetDailyNotes(paths)
	noteContents := make(map[string]string)
	for _, p := range paths {
		if note := m.vault.GetNote(p); note != nil {
			noteContents[p] = note.Content
		}
	}
	m.calendar.SetNoteContents(noteContents)

	// Load all calendar events: native events + ICS files from vault
	m.loadCalendarEvents()
	// Only refresh the calendar panel when calendar-related files change
	// to avoid unnecessary vault walks on every component refresh.
	if changedPath == "" || strings.HasSuffix(changedPath, ".ics") || strings.Contains(changedPath, "Planner") {
		m.refreshCalendarPanel()
	}

	// Directly refresh the task manager if it's currently active, so it
	// picks up changes immediately instead of waiting for the needsRefresh
	// flag to be checked on the next Update() cycle.
	if m.taskManager.IsActive() {
		m.taskManager.Refresh(m.vault)
	}

	// If the changed file is currently open in the editor, reload it
	if changedPath != "" && changedPath == m.activeNote {
		if note := m.vault.GetNote(changedPath); note != nil {
			m.editor.LoadContent(note.Content, m.editor.filePath)
		}
	}

	m.tfidfDirty = true
	m.needsRefresh = true
}

// countInboxItems reads Inbox.md and counts unchecked items.
func countInboxItems(vaultRoot string) int {
	data, err := os.ReadFile(filepath.Join(vaultRoot, "Inbox.md"))
	if err != nil {
		return 0
	}
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") && !strings.HasPrefix(trimmed, "- [x]") && !strings.HasPrefix(trimmed, "- [X]") {
			count++
		}
	}
	return count
}

func (m *Model) syncConfigToComponents() {
	m.sidebar.showIcons = m.config.ShowIcons
	m.sidebar.compactMode = m.config.CompactMode
	m.sidebar.SetShowHidden(m.config.ShowHiddenFiles)
	m.editor.showLineNumbers = m.config.LineNumbers
	m.editor.highlightCurrentLine = m.config.HighlightCurrentLine
	m.editor.autoCloseBrackets = m.config.AutoCloseBrackets
	m.editor.SetWordWrap(m.config.WordWrap)
	m.editor.tabSize = m.config.Editor.TabSize
	if m.editor.tabSize < 1 {
		m.editor.tabSize = 4
	}
	m.spellcheck.SetInlineEnabled(m.config.SpellCheck)
	// AI status indicator
	aiModel := ""
	switch m.config.AIProvider {
	case "ollama":
		aiModel = m.config.OllamaModel
	case "openai":
		aiModel = m.config.OpenAIModel
	case "nous":
		aiModel = "local"
	}
	m.statusbar.SetAIStatus(m.config.AIProvider, aiModel)
	m.autoSync.SetEnabled(m.config.GitAutoSync)
	if m.ghostWriter != nil {
		m.ghostWriter.SetEnabled(m.config.GhostWriter)
		m.ghostWriter.SetAI(m.aiConfig())
	}
	if m.autoTagger != nil {
		m.autoTagger.SetEnabled(m.config.AutoTag)
	}
	if m.autoLinkSuggest != nil {
		// Link suggestions are enabled when AI is configured (not local/empty)
		m.autoLinkSuggest.SetEnabled(m.config.AIProvider != "" && m.config.AIProvider != "local")
	}
	if m.vimState != nil {
		m.vimState.SetEnabled(m.config.VimMode)
	}
	m.pomodoro.SetGoal(m.config.PomodoroGoal)
}

func (m *Model) applyTagsToNote(tags []string) {
	content := m.editor.GetContent()
	lines := strings.Split(content, "\n")

	// Check if frontmatter exists
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		// Find the closing ---
		endIdx := -1
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				endIdx = i
				break
			}
		}

		if endIdx > 0 {
			// Look for existing tags: line in frontmatter
			tagLineIdx := -1
			for i := 1; i < endIdx; i++ {
				trimmed := strings.TrimSpace(lines[i])
				if strings.HasPrefix(trimmed, "tags:") {
					tagLineIdx = i
					break
				}
			}

			// Build the new tags line — merge with existing
			allTags := make(map[string]bool)
			if tagLineIdx >= 0 {
				// Parse existing tags
				existing := strings.TrimPrefix(strings.TrimSpace(lines[tagLineIdx]), "tags:")
				existing = strings.TrimSpace(existing)
				existing = strings.Trim(existing, "[]")
				for _, t := range strings.Split(existing, ",") {
					t = strings.TrimSpace(t)
					t = strings.Trim(t, "\"' ")
					if t != "" {
						allTags[t] = true
					}
				}
			}
			for _, t := range tags {
				allTags[t] = true
			}

			// Build sorted tag list
			var sortedTags []string
			for t := range allTags {
				sortedTags = append(sortedTags, t)
			}
			sort.Strings(sortedTags)
			newTagLine := "tags: [" + strings.Join(sortedTags, ", ") + "]"

			if tagLineIdx >= 0 {
				lines[tagLineIdx] = newTagLine
			} else {
				// Insert tags line before closing ---
				newLines := make([]string, 0, len(lines)+1)
				newLines = append(newLines, lines[:endIdx]...)
				newLines = append(newLines, newTagLine)
				newLines = append(newLines, lines[endIdx:]...)
				lines = newLines
			}

			newContent := strings.Join(lines, "\n")
			m.editor.LoadContent(newContent, m.activeNote)
			m.editor.modified = true
			// Save directly to disk
			if err := os.WriteFile(filepath.Join(m.vault.Root, m.activeNote), []byte(newContent), 0644); err != nil {
				m.statusbar.SetMessage("Failed to save tags: " + err.Error())
				return
			}
			// Re-scan vault for updated tags
			_ = m.vault.Scan()
			m.index = vault.NewIndex(m.vault)
			m.index.Build()
			return
		}
	}

	// No frontmatter — create one with tags
	var sortedTags []string
	seen := make(map[string]bool)
	for _, t := range tags {
		if !seen[t] {
			sortedTags = append(sortedTags, t)
			seen[t] = true
		}
	}
	sort.Strings(sortedTags)
	frontmatter := "---\ntags: [" + strings.Join(sortedTags, ", ") + "]\n---\n"
	newContent := frontmatter + content
	m.editor.LoadContent(newContent, m.activeNote)
	m.editor.modified = true
	if err := os.WriteFile(filepath.Join(m.vault.Root, m.activeNote), []byte(newContent), 0644); err != nil {
		m.statusbar.SetMessage("Failed to save tags: " + err.Error())
		return
	}
	_ = m.vault.Scan()
	m.index = vault.NewIndex(m.vault)
	m.index.Build()
}

func (m Model) saveCurrentNote() tea.Cmd {
	return func() tea.Msg {
		if m.activeNote == "" {
			return nil
		}
		content := m.editor.GetContent()
		path := filepath.Join(m.vault.Root, m.activeNote)
		// Atomic save: write to temp file then rename to avoid partial writes on crash
		tmpPath := path + ".tmp"
		if err := os.WriteFile(tmpPath, []byte(content), 0644); err != nil {
			_ = os.Remove(tmpPath)
			return saveResultMsg{err: err}
		}
		if err := os.Rename(tmpPath, path); err != nil {
			_ = os.Remove(tmpPath)
			return saveResultMsg{err: err}
		}
		// Incrementally update the search index for the saved file
		if m.vault.SearchIndex != nil {
			m.vault.SearchIndex.Update(m.activeNote, content)
		}
		return saveResultMsg{err: nil}
	}
}

// tryExpandSnippet checks if the word before the cursor (before the space just typed)
// matches a snippet trigger and replaces it with the expanded content.
func (m *Model) tryExpandSnippet() {
	if m.snippets == nil {
		return
	}
	if m.editor.cursor >= len(m.editor.content) {
		return
	}
	line := m.editor.content[m.editor.cursor]
	col := m.editor.col
	// col points after the space. The word is before the space.
	if col < 2 {
		return
	}
	// Find the word before the space (col-1 is the space)
	end := col - 1
	start := end
	for start > 0 && line[start-1] != ' ' && line[start-1] != '\t' {
		start--
	}
	word := line[start:end]
	if expanded, ok := m.snippets.TryExpand(word); ok {
		m.editor.saveSnapshot()
		// Remove the trigger word and the trailing space
		before := line[:start]
		after := line[col:]
		// Handle multi-line expansion
		expandedLines := strings.Split(expanded, "\n")
		if len(expandedLines) == 1 {
			m.editor.content[m.editor.cursor] = before + expandedLines[0] + after
			m.editor.col = start + len(expandedLines[0])
		} else {
			// First line
			m.editor.content[m.editor.cursor] = before + expandedLines[0]
			// Middle lines
			newContent := make([]string, 0, len(m.editor.content)+len(expandedLines)-1)
			newContent = append(newContent, m.editor.content[:m.editor.cursor+1]...)
			for i := 1; i < len(expandedLines)-1; i++ {
				newContent = append(newContent, expandedLines[i])
			}
			// Last line
			lastExp := expandedLines[len(expandedLines)-1]
			newContent = append(newContent, lastExp+after)
			newContent = append(newContent, m.editor.content[m.editor.cursor+1:]...)
			m.editor.content = newContent
			m.editor.cursor += len(expandedLines) - 1
			m.editor.col = len(lastExp)
		}
		m.editor.modified = true
		m.editor.countWords()
	}
}

// loadNoteWithoutBreadcrumb loads a note without pushing to breadcrumb history.
// Used by back/forward navigation to avoid corrupting the navigation stack.
func (m *Model) loadNoteWithoutBreadcrumb(relPath string) {
	note := m.vault.GetNote(relPath)
	if note == nil {
		return
	}
	// Save scroll position of the note we're leaving
	m.saveScrollPosition()

	m.activeNote = relPath
	m.editor.LoadContent(note.Content, relPath)
	m.statusbar.SetActiveNote(relPath)
	if m.ghostWriter != nil {
		m.ghostWriter.SetNoteTitle(relPath)
		tags := extractFrontmatterTags(note.Content)
		if len(tags) > 0 {
			m.ghostWriter.SetNoteTags(strings.Join(tags, ", "))
		} else {
			m.ghostWriter.SetNoteTags("")
		}
	}
	m.statusbar.SetWordCount(m.editor.GetWordCount())
	m.viewScroll = 0

	// Restore scroll position if we've seen this note before
	m.restoreScrollPosition(relPath)

	incoming := m.buildBacklinkItems(m.index.GetBacklinks(relPath), relPath)
	outgoing := m.buildOutgoingItems(m.index.GetOutgoingLinks(relPath))
	m.backlinks.SetLinks(incoming, outgoing)

	// Compute link suggestions from TF-IDF similarity
	if m.tfidfIndex != nil {
		existingLinks := m.index.GetOutgoingLinks(relPath)
		suggestions := SuggestMissingLinks(m.tfidfIndex, relPath, existingLinks)
		m.backlinks.SetSuggestions(suggestions)
	}

	// Refresh calendar panel if active
	if m.rightPanelCalendar || LayoutHasCalendarPanel(m.config.Layout) {
		m.refreshCalendarPanel()
	}

	m.bookmarks.AddRecent(relPath)
}

// refreshCalendarPanel reloads planner blocks, ICS events, and tasks into the calendar panel.
func (m *Model) refreshCalendarPanel() {
	m.calendarPanel.SetVaultRoot(m.vault.Root)
	noteContents := make(map[string]string)
	for _, p := range m.vault.SortedPaths() {
		if note := m.vault.GetNote(p); note != nil {
			noteContents[p] = note.Content
		}
	}
	helperBlocks, _ := loadPlannerBlocks(m.vault.Root)
	m.calendarPanel.Refresh(helperBlocks, noteContents)
	// Feed ICS events into the panel so cockpit layout shows calendar events
	m.calendarPanel.SetEvents(m.calendar.GetEvents())
}

func (m *Model) applyVimResult(r VimResult) tea.Cmd {
	// Handle text object operations (inline edit within/across lines)
	if r.TextOp != "" {
		m.editor.saveSnapshot()
		m.applyTextOp(r.TextOp, r.TextOpStartLine, r.TextOpStartCol, r.TextOpEndLine, r.TextOpEndCol)
	}

	if r.CursorSet {
		m.editor.cursor = r.NewCursor
		m.editor.col = r.NewCol
		// Clamp
		if m.editor.cursor < 0 {
			m.editor.cursor = 0
		}
		if len(m.editor.content) == 0 {
			m.editor.content = []string{""}
		}
		if m.editor.cursor >= len(m.editor.content) {
			m.editor.cursor = len(m.editor.content) - 1
		}
		if m.editor.col < 0 {
			m.editor.col = 0
		}
		if m.editor.cursor >= 0 && m.editor.cursor < len(m.editor.content) {
			if m.editor.col > len(m.editor.content[m.editor.cursor]) {
				m.editor.col = len(m.editor.content[m.editor.cursor])
			}
		}
	}
	if r.ScrollSet {
		m.editor.scroll = r.ScrollTo
	}
	if r.DeleteLine {
		if len(m.editor.content) > 1 && m.editor.cursor < len(m.editor.content) {
			m.editor.saveSnapshot()
			m.vimState.register = m.editor.content[m.editor.cursor]
			m.editor.content = append(m.editor.content[:m.editor.cursor], m.editor.content[m.editor.cursor+1:]...)
			if len(m.editor.content) == 0 {
				m.editor.content = []string{""}
			}
			if m.editor.cursor >= len(m.editor.content) {
				m.editor.cursor = len(m.editor.content) - 1
			}
			m.editor.col = 0
			m.editor.modified = true
		}
	}
	if r.JoinLine {
		if m.editor.cursor < len(m.editor.content)-1 {
			m.editor.saveSnapshot()
			m.editor.content[m.editor.cursor] += " " + strings.TrimSpace(m.editor.content[m.editor.cursor+1])
			m.editor.content = append(m.editor.content[:m.editor.cursor+1], m.editor.content[m.editor.cursor+2:]...)
			m.editor.modified = true
		}
	}
	if r.PasteBelow && m.vimState.register != "" {
		m.editor.saveSnapshot()
		newContent := make([]string, 0, len(m.editor.content)+1)
		newContent = append(newContent, m.editor.content[:m.editor.cursor+1]...)
		newContent = append(newContent, m.vimState.register)
		newContent = append(newContent, m.editor.content[m.editor.cursor+1:]...)
		m.editor.content = newContent
		m.editor.cursor++
		m.editor.col = 0
		m.editor.modified = true
	}
	if r.PasteAbove && m.vimState.register != "" {
		m.editor.saveSnapshot()
		newContent := make([]string, 0, len(m.editor.content)+1)
		newContent = append(newContent, m.editor.content[:m.editor.cursor]...)
		newContent = append(newContent, m.vimState.register)
		newContent = append(newContent, m.editor.content[m.editor.cursor:]...)
		m.editor.content = newContent
		m.editor.col = 0
		m.editor.modified = true
	}
	if r.Undo {
		m.editor.Undo()
	}
	if r.Redo {
		m.editor.Redo()
	}
	if r.InsertLine != "" {
		m.editor.saveSnapshot()
		newContent := make([]string, 0, len(m.editor.content)+1)
		newContent = append(newContent, m.editor.content[:m.editor.cursor+1]...)
		newContent = append(newContent, r.InsertLine)
		newContent = append(newContent, m.editor.content[m.editor.cursor+1:]...)
		m.editor.content = newContent
		m.editor.cursor++
		m.editor.col = 0
		m.editor.modified = true
	}
	if r.StatusMsg != "" {
		switch r.StatusMsg {
		case "save":
			return m.saveCurrentNote()
		case "quit":
			return m.triggerExitSplash()
		case "save_quit":
			m.saveCurrentNote()()
			return m.triggerExitSplash()
		default:
			m.statusbar.SetMessage(r.StatusMsg)
		}
	}
	if r.EnterInsert {
		mode := "VIM:INSERT"
		if rs := m.vimState.RecordingStatus(); rs != "" {
			mode += " [" + rs + "]"
		}
		m.statusbar.SetMode(mode)
	}
	if r.EnterNormal {
		mode := "VIM:NORMAL"
		if rs := m.vimState.RecordingStatus(); rs != "" {
			mode += " [" + rs + "]"
		}
		m.statusbar.SetMode(mode)
	}
	if r.EnterVisual {
		mode := "VIM:VISUAL"
		if rs := m.vimState.RecordingStatus(); rs != "" {
			mode += " [" + rs + "]"
		}
		m.statusbar.SetMode(mode)
	}
	if r.EnterCommand {
		m.statusbar.SetMode("VIM:COMMAND")
	}
	if r.FoldToggle {
		m.foldState.ToggleFold(m.editor.cursor, m.editor.content)
	}
	if r.FoldAll {
		m.foldState.FoldAll(m.editor.content)
	}
	if r.UnfoldAll {
		m.foldState.UnfoldAll()
	}

	// Macro recording start — update status bar
	if r.MacroStart != 0 {
		mode := "VIM:" + m.vimState.ModeString() + " [" + m.vimState.RecordingStatus() + "]"
		m.statusbar.SetMode(mode)
	}

	// Macro recording stop — update status bar
	if r.MacroStop {
		m.statusbar.SetMode("VIM:" + m.vimState.ModeString())
	}

	// Macro replay
	if r.MacroReplay != 0 {
		if !m.vimState.IsPlayingMacro() {
			keys := m.vimState.GetMacro(r.MacroReplay)
			if len(keys) > 0 {
				m.vimState.SetLastMacroRegister(r.MacroReplay)
				m.vimState.SetPlayingMacro(true)
				return func() tea.Msg {
					return vimMacroReplayMsg{keys: keys, idx: 0}
				}
			}
			m.statusbar.SetMessage("macro @" + string(r.MacroReplay) + " is empty")
		}
	}

	// Text object operations (inline region delete/change/yank)
	if r.TextOp != "" {
		m.editor.saveSnapshot()
		m.applyTextOp(r.TextOp, r.TextOpStartLine, r.TextOpStartCol, r.TextOpEndLine, r.TextOpEndCol)
	}

	// Ex command: :q! — force quit without saving
	if r.ExForceQuit {
		m.editor.modified = false // discard changes
		return m.triggerExitSplash()
	}

	// Ex command: :e <file> — open file by fuzzy match
	if r.ExOpenFile != "" {
		query := strings.ToLower(r.ExOpenFile)
		paths := m.vault.SortedPaths()
		// First try exact match, then fuzzy match
		bestMatch := ""
		for _, p := range paths {
			if strings.EqualFold(p, r.ExOpenFile) || strings.EqualFold(p, r.ExOpenFile+".md") {
				bestMatch = p
				break
			}
		}
		if bestMatch == "" {
			for _, p := range paths {
				if fuzzyMatch(strings.ToLower(p), query) {
					bestMatch = p
					break
				}
			}
		}
		if bestMatch != "" {
			m.loadNote(bestMatch)
			m.statusbar.SetMessage("opened " + bestMatch)
		} else {
			m.statusbar.SetMessage("file not found: " + r.ExOpenFile)
		}
	}

	// Ex command: :set option — toggle editor options
	if r.ExSetOption != "" {
		switch r.ExSetOption {
		case "number":
			m.editor.showLineNumbers = true
			m.config.LineNumbers = true
			m.statusbar.SetMessage("line numbers on")
		case "nonumber":
			m.editor.showLineNumbers = false
			m.config.LineNumbers = false
			m.statusbar.SetMessage("line numbers off")
		case "wrap":
			m.editor.SetWordWrap(true)
			m.config.WordWrap = true
			m.statusbar.SetMessage("word wrap on")
		case "nowrap":
			m.editor.SetWordWrap(false)
			m.config.WordWrap = false
			m.statusbar.SetMessage("word wrap off")
		}
	}

	// Ex command: :noh — clear search highlights
	if r.ExClearSearch {
		m.editor.ClearSearchHighlights()
	}

	// Ex command: :s substitution
	if r.ExSubstitute != nil {
		m.editor.saveSnapshot()
		m.editor.content = r.ExSubstitute.NewLines
		m.editor.modified = true
	}

	// Sync search highlights from vim state to editor for rendering
	if m.vimState != nil {
		if m.vimState.IsSearchActive() {
			m.editor.SetSearchHighlights(m.vimState.GetSearchMatches(), m.vimState.GetCurrentMatchIndex())
		} else {
			m.editor.ClearSearchHighlights()
		}
	}

	return nil
}

// applyTextOp performs an inline text operation (delete/change region within the editor).
// endCol is exclusive.
func (m *Model) applyTextOp(op string, startLine, startCol, endLine, endCol int) {
	if startLine >= len(m.editor.content) || endLine >= len(m.editor.content) {
		return
	}
	if startLine == endLine {
		// Single-line operation
		runes := []rune(m.editor.content[startLine])
		sc := startCol
		ec := endCol
		if sc > len(runes) {
			sc = len(runes)
		}
		if ec > len(runes) {
			ec = len(runes)
		}
		if sc > ec {
			return
		}
		m.editor.content[startLine] = string(runes[:sc]) + string(runes[ec:])
	} else {
		// Multi-line operation
		startRunes := []rune(m.editor.content[startLine])
		endRunes := []rune(m.editor.content[endLine])

		sc := startCol
		if sc > len(startRunes) {
			sc = len(startRunes)
		}
		ec := endCol
		if ec > len(endRunes) {
			ec = len(endRunes)
		}

		// Combine: keep before startCol on startLine + after endCol on endLine
		newLine := string(startRunes[:sc]) + string(endRunes[ec:])

		// Remove lines from startLine to endLine and replace with newLine
		newContent := make([]string, 0, len(m.editor.content)-(endLine-startLine))
		newContent = append(newContent, m.editor.content[:startLine]...)
		newContent = append(newContent, newLine)
		if endLine+1 < len(m.editor.content) {
			newContent = append(newContent, m.editor.content[endLine+1:]...)
		}
		m.editor.content = newContent
	}

	m.editor.modified = true
	m.editor.countWords()
}

func (m *Model) getAIModel() string {
	switch m.config.AIProvider {
	case "ollama":
		return m.config.OllamaModel
	case "openai":
		return m.config.OpenAIModel
	case "nous":
		return "nous"
	default:
		return m.config.OllamaModel
	}
}

func (m Model) aiConfig() AIConfig {
	return AIConfig{
		Provider:      m.config.AIProvider,
		Model:         m.getAIModel(),
		OllamaURL:     m.config.OllamaURL,
		APIKey:        m.config.OpenAIKey,
		NousURL:       m.config.NousURL,
		NousAPIKey:    m.config.NousAPIKey,
		NerveBinary:   m.config.NerveBinary,
		NerveModel:    m.config.NerveModel,
		NerveProvider: m.config.NerveProvider,
	}
}

func (m *Model) buildBacklinkItems(paths []string, targetNote string) []BacklinkItem {
	var items []BacklinkItem
	baseName := strings.TrimSuffix(filepath.Base(targetNote), ".md")
	for _, p := range paths {
		note := m.vault.GetNote(p)
		ctx := ""
		lineNum := 0
		if note != nil {
			lines := strings.Split(note.Content, "\n")
			for i, line := range lines {
				if strings.Contains(line, "[["+baseName+"]]") || strings.Contains(line, "[["+targetNote+"]]") {
					ctx = strings.TrimSpace(line)
					lineNum = i
					break
				}
			}
		}
		items = append(items, BacklinkItem{Path: p, Context: ctx, LineNum: lineNum})
	}
	return items
}

func (m *Model) buildOutgoingItems(paths []string) []BacklinkItem {
	var items []BacklinkItem
	for _, p := range paths {
		items = append(items, BacklinkItem{Path: p})
	}
	return items
}

// startSemanticBgIndex configures the semantic search and kicks off a
// background tea.Cmd to build or update the embedding index incrementally.
func (m *Model) startSemanticBgIndex() tea.Cmd {
	if m.semanticSearch.IsBgIndexing() {
		return nil
	}
	m.semanticSearch.SetVaultPath(m.vault.Root)
	cfg := m.aiConfig()
	m.semanticSearch.SetConfig(cfg)
	noteContents := make(map[string]string)
	for _, p := range m.vault.SortedPaths() {
		if note := m.vault.GetNote(p); note != nil {
			noteContents[p] = note.Content
		}
	}
	return m.semanticSearch.StartBackgroundIndex(noteContents)
}

func (m *Model) handlePluginOutput(pluginName, output string) {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "MSG:"):
			m.statusbar.SetMessage("[" + pluginName + "] " + strings.TrimPrefix(line, "MSG:"))
		case strings.HasPrefix(line, "CONTENT:"):
			// base64-encoded replacement content
			encoded := strings.TrimPrefix(line, "CONTENT:")
			if decoded, err := base64.StdEncoding.DecodeString(encoded); err == nil {
				m.editor.SetContent(string(decoded))
			}
		case strings.HasPrefix(line, "INSERT:"):
			encoded := strings.TrimPrefix(line, "INSERT:")
			if decoded, err := base64.StdEncoding.DecodeString(encoded); err == nil {
				m.editor.InsertText(string(decoded))
			}
		default:
			m.statusbar.SetMessage("[" + pluginName + "] " + line)
		}
	}
}

func (m *Model) runPluginSaveHooks() tea.Cmd {
	enabled := m.plugins.EnabledPlugins()
	if len(enabled) == 0 {
		return nil
	}
	notePath := filepath.Join(m.vault.Root, m.activeNote)
	return RunPluginHook(enabled, "on_save", notePath, m.editor.GetContent(), m.vault.Root)
}

func (m *Model) triggerExitSplash() tea.Cmd {
	// Save unsaved changes before exiting to prevent data loss
	if m.editor.modified && m.activeNote != "" {
		content := m.editor.GetContent()
		path := filepath.Join(m.vault.Root, m.activeNote)
		tmpPath := path + ".tmp"
		if err := os.WriteFile(tmpPath, []byte(content), 0644); err == nil {
			if err := os.Rename(tmpPath, path); err == nil {
				m.editor.modified = false
			} else {
				_ = os.Remove(tmpPath)
			}
		} else {
			_ = os.Remove(tmpPath)
		}
	}
	// Stop file watcher to avoid leaking goroutines/descriptors
	if m.fileWatcher != nil {
		m.fileWatcher.Stop()
	}
	// Save open tabs for session persistence
	if m.tabBar != nil {
		m.tabBar.SaveTabs(m.vault.Root)
	}
	// Save explorer (folder collapse) state for session persistence
	m.sidebar.SaveExplorerState(m.vault.Root)
	// Save scroll positions for session persistence
	m.saveScrollCache(m.vault.Root)
	// Auto-commit on exit if git sync is enabled
	if m.config.GitAutoSync && m.autoSync.isGitRepo() {
		gitIn := func(args ...string) (string, error) {
			fullArgs := append([]string{"-C", m.vault.Root}, args...)
			cmd := exec.Command("git", fullArgs...)
			cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
			out, err := cmd.CombinedOutput()
			return string(out), err
		}
		status, err := gitIn("status", "--porcelain")
		if err == nil && strings.TrimSpace(status) != "" {
			for _, line := range strings.Split(strings.TrimSpace(status), "\n") {
				if len(line) < 3 {
					continue
				}
				code := line[:2] // don't trim — spaces are meaningful in porcelain status
				file := strings.TrimSpace(line[3:])
				if file == "" {
					continue
				}
				// Handle renames: porcelain format is "R  old -> new"
				if strings.HasPrefix(code, "R") {
					if idx := strings.Index(file, " -> "); idx >= 0 {
						file = file[idx+4:]
					}
				}
				if _, err := gitIn("add", file); err != nil {
					continue
				}
				trimCode := strings.TrimSpace(code)
				var msg string
				switch {
				case trimCode == "??" || strings.Contains(code, "A"):
					msg = "vault: add " + file
				case strings.Contains(code, "D"):
					msg = "vault: remove " + file
				case strings.HasPrefix(code, "R"):
					msg = "vault: rename " + file
				default:
					msg = "vault: update " + file
				}
				_, _ = gitIn("commit", "-m", msg)
			}
			_, _ = gitIn("push", "--quiet")
		}
	}
	// Unload Ollama model to free resources
	if m.config.AIProvider == "ollama" {
		stopOllama(m.config.OllamaModel)
	}
	OllamaStopServer()
	m.showExitSplash = true
	m.exitSplash = NewExitSplash(m.vault.NoteCount(), time.Since(m.sessionStart))
	m.exitSplash.width = m.width
	m.exitSplash.height = m.height
	return m.exitSplash.Init()
}

func (m Model) clearMessageAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return clearMessageMsg{}
	})
}

// checkDayPlanned looks at today's daily note and marks the day as planned
// if it already contains a planning section (e.g. from a previous morning
// routine or plan-my-day run).
func (m *Model) checkDayPlanned() {
	today := time.Now().Format("2006-01-02")
	for _, note := range m.vault.SortedPaths() {
		if !strings.Contains(note, today) {
			continue
		}
		n := m.vault.GetNote(note)
		if n == nil {
			continue
		}
		content := n.Content
		if strings.Contains(content, "## Schedule") ||
			strings.Contains(content, "## Today's Focus") ||
			strings.Contains(content, "## Morning Briefing") ||
			strings.Contains(content, "## Active Threads") {
			m.statusbar.SetDayPlanned(true)
			return
		}
	}
}
