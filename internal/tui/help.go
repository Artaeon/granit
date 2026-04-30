package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type HelpOverlay struct {
	OverlayBase
	scroll    int
	searching bool
	query     string
}

func NewHelpOverlay() HelpOverlay {
	return HelpOverlay{}
}

func (h *HelpOverlay) Toggle() {
	h.active = !h.active
	h.scroll = 0
	h.searching = false
	h.query = ""
}

type helpSection struct {
	title    string
	bindings []helpBinding
}

type helpBinding struct {
	key  string
	desc string
}

var helpSections = []helpSection{
	// ---------------------------------------------------------------
	// GETTING STARTED
	// ---------------------------------------------------------------
	{
		title: "Getting Started",
		bindings: []helpBinding{
			{"", "Granit is a terminal-based knowledge management system."},
			{"", "It combines note-taking, task management, goals, habits,"},
			{"", "AI tools, and daily routines in a single TUI application."},
			{"", ""},
			{"", "The interface has 3 panels: Sidebar | Editor | Backlinks"},
			{"", "Use Tab to cycle between panels. Esc returns to sidebar."},
			{"", "Press Ctrl+X to open the command palette (search all commands)."},
			{"", "Press / in this help to search for any shortcut or feature."},
		},
	},
	// ---------------------------------------------------------------
	// NAVIGATION
	// ---------------------------------------------------------------
	{
		title: "Navigation",
		bindings: []helpBinding{
			{"Tab", "Cycle panels: sidebar → editor → backlinks"},
			{"Shift+Tab", "Cycle panels backward"},
			{"F1 / Alt+1", "Focus file sidebar"},
			{"F2 / Alt+2", "Focus editor"},
			{"F3 / Alt+3", "Focus backlinks panel"},
			{"Esc", "Close overlay / return to sidebar"},
			{"j / k / ↑ / ↓", "Navigate up/down in any list"},
			{"Enter", "Open selected file, link, or item"},
			{"Alt+Left", "Go back in note history"},
			{"Alt+Right", "Go forward in note history"},
		},
	},
	// ---------------------------------------------------------------
	// FILE OPERATIONS
	// ---------------------------------------------------------------
	{
		title: "File Operations",
		bindings: []helpBinding{
			{"Ctrl+P", "Quick open — fuzzy search all files in vault"},
			{"Ctrl+N", "Create a new note (with template picker)"},
			{"Ctrl+S", "Save current note to disk"},
			{"F4", "Rename current note"},
			{"Ctrl+X", "Command palette — search and run any command"},
			{"Ctrl+J", "Quick switch between recently opened files"},
			{"", ""},
			{"", "Notes are stored as plain .md files in your vault folder."},
			{"", "Links: [[note name]] creates a wikilink to another note."},
			{"", "Tags: #tag-name adds a searchable tag to your note."},
		},
	},
	// ---------------------------------------------------------------
	// EDITOR
	// ---------------------------------------------------------------
	{
		title: "Editor",
		bindings: []helpBinding{
			{"Ctrl+E", "Toggle between view mode and edit mode"},
			{"Ctrl+Z", "Undo last edit (Ctrl+U also works)"},
			{"Ctrl+Shift+Z", "Redo (Ctrl+Y also works)"},
			{"Ctrl+F", "Find text in current file"},
			{"Ctrl+H", "Find and replace in current file"},
			{"Ctrl+K", "Delete from cursor to end of line"},
			{"Ctrl+D", "Delete forward / select next occurrence of word"},
			{"Tab", "Insert indent (tab size from settings)"},
			{"Home / End", "Jump to start / end of line"},
			{"PgUp / PgDn", "Scroll one page up / down"},
			{"", ""},
			{"", "View Mode: j/k scroll, PgUp/PgDn page, g/G top/bottom"},
			{"", "Auto-save: if enabled, saves 2s after last keystroke."},
			{"", "Spell check: if enabled, underlines misspelled words."},
		},
	},
	// ---------------------------------------------------------------
	// DAILY WORKFLOW
	// ---------------------------------------------------------------
	{
		title: "Daily Workflow",
		bindings: []helpBinding{
			{"Alt+D", "Open today's daily note (auto-creates if needed)"},
			{"Alt+H", "Dashboard — scripture, tasks, projects, goals, habits, stats"},
			{"Alt+J", "Daily Jot — quick time-stamped bullets with history"},
			{"Alt+M", "Morning Routine — scripture, goal, tasks, habits, thoughts"},
			{"Alt+E", "Evening Review — reflect on accomplishments, plan tomorrow"},
			{"Alt+P", "Plan My Day — AI generates your daily schedule"},
			{"Alt+S", "Focus Session — timed work with goals and scratchpad"},
			{"Alt+B", "Habit Tracker — daily habits, goals, streaks"},
			{"Alt+T", "Time Tracker — track time per note/task"},
			{"Alt+I", "Quick Capture — jot down thoughts to inbox"},
			{"Alt+W", "Open this week's note"},
			{"Alt+[", "Navigate to previous daily note"},
			{"Alt+]", "Navigate to next daily note"},
			{"", ""},
			{"", "Morning flow: Alt+M → scripture → goal → tasks → habits → thoughts"},
			{"", "Evening flow: Alt+E → what you did → overdue audit → tomorrow"},
			{"", ""},
			{"", "Daily Jot stores entries in .granit/jots/ folder."},
			{"", "Scroll through today, yesterday, and older entries."},
			{"", "Press 't' in jot to add a task, 'e' to edit an entry."},
		},
	},
	// ---------------------------------------------------------------
	// TASK MANAGER
	// ---------------------------------------------------------------
	{
		title: "Task Manager (Ctrl+K)",
		bindings: []helpBinding{
			{"", "Open with Ctrl+K. Tasks are parsed from all vault notes."},
			{"", "Format: - [ ] task text #tag 📅 2026-04-01 🔼"},
			{"", ""},
			{"", "VIEWS (12 total — number keys jump directly):"},
			{"1", "Plan — overdue + today + tomorrow (time-blocked timeline)"},
			{"2", "Upcoming — tasks due in the next 7 days"},
			{"3", "All — every active task across the vault"},
			{"4", "Done — completed tasks (use X to archive >30d old)"},
			{"5", "Calendar — monthly grid; cursor day shows its tasks"},
			{"6", "Kanban — Backlog / Todo / In Progress / Done columns"},
			{"7", "Inbox — untriaged tasks (front-door for capture)"},
			{"8", "Stale — active tasks not touched in 7+ days"},
			{"9", "Project — grouped by Project field, section per project"},
			{"0", "Quick — priority ≥ medium AND estimate ≤ 30 min"},
			{"Tab", "Cycle to next view (also Tag / Review)"},
			{"", ""},
			{"", "CREATE & EDIT:"},
			{"a", "Add new task (auto-tagged: today on Plan, tomorrow on Upcoming)"},
			{"x / Enter", "Toggle task done/undone"},
			{"i", "Inline edit — adjust text + tags + dates in one input"},
			{".", "Quick edit — p:high d:tomorrow ~30m in one input"},
			{"d", "Set due date (date picker)"},
			{"r", "Reschedule (tomorrow / Monday / +1wk / +1mo / custom)"},
			{"p", "Cycle priority: none → low → medium → high → highest"},
			{"E", "Set time estimate (15m / 30m / 45m / 1h / 1.5h / 2h)"},
			{"e", "Expand / collapse subtasks"},
			{"b", "Add task dependency"},
			{"u", "Undo last action (10-deep stack)"},
			{"D", "Toggle compact / dense display (~60% more rows)"},
			{"", ""},
			{"", "PROJECT & TAG FILTERS:"},
			{"=", "Filter to cursor task's project (press again to clear)"},
			{"P", "Cycle priority filter: none → highest → high → medium → low"},
			{"#", "Filter by tag (popup picker)"},
			{"", ""},
			{"", "DELETE, ARCHIVE & TRIAGE:"},
			{"Ctrl+D", "Delete task — removes the line from disk (with y/n confirm)"},
			{"Delete", "Same as Ctrl+D — alternative for keyboards with a Delete key"},
			{"!", "Same as Ctrl+D — legacy alias kept for muscle memory"},
			{";", "Cycle triage state (inbox → triaged → scheduled → snoozed → dropped)"},
			{"m i", "Mark inbox"},
			{"m t", "Mark triaged (reviewed but not yet scheduled)"},
			{"m s", "Mark scheduled (committed time)"},
			{"m n", "Mark snoozed (defer)"},
			{"m d", "Mark dropped — hides from active views, recoverable"},
			{"m x", "Mark done"},
			{"X", "Archive ALL completed tasks >30 days (with confirm)"},
			{"", ""},
			{"", "FILTER & SEARCH (sticky across views and sessions):"},
			{"F", "Unified filter prompt: #tag p:high triage:scheduled sort:due text"},
			{"/", "Plain search (fuzzy — \"bygr\" matches \"buy groceries\")"},
			{"#", "Cycle single-tag filter"},
			{"P", "Cycle single-priority filter"},
			{"s", "Cycle sort (priority / date / A-Z / source / tag)"},
			{"*", "Saved filter views — name a filter combo, recall later"},
			{"c", "Clear all active filters"},
			{"", ""},
			{"", "PLANNING & FOCUS:"},
			{"B", "Time-block: schedule task to morning/midday/afternoon/evening"},
			{"n", "Add / edit task note"},
			{"z", "Snooze task (1h / 4h / tomorrow 9am)"},
			{"W", "Pin / unpin (pinned tasks sort to top)"},
			{"f", "Start focus session on cursor task"},
			{"y", "Toggle time tracker (▸ Nm badge appears on tracked task)"},
			{"R", "Batch reschedule all overdue (Plan view)"},
			{"g", "Jump to task in source note (opens file)"},
			{"", ""},
			{"", "AI & TEMPLATES:"},
			{"S", "AI breakdown — split task into subtasks via LLM"},
			{"A", "Auto-suggest priority (heuristic analysis)"},
			{"T", "Save cursor task as reusable template"},
			{"t", "Create task from template (1-9 to pick)"},
			{"v", "Bulk select mode (Space=select, x=toggle, d=date)"},
			{"", ""},
			{"", "RECOVERING DROPPED / SNOOZED TASKS:"},
			{"", "Dropped tasks are hidden but not deleted. To find them:"},
			{"", "  1. Press F to open the filter prompt"},
			{"", "  2. Type 'triage:dropped' (or 'triage:snoozed') and press Enter"},
			{"", "  3. Press 'm i' to mark a task back to inbox"},
			{"", ""},
			{"", "Priority emojis: 🔺 highest  ⏫ high  🔼 medium  🔽 low"},
			{"", "Due date: 📅 YYYY-MM-DD    Estimate: ~30m or ~2h"},
			{"", "Recurrence: 🔁 daily/weekly/monthly/3x-week"},
			{"", "Dependencies: depends:\"task name\"   Goal link: goal:G001"},
		},
	},
	// ---------------------------------------------------------------
	// EDITOR TABS — multi-tab editing in the main pane
	// ---------------------------------------------------------------
	{
		title: "Editor Tabs (Obsidian-style)",
		bindings: []helpBinding{
			{"", "Granit opens notes AND major features as tabs in the editor"},
			{"", "pane. Open as many as you want — they stack at the top."},
			{"", ""},
			{"Ctrl+Tab", "Cycle to NEXT open tab (when terminal allows it)"},
			{"Ctrl+Shift+Tab", "Cycle to PREVIOUS open tab (terminal-dependent)"},
			{"Ctrl+PageDown", "Cycle to NEXT tab — browser convention, ALWAYS works"},
			{"Ctrl+PageUp", "Cycle to PREVIOUS tab — browser convention, ALWAYS works"},
			{"Alt+.", "NEXT tab (one-handed, defeats terminal Ctrl+Tab intercepts)"},
			{"Alt+,", "PREVIOUS tab (mnemonic: < and > are , and . shifted)"},
			{"Ctrl+W", "Close current tab (saves first if it's a note)"},
			{"Ctrl+1 … Ctrl+9", "Jump directly to tab N by position"},
			{"", ""},
			{"", "Note: Ctrl+Tab is intercepted by gnome-terminal, alacritty, kitty,"},
			{"", "iTerm, and many others for THEIR own tab switching. If Ctrl+Tab"},
			{"", "doesn't work in your terminal, use Ctrl+PageDown / Alt+. instead."},
			{"", ""},
			{"", "Opening note tabs:"},
			{"Ctrl+P", "Quick open — fuzzy search any file in the vault"},
			{"Ctrl+J", "Quick switch between recently opened files"},
			{"Enter on sidebar", "Open file under cursor in current tab"},
			{"", ""},
			{"", "Opening feature tabs (each is its own tab, not an overlay):"},
			{"Ctrl+K", "Task Manager"},
			{"Ctrl+L", "Calendar"},
			{"Ctrl+G", "Note Graph"},
			{"Alt+H", "Dashboard"},
			{"Alt+B", "Habit Tracker"},
			{"Alt+X", "Spreadsheet — open .csv / .xlsx file (or pick template)"},
			{"Alt+C", "Command Center"},
			{"Ctrl+X", "Command palette — open ANY feature as a tab"},
			{"", ""},
			{"", "Layout & theme:"},
			{"F6", "Toggle light / dark theme on the fly"},
			{"Ctrl+,", "Settings (font size, theme, daily-notes folder, etc.)"},
			{"", ""},
			{"", "Notes:"},
			{"", "  • The tab bar at the very top shows all open tabs."},
			{"", "  • A leading ● in a tab title means unsaved changes (Ctrl+S)."},
			{"", "  • Auto-save (if enabled) saves 2s after the last keystroke."},
			{"", "  • Closing the last tab returns you to the file sidebar."},
			{"", "  • Tab state persists across sessions (.granit/tabs.json)."},
		},
	},
	// ---------------------------------------------------------------
	// KANBAN BOARD
	// ---------------------------------------------------------------
	{
		title: "Kanban Board",
		bindings: []helpBinding{
			{"", "Open via Ctrl+K → 6 (Kanban view) or command palette."},
			{"", "Standalone board also available (search 'Kanban' in Ctrl+X)."},
			{"", "Cards are parsed from vault tasks. Column positions persist."},
			{"", ""},
			{"h / ←", "Move to previous column"},
			{"l / →", "Move to next column"},
			{"j / ↓", "Navigate down in column"},
			{"k / ↑", "Navigate up in column"},
			{"m / Enter", "Move card to next column →"},
			{"M", "Move card to previous column ←"},
			{"x", "Toggle card done/not done"},
			{"Esc / q", "Close board (saves column positions)"},
			{"", ""},
			{"", "Configure columns in Settings → Kanban Columns."},
			{"", "Tag routing: assign tags to columns (e.g. #doing → In Progress)."},
			{"", "State persists in .granit/kanban-state.json."},
		},
	},
	// ---------------------------------------------------------------
	// GOALS
	// ---------------------------------------------------------------
	{
		title: "Goals Manager",
		bindings: []helpBinding{
			{"", "Open via command palette (search 'Goals')."},
			{"", "Goals are independent from projects — optionally linked."},
			{"", "Stored in .granit/goals.json."},
			{"", ""},
			{"a", "Create new goal (enter title → date → category)"},
			{"G", "AI generate milestones — LLM creates 4-8 concrete milestones"},
			{"m", "Add milestone manually"},
			{"x / Space", "Toggle milestone completion"},
			{"e", "Expand/collapse milestones"},
			{"d", "Set target date"},
			{"s", "Change status: active → paused → completed → archived"},
			{"C", "Set goal color"},
			{"n", "Edit goal notes"},
			{"J / K", "Reorder milestones within a goal"},
			{"", ""},
			{"", "VIEWS:"},
			{"1", "All goals"},
			{"2", "Active goals only"},
			{"3", "Completed goals"},
			{"4", "Goals needing review"},
			{"", ""},
			{"", "Review: set review frequency (weekly/monthly/quarterly)."},
			{"", "Goals appear in Dashboard and AI project planner context."},
		},
	},
	// ---------------------------------------------------------------
	// PROJECTS
	// ---------------------------------------------------------------
	{
		title: "Projects Manager",
		bindings: []helpBinding{
			{"", "Open via command palette (search 'Projects')."},
			{"", "Projects have phases, tasks, categories, priorities, and notes."},
			{"", "Stored in .granit/projects.json."},
			{"", ""},
			{"a", "Create new project"},
			{"e", "Edit project details"},
			{"s", "Change status (active/paused/completed/archived)"},
			{"c", "Set category (development/business/personal/health/etc.)"},
			{"d", "Set due date"},
			{"p", "Set priority (0-4)"},
			{"n", "Open project notes"},
			{"g", "Manage project goals and phases"},
			{"", ""},
			{"", "AI Project Planner: search 'AI Project Planner' in Ctrl+X."},
			{"", "Enter a project name and description — AI generates phases,"},
			{"", "milestones, tasks, folder structure, and tags."},
			{"", "Also auto-creates a linked Goal entry."},
		},
	},
	// ---------------------------------------------------------------
	// CALENDAR
	// ---------------------------------------------------------------
	{
		title: "Calendar (Ctrl+L)",
		bindings: []helpBinding{
			{"h / ←", "Previous day"},
			{"l / →", "Next day"},
			{"k / ↑", "Previous week"},
			{"j / ↓", "Next week"},
			{"[ / ]", "Previous / next month"},
			{"{ / }", "Previous / next year"},
			{"t / g", "Jump to today"},
			{"w / v", "Cycle view: Month → Week → 3-Day → Day → Agenda → Year"},
			{"a", "Create new event (full form: title, time, duration, location, color)"},
			{"e", "Edit event under cursor (pre-populates form)"},
			{"d", "Delete event under cursor"},
			{"b", "Block task at cursor hour (week/day views)"},
			{"Enter", "Select date / open daily note for that date"},
			{"", ""},
			{"", "Week/Day views use ½-hour grid with event detail popup."},
			{"", "Events support: time, duration, location, recurrence, color."},
		},
	},
	// ---------------------------------------------------------------
	// DAILY PLANNER
	// ---------------------------------------------------------------
	{
		title: "Daily Planner",
		bindings: []helpBinding{
			{"", "Open via command palette (search 'Daily Planner')."},
			{"", "Time-blocked schedule from 06:00 to 22:00 (30min slots)."},
			{"", "Three panels: Schedule | Unscheduled Tasks | Habits."},
			{"", ""},
			{"Tab", "Switch between panels"},
			{"j / k", "Navigate up/down within panel"},
			{"a", "Add block (1=task, 2=event, 3=break, 4=focus)"},
			{"d", "Delete block"},
			{"Space", "Toggle block completion"},
			{"f", "Start focus session on block"},
			{"m", "Move block to different time slot"},
			{"[ / ]", "Previous / next day"},
			{"s", "Save plan to file"},
			{"c", "Copy plan to clipboard"},
		},
	},
	// ---------------------------------------------------------------
	// FOCUS & PRODUCTIVITY
	// ---------------------------------------------------------------
	{
		title: "Focus & Productivity",
		bindings: []helpBinding{
			{"", "FOCUS MODE (Alt+Z):"},
			{"Alt+Z", "Toggle distraction-free writing — hides all panels"},
			{"", "Centers editor, optional word count goal."},
			{"", ""},
			{"", "FOCUS SESSION (Alt+S or task manager 'f'):"},
			{"", "Guided 4-phase work session:"},
			{"", "  1. Setup: pick duration (15/25/50/90min) and task"},
			{"", "  2. Work: timer + scratchpad for notes"},
			{"", "  3. Break: optional rest (5/10/15min)"},
			{"", "  4. Review: summarize what you accomplished"},
			{"Ctrl+P", "Pause/resume timer during session"},
			{"", ""},
			{"", "POMODORO TIMER (via command palette):"},
			{"", "Sessions auto-sync to Time Tracker."},
			{"Space", "Start/pause timer"},
			{"s", "Skip current phase (work/break)"},
			{"r", "Reset timer"},
			{"a", "Add task to pomodoro queue"},
			{"q", "Toggle queue panel visibility"},
			{"", "Phases: 25min work → 5min break → 25min → 15min long break"},
			{"", ""},
			{"", "TIME TRACKER (Alt+T):"},
			{"", "Tracks time per note/task. Clock in/out for sessions."},
			{"", "Data stored in .granit/timetracker.json."},
		},
	},
	// ---------------------------------------------------------------
	// HABITS
	// ---------------------------------------------------------------
	{
		title: "Habit Tracker",
		bindings: []helpBinding{
			{"Alt+B", "Open Habit Tracker"},
			{"", "Track daily habits with streaks and completion rates."},
			{"", ""},
			{"Space / Enter", "Toggle habit completion for today"},
			{"n", "Create new habit"},
			{"d", "Delete habit (with confirmation)"},
			{"1 / 2 / 3", "Switch tab: Habits | Goals | Streaks"},
			{"", ""},
			{"", "Habits appear in the Dashboard and Daily Planner."},
			{"", "Streaks reset if you miss a day. Stay consistent!"},
		},
	},
	// ---------------------------------------------------------------
	// AI FEATURES
	// ---------------------------------------------------------------
	{
		title: "AI Features",
		bindings: []helpBinding{
			{"", "PROVIDERS (configure in Settings → AI section):"},
			{"", "  Ollama — free, local LLMs (default: qwen2.5:0.5b)"},
			{"", "  OpenAI — cloud API (requires API key)"},
			{"", "  Nous   — local AI server at localhost:3333"},
			{"", "  Nerve  — local chatbot binary (multi-provider)"},
			{"", ""},
			{"", "AI TOOLS:"},
			{"S (tasks)", "Break task into subtasks via AI"},
			{"G (goals)", "Generate milestones for a goal via AI"},
			{"Alt+P", "Plan My Day — AI daily schedule with visual timeline (→22:00)"},
			{"Alt+M", "Morning Routine with AI briefing"},
			{"Ctrl+R", "AI research agent on current note (uses Claude)"},
			{"", ""},
			{"", "AI Chat: ask questions about your vault (Ctrl+X → 'AI Chat')"},
			{"", "AI Compose: generate a note from a topic prompt"},
			{"", "AI Templates: generate structured notes (meeting, blog, etc.)"},
			{"", "AI Project Planner: break down project into phases and tasks"},
			{"", "AI Scheduler: generate optimal daily schedule"},
			{"", "Writing Coach: AI analyzes writing quality"},
			{"", "Thread Weaver: synthesize multiple notes into one"},
			{"", "Note Chat: Q&A focused on a single note"},
			{"", ""},
			{"", "Ghost Writer (toggle in Settings):"},
			{"", "  Inline AI completions appear as you type."},
			{"", "  Tab — accept suggestion, Esc — dismiss."},
			{"", ""},
			{"", "Auto-Tagger (toggle in Settings):"},
			{"", "  Suggests tags automatically when you save a note."},
		},
	},
	// ---------------------------------------------------------------
	// VIEWS & TOOLS
	// ---------------------------------------------------------------
	{
		title: "Views & Tools",
		bindings: []helpBinding{
			{"Ctrl+G", "Note graph — visual map of connections between notes"},
			{"Ctrl+T", "Tag browser — browse and filter notes by tags"},
			{"Ctrl+O", "Outline — heading structure of current note"},
			{"Ctrl+B", "Bookmarks — starred and recently opened notes"},
			{"Ctrl+W", "Canvas — visual whiteboard with cards and links"},
			{"Ctrl+L", "Calendar — month/week/day/agenda views"},
			{"Ctrl+/", "Universal search — across notes, tasks, goals, habits"},
			{"Alt+C", "Command Center — 'what do I do right now?' dashboard"},
			{"Alt+G", "Git overlay — status, diff, commit, push, pull"},
			{"", ""},
			{"", "More tools via command palette (Ctrl+X):"},
			{"", "  Timeline, Mind Map, Dataview, Reading List,"},
			{"", "  Ideas Board, Knowledge Gaps, Note History,"},
			{"", "  Vault Stats, Flashcards, Quiz Mode, Language Learning,"},
			{"", "  Spell Check, Image Manager, Theme Editor, Workspaces"},
		},
	},
	// ---------------------------------------------------------------
	// CANVAS / WHITEBOARD
	// ---------------------------------------------------------------
	{
		title: "Canvas (Ctrl+W)",
		bindings: []helpBinding{
			{"↑/↓/←/→", "Move cursor on canvas"},
			{"n", "Create new card (enter title)"},
			{"d / x", "Delete card at cursor"},
			{"m", "Move card (arrow keys → Enter to place)"},
			{"L", "Link two cards (select source, then target)"},
			{"c", "Cycle card color"},
			{"+ / -", "Increase / decrease card width"},
			{"z", "Cycle zoom: Normal → Compact → Expanded"},
			{"Enter", "Open card's linked note in editor"},
			{"Esc / q", "Save and close canvas"},
		},
	},
	// ---------------------------------------------------------------
	// VIM MODE
	// ---------------------------------------------------------------
	{
		title: "Vim Mode (enable in Settings)",
		bindings: []helpBinding{
			{"", "MODES: Normal (default) | Insert | Visual | Command"},
			{"", ""},
			{"", "NORMAL MODE — movement and commands:"},
			{"h/j/k/l", "Left / Down / Up / Right"},
			{"w / b / e", "Word forward / backward / end"},
			{"0 / ^ / $", "Line start / first char / line end"},
			{"gg / G", "File start / file end"},
			{"{count}G", "Go to line number"},
			{"", ""},
			{"i / a", "Insert before / after cursor"},
			{"I / A", "Insert at line start / end"},
			{"o / O", "New line below / above"},
			{"dd / D", "Delete line / delete to end"},
			{"cc / C", "Change line / change to end"},
			{"yy / p / P", "Copy line / paste after / paste before"},
			{"x", "Delete character under cursor"},
			{"u / Ctrl+R", "Undo / Redo"},
			{".", "Repeat last action"},
			{"J", "Join current line with next"},
			{"", ""},
			{"v / V", "Visual char / line selection"},
			{"d / y / c", "Delete / yank / change selection (in visual)"},
			{"", ""},
			{"/ / ?", "Search forward / backward"},
			{"n / N", "Next / previous search match"},
			{"", ""},
			{":w", "Save"},
			{":q", "Close overlay"},
			{":wq", "Save and close"},
			{"", ""},
			{"q{a-z}", "Start macro recording to register"},
			{"@{a-z}", "Play macro from register"},
			{"@@", "Replay last macro"},
		},
	},
	// ---------------------------------------------------------------
	// GRAPH VIEW
	// ---------------------------------------------------------------
	{
		title: "Graph View (Ctrl+G)",
		bindings: []helpBinding{
			{"j / k", "Navigate between nodes"},
			{"Enter", "Open selected note in editor"},
			{"Tab", "Cycle through related nodes"},
			{"1", "Local graph — 2-hop neighbors of current note"},
			{"2", "All notes — full vault graph"},
			{"Esc", "Close graph view"},
			{"", ""},
			{"", "Colors: current=highlight, hubs=bright, orphans=dim."},
			{"", "Connections based on [[wikilinks]] between notes."},
		},
	},
	// ---------------------------------------------------------------
	// SIDEBAR
	// ---------------------------------------------------------------
	{
		title: "Explorer / Sidebar",
		bindings: []helpBinding{
			{"", "MODES (cycle with `m` when sidebar focused):"},
			{"files", "Folder/file tree (default) — every .md note in the vault"},
			{"types", "Typed objects grouped by Type — Capacities-style; only typed notes"},
			{"", ""},
			{"", "Tree Navigation:"},
			{"j / k / ↑ / ↓", "Move cursor up / down"},
			{"Enter / Space", "Open file — or expand/collapse folder"},
			{"Left / h", "Collapse folder or go to parent"},
			{"Right / l", "Expand folder or enter directory"},
			{"g / Home", "Jump to first item"},
			{"G / End", "Jump to last item"},
			{"PgUp / PgDn", "Scroll half page up / down"},
			{"", ""},
			{"", "Folder Operations (Files mode):"},
			{"z", "Collapse all folders"},
			{"Z", "Expand all folders"},
			{"Ctrl+T", "Toggle tree view ↔ flat view"},
			{"", ""},
			{"", "Bookmarks:"},
			{"b", "Pin / unpin the file or typed-object under cursor"},
			{"R", "Reveal active note in tree (expand parent folders, scroll, focus)"},
			{"s", "Cycle sort: name → modified → created → name"},
			{"", ""},
			{"", "Types mode (m to switch):"},
			{"j / k", "Auto-skip type headers — feels like one continuous list"},
			{"Enter", "Open the typed object's note in the editor"},
			{"/ + text", "Filter objects by title (case-insensitive); type headers stay visible"},
			{"", ""},
			{"", "Search:"},
			{"/", "Enter search mode (fuzzy filter files / objects)"},
			{"Backspace", "Delete search character"},
			{"Enter", "Exit search (keep filter)"},
			{"Esc", "Clear search and return to tree / type list"},
			{"", ""},
			{"", "Tab Switching:"},
			{"Ctrl+Tab / Ctrl+PageDown / Alt+.", "Next tab"},
			{"Ctrl+Shift+Tab / Ctrl+PageUp / Alt+,", "Previous tab"},
			{"Ctrl+1-9", "Jump to tab by position"},
			{"", ""},
			{"", "Folder state persists across sessions."},
			{"", "Hidden files toggled via Settings → Show Hidden Files."},
			{"", "Pinned files persist in .granit/sidebar-pinned.json."},
		},
	},
	// ---------------------------------------------------------------
	// AGENTS — multi-step AI with tools (Alt+A)
	// ---------------------------------------------------------------
	{
		title: "Agents — multi-step AI (Alt+A)",
		bindings: []helpBinding{
			{"Alt+A", "Open the Agent Runner — pick a preset, type a goal"},
			{"", ""},
			{"", "Beyond single-shot bots, agents run a Thought →"},
			{"", "Action → Observation loop, calling tools to gather"},
			{"", "evidence from the vault before answering."},
			{"", ""},
			{"", "BUILT-IN PRESETS:"},
			{"Research Synthesizer", "Given a topic, finds related notes and"},
			{"", "summarises patterns + open questions"},
			{"", ""},
			{"", "TOOL CATALOG (read):"},
			{"read_note", "Fetch the body of a markdown note"},
			{"list_notes", "Enumerate notes under a folder"},
			{"search_vault", "Find notes mentioning a query"},
			{"query_objects", "Filter typed objects by type + property=value"},
			{"query_tasks", "Filter tasks by status / due / priority"},
			{"get_today", "Today's date — call before any date-filtered query"},
			{"", ""},
			{"", "WRITE TOOLS (gated by approval, off in v1 presets):"},
			{"write_note", "Create or overwrite a markdown note"},
			{"create_task", "Append a task to Tasks.md"},
			{"create_object", "Create a typed-object note"},
			{"", ""},
			{"", "RUNNER KEYS:"},
			{"j/k", "Pick preset"},
			{"Enter", "Confirm preset / submit goal"},
			{"Esc", "Back / cancel run / close"},
			{"n", "New run (after one completes)"},
			{"", ""},
			{"", "SAFETY:"},
			{"", "  • Path containment refuses `../` and absolute paths"},
			{"", "  • Step budget caps loops at 8 iterations"},
			{"", "  • All writes require an Approve callback"},
			{"", "  • Audit transcript shows every step + tool call"},
			{"", ""},
			{"", "Full guide: docs/AGENTS.md in the granit repository."},
		},
	},
	// ---------------------------------------------------------------
	// TYPED OBJECTS — Capacities-style structured notes (Alt+O)
	// ---------------------------------------------------------------
	{
		title: "Typed Objects (Alt+O)",
		bindings: []helpBinding{
			{"Alt+O", "Open Object Browser — typed-note galleries"},
			{"Alt+@", "Insert Typed Mention — pick an object, paste as [[wikilink]] in editor"},
			{"", ""},
			{"", "Notes can declare a frontmatter type to be treated as"},
			{"", "structured objects (Person, Book, Project, Meeting,"},
			{"", "Idea — or any custom type). The Object Browser shows"},
			{"", "them as sortable galleries grouped by type."},
			{"", ""},
			{"", "FRONTMATTER:"},
			{"type:", "Required — sets the schema (e.g. type: person)"},
			{"title:", "Promoted to the gallery's Title column"},
			{"", "Other keys map to the type's declared properties"},
			{"", ""},
			{"", "OBJECT BROWSER KEYS:"},
			{"j/k", "Move cursor in the focused pane"},
			{"Tab", "Swap focus: type list ↔ gallery grid"},
			{"Enter", "On a type: focus the grid; on an object: open the note"},
			{"/", "Filter — matches title OR any property value"},
			{"Esc", "First press clears filter; second closes the tab"},
			{"", ""},
			{"", "BUILT-IN TYPES (ship out of the box):"},
			{"person 👤", "Friend, colleague, contact"},
			{"book 📚", "Reading list (active or done)"},
			{"project 🎯", "Multi-task initiative with a deadline"},
			{"meeting 🗣️", "Notes from a meeting, call, or 1:1"},
			{"idea 💡", "Nascent concept — pre-project, pre-decision"},
			{"", ""},
			{"", "PROJECT NOTES ↔ FOLDER BRIDGE (when repo: is set):"},
			{"Alt+\\", "Open the project's repo folder in your file manager"},
			{"Alt+'", "Copy the repo's absolute path to the clipboard"},
			{"Alt+N", "Quick-add a `- [ ] ` task line at end of project/goal note"},
			{"", ""},
			{"", "REPO TRACKER (Command palette → Repo Tracker):"},
			{"", "Scans RepoScanRoot (Settings → Files) for .git folders."},
			{"o", "Open focused repo in file manager"},
			{"c", "Copy focused repo's path to clipboard"},
			{"Enter", "Import repo as project note (or jump to existing one)"},
			{"r", "Re-scan + drop status cache"},
			{"", ""},
			{"", "SAVED VIEWS (Alt+V):"},
			{"n", "Create new object of this view's type (when set)"},
			{"D / Ctrl+D", "Delete focused object (with confirm)"},
			{"/", "Filter results by title substring"},
			{"r", "Re-evaluate against the latest index"},
			{"p", "Return to view picker"},
			{"", ""},
			{"", "CUSTOM TYPES:"},
			{"", "Drop a JSON file at .granit/types/<id>.json to add or"},
			{"", "override a type. Schema: id, name, icon, properties[]."},
			{"", "Vault overrides REPLACE built-ins of the same ID."},
			{"", ""},
			{"", "Full guide: docs/OBJECTS.md in the granit repository."},
		},
	},
	// ---------------------------------------------------------------
	// EXPORT & PUBLISHING
	// ---------------------------------------------------------------
	{
		title: "Export & Publishing",
		bindings: []helpBinding{
			{"", "All via command palette (Ctrl+X):"},
			{"", "  Export Note — HTML, plain text, or PDF"},
			{"", "  Publish Site — export entire vault as static HTML"},
			{"", "  Blog Publish — publish to Medium or GitHub Pages"},
			{"", "  Backup — create/restore/manage vault backups"},
			{"", "  Nextcloud Sync — sync with WebDAV server"},
			{"", "  Git — status, commit, push, pull (Alt+G)"},
			{"", "  Encrypt Note — AES-256 encryption for sensitive notes"},
		},
	},
	// ---------------------------------------------------------------
	// GRANIT PUBLISH — static site generator (CLI feature)
	// ---------------------------------------------------------------
	{
		title: "Publish — static site generator (CLI)",
		bindings: []helpBinding{
			{"", "Render any folder of markdown notes to a black-and-white"},
			{"", "static website. GitHub-Pages-ready, no JS framework, no"},
			{"", "build step. Quit granit (Ctrl+Q) and run from your shell:"},
			{"", ""},
			{"", "BUILD A SITE:"},
			{"", "  granit publish build <folder>"},
			{"", "  granit publish build ~/Notes/Research --title 'Research'"},
			{"", ""},
			{"", "PREVIEW LOCALLY (serves on http://localhost:8080):"},
			{"", "  granit publish preview <folder>"},
			{"", ""},
			{"", "INIT A CONFIG FILE:"},
			{"", "  granit publish init <folder>"},
			{"", "  → writes .granit/publish.json with sensible defaults"},
			{"", ""},
			{"", "FLAGS:"},
			{"--output <dir>", "Output directory (default ./dist)"},
			{"--title <name>", "Site title (default: folder name)"},
			{"--homepage <file>", "Note path used as index.html (e.g. README.md)"},
			{"--no-search", "Skip search index + JS shim"},
			{"--config <path>", "Use a specific publish.json"},
			{"", ""},
			{"", "WHAT YOU GET:"},
			{"", "  • Plain HTML + one CSS file + ~30 lines of vanilla JS"},
			{"", "  • Force-directed graph SVG (deterministic, JS-free)"},
			{"", "  • Per-note Contents outline + Prev/Next navigation"},
			{"", "  • Backlinks panel auto-generated for each note"},
			{"", "  • Tag pages — one per tag, plus a tag index"},
			{"", "  • Client-side fuzzy search on the homepage"},
			{"", "  • Light + dark mode via prefers-color-scheme"},
			{"", "  • .nojekyll dropped — files starting with _ work"},
			{"", "  • All links relative — site works at any URL subpath"},
			{"", ""},
			{"", "FRONTMATTER (per-note overrides):"},
			{"title:", "Override the H1/filename title fallback"},
			{"date:", "Shown under the title; sorts the index page"},
			{"tags:", "Array OR comma-string; merged with inline #tags"},
			{"publish: false", "Hide this note even when its folder is published"},
			{"", ""},
			{"", "WIKILINKS:"},
			{"", "  [[Note Name]] — resolves to the note's HTML page"},
			{"", "  [[Note Name|Display]] — custom link text"},
			{"", "  [[Note#section]] — preserves the anchor target"},
			{"", "  Unresolved targets render as plain text (no broken links)"},
			{"", ""},
			{"", "GITHUB PAGES DEPLOY:"},
			{"", "  granit publish build ./Notes --output ./docs"},
			{"", "  git add docs/ && git commit -m 'publish' && git push"},
			{"", "  → Repo Settings → Pages → Source: branch / docs"},
			{"", ""},
			{"", "CLOUDFLARE / NETLIFY / S3:"},
			{"", "  Output is plain static files. Drop dist/ into any host:"},
			{"", "  • npx wrangler pages deploy ./dist"},
			{"", "  • netlify deploy --prod --dir ./dist"},
			{"", "  • aws s3 sync ./dist s3://my-bucket --delete"},
			{"", ""},
			{"", "Full guide: docs/PUBLISH.md in the granit repository."},
		},
	},
	// ---------------------------------------------------------------
	// QUICK REFERENCE
	// ---------------------------------------------------------------
	{
		title: "Quick Reference — Task Format",
		bindings: []helpBinding{
			{"", "Tasks in notes follow this markdown format:"},
			{"", "  - [ ] task description here"},
			{"", "  - [x] completed task"},
			{"", ""},
			{"", "Add metadata inline:"},
			{"", "  📅 2026-04-15      due date"},
			{"", "  🔺 or ⏫ or 🔼 or 🔽   priority (highest/high/med/low)"},
			{"", "  ~30m or ~2h        time estimate"},
			{"", "  🔁 daily           recurrence"},
			{"", "  #tag               categorization"},
			{"", "  depends:\"other\"     dependency"},
			{"", "  goal:G001          link to goal"},
			{"", ""},
			{"", "Example:"},
			{"", "  - [ ] Write proposal 📅 2026-04-10 ⏫ ~2h #work"},
		},
	},
	{
		title: "Quick Reference — Links & Notes",
		bindings: []helpBinding{
			{"", "Wikilinks:    [[note name]]  or  [[note|display text]]"},
			{"", "Tags:         #tag-name (lowercase, hyphens OK)"},
			{"", "Headings:     # H1  ## H2  ### H3 (outline structure)"},
			{"", "Frontmatter:  YAML between --- at top of file"},
			{"", ""},
			{"", "Daily notes:  stored in configured daily notes folder"},
			{"", "              filename format: YYYY-MM-DD.md"},
			{"", "Weekly notes: stored in weekly notes folder"},
			{"", "Templates:    .granit/templates/ folder"},
		},
	},
	{
		title: "Quick Reference — Configuration Files",
		bindings: []helpBinding{
			{"", ".granit/                  — granit data directory"},
			{"", "  config.json            — all settings"},
			{"", "  projects.json          — project definitions"},
			{"", "  goals.json             — goal definitions"},
			{"", "  kanban-state.json      — kanban column positions"},
			{"", "  task-notes.json        — notes attached to tasks"},
			{"", "  pinned-tasks.json      — pinned task positions"},
			{"", "  task-templates.json    — saved task templates"},
			{"", "  timetracker.json       — time tracking data"},
			{"", "  scriptures.md          — daily bible verses / quotes"},
			{"", "  soul-note.md           — writing style for AI coach"},
			{"", "  templates/             — note templates"},
			{"", "  jots/                  — daily jot entries"},
		},
	},
	// ---------------------------------------------------------------
	// SCRIPTURE & DASHBOARD
	// ---------------------------------------------------------------
	{
		title: "Dashboard & Scripture",
		bindings: []helpBinding{
			{"Alt+H", "Open Dashboard — your daily command center"},
			{"", ""},
			{"", "The Dashboard shows:"},
			{"", "  • Daily scripture verse (rotates by date)"},
			{"", "  • Overdue tasks warning"},
			{"", "  • Today's tasks and recent notes"},
			{"", "  • Quick stats (notes, words, tags, folders)"},
			{"", "  • Writing streak and weekly activity"},
			{"", "  • Projects & goals summary"},
			{"", "  • Business pulse (#revenue/#client/#business tasks)"},
			{"", ""},
			{"", "Quick actions in Dashboard:"},
			{"n", "New note"},
			{"t", "Open task manager"},
			{"p", "Open projects"},
			{"g", "Open goals"},
			{"d", "Open daily note"},
			{"f", "Start focus session"},
			{"", ""},
			{"", "Scripture: customize .granit/scriptures.md"},
			{"", "Format: one verse per line, separated by ' — '"},
			{"", "Example: Trust in the LORD... — Proverbs 3:5-6"},
			{"", "21 built-in verses included as defaults."},
		},
	},
	// ---------------------------------------------------------------
	// APPLICATION
	// ---------------------------------------------------------------
	{
		title: "Application",
		bindings: []helpBinding{
			{"Ctrl+Q", "Quit Granit"},
			{"Ctrl+,", "Open Settings"},
			{"F5 / Alt+?", "Show this help"},
			{"", ""},
			{"", "Layouts (via command palette):"},
			{"", "  Default   — 3-panel: sidebar + editor + backlinks"},
			{"", "  Writer    — 2-panel: sidebar + editor"},
			{"", "  Minimal   — editor only"},
			{"", "  Reading   — editor + backlinks"},
			{"", "  Dashboard — 4-panel with outline"},
			{"", ""},
			{"", "Themes: Catppuccin Mocha (default), and more."},
			{"", "Customize via Settings or Theme Editor (Ctrl+X → Theme)."},
		},
	},
}

