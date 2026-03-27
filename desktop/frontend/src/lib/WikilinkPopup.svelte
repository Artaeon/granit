<script lang="ts">
  import { createEventDispatcher, onMount, tick } from 'svelte'
  import type { NoteInfo } from './types'

  export let x: number = 0
  export let y: number = 0
  export let query: string = ''
  export let notes: NoteInfo[] = []

  const dispatch = createEventDispatcher()
  const maxVisible = 8

  let selectedIndex = 0
  let listEl: HTMLDivElement

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

  $: filtered = query
    ? notes.filter(n => fuzzyMatch(n.title, query) || fuzzyMatch(n.relPath, query)).slice(0, 20)
    : notes.slice(0, 20)

  $: selectedIndex = Math.min(selectedIndex, Math.max(0, filtered.length - 1))

  $: visible = filtered.slice(0, maxVisible)

  export function handleKeydown(e: KeyboardEvent): boolean {
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault()
        selectedIndex = Math.min(selectedIndex + 1, filtered.length - 1)
        scrollToSelected()
        return true
      case 'ArrowUp':
        e.preventDefault()
        selectedIndex = Math.max(selectedIndex - 1, 0)
        scrollToSelected()
        return true
      case 'Enter':
        e.preventDefault()
        if (filtered[selectedIndex]) {
          dispatch('select', filtered[selectedIndex].title)
        }
        return true
      case 'Escape':
        e.preventDefault()
        dispatch('close')
        return true
      case 'Tab':
        e.preventDefault()
        if (filtered[selectedIndex]) {
          dispatch('select', filtered[selectedIndex].title)
        }
        return true
    }
    return false
  }

  function scrollToSelected() {
    tick().then(() => {
      const el = listEl?.querySelector(`[data-index="${selectedIndex}"]`)
      el?.scrollIntoView({ block: 'nearest' })
    })
  }

  function folderOf(path: string): string {
    const parts = path.split('/')
    return parts.length > 1 ? parts.slice(0, -1).join('/') : ''
  }
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
{#if filtered.length > 0}
  <div class="fixed z-50" style="left: {x}px; top: {y}px;">
    <div class="bg-ctp-surface0 border border-ctp-surface1 rounded-lg shadow-xl overflow-hidden min-w-[240px] max-w-[360px]">

      <!-- Header -->
      <div class="px-3 py-1.5 border-b border-ctp-surface1">
        <div class="flex items-center gap-1.5">
          <span class="text-[12px] text-ctp-blue font-semibold">Link to...</span>
          {#if query}
            <span class="text-[13px] text-ctp-text font-medium">{query}</span>
          {/if}
        </div>
      </div>

      <!-- Results list -->
      <div bind:this={listEl} class="max-h-[264px] overflow-y-auto py-1">
        {#each filtered.slice(0, maxVisible) as note, i}
          <div
            data-index={i}
            class="flex items-center gap-2 px-3 py-1.5 cursor-pointer transition-colors duration-75
              {i === selectedIndex ? 'bg-ctp-blue/15 text-ctp-text' : 'hover:bg-ctp-surface1/50 text-ctp-subtext1'}"
            on:click={() => dispatch('select', note.title)}
            on:mouseenter={() => selectedIndex = i}
          >
            <svg width="12" height="12" viewBox="0 0 16 16" fill="none"
              stroke="{i === selectedIndex ? 'var(--ctp-blue)' : 'var(--ctp-overlay0)'}"
              stroke-width="1.5" stroke-linecap="round">
              <path d="M3 2h10v12H3V2zm2 3h6m-6 3h4" />
            </svg>
            <div class="min-w-0 flex-1">
              <div class="text-[13px] truncate">{note.title}</div>
              {#if folderOf(note.relPath)}
                <div class="text-[11px] text-ctp-overlay1 truncate">{folderOf(note.relPath)}</div>
              {/if}
            </div>
          </div>
        {/each}
      </div>

      <!-- Scroll hint -->
      {#if filtered.length > maxVisible}
        <div class="px-3 py-1 border-t border-ctp-surface1 text-[11px] text-ctp-overlay1 text-right">
          +{filtered.length - maxVisible} more
        </div>
      {/if}
    </div>
  </div>
{/if}
