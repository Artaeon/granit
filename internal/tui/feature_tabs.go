package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// isFeatureTabPath reports whether a path produced by the
// tabbar (via CloseActive, NextTab, etc.) is a synthetic
// feature-tab path rather than a vault note path. Used by the
// tab-close handler to decide whether to call loadNote.
func isFeatureTabPath(path string) bool {
	return strings.HasPrefix(path, "feat:")
}

// This file holds the dispatch glue for the editor-tab migration
// (Phase 4). Each migrated overlay (TaskManager, DailyJot,
// Calendar, etc.) wires three branches here:
//
//   - renderFeatureTab — when the user's active tab is this
//     feature, render its View() in the editor pane (instead of
//     the editor or as a centered overlay).
//   - routeFeatureKey — keyboard messages get routed to the
//     feature's Update when its tab is active.
//   - closeFeature — Ctrl+W on a feature tab calls this so the
//     feature can drop its caches.
//
// Initially the switches are empty (or only contain pilots);
// migrating an overlay = one case per surface in each switch.
// Default cases preserve "render nothing / route to caller's
// fallback" so an unmigrated FeatureID can't blow up the app.

// hasActiveFeatureTab is a tiny helper so app_view.go doesn't
// have to handle the nil-guard at every call site. Exported via
// lower-case to keep the dispatch internal to this package.
func hasActiveFeatureTab(tb *TabBar) bool {
	if tb == nil {
		return false
	}
	_, ok := tb.ActiveFeature()
	return ok
}

// renderFeatureTab returns the rendered view for the given
// feature, sized to the editor pane. Empty string for unknown
// IDs — caller falls back to the welcome screen so the user
// doesn't see a blank editor.
func (m *Model) renderFeatureTab(id FeatureID, width, height int) string {
	switch id {
	case FeatTaskManager:
		m.taskManager.SetSize(width, height)
		return m.taskManager.View()
	case FeatDailyJot:
		m.dailyJot.SetSize(width, height)
		return m.dailyJot.View()
	case FeatCalendar:
		m.calendar.SetSize(width, height)
		return m.calendar.View()
	case FeatKanban:
		m.kanban.SetSize(width, height)
		return m.kanban.View()
	case FeatGoals:
		m.goalsMode.SetSize(width, height)
		return m.goalsMode.View()
	case FeatProject:
		m.projectMode.SetSize(width, height)
		return m.projectMode.View()
	case FeatGraph:
		m.graphView.SetSize(width, height)
		return m.graphView.View()
	}
	return ""
}

// routeFeatureKey dispatches a key message to the active feature
// tab's Update. Returns (model, cmd) — the caller decides
// whether the resulting model+cmd should propagate or be
// short-circuited.
//
// Returns ok=true when the key was actually routed (so the
// caller knows not to also run the legacy global-key switch),
// ok=false when the active feature has no migration yet (let
// the legacy paths handle it).
func (m *Model) routeFeatureKey(id FeatureID, msg tea.Msg) (Model, tea.Cmd, bool) {
	switch id {
	case FeatTaskManager:
		// Refresh task list if another component changed vault
		// files (mirrors the behavior of the legacy IsActive()
		// path that's now retired for TaskManager).
		if m.needsRefresh {
			m.taskManager.Refresh(m.vault)
			m.needsRefresh = false
		}
		var cmd tea.Cmd
		m.taskManager, cmd = m.taskManager.Update(msg)
		m.reportError("save task state", m.taskManager.ConsumeSaveError())
		if m.taskManager.WasFileChanged() {
			m.refreshComponents(m.taskManager.ActiveNotePath())
		}
		// Jump-to-source: closes the TaskManager tab and lands
		// the user on the requested note + line.
		if notePath, lineNum, ok := m.taskManager.GetJumpResult(); ok {
			if m.tabBar != nil {
				m.tabBar.CloseFeatureTab(FeatTaskManager)
			}
			m.taskManager.Close()
			m.loadNote(notePath)
			m.setSidebarCursorToFile(notePath)
			m.setFocus(focusEditor)
			if lineNum > 0 {
				m.editor.cursor = lineNum - 1
				m.editor.scroll = maxInt(0, lineNum-m.editor.height/2)
			}
		}
		// Focus session request — opens as a separate overlay
		// (focus session stays modal for now; it's transient).
		if task, ok := m.taskManager.GetFocusRequest(); ok {
			m.focusSession.SetSize(m.width, m.height)
			m.focusSession.OpenWithTask(m.vault.Root, task)
		}
		return *m, cmd, true

	case FeatDailyJot:
		var cmd tea.Cmd
		m.dailyJot, cmd = m.dailyJot.Update(msg)
		// DailyJot can close itself internally (e.g., user Esc)
		// — match the legacy behavior: load the promoted note
		// (if the user pressed the promote key on a jot) and
		// refresh the rest of the UI. Also close the tab when
		// the user dismisses internally so we don't leave a
		// dead-feature tab around.
		if !m.dailyJot.IsActive() {
			if notePath := m.dailyJot.GetPromotedNote(); notePath != "" {
				if m.tabBar != nil {
					m.tabBar.CloseFeatureTab(FeatDailyJot)
				}
				m.loadNote(notePath)
				m.setSidebarCursorToFile(notePath)
			} else if m.tabBar != nil {
				m.tabBar.CloseFeatureTab(FeatDailyJot)
			}
			m.refreshComponents("")
		}
		return *m, cmd, true
	}
	return *m, nil, false
}

// closeFeature is invoked when the user closes a feature tab
// via Ctrl+W (or any other tab-close path). The feature gets a
// chance to clear caches and reset its state. Unknown IDs
// silently do nothing.
func (m *Model) closeFeature(id FeatureID) {
	switch id {
	case FeatTaskManager:
		m.taskManager.Close()
	case FeatDailyJot:
		m.dailyJot.Close()
	case FeatCalendar:
		m.calendar.Close()
	case FeatKanban:
		m.kanban.Close()
	case FeatGoals:
		m.goalsMode.Close()
	case FeatProject:
		m.projectMode.Close()
	case FeatGraph:
		m.graphView.Close()
	}
}
