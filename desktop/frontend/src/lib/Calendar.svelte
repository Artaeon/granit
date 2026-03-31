<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<script lang="ts">
  import { createEventDispatcher, onMount, onDestroy } from 'svelte'
  import { createCalendarEvent, deleteCalendarEvent } from './api'
  export let data: any = null
  const dispatch = createEventDispatcher()

  type ViewMode = 'month' | 'week' | 'day'
  let viewMode: ViewMode = 'month'
  let year = new Date().getFullYear()
  let month = new Date().getMonth() + 1
  let selectedDate = new Date().toISOString().split('T')[0]
  let currentHour = new Date().getHours()
  let currentMinute = new Date().getMinutes()
  let timeInterval: ReturnType<typeof setInterval>

  // Event detail / creation state
  let showEventDetail: any = null
  let showCreateEvent = false
  let createDate = ''
  let createStartTime = ''
  let newEvent = { title: '', startTime: '', endTime: '', location: '', description: '', color: 'sapphire', recurrence: '', allDay: false }

  const eventColors = [
    { id: 'sapphire', label: 'Blue', var: '--ctp-sapphire' },
    { id: 'red', label: 'Red', var: '--ctp-red' },
    { id: 'green', label: 'Green', var: '--ctp-green' },
    { id: 'yellow', label: 'Yellow', var: '--ctp-yellow' },
    { id: 'mauve', label: 'Purple', var: '--ctp-mauve' },
    { id: 'peach', label: 'Orange', var: '--ctp-peach' },
    { id: 'teal', label: 'Teal', var: '--ctp-teal' },
  ]

  function colorVar(c: string) { return eventColors.find(ec => ec.id === c)?.var || '--ctp-sapphire' }

  onMount(() => {
    timeInterval = setInterval(() => { currentHour = new Date().getHours(); currentMinute = new Date().getMinutes() }, 15000)
  })
  onDestroy(() => clearInterval(timeInterval))

  function handleKeydown(e: KeyboardEvent) {
    if (showCreateEvent || showEventDetail) { if (e.key === 'Escape') { showCreateEvent = false; showEventDetail = null }; return }
    if (e.key === 'Escape') dispatch('close')
    if (e.key === '1') viewMode = 'month'
    if (e.key === '2') viewMode = 'week'
    if (e.key === '3') viewMode = 'day'
    if (e.key === 't' || e.key === 'T') goToday()
    if (e.key === 'n' || e.key === 'N') openCreateEvent(selectedDate, '')
  }

  // Data
  $: daysInMonth = new Date(year, month, 0).getDate()
  $: firstDay = new Date(year, month - 1, 1).getDay()
  $: days = Array.from({ length: daysInMonth }, (_, i) => i + 1)
  $: monthName = new Date(year, month - 1).toLocaleString('default', { month: 'long' })
  $: dailyNotes = new Set(data?.dailyNotes || [])
  $: tasks = data?.tasks || {}
  $: events = (data?.events || []) as any[]

  const todayStr = new Date().toISOString().split('T')[0]
  const hours = Array.from({ length: 17 }, (_, i) => i + 6)

  function fmtDate(y: number, m: number, d: number) { return `${y}-${String(m).padStart(2, '0')}-${String(d).padStart(2, '0')}` }
  function dateStr(day: number) { return fmtDate(year, month, day) }
  function taskCount(ds: string) { return tasks[ds]?.length || 0 }
  function eventsForDate(ds: string) { return events.filter((e: any) => (e.date || '').substring(0, 10) === ds) }
  function isToday(ds: string) { return ds === todayStr }
  function isWeekend(ds: string) { const d = new Date(ds + 'T00:00'); return d.getDay() === 0 || d.getDay() === 6 }

  function eventStartMinute(e: any): number {
    const t = e.time || (e.date?.length > 10 ? e.date.substring(11, 16) : '')
    if (!t) return 0
    const [h, m] = t.split(':').map(Number)
    return h * 60 + (m || 0)
  }
  function eventDurationMinutes(e: any): number {
    const startMin = eventStartMinute(e)
    if (e.endDate && e.endDate.length > 10) {
      const end = e.endDate.substring(11, 16)
      const [h, m] = end.split(':').map(Number)
      const endMin = h * 60 + (m || 0)
      if (endMin > startMin) return endMin - startMin
    }
    return 60
  }
  function eventsStartingAtHour(ds: string, hour: number) {
    return eventsForDate(ds).filter((e: any) => { if (e.allDay) return false; return Math.floor(eventStartMinute(e) / 60) === hour })
  }
  function allDayEvents(ds: string) { return eventsForDate(ds).filter((e: any) => e.allDay) }
  function fmtHour(h: number) { return `${String(h).padStart(2, '0')}:00` }
  function fmtTime(e: any) { return e.time || (e.date?.length > 10 ? e.date.substring(11, 16) : '') }
  function dayLabel(ds: string) { return new Date(ds + 'T00:00').toLocaleDateString('en', { weekday: 'short', month: 'short', day: 'numeric' }) }
  function shortDay(ds: string) { return new Date(ds + 'T00:00').toLocaleDateString('en', { weekday: 'short' }) }
  function dayNum(ds: string) { return ds.split('-')[2].replace(/^0/, '') }

  // Navigation
  function prevPeriod() {
    if (viewMode === 'month') { if (month === 1) { month = 12; year-- } else month-- }
    else if (viewMode === 'week') { const d = new Date(selectedDate); d.setDate(d.getDate() - 7); selectedDate = d.toISOString().split('T')[0]; year = d.getFullYear(); month = d.getMonth() + 1 }
    else { const d = new Date(selectedDate); d.setDate(d.getDate() - 1); selectedDate = d.toISOString().split('T')[0]; year = d.getFullYear(); month = d.getMonth() + 1 }
    dispatch('navigate', { year, month })
  }
  function nextPeriod() {
    if (viewMode === 'month') { if (month === 12) { month = 1; year++ } else month++ }
    else if (viewMode === 'week') { const d = new Date(selectedDate); d.setDate(d.getDate() + 7); selectedDate = d.toISOString().split('T')[0]; year = d.getFullYear(); month = d.getMonth() + 1 }
    else { const d = new Date(selectedDate); d.setDate(d.getDate() + 1); selectedDate = d.toISOString().split('T')[0]; year = d.getFullYear(); month = d.getMonth() + 1 }
    dispatch('navigate', { year, month })
  }
  function goToday() { const now = new Date(); year = now.getFullYear(); month = now.getMonth() + 1; selectedDate = todayStr; dispatch('navigate', { year, month }) }
  function selectDay(ds: string) { selectedDate = ds; if (viewMode === 'month') viewMode = 'day' }

  // Week
  function getWeekDays(ds: string): string[] {
    const d = new Date(ds + 'T00:00'); const dow = d.getDay(); const result: string[] = []
    for (let i = 0; i < 7; i++) { const dd = new Date(d); dd.setDate(d.getDate() - dow + i); result.push(dd.toISOString().split('T')[0]) }
    return result
  }
  $: weekDays = getWeekDays(selectedDate)

  // Mini calendar
  $: miniDays = Array.from({ length: new Date(year, month, 0).getDate() }, (_, i) => i + 1)
  $: miniFirstDay = new Date(year, month - 1, 1).getDay()

  $: headerLabel = viewMode === 'month' ? `${monthName} ${year}` : viewMode === 'week' ? `${dayLabel(weekDays[0])} — ${dayLabel(weekDays[6])}` : dayLabel(selectedDate)

  // Event CRUD
  function openCreateEvent(date: string, time: string) {
    createDate = date
    createStartTime = time
    newEvent = { title: '', startTime: time, endTime: time ? addMinutes(time, 60) : '', location: '', description: '', color: 'sapphire', recurrence: '', allDay: !time }
    showCreateEvent = true
    showEventDetail = null
  }
  function addMinutes(t: string, m: number): string {
    const [h, mm] = t.split(':').map(Number)
    const total = h * 60 + mm + m
    return `${String(Math.floor(total / 60) % 24).padStart(2, '0')}:${String(total % 60).padStart(2, '0')}`
  }
  async function saveNewEvent() {
    if (!newEvent.title.trim()) return
    try {
      await createCalendarEvent(newEvent.title, createDate, newEvent.allDay ? '' : newEvent.startTime, newEvent.allDay ? '' : newEvent.endTime, newEvent.location, newEvent.description, newEvent.color, newEvent.recurrence, newEvent.allDay)
      showCreateEvent = false
      dispatch('navigate', { year, month }) // refresh
    } catch (e) { console.error('create event failed', e) }
  }
  function openEventDetail(ev: any) { showEventDetail = ev; showCreateEvent = false }
  async function deleteEvent(ev: any) {
    if (!ev.id) return
    try {
      await deleteCalendarEvent(ev.id)
      showEventDetail = null
      dispatch('navigate', { year, month })
    } catch (e) { console.error('delete failed', e) }
  }

  function clickSlot(ds: string, hour: number) {
    const t = `${String(hour).padStart(2, '0')}:00`
    openCreateEvent(ds, t)
  }
