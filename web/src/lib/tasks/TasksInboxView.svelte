<!--
  TasksInboxView — the "untriaged" surface. Two AI buttons sit at the
  top:
    • ✨ AI triage — proposes priority + schedule per untriaged task
    • ✨ Detect deadlines — scans open tasks without a due_date and
      proposes ones whose title implies a deadline

  Each opens its own proposals panel below the buttons, with accept /
  skip per row + a discard-all dismiss. The task list at the bottom
  is the same TaskCard render the list view uses.

  Pure orchestration over the parent's stores: triage / deadline /
  filterCtl / dataCtl / viewCtl. The view doesn't fetch anything on
  its own; load() comes back as the onChanged prop so per-card
  mutations + accept actions refresh through the parent's coalesced
  reload.
-->
<script lang="ts">
  import TaskCard from './TaskCard.svelte';
  import type { Task } from '$lib/api';
  import type { TriageStore, DeadlineStore } from './aiAgentStore';
  import type { TasksFilterStateController } from './tasksFilterState.svelte';
  import type { TasksDataController } from './tasksData.svelte';
  import type { TasksViewStateController } from './tasksViewState.svelte';

  type Props = {
    triage: TriageStore;
    deadline: DeadlineStore;
    filterCtl: TasksFilterStateController;
    dataCtl: TasksDataController;
    viewCtl: TasksViewStateController;
    cursorIdx: number;
    selectedIds: Set<string>;
    load: () => Promise<void> | void;
    onOpenDetail: (t: Task) => void;
    onOpenContext: (t: Task, x: number, y: number) => void;
  };

  let {
    triage,
    deadline,
    filterCtl,
    dataCtl,
    viewCtl,
    cursorIdx,
    selectedIds = $bindable(),
    load,
    onOpenDetail,
    onOpenContext
  }: Props = $props();
</script>

<div class="max-w-3xl">
  <div class="flex items-baseline gap-3 mb-4">
    <p class="text-sm text-dim flex-1">
      Untriaged tasks. Decide for each: schedule, prioritize, drop, or snooze.
    </p>
    {#if $triage.busy}
      <button
        onclick={() => triage.cancel()}
        class="px-3 py-1.5 text-xs bg-surface0 text-warning rounded hover:bg-surface1 flex-shrink-0"
        title="Cancel the in-flight triage call"
      >✨ thinking… cancel</button>
    {:else}
      <button
        onclick={() => void triage.run()}
        disabled={filterCtl.filtered.length === 0}
        class="px-3 py-1.5 text-xs bg-surface1 text-secondary rounded hover:bg-surface2 disabled:opacity-50 flex-shrink-0"
        title="Ask AI to suggest priority + schedule for each untriaged task"
      >✨ AI triage</button>
    {/if}
    {#if $deadline.busy}
      <button
        onclick={() => deadline.cancel()}
        class="px-3 py-1.5 text-xs bg-surface0 text-warning rounded hover:bg-surface1 flex-shrink-0"
        title="Cancel the in-flight deadline scan"
      >✨ thinking… cancel</button>
    {:else}
      <button
        onclick={() => void deadline.run()}
        class="px-3 py-1.5 text-xs bg-surface1 text-secondary rounded hover:bg-surface2 disabled:opacity-50 flex-shrink-0"
        title="Scan all open tasks without a due date — propose ones whose title implies a clear deadline"
      >✨ Detect deadlines</button>
    {/if}
  </div>

  {#if $deadline.proposals.length > 0}
    <!-- Deadline proposals — operates across ALL open tasks without a
         due_date, not just inbox. Server already filtered blanks, so
         every row is a confident suggestion. Apply patches dueDate;
         skip just dismisses. -->
    <div class="mb-5 p-3 bg-surface0 border border-warning rounded">
      <div class="flex items-center mb-2">
        <div class="text-xs uppercase tracking-wider text-warning font-semibold flex-1">Detected deadlines ({$deadline.proposals.length})</div>
        <button
          onclick={() => deadline.discard()}
          class="text-[10px] text-dim hover:text-error"
          title="Drop all proposals without applying any"
        >discard</button>
      </div>
      <ul class="space-y-2">
        {#each $deadline.proposals as p (p.id)}
          {@const t = dataCtl.tasks.find((x) => x.id === p.id)}
          {#if t}
            <li class="flex items-start gap-2 text-xs">
              <div class="flex-1 min-w-0">
                <div class="text-text">{t.text}</div>
                <div class="text-dim mt-0.5">
                  due <span class="text-warning font-medium">{p.due_date}</span>
                  {#if p.rationale}<span class="italic"> — {p.rationale}</span>{/if}
                </div>
              </div>
              <button
                onclick={() => void deadline.apply(p, load)}
                disabled={$deadline.busy}
                class="px-2 py-0.5 bg-surface0 text-success rounded hover:bg-surface1"
              >accept</button>
              <button
                onclick={() => deadline.skip(p.id)}
                class="px-2 py-0.5 text-dim hover:text-text"
              >skip</button>
            </li>
          {/if}
        {/each}
      </ul>
    </div>
  {/if}

  {#if $triage.proposals.length > 0}
    <!-- AI suggestions panel. Each proposal has Accept / Skip;
         accepting applies the suggested priority + schedule to the
         matching task. -->
    <div class="mb-5 p-3 bg-surface1 border border-surface2 rounded">
      <div class="flex items-center mb-2">
        <div class="text-xs uppercase tracking-wider text-secondary font-semibold flex-1">AI suggestions ({$triage.proposals.length})</div>
        <button
          onclick={() => triage.discard()}
          class="text-[10px] text-dim hover:text-error"
          title="Drop all proposals without applying any"
        >discard</button>
      </div>
      <ul class="space-y-2">
        {#each $triage.proposals as p (p.id)}
          {@const t = dataCtl.tasks.find((x) => x.id === p.id)}
          {#if t}
            <li class="flex items-start gap-2 text-xs">
              <div class="flex-1 min-w-0">
                <div class="text-text">{t.text}</div>
                <div class="text-dim mt-0.5">
                  {p.priority === 0 ? 'drop' : `P${p.priority}`} · {p.schedule}
                  {#if p.rationale}<span class="italic"> — {p.rationale}</span>{/if}
                </div>
              </div>
              <button
                onclick={() => void triage.apply(p, load)}
                disabled={$triage.busy}
                class="px-2 py-0.5 bg-surface0 text-success rounded hover:bg-surface1"
              >accept</button>
              <button
                onclick={() => triage.skip(p.id)}
                class="px-2 py-0.5 text-dim hover:text-text"
              >skip</button>
            </li>
          {/if}
        {/each}
      </ul>
    </div>
  {/if}

  <div class="space-y-2">
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
</div>
