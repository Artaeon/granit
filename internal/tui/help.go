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
			{"Ctrl+U", "Undo last edit"},
			{"Ctrl+Y", "Redo (or accept ghost writer suggestion)"},
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
			{"", "VIEWS (number keys):"},
			{"1", "Plan — overdue + today + tomorrow (grouped, with mini timeline)"},
			{"2", "Upcoming — tasks due in the next 14 days (grouped by date)"},
			{"3", "All — every task across the vault"},
			{"4", "Completed — done tasks"},
			{"5", "Calendar — monthly calendar with task dots"},
			{"6", "Kanban — 4-column board (Backlog/Todo/In Progress/Done)"},
			{"Tab", "Cycle to next view"},
			{"", ""},
			{"", "TASK OPERATIONS:"},
			{"x / Enter", "Toggle task done/undone"},
			{"a", "Add new task (saved to Tasks.md)"},
			{"d", "Set due date (opens date picker)"},
			{"r", "Reschedule (1=tomorrow 2=next week 3=+1w 4=+1m 5=custom)"},
			{"p", "Cycle priority: none → low → medium → high → highest"},
			{"e", "Expand/collapse subtasks"},
			{"n", "Add or edit a note attached to this task"},
			{"g", "Jump to task in source note (opens file)"},
			{"f", "Start focus session on this task"},
			{"b", "Add dependency (this task depends on another)"},
			{"E", "Set time estimate (~30m, ~1h, ~2h)"},
			{"z", "Snooze task (1=1h, 2=4h, 3=tomorrow 9am)"},
			{"", ""},
			{"", "AI & AUTOMATION:"},
			{"S", "AI breakdown — split task into 3-7 subtasks via LLM"},
			{"A", "Auto-suggest priority (heuristic analysis)"},
			{"R", "Batch reschedule all overdue tasks (Today view)"},
			{"", ""},
			{"", "TEMPLATES & BULK:"},
			{"T", "Save current task as reusable template"},
			{"t", "Create task from template (1-9 to pick)"},
			{"v", "Bulk select mode (space=select, x=toggle, d=date)"},
			{"X", "Archive completed tasks older than 30 days"},
			{"W", "Pin/unpin task (pinned tasks stay at top)"},
			{"", ""},
			{"", "FILTERING & SORTING:"},
			{"/", "Search tasks by text"},
			{"#", "Filter by tag"},
			{"P", "Filter by priority level"},
			{"s", "Cycle sort: priority → date → A-Z → source → tag"},
			{"c", "Clear all active filters"},
			{"u", "Undo last action"},
			{"", ""},
			{"", "Priority emojis: 🔺 highest  ⏫ high  🔼 medium  🔽 low"},
			{"", "Due date: 📅 YYYY-MM-DD    Estimate: ~30m or ~2h"},
			{"", "Recurrence: 🔁 daily/weekly/monthly"},
			{"", "Dependencies: depends:\"task name\""},
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
			{"", "FOCUS MODE (Ctrl+Z):"},
			{"Ctrl+Z", "Toggle distraction-free writing — hides all panels"},
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
			{"", "Tree Navigation:"},
			{"j / k / ↑ / ↓", "Move cursor up / down"},
			{"Enter / Space", "Open file — or expand/collapse folder"},
			{"Left / h", "Collapse folder or go to parent"},
			{"Right / l", "Expand folder or enter directory"},
			{"g / Home", "Jump to first item"},
			{"G / End", "Jump to last item"},
			{"PgUp / PgDn", "Scroll half page up / down"},
			{"", ""},
			{"", "Folder Operations:"},
			{"z", "Collapse all folders"},
			{"Z", "Expand all folders"},
			{"", ""},
			{"", "Search:"},
			{"/", "Enter search mode (fuzzy filter files)"},
			{"Backspace", "Delete search character"},
			{"Enter", "Exit search (keep filter)"},
			{"Esc", "Clear search and return to tree"},
			{"", ""},
			{"", "Tab Switching:"},
			{"Ctrl+Tab", "Switch to next open tab"},
			{"Ctrl+Shift+Tab", "Switch to previous open tab"},
			{"Ctrl+1-9", "Jump to tab by position"},
			{"", ""},
			{"", "Folder state persists across sessions."},
			{"", "Hidden files toggled via Settings → Show Hidden Files."},
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
