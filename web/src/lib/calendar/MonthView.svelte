<script lang="ts">
  import type { CalendarEvent } from '$lib/api';
  import { addDays, eventDayKey, eventTypeColor, fmtDateISO, fmtTime, isSameDay, startOfMonth } from './utils';

  let {
    cursor,
    events,
    onClickEvent,
    onClickDay
  }: {
    cursor: Date;
    events: CalendarEvent[];
    onClickEvent: (ev: CalendarEvent) => void;
    onClickDay: (d: Date) => void;
  } = $props();

  let cells = $derived.by(() => {
    const ms = startOfMonth(cursor);
    const start = addDays(ms, -ms.getDay());
    return Array.from({ length: 42 }, (_, i) => {
      const d = addDays(start, i);
      return { date: d, iso: fmtDateISO(d), inMonth: d.getMonth() === cursor.getMonth() };
    });
  });

  let eventsByDay = $derived.by(() => {
    const m = new Map<string, CalendarEvent[]>();
    for (const ev of events) {
      const key = eventDayKey(ev);
      if (!key) continue;
      if (!m.has(key)) m.set(key, []);
      m.get(key)!.push(ev);
    }
    for (const [, evs] of m) {
      evs.sort((a, b) => {
        const sa = a.start ? new Date(a.start).getTime() : 0;
        const sb = b.start ? new Date(b.start).getTime() : 0;
        return sa - sb;
      });
    }
    return m;
  });

  const today = new Date();
</script>

<div class="border border-surface1 rounded overflow-hidden">
  <div class="grid grid-cols-7 bg-mantle text-xs text-dim">
    {#each ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'] as wd}
      <div class="px-2 py-1.5 border-b border-surface1">{wd}</div>
    {/each}
  </div>
  <div class="grid grid-cols-7 grid-rows-6 gap-px bg-surface1" style="height: 70vh">
    {#each cells as c}
      {@const isToday = isSameDay(c.date, today)}
      {@const events = eventsByDay.get(c.iso) ?? []}
      <div
        role="gridcell"
        class="p-1.5 overflow-hidden flex flex-col gap-0.5 hover:bg-surface0 transition-colors
          {c.inMonth ? 'bg-base' : 'bg-base opacity-50'}
          {isToday ? 'ring-1 ring-inset ring-primary/40' : ''}"
      >
        <button
          onclick={() => onClickDay(c.date)}
          class="text-left -mb-0.5 self-start"
          aria-label="open {c.iso}"
        >
          {#if isToday}
            <!-- Today gets a filled pill, matching the Google Calendar
                 visual cue. The pill is small enough to coexist with
                 the event chips below without crowding the cell. -->
            <span class="inline-flex items-center justify-center min-w-[20px] h-5 px-1.5 rounded-full bg-primary text-on-primary text-[11px] font-semibold">
              {c.date.getDate()}
            </span>
          {:else}
            <span class="text-xs text-subtext px-1">{c.date.getDate()}</span>
          {/if}
        </button>
        <div class="flex-1 space-y-0.5 overflow-hidden">
          {#each events.slice(0, 3) as ev}
            {@const col = eventTypeColor(ev)}
            <button
              onclick={() => onClickEvent(ev)}
              class="block w-full text-left text-[11px] px-1.5 py-0.5 rounded truncate"
              style="background: {col.bg}; color: {col.fg}; border-left: 2px solid {col.border}; {ev.done ? 'text-decoration: line-through; opacity: 0.7;' : ''}"
            >
              {ev.start ? fmtTime(new Date(ev.start)) + ' ' : ''}{ev.title}
            </button>
          {/each}
          {#if events.length > 3}
            <button
              onclick={() => onClickDay(c.date)}
              class="text-[10px] text-dim px-1.5 text-left hover:text-text"
            >+{events.length - 3} more</button>
          {/if}
        </div>
      </div>
    {/each}
  </div>
</div>