func (h HelpOverlay) Update(msg tea.Msg) (HelpOverlay, tea.Cmd) {
	if !h.active {
		return h, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()

		if h.searching {
			switch key {
			case "esc":
				if h.query != "" {
					h.query = ""
					h.scroll = 0
				} else {
					h.searching = false
				}
			case "backspace":
				if len(h.query) > 0 {
					h.query = TrimLastRune(h.query)
					h.scroll = 0
				}
			case "enter":
				h.searching = false
			default:
				if len(key) == 1 && key[0] >= 32 {
					h.query += key
					h.scroll = 0
				}
			}
			return h, nil
		}

		switch key {
		case "esc", "f5", "q":
			h.active = false
		case "/":
			h.searching = true
			h.query = ""
		case "up", "k":
			if h.scroll > 0 {
				h.scroll--
			}
		case "down", "j":
			allLines := h.buildLines()
			visH := h.visibleHeight()
			maxScroll := len(allLines) - visH
			if maxScroll < 0 {
				maxScroll = 0
			}
			if h.scroll < maxScroll {
				h.scroll++
			}
		case "pgup":
			h.scroll -= h.visibleHeight()
			if h.scroll < 0 {
				h.scroll = 0
			}
		case "pgdown":
			allLines := h.buildLines()
			visH := h.visibleHeight()
			h.scroll += visH
			maxScroll := len(allLines) - visH
			if maxScroll < 0 {
				maxScroll = 0
			}
			if h.scroll > maxScroll {
				h.scroll = maxScroll
			}
		case "g", "home":
			h.scroll = 0
		case "G", "end":
			allLines := h.buildLines()
			visH := h.visibleHeight()
			maxScroll := len(allLines) - visH
			if maxScroll < 0 {
				maxScroll = 0
			}
			h.scroll = maxScroll
		}
	}
	return h, nil
}

