<script lang="ts">
  import { createEventDispatcher } from 'svelte'

  export let notePath: string = ''
  export let noteTitle: string = ''

  const dispatch = createEventDispatcher()
  const api = (window as any).go?.main?.GranitApp

  type Connection = {
    relPath: string
    title: string
    score: number
    reason: string
  }

  let connections: Connection[] = []
  let loading = false
  let error = ''

  async function loadConnections() {
    if (!api || !notePath) return
    loading = true
    error = ''
    try {
      const result = await api.GetSmartConnections(notePath)
      connections = result || []
    } catch (e: any) {
      error = e?.message || 'Failed to find connections'
      connections = []
    }
    loading = false
  }

  $: if (notePath) loadConnections()

  function scoreColor(score: number): string {
    const pct = Math.round(score * 100)
    if (pct >= 70) return 'text-ctp-green'
    if (pct >= 40) return 'text-ctp-yellow'
    return 'text-ctp-peach'
  }

  function scoreBg(score: number): string {
    const pct = Math.round(score * 100)
    if (pct >= 70) return 'bg-ctp-green'
    if (pct >= 40) return 'bg-ctp-yellow'
    return 'bg-ctp-peach'
  }

  function barWidth(score: number): string {
    return `${Math.min(Math.round(score * 100), 100)}%`
  }

  function parseReasons(reason: string): Array<{ type: string; text: string }> {
    const parts = reason.split(' | ')
    return parts.map(p => {
      if (p.startsWith('shared:')) return { type: 'content', text: p }
      if (p.startsWith('tags:')) return { type: 'tags', text: p }
      if (p === 'mutual links') return { type: 'links', text: p }
      return { type: 'content', text: p }
    })
  }

  function reasonIcon(type: string): string {
    switch (type) {
      case 'tags': return '#'
      case 'links': return '~'
      default: return '*'
    }
  }

  function reasonColor(type: string): string {
    switch (type) {
      case 'tags': return 'text-ctp-blue'
      case 'links': return 'text-ctp-lavender'
      default: return 'text-ctp-overlay1'
    }
  }
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center pt-[6%]"
  style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)"
  on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-2xl h-[75vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-overlay flex flex-col overflow-hidden">
    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <div class="flex items-center gap-2">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-mauve)" stroke-width="1.5" stroke-linecap="round">
          <circle cx="8" cy="8" r="3" /><circle cx="3" cy="4" r="1.5" /><circle cx="13" cy="4" r="1.5" />
          <circle cx="3" cy="12" r="1.5" /><circle cx="13" cy="12" r="1.5" />
          <path d="M5.5 5.5l-1-1M10.5 5.5l1-1M5.5 10.5l-1 1M10.5 10.5l1 1" />
        </svg>
        <span class="text-sm font-semibold text-ctp-text">Smart Connections</span>
        <span class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded">
          {connections.length} found
        </span>
      </div>
      <div class="flex items-center gap-2">
        <button on:click={loadConnections}
          class="text-[12px] text-ctp-overlay1 hover:text-ctp-text transition-colors"
          title="Refresh">
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
            <path d="M2 8a6 6 0 0 1 10.5-4M14 8a6 6 0 0 1-10.5 4M2 4v4h4M14 12V8h-4" />
          </svg>
        </button>
        <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1 transition-colors"
          on:click={() => dispatch('close')}>esc</kbd>
      </div>
    </div>

    <!-- Current note info -->
    <div class="px-4 py-2 border-b border-ctp-surface0 bg-ctp-surface0/20">
      <div class="flex items-center gap-2">
        <span class="text-[12px] text-ctp-overlay1">For:</span>
        <span class="text-[13px] text-ctp-blue font-medium truncate">{noteTitle || notePath}</span>
      </div>
    </div>

    <!-- Content -->
    <div class="flex-1 overflow-y-auto">
      {#if loading}
        <div class="flex flex-col items-center py-16 gap-2">
          <div class="flex gap-1">
            <span class="w-2 h-2 rounded-full bg-ctp-mauve animate-bounce" style="animation-delay:0ms"></span>
            <span class="w-2 h-2 rounded-full bg-ctp-mauve animate-bounce" style="animation-delay:150ms"></span>
            <span class="w-2 h-2 rounded-full bg-ctp-mauve animate-bounce" style="animation-delay:300ms"></span>
          </div>
          <span class="text-ctp-overlay1 text-sm">Analyzing vault for connections...</span>
        </div>
      {:else if error}
        <div class="flex flex-col items-center py-16 gap-2">
          <span class="text-ctp-red text-sm">{error}</span>
        </div>
      {:else if connections.length === 0}
        <div class="flex flex-col items-center py-16 gap-2">
          <svg width="24" height="24" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-overlay1)" stroke-width="1.2" stroke-linecap="round" class="opacity-50">
            <circle cx="8" cy="8" r="5" /><path d="M8 5v3l2 2" />
          </svg>
          <span class="text-ctp-overlay1 text-sm">No related notes found</span>
          <span class="text-ctp-overlay1 text-[12px]">Add more content to find connections</span>
        </div>
      {:else}
        {#each connections as conn, i}
          <div class="px-4 py-3 hover:bg-ctp-surface0/40 transition-colors border-b border-ctp-surface0/30">
            <div class="flex items-start justify-between gap-3">
              <!-- Note info -->
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2 mb-1">
                  <span class="text-[12px] font-mono font-semibold {scoreColor(conn.score)} min-w-[32px]">
                    {Math.round(conn.score * 100)}%
                  </span>
                  <button
                    on:click={() => dispatch('open', conn.relPath)}
                    class="text-sm font-medium text-ctp-text hover:text-ctp-blue transition-colors truncate text-left">
                    {conn.title || conn.relPath}
                  </button>
                </div>

                <!-- Score bar -->
                <div class="flex items-center gap-2 pl-10 mb-1.5">
                  <div class="flex-1 h-1.5 rounded-full bg-ctp-surface0 overflow-hidden max-w-[160px]">
                    <div class="h-full rounded-full transition-all {scoreBg(conn.score)}"
                      style="width: {barWidth(conn.score)}"></div>
                  </div>
                </div>

                <!-- Reasons -->
                <div class="flex flex-wrap gap-1.5 pl-10">
                  {#each parseReasons(conn.reason) as r}
                    <span class="text-[12px] {reasonColor(r.type)} bg-ctp-surface0 px-1.5 py-0.5 rounded inline-flex items-center gap-0.5">
                      <span class="font-mono font-bold text-[11px]">{reasonIcon(r.type)}</span>
                      {r.text}
                    </span>
                  {/each}
                </div>
              </div>

              <!-- Actions -->
              <div class="flex flex-col gap-1 flex-shrink-0">
                <button
                  on:click={() => dispatch('open', conn.relPath)}
                  class="text-[12px] font-medium bg-ctp-surface0 text-ctp-text px-2 py-1 rounded hover:bg-ctp-surface1 transition-colors"
                  title="Open note">
                  Open
                </button>
                <button
                  on:click={() => dispatch('link', conn.relPath)}
                  class="text-[12px] font-medium bg-ctp-blue/20 text-ctp-blue px-2 py-1 rounded hover:bg-ctp-blue/30 transition-colors"
                  title="Insert [[wikilink]]">
                  Link
                </button>
              </div>
            </div>

            <!-- Path -->
            <div class="text-[12px] text-ctp-overlay1 pl-10 mt-1 truncate">{conn.relPath}</div>
          </div>
        {/each}
      {/if}
    </div>

    <!-- Footer -->
    <div class="flex items-center gap-3 px-4 py-2 border-t border-ctp-surface0 text-[12px] text-ctp-overlay1">
      <span>{connections.length} connections</span>
      <span class="text-ctp-surface1">&middot;</span>
      <span>ranked by TF-IDF similarity + tags + links</span>
      <span class="text-ctp-surface1">&middot;</span>
      <span>click to open</span>
    </div>
  </div>
</div>
