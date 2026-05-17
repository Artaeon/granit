<script lang="ts">
  import { onMount } from 'svelte';
  import { api, fmtDateISO } from '$lib/api';
  import type { CalendarEvent } from '$lib/api';
  import { onWsEvent } from '$lib/ws';

  // CalendarWeek — 7-day strip with the FIRST event title per day,
  // not just a count. "Mon: 09:00 Standup · +2" reads as a scannable
  // week-at-a-glance; "Mon: 3 events" was just noise.

  function startOfWeek(d: Date): Date {
    const x = new Date(d);
    x.setDate(d.getDate() - d.getDay());
    x.setHours(0, 0, 0, 0);
    return x;
  }
  function addDays(d: Date, n: number): Date {
    const x = new Date(d);
    x.setDate(d.getDate() + n);
    return x;
  }

  let events = $state<CalendarEvent[]>([]);
  let loaded = $state(false);
  async function load() {
    const s = startOfWeek(new Date());
    const e = addDays(s, 6);
    const feed = await api.calendar(fmtDateISO(s), fmtDateISO(e));
    events = feed.events;
    loaded = true;
  }
  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') load();
    });
  });

  function timeOf(e: CalendarEvent): string {
    if (!e.start) return '';
    // No Date() round-trip for floating wall-clock strings — same
    // shape the TodayStream widget uses, kept simple here since we
    // only need a HH:MM hint.
    if (!e.start.endsWith('Z') && !/[+-]\d{2}:\d{2}$/.test(e.start.slice(-6))) {
      return e.start.slice(11, 16);
    }
    const d = new Date(e.start);
    return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
  }

  let week = $derived.by(() => {
    const s = startOfWeek(new Date());
    return Array.from({ length: 7 }, (_, i) => {
      const d = addDays(s, i);
      const iso = fmtDateISO(d);
      const evs = events
        .filter((e) => (e.date ?? (e.start ? e.start.slice(0, 10) : '')) === iso)
        // Real events first, calendar-feed task rows are skipped — the
        // dashboard's tasks already live in TodayStream / TodayTasks
        // so doubling them here just clutters the week strip.
        .filter((e) => e.type === 'event' || e.type === 'ics_event');
      const timed = evs.filter((e) => e.start).map((e) => ({ e, t: timeOf(e) }))
        .sort((a, b) => a.t.localeCompare(b.t));
      const first = timed[0] ?? (evs[0] ? { e: evs[0], t: '' } : null);
      return { date: d, iso, events: evs, first };
    });
  });

  const todayISO = fmtDateISO(new Date());
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-3">
  <div class="flex items-baseline justify-between mb-2">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">This week</h2>
    <a href="/calendar" class="text-xs text-secondary hover:underline">cal →</a>
  </div>
  {#if !loaded}
    <ul class="space-y-1.5">
      {#each [0,1,2,3,4] as i (i)}
        <li class="flex items-baseline gap-2 py-0.5">
          <span class="w-14 h-3 bg-surface1 rounded animate-pulse"></span>
          <span class="flex-1 h-3 bg-surface1 rounded animate-pulse"></span>
        </li>
      {/each}
    </ul>
  {:else}
    <ul class="space-y-0.5">
      {#each week as d}
        {@const isToday = d.iso === todayISO}
        {@const isPast = d.iso < todayISO}
        <li class="flex items-baseline gap-2 py-0.5 px-1 -mx-1 rounded {isToday ? 'bg-surface1' : ''} {isPast ? 'opacity-40' : ''}">
          <a href="/calendar" class="w-14 flex-shrink-0 text-xs tabular-nums {isToday ? 'text-primary font-medium' : 'text-subtext'}">
            {d.date.toLocaleDateString(undefined, { weekday: 'short', day: 'numeric' })}
          </a>
          <span class="text-xs flex-1 truncate min-w-0">
            {#if d.first}
              {#if d.first.t}<span class="font-mono tabular-nums text-dim mr-1.5">{d.first.t}</span>{/if}<span class="text-text">{d.first.e.title}</span>
              {#if d.events.length > 1}<span class="text-dim"> · +{d.events.length - 1}</span>{/if}
            {:else}
              <span class="text-dim">—</span>
            {/if}
          </span>
        </li>
      {/each}
    </ul>
  {/if}
</section>
