<script lang="ts">
  import { onMount } from 'svelte';
  import { api, todayISO, type Task } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import TaskRow from '$lib/components/TaskRow.svelte';

  // InboxWidget is the user's "what should I touch right now?" surface.
  // We split open tasks into three actionable buckets:
  //
  //   Today      — overdue + due-today (sorted by priority then date)
  //   Quick wins — open, no scheduled commitment, estimated ≤30 min
  //                (or a heuristic "quick" task: short text, no due, P3+)
  //   Stale      — P1/P2 not touched in 7+ days — risk of dropping
  //
  // Each row is a TaskRow so the user can mark done in-place. We still
  // load triage=inbox so the section count matches the granit triage
  // semantic, but render from a broader "open" pool to surface stale work
  // that has slipped past its inbox state.

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

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') load();
    });
  });

  const STALE_DAYS = 7;
  const QUICK_MAX_MIN = 30;
  const today = todayISO();

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

  let todaySection = $derived.by(() => {
    return allOpen
      .filter((t) => t.dueDate && t.dueDate <= today)
      .sort(priCmp);
  });

  let quickWins = $derived.by(() => {
    // Prefer tasks with an explicit short estimate. Fall back to a
    // heuristic: short text, no due-date pressure, low priority — these
    // are the kind of "knock it off the list" wins the user wants.
    const explicit = allOpen.filter(
      (t) => (t.estimatedMinutes ?? 0) > 0 && (t.estimatedMinutes ?? 0) <= QUICK_MAX_MIN
    );
    if (explicit.length >= 3) {
      return explicit.sort((a, b) => (a.estimatedMinutes! - b.estimatedMinutes!)).slice(0, 6);
    }
    const heuristic = allOpen.filter(
      (t) =>
        !t.scheduledStart &&
        (!t.dueDate || t.dueDate >= today) &&
        t.text.length <= 60 &&
        (t.priority === 0 || t.priority >= 3)
    );
    return [...explicit, ...heuristic.filter((t) => !explicit.includes(t))].slice(0, 6);
  });

  let staleSection = $derived.by(() => {
    return allOpen
      .filter((t) => (t.priority === 1 || t.priority === 2))
      .filter((t) => daysSince(t.updatedAt ?? t.createdAt) >= STALE_DAYS)
      .sort((a, b) => daysSince(b.updatedAt ?? b.createdAt) - daysSince(a.updatedAt ?? a.createdAt))
      .slice(0, 6);
  });

  // Hide overlap: a task that's already in `today` shouldn't also show as
  // a quick win or stale. The user only needs to see it once.
  let quickWinsDeduped = $derived(quickWins.filter((t) => !todaySection.includes(t)));
  let staleDeduped = $derived(staleSection.filter((t) => !todaySection.includes(t) && !quickWinsDeduped.includes(t)));
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-4">
  <div class="flex items-baseline justify-between mb-3">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">
      Inbox
      {#if inbox.length > 0}
        <span class="ml-1 text-[10px] px-1.5 py-0.5 rounded bg-primary/15 text-primary tabular-nums">{inbox.length} to triage</span>
      {/if}
    </h2>
    <a href="/tasks" class="text-xs text-secondary hover:underline">triage →</a>
  </div>

  {#if loading && allOpen.length === 0}
    <div class="text-sm text-dim">loading…</div>
  {:else if allOpen.length === 0}
    <div class="text-sm text-success">all clear — inbox empty</div>
  {:else}
    {#if todaySection.length > 0}
      <h3 class="text-[11px] uppercase tracking-wider text-error mb-1.5 flex items-baseline gap-1.5">
        <span class="w-1.5 h-1.5 rounded-full bg-error inline-block"></span>
        Today · {todaySection.length}
      </h3>
      <div class="space-y-px mb-3">
        {#each todaySection.slice(0, 6) as t (t.id)}
          <TaskRow task={t} onChanged={load} />
        {/each}
        {#if todaySection.length > 6}
          <div class="text-[11px] text-dim pl-6">+{todaySection.length - 6} more</div>
        {/if}
      </div>
    {/if}

    {#if quickWinsDeduped.length > 0}
      <h3 class="text-[11px] uppercase tracking-wider text-info mb-1.5 flex items-baseline gap-1.5">
        <span class="w-1.5 h-1.5 rounded-full bg-info inline-block"></span>
        Quick wins · {quickWinsDeduped.length}
      </h3>
      <div class="space-y-px mb-3">
        {#each quickWinsDeduped as t (t.id)}
          <TaskRow task={t} onChanged={load} />
        {/each}
      </div>
    {/if}

    {#if staleDeduped.length > 0}
      <h3 class="text-[11px] uppercase tracking-wider text-warning mb-1.5 flex items-baseline gap-1.5">
        <span class="w-1.5 h-1.5 rounded-full bg-warning inline-block"></span>
        Stale · {staleDeduped.length}
        <span class="text-[10px] text-dim normal-case tracking-normal">untouched 7+ days</span>
      </h3>
      <div class="space-y-px mb-3">
        {#each staleDeduped as t (t.id)}
          <TaskRow task={t} onChanged={load} />
        {/each}
      </div>
    {/if}

    {#if todaySection.length === 0 && quickWinsDeduped.length === 0 && staleDeduped.length === 0}
      <div class="text-sm text-dim italic">nothing to action right now — open <a href="/tasks" class="text-secondary hover:underline">Tasks</a> to plan ahead</div>
    {/if}

    <div class="pt-3 mt-1 border-t border-surface1/60 flex items-center justify-between">
      <span class="text-[11px] text-dim">{allOpen.length} open total</span>
      <a href="/tasks" class="text-xs text-secondary hover:underline">Open Tasks page →</a>
    </div>
  {/if}
</section>
