<script lang="ts">
  // CalendarDashboardPanel — full-screen overlay surfacing the
  // calendar's full operating picture in a card grid. Visual
  // companion to the chat-mode "Calendar Manager" prelude; the
  // data shape is the CalendarContextBundle that powers that
  // prelude (web/src/lib/ai/calendarManagerContext.ts), so the
  // dashboard's "12 events in the window" matches the count the
  // chat quotes.
  //
  // Calendar dashboard is window-level (no specific entity) — its
  // scope is "today + 14 days" by default, which matches the
  // CAL window the AI prelude uses. One source of truth, two
  // surfaces.
  //
  // Layout: 1 col on mobile, 2 cols at md, 3 cols at xl.
  // Tap targets ≥40px on mobile.
  import { onMount } from 'svelte';
  import { api, todayISO, type CalendarEventEntry, type Task } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import {
    loadCalendarContext,
    type CalendarContextBundle
  } from '$lib/ai/calendarManagerContext';
  import {
    computeFreeSlots,
    countDeepMorningBlocks,
    type FreeSlotsDay
  } from './dashboardFreeSlots';

  let { onClose }: { onClose: () => void } = $props();

  let bundle = $state<CalendarContextBundle | null>(null);
  let loading = $state(true);
  let loadError = $state('');

  async function load() {
    loading = true;
    loadError = '';
    try {
      // Same shim shape as AIOverlay.svelte's calendar prelude.
      // listEvents + listTasks are the only fetches the bundle
      // needs; the loader does the filtering + sorting in-process.
      const b = await loadCalendarContext(
        {
          listEvents: async () => {
            const r = await api.listEvents();
            return r.events;
          },
          listTasks: async () => {
            const r = await api.listTasks({});
            return r.tasks;
          }
        },
        { todayISO: todayISO() }
      );
      bundle = b;
    } catch (e) {
      loadError = e instanceof Error ? e.message : String(e);
      toast.error('dashboard failed to load: ' + loadError);
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    void load();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        onClose();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  // ── Derived: event groups + week heat + free slots ─────────────
  // Group events by date for the "Upcoming events" card so the
  // user reads "Mon 12 May → 2 events" rather than a flat soup.
  // Order preserved from the bundle (already chronological).
  const eventsByDate = $derived.by(() => {
    const out = new Map<string, CalendarEventEntry[]>();
    if (!bundle) return out;
    for (const e of bundle.upcomingEvents) {
      const list = out.get(e.date) ?? [];
      list.push(e);
      out.set(e.date, list);
    }
    return out;
  });

  // Cap upcoming-events display at 10 entries. The bundle already
  // caps at 30; we additionally cap the rendered slice so a busy
  // calendar doesn't dominate the dashboard.
  const UPCOMING_VISIBLE = 10;
  const upcomingVisible = $derived(bundle ? bundle.upcomingEvents.slice(0, UPCOMING_VISIBLE) : []);
  const upcomingMore = $derived(
    bundle
      ? Math.max(0, (bundle.totals?.upcomingEvents ?? bundle.upcomingEvents.length) - upcomingVisible.length)
      : 0
  );

  // Week heat — events per day across the next 7 days starting
  // from today. Heavy days (>=3 events) tint warm; lighter days
  // stay neutral. Same WEEKDAY_LABELS Sun→Sat ordering as the
  // free-slot helper.
  const WEEKDAY_LABELS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
  function isoFromDate(d: Date): string {
    const y = d.getFullYear();
    const m = String(d.getMonth() + 1).padStart(2, '0');
    const day = String(d.getDate()).padStart(2, '0');
    return `${y}-${m}-${day}`;
  }
  const weekHeat = $derived.by(() => {
    if (!bundle) return [] as Array<{ date: string; weekday: string; count: number; isToday: boolean }>;
    const out: Array<{ date: string; weekday: string; count: number; isToday: boolean }> = [];
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    const todayKey = bundle.todayISO;
    for (let i = 0; i < 7; i++) {
      const d = new Date(today);
      d.setDate(d.getDate() + i);
      const iso = isoFromDate(d);
      const count = (eventsByDate.get(iso) ?? []).length;
      out.push({
        date: iso,
        weekday: WEEKDAY_LABELS[d.getDay()],
        count,
        isToday: iso === todayKey
      });
    }
    return out;
  });
  const weekHeatMax = $derived(weekHeat.reduce((m, b) => Math.max(m, b.count), 0));

  // Free slots — 5 weekday horizon. The helper handles weekend
  // skipping and the deep-work morning detection.
  const freeDays = $derived<FreeSlotsDay[]>(
    bundle ? computeFreeSlots(bundle.upcomingEvents, new Date(), 5) : []
  );
  const deepBlocks = $derived(countDeepMorningBlocks(freeDays));

  // Hero card numbers — sourced from the bundle's `totals` so the
  // number on the card matches what the AI sees verbatim.
  const eventCount = $derived(bundle ? (bundle.totals?.upcomingEvents ?? bundle.upcomingEvents.length) : 0);
  const overdueCount = $derived(bundle ? (bundle.totals?.overdue ?? bundle.overdue.length) : 0);
  const dueTodayCount = $derived(bundle ? (bundle.totals?.dueToday ?? bundle.dueToday.length) : 0);

  // Pretty date for the upcoming-events grouped list.
  // Returns a short two-line "Mon 12 May" / "today" treatment.
  function fmtDayHeader(iso: string, todayIso: string): string {
    if (iso === todayIso) return 'Today';
    const [y, m, d] = iso.split('-').map(Number);
    const dt = new Date(y, (m ?? 1) - 1, d);
    const wd = WEEKDAY_LABELS[dt.getDay()];
    const dd = String(d).padStart(2, '0');
    const month = dt.toLocaleString(undefined, { month: 'short' });
    return `${wd} ${dd} ${month}`;
  }

  // Days-overdue chip for the overdue card. Negative values
  // shouldn't happen here (bundle pre-filters) but we defend
  // against weirdly-formatted dueDate strings by returning
  // null and letting the chip fall back to "overdue".
  function daysOverdue(dueDate: string | undefined, todayIso: string): number | null {
    if (!dueDate) return null;
    const due = new Date(dueDate);
    const today = new Date(todayIso);
    if (Number.isNaN(due.getTime()) || Number.isNaN(today.getTime())) return null;
    const diff = Math.round((today.getTime() - due.getTime()) / (1000 * 60 * 60 * 24));
    return diff > 0 ? diff : null;
  }

  // ── Sort due-today by scheduled time so morning slots read
  // before afternoon. Tasks without a scheduledStart sink to the
  // bottom so the user sees the time-anchored stuff first.
  const dueTodaySorted = $derived.by(() => {
    if (!bundle) return [] as Task[];
    return [...bundle.dueToday].sort((a, b) => {
      const ka = a.scheduledStart ?? 'z';
      const kb = b.scheduledStart ?? 'z';
      return ka.localeCompare(kb);
    });
  });
</script>

<!-- Full-page overlay above the /calendar layout. Same modal
     contract as the project + goal dashboards: Esc + the X button
     are the only ways out. Hides body scroll while open. -->
<div class="fixed inset-0 z-50 bg-mantle/95 backdrop-blur-sm flex flex-col" role="dialog" aria-modal="true" aria-label="Calendar dashboard">
  <header class="flex-shrink-0 border-b border-surface1 bg-base/80 px-3 sm:px-6 py-3 flex items-center gap-3">
    <span class="w-3 h-3 rounded-full flex-shrink-0 bg-primary"></span>
    <div class="flex-1 min-w-0">
      <div class="flex items-baseline gap-2 flex-wrap">
        <h1 class="text-base sm:text-lg font-semibold text-text truncate">Calendar</h1>
        <span class="text-[10px] uppercase tracking-wider text-dim flex-shrink-0">Dashboard</span>
        {#if bundle}
          <span class="text-[11px] text-dim font-mono">{bundle.windowStart} → {bundle.windowEnd}</span>
        {/if}
      </div>
    </div>
    <button
      onclick={onClose}
      aria-label="close dashboard"
      class="min-w-[40px] min-h-[40px] flex items-center justify-center rounded text-subtext hover:text-error hover:bg-surface0"
      title="close (Esc)"
    >
      <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
        <path d="M6 6l12 12M6 18L18 6" />
      </svg>
    </button>
  </header>

  <div class="flex-1 overflow-y-auto">
    <div class="max-w-7xl mx-auto p-3 sm:p-6">
      {#if loading && !bundle}
        <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3 sm:gap-4">
          <div class="md:col-span-2 xl:col-span-3 bg-surface0 border border-surface1 rounded-lg p-4 animate-pulse h-40"></div>
          {#each [0, 1, 2, 3, 4] as i (i)}
            <div class="bg-surface0 border border-surface1 rounded-lg p-4 animate-pulse h-48"></div>
          {/each}
        </div>
      {:else if loadError && !bundle}
        <div class="text-sm text-error border border-error/30 bg-error/5 rounded px-4 py-3">
          Could not load dashboard: {loadError}
          <button onclick={() => void load()} class="ml-3 underline text-secondary">retry</button>
        </div>
      {:else if bundle}
        <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3 sm:gap-4">
          <!-- ── 1. Hero / scope ────────────────────────────────────
               The shape of the next two weeks at a glance: event
               count + overdue count + deep-work blocks. Numbers
               sourced from the bundle's totals so they match the
               AI prelude verbatim. -->
          <section class="md:col-span-2 xl:col-span-3 bg-surface0 border border-surface1 rounded-lg p-4 sm:p-5">
            <div class="flex items-baseline gap-2 flex-wrap mb-3">
              <h2 class="text-lg sm:text-xl font-semibold text-text">Next 14 days</h2>
              <span class="text-[11px] text-dim font-mono">today is {bundle.todayISO}</span>
            </div>
            <div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
              <div>
                <div class="text-2xl font-semibold text-text font-mono">{eventCount}</div>
                <div class="text-[10px] uppercase tracking-wider text-dim">events</div>
              </div>
              <div>
                <div class="text-2xl font-semibold {overdueCount > 0 ? 'text-error' : 'text-text'} font-mono">{overdueCount}</div>
                <div class="text-[10px] uppercase tracking-wider text-dim">overdue</div>
              </div>
              <div>
                <div class="text-2xl font-semibold {dueTodayCount > 0 ? 'text-warning' : 'text-text'} font-mono">{dueTodayCount}</div>
                <div class="text-[10px] uppercase tracking-wider text-dim">due today</div>
              </div>
              <div>
                <div class="text-2xl font-semibold text-text font-mono">{deepBlocks}</div>
                <div class="text-[10px] uppercase tracking-wider text-dim">deep-work mornings · next 5d</div>
              </div>
            </div>
          </section>

          <!-- ── 2. This week's heat ────────────────────────────────
               Tiny bar chart, plain divs. Heavy days (≥3 events)
               warm; lighter days neutral. Today's column primary-
               coloured so the eye locks onto "where the cursor is".
               aria-label communicates the bar's data to AT users. -->
          <section class="bg-surface0 border border-surface1 rounded-lg p-4">
            <div class="flex items-baseline gap-2 mb-3">
              <h3 class="text-xs uppercase tracking-wider text-dim font-medium flex-1">This week</h3>
              <span class="text-[11px] text-dim font-mono">{weekHeat.reduce((s, b) => s + b.count, 0)} events</span>
            </div>
            <div class="flex items-end gap-1.5 h-24" aria-label="events per day · next 7 days">
              {#each weekHeat as b (b.date)}
                {@const pct = weekHeatMax === 0 ? 0 : Math.max(4, Math.round((b.count / weekHeatMax) * 100))}
                {@const tone = b.isToday ? 'primary' : b.count >= 3 ? 'warning' : 'secondary'}
                <div
                  class="flex-1 flex flex-col items-center justify-end gap-1"
                  title="{b.weekday} {b.date} · {b.count} event{b.count === 1 ? '' : 's'}"
                >
                  <div class="text-[10px] text-subtext font-mono leading-none">{b.count}</div>
                  <div
                    class="w-full rounded-t transition-all"
                    style="height: {pct}%; background: color-mix(in srgb, var(--color-{tone}) {b.count === 0 ? 25 : 70}%, transparent);"
                  ></div>
                  <div class="text-[10px] {b.isToday ? 'text-primary' : 'text-dim'} font-mono leading-none">{b.weekday}</div>
                </div>
              {/each}
            </div>
          </section>

          <!-- ── 3. Upcoming events ─────────────────────────────────
               Grouped by date for scanability. The /calendar page
               accepts no first-class focus param today, so the
               clickable rows fall back to /calendar; the URL
               carries ?focus=<id> for forward compatibility (a
               future EventDetail deep-link will pick it up). -->
          <section class="bg-surface0 border border-surface1 rounded-lg p-4 md:col-span-2">
            <div class="flex items-baseline gap-2 mb-3">
              <h3 class="text-xs uppercase tracking-wider text-dim font-medium flex-1">Upcoming events</h3>
              <span class="text-[11px] text-dim font-mono">{eventCount}</span>
            </div>
            {#if upcomingVisible.length === 0}
              <p class="text-xs text-dim italic">No events in the next 14 days.</p>
            {:else}
              {@const groups = (() => {
                const seen = new Set<string>();
                return upcomingVisible.filter((e) => {
                  if (seen.has(e.date)) return false;
                  seen.add(e.date);
                  return true;
                }).map((e) => e.date);
              })()}
              <div class="space-y-3">
                {#each groups as date (date)}
                  <div>
                    <div class="text-[10px] uppercase tracking-wider text-dim mb-1 font-mono">{fmtDayHeader(date, bundle.todayISO)}</div>
                    <ul class="space-y-1">
                      {#each upcomingVisible.filter((e) => e.date === date) as e (e.id)}
                        <li class="px-2 py-1.5 min-h-[40px] rounded hover:bg-mantle">
                          <div class="flex items-baseline gap-2">
                            <a
                              href={`/calendar?focus=${encodeURIComponent(e.id)}`}
                              class="text-sm text-text flex-1 truncate hover:text-primary"
                            >{e.title}</a>
                            <span class="text-[10px] text-dim font-mono flex-shrink-0">
                              {#if e.start_time}{e.start_time}{#if e.end_time}–{e.end_time}{/if}{:else}all-day{/if}
                            </span>
                          </div>
                          {#if e.location || e.project_id || e.rrule}
                            <div class="text-[10px] text-dim mt-0.5 flex items-center gap-1.5 flex-wrap">
                              {#if e.location}<span>@ {e.location}</span>{/if}
                              {#if e.project_id}<a href={`/projects?p=${encodeURIComponent(e.project_id)}`} class="text-primary hover:underline">📁 {e.project_id}</a>{/if}
                              {#if e.rrule}<span>↻</span>{/if}
                            </div>
                          {/if}
                        </li>
                      {/each}
                    </ul>
                  </div>
                {/each}
              </div>
              {#if upcomingMore > 0}
                <a
                  href="/calendar"
                  class="block mt-3 text-xs text-secondary hover:underline"
                >+ {upcomingMore} more in /calendar →</a>
              {/if}
            {/if}
          </section>

          <!-- ── 4. Overdue tasks ───────────────────────────────────
               Bundle already sorts oldest dueDate first so the
               most-overdue rises to the top. /tasks doesn't
               currently surface ?id=<id> deep-link; the link falls
               back to /tasks?status=overdue which the tasks page
               does honour. -->
          <section class="bg-surface0 border border-surface1 rounded-lg p-4">
            <div class="flex items-baseline gap-2 mb-3">
              <h3 class="text-xs uppercase tracking-wider text-dim font-medium flex-1">Overdue</h3>
              <span class="text-[11px] {overdueCount > 0 ? 'text-error' : 'text-dim'} font-mono">{overdueCount}</span>
            </div>
            {#if bundle.overdue.length === 0}
              <p class="text-xs text-dim italic">Nothing overdue. 🎯</p>
            {:else}
              <ul class="space-y-1">
                {#each bundle.overdue as t (t.id)}
                  {@const d = daysOverdue(t.dueDate, bundle.todayISO)}
                  <li class="px-2 py-1.5 min-h-[40px] rounded hover:bg-mantle">
                    <a href={`/tasks?id=${encodeURIComponent(t.id)}`} class="block">
                      <div class="flex items-baseline gap-2">
                        <span class="w-1.5 h-1.5 rounded-full bg-error flex-shrink-0 mt-1.5"></span>
                        <span class="text-sm text-text flex-1 leading-snug">{t.text}</span>
                        <span class="flex items-baseline gap-1 flex-shrink-0">
                          {#if t.priority && t.priority > 0}
                            <span class="text-[10px] px-1 py-0.5 rounded bg-warning/15 text-warning font-mono">P{t.priority}</span>
                          {/if}
                          {#if d != null}
                            <span class="text-[10px] px-1 py-0.5 rounded bg-error/15 text-error font-mono">{d}d</span>
                          {/if}
                        </span>
                      </div>
                      {#if t.dueDate}
                        <div class="text-[10px] text-dim font-mono mt-0.5 ml-3.5">was due {t.dueDate}</div>
                      {/if}
                    </a>
                  </li>
                {/each}
              </ul>
            {/if}
          </section>

          <!-- ── 5. Due today ───────────────────────────────────────
               Sorted by scheduledStart so the timeline reads
               morning-first. Unscheduled tasks sink to the bottom
               so the user sees what's anchored vs. floating. -->
          <section class="bg-surface0 border border-surface1 rounded-lg p-4">
            <div class="flex items-baseline gap-2 mb-3">
              <h3 class="text-xs uppercase tracking-wider text-dim font-medium flex-1">Due today</h3>
              <span class="text-[11px] {dueTodayCount > 0 ? 'text-warning' : 'text-dim'} font-mono">{dueTodayCount}</span>
            </div>
            {#if dueTodaySorted.length === 0}
              <p class="text-xs text-dim italic">Nothing due today.</p>
            {:else}
              <ul class="space-y-1">
                {#each dueTodaySorted as t (t.id)}
                  <li class="px-2 py-1.5 min-h-[40px] rounded hover:bg-mantle">
                    <a href={`/tasks?id=${encodeURIComponent(t.id)}`} class="block">
                      <div class="flex items-baseline gap-2">
                        <span class="w-1.5 h-1.5 rounded-full bg-warning flex-shrink-0 mt-1.5"></span>
                        <span class="text-sm text-text flex-1 leading-snug">{t.text}</span>
                        <span class="flex items-baseline gap-1 flex-shrink-0">
                          {#if t.priority && t.priority > 0}
                            <span class="text-[10px] px-1 py-0.5 rounded bg-warning/15 text-warning font-mono">P{t.priority}</span>
                          {/if}
                          {#if t.scheduledStart}
                            <span class="text-[10px] px-1 py-0.5 rounded bg-secondary/15 text-secondary font-mono">{t.scheduledStart.slice(11, 16)}</span>
                          {/if}
                        </span>
                      </div>
                    </a>
                  </li>
                {/each}
              </ul>
            {/if}
          </section>

          <!-- ── 6. Free slot map ───────────────────────────────────
               Five weekday columns × free-slot rows. Plain divs,
               no chart library. Width of each slot is proportional
               to its duration so the eye can tell a 2-hour gap
               from a 1-hour one. Weekend rows are excluded by the
               helper (computeFreeSlots only emits weekdays). -->
          <section class="bg-surface0 border border-surface1 rounded-lg p-4 xl:col-span-3 md:col-span-2">
            <div class="flex items-baseline gap-2 mb-3">
              <h3 class="text-xs uppercase tracking-wider text-dim font-medium flex-1">Free slot map · next 5 weekdays</h3>
              <span class="text-[11px] text-dim font-mono">≥ 60 min · 09:00–18:00</span>
            </div>
            {#if freeDays.length === 0}
              <p class="text-xs text-dim italic">Free-slot map needs at least one weekday in the window.</p>
            {:else}
              <div class="grid grid-cols-1 sm:grid-cols-5 gap-2">
                {#each freeDays as day (day.date)}
                  <div class="flex flex-col gap-1">
                    <div class="flex items-baseline gap-1.5 text-[11px]">
                      <span class="text-subtext font-medium">{day.weekday}</span>
                      <span class="text-dim font-mono">{day.date.slice(5)}</span>
                      {#if day.hasDeepMorning}
                        <span class="ml-auto text-[9px] uppercase tracking-wider px-1 py-0.5 rounded bg-success/15 text-success" title="no events 09:00-12:00">deep</span>
                      {/if}
                    </div>
                    <div class="space-y-0.5">
                      {#if day.slots.length === 0}
                        <div class="px-2 py-1.5 rounded bg-mantle text-[10px] text-dim font-mono text-center italic">no free ≥60m</div>
                      {:else}
                        {#each day.slots as slot, idx (idx)}
                          {@const widthPct = Math.min(100, Math.round((slot.durationMinutes / ((18 - 9) * 60)) * 100))}
                          <div
                            class="px-2 py-1.5 rounded bg-secondary/10 border border-secondary/30 text-[11px] font-mono"
                            style="width: {Math.max(40, widthPct)}%"
                            title="{slot.durationMinutes} min free"
                          >
                            <span class="text-secondary">{slot.startLabel}–{slot.endLabel}</span>
                            <span class="text-dim ml-1">· {Math.round(slot.durationMinutes / 60 * 10) / 10}h</span>
                          </div>
                        {/each}
                      {/if}
                    </div>
                  </div>
                {/each}
              </div>
            {/if}
          </section>
        </div>
      {/if}
    </div>
  </div>
</div>
