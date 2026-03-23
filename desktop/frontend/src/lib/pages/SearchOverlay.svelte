<script lang="ts">
  import { createEventDispatcher, onMount, onDestroy, tick } from 'svelte'
  import { notes, navigateToPage, closeAllOverlays } from '../stores'
  import { search as searchApi } from '../api'
  import type { NoteInfo, SearchHit } from '../types'

  const dispatch = createEventDispatcher()

  let query = ''
  let results: SearchHit[] = []
  let noteResults: NoteInfo[] = []
  let searching = false
  let selectedIndex = 0
  let inputEl: HTMLInputElement
  let searchTimeout: ReturnType<typeof setTimeout>

  onMount(() => {
    tick().then(() => inputEl?.focus())
  })

  onDestroy(() => {
    clearTimeout(searchTimeout)
  })

  $: {
    // Title search (instant)
    noteResults = query.length > 0
      ? $notes.filter(n => n.title.toLowerCase().includes(query.toLowerCase())).slice(0, 5)
      : []
    selectedIndex = 0
  }

  function doSearch() {
    if (!query.trim()) { results = []; return }
    clearTimeout(searchTimeout)
    searchTimeout = setTimeout(async () => {
      searching = true
      try {
        results = await searchApi(query) || []
      } catch { results = [] }
      searching = false
    }, 300)
  }

  $: if (query) doSearch()

  $: allResults = [
    ...noteResults.map(n => ({ type: 'page' as const, relPath: n.relPath, title: n.title, context: '' })),
    ...results
      .filter(r => !noteResults.some(n => n.relPath === r.relPath))
      .map(r => ({ type: 'match' as const, relPath: r.relPath, title: r.title, context: r.matchLine })),
  ]

  function select(item: typeof allResults[0]) {
    navigateToPage(item.relPath)
    closeAllOverlays()
    dispatch('close')
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      selectedIndex = Math.min(selectedIndex + 1, allResults.length - 1)
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      selectedIndex = Math.max(selectedIndex - 1, 0)
    } else if (e.key === 'Enter') {
      e.preventDefault()
      if (allResults[selectedIndex]) select(allResults[selectedIndex])
    } else if (e.key === 'Escape') {
      dispatch('close')
    }
  }
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center pt-[12%]"
  style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)"
  on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-xl h-fit bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl overflow-hidden">
    <!-- Search input -->
    <div class="flex items-center gap-2 px-4 py-3 border-b border-ctp-surface0">
      <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-overlay1)" stroke-width="1.5" stroke-linecap="round">
        <circle cx="7" cy="7" r="4" /><path d="M10 10l3.5 3.5" />
      </svg>
      <input bind:this={inputEl} bind:value={query} placeholder="Search pages and content..."
        class="flex-1 bg-transparent text-[15px] text-ctp-text outline-none placeholder:text-ctp-overlay0 border-none"
        on:keydown={handleKeydown} />
      <kbd class="text-[11px] text-ctp-overlay0 bg-ctp-surface0 px-1.5 py-0.5 rounded">esc</kbd>
    </div>

    <!-- Results -->
    {#if allResults.length > 0}
      <div class="max-h-[400px] overflow-y-auto py-1">
        {#each allResults as item, i}
          <button on:click={() => select(item)}
            class="w-full text-left px-4 py-2 flex items-start gap-3 transition-colors
              {i === selectedIndex ? 'bg-ctp-blue/10' : 'hover:bg-ctp-surface0/40'}">
            <span class="text-[11px] font-medium px-1.5 py-0.5 rounded mt-0.5 flex-shrink-0
              {item.type === 'page' ? 'text-ctp-blue bg-ctp-blue/10' : 'text-ctp-overlay0 bg-ctp-surface0'}">
              {item.type === 'page' ? 'PAGE' : 'MATCH'}
            </span>
            <div class="min-w-0 flex-1">
              <div class="text-[14px] truncate"
                class:text-ctp-text={i === selectedIndex}
                class:text-ctp-subtext1={i !== selectedIndex}>
                {item.title}
              </div>
              {#if item.context}
                <div class="text-[12px] text-ctp-overlay0 truncate mt-0.5">{item.context}</div>
              {/if}
            </div>
          </button>
        {/each}
      </div>
    {:else if query && !searching}
      <div class="px-4 py-8 text-center text-[13px] text-ctp-overlay0">
        No results for "{query}"
      </div>
    {/if}

    <!-- Footer -->
    <div class="px-4 py-2 border-t border-ctp-surface0 text-[11px] text-ctp-overlay0 flex gap-4">
      <span><kbd class="bg-ctp-surface0 px-1 py-px rounded">&uarr;&darr;</kbd> navigate</span>
      <span><kbd class="bg-ctp-surface0 px-1 py-px rounded">Enter</kbd> open</span>
      <span><kbd class="bg-ctp-surface0 px-1 py-px rounded">Esc</kbd> close</span>
    </div>
  </div>
</div>
