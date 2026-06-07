<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { api, todayISO, type Task } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import TaskRow from '$lib/components/TaskRow.svelte';
  import { createCoalescedReload } from '$lib/util/coalesce';
  import { onLocalMidnight } from '$lib/util/midnightTick';

  // InboxWidget is the "what's slipping?" surface. Today / overdue
  // live in TodayTasksWidget; this one owns the buckets that AREN'T
  // already visible there:
  //
  //   Quick wins — open, no scheduled commitment, estimated ≤30 min
  //                (or a heuristic "quick" task: short text, no due, P3+)
  //   Stale      — P1/P2 not touched in 7+ days — risk of dropping
  //
  // Previously we ALSO surfaced a Today bucket here, which duplicated
  // TodayTasks's Today/Overdue sections row-for-row. The triage count
  // chip in the header still reflects the granit inbox semantic so the
  // widget remains useful as a triage launcher.

  let inbox = $state<Task[]>([]);
  let allOpen = $state<Task[]>([]);
  let loading = $state(false);

  async function load() {
    loading = true;
    try {
      const [tri, open] = await Promise.all([
        api.listTasks({ triage: 'inbox', status: 'open' }),
        api.listTasks({ status: 'open' })
      ]);
      inbox = tri.tasks;
      allOpen = open.tasks;
    } catch (e) {
      console.error('inbox widget: load failed', e);
    } finally {
      loading = false;
    }
  }

  const reload = createCoalescedReload(load, 600);
  // `today` reactive so quick-wins / stale derive against the live day
  // even on a dashboard left open past midnight.
  let today = $state(todayISO());
  let stopMidnight: (() => void) | null = null;
  onMount(() => {
    load();
    stopMidnight = onLocalMidnight(() => {
      today = todayISO();
      void load();
    });
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') reload.trigger();
    });
  });
  onDestroy(() => {
    reload.cancel();
    if (stopMidnight) stopMidnight();
  });

  const STALE_DAYS = 7;
  const QUICK_MAX_MIN = 30;

  function priCmp(a: Task, b: Task): number {
    const ap = a.priority || 99;
    const bp = b.priority || 99;
    if (ap !== bp) return ap - bp;
    const ad = a.dueDate ?? '~';
    const bd = b.dueDate ?? '~';
    return ad < bd ? -1 : ad > bd ? 1 : 0;
  }

  function daysSince(iso?: string): number {
    if (!iso) return Infinity;
    const t = new Date(iso).getTime();
    if (Number.isNaN(t)) return Infinity;
    return Math.floor((Date.now() - t) / 86_400_000);
  }

  // Filter out anything that's already due/overdue — TodayTasks owns
  // those rows; we don't want a row appearing in both lists.
  let notTodayOrOverdue = $derived.by(() =>
    allOpen.filter((t) => !t.dueDate || t.dueDate > today)
  );

  let quickWins = $derived.by(() => {
    const explicit = notTodayOrOverdue.filter(
      (t) => (t.estimatedMinutes ?? 0) > 0 && (t.estimatedMinutes ?? 0) <= QUICK_MAX_MIN
    );
    if (explicit.length >= 3) {
      return explicit.sort((a, b) => (a.estimatedMinutes! - b.estimatedMinutes!)).slice(0, 6);
    }
    const heuristic = notTodayOrOverdue.filter(
      (t) =>
        !t.scheduledStart &&
        t.text.length <= 60 &&
        (t.priority === 0 || t.priority >= 3)
    );
    return [...explicit, ...heuristic.filter((t) => !explicit.includes(t))].slice(0, 6);
  });

  let staleSection = $derived.by(() => {
    return notTodayOrOverdue
      .filter((t) => (t.priority === 1 || t.priority === 2))
      .filter((t) => daysSince(t.updatedAt ?? t.createdAt) >= STALE_DAYS)
      .sort((a, b) => daysSince(b.updatedAt ?? b.createdAt) - daysSince(a.updatedAt ?? a.createdAt))
      .slice(0, 6);
  });

  let staleDeduped = $derived(staleSection.filter((t) => !quickWins.includes(t)));
</script>

<section class="bg-surface0 border border-surface1 rounded-lg shadow-sm p-3">
  <div class="flex items-baseline justify-between mb-3">
    <h2 class="text-xs text-dim font-semibold">
      Inbox
      {#if inbox.length > 0}
        <span class="ml-1 text-[10px] px-1.5 py-0.5 rounded bg-surface1 text-primary tabular-nums">{inbox.length} to triage</span>
      {/if}
    </h2>
    <a href="/tasks" class="text-xs text-secondary hover:underline">triage →</a>
  </div>

  {#if loading && allOpen.length === 0}
    <div class="text-sm text-dim">loading…</div>
  {:else if allOpen.length === 0}
    <div class="text-sm text-success">all clear — inbox empty</div>
  {:else}
    {#if quickWins.length > 0}
      <h3 class="text-[11px] uppercase tracking-wider text-info mb-1.5 flex items-baseline gap-1.5">
        <span class="w-1.5 h-1.5 rounded-full bg-info inline-block"></span>
        Quick wins · {quickWins.length}
      </h3>
      <div class="space-y-px mb-3">
        {#each quickWins as t (t.id)}
          <TaskRow task={t} onChanged={load} />
        {/each}
      </div>
    {/if}

    {#if staleDeduped.length > 0}
      <h3 class="text-[11px] uppercase tracking-wider text-warning mb-1.5 flex items-baseline gap-1.5">
        <span class="w-1.5 h-1.5 rounded-full bg-warning inline-block"></span>
        Stale · {staleDeduped.length}
        <span class="text-[10px] text-dim normal-case tracking-normal">untouched {STALE_DAYS}+ days</span>
      </h3>
      <div class="space-y-px mb-3">
        {#each staleDeduped as t (t.id)}
          <TaskRow task={t} onChanged={load} />
        {/each}
      </div>
    {/if}

    {#if quickWins.length === 0 && staleDeduped.length === 0}
      <div class="text-sm text-dim italic">nothing slipping — Today/Overdue lives in the Tasks tile above</div>
    {/if}

    <div class="pt-3 mt-1 border-t border-surface1 flex items-center justify-between">
      <span class="text-[11px] text-dim">{allOpen.length} open total</span>
      <a href="/tasks" class="text-xs text-secondary hover:underline">Open Tasks page →</a>
    </div>
  {/if}
</section>
