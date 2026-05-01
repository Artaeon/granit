<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Task } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { inlineMd } from '$lib/util/inlineMd';

  let tasks = $state<Task[]>([]);
  async function load() {
    const list = await api.listTasks({ triage: 'inbox', status: 'open' });
    tasks = list.tasks;
  }
  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') load();
    });
  });
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-4">
  <div class="flex items-baseline justify-between mb-2">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Inbox · {tasks.length}</h2>
    <a href="/tasks" class="text-xs text-secondary hover:underline">triage →</a>
  </div>
  {#if tasks.length === 0}
    <div class="text-sm text-success">inbox empty 🎉</div>
  {:else}
    <ul class="space-y-1">
      {#each tasks.slice(0, 6) as t (t.id)}
        <li class="text-sm text-text truncate">{@html inlineMd(t.text)}</li>
      {/each}
      {#if tasks.length > 6}
        <li class="text-xs text-dim">+{tasks.length - 6} more</li>
      {/if}
    </ul>
  {/if}
</section>
