<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import type { FolderNode, FlatTreeItem } from './types'

  export let tree: FolderNode | null = null
  export let activeNotePath = ''
  export let outlineItems: any[] = []

  const dispatch = createEventDispatcher()

  let searchQuery = ''
  let expandedFolders = new Set<string>()
  let outlineExpanded = true

  $: flatItems = tree ? flattenTree(tree, 0) : []
  $: filteredItems = searchQuery
    ? flatItems.filter(item =>
        !item.isFolder && item.name.toLowerCase().includes(searchQuery.toLowerCase())
      )
    : flatItems

  $: noteCount = flatItems.filter(i => !i.isFolder).length
  $: folderCount = flatItems.filter(i => i.isFolder).length

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

  function selectNote(path: string) { dispatch('select', path) }

  function handleContextMenu(event: MouseEvent, path: string) {
    event.preventDefault()
    if (confirm(`Delete "${path}"?`)) dispatch('delete', path)
  }

  function clearSearch() { searchQuery = '' }

  const outlineColors: Record<number, string> = {
    1: 'var(--ctp-mauve)', 2: 'var(--ctp-blue)', 3: 'var(--ctp-sapphire)',
    4: 'var(--ctp-teal)', 5: 'var(--ctp-green)', 6: 'var(--ctp-yellow)',
  }
</script>

