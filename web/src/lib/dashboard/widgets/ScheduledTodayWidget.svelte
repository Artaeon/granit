<script lang="ts">
  import { onMount } from 'svelte';
  import { api, todayISO, type Task } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { inlineMd } from '$lib/util/inlineMd';

  let tasks = $state<Task[]>([]);

  async function load() {
    const list = await api.listTasks({ status: 'open' });
    tasks = list.tasks;
  }
  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') load();
    });
  });

  const today = todayISO();
  let scheduled = $derived(
    tasks
      .filter((t) => t.scheduledStart && t.scheduledStart.slice(0, 10) === today)
      .sort((a, b) => (a.scheduledStart! < b.scheduledStart! ? -1 : 1))
  );
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-4">
  <div class="flex items-baseline justify-between mb-2">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Scheduled today</h2>
    <a href="/calendar" class="text-xs text-secondary hover:underline">cal →</a>
  </div>
  {#if scheduled.length === 0}
    <div class="text-sm text-dim italic">nothing scheduled today</div>
  {:else}
    <ul class="space-y-1.5">
      {#each scheduled as t (t.id)}
        {@const time = new Date(t.scheduledStart!).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
        <li class="flex items-baseline gap-3">
          <span class="text-xs font-mono text-info w-12 flex-shrink-0">{time}</span>
          <span class="flex-1 text-sm {t.done ? 'line-through text-dim' : 'text-text'} min-w-0 truncate">{@html inlineMd(t.text)}</span>
          {#if t.durationMinutes}<span class="text-[10px] text-dim flex-shrink-0">{t.durationMinutes}m</span>{/if}
        </li>
      {/each}
    </ul>
  {/if}
</section>
