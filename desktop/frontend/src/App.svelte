<script lang="ts">
  import { onMount } from 'svelte'
  import Sidebar from './lib/Sidebar.svelte'
  import Editor from './lib/Editor.svelte'
  import Preview from './lib/Preview.svelte'
  import StatusBar from './lib/StatusBar.svelte'
  import TabBar from './lib/TabBar.svelte'
  import BacklinksPanel from './lib/BacklinksPanel.svelte'
  import CommandPalette from './lib/CommandPalette.svelte'
  import Settings from './lib/Settings.svelte'
  import GraphView from './lib/GraphView.svelte'
  import TagBrowser from './lib/TagBrowser.svelte'
  import Bookmarks from './lib/Bookmarks.svelte'
  import Help from './lib/Help.svelte'
  import Outline from './lib/Outline.svelte'
  import VaultStats from './lib/VaultStats.svelte'
  import Templates from './lib/Templates.svelte'
  import Trash from './lib/Trash.svelte'
  import GitPanel from './lib/GitPanel.svelte'
  import Bots from './lib/Bots.svelte'
  import Calendar from './lib/Calendar.svelte'
  import ExportPanel from './lib/ExportPanel.svelte'
  import FindReplace from './lib/FindReplace.svelte'
  import ContentSearch from './lib/ContentSearch.svelte'
  import QuickSwitcher from './lib/QuickSwitcher.svelte'
  import Canvas from './lib/Canvas.svelte'
  import Kanban from './lib/Kanban.svelte'
  import TaskManager from './lib/TaskManager.svelte'
  import Pomodoro from './lib/Pomodoro.svelte'
  import HabitTracker from './lib/HabitTracker.svelte'
  import DailyPlanner from './lib/DailyPlanner.svelte'
  import DailyBriefing from './lib/DailyBriefing.svelte'
  import JournalPrompts from './lib/JournalPrompts.svelte'
  import WritingCoach from './lib/WritingCoach.svelte'
  import Flashcards from './lib/Flashcards.svelte'
  import Quiz from './lib/Quiz.svelte'
  import MindMap from './lib/MindMap.svelte'
  import Timeline from './lib/Timeline.svelte'
  import AiChat from './lib/AiChat.svelte'
  import Snippets from './lib/Snippets.svelte'
  import TableEditor from './lib/TableEditor.svelte'
  import PluginManager from './lib/PluginManager.svelte'
  import Dataview from './lib/Dataview.svelte'
  import NoteHistory from './lib/NoteHistory.svelte'
  import WorkspaceManager from './lib/WorkspaceManager.svelte'
  import BackupRestore from './lib/BackupRestore.svelte'
  import AutoLink from './lib/AutoLink.svelte'
  import BlogPublisher from './lib/BlogPublisher.svelte'
  import EncryptionPanel from './lib/EncryptionPanel.svelte'
  import RecurringTasks from './lib/RecurringTasks.svelte'
  import SmartConnections from './lib/SmartConnections.svelte'
  import type { NoteDetail, FolderNode, NoteInfo, Tab, BacklinkEntry } from './lib/types'

  // @ts-ignore
  const api = () => window.go?.main?.GranitApp

  // Core state
  let vaultOpen = false
  let vaultPath = ''
  let tree: FolderNode | null = null
  let notes: NoteInfo[] = []
  let activeNote: NoteDetail | null = null
  let activeNotePath = ''
  let editorContent = ''
  let dirty = false
  let mode: 'edit' | 'preview' | 'split' = 'split'
  let focusMode = false
  let showSidebar = true
  let sidebarWidth = 260
  let paletteMode: 'files' | 'commands' = 'files'

  // Tab state
  let tabs: Tab[] = []
  let activeTabIndex = -1

  // Backlinks panel
  let showBacklinks = true
  let backlinksWidth = 260
  let backlinksData: BacklinkEntry[] = []

  // Resizing
  let resizing: 'sidebar' | 'backlinks' | null = null
  let resizeStartX = 0
  let resizeStartWidth = 0

  // Status bar extras
  let cursorLine = 0
  let cursorCol = 0
  let aiProvider = ''
  let gitBranch = ''
  let currentTheme = 'catppuccin-mocha'

  // Overlay state
  let overlays: Record<string, boolean> = {}
  function openOverlay(name: string) { closeAllOverlays(); overlays[name] = true; overlays = overlays }
  function closeOverlay(name: string) { overlays[name] = false; overlays = overlays }
  function closeAllOverlays() { for (const k in overlays) overlays[k] = false; overlays = overlays }

  // Overlay data
  let settingsData: any[] = []
  let graphData: any = null
  let tagsData: any[] = []
  let notesForTag: any[] = []
  let bookmarkData: any = { starred: [], recent: [] }
  let outlineData: any[] = []
  let statsData: any = {}
  let templatesData: any[] = []
  let trashData: any[] = []
  let gitStatus: string[] = []
  let gitLog: string[] = []
  let gitDiff = ''
  let gitMessage = ''
  let botsList: any[] = []
  let botsRef: Bots
  let calendarData: any = null
  let exportMessage = ''
  let searchResults: any[] = []
  let searchBusy = false

  // New feature overlay data
  let canvasData: any = null
  let kanbanData: any = null
  let allTasks: any[] = []
  let flashcardsData: any[] = []
  let quizData: any[] = []
  let mindMapData: any = null
  let timelineData: any[] = []
  let pluginsData: any[] = []
  let dataviewResults: any[] = []
  let noteHistoryData: any[] = []
  let autoLinkData: any[] = []
  let smartConnectionsData: any[] = []
  let recurringTasksData: any[] = []
  let dailyBriefingData: any = null

  const themeNames = [
    'catppuccin-mocha', 'catppuccin-latte', 'catppuccin-frappe', 'catppuccin-macchiato',
    'tokyo-night', 'gruvbox-dark', 'nord', 'dracula',
    'solarized-dark', 'solarized-light', 'rose-pine', 'rose-pine-dawn',
    'everforest-dark', 'kanagawa', 'one-dark',
    'github-dark', 'github-light', 'ayu-dark', 'ayu-light',
    'palenight', 'synthwave-84', 'nightfox', 'vesper',
    'poimandres', 'moonlight', 'vitesse-dark', 'min-light', 'oxocarbon',
    'matrix', 'cobalt2', 'monokai-pro', 'horizon', 'zenburn', 'iceberg', 'amber',
    'high-contrast-dark', 'high-contrast-light', 'deuteranopia', 'protanopia', 'tritanopia',
  ]

  onMount(async () => {
    try {
      try { currentTheme = await api().GetTheme() || 'catppuccin-mocha'; applyTheme(currentTheme) } catch {}
      vaultOpen = await api().IsVaultOpen()
      if (vaultOpen) await loadVault()
    } catch (e) { console.log('Waiting for vault...') }
  })

  function applyTheme(name: string) {
    if (name === 'catppuccin-mocha') {
      delete document.documentElement.dataset.theme
    } else {
      document.documentElement.dataset.theme = name
    }
  }

  async function handleThemeChange() {
    applyTheme(currentTheme)
    try { await api().SetTheme(currentTheme) } catch {}
  }

  async function loadVault() {
    try {
      vaultPath = await api().GetVaultPath()
      tree = await api().GetFolderTree()
      notes = await api().GetNotes() || []
      vaultOpen = true
      // Load extras
      try { gitBranch = await api().GetGitBranch() || '' } catch {}
      try {
        const settings = await api().GetAllSettings()
        const aiSetting = settings?.find((s: any) => s.key === 'ai_provider')
        if (aiSetting) aiProvider = aiSetting.value
      } catch {}
    } catch (e) { console.error('Failed to load vault:', e) }
  }

  async function refreshTree() {
    const [t, n] = await Promise.all([api().GetFolderTree(), api().GetNotes()])
    tree = t
    notes = n || []
  }

  async function handleOpenVault() {
    try {
      const dir = await api().SelectVaultDialog()
      if (dir) await loadVault()
    } catch (e) { console.error('Failed to open vault:', e) }
  }

  // ============ Tab System ============

  async function handleSelectNote(event: CustomEvent<string>) {
    const relPath = event.detail
    if (activeTabIndex >= 0 && tabs[activeTabIndex]?.dirty) await handleSave()

    // Check if already open in a tab
    const existingIdx = tabs.findIndex(t => t.relPath === relPath)
    if (existingIdx >= 0) {
      switchToTab(existingIdx)
      return
    }

    try {
      const note = await api().GetNote(relPath)
      activeNote = note
      activeNotePath = relPath
      editorContent = note?.content || ''
      dirty = false

      tabs = [...tabs, {
        relPath,
        title: note?.title || relPath.replace('.md', ''),
        dirty: false,
        content: note?.content || '',
        scrollPos: 0,
      }]
      activeTabIndex = tabs.length - 1

      api().AddRecent(relPath).catch(() => {})
      loadBacklinks(relPath)
      loadOutline(relPath)
    } catch (e) { console.error('Failed to load note:', e) }
  }

  function switchToTab(index: number) {
    if (index === activeTabIndex || index < 0 || index >= tabs.length) return

    // Save current tab state
    if (activeTabIndex >= 0 && tabs[activeTabIndex]) {
      tabs[activeTabIndex] = { ...tabs[activeTabIndex], content: editorContent, dirty }
    }

    activeTabIndex = index
    const tab = tabs[index]
    editorContent = tab.content
    dirty = tab.dirty
    activeNotePath = tab.relPath

    api().GetNote(tab.relPath).then(note => { activeNote = note }).catch(() => {})
    loadBacklinks(tab.relPath)
    loadOutline(tab.relPath)
  }

  function closeTab(index: number) {
    tabs = tabs.filter((_, i) => i !== index)

    if (tabs.length === 0) {
      activeTabIndex = -1
      activeNote = null
      activeNotePath = ''
      editorContent = ''
      dirty = false
      backlinksData = []
      outlineData = []
      return
    }

    if (index <= activeTabIndex) {
      activeTabIndex = Math.max(0, activeTabIndex - 1)
    }

    const tab = tabs[activeTabIndex]
    editorContent = tab.content
    dirty = tab.dirty
    activeNotePath = tab.relPath

    api().GetNote(tab.relPath).then(note => { activeNote = note }).catch(() => {})
    loadBacklinks(tab.relPath)
    loadOutline(tab.relPath)
  }

  async function loadBacklinks(relPath: string) {
    try { backlinksData = await api().GetBacklinkContext(relPath) || [] } catch { backlinksData = [] }
  }

  async function loadOutline(relPath: string) {
    try { outlineData = relPath ? await api().GetOutline(relPath) || [] : [] } catch { outlineData = [] }
  }

  // ============ Core Handlers ============

  async function handleSave() {
    if (!activeNotePath || !dirty) return
    try {
      await api().SaveNote(activeNotePath, editorContent)
      dirty = false
      activeNote = await api().GetNote(activeNotePath)
      if (activeTabIndex >= 0 && tabs[activeTabIndex]) {
        tabs[activeTabIndex].dirty = false
        tabs = tabs
      }
    } catch (e) { console.error('Failed to save:', e) }
  }

  function handleContentChange(event: CustomEvent<string>) {
    editorContent = event.detail
    dirty = true
    if (activeTabIndex >= 0 && tabs[activeTabIndex]) {
      tabs[activeTabIndex].dirty = true
      tabs[activeTabIndex].content = event.detail
      tabs = tabs
    }
  }

  async function handleCreateNote() {
    const name = prompt('Note name:')
    if (!name) return
    try {
      const fm = `---\ndate: ${new Date().toISOString().split('T')[0]}\n---\n\n# ${name}\n\n`
      const relPath = await api().CreateNote(name, fm)
      await refreshTree()
      handleSelectNote(new CustomEvent('select', { detail: relPath }))
    } catch (e) { console.error('Failed to create note:', e) }
  }

  async function handleDeleteNote(event: CustomEvent<string>) {
    const relPath = event.detail
    if (!confirm(`Delete "${relPath}"?`)) return
    try {
      await api().DeleteNote(relPath)
      // Close tab if open
      const tabIdx = tabs.findIndex(t => t.relPath === relPath)
      if (tabIdx >= 0) closeTab(tabIdx)
      else if (activeNotePath === relPath) { activeNote = null; activeNotePath = ''; editorContent = '' }
      await refreshTree()
    } catch (e) { console.error('Failed to delete:', e) }
  }

  function handleWikilinkClick(event: CustomEvent<string>) {
    const target = event.detail
    const match = notes.find(n => n.title.toLowerCase() === target.toLowerCase() || n.relPath.toLowerCase() === target.toLowerCase() + '.md')
    if (match) handleSelectNote(new CustomEvent('select', { detail: match.relPath }))
  }

  // ============ Resize Panels ============

  function startResize(panel: 'sidebar' | 'backlinks', e: MouseEvent) {
    resizing = panel
    resizeStartX = e.clientX
    resizeStartWidth = panel === 'sidebar' ? sidebarWidth : backlinksWidth
    e.preventDefault()
  }

  function handleMouseMove(e: MouseEvent) {
    if (!resizing) return
    const delta = e.clientX - resizeStartX
    if (resizing === 'sidebar') {
      sidebarWidth = Math.max(180, Math.min(400, resizeStartWidth + delta))
    } else {
      backlinksWidth = Math.max(180, Math.min(400, resizeStartWidth - delta))
    }
  }

  function stopResize() {
    if (resizing) {
      resizing = null
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
    }
  }

  $: if (resizing) {
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
  }

  // ============ Overlay Openers ============

  async function openSettings() { settingsData = await api().GetAllSettings(); openOverlay('settings') }
  async function openGraph() { graphData = await api().GetGraphData(activeNotePath || ''); openOverlay('graph') }
  async function openTags() { tagsData = await api().GetAllTags() || []; openOverlay('tags') }
  async function openBookmarks() { bookmarkData = await api().GetBookmarks(); openOverlay('bookmarks') }
  async function openOutlineOverlay() { outlineData = activeNotePath ? await api().GetOutline(activeNotePath) || [] : []; openOverlay('outline') }
  async function openStats() { statsData = await api().GetVaultStats(); openOverlay('stats') }
  async function openTemplates() { templatesData = await api().GetTemplates() || []; openOverlay('templates') }
  async function openTrash() { trashData = await api().GetTrashItems() || []; openOverlay('trash') }
  async function openGit() { gitMessage = ''; await refreshGit(); openOverlay('git') }
  async function openBots() { botsList = await api().GetBotList() || []; openOverlay('bots') }
  async function openCalendar() {
    const now = new Date()
    calendarData = await api().GetCalendarData(now.getFullYear(), now.getMonth() + 1)
    openOverlay('calendar')
  }
  function openExport() { exportMessage = ''; openOverlay('export') }
  function openHelp() { openOverlay('help') }
  function openFindReplace() { openOverlay('findReplace') }
  async function openQuickSwitcher() {
    try { bookmarkData = await api().GetBookmarks() } catch {}
    openOverlay('quickSwitcher')
  }
  function openContentSearch() { searchResults = []; searchBusy = false; openOverlay('contentSearch') }
  function openCanvas() { openOverlay('canvas') }
  async function openKanban() { openOverlay('kanban') }
  async function openTaskManager() { try { allTasks = await api().GetAllTasks() || [] } catch { allTasks = [] }; openOverlay('taskManager') }
  function openPomodoro() { openOverlay('pomodoro') }
  async function openHabitTracker() { openOverlay('habitTracker') }
  async function openDailyPlanner() { openOverlay('dailyPlanner') }
  async function openDailyBriefing() { try { dailyBriefingData = await api().GetDailyBriefing() } catch { dailyBriefingData = {} }; openOverlay('dailyBriefing') }
  function openJournalPrompts() { openOverlay('journalPrompts') }
  function openWritingCoach() { openOverlay('writingCoach') }
  async function openFlashcards() { if (activeNotePath) { try { flashcardsData = await api().GetFlashcards(activeNotePath) || [] } catch { flashcardsData = [] } }; openOverlay('flashcards') }
  async function openQuiz() { if (activeNotePath) { try { quizData = await api().GetQuizQuestions(activeNotePath) || [] } catch { quizData = [] } }; openOverlay('quiz') }
  async function openMindMap() { try { mindMapData = await api().GetMindMapData(activeNotePath || '') } catch { mindMapData = null }; openOverlay('mindMap') }
  async function openTimeline() { try { timelineData = await api().GetTimeline() || [] } catch { timelineData = [] }; openOverlay('timeline') }
  function openAiChat() { openOverlay('aiChat') }
  function openSnippets() { openOverlay('snippets') }
  function openTableEditor() { openOverlay('tableEditor') }
  async function openPluginManager() { try { pluginsData = await api().GetPlugins() || [] } catch { pluginsData = [] }; openOverlay('pluginManager') }
  function openDataview() { dataviewResults = []; openOverlay('dataview') }
  async function openNoteHistory() { if (activeNotePath) { try { noteHistoryData = await api().GetNoteHistory(activeNotePath) || [] } catch { noteHistoryData = [] } }; openOverlay('noteHistory') }
  function openWorkspaceManager() { openOverlay('workspaceManager') }
  function openBackupRestore() { openOverlay('backupRestore') }
  async function openAutoLink() { if (activeNotePath) { try { autoLinkData = await api().GetAutoLinkSuggestions(activeNotePath) || [] } catch { autoLinkData = [] } }; openOverlay('autoLink') }
  function openBlogPublisher() { openOverlay('blogPublisher') }
  function openEncryption() { openOverlay('encryption') }
  async function openRecurringTasks() { try { recurringTasksData = await api().GetRecurringTasks() || [] } catch { recurringTasksData = [] }; openOverlay('recurringTasks') }
  async function openSmartConnections() { if (activeNotePath) { try { smartConnectionsData = await api().GetSmartConnections(activeNotePath) || [] } catch { smartConnectionsData = [] } }; openOverlay('smartConnections') }

  async function refreshGit() {
    const [s, l, d] = await Promise.allSettled([api().GitStatus(), api().GitLog(), api().GitDiff()])
    gitStatus = s.status === 'fulfilled' ? (s.value || []) : []
    gitLog = l.status === 'fulfilled' ? (l.value || []) : []
    gitDiff = d.status === 'fulfilled' ? (d.value || '') : ''
  }

  // ============ Command Dispatch ============

  async function dispatchCommand(action: string) {
    closeAllOverlays()
    switch (action) {
      case 'open_file': paletteMode = 'files'; openOverlay('commandPalette'); break
      case 'new_note': handleCreateNote(); break
      case 'save_note': handleSave(); break
      case 'toggle_view': cycleMode(); break
      case 'settings': openSettings(); break
      case 'show_graph': openGraph(); break
      case 'show_tags': openTags(); break
      case 'show_help': openHelp(); break
      case 'show_outline': openOutlineOverlay(); break
      case 'show_bookmarks': openBookmarks(); break
      case 'find_in_file': case 'replace_in_file': openFindReplace(); break
      case 'show_stats': openStats(); break
      case 'new_from_template': openTemplates(); break
      case 'focus_mode': focusMode = !focusMode; break
      case 'quick_switch': openQuickSwitcher(); break
      case 'content_search': openOverlay('contentSearch'); break
      case 'show_trash': openTrash(); break
      case 'show_calendar': openCalendar(); break
      case 'show_bots': openBots(); break
      case 'export_note': openExport(); break
      case 'git_overlay': openGit(); break
      case 'refresh_vault': await api().RefreshVault(); await refreshTree(); break
      case 'toggle_sidebar': showSidebar = !showSidebar; break
      case 'toggle_backlinks': showBacklinks = !showBacklinks; break
      case 'show_canvas': openCanvas(); break
      case 'show_kanban': openKanban(); break
      case 'show_task_manager': case 'task_manager': openTaskManager(); break
      case 'show_pomodoro': openPomodoro(); break
      case 'show_habits': openHabitTracker(); break
      case 'show_daily_planner': openDailyPlanner(); break
      case 'show_daily_briefing': openDailyBriefing(); break
      case 'show_journal_prompts': openJournalPrompts(); break
      case 'show_writing_coach': openWritingCoach(); break
      case 'show_flashcards': openFlashcards(); break
      case 'show_quiz': openQuiz(); break
      case 'show_mind_map': openMindMap(); break
      case 'show_timeline': openTimeline(); break
      case 'show_ai_chat': openAiChat(); break
      case 'show_snippets': openSnippets(); break
      case 'show_table_editor': openTableEditor(); break
      case 'show_plugins': openPluginManager(); break
      case 'show_dataview': openDataview(); break
      case 'show_note_history': openNoteHistory(); break
      case 'show_workspaces': openWorkspaceManager(); break
      case 'show_backup': openBackupRestore(); break
      case 'show_auto_link': openAutoLink(); break
      case 'show_blog_publisher': openBlogPublisher(); break
      case 'show_encryption': openEncryption(); break
      case 'show_recurring_tasks': openRecurringTasks(); break
      case 'show_smart_connections': openSmartConnections(); break
      case 'toggle_bookmark':
        if (activeNotePath) { await api().ToggleBookmark(activeNotePath) }
        break
      case 'delete_note':
        if (activeNotePath) handleDeleteNote(new CustomEvent('delete', { detail: activeNotePath }))
        break
      case 'rename_note':
        if (activeNotePath) {
          const newName = prompt('New name:', activeNote?.title || '')
          if (newName) { const np = await api().RenameNote(activeNotePath, newName); await refreshTree(); handleSelectNote(new CustomEvent('s', { detail: np })) }
        }
        break
      case 'daily_note':
        const today = new Date().toISOString().split('T')[0]
        const existing = notes.find(n => n.relPath === today + '.md')
        if (existing) { handleSelectNote(new CustomEvent('s', { detail: existing.relPath })) }
        else { const p = await api().CreateNote(today, `---\ndate: ${today}\ntype: daily\ntags: [daily]\n---\n\n# ${today}\n\n## Tasks\n- [ ]\n\n## Notes\n\n`); await refreshTree(); handleSelectNote(new CustomEvent('s', { detail: p })) }
        break
      case 'quit': window.close(); break
      // Aliases: commands.ts uses short names, map to show_* openers
      case 'kanban': openKanban(); break
      case 'timeline': openTimeline(); break
      case 'mind_map': openMindMap(); break
      case 'flashcards': openFlashcards(); break
      case 'ai_chat': openAiChat(); break
      case 'habit_tracker': openHabitTracker(); break
      case 'pomodoro': openPomodoro(); break
      case 'writing_coach': openWritingCoach(); break
      case 'table_editor': openTableEditor(); break
      case 'plugin_manager': openPluginManager(); break
      case 'auto_link': openAutoLink(); break
      case 'smart_connections': openSmartConnections(); break
      case 'note_history': openNoteHistory(); break
      case 'vault_backup': openBackupRestore(); break
      case 'similar_notes': openSmartConnections(); break
      case 'link_assist': openAutoLink(); break
      case 'writing_stats': openWritingCoach(); break
      case 'plan_my_day': openDailyPlanner(); break
      case 'dashboard': openDailyBriefing(); break
      case 'spell_check': break // handled by CodeMirror
      case 'split_pane': mode = 'split'; break
      case 'global_replace': openFindReplace(); break
      case 'git_history': openGit(); break
      case 'publish_site': openBlogPublisher(); break
      case 'layout_default': break
      case 'layout_writer': showBacklinks = false; break
      case 'layout_minimal': showSidebar = false; showBacklinks = false; break
      case 'layout_reading': mode = 'preview'; break
      case 'weekly_note':
        const weekStart = new Date(); weekStart.setDate(weekStart.getDate() - weekStart.getDay() + 1)
        const weekStr = weekStart.toISOString().split('T')[0]
        const weekNote = notes.find(n => n.relPath.includes(weekStr))
        if (weekNote) { handleSelectNote(new CustomEvent('s', { detail: weekNote.relPath })) }
        else { const wp = await api().CreateNote(`weekly-${weekStr}`, `---\ndate: ${weekStr}\ntype: weekly\ntags: [weekly]\n---\n\n# Week of ${weekStr}\n\n## Goals\n- [ ]\n\n## Review\n\n`); await refreshTree(); handleSelectNote(new CustomEvent('s', { detail: wp })) }
        break
      default: break
    }
  }

  function cycleMode() {
    if (mode === 'edit') mode = 'preview'
    else if (mode === 'preview') mode = 'split'
    else mode = 'edit'
  }

  // ============ Keybindings ============

  function handleKeydown(event: KeyboardEvent) {
    // Tab cycling
    if (event.ctrlKey && event.key === 'Tab') {
      event.preventDefault()
      if (tabs.length > 1) {
        const next = event.shiftKey
          ? (activeTabIndex - 1 + tabs.length) % tabs.length
          : (activeTabIndex + 1) % tabs.length
        switchToTab(next)
      }
      return
    }

    // Close tab
    if ((event.ctrlKey || event.metaKey) && event.key === 'w') {
      event.preventDefault()
      if (activeTabIndex >= 0) closeTab(activeTabIndex)
      return
    }

    // Ctrl+Shift+F for content search
    if ((event.ctrlKey || event.metaKey) && event.shiftKey && event.key.toLowerCase() === 'f') {
      event.preventDefault()
      openContentSearch()
      return
    }

    if (event.ctrlKey || event.metaKey) {
      const key = event.key.toLowerCase()
      const handlers: Record<string, () => void> = {
        'p': () => { paletteMode = 'files'; openOverlay('commandPalette') },
        'x': () => { paletteMode = 'commands'; openOverlay('commandPalette') },
        's': () => handleSave(),
        'e': () => cycleMode(),
        'n': () => handleCreateNote(),
        ',': () => openSettings(),
        'g': () => openGraph(),
        't': () => openTags(),
        'o': () => openOutlineOverlay(),
        'b': () => { if (mode !== 'edit' && mode !== 'split') openBookmarks() },
        'f': () => openFindReplace(),
        'h': () => openFindReplace(),
        'l': () => openCalendar(),
        'r': () => openBots(),
        'z': () => { focusMode = !focusMode },
        'j': () => openQuickSwitcher(),
        'k': () => dispatchCommand('task_manager'),
        'q': () => window.close(),
      }
      if (handlers[key]) { event.preventDefault(); handlers[key]() }
    }
    if (event.key === 'Escape') closeAllOverlays()
    if (event.key === 'F4') dispatchCommand('rename_note')
    if (event.key === 'F5') openHelp()
    if (event.altKey) {
      if (event.key === 'd') { event.preventDefault(); dispatchCommand('daily_note') }
    }
  }
