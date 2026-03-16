<script lang="ts">
  import { createEventDispatcher } from 'svelte'

  export let notePath: string = ''

  const dispatch = createEventDispatcher()
  const api = (window as any).go?.main?.GranitApp

  let suggestions: Array<{ target: string; context: string; line: number }> = []
  let loading = false
  let error = ''
  let linkedSet = new Set<number>()

  async function loadSuggestions() {
    if (!api || !notePath) return
    loading = true
    error = ''
    try {
      const result = await api.GetAutoLinkSuggestions(notePath)
      suggestions = result || []
    } catch (e: any) {
      error = e?.message || 'Failed to load suggestions'
      suggestions = []
    }
    loading = false
  }

  $: if (notePath) loadSuggestions()

  function linkOne(index: number) {
    linkedSet.add(index)
    linkedSet = linkedSet
    const s = suggestions[index]
    dispatch('link', { target: s.target, line: s.line })
  }

  function linkAll() {
    for (let i = 0; i < suggestions.length; i++) {
      if (!linkedSet.has(i)) {
        linkedSet.add(i)
        const s = suggestions[i]
        dispatch('link', { target: s.target, line: s.line })
      }
    }
    linkedSet = linkedSet
  }

  $: pendingCount = suggestions.length - linkedSet.size
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center pt-[8%]"
  style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)"
  on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-xl h-[70vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-overlay flex flex-col overflow-hidden">
    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <div class="flex items-center gap-2">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-sapphire)" stroke-width="1.5" stroke-linecap="round">
          <path d="M6 4H4a2 2 0 0 0 0 4h2M10 4h2a2 2 0 0 1 0 4h-2M5 8h6" />
        </svg>
        <span class="text-sm font-semibold text-ctp-text">Auto-Link Suggestions</span>
        <span class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded">
          {suggestions.length} found
        </span>
      </div>
      <div class="flex items-center gap-2">
        {#if suggestions.length > 0 && pendingCount > 0}
          <button on:click={linkAll}
            class="text-[13px] font-medium bg-ctp-green/90 text-ctp-crust px-3 py-1 rounded-md hover:bg-ctp-green transition-colors">
            Link All ({pendingCount})
          </button>
        {/if}
        <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1 transition-colors"
          on:click={() => dispatch('close')}>esc</kbd>
      </div>
    </div>

    <!-- Content -->
    <div class="flex-1 overflow-y-auto py-2">
      {#if loading}
        <div class="flex flex-col items-center py-16 gap-2">
          <span class="text-ctp-overlay1 text-sm animate-pulse">Scanning for unlinked mentions...</span>
        </div>
      {:else if error}
        <div class="flex flex-col items-center py-16 gap-2">
          <span class="text-ctp-red text-sm">{error}</span>
        </div>
      {:else if suggestions.length === 0}
        <div class="flex flex-col items-center py-16 gap-2">
          <svg width="24" height="24" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-green)" stroke-width="1.5" stroke-linecap="round" class="opacity-50">
            <path d="M3 8l3 3 7-7" />
          </svg>
          <span class="text-ctp-overlay1 text-sm">No unlinked mentions found</span>
          <span class="text-ctp-overlay1 text-[12px]">All note references are already linked</span>
        </div>
      {:else}
        {#each suggestions as suggestion, i}
          {@const isLinked = linkedSet.has(i)}
          <div class="px-4 py-3 hover:bg-ctp-surface0/40 transition-colors border-b border-ctp-surface0/30"
            class:opacity-50={isLinked}>
            <div class="flex items-center justify-between mb-1.5">
              <div class="flex items-center gap-2">
                {#if isLinked}
                  <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-green)" stroke-width="2" stroke-linecap="round">
                    <path d="M3 8l3 3 7-7" />
                  </svg>
                {:else}
                  <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-blue)" stroke-width="1.5" stroke-linecap="round">
                    <path d="M6 4H4a2 2 0 0 0 0 4h2M10 4h2a2 2 0 0 1 0 4h-2M5 8h6" />
                  </svg>
                {/if}
                <span class="text-sm font-semibold text-ctp-blue">{suggestion.target}</span>
                <span class="text-[12px] text-ctp-overlay1">L{suggestion.line}</span>
                <span class="text-[12px] text-ctp-lavender font-mono">[[{suggestion.target}]]</span>
              </div>
              {#if !isLinked}
                <button on:click={() => linkOne(i)}
                  class="text-[12px] font-medium bg-ctp-blue/80 text-ctp-crust px-2.5 py-0.5 rounded hover:bg-ctp-blue transition-colors">
                  Link it
                </button>
              {:else}
                <span class="text-[12px] text-ctp-green font-medium">Linked</span>
              {/if}
            </div>
            <div class="text-[13px] text-ctp-subtext0 pl-5 truncate">{suggestion.context}</div>
          </div>
        {/each}
      {/if}
    </div>

    <!-- Footer -->
    <div class="flex items-center gap-3 px-4 py-2 border-t border-ctp-surface0 text-[12px] text-ctp-overlay1">
      <span>{linkedSet.size} linked</span>
      <span class="text-ctp-surface1">&middot;</span>
      <span>{pendingCount} remaining</span>
      <span class="text-ctp-surface1">&middot;</span>
      <span>click to link</span>
    </div>
  </div>
</div>
