package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/vault"
)
func (m Model) View() string {
	// Splash screen
	if m.showSplash {
		return m.splash.View()
	}

	// Exit splash screen
	if m.showExitSplash {
		return m.exitSplash.View()
	}

	if m.quitting {
		return ""
	}

	if m.width == 0 {
		return lipgloss.NewStyle().Foreground(mauve).Render("\n  Loading Granit...")
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
	layout := m.config.Layout
	if layout == "" {
		layout = "default"
	}

	// Calculate widths based on layout
	showSidebar := LayoutHasSidebar(layout)
	showBacklinks := LayoutHasBacklinks(layout)
	showOutline := LayoutHasOutline(layout)
	showCalPanel := LayoutHasCalendarPanel(layout)

	// Toggle: when rightPanelCalendar is set, swap backlinks for calendar panel
	if m.rightPanelCalendar && showBacklinks {
		showBacklinks = false
		showCalPanel = true
	}

	sidebarWidth := 0
	backlinksWidth := 0
	outlineWidth := 0
	calPanelWidth := 0

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
	if showCalPanel {
		calPanelWidth = m.width / 4
		if calPanelWidth < 28 {
			calPanelWidth = 28
		}
		if calPanelWidth > 35 {
			calPanelWidth = 35
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
	if showCalPanel {
		panelBorders += 2
	}
	if showOutline {
		panelBorders += 2
	}

	// Ensure panel widths + minimum editor (30) + borders fit in terminal width.
	minEditorWidth := 30
	totalPanels := sidebarWidth + backlinksWidth + calPanelWidth + outlineWidth
	available := m.width - panelBorders - 2
	if totalPanels+minEditorWidth > available {
		budget := available - minEditorWidth
		if budget < 0 {
			budget = 0
		}
		if totalPanels > 0 && budget > 0 {
			ratio := float64(budget) / float64(totalPanels)
			sidebarWidth = int(float64(sidebarWidth) * ratio)
			backlinksWidth = int(float64(backlinksWidth) * ratio)
			calPanelWidth = int(float64(calPanelWidth) * ratio)
			outlineWidth = int(float64(outlineWidth) * ratio)
			remainder := budget - sidebarWidth - backlinksWidth - calPanelWidth - outlineWidth
			sidebarWidth += remainder
		} else {
			sidebarWidth = 0
			backlinksWidth = 0
			calPanelWidth = 0
			outlineWidth = 0
		}
		if outlineWidth > 0 && outlineWidth < 10 {
			outlineWidth = 0
			panelBorders -= 2
		}
		if backlinksWidth > 0 && backlinksWidth < 10 {
			backlinksWidth = 0
			panelBorders -= 2
		}
		if calPanelWidth > 0 && calPanelWidth < 10 {
			calPanelWidth = 0
			panelBorders -= 2
		}
		if sidebarWidth > 0 && sidebarWidth < 10 {
			sidebarWidth = 0
			panelBorders -= 2
		}
	}

	editorWidth := m.width - sidebarWidth - backlinksWidth - calPanelWidth - outlineWidth - panelBorders - 2
	if editorWidth < 30 {
		editorWidth = 30
	}

	// Focus-aware borders
	sidebarBorderColor := surface1
	editorBorderColor := surface1
	backlinksBorderColor := surface1

	switch m.focus {
	case focusSidebar:
		sidebarBorderColor = FocusedBorderColor
	case focusEditor:
		editorBorderColor = FocusedBorderColor
	case focusBacklinks:
		backlinksBorderColor = FocusedBorderColor
	}

	// Tab bar
	var tabBarStr string
	if m.tabBar != nil && len(m.tabBar.Tabs()) > 0 {
		m.tabBar.SetModified(m.activeNote, m.editor.modified)
		tabBarStr = m.tabBar.Render(editorWidth, m.activeNote)
	}

	// Editor: view mode or edit mode
	var editorContent string
	if m.viewMode {
		editorContent = m.renderViewMode()
	} else {
		editorContent = m.editor.View()
	}

	// Folder-path breadcrumb (between tab bar and editor)
	breadcrumbStr := renderBreadcrumb(m.activeNote, editorWidth)

	// Zen layout hides tab bar and breadcrumb for distraction-free writing
	if layout == "zen" {
		tabBarStr = ""
		breadcrumbStr = ""
	}

	// Combine tab bar + breadcrumb + editor
	editorPanel := editorContent
	if tabBarStr != "" && breadcrumbStr != "" {
		editorPanel = tabBarStr + "\n" + breadcrumbStr + "\n" + editorContent
	} else if tabBarStr != "" {
		editorPanel = tabBarStr + "\n" + editorContent
	} else if breadcrumbStr != "" {
		editorPanel = breadcrumbStr + "\n" + editorContent
	}

	editor := EditorStyle.
		BorderForeground(editorBorderColor).
		Width(editorWidth).
		Height(contentHeight).
		Render(editorPanel)

	var view string
	if m.focusMode.IsActive() {
		focusView := m.focusMode.RenderEditor(editorContent, m.editor.GetWordCount())
		view = focusView
	} else {
		var content string
		switch layout {
		case "minimal":
			content = editor
		case "writer":
			sidebar := SidebarStyle.
				BorderForeground(sidebarBorderColor).
				Width(sidebarWidth).
				Height(contentHeight).
				Render(m.sidebar.View())
			if m.config.SidebarPosition == "right" {
				content = lipgloss.JoinHorizontal(lipgloss.Top, editor, sidebar)
			} else {
				content = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, editor)
			}
		case "reading":
			if showCalPanel {
				m.calendarPanel.SetSize(calPanelWidth, contentHeight)
				calPanel := lipgloss.NewStyle().
					BorderStyle(PanelBorder).
					BorderForeground(backlinksBorderColor).
					Width(calPanelWidth).
					Height(contentHeight).
					Render(m.calendarPanel.View())
				content = lipgloss.JoinHorizontal(lipgloss.Top, editor, calPanel)
			} else {
				backlinks := BacklinksStyle.
					BorderForeground(backlinksBorderColor).
					Width(backlinksWidth).
					Height(contentHeight).
					Render(m.backlinks.View())
				content = lipgloss.JoinHorizontal(lipgloss.Top, editor, backlinks)
			}
		case "zen":
			// Centered editor with constrained width, no borders
			maxContentWidth := 82
			if editorWidth > maxContentWidth {
				editorWidth = maxContentWidth
			}
			// Re-render editor panel with constrained width, no border
			zenEditor := lipgloss.NewStyle().
				Width(editorWidth).
				Height(contentHeight).
				Background(base).
				Padding(0, 1).
				Render(editorPanel)
			// Center it horizontally
			leftPad := (m.width - editorWidth - 2) / 2
			if leftPad < 0 {
				leftPad = 0
			}
			content = lipgloss.NewStyle().
				PaddingLeft(leftPad).
				Height(contentHeight + 2).
				Render(zenEditor)
		case "dashboard": // 4-panel: sidebar | editor | outline | backlinks
			var leftPanels, rightPanels []string
			if sidebarWidth > 0 {
				sidebar := SidebarStyle.
					BorderForeground(sidebarBorderColor).
					Width(sidebarWidth).
					Height(contentHeight).
					Render(m.sidebar.View())
				if m.config.SidebarPosition == "right" {
					rightPanels = append(rightPanels, sidebar)
				} else {
					leftPanels = append(leftPanels, sidebar)
				}
			}
			if outlineWidth > 0 {
				outlinePanelContent := m.outline.RenderPanel(m.editor.GetContent(), outlineWidth, contentHeight)
				outlinePanel := lipgloss.NewStyle().
					BorderStyle(PanelBorder).
					BorderForeground(surface1).
					Width(outlineWidth).
					Height(contentHeight).
					Render(outlinePanelContent)
				rightPanels = append(rightPanels, outlinePanel)
			}
			if backlinksWidth > 0 {
				backlinks := BacklinksStyle.
					BorderForeground(backlinksBorderColor).
					Width(backlinksWidth).
					Height(contentHeight).
					Render(m.backlinks.View())
				rightPanels = append(rightPanels, backlinks)
			}
			panels := append(leftPanels, editor)
			panels = append(panels, rightPanels...)
			content = lipgloss.JoinHorizontal(lipgloss.Top, panels...)
		case "taskboard":
			sidebar := SidebarStyle.
				BorderForeground(sidebarBorderColor).
				Width(sidebarWidth).
				Height(contentHeight).
				Render(m.sidebar.View())

			// Task summary panel
			taskPanelWidth := m.width / 4
			if taskPanelWidth < 25 {
				taskPanelWidth = 25
			}
			if taskPanelWidth > 40 {
				taskPanelWidth = 40
			}

			var taskContent strings.Builder
			taskContent.WriteString(lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  TASKS"))
			taskContent.WriteString("\n")
			taskContent.WriteString(DimStyle.Render(strings.Repeat("\u2500", taskPanelWidth-4)))
			taskContent.WriteString("\n\n")

			// Read Tasks.md
			today := time.Now().Format("2006-01-02")
			tasksPath := filepath.Join(m.vault.Root, "Tasks.md")
			taskLines := []string{}
			if data, err := os.ReadFile(tasksPath); err == nil {
				taskLines = strings.Split(string(data), "\n")
			}

			// Show overdue and today's tasks
			overdueCount := 0
			todayCount := 0
			upcomingCount := 0

			taskContent.WriteString(lipgloss.NewStyle().Foreground(red).Bold(true).Render("  Overdue") + "\n")
			for _, line := range taskLines {
				trimmed := strings.TrimSpace(line)
				if !strings.HasPrefix(trimmed, "- [ ]") {
					continue
				}
				if idx := strings.Index(trimmed, "\U0001f4c5 "); idx >= 0 {
					dateStr := trimmed[idx+len("\U0001f4c5 "):]
					if len(dateStr) >= 10 {
						dueDate := dateStr[:10]
						taskText := strings.TrimSpace(trimmed[5:])
						if eIdx := strings.Index(taskText, " \U0001f4c5"); eIdx >= 0 {
							taskText = taskText[:eIdx]
						}
						taskText = TruncateDisplay(taskText, taskPanelWidth-8)
						if dueDate < today {
							overdueCount++
							taskContent.WriteString("  " + lipgloss.NewStyle().Foreground(red).Render("\u2717 "+taskText) + "\n")
						} else if dueDate == today {
							todayCount++
						} else {
							upcomingCount++
						}
					}
				}
			}
			if overdueCount == 0 {
				taskContent.WriteString("  " + DimStyle.Render("none") + "\n")
			}

			taskContent.WriteString("\n" + lipgloss.NewStyle().Foreground(yellow).Bold(true).Render("  Today") + "\n")
			for _, line := range taskLines {
				trimmed := strings.TrimSpace(line)
				if !strings.HasPrefix(trimmed, "- [ ]") {
					continue
				}
				if idx := strings.Index(trimmed, "\U0001f4c5 "); idx >= 0 {
					dateStr := trimmed[idx+len("\U0001f4c5 "):]
					if len(dateStr) >= 10 {
						dueDate := dateStr[:10]
						taskText := strings.TrimSpace(trimmed[5:])
						if eIdx := strings.Index(taskText, " \U0001f4c5"); eIdx >= 0 {
							taskText = taskText[:eIdx]
						}
						taskText = TruncateDisplay(taskText, taskPanelWidth-8)
						if dueDate == today {
							taskContent.WriteString("  " + lipgloss.NewStyle().Foreground(yellow).Render("\u25cb "+taskText) + "\n")
						}
					}
				}
			}
			if todayCount == 0 {
				taskContent.WriteString("  " + DimStyle.Render("none") + "\n")
			}

			// Stats
			taskContent.WriteString("\n" + DimStyle.Render(strings.Repeat("\u2500", taskPanelWidth-4)) + "\n")
			taskContent.WriteString(fmt.Sprintf("  %s %d overdue  %s %d today  %s %d upcoming\n",
				lipgloss.NewStyle().Foreground(red).Render("\u25cf"), overdueCount,
				lipgloss.NewStyle().Foreground(yellow).Render("\u25cf"), todayCount,
				lipgloss.NewStyle().Foreground(green).Render("\u25cf"), upcomingCount))

			taskPanel := lipgloss.NewStyle().
				BorderStyle(PanelBorder).
				BorderForeground(surface1).
				Width(taskPanelWidth).
				Height(contentHeight).
				Background(base).
				Padding(0, 1).
				Render(taskContent.String())

			// Adjust editor width
			tbEditorWidth := m.width - sidebarWidth - taskPanelWidth - 6
			if tbEditorWidth < 30 {
				tbEditorWidth = 30
			}
			tbEditor := EditorStyle.
				BorderForeground(editorBorderColor).
				Width(tbEditorWidth).
				Height(contentHeight).
				Render(editorPanel)

			content = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, tbEditor, taskPanel)
		case "research":
			sidebar := SidebarStyle.
				BorderForeground(sidebarBorderColor).
				Width(sidebarWidth).
				Height(contentHeight).
				Render(m.sidebar.View())

			// Research/notes panel
			notesPanelWidth := m.width / 4
			if notesPanelWidth < 25 {
				notesPanelWidth = 25
			}
			if notesPanelWidth > 40 {
				notesPanelWidth = 40
			}

			var notesContent strings.Builder
			notesContent.WriteString(lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  RESEARCH"))
			notesContent.WriteString("\n")
			notesContent.WriteString(DimStyle.Render(strings.Repeat("\u2500", notesPanelWidth-4)))
			notesContent.WriteString("\n\n")

			// Recent notes
			notesContent.WriteString(lipgloss.NewStyle().Foreground(blue).Bold(true).Render("  Recent Notes") + "\n")
			paths := m.vault.SortedPaths()
			type recentNote struct {
				path  string
				words int
			}
			var recents []recentNote
			for _, p := range paths {
				note := m.vault.GetNote(p)
				if note == nil {
					continue
				}
				words := len(strings.Fields(note.Content))
				recents = append(recents, recentNote{path: p, words: words})
			}
			// Show up to 20 recent notes
			shown := 0
			for i := len(recents) - 1; i >= 0 && shown < 20; i-- {
				r := recents[i]
				name := filepath.Base(r.path)
				name = strings.TrimSuffix(name, ".md")
				name = TruncateDisplay(name, notesPanelWidth-12)
				style := lipgloss.NewStyle().Foreground(text)
				if r.path == m.activeNote {
					style = style.Foreground(mauve).Bold(true)
				}
				wordStr := DimStyle.Render(fmt.Sprintf(" %dw", r.words))
				notesContent.WriteString("  " + style.Render("\u00b7 "+name) + wordStr + "\n")
				shown++
			}

			// Backlinks of current note
			notesContent.WriteString("\n" + lipgloss.NewStyle().Foreground(green).Bold(true).Render("  Backlinks") + "\n")
			if m.activeNote != "" {
				backlinks := m.index.GetBacklinks(m.activeNote)
				if len(backlinks) == 0 {
					notesContent.WriteString("  " + DimStyle.Render("none") + "\n")
				}
				for _, bl := range backlinks {
					name := filepath.Base(bl)
					name = strings.TrimSuffix(name, ".md")
					if len(name) > notesPanelWidth-8 {
						name = name[:notesPanelWidth-11] + "..."
					}
					notesContent.WriteString("  " + lipgloss.NewStyle().Foreground(green).Render("\u2190 "+name) + "\n")
				}
			}

			// Outgoing links
			notesContent.WriteString("\n" + lipgloss.NewStyle().Foreground(blue).Bold(true).Render("  Links") + "\n")
			if m.activeNote != "" {
				note := m.vault.GetNote(m.activeNote)
				if note != nil && len(note.Links) > 0 {
					for _, link := range note.Links {
						link = TruncateDisplay(link, notesPanelWidth-8)
						notesContent.WriteString("  " + lipgloss.NewStyle().Foreground(blue).Render("\u2192 "+link) + "\n")
					}
				} else {
					notesContent.WriteString("  " + DimStyle.Render("none") + "\n")
				}
			}

			notesPanel := lipgloss.NewStyle().
				BorderStyle(PanelBorder).
				BorderForeground(surface1).
				Width(notesPanelWidth).
				Height(contentHeight).
				Background(base).
				Padding(0, 1).
				Render(notesContent.String())

			// Adjust editor width
			rsEditorWidth := m.width - sidebarWidth - notesPanelWidth - 6
			if rsEditorWidth < 30 {
				rsEditorWidth = 30
			}
			rsEditor := EditorStyle.
				BorderForeground(editorBorderColor).
				Width(rsEditorWidth).
				Height(contentHeight).
				Render(editorPanel)

			content = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, rsEditor, notesPanel)
		default: // "default" - 3-panel (with optional calendar toggle)
			sidebar := SidebarStyle.
				BorderForeground(sidebarBorderColor).
				Width(sidebarWidth).
				Height(contentHeight).
				Render(m.sidebar.View())
			if showCalPanel {
				m.calendarPanel.SetSize(calPanelWidth, contentHeight)
				calPanel := lipgloss.NewStyle().
					BorderStyle(PanelBorder).
					BorderForeground(backlinksBorderColor).
					Width(calPanelWidth).
					Height(contentHeight).
					Render(m.calendarPanel.View())
				if m.config.SidebarPosition == "right" {
					content = lipgloss.JoinHorizontal(lipgloss.Top, calPanel, editor, sidebar)
				} else {
					content = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, editor, calPanel)
				}
			} else {
				backlinks := BacklinksStyle.
					BorderForeground(backlinksBorderColor).
					Width(backlinksWidth).
					Height(contentHeight).
					Render(m.backlinks.View())
				if m.config.SidebarPosition == "right" {
					content = lipgloss.JoinHorizontal(lipgloss.Top, backlinks, editor, sidebar)
				} else {
					content = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, editor, backlinks)
				}
			}
		}
		// Breadcrumb bar (between content and status)
		var breadcrumbBar string
		if m.breadcrumb != nil && (len(m.breadcrumb.Pinned()) > 0 || m.breadcrumb.CanGoBack()) {
			breadcrumbBar = m.breadcrumb.RenderBar(m.width, m.activeNote)
		}
		// Pomodoro and clock-in status indicators in status bar
		m.statusbar.SetPomodoroStatus(m.pomodoro.StatusString())
		m.statusbar.SetClockInStatus(m.clockIn.StatusString())
		status := m.statusbar.View()
		if breadcrumbBar != "" {
			view = lipgloss.JoinVertical(lipgloss.Left, content, breadcrumbBar, status)
		} else {
			view = lipgloss.JoinVertical(lipgloss.Left, content, status)
		}
	}

	// Safety: truncate content to terminal height, preserving status bar at bottom.
	// The status bar is the last 2 lines; if the view overflows, trim the content
	// area instead of chopping off the status bar.
	if viewLines := strings.Split(view, "\n"); len(viewLines) > m.height {
		statusLines := 2
		if len(viewLines) >= statusLines {
			contentLines := viewLines[:len(viewLines)-statusLines]
			statusPart := viewLines[len(viewLines)-statusLines:]
			maxContent := m.height - statusLines
			if maxContent < 0 {
				maxContent = 0
			}
			if len(contentLines) > maxContent {
				contentLines = contentLines[:maxContent]
			}
			view = strings.Join(append(contentLines, statusPart...), "\n")
		} else {
			view = strings.Join(viewLines[:m.height], "\n")
		}
	}

	// Render overlays (in priority order)
	if m.helpOverlay.IsActive() {
		overlay := m.helpOverlay.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.settings.IsActive() {
		overlay := m.settings.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.vaultStats.IsActive() {
		overlay := m.vaultStats.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.graphView.IsActive() {
		overlay := m.graphView.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.tagBrowser.IsActive() {
		overlay := m.tagBrowser.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.outline.IsActive() {
		overlay := m.outline.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.bookmarks.IsActive() {
		overlay := m.bookmarks.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.findReplace.IsActive() {
		overlay := m.findReplace.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.templates.IsActive() {
		overlay := m.templates.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.quickSwitch.IsActive() {
		overlay := m.quickSwitch.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.trash.IsActive() {
		overlay := m.trash.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.canvas.IsActive() {
		overlay := m.canvas.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.calendar.IsActive() {
		overlay := m.calendar.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.bots.IsActive() {
		overlay := m.bots.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.export.IsActive() {
		overlay := m.export.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.git.IsActive() {
		overlay := m.git.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.plugins.IsActive() {
		overlay := m.plugins.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.contentSearch.IsActive() {
		overlay := m.contentSearch.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.globalReplace.IsActive() {
		overlay := m.globalReplace.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.aiTemplates.IsActive() {
		overlay := m.aiTemplates.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.languageLearning.IsActive() {
		overlay := m.languageLearning.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.habitTracker.IsActive() {
		overlay := m.habitTracker.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.ideasBoard.IsActive() {
		overlay := m.ideasBoard.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.universalSearch.IsActive() {
		overlay := m.universalSearch.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.goalsMode.IsActive() {
		overlay := m.goalsMode.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.focusSession.IsActive() {
		overlay := m.focusSession.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.standupGen.IsActive() {
		overlay := m.standupGen.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.dailyReview.IsActive() {
		overlay := m.dailyReview.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.noteHistory.IsActive() {
		overlay := m.noteHistory.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.smartConnect.IsActive() {
		overlay := m.smartConnect.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.writingStats.IsActive() {
		overlay := m.writingStats.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.quickCapture.IsActive() {
		overlay := m.quickCapture.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.dashboard.IsActive() {
		overlay := m.dashboard.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.mindMap.IsActive() {
		overlay := m.mindMap.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.journalPrompts.IsActive() {
		overlay := m.journalPrompts.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.clipManager.IsActive() {
		overlay := m.clipManager.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.dailyPlanner.IsActive() {
		overlay := m.dailyPlanner.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.aiScheduler.IsActive() {
		overlay := m.aiScheduler.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.planMyDay.IsActive() {
		overlay := m.planMyDay.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.aiProjectPlanner.IsActive() {
		overlay := m.aiProjectPlanner.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.recurringTasks.IsActive() {
		overlay := m.recurringTasks.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.notePreview.IsActive() {
		overlay := m.notePreview.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.scratchpad.IsActive() {
		overlay := m.scratchpad.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.backup.IsActive() {
		overlay := m.backup.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.nextcloudOverlay.IsActive() {
		overlay := m.nextcloudOverlay.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.tutorial.IsActive() {
		overlay := m.tutorial.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.projectMode.IsActive() {
		overlay := m.projectMode.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.projectDashboard.IsActive() {
		overlay := m.projectDashboard.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.commandCenter.IsActive() {
		overlay := m.commandCenter.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.nlSearch.IsActive() {
		overlay := m.nlSearch.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.writingCoach.IsActive() {
		overlay := m.writingCoach.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.dataview.IsActive() {
		overlay := m.dataview.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.timeTracker.IsActive() {
		overlay := m.timeTracker.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.knowledgeGaps.IsActive() {
		overlay := m.knowledgeGaps.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.spellcheck.IsActive() {
		overlay := m.spellcheck.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.publisher.IsActive() {
		overlay := m.publisher.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.luaOverlay.IsActive() {
		overlay := m.luaOverlay.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.flashcards.IsActive() {
		overlay := m.flashcards.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.quizMode.IsActive() {
		overlay := m.quizMode.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.learnDash.IsActive() {
		overlay := m.learnDash.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.aiChat.IsActive() {
		overlay := m.aiChat.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.composer.IsActive() {
		overlay := m.composer.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.knowledgeGraph.IsActive() {
		overlay := m.knowledgeGraph.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.tableEditor.IsActive() {
		overlay := m.tableEditor.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.semanticSearch.IsActive() {
		overlay := m.semanticSearch.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.threadWeaver.IsActive() {
		overlay := m.threadWeaver.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.noteChat.IsActive() {
		overlay := m.noteChat.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.kanban.IsActive() {
		overlay := m.kanban.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.vaultRefactor.IsActive() {
		overlay := m.vaultRefactor.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.dailyBriefing.IsActive() {
		overlay := m.dailyBriefing.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.encryption.IsActive() {
		overlay := m.encryption.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.backlinkPreview.IsActive() && !m.viewMode {
		popup := m.backlinkPreview.View()
		if popup != "" {
			view = m.overlayCenter(view, popup)
		}
	}
	if m.gitHistory.IsActive() {
		overlay := m.gitHistory.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.workspace.IsActive() {
		overlay := m.workspace.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.timeline.IsActive() {
		overlay := m.timeline.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.vaultSwitch.IsActive() {
		overlay := m.vaultSwitch.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.frontmatterEdit.IsActive() {
		overlay := m.frontmatterEdit.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.taskManager.IsActive() {
		overlay := m.taskManager.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.linkAssist.IsActive() {
		overlay := m.linkAssist.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.blogPublisher.IsActive() {
		overlay := m.blogPublisher.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.themeEditor.IsActive() {
		overlay := m.themeEditor.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.imageManager.IsActive() {
		overlay := m.imageManager.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.research.IsActive() {
		overlay := m.research.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.weeklyReview.IsActive() {
		overlay := m.weeklyReview.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.readingList.IsActive() {
		overlay := m.readingList.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.pomodoro.IsActive() {
		overlay := m.pomodoro.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.webClipper.IsActive() {
		overlay := m.webClipper.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.linkCompleter != nil && m.linkCompleter.IsActive() {
		overlay := m.linkCompleter.Render(m.width/2, m.height/2)
		if overlay != "" {
			view = m.overlayCenter(view, overlay)
		}
	}
	if m.slashMenu != nil && m.slashMenu.IsActive() {
		overlay := m.slashMenu.View()
		if overlay != "" {
			view = m.overlayCenter(view, overlay)
		}
	}
	if m.splitPane.IsActive() {
		view = m.splitPane.View()
	}
	if m.confirmDelete {
		overlay := m.renderConfirmDeleteOverlay()
		view = m.overlayCenter(view, overlay)
	}
	if m.pendingReload {
		overlay := m.renderPendingReloadOverlay()
		view = m.overlayCenter(view, overlay)
	}
	if m.commandPalette.IsActive() {
		overlay := m.commandPalette.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.searchMode {
		overlay := m.renderSearchOverlay()
		view = m.overlayCenter(view, overlay)
	}
	if m.newNoteMode {
		overlay := m.renderNewNoteOverlay()
		view = m.overlayCenter(view, overlay)
	}
	if m.extractMode {
		overlay := m.renderExtractNoteOverlay()
		view = m.overlayCenter(view, overlay)
	}
	if m.newFolderMode {
		overlay := m.renderNewFolderOverlay()
		view = m.overlayCenter(view, overlay)
	}
	if m.moveFileMode {
		overlay := m.renderMoveFileOverlay()
		view = m.overlayCenter(view, overlay)
	}

	// Toast notifications (top-right corner)
	if m.toast != nil && m.toast.HasItems() {
		toastView := m.toast.View()
		if toastView != "" {
			view = m.overlayTopRight(view, toastView)
		}
	}

	// Clamp output to terminal height to prevent scrolling artifacts
	lines := strings.Split(view, "\n")
	if len(lines) > m.height {
		lines = lines[:m.height]
		view = strings.Join(lines, "\n")
	}

	return view
}

// overlayTopRight places an overlay in the top-right corner of the background.
func (m Model) overlayTopRight(bg, overlay string) string {
	bgLines := strings.Split(bg, "\n")
	overlayLines := strings.Split(overlay, "\n")

	overlayWidth := 0
	for _, line := range overlayLines {
		w := lipgloss.Width(line)
		if w > overlayWidth {
			overlayWidth = w
		}
	}

	startX := m.width - overlayWidth - 2
	if startX < 0 {
		startX = 0
	}
	startY := 1

	result := make([]string, len(bgLines))
	copy(result, bgLines)

	pad := strings.Repeat(" ", startX)
	for i, overlayLine := range overlayLines {
		y := startY + i
		if y >= len(result) {
			break
		}
		right := ansiSkipCols(result[y], startX+lipgloss.Width(overlayLine))
		result[y] = pad + overlayLine + right
	}

	return strings.Join(result, "\n")
}

func (m Model) renderViewMode() string {
	var b strings.Builder
	contentWidth := m.editor.width - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Minimal header — just a thin separator (breadcrumb already shows the path)
	b.WriteString(DimStyle.Render(strings.Repeat("─", contentWidth)))
	b.WriteString("\n")

	// Render markdown content
	rendered := m.renderer.Render(m.editor.GetContent(), m.viewScroll)

	// Add scroll position indicator on the right edge
	vmTotalLines := m.renderer.RenderLineCount(m.editor.GetContent())
	vmViewportH := m.renderer.height - 4
	if vmViewportH < 1 {
		vmViewportH = 1
	}

	renderedLines := strings.Split(rendered, "\n")

	// Clamp each line width to prevent overflow when EditorStyle wraps.
	// Lines wider than contentWidth cause garbled rendering with overlays.
	maxLineW := contentWidth - 2
	if maxLineW < 10 {
		maxLineW = 10
	}
	for i, rl := range renderedLines {
		w := lipgloss.Width(rl)
		if w > maxLineW {
			// Truncate using plain text (ANSI-aware truncation is too complex;
			// the renderer should ideally produce lines within budget already).
			plain := stripAnsi(rl)
			renderedLines[i] = TruncateDisplay(plain, maxLineW)
		}
	}

	trackHeight := len(renderedLines)
	if trackHeight > 0 && vmTotalLines > vmViewportH {
		thumbSize := maxInt(1, trackHeight*vmViewportH/vmTotalLines)
		if thumbSize > trackHeight {
			thumbSize = trackHeight
		}
		vmMaxScroll := vmTotalLines - vmViewportH
		thumbPos := 0
		if vmMaxScroll > 0 {
			thumbPos = m.viewScroll * (trackHeight - thumbSize) / vmMaxScroll
		}
		if thumbPos+thumbSize > trackHeight {
			thumbPos = trackHeight - thumbSize
		}
		if thumbPos < 0 {
			thumbPos = 0
		}

		trackStyle := lipgloss.NewStyle().Foreground(surface0)
		thumbStyle := lipgloss.NewStyle().Foreground(mauve)

		for i := 0; i < trackHeight; i++ {
			var indicator string
			if i >= thumbPos && i < thumbPos+thumbSize {
				indicator = thumbStyle.Render("\u2588")
			} else {
				indicator = trackStyle.Render("\u2502")
			}
			renderedLines[i] = renderedLines[i] + " " + indicator
		}
	}

	b.WriteString(strings.Join(renderedLines, "\n"))

	return b.String()
}

// updateReadingProgress calculates the reading progress percentage
// based on the current scroll position and total rendered lines.
func (m *Model) updateReadingProgress() {
	totalLines := m.renderer.RenderLineCount(m.editor.GetContent())
	viewportHeight := m.renderer.height - 4
	if viewportHeight < 1 {
		viewportHeight = 1
	}

	maxScroll := totalLines - viewportHeight
	if maxScroll <= 0 {
		// Content fits in viewport — always 100%
		m.statusbar.SetReadingProgress(100)
		return
	}

	// Clamp viewScroll to valid range
	if m.viewScroll > maxScroll {
		m.viewScroll = maxScroll
	}

	percent := m.viewScroll * 100 / maxScroll
	if percent > 100 {
		percent = 100
	}
	m.statusbar.SetReadingProgress(percent)
}

func (m Model) renderSearchOverlay() string {
	width := m.width / 2
	if width < 40 {
		width = 40
	}
	if width > 80 {
		width = 80
	}

	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconSearchChar + " Quick Open")
	b.WriteString(title)
	b.WriteString("\n\n")

	prompt := SearchPromptStyle.Render(" > ")
	input := m.searchQuery + DimStyle.Render("_")
	b.WriteString(prompt + input)
	b.WriteString("\n")

	// Result count
	innerWidth := width - 4
	countStr := ""
	if m.searchQuery != "" {
		countStr = fmt.Sprintf("  %d result", len(m.searchResults))
		if len(m.searchResults) != 1 {
			countStr += "s"
		}
	}
	sepLine := DimStyle.Render(strings.Repeat("─", innerWidth))
	if countStr != "" {
		sepLine = DimStyle.Render(strings.Repeat("─", innerWidth-lipgloss.Width(countStr))) + DimStyle.Render(countStr)
	}
	b.WriteString(sepLine)
	b.WriteString("\n")

	maxResults := 10
	if len(m.searchResults) == 0 {
		b.WriteString(DimStyle.Render("  No results"))
	} else {
		for i := 0; i < len(m.searchResults) && i < maxResults; i++ {
			name := strings.TrimSuffix(m.searchResults[i], ".md")
			icon := lipgloss.NewStyle().Foreground(blue).Render(IconFileChar)
			if i == m.searchCursor {
				selectedBase := lipgloss.NewStyle().
					Background(surface0).
					Foreground(peach).
					Bold(true)
				matchOnSelected := lipgloss.NewStyle().
					Background(surface0).
					Foreground(yellow).
					Bold(true).
					Underline(true)
				highlighted := fuzzyHighlight(name, m.searchQuery, selectedBase, matchOnSelected)
				line := selectedBase.MaxWidth(innerWidth).Render("  " + icon + " " + highlighted)
				b.WriteString(line)
			} else {
				matchStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
				highlighted := fuzzyHighlight(name, m.searchQuery, NormalItemStyle, matchStyle)
				b.WriteString("  " + icon + " " + highlighted)
			}
			b.WriteString("\n")
		}
	}

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (m Model) renderNewNoteOverlay() string {
	width := m.width / 3
	if width < 40 {
		width = 40
	}

	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(green).
		Bold(true).
		Render("  " + IconNewChar + " New Note")
	b.WriteString(title)
	b.WriteString("\n\n")

	prompt := lipgloss.NewStyle().Foreground(green).Bold(true).Render(" Name: ")
	input := m.newNoteName + DimStyle.Render("_")
	b.WriteString(prompt + input)
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  Enter to create, Esc to cancel"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(green).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (m Model) renderExtractNoteOverlay() string {
	width := m.width / 3
	if width < 45 {
		width = 45
	}

	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(blue).
		Bold(true).
		Render("  " + IconLinkChar + " Extract to Note")
	b.WriteString(title)
	b.WriteString("\n\n")

	prompt := lipgloss.NewStyle().Foreground(blue).Bold(true).Render(" Name: ")
	input := m.extractName + DimStyle.Render("_")
	b.WriteString(prompt + input)
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  Enter to extract, Esc to cancel"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(blue).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (m Model) renderConfirmDeleteOverlay() string {
	width := 50

	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(red).
		Bold(true).
		Render("  " + IconTrashChar + " Delete Note")
	b.WriteString(title)
	b.WriteString("\n\n")

	b.WriteString("  Move to trash:\n")
	b.WriteString("  " + lipgloss.NewStyle().Foreground(peach).Bold(true).Render(m.confirmDeleteNote))
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  y/Enter: confirm  n/Esc: cancel"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(red).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (m Model) renderPendingReloadOverlay() string {
	width := 54

	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(peach).
		Bold(true).
		Render("  " + IconFileChar + " File Modified Externally")
	b.WriteString(title)
	b.WriteString("\n\n")

	b.WriteString("  The file has been changed outside Granit:\n")
	b.WriteString("  " + lipgloss.NewStyle().Foreground(blue).Bold(true).Render(m.pendingReloadPath))
	b.WriteString("\n\n")
	b.WriteString("  You have unsaved changes. Reload from disk?\n\n")
	b.WriteString(DimStyle.Render("  y: reload (discard changes)  n/Esc: keep editing"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(peach).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (m *Model) doReplace() {
	query := m.findReplace.GetFindQuery()
	replacement := m.findReplace.GetReplaceText()
	if query == "" {
		return
	}

	if m.findReplace.IsRegexMode() {
		m.doReplaceRegex(query, replacement, false)
		return
	}

	// Replace first occurrence from current position
	for i := m.editor.cursor; i < len(m.editor.content); i++ {
		lower := strings.ToLower(m.editor.content[i])
		idx := strings.Index(lower, strings.ToLower(query))
		if idx >= 0 {
			line := m.editor.content[i]
			m.editor.content[i] = line[:idx] + replacement + line[idx+len(query):]
			m.editor.modified = true
			m.editor.countWords()
			m.findReplace.UpdateMatches(m.editor.content)
			return
		}
	}
}

func (m *Model) doReplaceAll() {
	query := m.findReplace.GetFindQuery()
	replacement := m.findReplace.GetReplaceText()
	if query == "" {
		return
	}

	if m.findReplace.IsRegexMode() {
		m.doReplaceRegex(query, replacement, true)
		return
	}

	count := 0
	for i := range m.editor.content {
		lower := strings.ToLower(m.editor.content[i])
		lowerQuery := strings.ToLower(query)
		for strings.Contains(lower, lowerQuery) {
			idx := strings.Index(lower, lowerQuery)
			line := m.editor.content[i]
			m.editor.content[i] = line[:idx] + replacement + line[idx+len(query):]
			lower = strings.ToLower(m.editor.content[i])
			count++
		}
	}
	if count > 0 {
		m.editor.modified = true
		m.editor.countWords()
		m.findReplace.UpdateMatches(m.editor.content)
		m.statusbar.SetMessage(fmt.Sprintf("Replaced %d occurrences", count))
	}
}

// doReplaceRegex performs regex-aware replacement. When replaceAll is false,
// only the first match from the current cursor position is replaced.
func (m *Model) doReplaceRegex(pattern, replacement string, replaceAll bool) {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return
	}

	count := 0
	start := 0
	if !replaceAll {
		start = m.editor.cursor
	}

	for i := start; i < len(m.editor.content); i++ {
		line := m.editor.content[i]
		if replaceAll {
			newLine := re.ReplaceAllString(line, replacement)
			if newLine != line {
				// Count individual matches for the status message
				count += len(re.FindAllStringIndex(line, -1))
				m.editor.content[i] = newLine
			}
		} else {
			loc := re.FindStringIndex(line)
			if loc != nil {
				newLine := line[:loc[0]] + re.ReplaceAllString(line[loc[0]:loc[1]], replacement) + line[loc[1]:]
				m.editor.content[i] = newLine
				m.editor.modified = true
				m.editor.countWords()
				m.findReplace.UpdateMatches(m.editor.content)
				return
			}
		}
	}

	if count > 0 {
		m.editor.modified = true
		m.editor.countWords()
		m.findReplace.UpdateMatches(m.editor.content)
		m.statusbar.SetMessage(fmt.Sprintf("Replaced %d occurrences", count))
	}
}

// ---------------------------------------------------------------------------
// Folder management
// ---------------------------------------------------------------------------

func (m *Model) getVaultDirs() []string {
	dirSet := map[string]bool{".": true}
	for _, p := range m.vault.SortedPaths() {
		dir := filepath.Dir(p)
		if dir != "." {
			dirSet[dir] = true
			// Also add parent dirs
			for dir != "." {
				dirSet[dir] = true
				dir = filepath.Dir(dir)
			}
		}
	}
	dirs := make([]string, 0, len(dirSet))
	for d := range dirSet {
		if d == "." {
			dirs = append(dirs, "(root)")
		} else {
			dirs = append(dirs, d)
		}
	}
	sort.Strings(dirs)
	return dirs
}

func (m Model) updateNewFolder(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.newFolderMode = false
		return m, nil
	case "enter":
		if m.newFolderName != "" {
			folderPath := filepath.Join(m.vault.Root, m.newFolderName)
			if err := os.MkdirAll(folderPath, 0755); err == nil {
				// Create a .gitkeep so folder shows up
				m.statusbar.SetMessage("Created folder: " + m.newFolderName)
			}
		}
		m.newFolderMode = false
		return m, m.clearMessageAfter(2 * time.Second)
	case "backspace":
		if len(m.newFolderName) > 0 {
			m.newFolderName = m.newFolderName[:len(m.newFolderName)-1]
		}
		return m, nil
	default:
		char := msg.String()
		if len(char) == 1 && char[0] >= 32 {
			m.newFolderName += char
		}
		return m, nil
	}
}

func (m Model) updateMoveFile(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.moveFileMode = false
		return m, nil
	case "up", "k":
		if m.moveFileCursor > 0 {
			m.moveFileCursor--
		}
		return m, nil
	case "down", "j":
		if m.moveFileCursor < len(m.moveFileDirs)-1 {
			m.moveFileCursor++
		}
		return m, nil
	case "enter":
		if m.activeNote != "" && m.moveFileCursor < len(m.moveFileDirs) {
			targetDir := m.moveFileDirs[m.moveFileCursor]
			if targetDir == "(root)" {
				targetDir = ""
			}
			baseName := filepath.Base(m.activeNote)
			var newPath string
			if targetDir == "" {
				newPath = baseName
			} else {
				newPath = filepath.Join(targetDir, baseName)
			}

			if newPath != m.activeNote {
				oldFullPath := filepath.Join(m.vault.Root, m.activeNote)
				newFullPath := filepath.Join(m.vault.Root, newPath)
				_ = os.MkdirAll(filepath.Dir(newFullPath), 0755)
				if err := os.Rename(oldFullPath, newFullPath); err == nil {
					_ = m.vault.Scan()
					m.index = vault.NewIndex(m.vault)
					m.index.Build()
					paths := m.vault.SortedPaths()
					m.sidebar.SetFiles(paths)
					m.autocomplete.SetNotes(paths)
					m.statusbar.SetNoteCount(m.vault.NoteCount())
					m.loadNote(newPath)
					m.setSidebarCursorToFile(newPath)
					m.statusbar.SetMessage("Moved to " + newPath)
				}
			}
		}
		m.moveFileMode = false
		return m, m.clearMessageAfter(2 * time.Second)
	}
	return m, nil
}

func (m Model) renderNewFolderOverlay() string {
	width := m.width / 3
	if width < 40 {
		width = 40
	}

	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(peach).
		Bold(true).
		Render("  " + IconFolderChar + " New Folder")
	b.WriteString(title)
	b.WriteString("\n\n")

	prompt := lipgloss.NewStyle().Foreground(peach).Bold(true).Render(" Name: ")
	input := m.newFolderName + DimStyle.Render("_")
	b.WriteString(prompt + input)
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  Enter to create, Esc to cancel"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Use / for nested folders (e.g. projects/web)"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(peach).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (m Model) renderMoveFileOverlay() string {
	width := m.width / 3
	if width < 40 {
		width = 40
	}

	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(blue).
		Bold(true).
		Render("  " + IconFolderChar + " Move Note")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Moving: " + m.activeNote))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	b.WriteString("\n")

	visibleItems := m.height - 10
	if visibleItems < 5 {
		visibleItems = 5
	}

	start := 0
	if m.moveFileCursor >= visibleItems {
		start = m.moveFileCursor - visibleItems + 1
	}
	end := start + visibleItems
	if end > len(m.moveFileDirs) {
		end = len(m.moveFileDirs)
	}

	for i := start; i < end; i++ {
		dir := m.moveFileDirs[i]
		icon := lipgloss.NewStyle().Foreground(peach).Render(IconFolderChar)
		if i == m.moveFileCursor {
			line := lipgloss.NewStyle().
				Background(surface0).
				Foreground(peach).
				Bold(true).
				MaxWidth(width - 6).
				Render("  " + icon + " " + dir)
			b.WriteString(line)
		} else {
			b.WriteString("  " + icon + " " + NormalItemStyle.Render(dir))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Enter: move here  Esc: cancel"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(blue).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (m Model) overlayCenter(bg, overlay string) string {
	bgLines := strings.Split(bg, "\n")
	overlayLines := strings.Split(overlay, "\n")

	overlayHeight := len(overlayLines)
	overlayWidth := 0
	for _, line := range overlayLines {
		w := lipgloss.Width(line)
		if w > overlayWidth {
			overlayWidth = w
		}
	}

	startY := (m.height - overlayHeight) / 3
	startX := (m.width - overlayWidth) / 2
	if startY < 1 {
		startY = 1
	}
	if startX < 0 {
		startX = 0
	}

	result := make([]string, len(bgLines))
	copy(result, bgLines)

	pad := strings.Repeat(" ", startX)
	for i, overlayLine := range overlayLines {
		y := startY + i
		if y >= len(result) {
			break
		}
		right := ansiSkipCols(result[y], startX+lipgloss.Width(overlayLine))
		result[y] = pad + overlayLine + right
	}

	return strings.Join(result, "\n")
}

// ansiSkipCols returns the suffix of s after skipping n visual columns.
// ANSI escape sequences are correctly skipped without counting as width.
// A reset sequence is prepended to prevent color bleed from the skipped portion.
func ansiSkipCols(s string, n int) string {
	width := 0
	i := 0
	for i < len(s) && width < n {
		if s[i] == '\x1b' {
			// Skip entire ANSI escape sequence
			j := i + 1
			if j < len(s) && s[j] == '[' {
				j++
				for j < len(s) && s[j] != 'm' && s[j] != 'H' && s[j] != 'J' && s[j] != 'K' {
					j++
				}
				if j < len(s) {
					j++
				}
			}
			i = j
		} else {
			width++
			i++
		}
	}
	if i >= len(s) {
		return ""
	}
	return "\x1b[0m" + s[i:]
}


// applyVaultRefactor parses the AI refactor plan and applies file moves,
// tag additions, and wikilink insertions.
func (m *Model) applyVaultRefactor(plan string) {
	moveCount := 0
	for _, line := range strings.Split(plan, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "MOVE:") {
			continue
		}
		parts := strings.SplitN(line[5:], "|", 4)
		if len(parts) < 1 {
			continue
		}
		movePart := strings.TrimSpace(parts[0])
		arrow := strings.SplitN(movePart, "->", 2)
		if len(arrow) != 2 {
			continue
		}
		oldName := strings.TrimSpace(arrow[0])
		newName := strings.TrimSpace(arrow[1])

		oldPath := filepath.Join(m.vault.Root, oldName)
		newPath := filepath.Join(m.vault.Root, newName)

		if _, err := os.Stat(oldPath); os.IsNotExist(err) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(newPath), 0755); err != nil {
			continue
		}
		if err := os.Rename(oldPath, newPath); err != nil {
			continue
		}
		moveCount++

		for _, part := range parts[1:] {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "TAGS:") {
				tagStr := strings.TrimSpace(part[5:])
				tags := strings.Split(tagStr, ",")
				var cleanTags []string
				for _, t := range tags {
					t = strings.TrimSpace(t)
					t = strings.TrimPrefix(t, "#")
					if t != "" {
						cleanTags = append(cleanTags, t)
					}
				}
				if len(cleanTags) > 0 {
					m.addTagsToFile(newPath, cleanTags)
				}
			}
		}
	}

	if moveCount > 0 {
		_ = m.vault.Scan()
		m.index = vault.NewIndex(m.vault)
		m.index.Build()
		paths := m.vault.SortedPaths()
		m.sidebar.SetFiles(paths)
		m.autocomplete.SetNotes(paths)
		m.statusbar.SetNoteCount(m.vault.NoteCount())
		m.statusbar.SetMessage(fmt.Sprintf("Vault refactored: %d files reorganized", moveCount))
	} else {
		m.statusbar.SetMessage("Vault refactor: no changes applied")
	}
}

func (m *Model) addTagsToFile(path string, tags []string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	content := string(data)
	lines := strings.Split(content, "\n")

	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		fmEnd := -1
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				fmEnd = i
				break
			}
		}
		if fmEnd > 0 {
			hasTagsLine := false
			for i := 1; i < fmEnd; i++ {
				if strings.HasPrefix(strings.TrimSpace(lines[i]), "tags:") {
					hasTagsLine = true
					break
				}
			}
			if !hasTagsLine {
				tagLine := "tags: [" + strings.Join(tags, ", ") + "]"
				newLines := make([]string, 0, len(lines)+1)
				newLines = append(newLines, lines[:fmEnd]...)
				newLines = append(newLines, tagLine)
				newLines = append(newLines, lines[fmEnd:]...)
				_ = os.WriteFile(path, []byte(strings.Join(newLines, "\n")), 0644)
			}
			return
		}
	}

	fm := "---\ntags: [" + strings.Join(tags, ", ") + "]\n---\n\n"
	_ = os.WriteFile(path, []byte(fm+content), 0644)
}

// applyEncryptionResult handles the result of an encrypt/decrypt operation.
func (m *Model) applyEncryptionResult(result EncryptionResult) {
	if m.activeNote == "" {
		return
	}

	content := m.editor.GetContent()
	oldPath := filepath.Join(m.vault.Root, m.activeNote)

	switch result.Action {
	case encActionEncrypt:
		encrypted, err := m.encryption.EncryptContent(content)
		if err != nil {
			m.statusbar.SetError("Encryption failed: " + err.Error())
			return
		}
		newName := m.encryption.EncryptedName(m.activeNote)
		newPath := filepath.Join(m.vault.Root, newName)
		if err := os.WriteFile(newPath, []byte(encrypted), 0644); err != nil {
			m.statusbar.SetError("Failed to write encrypted file")
			return
		}
		// Remove the original unencrypted file
		_ = os.Remove(oldPath)
		_ = m.vault.Scan()
		m.index = vault.NewIndex(m.vault)
		m.index.Build()
		paths := m.vault.SortedPaths()
		m.sidebar.SetFiles(paths)
		m.autocomplete.SetNotes(paths)
		m.statusbar.SetNoteCount(m.vault.NoteCount())
		m.statusbar.SetMessage("Encrypted: " + m.activeNote + " -> " + newName)
		// Load the encrypted file (shows base64)
		m.loadNote(newName)
		m.setSidebarCursorToFile(newName)

	case encActionDecrypt:
		if !m.encryption.IsEncrypted(m.activeNote) {
			m.statusbar.SetWarning("Note is not encrypted")
			return
		}
		decrypted, err := m.encryption.DecryptContent(content)
		if err != nil {
			m.statusbar.SetError("Decryption failed - wrong passphrase?")
			return
		}
		newName := m.encryption.DecryptedName(m.activeNote)
		newPath := filepath.Join(m.vault.Root, newName)
		if err := os.WriteFile(newPath, []byte(decrypted), 0644); err != nil {
			m.statusbar.SetError("Failed to write decrypted file")
			return
		}
		// Remove the encrypted file
		_ = os.Remove(oldPath)
		_ = m.vault.Scan()
		m.index = vault.NewIndex(m.vault)
		m.index.Build()
		paths := m.vault.SortedPaths()
		m.sidebar.SetFiles(paths)
		m.autocomplete.SetNotes(paths)
		m.statusbar.SetNoteCount(m.vault.NoteCount())
		m.loadNote(newName)
		m.setSidebarCursorToFile(newName)
		m.statusbar.SetMessage("Decrypted: " + m.activeNote + " -> " + newName)
	}
}

// captureWorkspaceLayout snapshots the current TUI state into a WorkspaceLayout.
func (m *Model) captureWorkspaceLayout(name string) WorkspaceLayout {
	var openNotes []string
	if m.tabBar != nil {
		for _, tab := range m.tabBar.Tabs() {
			openNotes = append(openNotes, tab.Path)
		}
	}
	layoutName := m.config.Layout
	if layoutName == "" {
		layoutName = "default"
	}
	return WorkspaceLayout{
		Name:         name,
		ActiveNote:   m.activeNote,
		OpenNotes:    openNotes,
		SidebarFocus: m.focus == focusSidebar,
		ViewMode:     m.viewMode,
		Layout:       layoutName,
		CreatedAt:    time.Now().Format("2006-01-02 15:04:05"),
	}
}

// applyWorkspaceLayout restores a saved workspace layout.
func (m *Model) applyWorkspaceLayout(layout *WorkspaceLayout) {
	if layout == nil {
		return
	}
	// Restore layout mode
	if layout.Layout != "" {
		m.config.Layout = layout.Layout
		m.updateLayout()
	}
	// Restore open notes in tabs
	if m.tabBar != nil && len(layout.OpenNotes) > 0 {
		// Reset tabs by removing all then re-adding
		for _, tab := range m.tabBar.Tabs() {
			m.tabBar.RemoveTab(tab.Path)
		}
		for _, note := range layout.OpenNotes {
			if m.vault.GetNote(note) != nil {
				m.tabBar.AddTab(note)
			}
		}
	}
	// Restore active note
	if layout.ActiveNote != "" && m.vault.GetNote(layout.ActiveNote) != nil {
		m.loadNote(layout.ActiveNote)
		m.setSidebarCursorToFile(layout.ActiveNote)
	}
	// Restore view mode
	m.viewMode = layout.ViewMode
	if m.viewMode {
		m.statusbar.SetMode("VIEW")
		m.statusbar.SetViewMode(true)
		m.updateReadingProgress()
	} else {
		m.statusbar.SetMode("EDIT")
		m.statusbar.SetViewMode(false)
	}
	// Restore focus
	if layout.SidebarFocus {
		m.setFocus(focusSidebar)
	} else {
		m.setFocus(focusEditor)
	}
	m.statusbar.SetMessage("Loaded workspace: " + layout.Name)
}

func (m *Model) writeBriefingToDailyNote(briefingContent string) {
	today := time.Now().Format("2006-01-02")
	dailyName := today + ".md"
	dailyPath := filepath.Join(m.vault.Root, dailyName)

	existing, err := os.ReadFile(dailyPath)
	var writeErr error
	if err != nil {
		fallback := fmt.Sprintf("---\ndate: %s\ntype: daily\ntags: [daily]\n---\n\n# %s — {{weekday}}\n\n%s\n", today, today, briefingContent)
		content := m.dailyNoteContent(today, fallback)
		writeErr = os.WriteFile(dailyPath, []byte(content), 0644)
	} else {
		newContent := string(existing) + "\n\n---\n\n" + briefingContent + "\n"
		writeErr = os.WriteFile(dailyPath, []byte(newContent), 0644)
	}
	if writeErr != nil {
		m.statusbar.SetMessage("Failed to write daily briefing: " + writeErr.Error())
		return
	}

	_ = m.vault.Scan()
	m.index = vault.NewIndex(m.vault)
	m.index.Build()
	paths := m.vault.SortedPaths()
	m.sidebar.SetFiles(paths)
	m.autocomplete.SetNotes(paths)
	m.statusbar.SetNoteCount(m.vault.NoteCount())
	m.loadNote(dailyName)
	m.setSidebarCursorToFile(dailyName)
	m.setFocus(focusEditor)
	m.statusbar.SetMessage("Daily briefing written to " + dailyName)
}

// buildDailyFallback constructs the rich default daily note template.
// It uses {{weekday}} and other template variables that get expanded by dailyNoteContent.
func (m *Model) buildDailyFallback(date string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("---\ndate: %s\ntype: daily\ntags: [daily]\n---\n\n", date))
	b.WriteString(fmt.Sprintf("# %s — {{weekday}}\n\n", date))
	b.WriteString("## Morning\n- [ ] \n\n")
	b.WriteString("## Tasks\n")
	if len(m.config.DailyRecurringTasks) > 0 {
		b.WriteString("{{recurring_tasks}}\n")
	}
	b.WriteString("- [ ] \n\n")
	b.WriteString("## Notes\n\n\n")
	b.WriteString("## Reflection\n\n")
	carriedTasks := m.yesterdayIncompleteTasks()
	if len(carriedTasks) > 0 {
		b.WriteString("## Carried Forward\n")
		b.WriteString("{{carry_forward}}\n\n")
	}
	return b.String()
}

// openDailyNote creates (if needed) and opens today's daily note.
func (m *Model) openDailyNote() {
	today := time.Now().Format("2006-01-02")
	name := today + ".md"
	folder := m.config.DailyNotesFolder
	if folder != "" {
		name = filepath.Join(folder, name)
	}
	path := filepath.Join(m.vault.Root, name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			m.statusbar.SetMessage("Failed to create daily note folder: " + err.Error())
			return
		}
		fallback := m.buildDailyFallback(today)
		content := m.dailyNoteContent(today, fallback)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			m.statusbar.SetMessage("Failed to create daily note: " + err.Error())
			return
		}
		_ = m.vault.Scan()
		m.index = vault.NewIndex(m.vault)
		m.index.Build()
		m.sidebar.SetFiles(m.vault.SortedPaths())
		m.statusbar.SetNoteCount(m.vault.NoteCount())
		m.statusbar.SetMessage("Created daily note: " + name)
	}
	m.loadNote(name)
	m.setSidebarCursorToFile(name)
	m.setFocus(focusEditor)
}

// navigateDailyNote navigates to the previous (direction=-1) or next (direction=+1)
// daily note relative to the current note's date (or today if not on a daily note).
func (m *Model) navigateDailyNote(direction int) {
	currentDate := time.Now()
	if m.activeNote != "" {
		base := strings.TrimSuffix(filepath.Base(m.activeNote), ".md")
		if t, err := time.Parse("2006-01-02", base); err == nil {
			currentDate = t
		}
	}
	folder := m.config.DailyNotesFolder
	for i := 1; i <= 365; i++ {
		target := currentDate.AddDate(0, 0, direction*i)
		name := target.Format("2006-01-02") + ".md"
		if folder != "" {
			name = filepath.Join(folder, name)
		}
		if m.vault.GetNote(name) != nil {
			m.loadNote(name)
			m.setSidebarCursorToFile(name)
			m.setFocus(focusEditor)
			return
		}
	}
	m.statusbar.SetMessage("No daily note found in that direction")
}

// dailyNoteContent returns the initial content for a new daily note.
// If DailyNoteTemplate is configured and the template file exists in the vault,
// the template is loaded and variables are replaced:
//   - {{date}}           → the note's date (YYYY-MM-DD)
//   - {{title}}          → the note filename without extension
//   - {{weekday}}        → the full weekday name (e.g. "Monday")
//   - {{time}}           → current time in HH:MM format (e.g. "09:30")
//   - {{yesterday}}      → yesterday's date in YYYY-MM-DD format
//   - {{tomorrow}}       → tomorrow's date in YYYY-MM-DD format
//   - {{week_number}}    → ISO week number (e.g. "11")
//   - {{month_name}}     → full month name (e.g. "March")
//   - {{year}}           → 4-digit year (e.g. "2026")
//   - {{streak}}         → consecutive daily note streak count
//   - {{carry_forward}}  → incomplete tasks from yesterday's daily note
//   - {{recurring_tasks}} → configured recurring daily tasks as checkboxes
//
// If the template is not configured or the file cannot be read, fallback is used.
func (m *Model) dailyNoteContent(date, fallback string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		t = time.Now()
	}

	// Build template variable replacements
	yesterday := t.AddDate(0, 0, -1).Format("2006-01-02")
	tomorrow := t.AddDate(0, 0, 1).Format("2006-01-02")
	_, week := t.ISOWeek()
	weekNumber := strconv.Itoa(week)
	monthName := t.Month().String()
	year := strconv.Itoa(t.Year())
	currentTime := time.Now().Format("15:04")
	streak := strconv.Itoa(m.dailyNoteStreak())

	// Build carry-forward tasks
	carryForward := ""
	carriedTasks := m.yesterdayIncompleteTasks()
	if len(carriedTasks) > 0 {
		carryForward = strings.Join(carriedTasks, "\n")
	}

	// Build recurring tasks
	recurringTasks := ""
	if len(m.config.DailyRecurringTasks) > 0 {
		var lines []string
		for _, task := range m.config.DailyRecurringTasks {
			lines = append(lines, "- [ ] "+task)
		}
		recurringTasks = strings.Join(lines, "\n")
	}

	// Build overdue tasks list
	overdueTasks := ""
	todayTasksList := ""
	allTasks := ParseAllTasks(m.vault.Notes)
	var overdueLines, todayLines []string
	for _, task := range allTasks {
		if task.Done {
			continue
		}
		if tmIsOverdue(task.DueDate) {
			overdueLines = append(overdueLines, "- [ ] "+tmCleanText(task.Text))
		}
		if task.DueDate == date {
			todayLines = append(todayLines, "- [ ] "+tmCleanText(task.Text))
		}
	}
	if len(overdueLines) > 0 {
		overdueTasks = strings.Join(overdueLines, "\n")
	}
	if len(todayLines) > 0 {
		todayTasksList = strings.Join(todayLines, "\n")
	}

	// Build today's habits
	todayHabits := ""
	habitsPath := filepath.Join(m.vault.Root, "Habits", "habits.md")
	if data, err := os.ReadFile(habitsPath); err == nil {
		inSection := false
		var habitLines []string
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "## Habits" {
				inSection = true
				continue
			}
			if strings.HasPrefix(trimmed, "## ") && inSection {
				break
			}
			if inSection && strings.HasPrefix(trimmed, "|") && !strings.Contains(trimmed, "---") && !strings.Contains(trimmed, "Habit") {
				parts := strings.Split(trimmed, "|")
				if len(parts) >= 2 {
					name := strings.TrimSpace(parts[1])
					if name != "" {
						habitLines = append(habitLines, "- [ ] "+name)
					}
				}
			}
		}
		if len(habitLines) > 0 {
			todayHabits = strings.Join(habitLines, "\n")
		}
	}

	// Build today's schedule from planner blocks
	todaySchedule := ""
	if blocks := m.calendar.plannerBlocks[date]; len(blocks) > 0 {
		var schedLines []string
		for _, pb := range blocks {
			schedLines = append(schedLines, "- "+pb.StartTime+"-"+pb.EndTime+" "+pb.Text)
		}
		todaySchedule = strings.Join(schedLines, "\n")
	}

	// Build active goals summary
	activeGoals := ""
	gm := NewGoalsMode()
	gm.vaultRoot = m.vault.Root
	gm.loadGoals()
	var goalLines []string
	for _, g := range gm.goals {
		if g.Status == GoalStatusActive {
			line := "- " + g.Title
			if len(g.Milestones) > 0 {
				line += fmt.Sprintf(" (%d%%)", g.Progress())
			}
			if g.IsOverdue() {
				line += " **OVERDUE**"
			}
			goalLines = append(goalLines, line)
		}
	}
	if len(goalLines) > 0 {
		activeGoals = strings.Join(goalLines, "\n")
	}

	replaceVars := func(content string) string {
		content = strings.ReplaceAll(content, "{{date}}", date)
		content = strings.ReplaceAll(content, "{{title}}", date)
		content = strings.ReplaceAll(content, "{{weekday}}", t.Weekday().String())
		content = strings.ReplaceAll(content, "{{time}}", currentTime)
		content = strings.ReplaceAll(content, "{{yesterday}}", yesterday)
		content = strings.ReplaceAll(content, "{{tomorrow}}", tomorrow)
		content = strings.ReplaceAll(content, "{{week_number}}", weekNumber)
		content = strings.ReplaceAll(content, "{{month_name}}", monthName)
		content = strings.ReplaceAll(content, "{{year}}", year)
		content = strings.ReplaceAll(content, "{{streak}}", streak)
		content = strings.ReplaceAll(content, "{{carry_forward}}", carryForward)
		content = strings.ReplaceAll(content, "{{recurring_tasks}}", recurringTasks)
		content = strings.ReplaceAll(content, "{{overdue_tasks}}", overdueTasks)
		content = strings.ReplaceAll(content, "{{today_tasks}}", todayTasksList)
		content = strings.ReplaceAll(content, "{{today_habits}}", todayHabits)
		content = strings.ReplaceAll(content, "{{today_schedule}}", todaySchedule)
		content = strings.ReplaceAll(content, "{{active_goals}}", activeGoals)
		return content
	}

	tmplPath := m.config.DailyNoteTemplate
	if tmplPath == "" {
		return replaceVars(fallback)
	}
	absPath := filepath.Join(m.vault.Root, tmplPath)
	data, err := os.ReadFile(absPath)
	if err != nil {
		return replaceVars(fallback)
	}
	return replaceVars(string(data))
}

// dailyNoteStreak counts consecutive days backwards from today that have a daily note.
func (m *Model) dailyNoteStreak() int {
	streak := 0
	folder := m.config.DailyNotesFolder
	for d := 0; d < 365; d++ {
		date := time.Now().AddDate(0, 0, -d)
		name := date.Format("2006-01-02") + ".md"
		if folder != "" {
			name = filepath.Join(folder, name)
		}
		if m.vault.GetNote(name) != nil {
			streak++
		} else if d > 0 {
			// Today might not exist yet, that's ok — but a gap breaks the streak
			break
		} else {
			// Today doesn't exist, start counting from yesterday
			continue
		}
	}
	return streak
}

// yesterdayIncompleteTasks returns incomplete task lines from yesterday's daily note.
func (m *Model) yesterdayIncompleteTasks() []string {
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	name := yesterday + ".md"
	folder := m.config.DailyNotesFolder
	if folder != "" {
		name = filepath.Join(folder, name)
	}
	note := m.vault.GetNote(name)
	if note == nil {
		return nil
	}
	var tasks []string
	for _, line := range strings.Split(note.Content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [ ] ") && len(trimmed) > 6 {
			tasks = append(tasks, trimmed)
		}
	}
	return tasks
}

// weeklyNoteContent returns the initial content for a new weekly note.
// If WeeklyNoteTemplate is configured and the template file exists in the vault,
// the template is loaded and variables are replaced:
//   - {{week}}        → e.g. "2026-W11"
//   - {{year}}        → e.g. "2026"
//   - {{week_number}} → e.g. "11"
//   - {{week_start}}  → Monday date (YYYY-MM-DD)
//   - {{week_end}}    → Sunday date (YYYY-MM-DD)
//   - {{daily_links}} → wikilinks for each day of the week
//
// If the template is not configured or cannot be read, a default template is used.
func (m *Model) weeklyNoteContent(year, week int) string {
	weekStr := fmt.Sprintf("%d-W%02d", year, week)
	monday := isoWeekStart(year, week)
	sunday := monday.AddDate(0, 0, 6)

	// Build daily links
	var dailyLinks []string
	for d := 0; d < 7; d++ {
		day := monday.AddDate(0, 0, d)
		dailyLinks = append(dailyLinks, fmt.Sprintf("- [[%s]] %s", day.Format("2006-01-02"), day.Weekday().String()))
	}

	// Check for custom template
	if m.config.WeeklyNoteTemplate != "" {
		absPath := filepath.Join(m.vault.Root, m.config.WeeklyNoteTemplate)
		if data, err := os.ReadFile(absPath); err == nil {
			content := string(data)
			content = strings.ReplaceAll(content, "{{week}}", weekStr)
			content = strings.ReplaceAll(content, "{{year}}", fmt.Sprintf("%d", year))
			content = strings.ReplaceAll(content, "{{week_number}}", fmt.Sprintf("%d", week))
			content = strings.ReplaceAll(content, "{{week_start}}", monday.Format("2006-01-02"))
			content = strings.ReplaceAll(content, "{{week_end}}", sunday.Format("2006-01-02"))
			content = strings.ReplaceAll(content, "{{daily_links}}", strings.Join(dailyLinks, "\n"))
			return content
		}
	}

	// Default template
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("week: %s\n", weekStr))
	sb.WriteString("type: weekly\n")
	sb.WriteString("tags: [weekly]\n")
	sb.WriteString("---\n\n")
	sb.WriteString(fmt.Sprintf("# Week %d — %s to %s\n\n", week, monday.Format("Jan 2"), sunday.Format("Jan 2")))

	// Daily links
	sb.WriteString("## Daily Notes\n")
	for _, link := range dailyLinks {
		sb.WriteString(link + "\n")
	}
	sb.WriteString("\n")

	sb.WriteString("## Week Goals\n- [ ] \n\n")
	sb.WriteString("## Review\n\n### What went well?\n\n\n### What could improve?\n\n\n### Key takeaways\n\n")

	return sb.String()
}

// isoWeekStart returns the Monday of the given ISO week.
func isoWeekStart(year, week int) time.Time {
	// January 4 is always in week 1
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.Local)
	_, w := jan4.ISOWeek()
	// Calculate Monday of week 1, then add weeks
	monday := jan4.AddDate(0, 0, -int(jan4.Weekday()-time.Monday))
	if jan4.Weekday() == time.Sunday {
		monday = jan4.AddDate(0, 0, -6)
	}
	return monday.AddDate(0, 0, (week-w)*7)
}

// replaceFrontmatter replaces existing YAML frontmatter in content with newFM,
// or prepends it if none exists.
func replaceFrontmatter(content, newFM string) string {
	if strings.HasPrefix(strings.TrimSpace(content), "---") {
		lines := strings.SplitN(content, "\n", -1)
		// Find end of existing frontmatter
		endIdx := -1
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				endIdx = i
				break
			}
		}
		if endIdx >= 0 {
			// Replace from line 0 through endIdx with newFM
			rest := strings.Join(lines[endIdx+1:], "\n")
			return newFM + "\n" + rest
		}
	}
	// No existing frontmatter — prepend
	return newFM + "\n" + content
}
