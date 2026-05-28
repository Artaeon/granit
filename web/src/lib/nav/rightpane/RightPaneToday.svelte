<script lang="ts">
  // RightPaneToday — today's daily note preview stacked above today's
  // top tasks. Two stacked panes with sticky section headers so the
  // user can scroll within each.
  //
  // Daily note: first 300 chars of body (stripped frontmatter +
  // headings) so the preview is a real glance, not a wall of markdown.
  // Footer link jumps to the editor at today's date.
  //
  // Today tasks: same filter the 'tasks' option uses (overdue / due
  // today / scheduled today / P1) but capped at 8 instead of 15 to
  // keep both halves visible. Re-uses TaskCard compact for parity.

  import { onMount, onDestroy } from 'svelte';
  import { api, todayISO, type Note, type Task } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { createCoalescedReload } from '$lib/util/coalesce';
  import { onLocalMidnight } from '$lib/util/midnightTick';
  import TaskCard from '$lib/tasks/TaskCard.svelte';

  let daily = $state<Note | null>(null);
  let tasks = $state<Task[]>([]);
  let loadingNote = $state(true);
  let loadingTasks = $state(true);
  let noteError = $state(false);
  let tasksError = $state(false);
  let today = $state(todayISO());

  async function loadNote() {
    try {
      daily = await api.daily('today');
      noteError = false;
    } catch {
      noteError = true;
    } finally {
      loadingNote = false;
    }
  }
  async function loadTasks() {
    try {
      const r = await api.listTasks({ status: 'open' });
      tasks = r.tasks ?? [];
      tasksError = false;
    } catch {
      tasksError = true;
    } finally {
      loadingTasks = false;
    }
  }
  const reloadNote = createCoalescedReload(loadNote, 600);
  const reloadTasks = createCoalescedReload(loadTasks, 600);

  let stopMidnight: (() => void) | null = null;
  onMount(() => {
    void loadNote();
    void loadTasks();
    stopMidnight = onLocalMidnight(() => {
      today = todayISO();
      void loadNote();
      void loadTasks();
    });
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' && daily && ev.path === daily.path) reloadNote.trigger();
      // ws.ts has task.changed only (no task.removed); a rescan
      // covers the rare delete-via-line-edit path.
      if (ev.type === 'task.changed' || ev.type === 'vault.rescanned') reloadTasks.trigger();
    });
  });
  onDestroy(() => {
    reloadNote.cancel();
    reloadTasks.cancel();
    if (stopMidnight) stopMidnight();
  });

  // Strip frontmatter + heading lines + collapse to a single 300-char
  // preview so the user sees the prose, not the structure.
  let preview = $derived.by(() => {
    if (!daily?.body) return '';
    let body = daily.body;
    if (body.startsWith('---')) {
      const end = body.indexOf('\n---', 3);
      if (end !== -1) body = body.slice(end + 4);
    }
    const text = body
      .split('\n')
      .map((l) => l.trim())
      .filter((l) => l && !l.startsWith('#') && !l.startsWith('---') && !l.startsWith('- [ ]') && !l.startsWith('- [x]'))
      .join(' ');
    if (text.length <= 300) return text;
    return text.slice(0, 297).trimEnd() + '…';
  });

  // Same predicate as RightPaneTasks — see notes there. Kept inline
  // so the file is self-contained (the today filter is small enough
  // that extracting it is more friction than DRY).
  function isToday(t: Task, todayStr: string): boolean {
    if (t.done) return false;
    if (t.dueDate && t.dueDate <= todayStr) return true;
    if (t.scheduledStart && t.scheduledStart.slice(0, 10) === todayStr) return true;
    if (t.priority === 1) return true;
    return false;
  }

  let visibleTasks = $derived.by(() => {
    const filtered = tasks.filter((t) => isToday(t, today));
    filtered.sort((a, b) => {
      const pa = a.priority || 99;
      const pb = b.priority || 99;
      if (pa !== pb) return pa - pb;
      const da = a.dueDate ?? '￿';
      const db = b.dueDate ?? '￿';
      return da.localeCompare(db);
    });
    return filtered.slice(0, 8);
  });

  // Compact "Wed May 28" header label. Built locally so we don't
  // re-render past midnight on a stale string — `today` re-evaluates
  // via the midnight hook and this derived re-runs.
  let dateLabel = $derived.by(() => {
    const d = new Date(today + 'T00:00:00');
    return d.toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' });
  });
