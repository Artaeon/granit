<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { api, fmtDateISO, todayISO, type CalendarEvent, type Task, type Deadline } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { glyphForKind } from '$lib/calendar/eventTypes';
  import { createCoalescedReload } from '$lib/util/coalesce';

  // TodayStreamWidget — single span-2 widget that answers "what's
  // happening today" in one chronological feed. Today's events,
  // today's scheduled tasks, today's due tasks, and today's due
  // deadlines are merged into a single time-sorted list with past
  // items dimmed. A small two-day footer shows tomorrow + the day
  // after as compact event-and-task counts so the user reads "the
  // shape of right now AND what's coming" without scrolling.
  //
  // Why one widget: the existing dashboard had four separate
  // today-* widgets (today-tasks, scheduled-today, today-focus,
  // calendar-week). They worked but the user had to mentally merge
  // four lists to know what's next. This widget merges them once,
  // server-side-shape-aware, so the dashboard reads like a single
  // sentence rather than a checklist of checklists.

  type StreamRow = {
    /** HH:MM for sort + display, or '' for all-day. Empty sorts to top. */
    time: string;
    /** Past = before now, dims the row. */
    past: boolean;
    /** Visual category. */
    kind: 'event' | 'task-scheduled' | 'task-due' | 'deadline' | 'now';
    label: string;
    href: string;
    /** Optional secondary line (project, location, est minutes). */
    meta?: string;
    /** Optional id for keyed iteration. */
    id?: string;
    /** Single-char glyph for the event's type (meeting/focus/...).
     *  Empty when the event has no type or the row isn't an event. */
    typeGlyph?: string;
  };

  let events = $state<CalendarEvent[]>([]);
  let tasks = $state<Task[]>([]);
  let deadlines = $state<Deadline[]>([]);
  let loaded = $state(false);

  // Live "now" — refreshes once a minute so the past/future split
  // updates without a page reload. Cleared on destroy.
  let now = $state(new Date());
  let nowTick: ReturnType<typeof setInterval> | null = null;

  const today = todayISO();

  function startOfDay(d: Date): Date {
    const x = new Date(d); x.setHours(0, 0, 0, 0); return x;
  }
  function addDays(d: Date, n: number): Date {
    const x = new Date(d); x.setDate(d.getDate() + n); return x;
  }

  async function load() {
    // Three days of calendar feed — today + next two — so the same
    // call covers both the today stream and the upcoming preview.
    const start = fmtDateISO(new Date());
    const end = fmtDateISO(addDays(new Date(), 2));
    const [c, t, d] = await Promise.allSettled([
      api.calendar(start, end),
      api.listTasks({ status: 'open' }),
      api.tryListDeadlines()
    ]);
    events = c.status === 'fulfilled' ? c.value.events : [];
    tasks = t.status === 'fulfilled' ? t.value.tasks : [];
    deadlines = d.status === 'fulfilled' ? (d.value ?? []) : [];
    loaded = true;
  }

  // Coalesce the three-way reload (calendar + tasks + deadlines) into
  // one trailing refresh per 600ms window. Without this, editor
  // autosave bursts (one note.changed per keystroke) would refire the
  // full three-way fetch on every key, multiplying the per-keystroke
  // server cost by N widgets that follow the same pattern.
  const reload = createCoalescedReload(load, 600);

  onMount(() => {
    load();
    nowTick = setInterval(() => { now = new Date(); }, 60_000);
    return onWsEvent((ev) => {
      if (ev.type === 'task.changed') reload.trigger();
      else if (ev.type === 'note.changed' || ev.type === 'note.removed') reload.trigger();
      else if (ev.type === 'state.changed' && ev.path === '.granit/deadlines.json') reload.trigger();
    });
  });

  onDestroy(() => {
    reload.cancel();
    if (nowTick) clearInterval(nowTick);
  });

  // ICS events come down the wire in two shapes:
  //   - Zoned/UTC RFC3339:  "2026-05-12T11:00:00Z"
  //   - Floating wall-clock: "2026-05-12T13:00:00" (no Z, no offset)
  //
  // For zoned strings, a slice(11, 16) returns the UTC hour, which
  // for a UTC+2 user is 2h earlier than the wall-clock the calendar
  // grid actually displays (the grid uses new Date(...).getHours()
  // — local interpretation). The user reported an ICS event showing
  // 09-14 on the dashboard but 11-15 on the calendar; same root
  // cause as the prior calendar drift bug, different surface.
  //
  // Floating strings parse fine either way because the browser's
  // Date constructor treats no-offset ISO strings as local — we
  // also fall through to the cheap slice path for those so we
  // don't pay a Date() per row when the wire shape already matches
  // the wall clock.
  function isFloatingISO(s: string): boolean {
    // No trailing Z and no offset suffix (matches RFC3339 §5.6
    // shapes "+02:00", "-05:30"). Five-char suffix check covers
    // every offset form.
    if (s.endsWith('Z')) return false;
    const tail = s.slice(-6);
    if (/^[+-]\d{2}:\d{2}$/.test(tail)) return false;
    return true;
  }
  function eventDateKey(e: CalendarEvent): string {
    if (e.date) return e.date;
    if (!e.start) return '';
    // Zoned timestamps need to be reinterpreted in local TZ to pick
    // the right day — an event at 23:30 UTC is "tomorrow" for a
    // UTC+2 user but slice(0,10) would say "today".
    if (isFloatingISO(e.start)) return e.start.slice(0, 10);
    const d = new Date(e.start);
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  }

  function timeOf(e: CalendarEvent): string {
    if (!e.start) return ''; // all-day
    if (isFloatingISO(e.start)) return e.start.slice(11, 16);
    // Zoned → local wall-clock via Date.
    const d = new Date(e.start);
    return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
  }
  function endTimeOf(e: CalendarEvent): string {
    if (!e.end) return '';
    if (isFloatingISO(e.end)) return e.end.slice(11, 16);
    const d = new Date(e.end);
    return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
  }

  function nowHHMM(): string {
    const h = String(now.getHours()).padStart(2, '0');
    const m = String(now.getMinutes()).padStart(2, '0');
    return `${h}:${m}`;
  }

  // ----- Today's chronological stream -----
  let stream = $derived.by<StreamRow[]>(() => {
    if (!loaded) return [];
    const rows: StreamRow[] = [];
    const cutoff = nowHHMM();

    // Events for today (timed + all-day). The calendar feed ALSO
    // emits tasks (type: 'task_due' / 'task_scheduled') as event-
    // shaped rows for the calendar grid — we skip them here because
    // the tasks loop below adds them with the proper /tasks href, so
    // a click opens the task detail instead of /calendar.
    for (const e of events) {
      if (eventDateKey(e) !== today) continue;
      if (e.type !== 'event' && e.type !== 'ics_event') continue;
      const t = timeOf(e);
      rows.push({
        time: t,
        past: t !== '' && t < cutoff,
        kind: 'event',
        label: e.title,
        href: '/calendar',
        meta: e.location || (t === '' ? 'all-day' : undefined),
        id: `event-${e.title}-${t}-${e.start ?? e.date ?? ''}`,
        typeGlyph: glyphForKind(e.kind)
      });
    }

    // Tasks scheduled today (have an explicit start time)
    for (const t of tasks) {
      if (t.scheduledStart && t.scheduledStart.slice(0, 10) === today) {
        const time = t.scheduledStart.slice(11, 16);
        rows.push({
          time,
          past: time < cutoff,
          kind: 'task-scheduled',
          label: t.text,
          href: `/tasks?focus=${encodeURIComponent(t.id)}`,
          meta: t.estimatedMinutes ? `~${t.estimatedMinutes}m` : undefined,
          id: `task-sched-${t.id}`
        });
      } else if (t.dueDate === today) {
        // Due today but no scheduled time — sort to top with empty time
        rows.push({
          time: '',
          past: false,
          kind: 'task-due',
          label: t.text,
          href: `/tasks?focus=${encodeURIComponent(t.id)}`,
          meta: 'due today',
          id: `task-due-${t.id}`
        });
      }
    }

    // Deadlines due today (status !== 'met'/'missed' = still active)
    for (const d of deadlines) {
      if (d.date === today && d.status !== 'met' && d.status !== 'missed') {
        rows.push({
          time: '',
          past: false,
          kind: 'deadline',
          label: d.title,
          href: '/deadlines',
          meta: d.importance,
          id: `deadline-${d.id}`
        });
      }
    }

    // Sort: empty-time rows first (untimed obligations), then timed
    // ascending. Past items keep their position so the eye can scan
    // chronologically; dimming handles the "already done" cue.
    rows.sort((a, b) => {
      if (a.time === '' && b.time !== '') return -1;
      if (b.time === '' && a.time !== '') return 1;
      return a.time.localeCompare(b.time);
    });
    return rows;
  });

  // ----- Currently happening (the "now" headline) -----
  let nowEvent = $derived.by(() => {
    if (!loaded) return null;
    const cutoff = nowHHMM();
    for (const e of events) {
      if (eventDateKey(e) !== today) continue;
      if (!e.start || !e.end) continue;
      // timeOf / endTimeOf go through Date for zoned strings so
      // the comparison happens in the user's local wall clock,
      // matching `cutoff` (also local). Pre-fix this slice'd the
      // UTC digits and a UTC+2 user saw "happening now" land 2h
      // late (or early, depending on which event was current).
      const s = timeOf(e);
      const en = endTimeOf(e);
      if (s <= cutoff && cutoff < en) return e;
    }
    return null;
  });

  // ----- "Items left today" — count of upcoming non-past stream rows -----
  let leftToday = $derived(stream.filter((r) => !r.past).length);

  // ----- Tomorrow + day-after preview (compact) -----
  let upcoming = $derived.by(() => {
    if (!loaded) return [] as { date: Date; iso: string; eventCount: number; taskCount: number; firstEvent: string | null }[];
    return [1, 2].map((offset) => {
      const date = addDays(startOfDay(now), offset);
      const iso = fmtDateISO(date);
      const dayEvents = events.filter((e) => eventDateKey(e) === iso);
      const dayTasks = tasks.filter((t) => {
        if (t.scheduledStart && t.scheduledStart.slice(0, 10) === iso) return true;
        if (t.dueDate === iso) return true;
        return false;
      });
      // First timed event of the day, for the preview line
      const timed = dayEvents
        .filter((e) => e.start)
        .sort((a, b) => (a.start ?? '').localeCompare(b.start ?? ''));
      const first = timed[0];
      const firstEvent = first
        ? `${first.start!.slice(11, 16)} ${first.title}`
        : null;
      return {
        date,
        iso,
        eventCount: dayEvents.length,
        taskCount: dayTasks.length,
        firstEvent
      };
    });
  });

  function fmtDayLabel(d: Date): string {
    return d.toLocaleDateString(undefined, { weekday: 'long' });
  }

  function fmtTodayHeader(d: Date): string {
    return d.toLocaleDateString(undefined, { weekday: 'long', month: 'short', day: 'numeric' });
  }

  // Visual icon per row kind. Kept tiny — the meaning is in the
  // label, not the glyph.
  const KIND_ICON: Record<StreamRow['kind'], string> = {
    'event': '◷',
    'task-scheduled': '◉',
    'task-due': '◉',
    'deadline': '⚑',
    'now': '●'
  };

  // Tone per kind for the left rail accent.
  const KIND_TONE: Record<StreamRow['kind'], string> = {
    'event': 'text-primary',
    'task-scheduled': 'text-secondary',
    'task-due': 'text-warning',
    'deadline': 'text-error',
    'now': 'text-success'
  };
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-3 sm:p-4">
  <!-- Header. Day + date on the left, "N left" pill on the right.
       Single line, no decorative chrome. The "now" event lived
       here as a tiny inline pill before — too easy to miss. We
       moved it into a dedicated banner row below so what's
       happening RIGHT NOW reads as the loudest signal in the
       widget. -->
  <header class="flex items-baseline gap-3 mb-2">
    <h2 class="text-base font-semibold text-text">
      {fmtTodayHeader(now)}
    </h2>
    <span class="flex-1"></span>
    {#if loaded && stream.length > 0}
      <span class="text-[11px] text-dim font-mono tabular-nums">
        {leftToday}/{stream.length}
      </span>
    {/if}
  </header>

  <!-- Now-banner: the unified prominent "happening now" row. Both
       desktop and mobile see the same surface so the urgency reads
       identically across viewports. bg-success/10 tints the row
       without overwhelming the rest of the stream; the pulsing
       dot is the live indicator. Whole row clickable → /calendar
       so the user can jump to the event in context. Animation is
       pure CSS (no JS) and respects prefers-reduced-motion. -->
  {#if nowEvent}
    <a
      href="/calendar"
      title="Currently happening — open in calendar"
      class="block mb-2.5 px-3 py-2 rounded-md bg-success/10 border border-success/30 hover:bg-success/15 transition-colors"
    >
      <div class="flex items-center gap-2">
        <!-- Two-layer pulse: a solid dot + an expanding-fading ring
             behind it. Both use bg-success (Tailwind class) directly —
             earlier code used currentColor + --color-success-rgb CSS
             var, but that var doesn't exist in our token set so the
             fallback (light blue) shipped on every device. Now the
             ring renders the right green on every browser, no var
             lookup. -->
        <span class="relative flex-shrink-0 w-2 h-2" aria-hidden="true">
          <span class="now-pulse-ring absolute inset-0 rounded-full bg-success"></span>
          <span class="relative block w-2 h-2 rounded-full bg-success"></span>
        </span>
        <span class="text-[10px] uppercase tracking-wider font-semibold text-success flex-shrink-0">Now</span>
        <span class="text-sm font-medium text-text truncate flex-1 min-w-0">{nowEvent.title}</span>
        <span class="text-[11px] text-success/80 font-mono tabular-nums flex-shrink-0">
          {nowEvent.start!.slice(11, 16)}–{nowEvent.end!.slice(11, 16)}
        </span>
      </div>
    </a>
  {/if}

  {#if !loaded}
    <ul class="space-y-1">
      {#each [0, 1, 2] as i (i)}
        <li class="flex items-baseline gap-3 py-0.5">
          <span class="w-10 h-3 bg-surface1 rounded animate-pulse"></span>
          <span class="flex-1 h-3 bg-surface1 rounded animate-pulse"></span>
        </li>
      {/each}
    </ul>
  {:else if stream.length === 0}
    <p class="text-sm text-dim italic">
      Nothing scheduled today. Wide open.
    </p>
  {:else}
    <ul class="divide-y divide-surface1/40">
      {#each stream as r (r.id)}
        <li>
          <a
            href={r.href}
            class="flex items-baseline gap-2.5 py-1 px-1.5 -mx-1.5 rounded hover:bg-surface1 transition-colors {r.past ? 'opacity-40' : ''}"
          >
            <span class="w-10 flex-shrink-0 text-[11px] font-mono tabular-nums text-dim">
              {r.time || '—'}
            </span>
            <span class="text-xs flex-shrink-0 {KIND_TONE[r.kind]}" aria-hidden="true">
              {KIND_ICON[r.kind]}
            </span>
            <span class="flex-1 text-sm text-text truncate">
              {#if r.typeGlyph}<span class="font-mono text-[10px] text-dim mr-1">{r.typeGlyph}</span>{/if}{r.label}
            </span>
            {#if r.meta}
              <span class="text-[11px] text-dim flex-shrink-0 hidden sm:inline">{r.meta}</span>
            {/if}
          </a>
        </li>
      {/each}
    </ul>
  {/if}

  <!-- Tomorrow + day-after preview — single-line per day so the
       footer reads "thu: 09:00 dentist · +2 events" rather than
       eating a paragraph. Hidden when both days are empty. -->
  {#if loaded && upcoming.some((u) => u.eventCount > 0 || u.taskCount > 0)}
    <div class="mt-3 pt-2 border-t border-surface1 space-y-0.5">
      {#each upcoming as u (u.iso)}
        {@const empty = u.eventCount === 0 && u.taskCount === 0}
        <a
          href="/calendar"
          class="flex items-baseline gap-2 px-1.5 py-1 -mx-1.5 rounded hover:bg-surface1 transition-colors {empty ? 'opacity-50' : ''}"
        >
          <span class="w-16 flex-shrink-0 text-[10px] uppercase tracking-wider text-dim">
            {fmtDayLabel(u.date)}
          </span>
          <span class="flex-1 text-xs text-subtext truncate">
            {#if u.firstEvent}
              <span class="text-text">{u.firstEvent}</span>
              {#if u.eventCount > 1 || u.taskCount > 0}
                <span class="text-dim"> · +{u.eventCount - 1} ev{#if u.taskCount > 0}, {u.taskCount} task{u.taskCount === 1 ? '' : 's'}{/if}</span>
              {/if}
            {:else if !empty}
              {u.eventCount} event{u.eventCount === 1 ? '' : 's'} · {u.taskCount} task{u.taskCount === 1 ? '' : 's'}
            {:else}
              — open —
            {/if}
          </span>
        </a>
      {/each}
    </div>
  {/if}
</section>

<style>
  /* Now-banner pulsing ring. Grows from the solid dot's size to
     1.9× and fades to transparent, then resets. The ring is its
     own DOM element with bg-success applied via Tailwind, so the
     animation has zero colour ambiguity. Pure CSS — no JS per
     frame. Respects prefers-reduced-motion (sits at opacity 0.4
     scale 1 instead of breathing). */
  .now-pulse-ring {
    animation: now-pulse 1.6s cubic-bezier(0.4, 0, 0.6, 1) infinite;
    transform-origin: center;
  }
  @keyframes now-pulse {
    0%, 100% { transform: scale(1); opacity: 0.55; }
    50% { transform: scale(1.9); opacity: 0; }
  }
  @media (prefers-reduced-motion: reduce) {
    .now-pulse-ring { animation: none; opacity: 0.4; }
  }
</style>
