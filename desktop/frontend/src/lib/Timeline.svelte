<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'
  const dispatch = createEventDispatcher()
  const api = () => (window as any).go?.main?.GranitApp

  interface TimelineEntry {
    date: string
    title: string
    relPath: string
    tags: string[]
    wordCount: number
  }

  interface MonthGroup {
    label: string
    entries: TimelineEntry[]
  }

  let entries: TimelineEntry[] = []
  let loading = true
  let searchQuery = ''
  let tagFilter = ''
  let selectedIdx = -1

  // All unique tags
  let allTags: string[] = []

  $: filtered = filterEntries(entries, searchQuery, tagFilter)
  $: groups = groupByMonth(filtered)
  $: {
    const tagSet = new Set<string>()
    entries.forEach(e => e.tags?.forEach(t => tagSet.add(t)))
    allTags = [...tagSet].sort()
  }

  onMount(async () => {
    await loadTimeline()
  })

  async function loadTimeline() {
    loading = true
    try {
      entries = (await api()?.GetTimeline()) || []
    } catch (e) {
      console.error('Failed to load timeline:', e)
    }
    loading = false
  }

  function filterEntries(all: TimelineEntry[], query: string, tag: string): TimelineEntry[] {
    let result = all
    if (tag) {
      result = result.filter(e => e.tags?.includes(tag))
    }
    if (query) {
      const q = query.toLowerCase()
      result = result.filter(e =>
        e.title.toLowerCase().includes(q) ||
        e.relPath.toLowerCase().includes(q) ||
        e.tags?.some(t => t.toLowerCase().includes(q))
      )
    }
    return result
  }

  function groupByMonth(items: TimelineEntry[]): MonthGroup[] {
    const groupMap = new Map<string, TimelineEntry[]>()
    for (const entry of items) {
      const d = new Date(entry.date)
      const label = d.toLocaleDateString('en-US', { year: 'numeric', month: 'long' })
      if (!groupMap.has(label)) groupMap.set(label, [])
      groupMap.get(label)!.push(entry)
    }
    return Array.from(groupMap.entries()).map(([label, entries]) => ({ label, entries }))
  }

  function formatDate(dateStr: string): string {
    const d = new Date(dateStr)
    return d.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' })
  }

  function formatTime(dateStr: string): string {
    const d = new Date(dateStr)
    return d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' })
  }

  function fmtWordCount(n: number): string {
    if (n >= 1000) return (n / 1000).toFixed(1) + 'k'
    return String(n)
  }

  function tagColor(tag: string): string {
    // Deterministic color from tag name
    const colors = ['ctp-blue', 'ctp-mauve', 'ctp-teal', 'ctp-green', 'ctp-peach', 'ctp-pink', 'ctp-sapphire', 'ctp-lavender']
    let hash = 0
    for (let i = 0; i < tag.length; i++) hash = ((hash << 5) - hash + tag.charCodeAt(i)) | 0
    return colors[Math.abs(hash) % colors.length]
  }

  function openNote(relPath: string) {
    dispatch('select', relPath)
  }

  function clearFilters() {
    searchQuery = ''
    tagFilter = ''
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      dispatch('close')
    }
  }
</script>

