package tui

import (
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artaeon/granit/internal/objects"
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
// (close tab), Ctrl+P (palette), Ctrl+X (palette fallback),
// etc. would all be consumed by the feature's Update and
// never reach their handlers.
//
// Rule of thumb for adding: any chord with a Ctrl, Alt, or
// function-key prefix that's bound at the global level (in the
// `case "ctrl+X":` block of app_update.go around line 2670+)
// should be passthrough. Letters and Esc and arrows go to the
// focused feature. The audit covers every chord currently
// defined in the global switch — if a new global chord lands,
// add it here too (or feature tabs will swallow it).
func isPassthroughChord(key string) bool {
	switch key {
	// Tab management — close, cycle, jump, reopen
	case "ctrl+w",
		"ctrl+tab", "ctrl+shift+tab",
		"ctrl+pgup", "ctrl+pgdown", // browser-style tab cycle
		"alt+,", "alt+.", // one-handed tab cycle (defeats terminal intercepts)
		"ctrl+1", "ctrl+2", "ctrl+3", "ctrl+4", "ctrl+5",
		"ctrl+6", "ctrl+7", "ctrl+8", "ctrl+9",
		"ctrl+shift+t",
		"alt+shift+left", "alt+shift+right": // move tab
		return true
	// Quit / save / palette family — Ctrl+X is palette
	// fallback when no editor selection (this was the
	// originally-missed chord that trapped the user).
	case "ctrl+p", "ctrl+q", "ctrl+c", "ctrl+s", "ctrl+x":
		return true
	// Feature-opening Ctrl+ shortcuts
	case "ctrl+n", // New note
		"ctrl+e", // Toggle view/edit
		"ctrl+t", // Tags
		"ctrl+b", // Bookmarks
		"ctrl+f", // Find
		"ctrl+h", // Find & replace
		"ctrl+j", // Quick switch
		"ctrl+k", // Task Manager
		"ctrl+l", // Calendar
		"ctrl+r", // Bots
		"ctrl+g", // Graph
		"ctrl+o", // Outline
		"ctrl+,", // Settings
		"ctrl+/": // Help / shortcuts
		// Ctrl+Z is NOT passthrough — it's editor undo, must land
		// in the focused editor when one is active. Feature tabs
		// without an editor underneath simply ignore it.
		return true
	// Feature-opening Alt+ shortcuts (lowercase letters)
	case "alt+h", // Daily Hub
		"alt+j",          // Daily Jot
		"alt+m",          // Morning Routine
		"alt+b",          // Habit Tracker
		"alt+i",          // Quick Capture
		"alt+e",          // Daily Review
		"alt+p",          // Plan My Day
		"alt+l",          // Layout picker
		"alt+t",          // Time tracker
		"alt+x",          // Spreadsheet picker (CSV/XLSX)
		"alt+o",          // Object Browser (typed notes)
		"alt+v",          // Saved Views (smart collections)
		"alt+/",          // Inline AI action menu
		"alt+z",          // Focus / Zen mode (was ctrl+z; freed up for undo)
		"alt+n",          // Quick-add task on project/goal note
		"alt+a",          // Agent runner (multi-step AI)
		"alt+@",          // Typed-mention picker
		"alt+s",          // Focus session
		"alt+w",          // Weekly note
		"alt+c",          // Command Center
		"alt+d",          // Daily Briefing or similar
		"alt+f",          // Fold (editor); harmless on features
		"alt+g",          // Graph alternate
		"alt+r",          // Reload / refresh
		"alt+[", "alt+]", // Daily-note navigation
		"alt+left", "alt+right", // History navigation
		"alt+?": // Help
		return true
	// Capital Alt+ chords (Shift+Alt+letter)
	case "alt+W", // Profile picker
		"alt+C": // Command Center alt
		return true
	// Function keys + focus-pane chords
	case "f1", "f2", "f3", "f4", "f5", "f6",
		"alt+1", "alt+2", "alt+3":
		return true
	// Multi-key pane swap (Shift+Tab inside global handler)
	case "shift+tab":
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
	case FeatHabits:
		return CmdHabitTracker, true
	case FeatCommandCenter:
		return CmdCommandCenter, true
	case FeatSheetView:
		return CmdSheetView, true
	case FeatObjectBrowser:
		return CmdObjectBrowser, true
	case FeatSavedView:
		return CmdSavedViews, true
	case FeatRepoTracker:
		return CmdRepoTracker, true
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
	// Each migrated surface gets SetTabMode(true) before render
	// so it skips the overlay-mode 2/3-screen width clamp and
	// fills the editor pane. closeFeature resets it to false.
	switch id {
	case FeatTaskManager:
		m.taskManager.SetSize(width, height)
		m.taskManager.SetTabMode(true)
		// Refresh timer snapshot so the row renderer can show
		// the "▸ tracking" badge on the active task. Empty
		// task name means no timer running.
		if m.timeTracker.IsTimerRunning() {
			task, secs := m.timeTracker.ActiveTimerSnapshot()
			m.taskManager.SetActiveTimer(task, secs)
		} else {
			m.taskManager.SetActiveTimer("", 0)
		}
		return m.taskManager.View()
	case FeatDailyJot:
		m.dailyJot.SetSize(width, height)
		m.dailyJot.SetTabMode(true)
		return m.dailyJot.View()
	case FeatCalendar:
		m.calendar.SetSize(width, height)
		m.calendar.SetTabMode(true)
		return m.calendar.View()
	case FeatKanban:
		m.kanban.SetSize(width, height)
		m.kanban.SetTabMode(true)
		return m.kanban.View()
	case FeatGoals:
		m.goalsMode.SetSize(width, height)
		m.goalsMode.SetTabMode(true)
		return m.goalsMode.View()
	case FeatProject:
		m.projectMode.SetSize(width, height)
		m.projectMode.SetTabMode(true)
		return m.projectMode.View()
	case FeatGraph:
		m.graphView.SetSize(width, height)
		m.graphView.SetTabMode(true)
		return m.graphView.View()
	case FeatHabits:
		m.habitTracker.SetSize(width, height)
		m.habitTracker.SetTabMode(true)
		return m.habitTracker.View()
	case FeatCommandCenter:
		m.commandCenter.SetSize(width, height)
		return m.commandCenter.InlineView(width, height)
	case FeatSheetView:
		m.sheetView.SetSize(width, height)
		m.sheetView.SetTabMode(true)
		return m.sheetView.View()
	case FeatObjectBrowser:
		m.objectBrowser.SetSize(width, height)
		m.objectBrowser.SetTabMode(true)
		return m.objectBrowser.View()
	case FeatSavedView:
		m.savedViews.SetSize(width, height)
		m.savedViews.SetTabMode(true)
		return m.savedViews.View()
	case FeatRepoTracker:
		m.repoTracker.SetSize(width, height)
		m.repoTracker.SetTabMode(true)
		return m.repoTracker.View()
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
		// Timer-toggle request — TaskManager doesn't own the
		// TimeTracker (avoid circular ownership), so the model
		// drives Start/Stop based on the consumed-once flag.
		// Toggle: start tracking the cursor task if no timer
		// is running; stop the current timer if pressing 'y'
		// again (regardless of which task is showing).
		if t, ok := m.taskManager.GetTimerToggleRequest(); ok {
			if m.timeTracker.IsTimerRunning() {
				m.timeTracker.StopTimer()
			} else {
				m.timeTracker.StartTimer(t.NotePath, tmCleanText(t.Text))
			}
			m.reportError("persist time tracker", m.timeTracker.ConsumeSaveError())
		}
		// Esc/q in normal mode sets tm.active=false (legacy
		// overlay-dismissal behavior, can't easily change in
		// taskmanager.go without breaking other call sites).
		// In the tab world that's a zombie state — render still
		// shows the surface, but every subsequent key returns
		// early via `if !tm.active`. Detect it and close the
		// tab so Esc behaves like "dismiss the surface."
		if !m.taskManager.IsActive() && m.tabBar != nil && m.tabBar.HasFeatureTab(FeatTaskManager) {
			m.tabBar.CloseFeatureTab(FeatTaskManager)
			m.activeNote = ""
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
	case FeatCommandCenter:
		var cmd tea.Cmd
		m.commandCenter, cmd = m.commandCenter.Update(msg)
		if m.commandCenter.ShouldStartPomodoro() {
			m.pomodoro.Open()
			m.pomodoro.Start()
		}
		if task := m.commandCenter.CompletedTask(); task != nil {
			if task.NotePath != "" && task.LineNum >= 1 {
				if note := m.vault.GetNote(task.NotePath); note != nil {
					lines := strings.Split(note.Content, "\n")
					idx := task.LineNum - 1
					if idx < len(lines) {
						lines[idx] = strings.Replace(lines[idx], "- [ ]", "- [x]", 1)
						newContent := strings.Join(lines, "\n")
						if err := atomicWriteNote(filepath.Join(m.vault.Root, task.NotePath), newContent); err != nil {
							m.reportError("mark task done", err)
						} else {
							m.refreshComponents(task.NotePath)
						}
					}
				}
			}
		}
		if projName := m.commandCenter.SelectedProject(); projName != "" {
			m.projectMode.SetSize(m.width, m.height)
			m.projectMode.Open(m.vault.Root)
		}
		if habitName := m.commandCenter.ToggledHabit(); habitName != "" {
			m.habitTracker.dailyNotesFolder = m.config.DailyNotesFolder
			m.habitTracker.Open(m.vault.Root)
			m.habitTracker.toggleToday(habitName)
			m.habitTracker.Close()
		}
		if !m.commandCenter.IsActive() && m.tabBar != nil && m.tabBar.HasFeatureTab(FeatCommandCenter) {
			m.tabBar.CloseFeatureTab(FeatCommandCenter)
			m.activeNote = ""
		}
		return *m, cmd, true
	case FeatSheetView:
		var cmd tea.Cmd
		m.sheetView, cmd = m.sheetView.Update(msg)
		// Persist on Ctrl+S errors so the user sees them.
		if err := m.sheetView.ConsumeSaveError(); err != nil {
			m.reportError("save spreadsheet", err)
		}
		// 'q' inside the sheet requests tab close.
		if m.sheetView.ConsumePendingClose() {
			if m.tabBar != nil {
				m.tabBar.CloseFeatureTab(FeatSheetView)
			}
			m.sheetView.Close()
			m.activeNote = ""
		}
		return *m, cmd, true
	case FeatObjectBrowser:
		// Refresh the typed-objects index when the vault has
		// changed since the last interaction. Without this, if
		// the user edits a typed note's frontmatter (changes
		// status, adds a property) and tabs back, the gallery
		// stays stale until they manually re-open. Cheap on a
		// 1000-note vault (~1ms).
		if m.needsRefresh {
			if m.objectsRegistry == nil {
				m.objectsRegistry = objects.NewRegistry()
				if _, errs := m.objectsRegistry.LoadVaultDir(m.vault.Root); len(errs) > 0 {
					for _, err := range errs {
						m.reportError("object types", err)
					}
				}
			}
			m.objectsIndex = rebuildObjectsIndex(m.objectsRegistry, m.vault)
			m.objectBrowser.Refresh(m.objectsRegistry, m.objectsIndex)
			m.needsRefresh = false
		}
		var cmd tea.Cmd
		m.objectBrowser, cmd = m.objectBrowser.Update(msg)
		// Object selected → close tab and load the underlying note.
		if path, ok := m.objectBrowser.GetJumpRequest(); ok {
			if m.tabBar != nil {
				m.tabBar.CloseFeatureTab(FeatObjectBrowser)
			}
			m.objectBrowser.Close()
			m.loadNote(path)
			m.setSidebarCursorToFile(path)
			m.setFocus(focusEditor)
		}
		// Delete requested — remove the file from disk and refresh
		// every dependent component. Errors surface as a status
		// warning so the user knows if the file was missing or the
		// remove failed.
		if delPath, ok := m.objectBrowser.GetDeleteRequest(); ok {
			if err := m.deleteObjectFile(delPath); err != nil {
				m.reportError("delete object", err)
			} else {
				if m.tabBar != nil {
					m.tabBar.CloseFeatureTab(FeatObjectBrowser)
				}
				m.objectBrowser.Close()
				m.refreshComponents("")
				m.objectsIndex = rebuildObjectsIndex(m.objectsRegistry, m.vault)
				m.statusbar.SetMessage("✓ Deleted " + delPath)
				return *m, m.clearMessageAfter(3 * time.Second), true
			}
		}
		// New object requested — write the file, refresh vault state,
		// close the browser, and open the new note for the user to
		// fill in. Errors are surfaced via reportError so the user
		// sees a clear "couldn't create" status if e.g. the path
		// already exists.
		if relPath, content, ok := m.objectBrowser.GetCreateRequest(); ok {
			if err := m.createTypedObjectFile(relPath, content); err != nil {
				m.reportError("create object", err)
			} else {
				if m.tabBar != nil {
					m.tabBar.CloseFeatureTab(FeatObjectBrowser)
				}
				m.objectBrowser.Close()
				m.refreshComponents("")
				// Refresh the typed-objects index so the new note
				// appears in the registry-driven counts on next open.
				m.objectsIndex = rebuildObjectsIndex(m.objectsRegistry, m.vault)
				m.loadNote(relPath)
				m.setSidebarCursorToFile(relPath)
				m.setFocus(focusEditor)
				m.statusbar.SetMessage("✓ Created " + relPath)
			}
		}
		// User pressed Esc twice — close the tab.
		if !m.objectBrowser.IsActive() && m.tabBar != nil && m.tabBar.HasFeatureTab(FeatObjectBrowser) {
			m.tabBar.CloseFeatureTab(FeatObjectBrowser)
			m.activeNote = ""
		}
		return *m, cmd, true

	case FeatSavedView:
		// Re-evaluate the active view against a refreshed index when the
		// vault changed since the last interaction. Same rationale as
		// the FeatObjectBrowser branch above.
		if m.needsRefresh {
			m.objectsIndex = rebuildObjectsIndex(m.objectsRegistry, m.vault)
			m.savedViews.Refresh(m.objectsIndex)
			m.needsRefresh = false
		}
		var cmd tea.Cmd
		m.savedViews, cmd = m.savedViews.Update(msg)
		// Delete requested — same flow as FeatObjectBrowser. Two-step
		// confirmation already happened inside SavedViews; here we
		// just remove the file and refresh.
		if delPath, ok := m.savedViews.GetDeleteRequest(); ok {
			if err := m.deleteObjectFile(delPath); err != nil {
				m.reportError("delete object", err)
			} else {
				if m.tabBar != nil {
					m.tabBar.CloseFeatureTab(FeatSavedView)
				}
				m.savedViews.Close()
				m.refreshComponents("")
				m.objectsIndex = rebuildObjectsIndex(m.objectsRegistry, m.vault)
				m.statusbar.SetMessage("✓ Deleted " + delPath)
				return *m, m.clearMessageAfter(3 * time.Second), true
			}
		}
		// In-tab object creation: same flow as FeatObjectBrowser. Lets
		// the user capture from inside a saved view (e.g. add another
		// article while browsing "Articles to Read").
		if relPath, content, ok := m.savedViews.GetCreateRequest(); ok {
			if err := m.createTypedObjectFile(relPath, content); err != nil {
				m.reportError("create object", err)
			} else {
				if m.tabBar != nil {
					m.tabBar.CloseFeatureTab(FeatSavedView)
				}
				m.savedViews.Close()
				m.refreshComponents("")
				m.objectsIndex = rebuildObjectsIndex(m.objectsRegistry, m.vault)
				m.loadNote(relPath)
				m.setSidebarCursorToFile(relPath)
				m.setFocus(focusEditor)
				m.statusbar.SetMessage("✓ Created " + relPath)
				return *m, cmd, true
			}
		}
		if path, ok := m.savedViews.GetJumpRequest(); ok {
			if m.tabBar != nil {
				m.tabBar.CloseFeatureTab(FeatSavedView)
			}
			m.savedViews.Close()
			m.loadNote(path)
			m.setSidebarCursorToFile(path)
			m.setFocus(focusEditor)
		}
		if !m.savedViews.IsActive() && m.tabBar != nil && m.tabBar.HasFeatureTab(FeatSavedView) {
			m.tabBar.CloseFeatureTab(FeatSavedView)
			m.activeNote = ""
		}
		return *m, cmd, true

	case FeatRepoTracker:
		var cmd tea.Cmd
		m.repoTracker, cmd = m.repoTracker.Update(msg)
		// Surface action results (o / c) on the global status bar.
		// ConsumePendingStatus is consumed-once so calling on every
		// tick is safe.
		if msg := m.repoTracker.ConsumePendingStatus(); msg != "" {
			m.statusbar.SetMessage(msg)
			cmd = tea.Batch(cmd, m.clearMessageAfter(3*time.Second))
		}
		// Import: write the project note, refresh, open it.
		if relPath, content, ok := m.repoTracker.GetImportRequest(); ok {
			if err := m.createTypedObjectFile(relPath, content); err != nil {
				m.reportError("import repo", err)
			} else {
				if m.tabBar != nil {
					m.tabBar.CloseFeatureTab(FeatRepoTracker)
				}
				m.repoTracker.Close()
				m.refreshComponents("")
				m.objectsIndex = rebuildObjectsIndex(m.objectsRegistry, m.vault)
				m.loadNote(relPath)
				m.setSidebarCursorToFile(relPath)
				m.setFocus(focusEditor)
				m.statusbar.SetMessage("✓ Imported repo as " + relPath)
				return *m, cmd, true
			}
		}
		// Jump to existing note for already-imported row.
		if path, ok := m.repoTracker.GetJumpRequest(); ok {
			if m.tabBar != nil {
				m.tabBar.CloseFeatureTab(FeatRepoTracker)
			}
			m.repoTracker.Close()
			m.loadNote(path)
			m.setSidebarCursorToFile(path)
			m.setFocus(focusEditor)
		}
		if !m.repoTracker.IsActive() && m.tabBar != nil && m.tabBar.HasFeatureTab(FeatRepoTracker) {
			m.tabBar.CloseFeatureTab(FeatRepoTracker)
			m.activeNote = ""
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
		m.taskManager.SetTabMode(false)
		m.taskManager.Close()
	case FeatDailyJot:
		m.dailyJot.SetTabMode(false)
		m.dailyJot.Close()
	case FeatCalendar:
		m.calendar.SetTabMode(false)
		m.calendar.Close()
	case FeatKanban:
		m.kanban.SetTabMode(false)
		m.kanban.Close()
	case FeatGoals:
		m.goalsMode.SetTabMode(false)
		m.goalsMode.Close()
	case FeatProject:
		m.projectMode.SetTabMode(false)
		m.projectMode.Close()
	case FeatGraph:
		m.graphView.SetTabMode(false)
		m.graphView.Close()
	case FeatHabits:
		m.habitTracker.SetTabMode(false)
		m.habitTracker.Close()
	case FeatCommandCenter:
		m.commandCenter.Close()
	case FeatSheetView:
		m.sheetView.SetTabMode(false)
		m.sheetView.Close()
	case FeatObjectBrowser:
		m.objectBrowser.SetTabMode(false)
		m.objectBrowser.Close()
	case FeatSavedView:
		m.savedViews.SetTabMode(false)
		m.savedViews.Close()
	case FeatRepoTracker:
		m.repoTracker.SetTabMode(false)
		m.repoTracker.Close()
	}
}
