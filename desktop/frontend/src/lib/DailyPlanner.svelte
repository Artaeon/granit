<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'
  import { getCalendarData, getHabits, saveHabits, getAllTasks, toggleTask as apiToggleTask } from './api'
  import type { TaskItem } from './types'
  const dispatch = createEventDispatcher()

  interface TimeSlot {
    hour: number
    half: boolean     // false = :00, true = :30
    text: string
    type: 'empty' | 'task' | 'event' | 'break'
    done: boolean
    priority: number
    notePath: string
    lineNum: number
  }

  // State
  let slots: TimeSlot[] = []
  let unscheduled: TaskItem[] = []
  let calendarEvents: any[] = []
  let addingAt = -1
  let addBuf = ''
  let addType: 'task' | 'break' = 'task'
  let loaded = false

  const today = new Date()
  const todayStr = today.toISOString().slice(0, 10)
  const todayFormatted = today.toLocaleDateString('en', { weekday: 'long', month: 'long', day: 'numeric', year: 'numeric' })
  const currentHour = today.getHours()
  const currentMinute = today.getMinutes()

  // 32 half-hour slots: 06:00 to 21:30
  function initSlots(): TimeSlot[] {
    const result: TimeSlot[] = []
    for (let h = 6; h <= 21; h++) {
      result.push({ hour: h, half: false, text: '', type: 'empty', done: false, priority: 0, notePath: '', lineNum: 0 })
      result.push({ hour: h, half: true,  text: '', type: 'empty', done: false, priority: 0, notePath: '', lineNum: 0 })
    }
    return result
  }

  function slotTime(s: TimeSlot): string {
    const hh = String(s.hour).padStart(2, '0')
    const mm = s.half ? '30' : '00'
    return `${hh}:${mm}`
  }

  function fmtSlotTime(s: TimeSlot): string {
    return s.half ? '' : `${String(s.hour).padStart(2, '0')}:00`
  }

  function slotIndex(hour: number, half: boolean): number {
    return (hour - 6) * 2 + (half ? 1 : 0)
  }

  function currentSlotIndex(): number {
    const idx = slotIndex(currentHour, currentMinute >= 30)
    return Math.max(0, Math.min(idx, slots.length - 1))
  }

  // Priority helpers
  const priColors: Record<number, string> = {
    4: 'var(--ctp-red)', 3: 'var(--ctp-peach)', 2: 'var(--ctp-yellow)', 1: 'var(--ctp-blue)',
  }
  function priColor(p: number) { return priColors[p] || 'var(--ctp-overlay0)' }

  // Stats
  $: filledSlots = slots.filter(s => s.type !== 'empty')
  $: doneSlots = filledSlots.filter(s => s.done)
  $: totalMinutes = filledSlots.length * 30
  $: doneMinutes = doneSlots.length * 30
  $: progressPct = filledSlots.length > 0 ? Math.round((doneSlots.length / filledSlots.length) * 100) : 0
  $: unscheduledCount = unscheduled.length
  $: workloadStr = totalMinutes >= 60 ? `${Math.floor(totalMinutes/60)}h${totalMinutes%60 ? totalMinutes%60 + 'm' : ''}` : `${totalMinutes}m`

  // Actions
  function toggleSlot(idx: number) {
    slots[idx].done = !slots[idx].done
    slots = [...slots]
    savePlanner()
  }

  function clearSlot(idx: number) {
    slots[idx] = { ...slots[idx], text: '', type: 'empty', done: false, priority: 0, notePath: '', lineNum: 0 }
    slots = [...slots]
    savePlanner()
  }

  function addToSlot(idx: number) {
    const text = addBuf.trim()
    if (!text) { addingAt = -1; return }
    slots[idx] = { ...slots[idx], text, type: addType, done: false, priority: 0 }
    slots = [...slots]
    addBuf = ''
    addingAt = -1
    savePlanner()
  }

  function scheduleTask(task: TaskItem, idx: number) {
    slots[idx] = {
      ...slots[idx],
      text: task.text.replace(/📅\s*\d{4}-\d{2}-\d{2}/g, '').replace(/[🔺⏫🔼🔽⏰🔁]/g, '').replace(/~\d+(m|h)/g, '').replace(/\s{2,}/g, ' ').trim(),
      type: 'task',
      done: task.done,
      priority: task.priority || 0,
      notePath: task.notePath,
      lineNum: task.lineNum,
    }
    unscheduled = unscheduled.filter(t => t !== task)
    slots = [...slots]
    savePlanner()
  }

  async function toggleVaultTask(task: TaskItem) {
    try {
      await apiToggleTask(task.notePath, task.lineNum)
      task.done = !task.done
      unscheduled = [...unscheduled]
    } catch (e) { console.error('toggle failed', e) }
  }

  // Load / Save
  async function loadPlanner() {
    slots = initSlots()

    // Load saved state first
    try {
      const raw = await getHabits()
      if (raw) {
        const parsed = JSON.parse(raw)
        if (parsed._planner?.[todayStr]?.slots) {
          const saved = parsed._planner[todayStr].slots as TimeSlot[]
          for (let i = 0; i < Math.min(saved.length, slots.length); i++) {
            if (saved[i]?.text) {
              slots[i] = { ...slots[i], ...saved[i] }
            }
          }
        }
      }
    } catch { /* ignore */ }

    // Load calendar events
    try {
      const calData = await getCalendarData(today.getFullYear(), today.getMonth() + 1)
      if (calData?.events) {
        calendarEvents = (calData.events as any[]).filter((e: any) => (e.date || '').substring(0, 10) === todayStr)
        // Place timed events into slots
        for (const ev of calendarEvents) {
          const t = ev.time || (ev.date?.length > 10 ? ev.date.substring(11, 16) : '')
          if (t && !ev.allDay) {
            const [hh, mm] = t.split(':').map(Number)
            if (hh >= 6 && hh <= 21) {
              const idx = slotIndex(hh, mm >= 30)
              if (idx >= 0 && idx < slots.length && slots[idx].type === 'empty') {
                slots[idx] = { ...slots[idx], text: ev.title + (ev.location ? ` @ ${ev.location}` : ''), type: 'event', done: false, priority: 0 }
              }
            }
          }
        }
      }
    } catch { /* ignore */ }

    // Load vault tasks for today/overdue → unscheduled panel
    try {
      const allTasks = await getAllTasks()
      if (Array.isArray(allTasks)) {
        const scheduledTexts = new Set(slots.filter(s => s.text).map(s => s.text))
        unscheduled = allTasks.filter(t => {
          if (t.done) return false
          const due = t.dueDate || ''
          if (due !== todayStr && !(due !== '' && due < todayStr)) return false
          // Skip if already scheduled
          const clean = t.text.replace(/📅\s*\d{4}-\d{2}-\d{2}/g, '').replace(/[🔺⏫🔼🔽⏰🔁]/g, '').replace(/~\d+(m|h)/g, '').replace(/\s{2,}/g, ' ').trim()
          return !scheduledTexts.has(clean) && !scheduledTexts.has(t.text)
        }).sort((a, b) => (b.priority || 0) - (a.priority || 0))
      }
    } catch { /* ignore */ }

    slots = [...slots]
    loaded = true
  }

  async function savePlanner() {
    try {
      let data: any = {}
      try {
        const raw = await getHabits()
        if (raw) data = JSON.parse(raw)
        if (Array.isArray(data)) data = { _habits: data }
      } catch { /* ignore */ }
      if (!data._planner) data._planner = {}
      data._planner[todayStr] = { slots }
      await saveHabits(JSON.stringify(data))
    } catch { /* ignore */ }
  }

  // Drag state for scheduling unscheduled tasks
  let dragTask: TaskItem | null = null

  function handleDrop(idx: number) {
    if (dragTask && slots[idx].type === 'empty') {
      scheduleTask(dragTask, idx)
    }
    dragTask = null
  }

  onMount(() => { loadPlanner() })