</script>

<div class="flex flex-col h-full text-sm min-h-0">
  <header class="flex items-baseline gap-2 px-3 py-2 border-b border-surface1 flex-shrink-0">
    <h3 class="text-xs uppercase tracking-wider text-dim font-medium">Today</h3>
    <span class="text-[10px] text-dim tabular-nums">{dateLabel}</span>
  </header>

  <!-- Top section — daily note preview. 40% height so the tasks below
       have the bulk of the column. min-h-0 + overflow-y-auto so a
       large note doesn't push the tasks off-screen. -->
  <section class="border-b border-surface1 flex flex-col min-h-0 flex-shrink-0" style="height: 40%;">
    <div class="px-3 py-1 text-[10px] uppercase tracking-wider text-dim font-medium flex items-baseline gap-2">
      <span>Note</span>
      <a href="/notes/daily/{today}.md" class="text-[10px] text-secondary hover:underline ml-auto">open →</a>
    </div>
    <div class="flex-1 overflow-y-auto px-3 pb-2 min-h-0">
      {#if loadingNote}
        <div class="space-y-2">
          <div class="h-3 w-3/4 bg-surface1 rounded animate-pulse"></div>
          <div class="h-3 w-2/3 bg-surface1 rounded animate-pulse"></div>
          <div class="h-3 w-1/2 bg-surface1 rounded animate-pulse"></div>
        </div>
      {:else if noteError}
        <p class="text-dim italic text-xs">Couldn't load today's note.</p>
      {:else if preview === ''}
        <p class="text-dim italic text-xs">
          Today's note is empty —
          <a href="/notes/daily/{today}.md" class="text-secondary hover:underline">start writing →</a>
        </p>
      {:else}
        <p class="text-[13px] leading-relaxed text-text whitespace-pre-line">{preview}</p>
      {/if}
    </div>
  </section>

  <!-- Bottom section — today's top tasks. 60% height. Same swipe /
       press affordances as the dedicated 'tasks' option. -->
  <section class="flex flex-col flex-1 min-h-0">
    <div class="px-3 py-1 text-[10px] uppercase tracking-wider text-dim font-medium flex items-baseline gap-2 flex-shrink-0">
      <span>Tasks</span>
      <span class="text-[10px] tabular-nums text-dim">{visibleTasks.length}</span>
      <a href="/tasks" class="text-[10px] text-secondary hover:underline ml-auto">all →</a>
    </div>
    <div class="flex-1 overflow-y-auto px-2 pb-2 min-h-0">
      {#if loadingTasks}
        <div class="space-y-2 px-1">
          {#each [0, 1, 2] as i (i)}
            <div class="h-8 w-full bg-surface1 rounded animate-pulse"></div>
          {/each}
        </div>
      {:else if tasksError}
        <p class="px-2 text-dim italic text-xs">Couldn't load tasks.</p>
      {:else if visibleTasks.length === 0}
        <p class="px-2 text-dim italic text-xs">Nothing due today.</p>
      {:else}
        <ul class="space-y-1">
          {#each visibleTasks as t (t.id)}
            <li>
              <TaskCard
                task={t}
                compact={true}
                onChanged={() => reloadTasks.trigger()}
              />
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  </section>

  <footer class="border-t border-surface1 px-3 py-1.5 flex-shrink-0">
    <a href="/notes/daily/{today}.md" class="text-xs text-secondary hover:underline">Open daily note →</a>
  </footer>
</div>
