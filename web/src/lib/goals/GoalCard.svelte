<!--
  GoalCard — the card-body button used by /goals' cards view.
  Renders title + description + status pill + meta line + tags +
  progress bar + roll-up chips. The parent owns the surrounding
  <article> wrapper (because the wrapper's border tone is also
  parent state, and the AI "suggest next milestone" footer that
  lives inside the same article is a sibling of this component).

  Props
    goal      The Goal record. Drives every visible piece.
    progress  {done, total, pct} — precomputed by the parent because
              it depends on the parent's loaded openTasks +
              doneTasks state.
    rollup    Linked-task + matched-project context (open / done /
              project). Same shape the parent already computes per
              goal for the list view.
    stalled   Whether to render the stalled badge alongside the
              status pill. The parent owns the stalled-goal index
              (30-day no-activity + no recent completions) so the
              card just gets a boolean.
    onClick   Open the goal-detail drawer. Parent owns the URL +
              drawer state; we just notify.
-->
<script lang="ts">
  import type { Goal } from '$lib/api';
  import { inlineMd } from '$lib/util/inlineMd';
  import { fmtTargetDate, statusColor, targetChip } from './util';

  interface Rollup {
    open: number;
    done: number;
    project: { name: string; progress?: number } | null;
  }

  let {
    goal,
    progress,
    rollup,
    stalled,
    onClick
  }: {
    goal: Goal;
    progress: { done: number; total: number; pct: number };
    rollup: Rollup;
    stalled: boolean;
    onClick: () => void;
  } = $props();

  let sc = $derived(statusColor(goal.status));
  // Chip is suppressed on completed / archived to match the same
  // status-aware urgency rule the kanban card uses.
  let chip = $derived(
    goal.status === 'active' || goal.status === 'paused'
      ? targetChip(goal.target_date)
      : null
  );
</script>

<button
  type="button"
  onclick={onClick}
  class="w-full text-left p-4 flex flex-col gap-2"
>
  <div class="flex items-start gap-3">
    <div class="flex-1 min-w-0">
      <h2 class="text-base sm:text-lg font-semibold text-text break-words">{@html inlineMd(goal.title)}</h2>
      {#if goal.description}
        <p class="text-sm text-subtext mt-1 break-words">{@html inlineMd(goal.description)}</p>
      {/if}
    </div>
    <div class="flex flex-col items-end gap-1 flex-shrink-0">
      <span class="text-[10px] uppercase tracking-wider px-2 py-0.5 rounded {sc.bg} {sc.text}">
        {goal.status ?? 'active'}
      </span>
      {#if stalled}
        <span class="text-[10px] uppercase tracking-wider px-2 py-0.5 rounded bg-surface0 text-warning" title="No edits in 30+ days, no recent completed tasks">
          stalled
        </span>
      {/if}
    </div>
  </div>

  <div class="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-dim">
    {#if goal.target_date}
      <span class="inline-flex items-baseline gap-1.5">
        <span>🎯 {fmtTargetDate(goal.target_date)}</span>
        {#if chip}
          <span
            class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-medium tabular-nums whitespace-nowrap"
            style="background: color-mix(in srgb, var(--color-{chip.tone}) 14%, transparent); color: var(--color-{chip.tone});"
          >{chip.label}</span>
        {/if}
      </span>
    {/if}
    {#if goal.project}<span>📁 {goal.project}</span>{/if}
    {#if goal.venture}<span class="text-secondary">🏢 {goal.venture}</span>{/if}
    {#if goal.category}<span>· {goal.category}</span>{/if}
    {#if progress.total > 0}<span>{progress.done}/{progress.total} milestones</span>{/if}
    {#if goal.review_frequency}<span>↻ {goal.review_frequency}</span>{/if}
  </div>

  {#if goal.tags && goal.tags.length > 0}
    <div class="flex flex-wrap gap-1">
      {#each goal.tags as t}
        <span class="text-[10px] px-1.5 py-0.5 bg-surface1 text-subtext rounded">#{t}</span>
      {/each}
    </div>
  {/if}

  {#if progress.total > 0}
    <div class="mt-1">
      <div class="h-1.5 bg-mantle rounded-full overflow-hidden">
        <div class="h-full bg-primary transition-all" style="width: {progress.pct}%"></div>
      </div>
      <div class="text-[10px] text-dim mt-1">{progress.pct}% complete</div>
    </div>
  {/if}

  <!-- Roll-up chips: linked tasks (open + done) and the matched
       project. Renders nothing when a goal has no execution
       behind it so the cards for orphan goals don't get noisier.
       The chips look passive (they live inside the card-wide
       button) but stay visually distinct so they read as
       "context" rather than "card body". -->
  {#if rollup.open + rollup.done > 0 || rollup.project}
    <div class="flex flex-wrap items-center gap-1.5 pt-1.5 mt-1 border-t border-surface1 text-[11px]">
      {#if rollup.open > 0}
        <span class="px-1.5 py-0.5 bg-surface1 text-subtext rounded tabular-nums" title="open tasks linked to this goal">
          {rollup.open} open task{rollup.open === 1 ? '' : 's'}
        </span>
      {/if}
      {#if rollup.done > 0}
        <span class="px-1.5 py-0.5 bg-surface0 text-success rounded tabular-nums" title="completed tasks linked to this goal">
          {rollup.done} done
        </span>
      {/if}
      {#if rollup.project}
        <span class="px-1.5 py-0.5 bg-surface1 text-secondary rounded truncate max-w-[14rem]" title="linked project">
          📁 {rollup.project.name}
          {#if typeof rollup.project.progress === 'number'}
            <span class="opacity-70 tabular-nums ml-0.5">{rollup.project.progress}%</span>
          {/if}
        </span>
      {/if}
    </div>
  {/if}
</button>
