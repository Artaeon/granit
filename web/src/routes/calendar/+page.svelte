<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, type CalendarEvent, type CalendarFeed, type CalendarSource } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import {
    addDays,
    endOfWeek,
    fmtDateISO,
    sourceColorToken,
    startOfMonth,
    startOfWeek
  } from '$lib/calendar/utils';
  import HourGrid from '$lib/calendar/HourGrid.svelte';
  import MonthView from '$lib/calendar/MonthView.svelte';
  import AgendaView from '$lib/calendar/AgendaView.svelte';
  import YearView from '$lib/calendar/YearView.svelte';
  import MiniMonth from '$lib/calendar/MiniMonth.svelte';
  import EventDetail from '$lib/calendar/EventDetail.svelte';
  import QuickCreateScheduled from '$lib/calendar/QuickCreateScheduled.svelte';
  import CreateEvent from '$lib/calendar/CreateEvent.svelte';
  import UnifiedCreate from '$lib/calendar/UnifiedCreate.svelte';
  import TaskBacklog from '$lib/calendar/TaskBacklog.svelte';
  import Drawer from '$lib/components/Drawer.svelte';
  import { onWsEvent } from '$lib/ws';
  import { dragStore } from '$lib/calendar/dragStore';
  import { onDestroy } from 'svelte';

  type View = 'day' | '3day' | 'week' | 'month' | 'year' | 'agenda';

  // Persisted last-used view (per device).
  const VIEW_KEY = 'granit.calendar.view';
  let view = $state<View>(
    (typeof localStorage !== 'undefined' && (localStorage.getItem(VIEW_KEY) as View)) || 'week'
  );
  let cursor = $state(new Date());

  // Plan mode — splits the page into a left-side backlog of today's
  // open tasks and the regular hour grid. Persisted so the user's
  // choice carries across sessions / devices (per-device localStorage).
  // Plan mode is single-day in v1: turning it on forces view='day' so
  // the layout stays sensible (a 7-column week grid + side rail is too
  // tight on most screens).
  const PLAN_KEY = 'granit.calendar.planmode';
  let planMode = $state<boolean>(
    typeof localStorage !== 'undefined' && localStorage.getItem(PLAN_KEY) === '1'
  );

  $effect(() => {
    if (typeof localStorage !== 'undefined') {
      try { localStorage.setItem(VIEW_KEY, view); } catch {}
    }
  });

  $effect(() => {
    if (typeof localStorage === 'undefined') return;
    try { localStorage.setItem(PLAN_KEY, planMode ? '1' : '0'); } catch {}
  });

  function togglePlanMode() {
    planMode = !planMode;
    if (planMode) view = 'day';
  }

  // Clean exit on route change: drop any pending drag pick. Otherwise
  // a stale dragStore could corrupt the next page's pointer behaviour.
  onDestroy(() => dragStore.set(null));

  let feed = $state<CalendarFeed | null>(null);
  let loading = $state(false);

  let fetchFrom = $state(addDays(new Date(), -7));
  let fetchTo = $state(addDays(new Date(), 60));

  let selected = $state<CalendarEvent | null>(null);
  let detailOpen = $state(false);

  let createOpen = $state(false);
  let createDate = $state(new Date());
  let createHour = $state(9);
  let createMinute = $state(0);

  let createEventOpen = $state(false);
  let createEventDate = $state(new Date());

  let unifiedOpen = $state(false);
  let unifiedStart = $state(new Date());
  let unifiedEnd = $state(new Date());
  let unifiedKind = $state<'task' | 'event'>('task');

  let filterDrawerOpen = $state(false);
  let isMobile = $state(false);

  // Event-type filter: each toggle hides events of that type. Persisted so
  // the user's preference (e.g. "always hide ICS") sticks across sessions.
  type EventFilterKey = 'daily' | 'task_due' | 'task_scheduled' | 'event' | 'ics_event';
  const FILTER_KEY = 'granit.calendar.filters';
  let hidden = $state<Set<EventFilterKey>>(new Set());

  if (typeof localStorage !== 'undefined') {
    try {
      const raw = localStorage.getItem(FILTER_KEY);
      if (raw) hidden = new Set(JSON.parse(raw) as EventFilterKey[]);
    } catch {}
  }

  $effect(() => {
    if (typeof localStorage === 'undefined') return;
    try { localStorage.setItem(FILTER_KEY, JSON.stringify(Array.from(hidden))); } catch {}
  });

  function toggleType(t: EventFilterKey) {
    const next = new Set(hidden);
    if (next.has(t)) next.delete(t);
    else next.add(t);
    hidden = next;
  }

  // Per-source ICS toggles. Wired to granit's `disabled_calendars` list
  // (config.json) so flipping one here also silences it in the TUI on
  // next launch — single source of truth.
  let calSources = $state<CalendarSource[]>([]);
  let savingSources = $state(false);

  async function loadSources() {
    try {
      const r = await api.listCalendarSources();
      calSources = r.sources;
    } catch {
      // No vault access yet, or no .ics files — silently skip.
      calSources = [];
    }
  }

  async function toggleSource(src: CalendarSource) {
    // Snapshot the desired post-toggle state up front so subsequent
    // logic doesn't depend on `src.enabled` mutating mid-flight (the
    // `src` we received is a proxy backed by `calSources`; if anything
    // races with this handler the read could observe the wrong value).
    const targetId = src.id;
    const wasEnabled = src.enabled;
    const willBeEnabled = !wasEnabled;
    savingSources = true;
    // Optimistic flip — write a NEW array (not an in-place mutation
    // of the existing proxy), so Svelte's keyed each block re-runs
    // its bindings with the right `enabled` value before the network
    // round-trip completes. Without this the user saw the toggle stay
    // on its old visual state until the second click triggered a
    // re-render via the response replace, hence "click twice".
    calSources = calSources.map((s) =>
      s.id === targetId ? { ...s, enabled: willBeEnabled } : s
    );
    // Compute the new `disabled` list from the OPTIMISTIC state so
    // we send the canonical set the user just expressed intent for.
    const newDisabled = calSources.filter((s) => !s.enabled).map((s) => s.source);
    try {
      const r = await api.patchCalendarSources(newDisabled);
      // Replace with the server's authoritative shape (always a fresh
      // array reference — guards against any future shape drift).
      calSources = [...r.sources];
      await load(); // refresh feed with new disabled list
    } catch (e) {
      // Roll the optimistic flip back so the UI matches what the
      // server still has on disk.
      calSources = calSources.map((s) =>
        s.id === targetId ? { ...s, enabled: wasEnabled } : s
      );
      toast.error('save failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      savingSources = false;
    }
  }

  onMount(() => {
    const mq = window.matchMedia('(max-width: 767px)');
    isMobile = mq.matches;
    if (mq.matches) view = 'day';
    const handler = (e: MediaQueryListEvent) => (isMobile = e.matches);
    mq.addEventListener('change', handler);
    return () => mq.removeEventListener('change', handler);
  });

  async function load() {
    if (!$auth) return;
    loading = true;
    try {
      if (cursor < fetchFrom || cursor > fetchTo) {
        fetchFrom = addDays(cursor, -14);
        fetchTo = addDays(cursor, 60);
      }
      feed = await api.calendar(fmtDateISO(fetchFrom), fmtDateISO(fetchTo));
    } finally {
      loading = false;
    }
  }

  onMount(load);
  onMount(loadSources);
  onMount(() =>
    onWsEvent((ev) => {
      if (
        ev.type === 'note.changed' ||
        ev.type === 'note.removed' ||
        ev.type === 'event.changed' ||
        ev.type === 'event.removed'
      ) load();
    })
  );

  $effect(() => {
    void cursor;
    void view;
    load();
  });

  let allEvents = $derived(feed?.events ?? []);
  let events = $derived(allEvents.filter((e) => !hidden.has(e.type as EventFilterKey)));
  let typeCounts = $derived.by(() => {
    const c: Record<string, number> = {};
    for (const e of allEvents) c[e.type] = (c[e.type] ?? 0) + 1;
    return c;
  });

  let viewDays = $derived.by(() => {
    if (view === 'day') return [cursor];
    if (view === '3day') return Array.from({ length: 3 }, (_, i) => addDays(cursor, i));
    if (view === 'week') {
      const s = startOfWeek(cursor);
      return Array.from({ length: 7 }, (_, i) => addDays(s, i));
    }
    return [];
  });

  let monthEvents = $derived.by(() => {
    const ms = startOfMonth(cursor);
    const me = new Date(ms.getFullYear(), ms.getMonth() + 1, 0);
    return events.filter((ev) => {
      const key = ev.date ?? (ev.start ? ev.start.slice(0, 10) : '');
      if (!key) return false;
      return key >= fmtDateISO(ms) && key <= fmtDateISO(me);
    });
  });

  function prev() {
    if (view === 'day') cursor = addDays(cursor, -1);
    else if (view === '3day') cursor = addDays(cursor, -3);
    else if (view === 'week') cursor = addDays(cursor, -7);
    else if (view === 'month') cursor = new Date(cursor.getFullYear(), cursor.getMonth() - 1, 1);
    else if (view === 'year') cursor = new Date(cursor.getFullYear() - 1, cursor.getMonth(), 1);
    else cursor = addDays(cursor, -7);
  }
  function next() {
    if (view === 'day') cursor = addDays(cursor, 1);
    else if (view === '3day') cursor = addDays(cursor, 3);
    else if (view === 'week') cursor = addDays(cursor, 7);
    else if (view === 'month') cursor = new Date(cursor.getFullYear(), cursor.getMonth() + 1, 1);
    else if (view === 'year') cursor = new Date(cursor.getFullYear() + 1, cursor.getMonth(), 1);
    else cursor = addDays(cursor, 7);
  }
  function gotoToday() { cursor = new Date(); }

  // Keyboard shortcuts. Active only when nothing else has focus (so we
  // don't steal keystrokes from the create-event modal's inputs).
  // Mirrors Google Calendars default bindings so muscle memory carries
  // over: t = today, j/n = next, k/p = prev, d/w/m/y/a = view, ? = help.
  function isTextField(el: EventTarget | null): boolean {
    if (!(el instanceof HTMLElement)) return false;
    const tag = el.tagName;
    return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || el.isContentEditable;
  }

  let showShortcutHelp = $state(false);

  function onKeydown(e: KeyboardEvent) {
    if (isTextField(e.target)) return;
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    // Don't fight the create / detail drawers — they own their own
    // keyboard surface (Escape to close, Enter to submit).
    if (createOpen || createEventOpen || unifiedOpen || detailOpen) return;
    switch (e.key) {
      case 't': gotoToday(); break;
      case 'j': case 'n': next(); break;
      case 'k': case 'p': prev(); break;
      case 'd': view = 'day'; break;
      case 'w': view = 'week'; break;
      case 'x': view = '3day'; break; // m is taken by month
      case 'm': view = 'month'; break;
      case 'y': view = 'year'; break;
      case 'a': view = 'agenda'; break;
      case '?': showShortcutHelp = !showShortcutHelp; break;
      default: return;
    }
    e.preventDefault();
  }

  // Touch swipe to navigate. Triggered on the main grid container; a
  // horizontal swipe of >60px (with vertical movement <40px so we dont
  // hijack scroll) counts. Mobile users can flick between weeks the
  // same way they would on Google Calendar / iOS Calendar.
  let touchStartX = 0;
  let touchStartY = 0;
  let touchActive = false;
  function onTouchStart(e: TouchEvent) {
    if (e.touches.length !== 1) { touchActive = false; return; }
    touchStartX = e.touches[0].clientX;
    touchStartY = e.touches[0].clientY;
    touchActive = true;
  }
  function onTouchEnd(e: TouchEvent) {
    if (!touchActive) return;
    touchActive = false;
    if (e.changedTouches.length !== 1) return;
    const dx = e.changedTouches[0].clientX - touchStartX;
    const dy = e.changedTouches[0].clientY - touchStartY;
    if (Math.abs(dy) > 40) return; // mostly vertical → let scroll happen
    if (Math.abs(dx) < 60) return; // too short
    if (dx > 0) prev(); else next();
  }

  function clickEvent(ev: CalendarEvent) { selected = ev; detailOpen = true; }
  function clickSlot(date: Date, hour: number, minute: number) {
    createDate = date;
    createHour = hour;
    createMinute = minute;
    createOpen = true;
  }

  function onSlotRange(start: Date, end: Date) {
    unifiedStart = start;
    unifiedEnd = end;
    unifiedKind = 'task';
    unifiedOpen = true;
  }
  async function reschedule(taskId: string, newStart: Date) {
    try {
      await api.patchTask(taskId, { scheduledStart: newStart.toISOString() });
      await load();
    } catch (e) {
      console.error('reschedule failed', e);
    }
  }
  async function dropTask(id: string, start: Date, dur: number) {
    try {
      await api.patchTask(id, {
        scheduledStart: start.toISOString(),
        durationMinutes: dur
      });
      await load();
      toast.success('scheduled');
    } catch (e) {
      toast.error('schedule failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }
  async function resizeTask(taskId: string, durationMinutes: number) {
    try {
      await api.patchTask(taskId, { durationMinutes });
      await load();
    } catch (e) {
      console.error('resize failed', e);
    }
  }
  function clickDay(d: Date) { cursor = d; view = 'day'; }
  function pickDay(d: Date) { cursor = d; filterDrawerOpen = false; }

  let headline = $derived.by(() => {
    if (view === 'day') return cursor.toLocaleDateString(undefined, { weekday: isMobile ? 'short' : 'long', month: 'short', day: 'numeric' });
    if (view === '3day') {
      const e = addDays(cursor, 2);
      return `${cursor.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })} – ${e.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}`;
    }
    if (view === 'week') {
      const s = startOfWeek(cursor);
      const e = endOfWeek(cursor);
      if (s.getMonth() === e.getMonth()) {
        return `${s.toLocaleDateString(undefined, { month: 'short' })} ${s.getDate()}–${e.getDate()}`;
      }
      return `${s.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })} – ${e.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}`;
    }
    if (view === 'month') return cursor.toLocaleDateString(undefined, { month: 'long', year: 'numeric' });
    if (view === 'year') return String(cursor.getFullYear());
    return 'Agenda';
  });
</script>

{#snippet sidebarContent()}
  <div class="p-3 space-y-4">
    <!-- Single create button — opens the unified modal where the user can
         flip between task / event. The modal seeds itself with an hour
         slot starting at the next round hour. -->
    <button
      onclick={() => {
        const s = new Date();
        s.setMinutes(0, 0, 0);
        s.setHours(s.getHours() + 1);
        const e = new Date(s.getTime() + 60 * 60 * 1000);
        unifiedStart = s; unifiedEnd = e; unifiedKind = 'task'; unifiedOpen = true;
        filterDrawerOpen = false;
      }}
      class="w-full px-3 py-2.5 bg-primary text-mantle rounded text-sm font-medium hover:opacity-90"
    >
      + New task or event
    </button>
    <p class="text-[11px] text-dim italic px-1 -mt-2">…or click + drag on the grid</p>
    <MiniMonth cursor={cursor} selected={cursor} events={monthEvents} onPick={pickDay} />

    <!-- Per-file ICS source toggles — same `disabled_calendars` config
         the TUI uses, so flipping a switch here silences the file in
         both frontends. The most common reason to toggle: vaults often
         carry both per-source files (training.ics, faith.ics) and a
         merged.ics that combines them — disable one side. -->
    {#if calSources.length > 0}
      <div class="space-y-1 text-xs">
        <h3 class="text-dim uppercase tracking-wider mb-2">Calendar sources</h3>
        {#each calSources as s (s.id)}
          {@const tone = sourceColorToken(s.source)}
          <button
            onclick={() => toggleSource(s)}
            disabled={savingSources}
            class="w-full flex items-center gap-2 px-2 py-1 rounded hover:bg-surface0 {s.enabled ? '' : 'opacity-40'}"
            title="{s.path}{s.enabled ? '' : ' (disabled)'}"
          >
            <!-- Color dot matches the per-source rotation used on the
                 grid, so the legend doubles as a visual key for which
                 source a given event chip came from. Checkmark
                 overlaid on the dot when enabled. -->
            <span class="w-3 h-3 rounded-sm border flex items-center justify-center flex-shrink-0"
              style="border-color: var(--color-{tone}); background: {s.enabled ? `var(--color-${tone})` : 'transparent'}">
              {#if s.enabled}
                <svg viewBox="0 0 12 12" class="w-2.5 h-2.5 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
              {/if}
            </span>
            <span class="text-subtext flex-1 text-left truncate">{s.source}</span>
            {#if s.folder}<span class="text-dim text-[10px]">{s.folder}</span>{/if}
          </button>
        {/each}
        <p class="text-[10px] text-dim italic px-2 pt-1">colors match each source's events on the grid · syncs with granit TUI's <code>disabled_calendars</code></p>
      </div>
    {/if}

    <!-- Filters: each row toggles visibility of one event type. Counts are
         live across the loaded range so the user can see which buckets are
         empty before hiding them. -->
    <div class="space-y-1 text-xs">
      <h3 class="text-dim uppercase tracking-wider mb-2">Filters</h3>
      {#each [
        { key: 'daily', label: 'Daily note', tone: 'secondary' },
        { key: 'task_scheduled', label: 'Scheduled task', tone: 'primary' },
        { key: 'task_due', label: 'Task due', tone: 'warning' },
        { key: 'event', label: 'Event', tone: 'info' },
        { key: 'ics_event', label: 'ICS calendars', tone: 'info' }
      ] as f}
        {@const isHidden = hidden.has(f.key as EventFilterKey)}
        <button
          onclick={() => toggleType(f.key as EventFilterKey)}
          class="w-full flex items-center gap-2 px-2 py-1 rounded hover:bg-surface0 {isHidden ? 'opacity-40' : ''}"
        >
          <span class="w-2 h-2 rounded-full" style="background: var(--color-{f.tone})"></span>
          <span class="text-subtext flex-1 text-left">{f.label}</span>
          <span class="text-dim">{typeCounts[f.key] ?? 0}</span>
          {#if isHidden}<span class="text-dim text-[10px]">hidden</span>{/if}
        </button>
      {/each}
    </div>
  </div>
{/snippet}

<div class="flex h-full">
  <!-- Desktop sidebar -->
  <aside class="hidden md:block md:w-56 lg:w-64 border-r border-surface1 bg-mantle/50 flex-shrink-0 overflow-y-auto">
    {@render sidebarContent()}
  </aside>

  <!-- Mobile drawer -->
  <Drawer bind:open={filterDrawerOpen} side="left">
    {@render sidebarContent()}
  </Drawer>

  <div class="flex-1 flex flex-col min-w-0">
    <header class="flex items-center gap-1 sm:gap-2 px-3 py-2 border-b border-surface1 flex-shrink-0 flex-wrap">
      <button
        onclick={() => (filterDrawerOpen = true)}
        aria-label="filters"
        class="md:hidden w-9 h-9 flex items-center justify-center text-subtext hover:bg-surface0 rounded"
      >
        <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M3 6h18M6 12h12M9 18h6" stroke-linecap="round" />
        </svg>
      </button>
      <button onclick={gotoToday} class="px-2.5 py-1.5 text-sm bg-surface0 border border-surface1 rounded hover:border-primary">today</button>
      <button onclick={prev} aria-label="prev" class="w-8 h-8 flex items-center justify-center text-sm bg-surface0 border border-surface1 rounded hover:border-primary">‹</button>
      <button onclick={next} aria-label="next" class="w-8 h-8 flex items-center justify-center text-sm bg-surface0 border border-surface1 rounded hover:border-primary">›</button>
      <h2 class="text-sm sm:text-base text-text font-medium truncate">{headline}</h2>
      <span class="flex-1"></span>
      {#if loading}<span class="hidden sm:inline text-xs text-dim">loading…</span>{/if}
      <!-- Plan mode pill — distinct fill (secondary accent) when active
           so the user can see at a glance that scheduling-by-drag is
           live. Forces day view when toggled on (the side-rail layout
           collapses week-views below useful width). -->
      <button
        onclick={togglePlanMode}
        class="px-2.5 py-1.5 text-xs sm:text-sm rounded border flex items-center gap-1 transition-colors
          {planMode
            ? 'bg-secondary text-mantle border-secondary hover:opacity-90'
            : 'bg-surface0 border-surface1 text-subtext hover:border-secondary'}"
        title={planMode ? 'Exit Plan mode' : 'Enter Plan mode (drag tasks onto the grid)'}
      >
        <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <path d="M3 6h18M3 12h12M3 18h6"/>
        </svg>
        Plan
      </button>
      {#if planMode}
        <span class="hidden sm:inline-block text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-secondary/20 text-secondary border border-secondary/30">Plan mode</span>
      {/if}
      <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-xs sm:text-sm">
        <button
          class="px-2 sm:px-3 py-1.5 {view === 'day' ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}"
          onclick={() => (view = 'day')}
        >Day</button>
        <button
          class="px-2 sm:px-3 py-1.5 {view === '3day' ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}"
          onclick={() => (view = '3day')}
          title="3-day view"
        >3d</button>
        <button
          class="px-2 sm:px-3 py-1.5 {view === 'week' ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}"
          onclick={() => (view = 'week')}
        >Week</button>
        <button
          class="px-2 sm:px-3 py-1.5 {view === 'month' ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}"
          onclick={() => (view = 'month')}
        >Month</button>
        <button
          class="px-2 sm:px-3 py-1.5 {view === 'year' ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'} hidden sm:inline-block"
          onclick={() => (view = 'year')}
        >Year</button>
        <button
          class="px-2 sm:px-3 py-1.5 {view === 'agenda' ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}"
          onclick={() => (view = 'agenda')}
        >Agenda</button>
      </div>
      <button
        onclick={() => (showShortcutHelp = true)}
        aria-label="keyboard shortcuts"
        title="Keyboard shortcuts (?)"
        class="hidden md:flex w-8 h-8 items-center justify-center text-dim hover:text-text hover:bg-surface0 rounded text-xs font-mono"
      >?</button>
    </header>

    <div
      class="flex-1 overflow-hidden p-2 sm:p-3"
      role="region"
      aria-label="calendar grid"
      ontouchstart={onTouchStart}
      ontouchend={onTouchEnd}
    >
      {#if planMode && (view === 'day' || view === '3day' || view === 'week')}
        <!-- Plan layout: backlog on the left (desktop) / top
             (mobile horizontal scroller). The grid takes the rest.
             onTaskDrop is what wires backlog → grid drop semantics;
             slot drag-to-create stays on via onSlotRange. -->
        <div class="h-full flex flex-col md:flex-row gap-2 md:gap-3 min-h-0">
          <aside class="md:w-72 md:flex-shrink-0 h-32 md:h-auto overflow-x-auto md:overflow-visible">
            <TaskBacklog onRefresh={load} />
          </aside>
          <div class="flex-1 min-w-0 min-h-0">
            <HourGrid
              days={viewDays}
              events={events}
              onClickEvent={clickEvent}
              onClickSlot={clickSlot}
              onSlotRange={onSlotRange}
              onReschedule={reschedule}
              onResize={resizeTask}
              onTaskDrop={dropTask}
            />
          </div>
        </div>
      {:else if view === 'day' || view === '3day' || view === 'week'}
        <HourGrid days={viewDays} events={events} onClickEvent={clickEvent} onClickSlot={clickSlot} onSlotRange={onSlotRange} onReschedule={reschedule} onResize={resizeTask} />
      {:else if view === 'month'}
        <div class="h-full overflow-auto">
          <MonthView cursor={cursor} events={events} onClickEvent={clickEvent} onClickDay={clickDay} />
        </div>
      {:else if view === 'year'}
        <div class="h-full overflow-auto">
          <YearView cursor={cursor} events={events} onClickDay={(d) => { cursor = d; view = 'day'; }} />
        </div>
      {:else}
        <div class="overflow-y-auto h-full">
          <AgendaView events={events} onClickEvent={clickEvent} />
        </div>
      {/if}
    </div>
  </div>
</div>

<svelte:window onkeydown={onKeydown} />

{#if showShortcutHelp}
  <!-- Backdrop only closes when the click LANDS on the backdrop itself,
       not when it bubbles up from a child. Avoids the button-in-button
       HTML invalidity from a previous version while keeping the
       expected modal behavior (click outside to close). -->
  <div
    role="dialog"
    aria-modal="true"
    aria-labelledby="shortcuts-title"
    class="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4"
    onclick={(e) => { if (e.target === e.currentTarget) showShortcutHelp = false; }}
    onkeydown={(e) => { if (e.key === 'Escape') showShortcutHelp = false; }}
    tabindex="-1"
  >
    <div class="bg-mantle border border-surface1 rounded-lg p-5 max-w-sm w-full text-left shadow-xl">
      <h3 id="shortcuts-title" class="text-sm font-semibold text-text mb-3">Calendar shortcuts</h3>
      <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-xs">
        <dt class="font-mono text-primary">t</dt><dd class="text-subtext">jump to today</dd>
        <dt class="font-mono text-primary">j / n</dt><dd class="text-subtext">next period</dd>
        <dt class="font-mono text-primary">k / p</dt><dd class="text-subtext">previous period</dd>
        <dt class="font-mono text-primary">d</dt><dd class="text-subtext">day view</dd>
        <dt class="font-mono text-primary">x</dt><dd class="text-subtext">3-day view</dd>
        <dt class="font-mono text-primary">w</dt><dd class="text-subtext">week view</dd>
        <dt class="font-mono text-primary">m</dt><dd class="text-subtext">month view</dd>
        <dt class="font-mono text-primary">y</dt><dd class="text-subtext">year view</dd>
        <dt class="font-mono text-primary">a</dt><dd class="text-subtext">agenda view</dd>
        <dt class="font-mono text-primary">?</dt><dd class="text-subtext">toggle this help</dd>
      </dl>
      <p class="text-[11px] text-dim italic mt-3">On mobile: swipe left/right to navigate.</p>
      <button
        onclick={() => (showShortcutHelp = false)}
        class="mt-4 px-3 py-1.5 text-xs bg-surface0 border border-surface1 rounded hover:border-primary"
      >close</button>
    </div>
  </div>
{/if}

<EventDetail bind:open={detailOpen} event={selected} onChanged={load} />
<QuickCreateScheduled
  bind:open={createOpen}
  date={createDate}
  hour={createHour}
  minute={createMinute}
  defaultNotePath={`Jots/${fmtDateISO(createDate)}.md`}
  onCreated={load}
/>
<CreateEvent bind:open={createEventOpen} date={createEventDate} onCreated={load} />
<UnifiedCreate
  bind:open={unifiedOpen}
  start={unifiedStart}
  end={unifiedEnd}
  defaultKind={unifiedKind}
  defaultNotePath={`Jots/${fmtDateISO(unifiedStart)}.md`}
  onCreated={load}
/>
