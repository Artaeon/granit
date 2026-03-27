<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  export let data: any = null
  const dispatch = createEventDispatcher()

  let year = new Date().getFullYear()
  let month = new Date().getMonth() + 1
  let selectedDate = ''

  $: daysInMonth = new Date(year, month, 0).getDate()
  $: firstDay = new Date(year, month - 1, 1).getDay()
  $: days = Array.from({ length: daysInMonth }, (_, i) => i + 1)
  $: monthName = new Date(year, month - 1).toLocaleString('default', { month: 'long' })
  $: dailyNotes = new Set(data?.dailyNotes || [])
  $: tasks = data?.tasks || {}
  $: events = data?.events || []

  function dateStr(day: number) { return `${year}-${String(month).padStart(2,'0')}-${String(day).padStart(2,'0')}` }
  function hasNote(day: number) { return dailyNotes.has(dateStr(day)) }
  function hasTask(day: number) { return (tasks[dateStr(day)]?.length || 0) > 0 }
  function hasEvent(day: number) { return events.some((e: any) => e.date === dateStr(day)) }
  function isToday(day: number) { const t = new Date(); return t.getFullYear() === year && t.getMonth() + 1 === month && t.getDate() === day }

  function prevMonth() { if (month === 1) { month = 12; year-- } else month--; dispatch('navigate', { year, month }) }
  function nextMonth() { if (month === 12) { month = 1; year++ } else month++; dispatch('navigate', { year, month }) }
  function selectDay(day: number) { selectedDate = dateStr(day) }
</script>

<div class="fixed inset-0 z-50 flex justify-center items-center" style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-3xl h-[80vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden">
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <div class="flex items-center gap-3">
        <button on:click={prevMonth} class="text-ctp-overlay1 hover:text-ctp-text text-lg">&larr;</button>
        <span class="text-sm font-semibold text-ctp-text w-32 text-center">{monthName} {year}</span>
        <button on:click={nextMonth} class="text-ctp-overlay1 hover:text-ctp-text text-lg">&rarr;</button>
      </div>
      <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer" on:click={() => dispatch('close')}>esc</kbd>
    </div>
    <div class="flex flex-1 min-h-0">
      <!-- Calendar grid -->
      <div class="flex-1 p-4">
        <div class="grid grid-cols-7 gap-1 mb-1">
          {#each ['Sun','Mon','Tue','Wed','Thu','Fri','Sat'] as d}
            <div class="text-[12px] text-ctp-overlay1 text-center py-1">{d}</div>
          {/each}
        </div>
        <div class="grid grid-cols-7 gap-1">
          {#each Array(firstDay) as _}
            <div></div>
          {/each}
          {#each days as day}
            <button on:click={() => selectDay(day)}
              class="aspect-square rounded-lg flex flex-col items-center justify-center text-xs transition-colors
                {isToday(day) ? 'bg-ctp-blue text-ctp-crust font-bold' :
                 selectedDate === dateStr(day) ? 'bg-ctp-surface1 text-ctp-text' :
                 'hover:bg-ctp-surface0 text-ctp-text'}">
              <span>{day}</span>
              <div class="flex gap-0.5 mt-0.5">
                {#if hasNote(day)}<span class="w-1 h-1 rounded-full bg-ctp-green"></span>{/if}
                {#if hasTask(day)}<span class="w-1 h-1 rounded-full bg-ctp-yellow"></span>{/if}
                {#if hasEvent(day)}<span class="w-1 h-1 rounded-full bg-ctp-sapphire"></span>{/if}
              </div>
            </button>
          {/each}
        </div>
        <div class="flex gap-4 mt-3 text-[12px] text-ctp-overlay1">
          <span><span class="inline-block w-1.5 h-1.5 rounded-full bg-ctp-green mr-1"></span>Note</span>
          <span><span class="inline-block w-1.5 h-1.5 rounded-full bg-ctp-yellow mr-1"></span>Task</span>
          <span><span class="inline-block w-1.5 h-1.5 rounded-full bg-ctp-sapphire mr-1"></span>Event</span>
        </div>
      </div>
      <!-- Day detail -->
      <div class="w-64 border-l border-ctp-surface0 p-3 overflow-y-auto">
        {#if selectedDate}
          <h4 class="text-xs font-semibold text-ctp-mauve mb-2">{selectedDate}</h4>
          {#if tasks[selectedDate]?.length}
            <div class="mb-3">
              <div class="text-[12px] text-ctp-overlay1 uppercase mb-1">Tasks</div>
              {#each tasks[selectedDate] as task}
                <div class="flex items-start gap-1.5 py-0.5">
                  <input type="checkbox" checked={task.done} on:change={() => dispatch('toggleTask', { notePath: task.notePath, lineNum: task.lineNum })}
                    class="mt-0.5 accent-ctp-mauve" />
                  <span class="text-xs text-ctp-text {task.done ? 'line-through opacity-50' : ''}">{task.text}</span>
                </div>
              {/each}
            </div>
          {/if}
          {#each events.filter((e) => e.date === selectedDate) as ev}
            <div class="text-xs text-ctp-sapphire py-0.5">{ev.title}{ev.location ? ` @ ${ev.location}` : ''}</div>
          {/each}
          {#if hasNote(Number(selectedDate.split('-')[2]))}
            <button on:click={() => dispatch('openNote', selectedDate + '.md')} class="text-xs text-ctp-blue mt-2 hover:underline">Open daily note</button>
          {/if}
        {:else}
          <p class="text-xs text-ctp-overlay1">Select a day</p>
        {/if}
      </div>
    </div>
  </div>
</div>
