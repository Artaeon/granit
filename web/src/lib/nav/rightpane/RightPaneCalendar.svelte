<script lang="ts">
  // RightPaneCalendar — slim companion view of today + tomorrow's
  // calendar events. Fits in a 280-640px column, no horizontal
  // overflow. Compact rows: time | title | optional location.
  //
  // We use api.calendar(from, to) — the expanded read-only feed that
  // includes ICS events, tasks-as-events and deadlines — not
  // api.listEvents() which only returns native (editable) entries
  // stored in events.json. The full /calendar page makes the same
  // choice (see "expanded read-only render view" comment there).
  //
  // Refetch on WS event-touch events so dropping an event from the
  // calendar page updates the pane without a manual reload. The feed
  // also contains task_scheduled / task_due / deadline rows, but
  // refreshing only on event.* is good enough — the user pivots back
  // through the pane after any edit anyway.
  //
  // Recurring series are expanded server-side inside the from/to
  // window, so the right pane doesn't do any recurrence math itself.

  import { onMount, onDestroy } from 'svelte';
  import { api, type CalendarEvent } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { todayISO, fmtDateISO } from '$lib/util/date';

  let events = $state<CalendarEvent[]>([]);
  let loading = $state(true);
  let error = $state(false);

  // Gen counter guards against stale resolves after rapid content
  // switching — see TaskDetail.svelte's loadSubtasks for the pattern.
  let loadGen = 0;
  let destroyed = false;

  async function load() {
    const myGen = ++loadGen;
    try {
      const from = todayISO();
      const tom = new Date();
      tom.setDate(tom.getDate() + 1);
      const to = fmtDateISO(tom);
      const r = await api.calendar(from, to);
      if (destroyed || myGen !== loadGen) return;
      events = r.events ?? [];
      error = false;
    } catch {
      if (destroyed || myGen !== loadGen) return;
      error = true;
    } finally {
      if (!destroyed && myGen === loadGen) loading = false;
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
  onDestroy(() => { destroyed = true; });

  // Two date strings (local time) so we can bucket events without
  // any timezone gymnastics — events store `date` as YYYY-MM-DD in
  // the same local frame, and timed `start` is RFC3339 whose first
  // 10 chars are the local date when the backend formats it.
  let today = $derived(todayISO());
  let tomorrow = $derived.by(() => {
    const d = new Date();
    d.setDate(d.getDate() + 1);
    return fmtDateISO(d);
  });

  // CalendarEvent has either `date` (all-day) or `start` (timed,
  // RFC3339). Some rows (ics_event timed) only carry `start`; we
  // bucket on whichever exists.
  function evDate(ev: CalendarEvent): string {
    if (ev.date) return ev.date;
    if (ev.start) return ev.start.slice(0, 10);
    return '';
  }

  // Stable per-row key — CalendarEvent has no `id`. eventId covers
  // native events, taskId covers task_due / task_scheduled, and
  // notePath+title is a reasonable last resort for ICS occurrences
  // (which carry source + title + start). Falling all the way back
  // to start/date keeps the key non-empty for synthetic rows.
  function evKey(ev: CalendarEvent): string {
    return (
      ev.eventId ??
      ev.taskId ??
      `${ev.notePath ?? ev.source ?? ''}|${ev.title}|${ev.start ?? ev.date ?? ''}`
    );
  }

  // Sort key for time-of-day. All-day rows (no start) sort first.
  function sortKey(ev: CalendarEvent): string {
    return ev.start ?? '';
  }

  let todayEvents = $derived.by(() =>
    events
      .filter((e) => evDate(e) === today)
      .sort((a, b) => sortKey(a).localeCompare(sortKey(b)))
  );
  let tomorrowEvents = $derived.by(() =>
    events
      .filter((e) => evDate(e) === tomorrow)
      .sort((a, b) => sortKey(a).localeCompare(sortKey(b)))
  );

  function fmtTime(ev: CalendarEvent): string {
    if (ev.date && !ev.start) return 'all day';
    if (ev.start) {
      const d = new Date(ev.start);
      return d.toLocaleTimeString([], {
        hour: '2-digit',
        minute: '2-digit',
        hour12: false
      });
    }
    return 'all day';
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
            {#each todayEvents as ev (evKey(ev))}
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
            {#each tomorrowEvents as ev (evKey(ev))}
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
