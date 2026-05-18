<script lang="ts">
  // Weekly rollup that sits above the five review questions. Three
  // numbers, all answering "what shape did this week have?":
  //
  //   ✓ N tasks done           completedAt inside cursor's ISO week
  //   ⏰ N overdue              open & dueDate < today (today-relative,
  //                             so the count means the same thing
  //                             whether you're reviewing this week or
  //                             a past one)
  //   ◎ N goals moved          updated_at OR last_reviewed inside the
  //                             cursor week (one tally, dedup'd by id)
  //
  // Renders nothing while loading (no skeleton noise on a journaling
  // page) and a soft "no signal" line if every count is zero on a
  // current/future week — that way an empty week reads honestly
  // instead of as a load error.
  //
  // The data shape lives in this component, not in the parent, so
  // /review (and any future "weekly hub" surface) can drop in the
  // rollup with one tag.

  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import { startOfIsoWeek } from '$lib/util/isoWeek';
  import { fmtDateISO } from '$lib/util/date';

  type Props = {
    cursor: Date;
  };

  let { cursor }: Props = $props();

  let loaded = $state(false);
  let tasksDone = $state(0);
  let tasksOverdue = $state(0);
  let goalsMoved = $state(0);

  function withinWeek(iso: string | undefined, weekStart: Date, weekEnd: Date): boolean {
    if (!iso) return false;
    const t = new Date(iso).getTime();
    return t >= weekStart.getTime() && t < weekEnd.getTime();
  }

  async function load(cur: Date): Promise<void> {
    loaded = false;
    const weekStart = startOfIsoWeek(cur);
    const weekEnd = new Date(weekStart);
    weekEnd.setDate(weekEnd.getDate() + 7);

    const today = fmtDateISO(new Date());
    const [doneRes, overdueRes, goalsRes] = await Promise.allSettled([
      api.listTasks({ status: 'done' }),
      api.listTasks({ status: 'open', due_before: today }),
      api.listGoals()
    ]);

    if (doneRes.status === 'fulfilled') {
      tasksDone = doneRes.value.tasks.filter((t) => withinWeek(t.completedAt, weekStart, weekEnd))
        .length;
    } else {
      tasksDone = 0;
    }

    if (overdueRes.status === 'fulfilled') {
      tasksOverdue = overdueRes.value.tasks.filter(
        (t) => !t.done && !!t.dueDate && t.dueDate < today
      ).length;
    } else {
      tasksOverdue = 0;
    }

    if (goalsRes.status === 'fulfilled') {
      const moved = new Set<string>();
      for (const g of goalsRes.value.goals) {
        if (withinWeek(g.updated_at, weekStart, weekEnd)) moved.add(g.id);
        if (withinWeek(g.last_reviewed, weekStart, weekEnd)) moved.add(g.id);
        if (withinWeek(g.completed_at, weekStart, weekEnd)) moved.add(g.id);
      }
      goalsMoved = moved.size;
    } else {
      goalsMoved = 0;
    }

    loaded = true;
  }

  // Re-run on cursor change so navigating to a past week reshapes
  // the stats without manual refresh.
  $effect(() => {
    void cursor;
    void load(cursor);
  });

  let totalSignal = $derived(tasksDone + tasksOverdue + goalsMoved);
</script>

{#if loaded}
  <div class="mb-4 px-3 py-2 bg-surface0 border border-surface1 rounded text-xs flex items-center gap-4 flex-wrap">
    <span class="text-dim uppercase tracking-wider">This week</span>
    <span class="flex items-center gap-1">
      <span class="text-success" aria-hidden="true">✓</span>
      <span class="text-text font-medium tabular-nums">{tasksDone}</span>
      <span class="text-subtext">task{tasksDone === 1 ? '' : 's'} done</span>
    </span>
    <span class="flex items-center gap-1">
      <span class="text-error" aria-hidden="true">⏰</span>
      <span class="text-text font-medium tabular-nums">{tasksOverdue}</span>
      <span class="text-subtext">overdue</span>
    </span>
    <span class="flex items-center gap-1">
      <span class="text-secondary" aria-hidden="true">◎</span>
      <span class="text-text font-medium tabular-nums">{goalsMoved}</span>
      <span class="text-subtext">goal{goalsMoved === 1 ? '' : 's'} moved</span>
    </span>
    {#if totalSignal === 0}
      <span class="text-dim italic">quiet week — write what was actually here, not what isn't.</span>
    {/if}
  </div>
{/if}
