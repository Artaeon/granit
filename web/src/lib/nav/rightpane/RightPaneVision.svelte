<script lang="ts">
  // RightPaneVision — displays the pinned vision doc from the multi-doc
  // visions catalogue in read-only mode. Matches the KurzvisionWidget
  // pick logic (first pinned doc with non-empty content; otherwise
  // first pinned at all so the empty-state CTA still points the user
  // at the right tab).
  //
  // Footer link "Edit →" targets /vision?tab=<key> so the right pane
  // hands off to the full /vision page with the correct doc selected.

  import { onMount } from 'svelte';
  import { api, type VisionsStore, type VisionDoc } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';

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

  let pinned = $derived.by<VisionDoc | null>(() => {
    if (!store) return null;
    const withContent = store.docs.find((d) => d.pinned && (d.content?.trim() ?? '') !== '');
    if (withContent) return withContent;
    return store.docs.find((d) => d.pinned) ?? null;
  });

  let editHref = $derived.by(() => {
    if (!pinned) return '/vision';
    return `/vision?tab=${encodeURIComponent(pinned.key)}`;
  });
</script>

<div class="flex flex-col h-full text-sm">
  {#if loading}
    <div class="p-3 space-y-2">
      <div class="h-3 w-1/2 bg-surface1 rounded animate-pulse"></div>
      <div class="h-3 w-3/4 bg-surface1 rounded animate-pulse"></div>
      <div class="h-3 w-2/3 bg-surface1 rounded animate-pulse"></div>
    </div>
  {:else if error}
    <p class="p-3 text-dim italic text-xs">Couldn't load the vision.</p>
  {:else if pinned && (pinned.content?.trim() ?? '') !== ''}
    <header class="border-b border-surface1 px-3 py-2 flex-shrink-0">
      <h3 class="text-xs uppercase tracking-wider text-dim font-medium">{pinned.label}</h3>
    </header>
    <div class="flex-1 overflow-y-auto px-3 py-3 vision-body">
      <MarkdownRenderer body={pinned.content ?? ''} />
    </div>
    <footer class="border-t border-surface1 px-3 py-1.5 flex-shrink-0">
      <a href={editHref} class="text-xs text-secondary hover:underline">Edit →</a>
    </footer>
  {:else}
    <div class="p-4 space-y-2">
      <p class="text-xs text-dim italic">No pinned vision yet.</p>
      <a href="/vision" class="text-xs text-secondary hover:underline">Open Vision →</a>
    </div>
  {/if}
</div>

<style>
  /* Slim text inside the pane — the vision page tunes typography for
     a 720px column; here we have ~360px and want a compact read. */
  .vision-body :global(p) {
    font-size: 0.8125rem;
    line-height: 1.5;
    margin: 0 0 0.5rem 0;
  }
  .vision-body :global(h1),
  .vision-body :global(h2),
  .vision-body :global(h3) {
    font-size: 0.875rem;
    margin: 0.5rem 0 0.25rem 0;
  }
  .vision-body :global(ul),
  .vision-body :global(ol) {
    font-size: 0.8125rem;
    margin: 0 0 0.5rem 1rem;
  }
</style>
