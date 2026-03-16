<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'
  const dispatch = createEventDispatcher()
  const api = () => (window as any).go?.main?.GranitApp

  // ---------- Types ----------
  interface KanbanCard {
    id: string
    title: string
    notePath: string
    lineNum: number
    done: boolean
    manual: boolean // true if manually created (not from vault tasks)
  }

  interface KanbanColumn {
    id: string
    title: string
    color: string
    cards: KanbanCard[]
  }

  interface KanbanData {
    columns: KanbanColumn[]
  }

  // ---------- State ----------
  let columns: KanbanColumn[] = []
  let loading = true
  let addingToColumn: string | null = null
  let newCardTitle = ''

  // Drag state
  let dragCard: KanbanCard | null = null
  let dragFromCol: string | null = null
  let dragOverCol: string | null = null

  const DEFAULT_COLUMNS: KanbanColumn[] = [
    { id: 'todo', title: 'Todo', color: 'var(--ctp-blue)', cards: [] },
    { id: 'progress', title: 'In Progress', color: 'var(--ctp-yellow)', cards: [] },
    { id: 'done', title: 'Done', color: 'var(--ctp-green)', cards: [] },
  ]

  function genId() { return Math.random().toString(36).slice(2, 10) }

  // ---------- Lifecycle ----------
  onMount(async () => {
    await loadBoard()
    loading = false
  })

  async function loadBoard() {
    try {
      // Load saved board state
      const raw = await api()?.GetKanban()
      const saved: KanbanData = raw && raw !== '{}' ? JSON.parse(raw) : null

      if (saved && saved.columns && saved.columns.length > 0) {
        columns = saved.columns
      } else {
        columns = DEFAULT_COLUMNS.map(c => ({ ...c, cards: [] }))
      }

      // Load vault tasks and merge
      const tasks = (await api()?.GetAllTasks()) || []
      mergeVaultTasks(tasks)
    } catch {
      columns = DEFAULT_COLUMNS.map(c => ({ ...c, cards: [] }))
    }
  }

  function mergeVaultTasks(tasks: any[]) {
    // Collect existing vault-sourced card keys to avoid duplicates
    const existingKeys = new Set<string>()
    for (const col of columns) {
      for (const card of col.cards) {
        if (!card.manual && card.notePath) {
          existingKeys.add(`${card.notePath}:${card.lineNum}`)
        }
      }
    }

    for (const task of tasks) {
      const key = `${task.notePath}:${task.lineNum}`
      if (existingKeys.has(key)) continue

      const card: KanbanCard = {
        id: genId(),
        title: task.text,
        notePath: task.notePath,
        lineNum: task.lineNum,
        done: task.done,
        manual: false,
      }

      if (task.done) {
        columns[columns.length - 1].cards.push(card) // Done column
      } else if (hasWipTag(task.text)) {
        if (columns.length >= 2) columns[1].cards.push(card) // In Progress
      } else {
        columns[0].cards.push(card) // Todo
      }
    }

    columns = [...columns]
  }

  function hasWipTag(text: string): boolean {
    const lower = text.toLowerCase()
    return lower.includes('#wip') || lower.includes('#doing')
  }

  async function saveBoard() {
    const data: KanbanData = { columns }
    try { await api()?.SaveKanban(JSON.stringify(data)) } catch {}
  }

  // ---------- Card operations ----------
  function addCard(colId: string) {
    const title = newCardTitle.trim()
    if (!title) return

    const col = columns.find(c => c.id === colId)
    if (!col) return

    col.cards.push({
      id: genId(),
      title,
      notePath: '',
      lineNum: 0,
      done: false,
      manual: true,
    })

    newCardTitle = ''
    addingToColumn = null
    columns = [...columns]
    saveBoard()
  }

  function deleteCard(colId: string, cardId: string) {
    const col = columns.find(c => c.id === colId)
    if (!col) return
    col.cards = col.cards.filter(c => c.id !== cardId)
    columns = [...columns]
    saveBoard()
  }

  async function toggleCard(card: KanbanCard) {
    card.done = !card.done
    columns = [...columns]

    if (card.notePath && card.lineNum > 0) {
      try { await api()?.ToggleTask(card.notePath, card.lineNum) } catch {}
    }
    saveBoard()
  }

  function openNote(card: KanbanCard) {
    if (card.notePath) {
      dispatch('openNote', card.notePath)
    }
  }

  // ---------- Drag and Drop ----------
  function onDragStart(e: DragEvent, card: KanbanCard, colId: string) {
    dragCard = card
    dragFromCol = colId
    if (e.dataTransfer) {
      e.dataTransfer.effectAllowed = 'move'
      e.dataTransfer.setData('text/plain', card.id)
    }
  }

  function onDragOver(e: DragEvent, colId: string) {
    e.preventDefault()
    dragOverCol = colId
  }

  function onDragLeave(e: DragEvent) {
    dragOverCol = null
  }

  function onDrop(e: DragEvent, toColId: string) {
    e.preventDefault()
    dragOverCol = null

    if (!dragCard || !dragFromCol) return
    if (dragFromCol === toColId) { dragCard = null; dragFromCol = null; return }

    const fromCol = columns.find(c => c.id === dragFromCol)
    const toCol = columns.find(c => c.id === toColId)
    if (!fromCol || !toCol) return

    fromCol.cards = fromCol.cards.filter(c => c.id !== dragCard!.id)
    toCol.cards.push(dragCard!)

    dragCard = null
    dragFromCol = null
    columns = [...columns]
    saveBoard()
  }

  // ---------- Stats ----------
  $: totalCards = columns.reduce((sum, col) => sum + col.cards.length, 0)
  $: doneCards = columns.length > 0 ? columns[columns.length - 1].cards.length : 0
  $: pct = totalCards > 0 ? Math.round(doneCards * 100 / totalCards) : 0

  function noteName(path: string) { return path.replace(/\.md$/, '').split('/').pop() || path }