</script>

<div class="fixed inset-0 z-50 flex justify-center items-start pt-[2%]" style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-4xl h-[92vh] bg-ctp-mantle rounded-xl shadow-2xl flex flex-col overflow-hidden"
    style="border: 1px solid color-mix(in srgb, var(--ctp-surface0) 50%, transparent)">

    <!-- Header -->
    <div class="flex items-center justify-between px-5 py-3" style="border-bottom: 1px solid color-mix(in srgb, var(--ctp-surface0) 40%, transparent)">
      <div class="flex items-center gap-3">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-blue)" stroke-width="1.5" stroke-linecap="round">
          <rect x="2" y="2" width="12" height="12" rx="2" /><path d="M2 6h12M6 2v12" />
        </svg>
        <div>
          <span class="text-[15px] font-semibold text-ctp-text">Daily Planner</span>
          <span class="text-[12px] text-ctp-overlay1 ml-2">{todayFormatted}</span>
        </div>
      </div>
      <div class="flex items-center gap-4 text-[12px]">
        {#if filledSlots.length > 0}
          <span class="text-ctp-overlay1">{workloadStr} planned</span>
          <div class="flex items-center gap-1.5">
            <div class="w-20 h-1.5 bg-ctp-surface0 rounded-full overflow-hidden">
              <div class="h-full rounded-full transition-all" style="width:{progressPct}%; background:var(--ctp-green)"></div>
            </div>
            <span class="text-ctp-green font-medium">{progressPct}%</span>
          </div>
        {/if}
        {#if unscheduledCount > 0}
          <span class="text-ctp-yellow">{unscheduledCount} unscheduled</span>
        {/if}
        <kbd class="text-ctp-overlay1 bg-ctp-surface0/50 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1" on:click={() => dispatch('close')}>esc</kbd>
      </div>
    </div>

    <!-- Body: time grid + sidebar -->
    <div class="flex flex-1 min-h-0">

      <!-- Time grid -->
      <div class="flex-1 overflow-y-auto" id="planner-grid">
        <!-- All-day events -->
        {#if calendarEvents.some(e => e.allDay)}
          <div class="px-5 py-2" style="border-bottom: 1px solid color-mix(in srgb, var(--ctp-surface0) 30%, transparent)">
            <div class="text-[10px] text-ctp-overlay0 uppercase tracking-wider mb-1">All Day</div>
            {#each calendarEvents.filter(e => e.allDay) as ev}
              <div class="text-[12px] bg-ctp-sapphire/20 text-ctp-sapphire rounded-md px-2 py-1 mb-1">{ev.title}</div>
            {/each}
          </div>
        {/if}

        <!-- Half-hour slots -->
        {#each slots as slot, idx}
          {@const isNow = slot.hour === currentHour && ((slot.half && currentMinute >= 30) || (!slot.half && currentMinute < 30))}
          <div class="flex group relative"
            style="height: 2.25rem; border-bottom: 1px solid color-mix(in srgb, var(--ctp-surface0) {slot.half ? '10' : '25'}%, transparent);
              {isNow ? 'background: color-mix(in srgb, var(--ctp-blue) 5%, transparent)' : ''}"
            on:dragover|preventDefault
            on:drop|preventDefault={() => handleDrop(idx)}>

            <!-- Time label -->
            <div class="w-16 flex-shrink-0 text-right pr-3 -mt-2 text-[11px] text-ctp-overlay0">
              {fmtSlotTime(slot)}
            </div>

            <!-- Current time indicator -->
            {#if isNow && !slot.half && currentMinute < 30}
              <div class="absolute left-16 right-0 z-20 pointer-events-none" style="top: {(currentMinute / 30) * 100}%">
                <div class="flex items-center">
                  <div class="w-2 h-2 rounded-full bg-ctp-red -ml-1"></div>
                  <div class="flex-1 border-t-2 border-ctp-red"></div>
                </div>
              </div>
            {/if}

            <!-- Slot content -->
            <div class="flex-1 flex items-center px-2 min-w-0"
              style="border-left: 1px solid color-mix(in srgb, var(--ctp-surface0) 20%, transparent)">

              {#if addingAt === idx}
                <form on:submit|preventDefault={() => addToSlot(idx)} class="flex items-center gap-2 w-full">
                  <input bind:value={addBuf} placeholder="Task or break..." autofocus
                    class="flex-1 bg-transparent text-[12px] text-ctp-text outline-none placeholder:text-ctp-surface2" />
                  <select bind:value={addType} class="text-[10px] bg-ctp-surface0 text-ctp-text rounded px-1 py-0.5 border-none outline-none">
                    <option value="task">Task</option>
                    <option value="break">Break</option>
                  </select>
                  <button type="submit" class="text-[10px] text-ctp-blue">Add</button>
                  <button type="button" on:click={() => addingAt = -1} class="text-[10px] text-ctp-overlay1">Cancel</button>
                </form>

              {:else if slot.type !== 'empty'}
                <button on:click={() => toggleSlot(idx)}
                  class="w-3.5 h-3.5 rounded flex items-center justify-center flex-shrink-0 border transition-colors mr-2
                    {slot.done ? 'bg-ctp-green border-ctp-green' : 'border-ctp-surface2 hover:border-ctp-overlay0'}">
                  {#if slot.done}<svg width="8" height="8" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-crust)" stroke-width="3" stroke-linecap="round"><path d="M3 8l3 3 7-7" /></svg>{/if}
                </button>
                {#if slot.priority > 0}
                  <span class="w-1.5 h-1.5 rounded-full flex-shrink-0 mr-1" style="background:{priColor(slot.priority)}"></span>
                {/if}
                <span class="text-[12px] truncate mr-1
                  {slot.type === 'event' ? 'text-ctp-sapphire' : slot.type === 'break' ? 'text-ctp-overlay1 italic' : 'text-ctp-text'}
                  {slot.done ? 'line-through opacity-40' : ''}">{slot.text}</span>
                <button on:click={() => clearSlot(idx)}
                  class="text-[11px] text-ctp-overlay1 hover:text-ctp-red opacity-0 group-hover:opacity-100 transition-opacity ml-auto flex-shrink-0">&times;</button>

              {:else}
                <button on:click={() => { addingAt = idx; addBuf = '' }}
                  class="w-full h-full flex items-center opacity-0 group-hover:opacity-100 transition-opacity">
                  <span class="text-[11px] text-ctp-overlay0">+ Add</span>
                </button>
              {/if}
            </div>
          </div>
        {/each}
      </div>

      <!-- Sidebar: unscheduled tasks -->
      <div class="w-64 flex flex-col min-h-0" style="border-left: 1px solid color-mix(in srgb, var(--ctp-surface0) 30%, transparent)">
        <div class="px-3 py-2.5" style="border-bottom: 1px solid color-mix(in srgb, var(--ctp-surface0) 30%, transparent)">
          <div class="text-[11px] text-ctp-overlay0 uppercase tracking-wider font-medium">Unscheduled Tasks</div>
          <div class="text-[10px] text-ctp-overlay0 mt-0.5">Drag to schedule</div>
        </div>
        <div class="flex-1 overflow-y-auto p-2">
          {#if unscheduled.length === 0 && loaded}
            <div class="text-[12px] text-ctp-overlay1 text-center py-6">All tasks scheduled</div>
          {/if}
          {#each unscheduled as task}
            {@const est = task.estimatedMinutes}
            <div class="flex items-start gap-1.5 py-1.5 px-2 rounded-md hover:bg-ctp-surface0/30 transition-colors cursor-grab"
              draggable="true"
              on:dragstart={() => dragTask = task}>
              <button on:click={() => toggleVaultTask(task)}
                class="w-3.5 h-3.5 rounded flex items-center justify-center flex-shrink-0 border mt-0.5 transition-colors
                  {task.done ? 'bg-ctp-green border-ctp-green' : 'border-ctp-surface2 hover:border-ctp-overlay0'}">
                {#if task.done}<svg width="8" height="8" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-crust)" stroke-width="3" stroke-linecap="round"><path d="M3 8l3 3 7-7" /></svg>{/if}
              </button>
              <div class="flex-1 min-w-0">
                <div class="text-[11px] text-ctp-text leading-tight truncate" class:line-through={task.done} class:opacity-40={task.done}>
                  {task.text.replace(/📅\s*\d{4}-\d{2}-\d{2}/g, '').replace(/[🔺⏫🔼🔽⏰🔁]/g, '').replace(/~\d+(m|h)/g, '').replace(/\s{2,}/g, ' ').trim()}
                </div>
                <div class="flex items-center gap-1.5 mt-0.5">
                  {#if task.priority > 0}
                    <span class="w-1.5 h-1.5 rounded-full" style="background:{priColor(task.priority)}"></span>
                  {/if}
                  {#if est}
                    <span class="text-[9px] text-ctp-teal">{est >= 60 ? Math.floor(est/60) + 'h' + (est%60 ? est%60 + 'm' : '') : est + 'm'}</span>
                  {/if}
                  {#if task.dueDate && task.dueDate < todayStr}
                    <span class="text-[9px] text-ctp-red font-medium">overdue</span>
                  {/if}
                </div>
              </div>
            </div>
          {/each}
        </div>

        <!-- Syntax hint -->
        <div class="px-3 py-2 text-[10px] text-ctp-overlay0" style="border-top: 1px solid color-mix(in srgb, var(--ctp-surface0) 25%, transparent)">
          <div class="font-medium mb-0.5">Task syntax</div>
          <div><code class="text-ctp-text">📅</code> due &middot; <code class="text-ctp-red">🔺</code><code class="text-ctp-peach">⏫</code><code class="text-ctp-yellow">🔼</code><code class="text-ctp-blue">🔽</code> priority &middot; <code class="text-ctp-teal">~30m</code> estimate</div>
        </div>
      </div>
    </div>
  </div>
</div>
