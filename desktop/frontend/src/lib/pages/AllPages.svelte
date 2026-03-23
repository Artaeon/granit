<script lang="ts">
  import { notes, navigateToPage } from '../stores'
  import type { NoteInfo } from '../types'

  let search = ''
  let sortBy: 'title' | 'modified' | 'size' = 'modified'
  let sortAsc = false

  $: filteredNotes = $notes
    .filter(n => !search || n.title.toLowerCase().includes(search.toLowerCase()))
    .sort((a, b) => {
      let cmp = 0
      if (sortBy === 'title') cmp = a.title.localeCompare(b.title)
      else if (sortBy === 'modified') cmp = a.modTime.localeCompare(b.modTime)
      else if (sortBy === 'size') cmp = a.size - b.size
      return sortAsc ? cmp : -cmp
    })

  function toggleSort(col: 'title' | 'modified' | 'size') {
    if (sortBy === col) sortAsc = !sortAsc
    else { sortBy = col; sortAsc = col === 'title' }
  }

  function formatDate(iso: string): string {
    try {
      const d = new Date(iso)
      return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
    } catch { return iso }
  }

  function formatSize(bytes: number): string {
    if (bytes < 1024) return bytes + ' B'
    return (bytes / 1024).toFixed(1) + ' KB'
  }

  function sortIndicator(col: 'title' | 'modified' | 'size'): string {
    if (sortBy !== col) return ''
    return sortAsc ? ' \u2191' : ' \u2193'
  }
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="h-full overflow-y-auto bg-ctp-base">
  <div class="max-w-[780px] mx-auto px-8 py-8">
    <div class="flex items-center justify-between mb-6">
      <h1 class="text-xl font-bold text-ctp-text">All Pages</h1>
      <span class="text-[13px] text-ctp-overlay0">{filteredNotes.length} pages</span>
    </div>

    <!-- Search -->
    <div class="mb-4">
      <div class="flex items-center gap-2 px-3 py-2 bg-ctp-mantle rounded-lg border border-ctp-surface0/25">
        <svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-overlay0)" stroke-width="1.5" stroke-linecap="round">
          <circle cx="7" cy="7" r="4" /><path d="M10 10l3.5 3.5" />
        </svg>
        <input bind:value={search} placeholder="Filter pages..."
          class="bg-transparent text-[14px] text-ctp-text outline-none w-full placeholder:text-ctp-overlay0 border-none" />
      </div>
    </div>

    <!-- Table -->
    <div class="rounded-lg border border-ctp-surface0/25 overflow-hidden">
      <!-- Header -->
      <div class="flex items-center bg-ctp-mantle text-[12px] font-medium text-ctp-overlay0 border-b border-ctp-surface0/25">
        <button on:click={() => toggleSort('title')} class="flex-1 px-4 py-2 text-left hover:text-ctp-text transition-colors">
          Title{sortIndicator('title')}
        </button>
        <button on:click={() => toggleSort('modified')} class="w-32 px-4 py-2 text-left hover:text-ctp-text transition-colors">
          Modified{sortIndicator('modified')}
        </button>
        <button on:click={() => toggleSort('size')} class="w-20 px-4 py-2 text-right hover:text-ctp-text transition-colors">
          Size{sortIndicator('size')}
        </button>
      </div>

      <!-- Rows -->
      {#each filteredNotes as note}
        <button on:click={() => navigateToPage(note.relPath)}
          class="flex items-center w-full text-left hover:bg-ctp-surface0/30 transition-colors group">
          <div class="flex-1 px-4 py-2">
            <span class="text-[14px] text-ctp-text group-hover:text-ctp-blue transition-colors">{note.title}</span>
            {#if note.relPath.includes('/')}
              <span class="text-[11px] text-ctp-overlay0 ml-2">{note.relPath.split('/').slice(0, -1).join('/')}</span>
            {/if}
          </div>
          <div class="w-32 px-4 py-2 text-[12px] text-ctp-overlay0">
            {formatDate(note.modTime)}
          </div>
          <div class="w-20 px-4 py-2 text-[12px] text-ctp-overlay0 text-right">
            {formatSize(note.size)}
          </div>
        </button>
      {/each}

      {#if filteredNotes.length === 0}
        <div class="px-4 py-8 text-center text-[13px] text-ctp-overlay0">
          {search ? 'No pages match your search' : 'No pages in vault'}
        </div>
      {/if}
    </div>
  </div>
</div>
