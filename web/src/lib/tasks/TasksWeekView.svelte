<!--
  TasksWeekView — 8-column rolling-week board:
    • Overdue strip pinned above the grid (only when there ARE overdue
      tasks) — clicking "open in list" drops into List view filtered to
      smart=overdue.
    • Unscheduled column on the left — capture surface for tasks with
      no due date. Caps at 50 rows with a "…X more" hint.
    • Seven day columns from today (today first, then rolling). Each
      header is clickable: pressing one drops the user into List view
      filtered to that day so they can drill in. Today's column is
      primary-bordered.

  Pure orchestration. The week-column derivation lives in viewCtl
  (viewCtl.weekColumns); this view renders it.
-->
<script lang="ts">
  import TaskCard from './TaskCard.svelte';
  import type { Task } from '$lib/api';
  import type { TasksFilterStateController } from './tasksFilterState.svelte';
  import type { TasksDataController } from './tasksData.svelte';
  import type { TasksViewStateController } from './tasksViewState.svelte';

  type Props = {
    filterCtl: TasksFilterStateController;
    dataCtl: TasksDataController;
    viewCtl: TasksViewStateController;
    selectedIds: Set<string>;
    load: () => Promise<void> | void;
    onOpenDetail: (t: Task) => void;
    onOpenContext: (t: Task, x: number, y: number) => void;
  };

  let {
    filterCtl,
    dataCtl,
    viewCtl,
    selectedIds = $bindable(),
    load,
    onOpenDetail,
    onOpenContext
  }: Props = $props();

  // Drop into List view filtered to a header's day. Today / tomorrow
  // get their smart-filter shortcut; further days clear the smart
  // filter (the cursor still moves to today's list by default).
  function openDayInList(date: string, isToday: boolean) {
    viewCtl.view = 'list';
    filterCtl.q = '';
    if (isToday) filterCtl.smartFilter = 'today';
    else if (date === viewCtl.weekColumns.days[1]?.date) filterCtl.smartFilter = 'tomorrow';
    else filterCtl.smartFilter = '';
  }

  function openOverdueInList() {
    filterCtl.smartFilter = 'overdue';
    viewCtl.view = 'list';
  }
</script>

<!-- Week view — 8 columns: Unscheduled + 7 rolling days from today.
     Overdue tasks bubble up as a striped strip pinned above today's
     column so the user doesn't have to hunt them across past dates. -->
<div class="flex flex-col gap-2">
  {#if viewCtl.weekColumns.overdue.length > 0}
    <div class="bg-surface0 border border-error rounded p-2">
      <div class="flex items-baseline gap-2 mb-1.5">
        <h3 class="text-xs uppercase tracking-wider text-error font-medium">overdue</h3>
        <span class="text-[10px] font-mono text-dim">{viewCtl.weekColumns.overdue.length}</span>
        <button
          type="button"
          onclick={openOverdueInList}
          class="ml-auto text-[10px] text-error hover:underline font-mono"
        >open in list →</button>
      </div>
      <div class="space-y-1">
        {#each viewCtl.weekColumns.overdue.slice(0, 5) as t (t.id)}
          <TaskCard
            task={t}
            compact={viewCtl.compactCards}
            hasChildren={(dataCtl.childCount.get(t.id) ?? 0) > 0}
            childCount={dataCtl.childCount.get(t.id) ?? 0}
            collapsed={dataCtl.collapsedIds.has(t.id)}
            onToggleCollapse={() => dataCtl.toggleCollapsed(t.id)}
            onChanged={load}
            bind:selectedIds
            onOpenDetail={onOpenDetail}
            onContextMenu={onOpenContext}
          />
        {/each}
        {#if viewCtl.weekColumns.overdue.length > 5}
          <p class="text-[11px] text-dim italic px-1">…{viewCtl.weekColumns.overdue.length - 5} more</p>
        {/if}
      </div>
    </div>
  {/if}
  <div class="grid grid-cols-[minmax(10rem,1fr)_repeat(7,minmax(0,1fr))] gap-2 min-h-[20rem]">
    <!-- Unscheduled column — capture surface for tasks with no date.
         Caps at 50 with a "…X more" hint to keep the column scrollable
         without dumping the entire backlog inline. -->
    <div class="bg-surface0 border border-surface1 rounded p-2 flex flex-col min-h-0">
      <div class="flex items-baseline gap-2 mb-1.5 sticky top-0 bg-surface0 pb-1 border-b border-surface1">
        <h3 class="text-xs uppercase tracking-wider text-dim font-medium">unscheduled</h3>
        <span class="text-[10px] font-mono text-dim">{viewCtl.weekColumns.unscheduled.length}</span>
      </div>
      <div class="flex-1 overflow-y-auto space-y-1">
        {#each viewCtl.weekColumns.unscheduled.slice(0, 50) as t (t.id)}
          <TaskCard
            task={t}
            compact
            hasChildren={(dataCtl.childCount.get(t.id) ?? 0) > 0}
            childCount={dataCtl.childCount.get(t.id) ?? 0}
            collapsed={dataCtl.collapsedIds.has(t.id)}
            onToggleCollapse={() => dataCtl.toggleCollapsed(t.id)}
            onChanged={load}
            bind:selectedIds
            onOpenDetail={onOpenDetail}
            onContextMenu={onOpenContext}
          />
        {/each}
        {#if viewCtl.weekColumns.unscheduled.length > 50}
          <p class="text-[11px] text-dim italic px-1">…{viewCtl.weekColumns.unscheduled.length - 50} more</p>
        {/if}
        {#if viewCtl.weekColumns.unscheduled.length === 0}
          <p class="text-[11px] text-dim italic px-1">nothing untaken — good shape.</p>
        {/if}
      </div>
    </div>
    <!-- Seven day columns. The today column gets a primary border so
         the user's eye lands on it first. -->
    {#each viewCtl.weekColumns.days as col (col.date)}
      <div class="bg-surface0 border {col.isToday ? 'border-primary' : 'border-surface1'} rounded p-2 flex flex-col min-h-0">
        <div class="flex items-baseline gap-1.5 mb-1.5 sticky top-0 bg-surface0 pb-1 border-b border-surface1">
          <button
            type="button"
            onclick={() => openDayInList(col.date, col.isToday)}
            class="text-xs uppercase tracking-wider {col.isToday ? 'text-primary' : 'text-text'} font-medium hover:underline"
            title="open this day in the list view"
          >{col.label}</button>
          <span class="text-[10px] text-dim font-mono">{col.sublabel}</span>
          <span class="ml-auto text-[10px] font-mono text-dim">{col.tasks.length}</span>
        </div>
        <div class="flex-1 overflow-y-auto space-y-1">
          {#each col.tasks as t (t.id)}
            <TaskCard
              task={t}
              compact
              hasChildren={(dataCtl.childCount.get(t.id) ?? 0) > 0}
              childCount={dataCtl.childCount.get(t.id) ?? 0}
              collapsed={dataCtl.collapsedIds.has(t.id)}
              onToggleCollapse={() => dataCtl.toggleCollapsed(t.id)}
              onChanged={load}
              bind:selectedIds
              onOpenDetail={onOpenDetail}
              onContextMenu={onOpenContext}
            />
          {/each}
          {#if col.tasks.length === 0}
            <p class="text-[11px] text-dim italic px-1">—</p>
          {/if}
        </div>
      </div>
    {/each}
  </div>
</div>
