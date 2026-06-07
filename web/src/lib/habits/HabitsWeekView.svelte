<script lang="ts">
  // Week view — 7-column grid, each row a habit, each column a day.
  // Extracted verbatim from the /habits route. Reads toggle + busy off
  // the data controller; weekDays/shortDow/shortDate are pure derives.
  import { weekDays, shortDow, shortDate } from '$lib/habits/habitsDerives';
  import type { HabitInfo, HabitsResponse } from '$lib/api';
  import type { HabitsDataController } from '$lib/habits/habitsData.svelte';

  let {
    data,
    sortedHabits,
    dataCtl
  }: {
    data: HabitsResponse;
    sortedHabits: HabitInfo[];
    dataCtl: HabitsDataController;
  } = $props();

  const busy = $derived(dataCtl.busy);
  // Header day-columns come from the first habit's window (all habits
  // share the same 7-day window); empty when there are no habits.
  const days = $derived(sortedHabits.length > 0 ? weekDays(sortedHabits[0]) : []);
</script>

<!-- Mobile: the table doesn't fit in 7 columns at touch-target size, so
     it's allowed to scroll horizontally. min-w forces 44px-ish day cells;
     the inner aspect-square keeps cells balanced once there's room. -->
<div class="bg-surface0 border border-surface1 rounded-lg p-3 sm:p-4 overflow-x-auto">
  <table class="w-full border-separate border-spacing-1 min-w-[28rem]">
    <thead>
      <tr>
        <th class="w-32 sm:w-1/4 text-left text-[11px] text-dim font-medium pb-2">Habit</th>
        {#each days as d (d.date)}
          {@const isToday = d.date === data.today}
          <th
            class="text-center text-[11px] font-medium pb-2 {isToday ? 'text-primary' : 'text-dim'}"
          >
            <div>{shortDow(d.date)}</div>
            <div class="font-mono text-[10px] mt-0.5">{shortDate(d.date)}</div>
          </th>
        {/each}
      </tr>
    </thead>
    <tbody>
      {#each sortedHabits as h (h.name)}
        <tr>
          <td class="text-sm text-text break-words py-1 pr-2">
            {h.name}
            <div class="text-[11px] text-dim">
              🔥 {h.currentStreak}d · {h.last7Pct}% / 7d
            </div>
          </td>
          {#each weekDays(h) as d (d.date)}
            {@const isToday = d.date === data.today}
            {@const cellBusy = busy === `${h.name}|${d.date}`}
            <td class="p-0">
              <button
                type="button"
                onclick={() => dataCtl.toggleOnDate(h, d.date, !d.done)}
                disabled={cellBusy}
                class="w-full aspect-square min-h-9 rounded transition-colors hover:opacity-80 disabled:opacity-40
                  {d.done ? 'bg-success' : 'bg-surface1 hover:bg-surface2'}"
                class:ring-1={isToday}
                class:ring-primary={isToday}
                title={`${d.date}${d.done ? ' · done · click to undo' : ' · click to mark done'}`}
                aria-label={`toggle ${h.name} on ${d.date}`}
              ></button>
            </td>
          {/each}
        </tr>
      {/each}
    </tbody>
  </table>
</div>
