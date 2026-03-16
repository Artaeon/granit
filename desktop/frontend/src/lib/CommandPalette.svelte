<script lang="ts">
  import { createEventDispatcher, onMount, tick } from 'svelte'
  import type { NoteInfo } from './types'
  import { allCommands, iconSvg } from './commands'

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

  $: filteredCommands = mode === 'commands'
    ? (commandQuery
        ? allCommands.filter(c => fuzzyMatch(c.label, commandQuery) || fuzzyMatch(c.desc, commandQuery) || fuzzyMatch(c.category, commandQuery)).slice(0, 30)
        : allCommands.slice(0, 30))
    : []

  $: items = mode === 'commands' ? filteredCommands : filteredFiles
  $: selectedIndex = Math.min(selectedIndex, Math.max(0, items.length - 1))

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

  <div class="w-full max-w-[560px] bg-ctp-mantle rounded-2xl border border-ctp-surface0 overflow-hidden shadow-overlay">

    <!-- Input row -->
    <div class="flex items-center gap-3 px-5 py-4 border-b border-ctp-surface0">
      {#if mode === 'commands'}
        <span class="text-ctp-mauve text-lg font-bold">&gt;</span>
      {:else}
        <svg width="18" height="18" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-overlay0)" stroke-width="1.5" stroke-linecap="round">
          <circle cx="7" cy="7" r="4.5" /><path d="M11 11l3.5 3.5" />
        </svg>
      {/if}
      <input bind:this={inputEl} bind:value={query} on:keydown={handleKeydown}
        placeholder={mode === 'commands' ? 'Type a command...' : 'Search notes...'}
        class="flex-1 bg-transparent text-ctp-text text-[15px] outline-none placeholder:text-ctp-overlay0" />

      <!-- Mode tabs -->
      <div class="flex bg-ctp-surface0 rounded-lg p-0.5 gap-0.5">
        <button on:click={() => { mode = 'files'; query = ''; selectedIndex = 0 }}
          class="px-2 py-0.5 text-[10px] rounded-md transition-all {mode === 'files' ? 'bg-ctp-surface1 text-ctp-text' : 'text-ctp-overlay0 hover:text-ctp-subtext0'}">
          Files
        </button>
        <button on:click={() => { mode = 'commands'; query = ''; selectedIndex = 0 }}
          class="px-2 py-0.5 text-[10px] rounded-md transition-all {mode === 'commands' ? 'bg-ctp-surface1 text-ctp-text' : 'text-ctp-overlay0 hover:text-ctp-subtext0'}">
          Commands
        </button>
      </div>
    </div>

    <!-- Results -->
    <div bind:this={listEl} class="max-h-[400px] overflow-y-auto py-2">
      {#if mode === 'files'}
        {#each filteredFiles as note, i}
          <div data-index={i}
            class="flex items-center gap-3 px-5 py-2 cursor-pointer transition-colors duration-75
              {i === selectedIndex ? 'bg-ctp-surface0 border-l-2 border-ctp-blue' : 'hover:bg-ctp-surface0/50 border-l-2 border-transparent'}"
            on:click={() => dispatch('select', note.relPath)}
            on:mouseenter={() => selectedIndex = i}>
            <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="{i === selectedIndex ? 'var(--ctp-blue)' : 'var(--ctp-overlay0)'}" stroke-width="1.5" stroke-linecap="round">
              <path d="M3 2h10v12H3V2zm2 3h6m-6 3h4" />
            </svg>
            <div class="min-w-0 flex-1">
              <div class="text-[13px] truncate {i === selectedIndex ? 'text-ctp-text' : 'text-ctp-subtext1'}">{note.title}</div>
              {#if folderOf(note.relPath)}
                <div class="text-[10px] text-ctp-overlay0 truncate">{folderOf(note.relPath)}</div>
              {/if}
            </div>
          </div>
        {/each}

      {:else}
        {#each filteredCommands as cmd, i}
          <div data-index={i}
            class="flex items-center gap-3 px-5 py-2.5 cursor-pointer transition-colors duration-75
              {i === selectedIndex ? 'bg-ctp-surface0 border-l-2 border-ctp-mauve' : 'hover:bg-ctp-surface0/50 border-l-2 border-transparent'}"
            on:click={() => dispatch('command', cmd.action)}
            on:mouseenter={() => selectedIndex = i}>
            <div class="w-7 h-7 rounded-lg flex items-center justify-center flex-shrink-0
              {i === selectedIndex ? 'bg-ctp-blue/20' : 'bg-ctp-surface0'}">
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none"
                stroke="{i === selectedIndex ? 'var(--ctp-blue)' : 'var(--ctp-overlay1)'}"
                stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
                <path d="{getIcon(cmd.icon)}" />
              </svg>
            </div>
            <div class="min-w-0 flex-1">
              <div class="text-[13px] {i === selectedIndex ? 'text-ctp-text' : 'text-ctp-subtext1'}">{cmd.label}</div>
              <div class="text-[10px] text-ctp-overlay0 truncate">{cmd.desc}</div>
            </div>
            <div class="flex items-center gap-2 flex-shrink-0">
              {#if cmd.shortcut}
                <kbd class="text-[9px] text-ctp-overlay0 bg-ctp-surface0 border border-ctp-surface1 px-1.5 py-0.5 rounded font-mono">{cmd.shortcut}</kbd>
              {/if}
              <span class="text-[9px] text-ctp-surface2">{cmd.category}</span>
            </div>
          </div>
        {/each}
      {/if}

      {#if items.length === 0}
        <p class="px-5 py-8 text-center text-sm text-ctp-overlay0">
          {mode === 'files' ? 'No matching notes' : 'No matching commands'}
        </p>
      {/if}
    </div>

    <!-- Footer -->
    <div class="flex items-center justify-between px-5 py-2 border-t border-ctp-surface0 text-[10px] text-ctp-overlay0">
      <div class="flex gap-3">
        <span><kbd class="bg-ctp-surface0 px-1 py-px rounded">Tab</kbd> switch mode</span>
        <span><kbd class="bg-ctp-surface0 px-1 py-px rounded">&uarr;&darr;</kbd> navigate</span>
        <span><kbd class="bg-ctp-surface0 px-1 py-px rounded">Enter</kbd> select</span>
      </div>
      <span>{items.length} results</span>
    </div>
  </div>
</div>
