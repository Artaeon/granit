<script lang="ts">
  import { createEventDispatcher, onMount, tick } from 'svelte'
  import { parseMarkdownTable } from './api'

  const dispatch = createEventDispatcher()
  export let initialContent: string = ''
  export let initialLine: number = -1

  let headers: string[] = ['Column 1', 'Column 2', 'Column 3']
  let rows: string[][] = [['', '', ''], ['', '', '']]
  let alignments: ('left' | 'center' | 'right')[] = ['left', 'left', 'left']
  let startLine = -1
  let endLine = -1

  let curRow = -1 // -1 = header
  let curCol = 0
  let editing = false
  let editValue = ''
  let editInputEl: HTMLInputElement

  let error = ''

  onMount(async () => {
    if (initialContent && initialLine >= 0) {
      await parseTable()
    }
  })

  async function parseTable() {
    try {
      const result = await parseMarkdownTable(initialContent, initialLine)
      if (result) {
        headers = result.headers || ['Column 1', 'Column 2', 'Column 3']
        rows = result.rows?.length ? result.rows : [new Array(headers.length).fill('')]
        alignments = new Array(headers.length).fill('left')
        startLine = result.startLine
        endLine = result.endLine
      }
    } catch (e: any) {
      // If parse fails, use defaults (new table).
    }
  }

  function getCellValue(row: number, col: number): string {
    if (row === -1) return headers[col] || ''
    if (row >= 0 && row < rows.length && col < (rows[row]?.length || 0)) {
      return rows[row][col]
    }
    return ''
  }

  function setCellValue(row: number, col: number, value: string) {
    if (row === -1) {
      headers[col] = value
      headers = [...headers]
    } else if (row >= 0 && row < rows.length) {
      while (rows[row].length <= col) rows[row].push('')
      rows[row][col] = value
      rows = [...rows]
    }
  }

  function startEdit(row: number, col: number) {
    curRow = row
    curCol = col
    editing = true
    editValue = getCellValue(row, col)
    tick().then(() => editInputEl?.focus())
  }

  function confirmEdit() {
    setCellValue(curRow, curCol, editValue)
    editing = false
    editValue = ''
  }

  function cancelEdit() {
    editing = false
    editValue = ''
  }

  function handleCellKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault()
      confirmEdit()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      cancelEdit()
    } else if (e.key === 'Tab') {
      e.preventDefault()
      confirmEdit()
      // Move to next cell.
      let nextCol = curCol + 1
      let nextRow = curRow
      if (nextCol >= headers.length) {
        nextCol = 0
        nextRow++
      }
      if (nextRow < rows.length) {
        startEdit(nextRow, nextCol)
      }
    }
  }

  function addRow() {
    const newRow = new Array(headers.length).fill('')
    const insertIdx = curRow >= 0 ? curRow + 1 : rows.length
    rows = [...rows.slice(0, insertIdx), newRow, ...rows.slice(insertIdx)]
    curRow = insertIdx
  }

  function addColumn() {
    const insertCol = curCol + 1
    headers = [...headers.slice(0, insertCol), '', ...headers.slice(insertCol)]
    alignments = [...alignments.slice(0, insertCol), 'left' as const, ...alignments.slice(insertCol)]
    rows = rows.map(row => {
      const normalized = [...row]
      while (normalized.length < headers.length - 1) normalized.push('')
      return [...normalized.slice(0, insertCol), '', ...normalized.slice(insertCol)]
    })
    curCol = insertCol
  }

  function deleteRow() {
    if (curRow >= 0 && rows.length > 0) {
      rows = rows.filter((_, i) => i !== curRow)
      if (curRow >= rows.length) curRow = rows.length - 1
    }
  }

  function deleteColumn() {
    if (headers.length <= 1) return
    const col = curCol
    headers = headers.filter((_, i) => i !== col)
    alignments = alignments.filter((_, i) => i !== col)
    rows = rows.map(row => row.filter((_, i) => i !== col))
    if (curCol >= headers.length) curCol = headers.length - 1
  }

  function setAlignment(col: number, align: 'left' | 'center' | 'right') {
    alignments[col] = align
    alignments = [...alignments]
  }

  function cycleAlignment(col: number) {
    const cycle: ('left' | 'center' | 'right')[] = ['left', 'center', 'right']
    const current = cycle.indexOf(alignments[col] || 'left')
    alignments[col] = cycle[(current + 1) % 3]
    alignments = [...alignments]
  }

  function generateMarkdown(): string {
    const numCols = headers.length
    // Compute column widths.
    const colWidths = headers.map((h, i) => {
      let maxW = Math.max(h.length, 3)
      for (const row of rows) {
        if (row[i] && row[i].length > maxW) maxW = row[i].length
      }
      return maxW
    })

    let md = '|'
    for (let i = 0; i < numCols; i++) {
      md += ' ' + padCell(headers[i], colWidths[i], alignments[i]) + ' |'
    }
    md += '\n|'
    for (let i = 0; i < numCols; i++) {
      const a = alignments[i]
      if (a === 'center') {
        md += ' :' + '-'.repeat(colWidths[i] - 2) + ': |'
      } else if (a === 'right') {
        md += ' ' + '-'.repeat(colWidths[i] - 1) + ': |'
      } else {
        md += ' :' + '-'.repeat(colWidths[i] - 1) + ' |'
      }
    }
    for (const row of rows) {
      md += '\n|'
      for (let i = 0; i < numCols; i++) {
        const cell = row[i] || ''
        md += ' ' + padCell(cell, colWidths[i], alignments[i]) + ' |'
      }
    }
    return md
  }

  function padCell(s: string, width: number, align: string): string {
    if (s.length >= width) return s.slice(0, width)
    const pad = width - s.length
    if (align === 'center') {
      const left = Math.floor(pad / 2)
      const right = pad - left
      return ' '.repeat(left) + s + ' '.repeat(right)
    }
    if (align === 'right') {
      return ' '.repeat(pad) + s
    }
    return s + ' '.repeat(pad)
  }

  function copyMarkdown() {
    const md = generateMarkdown()
    navigator.clipboard.writeText(md)
    dispatch('copy', md)
  }

  function insertTable() {
    const md = generateMarkdown()
    dispatch('insert', { markdown: md, startLine, endLine })
  }

  function handleGlobalKeydown(e: KeyboardEvent) {
    if (editing) return
    if (e.key === 'Escape') {
      dispatch('close')
    }
  }
