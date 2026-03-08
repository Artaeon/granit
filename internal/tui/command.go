package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type CommandAction int

const (
	CmdNone CommandAction = iota
	CmdOpenFile
	CmdNewNote
	CmdSaveNote
	CmdDailyNote
	CmdToggleView
	CmdSettings
	CmdToggleSidebar
	CmdFocusEditor
	CmdFocusSidebar
	CmdFocusBacklinks
	CmdSearchInFile
	CmdRefreshVault
	CmdDeleteNote
	CmdRenameNote
	CmdShowGraph
	CmdShowTags
	CmdShowHelp
	CmdShowOutline
	CmdShowBookmarks
	CmdToggleBookmark
	CmdFindInFile
	CmdReplaceInFile
	CmdShowStats
	CmdNewFromTemplate
	CmdFocusMode
	CmdQuickSwitch
	CmdShowTrash
	CmdShowCanvas
	CmdShowCalendar
	CmdShowBots
	CmdNewFolder
	CmdMoveFile
	CmdExportNote
	CmdGitOverlay
	CmdPluginManager
	CmdContentSearch
	CmdSpellCheck
	CmdImportObsidian
	CmdPublishSite
	CmdSplitPane
	CmdRunLuaScript
	CmdFlashcards
	CmdQuizMode
	CmdLearnDashboard
	CmdAIChat
	CmdComposer
	CmdKnowledgeGraph
	CmdAutoLink
	CmdSimilarNotes
	CmdTableEditor
	CmdSemanticSearch
	CmdThreadWeaver
	CmdNoteChat
	CmdToggleGhostWriter
	CmdPomodoro
	CmdWebClip
	CmdToggleVim
	CmdPinNote
	CmdUnpinNote
	CmdNavBack
	CmdNavForward
	CmdKanban
	CmdZettelNote
	CmdVaultRefactor
	CmdDailyBriefing
	CmdEncryptNote
	CmdGitHistory
	CmdWorkspaces
	CmdTimeline
	CmdVaultSwitch
	CmdFoldToggle
	CmdFoldAll
	CmdUnfoldAll
	CmdFrontmatterEdit
	CmdResearchAgent
	CmdResearchFollowUp
	CmdImageManager
	CmdThemeEditor
	CmdTaskManager
	CmdLinkAssist
	CmdLayoutDefault
	CmdLayoutWriter
	CmdLayoutMinimal
	CmdLayoutReading
	CmdLayoutDashboard
	CmdBlogPublish
	CmdGlobalReplace
	CmdAITemplate
	CmdVaultAnalyzer
	CmdNoteEnhancer
	CmdDailyDigest
	CmdLanguageLearning
	CmdHabitTracker
	CmdFocusSession
	CmdStandupGenerator
	CmdNoteHistory
	CmdSmartConnections
	CmdWritingStats
	CmdQuickCapture
	CmdDashboard
	CmdMindMap
	CmdJournalPrompts
	CmdClipManager
	CmdDailyPlanner
	CmdAIScheduler
	CmdRecurringTasks
	CmdNotePreview
	CmdScratchpad
	CmdProjectMode
	CmdNLSearch
	CmdWritingCoach
	CmdDataview
	CmdTimeTracker
	CmdBackup
	CmdShowTutorial
	CmdMacroRecord
	CmdMacroPlay
	CmdCloseOtherTabs
	CmdCloseTabsToRight
	CmdTogglePinTab
	CmdReopenClosedTab
	CmdSmartPaste
	CmdQuit
)

type Command struct {
	Label    string
	Desc     string
	Shortcut string
	Action   CommandAction
	Icon     *string // pointer to icon char variable (nil = no icon)
}

