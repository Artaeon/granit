<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'

  export let results: any[] = []
  export let searching = false

  const dispatch = createEventDispatcher()

  let query = ''
  let inputEl: HTMLInputElement
  let selectedIndex = 0

  onMount(() => { if (inputEl) inputEl.focus() })

  function handleInput() {
    selectedIndex = 0
    if (query.length >= 2) {
      dispatch('search', query)
    } else {
      results = []
    }
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape') { dispatch('close'); return }
    if (event.key === 'ArrowDown') { event.preventDefault(); selectedIndex = Math.min(selectedIndex + 1, results.length - 1) }
    if (event.key === 'ArrowUp') { event.preventDefault(); selectedIndex = Math.max(selectedIndex - 1, 0) }
    if (event.key === 'Enter' && results[selectedIndex]) {
      dispatch('select', results[selectedIndex])
    }
  }

  // Group results by file
  $: groupedResults = (() => {
    const groups: Record<string, any[]> = {}
    for (const r of results) {
      if (!groups[r.relPath]) groups[r.relPath] = []
      groups[r.relPath].push(r)
    }
    return Object.entries(groups)
  })()
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center pt-[8%]" style="background:rgba(0,0,0,0.5);backdrop-filter:blur(2px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-2xl h-[70vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden">
    <!-- Search header -->
    <div class="flex items-center gap-2 px-4 py-3 border-b border-ctp-surface0">
      <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-overlay0)" stroke-width="1.5" stroke-linecap="round">
        <circle cx="7" cy="7" r="4" /><path d="M10 10l3.5 3.5" />
      </svg>
      <input type="text" bind:this={inputEl} bind:value={query}
        on:input={handleInput} on:keydown={handleKeydown}
        placeholder="Search across vault..."
        class="flex-1 bg-transparent text-sm text-ctp-text outline-none placeholder:text-ctp-overlay0" />
      {#if searching}
        <span class="text-[10px] text-ctp-overlay0 animate-pulse">Searching...</span>
      {/if}
      <kbd class="text-[10px] text-ctp-overlay0 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer" on:click={() => dispatch('close')}>esc</kbd>
    </div>

    <!-- Results -->
    <div class="flex-1 overflow-y-auto py-1">
      {#if results.length > 0}
        {#each groupedResults as [filePath, hits], gi}
          <div class="px-3 pt-2 pb-1">
            <span class="text-[10px] font-semibold text-ctp-blue truncate">{hits[0].title || filePath}</span>
            <span class="text-[10px] text-ctp-surface2 ml-1">{filePath}</span>
          </div>
          {#each hits as hit, hi}
            {@const flatIdx = results.indexOf(hit)}
            <button
              class="w-full flex items-start gap-2 px-4 py-1.5 text-left transition-colors"
              class:bg-ctp-surface0={flatIdx === selectedIndex}
              on:click={() => dispatch('select', hit)}>
              <span class="text-[10px] text-ctp-overlay0 font-mono w-8 text-right flex-shrink-0 pt-0.5">L{hit.line}</span>
              <span class="text-[12px] text-ctp-text truncate flex-1">{hit.matchLine}</span>
            </button>
          {/each}
        {/each}
      {:else if query.length >= 2}
        <p class="text-center text-sm text-ctp-overlay0 py-12">No results found</p>
      {:else}
        <p class="text-center text-sm text-ctp-overlay0 py-12">Type at least 2 characters to search</p>
      {/if}
    </div>

    <!-- Footer -->
    <div class="flex items-center gap-3 px-4 py-2 border-t border-ctp-surface0 text-[10px] text-ctp-surface2">
      <span>{results.length} matches</span>
      <span class="text-ctp-surface1">&middot;</span>
      <span>↑↓ navigate</span>
      <span class="text-ctp-surface1">&middot;</span>
      <span>↵ open</span>
    </div>
  </div>
</div>
