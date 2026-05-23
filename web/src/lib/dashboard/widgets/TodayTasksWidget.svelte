<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { api, todayISO, type Task } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { createCoalescedReload } from '$lib/util/coalesce';
  import TaskRow from '$lib/components/TaskRow.svelte';

  // Today's tasks — three buckets: overdue (urgent, error tone),
  // due-today (the centerpiece), and a small no-date queue so the
  // user can pull from their backlog without leaving the dashboard.
  // The header shows live counts so the user reads "what's the
  // shape of today" before scanning rows. Reload is coalesced
  // (600ms trailing) so the editor's autosave bursts don't
  // re-fetch the task list per keystroke.

  let tasks = $state<Task[]>([]);
  let loading = $state(false);
  let loadError = $state(false);

  async function load() {
    loading = true;
    loadError = false;
    try {
      const list = await api.listTasks({ status: 'open' });
      tasks = list.tasks;
    } catch (e) {
      // 401 / network failure: render empty state with a small
      // retry hint, not a stuck spinner.
      console.error('today-tasks widget: load failed', e);
      loadError = true;
    } finally {
      loading = false;
    }
  }

  const reload = createCoalescedReload(load, 600);
  onMount(() => {
    void load();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed' || ev.type === 'task.changed') {
        reload.trigger();
      }
    });
  });
  onDestroy(reload.cancel);

  const today = todayISO();
  let overdue = $derived(
    tasks
      .filter((t) => t.dueDate && t.dueDate < today)
      .sort((a, b) => (a.dueDate ?? '').localeCompare(b.dueDate ?? ''))
  );
  let dueToday = $derived(
    tasks
      .filter((t) => t.dueDate === today || (t.scheduledStart && t.scheduledStart.slice(0, 10) === today))
      .sort((a, b) => {
        // Scheduled-by-time first, then by priority. Time-blocked
        // tasks read more naturally in chronological order.
        const at = a.scheduledStart ?? '';
        const bt = b.scheduledStart ?? '';
        if (at && bt) return at.localeCompare(bt);
        if (at) return -1;
        if (bt) return 1;
        return (a.priority || 99) - (b.priority || 99);
      })
  );
  // No-date queue — surfaces a sample of unscheduled work so the
  // dashboard can act as a "what should I pick up next" launcher.
  // Capped at 5 to keep the widget tight on tall content.
  let noDate = $derived(
    tasks.filter((t) => !t.dueDate && !t.scheduledStart).slice(0, 5)
  );

  let totalOpen = $derived(tasks.length);
  let leftToday = $derived(overdue.length + dueToday.length);
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-3">
  <div class="flex items-baseline justify-between mb-3 gap-2">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Tasks</h2>
    {#if !loading && totalOpen > 0}
      <span class="text-[11px] text-dim font-mono tabular-nums">
        {leftToday > 0 ? `${leftToday} today · ` : ''}{totalOpen} open
      </span>
    {/if}
    <span class="flex-1"></span>
    <a href="/tasks" class="text-xs text-secondary hover:underline">all →</a>
  </div>

  {#if loading && tasks.length === 0}
    <!-- Skeleton — three placeholder rows so the layout stays put
         while the first load resolves. Removed the "loading…" text;
         silent shimmer reads as less noise. -->
    <div class="space-y-2">
      {#each [0, 1, 2] as i (i)}
        <div class="flex items-baseline gap-2">
          <span class="w-4 h-4 bg-surface1 rounded animate-pulse"></span>
          <span class="flex-1 h-4 bg-surface1 rounded animate-pulse {i === 1 ? 'w-3/4' : ''}"></span>
        </div>
      {/each}
    </div>
  {:else if loadError && tasks.length === 0}
    <p class="text-sm text-dim italic">Couldn't load tasks. <button class="underline hover:text-text" onclick={() => void load()}>retry</button></p>
  {:else}
    <!-- Three sections stack on narrow widget cells and flow into
         columns (auto-fit, min 240px) when the cell has horizontal
         room. The container is the widget's .widget-cell, set in
         routes/+page.svelte. -->
    <div class="task-sections">
      {#if overdue.length > 0}
        <section>
          <h3 class="text-[11px] uppercase tracking-wider text-error mb-1.5 font-semibold">
            Overdue · <span class="tabular-nums">{overdue.length}</span>
          </h3>
          <div class="space-y-px">
            {#each overdue.slice(0, 6) as t (t.id)}
              <TaskRow task={t} onChanged={load} />
            {/each}
            {#if overdue.length > 6}
              <a href="/tasks?view=overdue" class="block text-[11px] text-dim hover:text-error pl-6 mt-1">+{overdue.length - 6} more overdue</a>
            {/if}
          </div>
        </section>
      {/if}

      <section>
        <h3 class="text-[11px] uppercase tracking-wider text-dim mb-1.5">
          Today · <span class="tabular-nums">{dueToday.length}</span>
        </h3>
        {#if dueToday.length === 0}
          <p class="text-sm text-dim italic">
            {totalOpen === 0 ? 'inbox zero — nothing open at all' : 'nothing due today'}
          </p>
        {:else}
          <div class="space-y-px">
            {#each dueToday as t (t.id)}
              <TaskRow task={t} onChanged={load} />
            {/each}
          </div>
        {/if}
      </section>

      {#if noDate.length > 0 && totalOpen > 0}
        <section>
          <h3 class="text-[11px] uppercase tracking-wider text-dim mb-1.5">
            No date · top <span class="tabular-nums">{noDate.length}</span>
          </h3>
          <div class="space-y-px">
            {#each noDate as t (t.id)}
              <TaskRow task={t} onChanged={load} />
            {/each}
          </div>
        </section>
      {/if}
    </div>
  {/if}
</section>

<style>
  /* Vertical stack by default; auto-fit grid once the widget cell
     is wide enough that ≥2 columns fit at min 240px each. The
     min-width avoids the awkward "barely-2-column" state where rows
     wrap within each column. */
  .task-sections > section + section { margin-top: 0.75rem; }
  @container (min-width: 640px) {
    .task-sections {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
      gap: 1rem 1.25rem;
      align-items: start;
    }
    .task-sections > section + section { margin-top: 0; }
  }
</style>