var AllCommands = []Command{
	{Label: "Open File", Desc: "Quick open a file", Shortcut: "Ctrl+P", Action: CmdOpenFile, Icon: &IconSearchChar},
	{Label: "New Note", Desc: "Create a new note", Shortcut: "Ctrl+N", Action: CmdNewNote, Icon: &IconNewChar},
	{Label: "Save Note", Desc: "Save the current note", Shortcut: "Ctrl+S", Action: CmdSaveNote, Icon: &IconSaveChar},
	{Label: "Daily Note", Desc: "Open or create today's daily note", Shortcut: "", Action: CmdDailyNote, Icon: &IconDailyChar},
	{Label: "Toggle View/Edit", Desc: "Switch between view and edit mode", Shortcut: "Ctrl+E", Action: CmdToggleView, Icon: &IconViewChar},
	{Label: "Settings", Desc: "Open settings panel", Shortcut: "Ctrl+,", Action: CmdSettings, Icon: &IconSettingsChar},
	{Label: "Focus Editor", Desc: "Switch focus to the editor", Shortcut: "F2", Action: CmdFocusEditor, Icon: &IconEditChar},
	{Label: "Focus Sidebar", Desc: "Switch focus to the file sidebar", Shortcut: "F1", Action: CmdFocusSidebar, Icon: &IconFolderChar},
	{Label: "Focus Backlinks", Desc: "Switch focus to the backlinks panel", Shortcut: "F3", Action: CmdFocusBacklinks, Icon: &IconLinkChar},
	{Label: "Refresh Vault", Desc: "Rescan vault for changes", Shortcut: "", Action: CmdRefreshVault},
	{Label: "Delete Note", Desc: "Delete the current note", Shortcut: "", Action: CmdDeleteNote, Icon: &IconTrashChar},
	{Label: "Rename Note", Desc: "Rename the current note", Shortcut: "F4", Action: CmdRenameNote, Icon: &IconEditChar},
	{Label: "Show Graph", Desc: "Show note connection graph", Shortcut: "Ctrl+G", Action: CmdShowGraph, Icon: &IconGraphChar},
	{Label: "Show Tags", Desc: "Browse notes by tags", Shortcut: "Ctrl+T", Action: CmdShowTags, Icon: &IconTagChar},
	{Label: "Help", Desc: "Show keyboard shortcuts", Shortcut: "F5", Action: CmdShowHelp, Icon: &IconHelpChar},
	{Label: "Outline", Desc: "Show note heading outline", Shortcut: "Ctrl+O", Action: CmdShowOutline, Icon: &IconOutlineChar},
	{Label: "Bookmarks", Desc: "View starred & recent notes", Shortcut: "Ctrl+B", Action: CmdShowBookmarks, Icon: &IconBookmarkChar},
	{Label: "Toggle Bookmark", Desc: "Star/unstar current note", Shortcut: "", Action: CmdToggleBookmark, Icon: &IconBookmarkChar},
	{Label: "Find", Desc: "Search within current file", Shortcut: "Ctrl+F", Action: CmdFindInFile, Icon: &IconSearchChar},
	{Label: "Find & Replace", Desc: "Find and replace in file", Shortcut: "Ctrl+H", Action: CmdReplaceInFile, Icon: &IconSearchChar},
	{Label: "Vault Statistics", Desc: "Show vault stats & charts", Shortcut: "", Action: CmdShowStats, Icon: &IconGraphChar},
	{Label: "New from Template", Desc: "Create note from template", Shortcut: "", Action: CmdNewFromTemplate, Icon: &IconFileChar},
	{Label: "Focus Mode", Desc: "Distraction-free writing", Shortcut: "Ctrl+Z", Action: CmdFocusMode, Icon: &IconEditChar},
	{Label: "Quick Switch", Desc: "Switch between recent files", Shortcut: "Ctrl+J", Action: CmdQuickSwitch, Icon: &IconFileChar},
	{Label: "Trash", Desc: "View and restore deleted notes", Shortcut: "", Action: CmdShowTrash, Icon: &IconTrashChar},
	{Label: "Canvas", Desc: "Visual note canvas / whiteboard", Shortcut: "Ctrl+W", Action: CmdShowCanvas, Icon: &IconCanvasChar},
	{Label: "Calendar", Desc: "Calendar view with daily notes", Shortcut: "Ctrl+L", Action: CmdShowCalendar, Icon: &IconCalendarChar},
	{Label: "Bots", Desc: "AI bots for note analysis", Shortcut: "Ctrl+R", Action: CmdShowBots, Icon: &IconBotChar},
	{Label: "New Folder", Desc: "Create a new folder", Shortcut: "", Action: CmdNewFolder, Icon: &IconFolderChar},
	{Label: "Move File", Desc: "Move current note to a folder", Shortcut: "", Action: CmdMoveFile, Icon: &IconFolderChar},
	{Label: "Export Current Note", Desc: "Export note as HTML, text, or PDF", Shortcut: "", Action: CmdExportNote, Icon: &IconSaveChar},
	{Label: "Git: Status & Commit", Desc: "Git status, log, diff, commit, push, pull", Shortcut: "", Action: CmdGitOverlay, Icon: &IconBotChar},
	{Label: "Plugins", Desc: "Manage and run plugins", Shortcut: "", Action: CmdPluginManager, Icon: &IconSettingsChar},
	{Label: "Search Vault Contents", Desc: "Full-text search across all notes", Shortcut: "", Action: CmdContentSearch, Icon: &IconSearchChar},
	{Label: "Global Search & Replace", Desc: "Find and replace across all vault files", Shortcut: "", Action: CmdGlobalReplace, Icon: &IconSearchChar},
	{Label: "Spell Check", Desc: "Check spelling in current note", Shortcut: "", Action: CmdSpellCheck, Icon: &IconEditChar},
	{Label: "Import Obsidian Config", Desc: "Import settings from .obsidian/ directory", Shortcut: "", Action: CmdImportObsidian, Icon: &IconSettingsChar},
	{Label: "Publish Site", Desc: "Export vault as static HTML site", Shortcut: "", Action: CmdPublishSite, Icon: &IconSaveChar},
	{Label: "Publish to Blog", Desc: "Publish note to Medium or GitHub blog", Shortcut: "", Action: CmdBlogPublish, Icon: &IconSaveChar},
	{Label: "Split View", Desc: "View two notes side by side", Shortcut: "", Action: CmdSplitPane, Icon: &IconViewChar},
	{Label: "Lua Scripts", Desc: "Run Lua scripts from vault or global dir", Shortcut: "", Action: CmdRunLuaScript, Icon: &IconBotChar},
	{Label: "Flashcards", Desc: "Spaced repetition study from your notes", Shortcut: "", Action: CmdFlashcards, Icon: &IconBookmarkChar},
	{Label: "Quiz Mode", Desc: "Test your knowledge with auto-generated quizzes", Shortcut: "", Action: CmdQuizMode, Icon: &IconHelpChar},
	{Label: "Learning Dashboard", Desc: "Track study progress, streaks, mastery", Shortcut: "", Action: CmdLearnDashboard, Icon: &IconGraphChar},
	{Label: "AI Chat", Desc: "Ask questions about your vault", Shortcut: "", Action: CmdAIChat, Icon: &IconBotChar},
	{Label: "AI Compose Note", Desc: "Generate a note from a topic prompt", Shortcut: "", Action: CmdComposer, Icon: &IconNewChar},
	{Label: "AI Template", Desc: "Generate a full note from a template type + topic with AI", Shortcut: "", Action: CmdAITemplate, Icon: &IconBotChar},
	{Label: "Knowledge Graph AI", Desc: "Analyze clusters, hubs, orphans, suggestions", Shortcut: "", Action: CmdKnowledgeGraph, Icon: &IconGraphChar},
	{Label: "Auto-Link Suggestions", Desc: "Find unlinked mentions in current note", Shortcut: "", Action: CmdAutoLink, Icon: &IconLinkChar},
	{Label: "Similar Notes", Desc: "Find notes similar to current one (TF-IDF)", Shortcut: "", Action: CmdSimilarNotes, Icon: &IconSearchChar},
	{Label: "Table Editor", Desc: "Visual markdown table editor", Shortcut: "", Action: CmdTableEditor, Icon: &IconEditChar},
	{Label: "Semantic Search", Desc: "AI-powered meaning-based vault search", Shortcut: "", Action: CmdSemanticSearch, Icon: &IconSearchChar},
	{Label: "Thread Weaver", Desc: "Synthesize multiple notes into a new essay", Shortcut: "", Action: CmdThreadWeaver, Icon: &IconNewChar},
	{Label: "Chat with Note", Desc: "AI Q&A focused on current note", Shortcut: "", Action: CmdNoteChat, Icon: &IconBotChar},
	{Label: "Ghost Writer", Desc: "Toggle inline AI writing suggestions", Shortcut: "", Action: CmdToggleGhostWriter, Icon: &IconEditChar},
	{Label: "Pomodoro Timer", Desc: "Focus timer with writing stats", Shortcut: "", Action: CmdPomodoro, Icon: &IconDailyChar},
	{Label: "Web Clipper", Desc: "Save a web page as a markdown note", Shortcut: "", Action: CmdWebClip, Icon: &IconSaveChar},
	{Label: "Toggle Vim Mode", Desc: "Enable/disable Vim keybindings", Shortcut: "", Action: CmdToggleVim, Icon: &IconEditChar},
	{Label: "Start Macro Recording", Desc: "Record keystrokes into a Vim macro register (a-z)", Shortcut: "q+reg", Action: CmdMacroRecord, Icon: &IconEditChar},
	{Label: "Play Macro", Desc: "Replay a recorded Vim macro register", Shortcut: "@+reg", Action: CmdMacroPlay, Icon: &IconEditChar},
	{Label: "Pin Note", Desc: "Pin current note as a tab", Shortcut: "", Action: CmdPinNote, Icon: &IconBookmarkChar},
	{Label: "Unpin Note", Desc: "Unpin current note", Shortcut: "", Action: CmdUnpinNote, Icon: &IconBookmarkChar},
	{Label: "Pin/Unpin Tab", Desc: "Toggle pin on active tab", Shortcut: "", Action: CmdTogglePinTab, Icon: &IconBookmarkChar},
	{Label: "Close Other Tabs", Desc: "Close all tabs except the active one", Shortcut: "", Action: CmdCloseOtherTabs, Icon: &IconFileChar},
	{Label: "Close Tabs to the Right", Desc: "Close tabs after the active one", Shortcut: "", Action: CmdCloseTabsToRight, Icon: &IconFileChar},
	{Label: "Reopen Closed Tab", Desc: "Reopen the last closed tab", Shortcut: "", Action: CmdReopenClosedTab, Icon: &IconFileChar},
	{Label: "Navigate Back", Desc: "Go to previous note in history", Shortcut: "Alt+Left", Action: CmdNavBack, Icon: &IconFolderChar},
	{Label: "Navigate Forward", Desc: "Go to next note in history", Shortcut: "Alt+Right", Action: CmdNavForward, Icon: &IconFolderChar},
	{Label: "Kanban Board", Desc: "View tasks as a Kanban board", Shortcut: "", Action: CmdKanban, Icon: &IconCanvasChar},
	{Label: "New Zettelkasten Note", Desc: "Create a note with unique Zettelkasten ID", Shortcut: "", Action: CmdZettelNote, Icon: &IconNewChar},
	{Label: "AI Vault Refactor", Desc: "AI reorganizes folders, names, tags, and links", Shortcut: "", Action: CmdVaultRefactor, Icon: &IconBotChar},
	{Label: "Daily Briefing", Desc: "DeepCoven morning briefing with today's focus", Shortcut: "", Action: CmdDailyBriefing, Icon: &IconDailyChar},
	{Label: "Encrypt/Decrypt Note", Desc: "AES-256-GCM encryption for secure GitHub sync", Shortcut: "", Action: CmdEncryptNote, Icon: &IconSaveChar},
	{Label: "Git History", Desc: "View commit history and diffs for current note", Shortcut: "", Action: CmdGitHistory, Icon: &IconEditChar},
	{Label: "Workspaces", Desc: "Save and restore named workspace layouts", Shortcut: "", Action: CmdWorkspaces, Icon: &IconViewChar},
	{Label: "Timeline", Desc: "Chronological view of all notes", Shortcut: "", Action: CmdTimeline, Icon: &IconDailyChar},
	{Label: "Switch Vault", Desc: "Switch to a different vault", Shortcut: "", Action: CmdVaultSwitch, Icon: &IconFolderChar},
	{Label: "Toggle Fold", Desc: "Fold/unfold section under cursor", Shortcut: "", Action: CmdFoldToggle, Icon: &IconOutlineChar},
	{Label: "Fold All", Desc: "Collapse all sections", Shortcut: "", Action: CmdFoldAll, Icon: &IconOutlineChar},
	{Label: "Unfold All", Desc: "Expand all sections", Shortcut: "", Action: CmdUnfoldAll, Icon: &IconOutlineChar},
	{Label: "Edit Frontmatter", Desc: "Structured frontmatter property editor", Shortcut: "", Action: CmdFrontmatterEdit, Icon: &IconEditChar},
	{Label: "Deep Dive Research", Desc: "AI research agent — create notes from any topic via Claude Code", Shortcut: "", Action: CmdResearchAgent, Icon: &IconBotChar},
	{Label: "Research Follow-Up", Desc: "Go deeper on current note's topic via Claude Code", Shortcut: "", Action: CmdResearchFollowUp, Icon: &IconBotChar},
	{Label: "Vault Analyzer", Desc: "AI analysis of vault structure, gaps, and suggestions", Shortcut: "", Action: CmdVaultAnalyzer, Icon: &IconGraphChar},
	{Label: "Note Enhancer", Desc: "AI-enhance current note with links, structure, depth", Shortcut: "", Action: CmdNoteEnhancer, Icon: &IconEditChar},
	{Label: "Daily Digest", Desc: "Generate weekly review from recent vault activity", Shortcut: "", Action: CmdDailyDigest, Icon: &IconCalendarChar},
	{Label: "Language Learning", Desc: "Vocabulary tracker, practice sessions, grammar notes", Shortcut: "", Action: CmdLanguageLearning, Icon: &IconBookmarkChar},
	{Label: "Habit Tracker", Desc: "Daily habits, goals, streaks, and progress tracking", Shortcut: "", Action: CmdHabitTracker, Icon: &IconGraphChar},
	{Label: "Focus Session", Desc: "Guided work session with timer, tasks, and scratchpad", Shortcut: "", Action: CmdFocusSession, Icon: &IconDailyChar},
	{Label: "Daily Standup", Desc: "Auto-generate standup from git commits, tasks, and notes", Shortcut: "", Action: CmdStandupGenerator, Icon: &IconCalendarChar},
	{Label: "Note History", Desc: "Git version timeline and diff viewer for current note", Shortcut: "", Action: CmdNoteHistory, Icon: &IconOutlineChar},
	{Label: "Smart Connections", Desc: "Find semantically related notes using content similarity", Shortcut: "", Action: CmdSmartConnections, Icon: &IconLinkChar},
	{Label: "Writing Statistics", Desc: "Word counts, writing streaks, and productivity charts", Shortcut: "", Action: CmdWritingStats, Icon: &IconGraphChar},
	{Label: "Quick Capture", Desc: "Jot down a quick thought to inbox, daily, or tasks", Shortcut: "", Action: CmdQuickCapture, Icon: &IconNewChar},
	{Label: "Dashboard", Desc: "Vault home screen with tasks, notes, stats, and streaks", Shortcut: "", Action: CmdDashboard, Icon: &IconDailyChar},
	{Label: "Mind Map", Desc: "Visual mind map from note headings and wikilinks", Shortcut: "", Action: CmdMindMap, Icon: &IconGraphChar},
	{Label: "Journal Prompts", Desc: "Daily reflection prompts with guided journaling", Shortcut: "", Action: CmdJournalPrompts, Icon: &IconEditChar},
	{Label: "Clipboard Manager", Desc: "Browse and paste from clipboard history", Shortcut: "", Action: CmdClipManager, Icon: &IconOutlineChar},
	{Label: "Smart Paste (URL to Link)", Desc: "Paste URL as markdown link with selected text", Shortcut: "Ctrl+V", Action: CmdSmartPaste, Icon: &IconLinkChar},
	{Label: "Daily Planner", Desc: "Time-blocked daily schedule with tasks, events, habits", Shortcut: "", Action: CmdDailyPlanner, Icon: &IconCalendarChar},
	{Label: "AI Smart Scheduler", Desc: "AI-powered optimal schedule generation", Shortcut: "", Action: CmdAIScheduler, Icon: &IconBotChar},
	{Label: "Recurring Tasks", Desc: "Manage daily/weekly/monthly recurring tasks", Shortcut: "", Action: CmdRecurringTasks, Icon: &IconCalendarChar},
	{Label: "Note Preview", Desc: "Preview the note under cursor", Shortcut: "", Action: CmdNotePreview, Icon: &IconViewChar},
	{Label: "Scratchpad", Desc: "Floating persistent scratchpad", Shortcut: "", Action: CmdScratchpad, Icon: &IconEditChar},
	{Label: "Projects", Desc: "Project management with dashboards and categories", Shortcut: "", Action: CmdProjectMode, Icon: &IconFolderChar},
	{Label: "Natural Language Search", Desc: "AI-powered meaning-based vault search", Shortcut: "", Action: CmdNLSearch, Icon: &IconSearchChar},
	{Label: "Writing Coach", Desc: "AI writing analysis with persona support", Shortcut: "", Action: CmdWritingCoach, Icon: &IconBotChar},
	{Label: "Dataview Query", Desc: "Query notes by frontmatter properties", Shortcut: "", Action: CmdDataview, Icon: &IconGraphChar},
	{Label: "Time Tracker", Desc: "Track time per note/task with pomodoro stats", Shortcut: "", Action: CmdTimeTracker, Icon: &IconDailyChar},
	{Label: "Task Manager", Desc: "View, manage, and plan all tasks across vault", Shortcut: "Ctrl+K", Action: CmdTaskManager, Icon: &IconCalendarChar},
	{Label: "Link Assistant", Desc: "Find unlinked mentions and suggest wikilinks", Shortcut: "", Action: CmdLinkAssist, Icon: &IconLinkChar},
	{Label: "Image Manager", Desc: "Browse and manage vault images", Shortcut: "", Action: CmdImageManager, Icon: &IconViewChar},
	{Label: "Theme Editor", Desc: "Create and customize color themes", Shortcut: "", Action: CmdThemeEditor, Icon: &IconSettingsChar},
	{Label: "Default Layout", Desc: "3-panel: sidebar, editor, backlinks", Shortcut: "", Action: CmdLayoutDefault, Icon: &IconViewChar},
	{Label: "Writer Layout", Desc: "2-panel: sidebar, editor", Shortcut: "", Action: CmdLayoutWriter, Icon: &IconViewChar},
	{Label: "Minimal Layout", Desc: "Editor only", Shortcut: "", Action: CmdLayoutMinimal, Icon: &IconViewChar},
	{Label: "Reading Layout", Desc: "Editor + backlinks, no sidebar", Shortcut: "", Action: CmdLayoutReading, Icon: &IconViewChar},
	{Label: "Dashboard Layout", Desc: "4-panel: sidebar, editor, outline, backlinks", Shortcut: "", Action: CmdLayoutDashboard, Icon: &IconViewChar},
	{Label: "Vault Backup", Desc: "Create, restore, and manage vault backups", Shortcut: "", Action: CmdBackup, Icon: &IconSaveChar},
	{Label: "Show Tutorial", Desc: "Interactive walkthrough of Granit features", Shortcut: "", Action: CmdShowTutorial, Icon: &IconHelpChar},
	{Label: "Quit", Desc: "Exit Granit", Shortcut: "Ctrl+Q", Action: CmdQuit},
}

