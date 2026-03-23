<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import { vaultPath, notes, tree, currentView, currentPagePath, favorites, recentPages, navigateToPage, navigateToJournal } from '../stores'
  import type { NoteInfo, FolderNode, FlatTreeItem } from '../types'

  const dispatch = createEventDispatcher()

  export let currentTheme = 'catppuccin-mocha'
  export let themeNames: string[] = []

  let search = ''
  let recentExpanded = true
  let favoritesExpanded = true
  let toolsExpanded = false
  let filesExpanded = true
  let expandedFolders = new Set<string>()

  $: vaultName = $vaultPath ? $vaultPath.split('/').pop() : 'Granit'
  $: filteredNotes = search
    ? $notes.filter(n => n.title.toLowerCase().includes(search.toLowerCase()))
    : []
  $: recentNotes = $recentPages
    .map(p => $notes.find(n => n.relPath === p))
    .filter(Boolean) as NoteInfo[]
  $: favoriteNotes = $favorites
    .map(p => $notes.find(n => n.relPath === p))
    .filter(Boolean) as NoteInfo[]

  // File tree
  $: flatItems = $tree ? flattenTree($tree, 0) : []

  function flattenTree(node: FolderNode, depth: number): FlatTreeItem[] {
    const items: FlatTreeItem[] = []
    if (!node.children) return items
    const sorted = [...node.children].sort((a, b) => {
      if (a.isFolder && !b.isFolder) return -1
      if (!a.isFolder && b.isFolder) return 1
      return a.name.localeCompare(b.name)
    })
    for (const child of sorted) {
      items.push({
        name: child.name, path: child.path, isFolder: child.isFolder,
        depth, expanded: expandedFolders.has(child.path),
      })
      if (child.isFolder && expandedFolders.has(child.path) && child.children) {
        items.push(...flattenTree(child, depth + 1))
      }
    }
    return items
  }

  function toggleFolder(path: string) {
    if (expandedFolders.has(path)) expandedFolders.delete(path)
    else expandedFolders.add(path)
    expandedFolders = expandedFolders
  }
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="flex flex-col h-full bg-ctp-mantle select-none">
  <!-- Vault header -->
  <div class="flex items-center gap-2 px-4 h-12 border-b border-ctp-surface0/30 flex-shrink-0"
    style="--wails-draggable: drag">
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-mauve)" stroke-width="1.5" stroke-linecap="round" class="opacity-80 flex-shrink-0">
      <path d="M2 5h5l1.5-2H13a1 1 0 0 1 1 1v7a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V5z" />
    </svg>
    <span class="text-sm font-semibold text-ctp-subtext1 truncate">{vaultName}</span>
    <div class="flex-1"></div>
    <!-- New note -->
    <button on:click={() => dispatch('command', 'new_note')}
      class="w-6 h-6 flex items-center justify-center rounded text-ctp-overlay0 hover:text-ctp-text hover:bg-ctp-surface0/50 transition-colors"
      style="--wails-draggable: no-drag" data-tooltip="New note">
      <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
        <path d="M8 3v10M3 8h10" />
      </svg>
    </button>
  </div>

  <!-- Search -->
  <div class="px-3 py-2 flex-shrink-0">
    <button on:click={() => dispatch('command', 'search')}
      class="flex items-center gap-2 w-full px-2.5 py-1.5 bg-ctp-surface0/50 rounded-md border border-ctp-surface0/30 text-left hover:border-ctp-surface1 transition-colors">
      <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-overlay0)" stroke-width="1.5" stroke-linecap="round">
        <circle cx="7" cy="7" r="4" /><path d="M10 10l3.5 3.5" />
      </svg>
      <span class="text-[13px] text-ctp-overlay0 flex-1">Search...</span>
      <kbd class="text-[10px] text-ctp-overlay0 bg-ctp-crust px-1.5 py-0.5 rounded">Ctrl+K</kbd>
    </button>
  </div>

  {#if search}
    <!-- Search results -->
    <div class="flex-1 overflow-y-auto px-1">
      {#each filteredNotes.slice(0, 20) as note}
        <button on:click={() => { navigateToPage(note.relPath); search = '' }}
          class="w-full text-left px-3 py-1.5 rounded-md text-[13px] text-ctp-subtext1 hover:bg-ctp-surface0/50 hover:text-ctp-text truncate">
          {note.title}
        </button>
      {/each}
      {#if filteredNotes.length === 0}
        <p class="px-3 py-2 text-[12px] text-ctp-overlay0">No results</p>
      {/if}
    </div>
  {:else}
    <!-- Navigation -->
    <div class="flex-1 overflow-y-auto">
      <nav class="px-2 py-1 space-y-0.5">
        <!-- Journals -->
        <button on:click={() => navigateToJournal()}
          class="nav-item w-full"
          class:active={$currentView === 'journal'}>
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
            <path d="M2 5h12v8H2V5zm3-2v2m4-2v2M2 8h12" />
          </svg>
          <span>Journals</span>
        </button>

        <!-- All Pages -->
        <button on:click={() => dispatch('navigate', 'allPages')}
          class="nav-item w-full"
          class:active={$currentView === 'allPages'}>
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
            <path d="M4 2h8v12H4V2z" /><path d="M6 5h4m-4 2.5h3m-3 2.5h2" />
          </svg>
          <span>All Pages</span>
        </button>

        <!-- Graph View -->
        <button on:click={() => dispatch('navigate', 'graph')}
          class="nav-item w-full"
          class:active={$currentView === 'graph'}>
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
            <circle cx="5" cy="5" r="2" /><circle cx="11" cy="11" r="2" /><circle cx="12" cy="4" r="1.5" /><path d="M6.5 6l3 3M7 5h3.5" />
          </svg>
          <span>Graph View</span>
        </button>

        <!-- Dashboard -->
        <button on:click={() => dispatch('navigate', 'dashboard')}
          class="nav-item w-full"
          class:active={$currentView === 'dashboard'}>
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
            <path d="M2 8l6-6 6 6v5a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V8z" />
          </svg>
          <span>Dashboard</span>
        </button>
      </nav>

      <!-- Files section -->
      {#if flatItems.length > 0}
        <div class="mt-3">
          <button on:click={() => filesExpanded = !filesExpanded}
            class="section-header w-full">
            <svg width="8" height="8" viewBox="0 0 16 16" fill="currentColor" class="transition-transform" class:rotate-90={filesExpanded}>
              <path d="M6 4l4 4-4 4z" />
            </svg>
            <span>FILES</span>
          </button>
          {#if filesExpanded}
            <div class="px-1 space-y-0">
              {#each flatItems as item}
                {#if item.isFolder}
                  <button
                    class="w-full flex items-center gap-1.5 px-2 py-[5px] text-[13px] text-ctp-subtext0
                           hover:bg-ctp-surface0/50 hover:text-ctp-text rounded-md transition-all duration-75"
                    style="padding-left: {8 + item.depth * 14}px"
                    on:click={() => toggleFolder(item.path)}>
                    <svg width="9" height="9" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"
                      class="text-ctp-overlay1 transition-transform duration-150 flex-shrink-0"
                      class:rotate-90={item.expanded}>
                      <path d="M6 4l4 4-4 4" />
                    </svg>
                    <svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke-width="1.3" stroke-linecap="round" class="flex-shrink-0"
                      stroke={item.expanded ? 'var(--ctp-peach)' : 'var(--ctp-yellow)'}>
                      <path d="M2 5h5l1.5-2H13a1 1 0 0 1 1 1v7a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V5z" />
                    </svg>
                    <span class="truncate font-medium">{item.name}</span>
                  </button>
                {:else}
                  {@const isActive = item.path === $currentPagePath}
                  <button
                    class="w-full flex items-center gap-1.5 px-2 py-[5px] text-[13px] rounded-md transition-all duration-75"
                    class:text-ctp-blue={isActive}
                    class:font-semibold={isActive}
                    class:text-ctp-subtext1={!isActive}
                    class:hover:bg-ctp-surface0={!isActive}
                    class:hover:text-ctp-text={!isActive}
                    style="padding-left: {8 + item.depth * 14 + 12}px;
                           {isActive ? 'background: color-mix(in srgb, var(--ctp-blue) 12%, transparent);' : ''}"
                    on:click={() => navigateToPage(item.path)}>
                    <svg width="12" height="12" viewBox="0 0 16 16" fill="none"
                      stroke="{isActive ? 'var(--ctp-blue)' : 'var(--ctp-overlay1)'}"
                      stroke-width="1.3" stroke-linecap="round" class="flex-shrink-0">
                      <path d="M4 2h8v12H4V2zm2 3h4m-4 2.5h3" />
                    </svg>
                    <span class="truncate">{item.name.replace(/\.md$/, '')}</span>
                  </button>
                {/if}
              {/each}
            </div>
          {/if}
        </div>
      {/if}

      <!-- Favorites section -->
      {#if favoriteNotes.length > 0}
        <div class="mt-3">
          <button on:click={() => favoritesExpanded = !favoritesExpanded}
            class="section-header w-full">
            <svg width="8" height="8" viewBox="0 0 16 16" fill="currentColor" class="transition-transform" class:rotate-90={favoritesExpanded}>
              <path d="M6 4l4 4-4 4z" />
            </svg>
            <span>FAVORITES</span>
          </button>
          {#if favoritesExpanded}
            <div class="px-2 space-y-0.5">
              {#each favoriteNotes as note}
                <button on:click={() => navigateToPage(note.relPath)}
                  class="page-link w-full">
                  {note.title}
                </button>
              {/each}
            </div>
          {/if}
        </div>
      {/if}

      <!-- Recent section -->
      {#if recentNotes.length > 0}
        <div class="mt-3">
          <button on:click={() => recentExpanded = !recentExpanded}
            class="section-header w-full">
            <svg width="8" height="8" viewBox="0 0 16 16" fill="currentColor" class="transition-transform" class:rotate-90={recentExpanded}>
              <path d="M6 4l4 4-4 4z" />
            </svg>
            <span>RECENT</span>
          </button>
          {#if recentExpanded}
            <div class="px-2 space-y-0.5">
              {#each recentNotes.slice(0, 10) as note}
                <button on:click={() => navigateToPage(note.relPath)}
                  class="page-link w-full">
                  {note.title}
                </button>
              {/each}
            </div>
          {/if}
        </div>
      {/if}

      <!-- Tools section -->
      <div class="mt-3">
        <button on:click={() => toolsExpanded = !toolsExpanded}
          class="section-header w-full">
          <svg width="8" height="8" viewBox="0 0 16 16" fill="currentColor" class="transition-transform" class:rotate-90={toolsExpanded}>
            <path d="M6 4l4 4-4 4z" />
          </svg>
          <span>TOOLS</span>
        </button>
        {#if toolsExpanded}
          <nav class="px-2 space-y-0.5">
            <button on:click={() => dispatch('command', 'show_calendar')} class="nav-item w-full">
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
                <path d="M2 5h12v8H2V5zm3-2v2m4-2v2M2 8h12" />
              </svg>
              <span>Calendar</span>
            </button>
            <button on:click={() => dispatch('command', 'show_tags')} class="nav-item w-full">
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
                <path d="M1 8V1h7l6 6-7 7-6-6zm3-4a1 1 0 1 0 0 2 1 1 0 0 0 0-2" />
              </svg>
              <span>Tags</span>
            </button>
            <button on:click={() => dispatch('command', 'show_bookmarks')} class="nav-item w-full">
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
                <path d="M8 1l2 5h5l-4 3 1.5 5L8 11l-4.5 3L5 9 1 6h5z" />
              </svg>
              <span>Bookmarks</span>
            </button>
            <button on:click={() => dispatch('command', 'task_manager')} class="nav-item w-full">
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
                <path d="M3 4l2 2 4-4m-6 6l2 2 4-4" />
              </svg>
              <span>Tasks</span>
            </button>
            <button on:click={() => dispatch('command', 'ai_chat')} class="nav-item w-full">
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
                <path d="M5 5h6v5H5V5zM4 10h8v2H4v-2zM6 3h4m-2-2v2m-4 4h1m7 0h1" />
              </svg>
              <span>AI Chat</span>
            </button>
            <button on:click={() => dispatch('command', 'git_overlay')} class="nav-item w-full">
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
                <path d="M8 2v12M4 5a2 2 0 1 0 0-4 2 2 0 0 0 0 4zm0 10a2 2 0 1 0 0-4 2 2 0 0 0 0 4zm8-5a2 2 0 1 0 0-4 2 2 0 0 0 0 4z" />
              </svg>
              <span>Git</span>
            </button>
            <button on:click={() => dispatch('command', 'kanban')} class="nav-item w-full">
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
                <path d="M2 2h5v12H2V2zm7 0h5v12H9V2z" />
              </svg>
              <span>Kanban</span>
            </button>
            <button on:click={() => dispatch('command', 'flashcards')} class="nav-item w-full">
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
                <path d="M8 1L1 5l7 4 7-4-7-4zM1 8l7 4 7-4M1 11l7 4 7-4" />
              </svg>
              <span>Flashcards</span>
            </button>
            <button on:click={() => dispatch('command', 'pomodoro')} class="nav-item w-full">
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
                <circle cx="8" cy="8" r="6" /><path d="M8 4v4l3 2" />
              </svg>
              <span>Pomodoro</span>
            </button>
            <button on:click={() => dispatch('command', 'plan_my_day')} class="nav-item w-full">
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
                <path d="M3 4h10M3 8h10M3 12h10" />
              </svg>
              <span>Daily Planner</span>
            </button>
            <button on:click={() => dispatch('command', 'habit_tracker')} class="nav-item w-full">
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
                <path d="M3 4l2 2 4-4m-6 6l2 2 4-4" />
              </svg>
              <span>Habits</span>
            </button>
            <button on:click={() => dispatch('command', 'show_stats')} class="nav-item w-full">
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
                <path d="M3 13V8m4 5V5m4 8V2" />
              </svg>
              <span>Vault Stats</span>
            </button>
            <button on:click={() => dispatch('command', 'show_trash')} class="nav-item w-full">
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
                <path d="M4 4h8M5 4V3h6v1M3 4h10v9H3V4m4 2v5m2-5v5" />
              </svg>
              <span>Trash</span>
            </button>
            <button on:click={() => dispatch('command', 'show_projects')} class="nav-item w-full">
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
                <path d="M2 4h5l1-1h5a1 1 0 0 1 1 1v8a1 1 0 0 1-1 1H2V4z" />
              </svg>
              <span>Projects</span>
            </button>
            <button on:click={() => dispatch('command', 'export_note')} class="nav-item w-full">
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
                <path d="M8 2v8m-3-3l3 3 3-3M3 12h10" />
              </svg>
              <span>Export</span>
            </button>
            <button on:click={() => dispatch('command', 'open_commands')} class="nav-item w-full">
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
                <path d="M4 6l4 4 4-4" />
              </svg>
              <span>All Commands...</span>
            </button>
          </nav>
        {/if}
      </div>
    </div>
  {/if}

  <!-- Bottom: theme + actions -->
  <div class="flex-shrink-0 border-t border-ctp-surface0/30 px-3 py-2 space-y-2">
    <!-- Theme selector -->
    <div class="flex items-center gap-2">
      <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-overlay0)" stroke-width="1.5" stroke-linecap="round">
        <circle cx="8" cy="8" r="5" /><path d="M8 3v1m0 8v1M3 8h1m8 0h1" />
      </svg>
      <select bind:value={currentTheme} on:change={() => dispatch('themeChange', currentTheme)}
        class="flex-1 text-[12px] bg-ctp-surface0/50 text-ctp-subtext0 border border-ctp-surface0/30 rounded-md px-2 py-1 outline-none cursor-pointer hover:border-ctp-overlay0 transition-colors">
        {#each themeNames as name}
          <option value={name}>{name}</option>
        {/each}
      </select>
    </div>

    <div class="flex gap-1">
      <button on:click={() => dispatch('command', 'settings')}
        class="nav-item flex-1 justify-center">
        <svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
          <circle cx="8" cy="8" r="3" /><path d="M8 1v2m0 10v2M1 8h2m10 0h2M2.9 2.9l1.4 1.4m7.4 7.4l1.4 1.4M13.1 2.9l-1.4 1.4M4.3 11.7l-1.4 1.4" />
        </svg>
        <span>Settings</span>
      </button>
      <button on:click={() => dispatch('command', 'show_help')}
        class="nav-item flex-1 justify-center">
        <svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
          <circle cx="8" cy="8" r="7" /><path d="M6 6a2 2 0 0 1 4 0c0 1-2 1.5-2 3m0 2h.01" />
        </svg>
        <span>Help</span>
      </button>
    </div>
  </div>
</div>

<style>
  .nav-item {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.375rem 0.625rem;
    border-radius: 0.375rem;
    font-size: 13px;
    color: var(--ctp-subtext0);
    text-align: left;
    transition: all 75ms;
  }
  .nav-item:hover {
    background: color-mix(in srgb, var(--ctp-surface0) 50%, transparent);
    color: var(--ctp-text);
  }
  .nav-item.active {
    background: color-mix(in srgb, var(--ctp-blue) 12%, transparent);
    color: var(--ctp-blue);
  }
  .section-header {
    display: flex;
    align-items: center;
    gap: 0.375rem;
    padding: 0.25rem 0.75rem;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.08em;
    color: var(--ctp-overlay0);
    text-align: left;
  }
  .section-header:hover { color: var(--ctp-overlay1); }
  .page-link {
    display: block;
    padding: 0.25rem 0.625rem 0.25rem 1.25rem;
    border-radius: 0.25rem;
    font-size: 13px;
    color: var(--ctp-subtext0);
    text-align: left;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    transition: all 75ms;
  }
  .page-link:hover {
    background: color-mix(in srgb, var(--ctp-surface0) 40%, transparent);
    color: var(--ctp-text);
  }
  select {
    appearance: none;
    background-image: url('data:image/svg+xml,<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8" viewBox="0 0 16 16"><path fill="%236c7086" d="M4 6l4 4 4-4"/></svg>');
    background-repeat: no-repeat;
    background-position: right 6px center;
    padding-right: 20px;
  }
</style>
