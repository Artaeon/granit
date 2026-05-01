<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import Skeleton from '$lib/components/Skeleton.svelte';

  let pinned = $state<{ path: string; title: string }[]>([]);
  let loading = $state(false);

  async function load() {
    loading = true;
    try {
      const r = await api.listPinned();
      pinned = r.pinned;
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
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-4">
  <div class="flex items-baseline justify-between mb-3">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">★ Pinned</h2>
    <a href="/notes" class="text-xs text-secondary hover:underline">browse →</a>
  </div>
  {#if loading && pinned.length === 0}
    <div class="space-y-2">
      <Skeleton class="h-4 w-3/4" />
      <Skeleton class="h-4 w-2/3" />
      <Skeleton class="h-4 w-1/2" />
    </div>
  {:else if pinned.length === 0}
    <div class="text-sm text-dim italic leading-relaxed">
      pin notes you need fast access to from any device — passwords, nextcloud info, account tokens, install commands.
      open any note → tap ★ in the header.
    </div>
  {:else}
    <ul class="space-y-1.5">
      {#each pinned as p (p.path)}
        <li>
          <a href="/notes/{encodeURIComponent(p.path)}" class="flex items-baseline gap-2 group">
            <span class="text-warning">★</span>
            <span class="flex-1 text-text group-hover:text-primary truncate">{p.title}</span>
          </a>
        </li>
      {/each}
    </ul>
  {/if}
</section>
