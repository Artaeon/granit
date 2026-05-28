<script lang="ts">
  // RightPaneCalendar — slim companion view of today + tomorrow's
  // calendar events. Fits in a 280-640px column, no horizontal
  // overflow. Compact rows: time | title | optional location.
  //
  // The /events endpoint doesn't take start/end params — it returns
  // every event in the vault. We filter to today + tomorrow on the
  // client. Recurring events are NOT expanded here — the full /calendar
  // page handles that. Phase 1 surfaces native one-off events only,
  // which is what the user creates 95% of the time. Recurring series
  // surface as their base date (so a weekly stand-up still appears
  // on the day it was first created, just without the right-pane
  // doing recurrence math itself).
  //
  // Refetch on WS event-touch events so dropping an event from the
  // calendar page updates the pane without a manual reload.

  import { onMount } from 'svelte';
  import { api, type CalendarEventEntry } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { todayISO, fmtDateISO } from '$lib/util/date';

  let events = $state<CalendarEventEntry[]>([]);
  let loading = $state(true);
  let error = $state(false);

  async function load() {
    try {
      const r = await api.listEvents();
      events = r.events ?? [];
      error = false;
    } catch {
      error = true;
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    void load();
    return onWsEvent((ev) => {
      if (ev.type === 'event.changed' || ev.type === 'event.removed') {
        void load();
      }
    });
  });

  // Two date strings (local time) so we can bucket events without
  // any timezone gymnastics — events store `date` as YYYY-MM-DD in
  // the same local frame.
  let today = $derived(todayISO());
  let tomorrow = $derived.by(() => {
    const d = new Date();
    d.setDate(d.getDate() + 1);
    return fmtDateISO(d);
  });

  let todayEvents = $derived.by(() =>
    events
      .filter((e) => e.date === today)
      .sort((a, b) => (a.start_time ?? '').localeCompare(b.start_time ?? ''))
  );
  let tomorrowEvents = $derived.by(() =>
    events
      .filter((e) => e.date === tomorrow)
      .sort((a, b) => (a.start_time ?? '').localeCompare(b.start_time ?? ''))
  );

  function fmtTime(ev: CalendarEventEntry): string {
    if (!ev.start_time) return 'all day';
    return ev.start_time.slice(0, 5);
  }
</script>

<div class="flex flex-col h-full text-sm">
  {#if loading}
    <div class="p-3 space-y-2">
      <div class="h-3 w-1/2 bg-surface1 rounded animate-pulse"></div>
      <div class="h-3 w-3/4 bg-surface1 rounded animate-pulse"></div>
      <div class="h-3 w-2/3 bg-surface1 rounded animate-pulse"></div>
    </div>
  {:else if error}
    <p class="p-3 text-dim italic text-xs">Couldn't load events.</p>
  {:else}
    <div class="flex-1 overflow-y-auto px-2 py-2 space-y-3">
      <section>
        <h3 class="px-2 pb-1 text-[10px] uppercase tracking-wider text-dim font-medium">Today</h3>
        {#if todayEvents.length === 0}
          <p class="px-2 py-1 text-xs text-dim italic">Nothing scheduled</p>
        {:else}
          <ul class="space-y-0.5">
            {#each todayEvents as ev (ev.id)}
              <li class="px-2 py-1.5 rounded hover:bg-surface0 transition-colors">
                <div class="flex items-baseline gap-2 min-w-0">
                  <span class="text-[11px] font-mono text-secondary tabular-nums flex-shrink-0 w-12">{fmtTime(ev)}</span>
                  <span class="text-xs text-text truncate flex-1" title={ev.title}>{ev.title}</span>
                </div>
                {#if ev.location}
                  <div class="ml-14 text-[10px] text-dim truncate" title={ev.location}>{ev.location}</div>
                {/if}
              </li>
            {/each}
          </ul>
        {/if}
      </section>

      <section>
        <h3 class="px-2 pb-1 text-[10px] uppercase tracking-wider text-dim font-medium">Tomorrow</h3>
        {#if tomorrowEvents.length === 0}
          <p class="px-2 py-1 text-xs text-dim italic">Nothing scheduled</p>
        {:else}
          <ul class="space-y-0.5">
            {#each tomorrowEvents as ev (ev.id)}
              <li class="px-2 py-1.5 rounded hover:bg-surface0 transition-colors">
                <div class="flex items-baseline gap-2 min-w-0">
                  <span class="text-[11px] font-mono text-secondary tabular-nums flex-shrink-0 w-12">{fmtTime(ev)}</span>
                  <span class="text-xs text-text truncate flex-1" title={ev.title}>{ev.title}</span>
                </div>
                {#if ev.location}
                  <div class="ml-14 text-[10px] text-dim truncate" title={ev.location}>{ev.location}</div>
                {/if}
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    </div>

    <footer class="border-t border-surface1 px-3 py-1.5 flex-shrink-0">
      <a href="/calendar" class="text-xs text-secondary hover:underline">Open Calendar →</a>
    </footer>
  {/if}
</div>
