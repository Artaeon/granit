<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Note } from '$lib/api';
  import { onWsEvent } from '$lib/ws';

  let daily = $state<Note | null>(null);
  let loading = $state(false);

  async function load() {
    loading = true;
    try {
      daily = await api.daily('today');
    } finally {
      loading = false;
    }
  }
  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' && daily && ev.path === daily.path) load();
    });
  });

  let preview = $derived.by(() => {
    if (!daily?.body) return '';
    const lines = daily.body.split('\n').filter((l) => l.trim() && !l.startsWith('#')).slice(0, 3);
    return lines.join(' · ');
  });
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-4 flex flex-col">
  <div class="flex items-baseline justify-between mb-2">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Daily note</h2>
    {#if daily}
      <a href="/notes/{encodeURIComponent(daily.path)}" class="text-xs text-secondary hover:underline">edit →</a>
    {/if}
  </div>
  {#if loading && !daily}
    <div class="text-sm text-dim">loading…</div>
  {:else if daily}
    <a href="/notes/{encodeURIComponent(daily.path)}" class="block hover:opacity-90">
      <div class="text-base font-medium text-text">{daily.title}</div>
      <div class="text-xs text-dim font-mono mt-0.5">{daily.path}</div>
      {#if preview}
        <p class="text-sm text-subtext mt-2 line-clamp-2">{preview}</p>
      {/if}
    </a>
  {/if}
</section>
