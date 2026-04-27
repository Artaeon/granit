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
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/artaeon/granit/internal/vault"
)

// renderSidebarPanel renders the file sidebar with the given border style and dimensions.
func (m Model) renderSidebarPanel(border lipgloss.Border, borderColor lipgloss.TerminalColor, w, h int) string {
	return SidebarStyle.BorderStyle(border).
		BorderForeground(borderColor).
		Width(w).
		Height(h).
		Render(m.sidebar.View())
}

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
	overhead++ // persistent action bar
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
		sidebarWidth = clampInt(m.width/5, 22, 35)
	}
	if showBacklinks {
		backlinksWidth = clampInt(m.width/5, 22, 30)
	}
	if showCalPanel {
		calPanelWidth = clampInt(m.width/4, 28, 35)
	}
	if showOutline {
		outlineWidth = clampInt(m.width/7, 18, 25)
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

	// Focus-aware active/inactive styling (Border, Dimming, Thickness)
	sidebarBorderColor := surface0
	editorBorderColor := surface0
	backlinksBorderColor := surface0

	sidebarBorder := lipgloss.NormalBorder()
	editorBorder := lipgloss.NormalBorder()
	backlinksBorder := lipgloss.NormalBorder()

	switch m.focus {
	case focusSidebar:
		sidebarBorderColor = FocusedBorderColor
		sidebarBorder = lipgloss.RoundedBorder()
	case focusEditor:
		editorBorderColor = FocusedBorderColor
		editorBorder = lipgloss.ThickBorder() // thicker!
	case focusBacklinks:
		backlinksBorderColor = FocusedBorderColor
		backlinksBorder = lipgloss.RoundedBorder()
	}

	// Tab bar
	var tabBarStr string
	if m.tabBar != nil && len(m.tabBar.Tabs()) > 0 {
		m.tabBar.SetModified(m.activeNote, m.editor.modified)
		tabBarStr = m.tabBar.Render(editorWidth, m.activeNote)
	}

	// Editor pane: feature tab, view mode, edit mode, or welcome.
	//
	// Feature tabs (TaskManager etc.) take precedence — when one
	// is the active tab, its surface renders in place of the
	// editor instead of layering as a popup. activeNote stays
	// pinned to whichever note tab is open underneath, so when
	// the user closes the feature tab they land back on their
	// note.
	var editorContent string
	switch {
	case m.tabBar != nil && hasActiveFeatureTab(m.tabBar):
		id, _ := m.tabBar.ActiveFeature()
		editorContent = m.renderFeatureTab(id, editorWidth, contentHeight)
	case m.activeNote == "":
		editorContent = m.renderWelcomeScreen(editorWidth, contentHeight)
	case m.viewMode:
		editorContent = m.renderViewMode()
	default:
		editorContent = m.editor.View()
	}

	// Zen layout hides tab bar for distraction-free writing
	if layout == "zen" {
		tabBarStr = ""
	}

	// Combine tab bar + editor
	editorPanel := editorContent
	if tabBarStr != "" {
		editorPanel = tabBarStr + "\n" + editorContent
	}

	editor := EditorStyle.
		BorderStyle(editorBorder).
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
		case "writer":
			sidebar := m.renderSidebarPanel(sidebarBorder, sidebarBorderColor, sidebarWidth, contentHeight)
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
				backlinks := BacklinksStyle.BorderStyle(backlinksBorder).
					BorderForeground(backlinksBorderColor).
					Width(backlinksWidth).
					Height(contentHeight).
					Render(m.backlinks.View())
				content = lipgloss.JoinHorizontal(lipgloss.Top, editor, backlinks)
			}
		case "minimal", "zen":
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
				Height(contentHeight).
				Render(zenEditor)
		case "dashboard": // 4-panel: sidebar | editor | outline | backlinks
			var leftPanels, rightPanels []string
			if sidebarWidth > 0 {
				sidebar := m.renderSidebarPanel(sidebarBorder, sidebarBorderColor, sidebarWidth, contentHeight)
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
				backlinks := BacklinksStyle.BorderStyle(backlinksBorder).
					BorderForeground(backlinksBorderColor).
					Width(backlinksWidth).
					Height(contentHeight).
					Render(m.backlinks.View())
				rightPanels = append(rightPanels, backlinks)
			}
			panels := append(leftPanels, editor)
			panels = append(panels, rightPanels...)
			content = lipgloss.JoinHorizontal(lipgloss.Top, panels...)
		case "taskboard", "calendar", "cockpit":
			sidebar := m.renderSidebarPanel(sidebarBorder, sidebarBorderColor, sidebarWidth, contentHeight)

			// Right panel: calendar + tasks stacked vertically
			rightWidth := clampInt(m.width/4, 28, 40)

			topHeight := contentHeight / 2
			botHeight := contentHeight - topHeight - 2
			if botHeight < 4 {
				botHeight = 4
			}

			// Calendar section
			m.calendarPanel.SetSize(rightWidth, topHeight)
			calContent := m.calendarPanel.View()
			calPanel := lipgloss.NewStyle().
				BorderStyle(PanelBorder).
				BorderForeground(surface1).
				Width(rightWidth).
				Height(topHeight).
				Render(calContent)

			// Tasks section — use the real task parser
			var taskBuf strings.Builder
			taskBuf.WriteString(lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  TASKS") + "\n")
			taskBuf.WriteString(DimStyle.Render(strings.Repeat("─", rightWidth-4)) + "\n")

			allTasks := m.cachedTasks
			overdueCount := 0
			todayCount := 0
			upcomingCount := 0

			// Categorize tasks in a single pass
			var overdueLines, todayLines []string
			for _, t := range allTasks {
				if t.Done || t.DueDate == "" {
					continue
				}
				text := TruncateDisplay(tmCleanText(t.Text), rightWidth-8)
				if tmIsOverdue(t.DueDate) {
					overdueCount++
					if overdueCount <= 5 {
						overdueLines = append(overdueLines, "  "+lipgloss.NewStyle().Foreground(red).Render("✗ "+text))
					}
				} else if tmIsToday(t.DueDate) {
					todayCount++
					if todayCount <= 5 {
						todayLines = append(todayLines, "  "+lipgloss.NewStyle().Foreground(yellow).Render("○ "+text))
					}
				} else {
					upcomingCount++
				}
			}

			taskBuf.WriteString(lipgloss.NewStyle().Foreground(red).Bold(true).Render("  Overdue") + "\n")
			if len(overdueLines) == 0 {
				taskBuf.WriteString("  " + DimStyle.Render("none") + "\n")
			} else {
				taskBuf.WriteString(strings.Join(overdueLines, "\n") + "\n")
				if overdueCount > 5 {
					taskBuf.WriteString("  " + DimStyle.Render(fmt.Sprintf("  +%d more", overdueCount-5)) + "\n")
				}
			}

			taskBuf.WriteString("\n" + lipgloss.NewStyle().Foreground(yellow).Bold(true).Render("  Today") + "\n")
			if len(todayLines) == 0 {
				taskBuf.WriteString("  " + DimStyle.Render("none") + "\n")
			} else {
				taskBuf.WriteString(strings.Join(todayLines, "\n") + "\n")
			}

			taskBuf.WriteString("\n" + DimStyle.Render(strings.Repeat("─", rightWidth-4)) + "\n")
			taskBuf.WriteString(fmt.Sprintf("  %s %d overdue  %s %d today  %s %d upcoming",
				lipgloss.NewStyle().Foreground(red).Render("●"), overdueCount,
				lipgloss.NewStyle().Foreground(yellow).Render("●"), todayCount,
				lipgloss.NewStyle().Foreground(green).Render("●"), upcomingCount) + "\n")

			taskPanel := lipgloss.NewStyle().
				BorderStyle(PanelBorder).
				BorderForeground(surface1).
				Width(rightWidth).
				Height(botHeight).
				Background(base).
				Padding(0, 1).
				Render(taskBuf.String())

			rightSide := lipgloss.JoinVertical(lipgloss.Left, calPanel, taskPanel)

			// Adjust editor width
			cpEditorWidth := m.width - sidebarWidth - rightWidth - 6
			if cpEditorWidth < 30 {
				cpEditorWidth = 30
			}
			cpEditor := EditorStyle.
				BorderStyle(editorBorder).
				BorderForeground(editorBorderColor).
				Width(cpEditorWidth).
				Height(contentHeight).
				Render(editorPanel)

			if m.config.SidebarPosition == "right" {
				content = lipgloss.JoinHorizontal(lipgloss.Top, rightSide, cpEditor, sidebar)
			} else {
				content = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, cpEditor, rightSide)
			}
		case "stacked":
			sidebar := m.renderSidebarPanel(sidebarBorder, sidebarBorderColor, sidebarWidth, contentHeight)

			rightWidth := m.width - sidebarWidth - 4
			if rightWidth < 30 {
				rightWidth = 30
			}

			stTopHeight := contentHeight * 2 / 3
			stBotHeight := contentHeight - stTopHeight - 2
			if stBotHeight < 4 {
				stBotHeight = 4
			}

			topEditor := EditorStyle.
				BorderStyle(editorBorder).
				BorderForeground(editorBorderColor).
				Width(rightWidth).
				Height(stTopHeight).
				Render(editorPanel)

			// Bottom: outline left, backlinks right
			outW := rightWidth / 3
			if outW < 15 {
				outW = 15
			}
			blW := rightWidth - outW - 2
			if blW < 15 {
				blW = 15
			}

			outlinePanelContent := m.outline.RenderPanel(m.editor.GetContent(), outW, stBotHeight)
			outlinePanel := lipgloss.NewStyle().
				BorderStyle(PanelBorder).
				BorderForeground(surface1).
				Width(outW).
				Height(stBotHeight).
				Render(outlinePanelContent)

			backlinksPanel := BacklinksStyle.BorderStyle(backlinksBorder).
				BorderForeground(backlinksBorderColor).
				Width(blW).
				Height(stBotHeight).
				Render(m.backlinks.View())

			bottomBar := lipgloss.JoinHorizontal(lipgloss.Top, outlinePanel, backlinksPanel)
			rightSide := lipgloss.JoinVertical(lipgloss.Left, topEditor, bottomBar)

			if m.config.SidebarPosition == "right" {
				content = lipgloss.JoinHorizontal(lipgloss.Top, rightSide, sidebar)
			} else {
				content = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, rightSide)
			}
		case "cornell":
			sidebar := m.renderSidebarPanel(sidebarBorder, sidebarBorderColor, sidebarWidth, contentHeight)

			// Vertical split: editor (2/3) over notes panel (1/3)
			topHeight := contentHeight * 2 / 3
			bottomHeight := contentHeight - topHeight - 2 // -2 for border
			if bottomHeight < 4 {
				bottomHeight = 4
			}
			rightWidth := m.width - sidebarWidth - 4 // -4 for borders
			if rightWidth < 30 {
				rightWidth = 30
			}

			topEditor := EditorStyle.
				BorderStyle(editorBorder).
				BorderForeground(editorBorderColor).
				Width(rightWidth).
				Height(topHeight).
				Render(editorPanel)

			// Build notes panel content: outline + backlinks
			var cornellNotes strings.Builder
			cornellNotes.WriteString(lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  NOTES & SUMMARY"))
			cornellNotes.WriteString("\n")
			cornellNotes.WriteString(DimStyle.Render(strings.Repeat("─", rightWidth-4)))
			cornellNotes.WriteString("\n")

			// Outline headings
			cornellNotes.WriteString(lipgloss.NewStyle().Foreground(blue).Bold(true).Render("  Outline") + "\n")
			for _, line := range strings.Split(m.editor.GetContent(), "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "# ") {
					cornellNotes.WriteString("  " + lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(trimmed) + "\n")
				} else if strings.HasPrefix(trimmed, "## ") {
					cornellNotes.WriteString("    " + lipgloss.NewStyle().Foreground(blue).Render(trimmed) + "\n")
				} else if strings.HasPrefix(trimmed, "### ") {
					cornellNotes.WriteString("      " + lipgloss.NewStyle().Foreground(teal).Render(trimmed) + "\n")
				}
			}

			// Backlinks
			cornellNotes.WriteString("\n" + lipgloss.NewStyle().Foreground(green).Bold(true).Render("  Backlinks") + "\n")
			if m.activeNote != "" {
				bls := m.index.GetBacklinks(m.activeNote)
				if len(bls) == 0 {
					cornellNotes.WriteString("  " + DimStyle.Render("none") + "\n")
				}
				for _, bl := range bls {
					name := filepath.Base(bl)
					name = strings.TrimSuffix(name, ".md")
					cornellNotes.WriteString("  " + lipgloss.NewStyle().Foreground(green).Render("← "+name) + "\n")
				}
			}

			bottomPanel := lipgloss.NewStyle().
				BorderStyle(PanelBorder).
				BorderForeground(surface1).
				Width(rightWidth).
				Height(bottomHeight).
				Background(base).
				Padding(0, 1).
				Render(cornellNotes.String())

			rightSide := lipgloss.JoinVertical(lipgloss.Left, topEditor, bottomPanel)
			if m.config.SidebarPosition == "right" {
				content = lipgloss.JoinHorizontal(lipgloss.Top, rightSide, sidebar)
			} else {
				content = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, rightSide)
			}
		case "focus":
			sidebar := m.renderSidebarPanel(sidebarBorder, sidebarBorderColor, sidebarWidth, contentHeight)

			// Wide centered editor (max 100 chars), no backlinks
			focusEditorWidth := m.width - sidebarWidth - 4
			maxFocusWidth := 100
			if focusEditorWidth > maxFocusWidth {
				leftPad := (focusEditorWidth - maxFocusWidth) / 2
				focusEditor := lipgloss.NewStyle().
					Width(maxFocusWidth).
					Height(contentHeight).
					Background(base).
					Padding(0, 1).
					Render(editorPanel)
				rightArea := lipgloss.NewStyle().
					PaddingLeft(leftPad).
					Width(focusEditorWidth).
					Height(contentHeight).
					Render(focusEditor)
				if m.config.SidebarPosition == "right" {
					content = lipgloss.JoinHorizontal(lipgloss.Top, rightArea, sidebar)
				} else {
					content = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, rightArea)
				}
			} else {
				// Terminal too narrow for centering — just use full width
				focusEditor := EditorStyle.
					BorderStyle(editorBorder).
					BorderForeground(editorBorderColor).
					Width(focusEditorWidth).
					Height(contentHeight).
					Render(editorPanel)
				if m.config.SidebarPosition == "right" {
					content = lipgloss.JoinHorizontal(lipgloss.Top, focusEditor, sidebar)
				} else {
					content = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, focusEditor)
				}
			}
		case "widescreen":
			sidebar := m.renderSidebarPanel(sidebarBorder, sidebarBorderColor, sidebarWidth, contentHeight)

			// Panel widths
			outW := clampInt(m.width/8, 16, 22)
			blW := clampInt(m.width/7, 18, 25)
			calW := clampInt(m.width/7, 20, 28)

			wsEditorWidth := m.width - sidebarWidth - outW - blW - calW - 10
			if wsEditorWidth < 30 {
				wsEditorWidth = 30
			}

			outlinePanelContent := m.outline.RenderPanel(m.editor.GetContent(), outW, contentHeight)
			outlinePanel := lipgloss.NewStyle().
				BorderStyle(PanelBorder).
				BorderForeground(surface1).
				Width(outW).
				Height(contentHeight).
				Render(outlinePanelContent)

			wsEditor := EditorStyle.
				BorderStyle(editorBorder).
				BorderForeground(editorBorderColor).
				Width(wsEditorWidth).
				Height(contentHeight).
				Render(editorPanel)

			backlinks := BacklinksStyle.BorderStyle(backlinksBorder).
				BorderForeground(backlinksBorderColor).
				Width(blW).
				Height(contentHeight).
				Render(m.backlinks.View())

			m.calendarPanel.SetSize(calW, contentHeight)
			calPanel := lipgloss.NewStyle().
				BorderStyle(PanelBorder).
				BorderForeground(surface1).
				Width(calW).
				Height(contentHeight).
				Render(m.calendarPanel.View())

			content = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, outlinePanel, wsEditor, backlinks, calPanel)
		case "kanban":
			sidebar := m.renderSidebarPanel(sidebarBorder, sidebarBorderColor, sidebarWidth, contentHeight)

			kbWidth := clampInt(m.width/3, 30, 50)

			// Build mini kanban: 3 columns (Todo, Doing, Done)
			colW := (kbWidth - 6) / 3
			if colW < 8 {
				colW = 8
			}

			// Use cached task data (refreshed on save/vault change)
			kbTasks := m.cachedTasks

			var todoBuf, doingBuf, doneBuf strings.Builder
			todoBuf.WriteString(lipgloss.NewStyle().Foreground(yellow).Bold(true).Render(" Todo") + "\n")
			todoBuf.WriteString(DimStyle.Render(strings.Repeat("─", colW-2)) + "\n")
			doingBuf.WriteString(lipgloss.NewStyle().Foreground(blue).Bold(true).Render(" Doing") + "\n")
			doingBuf.WriteString(DimStyle.Render(strings.Repeat("─", colW-2)) + "\n")
			doneBuf.WriteString(lipgloss.NewStyle().Foreground(green).Bold(true).Render(" Done") + "\n")
			doneBuf.WriteString(DimStyle.Render(strings.Repeat("─", colW-2)) + "\n")

			todoCount, doingCount, doneCount := 0, 0, 0
			for _, t := range kbTasks {
				text := TruncateDisplay(tmCleanText(t.Text), colW-3)
				if t.Done {
					doneCount++
					if doneCount <= 8 {
						doneBuf.WriteString(lipgloss.NewStyle().Foreground(green).Render(" ✓ "+text) + "\n")
					}
				} else {
					// Check for #doing or #wip tag
					isDoing := false
					for _, tag := range t.Tags {
						if tag == "doing" || tag == "wip" || tag == "progress" {
							isDoing = true
							break
						}
					}
					if isDoing {
						doingCount++
						if doingCount <= 8 {
							doingBuf.WriteString(lipgloss.NewStyle().Foreground(blue).Render(" ◉ "+text) + "\n")
						}
					} else {
						todoCount++
						if todoCount <= 8 {
							todoBuf.WriteString(lipgloss.NewStyle().Foreground(yellow).Render(" ○ "+text) + "\n")
						}
					}
				}
			}

			// Count footers
			todoBuf.WriteString("\n" + DimStyle.Render(fmt.Sprintf(" %d items", todoCount)) + "\n")
			doingBuf.WriteString("\n" + DimStyle.Render(fmt.Sprintf(" %d items", doingCount)) + "\n")
			doneBuf.WriteString("\n" + DimStyle.Render(fmt.Sprintf(" %d items", doneCount)) + "\n")

			todoCol := lipgloss.NewStyle().Width(colW).Render(todoBuf.String())
			doingCol := lipgloss.NewStyle().Width(colW).Render(doingBuf.String())
			doneCol := lipgloss.NewStyle().Width(colW).Render(doneBuf.String())

			kbContent := lipgloss.JoinHorizontal(lipgloss.Top, todoCol, doingCol, doneCol)
			kbPanel := lipgloss.NewStyle().
				BorderStyle(PanelBorder).
				BorderForeground(surface1).
				Width(kbWidth).
				Height(contentHeight).
				Background(base).
				Padding(0, 1).
				Render(kbContent)

			kbEditorWidth := m.width - sidebarWidth - kbWidth - 6
			if kbEditorWidth < 30 {
				kbEditorWidth = 30
			}
			kbEditor := EditorStyle.
				BorderStyle(editorBorder).
				BorderForeground(editorBorderColor).
				Width(kbEditorWidth).
				Height(contentHeight).
				Render(editorPanel)

			if m.config.SidebarPosition == "right" {
				content = lipgloss.JoinHorizontal(lipgloss.Top, kbPanel, kbEditor, sidebar)
			} else {
				content = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, kbEditor, kbPanel)
			}
		case "presenter":
			// Full-width centered rendered markdown, no borders
			maxW := 100
			renderW := m.width - 4
			if renderW > maxW {
				renderW = maxW
			}
			if renderW < 30 {
				renderW = 30
			}
			m.renderer.SetSize(renderW, contentHeight)
			rendered := m.renderer.Render(m.editor.GetContent(), m.viewScroll)

			presenterPanel := lipgloss.NewStyle().
				Width(renderW).
				Height(contentHeight).
				Background(base).
				Padding(0, 2).
				Render(rendered)

			leftPad := (m.width - renderW - 4) / 2
			if leftPad < 0 {
				leftPad = 0
			}
			content = lipgloss.NewStyle().
				PaddingLeft(leftPad).
				Height(contentHeight).
				Render(presenterPanel)
		case "preview":
			// Editor on left, rendered markdown preview on right
			halfW := (m.width - 4) / 2
			if halfW < 30 {
				halfW = 30
			}

			previewEditor := EditorStyle.
				BorderStyle(editorBorder).
				BorderForeground(editorBorderColor).
				Width(halfW).
				Height(contentHeight).
				Render(editorPanel)

			// Render markdown preview
			m.renderer.SetSize(halfW-2, contentHeight)
			rendered := m.renderer.Render(m.editor.GetContent(), m.viewScroll)
			previewPanel := lipgloss.NewStyle().
				BorderStyle(PanelBorder).
				BorderForeground(surface1).
				Width(halfW).
				Height(contentHeight).
				Background(base).
				Render(rendered)

			content = lipgloss.JoinHorizontal(lipgloss.Top, previewEditor, previewPanel)
		default: // "default", "research" - 3-panel (with optional calendar toggle)
			sidebar := m.renderSidebarPanel(sidebarBorder, sidebarBorderColor, sidebarWidth, contentHeight)
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
				backlinks := BacklinksStyle.BorderStyle(backlinksBorder).
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
		m.statusbar.SetFocusSessionStatus(m.focusSession.StatusString())
		m.statusbar.SetClockInStatus(m.clockIn.StatusString())
		m.statusbar.SetDirty(m.editor.modified)
		if layout == "zen" || layout == "minimal" || layout == "presenter" {
			// Minimal status for zen/presenter: just filename + word count, no help bar
			zenInfo := lipgloss.NewStyle().Foreground(surface1).Render(
				"  " + m.activeNote + "  " + fmt.Sprintf("%d words", m.editor.GetWordCount()))
			zenBar := lipgloss.NewStyle().Background(surface0).Width(m.width).Render(zenInfo)
			view = lipgloss.JoinVertical(lipgloss.Left, content, zenBar)
		} else {
			actionBar := m.renderActionBar(m.width)
			status := m.statusbar.View()
			if breadcrumbBar != "" {
				view = lipgloss.JoinVertical(lipgloss.Left, content, actionBar, breadcrumbBar, status)
			} else {
				view = lipgloss.JoinVertical(lipgloss.Left, content, actionBar, status)
			}
		}
	}

	// Safety: truncate content to terminal height, preserving status bar at bottom.
	// Dynamically compute status bar height (2 normally, 3 with toast notification).
	if viewLines := strings.Split(view, "\n"); len(viewLines) > m.height {
		statusLines := strings.Count(m.statusbar.View(), "\n") + 1
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
	// Graph retired from overlay rendering in Phase 4 — feature tab.
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
	// Calendar retired from overlay rendering in Phase 4 — now
	// a feature tab in the editor pane.
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
	if m.habitTracker.IsActive() && !(m.tabBar != nil && m.tabBar.HasFeatureTab(FeatHabits)) {
		// Habit tracker now opens as an editor tab; skip the
		// centered-overlay render path when the feature tab is
		// the foreground — without this guard, the surface
		// rendered TWICE (once in the editor pane, once as the
		// overlay on top of it). Keep the overlay path for
		// callers that still open habit tracker without going
		// through CmdHabitTracker (none today, but defensive).
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
	// Goals retired from overlay rendering in Phase 4 — feature tab.
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
	if m.dailyHub.IsActive() {
		ctx := m.buildWidgetCtx()
		overlay := m.dailyHub.Render(m.width-6, m.height-6, ctx)
		view = m.overlayCenter(view, overlay)
	}
	if m.profilePicker.IsActive() {
		view = m.overlayCenter(view, m.profilePicker.View())
	}
	if m.triageQueue.IsActive() {
		view = m.overlayCenter(view, m.triageQueue.View())
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
	if m.blogDraft.IsActive() {
		overlay := m.blogDraft.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.taskTriage.IsActive() {
		overlay := m.taskTriage.View()
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
	// Projects retired from overlay rendering in Phase 4 — feature tab.
	if m.projectDashboard.IsActive() {
		overlay := m.projectDashboard.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.commandCenter.IsActive() && featureTabIsForeground(m.tabBar, FeatCommandCenter) && !(m.tabBar != nil && hasActiveFeatureTab(m.tabBar)) {
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
	// Kanban retired from overlay rendering in Phase 4 — feature tab.
	if m.vaultRefactor.IsActive() {
		overlay := m.vaultRefactor.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.dailyBriefing.IsActive() {
		overlay := m.dailyBriefing.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.devotional.IsActive() {
		overlay := m.devotional.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.morningRoutine.IsActive() {
		overlay := m.morningRoutine.View()
		view = m.overlayCenter(view, overlay)
	}
	// DailyJot retired from overlay rendering in Phase 4 — now
	// a feature tab in the editor pane (see renderFeatureTab).
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
	// TaskManager retired from overlay rendering in Phase 4 —
	// it now lives as a feature tab in the editor pane (see
	// app_view.go editorContent branch + feature_tabs.go
	// renderFeatureTab). Keeping a guard comment here so a
	// future audit doesn't reintroduce the overlay path.
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
	if m.layoutPicker.IsActive() {
		overlay := m.layoutPicker.View()
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
	if m.statusTray.IsActive() {
		overlay := m.statusTray.View()
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

	shadowColor := lipgloss.Color("#11111B")
	shadowStyle := lipgloss.NewStyle().Background(shadowColor).Foreground(shadowColor)

	for i, overlayLine := range overlayLines {
		y := startY + i
		if y >= len(result) {
			break
		}
		overlayW := lipgloss.Width(overlayLine)
		marginLen := overlayWidth - overlayW
		if marginLen < 0 {
			marginLen = 0
		}
		margin := strings.Repeat(" ", marginLen)

		rightFill := ""
		skipRight := startX + overlayWidth
		if i > 0 {
			rightFill = "  "
			skipRight += 2
		}

		left := ansiTakeCols(result[y], startX)
		right := ansiSkipCols(result[y], skipRight)
		lineStr := left + overlayLine + margin
		if rightFill != "" {
			lineStr += shadowStyle.Render(rightFill)
		}
		result[y] = lineStr + right
	}

	bottomY := startY + len(overlayLines)
	if bottomY < len(result) {
		if overlayWidth > 0 {
			bottomPad := strings.Repeat(" ", startX+2)
			bottomFill := strings.Repeat(" ", overlayWidth)
			right := ansiSkipCols(result[bottomY], startX+2+overlayWidth)
			result[bottomY] = bottomPad + shadowStyle.Render(bottomFill) + right
		}
	}

	return strings.Join(result, "\n")
}

func (m Model) renderViewMode() string {
	var b strings.Builder
	contentWidth := m.editor.width - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Header: reading bar with title, reading time, and progress
	vmTotal := m.renderer.RenderLineCount(m.editor.GetContent())
	vmVP := m.renderer.height - 4
	if vmVP < 1 {
		vmVP = 1
	}
	progress := 0
	if vmTotal > vmVP {
		progress = m.viewScroll * 100 / (vmTotal - vmVP)
		if progress > 100 {
			progress = 100
		}
	} else {
		progress = 100
	}

	// Progress bar: thin line with filled portion
	barWidth := contentWidth - 2
	if barWidth < 10 {
		barWidth = 10
	}
	filled := barWidth * progress / 100
	barStyle := lipgloss.NewStyle().Foreground(mauve)
	dimBar := lipgloss.NewStyle().Foreground(surface0)
	b.WriteString(" " + barStyle.Render(strings.Repeat("━", filled)) + dimBar.Render(strings.Repeat("─", barWidth-filled)))
	b.WriteString("\n")

	rendered := m.renderer.Render(m.editor.GetContent(), m.viewScroll)
	b.WriteString(rendered)

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

// renderWelcomeScreen shows a help screen when no note is open.
func (m Model) renderWelcomeScreen(width, height int) string {
	if m.vault != nil {
		cc := m.commandCenter
		cc.SetSize(width, height)
		return cc.InlineView(width, height)
	}

	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	headingStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(subtext0)
	dimStyle := lipgloss.NewStyle().Foreground(overlay0)

	renderSection := func(items []struct{ key, desc string }) string {
		var s strings.Builder
		for _, item := range items {
			s.WriteString(fmt.Sprintf("  %s  %s\n",
				keyStyle.Render(fmt.Sprintf("%-12s", item.key)),
				descStyle.Render(item.desc)))
		}
		return s.String()
	}

	var b strings.Builder

	// Center content vertically
	contentLines := 26
	topPad := (height - contentLines) / 3
	if topPad < 1 {
		topPad = 1
	}
	for i := 0; i < topPad; i++ {
		b.WriteString("\n")
	}

	// Title
	b.WriteString("  " + titleStyle.Render("Welcome to Granit") + "\n")
	b.WriteString("  " + dimStyle.Render("Your terminal-native note-taking system") + "\n\n")

	// Getting started
	b.WriteString("  " + headingStyle.Render("Getting Started") + "\n")
	b.WriteString("  " + dimStyle.Render(strings.Repeat("─", 36)) + "\n")
	b.WriteString(renderSection([]struct{ key, desc string }{
		{"Ctrl+N", "Create a new note"},
		{"Enter", "Open selected note in explorer"},
		{"/", "Search files in explorer"},
		{"Ctrl+P", "Quick open (search all notes)"},
		{"Alt+D", "Open today's daily note"},
	}))

	b.WriteString("\n")
	b.WriteString("  " + headingStyle.Render("Navigation") + "\n")
	b.WriteString("  " + dimStyle.Render(strings.Repeat("─", 36)) + "\n")
	b.WriteString(renderSection([]struct{ key, desc string }{
		{"Tab", "Switch between panels"},
		{"Ctrl+E", "Toggle edit / view mode"},
		{"Ctrl+K", "Open task manager"},
		{"Ctrl+X", "Open command palette"},
		{"Ctrl+R", "Open AI bots"},
		{"F5", "Show all keyboard shortcuts"},
	}))

	b.WriteString("\n")
	b.WriteString("  " + headingStyle.Render("Explorer") + "\n")
	b.WriteString("  " + dimStyle.Render(strings.Repeat("─", 36)) + "\n")
	b.WriteString(renderSection([]struct{ key, desc string }{
		{"z / Z", "Collapse / expand all folders"},
		{"\u2190 / \u2192", "Fold / unfold folder"},
		{"n", "New note  |  d  Delete note"},
		{"r", "Rename note"},
	}))

	b.WriteString("\n")
	b.WriteString("  " + dimStyle.Render("Select a note from the explorer or press Ctrl+N to get started.") + "\n")

	return b.String()
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
		BorderStyle(PanelBorder).
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
		BorderStyle(PanelBorder).
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
		BorderStyle(PanelBorder).
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
		BorderStyle(PanelBorder).
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
		BorderStyle(PanelBorder).
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
		m.reportInfo("Replaced %d occurrences", count)
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
		m.reportInfo("Replaced %d occurrences", count)
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
			m.newFolderName = TrimLastRune(m.newFolderName)
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
		BorderStyle(PanelBorder).
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
		BorderStyle(PanelBorder).
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

	shadowColor := lipgloss.Color("#11111B")
	shadowStyle := lipgloss.NewStyle().Background(shadowColor).Foreground(shadowColor)

	for i, overlayLine := range overlayLines {
		y := startY + i
		if y >= len(result) {
			break
		}
		overlayW := lipgloss.Width(overlayLine)
		marginLen := overlayWidth - overlayW
		if marginLen < 0 {
			marginLen = 0
		}
		margin := strings.Repeat(" ", marginLen)

		rightFill := ""
		skipRight := startX + overlayWidth
		if i > 0 {
			rightFill = "  "
			skipRight += 2
		}

		left := ansiTakeCols(result[y], startX)
		right := ansiSkipCols(result[y], skipRight)
		lineStr := left + overlayLine + margin
		if rightFill != "" {
			lineStr += shadowStyle.Render(rightFill)
		}
		result[y] = lineStr + right
	}

	bottomY := startY + len(overlayLines)
	if bottomY < len(result) {
		if overlayWidth > 0 {
			bottomPad := strings.Repeat(" ", startX+2)
			bottomFill := strings.Repeat(" ", overlayWidth)
			right := ansiSkipCols(result[bottomY], startX+2+overlayWidth)
			result[bottomY] = bottomPad + shadowStyle.Render(bottomFill) + right
		}
	}

	return strings.Join(result, "\n")
}

// ansiTakeCols returns the prefix of s up to n visual columns.
func ansiTakeCols(s string, n int) string {
	width := 0
	i := 0
	for i < len(s) && width < n {
		if s[i] == '\x1b' {
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
			r, size := utf8.DecodeRuneInString(s[i:])
			w := runewidth.RuneWidth(r)
			if width+w > n {
				break
			}
			width += w
			i += size
		}
	}
	if i >= len(s) {
		return s
	}
	return s[:i] + "\x1b[0m"
}

// ansiSkipCols returns the suffix of s after skipping n visual columns.
// ANSI escape sequences are correctly skipped without counting as width.
// A reset sequence is prepended to prevent color bleed from the skipped portion.
func ansiSkipCols(s string, n int) string {
	width := 0
	i := 0
	for i < len(s) && width < n {
		if s[i] == '\x1b' {
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
			r, size := utf8.DecodeRuneInString(s[i:])
			width += runewidth.RuneWidth(r)
			i += size
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
		m.reportError("add tags (read)", err)
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
				m.reportError("add tags", atomicWriteNote(path, strings.Join(newLines, "\n")))
			}
			return
		}
	}

	fm := "---\ntags: [" + strings.Join(tags, ", ") + "]\n---\n\n"
	m.reportError("add tags", atomicWriteNote(path, fm+content))
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
			m.reportError("encrypt note", err)
			return
		}
		newName := m.encryption.EncryptedName(m.activeNote)
		newPath := filepath.Join(m.vault.Root, newName)
		// Atomic write: tmp + rename. If interrupted partway through, neither
		// the encrypted nor the plaintext copy is lost.
		tmpPath := newPath + ".tmp"
		if err := os.WriteFile(tmpPath, []byte(encrypted), 0644); err != nil {
			_ = os.Remove(tmpPath)
			m.reportError("write encrypted file", err)
			return
		}
		if err := os.Rename(tmpPath, newPath); err != nil {
			_ = os.Remove(tmpPath)
			m.reportError("write encrypted file", err)
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
		// Atomic write: tmp + rename so a partial write cannot leave a
		// truncated plaintext file while we delete the ciphertext below.
		tmpPath := newPath + ".tmp"
		if err := os.WriteFile(tmpPath, []byte(decrypted), 0644); err != nil {
			_ = os.Remove(tmpPath)
			m.reportError("write decrypted file", err)
			return
		}
		if err := os.Rename(tmpPath, newPath); err != nil {
			_ = os.Remove(tmpPath)
			m.reportError("write decrypted file", err)
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
	folder := m.config.DailyNotesFolder
	if folder != "" {
		dailyName = filepath.Join(folder, dailyName)
	}
	dailyPath := filepath.Join(m.vault.Root, dailyName)

	existing, err := os.ReadFile(dailyPath)
	var writeErr error
	if err != nil {
		if mkErr := os.MkdirAll(filepath.Dir(dailyPath), 0755); mkErr != nil {
			m.reportError("create daily note folder", mkErr)
			return
		}
		fallback := fmt.Sprintf("---\ndate: %s\ntype: daily\ntags: [daily]\n---\n\n# %s — {{weekday}}\n\n%s\n", today, today, briefingContent)
		content := m.dailyNoteContent(today, fallback)
		writeErr = atomicWriteNote(dailyPath, content)
	} else {
		// Use replaceDailySection to prevent duplicate briefings on re-run
		newContent := replaceDailySection(string(existing), briefingContent, "## Morning Briefing")
		writeErr = atomicWriteNote(dailyPath, newContent)
	}
	if writeErr != nil {
		m.reportError("write daily briefing", writeErr)
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
			m.reportError("create daily note folder", err)
			return
		}
		fallback := m.buildDailyFallback(today)
		content := m.dailyNoteContent(today, fallback)
		if err := atomicWriteNote(path, content); err != nil {
			m.reportError("create daily note", err)
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
	allTasks := m.currentTasks()
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
