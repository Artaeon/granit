<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Vision } from '$lib/api';
  import { onWsEvent } from '$lib/ws';

  // VisionContextStrip is the small "why" banner that sits at the
  // top of planning pages (/goals, /projects, /deadlines) so the
  // user's mission + season focus are visible while drilling into
  // tactics. Renders nothing when no vision is set — these pages
  // already work without it; the strip is additive context, not a
  // dependency.
  //
  // Single fetch on mount, refreshes via the existing vision WS
  // path. Cheap; the response is small and heavily cached at the
  // browser level when behind a real CDN.

  let vision = $state<Vision | null>(null);

  async function load() {
    try {
      vision = await api.getVision();
    } catch {
      vision = null;
    }
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/vision.json') load();
    });
  });

  // Render iff at least one of mission / season_focus is set. A
  // values-only vision wouldn't fit this strip's purpose (the strip
  // is about the "why" of what you're about to plan).
  let show = $derived(!!vision && (!!vision.mission || !!vision.season_focus));
</script>

{#if show && vision}
  <a
    href="/vision"
    class="block mb-4 px-3 py-2 bg-surface0/40 border border-surface1 rounded text-xs hover:border-primary/40 transition-colors group"
    title="Open vision"
  >
    <div class="flex items-baseline gap-3 flex-wrap">
      {#if vision.season_focus}
        <span class="text-dim uppercase tracking-wider">Season focus:</span>
        <span class="text-text font-medium">{vision.season_focus}</span>
        {#if vision.season_day && vision.season_total}
          <span class="text-dim">· day {vision.season_day} of {vision.season_total}</span>
        {/if}
      {:else if vision.mission}
        <span class="text-dim uppercase tracking-wider">Mission:</span>
        <span class="text-text font-medium italic font-serif">{vision.mission}</span>
      {/if}
      <span class="flex-1"></span>
      <span class="text-dim group-hover:text-primary transition-colors">vision →</span>
    </div>
  </a>
{/if}