</script>

<svelte:window on:keydown={handleKeydown} on:mousemove={handleMouseMove} on:mouseup={stopResize} />

{#if !vaultOpen}
  <div class="flex items-center justify-center h-full bg-ctp-base">
    <div class="text-center space-y-8" style="animation: fadeSlideUp 400ms ease-out">
      <pre class="text-ctp-mauve text-sm leading-tight font-mono select-none" style="filter: drop-shadow(0 0 20px color-mix(in srgb, var(--ctp-mauve) 20%, transparent))">
 ██████╗ ██████╗  █████╗ ███╗   ██╗██╗████████╗
██╔════╝ ██╔══██╗██╔══██╗████╗  ██║██║╚══██╔══╝
██║  ███╗██████╔╝███████║██╔██╗ ██║██║   ██║
██║   ██║██╔══██╗██╔══██║██║╚██╗██║██║   ██║
╚██████╔╝██║  ██║██║  ██║██║ ╚████║██║   ██║
 ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝╚═╝   ╚═╝</pre>
      <div class="space-y-1">
        <p class="text-ctp-subtext0 text-lg font-light tracking-wide">Terminal Knowledge Manager</p>
        <p class="text-ctp-surface2 text-xs tracking-wider">Notes &middot; Links &middot; Ideas</p>
      </div>
      <button on:click={handleOpenVault}
        class="block mx-auto px-10 py-3 bg-ctp-blue text-ctp-crust rounded-lg font-semibold
               hover:shadow-lg hover:shadow-ctp-blue/20 hover:-translate-y-0.5
               active:translate-y-0 transition-all duration-200 text-sm tracking-wide">
        Open Vault
      </button>
      <div class="flex items-center justify-center gap-4 text-ctp-overlay0 text-[11px]">
        <span class="flex items-center gap-1.5">
          <kbd class="bg-ctp-surface0 px-1.5 py-0.5 rounded text-[10px]">Ctrl+O</kbd> browse
        </span>
      </div>
    </div>
  </div>
{:else}
  <div class="flex flex-col h-full bg-ctp-base">
    <!-- Title Bar -->
    <div class="title-bar flex items-center h-[38px] px-4 bg-ctp-crust border-b border-ctp-surface0/40 select-none gap-3"
      style="--wails-draggable: drag">
      <!-- Sidebar toggle -->
      <button on:click={() => showSidebar = !showSidebar}
        class="w-7 h-7 flex items-center justify-center rounded-md transition-colors"
        class:text-ctp-blue={showSidebar}
        class:text-ctp-overlay1={!showSidebar}
        class:hover:bg-ctp-surface0={true}
        style="--wails-draggable: no-drag"
        data-tooltip="Toggle sidebar">
        <svg width="15" height="15" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
          {#if showSidebar}
            <rect x="1" y="2" width="5" height="12" rx="1" /><path d="M9 4h5M9 8h5M9 12h3" />
          {:else}
            <path d="M2 4h12M2 8h12M2 12h12" />
          {/if}
        </svg>
      </button>

      <!-- Vault name with icon -->
      <div class="flex items-center gap-1.5">
        <svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-mauve)" stroke-width="1.5" stroke-linecap="round" class="opacity-70">
          <path d="M2 5h5l1.5-2H13a1 1 0 0 1 1 1v7a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V5z" />
        </svg>
        <span class="text-[13px] font-semibold text-ctp-subtext0">{vaultPath ? vaultPath.split('/').pop() : 'Granit'}</span>
      </div>

      <!-- Breadcrumbs -->
      {#if activeNotePath}
        <div class="flex items-center gap-1 overflow-hidden">
          {#each activeNotePath.split('/') as segment, i}
            <svg width="8" height="8" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-surface2)" stroke-width="2" stroke-linecap="round"><path d="M6 4l4 4-4 4" /></svg>
            {#if i < activeNotePath.split('/').length - 1}
              <span class="text-[12px] text-ctp-overlay0 truncate max-w-[80px]">{segment}</span>
            {:else}
              <span class="text-[12px] text-ctp-text font-medium truncate max-w-[160px]">{segment.replace('.md', '')}</span>
            {/if}
          {/each}
          {#if dirty}
            <span class="w-2 h-2 rounded-full bg-ctp-peach animate-pulse ml-1 flex-shrink-0" title="Unsaved"></span>
          {/if}
        </div>
      {/if}

      <div class="flex-1"></div>

      <!-- Right side controls -->
      <div class="flex items-center gap-1">
        <!-- Backlinks toggle -->
        <button on:click={() => showBacklinks = !showBacklinks}
          class="w-7 h-7 flex items-center justify-center rounded-md transition-colors"
          class:text-ctp-blue={showBacklinks}
          class:bg-ctp-blue={showBacklinks}
          class:bg-opacity-10={showBacklinks}
          class:text-ctp-overlay0={!showBacklinks}
          class:hover:bg-ctp-surface0={!showBacklinks}
          style="--wails-draggable: no-drag"
          data-tooltip="Backlinks">
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
            <circle cx="5" cy="5" r="3" /><circle cx="11" cy="11" r="3" /><path d="M7.5 7.5l1 1" />
          </svg>
        </button>

        <!-- Command palette -->
        <button on:click={() => { paletteMode = 'commands'; openOverlay('commandPalette') }}
          class="w-7 h-7 flex items-center justify-center rounded-md text-ctp-overlay1 hover:bg-ctp-surface0 hover:text-ctp-text transition-colors"
          style="--wails-draggable: no-drag"
          data-tooltip="Commands (Ctrl+X)">
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
            <path d="M4 6l4 4 4-4" />
          </svg>
        </button>

        <!-- Theme dropdown -->
        <select bind:value={currentTheme} on:change={handleThemeChange}
          class="text-[11px] bg-ctp-surface0/60 text-ctp-subtext0 border border-ctp-surface0/70 rounded-md px-2 py-1 outline-none cursor-pointer hover:border-ctp-overlay0 transition-colors appearance-none pr-5"
          style="--wails-draggable: no-drag; background-image: url('data:image/svg+xml,<svg xmlns=%22http://www.w3.org/2000/svg%22 width=%228%22 height=%228%22 viewBox=%220 0 16 16%22><path fill=%22%236c7086%22 d=%22M4 6l4 4 4-4%22/></svg>'); background-repeat: no-repeat; background-position: right 6px center;">
          {#each themeNames as name}
            <option value={name}>{name}</option>
          {/each}
        </select>

        <!-- Search -->
        <button on:click={() => { paletteMode = 'files'; openOverlay('commandPalette') }}
          class="w-7 h-7 flex items-center justify-center rounded-md text-ctp-overlay1 hover:bg-ctp-surface0 hover:text-ctp-text transition-colors"
          style="--wails-draggable: no-drag"
          data-tooltip="Search (Ctrl+P)">
        <svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
          <circle cx="7" cy="7" r="4" /><path d="M10 10l3.5 3.5" />
        </svg>
      </button>
    </div>
    </div>

    {#if focusMode && activeNote}
      <!-- Focus Mode (Zen Writing) -->
      <div class="flex flex-col flex-1 min-h-0">
        <div class="flex items-center justify-between px-6 h-10 text-ctp-overlay0 text-[12px] border-b border-ctp-surface0/30 select-none">
          <button on:click={() => focusMode = false}
            class="flex items-center gap-1.5 hover:text-ctp-text transition-colors">
            <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
              <path d="M10 2L4 8l6 6" />
            </svg>
            Exit Focus
          </button>
          <div class="flex items-center gap-4 text-ctp-surface2">
            <span>{activeNote?.title || ''}</span>
            <span class="text-ctp-surface1">&middot;</span>
            <span>{editorContent.split(/\s+/).filter(Boolean).length} words</span>
            {#if dirty}
              <span class="w-1.5 h-1.5 rounded-full bg-ctp-peach animate-pulse"></span>
            {/if}
          </div>
          <span class="text-ctp-surface2 text-[10px]">Esc to exit</span>
        </div>
        <div class="flex-1 flex justify-center overflow-hidden">
          <div class="w-full max-w-[780px]">
            <Editor content={editorContent} {dirty}
              on:change={handleContentChange} on:save={handleSave}
              on:cursor={(e) => { cursorLine = e.detail.line; cursorCol = e.detail.col }} />
          </div>
        </div>
      </div>
    {:else}
      <!-- Normal Layout -->
      <div class="flex flex-1 min-h-0">
        <!-- Sidebar -->
        {#if showSidebar}
          <div class="flex-shrink-0 border-r border-ctp-surface0" style="width: {sidebarWidth}px">
            <Sidebar {tree} {activeNotePath} outlineItems={outlineData}
              on:select={handleSelectNote} on:create={handleCreateNote} on:delete={handleDeleteNote}
              on:jumpToLine={(e) => { /* TODO: scroll editor */ }} />
          </div>
          <div class="resize-handle w-1 cursor-col-resize hover:bg-ctp-blue/20 active:bg-ctp-blue/40 transition-colors"
            role="separator" aria-orientation="vertical"
            on:mousedown={(e) => startResize('sidebar', e)}></div>
        {/if}

        <!-- Center: tabs + editor/preview -->
        <div class="flex-1 flex flex-col min-w-0">
          {#if tabs.length > 0}
            <TabBar {tabs} activeIndex={activeTabIndex}
              on:select={(e) => switchToTab(e.detail)}
              on:close={(e) => closeTab(e.detail)} />
          {/if}

          <div class="flex-1 flex min-h-0">
            {#if !activeNote}
              <div class="flex-1 flex items-center justify-center">
                <div class="text-center space-y-5" style="animation: fadeSlideUp 300ms ease-out">
                  <svg width="48" height="48" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-surface2)" stroke-width="0.8" stroke-linecap="round" class="mx-auto opacity-40">
                    <path d="M4 2h8v12H4V2z" /><path d="M6 5h4m-4 2.5h3m-3 2.5h2" />
                  </svg>
                  <div class="space-y-1">
                    <p class="text-ctp-overlay0 text-[15px] font-light">Select a note to begin</p>
                    <p class="text-ctp-surface2 text-[12px]">or use a shortcut below</p>
                  </div>
                  <div class="flex items-center justify-center gap-6 text-[11px] text-ctp-overlay0">
                    <button on:click={() => { paletteMode = 'files'; openOverlay('commandPalette') }}
                      class="flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-ctp-surface0/50 hover:bg-ctp-surface0 transition-colors">
                      <kbd class="text-[10px] bg-ctp-surface0 px-1 py-0.5 rounded">Ctrl+P</kbd> Search
                    </button>
                    <button on:click={handleCreateNote}
                      class="flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-ctp-surface0/50 hover:bg-ctp-surface0 transition-colors">
                      <kbd class="text-[10px] bg-ctp-surface0 px-1 py-0.5 rounded">Ctrl+N</kbd> Create
                    </button>
                    <button on:click={openQuickSwitcher}
                      class="flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-ctp-surface0/50 hover:bg-ctp-surface0 transition-colors">
                      <kbd class="text-[10px] bg-ctp-surface0 px-1 py-0.5 rounded">Ctrl+J</kbd> Switch
                    </button>
                  </div>
                </div>
              </div>
            {:else if mode === 'edit'}
              <Editor content={editorContent} {dirty}
                on:change={handleContentChange} on:save={handleSave}
                on:cursor={(e) => { cursorLine = e.detail.line; cursorCol = e.detail.col }} />
            {:else if mode === 'preview'}
              <Preview content={editorContent} on:wikilink={handleWikilinkClick} />
            {:else}
              <div class="flex-1 flex">
                <div class="w-1/2 border-r border-ctp-surface0">
                  <Editor content={editorContent} {dirty}
                    on:change={handleContentChange} on:save={handleSave}
                    on:cursor={(e) => { cursorLine = e.detail.line; cursorCol = e.detail.col }} />
                </div>
                <div class="w-1/2">
                  <Preview content={editorContent} on:wikilink={handleWikilinkClick} />
                </div>
              </div>
            {/if}
          </div>
        </div>

        <!-- Backlinks Panel -->
        {#if showBacklinks && activeNote}
          <div class="resize-handle w-1 cursor-col-resize hover:bg-ctp-blue/20 active:bg-ctp-blue/40 transition-colors"
            role="separator" aria-orientation="vertical"
            on:mousedown={(e) => startResize('backlinks', e)}></div>
          <div class="flex-shrink-0 border-l border-ctp-surface0" style="width: {backlinksWidth}px">
            <BacklinksPanel incoming={backlinksData} outgoing={activeNote?.links || []}
              on:openNote={(e) => handleSelectNote(new CustomEvent('s', { detail: e.detail }))} />
          </div>
        {/if}
      </div>

      <StatusBar notePath={activeNotePath} {dirty} {mode}
        wordCount={activeNote?.wordCount || 0}
        charCount={editorContent.length}
        {cursorLine} {cursorCol}
        {aiProvider} themeName={currentTheme} {gitBranch}
        on:toggleMode={cycleMode} />
    {/if}
  </div>

  <!-- Overlays -->
  {#if overlays.commandPalette}
    <CommandPalette {notes} initialMode={paletteMode}
      on:select={(e) => { closeAllOverlays(); handleSelectNote(e) }}
      on:command={(e) => { closeAllOverlays(); dispatchCommand(e.detail) }}
      on:close={() => closeOverlay('commandPalette')} />
  {/if}
  {#if overlays.settings}
    <Settings settings={settingsData} on:update={async (e) => { await api().UpdateSetting(e.detail.key, e.detail.value); settingsData = await api().GetAllSettings() }}
      on:close={() => closeOverlay('settings')} />
  {/if}
  {#if overlays.graph}
    <GraphView data={graphData} on:select={(e) => { closeOverlay('graph'); handleSelectNote(new CustomEvent('s', { detail: e.detail })) }}
      on:close={() => closeOverlay('graph')} />
  {/if}
  {#if overlays.tags}
    <TagBrowser tags={tagsData} {notesForTag}
      on:selectTag={async (e) => { notesForTag = await api().GetNotesForTag(e.detail) || [] }}
      on:openNote={(e) => { closeOverlay('tags'); handleSelectNote(new CustomEvent('s', { detail: e.detail })) }}
      on:close={() => closeOverlay('tags')} />
  {/if}
  {#if overlays.bookmarks}
    <Bookmarks starred={bookmarkData.starred || []} recent={bookmarkData.recent || []}
      on:openNote={(e) => { closeOverlay('bookmarks'); handleSelectNote(new CustomEvent('s', { detail: e.detail })) }}
      on:unstar={async (e) => { await api().ToggleBookmark(e.detail); bookmarkData = await api().GetBookmarks() }}
      on:close={() => closeOverlay('bookmarks')} />
  {/if}
  {#if overlays.help}
    <Help on:close={() => closeOverlay('help')} />
  {/if}
  {#if overlays.outline}
    <Outline items={outlineData} on:jump={(e) => { closeOverlay('outline') }}
      on:close={() => closeOverlay('outline')} />
  {/if}
  {#if overlays.stats}
    <VaultStats stats={statsData} on:close={() => closeOverlay('stats')} />
  {/if}
  {#if overlays.templates}
    <Templates templates={templatesData}
      on:create={async (e) => { const p = await api().CreateFromTemplate(e.detail.idx, e.detail.name); closeOverlay('templates'); await refreshTree(); handleSelectNote(new CustomEvent('s', { detail: p })) }}
      on:close={() => closeOverlay('templates')} />
  {/if}
  {#if overlays.trash}
    <Trash items={trashData}
      on:restore={async (e) => { await api().RestoreFromTrash(e.detail); trashData = await api().GetTrashItems() || []; await refreshTree() }}
      on:purge={async (e) => { await api().PurgeFromTrash(e.detail); trashData = await api().GetTrashItems() || [] }}
      on:close={() => closeOverlay('trash')} />
  {/if}
  {#if overlays.git}
    <GitPanel statusLines={gitStatus} logLines={gitLog} diffText={gitDiff} message={gitMessage}
      on:refresh={refreshGit}
      on:commit={async (e) => { try { gitMessage = await api().GitCommit(e.detail); await refreshGit() } catch (err) { gitMessage = 'Commit failed: ' + err } }}
      on:push={async () => { try { gitMessage = await api().GitPush() } catch (err) { gitMessage = 'Push failed: ' + err } }}
      on:pull={async () => { try { gitMessage = await api().GitPull(); await refreshGit() } catch (err) { gitMessage = 'Pull failed: ' + err } }}
      on:close={() => closeOverlay('git')} />
  {/if}
  {#if overlays.bots}
    <Bots bind:this={botsRef} bots={botsList}
      on:run={async (e) => { try { const r = await api().RunBot(e.detail.kind, activeNotePath || '', e.detail.question || ''); botsRef.setResult(r) } catch (err) { botsRef.setError(String(err)) } }}
      on:close={() => closeOverlay('bots')} />
  {/if}
  {#if overlays.calendar}
    <Calendar data={calendarData}
      on:navigate={async (e) => { calendarData = await api().GetCalendarData(e.detail.year, e.detail.month) }}
      on:toggleTask={async (e) => { await api().ToggleTask(e.detail.notePath, e.detail.lineNum); const now = new Date(); calendarData = await api().GetCalendarData(now.getFullYear(), now.getMonth() + 1) }}
      on:openNote={(e) => { closeOverlay('calendar'); handleSelectNote(new CustomEvent('s', { detail: e.detail })) }}
      on:close={() => closeOverlay('calendar')} />
  {/if}
  {#if overlays.export}
    <ExportPanel notePath={activeNotePath} message={exportMessage}
      on:export={async (e) => { try {
        if (e.detail === 'html') exportMessage = await api().ExportHTML(activeNotePath)
        else if (e.detail === 'text') exportMessage = await api().ExportText(activeNotePath)
        else if (e.detail === 'pdf') exportMessage = await api().ExportPDF(activeNotePath)
        else if (e.detail === 'all') exportMessage = await api().ExportAll()
      } catch (err) { exportMessage = 'Error: ' + err } }}
      on:close={() => closeOverlay('export')} />
  {/if}
  {#if overlays.findReplace}
    <FindReplace content={editorContent}
      on:replace={(e) => { editorContent = e.detail; dirty = true }}
      on:close={() => closeOverlay('findReplace')} />
  {/if}
  {#if overlays.contentSearch}
    <ContentSearch results={searchResults} searching={searchBusy}
      on:search={async (e) => { searchBusy = true; try { searchResults = await api().Search(e.detail) || [] } catch { searchResults = [] } searchBusy = false }}
      on:select={(e) => { closeOverlay('contentSearch'); handleSelectNote(new CustomEvent('s', { detail: e.detail.relPath })) }}
      on:close={() => closeOverlay('contentSearch')} />
  {/if}
  {#if overlays.quickSwitcher}
    <QuickSwitcher {notes} starred={bookmarkData?.starred || []} recent={bookmarkData?.recent || []}
      on:select={(e) => { closeOverlay('quickSwitcher'); handleSelectNote(new CustomEvent('s', { detail: e.detail })) }}
      on:close={() => closeOverlay('quickSwitcher')} />
  {/if}
  {#if overlays.canvas}
    <Canvas on:close={() => closeOverlay('canvas')} />
  {/if}
  {#if overlays.kanban}
    <Kanban on:close={() => closeOverlay('kanban')}
      on:openNote={(e) => { closeOverlay('kanban'); handleSelectNote(new CustomEvent('s', { detail: e.detail })) }} />
  {/if}
  {#if overlays.taskManager}
    <TaskManager tasks={allTasks}
      on:openNote={(e) => { closeOverlay('taskManager'); handleSelectNote(new CustomEvent('s', { detail: e.detail })) }}
      on:close={() => closeOverlay('taskManager')} />
  {/if}
  {#if overlays.pomodoro}
    <Pomodoro on:close={() => closeOverlay('pomodoro')} />
  {/if}
  {#if overlays.habitTracker}
    <HabitTracker on:close={() => closeOverlay('habitTracker')} />
  {/if}
  {#if overlays.dailyPlanner}
    <DailyPlanner on:close={() => closeOverlay('dailyPlanner')} />
  {/if}
  {#if overlays.dailyBriefing}
    <DailyBriefing data={dailyBriefingData}
      on:openNote={(e) => { closeOverlay('dailyBriefing'); handleSelectNote(new CustomEvent('s', { detail: e.detail })) }}
      on:close={() => closeOverlay('dailyBriefing')} />
  {/if}
  {#if overlays.journalPrompts}
    <JournalPrompts
      on:create={async (e) => { const p = await api().CreateNote(e.detail.name, e.detail.content); closeOverlay('journalPrompts'); await refreshTree(); handleSelectNote(new CustomEvent('s', { detail: p })) }}
      on:close={() => closeOverlay('journalPrompts')} />
  {/if}
  {#if overlays.writingCoach}
    <WritingCoach content={editorContent} notePath={activeNotePath}
      on:close={() => closeOverlay('writingCoach')} />
  {/if}
  {#if overlays.flashcards}
    <Flashcards cards={flashcardsData} notePath={activeNotePath}
      on:close={() => closeOverlay('flashcards')} />
  {/if}
  {#if overlays.quiz}
    <Quiz notePath={activeNotePath}
      on:close={() => closeOverlay('quiz')} />
  {/if}
  {#if overlays.mindMap}
    <MindMap notePath={activeNotePath}
      on:select={(e) => { closeOverlay('mindMap'); handleSelectNote(new CustomEvent('s', { detail: e.detail })) }}
      on:close={() => closeOverlay('mindMap')} />
  {/if}
  {#if overlays.timeline}
    <Timeline entries={timelineData}
      on:select={(e) => { closeOverlay('timeline'); handleSelectNote(new CustomEvent('s', { detail: e.detail })) }}
      on:close={() => closeOverlay('timeline')} />
  {/if}
  {#if overlays.aiChat}
    <AiChat noteTitle={activeNote?.title || ''} noteContent={editorContent}
      on:close={() => closeOverlay('aiChat')} />
  {/if}
  {#if overlays.snippets}
    <Snippets on:close={() => closeOverlay('snippets')} />
  {/if}
  {#if overlays.tableEditor}
    <TableEditor
      on:insert={(e) => { closeOverlay('tableEditor') }}
      on:close={() => closeOverlay('tableEditor')} />
  {/if}
  {#if overlays.pluginManager}
    <PluginManager plugins={pluginsData}
      on:close={() => closeOverlay('pluginManager')} />
  {/if}
  {#if overlays.dataview}
    <Dataview
      on:openNote={(e) => { closeOverlay('dataview'); handleSelectNote(new CustomEvent('s', { detail: e.detail })) }}
      on:close={() => closeOverlay('dataview')} />
  {/if}
  {#if overlays.noteHistory}
    <NoteHistory notePath={activeNotePath} entries={noteHistoryData}
      on:close={() => closeOverlay('noteHistory')} />
  {/if}
  {#if overlays.workspaceManager}
    <WorkspaceManager on:close={() => closeOverlay('workspaceManager')} />
  {/if}
  {#if overlays.backupRestore}
    <BackupRestore on:close={() => closeOverlay('backupRestore')} />
  {/if}
  {#if overlays.autoLink}
    <AutoLink suggestions={autoLinkData} notePath={activeNotePath}
      on:close={() => closeOverlay('autoLink')} />
  {/if}
  {#if overlays.blogPublisher}
    <BlogPublisher notePath={activeNotePath} noteTitle={activeNote?.title || ''}
      on:close={() => closeOverlay('blogPublisher')} />
  {/if}
  {#if overlays.encryption}
    <EncryptionPanel notePath={activeNotePath}
      on:close={() => closeOverlay('encryption')} />
  {/if}
  {#if overlays.recurringTasks}
    <RecurringTasks tasks={recurringTasksData}
      on:openNote={(e) => { closeOverlay('recurringTasks'); handleSelectNote(new CustomEvent('s', { detail: e.detail })) }}
      on:close={() => closeOverlay('recurringTasks')} />
  {/if}
  {#if overlays.smartConnections}
    <SmartConnections notePath={activeNotePath} noteTitle={activeNote?.title || ''}
      on:openNote={(e) => { closeOverlay('smartConnections'); handleSelectNote(new CustomEvent('s', { detail: e.detail })) }}
      on:close={() => closeOverlay('smartConnections')} />
  {/if}
{/if}
