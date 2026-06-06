<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { auth } from '$lib/stores/auth';
  import CalendarAgent from '$lib/calendar/CalendarAgent.svelte';
  import { EVENT_TYPES } from '$lib/calendar/eventTypes';
  import { toast } from '$lib/components/toast';
  import { mediaQuery } from '$lib/util/mediaQuery';
  import { loadStored, loadStoredString, saveStored, saveStoredString } from '$lib/util/storage';
  import {
    createCalendarViewState,
    type View,
    type HourDensity
  } from '$lib/calendar/calendarViewState.svelte';
  import {
    createCalendarFilterState,
    type EventFilterKey
  } from '$lib/calendar/calendarFilterState.svelte';
  import { createCalendarData } from '$lib/calendar/calendarData.svelte';
  import { createCalendarDetail } from '$lib/calendar/calendarDetail.svelte';
  import { createCalendarCreateDialogs } from '$lib/calendar/calendarCreateDialogs.svelte';
  import { createCalendarRecurringScope } from '$lib/calendar/calendarRecurringScope.svelte';
  import { createCalendarQuickEvent } from '$lib/calendar/calendarQuickEvent.svelte';
  import { installCalendarLifecycle } from '$lib/calendar/calendarLifecycle';
  import { createCalendarKeyboard, createCalendarSwipe } from '$lib/calendar/calendarKeyboard.svelte';
  import { createCalendarEventMutations } from '$lib/calendar/calendarEventMutations';
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
  import ContentPipelineOverlay from '$lib/calendar/ContentPipelineOverlay.svelte';
  import ContentChannelLanes from '$lib/calendar/ContentChannelLanes.svelte';
  import AgendaView from '$lib/calendar/AgendaView.svelte';
  import YearView from '$lib/calendar/YearView.svelte';
  import MiniMonth from '$lib/calendar/MiniMonth.svelte';
  import { sourceColors, setSourceColor, applySourceColor, type CalendarTone } from '$lib/calendar/sourceColors';
  import EventDetail from '$lib/calendar/EventDetail.svelte';
  import QuickCreateScheduled from '$lib/calendar/QuickCreateScheduled.svelte';
  import CreateEvent from '$lib/calendar/CreateEvent.svelte';
  import UnifiedCreate from '$lib/calendar/UnifiedCreate.svelte';
  import FindTime from '$lib/calendar/FindTime.svelte';
  import HeaderToolbar from '$lib/calendar/HeaderToolbar.svelte';
  import CalendarFilterChips from '$lib/calendar/CalendarFilterChips.svelte';
  import RecurringScopePicker from '$lib/calendar/RecurringScopePicker.svelte';
  import TaskBacklog from '$lib/calendar/TaskBacklog.svelte';
  import Drawer from '$lib/components/Drawer.svelte';
  import { dragStore } from '$lib/calendar/dragStore';
  import { onDestroy } from 'svelte';

  // View / display / navigation state lives in
  // $lib/calendar/calendarViewState. Read via viewCtl.X.
  const viewCtl = createCalendarViewState();

  // Loaded data (dataCtl.feed / dataCtl.nativeEvents / dataCtl.habits / dataCtl.allProjects /
  // dataCtl.calSources) + every load function (load / loadNativeEvents /
  // loadHabits / loadAllProjects / loadSources / toggleSource) +
  // the prefetch window (dataCtl.fetchFrom / dataCtl.fetchTo) live in
  // $lib/calendar/calendarData. The page still owns the onMount +
  // WS subscription orchestration; it calls into dataCtl.X.
  const dataCtl = createCalendarData({
    getCursor: () => viewCtl.cursor,
    isAuthed: () => !!$auth
  });

  // Filter dimensions + every filter derivation + the FILTER_CHIPS
  // catalog live in $lib/calendar/calendarFilterState. Read via
  // filterCtl.X.
  const filterCtl = createCalendarFilterState({
    getAllEvents: () => dataCtl.feed?.events ?? [],
    getAllProjects: () => dataCtl.allProjects,
    getCursor: () => viewCtl.cursor
  });

  // Clean exit on route change: drop any pending drag pick. Otherwise
  // a stale dragStore could corrupt the next page's pointer behaviour.
  onDestroy(() => dragStore.set(null));

  // dataCtl.feed / dataCtl.habits / dataCtl.loading / dataCtl.nativeEvents / dataCtl.fetchFrom / dataCtl.fetchTo
  // and the loadNativeEvents function moved into dataCtl.
  let agentOpen = $state(false);

  // Detail drawer state + clickEvent router live in calendarDetail.
  // clickEvent routes goal_target → /goals, meal_slot → toggle done,
  // everything else → open the EventDetail drawer + publish to
  // workspaceContext.
  const detCtl = createCalendarDetail({ dataCtl });

  // Four create-dialog state slots + their openers (clickSlot,
  // onSlotRange, onFindTimePick) live in calendarCreateDialogs.
  const dlgCtl = createCalendarCreateDialogs();

  // Recurring-scope prompt — lives in calendarRecurringScope. The
  // pill-row UI hooks into recurCtl.prompt; await recurCtl.ask(title,
  // action) from the move / resize flows to get the user's choice.
  const recurCtl = createCalendarRecurringScope();
  const askRecurringScope = recurCtl.ask;
  // onFindTimePick lives in dlgCtl; alias for the prop callback below.
  const onFindTimePick = dlgCtl.onFindTimePick;

  // filterCtl.filterDrawerOpen moved into filterCtl.
  // Reactive mobile flag via the shared mediaQuery store. Auto-cleans
  // up on component destroy. The first-mount "force day viewCtl.view on
  // mobile" rule still applies, see the $effect below.
  const isMobile = mediaQuery('(max-width: 767px)');
  let _mobileViewForced = $state(false);
  $effect(() => {
    if ($isMobile && !_mobileViewForced) {
      viewCtl.view = 'day';
      _mobileViewForced = true;
    }
  });

  // filterCtl.hidden / filterCtl.projectFilter / filterCtl.kindFilter + FILTER_CHIPS catalog +
  // their persistence + filterCtl.toggleType / filterCtl.toggleKindFilter / filterCtl.clearKindFilter
  // moved into $lib/calendar/calendarFilterState. Read via filterCtl.X.

  // dataCtl.allProjects + loadAllProjects moved into dataCtl.

  // filterCtl.colorByProject + persistence moved into filterCtl.

  // dataCtl.calSources + dataCtl.savingSources + loadSources + toggleSource moved
  // into dataCtl.

  // Mobile detection moved to the mediaQuery store + $effect above.

  // Deep-link: ?plan=1 (optionally with &project=NAME) flips on plan
  // mode so other pages — e.g. the project detail's "schedule next
  // action" button — can hand off into the calendar in the right state.
  // ?project=NAME (without &plan) just scopes the viewCtl.view to that
  // project — the "open this project's calendar" hand-off.
  onMount(() => {
    if (typeof window === 'undefined') return;
    const url = new URL(window.location.href);
    if (url.searchParams.get('plan') === '1' && !viewCtl.planMode) {
      viewCtl.planMode = true;
      viewCtl.view = 'day';
    }
    const proj = url.searchParams.get('project');
    if (proj) filterCtl.projectFilter = proj;
    // ?agent=1 launches the Calendar Agent — used by the chat sidebar.
    if (url.searchParams.get('agent') === '1') {
      agentOpen = true;
      const params = new URLSearchParams(url.searchParams);
      params.delete('agent');
      void goto(`/calendar${params.toString() ? '?' + params : ''}`, {
        replaceState: true,
        keepFocus: true
      });
    }
  });

  // Initial loads + WS subscription live in calendarLifecycle.
  onMount(() => installCalendarLifecycle({ dataCtl }));

  // dataCtl.load() reads dataCtl.fetchFrom/dataCtl.fetchTo synchronously and may reassign them
  // when viewCtl.cursor walks outside the prefetch window. Without untrack, the
  // re-assignment refires this effect (one extra fetch per far-jump
  // navigation). The explicit `void` list above is the actual dep set.
  $effect(() => {
    void viewCtl.cursor;
    void viewCtl.view;
    untrack(() => dataCtl.load());
  });

  let allEvents = $derived(dataCtl.feed?.events ?? []);
  // Apply per-source color overrides so an ICS calendar the user
  // tinted 'green' renders that way on every viewCtl.view. Pure transform
  // on the derived event list — no extra fetches, no storage round
  // trip. The override is per-device (localStorage); future cross-
  // device sync can layer over the same map without touching the
  // render path.
  //
  // Project filter and project-tint layer on top in two stages:
  //   1. If filterCtl.projectFilter is set, drop every row whose project_id !=
  //      the picked project — the calendar becomes a project board.
  //   2. If filterCtl.colorByProject is on, override the row's `color` with the
  //      project's saved colour (events.json filterCtl.events). The override
  //      runs through the existing eventTypeColor mapping (red/yellow
  //      /green/...), so it composes cleanly with the per-source
  //      and per-event paths.
  // filterCtl.projectColorMap + filterCtl.events derivations moved into filterCtl.

  // ── Natural-language quick-create ────────────────────────────────
  // Single text input above the grid: "lunch tomorrow 12pm 1h" →
  // event. Reuses the deterministic regex parser in
  // calendar/quickCreate.ts (no LLM call), so the flow stays fast,
  // offline-friendly, and predictable. Live preview shows what we
  // recognised so the user can see whether the parse worked before
  // hitting Enter — saves the "type, submit, get a wrong event,
  // delete, retry" loop.
  // Quick-event input + parse + submit live in calendarQuickEvent.
  const quickEvtCtl = createCalendarQuickEvent({ dataCtl });
  const submitQuickEvent = quickEvtCtl.submit;
  // filterCtl.typeCounts + filterCtl.visibleFilterChips derivations moved into filterCtl.

  // Pipeline / channel-lanes overlay — a single toggle that renders
  // two different content-pipeline grouping shapes depending on the
  // active viewCtl.view: kanban by status on month, swim lanes by channel on
  // week / workweek. The user toggles ONCE and gets the right
  // grouping for the day-axis they're already looking at. Toggled
  // from a header button that only appears when content filterCtl.events exist
  // (so non-content users see no extra chrome) AND the active viewCtl.view
  // has a useful pipeline shape (day / year / agenda don't).
  // viewCtl.pipelineMode moved into viewCtl. The button-available + label
  // derivations stay here because they straddle viewCtl.view + filterCtl.typeCounts
  // (which lives in the upcoming filter controller). Auto-close
  // effect uses viewCtl.pipelineMode getter/setter.
  let pipelineButtonAvailable = $derived(
    (viewCtl.view === 'month' || viewCtl.view === 'week' || viewCtl.view === 'workweek') &&
      (filterCtl.typeCounts['content_event'] ?? 0) > 0
  );
  $effect(() => {
    if (viewCtl.pipelineMode && !pipelineButtonAvailable) viewCtl.pipelineMode = false;
  });
  let pipelineButtonLabel = $derived(viewCtl.view === 'month' ? 'Pipeline' : 'Channels');

  // viewCtl.viewDays moved into viewCtl.

  // filterCtl.monthEvents + filterCtl.agendaEvents derivations moved into filterCtl.

  // prev / next / gotoToday moved into viewCtl.

  // Keyboard shortcuts + touch-swipe-to-navigate live in
  // calendarKeyboard.svelte. The keyboard controller exposes
  // kbCtl.showShortcutHelp + onKeydown; the swipe handlers attach to the
  // grid container's ontouchstart/end below.
  const kbCtl = createCalendarKeyboard({
    viewCtl,
    dlgCtl,
    detCtl,
    openAgent: () => (agentOpen = true)
  });
  const onKeydown = kbCtl.onKeydown;
  const { onTouchStart, onTouchEnd } = createCalendarSwipe(viewCtl);

  // clickEvent + toggleMealEvent live in detCtl. Bind the local
  // alias so all existing call sites keep their one-word reference.
  const clickEvent = detCtl.clickEvent;
  const toggleMealEvent = detCtl.toggleMealEvent;
  const clickSlot = dlgCtl.clickSlot;
  const onSlotRange = dlgCtl.onSlotRange;

  // moveEvent / resizeEvent / dropTask / reschedule — calendar event
  // mutation dispatch. Routes by event type (events.json / ICS / task),
  // pipes recurring events through askRecurringScope, and ends every
  // path with dataCtl.load() so optimistic drag visuals reconcile.
  // See calendarEventMutations.ts for the why on midnight clamps,
  // override_key preservation, and skip+create ordering.
  const mut = createCalendarEventMutations({ dataCtl, askRecurringScope });
  const moveEvent = mut.moveEvent;
  const resizeEvent = mut.resizeEvent;
  const dropTask = mut.dropTask;
  const reschedule = mut.reschedule;
  function clickDay(d: Date) { viewCtl.cursor = d; viewCtl.view = 'day'; }
  function pickDay(d: Date) { viewCtl.cursor = d; filterCtl.filterDrawerOpen = false; }

  // ── AI: Plan my week ─────────────────────────────────────────────
  // Looks across all open UNSCHEDULED tasks (no scheduledStart yet)
  // and asks the AI to suggest a day for each, weighted against the
  // visible 7-day load (event count per day). The user already has
  // triage on /tasks for what to ignore and Top-3 for what to start;
  // this one picks WHEN to do the rest. Streams via chatStream so
  // tokens land progressively, and the response renders as a banner
  // between the toolbar and the grid — same slot as quick-create —
  // so the user can scroll the calendar while reading the plan.
  // Cap: at most 30 unscheduled tasks fed in, to keep prompt size
  // predictable on long backlogs.
  // AI page-local helpers (Plan my week, Find free time, Day
  // insight) used to live inline here as long fetch-and-render
  // blocks. All three prompts now run through the chat sidebar's
  // Calendar Agent (Run agent → Find focus block / Week shape /
  // Clear one meeting), which shares the same chatStream pipeline.
  // The 300-odd lines they used to take have been retired.

  let headline = $derived.by(() => {
    if (viewCtl.view === 'day') return viewCtl.cursor.toLocaleDateString(undefined, { weekday: $isMobile ? 'short' : 'long', month: 'short', day: 'numeric' });
    if (viewCtl.view === 'week') {
      const s = startOfWeek(viewCtl.cursor);
      const e = endOfWeek(viewCtl.cursor);
      if (s.getMonth() === e.getMonth()) {
        return `${s.toLocaleDateString(undefined, { month: 'short' })} ${s.getDate()}–${e.getDate()}`;
      }
      return `${s.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })} – ${e.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}`;
    }
    if (viewCtl.view === 'workweek') {
      const s = addDays(startOfWeek(viewCtl.cursor), 1); // Mon
      const e = addDays(s, 4); // Fri
      if (s.getMonth() === e.getMonth()) {
        return `${s.toLocaleDateString(undefined, { month: 'short' })} ${s.getDate()}–${e.getDate()} (Mon–Fri)`;
      }
      return `${s.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })} – ${e.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })} (Mon–Fri)`;
    }
    if (viewCtl.view === 'month') return viewCtl.cursor.toLocaleDateString(undefined, { month: 'long', year: 'numeric' });
    if (viewCtl.view === 'year') return String(viewCtl.cursor.getFullYear());
    if (viewCtl.view === 'agenda') return 'Agenda · next 30 days';
    return '';
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
        dlgCtl.unifiedStart = s; dlgCtl.unifiedEnd = e; dlgCtl.unifiedKind = 'task'; dlgCtl.unifiedOpen = true;
        filterCtl.filterDrawerOpen = false;
      }}
      class="w-full px-3 py-2.5 bg-primary text-on-primary rounded text-sm font-medium hover:opacity-90"
    >
      + New task or event
    </button>
    <p class="text-[11px] text-dim italic px-1 -mt-2">…or click + drag on the grid</p>
    <MiniMonth cursor={viewCtl.cursor} selected={viewCtl.cursor} events={filterCtl.monthEvents} onPick={pickDay} />

    <!-- Project filter — turn the calendar into a project-management
         board for one project at a time. Empty value = "all projects".
         Pairs with the per-event project picker; an event linked to
         project X shows up only when the filter is empty or set to X.
         "Colour by project" toggle below tints linked rows with the
         project's saved colour for at-a-glance visual grouping. -->
    <!-- Event-type filter strip. Each chip toggles a single type
         into the filter set; empty set = no filter (everything
         visible). The 'Untyped' chip includes filterCtl.events with no kind
         declared so a user can isolate the legacy un-tagged subset. -->
    <div class="space-y-1.5 text-xs">
      <h3 class="text-dim uppercase tracking-wider mb-2 flex items-center gap-2">
        <span>Event type</span>
        {#if filterCtl.kindFilter.size > 0}
          <button
            type="button"
            onclick={filterCtl.clearKindFilter}
            class="ml-auto text-[10px] text-warning hover:text-error normal-case"
          >clear ({filterCtl.kindFilter.size})</button>
        {/if}
      </h3>
      <div class="flex items-center gap-1 flex-wrap">
        {#each EVENT_TYPES as t (t.id)}
          {@const on = filterCtl.kindFilter.has(t.id)}
          <button
            type="button"
            onclick={() => filterCtl.toggleKindFilter(t.id)}
            aria-pressed={on}
            title={t.description}
            class="inline-flex items-center gap-1 px-1.5 py-1 text-[11px] font-medium border transition-colors {on ? 'bg-primary text-on-primary border-primary' : 'bg-surface0 text-text border-surface1 hover:border-primary'}"
          >
            <span
              class="inline-flex items-center justify-center w-3.5 h-3.5 text-[9px] font-bold font-mono leading-none"
              style:background={on ? 'transparent' : `color-mix(in srgb, var(--color-${t.color}) 22%, transparent)`}
              style:color={on ? undefined : `var(--color-${t.color})`}
            >{t.glyph}</span>
            <span>{t.label}</span>
          </button>
        {/each}
        <button
          type="button"
          onclick={() => filterCtl.toggleKindFilter('__untyped')}
          aria-pressed={filterCtl.kindFilter.has('__untyped')}
          title="Events with no type set"
          class="inline-flex items-center gap-1 px-1.5 py-1 text-[11px] font-medium border transition-colors {filterCtl.kindFilter.has('__untyped') ? 'bg-primary text-on-primary border-primary' : 'bg-surface0 text-dim border-surface1 hover:border-primary'}"
        >Untyped</button>
      </div>
    </div>

    {#if dataCtl.allProjects.length > 0}
      <div class="space-y-1.5 text-xs">
        <h3 class="text-dim uppercase tracking-wider mb-2">Project board</h3>
        <select
          bind:value={filterCtl.projectFilter}
          aria-label="filter calendar by project"
          class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
        >
          <option value="">All projects</option>
          {#each dataCtl.allProjects as p (p.name)}
            <option value={p.name}>{p.name}</option>
          {/each}
        </select>
        <label class="flex items-center gap-2 px-1 py-0.5 text-[11px] text-subtext cursor-pointer select-none">
          <input
            type="checkbox"
            bind:checked={filterCtl.colorByProject}
            class="w-3.5 h-3.5 accent-primary"
          />
          Colour by project
        </label>
        {#if filterCtl.projectFilter}
          <p class="text-[10px] text-dim italic px-1 leading-snug">
            Showing only filterCtl.events + tasks linked to <span class="text-secondary">{filterCtl.projectFilter}</span>.
            <button
              type="button"
              onclick={() => (filterCtl.projectFilter = '')}
              class="text-[10px] text-warning hover:underline ml-1"
            >clear</button>
          </p>
        {/if}
      </div>
    {/if}

    <!-- Per-file ICS source toggles — same `disabled_calendars` config
         the TUI uses, so flipping a switch here silences the file in
         both frontends. The most common reason to toggle: vaults often
         carry both per-source files (training.ics, faith.ics) and a
         merged.ics that combines them — disable one side. -->
    {#if dataCtl.calSources.length > 0}
      <div class="space-y-1 text-xs">
        <h3 class="text-dim uppercase tracking-wider mb-2">Calendar sources</h3>
        {#each dataCtl.calSources as s (s.id)}
          {@const tone = sourceColorToken(s.source)}
          {@const customTone = $sourceColors[s.source] ?? ''}
          {@const displayTone = customTone || tone}
          {@const srcCount = filterCtl.typeCounts['ics_event'] !== undefined
            ? allEvents.filter((e) => e.type === 'ics_event' && e.source === s.source).length
            : 0}
          <!-- Stream R: always-visible color dot at the LEFT of the
               row so the user can read "blue dot = training.ics" at
               a glance. Toggle check moved to a small box on the
               far right; color-picker swatches still surface on
               hover only so the row stays visually quiet. -->
          <div class="flex items-center gap-1.5 px-1 group">
            <!-- Always-visible color dot. Dim when the source is
                 disabled so the row still tells the truth about
                 visibility without burying the colour cue. -->
            <span
              class="w-2.5 h-2.5 rounded-full flex-shrink-0 {s.enabled ? '' : 'opacity-40'}"
              style="background: var(--color-{displayTone})"
              aria-hidden="true"
            ></span>
            <button
              onclick={() => dataCtl.toggleSource(s)}
              disabled={dataCtl.savingSources}
              class="flex items-center gap-2 px-1 py-1 rounded hover:bg-surface0 flex-1 min-w-0 {s.enabled ? '' : 'opacity-40'}"
              title="{s.path}{s.enabled ? '' : ' (disabled)'}"
            >
              <span class="text-subtext flex-1 text-left truncate">{s.source}</span>
              {#if srcCount > 0}
                <span class="text-dim font-mono tabular-nums text-[10px]">{srcCount}</span>
              {/if}
              {#if s.folder}<span class="text-dim text-[10px]">{s.folder}</span>{/if}
              <!-- Visibility checkbox shifted to the right edge so the
                   colour dot can own the leftmost column. Mirrors the
                   sidebar Filters row shape. -->
              <span class="w-3 h-3 rounded-sm border flex items-center justify-center flex-shrink-0"
                style="border-color: var(--color-{displayTone}); background: {s.enabled ? `var(--color-${displayTone})` : 'transparent'}">
                {#if s.enabled}
                  <svg viewBox="0 0 12 12" class="w-2.5 h-2.5 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
                {/if}
              </span>
            </button>
            <!-- Per-source color picker — hover-only swatch row so the
                 default state stays uncluttered. Empty swatch resets
                 to the auto-rotation default. -->
            <div class="hidden group-hover:flex items-center gap-0.5 flex-shrink-0">
              {#each ['', 'red', 'yellow', 'orange', 'green', 'blue', 'purple', 'cyan', 'pink'] as t}
                <button
                  type="button"
                  onclick={() => setSourceColor(s.source, t as CalendarTone)}
                  aria-label={t || 'auto'}
                  title={t ? `Color: ${t}` : 'Auto (default rotation)'}
                  class="w-3 h-3 rounded-full border transition-transform hover:scale-125 {customTone === t ? 'ring-1 ring-text scale-125' : ''}"
                  style={t ? `background: var(--color-${t}); border-color: var(--color-${t})` : 'background: transparent; border-color: var(--color-dim)'}
                ></button>
              {/each}
            </div>
          </div>
        {/each}
        <p class="text-[10px] text-dim italic px-2 pt-1">colour dot = source legend · hover to recolour · toggles sync with granit TUI's <code>disabled_calendars</code></p>
      </div>
    {/if}

    <!-- Filters: each row toggles visibility of one event type. Counts are
         live across the loaded range so the user can see which buckets are
         empty before hiding them. -->
    <div class="space-y-1 text-xs">
      <h3 class="text-dim uppercase tracking-wider mb-2">Filters</h3>
      {#each filterCtl.visibleFilterChips as f (f.key)}
        {@const isHidden = filterCtl.hidden.has(f.key)}
        <button
          onclick={() => filterCtl.toggleType(f.key)}
          class="w-full flex items-center gap-2 px-2 py-1 rounded hover:bg-surface0 {isHidden ? 'opacity-40' : ''}"
        >
          <span class="w-2 h-2 rounded-full" style="background: var(--color-{f.tone})"></span>
          <span class="text-subtext flex-1 text-left">{f.label}</span>
          <span class="text-dim">{filterCtl.typeCounts[f.key] ?? 0}</span>
          {#if isHidden}<span class="text-dim text-[10px]">hidden</span>{/if}
        </button>
      {/each}
    </div>
  </div>
{/snippet}

<div class="flex h-full">
  <!-- Desktop sidebar -->
  <aside class="hidden md:block md:w-56 lg:w-64 border-r border-surface1 bg-mantle flex-shrink-0 overflow-y-auto">
    {@render sidebarContent()}
  </aside>

  <!-- Mobile drawer -->
  <Drawer bind:open={filterCtl.filterDrawerOpen} side="left">
    {@render sidebarContent()}
  </Drawer>

  <div class="flex-1 flex flex-col min-w-0">
    <HeaderToolbar
      bind:view={viewCtl.view}
      bind:cursor={viewCtl.cursor}
      {headline}
      loading={dataCtl.loading}
      bind:monthDensity={viewCtl.monthDensity}
      bind:hourDensity={viewCtl.hourDensity}
      planMode={viewCtl.planMode}
      onPrev={viewCtl.prev}
      onNext={viewCtl.next}
      onGotoToday={viewCtl.gotoToday}
      onTogglePlanMode={viewCtl.togglePlanMode}
      onFindTime={() => (dlgCtl.findTimeOpen = true)}
      onShowShortcuts={() => (kbCtl.showShortcutHelp = true)}
      onOpenFilterDrawer={() => (filterCtl.filterDrawerOpen = true)}
      onCapture={() => {
        // Seed UnifiedCreate with the next round hour so the user
        // doesn't have to re-type the time. Same default the sidebar
        // "+ New task or event" button uses.
        const s = new Date();
        s.setMinutes(0, 0, 0);
        s.setHours(s.getHours() + 1);
        const e = new Date(s.getTime() + 60 * 60 * 1000);
        dlgCtl.unifiedStart = s;
        dlgCtl.unifiedEnd = e;
        dlgCtl.unifiedKind = 'event';
        dlgCtl.unifiedOpen = true;
      }}
    />

    <!-- AI inline panels (Find Free Time / Day Insight / Plan My Week)
         retired alongside their header buttons. Same prompts now live
         in the chat sidebar's Calendar Agent — runnable from any page
         with the Run-Agent chip. -->

    <!-- Quick-create bar. Sits between the toolbar and the grid so
         it's always visible without crowding the controls row. The
         parsed preview replaces the input's helper text the moment
         we recognise a valid date+time, giving the user instant
         feedback that "fri 2pm 30m" actually became next Friday
         14:00–14:30 before they commit. -->
    <form
      class="flex items-center gap-2 px-3 py-1.5 border-b border-surface1 flex-shrink-0"
      onsubmit={(e) => { e.preventDefault(); void submitQuickEvent(); }}
    >
      <span class="text-base flex-shrink-0" aria-hidden="true">＋</span>
      <input
        bind:value={quickEvtCtl.quickInput}
        placeholder='e.g. "lunch tomorrow 12pm 1h" or "team mtg fri 14:00-15:00"'
        class="flex-1 min-w-0 px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
        aria-label="quick-create event"
        disabled={quickEvtCtl.quickBusy}
      />
      {#if quickEvtCtl.quickInput.trim()}
        <span class="hidden md:inline text-[11px] text-dim font-mono truncate max-w-md">
          {#if quickEvtCtl.quickParse?.ok && quickEvtCtl.quickParse.event}
            <span class="text-success">✓</span>
            {quickEvtCtl.quickParse.event.title} · {quickEvtCtl.quickParse.event.date}{quickEvtCtl.quickParse.event.startTime ? ` · ${quickEvtCtl.quickParse.event.startTime}${quickEvtCtl.quickParse.event.endTime ? `–${quickEvtCtl.quickParse.event.endTime}` : ''}` : ' · all-day'}
          {:else}
            <span class="text-warning">{quickEvtCtl.quickParse?.hint ?? 'parsing…'}</span>
          {/if}
        </span>
        <button
          type="submit"
          disabled={!quickEvtCtl.quickParse?.ok || quickEvtCtl.quickBusy}
          class="px-2.5 py-1 text-xs bg-primary text-on-primary rounded font-medium disabled:opacity-40 flex-shrink-0"
        >{quickEvtCtl.quickBusy ? '…' : 'Add'}</button>
      {/if}
    </form>

    <!-- Quick-filter chips — extracted to CalendarFilterChips so the
         row matches the Tasks page's QuickFilterChips shape (Stream R).
         Drives the same `filterCtl.hidden` Set<EventFilterKey> the sidebar
         Filters section uses, so a type toggled here is also toggled
         in the sidebar. -->
    <CalendarFilterChips
      chips={filterCtl.visibleFilterChips}
      hidden={filterCtl.hidden}
      typeCounts={filterCtl.typeCounts}
      onToggle={filterCtl.toggleType}
      onClearAll={() => (filterCtl.hidden = new Set())}
    />

    {#if pipelineButtonAvailable}
      <!-- Pipeline toggle — month viewCtl.view gets kanban-by-status; week /
           workweek viewCtl.view gets swim-lanes-by-channel. Both surface the
           same content filterCtl.events grouped for the day-axis the user is
           already looking at. Hidden in day / year / agenda since
           there's no useful grouping shape for those. -->
      <div class="flex items-center gap-2 px-3 py-1 border-b border-surface1 flex-shrink-0 bg-mantle">
        <button
          type="button"
          onclick={() => (viewCtl.pipelineMode = !viewCtl.pipelineMode)}
          aria-pressed={viewCtl.pipelineMode}
          title={viewCtl.pipelineMode
            ? `Close the ${viewCtl.view === 'month' ? 'pipeline kanban' : 'channel lanes'}`
            : viewCtl.view === 'month'
              ? 'Group content events by status in a kanban overlay'
              : 'Group content events by channel into horizontal lanes'}
          class="inline-flex items-center gap-1.5 px-2 py-1 rounded text-xs font-medium border transition-colors {viewCtl.pipelineMode ? 'bg-lavender text-on-primary border-lavender' : 'bg-surface0 text-subtext border-surface1 hover:border-lavender hover:text-text'}"
        >
          <span
            class="inline-flex items-center justify-center w-4 h-4 rounded text-[9px] font-mono font-semibold"
            style={viewCtl.pipelineMode ? 'background: rgba(0,0,0,0.18)' : 'background: color-mix(in srgb, var(--color-lavender) 18%, transparent); color: var(--color-lavender)'}
            aria-hidden="true"
          >C</span>
          {pipelineButtonLabel}
          <span class="font-mono tabular-nums opacity-80">{filterCtl.typeCounts['content_event'] ?? 0}</span>
        </button>
        <span class="text-[11px] text-dim">{viewCtl.view === 'month' ? 'kanban by status' : 'swim lanes by channel'}</span>
      </div>
    {/if}

    <div
      class="flex-1 overflow-hidden p-2 sm:p-3"
      role="region"
      aria-label="calendar grid"
      ontouchstart={onTouchStart}
      ontouchend={onTouchEnd}
    >
      {#if viewCtl.planMode && (viewCtl.view === 'day' || viewCtl.view === 'week' || viewCtl.view === 'workweek')}
        <!-- Plan layout: backlog on the left (desktop) / top scroller
             (mobile). The mobile strip used to be 128px tall and read
             one task per line in a tight horizontal scroller — too
             cramped to scan, too short to drag from. Bumped to 44dvh
             on phones (clamped at 320px) so the backlog feels like a
             real surface, not a footnote. AI button + 4-5 cards visible
             before scroll. The grid keeps the rest of the viewport
             via flex-1. -->
        <div class="h-full flex flex-col md:flex-row gap-2 md:gap-3 min-h-0">
          <aside class="md:w-72 md:flex-shrink-0 md:h-auto flex-shrink-0 overflow-hidden md:overflow-visible" style="height: clamp(220px, 44dvh, 320px);">
            <TaskBacklog onRefresh={() => dataCtl.load()} />
          </aside>
          <div class="flex-1 min-w-0 min-h-0">
            <HourGrid
              days={viewCtl.viewDays}
              events={filterCtl.events}
              habits={dataCtl.habits}
              onClickEvent={clickEvent}
              onClickSlot={clickSlot}
              onSlotRange={onSlotRange}
              onReschedule={reschedule}
              onMove={moveEvent}
              onResize={resizeEvent}
              writableSources={dataCtl.calSources.filter((s) => s.writable).map((s) => s.source)}
              onTaskDrop={dropTask}
              hourPx={viewCtl.hourPx}
            />
          </div>
        </div>
      {:else if viewCtl.view === 'day' || viewCtl.view === 'week' || viewCtl.view === 'workweek'}
        <div class="relative h-full">
          <HourGrid days={viewCtl.viewDays} events={filterCtl.events} habits={dataCtl.habits} onClickEvent={clickEvent} onClickSlot={clickSlot} onSlotRange={onSlotRange} onReschedule={reschedule} onMove={moveEvent} onResize={resizeEvent} writableSources={dataCtl.calSources.filter((s) => s.writable).map((s) => s.source)} hourPx={viewCtl.hourPx} />
          {#if viewCtl.pipelineMode && (viewCtl.view === 'week' || viewCtl.view === 'workweek')}
            <!-- Swim-lane overlay for week-axis content production
                 planning. Same overlay chrome as ContentPipelineOverlay
                 but grouped by channel rather than status — the user
                 reads horizontally for each platform's week-at-a-glance. -->
            <ContentChannelLanes
              days={viewCtl.viewDays}
              events={filterCtl.events}
              onClickEvent={clickEvent}
              onClose={() => (viewCtl.pipelineMode = false)}
            />
          {/if}
        </div>
      {:else if viewCtl.view === 'month'}
        <div class="relative h-full overflow-auto">
          <MonthView cursor={viewCtl.cursor} events={filterCtl.events} density={viewCtl.monthDensity} onClickEvent={clickEvent} onClickDay={clickDay} />
          {#if viewCtl.pipelineMode}
            <!-- Kanban overlay sits absolute over the month grid so
                 the underlying dates stay visually present (closing
                 returns to the same scroll position, no transition
                 cost). All status grouping logic lives inside the
                 component; the page just hands it the filtered
                 event list + the same click handler the grid uses. -->
            <ContentPipelineOverlay
              events={filterCtl.events}
              onClickEvent={clickEvent}
              onClose={() => (viewCtl.pipelineMode = false)}
            />
          {/if}
        </div>
      {:else if viewCtl.view === 'year'}
        <div class="h-full overflow-auto">
          <YearView cursor={viewCtl.cursor} events={filterCtl.events} onClickDay={(d) => { viewCtl.cursor = d; viewCtl.view = 'day'; }} />
        </div>
      {:else if viewCtl.view === 'agenda'}
        <!-- Agenda is the flat 30-day next-up list. Scoped to
             `filterCtl.agendaEvents` (rolling viewCtl.cursor → +30d) so prev/next
             walks weeks of agenda content without re-fetching the
             whole dataCtl.feed. The onCreate hook fires from the empty-state
             "+ Create event" CTA so a wide-open week is one click
             from filling the first slot. -->
        <div class="overflow-y-auto h-full">
          <AgendaView
            events={filterCtl.agendaEvents}
            onClickEvent={clickEvent}
            onCreate={() => {
              const s = new Date();
              s.setMinutes(0, 0, 0);
              s.setHours(s.getHours() + 1);
              const e = new Date(s.getTime() + 60 * 60 * 1000);
              dlgCtl.unifiedStart = s;
              dlgCtl.unifiedEnd = e;
              dlgCtl.unifiedKind = 'event';
              dlgCtl.unifiedOpen = true;
            }}
          />
        </div>
      {/if}
    </div>
  </div>
</div>

<svelte:window onkeydown={onKeydown} />

{#if kbCtl.showShortcutHelp}
  <!-- Backdrop only closes when the click LANDS on the backdrop itself,
       not when it bubbles up from a child. Avoids the button-in-button
       HTML invalidity from a previous version while keeping the
       expected modal behavior (click outside to close). -->
  <div
    role="dialog"
    aria-modal="true"
    aria-labelledby="shortcuts-title"
    class="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4"
    onclick={(e) => { if (e.target === e.currentTarget) kbCtl.showShortcutHelp = false; }}
    onkeydown={(e) => { if (e.key === 'Escape') kbCtl.showShortcutHelp = false; }}
    tabindex="-1"
  >
    <div class="bg-mantle border border-surface1 rounded-lg p-5 max-w-sm w-full max-h-[90dvh] overflow-y-auto text-left shadow-xl">
      <h3 id="shortcuts-title" class="text-sm font-semibold text-text mb-3">Calendar shortcuts</h3>
      <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-xs">
        <dt class="font-mono text-primary">t</dt><dd class="text-subtext">jump to today</dd>
        <dt class="font-mono text-primary">j / n</dt><dd class="text-subtext">next period</dd>
        <dt class="font-mono text-primary">k / p</dt><dd class="text-subtext">previous period</dd>
        <dt class="font-mono text-primary">d</dt><dd class="text-subtext">day viewCtl.view</dd>
        <dt class="font-mono text-primary">w</dt><dd class="text-subtext">week viewCtl.view</dd>
        <dt class="font-mono text-primary">Shift+W</dt><dd class="text-subtext">workweek (Mon–Fri only)</dd>
        <dt class="font-mono text-primary">m</dt><dd class="text-subtext">month viewCtl.view</dd>
        <dt class="font-mono text-primary">y</dt><dd class="text-subtext">year viewCtl.view</dd>
        <dt class="font-mono text-primary">a</dt><dd class="text-subtext">agenda viewCtl.view (next 30 days)</dd>
        <dt class="font-mono text-primary">f</dt><dd class="text-subtext">find time (open free-slot finder)</dd>
        <dt class="font-mono text-primary">Shift+A</dt><dd class="text-subtext">open AI agent (scoped to visible window + project filter)</dd>
        <dt class="font-mono text-primary">?</dt><dd class="text-subtext">toggle this help</dd>
      </dl>
      <p class="text-[11px] text-dim italic mt-3">On mobile: swipe left/right to navigate.</p>
      <button
        onclick={() => (kbCtl.showShortcutHelp = false)}
        class="mt-4 px-3 py-1.5 text-xs bg-surface0 border border-surface1 rounded hover:border-primary"
      >close</button>
    </div>
  </div>
{/if}

<!-- Recurring-scope picker — modal-like overlay shown when a drag-
     move or resize hits a recurring event. Replaces the stacked
     native confirm() chain that previously blocked the event loop
     and ignored our theme. The picker resolves the async
     askRecurringScope() Promise via its onChoose / onCancel hooks. -->
{#if recurCtl.prompt?.open}
  <div
    role="dialog"
    aria-modal="true"
    aria-labelledby="recurring-scope-title"
    class="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4"
    onclick={(e) => {
      if (e.target === e.currentTarget && recurCtl.prompt) recurCtl.prompt.onCancel();
    }}
    onkeydown={(e) => {
      if (e.key === 'Escape' && recurCtl.prompt) recurCtl.prompt.onCancel();
    }}
    tabindex="-1"
  >
    <div class="bg-mantle border border-surface1 rounded-lg p-4 max-w-md w-full shadow-xl">
      <h3 id="recurring-scope-title" class="text-sm font-semibold text-text mb-1">
        {recurCtl.prompt.action === 'move' ? 'Move' : 'Resize'} recurring event
      </h3>
      <p class="text-xs text-dim italic mb-2">
        Pick a scope. "Just this occurrence" keeps the rest of the series in place.
      </p>
      <RecurringScopePicker
        eventTitle={recurCtl.prompt.title}
        action={recurCtl.prompt.action}
        seriesTone={recurCtl.prompt.seriesTone}
        onChoose={(s) => {
          // The picker emits 'this' | 'future' | 'series'. We only
          // surface this / series for move + resize; future is not
          // a supported recurring-edit semantic for drag yet.
          if (s === 'future') return;
          recurCtl.prompt?.onChoose(s);
        }}
        onCancel={() => recurCtl.prompt?.onCancel()}
      />
    </div>
  </div>
{/if}

<EventDetail bind:open={detCtl.detailOpen} event={detCtl.selected} onChanged={() => dataCtl.load()} />
<QuickCreateScheduled
  bind:open={dlgCtl.createOpen}
  date={dlgCtl.createDate}
  hour={dlgCtl.createHour}
  minute={dlgCtl.createMinute}
  defaultNotePath={`Jots/${fmtDateISO(dlgCtl.createDate)}.md`}
  onCreated={() => dataCtl.load()}
/>
<CreateEvent
  bind:open={dlgCtl.createEventOpen}
  date={dlgCtl.createEventDate}
  existingEvents={filterCtl.events}
  defaultProjectId={filterCtl.projectFilter}
  onCreated={() => dataCtl.load()}
/>
<UnifiedCreate
  bind:open={dlgCtl.unifiedOpen}
  start={dlgCtl.unifiedStart}
  end={dlgCtl.unifiedEnd}
  defaultKind={dlgCtl.unifiedKind}
  defaultNotePath={`Jots/${fmtDateISO(dlgCtl.unifiedStart)}.md`}
  onCreated={() => dataCtl.load()}
/>
<FindTime bind:open={dlgCtl.findTimeOpen} events={filterCtl.events} onPick={onFindTimePick} />

<!-- Calendar Agent — scoped to the currently-visible fetch
     window AND the active project filter so the agent sees
     roughly what the user is looking at. Without the project
     filter intersection, asking "rename the client meetings"
     while filtered to a venture would surprise the user by
     proposing renames across all filterCtl.events. -->
<CalendarAgent
  open={agentOpen}
  events={dataCtl.nativeEvents.filter((e) =>
    e.date >= fmtDateISO(dataCtl.fetchFrom) &&
    e.date <= fmtDateISO(dataCtl.fetchTo) &&
    (!filterCtl.projectFilter || e.project_id === filterCtl.projectFilter)
  )}
  todayISO={fmtDateISO(new Date())}
  knownProjects={dataCtl.allProjects.map((p) => p.name)}
  onClose={() => (agentOpen = false)}
  onChanged={() => { void dataCtl.load(); void dataCtl.loadNativeEvents(); }}
/>

