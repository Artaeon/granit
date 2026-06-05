<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { auth } from '$lib/stores/auth';
  import { api, type CalendarEvent, type CalendarEventEntry, type CalendarFeed, type CalendarSource, type HabitInfo, type Project, type Task } from '$lib/api';
  import CalendarAgent from '$lib/calendar/CalendarAgent.svelte';
  import { EVENT_TYPES } from '$lib/calendar/eventTypes';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
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
  import { parseEventInput, type ParseResult } from '$lib/calendar/quickCreate';
  import TaskBacklog from '$lib/calendar/TaskBacklog.svelte';
  import Drawer from '$lib/components/Drawer.svelte';
  import { onWsEvent } from '$lib/ws';
  import { dragStore } from '$lib/calendar/dragStore';
  import { onDestroy } from 'svelte';
  import { workspaceContext } from '$lib/workspace/workspaceContext.svelte';

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

  // Find-time modal — surfaces free gaps in the active calendar dataCtl.feed
  // for a chosen duration. Picking a gap seeds UnifiedCreate so the
  // user can lock in a title without re-typing the time.
  let findTimeOpen = $state(false);

  // Recurring-scope prompt — replaces the stacked native confirm()
  // dialogs the drag-move + resize flows used to fire. The prompt is
  // a presentational pill row (RecurringScopePicker) instead of a
  // browser modal, so it doesn't block the event loop and obeys our
  // theme. Wrapped in a Promise (askRecurringScope) so the calling
  // flow can `await` a user choice. null = cancel; the caller is
  // responsible for reverting the visual drag state.
  let recurringScopePrompt = $state<{
    open: boolean;
    title: string;
    action: 'move' | 'resize';
    seriesTone: 'error' | 'warning' | 'subtext';
    onChoose: (scope: 'this' | 'series') => void;
    onCancel: () => void;
  } | null>(null);

  function askRecurringScope(
    title: string,
    action: 'move' | 'resize'
  ): Promise<'this' | 'series' | null> {
    return new Promise((resolve) => {
      recurringScopePrompt = {
        open: true,
        title,
        action,
        // Move/resize are less destructive than delete — the series
        // button stays warning (orange) not error (red). The picker
        // already special-cases this via the seriesTone prop.
        seriesTone: 'warning',
        onChoose: (s) => {
          recurringScopePrompt = null;
          resolve(s);
        },
        onCancel: () => {
          recurringScopePrompt = null;
          resolve(null);
        }
      };
    });
  }
  function onFindTimePick(start: Date, durationMinutes: number) {
    unifiedStart = start;
    unifiedEnd = new Date(start.getTime() + durationMinutes * 60_000);
    unifiedKind = 'event';
    unifiedOpen = true;
  }

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

  // load + loadHabits moved into dataCtl.

  onMount(() => dataCtl.load());
  onMount(() => dataCtl.loadSources());
  onMount(() => dataCtl.loadHabits());
  onMount(() => dataCtl.loadAllProjects());
  onMount(() => dataCtl.loadNativeEvents());
  onMount(() =>
    onWsEvent((ev) => {
      if (
        ev.type === 'note.changed' ||
        ev.type === 'note.removed' ||
        ev.type === 'event.changed' ||
        ev.type === 'event.removed' ||
        // task.changed fires from handlers_tasks.go on create / patch /
        // schedule / delete. Without it, dropping a task on the grid
        // or creating one via UnifiedCreate wouldn't repaint until the
        // user reloaded — the file-watcher's note.changed often races
        // the same-process write debounce and skips it.
        ev.type === 'task.changed'
      ) {
        dataCtl.load();
        // Habits live inside daily notes — a note change might mean a
        // habit was ticked. Refetch alongside the event dataCtl.feed.
        dataCtl.loadHabits();
      }
      // Refresh native event entries on any event change so the
      // Calendar Agent's scope reflects current state.
      if (ev.type === 'event.changed' || ev.type === 'event.removed') {
        dataCtl.loadNativeEvents();
      }
      // Deadlines are an overlay on the dataCtl.feed — refetch when the
      // server signals .granit/deadlines.json changed (TUI edit, web
      // edit in another tab, or anything else that calls SaveAll).
      if (ev.type === 'state.changed' && ev.path === '.granit/deadlines.json') dataCtl.load();
      // ICS mutations (create/edit/delete a new event in a subscribed
      // .ics file) broadcast as state.changed with a calendar path —
      // e.g. "calendars/personal.ics" or "merged.ics". Match the
      // path-shape rather than enumerate sources so new calendars added
      // mid-session refresh automatically.
      if (ev.type === 'state.changed' && ev.path && /\.ics$/.test(ev.path)) dataCtl.load();
      // Project metadata changed (rename, colour, status) — refresh
      // the picker so the filter dropdown stays in sync. We don't
      // touch the event dataCtl.feed here; project_id on filterCtl.events is captured
      // at write time, so a project rename doesn't transitively
      // re-key past filterCtl.events (matches the deliberate Task.Project shape).
      if (ev.type === 'project.changed' || ev.type === 'project.removed') {
        dataCtl.loadAllProjects();
      }
    })
  );

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
  let quickInput = $state('');
  let quickBusy = $state(false);
  const quickParse = $derived<ParseResult | null>(
    quickInput.trim() ? parseEventInput(quickInput) : null
  );

  async function submitQuickEvent() {
    if (!quickParse?.ok || !quickParse.event || quickBusy) return;
    quickBusy = true;
    try {
      const ev = quickParse.event;
      await api.createEvent({
        title: ev.title,
        date: ev.date,
        start_time: ev.startTime,
        end_time: ev.endTime
      });
      quickInput = '';
      toast.success('event created');
      await dataCtl.load();
    } catch (err) {
      toast.error('create failed: ' + (errorMessage(err)));
    } finally {
      quickBusy = false;
    }
  }
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

  // Keyboard shortcuts. Active only when nothing else has focus (so we
  // don't steal keystrokes from the create-event modal's inputs).
  // Mirrors Google Calendars default bindings so muscle memory carries
  // over: t = today, j/n = next, k/p = prev, d/w/m/y/a = viewCtl.view, ? = help.
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
    if (createOpen || createEventOpen || unifiedOpen || detailOpen || findTimeOpen) return;
    switch (e.key) {
      case 't': viewCtl.gotoToday(); break;
      case 'j': case 'n': viewCtl.next(); break;
      case 'k': case 'p': viewCtl.prev(); break;
      case 'd': viewCtl.view = 'day'; break;
      case 'w': viewCtl.view = 'week'; break;
      case 'W': viewCtl.view = 'workweek'; break; // Shift+W = workweek (Mon–Fri)
      case 'm': viewCtl.view = 'month'; break;
      case 'y': viewCtl.view = 'year'; break;
      case 'a': viewCtl.view = 'agenda'; break; // 'a' = agenda viewCtl.view (matches
                                        // Google Calendar). Shift+A
                                        // opens the calendar agent.
      case 'A': agentOpen = true; break;
      case 'f': findTimeOpen = true; break; // 'f' = find a free slot
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
    if (dx > 0) viewCtl.prev(); else viewCtl.next();
  }

  function clickEvent(ev: CalendarEvent) {
    // Goal targets are anchors back to /goals — opening the calendar
    // EventDetail for them would imply edit/reschedule semantics the
    // backend doesn't support (a goal's target_date is a property of
    // the goal, not a standalone event). Jump straight to the goal so
    // the user can adjust it from the source of truth.
    if (ev.type === 'goal_target' && ev.eventId) {
      goto(`/goals?focus=${encodeURIComponent(ev.eventId)}`);
      return;
    }
    // Meal slots are synthesized markers — no editable event exists
    // server-side, so the detail modal would offer reschedule/delete
    // that don't apply. Toggle done in-place instead; mirrors the
    // dashboard widget's interaction so a tick anywhere syncs to
    // both surfaces via the daily note.
    if (ev.type === 'meal_slot' && ev.start) {
      void toggleMealEvent(ev);
      return;
    }
    selected = ev;
    detailOpen = true;
    // Publish to the workspace context bus so an AI pane in the
    // adjacent slot can surface this event as context.
    workspaceContext.publish({
      paneKind: 'calendar',
      itemId: ev.eventId ?? `${ev.date ?? ''}|${ev.title ?? ''}`,
      label: ev.title ?? 'untitled event',
      excerpt: ev.date ?? undefined
    });
  }

  // toggleMealEvent flips the done state of a meal_slot event by
  // patching the underlying daily-note row. The (time, date) tuple
  // is enough to identify the slot — a day rarely has two meals at
  // the same minute, and the API's ApplyPatch matches on time-alone
  // when name is empty. We deliberately DON'T pass ev.title because
  // it carries the rendered "Breakfast — Haferflocken" combined
  // label which doesn't match the slot's bare Name field.
  async function toggleMealEvent(ev: CalendarEvent) {
    if (!ev.start) return;
    const start = new Date(ev.start);
    if (Number.isNaN(start.getTime())) return;
    const hh = String(start.getHours()).padStart(2, '0');
    const mm = String(start.getMinutes()).padStart(2, '0');
    const dateISO = ev.date ?? fmtDateISO(start);
    try {
      await api.patchMeal({
        time: `${hh}:${mm}`,
        date: dateISO,
        done: !ev.done
      });
      await dataCtl.load();
    } catch (e) {
      toast.error('Toggle meal failed: ' + errorMessage(e));
    }
  }
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
    const fmt = (d: Date) =>
      `${d.toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' })} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
    try {
      await api.patchTask(taskId, { scheduledStart: newStart.toISOString() });
      await dataCtl.load();
      toast.success(`Rescheduled to ${fmt(newStart)}`);
    } catch (e) {
      const msg = errorMessage(e);
      toast.error('Reschedule failed: ' + msg);
    }
  }
  async function dropTask(id: string, start: Date, dur: number) {
    try {
      await api.patchTask(id, {
        scheduledStart: start.toISOString(),
        durationMinutes: dur
      });
      await dataCtl.load();
      toast.success('scheduled');
    } catch (e) {
      toast.error('schedule failed: ' + (errorMessage(e)));
    }
  }
  // Move dispatch — receives the full event + new start Date and
  // routes to the right patch endpoint by event kind. Tasks already
  // have their own onReschedule path (taskId-based, unchanged); this
  // handler covers events.json + writable ICS. Duration is preserved
  // — drag-to-move only changes the start; drag-to-resize changes
  // duration. Two distinct gestures, two patch endpoints.
  async function moveEvent(ev: CalendarEvent, newStart: Date) {
    // Format helper for the success toast — keeps the toast call
    // sites clean, and the user gets unambiguous confirmation of
    // the actual time the move resolved to.
    const fmt = (d: Date) =>
      `${d.toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' })} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
    try {
      // Surface a clear toast when the user drag-released an event we
      // can't actually patch — the most common cause is a legacy
      // events.json entry without an ID (created before the ID-mint
      // path was added) or an ICS event whose source isn't writable.
      // Without this, the drag visually "works" (ghost moves with the
      // pointer) but the event snaps back on release with no feedback.
      if (!ev.eventId && !ev.taskId) {
        toast.error('This event is missing an ID and can\'t be moved. Try editing it in the detail view to mint one.');
        return;
      }
      if (ev.type === 'ics_event' && ev.source) {
        // ICS-specific gate: even if eventId is set, the source must
        // be writable. The HourGrid filters this in isMovable, but
        // a stale writableSources prop can let a drag fire that the
        // server then rejects with 403. Catch it here with a clear
        // message instead of a generic patchICSEvent failure toast.
        //
        // Prefer the server-stamped event.editable flag — it tracks
        // the actual file's location at feed time and survives
        // duplicate-filename scenarios where two .ics files share a
        // name but only one is writable. Falls back to dataCtl.calSources
        // lookup for legacy entries that pre-date the flag.
        const w =
          typeof ev.editable === 'boolean'
            ? ev.editable
            : !!dataCtl.calSources.find((s) => s.source === ev.source)?.writable;
        if (!w) {
          toast.error(`Read-only calendar (${ev.source}) — move the file to <vault>/calendars/ to enable edits.`);
          return;
        }
      }
      // Recurring-series UX: a recurring event has TWO valid drag
      // semantics — "this occurrence only" (per-instance override) or
      // "the whole series" (rewrite the base). Stream R replaced the
      // stacked native confirm() dialogs (where Esc-to-abort silently
      // triggered the destructive series path) with an explicit
      // RecurringScopePicker rendered inline on the page. The picker
      // resolves to 'this' | 'series' | null; null = user cancelled,
      // so we abort and let the drag visually revert via dataCtl.load().
      let recurringMode: 'series' | 'instance' | null = null;
      if (ev.rrule && ev.eventId) {
        const scope = await askRecurringScope(ev.title ?? 'this event', 'move');
        if (!scope) {
          // User cancelled — re-fetch so the optimistic drag visual
          // snaps back to the original slot. Without this, the ghost
          // releases at the new spot but the underlying event hasn't
          // moved, leaving the UI confused until the next refresh.
          await dataCtl.load();
          return;
        }
        recurringMode = scope === 'this' ? 'instance' : 'series';
      }
      // ICS "just this one" — fire skip + create-standalone in the
      // same order EventDetail uses. Falls through on failure with a
      // toast so the user can retry; the source occurrence is only
      // EXDATE'd AFTER the standalone create succeeds wouldn't be
      // safer here because the user already accepted the move, and a
      // partial state (extra event, no skip) leaves both visible —
      // less confusing than (skip done, no replacement) which leaves
      // a hole.
      if (recurringMode === 'instance' && ev.type === 'ics_event' && ev.eventId && ev.source && ev.start) {
        try {
          // Skip the ORIGINAL anchor date so the series no longer
          // renders the occurrence the user dragged.
          await api.skipICSOccurrence(ev.source, ev.eventId, ev.start);
        } catch (e) {
          toast.error('Move (skip) failed: ' + errorMessage(e));
          return;
        }
        const dur = ev.durationMinutes ?? 60;
        const endD = new Date(newStart.getTime() + dur * 60_000);
        try {
          await api.createICSEvent(ev.source, {
            summary: ev.title,
            start: newStart.toISOString(),
            end: endD.toISOString(),
            location: ev.location ?? undefined
          });
        } catch (e) {
          toast.error(
            'Move (create standalone) failed — the original occurrence is now hidden: ' +
              errorMessage(e)
          );
          return;
        }
        await dataCtl.load();
        toast.success(`Moved this occurrence to ${fmt(newStart)}`);
        return;
      }
      // Per-instance override path: write a single override entry
      // keyed by the occurrence's UTC ANCHOR (the series-base time
      // for this occurrence, NOT the currently rendered time). When
      // an override is already active, `ev.start` reflects the
      // overridden time, so re-deriving the key from `ev.start`
      // would mint a fresh override at the wrong anchor and the
      // original override would stay buried in the map. Use
      // ev.override_key when present (canonical anchor surfaced by
      // the calendar dataCtl.feed); fall back to ev.start for first-time
      // overrides where the rendered time IS the anchor.
      if (recurringMode === 'instance' && ev.type === 'event' && ev.eventId && ev.start) {
        const dateStr = `${newStart.getFullYear()}-${String(newStart.getMonth() + 1).padStart(2, '0')}-${String(newStart.getDate()).padStart(2, '0')}`;
        const startTime = `${String(newStart.getHours()).padStart(2, '0')}:${String(newStart.getMinutes()).padStart(2, '0')}`;
        const dur = ev.durationMinutes ?? 30;
        const startMinOnly = newStart.getHours() * 60 + newStart.getMinutes();
        const maxEndMinOnly = 24 * 60 - 1;
        const endMin = Math.min(startMinOnly + dur, maxEndMinOnly);
        const endTime = `${String(Math.floor(endMin / 60)).padStart(2, '0')}:${String(endMin % 60).padStart(2, '0')}`;
        // Slice the floating ISO directly instead of round-tripping
        // through Date+toISOString. The backend now emits start/end
        // as floating wall-clock ("2026-05-09T08:00:00", no Z, no
        // offset) and keys overrides by the same wall-clock digits.
        // new Date(...).toISOString() would re-anchor those digits
        // to the client zone and emit a UTC-shifted key — on a
        // UTC+2 client the key would land 2hr ahead of the anchor,
        // and the override would silently mint at the wrong slot.
        // The leading 19 chars of `ev.start` always carry the
        // YYYY-MM-DDTHH:MM:SS shape the server expects.
        const key = ev.override_key ?? ev.start.slice(0, 19);
        await api.overrideEventOccurrence(ev.eventId, key, {
          date: dateStr,
          start_time: startTime,
          end_time: endTime
        });
        await dataCtl.load();
        toast.success(`Moved this occurrence to ${fmt(newStart)}`);
        return;
      }
      if (ev.type === 'event' && ev.eventId) {
        const dateStr = `${newStart.getFullYear()}-${String(newStart.getMonth() + 1).padStart(2, '0')}-${String(newStart.getDate()).padStart(2, '0')}`;
        const startTime = `${String(newStart.getHours()).padStart(2, '0')}:${String(newStart.getMinutes()).padStart(2, '0')}`;
        // Preserve duration: take the event's old duration in
        // minutes, add to the new start to compute the new end. The
        // event used to span 14:30–16:00; dragging to 09:15 should
        // produce 09:15–10:45, not collapse to a zero-length event.
        //
        // Midnight clamp: events.json carries one `date` plus HH:MM
        // start/end strings — the schema can't represent an event
        // whose end falls on the next calendar day. Without this
        // clamp, dragging a 60-min event to 23:30 would emit
        // end_time="00:30", which the backend's validateEventTimes
        // refuses ("end_time must be after start_time"); the move
        // looked successful on the grid, then reverted on reload —
        // exactly the "places it somewhere else" symptom the user
        // reported. Clamp to 23:59 so the move always lands; the
        // user can extend it manually if they want a true cross-
        // midnight event (today not supported).
        const dur = ev.durationMinutes ?? 30;
        const startMin = newStart.getHours() * 60 + newStart.getMinutes();
        const maxEndMin = 24 * 60 - 1; // 23:59
        const endMin = Math.min(startMin + dur, maxEndMin);
        const endTime = `${String(Math.floor(endMin / 60)).padStart(2, '0')}:${String(endMin % 60).padStart(2, '0')}`;
        await api.patchEvent(ev.eventId, { date: dateStr, start_time: startTime, end_time: endTime });
      } else if (ev.type === 'ics_event' && ev.eventId && ev.source) {
        const dur = ev.durationMinutes ?? 60;
        const endD = new Date(newStart.getTime() + dur * 60_000);
        await api.patchICSEvent(ev.source, ev.eventId, {
          start: newStart.toISOString(),
          end: endD.toISOString()
        });
      } else {
        // Caught: the event doesn't match any of the known dispatch
        // branches (event / ics_event / task). Surface so the user
        // doesn't see a silent failure.
        toast.error(`Can't move this event type (${ev.type ?? 'unknown'}).`);
        return;
      }
      await dataCtl.load();
      toast.success(`Moved to ${fmt(newStart)}`);
    } catch (e) {
      const msg = errorMessage(e);
      toast.error('Move failed: ' + msg);
    }
  }

  // Resize dispatch — receives the full event so we can route to the
  // right patch endpoint based on event kind. Tasks → patchTask
  // (durationMinutes wires through to scheduledStart + duration in the
  // task store). events.json → patchEvent with a new HH:MM end_time
  // computed from start + duration. ICS → patchICSEvent with an
  // RFC3339 end built from the same arithmetic, since ICS stores
  // start/end as full timestamps not separate date+time fields.
  async function resizeEvent(ev: CalendarEvent, durationMinutes: number) {
    try {
      // Mirrors moveEvent's recurring chooser via the in-page picker
      // (Stream R). null = user cancelled — dataCtl.load() snaps the resize
      // visual back to its original duration.
      let recurringMode: 'series' | 'instance' | null = null;
      if (ev.rrule && !ev.taskId && ev.eventId) {
        const scope = await askRecurringScope(ev.title ?? 'this event', 'resize');
        if (!scope) {
          await dataCtl.load();
          return;
        }
        recurringMode = scope === 'this' ? 'instance' : 'series';
      }
      // ICS "just this one" resize — skip + create-standalone with the
      // edited duration. Same failure ordering as moveEvent.
      if (recurringMode === 'instance' && ev.type === 'ics_event' && ev.eventId && ev.source && ev.start) {
        try {
          await api.skipICSOccurrence(ev.source, ev.eventId, ev.start);
        } catch (e) {
          toast.error('Resize (skip) failed: ' + errorMessage(e));
          return;
        }
        const startD = new Date(ev.start);
        const endD = new Date(startD.getTime() + durationMinutes * 60_000);
        try {
          await api.createICSEvent(ev.source, {
            summary: ev.title,
            start: startD.toISOString(),
            end: endD.toISOString(),
            location: ev.location ?? undefined
          });
        } catch (e) {
          toast.error(
            'Resize (create standalone) failed — the original occurrence is now hidden: ' +
              errorMessage(e)
          );
          return;
        }
        await dataCtl.load();
        return;
      }
      if (recurringMode === 'instance' && ev.type === 'event' && ev.eventId && ev.start) {
        const startD = new Date(ev.start);
        const startMin = startD.getHours() * 60 + startD.getMinutes();
        const maxEndMin = 24 * 60 - 1;
        const endMin = Math.min(startMin + durationMinutes, maxEndMin);
        const endTime = `${String(Math.floor(endMin / 60)).padStart(2, '0')}:${String(endMin % 60).padStart(2, '0')}`;
        // See moveEvent: prefer the surfaced override_key over re-
        // deriving from ev.start so we don't mint a fresh override
        // at an already-overridden time. Slice the floating ISO
        // directly instead of new Date(...).toISOString() — see
        // the long-form note in moveEvent for why round-tripping
        // through Date silently shifts the key by the client offset.
        const key = ev.override_key ?? ev.start.slice(0, 19);
        // Resize keeps the start_time on the original occurrence date
        // unchanged — only end_time shifts. We still send start_time so
        // the override carries a complete (start, end) pair and the
        // expander doesn't have to merge with the series.
        const startTime = `${String(startD.getHours()).padStart(2, '0')}:${String(startD.getMinutes()).padStart(2, '0')}`;
        await api.overrideEventOccurrence(ev.eventId, key, {
          start_time: startTime,
          end_time: endTime
        });
        await dataCtl.load();
        return;
      }
      if (ev.taskId) {
        await api.patchTask(ev.taskId, { durationMinutes });
      } else if (ev.type === 'event' && ev.eventId && ev.start) {
        // events.json is keyed on date + HH:MM strings. The schema
        // can't represent an event ending on the next calendar day,
        // so a resize that would push end past 23:59 must clamp.
        // Without the clamp, the backend's validateEventTimes refuses
        // ("end_time must be after start_time", string compare on
        // HH:MM) and the resize silently reverts — that's part of the
        // "drag make it longer ... places it somewhere else" report.
        const startD = new Date(ev.start);
        const startMin = startD.getHours() * 60 + startD.getMinutes();
        const maxEndMin = 24 * 60 - 1; // 23:59
        const endMin = Math.min(startMin + durationMinutes, maxEndMin);
        const endTime = `${String(Math.floor(endMin / 60)).padStart(2, '0')}:${String(endMin % 60).padStart(2, '0')}`;
        await api.patchEvent(ev.eventId, { end_time: endTime });
      } else if (ev.type === 'ics_event' && ev.eventId && ev.source && ev.start) {
        // Same editable gate as moveEvent — read-only sources can't
        // be resized either. Prefer the server-stamped flag.
        const w =
          typeof ev.editable === 'boolean'
            ? ev.editable
            : !!dataCtl.calSources.find((s) => s.source === ev.source)?.writable;
        if (!w) {
          toast.error(`Read-only calendar (${ev.source}) — can't resize this event.`);
          return;
        }
        // ICS uses RFC3339 — full timestamps, so cross-midnight is
        // representable. No clamp needed; the writer will normalize
        // to UTC on emit.
        const startD = new Date(ev.start);
        const endD = new Date(startD.getTime() + durationMinutes * 60_000);
        await api.patchICSEvent(ev.source, ev.eventId, { end: endD.toISOString() });
      }
      await dataCtl.load();
    } catch (e) {
      // Surface a toast so the user sees the resize failed instead
      // of watching the bar snap back silently. Mirrors moveEvent's
      // error path — every drag gesture should give clear feedback
      // on outcome.
      const msg = errorMessage(e);
      toast.error('Resize failed: ' + msg);
    }
  }
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
        unifiedStart = s; unifiedEnd = e; unifiedKind = 'task'; unifiedOpen = true;
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
      onFindTime={() => (findTimeOpen = true)}
      onShowShortcuts={() => (showShortcutHelp = true)}
      onOpenFilterDrawer={() => (filterCtl.filterDrawerOpen = true)}
      onCapture={() => {
        // Seed UnifiedCreate with the next round hour so the user
        // doesn't have to re-type the time. Same default the sidebar
        // "+ New task or event" button uses.
        const s = new Date();
        s.setMinutes(0, 0, 0);
        s.setHours(s.getHours() + 1);
        const e = new Date(s.getTime() + 60 * 60 * 1000);
        unifiedStart = s;
        unifiedEnd = e;
        unifiedKind = 'event';
        unifiedOpen = true;
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
        bind:value={quickInput}
        placeholder='e.g. "lunch tomorrow 12pm 1h" or "team mtg fri 14:00-15:00"'
        class="flex-1 min-w-0 px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
        aria-label="quick-create event"
        disabled={quickBusy}
      />
      {#if quickInput.trim()}
        <span class="hidden md:inline text-[11px] text-dim font-mono truncate max-w-md">
          {#if quickParse?.ok && quickParse.event}
            <span class="text-success">✓</span>
            {quickParse.event.title} · {quickParse.event.date}{quickParse.event.startTime ? ` · ${quickParse.event.startTime}${quickParse.event.endTime ? `–${quickParse.event.endTime}` : ''}` : ' · all-day'}
          {:else}
            <span class="text-warning">{quickParse?.hint ?? 'parsing…'}</span>
          {/if}
        </span>
        <button
          type="submit"
          disabled={!quickParse?.ok || quickBusy}
          class="px-2.5 py-1 text-xs bg-primary text-on-primary rounded font-medium disabled:opacity-40 flex-shrink-0"
        >{quickBusy ? '…' : 'Add'}</button>
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
              unifiedStart = s;
              unifiedEnd = e;
              unifiedKind = 'event';
              unifiedOpen = true;
            }}
          />
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
        onclick={() => (showShortcutHelp = false)}
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
{#if recurringScopePrompt?.open}
  <div
    role="dialog"
    aria-modal="true"
    aria-labelledby="recurring-scope-title"
    class="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4"
    onclick={(e) => {
      if (e.target === e.currentTarget && recurringScopePrompt) recurringScopePrompt.onCancel();
    }}
    onkeydown={(e) => {
      if (e.key === 'Escape' && recurringScopePrompt) recurringScopePrompt.onCancel();
    }}
    tabindex="-1"
  >
    <div class="bg-mantle border border-surface1 rounded-lg p-4 max-w-md w-full shadow-xl">
      <h3 id="recurring-scope-title" class="text-sm font-semibold text-text mb-1">
        {recurringScopePrompt.action === 'move' ? 'Move' : 'Resize'} recurring event
      </h3>
      <p class="text-xs text-dim italic mb-2">
        Pick a scope. "Just this occurrence" keeps the rest of the series in place.
      </p>
      <RecurringScopePicker
        eventTitle={recurringScopePrompt.title}
        action={recurringScopePrompt.action}
        seriesTone={recurringScopePrompt.seriesTone}
        onChoose={(s) => {
          // The picker emits 'this' | 'future' | 'series'. We only
          // surface this / series for move + resize; future is not
          // a supported recurring-edit semantic for drag yet.
          if (s === 'future') return;
          recurringScopePrompt?.onChoose(s);
        }}
        onCancel={() => recurringScopePrompt?.onCancel()}
      />
    </div>
  </div>
{/if}

<EventDetail bind:open={detailOpen} event={selected} onChanged={() => dataCtl.load()} />
<QuickCreateScheduled
  bind:open={createOpen}
  date={createDate}
  hour={createHour}
  minute={createMinute}
  defaultNotePath={`Jots/${fmtDateISO(createDate)}.md`}
  onCreated={() => dataCtl.load()}
/>
<CreateEvent
  bind:open={createEventOpen}
  date={createEventDate}
  existingEvents={filterCtl.events}
  defaultProjectId={filterCtl.projectFilter}
  onCreated={() => dataCtl.load()}
/>
<UnifiedCreate
  bind:open={unifiedOpen}
  start={unifiedStart}
  end={unifiedEnd}
  defaultKind={unifiedKind}
  defaultNotePath={`Jots/${fmtDateISO(unifiedStart)}.md`}
  onCreated={() => dataCtl.load()}
/>
<FindTime bind:open={findTimeOpen} events={filterCtl.events} onPick={onFindTimePick} />

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

