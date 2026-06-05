<!--
  TasksCardList — the shared TaskCard-rendering loop used by the
  stale / quickwins / review small views (and the inbox view body
  below the proposal panels). Honours the parent's subtree-collapse
  state (a child task hidden by its parent's collapse is skipped)
  and the keyboard cursor's primary ring.

  Pure presentation. The only variable bit between views is `dim`,
  which the review view sets to true so completed tasks render
  visually softened.
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
    cursorIdx: number;
    selectedIds: Set<string>;
    /** Soften the list visually — review view uses this for completed
     *  tasks. Other views leave it false. */
    dim?: boolean;
    load: () => Promise<void> | void;
    onOpenDetail: (t: Task) => void;
    onOpenContext: (t: Task, x: number, y: number) => void;
  };

  let {
    filterCtl,
    dataCtl,
    viewCtl,
    cursorIdx,
    selectedIds = $bindable(),
    dim = false,
    load,
    onOpenDetail,
    onOpenContext
  }: Props = $props();
</script>

<div class="space-y-2 {dim ? 'opacity-80' : ''}">
  {#each filterCtl.filtered.filter((tt) => !dataCtl.isHiddenByCollapse(tt.id, dataCtl.collapsedIds)) as t (t.id)}
    <div data-task-id={t.id} class={cursorIdx >= 0 && filterCtl.filtered[cursorIdx]?.id === t.id ? 'ring-2 ring-primary/40 rounded' : ''}>
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
    </div>
  {/each}
</div>