type CommandPalette struct {
	active   bool
	query    string
	filtered []Command
	cursor   int
	width    int
	height   int
	result   CommandAction
}

func NewCommandPalette() CommandPalette {
	return CommandPalette{
		filtered: AllCommands,
	}
}

func (cp *CommandPalette) SetSize(width, height int) {
	cp.width = width
	cp.height = height
}

func (cp *CommandPalette) Open() {
	cp.active = true
	cp.query = ""
	cp.filtered = AllCommands
	cp.cursor = 0
	cp.result = CmdNone
}

func (cp *CommandPalette) Close() {
	cp.active = false
	cp.query = ""
}

func (cp *CommandPalette) IsActive() bool {
	return cp.active
}

func (cp *CommandPalette) Result() CommandAction {
	r := cp.result
	cp.result = CmdNone
	return r
}

func (cp *CommandPalette) filterCommands() {
	if cp.query == "" {
		cp.filtered = AllCommands
		return
	}
	query := strings.ToLower(cp.query)
	cp.filtered = nil
	for _, cmd := range AllCommands {
		if cmdFuzzyMatch(strings.ToLower(cmd.Label), query) ||
			cmdFuzzyMatch(strings.ToLower(cmd.Desc), query) {
			cp.filtered = append(cp.filtered, cmd)
		}
	}
	if cp.cursor >= len(cp.filtered) {
		cp.cursor = maxInt(0, len(cp.filtered)-1)
	}
}

