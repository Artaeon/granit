<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type VisionsStore, type VisionDoc } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';

  // Kurzvision widget — surfaces the pinned vision doc (typically
  // "Kurzversion") on the today view. Reads the multi-doc visions
  // catalogue, picks the first doc with pinned=true, renders its
  // markdown. Empty state directs the user to /vision to set or
  // pin a doc; the widget is hideable like any other so users
  // without a pinned doc can collapse it via Customize.
  //
  // Refetches on WS state.changed for .granit/visions.json so that
  // editing the vision in /vision reflects on the dashboard without
  // a manual reload.

  interface Props {
    vaultPath?: string;
  }
  let { vaultPath: _vaultPath = '' }: Props = $props();

  let store = $state<VisionsStore | null>(null);
  let loading = $state(true);
  let error = $state(false);

  async function load() {
    try {
      store = await api.listVisions();
      error = false;
    } catch {
      error = true;
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    void load();
    return onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/visions.json') {
        void load();
      }
    });
  });

  // First pinned doc with non-empty content wins. If only an empty
  // doc is pinned, we still show that as the active surface (so
  // the empty-state CTA points the user at the right tab) — but
  // prefer one with actual text if both exist.
  let pinned = $derived.by<VisionDoc | null>(() => {
    if (!store) return null;
    const withContent = store.docs.find((d) => d.pinned && (d.content?.trim() ?? '') !== '');
    if (withContent) return withContent;
    return store.docs.find((d) => d.pinned) ?? null;
  });
</script>

<section class="bg-mantle border border-surface1 rounded-lg p-4">
  <header class="flex items-baseline gap-2 mb-2">
    <h2 class="text-xs text-dim font-semibold">
      {pinned?.label ?? 'Kurzvision'}
    </h2>
    <span class="flex-1"></span>
    <a href="/vision" class="text-xs text-secondary hover:underline">edit →</a>
  </header>

  {#if loading}
    <div class="space-y-2">
      <div class="h-3 bg-surface1 rounded animate-pulse w-3/4"></div>
      <div class="h-3 bg-surface1 rounded animate-pulse w-1/2"></div>
    </div>
  {:else if error}
    <p class="text-sm text-dim italic">Couldn't load the vision.</p>
  {:else if pinned && (pinned.content?.trim() ?? '') !== ''}
    <div class="kurzvision-body">
      <MarkdownRenderer body={pinned.content ?? ''} />
    </div>
  {:else}
    <p class="text-sm text-dim">
      No vision pinned to today.
      <a href="/vision" class="text-secondary hover:underline">Set a Kurzversion →</a>
    </p>
  {/if}
</section>

<style>
  /* Compact body styling — vision widget is supposed to read as a
     quiet anchor, not a wall of prose. Smaller leading than the
     full /vision page; users open /vision when they want to read
     the long form. */
  :global(.kurzvision-body) {
    font-size: 0.875rem;
    line-height: 1.55;
    color: var(--color-text);
  }
  :global(.kurzvision-body p) {
    margin: 0 0 0.5em 0;
  }
  :global(.kurzvision-body p:last-child) {
    margin-bottom: 0;
  }
</style>