func (h HelpOverlay) visibleHeight() int {
	visH := h.height - 10
	if visH < 10 {
		visH = 10
	}
	return visH
}

func (h HelpOverlay) buildLines() []string {
	query := strings.ToLower(h.query)

	var allLines []string
	for _, section := range helpSections {
		var sectionLines []string
		hasMatch := false

		for _, binding := range section.bindings {
			if query != "" {
				combined := strings.ToLower(binding.key + " " + binding.desc + " " + section.title)
				if !strings.Contains(combined, query) {
					continue
				}
			}
			hasMatch = true

			if binding.key == "" && binding.desc == "" {
				sectionLines = append(sectionLines, "")
			} else if binding.key == "" {
				sectionLines = append(sectionLines, DimStyle.Render("    "+binding.desc))
			} else {
				keyStyle := lipgloss.NewStyle().
					Foreground(lavender).
					Bold(true).
					Width(20).
					Render("    " + binding.key)
				descStyle := lipgloss.NewStyle().
					Foreground(text).
					Render(binding.desc)
				sectionLines = append(sectionLines, keyStyle+descStyle)
			}
		}

		if !hasMatch {
			continue
		}

		sectionTitle := lipgloss.NewStyle().
			Foreground(blue).
			Bold(true).
			Render("  " + section.title)
		allLines = append(allLines, "")
		allLines = append(allLines, sectionTitle)
		allLines = append(allLines, DimStyle.Render("  "+strings.Repeat("─", 30)))
		allLines = append(allLines, sectionLines...)
	}

	if len(allLines) == 0 && query != "" {
		allLines = append(allLines, "")
		allLines = append(allLines, DimStyle.Render("  No matches for \""+h.query+"\""))
		allLines = append(allLines, DimStyle.Render("  Try a shorter or different search term."))
	}

	return allLines
}

