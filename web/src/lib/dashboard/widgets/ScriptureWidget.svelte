<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Scripture } from '$lib/api';

  // Verse-of-the-day card on the dashboard. Server-side rotation is
  // deterministic by date so a refresh shows the same verse all day —
  // we don't refetch unless the user clicks through to /scripture.

  let verse = $state<Scripture | null>(null);

  onMount(async () => {
    try {
      verse = await api.todayScripture();
    } catch {
      verse = null;
    }
  });
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-4">
  <div class="flex items-baseline justify-between mb-2">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Today's verse</h2>
    <a href="/scripture" class="text-xs text-secondary hover:underline">all →</a>
  </div>
  {#if verse}
    <a href="/scripture" class="block group">
      <blockquote class="text-sm text-text leading-relaxed font-serif italic group-hover:text-primary">
        "{verse.text}"
      </blockquote>
      {#if verse.source}
        <cite class="text-xs text-subtext mt-2 block not-italic">— {verse.source}</cite>
      {/if}
    </a>
  {:else}
    <div class="text-sm text-dim italic">No verse loaded.</div>
  {/if}
</section>
