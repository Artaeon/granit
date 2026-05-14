<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { auth } from '$lib/stores/auth';
  import { api, type CalendarEvent, type CalendarEventEntry, type CalendarFeed, type CalendarSource, type HabitInfo, type Project, type Task } from '$lib/api';
  import CalendarAgent from '$lib/calendar/CalendarAgent.svelte';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import { mediaQuery } from '$lib/util/mediaQuery';
  import { loadStored, loadStoredString, saveStored, saveStoredString } from '$lib/util/storage';
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
  import { sourceColors, setSourceColor, applySourceColor, type CalendarTone } from '$lib/calendar/sourceColors';
  import EventDetail from '$lib/calendar/EventDetail.svelte';
  import QuickCreateScheduled from '$lib/calendar/QuickCreateScheduled.svelte';
  import CreateEvent from '$lib/calendar/CreateEvent.svelte';
  import UnifiedCreate from '$lib/calendar/UnifiedCreate.svelte';
  import { parseEventInput, type ParseResult } from '$lib/calendar/quickCreate';
  import TaskBacklog from '$lib/calendar/TaskBacklog.svelte';
  import Drawer from '$lib/components/Drawer.svelte';
  import { onWsEvent } from '$lib/ws';
  import { dragStore } from '$lib/calendar/dragStore';
  import { onDestroy } from 'svelte';

  type View = 'day' | 'week' | 'month' | 'year';

  // Persisted last-used view (per device). On a fresh visit (no
  // saved preference) we default to 'day' on small screens because
  // the 7-column week grid is unreadable below ~640px — Google
  // Calendar does the same. Saved preferences win regardless of
  // device, so a user who explicitly picked 'week' on mobile keeps
  // that choice.
  const VIEW_KEY = 'granit.calendar.view';
  function defaultView(): View {
    if (typeof window === 'undefined') return 'week';
    const saved = loadStoredString(VIEW_KEY, '');
    if (saved) return saved as View;
    return window.innerWidth < 640 ? 'day' : 'week';
  }
  let view = $state<View>(defaultView());
  let cursor = $state(new Date());

  // Plan mode — splits the page into a left-side backlog of today's
  // open tasks and the regular hour grid. Persisted so the user's
  // choice carries across sessions / devices (per-device localStorage).
  // Plan mode is single-day in v1: turning it on forces view='day' so
  // the layout stays sensible (a 7-column week grid + side rail is too
  // tight on most screens).
  const PLAN_KEY = 'granit.calendar.planmode';
  let planMode = $state<boolean>(loadStoredString(PLAN_KEY, '0') === '1');

  $effect(() => saveStoredString(VIEW_KEY, view));
  $effect(() => saveStoredString(PLAN_KEY, planMode ? '1' : '0'));

  function togglePlanMode() {
    planMode = !planMode;
    if (planMode) view = 'day';
  }

  // Clean exit on route change: drop any pending drag pick. Otherwise
  // a stale dragStore could corrupt the next page's pointer behaviour.
  onDestroy(() => dragStore.set(null));

  let feed = $state<CalendarFeed | null>(null);
  let habits = $state<HabitInfo[]>([]);
  let loading = $state(false);

  // Native calendar event entries — the editable rows the
  // Calendar Agent operates on. Loaded separately from `feed`
  // (which is the expanded read-only render view including ICS
  // sources, tasks, deadlines). Refreshed on event.* WS frames.
  let nativeEvents = $state<CalendarEventEntry[]>([]);
  let agentOpen = $state(false);
  async function loadNativeEvents() {
    if (!$auth) return;
    try {
      const r = await api.listEvents();
      nativeEvents = r.events;
    } catch {
      nativeEvents = [];
    }
  }

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
  // Reactive mobile flag via the shared mediaQuery store. Auto-cleans
  // up on component destroy. The first-mount "force day view on
  // mobile" rule still applies, see the $effect below.
  const isMobile = mediaQuery('(max-width: 767px)');
  let _mobileViewForced = $state(false);
  $effect(() => {
    if ($isMobile && !_mobileViewForced) {
      view = 'day';
      _mobileViewForced = true;
    }
  });

  // Event-type filter: each toggle hides events of that type. Persisted so
  // the user's preference (e.g. "always hide ICS") sticks across sessions.
  type EventFilterKey = 'daily' | 'task_due' | 'task_scheduled' | 'event' | 'ics_event' | 'deadline';
  const FILTER_KEY = 'granit.calendar.filters';
  let hidden = $state<Set<EventFilterKey>>(
    new Set(loadStored<EventFilterKey[]>(FILTER_KEY, []))
  );

  $effect(() => saveStored(FILTER_KEY, Array.from(hidden)));

  function toggleType(t: EventFilterKey) {
    const next = new Set(hidden);
    if (next.has(t)) next.delete(t);
    else next.add(t);
    hidden = next;
  }

  // Project filter — when set to a non-empty project name, the grid
  // only shows events + tasks linked to that project (via project_id
  // on the wire shape). Persisted per-device so a user using the
  // calendar as a project board stays scoped on reload. Empty = "all".
  const PROJECT_FILTER_KEY = 'granit.calendar.project';
  let projectFilter = $state<string>(loadStoredString(PROJECT_FILTER_KEY, ''));
  $effect(() => saveStoredString(PROJECT_FILTER_KEY, projectFilter));

  // Project list — used by the filter dropdown + the "colour by
  // project" overlay. Loaded once on mount; refreshed on demand if a
  // ws project.changed broadcast lands. Failure degrades to empty.
  let allProjects = $state<Project[]>([]);
  async function loadAllProjects() {
    try {
      const r = await api.listProjects();
      allProjects = r.projects ?? [];
    } catch {
      allProjects = [];
    }
  }

  // Colour-by-project toggle. Off by default (the per-event colour
  // and the per-source ICS colour rotation already give visual
  // separation); flipping it on tints every project-linked row with
  // the project's `color` field, so a project board view becomes
  // visually unified.
  const COLOR_BY_PROJECT_KEY = 'granit.calendar.colorByProject';
  let colorByProject = $state<boolean>(loadStoredString(COLOR_BY_PROJECT_KEY, '0') === '1');
  $effect(() => saveStoredString(COLOR_BY_PROJECT_KEY, colorByProject ? '1' : '0'));

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
      toast.error('save failed: ' + (errorMessage(e)));
    } finally {
      savingSources = false;
    }
  }

  // Mobile detection moved to the mediaQuery store + $effect above.

  // Deep-link: ?plan=1 (optionally with &project=NAME) flips on plan
  // mode so other pages — e.g. the project detail's "schedule next
  // action" button — can hand off into the calendar in the right state.
  // ?project=NAME (without &plan) just scopes the view to that
  // project — the "open this project's calendar" hand-off.
  onMount(() => {
    if (typeof window === 'undefined') return;
    const url = new URL(window.location.href);
    if (url.searchParams.get('plan') === '1' && !planMode) {
      planMode = true;
      view = 'day';
    }
    const proj = url.searchParams.get('project');
    if (proj) projectFilter = proj;
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

  // Habits power the per-day "habits" overlay row in HourGrid.
  // Independent of the event feed — toggling a habit shouldn't refetch
  // the calendar (and vice versa).
  async function loadHabits() {
    if (!$auth) return;
    try {
      const r = await api.listHabits();
      habits = r.habits;
    } catch {
      habits = [];
    }
  }

  onMount(load);
  onMount(loadSources);
  onMount(loadHabits);
  onMount(loadAllProjects);
  onMount(loadNativeEvents);
  onMount(() =>
    onWsEvent((ev) => {
      if (
        ev.type === 'note.changed' ||
        ev.type === 'note.removed' ||
        ev.type === 'event.changed' ||
        ev.type === 'event.removed'
      ) {
        load();
        // Habits live inside daily notes — a note change might mean a
        // habit was ticked. Refetch alongside the event feed.
        loadHabits();
      }
      // Refresh native event entries on any event change so the
      // Calendar Agent's scope reflects current state.
      if (ev.type === 'event.changed' || ev.type === 'event.removed') {
        loadNativeEvents();
      }
      // Deadlines are an overlay on the feed — refetch when the
      // server signals .granit/deadlines.json changed (TUI edit, web
      // edit in another tab, or anything else that calls SaveAll).
      if (ev.type === 'state.changed' && ev.path === '.granit/deadlines.json') load();
      // Project metadata changed (rename, colour, status) — refresh
      // the picker so the filter dropdown stays in sync. We don't
      // touch the event feed here; project_id on events is captured
      // at write time, so a project rename doesn't transitively
      // re-key past events (matches the deliberate Task.Project shape).
      if (ev.type === 'project.changed' || ev.type === 'project.removed') {
        loadAllProjects();
      }
    })
  );

  // load() reads fetchFrom/fetchTo synchronously and may reassign them
  // when cursor walks outside the prefetch window. Without untrack, the
  // re-assignment refires this effect (one extra fetch per far-jump
  // navigation). The explicit `void` list above is the actual dep set.
  $effect(() => {
    void cursor;
    void view;
    untrack(() => load());
  });

  let allEvents = $derived(feed?.events ?? []);
  // Apply per-source color overrides so an ICS calendar the user
  // tinted 'green' renders that way on every view. Pure transform
  // on the derived event list — no extra fetches, no storage round
  // trip. The override is per-device (localStorage); future cross-
  // device sync can layer over the same map without touching the
  // render path.
  //
  // Project filter and project-tint layer on top in two stages:
  //   1. If projectFilter is set, drop every row whose project_id !=
  //      the picked project — the calendar becomes a project board.
  //   2. If colorByProject is on, override the row's `color` with the
  //      project's saved colour (events.json events). The override
  //      runs through the existing eventTypeColor mapping (red/yellow
  //      /green/...), so it composes cleanly with the per-source
  //      and per-event paths.
  let projectColorMap = $derived.by(() => {
    const m = new Map<string, string>();
    for (const p of allProjects) {
      if (p.name && p.color) m.set(p.name, p.color);
    }
    return m;
  });
  let events = $derived(
    allEvents
      .filter((e) => !hidden.has(e.type as EventFilterKey))
      .filter((e) => {
        if (!projectFilter) return true;
        return e.project_id === projectFilter;
      })
      .map((e) => applySourceColor(e, $sourceColors))
      .map((e) => {
        if (!colorByProject || !e.project_id) return e;
        const c = projectColorMap.get(e.project_id);
        if (!c) return e;
        // Only override the user-event color path — task / deadline
        // rows have their own meaningful colour rules (priority,
        // importance) we shouldn't trample.
        if (e.type !== 'event' && e.type !== 'task_scheduled' && e.type !== 'task_due') return e;
        return { ...e, color: c };
      })
  );

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
      await load();
    } catch (err) {
      toast.error('create failed: ' + (errorMessage(err)));
    } finally {
      quickBusy = false;
    }
  }
  let typeCounts = $derived.by(() => {
    const c: Record<string, number> = {};
    for (const e of allEvents) c[e.type] = (c[e.type] ?? 0) + 1;
    return c;
  });

  let viewDays = $derived.by(() => {
    if (view === 'day') return [cursor];
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
    else if (view === 'week') cursor = addDays(cursor, -7);
    else if (view === 'month') cursor = new Date(cursor.getFullYear(), cursor.getMonth() - 1, 1);
    else if (view === 'year') cursor = new Date(cursor.getFullYear() - 1, cursor.getMonth(), 1);
  }
  function next() {
    if (view === 'day') cursor = addDays(cursor, 1);
    else if (view === 'week') cursor = addDays(cursor, 7);
    else if (view === 'month') cursor = new Date(cursor.getFullYear(), cursor.getMonth() + 1, 1);
    else if (view === 'year') cursor = new Date(cursor.getFullYear() + 1, cursor.getMonth(), 1);
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
      case 'm': view = 'month'; break;
      case 'y': view = 'year'; break;
      case 'a': case 'A': agentOpen = true; break; // 'a' opens the agent
                                                   // (matches other pages
                                                   // now that the agenda
                                                   // view is gone).
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
    const fmt = (d: Date) =>
      `${d.toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' })} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
    try {
      await api.patchTask(taskId, { scheduledStart: newStart.toISOString() });
      await load();
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
      await load();
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
        // name but only one is writable. Falls back to calSources
        // lookup for legacy entries that pre-date the flag.
        const w =
          typeof ev.editable === 'boolean'
            ? ev.editable
            : !!calSources.find((s) => s.source === ev.source)?.writable;
        if (!w) {
          toast.error(`Read-only calendar (${ev.source}) — move the file to <vault>/calendars/ to enable edits.`);
          return;
        }
      }
      // Recurring-series UX: a recurring event has TWO valid drag
      // semantics — "this occurrence only" (per-instance override) or
      // "the whole series" (rewrite the base). Native browser dialogs
      // only expose two-choice yes/no, so we flip the question to a
      // confirm: OK = just this one (the friendly default that won't
      // surprise the user with calendar-wide cascading changes),
      // Cancel-bypass = whole series after a second confirm. ICS
      // events take the series-only path because their patch endpoint
      // doesn't support overrides today.
      let recurringMode: 'series' | 'instance' | null = null;
      if (ev.rrule && ev.eventId) {
        if (ev.type === 'event') {
          const justThisOne = confirm(
            `"${ev.title}" is a recurring event.\n\nOK: Move just this occurrence\nCancel: Move the entire series (every occurrence shifts by the same delta)\n\nClose this dialog to abort.`
          );
          if (justThisOne) {
            recurringMode = 'instance';
          } else {
            const confirmSeries = confirm(
              `Move ALL occurrences of "${ev.title}"? This rewrites the series base — every past and future instance shifts.`
            );
            if (!confirmSeries) return;
            recurringMode = 'series';
          }
        } else {
          // ICS branch — same scope picker as native events. The
          // backend grew skip + create-standalone primitives so we
          // can now offer "just this one" for ICS too. EventDetail
          // already uses the same pattern; this is the drag-move
          // counterpart so the UX stays consistent across surfaces.
          const justThisOne = confirm(
            `"${ev.title}" is a recurring ICS event.\n\nOK: Move just this occurrence (the original date is EXDATE'd; a standalone VEVENT lands at the new time)\nCancel: Move the entire series (every occurrence shifts by the same delta)\n\nClose this dialog to abort.`
          );
          if (justThisOne) {
            recurringMode = 'instance';
          } else {
            const confirmSeries = confirm(
              `Move ALL occurrences of "${ev.title}"? This rewrites the series base — every past and future instance shifts.`
            );
            if (!confirmSeries) return;
            recurringMode = 'series';
          }
        }
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
        await load();
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
      // the calendar feed); fall back to ev.start for first-time
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
        await load();
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
      await load();
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
      // Mirrors moveEvent's recurring chooser: this-one vs whole-series.
      let recurringMode: 'series' | 'instance' | null = null;
      if (ev.rrule && !ev.taskId && ev.eventId) {
        if (ev.type === 'event') {
          const justThisOne = confirm(
            `"${ev.title}" is a recurring event.\n\nOK: Resize just this occurrence\nCancel: Resize the entire series\n\nClose this dialog to abort.`
          );
          if (justThisOne) {
            recurringMode = 'instance';
          } else {
            const confirmSeries = confirm(
              `Resize ALL occurrences of "${ev.title}"?`
            );
            if (!confirmSeries) return;
            recurringMode = 'series';
          }
        } else {
          // ICS: same skip + create-standalone pattern as moveEvent.
          // Resize-just-this is detach the occurrence and write a
          // replacement VEVENT with the new end time.
          const justThisOne = confirm(
            `"${ev.title}" is a recurring ICS event.\n\nOK: Resize just this occurrence (the original date is EXDATE'd; a standalone VEVENT lands with the new duration)\nCancel: Resize the entire series\n\nClose this dialog to abort.`
          );
          if (justThisOne) {
            recurringMode = 'instance';
          } else {
            const confirmSeries = confirm(
              `Resize ALL occurrences of "${ev.title}"?`
            );
            if (!confirmSeries) return;
            recurringMode = 'series';
          }
        }
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
        await load();
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
        await load();
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
            : !!calSources.find((s) => s.source === ev.source)?.writable;
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
      await load();
    } catch (e) {
      // Surface a toast so the user sees the resize failed instead
      // of watching the bar snap back silently. Mirrors moveEvent's
      // error path — every drag gesture should give clear feedback
      // on outcome.
      const msg = errorMessage(e);
      toast.error('Resize failed: ' + msg);
    }
  }
  function clickDay(d: Date) { cursor = d; view = 'day'; }
  function pickDay(d: Date) { cursor = d; filterDrawerOpen = false; }

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
    if (view === 'day') return cursor.toLocaleDateString(undefined, { weekday: $isMobile ? 'short' : 'long', month: 'short', day: 'numeric' });
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
        filterDrawerOpen = false;
      }}
      class="w-full px-3 py-2.5 bg-primary text-on-primary rounded text-sm font-medium hover:opacity-90"
    >
      + New task or event
    </button>
    <p class="text-[11px] text-dim italic px-1 -mt-2">…or click + drag on the grid</p>
    <MiniMonth cursor={cursor} selected={cursor} events={monthEvents} onPick={pickDay} />

    <!-- Project filter — turn the calendar into a project-management
         board for one project at a time. Empty value = "all projects".
         Pairs with the per-event project picker; an event linked to
         project X shows up only when the filter is empty or set to X.
         "Colour by project" toggle below tints linked rows with the
         project's saved colour for at-a-glance visual grouping. -->
    {#if allProjects.length > 0}
      <div class="space-y-1.5 text-xs">
        <h3 class="text-dim uppercase tracking-wider mb-2">Project board</h3>
        <select
          bind:value={projectFilter}
          aria-label="filter calendar by project"
          class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
        >
          <option value="">All projects</option>
          {#each allProjects as p (p.name)}
            <option value={p.name}>{p.name}</option>
          {/each}
        </select>
        <label class="flex items-center gap-2 px-1 py-0.5 text-[11px] text-subtext cursor-pointer select-none">
          <input
            type="checkbox"
            bind:checked={colorByProject}
            class="w-3.5 h-3.5 accent-primary"
          />
          Colour by project
        </label>
        {#if projectFilter}
          <p class="text-[10px] text-dim italic px-1 leading-snug">
            Showing only events + tasks linked to <span class="text-secondary">{projectFilter}</span>.
            <button
              type="button"
              onclick={() => (projectFilter = '')}
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
    {#if calSources.length > 0}
      <div class="space-y-1 text-xs">
        <h3 class="text-dim uppercase tracking-wider mb-2">Calendar sources</h3>
        {#each calSources as s (s.id)}
          {@const tone = sourceColorToken(s.source)}
          {@const customTone = $sourceColors[s.source] ?? ''}
          {@const displayTone = customTone || tone}
          <div class="flex items-center gap-1 px-1 group">
            <button
              onclick={() => toggleSource(s)}
              disabled={savingSources}
              class="flex items-center gap-2 px-2 py-1 rounded hover:bg-surface0 flex-1 min-w-0 {s.enabled ? '' : 'opacity-40'}"
              title="{s.path}{s.enabled ? '' : ' (disabled)'}"
            >
              <span class="w-3 h-3 rounded-sm border flex items-center justify-center flex-shrink-0"
                style="border-color: var(--color-{displayTone}); background: {s.enabled ? `var(--color-${displayTone})` : 'transparent'}">
                {#if s.enabled}
                  <svg viewBox="0 0 12 12" class="w-2.5 h-2.5 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
                {/if}
              </span>
              <span class="text-subtext flex-1 text-left truncate">{s.source}</span>
              {#if s.folder}<span class="text-dim text-[10px]">{s.folder}</span>{/if}
            </button>
            <!-- Per-source color picker. Hidden until hover so the
                 row stays visually quiet, then a small swatch row
                 appears with all tone options. Empty swatch resets
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
        <p class="text-[10px] text-dim italic px-2 pt-1">hover a row to recolour · colours sync per device · toggles sync with granit TUI's <code>disabled_calendars</code></p>
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
        { key: 'ics_event', label: 'ICS calendars', tone: 'info' },
        { key: 'deadline', label: 'Deadlines', tone: 'error' }
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
  <aside class="hidden md:block md:w-56 lg:w-64 border-r border-surface1 bg-mantle flex-shrink-0 overflow-y-auto">
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
      <!-- Jump to a specific date. Hidden on the smallest screens
           where the header is already crowded; the prev/next +
           "today" buttons cover the common case. The input is
           always reset back to empty after a navigation so re-
           selecting the same date triggers the jump again. -->
      <input
        type="date"
        aria-label="jump to date"
        title="Jump to date"
        onchange={(e) => {
          const v = (e.target as HTMLInputElement).value;
          if (!v) return;
          const [y, mo, d] = v.split('-').map(Number);
          if (y && mo && d) cursor = new Date(y, mo - 1, d);
          (e.target as HTMLInputElement).value = '';
        }}
        class="hidden sm:block px-1.5 py-1 text-xs bg-surface0 border border-surface1 rounded text-dim hover:border-primary focus:border-primary focus:outline-none"
      />
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
      <!-- Calendar Agent (Plan my week, Find free time, Day insight,
           Dashboard) lives in the chat sidebar — Run agent on
           /calendar. Header stays minimal: nav + view switcher. -->
      <!-- View switcher — Apple Calendar set: Day / Week / Month / Year.
           3-day and Agenda were retired alongside the AI toolbar
           cleanup; Agenda's content lives in /tasks + the chat
           sidebar's "what's coming up" prompts. -->
      <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-xs sm:text-sm">
        <button
          class="px-2 sm:px-3 py-1.5 {view === 'day' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
          onclick={() => (view = 'day')}
        >Day</button>
        <button
          class="px-2 sm:px-3 py-1.5 {view === 'week' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
          onclick={() => (view = 'week')}
        >Week</button>
        <button
          class="px-2 sm:px-3 py-1.5 {view === 'month' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
          onclick={() => (view = 'month')}
        >Month</button>
        <button
          class="px-2 sm:px-3 py-1.5 {view === 'year' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'} hidden sm:inline-block"
          onclick={() => (view = 'year')}
        >Year</button>
      </div>
      <button
        onclick={() => (showShortcutHelp = true)}
        aria-label="keyboard shortcuts"
        title="Keyboard shortcuts (?)"
        class="hidden md:flex w-8 h-8 items-center justify-center text-dim hover:text-text hover:bg-surface0 rounded text-xs font-mono"
      >?</button>
    </header>

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

    <div
      class="flex-1 overflow-hidden p-2 sm:p-3"
      role="region"
      aria-label="calendar grid"
      ontouchstart={onTouchStart}
      ontouchend={onTouchEnd}
    >
      {#if planMode && (view === 'day' || view === 'week')}
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
              habits={habits}
              onClickEvent={clickEvent}
              onClickSlot={clickSlot}
              onSlotRange={onSlotRange}
              onReschedule={reschedule}
              onMove={moveEvent}
              onResize={resizeEvent}
              writableSources={calSources.filter((s) => s.writable).map((s) => s.source)}
              onTaskDrop={dropTask}
            />
          </div>
        </div>
      {:else if view === 'day' || view === 'week'}
        <HourGrid days={viewDays} events={events} habits={habits} onClickEvent={clickEvent} onClickSlot={clickSlot} onSlotRange={onSlotRange} onReschedule={reschedule} onMove={moveEvent} onResize={resizeEvent} writableSources={calSources.filter((s) => s.writable).map((s) => s.source)} />
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
    <div class="bg-mantle border border-surface1 rounded-lg p-5 max-w-sm w-full max-h-[90dvh] overflow-y-auto text-left shadow-xl">
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

<EventDetail bind:open={detailOpen} event={selected} onChanged={load} />
<QuickCreateScheduled
  bind:open={createOpen}
  date={createDate}
  hour={createHour}
  minute={createMinute}
  defaultNotePath={`Jots/${fmtDateISO(createDate)}.md`}
  onCreated={load}
/>
<CreateEvent
  bind:open={createEventOpen}
  date={createEventDate}
  existingEvents={events}
  defaultProjectId={projectFilter}
  onCreated={load}
/>
<UnifiedCreate
  bind:open={unifiedOpen}
  start={unifiedStart}
  end={unifiedEnd}
  defaultKind={unifiedKind}
  defaultNotePath={`Jots/${fmtDateISO(unifiedStart)}.md`}
  onCreated={load}
/>

<!-- Calendar Agent — scoped to the currently-visible fetch
     window AND the active project filter so the agent sees
     roughly what the user is looking at. Without the project
     filter intersection, asking "rename the client meetings"
     while filtered to a venture would surprise the user by
     proposing renames across all events. -->
<CalendarAgent
  open={agentOpen}
  events={nativeEvents.filter((e) =>
    e.date >= fmtDateISO(fetchFrom) &&
    e.date <= fmtDateISO(fetchTo) &&
    (!projectFilter || e.project_id === projectFilter)
  )}
  todayISO={fmtDateISO(new Date())}
  knownProjects={allProjects.map((p) => p.name)}
  onClose={() => (agentOpen = false)}
  onChanged={() => { void load(); void loadNativeEvents(); }}
/>

