<script lang="ts">
  import type { CalendarEvent } from '$lib/api';
  import { addDays, eventDayKey, eventTypeColor, fmtDateISO, fmtTime, isAllDay, isSameDay, startOfMonth } from './utils';

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

  // Per-day sort: all-day rows first (so they read like a banner at
  // the top of the cell), then timed events by start. Stable on equal
  // start times — preserves the feed's emit order so two 09:00 events
  // don't jitter between renders.
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
        const aAll = isAllDay(a) ? 1 : 0;
        const bAll = isAllDay(b) ? 1 : 0;
        if (aAll !== bAll) return bAll - aAll; // all-day first
        const sa = a.start ? new Date(a.start).getTime() : 0;
        const sb = b.start ? new Date(b.start).getTime() : 0;
        return sa - sb;
      });
    }
    return m;
  });

  // Build a tooltip string with the full title and times — the chip
  // body is intentionally narrow (just a colored bar + clipped title)
  // so the hover-title gives the user the full information without
  // forcing the cell to grow.
  function chipTooltip(ev: CalendarEvent): string {
    const t = ev.title;
    const r = ev.rrule ? ' (recurring)' : '';
    if (isAllDay(ev)) return `${t}${r} · all-day`;
    if (!ev.start) return `${t}${r}`;
    const s = new Date(ev.start);
    const e = ev.end ? new Date(ev.end) : (ev.durationMinutes ? new Date(s.getTime() + ev.durationMinutes * 60_000) : null);
    return e ? `${t}${r} · ${fmtTime(s)}–${fmtTime(e)}` : `${t}${r} · ${fmtTime(s)}`;
  }

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
        <div class="flex-1 space-y-px overflow-hidden">
          {#each events.slice(0, 3) as ev}
            {@const col = eventTypeColor(ev)}
            {@const allDay = isAllDay(ev)}
            <button
              onclick={() => onClickEvent(ev)}
              class="group/chip flex items-center gap-1 w-full text-left text-[11px] leading-tight px-1 py-px rounded-sm truncate hover:opacity-90"
              style={allDay
                ? `background: ${col.bg}; color: ${col.fg}; ${ev.done ? 'text-decoration: line-through; opacity: 0.7;' : ''}`
                : `color: var(--color-text); ${ev.done ? 'text-decoration: line-through; opacity: 0.7;' : ''}`}
              title={chipTooltip(ev)}
            >
              {#if allDay}
                {#if ev.rrule}<span class="opacity-70 text-[9px]" aria-hidden="true">↻</span>{/if}
                <span class="truncate flex-1">{ev.title}</span>
              {:else}
                <!-- Timed event: a tiny colored dot stands in for the
                     bar/start-time. Reads like Google Calendar's month
                     grid — visual weight goes to the title, not the
                     time, since the time is one hover away. -->
                <span
                  class="w-1.5 h-1.5 rounded-full flex-shrink-0"
                  style="background: {col.border}"
                  aria-hidden="true"
                ></span>
                {#if ev.rrule}<span class="opacity-50 text-[9px] flex-shrink-0" aria-hidden="true">↻</span>{/if}
                <span class="truncate flex-1 text-subtext group-hover/chip:text-text">{ev.title}</span>
              {/if}
            </button>
          {/each}
          {#if events.length > 3}
            <button
              onclick={() => onClickDay(c.date)}
              class="text-[10px] text-dim px-1 text-left hover:text-text"
            >+{events.length - 3} more</button>
          {/if}
        </div>
      </div>
    {/each}
  </div>
</div>
