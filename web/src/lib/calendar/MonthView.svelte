<script lang="ts">
  import type { CalendarEvent } from '$lib/api';
  import { addDays, eventDayKey, eventTypeColor, fmtDateISO, fmtTime, isAllDay, isSameDay, startOfMonth } from './utils';

  let {
    cursor,
    events,
    density = 'comfy',
    onClickEvent,
    onClickDay
  }: {
    cursor: Date;
    events: CalendarEvent[];
    /** comfy = 3 chips/cell with bigger text · compact = 6 chips/cell
     *  with tighter chips. Defaults to comfy when the prop is omitted
     *  so existing call sites keep their previous behaviour. */
    density?: 'comfy' | 'compact';
    onClickEvent: (ev: CalendarEvent) => void;
    onClickDay: (d: Date) => void;
  } = $props();

  // Per-cell event budget. Compact density buys two more visible chips
  // at the cost of slightly smaller text — useful for busy months.
  const maxChips = $derived(density === 'compact' ? 6 : 3);
  const chipText = $derived(density === 'compact' ? 'text-[10px]' : 'text-[11px]');

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

  // "+N more" popover state. We render at most one popover at a time
  // — anchored to the clicked cell by an absolute-positioned bubble
  // inside the cell, so it tracks viewport scroll without floating-ui.
  // Clicking outside / pressing Escape closes it. Clicking an event
  // inside the popover fires onClickEvent (same as a chip) so the
  // detail-panel UX stays consistent with the rest of the grid.
  let popoverIso = $state<string | null>(null);
  function openPopover(iso: string, e: Event) {
    e.stopPropagation();
    popoverIso = popoverIso === iso ? null : iso;
  }
  function closePopover() { popoverIso = null; }
  function onPopoverKey(e: KeyboardEvent) {
    if (e.key === 'Escape') closePopover();
  }

  const today = new Date();
</script>

