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
	CmdWeeklyNote
	CmdToggleView
	CmdSettings
	CmdToggleSidebar
	CmdFocusEditor
	CmdFocusSidebar
	CmdFocusBacklinks
	CmdSearchInFile // deprecated: use CmdFindInFile (kept for backward compat)
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
	CmdDailyBriefing // canonical: Morning Routine (Alt+M) dispatches here
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
	CmdLayoutZen
	CmdLayoutTaskboard
	CmdLayoutResearch
	CmdLayoutCalendar
	CmdLayoutCornell
	CmdLayoutFocus
	CmdCycleLayout
	CmdLayoutPicker
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
	CmdDailyReview
	CmdNoteHistory
	CmdSmartConnections
	CmdWritingStats
	CmdQuickCapture
	CmdDashboard
	CmdMindMap
	CmdJournalPrompts
	CmdClipManager
	CmdDailyPlanner
	CmdAIScheduler // deprecated: use CmdPlanMyDay
	CmdTaskTriage
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
	CmdToggleRegex
	CmdToggleWordWrap
	CmdKnowledgeGaps
	CmdExtractToNote
	CmdToggleSpellCheck
	CmdPrevDailyNote
	CmdNextDailyNote
	CmdPlanMyDay
	CmdCommandCenter
	CmdClockIn
	CmdClockOut
	CmdNextcloudSync
	CmdNousStatus
	CmdReadingList
	CmdWeeklyReview
	CmdAIProjectPlanner
	CmdProjectDashboard
	CmdGoalsMode
	CmdCopyDailyPlan
	CmdUniversalSearch
	CmdIdeasBoard
	CmdDailyJot
	CmdMorningRoutine
	CmdEveningReview
	CmdBlogDraft
	CmdQuit
)

// Command categories for palette grouping.
const (
	CatNavigation  = "Navigation & Files"
	CatEditor      = "Editor"
	CatSearch      = "Search"
	CatKnowledge   = "Knowledge Graph"
	CatAI          = "AI & Analysis"
	CatTasks       = "Tasks & Planning"
	CatDaily       = "Daily & Calendar"
	CatProjects    = "Projects & Goals"
	CatLearning    = "Learning"
	CatPublish     = "Publish & Sync"
	CatSettings    = "Settings & System"
)

type Command struct {
	Label    string
	Desc     string
	Shortcut string
	Action   CommandAction
	Icon     *string // pointer to icon char variable (nil = no icon)
	Category string  // palette grouping category
}

