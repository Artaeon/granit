<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'
  import type { KanbanCard, KanbanColumn } from './types'
  import { getAllTasks, getKanban, saveKanban, toggleTask } from './api'
  const dispatch = createEventDispatcher()
  interface BoardData { columns: KanbanColumn[] }

  let columns: KanbanColumn[] = []
  let loading = true
  let addingToColumn: string | null = null
  let newCardTitle = ''
  let editingCard: KanbanCard | null = null
  let searchQuery = ''
  let addingColumn = false
  let newColumnTitle = ''

  // Drag state
  let dragCard: KanbanCard | null = null
  let dragFromCol: string | null = null
  let dragOverCol: string | null = null

  const DEFAULT_COLUMNS: KanbanColumn[] = [
    { id: 'backlog', title: 'Backlog', color: 'var(--ctp-overlay0)', cards: [] },
    { id: 'todo', title: 'Todo', color: 'var(--ctp-blue)', cards: [] },
    { id: 'progress', title: 'In Progress', color: 'var(--ctp-yellow)', cards: [] },
    { id: 'review', title: 'Review', color: 'var(--ctp-mauve)', cards: [] },
    { id: 'done', title: 'Done', color: 'var(--ctp-green)', cards: [] },
  ]

  function genId() { return Math.random().toString(36).slice(2, 10) }

  onMount(async () => {
    await loadBoard(); loading = false
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') dispatch('close') }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  async function loadBoard() {
    try {
      const raw = await getKanban()
      let saved: BoardData | null = null
      try { saved = raw && raw !== '{}' && raw !== '' ? JSON.parse(raw) : null } catch { saved = null }
      if (saved?.columns?.length) {
        // Ensure all cards have required fields (defensive against incomplete saved data)
        columns = saved.columns.map(col => ({
          ...col,
          cards: (col.cards || []).map(card => ({
            ...card,
            tags: card.tags || [],
            notePath: card.notePath || '',
            dueDate: card.dueDate || '',
            priority: card.priority || 0,
          }))
        }))
      } else {
        columns = DEFAULT_COLUMNS.map(c => ({ ...c, cards: [] }))
      }
      const tasks = (await getAllTasks()) || []
      mergeVaultTasks(tasks)
    } catch {
      columns = DEFAULT_COLUMNS.map(c => ({ ...c, cards: [] }))
    }
  }

  function extractPriority(text: string): number {
    if (text.includes('\u{1F53A}') || text.includes('!!')) return 4
    if (text.includes('\u{23EB}')) return 3
    if (text.includes('\u{1F53C}')) return 2
    if (text.includes('\u{1F53D}')) return 1
    return 0
  }

  function extractDueDate(text: string): string {
    const m = text.match(/📅\s*(\d{4}-\d{2}-\d{2})/) || text.match(/due:\s*(\d{4}-\d{2}-\d{2})/i)
    return m ? m[1] : ''
  }

  function extractTags(text: string): string[] {
    return text.match(/#[a-zA-Z][\w\-\/]*/g) || []
  }

  function hasWipTag(text: string): boolean {
    const l = text.toLowerCase()
    return l.includes('#wip') || l.includes('#doing') || l.includes('#inprogress')
  }

  function mergeVaultTasks(tasks: any[]) {
    if (columns.length === 0) return
    const existing = new Set<string>()
    for (const col of columns) {
      for (const card of col.cards) {
        if (!card.manual && card.notePath) existing.add(`${card.notePath}:${card.lineNum}`)
      }
    }

    for (const task of tasks) {
      const key = `${task.notePath}:${task.lineNum}`
      if (existing.has(key)) continue

      const card: KanbanCard = {
        id: genId(), title: task.text, notePath: task.notePath, lineNum: task.lineNum,
        done: task.done, manual: false, priority: extractPriority(task.text),
        dueDate: extractDueDate(task.text), tags: extractTags(task.text), columnId: '',
      }

      if (task.done) {
        if (columns.length === 0) continue
        const doneCol = columns[columns.length - 1]
        card.columnId = doneCol.id
        doneCol.cards.push(card)
      } else if (hasWipTag(task.text)) {
        const progCol = columns.find(c => c.title.toLowerCase().includes('progress')) || columns[Math.min(2, columns.length - 1)]
        card.columnId = progCol.id
        progCol.cards.push(card)
      } else {
        const todoCol = columns.find(c => c.title.toLowerCase().includes('todo')) || columns[0]
        card.columnId = todoCol.id
        todoCol.cards.push(card)
      }
    }
    columns = [...columns]
  }

  async function saveBoard() {
    try { await saveKanban(JSON.stringify({ columns })) } catch {}
  }

  function addCard(colId: string) {
    if (!newCardTitle.trim()) return
    const col = columns.find(c => c.id === colId)
    if (!col) return
    col.cards.push({
      id: genId(), title: newCardTitle.trim(), notePath: '', lineNum: 0,
      done: false, manual: true, priority: 0, dueDate: '', tags: extractTags(newCardTitle), columnId: colId,
    })
    newCardTitle = ''; addingToColumn = null; columns = [...columns]; saveBoard()
  }

  function deleteCard(colId: string, cardId: string) {
    const col = columns.find(c => c.id === colId)
    if (col) { col.cards = col.cards.filter(c => c.id !== cardId); columns = [...columns]; saveBoard() }
  }

  async function toggleCard(card: KanbanCard) {
    card.done = !card.done; columns = [...columns]
    if (card.notePath && card.lineNum > 0) { try { await toggleTask(card.notePath, card.lineNum) } catch {} }
    saveBoard()
  }

  function addColumn() {
    if (!newColumnTitle.trim()) return
    const colors = ['var(--ctp-blue)', 'var(--ctp-green)', 'var(--ctp-yellow)', 'var(--ctp-mauve)', 'var(--ctp-peach)', 'var(--ctp-teal)']
    columns.push({ id: genId(), title: newColumnTitle.trim(), color: colors[columns.length % colors.length], cards: [] })
    newColumnTitle = ''; addingColumn = false; columns = [...columns]; saveBoard()
  }

  function deleteColumn(colId: string) {
    columns = columns.filter(c => c.id !== colId); saveBoard()
  }

  // Drag and drop
  function onDragStart(e: DragEvent, card: KanbanCard, colId: string) {
    dragCard = card; dragFromCol = colId
    if (e.dataTransfer) { e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setData('text/plain', card.id) }
  }
  function onDragEnd() { dragCard = null; dragFromCol = null; dragOverCol = null }
  function onDragOver(e: DragEvent, colId: string) { e.preventDefault(); dragOverCol = colId }
  function onDragLeave() { dragOverCol = null }
  function onDrop(e: DragEvent, toColId: string) {
    e.preventDefault(); dragOverCol = null
    if (!dragCard || !dragFromCol || dragFromCol === toColId) { dragCard = null; dragFromCol = null; return }
    const from = columns.find(c => c.id === dragFromCol), to = columns.find(c => c.id === toColId)
    if (!from || !to) return
    from.cards = from.cards.filter(c => c.id !== dragCard!.id)
    dragCard!.columnId = toColId
    to.cards.push(dragCard!)
    dragCard = null; dragFromCol = null; columns = [...columns]; saveBoard()
  }

  // Filtered cards per column
  function filteredCards(cards: KanbanCard[]): KanbanCard[] {
    if (!searchQuery.trim()) return cards
    const q = searchQuery.toLowerCase()
    return cards.filter(c => c.title?.toLowerCase().includes(q) || c.notePath?.toLowerCase().includes(q))
  }

  $: totalCards = columns.reduce((s, c) => s + c.cards.length, 0)
  $: doneCards = columns.length > 0 ? columns[columns.length - 1].cards.length : 0
  $: pct = totalCards > 0 ? Math.round(doneCards * 100 / totalCards) : 0

  function priorityStripe(p: number): string {
    switch (p) {
      case 4: return 'var(--ctp-red)'
      case 3: return 'var(--ctp-peach)'
      case 2: return 'var(--ctp-yellow)'
      case 1: return 'var(--ctp-blue)'
      default: return 'transparent'
    }
  }
  function noteName(p: string) { return p.replace(/\.md$/, '').split('/').pop() || p }
</script>

<div class="fixed inset-0 z-50 flex justify-center pt-[3%]" style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)" on:click|self={() => dispatch('close')}>
  <div class="w-[94vw] max-w-6xl h-[88vh] bg-ctp-mantle rounded-xl shadow-overlay flex flex-col overflow-hidden"
    style="border: 1px solid color-mix(in srgb, var(--ctp-surface0) 50%, transparent)">

    <!-- Header -->
    <div class="flex items-center justify-between px-5 py-3" style="border-bottom: 1px solid color-mix(in srgb, var(--ctp-surface0) 40%, transparent)">
      <div class="flex items-center gap-3">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-mauve)" stroke-width="1.5" stroke-linecap="round">
          <rect x="1" y="2" width="4" height="12" rx="1" /><rect x="6" y="2" width="4" height="8" rx="1" /><rect x="11" y="2" width="4" height="10" rx="1" />
        </svg>
        <span class="text-[15px] font-semibold text-ctp-text">Kanban Board</span>
        <span class="text-[12px] text-ctp-overlay1">{totalCards} cards</span>
        {#if totalCards > 0}
          <!-- Progress bar -->
          <div class="w-20 h-1.5 bg-ctp-surface0 rounded-full overflow-hidden">
            <div class="h-full bg-ctp-green rounded-full transition-all" style="width:{pct}%"></div>
          </div>
          <span class="text-[11px] text-ctp-green font-medium">{pct}%</span>
        {/if}
      </div>
      <div class="flex items-center gap-3">
        <input class="bg-ctp-base/50 rounded-md px-3 py-1 text-[12px] text-ctp-text placeholder:text-ctp-overlay0 outline-none w-40"
          style="border: 1px solid color-mix(in srgb, var(--ctp-surface0) 40%, transparent)"
          placeholder="Filter cards..." bind:value={searchQuery} />
        <button class="px-2 py-1 text-[12px] text-ctp-overlay1 hover:text-ctp-blue transition-colors" on:click={loadBoard}>Refresh</button>
        <button class="px-2 py-1 text-[12px] text-ctp-overlay1 hover:text-ctp-green transition-colors" on:click={() => addingColumn = true}>+ Column</button>
        <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0/50 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1 transition-colors"
          on:click={() => dispatch('close')}>esc</kbd>
      </div>
    </div>

    {#if loading}
      <div class="flex-1 flex items-center justify-center"><span class="text-sm text-ctp-overlay1">Loading...</span></div>
    {:else}
      <div class="flex-1 flex gap-3 p-3 overflow-x-auto overflow-y-hidden">
        {#each columns as col}
          <div class="kanban-col" class:drag-over={dragOverCol === col.id}
            on:dragover={e => onDragOver(e, col.id)} on:dragleave={onDragLeave} on:drop={e => onDrop(e, col.id)}>

            <!-- Column header -->
            <div class="flex items-center gap-2 px-3 py-2.5 flex-shrink-0"
              style="border-bottom: 1px solid color-mix(in srgb, var(--ctp-surface0) 30%, transparent)">
              <div class="w-2 h-2 rounded-full flex-shrink-0" style="background:{col.color}"></div>
              <span class="text-[13px] font-semibold text-ctp-text flex-1 truncate">{col.title}</span>
              <span class="text-[11px] text-ctp-overlay0 bg-ctp-surface0/50 px-1.5 py-0.5 rounded-full">{filteredCards(col.cards).length}</span>
              <button class="text-[13px] text-ctp-overlay0 hover:text-ctp-blue transition-colors"
                on:click={() => { addingToColumn = col.id; newCardTitle = '' }}>+</button>
              {#if col.cards.length === 0}
                <button class="text-[11px] text-ctp-overlay0 hover:text-ctp-red transition-colors"
                  on:click={() => deleteColumn(col.id)}>×</button>
              {/if}
            </div>

            <!-- Cards -->
            <div class="flex-1 overflow-y-auto p-2 flex flex-col gap-1.5">
              {#if addingToColumn === col.id}
                <div class="bg-ctp-base rounded-lg p-2.5" style="border: 1px solid var(--ctp-blue)">
                  <input class="w-full bg-transparent text-[13px] text-ctp-text placeholder:text-ctp-overlay0 outline-none"
                    placeholder="Card title..." bind:value={newCardTitle}
                    on:keydown={e => { if (e.key === 'Enter') addCard(col.id); if (e.key === 'Escape') addingToColumn = null }} autofocus />
                  <div class="flex justify-end gap-1.5 mt-2">
                    <button class="text-[11px] text-ctp-overlay1 hover:text-ctp-text px-2 py-0.5" on:click={() => addingToColumn = null}>Cancel</button>
                    <button class="text-[11px] text-ctp-crust bg-ctp-blue px-2.5 py-0.5 rounded hover:opacity-90" on:click={() => addCard(col.id)}>Add</button>
                  </div>
                </div>
              {/if}

              {#each filteredCards(col.cards) as card}
                <div class="kanban-card group" draggable="true" on:dragstart={e => onDragStart(e, card, col.id)} on:dragend={onDragEnd}>
                  <!-- Priority stripe -->
                  {#if card.priority > 0}
                    <div class="priority-stripe" style="background:{priorityStripe(card.priority)}"></div>
                  {/if}

                  <div class="flex items-start gap-2 p-2.5">
                    <button class="checkbox mt-0.5" class:done={card.done}
                      on:click|stopPropagation={() => toggleCard(card)}>
                      {#if card.done}<svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-crust)" stroke-width="2.5" stroke-linecap="round"><path d="M3 8l3.5 3.5L13 5" /></svg>{/if}
                    </button>
                    <div class="flex-1 min-w-0">
                      <div class="text-[13px] text-ctp-text leading-snug" class:line-through={card.done} class:opacity-40={card.done}>{card.title.replace(/📅\s*\d{4}-\d{2}-\d{2}/g, '').replace(/[🔺⏫🔼🔽]/g, '').trim()}</div>
                      <div class="flex items-center gap-2 mt-1 flex-wrap">
                        {#if card.notePath}
                          <button class="text-[11px] text-ctp-overlay0 hover:text-ctp-blue transition-colors truncate"
                            on:click|stopPropagation={() => dispatch('openNote', card.notePath)}>{noteName(card.notePath)}</button>
                        {/if}
                        {#if card.dueDate}
                          <span class="text-[10px] px-1.5 py-0.5 rounded-full"
                            style="color:var(--ctp-yellow);background:color-mix(in srgb, var(--ctp-yellow) 10%, transparent)">{card.dueDate}</span>
                        {/if}
                        {#each (card.tags || []).slice(0, 3) as tag}
                          <span class="text-[10px] text-ctp-teal">{tag}</span>
                        {/each}
                      </div>
                    </div>
                    <button class="flex-shrink-0 text-ctp-overlay0 hover:text-ctp-red opacity-0 group-hover:opacity-100 transition-opacity"
                      on:click|stopPropagation={() => deleteCard(col.id, card.id)}>
                      <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M4 4l8 8M12 4l-8 8" /></svg>
                    </button>
                  </div>
                </div>
              {/each}

              {#if filteredCards(col.cards).length === 0 && addingToColumn !== col.id}
                <div class="flex flex-col items-center py-6 gap-1 opacity-40">
                  <span class="text-[11px] text-ctp-overlay0">Empty</span>
                </div>
              {/if}
            </div>
          </div>
        {/each}

        <!-- Add column -->
        {#if addingColumn}
          <div class="kanban-col" style="min-width:200px">
            <div class="p-3">
              <input class="w-full bg-transparent text-[13px] text-ctp-text placeholder:text-ctp-overlay0 outline-none mb-2"
                style="border-bottom: 1px solid var(--ctp-surface0)"
                placeholder="Column name..." bind:value={newColumnTitle}
                on:keydown={e => { if (e.key === 'Enter') addColumn(); if (e.key === 'Escape') addingColumn = false }} autofocus />
              <div class="flex gap-1.5">
                <button class="text-[11px] text-ctp-overlay1 hover:text-ctp-text px-2 py-0.5" on:click={() => addingColumn = false}>Cancel</button>
                <button class="text-[11px] text-ctp-crust bg-ctp-blue px-2.5 py-0.5 rounded" on:click={addColumn}>Add</button>
              </div>
            </div>
          </div>
        {/if}
      </div>
    {/if}

    <div class="px-5 py-2 flex items-center gap-4 text-[11px] text-ctp-overlay0"
      style="border-top: 1px solid color-mix(in srgb, var(--ctp-surface0) 30%, transparent)">
      <span>Drag cards between columns</span>
      <span><code class="text-ctp-yellow">#wip</code> <code class="text-ctp-yellow">#doing</code> → In Progress</span>
      <span>Priority colors: left stripe</span>
    </div>
  </div>
</div>

<style>
  .kanban-col {
    flex: 1;
    min-width: 200px;
    max-width: 300px;
    display: flex;
    flex-direction: column;
    background: color-mix(in srgb, var(--ctp-base) 80%, transparent);
    border-radius: 0.5rem;
    border: 1px solid color-mix(in srgb, var(--ctp-surface0) 30%, transparent);
    transition: border-color 100ms;
  }
  .kanban-col.drag-over { border-color: var(--ctp-blue); }
  .kanban-card {
    position: relative;
    background: var(--ctp-mantle);
    border: 1px solid color-mix(in srgb, var(--ctp-surface0) 40%, transparent);
    border-radius: 0.375rem;
    cursor: grab;
    overflow: hidden;
    transition: border-color 75ms, box-shadow 75ms;
  }
  .kanban-card:hover { border-color: var(--ctp-surface1); }
  .kanban-card:active { cursor: grabbing; }
  .priority-stripe { position: absolute; left: 0; top: 0; bottom: 0; width: 3px; }
  .checkbox { flex-shrink: 0; width: 15px; height: 15px; border-radius: 3px; border: 1.5px solid var(--ctp-surface2); display: flex; align-items: center; justify-content: center; transition: all 100ms; }
  .checkbox.done { border-color: var(--ctp-green); background: var(--ctp-green); }
  .checkbox:hover:not(.done) { border-color: var(--ctp-blue); }
</style>
