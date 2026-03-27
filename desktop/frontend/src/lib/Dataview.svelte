<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'
  import { runDataviewQuery } from './api'
  const dispatch = createEventDispatcher()

  let query = ''
  let results: Record<string, any>[] = []
  let columns: string[] = []
  let loading = false
  let error = ''
  let sortCol = ''
  let sortDesc = false
  let showExamples = true
  const exampleQueries = [
    { label: 'All notes sorted by date', query: 'SORT date DESC' },
    { label: 'Notes in projects folder', query: 'FROM "projects" SORT title' },
    { label: 'Notes tagged "meeting"', query: 'WHERE tags CONTAINS "meeting" SORT date DESC' },
    { label: 'Recent notes (limit 10)', query: 'SORT date DESC LIMIT 10' },
    { label: 'Notes with status "active"', query: 'FROM "projects" WHERE status = "active"' },
    { label: 'Large notes (by word count)', query: 'SORT words DESC LIMIT 20' },
  ]

  async function runQuery() {
    if (!query.trim()) return
    loading = true
    error = ''
    showExamples = false
    try {
      const raw = await runDataviewQuery(query)
      results = raw || []
      if (results.length > 0) {
        // Collect all unique column keys
        const colSet = new Set<string>()
        colSet.add('title')
        colSet.add('path')
        for (const row of results) {
          for (const key of Object.keys(row)) {
            colSet.add(key)
          }
        }
        // Order: title, path, date, tags, then rest
        const priority = ['title', 'path', 'date', 'tags']
        columns = [...priority.filter(c => colSet.has(c)), ...[...colSet].filter(c => !priority.includes(c)).sort()]
      } else {
        columns = []
      }
      sortCol = ''
      sortDesc = false
    } catch (e: any) {
      error = e.message || 'Query failed'
      results = []
      columns = []
    }
    loading = false
  }

  function sortByColumn(col: string) {
    if (sortCol === col) {
      sortDesc = !sortDesc
    } else {
      sortCol = col
      sortDesc = false
    }
    results = [...results].sort((a, b) => {
      const va = String(a[col] ?? '')
      const vb = String(b[col] ?? '')
      return sortDesc ? vb.localeCompare(va) : va.localeCompare(vb)
    })
  }

  function useExample(q: string) {
    query = q
    runQuery()
  }

  function openNote(path: string) {
    dispatch('open-note', path)
  }

  function exportCSV() {
    if (results.length === 0) return
    const header = columns.join(',')
    const rows = results.map(r => columns.map(c => {
      const val = String(r[c] ?? '').replace(/"/g, '""')
      return `"${val}"`
    }).join(','))
    const csv = [header, ...rows].join('\n')
    const blob = new Blob([csv], { type: 'text/csv' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'dataview-results.csv'
    a.click()
    URL.revokeObjectURL(url)
  }

  function formatCellValue(val: any): string {
    if (val === null || val === undefined) return ''
    if (Array.isArray(val)) return val.join(', ')
    return String(val)
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      runQuery()
    }
  }
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center pt-[4%]" style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-4xl h-[85vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden">
    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <div class="flex items-center gap-2">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-blue)" stroke-width="1.5" stroke-linecap="round">
          <rect x="2" y="2" width="12" height="12" rx="1" />
          <path d="M2 6h12M6 6v8M10 6v8" />
        </svg>
        <span class="text-sm font-semibold text-ctp-text">Dataview Query</span>
      </div>
      <div class="flex items-center gap-2">
        {#if results.length > 0}
          <button on:click={exportCSV}
            class="text-[12px] font-medium bg-ctp-surface0 text-ctp-subtext0 px-2.5 py-1 rounded hover:bg-ctp-surface1 transition-colors">
            Export CSV
          </button>
        {/if}
        <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1 transition-colors"
          on:click={() => dispatch('close')}>esc</kbd>
      </div>
    </div>

    <!-- Query input -->
    <div class="px-4 py-3 border-b border-ctp-surface0 bg-ctp-base/30">
      <div class="flex gap-2">
        <textarea bind:value={query}
          on:keydown={handleKeydown}
          placeholder="FROM &quot;folder&quot; WHERE field = &quot;value&quot; SORT field DESC LIMIT 10"
          rows="2"
          class="flex-1 px-3 py-2 text-[13px] font-mono bg-ctp-surface0/60 text-ctp-text rounded-lg border border-ctp-surface1 outline-none resize-none focus:border-ctp-blue transition-colors placeholder:text-ctp-surface2/50"></textarea>
        <button on:click={runQuery}
          disabled={loading || !query.trim()}
          class="self-end px-4 py-2 text-[13px] font-semibold bg-ctp-blue text-ctp-crust rounded-lg hover:opacity-90 transition-opacity disabled:opacity-40 shrink-0">
          {loading ? 'Running...' : 'Run Query'}
        </button>
      </div>
      <div class="flex items-center gap-3 mt-1.5">
        <span class="text-[12px] text-ctp-overlay1">Ctrl+Enter to run</span>
        {#if results.length > 0}
          <span class="text-[12px] text-ctp-green">{results.length} result{results.length === 1 ? '' : 's'}</span>
        {/if}
      </div>
    </div>

    <!-- Error -->
    {#if error}
      <div class="px-4 py-2 text-[13px] text-ctp-red border-b border-ctp-surface0 bg-ctp-red/5">
        {error}
      </div>
    {/if}

    <!-- Content -->
    <div class="flex-1 overflow-auto">
      {#if showExamples && results.length === 0 && !loading}
        <!-- Example queries -->
        <div class="p-4">
          <h3 class="text-[13px] font-semibold text-ctp-subtext0 uppercase tracking-wider mb-3">Example Queries</h3>
          <div class="grid gap-2">
            {#each exampleQueries as ex}
              <button on:click={() => useExample(ex.query)}
                class="text-left px-3 py-2.5 bg-ctp-base rounded-lg border border-ctp-surface0 hover:border-ctp-surface1 transition-colors group">
                <div class="text-[13px] text-ctp-text group-hover:text-ctp-blue transition-colors">{ex.label}</div>
                <div class="text-[12px] font-mono text-ctp-overlay1 mt-0.5">{ex.query}</div>
              </button>
            {/each}
          </div>
          <div class="mt-4 text-[12px] text-ctp-overlay1 space-y-1">
            <p class="font-semibold text-ctp-subtext0">Query Syntax:</p>
            <p><span class="font-mono text-ctp-blue">FROM</span> "folder" or #tag - filter by source</p>
            <p><span class="font-mono text-ctp-blue">WHERE</span> field = "value" - filter by frontmatter (=, !=, CONTAINS, &gt;, &lt;)</p>
            <p><span class="font-mono text-ctp-blue">SORT</span> field [ASC|DESC] - sort results</p>
            <p><span class="font-mono text-ctp-blue">LIMIT</span> n - limit number of results</p>
            <p class="mt-1">Fields: title, path, date, tags, words, folder, or any frontmatter key</p>
          </div>
        </div>
      {:else if loading}
        <div class="flex items-center justify-center py-16">
          <span class="text-ctp-overlay1 text-sm">Querying vault...</span>
        </div>
      {:else if results.length === 0 && query.trim()}
        <div class="flex flex-col items-center py-16 gap-2">
          <p class="text-ctp-overlay1 text-sm">No results found</p>
          <button on:click={() => { showExamples = true }}
            class="text-[13px] text-ctp-blue hover:underline">Show examples</button>
        </div>
      {:else if results.length > 0}
        <!-- Results table -->
        <table class="w-full text-[13px]">
          <thead class="sticky top-0 bg-ctp-mantle border-b border-ctp-surface0">
            <tr>
              {#each columns as col}
                <th class="text-left px-3 py-2 text-[12px] font-semibold text-ctp-subtext0 uppercase tracking-wider cursor-pointer hover:text-ctp-text transition-colors select-none"
                  on:click={() => sortByColumn(col)}>
                  <span class="flex items-center gap-1">
                    {col}
                    {#if sortCol === col}
                      <span class="text-ctp-blue">{sortDesc ? '\u2193' : '\u2191'}</span>
                    {/if}
                  </span>
                </th>
              {/each}
            </tr>
          </thead>
          <tbody>
            {#each results as row, i}
              <tr class="border-b border-ctp-surface0/30 hover:bg-ctp-surface0/30 transition-colors cursor-pointer"
                on:click={() => openNote(row.path)}>
                {#each columns as col}
                  <td class="px-3 py-2 {col === 'title' ? 'text-ctp-blue font-medium' : 'text-ctp-text'} max-w-[200px] truncate">
                    {formatCellValue(row[col])}
                  </td>
                {/each}
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </div>
  </div>
</div>
