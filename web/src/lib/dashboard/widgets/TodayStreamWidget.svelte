<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { api, fmtDateISO, todayISO, type CalendarEvent, type Task, type Deadline } from '$lib/api';
  import { onWsEvent } from '$lib/ws';

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

  onMount(() => {
    load();
    nowTick = setInterval(() => { now = new Date(); }, 60_000);
    return onWsEvent((ev) => {
      if (ev.type === 'task.changed') load();
      if (ev.type === 'note.changed' || ev.type === 'note.removed') load();
      if (ev.type === 'state.changed' && ev.path === '.granit/deadlines.json') load();
    });
  });

  onDestroy(() => {
    if (nowTick) clearInterval(nowTick);
  });

  function eventDateKey(e: CalendarEvent): string {
    return e.date ?? (e.start ? e.start.slice(0, 10) : '');
  }

  function timeOf(e: CalendarEvent): string {
    if (!e.start) return ''; // all-day
    return e.start.slice(11, 16); // "HH:MM"
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
        id: `event-${e.title}-${t}-${e.start ?? e.date ?? ''}`
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
      const s = e.start.slice(11, 16);
      const en = e.end.slice(11, 16);
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

<section class="bg-surface0 border border-surface1 rounded-lg p-4 sm:p-5">
  <!-- Header. Day + date on the left, "N left" pill on the right.
       Reads like a sentence; no decorative chrome. -->
  <header class="flex items-baseline gap-3 mb-3">
    <h2 class="text-base sm:text-lg font-semibold text-text">
      {fmtTodayHeader(now)}
    </h2>
    <span class="flex-1"></span>
    {#if loaded && stream.length > 0}
      <span class="text-xs text-dim">
        {leftToday} left
      </span>
    {/if}
  </header>

  <!-- Now line: current event if any, otherwise current time. -->
  {#if nowEvent}
    <div class="flex items-baseline gap-2 mb-3 px-3 py-2 rounded bg-surface0 border border-success">
      <span class="text-success text-sm">●</span>
      <span class="text-xs uppercase tracking-wider text-success font-semibold">Now</span>
      <a href="/calendar" class="text-sm text-text font-medium truncate flex-1 hover:underline">
        {nowEvent.title}
      </a>
      <span class="text-[11px] text-dim font-mono tabular-nums">
        {nowEvent.start!.slice(11, 16)}–{nowEvent.end!.slice(11, 16)}
      </span>
    </div>
  {/if}

  {#if !loaded}
    <ul class="space-y-1.5">
      {#each [0, 1, 2] as i (i)}
        <li class="flex items-baseline gap-3 py-1">
          <span class="w-12 h-3 bg-surface1 rounded animate-pulse"></span>
          <span class="flex-1 h-3 bg-surface1 rounded animate-pulse"></span>
        </li>
      {/each}
    </ul>
  {:else if stream.length === 0}
    <p class="text-sm text-dim italic">
      Nothing scheduled today. Wide open.
    </p>
  {:else}
    <ul class="space-y-1">
      {#each stream as r (r.id)}
        <li>
          <a
            href={r.href}
            class="flex items-baseline gap-3 py-1 px-2 -mx-2 rounded hover:bg-surface1 transition-colors {r.past ? 'opacity-45' : ''}"
          >
            <span class="w-12 flex-shrink-0 text-xs font-mono tabular-nums text-dim">
              {r.time || '—'}
            </span>
            <span class="text-xs flex-shrink-0 {KIND_TONE[r.kind]}" aria-hidden="true">
              {KIND_ICON[r.kind]}
            </span>
            <span class="flex-1 text-sm text-text truncate {r.past ? 'line-through decoration-dim/40' : ''}">
              {r.label}
            </span>
            {#if r.meta}
              <span class="text-[11px] text-dim flex-shrink-0 hidden sm:inline">{r.meta}</span>
            {/if}
          </a>
        </li>
      {/each}
    </ul>
  {/if}

  <!-- Tomorrow + day-after compact preview. Single-line summary
       per day with count + first event. Hidden when both days
       are empty so the widget stays tight. -->
  {#if loaded && upcoming.some((u) => u.eventCount > 0 || u.taskCount > 0)}
    <div class="mt-4 pt-3 border-t border-surface1 grid grid-cols-1 sm:grid-cols-2 gap-2">
      {#each upcoming as u (u.iso)}
        <a
          href="/calendar"
          class="flex flex-col gap-0.5 px-3 py-2 rounded bg-surface1 hover:bg-surface1 transition-colors {u.eventCount === 0 && u.taskCount === 0 ? 'opacity-60' : ''}"
        >
          <span class="text-[10px] uppercase tracking-wider text-dim">
            {fmtDayLabel(u.date)}
          </span>
          <span class="text-sm text-subtext">
            {#if u.firstEvent}
              <span class="text-text font-medium truncate">{u.firstEvent}</span>
            {:else if u.eventCount > 0 || u.taskCount > 0}
              {u.eventCount} event{u.eventCount === 1 ? '' : 's'} · {u.taskCount} task{u.taskCount === 1 ? '' : 's'}
            {:else}
              — open day —
            {/if}
          </span>
          {#if u.firstEvent && (u.eventCount > 1 || u.taskCount > 0)}
            <span class="text-[11px] text-dim">
              + {u.eventCount - 1} event{u.eventCount - 1 === 1 ? '' : 's'} · {u.taskCount} task{u.taskCount === 1 ? '' : 's'}
            </span>
          {/if}
        </a>
      {/each}
    </div>
  {/if}
</section>