<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center pt-[5%]" style="background:rgba(0,0,0,0.55);backdrop-filter:blur(3px)"
  on:click|self={() => dispatch('close')} on:keydown={handleKeydown}>
  <div class="w-full max-w-2xl h-[85vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden">

    <!-- Header -->
    <div class="flex items-center justify-between px-5 py-3 border-b border-ctp-surface0">
      <div class="flex items-center gap-3">
        <span class="text-sm font-semibold text-ctp-mauve">Timeline</span>
        <span class="text-[12px] text-ctp-overlay1">{filtered.length} notes</span>
      </div>
      <!-- svelte-ignore a11y-click-events-have-key-events -->
      <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer" on:click={() => dispatch('close')}>esc</kbd>
    </div>

    <!-- Search & Filter Bar -->
    <div class="px-5 py-3 border-b border-ctp-surface0 space-y-2">
      <div class="flex gap-2">
        <input type="text" bind:value={searchQuery}
          placeholder="Search timeline..."
          class="flex-1 bg-ctp-base border border-ctp-surface0 rounded-lg px-3 py-1.5 text-sm text-ctp-text placeholder:text-ctp-surface2 focus:outline-none focus:border-ctp-mauve/50" />
        {#if searchQuery || tagFilter}
          <!-- svelte-ignore a11y-click-events-have-key-events -->
          <button class="px-3 py-1.5 bg-ctp-surface0 rounded-lg text-[12px] text-ctp-overlay1 hover:text-ctp-text transition-colors"
            on:click={clearFilters}>Clear</button>
        {/if}
      </div>

      <!-- Tag filter pills -->
      {#if allTags.length > 0}
        <div class="flex flex-wrap gap-1.5 max-h-16 overflow-y-auto">
          {#each allTags.slice(0, 20) as tag}
            <!-- svelte-ignore a11y-click-events-have-key-events -->
            <span class="text-[12px] px-2 py-0.5 rounded-full cursor-pointer transition-colors
              {tagFilter === tag
                ? 'bg-ctp-mauve/30 text-ctp-mauve border border-ctp-mauve/50'
                : 'bg-ctp-surface0 text-ctp-overlay1 border border-transparent hover:text-ctp-text'}"
              on:click={() => { tagFilter = tagFilter === tag ? '' : tag }}>
              #{tag}
            </span>
          {/each}
        </div>
      {/if}
    </div>

    <!-- Timeline Body -->
    <div class="flex-1 overflow-y-auto">
      {#if loading}
        <div class="flex items-center justify-center p-12">
          <span class="text-ctp-overlay1 text-sm">Loading timeline...</span>
        </div>
      {:else if filtered.length === 0}
        <div class="flex items-center justify-center p-12">
          <span class="text-ctp-overlay1 text-sm">
            {entries.length === 0 ? 'No notes in vault' : 'No matches found'}
          </span>
        </div>
      {:else}
        <div class="relative px-5 py-4">
          <!-- Vertical timeline line -->
          <div class="absolute left-[2.15rem] top-0 bottom-0 w-px bg-ctp-surface1"></div>

          {#each groups as group}
            <!-- Month header -->
            <div class="relative flex items-center gap-3 mb-3 mt-2 first:mt-0">
              <div class="w-4 h-4 rounded-full bg-ctp-mauve z-10 shrink-0 ml-[0.2rem]"></div>
              <span class="text-xs font-semibold text-ctp-mauve uppercase tracking-wider">{group.label}</span>
              <div class="flex-1 h-px bg-ctp-surface1"></div>
              <span class="text-[12px] text-ctp-overlay1">{group.entries.length} notes</span>
            </div>

            <!-- Entries -->
            {#each group.entries as entry, i}
              <!-- svelte-ignore a11y-click-events-have-key-events -->
              <div class="relative flex gap-3 pl-1 mb-1 group cursor-pointer"
                on:click={() => openNote(entry.relPath)}>
                <!-- Timeline dot -->
                <div class="w-2.5 h-2.5 rounded-full bg-ctp-surface1 group-hover:bg-ctp-blue z-10 shrink-0 mt-2 ml-[0.5rem] transition-colors"></div>

                <!-- Entry card -->
                <div class="flex-1 bg-ctp-base rounded-lg p-3 border border-transparent group-hover:border-ctp-surface1 transition-colors ml-2">
                  <div class="flex items-start justify-between gap-2">
                    <div class="flex-1 min-w-0">
                      <!-- Title -->
                      <div class="text-sm text-ctp-text font-medium truncate group-hover:text-ctp-blue transition-colors">
                        {entry.title || entry.relPath}
                      </div>
                      <!-- Date and word count -->
                      <div class="flex items-center gap-2 mt-0.5">
                        <span class="text-[12px] text-ctp-overlay1">{formatDate(entry.date)}</span>
                        <span class="text-[12px] text-ctp-overlay1">|</span>
                        <span class="text-[12px] text-ctp-overlay1">{fmtWordCount(entry.wordCount)} words</span>
                      </div>
                    </div>
                    <!-- Date badge -->
                    <div class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded shrink-0">
                      {formatTime(entry.date)}
                    </div>
                  </div>
                  <!-- Tags -->
                  {#if entry.tags && entry.tags.length > 0}
                    <div class="flex flex-wrap gap-1 mt-1.5">
                      {#each entry.tags.slice(0, 4) as tag}
                        <span class="text-[11px] text-{tagColor(tag)} bg-ctp-surface0 px-1.5 py-0.5 rounded-full">#{tag}</span>
                      {/each}
                      {#if entry.tags.length > 4}
                        <span class="text-[11px] text-ctp-overlay1">+{entry.tags.length - 4}</span>
                      {/if}
                    </div>
                  {/if}
                </div>
              </div>
            {/each}
          {/each}
        </div>
      {/if}
    </div>

    <!-- Footer -->
    <div class="px-5 py-2 border-t border-ctp-surface0 flex gap-4 text-[12px] text-ctp-overlay1">
      <span>Click entry to open note</span>
      <span class="ml-auto">
        {#if tagFilter}
          Filtered by: <span class="text-ctp-mauve">#{tagFilter}</span>
        {/if}
      </span>
    </div>
  </div>
</div>