var AllCommands = []Command{
	// ── Navigation & Files ──
	{Label: "Open File", Desc: "Quick open a file", Shortcut: "Ctrl+P", Action: CmdOpenFile, Icon: &IconSearchChar, Category: CatNavigation},
	{Label: "New Note", Desc: "Create a new note", Shortcut: "Ctrl+N", Action: CmdNewNote, Icon: &IconNewChar, Category: CatNavigation},
	{Label: "Save Note", Desc: "Save the current note", Shortcut: "Ctrl+S", Action: CmdSaveNote, Icon: &IconSaveChar, Category: CatNavigation},
	{Label: "Quick Switch", Desc: "Switch between recent files", Shortcut: "Ctrl+J", Action: CmdQuickSwitch, Icon: &IconFileChar, Category: CatNavigation},
	{Label: "New from Template", Desc: "Create note from template", Shortcut: "", Action: CmdNewFromTemplate, Icon: &IconFileChar, Category: CatNavigation},
	{Label: "New Zettelkasten Note", Desc: "Create a note with unique Zettelkasten ID", Shortcut: "", Action: CmdZettelNote, Icon: &IconNewChar, Category: CatNavigation},
	{Label: "Delete Note", Desc: "Delete the current note", Shortcut: "", Action: CmdDeleteNote, Icon: &IconTrashChar, Category: CatNavigation},
	{Label: "Rename Note", Desc: "Rename the current note", Shortcut: "F4", Action: CmdRenameNote, Icon: &IconEditChar, Category: CatNavigation},
	{Label: "New Folder", Desc: "Create a new folder", Shortcut: "", Action: CmdNewFolder, Icon: &IconFolderChar, Category: CatNavigation},
	{Label: "Move File", Desc: "Move current note to a folder", Shortcut: "", Action: CmdMoveFile, Icon: &IconFolderChar, Category: CatNavigation},
	{Label: "Navigate Back", Desc: "Go to previous note in history", Shortcut: "Alt+Left", Action: CmdNavBack, Icon: &IconFolderChar, Category: CatNavigation},
	{Label: "Navigate Forward", Desc: "Go to next note in history", Shortcut: "Alt+Right", Action: CmdNavForward, Icon: &IconFolderChar, Category: CatNavigation},
	{Label: "Switch Vault", Desc: "Switch to a different vault", Shortcut: "", Action: CmdVaultSwitch, Icon: &IconFolderChar, Category: CatNavigation},
	{Label: "Pin Note", Desc: "Pin current note as a tab", Shortcut: "", Action: CmdPinNote, Icon: &IconBookmarkChar, Category: CatNavigation},
	{Label: "Unpin Note", Desc: "Unpin current note", Shortcut: "", Action: CmdUnpinNote, Icon: &IconBookmarkChar, Category: CatNavigation},
	{Label: "Pin/Unpin Tab", Desc: "Toggle pin on active tab", Shortcut: "", Action: CmdTogglePinTab, Icon: &IconBookmarkChar, Category: CatNavigation},
	{Label: "Close Other Tabs", Desc: "Close all tabs except the active one", Shortcut: "", Action: CmdCloseOtherTabs, Icon: &IconFileChar, Category: CatNavigation},
	{Label: "Close Tabs to the Right", Desc: "Close tabs after the active one", Shortcut: "", Action: CmdCloseTabsToRight, Icon: &IconFileChar, Category: CatNavigation},
	{Label: "Reopen Closed Tab", Desc: "Reopen the last closed tab", Shortcut: "", Action: CmdReopenClosedTab, Icon: &IconFileChar, Category: CatNavigation},
	{Label: "Trash", Desc: "View and restore deleted notes", Shortcut: "", Action: CmdShowTrash, Icon: &IconTrashChar, Category: CatNavigation},
	{Label: "Quick Capture", Desc: "Jot down a quick thought to inbox, daily, or tasks", Shortcut: "", Action: CmdQuickCapture, Icon: &IconNewChar, Category: CatNavigation},
	{Label: "Web Clipper", Desc: "Save a web page as a markdown note", Shortcut: "", Action: CmdWebClip, Icon: &IconSaveChar, Category: CatNavigation},

	// ── Editor ──
	{Label: "Toggle View/Edit", Desc: "Switch between view and edit mode", Shortcut: "Ctrl+E", Action: CmdToggleView, Icon: &IconViewChar, Category: CatEditor},
	{Label: "Focus Mode", Desc: "Distraction-free writing", Shortcut: "Ctrl+Z", Action: CmdFocusMode, Icon: &IconEditChar, Category: CatEditor},
	{Label: "Focus Editor", Desc: "Switch focus to the editor", Shortcut: "Alt+2", Action: CmdFocusEditor, Icon: &IconEditChar, Category: CatEditor},
	{Label: "Focus Sidebar", Desc: "Switch focus to the file sidebar", Shortcut: "Alt+1", Action: CmdFocusSidebar, Icon: &IconFolderChar, Category: CatEditor},
	{Label: "Focus Backlinks", Desc: "Switch focus to the backlinks panel", Shortcut: "Alt+3", Action: CmdFocusBacklinks, Icon: &IconLinkChar, Category: CatEditor},
	{Label: "Toggle Sidebar", Desc: "Show or hide the file sidebar", Shortcut: "", Action: CmdToggleSidebar, Icon: &IconFolderChar, Category: CatEditor},
	{Label: "Split View", Desc: "View two notes side by side", Shortcut: "", Action: CmdSplitPane, Icon: &IconViewChar, Category: CatEditor},
	{Label: "Note Preview", Desc: "Preview the note under cursor", Shortcut: "", Action: CmdNotePreview, Icon: &IconViewChar, Category: CatEditor},
	{Label: "Outline", Desc: "Show note heading outline", Shortcut: "Ctrl+O", Action: CmdShowOutline, Icon: &IconOutlineChar, Category: CatEditor},
	{Label: "Toggle Fold", Desc: "Fold/unfold section under cursor", Shortcut: "", Action: CmdFoldToggle, Icon: &IconOutlineChar, Category: CatEditor},
	{Label: "Fold All", Desc: "Collapse all sections", Shortcut: "", Action: CmdFoldAll, Icon: &IconOutlineChar, Category: CatEditor},
	{Label: "Unfold All", Desc: "Expand all sections", Shortcut: "", Action: CmdUnfoldAll, Icon: &IconOutlineChar, Category: CatEditor},
	{Label: "Toggle Vim Mode", Desc: "Enable/disable Vim keybindings", Shortcut: "", Action: CmdToggleVim, Icon: &IconEditChar, Category: CatEditor},
	{Label: "Toggle Word Wrap", Desc: "Wrap long lines at viewport width", Shortcut: "", Action: CmdToggleWordWrap, Icon: &IconEditChar, Category: CatEditor},
	{Label: "Start Macro Recording", Desc: "Record keystrokes into a Vim macro register (a-z)", Shortcut: "q+reg", Action: CmdMacroRecord, Icon: &IconEditChar, Category: CatEditor},
	{Label: "Play Macro", Desc: "Replay a recorded Vim macro register", Shortcut: "@+reg", Action: CmdMacroPlay, Icon: &IconEditChar, Category: CatEditor},
	{Label: "Table Editor", Desc: "Visual markdown table editor", Shortcut: "", Action: CmdTableEditor, Icon: &IconEditChar, Category: CatEditor},
	{Label: "Edit Frontmatter", Desc: "Structured frontmatter property editor", Shortcut: "", Action: CmdFrontmatterEdit, Icon: &IconEditChar, Category: CatEditor},
	{Label: "Spell Check", Desc: "Check spelling in current note", Shortcut: "", Action: CmdSpellCheck, Icon: &IconEditChar, Category: CatEditor},
	{Label: "Toggle Spell Check", Desc: "Enable/disable inline spell checking", Shortcut: "", Action: CmdToggleSpellCheck, Icon: &IconEditChar, Category: CatEditor},
	{Label: "Smart Paste (URL to Link)", Desc: "Paste URL as markdown link with selected text", Shortcut: "Ctrl+V", Action: CmdSmartPaste, Icon: &IconLinkChar, Category: CatEditor},
	{Label: "Extract to Note", Desc: "Move selection to a new note, leave wikilink", Shortcut: "", Action: CmdExtractToNote, Icon: &IconLinkChar, Category: CatEditor},
	{Label: "Clipboard Manager", Desc: "Browse and paste from clipboard history", Shortcut: "", Action: CmdClipManager, Icon: &IconOutlineChar, Category: CatEditor},
	{Label: "Scratchpad", Desc: "Floating persistent scratchpad", Shortcut: "", Action: CmdScratchpad, Icon: &IconEditChar, Category: CatEditor},
	{Label: "Journal Prompts", Desc: "Daily reflection prompts with guided journaling", Shortcut: "", Action: CmdJournalPrompts, Icon: &IconEditChar, Category: CatEditor},

	// ── Search ──
	{Label: "Find", Desc: "Search within current file", Shortcut: "Ctrl+F", Action: CmdFindInFile, Icon: &IconSearchChar, Category: CatSearch},
	{Label: "Find & Replace", Desc: "Find and replace in file", Shortcut: "Ctrl+H", Action: CmdReplaceInFile, Icon: &IconSearchChar, Category: CatSearch},
	{Label: "Search Vault Contents", Desc: "Full-text search across all notes", Shortcut: "", Action: CmdContentSearch, Icon: &IconSearchChar, Category: CatSearch},
	{Label: "Search Everything", Desc: "Search across notes, tasks, goals, and habits", Shortcut: "", Action: CmdUniversalSearch, Icon: &IconSearchChar, Category: CatSearch},
	{Label: "Global Search & Replace", Desc: "Find and replace across all vault files", Shortcut: "", Action: CmdGlobalReplace, Icon: &IconSearchChar, Category: CatSearch},
	{Label: "Toggle Regex Search", Desc: "Switch between plain text and regex mode in active search", Shortcut: "Alt+R", Action: CmdToggleRegex, Icon: &IconSearchChar, Category: CatSearch},
	{Label: "Similar Notes", Desc: "Find notes similar to current one (TF-IDF)", Shortcut: "", Action: CmdSimilarNotes, Icon: &IconSearchChar, Category: CatSearch},
	{Label: "Semantic Search", Desc: "AI-powered meaning-based vault search", Shortcut: "", Action: CmdSemanticSearch, Icon: &IconSearchChar, Category: CatSearch},
	{Label: "Natural Language Search", Desc: "AI-powered meaning-based vault search", Shortcut: "", Action: CmdNLSearch, Icon: &IconSearchChar, Category: CatSearch},

	// ── Knowledge Graph ──
	{Label: "Show Graph", Desc: "Show note connection graph", Shortcut: "Ctrl+G", Action: CmdShowGraph, Icon: &IconGraphChar, Category: CatKnowledge},
	{Label: "Show Tags", Desc: "Browse notes by tags", Shortcut: "Ctrl+T", Action: CmdShowTags, Icon: &IconTagChar, Category: CatKnowledge},
	{Label: "Bookmarks", Desc: "View starred & recent notes", Shortcut: "Ctrl+B", Action: CmdShowBookmarks, Icon: &IconBookmarkChar, Category: CatKnowledge},
	{Label: "Toggle Bookmark", Desc: "Star/unstar current note", Shortcut: "", Action: CmdToggleBookmark, Icon: &IconBookmarkChar, Category: CatKnowledge},
	{Label: "Auto-Link Suggestions", Desc: "Find unlinked mentions in current note", Shortcut: "", Action: CmdAutoLink, Icon: &IconLinkChar, Category: CatKnowledge},
	{Label: "Link Assistant", Desc: "Find unlinked mentions and suggest wikilinks", Shortcut: "", Action: CmdLinkAssist, Icon: &IconLinkChar, Category: CatKnowledge},
	{Label: "Smart Connections", Desc: "Find semantically related notes using content similarity", Shortcut: "", Action: CmdSmartConnections, Icon: &IconLinkChar, Category: CatKnowledge},
	{Label: "Canvas", Desc: "Visual note canvas / whiteboard", Shortcut: "Ctrl+W", Action: CmdShowCanvas, Icon: &IconCanvasChar, Category: CatKnowledge},
	{Label: "Mind Map", Desc: "Visual mind map from note headings and wikilinks", Shortcut: "", Action: CmdMindMap, Icon: &IconGraphChar, Category: CatKnowledge},
	{Label: "Timeline", Desc: "Chronological view of all notes", Shortcut: "", Action: CmdTimeline, Icon: &IconDailyChar, Category: CatKnowledge},
	{Label: "Vault Statistics", Desc: "Show vault stats & charts", Shortcut: "", Action: CmdShowStats, Icon: &IconGraphChar, Category: CatKnowledge},
	{Label: "Dataview Query", Desc: "Query notes by frontmatter properties", Shortcut: "", Action: CmdDataview, Icon: &IconGraphChar, Category: CatKnowledge},
	{Label: "Note History", Desc: "Git version timeline and diff viewer for current note", Shortcut: "", Action: CmdNoteHistory, Icon: &IconOutlineChar, Category: CatKnowledge},

	// ── AI & Analysis ──
	{Label: "AI Chat", Desc: "Ask questions about your vault", Shortcut: "", Action: CmdAIChat, Icon: &IconBotChar, Category: CatAI},
	{Label: "Chat with Note", Desc: "AI Q&A focused on current note", Shortcut: "", Action: CmdNoteChat, Icon: &IconBotChar, Category: CatAI},
	{Label: "AI Bot Panel", Desc: "AI bots for note analysis", Shortcut: "Ctrl+R", Action: CmdShowBots, Icon: &IconBotChar, Category: CatAI},
	{Label: "AI Compose Note", Desc: "Generate a note from a topic prompt", Shortcut: "", Action: CmdComposer, Icon: &IconNewChar, Category: CatAI},
	{Label: "AI Template", Desc: "Generate a full note from a template type + topic with AI", Shortcut: "", Action: CmdAITemplate, Icon: &IconBotChar, Category: CatAI},
	{Label: "Ghost Writer", Desc: "Toggle inline AI writing suggestions", Shortcut: "", Action: CmdToggleGhostWriter, Icon: &IconEditChar, Category: CatAI},
	{Label: "Writing Coach", Desc: "AI writing analysis with persona support", Shortcut: "", Action: CmdWritingCoach, Icon: &IconBotChar, Category: CatAI},
	{Label: "Thread Weaver", Desc: "Synthesize multiple notes into a new essay", Shortcut: "", Action: CmdThreadWeaver, Icon: &IconNewChar, Category: CatAI},
	{Label: "Blog Draft", Desc: "Multi-stage AI blog post writer", Shortcut: "", Action: CmdBlogDraft, Icon: &IconEditChar, Category: CatAI},
	{Label: "Note Enhancer", Desc: "AI-enhance current note with links, structure, depth", Shortcut: "", Action: CmdNoteEnhancer, Icon: &IconEditChar, Category: CatAI},
	{Label: "Deep Dive Research", Desc: "AI research agent — create notes from any topic via Claude Code", Shortcut: "", Action: CmdResearchAgent, Icon: &IconBotChar, Category: CatAI},
	{Label: "Research Follow-Up", Desc: "Go deeper on current note's topic via Claude Code", Shortcut: "", Action: CmdResearchFollowUp, Icon: &IconBotChar, Category: CatAI},
	{Label: "Knowledge Graph AI", Desc: "Analyze clusters, hubs, orphans, suggestions", Shortcut: "", Action: CmdKnowledgeGraph, Icon: &IconGraphChar, Category: CatAI},
	{Label: "AI Knowledge Gaps", Desc: "Find missing topics, stale notes, orphans, and unlinked clusters", Shortcut: "", Action: CmdKnowledgeGaps, Icon: &IconGraphChar, Category: CatAI},
	{Label: "AI Vault Refactor", Desc: "AI reorganizes folders, names, tags, and links", Shortcut: "", Action: CmdVaultRefactor, Icon: &IconBotChar, Category: CatAI},
	{Label: "Vault Analyzer", Desc: "AI analysis of vault structure, gaps, and suggestions", Shortcut: "", Action: CmdVaultAnalyzer, Icon: &IconGraphChar, Category: CatAI},
	{Label: "Smart Task Triage", Desc: "AI-powered daily task prioritization", Shortcut: "", Action: CmdTaskTriage, Icon: &IconBotChar, Category: CatAI},
	{Label: "Nous Status", Desc: "Check local Nous AI server connection and status", Shortcut: "", Action: CmdNousStatus, Icon: &IconBotChar, Category: CatAI},

	// ── Tasks & Planning ──
	{Label: "Task Manager", Desc: "View, manage, and plan all tasks across vault", Shortcut: "Ctrl+K", Action: CmdTaskManager, Icon: &IconCalendarChar, Category: CatTasks},
	{Label: "Kanban Board", Desc: "View tasks as a Kanban board", Shortcut: "", Action: CmdKanban, Icon: &IconCanvasChar, Category: CatTasks},
	{Label: "Plan My Day", Desc: "One-click AI daily plan with schedule, goals, and advice", Shortcut: "Alt+P", Action: CmdPlanMyDay, Icon: &IconBotChar, Category: CatTasks},
	{Label: "Command Center", Desc: "What do I do RIGHT NOW? dashboard", Shortcut: "Alt+C", Action: CmdCommandCenter, Icon: &IconCalendarChar, Category: CatTasks},
	{Label: "Daily Planner", Desc: "Time-blocked daily schedule with tasks, events, habits", Shortcut: "", Action: CmdDailyPlanner, Icon: &IconCalendarChar, Category: CatTasks},
	{Label: "Recurring Tasks", Desc: "Manage daily/weekly/monthly recurring tasks", Shortcut: "", Action: CmdRecurringTasks, Icon: &IconCalendarChar, Category: CatTasks},
	{Label: "Pomodoro Timer", Desc: "Focus timer with writing stats", Shortcut: "", Action: CmdPomodoro, Icon: &IconDailyChar, Category: CatTasks},
	{Label: "Focus Session", Desc: "Guided work session with timer, tasks, and scratchpad", Shortcut: "", Action: CmdFocusSession, Icon: &IconDailyChar, Category: CatTasks},
	{Label: "Clock In", Desc: "Start a work session timer", Shortcut: "", Action: CmdClockIn, Icon: &IconDailyChar, Category: CatTasks},
	{Label: "Clock Out", Desc: "Stop work session and log time", Shortcut: "", Action: CmdClockOut, Icon: &IconDailyChar, Category: CatTasks},
	{Label: "Time Tracker", Desc: "Track time per note/task with pomodoro stats", Shortcut: "", Action: CmdTimeTracker, Icon: &IconDailyChar, Category: CatTasks},
	{Label: "Copy Daily Plan", Desc: "Copy today's schedule, tasks, and habits to clipboard for sharing", Shortcut: "", Action: CmdCopyDailyPlan, Icon: &IconCalendarChar, Category: CatTasks},

	// ── Daily & Calendar ──
	{Label: "Dashboard", Desc: "Vault home screen with tasks, notes, stats, and streaks", Shortcut: "Alt+H", Action: CmdDashboard, Icon: &IconDailyChar, Category: CatDaily},
	{Label: "Morning Routine", Desc: "Start your day — scripture, briefing, plan, top priorities", Shortcut: "Alt+M", Action: CmdMorningRoutine, Icon: &IconDailyChar, Category: CatDaily},
	{Label: "Daily Review", Desc: "End-of-day review: celebrate, reschedule, reflect", Shortcut: "Alt+E", Action: CmdDailyReview, Icon: &IconOutlineChar, Category: CatDaily},
	{Label: "Daily Note", Desc: "Open or create today's daily note", Shortcut: "Alt+D", Action: CmdDailyNote, Icon: &IconDailyChar, Category: CatDaily},
	{Label: "Previous Daily Note", Desc: "Navigate to the previous daily note", Shortcut: "Alt+[", Action: CmdPrevDailyNote, Icon: &IconDailyChar, Category: CatDaily},
	{Label: "Next Daily Note", Desc: "Navigate to the next daily note", Shortcut: "Alt+]", Action: CmdNextDailyNote, Icon: &IconDailyChar, Category: CatDaily},
	{Label: "Weekly Note", Desc: "Open or create this week's note", Shortcut: "Alt+W", Action: CmdWeeklyNote, Icon: &IconCalendarChar, Category: CatDaily},
	{Label: "Calendar", Desc: "Calendar view with daily notes", Shortcut: "Ctrl+L", Action: CmdShowCalendar, Icon: &IconCalendarChar, Category: CatDaily},
	{Label: "Daily Jot", Desc: "Quick time-stamped bullets — scroll through today, yesterday, and beyond", Shortcut: "Alt+J", Action: CmdDailyJot, Icon: &IconEditChar, Category: CatDaily},
	{Label: "Daily Standup", Desc: "Auto-generate standup from git commits, tasks, and notes", Shortcut: "", Action: CmdStandupGenerator, Icon: &IconCalendarChar, Category: CatDaily},
	{Label: "Daily Digest", Desc: "Generate weekly review from recent vault activity", Shortcut: "", Action: CmdDailyDigest, Icon: &IconCalendarChar, Category: CatDaily},
	{Label: "Weekly Review", Desc: "Guided weekly review with tasks, wins, lessons, priorities", Shortcut: "", Action: CmdWeeklyReview, Icon: &IconCalendarChar, Category: CatDaily},
	{Label: "Writing Statistics", Desc: "Word counts, writing streaks, and productivity charts", Shortcut: "", Action: CmdWritingStats, Icon: &IconGraphChar, Category: CatDaily},

	// ── Projects & Goals ──
	{Label: "Projects", Desc: "Project management with dashboards and categories", Shortcut: "", Action: CmdProjectMode, Icon: &IconFolderChar, Category: CatProjects},
	{Label: "Project Dashboard", Desc: "Cross-project overview with progress, blockers, and deadlines", Shortcut: "", Action: CmdProjectDashboard, Icon: &IconFolderChar, Category: CatProjects},
	{Label: "AI Project Planner", Desc: "Break down a project idea into phases, milestones, and tasks with AI", Shortcut: "", Action: CmdAIProjectPlanner, Icon: &IconBotChar, Category: CatProjects},
	{Label: "Goals", Desc: "Standalone goal manager with milestones, categories, timelines, and progress tracking", Shortcut: "", Action: CmdGoalsMode, Icon: &IconBookmarkChar, Category: CatProjects},
	{Label: "Ideas Board", Desc: "Kanban board for brainstorming — capture, explore, validate, and convert ideas to goals or tasks", Shortcut: "", Action: CmdIdeasBoard, Icon: &IconCanvasChar, Category: CatProjects},

	// ── Learning ──
	{Label: "Flashcards", Desc: "Spaced repetition study from your notes", Shortcut: "", Action: CmdFlashcards, Icon: &IconBookmarkChar, Category: CatLearning},
	{Label: "Quiz Mode", Desc: "Test your knowledge with auto-generated quizzes", Shortcut: "", Action: CmdQuizMode, Icon: &IconHelpChar, Category: CatLearning},
	{Label: "Learning Dashboard", Desc: "Track study progress, streaks, mastery", Shortcut: "", Action: CmdLearnDashboard, Icon: &IconGraphChar, Category: CatLearning},
	{Label: "Language Learning", Desc: "Vocabulary tracker, practice sessions, grammar notes", Shortcut: "", Action: CmdLanguageLearning, Icon: &IconBookmarkChar, Category: CatLearning},
	{Label: "Habit Tracker", Desc: "Daily habits, goals, streaks, and progress tracking", Shortcut: "", Action: CmdHabitTracker, Icon: &IconGraphChar, Category: CatLearning},
	{Label: "Reading List", Desc: "Track URLs and articles to read later", Shortcut: "", Action: CmdReadingList, Icon: &IconBookmarkChar, Category: CatLearning},

	// ── Publish & Sync ──
	{Label: "Git: Status & Commit", Desc: "Git status, log, diff, commit, push, pull", Shortcut: "", Action: CmdGitOverlay, Icon: &IconBotChar, Category: CatPublish},
	{Label: "Git History", Desc: "View commit history and diffs for current note", Shortcut: "", Action: CmdGitHistory, Icon: &IconEditChar, Category: CatPublish},
	{Label: "Export Current Note", Desc: "Export note as HTML, text, or PDF", Shortcut: "", Action: CmdExportNote, Icon: &IconSaveChar, Category: CatPublish},
	{Label: "Publish Site", Desc: "Export vault as static HTML site", Shortcut: "", Action: CmdPublishSite, Icon: &IconSaveChar, Category: CatPublish},
	{Label: "Publish to Blog", Desc: "Publish note to Medium or GitHub blog", Shortcut: "", Action: CmdBlogPublish, Icon: &IconSaveChar, Category: CatPublish},
	{Label: "Encrypt/Decrypt Note", Desc: "AES-256-GCM encryption for secure GitHub sync", Shortcut: "", Action: CmdEncryptNote, Icon: &IconSaveChar, Category: CatPublish},
	{Label: "Nextcloud Sync", Desc: "Sync vault with Nextcloud via WebDAV", Shortcut: "", Action: CmdNextcloudSync, Icon: &IconSaveChar, Category: CatPublish},
	{Label: "Vault Backup", Desc: "Create, restore, and manage vault backups", Shortcut: "", Action: CmdBackup, Icon: &IconSaveChar, Category: CatPublish},

	// ── Settings & System ──
	{Label: "Settings", Desc: "Open settings panel", Shortcut: "Ctrl+,", Action: CmdSettings, Icon: &IconSettingsChar, Category: CatSettings},
	{Label: "Help", Desc: "Show keyboard shortcuts", Shortcut: "Alt+?", Action: CmdShowHelp, Icon: &IconHelpChar, Category: CatSettings},
	{Label: "Show Tutorial", Desc: "Interactive walkthrough of Granit features", Shortcut: "", Action: CmdShowTutorial, Icon: &IconHelpChar, Category: CatSettings},
	{Label: "Theme Editor", Desc: "Create and customize color themes", Shortcut: "", Action: CmdThemeEditor, Icon: &IconSettingsChar, Category: CatSettings},
	{Label: "Plugins", Desc: "Manage and run plugins", Shortcut: "", Action: CmdPluginManager, Icon: &IconSettingsChar, Category: CatSettings},
	{Label: "Lua Scripts", Desc: "Run Lua scripts from vault or global dir", Shortcut: "", Action: CmdRunLuaScript, Icon: &IconBotChar, Category: CatSettings},
	{Label: "Import Obsidian Config", Desc: "Import settings from .obsidian/ directory", Shortcut: "", Action: CmdImportObsidian, Icon: &IconSettingsChar, Category: CatSettings},
	{Label: "Workspaces", Desc: "Save and restore named workspace layouts", Shortcut: "", Action: CmdWorkspaces, Icon: &IconViewChar, Category: CatSettings},
	{Label: "Layout Picker", Desc: "Browse and preview all layouts with visual diagrams", Shortcut: "Alt+L", Action: CmdLayoutPicker, Icon: &IconViewChar, Category: CatSettings},
	{Label: "Cycle Layout", Desc: "Switch to next layout quickly", Shortcut: "", Action: CmdCycleLayout, Icon: &IconViewChar, Category: CatSettings},
	{Label: "Image Manager", Desc: "Browse and manage vault images", Shortcut: "", Action: CmdImageManager, Icon: &IconViewChar, Category: CatSettings},
	{Label: "Refresh Vault", Desc: "Rescan vault for changes", Shortcut: "", Action: CmdRefreshVault, Category: CatSettings},
	{Label: "Quit", Desc: "Exit Granit", Shortcut: "Ctrl+Q", Action: CmdQuit, Category: CatSettings},
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

	// Support "category:" prefix filtering (e.g. "ai:", "search:", "daily:")
	var catFilter string
	if idx := strings.Index(query, ":"); idx > 0 {
		catFilter = query[:idx]
		query = strings.TrimSpace(query[idx+1:])
	}

	cp.filtered = nil
	for _, cmd := range AllCommands {
		if catFilter != "" {
			if !cmdFuzzyMatch(strings.ToLower(cmd.Category), catFilter) {
				continue
			}
		}
		if query == "" ||
			cmdFuzzyMatch(strings.ToLower(cmd.Label), query) ||
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
	if width < 60 {
		width = 60
	}
	if width > 100 {
		width = 100
	}

	// PanelBorder = 2 chars (1 left, 1 right)
	// Padding(1, 2) = 4 chars (2 left, 2 right)
	// Total overhead = 6 chars
	innerW := width - 6
	if innerW < 10 {
		innerW = 10
	}

	var b strings.Builder

	// Header
	promptStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	queryStyle := lipgloss.NewStyle().Foreground(text)
	placeholderStyle := lipgloss.NewStyle().Foreground(surface1).Italic(true)

	displayQuery := cp.query
	if displayQuery == "" {
		displayQuery = placeholderStyle.Render("Search commands...")
	} else {
		displayQuery = queryStyle.Render(displayQuery)
	}

	cursor := lipgloss.NewStyle().Foreground(blue).Bold(true).Render("▌")
	headerStr := promptStyle.Render(" " + IconSearchChar + " ") + displayQuery + cursor
	b.WriteString(lipgloss.NewStyle().Padding(0, 1).Render(headerStr) + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface0).Render(strings.Repeat("─", innerW)) + "\n")

	// Results
	maxVisible := cp.height/2 - 5
	if maxVisible < 8 {
		maxVisible = 8
	}
	if maxVisible > 18 {
		maxVisible = 18
	}

	showCategories := cp.query == ""

	if len(cp.filtered) == 0 {
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(overlay0).Italic(true).Padding(0, 2).Render("No matching commands found") + "\n")
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

			// Category header: show when browsing (no query) and category changes
			if showCategories && cmd.Category != "" {
				showHeader := i == start
				if !showHeader && i > 0 && cp.filtered[i-1].Category != cmd.Category {
					showHeader = true
				}
				if showHeader {
					label := " " + cmd.Category + " "
					headerW := lipgloss.Width(label) + 4 // "── " prefix + " " suffix
					trail := innerW - headerW
					if trail < 0 {
						trail = 0
					}
					catLine := lipgloss.NewStyle().Foreground(surface1).Render("── ") +
						lipgloss.NewStyle().Foreground(overlay0).Bold(true).Render(cmd.Category) +
						lipgloss.NewStyle().Foreground(surface1).Render(" " + strings.Repeat("─", trail))
					b.WriteString(catLine + "\n")
				}
			}

			icon := "  "
			if cmd.Icon != nil {
				icon = *cmd.Icon + " "
			}

			shortcutStr := ""
			if cmd.Shortcut != "" {
				shortcutStr = " " + cmd.Shortcut + " "
			}

			descRunes := []rune(" - " + cmd.Desc)

			// Width of purely the left base prefix and right suffix
			leftBase := "   " + icon + cmd.Label
			if i == cp.cursor {
				leftBase = "  " + icon + cmd.Label // The accent bar makes it 2 wide instead of 3 spaces
			}
			leftBaseW := lipgloss.Width(leftBase)
			shortcutW := lipgloss.Width(shortcutStr)

			availableDesc := innerW - leftBaseW - shortcutW
			displayDesc := string(descRunes)
			if availableDesc > 2 && len(descRunes) > availableDesc {
				displayDesc = string(descRunes[:availableDesc-1]) + "…"
			} else if availableDesc <= 2 {
				displayDesc = ""
			}

			if i == cp.cursor {
				leftCol := lipgloss.NewStyle().
					Foreground(mauve).
					Bold(true).
					Render(ThemeAccentBar + " ")
				leftCol += lipgloss.NewStyle().Foreground(mauve).Render(icon)
				leftCol += lipgloss.NewStyle().Foreground(text).Bold(true).Render(cmd.Label)
				leftCol += lipgloss.NewStyle().Foreground(overlay0).Render(displayDesc)

				rightCol := lipgloss.NewStyle().
					Foreground(crust).
					Background(mauve).
					Bold(true).
					Render(shortcutStr)

				leftWidth := lipgloss.Width(leftCol)
				rightWidth := lipgloss.Width(rightCol)
				pad := innerW - leftWidth - rightWidth
				if pad < 0 {
					pad = 0
				}

				rowContents := leftCol + strings.Repeat(" ", pad) + rightCol
				b.WriteString(lipgloss.NewStyle().Background(surface0).Render(rowContents) + "\n")
			} else {
				leftCol := "   " + lipgloss.NewStyle().Foreground(overlay0).Render(icon)
				leftCol += lipgloss.NewStyle().Foreground(subtext0).Render(cmd.Label)
				leftCol += lipgloss.NewStyle().Foreground(surface1).Render(displayDesc)

				rightCol := ""
				if shortcutStr != "" {
					rightCol = lipgloss.NewStyle().Foreground(overlay0).Render(shortcutStr)
				}

				leftWidth := lipgloss.Width(leftCol)
				rightWidth := lipgloss.Width(rightCol)
				pad := innerW - leftWidth - rightWidth
				if pad < 0 {
					pad = 0
				}

				rowContents := leftCol + strings.Repeat(" ", pad) + rightCol
				b.WriteString(rowContents + "\n")
			}
		}

		b.WriteString(lipgloss.NewStyle().Foreground(surface0).Render(strings.Repeat("─", innerW)) + "\n")

		helpText := " ↑/↓ Navigate • ↵ Select • Esc Close"
		posText := fmt.Sprintf("%d/%d ", cp.cursor+1, len(cp.filtered))

		footerLeft := lipgloss.NewStyle().Foreground(overlay0).Render(helpText)
		footerRight := lipgloss.NewStyle().Foreground(overlay0).Render(posText)
		gap := innerW - lipgloss.Width(footerLeft) - lipgloss.Width(footerRight)
		if gap < 0 {
			gap = 0
		}
		b.WriteString(footerLeft + strings.Repeat(" ", gap) + footerRight)
	}

	return lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(base).
		Render(b.String())
}