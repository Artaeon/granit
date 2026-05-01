<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Note } from '$lib/api';
  import { onWsEvent } from '$lib/ws';

  let notes = $state<Note[]>([]);

  async function load() {
    const list = await api.listNotes({ limit: 8 });
    notes = list.notes;
  }
  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') load();
    });
  });

  function fmt(d: string): string {
    return new Date(d).toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
  }
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-4">
  <div class="flex items-baseline justify-between mb-2">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Recent</h2>
    <a href="/notes" class="text-xs text-secondary hover:underline">all →</a>
  </div>
  <ul class="space-y-1">
    {#each notes as n (n.path)}
      <li class="flex items-baseline gap-2">
        <a href="/notes/{encodeURIComponent(n.path)}" class="flex-1 text-sm text-text hover:text-primary truncate">{n.title}</a>
        <span class="text-[10px] text-dim flex-shrink-0">{fmt(n.modTime)}</span>
      </li>
    {/each}
  </ul>
</section>
