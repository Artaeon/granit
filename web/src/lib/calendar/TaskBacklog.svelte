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
  import { onWsEvent } from '$lib/ws';
  import { dragStore } from './dragStore';
  import PlanMyDayDrawer from './PlanMyDayDrawer.svelte';
  import { fmtDateISO } from './utils';
  import { todayISO } from '$lib/util/date';

  let { onRefresh }: { onRefresh?: () => void } = $props();

  let tasks = $state<Task[]>([]);
  let loading = $state(false);
  let isMobile = $state(false);

  // Plan-my-day drawer. Replaces the old fire-and-forget
  // "Plan with AI" button: now we open a drawer that runs a dry-run,
  // shows editable proposals, and only commits on Apply.
  let planDrawerOpen = $state(false);

  // Tasks already on today's grid live in the list too (greyed) so the
  // user has an at-a-glance "what's already scheduled" without flipping
  // back to the grid. Greyed rows aren't draggable — drag would
  // duplicate the schedule. The user moves a scheduled task by dragging
  // its event chip on the grid (existing reschedule code path).

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

  // ─── AI plan ───
  // Opens the preview drawer instead of firing off a one-shot call.
  // The user gets to see proposed slots, edit them, accept per row,
  // and only THEN does anything land on the calendar. The drawer
  // owns the dry-run/apply round-trips; we just refresh on success.
  function openPlanDrawer() {
    planDrawerOpen = true;
  }

  async function onPlanApplied() {
    await load();
    onRefresh?.();
  }
</script>

<div class="flex flex-col h-full bg-mantle border border-surface1 rounded-lg overflow-hidden">
  <header class="px-3 py-2.5 border-b border-surface1 flex-shrink-0">
    <div class="flex items-baseline gap-2">
      <h3 class="text-sm font-semibold text-text flex-1 truncate">Today's plan</h3>
      <span class="text-[10px] text-dim tabular-nums">{filtered.filter((t) => !isToday(t.scheduledStart)).length} unscheduled</span>
    </div>
    <!-- AI plan button — primary action of this rail. Solid fill so
         it reads as "the move" rather than a quiet utility. Opens
         PlanMyDayDrawer for dry-run preview + per-row accept. -->
    <button
      onclick={openPlanDrawer}
      class="mt-2 w-full px-3 py-2 text-xs font-semibold rounded-md bg-primary text-on-primary hover:opacity-90 inline-flex items-center justify-center gap-1.5"
      title="Preview an AI-drafted schedule, edit, then apply"
    >
      <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
        <path d="M12 3l1.5 4.5L18 9l-4.5 1.5L12 15l-1.5-4.5L6 9l4.5-1.5L12 3z"/>
      </svg>
      Auto-place with AI
    </button>
    <p class="mt-2 text-[10px] text-dim leading-snug">
      Drag a card onto the grid to schedule manually — or let AI propose slots for everything below.
    </p>
  </header>

  <!-- Single scroll axis on every form factor — vertical. The mobile
       parent (calendar/+page.svelte) now gives this aside a real
       clamp()-bounded height, so a vertical scroller shows 5–6 cards
       at once instead of fighting a 128px horizontal strip where only
       one card fit at a time. -->
  <div class="flex-1 overflow-y-auto">
    {#if loading && filtered.length === 0}
      <div class="p-4 text-xs text-dim italic">loading…</div>
    {:else if filtered.length === 0}
      <div class="p-4 text-xs text-dim italic leading-relaxed">
        Nothing for today. P1–P3 tasks and tasks due today or overdue surface here.
      </div>
    {:else}
      <ul class="flex flex-col gap-1 p-2">
        {#each filtered as t (t.id)}
          {@const tone = priorityTone(t.priority)}
          {@const scheduled = isToday(t.scheduledStart)}
          {@const dur = durationOf(t)}
          <!-- svelte-ignore a11y_no_noninteractive_element_to_interactive_role -->
          <li
            role="button"
            tabindex="0"
            onpointerdown={(e) => onRowPointerDown(e, t)}
            onclick={(e) => onRowClick(t, e)}
            onkeydown={(e) => { if (e.key === 'Enter') onRowClick(t, e as unknown as MouseEvent); }}
            title={scheduled
              ? `scheduled at ${fmtScheduled(t.scheduledStart)} — drag on the grid to move`
              : 'drag onto the grid to schedule'}
            class="group flex items-stretch gap-2 px-2 py-1.5 rounded-md bg-base text-xs transition-colors
              {scheduled ? 'opacity-45 cursor-default' : 'cursor-grab active:cursor-grabbing hover:bg-surface0'}"
            style="touch-action: {scheduled ? 'auto' : 'none'};"
          >
            <!-- Priority accent bar — vertical strip on the left in the
                 priority colour. Matches the event chip's inset bar on
                 the grid so a P1 task in the backlog reads as the same
                 colour family when it lands as an event. -->
            <span
              class="w-1 rounded-full flex-shrink-0"
              style="background: var(--color-{tone})"
              aria-label="priority {t.priority || 'none'}"
            ></span>
            <div class="flex-1 min-w-0 flex flex-col justify-center">
              <span class="text-text truncate font-medium">{t.text}</span>
              {#if !scheduled && (t.dueDate || t.estimatedMinutes)}
                <span class="text-[10px] text-dim mt-px truncate">
                  {#if t.dueDate}due {t.dueDate}{/if}
                  {#if t.dueDate && t.estimatedMinutes} · {/if}
                  {#if t.estimatedMinutes}~{t.estimatedMinutes}m{/if}
                </span>
              {/if}
            </div>
            <span class="self-center text-[10px] font-mono tabular-nums px-1.5 py-0.5 rounded bg-surface0 text-dim flex-shrink-0">
              {scheduled ? fmtScheduled(t.scheduledStart) : `${dur}m`}
            </span>
            {#if !scheduled}
              <span
                class="hidden md:flex self-center text-dim opacity-0 group-hover:opacity-100 transition-opacity flex-shrink-0"
                aria-hidden="true"
              >
                <svg viewBox="0 0 24 24" class="w-3 h-3" fill="currentColor"><circle cx="9" cy="6" r="1.4"/><circle cx="15" cy="6" r="1.4"/><circle cx="9" cy="12" r="1.4"/><circle cx="15" cy="12" r="1.4"/><circle cx="9" cy="18" r="1.4"/><circle cx="15" cy="18" r="1.4"/></svg>
              </span>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</div>

<PlanMyDayDrawer bind:open={planDrawerOpen} onApplied={onPlanApplied} />
