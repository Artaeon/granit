<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type CalendarEvent, fmtDateISO } from '$lib/api';
  import { onWsEvent } from '$lib/ws';

  // NowWidget answers "what's happening right now?" — current local
  // time + the upcoming event/scheduled task. Big and quiet so it sits
  // at the top of the dashboard like a focal point.

  let now = $state(new Date());
  let events = $state<CalendarEvent[]>([]);

  onMount(() => {
    const tick = setInterval(() => (now = new Date()), 30_000);
    load();
    const unsub = onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed' || ev.type === 'event.changed') load();
    });
    return () => { clearInterval(tick); unsub(); };
  });

  async function load() {
    try {
      const today = new Date();
      const inAWeek = new Date(today.getTime() + 7 * 24 * 60 * 60 * 1000);
      const r = await api.calendar(fmtDateISO(today), fmtDateISO(inAWeek));
      events = r.events;
    } catch {
      events = [];
    }
  }

  // The next event/task with a start time strictly in the future.
  // Tasks with `done` are excluded — already handled.
  let next = $derived.by(() => {
    const fut = events
      .filter((e) => e.start && new Date(e.start).getTime() > now.getTime() && !e.done)
      .map((e) => ({ ev: e, when: new Date(e.start!).getTime() }))
      .sort((a, b) => a.when - b.when);
    return fut[0]?.ev ?? null;
  });

  // The currently-happening event (start ≤ now ≤ end).
  let current = $derived.by(() => {
    return events.find((e) => {
      if (!e.start) return false;
      const s = new Date(e.start).getTime();
      const dur = e.durationMinutes ?? 30;
      const end = e.end ? new Date(e.end).getTime() : s + dur * 60_000;
      return s <= now.getTime() && now.getTime() <= end && !e.done;
    }) ?? null;
  });

  function fmtTime(d: Date): string {
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }
  function relTime(t: number): string {
    const mins = Math.round((t - now.getTime()) / 60_000);
    if (mins < 1) return 'now';
    if (mins < 60) return `in ${mins} min`;
    if (mins < 24 * 60) return `in ${Math.round(mins / 60)}h`;
    const days = Math.round(mins / (24 * 60));
    return `in ${days}d`;
  }

  function tone(t: string): string {
    if (t === 'event' || t === 'ics_event') return 'info';
    if (t === 'task_scheduled') return 'primary';
    if (t === 'task_due') return 'warning';
    return 'subtext';
  }
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-4">
  <div class="flex items-baseline gap-2 mb-3">
    <span class="text-2xl sm:text-3xl font-mono font-semibold text-text tabular-nums">{fmtTime(now)}</span>
    <span class="text-xs text-dim">{now.toLocaleDateString(undefined, { weekday: 'long', month: 'short', day: 'numeric' })}</span>
  </div>

  {#if current}
    <div class="rounded p-2.5 mb-2" style="background: color-mix(in srgb, var(--color-{tone(current.type)}) 14%, transparent); border-left: 3px solid var(--color-{tone(current.type)})">
      <div class="text-[10px] uppercase tracking-wider text-dim">happening now</div>
      <div class="text-sm text-text font-medium truncate">{current.title}</div>
      {#if current.location}<div class="text-xs text-dim truncate">@ {current.location}</div>{/if}
    </div>
  {/if}

  {#if next}
    <div>
      <div class="text-[10px] uppercase tracking-wider text-dim mb-1">up next</div>
      <div class="flex items-baseline gap-2">
        <span class="w-2 h-2 rounded-full flex-shrink-0" style="background: var(--color-{tone(next.type)})"></span>
        <span class="text-sm text-text flex-1 truncate">{next.title}</span>
        <span class="text-xs text-dim font-mono">{fmtTime(new Date(next.start!))}</span>
      </div>
      <div class="text-[11px] text-dim ml-4">{relTime(new Date(next.start!).getTime())}</div>
    </div>
  {:else if !current}
    <p class="text-sm text-dim italic">Nothing scheduled in the next week.</p>
  {/if}
</section>