</script>

<div class="fixed inset-0 z-50 flex justify-center pt-[4%]" style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)" on:click|self={() => dispatch('close')}>
  <div class="w-[90vw] max-w-5xl h-[85vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-overlay flex flex-col overflow-hidden">

    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-2.5 border-b border-ctp-surface0">
      <div class="flex items-center gap-3">
        <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-mauve)" stroke-width="1.5" stroke-linecap="round">
          <rect x="1" y="2" width="4" height="12" rx="1" /><rect x="6" y="2" width="4" height="8" rx="1" /><rect x="11" y="2" width="4" height="10" rx="1" />
        </svg>
        <span class="text-sm font-semibold text-ctp-text">Kanban Board</span>
        <span class="text-[12px] text-ctp-overlay1">{totalCards} tasks</span>
        {#if totalCards > 0}
          <span class="text-[12px] text-ctp-green font-medium">{pct}% done</span>
        {/if}
      </div>
      <div class="flex items-center gap-2">
        <button class="px-2 py-1 text-[13px] text-ctp-overlay1 hover:text-ctp-blue transition-colors"
          on:click={loadBoard}>Refresh</button>
        <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1 transition-colors"
          on:click={() => dispatch('close')}>esc</kbd>
      </div>
    </div>

    {#if loading}
      <div class="flex-1 flex items-center justify-center">
        <span class="text-sm text-ctp-overlay1">Loading tasks...</span>
      </div>
    {:else}
      <!-- Board -->
      <div class="flex-1 flex gap-3 p-3 overflow-x-auto overflow-y-hidden">
        {#each columns as col}
          <div class="flex-1 min-w-[220px] max-w-[320px] flex flex-col bg-ctp-base rounded-lg border transition-colors"
            class:border-ctp-blue={dragOverCol === col.id}
            class:border-ctp-surface0={dragOverCol !== col.id}
            on:dragover={e => onDragOver(e, col.id)}
            on:dragleave={onDragLeave}
            on:drop={e => onDrop(e, col.id)}>

            <!-- Column header -->
            <div class="flex items-center justify-between px-3 py-2 border-b border-ctp-surface0">
              <div class="flex items-center gap-2">
                <div class="w-2.5 h-2.5 rounded-full" style="background:{col.color}"></div>
                <span class="text-[13px] font-semibold text-ctp-text">{col.title}</span>
                <span class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded-full">{col.cards.length}</span>
              </div>
              <button class="text-[14px] text-ctp-overlay1 hover:text-ctp-blue transition-colors leading-none"
                on:click={() => { addingToColumn = col.id; newCardTitle = '' }}>+</button>
            </div>

            <!-- Cards list -->
            <div class="flex-1 overflow-y-auto p-2 flex flex-col gap-1.5">
              {#if addingToColumn === col.id}
                <div class="bg-ctp-mantle border border-ctp-blue rounded-lg p-2">
                  <input class="w-full bg-transparent text-sm text-ctp-text placeholder-ctp-overlay0 focus:outline-none"
                    placeholder="Card title..."
                    bind:value={newCardTitle}
                    on:keydown={e => {
                      if (e.key === 'Enter') addCard(col.id)
                      if (e.key === 'Escape') addingToColumn = null
                    }}
                    autofocus />
                  <div class="flex justify-end gap-1.5 mt-1.5">
                    <button class="text-[12px] text-ctp-overlay1 hover:text-ctp-text px-1.5 py-0.5"
                      on:click={() => addingToColumn = null}>Cancel</button>
                    <button class="text-[12px] text-ctp-base bg-ctp-blue px-2 py-0.5 rounded hover:opacity-90"
                      on:click={() => addCard(col.id)}>Add</button>
                  </div>
                </div>
              {/if}

              {#each col.cards as card}
                <div class="group bg-ctp-mantle border border-ctp-surface0 rounded-lg p-2.5 hover:border-ctp-surface1 transition-colors cursor-grab active:cursor-grabbing"
                  draggable="true"
                  on:dragstart={e => onDragStart(e, card, col.id)}>
                  <div class="flex items-start gap-2">
                    <!-- Checkbox -->
                    <button class="mt-0.5 flex-shrink-0 w-4 h-4 rounded border transition-colors flex items-center justify-center"
                      class:border-ctp-green={card.done}
                      class:bg-ctp-green={card.done}
                      class:border-ctp-surface2={!card.done}
                      on:click|stopPropagation={() => toggleCard(card)}>
                      {#if card.done}
                        <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-base)" stroke-width="2.5" stroke-linecap="round"><path d="M3 8l3.5 3.5L13 5" /></svg>
                      {/if}
                    </button>

                    <div class="flex-1 min-w-0">
                      <div class="text-[13px] text-ctp-text leading-snug"
                        class:line-through={card.done}
                        class:opacity-50={card.done}>
                        {card.title}
                      </div>
                      {#if card.notePath}
                        <button class="text-[12px] text-ctp-overlay1 hover:text-ctp-blue truncate block mt-0.5 transition-colors"
                          on:click|stopPropagation={() => openNote(card)}>
                          {noteName(card.notePath)}
                        </button>
                      {:else}
                        <span class="text-[12px] text-ctp-overlay1 mt-0.5 block">manual</span>
                      {/if}
                    </div>

                    <!-- Delete -->
                    <button class="flex-shrink-0 text-ctp-overlay1 hover:text-ctp-red opacity-0 group-hover:opacity-100 transition-opacity"
                      on:click|stopPropagation={() => deleteCard(col.id, card.id)}>
                      <svg width="11" height="11" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M4 4l8 8M12 4l-8 8" /></svg>
                    </button>
                  </div>
                </div>
              {/each}

              {#if col.cards.length === 0 && addingToColumn !== col.id}
                <div class="flex flex-col items-center py-8 gap-1.5">
                  <svg width="20" height="20" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-surface2)" stroke-width="1" class="opacity-40">
                    <rect x="3" y="3" width="10" height="10" rx="2" />
                  </svg>
                  <span class="text-[12px] text-ctp-overlay1">No tasks</span>
                </div>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    {/if}

    <!-- Footer -->
    <div class="px-4 py-1.5 border-t border-ctp-surface0 flex items-center gap-4 text-[12px] text-ctp-overlay1">
      <span>Drag cards between columns</span>
      <span>Click checkbox to toggle</span>
      <span>Click note name to open</span>
      <span>Tasks with <code class="text-ctp-yellow">#wip</code> or <code class="text-ctp-yellow">#doing</code> go to In Progress</span>
    </div>
  </div>
</div>