</script>

<svelte:window on:keydown={handleGlobalKeydown} />

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center items-start pt-[6%]"
  style="background: rgba(17,17,27,0.6); backdrop-filter: blur(8px);"
  on:click|self={() => dispatch('close')}>

  <div class="w-full max-w-3xl max-h-[85vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden">

    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <div class="flex items-center gap-2">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-mauve)" stroke-width="1.5" stroke-linecap="round">
          <path d="M2 3h12v10H2V3zM2 6h12M2 9h12M6 3v10M10 3v10" />
        </svg>
        <span class="text-sm font-semibold text-ctp-text">Table Editor</span>
        <span class="text-[12px] px-1.5 py-0.5 rounded bg-ctp-surface0 text-ctp-overlay1">
          {headers.length} cols &times; {rows.length} rows
        </span>
      </div>
      <div class="flex items-center gap-2">
        <button on:click={copyMarkdown}
          class="text-[12px] text-ctp-overlay1 hover:text-ctp-blue px-2 py-0.5 rounded bg-ctp-surface0 transition-colors">
          Copy MD
        </button>
        <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer" on:click={() => dispatch('close')}>esc</kbd>
      </div>
    </div>

    {#if error}
      <div class="px-4 py-2 text-xs text-ctp-red bg-ctp-surface0">{error}</div>
    {/if}

    <!-- Table grid -->
    <div class="flex-1 overflow-auto px-4 py-3">
      <table class="w-full border-collapse">
        <!-- Alignment controls row -->
        <thead>
          <tr>
            <th class="w-8"></th>
            {#each headers as _, colIdx}
              <th class="px-1 pb-1">
                <div class="flex items-center justify-center gap-1">
                  <button
                    on:click={() => setAlignment(colIdx, 'left')}
                    class="w-5 h-5 rounded text-[11px] transition-colors
                      {alignments[colIdx] === 'left' ? 'bg-ctp-blue/20 text-ctp-blue' : 'text-ctp-overlay1 hover:text-ctp-subtext0 bg-ctp-surface0'}">
                    L
                  </button>
                  <button
                    on:click={() => setAlignment(colIdx, 'center')}
                    class="w-5 h-5 rounded text-[11px] transition-colors
                      {alignments[colIdx] === 'center' ? 'bg-ctp-blue/20 text-ctp-blue' : 'text-ctp-overlay1 hover:text-ctp-subtext0 bg-ctp-surface0'}">
                    C
                  </button>
                  <button
                    on:click={() => setAlignment(colIdx, 'right')}
                    class="w-5 h-5 rounded text-[11px] transition-colors
                      {alignments[colIdx] === 'right' ? 'bg-ctp-blue/20 text-ctp-blue' : 'text-ctp-overlay1 hover:text-ctp-subtext0 bg-ctp-surface0'}">
                    R
                  </button>
                </div>
              </th>
            {/each}
            <th class="w-8"></th>
          </tr>

          <!-- Header row -->
          <tr>
            <th class="w-8 text-[11px] text-ctp-overlay1 font-normal">H</th>
            {#each headers as header, colIdx}
              <th
                class="border border-ctp-surface1 px-0 py-0 relative
                  {curRow === -1 && curCol === colIdx ? 'ring-2 ring-ctp-blue ring-inset' : ''}"
                on:click={() => { if (!editing) startEdit(-1, colIdx) }}
              >
                {#if editing && curRow === -1 && curCol === colIdx}
                  <input
                    bind:this={editInputEl}
                    bind:value={editValue}
                    on:keydown={handleCellKeydown}
                    on:blur={confirmEdit}
                    class="w-full h-full px-2 py-1.5 bg-ctp-surface0 text-ctp-text text-xs font-semibold outline-none"
                  />
                {:else}
                  <div class="px-2 py-1.5 text-xs font-semibold text-ctp-mauve cursor-pointer hover:bg-ctp-surface0/50 min-h-[28px]
                    {alignments[colIdx] === 'center' ? 'text-center' : alignments[colIdx] === 'right' ? 'text-right' : 'text-left'}">
                    {header || '\u00A0'}
                  </div>
                {/if}
              </th>
            {/each}
            <th class="w-8"></th>
          </tr>
        </thead>

        <!-- Data rows -->
        <tbody>
          {#each rows as row, rowIdx}
            <tr>
              <td class="w-8 text-[11px] text-ctp-overlay1 text-center">{rowIdx + 1}</td>
              {#each headers as _, colIdx}
                <td
                  class="border border-ctp-surface1 px-0 py-0 relative
                    {curRow === rowIdx && curCol === colIdx ? 'ring-2 ring-ctp-blue ring-inset' : ''}"
                  on:click={() => { if (!editing) startEdit(rowIdx, colIdx) }}
                >
                  {#if editing && curRow === rowIdx && curCol === colIdx}
                    <input
                      bind:this={editInputEl}
                      bind:value={editValue}
                      on:keydown={handleCellKeydown}
                      on:blur={confirmEdit}
                      class="w-full h-full px-2 py-1.5 bg-ctp-surface0 text-ctp-text text-xs outline-none"
                    />
                  {:else}
                    <div class="px-2 py-1.5 text-xs text-ctp-text cursor-pointer hover:bg-ctp-surface0/30 min-h-[28px]
                      {alignments[colIdx] === 'center' ? 'text-center' : alignments[colIdx] === 'right' ? 'text-right' : 'text-left'}">
                      {row[colIdx] || '\u00A0'}
                    </div>
                  {/if}
                </td>
              {/each}
              <td class="w-8">
                <button on:click={() => { curRow = rowIdx; deleteRow() }}
                  class="w-5 h-5 rounded text-[12px] text-ctp-overlay1 hover:text-ctp-red hover:bg-ctp-surface0 transition-colors mx-auto block opacity-0 group-hover:opacity-100"
                  style="opacity: 0.3;"
                  on:mouseenter={(e) => { e.currentTarget.style.opacity = '1' }}
                  on:mouseleave={(e) => { e.currentTarget.style.opacity = '0.3' }}>
                  &times;
                </button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <!-- Actions bar -->
    <div class="flex items-center justify-between px-4 py-3 border-t border-ctp-surface0">
      <div class="flex items-center gap-2">
        <button on:click={addRow}
          class="px-3 py-1.5 bg-ctp-surface0 text-ctp-text rounded-lg text-xs hover:bg-ctp-surface1 transition-colors flex items-center gap-1.5">
          <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
            <path d="M8 3v10M3 8h10" />
          </svg>
          Row
        </button>
        <button on:click={addColumn}
          class="px-3 py-1.5 bg-ctp-surface0 text-ctp-text rounded-lg text-xs hover:bg-ctp-surface1 transition-colors flex items-center gap-1.5">
          <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
            <path d="M8 3v10M3 8h10" />
          </svg>
          Column
        </button>
        <span class="text-ctp-surface1">|</span>
        <button on:click={deleteRow} disabled={rows.length === 0 || curRow < 0}
          class="px-3 py-1.5 bg-ctp-surface0 text-ctp-text rounded-lg text-xs hover:bg-ctp-surface1 transition-colors disabled:opacity-30 disabled:cursor-not-allowed">
          - Row
        </button>
        <button on:click={deleteColumn} disabled={headers.length <= 1}
          class="px-3 py-1.5 bg-ctp-surface0 text-ctp-text rounded-lg text-xs hover:bg-ctp-surface1 transition-colors disabled:opacity-30 disabled:cursor-not-allowed">
          - Col
        </button>
      </div>
      <div class="flex items-center gap-2">
        <button on:click={() => dispatch('close')}
          class="px-3 py-1.5 bg-ctp-surface0 text-ctp-text rounded-lg text-xs hover:bg-ctp-surface1 transition-colors">
          Cancel
        </button>
        <button on:click={insertTable}
          class="px-4 py-1.5 bg-ctp-blue text-ctp-crust rounded-lg text-xs font-medium hover:opacity-90 transition-opacity">
          Insert Table
        </button>
      </div>
    </div>

    <!-- Markdown preview -->
    <div class="border-t border-ctp-surface0 px-4 py-2">
      <details>
        <summary class="text-[12px] text-ctp-overlay1 cursor-pointer select-none hover:text-ctp-subtext0">
          Preview Markdown
        </summary>
        <pre class="mt-2 p-3 bg-ctp-crust rounded-lg text-[13px] text-ctp-text font-mono overflow-x-auto whitespace-pre">{generateMarkdown()}</pre>
      </details>
    </div>
  </div>
</div>
