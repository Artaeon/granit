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

// isPassthroughChord reports whether a key chord must bypass
// any active feature tab's Update and reach the global key
// dispatcher. These are the keys a power user expects to ALWAYS
// work regardless of focus — without this list, opening
// TaskManager as a tab would trap the user because Ctrl+W
// (close tab), Ctrl+P (palette), Ctrl+Tab (cycle), etc. would
// all be consumed by taskManager.Update and never reach their
// handlers.
//
// Add to this list with care — anything in here CANNOT be used
// as a feature-internal binding. The list deliberately stays
// minimal: tab management, palette, quit, save, and the
// shortcut chords for opening features (so the user can switch
// to a different feature from inside one).
func isPassthroughChord(key string) bool {
	switch key {
	// Tab management
	case "ctrl+w",
		"ctrl+tab", "ctrl+shift+tab",
		"ctrl+1", "ctrl+2", "ctrl+3", "ctrl+4", "ctrl+5",
		"ctrl+6", "ctrl+7", "ctrl+8", "ctrl+9",
		"ctrl+shift+t":
		return true
	// Palette + quit + save
	case "ctrl+p", "ctrl+q", "ctrl+c", "ctrl+s":
		return true
	// Feature-opening shortcuts — let the user switch from
	// inside one feature to another without first closing.
	case "alt+h",     // Daily Hub
		"alt+j",      // Daily Jot
		"alt+m",      // Morning Routine
		"alt+b",      // Habit Tracker
		"alt+i",      // Quick Capture
		"alt+e",      // Daily Review
		"alt+p",      // Plan My Day
		"alt+l",      // Layout picker
		"alt+W",      // Profile picker (Shift+Alt+W)
		"alt+left", "alt+right", // Navigation history
		"ctrl+k",     // Task Manager
		"ctrl+g",     // Graph view
		"ctrl+o":     // Quick switch
		return true
	// Focus-pane chords
	case "f1", "f2", "f3", "alt+1", "alt+2", "alt+3":
		return true
	}
	return false
}

// reopenFeatureCommand maps a synthetic feature-tab path back
// to the CommandAction that opens that feature. Used by
// Ctrl+Shift+T (CmdReopenClosedTab) so feature tabs survive
// the closed-history → reopen round trip with their full init
// + enrichment, not as a blank-state shell.
//
// Returns ok=false for unknown feature IDs (forward-compat with
// future Lua-defined features whose reopen would have to come
// from the Lua bridge instead of a built-in CommandAction).
func reopenFeatureCommand(path string) (CommandAction, bool) {
	if !isFeatureTabPath(path) {
		return CmdNone, false
	}
	id := FeatureID(strings.TrimPrefix(path, "feat:"))
	switch id {
	case FeatTaskManager:
		return CmdTaskManager, true
	case FeatDailyJot:
		return CmdDailyJot, true
	case FeatCalendar:
		return CmdShowCalendar, true
	case FeatKanban:
		return CmdKanban, true
	case FeatGoals:
		return CmdGoalsMode, true
	case FeatProject:
		return CmdProjectMode, true
	case FeatGraph:
		return CmdShowGraph, true
	}
	return CmdNone, false
}

// activateTabByPath dispatches a tab-switch path to the right
// activator. For note paths, loadNote + sidebar update. For
// feature paths ("feat:<id>"), clear activeNote so the render
// branch picks up the feature view; sidebar stays put. Used by
// Ctrl+Tab / Ctrl+Shift+Tab / Ctrl+1..9 — without this they
// silently no-op'd on feature tabs because loadNote's vault
// lookup returned nil.
func (m *Model) activateTabByPath(path string) {
	if path == "" {
		return
	}
	if isFeatureTabPath(path) {
		// Feature tab now active. Render branch consults
		// ActiveFeature so we don't have to dispatch by id here.
		m.activeNote = ""
		return
	}
	if path == m.activeNote {
		return
	}
	m.loadNote(path)
	m.setSidebarCursorToFile(path)
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

// featureTabIsForeground gates the legacy `if m.X.IsActive()`
// key-routing blocks for surfaces that migrated render-only
// (Calendar / Kanban / Goals / Projects / Graph). Their Update
// logic still lives in those legacy blocks, but we only want
// them to fire when the user is actually looking at that
// feature — otherwise opening a Calendar tab and switching to
// a note tab would have Calendar swallow every keystroke meant
// for the editor.
//
// Returns true when:
//   - there is no tabbar (caller should behave as before), OR
//   - the active tab IS this feature.
//
// Returns false when a different tab (note or another feature)
// is active — caller should skip its IsActive-based dispatch.
func featureTabIsForeground(tb *TabBar, id FeatureID) bool {
	if tb == nil {
		return true
	}
	af, ok := tb.ActiveFeature()
	if !ok {
		// No feature tab is active. The legacy block fired in
		// the pre-Phase-4 world whenever IsActive was true; it
		// shouldn't fire now if a note tab took focus, because
		// the user is editing that note. Return false so legacy
		// surfaces stop routing keys when a note tab is active.
		return false
	}
	return af == id
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
