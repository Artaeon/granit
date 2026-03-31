<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import LeftSidebar from './lib/layout/LeftSidebar.svelte'
  import MainContent from './lib/layout/MainContent.svelte'
  import RightSidebar from './lib/layout/RightSidebar.svelte'
  import SearchOverlay from './lib/pages/SearchOverlay.svelte'
  import CommandPalette from './lib/CommandPalette.svelte'
  import Settings from './lib/Settings.svelte'
  import InputDialog from './lib/InputDialog.svelte'
  import type { NoteDetail, NoteInfo } from './lib/types'
  import {
    vaultOpen, vaultPath, notes, tree, currentView, currentPagePath,
    showLeftSidebar, showRightSidebar, activeNote, navigateToPage,
    navigateToJournal, navigateBack, navigateForward, focusMode,
    overlays, openOverlay, closeOverlay, closeAllOverlays,
  } from './lib/stores'
  import * as api from './lib/api'

  // @ts-ignore - Wails runtime events
  import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime'

  // Theme
  let currentTheme = 'catppuccin-mocha'
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

  // Command palette
  let paletteMode: 'files' | 'commands' = 'files'

  // Overlay data
  let settingsData: any[] = []

  // Input dialog
  let inputDialog = { show: false, title: '', placeholder: '', value: '', action: '', callback: (_v: string) => {} }
  function showInputDialog(title: string, placeholder: string, value: string, action: string, callback: (v: string) => void) {
    inputDialog = { show: true, title, placeholder, value, action, callback }
  }
  function closeInputDialog() { inputDialog = { ...inputDialog, show: false } }

  // Dynamic overlay imports data
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
  let pluginsData: any[] = []
  let autoLinkData: any[] = []
  let noteHistoryData: any[] = []
  let dataviewResults: any[] = []
  let editorContent = ''
  let findReplaceMode: 'find' | 'replace' = 'find'

  let cancelVaultChanged: (() => void) | null = null

  onMount(async () => {
    try {
      try { currentTheme = await api.getTheme() || 'catppuccin-mocha'; applyTheme(currentTheme) } catch (e) { console.error('[Granit] getTheme failed:', e) }
      const isOpen = await api.isVaultOpen()
      vaultOpen.set(isOpen)
      if (isOpen) await loadVault()
    } catch (e) { console.error('[Granit] onMount init failed:', e) }

    try {
      cancelVaultChanged = EventsOn('vault:changed', async () => {
        if ($vaultOpen) await refreshTree()
      })
    } catch (e) { console.error('[Granit] EventsOn failed:', e) }
  })

  onDestroy(() => {
    if (cancelVaultChanged) cancelVaultChanged()
  })

  function applyTheme(name: string) {
    if (name === 'catppuccin-mocha') delete document.documentElement.dataset.theme
    else document.documentElement.dataset.theme = name
  }

  async function handleThemeChange() {
    applyTheme(currentTheme)
    try { await api.setTheme(currentTheme) } catch {}
  }

  async function loadVault() {
    try {
      const path = await api.getVaultPath()
      const t = await api.getFolderTree()
      const n = await api.getNotes() || []
      vaultPath.set(path)
      tree.set(t)
      notes.set(n)
      vaultOpen.set(true)
    } catch (e) { console.error('Failed to load vault:', e) }
  }

  async function refreshTree() {
    const [t, n] = await Promise.allSettled([api.getFolderTree(), api.getNotes()])
    if (t.status === 'fulfilled') tree.set(t.value)
    if (n.status === 'fulfilled') notes.set(n.value || [])
  }

  async function handleOpenVault() {
    try {
      const dir = await api.selectVaultDialog()
      if (dir) await loadVault()
    } catch (e) { console.error('Failed to open vault:', e) }
  }

  function handleNavigate(event: CustomEvent<string>) {
    const view = event.detail
    if (view === 'allPages') currentView.set('allPages')
    else if (view === 'graph') currentView.set('graph')
    else if (view === 'dashboard') currentView.set('dashboard')
    else if (view === 'journal') navigateToJournal()
  }

  // Overlay openers
  async function openSettings() { settingsData = await api.getAllSettings(); openOverlay('settings') }
  async function openGraph() { graphData = await api.getGraphData($currentPagePath || ''); openOverlay('graph') }
  async function openTags() { tagsData = await api.getAllTags() || []; openOverlay('tags') }
  async function openBookmarks() { bookmarkData = await api.getBookmarks(); openOverlay('bookmarks') }
  async function openStats() { statsData = await api.getVaultStats(); openOverlay('stats') }
  async function openTemplates() { templatesData = await api.getTemplates() || []; openOverlay('templates') }
  async function openTrash() { trashData = await api.getTrashItems() || []; openOverlay('trash') }
  async function openGit() { gitMessage = ''; await refreshGit(); openOverlay('git') }
  async function openBots() { botsList = await api.getBotList() || []; openOverlay('bots') }
  async function openCalendar() {
    const now = new Date()
    calendarData = await api.getCalendarData(now.getFullYear(), now.getMonth() + 1)
    openOverlay('calendar')
  }
  function openExport() { exportMessage = ''; openOverlay('export') }
  function openHelp() { openOverlay('help') }
  async function openQuickSwitcher() { try { bookmarkData = await api.getBookmarks() } catch {}; openOverlay('quickSwitcher') }
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
  async function openPluginManager() { try { pluginsData = await api.getPlugins() || [] } catch { pluginsData = [] }; openOverlay('pluginManager') }
  function openDataview() { dataviewResults = []; openOverlay('dataview') }
  async function openNoteHistory() { if ($currentPagePath) { try { noteHistoryData = await api.getNoteHistory($currentPagePath) || [] } catch { noteHistoryData = [] } }; openOverlay('noteHistory') }
  function openWorkspaceManager() { openOverlay('workspaceManager') }
  function openBackupRestore() { openOverlay('backupRestore') }
  async function openAutoLink() { if ($currentPagePath) { try { autoLinkData = await api.getAutoLinkSuggestions($currentPagePath) || [] } catch { autoLinkData = [] } }; openOverlay('autoLink') }
  function openBlogPublisher() { openOverlay('blogPublisher') }
  function openEncryption() { openOverlay('encryption') }
  function openRecurringTasks() { openOverlay('recurringTasks') }
  function openSmartConnections() { openOverlay('smartConnections') }
  function openProjects() { openOverlay('projects') }

  async function openOutline() {
    if ($currentPagePath) { try { outlineData = await api.getOutline($currentPagePath) || [] } catch { outlineData = [] } }
    openOverlay('outline')
  }
  function openFindReplace(mode: 'find' | 'replace') { findReplaceMode = mode; openOverlay('findReplace') }

  async function refreshGit() {
    const [s, l, d] = await Promise.allSettled([api.gitStatus(), api.gitLog(), api.gitDiff()])
    gitStatus = s.status === 'fulfilled' ? (s.value || []) : []
    gitLog = l.status === 'fulfilled' ? (l.value || []) : []
    gitDiff = d.status === 'fulfilled' ? (d.value || '') : ''
  }

  function handleCreateNote() {
    showInputDialog('New Note', 'Enter note name...', '', 'Create', async (name) => {
      closeInputDialog()
      try {
        const relPath = await api.createNote(name, '- \n')
        await refreshTree()
        navigateToPage(relPath)
      } catch (e) { console.error('Failed to create note:', e) }
    })
  }

  function handleSelectNote(event: CustomEvent<string>) {
    navigateToPage(event.detail)
  }

  // Command dispatch — handles all 106+ commands from commands.ts
  async function dispatchCommand(action: string) {
    closeAllOverlays()
    switch (action) {
      // File Operations
      case 'open_file': paletteMode = 'files'; openOverlay('commandPalette'); break
      case 'search': openOverlay('searchOverlay'); break
      case 'open_commands': paletteMode = 'commands'; openOverlay('commandPalette'); break
      case 'new_note': handleCreateNote(); break
      case 'save_note': /* auto-saved by block editor */ break
      case 'delete_note':
        if ($currentPagePath) {
          const pathToDelete = $currentPagePath
          try { await api.deleteNote(pathToDelete); await refreshTree(); navigateToJournal() }
          catch (e) { console.error('Delete failed:', e) }
        }
        break
      case 'rename_note':
        if ($currentPagePath && $activeNote) {
          const pathToRename = $currentPagePath
          showInputDialog('Rename Note', 'Enter new name...', $activeNote.title || '', 'Rename', async (newName) => {
            closeInputDialog()
            try {
              const np = await api.renameNote(pathToRename, newName)
              await refreshTree(); navigateToPage(np)
            } catch (e) { console.error('Rename failed:', e) }
          })
        }
        break
      case 'new_folder':
        showInputDialog('New Folder', 'Folder name...', '', 'Create', async (name) => {
          closeInputDialog()
          try { await api.createFolder(name); await refreshTree() } catch (e) { console.error(e) }
        })
        break
      case 'move_file':
        if ($currentPagePath) {
          const pathToMove = $currentPagePath
          showInputDialog('Move File', 'Destination folder...', '', 'Move', async (dest) => {
            closeInputDialog()
            try { const np = await api.moveFile(pathToMove, dest); await refreshTree(); navigateToPage(np) }
            catch (e) { console.error('Move failed:', e) }
          })
        }
        break
      case 'new_from_template': openTemplates(); break
      case 'quick_capture':
        showInputDialog('Quick Capture', 'Jot down a thought...', '', 'Capture', async (text) => {
          closeInputDialog()
          if (!text.trim()) return
          const today = new Date().toISOString().split('T')[0]
          try {
            const note = await api.ensureJournalNote(today)
            const content = (note?.content || '') + `\n- ${text.trim()}\n`
            await api.saveNote(note.relPath, content)
          } catch (e) { console.error('Capture failed:', e) }
        })
        break
      case 'refresh_vault': await api.refreshVault(); await refreshTree(); break

      // Navigation
      case 'daily_note': navigateToJournal(); break
      case 'prev_daily': case 'next_daily': {
        const today = new Date()
        const offset = action === 'prev_daily' ? -1 : 1
        today.setDate(today.getDate() + offset)
        const dateStr = today.toISOString().split('T')[0]
        try {
          const note = await api.ensureJournalNote(dateStr)
          if (note) navigateToPage(note.relPath)
        } catch {}
        break
      }
      case 'weekly_note': {
        const now = new Date()
        const jan1 = new Date(now.getFullYear(), 0, 1)
        const week = Math.ceil(((now.getTime() - jan1.getTime()) / 86400000 + jan1.getDay() + 1) / 7)
        const weekStr = `${now.getFullYear()}-W${String(week).padStart(2, '0')}`
        try { const p = await api.createNote(weekStr, `- \n`); await refreshTree(); navigateToPage(p) }
        catch { /* may already exist */ const match = $notes.find(n => n.title === weekStr); if (match) navigateToPage(match.relPath) }
        break
      }
      case 'quick_switch': openQuickSwitcher(); break
      case 'nav_back': navigateBack(); break
      case 'nav_forward': navigateForward(); break
      case 'vault_switch': handleOpenVault(); break

      // Editor
      case 'toggle_view': /* always in edit mode in outliner */ break
      case 'find_in_file': openFindReplace('find'); break
      case 'replace_in_file': openFindReplace('replace'); break
      case 'toggle_bookmark':
        if ($currentPagePath) { await api.toggleBookmark($currentPagePath) }
        break
      case 'focus_mode':
        focusMode.update(v => {
          const next = !v
          showLeftSidebar.set(!next)
          showRightSidebar.set(false)
          return next
        })
        break
      case 'toggle_vim': /* not applicable in CodeMirror block mode */ break
      case 'toggle_word_wrap': /* handled per-block in CodeMirror */ break
      case 'extract_to_note':
        showInputDialog('Extract to Note', 'New note name...', '', 'Extract', async (name) => {
          closeInputDialog()
          try { const p = await api.createNote(name, '- \n'); await refreshTree(); navigateToPage(p) }
          catch (e) { console.error(e) }
        })
        break
      case 'table_editor': case 'show_table_editor': openTableEditor(); break
      case 'frontmatter_edit': /* open note in page view for editing */ break
      case 'spell_check': /* not available in web runtime */ break

      // Views & Panels
      case 'show_graph': currentView.set('graph'); break
      case 'show_tags': openTags(); break
      case 'show_outline': openOutline(); break
      case 'show_bookmarks': openBookmarks(); break
      case 'show_stats': openStats(); break
      case 'show_trash': openTrash(); break
      case 'show_calendar': openCalendar(); break
      case 'show_canvas': openCanvas(); break
      case 'toggle_sidebar': showLeftSidebar.update(v => !v); break
      case 'split_pane': showRightSidebar.update(v => !v); break
      case 'timeline': case 'show_timeline': openTimeline(); break
      case 'mind_map': case 'show_mind_map': openMindMap(); break
      case 'dashboard': currentView.set('dashboard'); break
      case 'kanban': case 'show_kanban': openKanban(); break

      // Layout presets
      case 'layout_default': showLeftSidebar.set(true); showRightSidebar.set(true); focusMode.set(false); break
      case 'layout_writer': showLeftSidebar.set(true); showRightSidebar.set(false); focusMode.set(false); break
      case 'layout_minimal': showLeftSidebar.set(false); showRightSidebar.set(false); focusMode.set(true); break
      case 'layout_reading': showLeftSidebar.set(false); showRightSidebar.set(true); focusMode.set(false); break
      case 'layout_dashboard': showLeftSidebar.set(true); showRightSidebar.set(true); focusMode.set(false); currentView.set('dashboard'); break

      // Search
      case 'content_search': openOverlay('searchOverlay'); break
      case 'global_replace': openFindReplace('replace'); break
      case 'similar_notes': case 'smart_connections': case 'show_smart_connections': openSmartConnections(); break
      case 'auto_link': case 'show_auto_link': case 'link_assist': openAutoLink(); break

      // AI & Bots
      case 'show_bots': openBots(); break
      case 'ai_chat': case 'show_ai_chat': openAiChat(); break
      case 'ai_compose':
        showInputDialog('AI Compose', 'What should the note be about?', '', 'Generate', async (topic) => {
          closeInputDialog()
          try { const result = await api.chatWithAI(`Write a note about: ${topic}`); const p = await api.createNote(topic, result); await refreshTree(); navigateToPage(p) }
          catch (e) { console.error(e) }
        })
        break
      case 'ai_template':
        showInputDialog('AI Template', 'Template type + topic (e.g. "meeting notes for project X")', '', 'Generate', async (prompt) => {
          closeInputDialog()
          try { const result = await api.chatWithAI(`Generate a note template for: ${prompt}`); const name = prompt.split(' ').slice(0, 4).join(' '); const p = await api.createNote(name, result); await refreshTree(); navigateToPage(p) }
          catch (e) { console.error(e) }
        })
        break
      case 'writing_coach': case 'show_writing_coach': case 'writing_stats': openWritingCoach(); break
      case 'note_enhancer':
        if ($currentPagePath) { openBots() }
        break
      case 'research_agent':
        if ($currentPagePath) { openAiChat() }
        break
      case 'show_daily_planner': case 'plan_my_day': openDailyPlanner(); break
      case 'toggle_ghost_writer': /* not implemented in desktop */ break
      case 'knowledge_gaps': case 'vault_analyzer': openSmartConnections(); break

      // Git
      case 'git_overlay': openGit(); break
      case 'git_history': case 'note_history': case 'show_note_history': openNoteHistory(); break

      // Export & Import
      case 'export_note': openExport(); break
      case 'publish_site': case 'show_blog_publisher': openBlogPublisher(); break
      case 'vault_backup': case 'show_backup': openBackupRestore(); break
      case 'import_obsidian':
        showInputDialog('Import Obsidian', 'Path to .obsidian/ directory...', '', 'Import', async (_path) => {
          closeInputDialog()
          // Obsidian import handled by backend
        })
        break

      // Tools
      case 'settings': openSettings(); break
      case 'show_help': case 'show_tutorial': openHelp(); break
      case 'plugin_manager': case 'show_plugins': openPluginManager(); break
      case 'task_manager': case 'show_task_manager': openTaskManager(); break
      case 'pomodoro': case 'show_pomodoro': openPomodoro(); break
      case 'clock_in': case 'clock_out': openPomodoro(); break
      case 'habit_tracker': case 'show_habits': openHabitTracker(); break
      case 'flashcards': case 'show_flashcards': openFlashcards(); break
      case 'show_quiz': openQuiz(); break
      case 'command_center': case 'show_daily_briefing': openDailyBriefing(); break
      case 'show_journal_prompts': openJournalPrompts(); break
      case 'show_snippets': openSnippets(); break
      case 'show_dataview': openDataview(); break
      case 'show_workspaces': openWorkspaceManager(); break
      case 'show_encryption': openEncryption(); break
      case 'show_recurring_tasks': openRecurringTasks(); break
      case 'show_projects': openProjects(); break

      case 'quit': window.close(); break
      default: console.log('Unhandled command:', action); break
    }
  }

  function handleCommand(event: CustomEvent<string>) {
    dispatchCommand(event.detail)
  }

  // Keybindings
  function handleKeydown(event: KeyboardEvent) {
    if (event.ctrlKey || event.metaKey) {
      const key = event.key.toLowerCase()
      const handlers: Record<string, () => void> = {
        'k': () => dispatchCommand('content_search'),
        'p': () => dispatchCommand('open_file'),
        'x': () => dispatchCommand('open_commands'),
        'n': () => dispatchCommand('new_note'),
        ',': () => dispatchCommand('settings'),
        'g': () => dispatchCommand('show_graph'),
        'r': () => dispatchCommand('show_bots'),
        'l': () => dispatchCommand('show_calendar'),
        'j': () => dispatchCommand('quick_switch'),
        'f': () => dispatchCommand('find_in_file'),
        'h': () => dispatchCommand('replace_in_file'),
        'e': () => dispatchCommand('toggle_view'),
        'b': () => dispatchCommand('show_bookmarks'),
        't': () => dispatchCommand('show_tags'),
        'o': () => dispatchCommand('show_outline'),
        's': () => dispatchCommand('save_note'),
        'w': () => dispatchCommand('show_canvas'),
        'q': () => dispatchCommand('quit'),
      }
      if (handlers[key]) { event.preventDefault(); handlers[key]() }

      // Ctrl+Shift combos
      if (event.shiftKey && key === 'z') { event.preventDefault(); dispatchCommand('focus_mode') }
    }
    if (event.key === 'Escape') {
      if ($focusMode) { focusMode.set(false); showLeftSidebar.set(true) }
      else closeAllOverlays()
    }
    if (event.key === 'F4') dispatchCommand('rename_note')
    if (event.key === 'F5') dispatchCommand('show_help')
    if (event.altKey) {
      const key = event.key.toLowerCase()
      if (key === 'd') { event.preventDefault(); dispatchCommand('daily_note') }
      if (key === 'p') { event.preventDefault(); dispatchCommand('plan_my_day') }
      if (key === 'c') { event.preventDefault(); dispatchCommand('command_center') }
      if (key === 'w') { event.preventDefault(); dispatchCommand('weekly_note') }
      if (key === '[' || key === 'arrowleft') { event.preventDefault(); dispatchCommand('nav_back') }
      if (key === ']' || key === 'arrowright') { event.preventDefault(); dispatchCommand('nav_forward') }
    }
  }
