<!--
  TasksViewToolbar — the secondary toolbar that shows above list +
  kanban views. List view exposes group + sort selectors; kanban
  exposes a columns selector. A passive-stats cluster (done today /
  done 7d / Σ est budget) lives on the right so the user has a one-
  line at-a-glance signal without the previous 14-chip stat row.

  Parent gates rendering on `viewCtl.view === 'list' || 'kanban'` —
  this component renders its own inner branch for the selector set.
-->
<script lang="ts">
  import { fmtEstBudget } from './tasksHelpers';
  import type { TasksViewStateController } from './tasksViewState.svelte';
  import type { TasksDataController } from './tasksData.svelte';

  type Props = {
    viewCtl: TasksViewStateController;
    dataCtl: TasksDataController;
  };

  let { viewCtl, dataCtl }: Props = $props();
</script>

<div class="px-3 py-1.5 border-b border-surface1 flex items-center gap-2 text-xs flex-shrink-0 flex-wrap bg-mantle">
  {#if viewCtl.view === 'list'}
    <span class="text-dim text-[11px] select-none">group</span>
    <select
      bind:value={viewCtl.groupBy}
      title="How to split the list into sections"
      class="bg-surface0 border border-surface1 rounded px-2 py-0.5 text-text"
    >
      <option value="due">due date</option>
      <option value="priority">priority</option>
      <option value="tag">tag</option>
      <option value="project">project</option>
      <option value="goal">goal</option>
      <option value="deadline">deadline</option>
      <option value="note">note</option>
    </select>
    <span class="text-dim text-[11px] select-none">sort</span>
    <select
      bind:value={viewCtl.sortBy}
      title="How to order tasks inside each group"
      class="bg-surface0 border border-surface1 rounded px-2 py-0.5 text-text"
    >
      <option value="auto">auto</option>
      <option value="priority">priority</option>
      <option value="due">due</option>
      <option value="age">age (oldest first)</option>
      <option value="alpha">A → Z</option>
      <option value="estimate">estimate (smallest)</option>
    </select>
  {:else}
    <span class="text-dim text-[11px] select-none">columns</span>
    <select bind:value={viewCtl.kanbanMode} class="bg-surface0 border border-surface1 rounded px-2 py-0.5 text-text">
      <option value="priority">priority</option>
      <option value="due">due</option>
      <option value="triage">triage (granit)</option>
      <option value="config">config</option>
    </select>
  {/if}
  <span class="flex-1"></span>
  <!-- Passive stats — done today / done 7d / Σ est budget. One-line
       glance signal. The full set (noEstCount, avgPriority, snoozed)
       lives in the filter panel. -->
  {#if dataCtl.stats.doneToday > 0}
    <span class="text-success font-mono tabular-nums select-none" title="Completed today">✓ {dataCtl.stats.doneToday}</span>
  {/if}
  {#if dataCtl.stats.doneWeek > 0}
    <span class="text-success/80 font-mono tabular-nums select-none" title="Completed in the last 7 days">7d ✓ {dataCtl.stats.doneWeek}</span>
  {/if}
  {#if dataCtl.stats.sumEstMin > 0}
    <span class="text-secondary font-mono tabular-nums select-none" title="Total estimated minutes across open non-snoozed tasks. 8h = one day-block.">Σ {fmtEstBudget(dataCtl.stats.sumEstMin)}</span>
  {/if}
</div>
