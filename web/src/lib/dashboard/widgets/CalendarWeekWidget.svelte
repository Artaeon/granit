<script lang="ts">
  import { onMount } from 'svelte';
  import { api, fmtDateISO } from '$lib/api';
  import type { CalendarEvent } from '$lib/api';
  import { onWsEvent } from '$lib/ws';

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
  async function load() {
    const s = startOfWeek(new Date());
    const e = addDays(s, 6);
    const feed = await api.calendar(fmtDateISO(s), fmtDateISO(e));
    events = feed.events;
  }
  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') load();
    });
  });

  let week = $derived.by(() => {
    const s = startOfWeek(new Date());
    return Array.from({ length: 7 }, (_, i) => {
      const d = addDays(s, i);
      const iso = fmtDateISO(d);
      const evs = events.filter((e) => (e.date ?? (e.start ? e.start.slice(0, 10) : '')) === iso);
      return { date: d, iso, events: evs };
    });
  });

  const todayISO = fmtDateISO(new Date());
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-4">
  <div class="flex items-baseline justify-between mb-3">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">This week</h2>
    <a href="/calendar" class="text-xs text-secondary hover:underline">cal →</a>
  </div>
  <ul class="space-y-2">
    {#each week as d}
      {@const isToday = d.iso === todayISO}
      <li class="flex items-baseline gap-3">
        <a href="/calendar" class="w-20 flex-shrink-0 {isToday ? 'text-primary font-medium' : 'text-subtext'}">
          {d.date.toLocaleDateString(undefined, { weekday: 'short', day: 'numeric' })}
        </a>
        <span class="text-xs text-dim flex-1 truncate">
          {#if d.events.length === 0}—{:else}{d.events.length} event{d.events.length !== 1 ? 's' : ''}{/if}
        </span>
      </li>
    {/each}
  </ul>
</section>
