<script lang="ts">
  // RightPaneTasks — a slim today-focused list. Filters open tasks
  // down to a "today" bucket using the same heuristic the dashboard
  // TodayTasksWidget pattern would: due today (or earlier), or
  // scheduled to start today, or priority=P1 (the "do it today no
  // matter what" tier). Sorted priority-then-due so the most-urgent
  // surface first. Caps at 15 rows so the pane stays scannable in a
  // 360px column.
  //
  // Re-uses the canonical TaskCard with compact={true} so the
  // interactions (toggle done, open detail, swipe, snooze) match
  // /tasks. The page handles its own WS refresh via
  // `task.changed`/`task.removed`.

  import { onMount, onDestroy } from 'svelte';
  import { api, todayISO, type Task } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { createCoalescedReload } from '$lib/util/coalesce';
  import { onLocalMidnight } from '$lib/util/midnightTick';
  import TaskCard from '$lib/tasks/TaskCard.svelte';
  import RightPaneSection from './RightPaneSection.svelte';

  let tasks = $state<Task[]>([]);
  let loading = $state(true);
  let error = $state(false);
  let today = $state(todayISO());

  async function load() {
    try {
      // Pull every open task; filtering is cheap and the server's
      // shape doesn't carry a "today bucket" param. Same approach the
      // /tasks page uses for its Today view.
      const r = await api.listTasks({ status: 'open' });
      tasks = r.tasks ?? [];
      error = false;
    } catch {
      error = true;
    } finally {
      loading = false;
    }
  }
  const reload = createCoalescedReload(load, 600);

  let stopMidnight: (() => void) | null = null;
  onMount(() => {
    void load();
    stopMidnight = onLocalMidnight(() => {
      today = todayISO();
      void load();
    });
    return onWsEvent((ev) => {
      // ws.ts only emits task.changed (no task.removed) — a delete
      // surfaces as a vault rescan or note.changed on the parent
      // note, so we trigger on both signals for completeness.
      if (ev.type === 'task.changed') reload.trigger();
      if (ev.type === 'note.changed' || ev.type === 'vault.rescanned') reload.trigger();
    });
  });
  onDestroy(() => {
    reload.cancel();
    if (stopMidnight) stopMidnight();
  });

  // "Today" predicate. Mirrors the user's mental model: anything
  // overdue, anything due today, anything scheduled today, plus
  // anything P1 (must-do-today). The dueDate check uses string
  // compare since both values are YYYY-MM-DD and that ordering is
  // identical to chronological order.
  function isToday(t: Task, todayStr: string): boolean {
    if (t.done) return false;
    if (t.dueDate && t.dueDate <= todayStr) return true;
    if (t.scheduledStart && t.scheduledStart.slice(0, 10) === todayStr) return true;
    if (t.priority === 1) return true;
    return false;
  }

  let visible = $derived.by(() => {
    const filtered = tasks.filter((t) => isToday(t, today));
    // Priority ascending (1 = highest), then dueDate ascending. Tasks
    // without dueDate sort after dated ones inside the same priority.
    filtered.sort((a, b) => {
      const pa = a.priority || 99;
      const pb = b.priority || 99;
      if (pa !== pb) return pa - pb;
      const da = a.dueDate ?? '￿';
      const db = b.dueDate ?? '￿';
      return da.localeCompare(db);
    });
    return filtered.slice(0, 15);
  });

  // Inline-add stays in the daily note so the new task surfaces with
  // every other today-task automatically. Empty-string short-circuits
  // when the user dismisses without typing.
  let addingText = $state('');
  let adding = $state(false);
  let addError = $state(false);
  async function addTask() {
    const text = addingText.trim();
    if (!text || adding) return;
    adding = true;
    addError = false;
    try {
      const t = await api.createTask({
        notePath: `daily/${today}.md`,
        text,
        section: 'Tasks'
      });
      addingText = '';
      // Optimistic prepend so the user sees the result without
      // waiting for the WS round-trip.
      tasks = [t, ...tasks];
    } catch {
      addError = true;
    } finally {
      adding = false;
    }
  }
</script>

<RightPaneSection
  title="Today's tasks"
  badge={String(visible.length)}
  footerLabel="Open tasks page"
  footerHref="/tasks"
>
  {#snippet headerTrailing()}
    <form
      class="flex items-center gap-1"
      onsubmit={(e) => {
        e.preventDefault();
        void addTask();
      }}
    >
      <input
        type="text"
        bind:value={addingText}
        placeholder="+ add"
        aria-label="Add task to today"
        class="text-xs bg-surface0 border border-surface1 rounded px-2 py-0.5 w-24 focus:w-36 focus:outline-none focus:border-primary transition-all text-text placeholder:text-dim"
        disabled={adding}
      />
    </form>
  {/snippet}

  {#if loading}
    <div class="space-y-2 px-1">
      {#each [0, 1, 2, 3] as i (i)}
        <div class="h-8 w-full bg-surface1 rounded animate-pulse"></div>
      {/each}
    </div>
  {:else if error}
    <p class="px-2 py-2 text-dim italic text-xs">Couldn't load tasks.</p>
  {:else if visible.length === 0}
    <p class="px-2 py-2 text-dim italic text-xs">Nothing due today.</p>
  {:else}
    <ul class="space-y-1">
      {#each visible as t (t.id)}
        <li>
          <TaskCard
            task={t}
            compact={true}
            onChanged={() => reload.trigger()}
          />
        </li>
      {/each}
    </ul>
  {/if}
  {#if addError}
    <p class="px-2 pt-2 text-[11px] text-error">Couldn't add — try again.</p>
  {/if}
</RightPaneSection>
