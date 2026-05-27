<script lang="ts">
  import { onMount } from 'svelte';
  import type { Task } from '$lib/api';
  import TaskCard from './TaskCard.svelte';
  import { applyNextPriority, toggleDoneOf } from './taskActions';
  import { makeKanbanKeyHandler, type KanbanCol } from './useKanbanKeyboard';

  let {
    tasks,
    onChanged,
    selectedIds = $bindable(new Set<string>()),
    onOpenDetail,
    onContextMenu
  }: {
    tasks: Task[];
    onChanged: () => void | Promise<void>;
    selectedIds?: Set<string>;
    onOpenDetail?: (t: Task) => void;
    onContextMenu?: (t: Task, x: number, y: number) => void;
  } = $props();

  // Six columns mirroring TUI's TriageState. Order matches the user's
  // mental model of triage flow: anything new lands in inbox; the user
  // routes it to triaged (will do), scheduled (committed time), done,
  // dropped (deliberately not doing), or snoozed (later).
  const cols = [
    { key: 'inbox', label: 'Inbox', tone: 'subtext', help: 'untriaged' },
    { key: 'triaged', label: 'Triaged', tone: 'info', help: 'in-flight' },
    { key: 'scheduled', label: 'Scheduled', tone: 'primary', help: 'has a time' },
    { key: 'snoozed', label: 'Snoozed', tone: 'warning', help: 'waking soon' },
    { key: 'done', label: 'Done', tone: 'success', help: 'finished' },
    { key: 'dropped', label: 'Dropped', tone: 'dim', help: 'not doing' }
  ] as const;

  let grouped = $derived.by(() => {
    const m: Record<string, Task[]> = { inbox: [], triaged: [], scheduled: [], snoozed: [], done: [], dropped: [] };
    for (const t of tasks) {
      const k = t.triage || (t.done ? 'done' : 'inbox');
      (m[k] ??= []).push(t);
    }
    return m;
  });

  // ── Keyboard navigation (shared with Kanban / Eisenhower) ─────────
  let cursorIdx = $state<number>(-1);
  let navCols = $derived.by((): KanbanCol[] =>
    cols.map((c) => ({ key: c.key, ids: (grouped[c.key] ?? []).map((t) => t.id) }))
  );
  let cursorTaskId = $derived.by((): string | null => {
    if (cursorIdx < 0) return null;
    let n = cursorIdx;
    for (const c of navCols) {
      if (n < c.ids.length) return c.ids[n];
      n -= c.ids.length;
    }
    return null;
  });
  const taskById = (id: string) => tasks.find((t) => t.id === id);

  async function toggleDone(t: Task) {
    try {
      await toggleDoneOf(t);
      await onChanged?.();
    } catch {}
  }
  async function cyclePriority(t: Task) {
    try {
      await applyNextPriority(t);
      await onChanged?.();
    } catch {}
  }

  onMount(() => {
    const handler = makeKanbanKeyHandler({
      taskById: (id) => taskById(id) ?? null,
      getCursorIdx: () => cursorIdx,
      setCursorIdx: (n) => (cursorIdx = n),
      getColumns: () => navCols,
      selectedIds: () => selectedIds,
      setSelectedIds: (s) => (selectedIds = s),
      onOpenDetail: onOpenDetail ? (t) => onOpenDetail(t) : undefined,
      onToggleDone: (t) => void toggleDone(t),
      onCyclePriority: (t) => void cyclePriority(t)
    });
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  });
</script>

<div class="flex gap-3 overflow-x-auto pb-3 h-full">
  {#each cols as c}
    {@const list = grouped[c.key] ?? []}
    <section class="w-72 flex-shrink-0 flex flex-col bg-surface0 border border-surface1 rounded">
      <header class="px-3 py-2 border-b border-surface1 flex items-baseline gap-2">
        <h3 class="text-xs uppercase tracking-wider font-medium" style="color: var(--color-{c.tone})">{c.label}</h3>
        <span class="text-[10px] text-dim">{list.length}</span>
        <span class="ml-auto text-[10px] text-dim italic">{c.help}</span>
      </header>
      <div class="flex-1 overflow-y-auto p-2 space-y-2 min-h-[8rem]">
        {#each list as t (t.id)}
          <div
            data-kanban-task-id={t.id}
            class="rounded {cursorTaskId === t.id ? 'outline outline-1 outline-secondary outline-offset-1' : ''}"
          >
            <TaskCard
              task={t}
              compact
              onChanged={onChanged}
              bind:selectedIds
              onOpenDetail={onOpenDetail}
              onContextMenu={onContextMenu}
            />
          </div>
        {/each}
        {#if list.length === 0}
          <div class="text-[11px] text-dim/60 text-center py-6 border border-dashed border-surface1 rounded">drop tasks here</div>
        {/if}
      </div>
    </section>
  {/each}
</div>
