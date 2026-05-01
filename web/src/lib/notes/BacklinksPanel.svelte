<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';

  let { path, onNavigate }: { path: string; onNavigate?: (target: string) => void } = $props();

  interface LinksData {
    outgoing: string[];
    backlinks: { path: string; title: string }[];
  }

  let data = $state<LinksData | null>(null);
  let loading = $state(false);

  async function load() {
    if (!path) return;
    loading = true;
    try {
      data = await api.req<LinksData>(`/links/${encodeURI(path)}`);
    } catch (e) {
      data = { outgoing: [], backlinks: [] };
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void path;
    load();
  });
</script>

{#if loading && !data}
  <div class="text-xs text-dim italic px-2 py-1">loading…</div>
{:else if data}
  {#if data.backlinks.length > 0}
    <div class="space-y-px">
      {#each data.backlinks as bl}
        <a href="/notes/{encodeURIComponent(bl.path)}" class="block px-2 py-1 text-sm text-text hover:bg-surface0 rounded truncate">
          ← {bl.title}
        </a>
      {/each}
    </div>
  {:else}
    <div class="text-xs text-dim italic px-2 py-1">no backlinks</div>
  {/if}
  {#if data.outgoing.length > 0}
    <div class="text-xs uppercase tracking-wider text-dim mt-3 mb-1 px-2">Outgoing</div>
    <div class="space-y-px">
      {#each data.outgoing as link}
        <button
          type="button"
          onclick={() => onNavigate?.(link)}
          class="block w-full text-left px-2 py-1 text-sm text-secondary hover:bg-surface0 rounded truncate"
        >
          → {link}
        </button>
      {/each}
    </div>
  {/if}
{/if}