<!-- Outer wrapper keeps a rounded border around the entire grid.
     We use `overflow-visible` on this container (and on the cells)
     so the per-cell "+N more" popovers can spill above the grid
     without being clipped — the cells render the popover absolute-
     positioned inside themselves so it stays anchored on scroll. -->
{#if popoverIso}
  <!-- Transparent backdrop catches outside clicks. Sits below the
       popover (z-20 vs z-30) but above the grid so a click on any
       OTHER cell closes the current popover instead of jumping to
       that cell. -->
  <button
    class="fixed inset-0 z-20 cursor-default"
    aria-label="close popover"
    onclick={closePopover}
  ></button>
{/if}
<div class="border border-surface1 rounded overflow-visible relative" onkeydown={onPopoverKey} role="presentation">
  <div class="grid grid-cols-7 bg-mantle text-xs text-dim rounded-t">
    {#each ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'] as wd}
      <div class="px-2 py-1.5 border-b border-surface1">{wd}</div>
    {/each}
  </div>
  <div class="grid grid-cols-7 grid-rows-6 gap-px bg-surface1" style="height: 70vh">
    {#each cells as c, idx}
      {@const isToday = isSameDay(c.date, today)}
      {@const events = eventsByDay.get(c.iso) ?? []}
      <!-- Edge-aware popover anchoring. The 42-cell grid is laid out
           in 6 rows × 7 columns. For columns 4..6 (the right half of
           the week), anchor the popover to the cell's right edge so it
           doesn't spill past the viewport on a small laptop. For the
           last two rows (idx >= 28), anchor it to the cell's bottom so
           it doesn't sink under the page chrome. -->
      {@const gridCol = idx % 7}
      {@const gridRow = Math.floor(idx / 7)}
      {@const popoverPos =
        (gridCol >= 4 ? 'right-1' : 'left-1') + ' ' + (gridRow >= 4 ? 'bottom-7' : 'top-7')}
      <div
        role="gridcell"
        class="relative p-1.5 overflow-visible flex flex-col gap-0.5 hover:bg-surface0 transition-colors
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
          {#each events.slice(0, maxChips) as ev}
            {@const col = eventTypeColor(ev)}
            {@const allDay = isAllDay(ev)}
            <button
              onclick={() => onClickEvent(ev)}
              class="group/chip flex items-center gap-1 w-full text-left {chipText} leading-tight px-1 py-px rounded-sm truncate hover:opacity-90"
              style={allDay
                ? `background: ${col.bg}; color: ${col.fg}; ${ev.done ? 'text-decoration: line-through; opacity: 0.7;' : ''}`
                : `color: var(--color-text); ${ev.done ? 'text-decoration: line-through; opacity: 0.7;' : ''}`}
              title={chipTooltip(ev)}
            >
              {#if ev.kind === 'content'}
                <!-- Content accent: a thin lavender left-edge band so
                     content events are glanceable in mixed month grids
                     without extra chrome. Status letter (I/D/R/S/P/A)
                     surfaces on hover/focus only — keeps the rest state
                     uncluttered for users with packed days. -->
                <span class="w-0.5 self-stretch rounded-sm flex-shrink-0" style="background: var(--color-lavender)" aria-hidden="true"></span>
                {#if ev.status}
                  <span
                    class="text-[9px] font-bold uppercase tabular-nums opacity-0 group-hover/chip:opacity-100 transition-opacity flex-shrink-0 text-lavender"
                    title={`Status: ${ev.status}`}
                    aria-label={`Status: ${ev.status}`}
                  >{ev.status.charAt(0)}</span>
                {/if}
              {/if}
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
          {#if events.length > maxChips}
            <button
              onclick={(e) => openPopover(c.iso, e)}
              class="text-[10px] text-dim px-1 text-left hover:text-text"
              aria-expanded={popoverIso === c.iso}
              aria-label="show all {events.length} events on {c.iso}"
            >+{events.length - maxChips} more</button>
          {/if}
        </div>
        {#if popoverIso === c.iso}
          <!-- Anchored popover — sits inside the cell so it tracks
               the grid as it scrolls. Width is fixed; horizontal
               offset is auto so the browser keeps it in viewport on
               right-edge cells. Click backdrop / Escape close it. -->
          <div
            class="absolute z-30 {popoverPos} w-56 max-h-72 overflow-y-auto bg-mantle border border-surface1 rounded shadow-lg p-2 space-y-0.5"
            role="dialog"
            aria-label="events on {c.iso}"
            onclick={(e) => e.stopPropagation()}
            onkeydown={onPopoverKey}
            tabindex="-1"
          >
            <div class="flex items-center justify-between mb-1 px-0.5">
              <span class="text-[11px] text-subtext font-medium">
                {c.date.toLocaleDateString(undefined, { weekday: 'long', month: 'short', day: 'numeric' })}
              </span>
              <button
                onclick={closePopover}
                aria-label="close"
                class="text-dim hover:text-text leading-none w-4 h-4 flex items-center justify-center text-xs"
              >×</button>
            </div>
            {#each events as ev}
              {@const col = eventTypeColor(ev)}
              {@const allDay = isAllDay(ev)}
              <button
                onclick={() => { closePopover(); onClickEvent(ev); }}
                class="flex items-center gap-1.5 w-full text-left px-1.5 py-1 rounded hover:bg-surface0 text-[11px] leading-tight"
                title={chipTooltip(ev)}
              >
                <span
                  class="w-1.5 h-1.5 rounded-full flex-shrink-0"
                  style="background: {col.border}"
                  aria-hidden="true"
                ></span>
                {#if !allDay && ev.start}
                  <span class="font-mono text-[10px] text-dim w-9 flex-shrink-0">{fmtTime(new Date(ev.start))}</span>
                {:else}
                  <span class="font-mono text-[10px] text-dim w-9 flex-shrink-0">all-day</span>
                {/if}
                <span class="truncate flex-1 text-text {ev.done ? 'line-through opacity-60' : ''}">{ev.title}</span>
                {#if ev.rrule}<span class="text-dim text-[9px] flex-shrink-0" aria-hidden="true">↻</span>{/if}
              </button>
            {/each}
            <button
              onclick={() => { closePopover(); onClickDay(c.date); }}
              class="block w-full text-left text-[10px] text-dim px-1.5 py-1 hover:text-text border-t border-surface1 mt-1 pt-1.5"
            >Open {c.date.toLocaleDateString(undefined, { weekday: 'short' })} in day view →</button>
          </div>
        {/if}
      </div>
    {/each}
  </div>
</div>
