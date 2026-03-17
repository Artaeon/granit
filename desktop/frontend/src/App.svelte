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
  import FindReplace from './lib/FindReplace.svelte'
  import ContentSearch from './lib/ContentSearch.svelte'
  import QuickSwitcher from './lib/QuickSwitcher.svelte'
  import Bookmarks from './lib/Bookmarks.svelte'
  import Help from './lib/Help.svelte'
  import Outline from './lib/Outline.svelte'
  import InputDialog from './lib/InputDialog.svelte'
  import ContextMenu from './lib/ContextMenu.svelte'
  import type { NoteDetail, FolderNode, NoteInfo, Tab, BacklinkEntry } from './lib/types'

  // @ts-ignore - Wails runtime events
  import { EventsOn } from '../wailsjs/runtime/runtime'

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
  let editorRef: any

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
  let botsRef: any
  let calendarData: any = null
  let exportMessage = ''
  let searchResults: any[] = []
  let searchBusy = false

  // New feature overlay data
  let pluginsData: any[] = []
  let autoLinkData: any[] = []
  let noteHistoryData: any[] = []
  let dataviewResults: any[] = []

  // Input dialog state
  let inputDialog: { show: boolean; title: string; placeholder: string; value: string; action: string; callback: (v: string) => void } = { show: false, title: '', placeholder: '', value: '', action: '', callback: () => {} }
  function showInputDialog(title: string, placeholder: string, value: string, action: string, callback: (v: string) => void) {
    inputDialog = { show: true, title, placeholder, value, action, callback }
  }
  function closeInputDialog() { inputDialog = { ...inputDialog, show: false } }

  // Context menu state
  let contextMenu: { show: boolean; x: number; y: number; path: string } = { show: false, x: 0, y: 0, path: '' }
  function closeContextMenu() { contextMenu = { ...contextMenu, show: false } }

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

    // Listen for file system changes (auto-refresh)
    try {
      EventsOn('vault:changed', async () => {
        if (vaultOpen) await refreshTree()
      })
    } catch {}
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
        cursorPos: 0,
      }]
      activeTabIndex = tabs.length - 1

      api().AddRecent(relPath).catch(() => {})
      loadBacklinks(relPath)
      loadOutline(relPath)
    } catch (e) { console.error('Failed to load note:', e) }
  }

  function switchToTab(index: number) {
    if (index === activeTabIndex || index < 0 || index >= tabs.length) return

    // Save current tab state (content, dirty, scroll & cursor positions)
    if (activeTabIndex >= 0 && tabs[activeTabIndex]) {
      tabs[activeTabIndex] = {
        ...tabs[activeTabIndex],
        content: editorContent,
        dirty,
        scrollPos: editorRef?.getScrollPos?.() ?? 0,
        cursorPos: editorRef?.getCursorPos?.() ?? 0,
      }
    }

    activeTabIndex = index
    const tab = tabs[index]
    editorContent = tab.content
    dirty = tab.dirty
    activeNotePath = tab.relPath

    // Restore scroll & cursor after the editor updates with new content
    requestAnimationFrame(() => {
      if (editorRef) {
        editorRef.setCursorPos?.(tab.cursorPos ?? 0)
        editorRef.setScrollPos?.(tab.scrollPos ?? 0)
      }
    })

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

  function handleCreateNote() {
    showInputDialog('New Note', 'Enter note name...', '', 'Create', async (name) => {
      closeInputDialog()
      try {
        const fm = `---\ndate: ${new Date().toISOString().split('T')[0]}\n---\n\n# ${name}\n\n`
        const relPath = await api().CreateNote(name, fm)
        await refreshTree()
        handleSelectNote(new CustomEvent('select', { detail: relPath }))
      } catch (e) { console.error('Failed to create note:', e) }
    })
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
  function openTaskManager() { openOverlay('taskManager') }
  function openPomodoro() { openOverlay('pomodoro') }
  async function openHabitTracker() { openOverlay('habitTracker') }
  async function openDailyPlanner() { openOverlay('dailyPlanner') }
  function openDailyBriefing() { openOverlay('dailyBriefing') }
  function openJournalPrompts() { openOverlay('journalPrompts') }
  function openWritingCoach() { openOverlay('writingCoach') }
  function openFlashcards() { openOverlay('flashcards') }
  function openQuiz() { openOverlay('quiz') }
  function openMindMap() { openOverlay('mindMap') }
  function openTimeline() { openOverlay('timeline') }
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
  function openRecurringTasks() { openOverlay('recurringTasks') }
  function openSmartConnections() { openOverlay('smartConnections') }

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
          showInputDialog('Rename Note', 'Enter new name...', activeNote?.title || '', 'Rename', async (newName) => {
            closeInputDialog()
            const np = await api().RenameNote(activeNotePath, newName)
            await refreshTree()
            handleSelectNote(new CustomEvent('s', { detail: np }))
          })
        }
        break
      case 'daily_note': {
        const today = new Date().toISOString().split('T')[0]
        const existing = notes.find(n => n.relPath === today + '.md')
        if (existing) { handleSelectNote(new CustomEvent('s', { detail: existing.relPath })) }
        else { const p = await api().CreateNote(today, `---\ndate: ${today}\ntype: daily\ntags: [daily]\n---\n\n# ${today}\n\n## Tasks\n- [ ]\n\n## Notes\n\n`); await refreshTree(); handleSelectNote(new CustomEvent('s', { detail: p })) }
        break
      }
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
      case 'new_folder':
        showInputDialog('New Folder', 'Folder name...', '', 'Create', async (name) => {
          closeInputDialog()
          try { await api().CreateFolder(name); await refreshTree() } catch (e) { console.error(e) }
        })
        break
      case 'weekly_note': {
        const weekStart = new Date(); weekStart.setDate(weekStart.getDate() - weekStart.getDay() + 1)
        const weekStr = weekStart.toISOString().split('T')[0]
        const weekNote = notes.find(n => n.relPath.includes(weekStr))
        if (weekNote) { handleSelectNote(new CustomEvent('s', { detail: weekNote.relPath })) }
        else { const wp = await api().CreateNote(`weekly-${weekStr}`, `---\ndate: ${weekStr}\ntype: weekly\ntags: [weekly]\n---\n\n# Week of ${weekStr}\n\n## Goals\n- [ ]\n\n## Review\n\n`); await refreshTree(); handleSelectNote(new CustomEvent('s', { detail: wp })) }
        break
      }
      // Previously unhandled commands — now wired up
      case 'ai_compose': openAiChat(); break
      case 'ai_template': openTemplates(); break
      case 'clock_in': case 'clock_out': openPomodoro(); break
      case 'command_center': { paletteMode = 'commands'; openOverlay('commandPalette') }; break
      case 'extract_to_note':
        if (activeNotePath && editorContent) {
          showInputDialog('Extract to Note', 'New note name...', '', 'Extract', async (name) => {
            closeInputDialog()
            try {
              const p = await api().CreateNote(name, `---\ndate: ${new Date().toISOString().split('T')[0]}\n---\n\n`)
              await refreshTree()
              handleSelectNote(new CustomEvent('s', { detail: p }))
            } catch (e) { console.error(e) }
          })
        }
        break
      case 'frontmatter_edit': break // handled in-editor
      case 'import_obsidian': break // TODO: needs import wizard
      case 'knowledge_gaps': openSmartConnections(); break
      case 'layout_dashboard': showSidebar = true; showBacklinks = true; break
      case 'move_file':
        if (activeNotePath) {
          showInputDialog('Move to Folder', 'Folder path...', '', 'Move', async (dir) => {
            closeInputDialog()
            try { await api().MoveFile(activeNotePath, dir); await refreshTree() } catch (e) { console.error(e) }
          })
        }
        break
      case 'nav_back': case 'nav_forward': break // TODO: navigation history stack
      case 'next_daily': case 'prev_daily': {
        const d = new Date()
        d.setDate(d.getDate() + (action === 'next_daily' ? 1 : -1))
        const ds = d.toISOString().split('T')[0]
        const dn = notes.find(n => n.relPath.includes(ds))
        if (dn) handleSelectNote(new CustomEvent('s', { detail: dn.relPath }))
        break
      }
      case 'note_enhancer': openBots(); break
      case 'quick_capture':
        showInputDialog('Quick Capture', 'Quick thought...', '', 'Save', async (text) => {
          closeInputDialog()
          try {
            const ts = new Date().toISOString().replace(/[:.]/g, '-')
            await api().CreateNote(`inbox-${ts}`, `---\ndate: ${new Date().toISOString().split('T')[0]}\ntags: [inbox]\n---\n\n${text}\n`)
            await refreshTree()
          } catch (e) { console.error(e) }
        })
        break
      case 'research_agent': openAiChat(); break
      case 'show_tutorial': openHelp(); break
      case 'toggle_ghost_writer': break // TODO: needs CodeMirror extension
      case 'toggle_vim': break // TODO: needs @codemirror/vim
      case 'toggle_word_wrap': break // CodeMirror handles this
      case 'vault_analyzer': openStats(); break
      case 'vault_switch': handleOpenVault(); break
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
        'o': () => { vaultOpen ? openOutlineOverlay() : handleOpenVault() },
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

<svelte:window on:keydown={handleKeydown} on:mousemove={handleMouseMove} on:mouseup={stopResize}
  on:beforeunload={(e) => { if (dirty) { e.preventDefault(); e.returnValue = '' } }} />

{#if !vaultOpen}
  <div class="flex items-center justify-center h-full bg-ctp-base overflow-hidden">
    <div class="text-center space-y-8">
      <!-- Logo with gradient glow backdrop -->
      <div class="relative splash-logo">
        <div class="absolute inset-0 flex items-center justify-center pointer-events-none" aria-hidden="true">
          <div class="w-[320px] h-[120px] rounded-full opacity-30 blur-[60px]"
            style="background: radial-gradient(ellipse, var(--ctp-mauve), var(--ctp-blue), transparent 70%)"></div>
        </div>
        <pre class="relative text-ctp-mauve text-sm leading-tight font-mono select-none" style="filter: drop-shadow(0 0 24px color-mix(in srgb, var(--ctp-mauve) 25%, transparent))">
 ██████╗ ██████╗  █████╗ ███╗   ██╗██╗████████╗
██╔════╝ ██╔══██╗██╔══██╗████╗  ██║██║╚══██╔══╝
██║  ███╗██████╔╝███████║██╔██╗ ██║██║   ██║
██║   ██║██╔══██╗██╔══██║██║╚██╗██║██║   ██║
╚██████╔╝██║  ██║██║  ██║██║ ╚████║██║   ██║
 ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝╚═╝   ╚═╝</pre>
      </div>

      <!-- Tagline with typewriter effect -->
      <div class="splash-tagline space-y-1.5">
        <p class="text-ctp-subtext0 text-lg font-light tracking-wide splash-typewriter">Knowledge Manager</p>
        <p class="text-ctp-overlay1 text-xs tracking-wider splash-typewriter-sub">Notes &middot; Links &middot; Ideas</p>
        <p class="text-ctp-surface2 text-[11px] tracking-widest mt-2 splash-version">v0.1.0</p>
      </div>

      <!-- Open Vault button with gradient and glow -->
      <div class="splash-actions">
        <button on:click={handleOpenVault}
          class="group relative block mx-auto px-12 py-3.5 rounded-xl font-semibold text-sm tracking-wide
                 text-ctp-crust overflow-hidden
                 hover:-translate-y-0.5 active:translate-y-0 transition-all duration-200"
          style="background: linear-gradient(135deg, var(--ctp-blue), var(--ctp-mauve)); box-shadow: 0 0 24px color-mix(in srgb, var(--ctp-blue) 30%, transparent), 0 4px 16px rgba(0,0,0,0.2)">
          <span class="relative z-10 flex items-center gap-2">
            <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
              <path d="M2 5h5l1.5-2H13a1 1 0 0 1 1 1v7a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V5z" />
            </svg>
            Open Vault
          </span>
          <div class="absolute inset-0 opacity-0 group-hover:opacity-100 transition-opacity duration-300"
            style="background: linear-gradient(135deg, var(--ctp-mauve), var(--ctp-blue))"></div>
        </button>

        <div class="flex items-center justify-center gap-4 text-ctp-overlay1 text-[13px] mt-4">
          <span class="flex items-center gap-1.5">
            <kbd class="bg-ctp-surface0 px-1.5 py-0.5 rounded text-[12px]">Ctrl+O</kbd> browse
          </span>
        </div>
      </div>

      <!-- Recent vaults placeholder -->
      <div class="splash-recent pt-2">
        <p class="text-ctp-surface2 text-[12px] uppercase tracking-widest mb-3">Recent Vaults</p>
        <div class="flex flex-col items-center gap-1.5">
          <button class="flex items-center gap-2 px-4 py-2 rounded-lg text-[13px] text-ctp-overlay1 bg-ctp-surface0/30 hover:bg-ctp-surface0/60 transition-colors w-56 text-left">
            <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" class="text-ctp-mauve opacity-60 flex-shrink-0">
              <path d="M2 5h5l1.5-2H13a1 1 0 0 1 1 1v7a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V5z" />
            </svg>
            <span class="truncate text-ctp-subtext0">No recent vaults</span>
          </button>
        </div>
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
        <span class="text-sm font-semibold text-ctp-subtext0">{vaultPath ? vaultPath.split('/').pop() : 'Granit'}</span>
      </div>

      <!-- Breadcrumbs -->
      {#if activeNotePath}
        <div class="flex items-center gap-1 overflow-hidden">
          {#each activeNotePath.split('/') as segment, i}
            <svg width="8" height="8" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-surface2)" stroke-width="2" stroke-linecap="round"><path d="M6 4l4 4-4 4" /></svg>
            {#if i < activeNotePath.split('/').length - 1}
              <span class="text-[13px] text-ctp-overlay1 truncate max-w-[80px]">{segment}</span>
            {:else}
              <span class="text-[13px] text-ctp-text font-medium truncate max-w-[160px]">{segment.replace('.md', '')}</span>
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
          class:text-ctp-overlay1={!showBacklinks}
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
          class="text-[13px] bg-ctp-surface0/60 text-ctp-subtext0 border border-ctp-surface0/70 rounded-md px-2 py-1 outline-none cursor-pointer hover:border-ctp-overlay0 transition-colors appearance-none pr-5"
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
        <div class="flex items-center justify-between px-6 h-10 text-ctp-overlay1 text-[13px] border-b border-ctp-surface0/30 select-none">
          <button on:click={() => focusMode = false}
            class="flex items-center gap-1.5 hover:text-ctp-text transition-colors">
            <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
              <path d="M10 2L4 8l6 6" />
            </svg>
            Exit Focus
          </button>
          <div class="flex items-center gap-4 text-ctp-overlay1">
            <span>{activeNote?.title || ''}</span>
            <span class="text-ctp-surface1">&middot;</span>
            <span>{editorContent.split(/\s+/).filter(Boolean).length} words</span>
            {#if dirty}
              <span class="w-1.5 h-1.5 rounded-full bg-ctp-peach animate-pulse"></span>
            {/if}
          </div>
          <span class="text-ctp-overlay1 text-[12px]">Esc to exit</span>
        </div>
        <div class="flex-1 flex justify-center overflow-hidden">
          <div class="w-full max-w-[780px]">
            <Editor bind:this={editorRef} content={editorContent} {dirty}
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
              on:contextmenu={(e) => { contextMenu = { show: true, x: e.detail.x, y: e.detail.y, path: e.detail.path } }}
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
              {@const hour = new Date().getHours()}
              {@const greeting = hour < 12 ? 'Good morning' : hour < 17 ? 'Good afternoon' : 'Good evening'}
              {@const todayFormatted = new Date().toLocaleDateString('en-US', { weekday: 'long', year: 'numeric', month: 'long', day: 'numeric' })}
              <div class="flex-1 flex items-center justify-center">
                <div class="empty-state-container text-center max-w-lg mx-auto px-6">
                  <!-- Greeting -->
                  <div class="empty-state-greeting mb-2">
                    <p class="text-ctp-text text-2xl font-light tracking-wide">{greeting}</p>
                    <p class="text-ctp-overlay1 text-[13px] mt-1">{todayFormatted}</p>
                  </div>

                  <!-- Quick action cards — 2x2 grid -->
                  <div class="empty-state-cards grid grid-cols-2 gap-3 mt-8">
                    <button on:click={handleCreateNote}
                      class="group flex flex-col items-start gap-2 p-4 rounded-xl bg-ctp-surface0/30 border border-ctp-surface0/50
                             hover:bg-ctp-surface0/60 hover:border-ctp-overlay0/40 hover:-translate-y-0.5
                             transition-all duration-200 text-left">
                      <div class="flex items-center justify-between w-full">
                        <div class="w-9 h-9 rounded-lg flex items-center justify-center bg-ctp-green/10 text-ctp-green">
                          <svg width="18" height="18" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
                            <path d="M8 3v10M3 8h10" />
                          </svg>
                        </div>
                        <kbd class="text-[11px] text-ctp-surface2 bg-ctp-surface0/60 px-1.5 py-0.5 rounded opacity-0 group-hover:opacity-100 transition-opacity">Ctrl+N</kbd>
                      </div>
                      <div>
                        <p class="text-ctp-text text-[14px] font-medium">New Note</p>
                        <p class="text-ctp-overlay1 text-[12px] mt-0.5">Create a blank note</p>
                      </div>
                    </button>

                    <button on:click={() => dispatchCommand('daily_note')}
                      class="group flex flex-col items-start gap-2 p-4 rounded-xl bg-ctp-surface0/30 border border-ctp-surface0/50
                             hover:bg-ctp-surface0/60 hover:border-ctp-overlay0/40 hover:-translate-y-0.5
                             transition-all duration-200 text-left">
                      <div class="flex items-center justify-between w-full">
                        <div class="w-9 h-9 rounded-lg flex items-center justify-center bg-ctp-peach/10 text-ctp-peach">
                          <svg width="18" height="18" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
                            <rect x="2" y="2" width="12" height="12" rx="2" /><path d="M2 6h12M5 2v2M11 2v2" />
                          </svg>
                        </div>
                        <kbd class="text-[11px] text-ctp-surface2 bg-ctp-surface0/60 px-1.5 py-0.5 rounded opacity-0 group-hover:opacity-100 transition-opacity">Alt+D</kbd>
                      </div>
                      <div>
                        <p class="text-ctp-text text-[14px] font-medium">Daily Note</p>
                        <p class="text-ctp-overlay1 text-[12px] mt-0.5">Open today's journal</p>
                      </div>
                    </button>

                    <button on:click={() => openContentSearch()}
                      class="group flex flex-col items-start gap-2 p-4 rounded-xl bg-ctp-surface0/30 border border-ctp-surface0/50
                             hover:bg-ctp-surface0/60 hover:border-ctp-overlay0/40 hover:-translate-y-0.5
                             transition-all duration-200 text-left">
                      <div class="flex items-center justify-between w-full">
                        <div class="w-9 h-9 rounded-lg flex items-center justify-center bg-ctp-blue/10 text-ctp-blue">
                          <svg width="18" height="18" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
                            <circle cx="7" cy="7" r="4" /><path d="M10 10l3.5 3.5" />
                          </svg>
                        </div>
                        <kbd class="text-[11px] text-ctp-surface2 bg-ctp-surface0/60 px-1.5 py-0.5 rounded opacity-0 group-hover:opacity-100 transition-opacity">Ctrl+Shift+F</kbd>
                      </div>
                      <div>
                        <p class="text-ctp-text text-[14px] font-medium">Search Vault</p>
                        <p class="text-ctp-overlay1 text-[12px] mt-0.5">Find across all notes</p>
                      </div>
                    </button>

                    <button on:click={openQuickSwitcher}
                      class="group flex flex-col items-start gap-2 p-4 rounded-xl bg-ctp-surface0/30 border border-ctp-surface0/50
                             hover:bg-ctp-surface0/60 hover:border-ctp-overlay0/40 hover:-translate-y-0.5
                             transition-all duration-200 text-left">
                      <div class="flex items-center justify-between w-full">
                        <div class="w-9 h-9 rounded-lg flex items-center justify-center bg-ctp-mauve/10 text-ctp-mauve">
                          <svg width="18" height="18" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
                            <path d="M3 4h10M3 8h7M3 12h5" /><path d="M12 10l2 2-2 2" />
                          </svg>
                        </div>
                        <kbd class="text-[11px] text-ctp-surface2 bg-ctp-surface0/60 px-1.5 py-0.5 rounded opacity-0 group-hover:opacity-100 transition-opacity">Ctrl+J</kbd>
                      </div>
                      <div>
                        <p class="text-ctp-text text-[14px] font-medium">Quick Switch</p>
                        <p class="text-ctp-overlay1 text-[12px] mt-0.5">Jump to any note fast</p>
                      </div>
                    </button>
                  </div>
                </div>
              </div>
            {:else if mode === 'edit'}
              <Editor bind:this={editorRef} content={editorContent} {dirty}
                on:change={handleContentChange} on:save={handleSave}
                on:cursor={(e) => { cursorLine = e.detail.line; cursorCol = e.detail.col }} />
            {:else if mode === 'preview'}
              <Preview content={editorContent} on:wikilink={handleWikilinkClick} />
            {:else}
              <div class="flex-1 flex">
                <div class="w-1/2 border-r border-ctp-surface0">
                  <Editor bind:this={editorRef} content={editorContent} {dirty}
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
    {#await import('./lib/GraphView.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} data={graphData} on:select={(e) => { closeOverlay('graph'); handleSelectNote(new CustomEvent('s', { detail: e.detail })) }}
        on:close={() => closeOverlay('graph')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.tags}
    {#await import('./lib/TagBrowser.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} tags={tagsData} {notesForTag}
        on:selectTag={async (e) => { notesForTag = await api().GetNotesForTag(e.detail) || [] }}
        on:openNote={(e) => { closeOverlay('tags'); handleSelectNote(new CustomEvent('s', { detail: e.detail })) }}
        on:close={() => closeOverlay('tags')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
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
    {#await import('./lib/VaultStats.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} stats={statsData} on:close={() => closeOverlay('stats')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.templates}
    {#await import('./lib/Templates.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} templates={templatesData}
        on:create={async (e) => { const p = await api().CreateFromTemplate(e.detail.idx, e.detail.name); closeOverlay('templates'); await refreshTree(); handleSelectNote(new CustomEvent('s', { detail: p })) }}
        on:close={() => closeOverlay('templates')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.trash}
    {#await import('./lib/Trash.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} items={trashData}
        on:restore={async (e) => { await api().RestoreFromTrash(e.detail); trashData = await api().GetTrashItems() || []; await refreshTree() }}
        on:purge={async (e) => { await api().PurgeFromTrash(e.detail); trashData = await api().GetTrashItems() || [] }}
        on:close={() => closeOverlay('trash')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.git}
    {#await import('./lib/GitPanel.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} statusLines={gitStatus} logLines={gitLog} diffText={gitDiff} message={gitMessage}
        on:refresh={refreshGit}
        on:commit={async (e) => { try { gitMessage = await api().GitCommit(e.detail); await refreshGit() } catch (err) { gitMessage = 'Commit failed: ' + err } }}
        on:push={async () => { try { gitMessage = await api().GitPush() } catch (err) { gitMessage = 'Push failed: ' + err } }}
        on:pull={async () => { try { gitMessage = await api().GitPull(); await refreshGit() } catch (err) { gitMessage = 'Pull failed: ' + err } }}
        on:close={() => closeOverlay('git')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.bots}
    {#await import('./lib/Bots.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} bind:this={botsRef} bots={botsList}
        on:run={async (e) => { try { const r = await api().RunBot(e.detail.kind, activeNotePath || '', e.detail.question || ''); botsRef.setResult(r) } catch (err) { botsRef.setError(String(err)) } }}
        on:close={() => closeOverlay('bots')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.calendar}
    {#await import('./lib/Calendar.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} data={calendarData}
        on:navigate={async (e) => { calendarData = await api().GetCalendarData(e.detail.year, e.detail.month) }}
        on:toggleTask={async (e) => { await api().ToggleTask(e.detail.notePath, e.detail.lineNum); const now = new Date(); calendarData = await api().GetCalendarData(now.getFullYear(), now.getMonth() + 1) }}
        on:openNote={(e) => { closeOverlay('calendar'); handleSelectNote(new CustomEvent('s', { detail: e.detail })) }}
        on:close={() => closeOverlay('calendar')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.export}
    {#await import('./lib/ExportPanel.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} notePath={activeNotePath} message={exportMessage}
        on:export={async (e) => { try {
          if (e.detail === 'html') exportMessage = await api().ExportHTML(activeNotePath)
          else if (e.detail === 'text') exportMessage = await api().ExportText(activeNotePath)
          else if (e.detail === 'pdf') exportMessage = await api().ExportPDF(activeNotePath)
          else if (e.detail === 'all') exportMessage = await api().ExportAll()
        } catch (err) { exportMessage = 'Error: ' + err } }}
        on:close={() => closeOverlay('export')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
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
    {#await import('./lib/Canvas.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} on:close={() => closeOverlay('canvas')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.kanban}
    {#await import('./lib/Kanban.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} on:close={() => closeOverlay('kanban')}
        on:openNote={(e) => { closeOverlay('kanban'); handleSelectNote(new CustomEvent('s', { detail: e.detail })) }} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.taskManager}
    {#await import('./lib/TaskManager.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default}
        on:openNote={(e) => { closeOverlay('taskManager'); handleSelectNote(new CustomEvent('s', { detail: e.detail })) }}
        on:close={() => closeOverlay('taskManager')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.pomodoro}
    {#await import('./lib/Pomodoro.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} on:close={() => closeOverlay('pomodoro')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.habitTracker}
    {#await import('./lib/HabitTracker.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} on:close={() => closeOverlay('habitTracker')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.dailyPlanner}
    {#await import('./lib/DailyPlanner.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} on:close={() => closeOverlay('dailyPlanner')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.dailyBriefing}
    {#await import('./lib/DailyBriefing.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default}
        on:openNote={(e) => { closeOverlay('dailyBriefing'); handleSelectNote(new CustomEvent('s', { detail: e.detail })) }}
        on:close={() => closeOverlay('dailyBriefing')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.journalPrompts}
    {#await import('./lib/JournalPrompts.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default}
        on:create={async (e) => { const p = await api().CreateNote(e.detail.name, e.detail.content); closeOverlay('journalPrompts'); await refreshTree(); handleSelectNote(new CustomEvent('s', { detail: p })) }}
        on:close={() => closeOverlay('journalPrompts')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.writingCoach}
    {#await import('./lib/WritingCoach.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} content={editorContent} notePath={activeNotePath}
        on:close={() => closeOverlay('writingCoach')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.flashcards}
    {#await import('./lib/Flashcards.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} notePath={activeNotePath}
        on:close={() => closeOverlay('flashcards')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.quiz}
    {#await import('./lib/Quiz.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} notePath={activeNotePath}
        on:close={() => closeOverlay('quiz')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.mindMap}
    {#await import('./lib/MindMap.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} notePath={activeNotePath}
        on:select={(e) => { closeOverlay('mindMap'); handleSelectNote(new CustomEvent('s', { detail: e.detail })) }}
        on:close={() => closeOverlay('mindMap')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.timeline}
    {#await import('./lib/Timeline.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default}
        on:select={(e) => { closeOverlay('timeline'); handleSelectNote(new CustomEvent('s', { detail: e.detail })) }}
        on:close={() => closeOverlay('timeline')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.aiChat}
    {#await import('./lib/AiChat.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} noteTitle={activeNote?.title || ''} noteContent={editorContent}
        on:close={() => closeOverlay('aiChat')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.snippets}
    {#await import('./lib/Snippets.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} on:close={() => closeOverlay('snippets')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.tableEditor}
    {#await import('./lib/TableEditor.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default}
        on:insert={(e) => { closeOverlay('tableEditor') }}
        on:close={() => closeOverlay('tableEditor')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.pluginManager}
    {#await import('./lib/PluginManager.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} plugins={pluginsData}
        on:close={() => closeOverlay('pluginManager')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.dataview}
    {#await import('./lib/Dataview.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default}
        on:openNote={(e) => { closeOverlay('dataview'); handleSelectNote(new CustomEvent('s', { detail: e.detail })) }}
        on:close={() => closeOverlay('dataview')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.noteHistory}
    {#await import('./lib/NoteHistory.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} notePath={activeNotePath} entries={noteHistoryData}
        on:close={() => closeOverlay('noteHistory')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.workspaceManager}
    {#await import('./lib/WorkspaceManager.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} on:close={() => closeOverlay('workspaceManager')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.backupRestore}
    {#await import('./lib/BackupRestore.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} on:close={() => closeOverlay('backupRestore')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.autoLink}
    {#await import('./lib/AutoLink.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} suggestions={autoLinkData} notePath={activeNotePath}
        on:close={() => closeOverlay('autoLink')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.blogPublisher}
    {#await import('./lib/BlogPublisher.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} notePath={activeNotePath} noteTitle={activeNote?.title || ''}
        on:close={() => closeOverlay('blogPublisher')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.encryption}
    {#await import('./lib/EncryptionPanel.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} notePath={activeNotePath}
        on:close={() => closeOverlay('encryption')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.recurringTasks}
    {#await import('./lib/RecurringTasks.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default}
        on:openNote={(e) => { closeOverlay('recurringTasks'); handleSelectNote(new CustomEvent('s', { detail: e.detail })) }}
        on:close={() => closeOverlay('recurringTasks')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}
  {#if overlays.smartConnections}
    {#await import('./lib/SmartConnections.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} notePath={activeNotePath} noteTitle={activeNote?.title || ''}
        on:openNote={(e) => { closeOverlay('smartConnections'); handleSelectNote(new CustomEvent('s', { detail: e.detail })) }}
        on:close={() => closeOverlay('smartConnections')} />
    {:catch}
      <div class="overlay-loading" style="cursor:pointer" on:click={() => closeAllOverlays()}>
        <span style="color:var(--ctp-red);font-size:13px;position:absolute;bottom:45%;text-align:center">Failed to load. Click to close.</span>
      </div>
    {/await}
  {/if}

  <!-- Input Dialog (replaces browser prompt()) -->
  {#if inputDialog.show}
    <InputDialog title={inputDialog.title} placeholder={inputDialog.placeholder} value={inputDialog.value} action={inputDialog.action}
      on:confirm={(e) => inputDialog.callback(e.detail)}
      on:cancel={closeInputDialog} />
  {/if}

  <!-- Context Menu -->
  {#if contextMenu.show}
    <ContextMenu x={contextMenu.x} y={contextMenu.y}
      items={[
        { label: 'Open in New Tab', action: 'open', icon: 'open' },
        { label: 'Rename', action: 'rename', icon: 'rename', shortcut: 'F4' },
        { label: 'Move to Folder', action: 'move', icon: 'move' },
        { label: 'Copy Path', action: 'copy_path', icon: 'copy' },
        { label: 'Bookmark', action: 'bookmark', icon: 'star' },
        { label: '', action: '', separator: true },
        { label: 'Delete', action: 'delete', icon: 'delete', danger: true },
      ]}
      on:action={async (e) => {
        const path = contextMenu.path
        closeContextMenu()
        switch (e.detail) {
          case 'open': handleSelectNote(new CustomEvent('s', { detail: path })); break
          case 'rename':
            showInputDialog('Rename', 'New name...', path.replace('.md', '').split('/').pop() || '', 'Rename', async (name) => {
              closeInputDialog()
              const np = await api().RenameNote(path, name); await refreshTree()
              handleSelectNote(new CustomEvent('s', { detail: np }))
            }); break
          case 'move':
            showInputDialog('Move to Folder', 'Folder path...', '', 'Move', async (dir) => {
              closeInputDialog()
              await api().MoveFile(path, dir); await refreshTree()
            }); break
          case 'copy_path': navigator.clipboard.writeText(path); break
          case 'bookmark': await api().ToggleBookmark(path); break
          case 'delete': handleDeleteNote(new CustomEvent('d', { detail: path })); break
        }
      }}
      on:close={closeContextMenu} />
  {/if}
{/if}

<style>
  /* Splash screen phased reveal animations */
  .splash-logo {
    animation: splashFadeIn 600ms ease-out both;
  }
  .splash-tagline {
    animation: splashFadeIn 500ms ease-out 400ms both;
  }
  .splash-typewriter {
    display: inline-block;
    overflow: hidden;
    white-space: nowrap;
    border-right: 2px solid var(--ctp-mauve);
    animation: splashFadeIn 500ms ease-out 400ms both, splashTypewriter 1.2s steps(18) 500ms both, splashBlinkCaret 600ms step-end 500ms 3;
  }
  .splash-typewriter-sub {
    animation: splashFadeIn 400ms ease-out 1000ms both;
  }
  .splash-version {
    animation: splashFadeIn 400ms ease-out 1200ms both;
  }
  .splash-actions {
    animation: splashSlideUp 500ms ease-out 800ms both;
  }
  .splash-recent {
    animation: splashSlideUp 500ms ease-out 1100ms both;
  }

  @keyframes splashFadeIn {
    from { opacity: 0; }
    to { opacity: 1; }
  }
  @keyframes splashSlideUp {
    from { opacity: 0; transform: translateY(16px); }
    to { opacity: 1; transform: translateY(0); }
  }
  @keyframes splashTypewriter {
    from { max-width: 0; }
    to { max-width: 100%; }
  }
  @keyframes splashBlinkCaret {
    50% { border-color: transparent; }
  }

  /* Empty state (no active note) animations */
  .empty-state-greeting {
    animation: emptyFadeIn 400ms ease-out both;
  }
  .empty-state-cards {
    animation: emptySlideUp 400ms ease-out 150ms both;
  }

  @keyframes emptyFadeIn {
    from { opacity: 0; transform: translateY(-8px); }
    to { opacity: 1; transform: translateY(0); }
  }
  @keyframes emptySlideUp {
    from { opacity: 0; transform: translateY(12px); }
    to { opacity: 1; transform: translateY(0); }
  }
</style>
