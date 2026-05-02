<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Task, type AppConfig } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import TaskCard from './TaskCard.svelte';

  // mode='config' means "render columns from KanbanColumns + KanbanColumnTags
  // in the user's config". The other three modes mirror the TUI's view modes:
  // priority / due / triage. `swimlane` slices each row by an additional
  // dimension — TUI taskmanager.go uses these same axes. Sort/WIP state
  // is per-column and persisted to localStorage, so picking a column
  // ordering in one session sticks across reloads.
  let {
    tasks,
    mode = $bindable('priority'),
    swimlane = $bindable<'none' | 'project' | 'tag' | 'priority'>('none'),
    onChanged,
    selectedIds = $bindable(new Set<string>()),
    onOpenDetail
  }: {
    tasks: Task[];
    mode?: 'priority' | 'due' | 'triage' | 'config';
    swimlane?: 'none' | 'project' | 'tag' | 'priority';
    onChanged?: () => void;
    selectedIds?: Set<string>;
    onOpenDetail?: (t: Task) => void;
  } = $props();

  type Column = {
    key: string;
    label: string;
    tasks: Task[];
    tone?: string;
    /** Optional WIP limit (config-driven mode only). Display as "N/limit"
     *  in the header; tint warning when exceeded. 0 / undefined = no limit. */
    wip?: number;
    /** Tag list this column "owns" — used by drag-drop to add the right
     *  tag to a task when it lands here. Config mode only. */
    routeTags?: string[];
  };

  // ---------------------------------------------------------------------------
  // Server config — fetched once on mount; if KanbanColumns is set we expose
  // a 'config' mode automatically. We don't block on the fetch; if it fails
  // we silently fall back to the legacy modes.
  // ---------------------------------------------------------------------------
  let config = $state<AppConfig | null>(null);
  onMount(() => {
    api.getConfig().then((c) => (config = c)).catch(() => {});
  });
  let hasConfigMode = $derived(!!(config?.kanban_columns && config.kanban_columns.length > 0));

  // ---------------------------------------------------------------------------
  // Per-column sort mode (priority / due / created / updated). Persisted to
  // localStorage so a user who sorts "Done" by completedAt asc keeps that
  // ordering across reloads.
  // ---------------------------------------------------------------------------
  type SortMode = 'priority' | 'due' | 'created' | 'updated';
  const SORT_KEY = 'granit.tasks.kanban.colSort';

  let colSort = $state<Record<string, SortMode>>({});
  onMount(() => {
    try {
      const raw = localStorage.getItem(SORT_KEY);
      if (raw) colSort = JSON.parse(raw);
    } catch {}
  });
  function setColSort(key: string, m: SortMode) {
    colSort = { ...colSort, [key]: m };
    try {
      localStorage.setItem(SORT_KEY, JSON.stringify(colSort));
    } catch {}
  }

  function applySort(list: Task[], m: SortMode): Task[] {
    const out = [...list];
    switch (m) {
      case 'due':
        out.sort((a, b) => {
          const da = a.dueDate || '';
          const db = b.dueDate || '';
          if (da === db) return b.priority - a.priority;
          if (!da) return 1;
          if (!db) return -1;
          return da.localeCompare(db);
        });
        break;
      case 'created':
        out.sort((a, b) => (b.createdAt || '').localeCompare(a.createdAt || ''));
        break;
      case 'updated':
        out.sort((a, b) => (b.updatedAt || '').localeCompare(a.updatedAt || ''));
        break;
      case 'priority':
      default:
        out.sort((a, b) => {
          // P1/P2/P3 ascending priority numbers but P0 ("none") goes last.
          const ap = a.priority === 0 ? 99 : a.priority;
          const bp = b.priority === 0 ? 99 : b.priority;
          if (ap !== bp) return ap - bp;
          return (a.dueDate || '￿').localeCompare(b.dueDate || '￿');
        });
    }
    return out;
  }

  // ---------------------------------------------------------------------------
  // Column derivation
  // ---------------------------------------------------------------------------

  function columnsForConfig(cfg: AppConfig): Column[] {
    const names = cfg.kanban_columns || [];
    const tagMap = cfg.kanban_column_tags || {};
    const wipMap = cfg.kanban_column_wip || {};

    // Parse tag mapping: "#doing,#wip" → ["doing", "wip"] (case-insensitive,
    // hashes optional in either the config value or the task's tag list).
    const colTags: Record<string, string[]> = {};
    for (const [name, raw] of Object.entries(tagMap)) {
      colTags[name] = String(raw)
        .split(',')
        .map((t) => t.trim().replace(/^#/, '').toLowerCase())
        .filter(Boolean);
    }

    const lastIdx = names.length - 1;
    const tones = ['text-info', 'text-warning', 'text-success', 'text-secondary', 'text-error'];
    const cols: Column[] = names.map((name, i) => ({
      key: name,
      label: name,
      tasks: [],
      tone: i === lastIdx ? 'text-success' : tones[i % tones.length],
      wip: wipMap[name] || 0,
      routeTags: colTags[name]
    }));

    for (const t of tasks) {
      // Done tasks always go to the rightmost column — that's the
      // semantic the TUI uses. Without this, ticking off a card
      // would leave it stranded in its source column.
      if (t.done) {
        cols[lastIdx].tasks.push(t);
        continue;
      }
      const taskTags = (t.tags || []).map((s) => s.toLowerCase());
      let placed = false;
      for (let i = 0; i < cols.length; i++) {
        if (i === lastIdx) continue; // Done column reserved for done tasks
        const tags = cols[i].routeTags || [];
        if (tags.some((tag) => taskTags.includes(tag))) {
          cols[i].tasks.push(t);
          placed = true;
          break;
        }
      }
      if (!placed) cols[0].tasks.push(t); // backlog default
    }
    return cols;
  }

  function columnsForMode(): Column[] {
    if (mode === 'config' && config && hasConfigMode) {
      return columnsForConfig(config);
    }
    if (mode === 'priority') {
      const buckets: Record<string, Task[]> = { '1': [], '2': [], '3': [], '0': [], done: [] };
      for (const t of tasks) {
        if (t.done) buckets.done.push(t);
        else buckets[String(t.priority)].push(t);
      }
      return [
        { key: '1', label: 'P1 — high', tasks: buckets['1'], tone: 'text-error' },
        { key: '2', label: 'P2 — medium', tasks: buckets['2'], tone: 'text-warning' },
        { key: '3', label: 'P3 — low', tasks: buckets['3'], tone: 'text-info' },
        { key: '0', label: 'no priority', tasks: buckets['0'], tone: 'text-subtext' },
        { key: 'done', label: 'done', tasks: buckets.done, tone: 'text-success' }
      ];
    }
    if (mode === 'due') {
      const today = new Date().toISOString().slice(0, 10);
      const buckets: Record<string, Task[]> = { overdue: [], today: [], upcoming: [], no_date: [], done: [] };
      for (const t of tasks) {
        if (t.done) buckets.done.push(t);
        else if (!t.dueDate && !t.scheduledStart) buckets.no_date.push(t);
        else {
          const d = t.dueDate ?? (t.scheduledStart ? t.scheduledStart.slice(0, 10) : '');
          if (d < today) buckets.overdue.push(t);
          else if (d === today) buckets.today.push(t);
          else buckets.upcoming.push(t);
        }
      }
      return [
        { key: 'overdue', label: 'overdue', tasks: buckets.overdue, tone: 'text-error' },
        { key: 'today', label: 'today', tasks: buckets.today, tone: 'text-warning' },
        { key: 'upcoming', label: 'upcoming', tasks: buckets.upcoming, tone: 'text-secondary' },
        { key: 'no_date', label: 'no date', tasks: buckets.no_date, tone: 'text-subtext' },
        { key: 'done', label: 'done', tasks: buckets.done, tone: 'text-success' }
      ];
    }
    // triage
    const buckets: Record<string, Task[]> = { inbox: [], triaged: [], scheduled: [], done: [], dropped: [], snoozed: [] };
    for (const t of tasks) {
      const k = t.triage || (t.done ? 'done' : 'inbox');
      (buckets[k] ??= []).push(t);
    }
    return [
      { key: 'inbox', label: 'inbox', tasks: buckets.inbox, tone: 'text-warning' },
      { key: 'triaged', label: 'triaged', tasks: buckets.triaged, tone: 'text-secondary' },
      { key: 'scheduled', label: 'scheduled', tasks: buckets.scheduled, tone: 'text-info' },
      { key: 'snoozed', label: 'snoozed', tasks: buckets.snoozed, tone: 'text-dim' },
      { key: 'done', label: 'done', tasks: buckets.done, tone: 'text-success' },
      { key: 'dropped', label: 'dropped', tasks: buckets.dropped, tone: 'text-dim' }
    ].filter((c) => c.tasks.length > 0 || c.key !== 'dropped');
  }

  // Effective column list. We re-sort each column according to the
  // user's per-column sort mode (default: priority).
  let columns = $derived.by((): Column[] => {
    const base = columnsForMode();
    return base.map((c) => ({
      ...c,
      tasks: applySort(c.tasks, colSort[c.key] ?? 'priority')
    }));
  });

  // ---------------------------------------------------------------------------
  // Swimlanes — slice the columns into rows by project / tag / priority.
  // Implementation: we keep `columns` as the canonical list; when swimlane
  // is active, we group each column's tasks into lanes keyed by the same
  // dimension, and the render walks lanes-major then columns-major.
  // ---------------------------------------------------------------------------
  type Lane = { key: string; label: string };
  let lanes = $derived.by((): Lane[] => {
    if (swimlane === 'none') return [{ key: '__all__', label: '' }];
    const seen = new Map<string, string>();
    if (swimlane === 'project') {
      for (const t of tasks) {
        const k = t.projectId || '(no project)';
        if (!seen.has(k)) seen.set(k, k);
      }
    } else if (swimlane === 'tag') {
      for (const t of tasks) {
        const ts = t.tags && t.tags.length ? t.tags : ['(untagged)'];
        for (const tag of ts) if (!seen.has(tag)) seen.set(tag, '#' + tag.replace('(untagged)', 'untagged'));
      }
    } else if (swimlane === 'priority') {
      seen.set('1', 'P1 — high');
      seen.set('2', 'P2 — medium');
      seen.set('3', 'P3 — low');
      seen.set('0', 'no priority');
    }
    const arr: Lane[] = [];
    for (const [k, label] of seen) arr.push({ key: k, label });
    arr.sort((a, b) => a.label.localeCompare(b.label));
    return arr;
  });

  function laneKeyOf(t: Task): string {
    if (swimlane === 'project') return t.projectId || '(no project)';
    if (swimlane === 'priority') return String(t.priority);
    if (swimlane === 'tag') {
      // A task can belong to multiple tag lanes — we render it under
      // EACH of its tags so the user can find it from any lane.
      // The render path handles this by calling tasksFor(col, lane)
      // which slices on (col.key, lane.key) per-task.
      return ''; // not used for tag lane
    }
    return '__all__';
  }

  function tasksFor(col: Column, lane: Lane): Task[] {
    if (swimlane === 'none') return col.tasks;
    if (swimlane === 'tag') {
      return col.tasks.filter((t) => {
        const tags = t.tags && t.tags.length ? t.tags : ['(untagged)'];
        return tags.includes(lane.key);
      });
    }
    return col.tasks.filter((t) => laneKeyOf(t) === lane.key);
  }

  // ---------------------------------------------------------------------------
  // Mobile collapse — same logic as before; just keyed by column.
  // ---------------------------------------------------------------------------
  let isDesktop = $state(false);
  let collapsed = $state<Record<string, boolean>>({});
  onMount(() => {
    const mq = window.matchMedia('(min-width: 768px)');
    isDesktop = mq.matches;
    const handler = (e: MediaQueryListEvent) => (isDesktop = e.matches);
    mq.addEventListener('change', handler);
    return () => mq.removeEventListener('change', handler);
  });

  $effect(() => {
    if (!isDesktop && Object.keys(collapsed).length === 0) {
      const next: Record<string, boolean> = {};
      for (const c of columns) {
        if (c.tasks.length === 0 || c.key === 'done' || c.key === 'dropped' || c.key === 'snoozed') {
          next[c.key] = true;
        }
      }
      collapsed = next;
    }
  });

  function toggle(key: string) {
    if (isDesktop) return;
    collapsed = { ...collapsed, [key]: !collapsed[key] };
  }

  // ---------------------------------------------------------------------------
  // Drag and drop. Existing behavior was non-existent on the kanban; we add
  // HTML5 dnd that maps drops to either:
  //   - mode='priority': set priority to the drop column's priority.
  //   - mode='due': overdue/today/upcoming → set dueDate accordingly; no_date → clear dueDate.
  //   - mode='triage': set triage state to the column key.
  //   - mode='config': add the column's first routing tag to the task text and
  //     remove tags belonging to other columns. Done column → done=true.
  // Multi-task: when dragging a selected task while >1 is selected, the same
  // patch is applied to all selected ids. This mirrors the TUI's bulk-move
  // (`m` on the visual selection set).
  // ---------------------------------------------------------------------------

  let draggingId = $state<string | null>(null);
  let dragOverCol = $state<string | null>(null);

  function onDragStart(t: Task, e: DragEvent) {
    draggingId = t.id;
    if (e.dataTransfer) {
      e.dataTransfer.effectAllowed = 'move';
      e.dataTransfer.setData('text/plain', t.id);
    }
  }
  function onDragEnd() {
    draggingId = null;
    dragOverCol = null;
  }
  function onDragOver(e: DragEvent, colKey: string) {
    e.preventDefault();
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
    dragOverCol = colKey;
  }
  function onDragLeave(colKey: string) {
    if (dragOverCol === colKey) dragOverCol = null;
  }

  async function onDrop(e: DragEvent, col: Column) {
    e.preventDefault();
    dragOverCol = null;
    const id = draggingId;
    draggingId = null;
    if (!id) return;
    // Multi-task move: if the dragged card is part of the bulk selection,
    // apply the patch to ALL selected ids.
    const ids = selectedIds.has(id) && selectedIds.size > 1 ? Array.from(selectedIds) : [id];
    const patch = patchForColumn(col);
    if (!patch) return;
    let ok = 0;
    let fail = 0;
    for (const tid of ids) {
      try {
        await api.patchTask(tid, patch);
        ok++;
      } catch {
        fail++;
      }
    }
    if (ids.length > 1) {
      if (fail === 0) toast.success(`moved ${ok} tasks`);
      else toast.warning(`moved ${ok}, ${fail} failed`);
    }
    onChanged?.();
  }

  function patchForColumn(col: Column): Parameters<typeof api.patchTask>[1] | null {
    if (mode === 'priority') {
      if (col.key === 'done') return { done: true };
      const p = Number(col.key);
      return { priority: p, done: false };
    }
    if (mode === 'due') {
      if (col.key === 'done') return { done: true };
      if (col.key === 'no_date') return { dueDate: '', done: false };
      if (col.key === 'today') return { dueDate: new Date().toISOString().slice(0, 10), done: false };
      if (col.key === 'overdue') {
        const d = new Date();
        d.setDate(d.getDate() - 1);
        return { dueDate: d.toISOString().slice(0, 10), done: false };
      }
      if (col.key === 'upcoming') {
        const d = new Date();
        d.setDate(d.getDate() + 7);
        return { dueDate: d.toISOString().slice(0, 10), done: false };
      }
      return null;
    }
    if (mode === 'triage') {
      type Tg = NonNullable<Task['triage']>;
      const t = col.key as Tg;
      const patch: Parameters<typeof api.patchTask>[1] = { triage: t };
      if (t === 'done') patch.done = true;
      else if (t !== 'snoozed') patch.done = false;
      return patch;
    }
    // Config mode: tag-based routing. Adding a tag means rewriting task.text;
    // we don't have a "tags" patch, so we append/remove '#tag' tokens.
    if (mode === 'config') {
      if (col.key === (config?.kanban_columns?.[config.kanban_columns.length - 1] ?? '')) {
        return { done: true };
      }
      const target = (col.routeTags ?? [])[0];
      if (!target) return { done: false };
      // The user can refresh once and the WS event will reload; we don't
      // round-trip the text rewrite here because the API doesn't expose a
      // tag patch and rewriting text safely (preserving inline markdown,
      // emoji priority markers, etc.) is outside scope. Surface a toast
      // explaining that config-mode column moves only flip done state.
      toast.info('config columns route by tag — edit the task to add/remove tags');
      return { done: false };
    }
    return null;
  }

  function fmtMin(total: number): string {
    if (total === 0) return '';
    if (total < 60) return total + 'm';
    const h = Math.floor(total / 60);
    const m = total % 60;
    return m === 0 ? h + 'h' : h + 'h ' + m + 'm';
  }
</script>

<!-- Top toolbar: swimlane selector + mode-aware hints -->
<div class="flex items-center gap-3 mb-3 text-xs text-dim flex-wrap">
  {#if hasConfigMode}
    <button
      class="px-2 py-1 rounded {mode === 'config' ? 'bg-primary/20 text-primary' : 'bg-surface0 text-subtext hover:bg-surface1'}"
      onclick={() => (mode = 'config')}
      title="use {config!.kanban_columns?.length ?? 0} columns from .granit.json"
    >config columns</button>
  {/if}
  <span>swimlanes</span>
  <select
    bind:value={swimlane}
    class="bg-surface0 border border-surface1 rounded px-2 py-1 text-text"
  >
    <option value="none">none</option>
    <option value="project">project</option>
    <option value="tag">tag</option>
    <option value="priority">priority</option>
  </select>
</div>

{#each lanes as lane (lane.key)}
  {#if swimlane !== 'none' && lane.label}
    <h3 class="text-xs uppercase tracking-wider text-subtext font-medium mt-4 mb-2 border-b border-surface1 pb-1">
      {lane.label}
    </h3>
  {/if}
  <div class="flex flex-col md:flex-row gap-3 md:overflow-x-auto md:pb-3" style="min-height: 40vh">
    {#each columns as col (col.key + ':' + lane.key)}
      {@const colTasks = tasksFor(col, lane)}
      {@const isCollapsed = !isDesktop && !!collapsed[col.key]}
      {@const total = colTasks.length}
      {@const limit = col.wip ?? 0}
      {@const overWip = limit > 0 && total > limit}
      {@const estTotal = colTasks.reduce((s, t) => s + (t.estimatedMinutes ?? 0), 0)}
      {@const isDropTarget = dragOverCol === col.key + ':' + lane.key}
      <div
        class="bg-mantle/50 border rounded flex flex-col md:flex-shrink-0 md:w-72
          {isDropTarget ? 'border-primary/60 bg-primary/5' : 'border-surface1'}
          {overWip ? 'ring-1 ring-error/40' : ''}"
        ondragover={(e) => onDragOver(e, col.key + ':' + lane.key)}
        ondragleave={() => onDragLeave(col.key + ':' + lane.key)}
        ondrop={(e) => onDrop(e, col)}
        role="region"
        aria-label="column {col.label}"
      >
        <button
          type="button"
          onclick={() => toggle(col.key)}
          class="flex items-baseline justify-between gap-2 p-3 md:p-2 md:cursor-default text-left"
        >
          <h3 class="text-xs uppercase tracking-wider font-medium {col.tone ?? 'text-dim'}">{col.label}</h3>
          <span class="text-xs text-dim flex items-center gap-2">
            {#if limit > 0}
              <span class={overWip ? 'text-error font-semibold' : 'text-dim'} title="WIP limit">
                {total} / {limit}
              </span>
            {:else}
              <span>{total}</span>
            {/if}
            {#if estTotal > 0}
              <span class="text-[10px] font-mono text-subtext" title="estimated total">{fmtMin(estTotal)}</span>
            {/if}
            <span class="md:hidden text-base text-subtext leading-none">{isCollapsed ? '▸' : '▾'}</span>
          </span>
        </button>

        <!-- Per-column sort mode picker (desktop only — mobile real estate is too tight). -->
        {#if !isCollapsed && lane.key === lanes[0].key}
          <div class="hidden md:flex items-center gap-1 px-2 pb-1 text-[10px] text-dim">
            <span>sort</span>
            <select
              value={colSort[col.key] ?? 'priority'}
              onchange={(e) => setColSort(col.key, (e.currentTarget as HTMLSelectElement).value as SortMode)}
              class="bg-surface0 border border-surface1 rounded px-1 py-0.5 text-text text-[10px]"
            >
              <option value="priority">priority</option>
              <option value="due">due</option>
              <option value="created">created</option>
              <option value="updated">updated</option>
            </select>
          </div>
        {/if}

        {#if !isCollapsed}
          <div class="px-2 pb-2 md:px-2 space-y-2 md:overflow-y-auto md:flex-1">
            {#if colTasks.length === 0}
              <div class="text-xs text-dim italic px-1 pb-2">empty</div>
            {:else}
              {#each colTasks as t (t.id)}
                <div
                  draggable="true"
                  ondragstart={(e) => onDragStart(t, e)}
                  ondragend={onDragEnd}
                  role="listitem"
                  class="cursor-move {draggingId === t.id ? 'opacity-50' : ''}"
                >
                  <TaskCard
                    task={t}
                    compact
                    onChanged={() => onChanged?.()}
                    bind:selectedIds
                    onOpenDetail={onOpenDetail}
                  />
                </div>
              {/each}
            {/if}
          </div>
        {/if}
      </div>
    {/each}
  </div>
{/each}