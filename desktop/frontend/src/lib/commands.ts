export interface Command {
  action: string
  label: string
  desc: string
  shortcut: string
  icon: string
  category: string
}

export const allCommands: Command[] = [
  // File Operations
  { action: 'open_file', label: 'Open File', desc: 'Quick open a file', shortcut: 'Ctrl+P', icon: 'search', category: 'File' },
  { action: 'new_note', label: 'New Note', desc: 'Create a new note', shortcut: 'Ctrl+N', icon: 'plus', category: 'File' },
  { action: 'save_note', label: 'Save Note', desc: 'Save the current note', shortcut: 'Ctrl+S', icon: 'save', category: 'File' },
  { action: 'delete_note', label: 'Delete Note', desc: 'Delete the current note', shortcut: '', icon: 'trash', category: 'File' },
  { action: 'rename_note', label: 'Rename Note', desc: 'Rename the current note', shortcut: 'F4', icon: 'edit', category: 'File' },
  { action: 'new_folder', label: 'New Folder', desc: 'Create a new folder', shortcut: '', icon: 'folder', category: 'File' },
  { action: 'move_file', label: 'Move File', desc: 'Move current note to a folder', shortcut: '', icon: 'folder', category: 'File' },
  { action: 'new_from_template', label: 'New from Template', desc: 'Create note from template', shortcut: '', icon: 'template', category: 'File' },
  { action: 'quick_capture', label: 'Quick Capture', desc: 'Jot down a quick thought to inbox', shortcut: '', icon: 'plus', category: 'File' },
  { action: 'refresh_vault', label: 'Refresh Vault', desc: 'Rescan vault for changes', shortcut: '', icon: 'refresh', category: 'File' },

  // Navigation
  { action: 'daily_note', label: 'Daily Note', desc: 'Open or create today\'s daily note', shortcut: 'Alt+D', icon: 'calendar', category: 'Navigate' },
  { action: 'prev_daily', label: 'Previous Daily Note', desc: 'Navigate to the previous daily note', shortcut: 'Alt+[', icon: 'calendar', category: 'Navigate' },
  { action: 'next_daily', label: 'Next Daily Note', desc: 'Navigate to the next daily note', shortcut: 'Alt+]', icon: 'calendar', category: 'Navigate' },
  { action: 'weekly_note', label: 'Weekly Note', desc: 'Open or create this week\'s note', shortcut: 'Alt+W', icon: 'calendar', category: 'Navigate' },
  { action: 'quick_switch', label: 'Quick Switch', desc: 'Switch between recent files', shortcut: 'Ctrl+J', icon: 'switch', category: 'Navigate' },
  { action: 'nav_back', label: 'Navigate Back', desc: 'Go to previous note in history', shortcut: 'Alt+Left', icon: 'arrow-left', category: 'Navigate' },
  { action: 'nav_forward', label: 'Navigate Forward', desc: 'Go to next note in history', shortcut: 'Alt+Right', icon: 'arrow-right', category: 'Navigate' },
  { action: 'vault_switch', label: 'Switch Vault', desc: 'Switch to a different vault', shortcut: '', icon: 'folder', category: 'Navigate' },

  // Editor
  { action: 'toggle_view', label: 'Toggle View/Edit', desc: 'Switch between view and edit mode', shortcut: 'Ctrl+E', icon: 'eye', category: 'Editor' },
  { action: 'find_in_file', label: 'Find in File', desc: 'Search within current file', shortcut: 'Ctrl+F', icon: 'search', category: 'Editor' },
  { action: 'replace_in_file', label: 'Find & Replace', desc: 'Find and replace in file', shortcut: 'Ctrl+H', icon: 'search', category: 'Editor' },
  { action: 'toggle_bookmark', label: 'Toggle Bookmark', desc: 'Star/unstar current note', shortcut: '', icon: 'star', category: 'Editor' },
  { action: 'focus_mode', label: 'Focus Mode', desc: 'Distraction-free writing', shortcut: 'Ctrl+Z', icon: 'maximize', category: 'Editor' },
  { action: 'toggle_vim', label: 'Toggle Vim Mode', desc: 'Enable/disable Vim keybindings', shortcut: '', icon: 'terminal', category: 'Editor' },
  { action: 'toggle_word_wrap', label: 'Toggle Word Wrap', desc: 'Wrap long lines at viewport width', shortcut: '', icon: 'wrap', category: 'Editor' },
  { action: 'extract_to_note', label: 'Extract to Note', desc: 'Move selection to a new note, leave wikilink', shortcut: '', icon: 'link', category: 'Editor' },
  { action: 'table_editor', label: 'Table Editor', desc: 'Visual markdown table editor', shortcut: '', icon: 'table', category: 'Editor' },
  { action: 'frontmatter_edit', label: 'Edit Frontmatter', desc: 'Structured frontmatter editor', shortcut: '', icon: 'edit', category: 'Editor' },
  { action: 'spell_check', label: 'Spell Check', desc: 'Check spelling in current note', shortcut: '', icon: 'check', category: 'Editor' },

  // Views & Panels
  { action: 'show_graph', label: 'Graph View', desc: 'Show note connection graph', shortcut: 'Ctrl+G', icon: 'graph', category: 'View' },
  { action: 'show_tags', label: 'Tag Browser', desc: 'Browse notes by tags', shortcut: 'Ctrl+T', icon: 'tag', category: 'View' },
  { action: 'show_outline', label: 'Outline', desc: 'Show note heading outline', shortcut: 'Ctrl+O', icon: 'list', category: 'View' },
  { action: 'show_bookmarks', label: 'Bookmarks', desc: 'View starred & recent notes', shortcut: 'Ctrl+B', icon: 'star', category: 'View' },
  { action: 'show_stats', label: 'Vault Statistics', desc: 'Show vault stats & charts', shortcut: '', icon: 'bar-chart', category: 'View' },
  { action: 'show_trash', label: 'Trash', desc: 'View and restore deleted notes', shortcut: '', icon: 'trash', category: 'View' },
  { action: 'show_calendar', label: 'Calendar', desc: 'Calendar view with daily notes', shortcut: 'Ctrl+L', icon: 'calendar', category: 'View' },
  { action: 'show_canvas', label: 'Canvas', desc: 'Visual note canvas / whiteboard', shortcut: 'Ctrl+W', icon: 'canvas', category: 'View' },
  { action: 'toggle_sidebar', label: 'Toggle Sidebar', desc: 'Show or hide the file sidebar', shortcut: '', icon: 'sidebar', category: 'View' },
  { action: 'split_pane', label: 'Split View', desc: 'View two notes side by side', shortcut: '', icon: 'columns', category: 'View' },
  { action: 'timeline', label: 'Timeline', desc: 'Chronological view of all notes', shortcut: '', icon: 'clock', category: 'View' },
  { action: 'mind_map', label: 'Mind Map', desc: 'Visual mind map from headings', shortcut: '', icon: 'graph', category: 'View' },
  { action: 'dashboard', label: 'Dashboard', desc: 'Vault home screen with tasks and stats', shortcut: '', icon: 'home', category: 'View' },
  { action: 'kanban', label: 'Kanban Board', desc: 'View tasks as a Kanban board', shortcut: '', icon: 'columns', category: 'View' },

  // Layout
  { action: 'layout_default', label: 'Default Layout', desc: '3-panel: sidebar, editor, backlinks', shortcut: '', icon: 'layout', category: 'Layout' },
  { action: 'layout_writer', label: 'Writer Layout', desc: '2-panel: sidebar, editor', shortcut: '', icon: 'layout', category: 'Layout' },
  { action: 'layout_minimal', label: 'Minimal Layout', desc: 'Editor only', shortcut: '', icon: 'layout', category: 'Layout' },
  { action: 'layout_reading', label: 'Reading Layout', desc: 'Editor + backlinks, no sidebar', shortcut: '', icon: 'layout', category: 'Layout' },
  { action: 'layout_dashboard', label: 'Dashboard Layout', desc: '4-panel overview', shortcut: '', icon: 'layout', category: 'Layout' },

  // Search
  { action: 'content_search', label: 'Search Vault', desc: 'Full-text search across all notes', shortcut: '', icon: 'search', category: 'Search' },
  { action: 'global_replace', label: 'Global Replace', desc: 'Find and replace across all files', shortcut: '', icon: 'search', category: 'Search' },
  { action: 'similar_notes', label: 'Similar Notes', desc: 'Find notes similar to current one', shortcut: '', icon: 'search', category: 'Search' },
  { action: 'auto_link', label: 'Auto-Link Suggestions', desc: 'Find unlinked mentions', shortcut: '', icon: 'link', category: 'Search' },
  { action: 'link_assist', label: 'Link Assistant', desc: 'Suggest wikilinks for current note', shortcut: '', icon: 'link', category: 'Search' },
  { action: 'smart_connections', label: 'Smart Connections', desc: 'Find semantically related notes', shortcut: '', icon: 'link', category: 'Search' },

  // AI & Bots
  { action: 'show_bots', label: 'AI Bots', desc: 'AI bots for note analysis', shortcut: 'Ctrl+R', icon: 'bot', category: 'AI' },
  { action: 'ai_chat', label: 'AI Chat', desc: 'Ask questions about your vault', shortcut: '', icon: 'bot', category: 'AI' },
  { action: 'ai_compose', label: 'AI Compose Note', desc: 'Generate a note from a prompt', shortcut: '', icon: 'bot', category: 'AI' },
  { action: 'ai_template', label: 'AI Template', desc: 'Generate from template + topic', shortcut: '', icon: 'bot', category: 'AI' },
  { action: 'writing_coach', label: 'Writing Coach', desc: 'AI writing analysis', shortcut: '', icon: 'bot', category: 'AI' },
  { action: 'note_enhancer', label: 'Note Enhancer', desc: 'AI-enhance with links and depth', shortcut: '', icon: 'bot', category: 'AI' },
  { action: 'research_agent', label: 'Deep Dive Research', desc: 'AI research agent', shortcut: '', icon: 'bot', category: 'AI' },
  { action: 'plan_my_day', label: 'Plan My Day', desc: 'AI daily plan with schedule', shortcut: 'Alt+P', icon: 'bot', category: 'AI' },
  { action: 'toggle_ghost_writer', label: 'Ghost Writer', desc: 'Toggle AI writing suggestions', shortcut: '', icon: 'bot', category: 'AI' },
  { action: 'knowledge_gaps', label: 'Knowledge Gaps', desc: 'Find missing topics and orphans', shortcut: '', icon: 'graph', category: 'AI' },
  { action: 'vault_analyzer', label: 'Vault Analyzer', desc: 'AI analysis of vault structure', shortcut: '', icon: 'graph', category: 'AI' },

  // Git
  { action: 'git_overlay', label: 'Git: Status & Commit', desc: 'Git status, log, diff, commit, push, pull', shortcut: '', icon: 'git', category: 'Git' },
  { action: 'git_history', label: 'Git History', desc: 'Commit history for current note', shortcut: '', icon: 'git', category: 'Git' },
  { action: 'note_history', label: 'Note History', desc: 'Version timeline for current note', shortcut: '', icon: 'history', category: 'Git' },

  // Export & Import
  { action: 'export_note', label: 'Export Note', desc: 'Export as HTML, text, or PDF', shortcut: '', icon: 'download', category: 'Export' },
  { action: 'publish_site', label: 'Publish Site', desc: 'Export vault as static HTML', shortcut: '', icon: 'globe', category: 'Export' },
  { action: 'vault_backup', label: 'Vault Backup', desc: 'Create and manage backups', shortcut: '', icon: 'save', category: 'Export' },
  { action: 'import_obsidian', label: 'Import Obsidian', desc: 'Import from .obsidian/ directory', shortcut: '', icon: 'download', category: 'Export' },

  // Tools
  { action: 'settings', label: 'Settings', desc: 'Open settings panel', shortcut: 'Ctrl+,', icon: 'settings', category: 'Tools' },
  { action: 'show_help', label: 'Keyboard Shortcuts', desc: 'Show all keyboard shortcuts', shortcut: 'F5', icon: 'help', category: 'Tools' },
  { action: 'plugin_manager', label: 'Plugins', desc: 'Manage and run plugins', shortcut: '', icon: 'puzzle', category: 'Tools' },
  { action: 'task_manager', label: 'Task Manager', desc: 'View all tasks across vault', shortcut: 'Ctrl+K', icon: 'check-list', category: 'Tools' },
  { action: 'pomodoro', label: 'Pomodoro Timer', desc: 'Focus timer with stats', shortcut: '', icon: 'clock', category: 'Tools' },
  { action: 'clock_in', label: 'Clock In', desc: 'Start a work session', shortcut: '', icon: 'clock', category: 'Tools' },
  { action: 'clock_out', label: 'Clock Out', desc: 'Stop session and log time', shortcut: '', icon: 'clock', category: 'Tools' },
  { action: 'habit_tracker', label: 'Habit Tracker', desc: 'Daily habits, goals, streaks', shortcut: '', icon: 'check-list', category: 'Tools' },
  { action: 'flashcards', label: 'Flashcards', desc: 'Spaced repetition study', shortcut: '', icon: 'layers', category: 'Tools' },
  { action: 'writing_stats', label: 'Writing Statistics', desc: 'Word counts and productivity', shortcut: '', icon: 'bar-chart', category: 'Tools' },
  { action: 'command_center', label: 'Command Center', desc: 'What do I do RIGHT NOW?', shortcut: 'Alt+C', icon: 'zap', category: 'Tools' },
  { action: 'show_tutorial', label: 'Tutorial', desc: 'Interactive walkthrough', shortcut: '', icon: 'help', category: 'Tools' },

  // Overlays & Misc
  { action: 'show_daily_briefing', label: 'Daily Briefing', desc: 'Morning briefing with today\'s focus', shortcut: '', icon: 'zap', category: 'Tools' },
  { action: 'show_journal_prompts', label: 'Journal Prompts', desc: 'Daily reflection prompts', shortcut: '', icon: 'edit', category: 'Tools' },
  { action: 'show_quiz', label: 'Quiz', desc: 'Auto-generated quizzes from vault', shortcut: '', icon: 'layers', category: 'Tools' },
  { action: 'show_snippets', label: 'Snippets', desc: 'Saved text snippets', shortcut: '', icon: 'save', category: 'Tools' },
  { action: 'show_dataview', label: 'Dataview', desc: 'Query notes by properties', shortcut: '', icon: 'search', category: 'Tools' },
  { action: 'show_workspaces', label: 'Workspaces', desc: 'Save and restore layouts', shortcut: '', icon: 'layout', category: 'Tools' },
  { action: 'show_encryption', label: 'Encryption', desc: 'Encrypt/decrypt notes', shortcut: '', icon: 'lock', category: 'Tools' },
  { action: 'show_recurring_tasks', label: 'Recurring Tasks', desc: 'Manage recurring tasks', shortcut: '', icon: 'check-list', category: 'Tools' },
  { action: 'show_projects', label: 'Projects', desc: 'Project & goals manager', shortcut: '', icon: 'folder', category: 'Tools' },

  // App
  { action: 'quit', label: 'Quit', desc: 'Exit Granit', shortcut: 'Ctrl+Q', icon: 'x', category: 'App' },
]

