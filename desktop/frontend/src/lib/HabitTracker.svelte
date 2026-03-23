<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'
  import { getHabits, saveHabits } from './api'
  const dispatch = createEventDispatcher()

  interface Habit {
    name: string
    created: string
    log: Record<string, boolean> // date -> completed
  }

  let habits: Habit[] = []
  let newHabitName = ''
  let showAddInput = false

  // 7-day rolling view
  $: today = new Date()
  $: days = Array.from({ length: 7 }, (_, i) => {
    const d = new Date(today)
    d.setDate(d.getDate() - 6 + i)
    return d.toISOString().slice(0, 10)
  })

  $: todayStr = today.toISOString().slice(0, 10)
  $: dayLabels = days.map(d => {
    const dt = new Date(d + 'T00:00:00')
    return dt.toLocaleDateString('en', { weekday: 'short' })
  })

  $: completedToday = habits.filter(h => h.log[todayStr]).length
  $: completionPercent = habits.length > 0 ? Math.round((completedToday / habits.length) * 100) : 0

  function streak(habit: Habit): number {
    let count = 0
    const d = new Date(today)
    while (true) {
      const ds = d.toISOString().slice(0, 10)
      if (!habit.log[ds]) break
      count++
      d.setDate(d.getDate() - 1)
    }
    return count
  }

  function toggleDay(habitIdx: number, date: string) {
    habits[habitIdx].log[date] = !habits[habitIdx].log[date]
    habits = habits
    persistHabits()
  }

  function addHabit() {
    const name = newHabitName.trim()
    if (!name) return
    habits = [...habits, { name, created: todayStr, log: {} }]
    newHabitName = ''
    showAddInput = false
    persistHabits()
  }

  function deleteHabit(idx: number) {
    habits = habits.filter((_, i) => i !== idx)
    persistHabits()
  }

  async function loadHabitsData() {
    try {
      const raw = await getHabits()
      if (raw) {
        const parsed = JSON.parse(raw)
        if (Array.isArray(parsed)) habits = parsed
      }
    } catch { /* empty vault or no data */ }
  }

  async function persistHabits() {
    try {
      await saveHabits(JSON.stringify(habits))
    } catch { /* ignore */ }
  }

  onMount(() => { loadHabitsData() })
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center pt-[6%]" style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-2xl bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden" style="max-height:85vh">
    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <div class="flex items-center gap-2">
        <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-green)" stroke-width="1.5" stroke-linecap="round">
          <path d="M3 8l3 3 7-7" />
        </svg>
        <span class="text-sm font-semibold text-ctp-green">Habit Tracker</span>
      </div>
      <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1 transition-colors"
        on:click={() => dispatch('close')}>esc</kbd>
    </div>

    <!-- Progress bar -->
    <div class="px-4 py-3 border-b border-ctp-surface0">
      <div class="flex items-center justify-between mb-1.5">
        <span class="text-xs text-ctp-overlay1">Today's Progress</span>
        <span class="text-xs font-medium" class:text-ctp-green={completionPercent === 100} class:text-ctp-yellow={completionPercent > 0 && completionPercent < 100} class:text-ctp-overlay1={completionPercent === 0}>
          {completedToday}/{habits.length} &middot; {completionPercent}%
        </span>
      </div>
      <div class="w-full h-1.5 bg-ctp-surface0 rounded-full overflow-hidden">
        <div class="h-full rounded-full transition-all duration-300"
          class:bg-ctp-green={completionPercent === 100}
          class:bg-ctp-yellow={completionPercent > 0 && completionPercent < 100}
          class:bg-ctp-surface1={completionPercent === 0}
          style="width: {completionPercent}%"></div>
      </div>
    </div>

    <!-- Habits list -->
    <div class="flex-1 overflow-y-auto">
      {#if habits.length === 0}
        <div class="flex flex-col items-center py-12 gap-2">
          <svg width="24" height="24" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-surface2)" stroke-width="1" stroke-linecap="round" class="opacity-40">
            <path d="M3 8l3 3 7-7" />
          </svg>
          <p class="text-sm text-ctp-overlay1">No habits tracked yet</p>
          <p class="text-[13px] text-ctp-overlay1">Add your first habit to get started</p>
        </div>
      {:else}
        <!-- Day headers -->
        <div class="grid items-center px-4 py-2 border-b border-ctp-surface0 text-[12px] text-ctp-overlay1" style="grid-template-columns: 1fr 28px repeat(7, 32px) 50px">
          <span>Habit</span>
          <span></span>
          {#each days as day, i}
            <span class="text-center font-medium" class:text-ctp-blue={day === todayStr}>
              {dayLabels[i]}
            </span>
          {/each}
          <span class="text-center">Streak</span>
        </div>

        {#each habits as habit, hi}
          <div class="grid items-center px-4 py-2.5 hover:bg-ctp-surface0/30 transition-colors border-b border-ctp-surface0/50" style="grid-template-columns: 1fr 28px repeat(7, 32px) 50px">
            <!-- Habit name -->
            <div class="flex items-center gap-2 min-w-0">
              <span class="text-sm text-ctp-text truncate">{habit.name}</span>
            </div>

            <!-- Delete button -->
            <button on:click={() => deleteHabit(hi)}
              class="text-[12px] text-ctp-overlay1 hover:text-ctp-red transition-colors w-5 h-5 flex items-center justify-center rounded hover:bg-ctp-surface0">
              &times;
            </button>

            <!-- Day checkboxes -->
            {#each days as day}
              <div class="flex justify-center">
                <button on:click={() => toggleDay(hi, day)}
                  class="w-6 h-6 rounded-md flex items-center justify-center transition-all text-xs
                    {habit.log[day] ? 'bg-ctp-green text-ctp-crust' :
                     day === todayStr ? 'bg-ctp-surface1 text-ctp-overlay1 hover:bg-ctp-yellow/30' :
                     'bg-ctp-surface0 text-ctp-overlay1 hover:bg-ctp-surface1'}">
                  {#if habit.log[day]}
                    <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round">
                      <path d="M3 8l3 3 7-7" />
                    </svg>
                  {/if}
                </button>
              </div>
            {/each}

            <!-- Streak -->
            <div class="flex items-center justify-center gap-1">
              {#if streak(habit) > 0}
                <span class="text-xs font-bold text-ctp-peach">{streak(habit)}d</span>
                <span class="text-[12px]">🔥</span>
              {:else}
                <span class="text-xs text-ctp-overlay1">0d</span>
              {/if}
            </div>
          </div>
        {/each}
      {/if}
    </div>

    <!-- Add habit -->
    <div class="px-4 py-3 border-t border-ctp-surface0">
      {#if showAddInput}
        <form on:submit|preventDefault={addHabit} class="flex items-center gap-2">
          <input bind:value={newHabitName}
            placeholder="New habit name..."
            class="flex-1 bg-ctp-surface0 text-ctp-text text-sm px-3 py-1.5 rounded-lg border border-ctp-surface1 focus:border-ctp-mauve focus:outline-none placeholder:text-ctp-surface2"
            autofocus />
          <button type="submit" class="px-3 py-1.5 bg-ctp-green text-ctp-crust text-xs font-medium rounded-lg hover:opacity-90 transition-opacity">Add</button>
          <button type="button" on:click={() => { showAddInput = false; newHabitName = '' }}
            class="px-3 py-1.5 bg-ctp-surface0 text-ctp-overlay1 text-xs rounded-lg hover:bg-ctp-surface1 transition-colors">Cancel</button>
        </form>
      {:else}
        <button on:click={() => showAddInput = true}
          class="flex items-center gap-1.5 text-xs text-ctp-overlay1 hover:text-ctp-green transition-colors">
          <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
            <path d="M8 3v10M3 8h10" />
          </svg>
          Add new habit
        </button>
      {/if}
    </div>
  </div>
</div>
