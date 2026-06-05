<!--
  TasksFilterDrawer — the slide-out filter panel rendered inside
  TasksPane's right-side Drawer. Pure presentation: every dimension
  binds against the filter / data controllers the parent owns, so
  switching to a different filter store is one prop swap.

  The drawer renders its content in the DOM at all times (the
  parent Drawer keeps it mounted, just translated off-screen when
  closed) so the global `/` page-search shortcut can still focus
  the embedded search input via [data-page-search="1"].
-->
<script lang="ts">
  import { fmtEstBudget } from './tasksHelpers';
  import type { TasksFilterStateController } from './tasksFilterState.svelte';
  import type { TasksDataController } from './tasksData.svelte';

  type Props = {
    filterCtl: TasksFilterStateController;
    dataCtl: TasksDataController;
    onClose: () => void;
  };

  let { filterCtl, dataCtl, onClose }: Props = $props();
</script>

<div class="p-4 space-y-4">
  <!-- Title + close hint — the panel is the same surface the
       toolbar's Filter button opens. -->
  <div class="flex items-center justify-between border-b border-surface1 pb-2 -mt-1">
    <h2 class="text-sm font-semibold text-text">Filters</h2>
    <button
      type="button"
      onclick={onClose}
      aria-label="close filter panel"
      title="Close (Esc)"
      class="text-dim hover:text-text text-xs px-1.5 py-0.5"
    >esc</button>
  </div>

  <!-- Search. data-page-search="1" lets the global `/` shortcut
       focus this input even when the drawer is off-screen. -->
  <div>
    <label class="text-xs uppercase tracking-wider text-dim mb-1 block" for="tasks-search">Search</label>
    <input
      id="tasks-search"
      bind:value={filterCtl.q}
      placeholder="search task text or path…"
      data-page-search="1"
      class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
    />
  </div>

  <div>
    <div class="text-xs uppercase tracking-wider text-dim mb-2">Status</div>
    <div class="flex flex-col gap-1 text-sm">
      {#each ['open', 'done', 'all'] as v}
        <button
          class="text-left px-3 py-2 rounded {filterCtl.status === v ? 'bg-surface1 text-text' : 'text-subtext hover:bg-surface0'}"
          onclick={() => (filterCtl.status = v as typeof filterCtl.status)}
        >
          <span class="capitalize">{v}</span>
          {#if v === 'open'}<span class="text-xs text-dim ml-1">{dataCtl.countOpen}</span>{/if}
          {#if v === 'done'}<span class="text-xs text-dim ml-1">{dataCtl.countDone}</span>{/if}
        </button>
      {/each}
    </div>
  </div>

  <!-- Archived view toggle. Default hides archived (soft-deleted via
       TaskDetail's Archive button). "Show all" dims archived inline;
       "Archived only" is the restore drawer. -->
  <div>
    <div class="text-xs uppercase tracking-wider text-dim mb-2">Archived</div>
    <div class="flex flex-col gap-1 text-sm">
      <button
        class="text-left px-3 py-2 rounded {filterCtl.archivedMode === 'hide' ? 'bg-surface1 text-text' : 'text-subtext hover:bg-surface0'}"
        onclick={() => (filterCtl.archivedMode = 'hide')}
        title="Hide archived tasks (default)"
      >Hide</button>
      <button
        class="text-left px-3 py-2 rounded {filterCtl.archivedMode === 'show' ? 'bg-surface1 text-text' : 'text-subtext hover:bg-surface0'}"
        onclick={() => (filterCtl.archivedMode = 'show')}
        title="Show active + archived together"
      >Show all</button>
      <button
        class="text-left px-3 py-2 rounded {filterCtl.archivedMode === 'only' ? 'bg-surface1 text-warning' : 'text-warning hover:bg-surface0'}"
        onclick={() => (filterCtl.archivedMode = 'only')}
        title="Only archived — used for restore"
      >Archived only</button>
    </div>
  </div>

  <!-- Source filter — "all" surfaces every `- [ ]` line in the
       vault; "Task notes only" narrows to daily/Tasks/Projects/Daily
       notes. Flip when reading-note bullets pollute the view. -->
  <div>
    <div class="text-xs uppercase tracking-wider text-dim mb-2">Source</div>
    <div class="flex flex-col gap-1 text-sm">
      <button
        class="text-left px-3 py-2 rounded {filterCtl.sourceFilter === 'all' ? 'bg-surface1 text-text' : 'text-subtext hover:bg-surface0'}"
        onclick={() => (filterCtl.sourceFilter = 'all')}
        title="Show every - [ ] checkbox the parser found in the vault"
      >All notes</button>
      <button
        class="text-left px-3 py-2 rounded {filterCtl.sourceFilter === 'task-notes' ? 'bg-surface1 text-text' : 'text-subtext hover:bg-surface0'}"
        onclick={() => (filterCtl.sourceFilter = 'task-notes')}
        title="Daily notes, Tasks/, Projects/, Daily/ — skip bullets in arbitrary notes"
      >Task notes only</button>
    </div>
  </div>

  <div>
    <div class="text-xs uppercase tracking-wider text-dim mb-2">Priority</div>
    <div class="flex flex-col gap-1 text-sm">
      <button class="text-left px-3 py-2 rounded {filterCtl.priorityFilter === '' ? 'bg-surface1' : 'hover:bg-surface0'} text-subtext" onclick={() => (filterCtl.priorityFilter = '')}>any</button>
      <button class="text-left px-3 py-2 rounded {filterCtl.priorityFilter === 1 ? 'bg-surface0 text-error' : 'hover:bg-surface1 text-error'}" onclick={() => (filterCtl.priorityFilter = 1)}>P1 high</button>
      <button class="text-left px-3 py-2 rounded {filterCtl.priorityFilter === 2 ? 'bg-surface0 text-warning' : 'hover:bg-surface1 text-warning'}" onclick={() => (filterCtl.priorityFilter = 2)}>P2 medium</button>
      <button class="text-left px-3 py-2 rounded {filterCtl.priorityFilter === 3 ? 'bg-surface0 text-info' : 'hover:bg-surface1 text-info'}" onclick={() => (filterCtl.priorityFilter = 3)}>P3 low</button>
    </div>
  </div>

  {#if dataCtl.projects.length > 0}
    <div>
      <div class="text-xs uppercase tracking-wider text-dim mb-2">Projects</div>
      <div class="flex flex-col gap-1 text-sm">
        <button class="text-left px-3 py-2 rounded {filterCtl.projectFilter === '' ? 'bg-surface1' : 'hover:bg-surface0'} text-subtext" onclick={() => (filterCtl.projectFilter = '')}>all</button>
        {#each dataCtl.projects.slice(0, 12) as p}
          <button
            class="text-left px-3 py-2 rounded text-sm truncate {filterCtl.projectFilter === p.name ? 'bg-surface1 text-text' : 'text-subtext hover:bg-surface0'}"
            onclick={() => (filterCtl.projectFilter = filterCtl.projectFilter === p.name ? '' : p.name)}
            title={p.description}
          >{p.name}</button>
        {/each}
      </div>
    </div>
  {/if}

  {#if dataCtl.allTags.length > 0}
    <div>
      <div class="text-xs uppercase tracking-wider text-dim mb-2">Tags</div>
      <div class="flex flex-wrap gap-1">
        {#each dataCtl.allTags.slice(0, 24) as t}
          {@const active = filterCtl.tagFilters.includes(t)}
          <button
            class="text-xs px-2 py-1 rounded {active ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1'}"
            onclick={() => (filterCtl.tagFilters = active ? filterCtl.tagFilters.filter((x) => x !== t) : [...filterCtl.tagFilters, t])}
            title={active ? `Remove #${t} from filter` : `Add #${t} to filter (AND-combine with current)`}
          >#{t}</button>
        {/each}
      </div>
    </div>
  {/if}

  {#if dataCtl.goals.length > 0}
    <div>
      <div class="text-xs uppercase tracking-wider text-dim mb-2">Goals</div>
      <div class="flex flex-col gap-1 text-sm">
        <button
          class="text-left px-3 py-2 rounded {filterCtl.goalFilter === '' ? 'bg-surface1' : 'hover:bg-surface0'} text-subtext"
          onclick={() => (filterCtl.goalFilter = '')}
        >all</button>
        {#each dataCtl.goals.slice(0, 12) as g}
          <button
            class="text-left px-3 py-2 rounded text-sm truncate {filterCtl.goalFilter === g.id ? 'bg-surface0 text-info' : 'text-subtext hover:bg-surface1'}"
            onclick={() => (filterCtl.goalFilter = filterCtl.goalFilter === g.id ? '' : g.id)}
            title={g.description}
          >
            <span class="font-mono text-[10px] text-dim mr-1">{g.id}</span>
            {g.title}
          </button>
        {/each}
      </div>
    </div>
  {/if}

  {#if dataCtl.deadlines.length > 0}
    <div>
      <div class="text-xs uppercase tracking-wider text-dim mb-2">Deadlines</div>
      <div class="flex flex-col gap-1 text-sm">
        <button
          class="text-left px-3 py-2 rounded {filterCtl.deadlineFilter === '' ? 'bg-surface1' : 'hover:bg-surface0'} text-subtext"
          onclick={() => (filterCtl.deadlineFilter = '')}
        >all</button>
        {#each dataCtl.deadlines.slice(0, 12) as d}
          <button
            class="text-left px-3 py-2 rounded text-sm truncate {filterCtl.deadlineFilter === d.id ? 'bg-surface0 text-warning' : 'text-subtext hover:bg-surface1'}"
            onclick={() => (filterCtl.deadlineFilter = filterCtl.deadlineFilter === d.id ? '' : d.id)}
            title={d.description}
          >
            <span class="font-mono text-[10px] text-dim mr-1">{d.date}</span>
            {d.title}
          </button>
        {/each}
      </div>
    </div>
  {/if}

  <button
    onclick={() => filterCtl.clearAll()}
    class="w-full text-xs text-dim hover:text-text underline pt-2"
  >reset filters</button>

  <!-- Passive stats at the bottom of the panel. avgPriority /
       noEstCount / snoozed live here so the main chrome stays calm. -->
  <div class="border-t border-surface1 pt-3">
    <div class="text-xs uppercase tracking-wider text-dim mb-2">Stats</div>
    <div class="grid grid-cols-2 gap-1.5 text-xs">
      <div class="flex items-baseline justify-between px-2 py-1.5 bg-surface0 rounded font-mono tabular-nums">
        <span class="text-dim">open</span>
        <span class="text-text font-semibold">{dataCtl.stats.open}</span>
      </div>
      {#if dataCtl.stats.snoozed > 0}
        <div class="flex items-baseline justify-between px-2 py-1.5 bg-surface0 rounded font-mono tabular-nums" title="Currently snoozed">
          <span class="text-dim">snoozed</span>
          <span class="text-dim font-semibold">{dataCtl.stats.snoozed}</span>
        </div>
      {/if}
      {#if dataCtl.stats.doneToday > 0}
        <div class="flex items-baseline justify-between px-2 py-1.5 bg-surface0 rounded font-mono tabular-nums" title="Completed today">
          <span class="text-dim">done today</span>
          <span class="text-success font-semibold">{dataCtl.stats.doneToday}</span>
        </div>
      {/if}
      {#if dataCtl.stats.doneWeek > 0}
        <div class="flex items-baseline justify-between px-2 py-1.5 bg-surface0 rounded font-mono tabular-nums" title="Completed in the last 7 days — rolling weekly velocity">
          <span class="text-dim">done · 7d</span>
          <span class="text-success font-semibold">{dataCtl.stats.doneWeek}</span>
        </div>
      {/if}
      {#if dataCtl.stats.sumEstMin > 0}
        <div class="flex items-baseline justify-between px-2 py-1.5 bg-surface0 rounded font-mono tabular-nums" title="Total estimated minutes across open non-snoozed tasks. 8h = one day-block.">
          <span class="text-dim">Σ est</span>
          <span class="text-secondary font-semibold">{fmtEstBudget(dataCtl.stats.sumEstMin)}</span>
        </div>
      {/if}
      {#if dataCtl.stats.noEstCount > 0}
        <div class="flex items-baseline justify-between px-2 py-1.5 bg-surface0 rounded font-mono tabular-nums" title="Open tasks with no time estimate — add est:30m to make Σ accurate">
          <span class="text-dim">no estimate</span>
          <span class="text-dim font-semibold">{dataCtl.stats.noEstCount}</span>
        </div>
      {/if}
      {#if dataCtl.stats.avgPriority > 0}
        {@const ap = dataCtl.stats.avgPriority}
        {@const apTone = ap < 1.5 ? 'text-error' : ap < 2.5 ? 'text-warning' : 'text-info'}
        <div class="flex items-baseline justify-between px-2 py-1.5 bg-surface0 rounded font-mono tabular-nums" title="Average priority across prioritised open tasks (1=high, 3=low)">
          <span class="text-dim">avg pri</span>
          <span class="{apTone} font-semibold">P{ap.toFixed(1)}</span>
        </div>
      {/if}
    </div>
  </div>
</div>