func (h HelpOverlay) View() string {
	width := h.width * 3 / 4
	if width < 60 {
		width = 60
	}
	if width > 100 {
		width = 100
	}

	var b strings.Builder

	logo := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconHelpChar + " Granit — Help & Documentation")
	countStyle := DimStyle
	totalBindings := 0
	for _, s := range helpSections {
		totalBindings += len(s.bindings)
	}
	countLabel := countStyle.Render(strings.Repeat(" ", 4) + "(" + smallNum(len(helpSections)) + " sections)")
	b.WriteString(logo + countLabel)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	b.WriteString("\n")

	if h.searching || h.query != "" {
		prompt := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  " + IconSearchChar + " ")
		qText := lipgloss.NewStyle().Foreground(text).Render(h.query)
		if h.searching {
			qText += lipgloss.NewStyle().Foreground(overlay0).Render("_")
		}
		b.WriteString(prompt + qText)
		b.WriteString("\n")
		b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
		b.WriteString("\n")
	}

	allLines := h.buildLines()

	visH := h.visibleHeight()
	if h.searching || h.query != "" {
		visH -= 2
	}

	maxScroll := len(allLines) - visH
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := h.scroll
	if scroll > maxScroll {
		scroll = maxScroll
	}

	end := scroll + visH
	if end > len(allLines) {
		end = len(allLines)
	}

	for i := scroll; i < end; i++ {
		b.WriteString(allLines[i])
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	// Scroll indicator
	if maxScroll > 0 {
		pct := 0
		if maxScroll > 0 {
			pct = scroll * 100 / maxScroll
		}
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  " + strings.Repeat("─", 20)))
		scrollInfo := lipgloss.NewStyle().Foreground(overlay0).Render(
			"  " + smallNum(scroll+1) + "-" + smallNum(end) + " of " + smallNum(len(allLines)) + " lines (" + smallNum(pct) + "%)")
		b.WriteString(scrollInfo)
	}

	b.WriteString("\n")
	if h.searching {
		b.WriteString(DimStyle.Render("  type to filter  Enter: done  Esc: clear"))
	} else {
		filterInfo := ""
		if h.query != "" {
			filterInfo = lipgloss.NewStyle().Foreground(yellow).Render("  filter: \""+h.query+"\"  ") +
				DimStyle.Render("Esc: clear  ")
		}
		b.WriteString(filterInfo + DimStyle.Render("  /: search  j/k: scroll  PgUp/PgDn  g/G: top/bottom  q: close"))
	}

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}