// Icon SVG paths (16x16 viewBox)
export const iconSvg: Record<string, string> = {
  'search': 'M11 7a4 4 0 1 0-8 0 4 4 0 0 0 8 0zm1.5.5-3.5 3.5',
  'plus': 'M8 2v12M2 8h12',
  'save': 'M3 3h8l2 2v8H3V3zm2 0v4h6V3m-1 7H6',
  'trash': 'M4 4h8M5 4V3h6v1M3 4h10v9H3V4m4 2v5m2-5v5',
  'edit': 'M11.5 1.5l3 3L5 14H2v-3L11.5 1.5z',
  'folder': 'M2 4h5l1-1h5a1 1 0 0 1 1 1v8a1 1 0 0 1-1 1H2V4z',
  'template': 'M3 2h10v12H3V2zm2 3h6m-6 3h4m-4 3h6',
  'refresh': 'M3 8a5 5 0 0 1 9.5-2M13 8a5 5 0 0 1-9.5 2',
  'calendar': 'M2 5h12v8H2V5zm3-2v2m4-2v2M2 8h12',
  'eye': 'M1 8s3-5 7-5 7 5 7 5-3 5-7 5-7-5-7-5zm5 0a2 2 0 1 0 4 0 2 2 0 0 0-4 0',
  'star': 'M8 1l2 5h5l-4 3 1.5 5L8 11l-4.5 3L5 9 1 6h5z',
  'maximize': 'M4 1h-3v3m12-3h-3m3 12v-3m-12 3h3',
  'graph': 'M5 5a2 2 0 1 0 0-4 2 2 0 0 0 0 4zm6 6a2 2 0 1 0 0-4 2 2 0 0 0 0 4zM6.5 4.5l4 4',
  'tag': 'M1 8V1h7l6 6-7 7-6-6zm3-4a1 1 0 1 0 0 2 1 1 0 0 0 0-2',
  'list': 'M3 4h10M3 8h10M3 12h10',
  'columns': 'M2 2h5v12H2V2zm7 0h5v12H9V2z',
  'link': 'M6.5 7.5l3-3a2.1 2.1 0 0 1 3 3l-3 3m-3-3l-3 3a2.1 2.1 0 0 0 3 3l3-3',
  'bar-chart': 'M3 13V8m4 5V5m4 8V2',
  'bot': 'M5 5h6v5H5V5zM4 10h8v2H4v-2zM6 3h4m-2-2v2m-4 4h1m7 0h1',
  'git': 'M8 2v12M4 5a2 2 0 1 0 0-4 2 2 0 0 0 0 4zm0 10a2 2 0 1 0 0-4 2 2 0 0 0 0 4zm8-5a2 2 0 1 0 0-4 2 2 0 0 0 0 4z',
  'download': 'M8 2v8m-3-3l3 3 3-3M3 12h10',
  'globe': 'M8 1a7 7 0 1 0 0 14A7 7 0 0 0 8 1zM1 8h14M8 1c2 2 3 4 3 7s-1 5-3 7M8 1c-2 2-3 4-3 7s1 5 3 7',
  'settings': 'M8 5.5a2.5 2.5 0 1 0 0 5 2.5 2.5 0 0 0 0-5zM13 8l1.5-1m-13 2L0 8m3-5L1.5 2m11 0L14 3M3 13l-1.5 1m11 0L14 13',
  'help': 'M8 1a7 7 0 1 0 0 14A7 7 0 0 0 8 1zM6 6a2 2 0 0 1 4 0c0 1-2 1.5-2 3m0 2h.01',
  'puzzle': 'M3 6V3h4a2 2 0 0 1 0 3H3zm10 0V3H9a2 2 0 0 0 0 3h4z',
  'check-list': 'M3 4l2 2 4-4m-6 6l2 2 4-4',
  'clock': 'M8 1a7 7 0 1 0 0 14A7 7 0 0 0 8 1zM8 4v4l3 2',
  'layers': 'M8 1L1 5l7 4 7-4-7-4zM1 8l7 4 7-4M1 11l7 4 7-4',
  'zap': 'M9 1L4 8h4l-1 7 5-7H8l1-7z',
  'x': 'M3 3l10 10M13 3L3 13',
  'home': 'M2 8l6-6 6 6v5a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V8z',
  'terminal': 'M2 3h12v10H2V3zm3 3l2 2-2 2m4 0h2',
  'arrow-left': 'M10 2L4 8l6 6',
  'arrow-right': 'M6 2l6 6-6 6',
  'switch': 'M3 5h10l-3-3m3 9H3l3 3',
  'history': 'M8 1a7 7 0 1 0 0 14A7 7 0 0 0 8 1zM8 4v4l-3 2',
  'sidebar': 'M2 2h12v12H2V2zm4 0v12',
  'wrap': 'M3 4h10M3 8h7a2 2 0 0 1 0 4H8l2-2m-2 4l2-2M3 12h3',
  'table': 'M2 2h12v12H2V2zm0 4h12m0 4H2m5-8v12',
  'check': 'M3 8l3 3 7-7',
  'canvas': 'M2 2h5v5H2V2zm7 0h5v5H9V2zm-7 7h5v5H2V9zm7 0h5v5H9V9z',
  'lock': 'M4 7h8v6H4V7zm2-2a2 2 0 0 1 4 0v2H6V5z',
  'layout': 'M2 2h12v12H2V2zm4 0v12m4-12v12',
}
