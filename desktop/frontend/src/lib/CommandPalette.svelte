<script lang="ts">
  import { createEventDispatcher, onMount, tick } from 'svelte'
  import type { NoteInfo } from './types'
  import { allCommands, iconSvg } from './commands'
  import type { Command } from './commands'

  export let notes: NoteInfo[] = []
  export let initialMode: 'files' | 'commands' = 'files'

  const dispatch = createEventDispatcher()

  let query = ''
  let selectedIndex = 0
  let inputEl: HTMLInputElement
  let listEl: HTMLDivElement
  let mode: 'files' | 'commands' = initialMode

  onMount(async () => {
    if (initialMode === 'commands') {
      mode = 'commands'
      query = ''
    }
    await tick()
    inputEl?.focus()
  })

  function fuzzyMatch(text: string, q: string): boolean {
    const tl = text.toLowerCase()
    const ql = q.toLowerCase()
    if (tl.includes(ql)) return true
    let qi = 0
    for (let i = 0; i < tl.length && qi < ql.length; i++) {
      if (tl[i] === ql[qi]) qi++
    }
    return qi === ql.length
  }

  /** Returns HTML string with matched characters wrapped in <mark> tags */
  function fuzzyHighlight(text: string, q: string): string {
    if (!q) return text
    const tl = text.toLowerCase()
    const ql = q.toLowerCase()

    // Try substring match first (highlight contiguous run)
    const subIdx = tl.indexOf(ql)
    if (subIdx !== -1) {
      const before = text.slice(0, subIdx)
      const match = text.slice(subIdx, subIdx + ql.length)
      const after = text.slice(subIdx + ql.length)
      return escapeHtml(before) + '<mark class="text-ctp-blue bg-transparent font-bold">' + escapeHtml(match) + '</mark>' + escapeHtml(after)
    }

    // Fuzzy: highlight individual matched characters
    const indices: number[] = []
    let qi = 0
    for (let i = 0; i < tl.length && qi < ql.length; i++) {
      if (tl[i] === ql[qi]) {
        indices.push(i)
        qi++
      }
    }
    if (qi < ql.length) return escapeHtml(text)

    const matchSet = new Set(indices)
    let result = ''
    for (let i = 0; i < text.length; i++) {
      if (matchSet.has(i)) {
        result += '<mark class="text-ctp-blue bg-transparent font-bold">' + escapeHtml(text[i]) + '</mark>'
      } else {
        result += escapeHtml(text[i])
      }
    }
    return result
  }

  function escapeHtml(s: string): string {
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  }

  // Category display order for grouped command view
  const categoryOrder = ['File', 'Navigate', 'Editor', 'View', 'Layout', 'Search', 'AI', 'Git', 'Export', 'Tools', 'App']

  interface CategoryGroup {
    category: string
    commands: { cmd: Command; globalIndex: number }[]
  }

  function groupByCategory(cmds: Command[]): CategoryGroup[] {
    const map = new Map<string, { cmd: Command; globalIndex: number }[]>()
    cmds.forEach((cmd, i) => {
      const list = map.get(cmd.category) || []
      list.push({ cmd, globalIndex: i })
      map.set(cmd.category, list)
    })
    const groups: CategoryGroup[] = []
    for (const cat of categoryOrder) {
      if (map.has(cat)) groups.push({ category: cat, commands: map.get(cat)! })
    }
    // Include any categories not in the predefined order
    for (const [cat, cmds] of map) {
      if (!categoryOrder.includes(cat)) groups.push({ category: cat, commands: cmds })
    }
    return groups
  }

  $: {
    if (query.startsWith('>')) {
      mode = 'commands'
    } else if (initialMode === 'commands' && query === '') {
      mode = 'commands'
    } else if (!query.startsWith('>') && initialMode !== 'commands') {
      mode = 'files'
    }
  }

  $: commandQuery = mode === 'commands' ? (query.startsWith('>') ? query.slice(1).trim() : query) : ''

  $: filteredFiles = mode === 'files'
    ? (query
        ? notes.filter(n => fuzzyMatch(n.title, query) || fuzzyMatch(n.relPath, query)).slice(0, 25)
        : notes.slice(0, 25))
    : []

  $: totalFileCount = mode === 'files'
    ? (query
        ? notes.filter(n => fuzzyMatch(n.title, query) || fuzzyMatch(n.relPath, query)).length
        : notes.length)
    : 0

  $: filteredCommands = mode === 'commands'
    ? (commandQuery
        ? allCommands.filter(c => fuzzyMatch(c.label, commandQuery) || fuzzyMatch(c.desc, commandQuery) || fuzzyMatch(c.category, commandQuery)).slice(0, 30)
        : allCommands.slice(0, 30))
    : []

  $: totalCommandCount = mode === 'commands'
    ? (commandQuery
        ? allCommands.filter(c => fuzzyMatch(c.label, commandQuery) || fuzzyMatch(c.desc, commandQuery) || fuzzyMatch(c.category, commandQuery)).length
        : allCommands.length)
    : 0

  $: commandGroups = mode === 'commands' ? groupByCategory(filteredCommands) : []

  $: items = mode === 'commands' ? filteredCommands : filteredFiles
  $: selectedIndex = Math.min(selectedIndex, Math.max(0, items.length - 1))

  $: activeQuery = mode === 'files' ? query : commandQuery

  function handleKeydown(event: KeyboardEvent) {
    switch (event.key) {
      case 'ArrowDown':
        event.preventDefault()
        selectedIndex = Math.min(selectedIndex + 1, items.length - 1)
        scrollToSelected()
        break
      case 'ArrowUp':
        event.preventDefault()
        selectedIndex = Math.max(selectedIndex - 1, 0)
        scrollToSelected()
        break
      case 'Enter':
        event.preventDefault()
        if (mode === 'commands' && filteredCommands[selectedIndex]) {
          dispatch('command', filteredCommands[selectedIndex].action)
        } else if (mode === 'files' && filteredFiles[selectedIndex]) {
          dispatch('select', filteredFiles[selectedIndex].relPath)
        }
        break
      case 'Escape':
        dispatch('close')
        break
      case 'Tab':
        event.preventDefault()
        mode = mode === 'files' ? 'commands' : 'files'
        query = ''
        selectedIndex = 0
        break
    }
  }

  function scrollToSelected() {
    tick().then(() => {
      const el = listEl?.querySelector(`[data-index="${selectedIndex}"]`)
      el?.scrollIntoView({ block: 'nearest' })
    })
  }

  function getIcon(name: string) {
    return iconSvg[name] || iconSvg['search'] || ''
  }

  function folderOf(path: string) {
    const parts = path.split('/')
    return parts.length > 1 ? parts.slice(0, -1).join('/') : ''
  }
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center pt-[12vh]"
  style="background: rgba(17,17,27,0.6); backdrop-filter: blur(8px);"
  on:click|self={() => dispatch('close')}>

  <div class="w-full max-w-[600px] bg-ctp-mantle rounded-xl border border-ctp-surface0 overflow-hidden shadow-overlay">

    <!-- Input row -->
    <div class="px-5 pt-5 pb-0">
      <div class="flex items-center gap-3">
        {#if mode === 'commands'}
          <span class="text-ctp-mauve text-xl font-bold">&gt;</span>
        {:else}
          <svg width="20" height="20" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-overlay1)" stroke-width="1.5" stroke-linecap="round">
            <circle cx="7" cy="7" r="4.5" /><path d="M11 11l3.5 3.5" />
          </svg>
        {/if}
        <input bind:this={inputEl} bind:value={query} on:keydown={handleKeydown}
          placeholder={mode === 'commands' ? 'Type a command...' : 'Search notes...'}
          class="flex-1 bg-transparent text-ctp-text text-lg py-0.5 outline-none border-none placeholder:text-ctp-overlay0" />
      </div>
      <p class="text-[12px] text-ctp-overlay0 px-5 pb-2">Type &gt; for commands</p>
    </div>
    <div class="border-b border-ctp-surface0/30"></div>

    <!-- Results -->
    <div bind:this={listEl} class="max-h-[420px] overflow-y-auto py-1">
      {#if mode === 'files'}
        {#each filteredFiles as note, i}
          <div data-index={i}
            class="flex items-center gap-3 px-5 py-2.5 cursor-pointer transition-colors duration-75
              {i === selectedIndex ? 'bg-ctp-surface0' : 'hover:bg-ctp-surface0/50'}"
            on:click={() => dispatch('select', note.relPath)}
            on:mouseenter={() => selectedIndex = i}>
            <svg class="flex-shrink-0 opacity-30" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-overlay0)" stroke-width="1.5" stroke-linecap="round">
              <path d="M3 2h10v12H3V2zm2 3h6m-6 3h4" />
            </svg>
            <div class="min-w-0 flex-1">
              <div class="text-sm truncate {i === selectedIndex ? 'text-ctp-text' : 'text-ctp-subtext1'}">{@html fuzzyHighlight(note.title, activeQuery)}</div>
              {#if folderOf(note.relPath)}
                <div class="text-[12px] text-ctp-overlay0 truncate">{folderOf(note.relPath)}</div>
              {/if}
            </div>
          </div>
        {/each}

      {:else}
        {#each commandGroups as group}
          <!-- Category header -->
          <div class="px-5 pt-3 pb-1 flex items-center gap-2">
            <span class="text-[11px] font-semibold uppercase tracking-wider text-ctp-overlay0">{group.category}</span>
            <div class="flex-1 h-px bg-ctp-surface0"></div>
          </div>
          {#each group.commands as { cmd, globalIndex }}
            <div data-index={globalIndex}
              class="flex items-center gap-3 px-5 py-2.5 cursor-pointer transition-colors duration-75
                {globalIndex === selectedIndex ? 'bg-ctp-surface0' : 'hover:bg-ctp-surface0/50'}"
              on:click={() => dispatch('command', cmd.action)}
              on:mouseenter={() => selectedIndex = globalIndex}>
              <div class="w-7 h-7 rounded-lg flex items-center justify-center flex-shrink-0 opacity-30 bg-ctp-surface0">
                <svg width="14" height="14" viewBox="0 0 16 16" fill="none"
                  stroke="var(--ctp-overlay1)"
                  stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
                  <path d="{getIcon(cmd.icon)}" />
                </svg>
              </div>
              <div class="min-w-0 flex-1">
                <div class="text-sm {globalIndex === selectedIndex ? 'text-ctp-text' : 'text-ctp-subtext1'}">{@html fuzzyHighlight(cmd.label, activeQuery)}</div>
                <div class="text-[12px] text-ctp-overlay1 truncate">{cmd.desc}</div>
              </div>
              <div class="flex items-center gap-2 flex-shrink-0">
                {#if cmd.shortcut}
                  <kbd class="text-[11px] text-ctp-overlay1 bg-ctp-surface0 border border-ctp-surface1 px-1.5 py-0.5 rounded font-mono">{cmd.shortcut}</kbd>
                {/if}
              </div>
            </div>
          {/each}
        {/each}
      {/if}

      {#if items.length === 0}
        <p class="px-5 py-8 text-center text-sm text-ctp-overlay1">
          {mode === 'files' ? 'No matching notes' : 'No matching commands'}
        </p>
      {/if}
    </div>

    <!-- Footer -->
    <div class="flex items-center justify-end px-5 py-2.5 border-t border-ctp-surface0 text-[12px] text-ctp-overlay0">
      {#if mode === 'files'}
        {filteredFiles.length} of {totalFileCount} notes
      {:else}
        {filteredCommands.length} of {totalCommandCount} commands
      {/if}
    </div>
  </div>
</div>