// cmdFuzzyMatch performs a fuzzy substring match for command filtering.
// This is separate from the sidebar's fuzzyMatch to keep the command palette
// self-contained and allow independent tuning of matching behavior.
func cmdFuzzyMatch(str, pattern string) bool {
	pi := 0
	for si := 0; si < len(str) && pi < len(pattern); si++ {
		if str[si] == pattern[pi] {
			pi++
		}
	}
	return pi == len(pattern)
}

func (cp CommandPalette) Update(msg tea.Msg) (CommandPalette, tea.Cmd) {
	if !cp.active {
		return cp, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			cp.active = false
			return cp, nil
		case "enter":
			if len(cp.filtered) > 0 && cp.cursor < len(cp.filtered) {
				cp.result = cp.filtered[cp.cursor].Action
			}
			cp.active = false
			return cp, nil
		case "up", "ctrl+k":
			if cp.cursor > 0 {
				cp.cursor--
			}
			return cp, nil
		case "down", "ctrl+j":
			if cp.cursor < len(cp.filtered)-1 {
				cp.cursor++
			}
			return cp, nil
		case "backspace":
			if len(cp.query) > 0 {
				cp.query = cp.query[:len(cp.query)-1]
				cp.filterCommands()
			}
			return cp, nil
		default:
			char := msg.String()
			if len(char) == 1 && char[0] >= 32 {
				cp.query += char
				cp.filterCommands()
			}
			return cp, nil
		}
	}
	return cp, nil
}

