<script lang="ts">
  // Heatmap view — year-at-a-glance per habit. Extracted from the
  // /habits route as part of splitting its four inline views into
  // components (KISS / no god file). Pure render over the habit list —
  // the Heatmap component owns layout + tooltips; we just feed it
  // {date, value} pairs (value = 1 done, 0 otherwise — binary maxes the
  // colour scale at the brightest "completed" green).
  import Heatmap from '$lib/components/Heatmap.svelte';
  import type { HabitInfo } from '$lib/api';

  let { sortedHabits }: { sortedHabits: HabitInfo[] } = $props();
</script>

<div class="space-y-4">
  {#each sortedHabits as h (h.name)}
    <article class="bg-surface0 border border-surface1 rounded-lg p-3">
      <header class="flex items-baseline gap-2 mb-3">
        <h3 class="text-sm font-medium text-text">{h.name}</h3>
        <span class="text-[11px] text-dim font-mono tabular-nums">
          {h.currentStreak}d streak · best {h.longestStreak}d
        </span>
        <span class="text-[11px] text-dim font-mono tabular-nums ml-auto">
          {h.last30Pct}% / 30d
        </span>
      </header>
      <Heatmap
        cells={h.days.map((d) => ({ date: d.date, value: d.done ? 1 : 0 }))}
        maxValue={1}
        legendLabels={['none', '', '', '', 'done']}
      />
    </article>
  {/each}
</div>
