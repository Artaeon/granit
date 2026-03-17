<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import type { FolderNode, FlatTreeItem } from './types'

  export let tree: FolderNode | null = null
  export let activeNotePath = ''
  export let outlineItems: any[] = []
  export let vaultName = 'Granit'

  const dispatch = createEventDispatcher()

  let searchQuery = ''
  let expandedFolders = new Set<string>()
  let outlineExpanded = true
  let viewMode: 'tree' | 'flat' = 'tree'

  $: flatItems = tree ? flattenTree(tree, 0) : []
  $: allFlatNotes = tree ? getAllNotes(tree) : []
  $: filteredItems = searchQuery
    ? (viewMode === 'flat' ? allFlatNotes : flatItems).filter(item =>
        !item.isFolder && item.name.toLowerCase().includes(searchQuery.toLowerCase())
      )
    : viewMode === 'flat'
      ? allFlatNotes
      : flatItems

  $: noteCount = (tree ? getAllNotes(tree) : []).length
  $: folderCount = flatItems.filter(i => i.isFolder).length
  $: folderNoteCounts = tree ? buildFolderCounts(tree) : new Map<string, number>()

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

  /** Get all notes in flat alphabetical order with folder path as secondary info */
  function getAllNotes(node: FolderNode, parentPath = ''): FlatTreeItem[] {
    const notes: FlatTreeItem[] = []
    if (!node.children) return notes
    for (const child of node.children) {
      if (child.isFolder && child.children) {
        notes.push(...getAllNotes(child, child.path))
      } else if (!child.isFolder) {
        notes.push({
          name: child.name,
          path: child.path,
          isFolder: false,
          depth: 0,
          expanded: false,
        })
      }
    }
    return notes.sort((a, b) => a.name.localeCompare(b.name))
  }

  /** Count the total notes inside each folder recursively */
  function buildFolderCounts(node: FolderNode): Map<string, number> {
    const counts = new Map<string, number>()
    function countNotes(n: FolderNode): number {
      if (!n.children) return 0
      let total = 0
      for (const child of n.children) {
        if (child.isFolder) {
          total += countNotes(child)
        } else {
          total += 1
        }
      }
      counts.set(n.path, total)
      return total
    }
    countNotes(node)
    return counts
  }

  /** Collect all folder paths from the tree */
  function collectFolderPaths(node: FolderNode): string[] {
    const paths: string[] = []
    if (!node.children) return paths
    for (const child of node.children) {
      if (child.isFolder) {
        paths.push(child.path)
        paths.push(...collectFolderPaths(child))
      }
    }
    return paths
  }

  function expandAll() {
    if (!tree) return
    const allPaths = collectFolderPaths(tree)
    expandedFolders = new Set(allPaths)
  }

  function collapseAll() {
    expandedFolders = new Set()
  }

  /** Returns HTML with matching portion highlighted */
  function highlightMatch(name: string, query: string): string {
    if (!query) return escapeHtml(name)
    const lower = name.toLowerCase()
    const idx = lower.indexOf(query.toLowerCase())
    if (idx === -1) return escapeHtml(name)
    const before = name.slice(0, idx)
    const match = name.slice(idx, idx + query.length)
    const after = name.slice(idx + query.length)
    return `${escapeHtml(before)}<span class="text-ctp-blue bg-ctp-blue/20 rounded-sm px-[1px]">${escapeHtml(match)}</span>${escapeHtml(after)}`
  }

  function escapeHtml(s: string): string {
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  }

  /** Extract the folder portion from a full note path */
  function getFolderPath(path: string): string {
    const idx = path.lastIndexOf('/')
    return idx > 0 ? path.substring(0, idx) : ''
  }

  function toggleFolder(path: string) {
    if (expandedFolders.has(path)) expandedFolders.delete(path)
    else expandedFolders.add(path)
    expandedFolders = expandedFolders
  }

  function selectNote(path: string) { dispatch('select', path) }

  function handleContextMenu(event: MouseEvent, path: string) {
    event.preventDefault()
    dispatch('contextmenu', { x: event.clientX, y: event.clientY, path })
  }

  function clearSearch() { searchQuery = '' }

  const outlineColors: Record<number, string> = {
    1: 'var(--ctp-mauve)', 2: 'var(--ctp-blue)', 3: 'var(--ctp-sapphire)',
    4: 'var(--ctp-teal)', 5: 'var(--ctp-green)', 6: 'var(--ctp-yellow)',
  }
</script>