func (cp CommandPalette) View() string {
	width := cp.width * 2 / 5
	if width < 55 {
		width = 55
	}
	if width > 80 {
		width = 80
	}

	innerW := width - 6

	var b strings.Builder

	// Search input — clean, prominent
	promptStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	searchBg := lipgloss.NewStyle().
		Background(surface0).
		Foreground(text).
		Width(innerW - 2).
		Padding(0, 1)
	cursor := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("│")
	b.WriteString(searchBg.Render(promptStyle.Render("> ") + cp.query + cursor))
	b.WriteString("\n")

	// Results
	maxVisible := cp.height/2 - 4
	if maxVisible < 8 {
		maxVisible = 8
	}
	if maxVisible > 18 {
		maxVisible = 18
	}

	if len(cp.filtered) == 0 {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Italic(true).Render("  No matching commands"))
		b.WriteString("\n")
	} else {
		start := 0
		if cp.cursor >= start+maxVisible {
			start = cp.cursor - maxVisible + 1
		}
		if cp.cursor < start {
			start = cp.cursor
		}
		end := start + maxVisible
		if end > len(cp.filtered) {
			end = len(cp.filtered)
		}

		for i := start; i < end; i++ {
			cmd := cp.filtered[i]

			// Icon
			icon := "  "
			if cmd.Icon != nil {
				icon = *cmd.Icon + " "
			}

			// Shortcut badge (right-aligned)
			shortcutStr := ""
			if cmd.Shortcut != "" {
				shortcutStr = lipgloss.NewStyle().
					Foreground(overlay0).
					Background(surface0).
					Render(" " + cmd.Shortcut + " ")
			}

			if i == cp.cursor {
				accentBar := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(ThemeAccentBar)
				nameStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
				descStyle := lipgloss.NewStyle().Foreground(overlay0)
				iconStyle := lipgloss.NewStyle().Foreground(mauve)

				left := accentBar + " " + iconStyle.Render(icon) + nameStyle.Render(cmd.Label)
				desc := descStyle.Render("  " + cmd.Desc)

				// Truncate desc if line too long
				leftW := lipgloss.Width(left)
				shortcutW := lipgloss.Width(shortcutStr)
				available := innerW - leftW - shortcutW - 1
				if available > 4 {
					if lipgloss.Width(desc) > available {
						desc = descStyle.Render("  " + cmd.Desc[:maxInt(0, available-4)] + "…")
					}
				} else {
					desc = ""
				}

				line := left + desc
				if shortcutStr != "" {
					gap := innerW - lipgloss.Width(line) - shortcutW
					if gap < 1 {
						gap = 1
					}
					line += strings.Repeat(" ", gap) + shortcutStr
				}

				// Full-row highlight
				lineW := lipgloss.Width(line)
				if lineW < innerW {
					line += lipgloss.NewStyle().Background(surface0).Render(strings.Repeat(" ", innerW-lineW))
				}
				b.WriteString(line)
			} else {
				iconStyle := lipgloss.NewStyle().Foreground(blue)
				nameStyle := lipgloss.NewStyle().Foreground(text)
				descStyle := lipgloss.NewStyle().Foreground(overlay0)

				left := "  " + iconStyle.Render(icon) + nameStyle.Render(cmd.Label)
				desc := descStyle.Render("  " + cmd.Desc)

				leftW := lipgloss.Width(left)
				shortcutW := lipgloss.Width(shortcutStr)
				available := innerW - leftW - shortcutW - 1
				if available > 4 {
					if lipgloss.Width(desc) > available {
						desc = descStyle.Render("  " + cmd.Desc[:maxInt(0, available-4)] + "…")
					}
				} else {
					desc = ""
				}

				line := left + desc
				if shortcutStr != "" {
					gap := innerW - lipgloss.Width(line) - shortcutW
					if gap < 1 {
						gap = 1
					}
					line += strings.Repeat(" ", gap) + shortcutStr
				}
				b.WriteString(line)
			}
			b.WriteString("\n")
		}

		// Scroll indicator
		if len(cp.filtered) > maxVisible {
			pos := fmt.Sprintf("  %d/%d", cp.cursor+1, len(cp.filtered))
			b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render(pos))
		}
	}

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 1).
		Width(width)

	return border.Render(b.String())
}

