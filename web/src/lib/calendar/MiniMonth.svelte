<script lang="ts">
  import { addDays, fmtDateISO, isSameDay, startOfMonth } from './utils';
  import type { CalendarEvent } from '$lib/api';

  let {
    cursor,
    selected,
    events = [],
    onPick
  }: {
    cursor: Date;
    selected: Date;
    events?: CalendarEvent[];
    onPick: (d: Date) => void;
  } = $props();

  // Tracked locally so prev/next can scroll independently of the prop.
  let monthCursor = $state(new Date());
  $effect(() => {
    monthCursor = new Date(cursor.getFullYear(), cursor.getMonth(), 1);
  });

  let cells = $derived.by(() => {
    const ms = startOfMonth(monthCursor);
    const start = addDays(ms, -ms.getDay());
    const out: { date: Date; iso: string; inMonth: boolean; hasEvents: boolean }[] = [];
    const dayKeys = new Set(events.map((e) => e.date ?? (e.start ? e.start.slice(0, 10) : '')).filter(Boolean));
    for (let i = 0; i < 42; i++) {
      const d = addDays(start, i);
      out.push({
        date: d,
        iso: fmtDateISO(d),
        inMonth: d.getMonth() === monthCursor.getMonth(),
        hasEvents: dayKeys.has(fmtDateISO(d))
      });
    }
    return out;
  });

  function prev() {
    monthCursor = new Date(monthCursor.getFullYear(), monthCursor.getMonth() - 1, 1);
  }
  function next() {
    monthCursor = new Date(monthCursor.getFullYear(), monthCursor.getMonth() + 1, 1);
  }

  const today = new Date();
  const monthName = $derived(monthCursor.toLocaleDateString(undefined, { month: 'long', year: 'numeric' }));
</script>

<div class="bg-mantle border border-surface1 rounded p-3 select-none">
  <div class="flex items-center justify-between mb-2">
    <button onclick={prev} class="text-dim hover:text-text px-1">‹</button>
    <span class="text-sm font-medium text-text">{monthName}</span>
    <button onclick={next} class="text-dim hover:text-text px-1">›</button>
  </div>
  <div class="grid grid-cols-7 gap-px text-center text-[10px] text-dim mb-1">
    {#each ['S', 'M', 'T', 'W', 'T', 'F', 'S'] as d}
      <div>{d}</div>
    {/each}
  </div>
  <div class="grid grid-cols-7 gap-px">
    {#each cells as c}
      {@const isToday = isSameDay(c.date, today)}
      {@const isSelected = isSameDay(c.date, selected)}
      <button
        onclick={() => onPick(c.date)}
        class="aspect-square text-xs rounded relative
          {isSelected ? 'bg-primary text-on-primary font-medium' : ''}
          {!isSelected && isToday ? 'text-primary font-medium ring-1 ring-primary/40' : ''}
          {!isSelected && !c.inMonth ? 'text-dim opacity-50' : ''}
          {!isSelected && c.inMonth && !isToday ? 'text-text hover:bg-surface0' : ''}"
      >
        {c.date.getDate()}
        {#if c.hasEvents && !isSelected}
          <span class="absolute bottom-0.5 left-1/2 -translate-x-1/2 w-1 h-1 rounded-full bg-secondary"></span>
        {/if}
      </button>
    {/each}
  </div>
</div>
