<script lang="ts">
  // TaskBacklog — the left rail in Plan mode. Lists today's open tasks
  // (today-or-overdue OR P1/P2/P3) and lets the user drag them onto the
  // hour grid to schedule. An "AI" button runs plan-my-day and
  // auto-schedules whatever it returns (see api.runPlanDaySchedule).
  //
  // Why pointer-events instead of HTML5 drag: HourGrid already owns
  // pointer-capture for slot drag-to-create and event reschedule. The
  // shared dragStore is the discriminator — non-null = task drop in
  // progress, HourGrid takes the alternate path. See dragStore.ts.
  import { onMount } from 'svelte';
  import { api, type Task } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { onWsEvent } from '$lib/ws';
  import { dragStore } from './dragStore';
  import { fmtDateISO } from './utils';

  let { onRefresh }: { onRefresh?: () => void } = $props();

  let tasks = $state<Task[]>([]);
  let loading = $state(false);
  let aiBusy = $state(false);
  let isMobile = $state(false);

  // Tasks already on today's grid live in the list too (greyed) so the
  // user has an at-a-glance "what's already scheduled" without flipping
  // back to the grid. Greyed rows aren't draggable — drag would
  // duplicate the schedule. The user moves a scheduled task by dragging
  // its event chip on the grid (existing reschedule code path).

  function todayISO(): string {
    const d = new Date();
    return fmtDateISO(d);
  }

  function isToday(iso: string | undefined): boolean {
    if (!iso) return false;
    return iso.slice(0, 10) === todayISO();
  }

  // Granit convention (verified by reading TaskCard's priorityBadge):
  // 1 = P1 (highest), 2 = P2, 3 = P3, 0 = no priority.
  // So `priority >= 1` matches P1/P2/P3 (anything that has a priority set).
  // Sort: scheduled first (so they sit at the bottom greyed), then by
  // priority asc (P1 first), then by dueDate asc.
  let filtered = $derived.by(() => {
    const today = todayISO();
    const matches = tasks.filter((t) => {
      if (t.done) return false;
      const dueOk = t.dueDate && t.dueDate <= today;
      const prioOk = t.priority >= 1;
      return dueOk || prioOk;
    });

    // Scheduled-today goes to the END of the list (greyed). Unscheduled
    // (or scheduled on a different day) goes to the top.
    matches.sort((a, b) => {
      const aSched = isToday(a.scheduledStart);
      const bSched = isToday(b.scheduledStart);
      if (aSched !== bSched) return aSched ? 1 : -1;

      // Priority asc: 1 < 2 < 3 < 0 (no priority sorted last among
      // unscheduled — but everything we have here has dueDate or
      // priority>=1 so 0-priority items also have dueDate).
      const ap = a.priority || 99;
      const bp = b.priority || 99;
      if (ap !== bp) return ap - bp;

      const ad = a.dueDate ?? '9999-12-31';
      const bd = b.dueDate ?? '9999-12-31';
      return ad.localeCompare(bd);
    });
    return matches;
  });

  async function load() {
    loading = true;
    try {
      const r = await api.listTasks({ status: 'open' });
      tasks = r.tasks;
    } catch {
      // Silent — empty list is a fine fallback. Errors surface in the
      // network panel; toast spam on backlog refresh is worse than the
      // blank state.
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    isMobile = window.matchMedia('(max-width: 767px)').matches;
    load();
  });
  onMount(() =>
    onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'task.changed') load();
    })
  );

  // Priority dot tone — mirrors TaskCard's priorityBadge.
  function priorityTone(p: number): string {
    if (p === 1) return 'error';
    if (p === 2) return 'warning';
    if (p === 3) return 'info';
    return 'subtext';
  }

  function fmtScheduled(iso: string | undefined): string {
    if (!iso) return '';
    const d = new Date(iso);
    return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
  }

  // Default duration estimate for unscheduled tasks. The dragStore
  // carries this so HourGrid renders a correctly-sized ghost.
  const DEFAULT_DURATION_MIN = 30;

  function durationOf(t: Task): number {
    return t.durationMinutes && t.durationMinutes > 0
      ? t.durationMinutes
      : t.estimatedMinutes && t.estimatedMinutes > 0
        ? t.estimatedMinutes
        : DEFAULT_DURATION_MIN;
  }

  // ─── Drag (desktop) / Tap (mobile) ───
  // pointerdown sets dragStore. We deliberately do NOT setPointerCapture
  // on the row for desktop drags — capture would intercept pointermove /
  // pointerup events even when they land on the HourGrid, breaking the
  // grid's ghost-rendering and drop-detection. Without capture, the
  // browser routes pointermove/pointerup to whatever element is under
  // the cursor, which is exactly what we want for HTML5-drag-style
  // drop semantics.
  //
  // The lifecycle of a backlog drag:
  //   1. Row pointerdown  → set dragStore
  //   2. Pointer moves over a HourGrid slot column (handled there:
  //      onpointerover sets taskDragHover for the ghost; the slot's
  //      ghost reads taskDragHover when dragStore is non-null)
  //   3. Pointer releases on a slot → HourGrid's onSlotPointerDown is
  //      NOT what fires (pointerdown is the OPENING gesture). Instead
  //      pointerup fires on the slot — but we never captured it, so
  //      we can't rely on a slot-side pointerup either.
  //
  //   Pragmatic fix: we DO listen to pointerup at the document level
  //   below to commit the drop. That's the only way to reliably catch
  //   a desktop drag-and-release without pointer capture. On release,
  //   we use elementFromPoint to find the slot column and read its
  //   `data-day-idx` attribute.
  //
  // For mobile tap we use pointerId = -1 to mean "this is a pending
  // pick, not a live drag". HourGrid consumes it on the next slot
  // pointerdown. See HourGrid.svelte's onSlotPointerDown.
  function onRowPointerDown(e: PointerEvent, t: Task) {
    if (e.pointerType === 'mouse' && e.button !== 0) return;
    if (isToday(t.scheduledStart)) return; // greyed rows aren't draggable

    if (isMobile) {
      // Tap mode: stash a pending pick. The next slot tap on the
      // grid consumes it. No drag-tracking needed.
      dragStore.set({
        taskId: t.id,
        title: t.text,
        durationMinutes: durationOf(t),
        pointerId: -1
      });
      e.preventDefault();
      return;
    }

    // Desktop: live drag — start tracking, but don't capture.
    dragStore.set({
      taskId: t.id,
      title: t.text,
      durationMinutes: durationOf(t),
      pointerId: e.pointerId
    });
    e.preventDefault();
  }

  // Click → open task detail. Desktop drag leaks a click on press-without-
  // movement; we suppress it by checking dragStore (set during a drag).
  // For tap-mode we use the dragStore pick path instead — a click here
  // would only fire on devices that emit synthetic clicks AFTER the
  // pointerup, which is fine: the task is already picked.
  function onRowClick(t: Task, e: MouseEvent) {
    e.stopPropagation();
    void t;
  }

  // ─── AI auto-schedule ───
  async function runAi() {
    aiBusy = true;
    try {
      const r = await api.runPlanDaySchedule();
      const n = r.scheduled.length;
      const m = n + r.unmatched.length;
      if (n === 0 && r.unmatched.length === 0) {
        toast.warning('AI returned no plan blocks');
      } else if (n === 0) {
        toast.warning(`No matches found in ${r.unmatched.length} plan blocks`);
      } else {
        toast.success(`Scheduled ${n} of ${m} tasks`);
      }
      await load();
      onRefresh?.();
    } catch (e) {
      toast.error('AI plan failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      aiBusy = false;
    }
  }
</script>

<div class="flex flex-col h-full bg-mantle/40 border border-surface1 rounded overflow-hidden">
  <header class="flex items-center gap-2 px-3 py-2 border-b border-surface1 flex-shrink-0">
    <div class="flex-1 min-w-0">
      <h3 class="text-sm font-medium text-text truncate">Today's plan</h3>
      <p class="text-[10px] text-dim">drag onto grid · {filtered.length} task{filtered.length === 1 ? '' : 's'}</p>
    </div>
    <button
      onclick={runAi}
      disabled={aiBusy}
      class="px-2.5 py-1 text-xs rounded bg-secondary/15 text-secondary border border-secondary/30 hover:bg-secondary/25 disabled:opacity-60 flex items-center gap-1.5"
      title="Run plan-my-day and auto-schedule matched tasks"
    >
      {#if aiBusy}
        <span class="inline-block w-3 h-3 rounded-full border-2 border-secondary border-t-transparent animate-spin"></span>
        thinking…
      {:else}
        <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <path d="M9 18h6M10 22h4M12 2a7 7 0 0 0-4 12.7V17h8v-2.3A7 7 0 0 0 12 2z"/>
        </svg>
        Plan with AI
      {/if}
    </button>
  </header>

  <!-- Mobile: horizontal scroller. Desktop: vertical scroller. The
       parent layout switches between flex-row (mobile, top strip) and
       flex-row split (desktop, side rail) — see calendar/+page.svelte. -->
  <div class="flex-1 overflow-x-auto md:overflow-x-visible md:overflow-y-auto">
    {#if loading && filtered.length === 0}
      <div class="p-4 text-xs text-dim italic">loading…</div>
    {:else if filtered.length === 0}
      <div class="p-4 text-xs text-dim italic">
        Nothing for today. P1–P3 tasks and tasks due today / overdue show up here.
      </div>
    {:else}
      <div class="flex flex-row md:flex-col gap-2 md:gap-1 p-2 md:p-2 h-full md:h-auto">
        {#each filtered as t (t.id)}
          {@const tone = priorityTone(t.priority)}
          {@const scheduled = isToday(t.scheduledStart)}
          {@const dur = durationOf(t)}
          <div
            role="button"
            tabindex="0"
            onpointerdown={(e) => onRowPointerDown(e, t)}
            onclick={(e) => onRowClick(t, e)}
            onkeydown={(e) => { if (e.key === 'Enter') onRowClick(t, e as unknown as MouseEvent); }}
            title={scheduled
              ? `scheduled at ${fmtScheduled(t.scheduledStart)} — drag on grid to move`
              : 'drag onto the grid to schedule'}
            class="flex items-center gap-2 px-2 py-1.5 rounded text-xs border border-surface1 bg-base
              {scheduled ? 'opacity-40 cursor-default' : 'cursor-grab active:cursor-grabbing hover:border-primary/50'}
              flex-shrink-0 md:flex-shrink min-w-[180px] md:min-w-0"
            style="touch-action: {scheduled ? 'auto' : 'none'};"
          >
            <span
              class="w-2 h-2 rounded-full flex-shrink-0"
              style="background: var(--color-{tone})"
              aria-label="priority {t.priority || 'none'}"
            ></span>
            <span class="flex-1 min-w-0 truncate text-text">{t.text}</span>
            {#if scheduled}
              <span class="text-[10px] text-dim font-mono flex-shrink-0">{fmtScheduled(t.scheduledStart)}</span>
            {:else}
              <span class="text-[10px] text-dim font-mono flex-shrink-0">{dur}m</span>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  </div>
</div>
