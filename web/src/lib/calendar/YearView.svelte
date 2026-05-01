<script lang="ts">
  import type { CalendarEvent } from '$lib/api';
  import { addDays, eventDayKey, fmtDateISO, isSameDay, startOfMonth } from './utils';

  let {
    cursor,
    events,
    onClickDay
  }: {
    cursor: Date;
    events: CalendarEvent[];
    onClickDay: (d: Date) => void;
  } = $props();

  // Density per day: number of events on that day. Used to color cells from
  // surface0 (no events) → primary (busy day).
  let density = $derived.by(() => {
    const m = new Map<string, number>();
    for (const ev of events) {
      const key = eventDayKey(ev);
      if (!key) continue;
      m.set(key, (m.get(key) ?? 0) + 1);
    }
    return m;
  });

  function tone(count: number): string {
    if (count === 0) return 'background: var(--color-surface0); color: var(--color-dim);';
    const intensity = Math.min(count, 6) / 6; // saturate at 6+
    const pct = 14 + Math.round(intensity * 60); // 14% → 74%
    return `background: color-mix(in srgb, var(--color-primary) ${pct}%, transparent); color: var(--color-text);`;
  }

  // Build a 12-month grid for the cursor's year.
  let months = $derived.by(() => {
    const y = cursor.getFullYear();
    return Array.from({ length: 12 }, (_, mi) => {
      const ms = new Date(y, mi, 1);
      const startCol = ms.getDay(); // 0=Sun
      const daysInMonth = new Date(y, mi + 1, 0).getDate();
      const cells: ({ date: Date; iso: string } | null)[] = [];
      for (let i = 0; i < startCol; i++) cells.push(null);
      for (let d = 1; d <= daysInMonth; d++) {
        const date = new Date(y, mi, d);
        cells.push({ date, iso: fmtDateISO(date) });
      }
      while (cells.length % 7 !== 0) cells.push(null);
      return { name: ms.toLocaleDateString(undefined, { month: 'long' }), cells };
    });
  });

  const today = new Date();
  const wd = ['S', 'M', 'T', 'W', 'T', 'F', 'S'];
  // Suppress "addDays not used" warning — kept for parity with other views.
  void addDays;
  void startOfMonth;
</script>

<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4 p-2">
  {#each months as m, mi}
    <section class="border border-surface1 rounded p-3 bg-base">
      <h3 class="text-sm font-semibold text-text mb-2">{m.name}</h3>
      <div class="grid grid-cols-7 gap-0.5 text-[10px] text-dim mb-1">
        {#each wd as d}<div class="text-center">{d}</div>{/each}
      </div>
      <div class="grid grid-cols-7 gap-0.5">
        {#each m.cells as cell}
          {#if cell}
            {@const isToday = isSameDay(cell.date, today)}
            {@const count = density.get(cell.iso) ?? 0}
            <button
              onclick={() => onClickDay(cell.date)}
              title={count > 0 ? `${count} event${count !== 1 ? 's' : ''}` : 'no events'}
              class="aspect-square rounded text-[11px] font-mono flex items-center justify-center hover:ring-1 hover:ring-primary {isToday ? 'ring-1 ring-primary font-semibold' : ''}"
              style={tone(count)}
            >{cell.date.getDate()}</button>
          {:else}
            <div class="aspect-square"></div>
          {/if}
        {/each}
      </div>
    </section>
  {/each}
</div>
