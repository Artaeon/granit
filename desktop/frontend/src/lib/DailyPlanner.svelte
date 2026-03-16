<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'
  const dispatch = createEventDispatcher()
  const api = () => (window as any).go?.main?.GranitApp

  interface PlannerItem {
    id: string
    text: string
    done: boolean
    priority: number // 0=none, 1=low, 2=med, 3=high
    block: string // 'morning' | 'afternoon' | 'evening'
  }

  let items: PlannerItem[] = []
  let newTaskText = ''
  let newTaskBlock = 'morning'
  let newTaskPriority = 0
  let showAddInput = false
  let calendarEvents: any[] = []
  let vaultTasks: any[] = []

  const today = new Date()
  const todayStr = today.toISOString().slice(0, 10)
  const todayFormatted = today.toLocaleDateString('en', { weekday: 'long', month: 'long', day: 'numeric', year: 'numeric' })

  const blocks = [
    { key: 'morning', label: 'Morning', icon: '☀️', hours: '6:00 - 12:00', color: 'var(--ctp-yellow)' },
    { key: 'afternoon', label: 'Afternoon', icon: '🌤️', hours: '12:00 - 17:00', color: 'var(--ctp-peach)' },
    { key: 'evening', label: 'Evening', icon: '🌙', hours: '17:00 - 22:00', color: 'var(--ctp-mauve)' },
  ]

  const priorities = [
    { value: 0, label: 'None', color: 'var(--ctp-overlay0)' },
    { value: 1, label: 'Low', color: 'var(--ctp-blue)' },
    { value: 2, label: 'Medium', color: 'var(--ctp-yellow)' },
    { value: 3, label: 'High', color: 'var(--ctp-red)' },
  ]

  function itemsForBlock(block: string): PlannerItem[] {
    return items.filter(i => i.block === block).sort((a, b) => b.priority - a.priority)
  }

  function addTask() {
    const text = newTaskText.trim()
    if (!text) return
    items = [...items, {
      id: Date.now().toString(36),
      text,
      done: false,
      priority: newTaskPriority,
      block: newTaskBlock,
    }]
    newTaskText = ''
    newTaskPriority = 0
    showAddInput = false
    savePlanner()
  }

  function toggleItem(id: string) {
    items = items.map(i => i.id === id ? { ...i, done: !i.done } : i)
    savePlanner()
  }

  function deleteItem(id: string) {
    items = items.filter(i => i.id !== id)
    savePlanner()
  }

  function priorityColor(p: number): string {
    return priorities[p]?.color || 'var(--ctp-overlay0)'
  }

  function priorityLabel(p: number): string {
    return priorities[p]?.label || ''
  }

  async function loadPlanner() {
    // Load calendar events for today
    try {
      const calData = await api()?.GetCalendarData(today.getFullYear(), today.getMonth() + 1)
      if (calData?.events) {
        calendarEvents = calData.events.filter((e: any) => e.date === todayStr)
      }
      // Load tasks from vault for today
      if (calData?.tasks?.[todayStr]) {
        vaultTasks = calData.tasks[todayStr]
      }
    } catch { /* ignore */ }

    // Load saved planner data
    try {
      const raw = await api()?.GetHabits() // reuse storage
      if (raw) {
        const parsed = JSON.parse(raw)
        if (parsed._planner?.[todayStr]) {
          items = parsed._planner[todayStr]
        }
      }
    } catch { /* ignore */ }
  }

  async function savePlanner() {
    try {
      let data: any = {}
      try {
        const raw = await api()?.GetHabits()
        if (raw) data = JSON.parse(raw)
        if (Array.isArray(data)) data = { _habits: data }
      } catch { /* ignore */ }
      if (!data._planner) data._planner = {}
      data._planner[todayStr] = items
      await api()?.SaveHabits(JSON.stringify(data))
    } catch { /* ignore */ }
  }

  $: completedCount = items.filter(i => i.done).length
  $: totalCount = items.length

  onMount(() => { loadPlanner() })
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center pt-[6%]" style="background:rgba(0,0,0,0.5);backdrop-filter:blur(2px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-2xl bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden" style="max-height:85vh">
    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <div>
        <div class="flex items-center gap-2">
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-blue)" stroke-width="1.5" stroke-linecap="round">
            <rect x="2" y="2" width="12" height="12" rx="2" /><path d="M2 6h12M6 2v12" />
          </svg>
          <span class="text-sm font-semibold text-ctp-blue">Daily Planner</span>
        </div>
        <div class="text-[11px] text-ctp-overlay0 mt-0.5 ml-6">{todayFormatted}</div>
      </div>
      <div class="flex items-center gap-3">
        {#if totalCount > 0}
          <span class="text-[11px] text-ctp-overlay0">{completedCount}/{totalCount} done</span>
        {/if}
        <kbd class="text-[10px] text-ctp-overlay0 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1 transition-colors"
          on:click={() => dispatch('close')}>esc</kbd>
      </div>
    </div>

    <!-- Content -->
    <div class="flex-1 overflow-y-auto p-4 space-y-4">
      <!-- Calendar events for today -->
      {#if calendarEvents.length > 0}
        <div class="bg-ctp-base rounded-lg p-3 border border-ctp-surface0">
          <div class="text-[10px] text-ctp-overlay0 uppercase tracking-wider mb-2 font-medium">Calendar Events</div>
          {#each calendarEvents as ev}
            <div class="flex items-center gap-2 py-1">
              <span class="w-1.5 h-1.5 rounded-full bg-ctp-sapphire flex-shrink-0"></span>
              <span class="text-xs text-ctp-text">{ev.title}</span>
              {#if ev.time}
                <span class="text-[10px] text-ctp-overlay0 ml-auto">{ev.time}</span>
              {/if}
            </div>
          {/each}
        </div>
      {/if}

      <!-- Vault tasks due today -->
      {#if vaultTasks.length > 0}
        <div class="bg-ctp-base rounded-lg p-3 border border-ctp-surface0">
          <div class="text-[10px] text-ctp-overlay0 uppercase tracking-wider mb-2 font-medium">Vault Tasks Due Today</div>
          {#each vaultTasks as task}
            <div class="flex items-center gap-2 py-1">
              <input type="checkbox" checked={task.done}
                on:change={() => dispatch('toggleTask', { notePath: task.notePath, lineNum: task.lineNum })}
                class="accent-ctp-mauve" />
              <span class="text-xs text-ctp-text" class:line-through={task.done} class:opacity-50={task.done}>{task.text}</span>
            </div>
          {/each}
        </div>
      {/if}

      <!-- Time blocks -->
      {#each blocks as block}
        <div class="bg-ctp-base rounded-lg border border-ctp-surface0 overflow-hidden">
          <div class="flex items-center gap-2 px-3 py-2 border-b border-ctp-surface0/50">
            <span class="text-sm">{block.icon}</span>
            <span class="text-xs font-semibold" style="color: {block.color}">{block.label}</span>
            <span class="text-[10px] text-ctp-overlay0 ml-1">{block.hours}</span>
            <span class="text-[10px] text-ctp-surface2 ml-auto">{itemsForBlock(block.key).length} items</span>
          </div>

          <div class="p-2">
            {#if itemsForBlock(block.key).length === 0}
              <div class="text-[11px] text-ctp-surface2 py-2 px-2">No tasks scheduled</div>
            {/if}

            {#each itemsForBlock(block.key) as item}
              <div class="flex items-center gap-2 py-1.5 px-2 hover:bg-ctp-surface0/30 rounded-md group transition-colors">
                <button on:click={() => toggleItem(item.id)}
                  class="w-4 h-4 rounded flex items-center justify-center flex-shrink-0 border transition-colors
                    {item.done ? 'bg-ctp-green border-ctp-green' : 'border-ctp-surface2 hover:border-ctp-overlay0'}">
                  {#if item.done}
                    <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-crust)" stroke-width="2.5" stroke-linecap="round">
                      <path d="M3 8l3 3 7-7" />
                    </svg>
                  {/if}
                </button>

                {#if item.priority > 0}
                  <span class="w-1.5 h-1.5 rounded-full flex-shrink-0" style="background: {priorityColor(item.priority)}" title={priorityLabel(item.priority)}></span>
                {/if}

                <span class="text-xs text-ctp-text flex-1" class:line-through={item.done} class:opacity-50={item.done}>{item.text}</span>

                <button on:click={() => deleteItem(item.id)}
                  class="text-[10px] text-ctp-surface2 hover:text-ctp-red opacity-0 group-hover:opacity-100 transition-opacity px-1">&times;</button>
              </div>
            {/each}
          </div>
        </div>
      {/each}
    </div>

    <!-- Add task -->
    <div class="px-4 py-3 border-t border-ctp-surface0">
      {#if showAddInput}
        <form on:submit|preventDefault={addTask} class="space-y-2">
          <input bind:value={newTaskText}
            placeholder="What needs to be done?"
            class="w-full bg-ctp-surface0 text-ctp-text text-sm px-3 py-1.5 rounded-lg border border-ctp-surface1 focus:border-ctp-mauve focus:outline-none placeholder:text-ctp-overlay0"
            autofocus />
          <div class="flex items-center gap-2">
            <select bind:value={newTaskBlock}
              class="bg-ctp-surface0 text-ctp-text text-xs px-2 py-1 rounded border border-ctp-surface1 focus:outline-none">
              {#each blocks as b}
                <option value={b.key}>{b.label}</option>
              {/each}
            </select>
            <select bind:value={newTaskPriority}
              class="bg-ctp-surface0 text-ctp-text text-xs px-2 py-1 rounded border border-ctp-surface1 focus:outline-none">
              {#each priorities as p}
                <option value={p.value}>{p.label}</option>
              {/each}
            </select>
            <div class="flex-1"></div>
            <button type="submit" class="px-3 py-1 bg-ctp-blue text-ctp-crust text-xs font-medium rounded-lg hover:opacity-90 transition-opacity">Add</button>
            <button type="button" on:click={() => { showAddInput = false; newTaskText = '' }}
              class="px-3 py-1 bg-ctp-surface0 text-ctp-overlay1 text-xs rounded-lg hover:bg-ctp-surface1 transition-colors">Cancel</button>
          </div>
        </form>
      {:else}
        <button on:click={() => showAddInput = true}
          class="flex items-center gap-1.5 text-xs text-ctp-overlay1 hover:text-ctp-blue transition-colors">
          <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
            <path d="M8 3v10M3 8h10" />
          </svg>
          Add task to planner
        </button>
      {/if}
    </div>
  </div>
</div>
