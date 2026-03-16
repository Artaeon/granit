<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'
  import type { NoteInfo } from './types'

  export let notes: NoteInfo[] = []
  export let starred: string[] = []
  export let recent: string[] = []

  const dispatch = createEventDispatcher()

  let query = ''
  let inputEl: HTMLInputElement
  let selectedIndex = 0

  onMount(() => { if (inputEl) inputEl.focus() })

  function fuzzyMatch(text: string, pattern: string): boolean {
    const p = pattern.toLowerCase()
    const t = text.toLowerCase()
    if (t.includes(p)) return true
    let pi = 0
    for (let i = 0; i < t.length && pi < p.length; i++) {
      if (t[i] === p[pi]) pi++
    }
    return pi === p.length
  }

  $: filteredNotes = (() => {
    let list = notes

    if (query) {
      list = list.filter(n => fuzzyMatch(n.title, query) || fuzzyMatch(n.relPath, query))
    }

    // Sort: starred first, then recent, then alphabetical
    const starredSet = new Set(starred)
    const recentMap = new Map(recent.map((r, i) => [r, i]))

    return list.sort((a, b) => {
      const aStarred = starredSet.has(a.relPath) ? 0 : 1
      const bStarred = starredSet.has(b.relPath) ? 0 : 1
      if (aStarred !== bStarred) return aStarred - bStarred

      const aRecent = recentMap.has(a.relPath) ? recentMap.get(a.relPath)! : 999
      const bRecent = recentMap.has(b.relPath) ? recentMap.get(b.relPath)! : 999
      if (aRecent !== bRecent) return aRecent - bRecent

      return a.title.localeCompare(b.title)
    }).slice(0, 30)
  })()

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape') { dispatch('close'); return }
    if (event.key === 'ArrowDown') { event.preventDefault(); selectedIndex = Math.min(selectedIndex + 1, filteredNotes.length - 1) }
    if (event.key === 'ArrowUp') { event.preventDefault(); selectedIndex = Math.max(selectedIndex - 1, 0) }
    if (event.key === 'Enter' && filteredNotes[selectedIndex]) {
      dispatch('select', filteredNotes[selectedIndex].relPath)
    }
  }

  $: selectedIndex = Math.min(selectedIndex, Math.max(0, filteredNotes.length - 1))
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center pt-[12%]" style="background:rgba(0,0,0,0.5);backdrop-filter:blur(2px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-lg max-h-[50vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden">
    <!-- Search input -->
    <div class="flex items-center gap-2 px-4 py-3 border-b border-ctp-surface0">
      <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-mauve)" stroke-width="1.5" stroke-linecap="round">
        <path d="M4 2h8v12H4V2z" /><path d="M6 5h4m-4 2.5h3" />
      </svg>
      <input type="text" bind:this={inputEl} bind:value={query}
        on:keydown={handleKeydown}
        placeholder="Quick switch..."
        class="flex-1 bg-transparent text-sm text-ctp-text outline-none placeholder:text-ctp-overlay0" />
      <kbd class="text-[10px] text-ctp-overlay0 bg-ctp-surface0 px-1.5 py-0.5 rounded">Ctrl+J</kbd>
    </div>

    <!-- Note list -->
    <div class="flex-1 overflow-y-auto py-1">
      {#each filteredNotes as note, i}
        {@const isStarred = starred.includes(note.relPath)}
        {@const isRecent = recent.includes(note.relPath)}
        <button
          class="w-full flex items-center gap-2 px-4 py-2 text-left transition-colors"
          class:bg-ctp-surface0={i === selectedIndex}
          class:text-ctp-text={i === selectedIndex}
          class:text-ctp-subtext1={i !== selectedIndex}
          on:click={() => dispatch('select', note.relPath)}>
          {#if isStarred}
            <span class="text-ctp-yellow text-[12px] flex-shrink-0">★</span>
          {:else if isRecent}
            <span class="text-ctp-overlay0 text-[12px] flex-shrink-0">◷</span>
          {:else}
            <span class="text-ctp-surface2 text-[12px] flex-shrink-0">◇</span>
          {/if}
          <span class="text-[13px] truncate flex-1">{note.title}</span>
          <span class="text-[10px] text-ctp-overlay0 truncate max-w-[120px]">{note.relPath}</span>
        </button>
      {/each}

      {#if filteredNotes.length === 0}
        <p class="text-center text-sm text-ctp-overlay0 py-8">No notes found</p>
      {/if}
    </div>
  </div>
</div>
