<!--
  GoalKanbanCard — one card in the kanban column on /goals.
  Extracted out of web/src/routes/goals/+page.svelte (1815 lines) so
  the kanban view's {#each} loop becomes a clean
  `<GoalKanbanCard {goal} {progress} onClick={...} />` instead of
  inlining the same 30-line card markup that the parent's other
  views (cards / list) already duplicate similar versions of.

  Props
    goal       The Goal record. Status + target_date + venture +
               project drive the visual cues.
    progress   {done, total, pct} — precomputed by the parent
               because it depends on the parent's loaded openTasks
               + doneTasks state, and passing the tasks down would
               be a larger refactor for the same render.
    onClick    Called when the user taps the card. Parent owns
               openDetail() because the detail drawer is its
               concern.
-->
<script lang="ts">
  import type { Goal } from '$lib/api';
  import { inlineMd } from '$lib/util/inlineMd';
  import { fmtTargetDate, goalTargetTone, targetChip } from './util';

  let {
    goal,
    progress,
    onClick
  }: {
    goal: Goal;
    progress: { done: number; total: number; pct: number };
    onClick: () => void;
  } = $props();

  // Derive once per goal change so the template stays declarative.
  // chip is suppressed on completed / archived to match the
  // urgency-border rule — a chip on a past-completed goal would
  // shout the same way the border used to.
  let tone = $derived(goalTargetTone(goal.status, goal.target_date));
  let chip = $derived(
    goal.status === 'active' || goal.status === 'paused'
      ? targetChip(goal.target_date)
      : null
  );
</script>

<button
  type="button"
  onclick={onClick}
  class="w-full text-left p-2.5 bg-mantle rounded border border-surface1 hover:border-primary transition-colors {tone ? 'border-l-4' : ''}"
  style={tone ? `border-left-color: var(--color-${tone});` : ''}
>
  <div class="text-sm font-medium text-text break-words leading-snug">{@html inlineMd(goal.title)}</div>
  <div class="flex flex-wrap items-baseline gap-x-2 gap-y-0.5 mt-1.5 text-[11px] text-dim">
    {#if goal.target_date}
      <span class="font-mono tabular-nums">{fmtTargetDate(goal.target_date)}</span>
    {/if}
    {#if chip}
      <span class="tabular-nums" style="color: var(--color-{chip.tone});">{chip.label}</span>
    {/if}
  </div>
  {#if goal.venture || goal.project}
    <div class="text-[11px] text-secondary truncate mt-0.5">
      {#if goal.venture}🏢 {goal.venture}{/if}
      {#if goal.venture && goal.project} · {/if}
      {#if goal.project}📁 {goal.project}{/if}
    </div>
  {/if}
  {#if progress.total > 0}
    <div class="mt-1.5">
      <div class="h-1 bg-surface1 rounded-full overflow-hidden">
        <div class="h-full bg-primary" style="width: {progress.pct}%"></div>
      </div>
      <div class="text-[10px] text-dim mt-0.5 tabular-nums">{progress.done}/{progress.total} · {progress.pct}%</div>
    </div>
  {/if}
</button>