<div class="flex flex-col h-full bg-ctp-mantle select-none">
  <!-- Header: vault name + actions -->
  <div class="flex items-center justify-between px-3 py-2.5 group/header">
    <div class="flex items-center gap-2 min-w-0">
      <!-- Vault icon -->
      <svg width="15" height="15" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-subtext0)" stroke-width="1.4" stroke-linecap="round" class="flex-shrink-0">
        <path d="M2 5h5l1.5-2H13a1 1 0 0 1 1 1v7a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V5z" />
      </svg>
      <span class="text-[14px] font-medium text-ctp-text truncate">{vaultName}</span>
    </div>
    <div class="flex items-center gap-0.5">
      <!-- Tree/Flat toggle -->
      <button on:click={() => viewMode = viewMode === 'tree' ? 'flat' : 'tree'}
        class="w-[26px] h-[26px] flex items-center justify-center rounded-[4px] text-ctp-overlay1
               hover:bg-ctp-surface0/60 hover:text-ctp-text transition-all duration-100
               opacity-0 group-hover/header:opacity-100"
        title="{viewMode === 'tree' ? 'Switch to flat list' : 'Switch to tree view'}">
        {#if viewMode === 'tree'}
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
            <path d="M3 4h10M3 8h10M3 12h10" />
          </svg>
        {:else}
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
            <path d="M4 3v10M4 6h4M4 10h6M8 6v2M10 10v2" />
          </svg>
        {/if}
      </button>
      <!-- New note -->
      <button on:click={() => dispatch('create')}
        class="w-[26px] h-[26px] flex items-center justify-center rounded-[4px] text-ctp-overlay1
               hover:bg-ctp-surface0/60 hover:text-ctp-text transition-all duration-100"
        title="New note (Ctrl+N)">
        <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <path d="M8 3v10M3 8h10" />
        </svg>
      </button>
    </div>
  </div>

  <!-- Search -->
  <div class="px-2 pb-2">
    <div class="flex items-center gap-2 px-2 py-[5px] rounded-[4px] bg-ctp-surface0/40 transition-all duration-150
                focus-within:bg-ctp-surface0/70">
      <svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-overlay1)" stroke-width="1.5" stroke-linecap="round" class="flex-shrink-0 opacity-60">
        <circle cx="7" cy="7" r="4" /><path d="M10 10l3.5 3.5" />
      </svg>
      <input type="text" bind:value={searchQuery} placeholder="Search..."
        class="flex-1 bg-transparent text-[13px] text-ctp-text outline-none placeholder:text-ctp-overlay0 border-none" />
      {#if searchQuery}
        <button on:click={clearSearch}
          class="w-5 h-5 flex items-center justify-center rounded text-ctp-overlay1 hover:text-ctp-text hover:bg-ctp-surface0 transition-colors">
          <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
            <path d="M4 4l8 8M12 4l-8 8" />
          </svg>
        </button>
      {/if}
    </div>
  </div>

  <!-- File tree -->
  <div class="flex-1 overflow-y-auto px-1.5 pb-2">
    {#each filteredItems as item}
      {#if item.isFolder}
        <button
          class="w-full flex items-center gap-1 min-h-[28px] px-2 py-[3px] text-[13px] text-ctp-subtext1
                 hover:bg-ctp-surface0/40 rounded-[4px] transition-all duration-75 group"
          style="padding-left: {6 + item.depth * 16}px"
          on:click={() => toggleFolder(item.path)}>
          <!-- Chevron: hidden by default, visible on hover -->
          <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"
            class="text-ctp-overlay0 transition-all duration-150 flex-shrink-0 opacity-0 group-hover:opacity-100"
            class:rotate-90={item.expanded}
            class:!opacity-100={item.expanded}>
            <path d="M6 4l4 4-4 4" />
          </svg>
          <span class="truncate flex-1 text-left">{item.name}</span>
          {#if folderNoteCounts.has(item.path)}
            <span class="text-[11px] text-ctp-overlay0 font-normal tabular-nums ml-auto opacity-0 group-hover:opacity-100 transition-opacity duration-100">{folderNoteCounts.get(item.path)}</span>
          {/if}
        </button>
      {:else}
        {@const isActive = item.path === activeNotePath}
        <button
          class="w-full flex items-center gap-1.5 min-h-[28px] px-2 py-[3px] text-[13px] rounded-[4px] transition-all duration-75 group
                 {isActive ? 'bg-ctp-surface0 text-ctp-text font-medium' : 'text-ctp-subtext1 hover:bg-ctp-surface0/40'}"
          style="padding-left: {6 + (viewMode === 'flat' ? 0 : item.depth * 16) + 14}px"
          on:click={() => selectNote(item.path)}
          on:contextmenu={(e) => handleContextMenu(e, item.path)}>
          <!-- Page icon: hidden by default, faintly visible on hover, visible when active -->
          <svg width="13" height="13" viewBox="0 0 16 16" fill="none"
            stroke="currentColor"
            stroke-width="1.3" stroke-linecap="round"
            class="flex-shrink-0 transition-all duration-100
                   {isActive ? 'opacity-50' : 'opacity-0 group-hover:opacity-30'}">
            <path d="M4 2h8v12H4V2zm2 3h4m-4 2.5h3" />
          </svg>
          <div class="flex flex-col min-w-0 flex-1">
            {#if searchQuery}
              <span class="truncate">{@html highlightMatch(item.name, searchQuery)}</span>
            {:else}
              <span class="truncate">{item.name}</span>
            {/if}
            {#if viewMode === 'flat'}
              {@const folder = getFolderPath(item.path)}
              {#if folder}
                <span class="text-[11px] text-ctp-overlay0 truncate leading-tight">{folder}</span>
              {/if}
            {/if}
          </div>
          <!-- Hover-reveal action: context menu trigger -->
          <div class="flex items-center gap-0.5 ml-auto opacity-0 group-hover:opacity-100 transition-opacity duration-100">
            <button on:click|stopPropagation={(e) => handleContextMenu(e, item.path)}
              class="w-[22px] h-[22px] flex items-center justify-center rounded-[4px] text-ctp-overlay0 hover:bg-ctp-surface1 hover:text-ctp-text transition-colors">
              <svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor">
                <circle cx="4" cy="8" r="1.2" /><circle cx="8" cy="8" r="1.2" /><circle cx="12" cy="8" r="1.2" />
              </svg>
            </button>
          </div>
        </button>
      {/if}
    {/each}

    {#if filteredItems.length === 0}
      <div class="flex flex-col items-center justify-center py-10 gap-3">
        <svg width="28" height="28" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-overlay0)" stroke-width="0.8" stroke-linecap="round" class="opacity-40">
          <path d="M4 2h8v12H4V2zm2 3h4m-4 2.5h3" />
        </svg>
        <p class="text-[13px] text-ctp-overlay0 text-center">
          {searchQuery ? 'No matching notes' : 'Empty vault'}
        </p>
        {#if !searchQuery}
          <p class="text-[12px] text-ctp-overlay0 text-center">
            Press <kbd class="bg-ctp-surface0 px-1.5 py-0.5 rounded text-[11px]">Ctrl+N</kbd> to create
          </p>
        {/if}
      </div>
    {/if}
  </div>

  <!-- Outline section -->
  {#if outlineItems.length > 0}
    <div class="border-t border-ctp-surface0/40 flex-shrink-0">
      <button class="w-full flex items-center justify-between px-3 py-2 hover:bg-ctp-surface0/30 transition-colors"
        on:click={() => outlineExpanded = !outlineExpanded}>
        <span class="text-[12px] font-medium text-ctp-subtext0">Outline</span>
        <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-overlay0)" stroke-width="2" stroke-linecap="round"
          class="transition-transform duration-150" class:rotate-90={outlineExpanded}>
          <path d="M6 4l4 4-4 4" />
        </svg>
      </button>
      {#if outlineExpanded}
        <div class="overflow-y-auto max-h-[200px] px-1.5 pb-2">
          {#each outlineItems as item}
            <button on:click={() => dispatch('jumpToLine', item.line)}
              class="w-full flex items-center gap-1.5 min-h-[26px] px-2 py-[3px] text-[13px] hover:bg-ctp-surface0/40 rounded-[4px] text-left transition-colors"
              style="padding-left: {6 + (item.level - 1) * 12}px">
              <span class="w-1 h-1 rounded-full flex-shrink-0 opacity-50" style="background: {outlineColors[item.level] || 'var(--ctp-text)'}"></span>
              <span class="truncate text-ctp-subtext1"
                style="font-weight: {item.level <= 2 ? '500' : '400'}">
                {item.text}
              </span>
            </button>
          {/each}
        </div>
      {/if}
    </div>
  {/if}

  <!-- Footer -->
  <div class="px-3 py-1.5 border-t border-ctp-surface0/30 flex items-center gap-1.5 text-[11px] text-ctp-overlay0">
    <span>{noteCount} notes</span>
    <span class="text-ctp-surface1 select-none">&middot;</span>
    <span>{folderCount} folders</span>
  </div>
</div>