</script>

<svelte:window on:keydown={handleKeydown} />

<div class="fixed inset-0 z-50 flex justify-center items-start pt-[2%]" style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-5xl h-[92vh] bg-ctp-mantle rounded-xl shadow-2xl flex flex-col overflow-hidden" style="border: 1px solid color-mix(in srgb, var(--ctp-surface0) 50%, transparent)">

    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-2.5" style="border-bottom: 1px solid color-mix(in srgb, var(--ctp-surface0) 40%, transparent)">
      <div class="flex items-center gap-3">
        <button on:click={goToday} class="text-[12px] px-3 py-1 rounded-md border border-ctp-surface1 text-ctp-text hover:bg-ctp-surface0 transition-colors">Today</button>
        <button on:click={prevPeriod} class="w-7 h-7 flex items-center justify-center rounded-full hover:bg-ctp-surface0 text-ctp-overlay1 hover:text-ctp-text"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M15 18l-6-6 6-6"/></svg></button>
        <button on:click={nextPeriod} class="w-7 h-7 flex items-center justify-center rounded-full hover:bg-ctp-surface0 text-ctp-overlay1 hover:text-ctp-text"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18l6-6-6-6"/></svg></button>
        <span class="text-[15px] font-semibold text-ctp-text ml-1">{headerLabel}</span>
      </div>
      <div class="flex items-center gap-2">
        <button on:click={() => openCreateEvent(selectedDate, '')} class="text-[12px] px-3 py-1 rounded-md bg-ctp-blue text-ctp-crust font-medium hover:opacity-90 transition-opacity">+ Event</button>
        {#each [{ id: 'day', label: 'Day' }, { id: 'week', label: 'Week' }, { id: 'month', label: 'Month' }] as v}
          <button class="px-2.5 py-1 text-[12px] rounded-md transition-all {viewMode === v.id ? 'bg-ctp-surface1 text-ctp-text font-medium' : 'text-ctp-overlay1 hover:text-ctp-text hover:bg-ctp-surface0/40'}" on:click={() => viewMode = v.id}>{v.label}</button>
        {/each}
        <kbd class="text-[11px] text-ctp-overlay1 bg-ctp-surface0/50 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1 ml-2" on:click={() => dispatch('close')}>esc</kbd>
      </div>
    </div>

    <div class="flex flex-1 min-h-0">
      <!-- Sidebar -->
      <div class="w-48 p-3 flex flex-col gap-3 overflow-y-auto" style="border-right: 1px solid color-mix(in srgb, var(--ctp-surface0) 30%, transparent)">
        <div>
          <div class="text-[11px] font-semibold text-ctp-text mb-1.5 text-center">{monthName} {year}</div>
          <div class="grid grid-cols-7 gap-px">
            {#each ['S','M','T','W','T','F','S'] as d}<div class="text-[9px] text-ctp-overlay0 text-center py-0.5">{d}</div>{/each}
            {#each Array(miniFirstDay) as _}<div></div>{/each}
            {#each miniDays as day}
              {@const ds = dateStr(day)}
              <button on:click={() => { selectedDate = ds; dispatch('navigate', { year, month }) }}
                class="text-[10px] w-5 h-5 flex items-center justify-center rounded-full transition-colors
                  {isToday(ds) ? 'bg-ctp-blue text-ctp-crust font-bold' : selectedDate === ds ? 'bg-ctp-surface1 text-ctp-text' : 'text-ctp-subtext0 hover:bg-ctp-surface0'}">{day}</button>
            {/each}
          </div>
        </div>
        <div class="flex flex-col gap-1 text-[10px] text-ctp-overlay1 mt-1">
          {#each eventColors.slice(0, 4) as c}
            <div class="flex items-center gap-1.5"><span class="w-2 h-2 rounded-sm" style="background:var({c.var})"></span>{c.label}</div>
          {/each}
        </div>
        <!-- Day detail -->
        {#if selectedDate}
          <div class="mt-1 pt-2" style="border-top: 1px solid color-mix(in srgb, var(--ctp-surface0) 30%, transparent)">
            <div class="text-[11px] font-semibold text-ctp-mauve mb-1.5">{dayLabel(selectedDate)}</div>
            {#if tasks[selectedDate]?.length}
              <div class="mb-2">
                <div class="text-[10px] text-ctp-overlay0 uppercase tracking-wider mb-1">Tasks</div>
                {#each tasks[selectedDate] as task}
                  <div class="flex items-start gap-1.5 py-0.5">
                    <input type="checkbox" checked={task.done} on:change={() => dispatch('toggleTask', { notePath: task.notePath, lineNum: task.lineNum })} class="mt-0.5 accent-ctp-mauve w-3 h-3" />
                    <span class="text-[11px] text-ctp-text leading-tight" class:line-through={task.done} class:opacity-40={task.done}>{task.text}</span>
                  </div>
                {/each}
              </div>
            {/if}
            {#each eventsForDate(selectedDate) as ev}
              <button on:click={() => openEventDetail(ev)} class="flex items-center gap-1.5 py-0.5 w-full text-left hover:bg-ctp-surface0/30 rounded px-1 -mx-1 transition-colors">
                <span class="w-1.5 h-1.5 rounded-full flex-shrink-0" style="background:var({colorVar(ev.color)})"></span>
                <span class="text-[11px] text-ctp-text leading-tight truncate">{fmtTime(ev) ? fmtTime(ev) + ' ' : ''}{ev.title}</span>
              </button>
            {/each}
            {#if dailyNotes.has(selectedDate)}
              <button on:click={() => dispatch('openNote', selectedDate + '.md')} class="text-[11px] text-ctp-blue mt-2 hover:underline block">Open daily note</button>
            {/if}
          </div>
        {/if}
      </div>

      <!-- Main view -->
      <div class="flex-1 flex flex-col min-h-0">

        {#if viewMode === 'month'}
          <div class="flex-1 p-3 overflow-y-auto">
            <div class="grid grid-cols-7 gap-px bg-ctp-surface0/30 rounded-lg overflow-hidden" style="min-height:100%">
              {#each ['Sun','Mon','Tue','Wed','Thu','Fri','Sat'] as d, i}
                <div class="text-[11px] text-ctp-overlay1 text-center py-1.5 bg-ctp-mantle font-medium {i===0||i===6?'text-ctp-overlay0':''}">{d}</div>
              {/each}
              {#each Array(firstDay) as _}<div class="bg-ctp-base/30 min-h-[5rem]"></div>{/each}
              {#each days as day}
                {@const ds = dateStr(day)}
                {@const dayEvents = eventsForDate(ds)}
                {@const tc = taskCount(ds)}
                <button on:click={() => selectDay(ds)} class="min-h-[5rem] p-1 text-left transition-colors flex flex-col {isWeekend(ds)?'bg-ctp-base/20':'bg-ctp-base/50'} {selectedDate===ds?'ring-1 ring-ctp-blue ring-inset':''} hover:bg-ctp-surface0/30">
                  <div class="flex items-center justify-between mb-0.5">
                    <span class="text-[12px] w-6 h-6 flex items-center justify-center rounded-full {isToday(ds)?'bg-ctp-blue text-ctp-crust font-bold':'text-ctp-subtext0'}">{day}</span>
                    {#if dailyNotes.has(ds)}<span class="w-1.5 h-1.5 rounded-full bg-ctp-green flex-shrink-0"></span>{/if}
                  </div>
                  {#each dayEvents.slice(0, 3) as ev}
                    <div class="text-[10px] px-1 py-px rounded truncate mb-px" style="color:var({colorVar(ev.color)}); background:color-mix(in srgb, var({colorVar(ev.color)}) 15%, transparent)">
                      {fmtTime(ev) ? fmtTime(ev) + ' ' : ''}{ev.title}
                    </div>
                  {/each}
                  {#if dayEvents.length > 3}<div class="text-[9px] text-ctp-overlay0 px-1">+{dayEvents.length - 3} more</div>{/if}
                  {#if tc > 0}<div class="text-[10px] px-1 py-px text-ctp-yellow mt-auto">{tc} task{tc > 1 ? 's' : ''}</div>{/if}
                </button>
              {/each}
            </div>
          </div>

        {:else if viewMode === 'week'}
          {@const anyAllDay = weekDays.some(d => allDayEvents(d).length > 0)}
          {#if anyAllDay}
            <div class="flex" style="border-bottom: 1px solid color-mix(in srgb, var(--ctp-surface0) 40%, transparent)">
              <div class="w-14 flex-shrink-0 text-[10px] text-ctp-overlay0 text-right pr-2 py-1">all-day</div>
              {#each weekDays as ds}
                <div class="flex-1 py-1 px-0.5 min-h-[1.5rem]" style="border-left: 1px solid color-mix(in srgb, var(--ctp-surface0) 20%, transparent)">
                  {#each allDayEvents(ds) as ev}
                    <button on:click={() => openEventDetail(ev)} class="text-[10px] rounded px-1 py-px truncate mb-px block w-full text-left hover:opacity-80" style="color:var({colorVar(ev.color)}); background:color-mix(in srgb, var({colorVar(ev.color)}) 15%, transparent)">{ev.title}</button>
                  {/each}
                </div>
              {/each}
            </div>
          {/if}
          <div class="flex" style="border-bottom: 1px solid color-mix(in srgb, var(--ctp-surface0) 40%, transparent)">
            <div class="w-14 flex-shrink-0"></div>
            {#each weekDays as ds}
              <button on:click={() => { selectedDate = ds; viewMode = 'day' }} class="flex-1 py-2 text-center transition-colors hover:bg-ctp-surface0/20" style="border-left: 1px solid color-mix(in srgb, var(--ctp-surface0) 20%, transparent)">
                <div class="text-[10px] text-ctp-overlay1 uppercase">{shortDay(ds)}</div>
                <div class="text-[15px] mt-0.5 w-8 h-8 flex items-center justify-center rounded-full mx-auto {isToday(ds)?'bg-ctp-blue text-ctp-crust font-bold':'text-ctp-text'}">{dayNum(ds)}</div>
              </button>
            {/each}
          </div>
          <div class="flex-1 overflow-y-auto relative">
            {#each hours as hour}
              <div class="flex" style="height:3.5rem; border-bottom: 1px solid color-mix(in srgb, var(--ctp-surface0) 15%, transparent)">
                <div class="w-14 flex-shrink-0 text-[10px] text-ctp-overlay0 text-right pr-2 -mt-2">{fmtHour(hour)}</div>
                {#each weekDays as ds}
                  {@const hourEvents = eventsStartingAtHour(ds, hour)}
                  <div class="flex-1 relative cursor-pointer" on:dblclick={() => clickSlot(ds, hour)}
                    style="border-left: 1px solid color-mix(in srgb, var(--ctp-surface0) 15%, transparent); {isWeekend(ds)?'background:color-mix(in srgb, var(--ctp-surface0) 5%, transparent)':''}">
                    {#each hourEvents as ev, ei}
                      {@const dur = eventDurationMinutes(ev)}
                      {@const startMin = eventStartMinute(ev)}
                      {@const topPct = ((startMin % 60) / 60) * 100}
                      {@const heightRem = Math.max(1.2, (dur / 60) * 3.5)}
                      <button on:click|stopPropagation={() => openEventDetail(ev)} class="absolute rounded px-1 py-px text-[10px] overflow-hidden z-10 hover:z-30 transition-colors cursor-pointer"
                        style="top:{topPct}%; height:{heightRem}rem; left:{ei * 30 + 2}px; right:2px; color:var({colorVar(ev.color)}); background:color-mix(in srgb, var({colorVar(ev.color)}) 18%, transparent); border-left: 3px solid var({colorVar(ev.color)})">
                        <div class="font-medium truncate">{fmtTime(ev)} {ev.title}</div>
                        {#if dur > 45 && ev.location}<div class="text-[9px] opacity-70 truncate">{ev.location}</div>{/if}
                      </button>
                    {/each}
                    {#if isToday(ds) && hour === currentHour}
                      <div class="absolute left-0 right-0 z-20 pointer-events-none" style="top:{(currentMinute/60)*100}%">
                        <div class="flex items-center"><div class="w-2 h-2 rounded-full bg-ctp-red -ml-1"></div><div class="flex-1 border-t-2 border-ctp-red"></div></div>
                      </div>
                    {/if}
                  </div>
                {/each}
              </div>
            {/each}
          </div>

        {:else}
          {@const dayAllDay = allDayEvents(selectedDate)}
          {#if dayAllDay.length > 0}
            <div class="px-4 py-2" style="border-bottom: 1px solid color-mix(in srgb, var(--ctp-surface0) 40%, transparent)">
              <div class="text-[10px] text-ctp-overlay0 uppercase tracking-wider mb-1">All Day</div>
              {#each dayAllDay as ev}
                <button on:click={() => openEventDetail(ev)} class="text-[12px] rounded-md px-2 py-1 mb-1 block hover:opacity-80 transition-opacity" style="color:var({colorVar(ev.color)}); background:color-mix(in srgb, var({colorVar(ev.color)}) 15%, transparent)">{ev.title}{ev.location ? ` @ ${ev.location}` : ''}</button>
              {/each}
            </div>
          {/if}
          {#if tasks[selectedDate]?.length}
            <div class="px-4 py-2" style="border-bottom: 1px solid color-mix(in srgb, var(--ctp-surface0) 30%, transparent)">
              <div class="text-[10px] text-ctp-overlay0 uppercase tracking-wider mb-1">Tasks</div>
              {#each tasks[selectedDate] as task}
                <div class="flex items-center gap-2 py-0.5">
                  <input type="checkbox" checked={task.done} on:change={() => dispatch('toggleTask', { notePath: task.notePath, lineNum: task.lineNum })} class="accent-ctp-mauve w-3 h-3" />
                  <span class="text-[12px] text-ctp-text" class:line-through={task.done} class:opacity-40={task.done}>{task.text}</span>
                </div>
              {/each}
            </div>
          {/if}
          <div class="flex-1 overflow-y-auto">
            {#each hours as hour}
              {@const hourEvents = eventsStartingAtHour(selectedDate, hour)}
              <div class="flex" style="height:3.5rem; border-bottom: 1px solid color-mix(in srgb, var(--ctp-surface0) 15%, transparent)">
                <div class="w-16 flex-shrink-0 text-[11px] text-ctp-overlay0 text-right pr-3 -mt-2">{fmtHour(hour)}</div>
                <div class="flex-1 relative cursor-pointer" on:dblclick={() => clickSlot(selectedDate, hour)}
                  style="border-left: 1px solid color-mix(in srgb, var(--ctp-surface0) 20%, transparent)">
                  {#each hourEvents as ev}
                    {@const dur = eventDurationMinutes(ev)}
                    {@const startMin = eventStartMinute(ev)}
                    {@const topPct = ((startMin % 60) / 60) * 100}
                    {@const heightRem = Math.max(1.5, (dur / 60) * 3.5)}
                    <button on:click|stopPropagation={() => openEventDetail(ev)} class="absolute left-1 right-1 rounded-md px-2 py-1 z-10 hover:z-30 transition-colors overflow-hidden cursor-pointer"
                      style="top:{topPct}%; height:{heightRem}rem; color:var({colorVar(ev.color)}); background:color-mix(in srgb, var({colorVar(ev.color)}) 18%, transparent); border-left: 3px solid var({colorVar(ev.color)})">
                      <div class="text-[12px] font-medium truncate">{ev.title}</div>
                      <div class="text-[10px] opacity-70 truncate">{fmtTime(ev)}{ev.location ? ` @ ${ev.location}` : ''}</div>
                    </button>
                  {/each}
                  {#if isToday(selectedDate) && hour === currentHour}
                    <div class="absolute left-0 right-0 z-20 pointer-events-none" style="top:{(currentMinute/60)*100}%">
                      <div class="flex items-center"><div class="w-2.5 h-2.5 rounded-full bg-ctp-red -ml-1.5"></div><div class="flex-1 border-t-2 border-ctp-red"></div></div>
                    </div>
                  {/if}
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    </div>
  </div>

  <!-- Event Detail Popup -->
  {#if showEventDetail}
    <div class="fixed inset-0 z-[60] flex items-center justify-center" style="background:rgba(17,17,27,0.4)" on:click|self={() => showEventDetail = null}>
      <div class="bg-ctp-mantle rounded-xl shadow-2xl w-80 overflow-hidden" style="border: 1px solid color-mix(in srgb, var(--ctp-surface0) 50%, transparent)">
        <div class="h-1.5 rounded-t-xl" style="background:var({colorVar(showEventDetail.color)})"></div>
        <div class="p-4">
          <h3 class="text-[15px] font-semibold text-ctp-text mb-1">{showEventDetail.title}</h3>
          {#if fmtTime(showEventDetail)}
            <div class="text-[12px] text-ctp-subtext0 mb-1">{fmtTime(showEventDetail)}{showEventDetail.endDate?.length > 10 ? ' – ' + showEventDetail.endDate.substring(11, 16) : ''}</div>
          {:else}
            <div class="text-[12px] text-ctp-subtext0 mb-1">All day</div>
          {/if}
          {#if showEventDetail.location}
            <div class="flex items-center gap-1.5 text-[12px] text-ctp-overlay1 mb-1">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2C8.13 2 5 5.13 5 9c0 5.25 7 13 7 13s7-7.75 7-13c0-3.87-3.13-7-7-7z"/><circle cx="12" cy="9" r="2.5"/></svg>
              {showEventDetail.location}
            </div>
          {/if}
          {#if showEventDetail.description}
            <div class="text-[12px] text-ctp-subtext0 mt-2 pt-2" style="border-top: 1px solid color-mix(in srgb, var(--ctp-surface0) 30%, transparent)">{showEventDetail.description}</div>
          {/if}
          {#if showEventDetail.recurrence}
            <div class="text-[11px] text-ctp-overlay0 mt-2">Repeats {showEventDetail.recurrence}</div>
          {/if}
          <div class="flex items-center gap-2 mt-3 pt-2" style="border-top: 1px solid color-mix(in srgb, var(--ctp-surface0) 30%, transparent)">
            {#if showEventDetail.id}
              <button on:click={() => deleteEvent(showEventDetail)} class="text-[12px] text-ctp-red hover:underline">Delete</button>
            {/if}
            <div class="flex-1"></div>
            <button on:click={() => showEventDetail = null} class="text-[12px] text-ctp-overlay1 hover:text-ctp-text px-3 py-1 rounded-md hover:bg-ctp-surface0 transition-colors">Close</button>
          </div>
        </div>
      </div>
    </div>
  {/if}

  <!-- Create Event Dialog -->
  {#if showCreateEvent}
    <div class="fixed inset-0 z-[60] flex items-center justify-center" style="background:rgba(17,17,27,0.4)" on:click|self={() => showCreateEvent = false}>
      <div class="bg-ctp-mantle rounded-xl shadow-2xl w-96 overflow-hidden" style="border: 1px solid color-mix(in srgb, var(--ctp-surface0) 50%, transparent)">
        <div class="px-4 py-3 flex items-center justify-between" style="border-bottom: 1px solid color-mix(in srgb, var(--ctp-surface0) 40%, transparent)">
          <span class="text-[14px] font-semibold text-ctp-text">New Event</span>
          <button on:click={() => showCreateEvent = false} class="text-ctp-overlay1 hover:text-ctp-text">&times;</button>
        </div>
        <form on:submit|preventDefault={saveNewEvent} class="p-4 space-y-3">
          <input bind:value={newEvent.title} placeholder="Event title" autofocus
            class="w-full bg-ctp-base rounded-md px-3 py-2 text-[13px] text-ctp-text placeholder:text-ctp-surface2 outline-none" style="border: 1px solid color-mix(in srgb, var(--ctp-surface0) 50%, transparent)" />
          <div class="flex items-center gap-2">
            <label class="flex items-center gap-1.5 text-[12px] text-ctp-overlay1">
              <input type="checkbox" bind:checked={newEvent.allDay} class="accent-ctp-mauve" /> All day
            </label>
            <span class="text-[12px] text-ctp-overlay0 ml-auto">{createDate}</span>
          </div>
          {#if !newEvent.allDay}
            <div class="flex gap-2">
              <input type="time" bind:value={newEvent.startTime} class="flex-1 bg-ctp-base rounded-md px-3 py-1.5 text-[12px] text-ctp-text outline-none" style="border: 1px solid color-mix(in srgb, var(--ctp-surface0) 40%, transparent)" />
              <span class="text-ctp-overlay0 self-center">—</span>
              <input type="time" bind:value={newEvent.endTime} class="flex-1 bg-ctp-base rounded-md px-3 py-1.5 text-[12px] text-ctp-text outline-none" style="border: 1px solid color-mix(in srgb, var(--ctp-surface0) 40%, transparent)" />
            </div>
          {/if}
          <input bind:value={newEvent.location} placeholder="Location (optional)"
            class="w-full bg-ctp-base rounded-md px-3 py-1.5 text-[12px] text-ctp-text placeholder:text-ctp-surface2 outline-none" style="border: 1px solid color-mix(in srgb, var(--ctp-surface0) 40%, transparent)" />
          <textarea bind:value={newEvent.description} placeholder="Description (optional)" rows="2"
            class="w-full bg-ctp-base rounded-md px-3 py-1.5 text-[12px] text-ctp-text placeholder:text-ctp-surface2 outline-none resize-none" style="border: 1px solid color-mix(in srgb, var(--ctp-surface0) 40%, transparent)"></textarea>
          <div class="flex items-center gap-2">
            <span class="text-[11px] text-ctp-overlay0">Color:</span>
            {#each eventColors as c}
              <button type="button" on:click={() => newEvent.color = c.id}
                class="w-5 h-5 rounded-full transition-all {newEvent.color === c.id ? 'ring-2 ring-offset-1 ring-ctp-text' : 'opacity-60 hover:opacity-100'}"
                style="background:var({c.var}); ring-offset-color: var(--ctp-mantle)" title={c.label}></button>
            {/each}
          </div>
          <select bind:value={newEvent.recurrence} class="w-full bg-ctp-base rounded-md px-3 py-1.5 text-[12px] text-ctp-text outline-none" style="border: 1px solid color-mix(in srgb, var(--ctp-surface0) 40%, transparent)">
            <option value="">Does not repeat</option>
            <option value="daily">Daily</option>
            <option value="weekly">Weekly</option>
            <option value="monthly">Monthly</option>
            <option value="yearly">Yearly</option>
          </select>
          <div class="flex justify-end gap-2 pt-1">
            <button type="button" on:click={() => showCreateEvent = false} class="px-4 py-1.5 text-[12px] text-ctp-overlay1 rounded-md hover:bg-ctp-surface0 transition-colors">Cancel</button>
            <button type="submit" class="px-4 py-1.5 text-[12px] bg-ctp-blue text-ctp-crust font-medium rounded-md hover:opacity-90 transition-opacity">Save</button>
          </div>
        </form>
      </div>
    </div>
  {/if}
</div>