</script>

<svelte:window on:keydown={handleKeydown} />

{#if !$vaultOpen}
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
        <p class="text-ctp-subtext0 text-lg font-light tracking-wide">Knowledge Manager</p>
        <p class="text-ctp-overlay1 text-xs tracking-wider">Notes &middot; Links &middot; Ideas</p>
      </div>
      <button on:click={handleOpenVault}
        class="block mx-auto px-10 py-3 bg-ctp-blue text-ctp-crust rounded-lg font-semibold
               hover:shadow-lg hover:shadow-ctp-blue/20 hover:-translate-y-0.5
               active:translate-y-0 transition-all duration-200 text-sm tracking-wide">
        Open Vault
      </button>
      <div class="flex items-center justify-center gap-4 text-ctp-overlay1 text-[13px]">
        <span class="flex items-center gap-1.5">
          <kbd class="bg-ctp-surface0 px-1.5 py-0.5 rounded text-[12px]">Ctrl+O</kbd> browse
        </span>
      </div>
    </div>
  </div>
{:else}
  <div class="flex h-full bg-ctp-base">
    <!-- Left Sidebar -->
    {#if $showLeftSidebar}
      <div class="flex-shrink-0 w-[240px] sidebar-border-r">
        <LeftSidebar
          {currentTheme}
          {themeNames}
          on:navigate={handleNavigate}
          on:command={handleCommand}
          on:themeChange={(e) => { currentTheme = e.detail; handleThemeChange() }}
        />
      </div>
    {/if}

    <!-- Main Content -->
    <div class="flex-1 min-w-0">
      <MainContent on:command={handleCommand} />
    </div>

    <!-- Right Sidebar -->
    {#if $showRightSidebar}
      <RightSidebar on:navigate={(e) => navigateToPage(e.detail)} />
    {/if}
  </div>

  <!-- Overlays -->
  {#if $overlays.searchOverlay}
    <SearchOverlay on:close={() => closeOverlay('searchOverlay')} />
  {/if}
  {#if $overlays.commandPalette}
    <CommandPalette notes={$notes} initialMode={paletteMode}
      on:select={(e) => { closeAllOverlays(); handleSelectNote(e) }}
      on:command={(e) => { closeAllOverlays(); dispatchCommand(e.detail) }}
      on:close={() => closeOverlay('commandPalette')} />
  {/if}
  {#if $overlays.settings}
    <Settings settings={settingsData} on:update={async (e) => { await api.updateSetting(e.detail.key, e.detail.value); settingsData = await api.getAllSettings() }}
      on:close={() => closeOverlay('settings')} />
  {/if}
  {#if $overlays.outline}
    {#await import('./lib/Outline.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} items={outlineData}
        on:close={() => closeOverlay('outline')} />
    {/await}
  {/if}
  {#if $overlays.findReplace}
    {#await import('./lib/FindReplace.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default}
        on:close={() => closeOverlay('findReplace')} />
    {/await}
  {/if}
  {#if $overlays.help}
    {#await import('./lib/Help.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} on:close={() => closeOverlay('help')} />
    {/await}
  {/if}
  {#if $overlays.graph}
    {#await import('./lib/GraphView.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} data={graphData} on:select={(e) => { closeOverlay('graph'); navigateToPage(e.detail) }}
        on:close={() => closeOverlay('graph')} />
    {/await}
  {/if}
  {#if $overlays.tags}
    {#await import('./lib/TagBrowser.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} tags={tagsData} {notesForTag}
        on:selectTag={async (e) => { notesForTag = await api.getNotesForTag(e.detail) || [] }}
        on:openNote={(e) => { closeOverlay('tags'); navigateToPage(e.detail) }}
        on:close={() => closeOverlay('tags')} />
    {/await}
  {/if}
  {#if $overlays.bookmarks}
    {#await import('./lib/Bookmarks.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} starred={bookmarkData.starred || []} recent={bookmarkData.recent || []}
        on:openNote={(e) => { closeOverlay('bookmarks'); navigateToPage(e.detail) }}
        on:unstar={async (e) => { await api.toggleBookmark(e.detail); bookmarkData = await api.getBookmarks() }}
        on:close={() => closeOverlay('bookmarks')} />
    {/await}
  {/if}
  {#if $overlays.stats}
    {#await import('./lib/VaultStats.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} stats={statsData} on:close={() => closeOverlay('stats')} />
    {/await}
  {/if}
  {#if $overlays.templates}
    {#await import('./lib/Templates.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} templates={templatesData}
        on:create={async (e) => { const p = await api.createFromTemplate(e.detail.idx, e.detail.name); closeOverlay('templates'); await refreshTree(); navigateToPage(p) }}
        on:close={() => closeOverlay('templates')} />
    {/await}
  {/if}
  {#if $overlays.trash}
    {#await import('./lib/Trash.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} items={trashData}
        on:restore={async (e) => { await api.restoreFromTrash(e.detail); trashData = await api.getTrashItems() || []; await refreshTree() }}
        on:purge={async (e) => { await api.purgeFromTrash(e.detail); trashData = await api.getTrashItems() || [] }}
        on:close={() => closeOverlay('trash')} />
    {/await}
  {/if}
  {#if $overlays.git}
    {#await import('./lib/GitPanel.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} statusLines={gitStatus} logLines={gitLog} diffText={gitDiff} message={gitMessage}
        on:refresh={refreshGit}
        on:commit={async (e) => { try { gitMessage = await api.gitCommit(e.detail); await refreshGit() } catch (err) { gitMessage = 'Commit failed: ' + err } }}
        on:push={async () => { try { gitMessage = await api.gitPush() } catch (err) { gitMessage = 'Push failed: ' + err } }}
        on:pull={async () => { try { gitMessage = await api.gitPull(); await refreshGit() } catch (err) { gitMessage = 'Pull failed: ' + err } }}
        on:close={() => closeOverlay('git')} />
    {/await}
  {/if}
  {#if $overlays.bots}
    {#await import('./lib/Bots.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} bind:this={botsRef} bots={botsList}
        on:run={async (e) => { try { const r = await api.runBot(e.detail.kind, $currentPagePath || '', e.detail.question || ''); botsRef.setResult(r) } catch (err) { botsRef.setError(String(err)) } }}
        on:close={() => closeOverlay('bots')} />
    {/await}
  {/if}
  {#if $overlays.calendar}
    {#await import('./lib/Calendar.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} data={calendarData}
        on:navigate={async (e) => { calendarData = await api.getCalendarData(e.detail.year, e.detail.month) }}
        on:openNote={(e) => { closeOverlay('calendar'); navigateToPage(e.detail) }}
        on:toggleTask={async (e) => { try { await api.toggleTask(e.detail.notePath, e.detail.lineNum); calendarData = await api.getCalendarData(new Date().getFullYear(), new Date().getMonth() + 1) } catch {} }}
        on:close={() => closeOverlay('calendar')} />
    {/await}
  {/if}
  {#if $overlays.export}
    {#await import('./lib/ExportPanel.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} notePath={$currentPagePath} message={exportMessage}
        on:export={async (e) => { try {
          if (e.detail === 'html') exportMessage = await api.exportHTML($currentPagePath)
          else if (e.detail === 'text') exportMessage = await api.exportText($currentPagePath)
          else if (e.detail === 'pdf') exportMessage = await api.exportPDF($currentPagePath)
          else if (e.detail === 'all') exportMessage = await api.exportAll()
        } catch (err) { exportMessage = 'Error: ' + err } }}
        on:close={() => closeOverlay('export')} />
    {/await}
  {/if}
  {#if $overlays.quickSwitcher}
    {#await import('./lib/QuickSwitcher.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} notes={$notes} starred={bookmarkData?.starred || []} recent={bookmarkData?.recent || []}
        on:select={(e) => { closeOverlay('quickSwitcher'); navigateToPage(e.detail) }}
        on:close={() => closeOverlay('quickSwitcher')} />
    {/await}
  {/if}
  {#if $overlays.contentSearch}
    {#await import('./lib/ContentSearch.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} results={searchResults} searching={searchBusy}
        on:search={async (e) => { searchBusy = true; try { searchResults = await api.search(e.detail) || [] } catch { searchResults = [] } searchBusy = false }}
        on:select={(e) => { closeOverlay('contentSearch'); navigateToPage(e.detail.relPath) }}
        on:close={() => closeOverlay('contentSearch')} />
    {/await}
  {/if}
  {#if $overlays.canvas}
    {#await import('./lib/Canvas.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} on:close={() => closeOverlay('canvas')} />
    {/await}
  {/if}
  {#if $overlays.kanban}
    {#await import('./lib/Kanban.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} on:close={() => closeOverlay('kanban')}
        on:openNote={(e) => { closeOverlay('kanban'); navigateToPage(e.detail) }} />
    {/await}
  {/if}
  {#if $overlays.taskManager}
    {#await import('./lib/TaskManager.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default}
        on:openNote={(e) => { closeOverlay('taskManager'); navigateToPage(e.detail) }}
        on:close={() => closeOverlay('taskManager')} />
    {/await}
  {/if}
  {#if $overlays.pomodoro}
    {#await import('./lib/Pomodoro.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} on:close={() => closeOverlay('pomodoro')} />
    {/await}
  {/if}
  {#if $overlays.habitTracker}
    {#await import('./lib/HabitTracker.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} on:close={() => closeOverlay('habitTracker')} />
    {/await}
  {/if}
  {#if $overlays.dailyPlanner}
    {#await import('./lib/DailyPlanner.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} on:close={() => closeOverlay('dailyPlanner')} />
    {/await}
  {/if}
  {#if $overlays.dailyBriefing}
    {#await import('./lib/DailyBriefing.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default}
        on:openNote={(e) => { closeOverlay('dailyBriefing'); navigateToPage(e.detail) }}
        on:close={() => closeOverlay('dailyBriefing')} />
    {/await}
  {/if}
  {#if $overlays.journalPrompts}
    {#await import('./lib/JournalPrompts.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default}
        on:create={async (e) => { const p = await api.createNote(e.detail.name, e.detail.content); closeOverlay('journalPrompts'); await refreshTree(); navigateToPage(p) }}
        on:close={() => closeOverlay('journalPrompts')} />
    {/await}
  {/if}
  {#if $overlays.writingCoach}
    {#await import('./lib/WritingCoach.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} content={editorContent} notePath={$currentPagePath}
        on:close={() => closeOverlay('writingCoach')} />
    {/await}
  {/if}
  {#if $overlays.flashcards}
    {#await import('./lib/Flashcards.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} notePath={$currentPagePath}
        on:close={() => closeOverlay('flashcards')} />
    {/await}
  {/if}
  {#if $overlays.quiz}
    {#await import('./lib/Quiz.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} notePath={$currentPagePath}
        on:close={() => closeOverlay('quiz')} />
    {/await}
  {/if}
  {#if $overlays.mindMap}
    {#await import('./lib/MindMap.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} notePath={$currentPagePath}
        on:select={(e) => { closeOverlay('mindMap'); navigateToPage(e.detail) }}
        on:close={() => closeOverlay('mindMap')} />
    {/await}
  {/if}
  {#if $overlays.timeline}
    {#await import('./lib/Timeline.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default}
        on:select={(e) => { closeOverlay('timeline'); navigateToPage(e.detail) }}
        on:close={() => closeOverlay('timeline')} />
    {/await}
  {/if}
  {#if $overlays.aiChat}
    {#await import('./lib/AiChat.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} noteTitle={$activeNote?.title || ''} noteContent={editorContent}
        on:close={() => closeOverlay('aiChat')} />
    {/await}
  {/if}
  {#if $overlays.snippets}
    {#await import('./lib/Snippets.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} on:close={() => closeOverlay('snippets')} />
    {/await}
  {/if}
  {#if $overlays.tableEditor}
    {#await import('./lib/TableEditor.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default}
        on:insert={(e) => { closeOverlay('tableEditor') }}
        on:close={() => closeOverlay('tableEditor')} />
    {/await}
  {/if}
  {#if $overlays.pluginManager}
    {#await import('./lib/PluginManager.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} plugins={pluginsData}
        on:close={() => closeOverlay('pluginManager')} />
    {/await}
  {/if}
  {#if $overlays.dataview}
    {#await import('./lib/Dataview.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default}
        on:openNote={(e) => { closeOverlay('dataview'); navigateToPage(e.detail) }}
        on:close={() => closeOverlay('dataview')} />
    {/await}
  {/if}
  {#if $overlays.noteHistory}
    {#await import('./lib/NoteHistory.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} notePath={$currentPagePath} entries={noteHistoryData}
        on:close={() => closeOverlay('noteHistory')} />
    {/await}
  {/if}
  {#if $overlays.workspaceManager}
    {#await import('./lib/WorkspaceManager.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} on:close={() => closeOverlay('workspaceManager')} />
    {/await}
  {/if}
  {#if $overlays.backupRestore}
    {#await import('./lib/BackupRestore.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} on:close={() => closeOverlay('backupRestore')} />
    {/await}
  {/if}
  {#if $overlays.autoLink}
    {#await import('./lib/AutoLink.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} suggestions={autoLinkData} notePath={$currentPagePath}
        on:close={() => closeOverlay('autoLink')} />
    {/await}
  {/if}
  {#if $overlays.blogPublisher}
    {#await import('./lib/BlogPublisher.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} notePath={$currentPagePath} noteTitle={$activeNote?.title || ''}
        on:close={() => closeOverlay('blogPublisher')} />
    {/await}
  {/if}
  {#if $overlays.encryption}
    {#await import('./lib/EncryptionPanel.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} notePath={$currentPagePath}
        on:close={() => closeOverlay('encryption')} />
    {/await}
  {/if}
  {#if $overlays.recurringTasks}
    {#await import('./lib/RecurringTasks.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default}
        on:openNote={(e) => { closeOverlay('recurringTasks'); navigateToPage(e.detail) }}
        on:close={() => closeOverlay('recurringTasks')} />
    {/await}
  {/if}
  {#if $overlays.smartConnections}
    {#await import('./lib/SmartConnections.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default} notePath={$currentPagePath} noteTitle={$activeNote?.title || ''}
        on:openNote={(e) => { closeOverlay('smartConnections'); navigateToPage(e.detail) }}
        on:close={() => closeOverlay('smartConnections')} />
    {/await}
  {/if}

  {#if $overlays.projects}
    {#await import('./lib/ProjectManager.svelte')}
      <div class="overlay-loading"></div>
    {:then mod}
      <svelte:component this={mod.default}
        on:openNote={(e) => { closeOverlay('projects'); navigateToPage(e.detail) }}
        on:close={() => closeOverlay('projects')} />
    {/await}
  {/if}

  <!-- Input Dialog -->
  {#if inputDialog.show}
    <InputDialog title={inputDialog.title} placeholder={inputDialog.placeholder} value={inputDialog.value} action={inputDialog.action}
      on:confirm={(e) => inputDialog.callback(e.detail)}
      on:cancel={closeInputDialog} />
  {/if}
{/if}