<div class="flex flex-col h-full bg-ctp-mantle select-none">
  <!-- Header -->
  <div class="flex items-center justify-between px-4 pt-3 pb-2">
    <span class="text-[10px] font-bold text-ctp-overlay0 uppercase tracking-[0.15em]">Explorer</span>
    <button on:click={() => dispatch('create')}
      class="w-[26px] h-[26px] flex items-center justify-center rounded-md text-ctp-overlay1
             hover:bg-ctp-surface0 hover:text-ctp-text transition-all duration-100"
      data-tooltip="New note"
      title="New note (Ctrl+N)">
      <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
        <path d="M8 3v10M3 8h10" />
      </svg>
    </button>
  </div>

  <!-- Search -->
  <div class="px-3 pb-2">
    <div class="search-input-wrapper flex items-center gap-2 px-2.5 py-[6px] bg-ctp-base/60 rounded-lg border border-ctp-surface0/70 transition-all duration-150">
      <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-overlay0)" stroke-width="1.5" stroke-linecap="round" class="flex-shrink-0">
        <circle cx="7" cy="7" r="4" /><path d="M10 10l3.5 3.5" />
      </svg>
      <input type="text" bind:value={searchQuery} placeholder="Filter notes..."
        class="flex-1 bg-transparent text-[12px] text-ctp-text outline-none placeholder:text-ctp-surface2 border-none" />
      {#if searchQuery}
        <button on:click={clearSearch}
          class="w-4 h-4 flex items-center justify-center rounded text-ctp-overlay0 hover:text-ctp-text hover:bg-ctp-surface0 transition-colors text-[11px] p-0 bg-transparent border-0">
          &times;
        </button>
      {/if}
    </div>
  </div>

  <!-- File tree -->
  <div class="flex-1 overflow-y-auto px-2 pb-2">
    {#each filteredItems as item}
      {#if item.isFolder}
        <button
          class="w-full flex items-center gap-1.5 px-2 py-[5px] text-[12px] text-ctp-subtext0
                 hover:bg-ctp-surface0/50 rounded-md transition-all duration-75 group"
          style="padding-left: {8 + item.depth * 14}px"
          on:click={() => toggleFolder(item.path)}>
          <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"
            class="text-ctp-overlay0 transition-transform duration-150 flex-shrink-0"
            class:rotate-90={item.expanded}>
            <path d="M6 4l4 4-4 4" />
          </svg>
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke-width="1.3" stroke-linecap="round" class="flex-shrink-0"
            stroke={item.expanded ? 'var(--ctp-peach)' : 'var(--ctp-yellow)'}>
            <path d="M2 5h5l1.5-2H13a1 1 0 0 1 1 1v7a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V5z" />
          </svg>
          <span class="truncate font-medium">{item.name}</span>
        </button>
      {:else}
        {@const isActive = item.path === activeNotePath}
        <button
          class="w-full flex items-center gap-1.5 px-2 py-[5px] text-[12px] rounded-md transition-all duration-75 group"
          class:text-ctp-blue={isActive}
          class:font-medium={isActive}
          class:text-ctp-subtext1={!isActive}
          class:hover:bg-ctp-surface0={!isActive}
          style="padding-left: {8 + item.depth * 14 + 12}px;
                 {isActive ? 'background: color-mix(in srgb, var(--ctp-blue) 12%, transparent);' : ''}"
          on:click={() => selectNote(item.path)}
          on:contextmenu={(e) => handleContextMenu(e, item.path)}>
          {#if isActive}
            <div class="w-[3px] h-3 rounded-full bg-ctp-blue absolute left-1 flex-shrink-0"></div>
          {/if}
          <svg width="13" height="13" viewBox="0 0 16 16" fill="none"
            stroke="{isActive ? 'var(--ctp-blue)' : 'var(--ctp-surface2)'}"
            stroke-width="1.3" stroke-linecap="round" class="flex-shrink-0">
            <path d="M4 2h8v12H4V2zm2 3h4m-4 2.5h3" />
          </svg>
          <span class="truncate">{item.name}</span>
        </button>
      {/if}
    {/each}

    {#if filteredItems.length === 0}
      <div class="flex flex-col items-center justify-center py-8 gap-2">
        <svg width="24" height="24" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-surface2)" stroke-width="1" stroke-linecap="round" class="opacity-50">
          <path d="M4 2h8v12H4V2zm2 3h4m-4 2.5h3" />
        </svg>
        <p class="text-[11px] text-ctp-overlay0 text-center">
          {searchQuery ? 'No matching notes' : 'Empty vault'}
        </p>
        {#if !searchQuery}
          <p class="text-[10px] text-ctp-surface2 text-center">Press Ctrl+N to create</p>
        {/if}
      </div>
    {/if}
  </div>

  <!-- Outline section -->
  {#if outlineItems.length > 0}
    <div class="border-t border-ctp-surface0/50 flex-shrink-0">
      <button class="w-full flex items-center justify-between px-4 py-2 hover:bg-ctp-surface0/30 transition-colors"
        on:click={() => outlineExpanded = !outlineExpanded}>
        <span class="text-[10px] font-bold text-ctp-overlay0 uppercase tracking-[0.15em]">Outline</span>
        <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-overlay0)" stroke-width="2" stroke-linecap="round"
          class="transition-transform duration-150" class:rotate-90={outlineExpanded}>
          <path d="M6 4l4 4-4 4" />
        </svg>
      </button>
      {#if outlineExpanded}
        <div class="overflow-y-auto max-h-[200px] px-2 pb-2">
          {#each outlineItems as item}
            <button on:click={() => dispatch('jumpToLine', item.line)}
              class="w-full flex items-center gap-1 px-2 py-[3px] text-[11px] hover:bg-ctp-surface0/50 rounded-md text-left transition-colors"
              style="padding-left: {8 + (item.level - 1) * 12}px">
              <span class="w-1 h-1 rounded-full flex-shrink-0" style="background: {outlineColors[item.level] || 'var(--ctp-text)'}"></span>
              <span class="truncate"
                style="color: {outlineColors[item.level] || 'var(--ctp-text)'}; font-weight: {item.level <= 2 ? '600' : '400'}">
                {item.text}
              </span>
            </button>
          {/each}
        </div>
      {/if}
    </div>
  {/if}

  <!-- Footer -->
  <div class="px-4 py-2 border-t border-ctp-surface0/50 flex items-center gap-2 text-[10px] text-ctp-surface2">
    <span>{noteCount} notes</span>
    <span class="text-ctp-surface1">&middot;</span>
    <span>{folderCount} folders</span>
  </div>
</div>
