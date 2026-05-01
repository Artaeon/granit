<script lang="ts">
  import { onMount } from 'svelte';
  import { api, todayISO, type Task } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import TaskRow from '$lib/components/TaskRow.svelte';

  let tasks = $state<Task[]>([]);
  let loading = $state(false);

  async function load() {
    loading = true;
    try {
      const list = await api.listTasks({ status: 'open' });
      tasks = list.tasks;
    } finally {
      loading = false;
    }
  }
  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') load();
    });
  });

  const today = todayISO();
  let overdue = $derived(tasks.filter((t) => t.dueDate && t.dueDate < today));
  let dueToday = $derived(tasks.filter((t) => t.dueDate === today));
  let noDate = $derived(tasks.filter((t) => !t.dueDate && !t.scheduledStart).slice(0, 6));
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-4">
  <div class="flex items-baseline justify-between mb-3">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Tasks</h2>
    <a href="/tasks" class="text-xs text-secondary hover:underline">all →</a>
  </div>
  {#if loading && tasks.length === 0}
    <div class="text-sm text-dim">loading…</div>
  {:else}
    {#if overdue.length > 0}
      <h3 class="text-[11px] uppercase tracking-wider text-error mb-1.5 mt-1">Overdue · {overdue.length}</h3>
      <div class="space-y-px mb-3">
        {#each overdue.slice(0, 6) as t (t.id)}
          <TaskRow task={t} onChanged={load} />
        {/each}
      </div>
    {/if}
    <h3 class="text-[11px] uppercase tracking-wider text-dim mb-1.5">Today · {dueToday.length}</h3>
    {#if dueToday.length === 0}
      <div class="text-sm text-dim italic mb-3">nothing due today</div>
    {:else}
      <div class="space-y-px mb-3">
        {#each dueToday as t (t.id)}
          <TaskRow task={t} onChanged={load} />
        {/each}
      </div>
    {/if}
    {#if noDate.length > 0}
      <h3 class="text-[11px] uppercase tracking-wider text-dim mb-1.5">No date · top {noDate.length}</h3>
      <div class="space-y-px">
        {#each noDate as t (t.id)}
          <TaskRow task={t} onChanged={load} />
        {/each}
      </div>
    {/if}
  {/if}
</section>
