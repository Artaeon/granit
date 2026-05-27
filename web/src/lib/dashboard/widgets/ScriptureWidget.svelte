<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { api, type Scripture } from '$lib/api';
  import { onLocalMidnight } from '$lib/util/midnightTick';

  // Verse-of-the-day card on the dashboard. Server-side rotation is
  // deterministic by date so a refresh shows the same verse all day —
  // we don't refetch unless the user clicks through to /scripture.
  //
  // Three load states are distinct: pre-load (skeleton lines), loaded
  // (verse + source), and error (specific message + retry). The
  // previous silent-fail fallback ("No verse loaded.") gave the user
  // no idea whether their /scripture catalogue was empty or the
  // network just blipped.

  let verse = $state<Scripture | null>(null);
  let loaded = $state(false);
  let loadError = $state('');

  async function load() {
    loadError = '';
    try {
      verse = await api.todayScripture();
    } catch (e) {
      verse = null;
      loadError = e instanceof Error ? e.message : String(e);
    } finally {
      loaded = true;
    }
  }

  let stopMidnight: (() => void) | null = null;
  onMount(() => {
    void load();
    // Server rotates the verse deterministically by date — refetch
    // at local midnight so the dashboard shows the new day's verse
    // without a manual reload.
    stopMidnight = onLocalMidnight(() => { void load(); });
  });
  onDestroy(() => { if (stopMidnight) stopMidnight(); });
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-3">
  <div class="flex items-baseline justify-between mb-2">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Today's verse</h2>
    <a href="/scripture" class="text-xs text-secondary hover:underline">all →</a>
  </div>
  {#if !loaded}
    <!-- Skeleton lines mirror the verse + cite shape so the card
         doesn't reflow when the real content lands. -->
    <div class="space-y-1.5" aria-hidden="true">
      <div class="h-3 w-full rounded bg-surface1 animate-pulse"></div>
      <div class="h-3 w-5/6 rounded bg-surface1 animate-pulse"></div>
      <div class="h-2.5 w-1/3 rounded bg-surface1 animate-pulse mt-2"></div>
    </div>
  {:else if verse}
    <a href="/scripture" class="block group">
      <blockquote class="text-sm text-text leading-relaxed font-serif italic group-hover:text-primary">
        "{verse.text}"
      </blockquote>
      {#if verse.source}
        <cite class="text-xs text-subtext mt-2 block not-italic">— {verse.source}</cite>
      {/if}
    </a>
  {:else if loadError}
    <!-- Error state — specific enough that the user knows what to
         do. The retry button skips the toast machinery (this widget
         is one of many; a global toast would be louder than this
         single failure warrants). -->
    <div class="text-xs text-dim space-y-1.5">
      <p>Couldn't load today's verse.</p>
      <button
        type="button"
        onclick={load}
        class="text-secondary hover:underline"
      >Retry</button>
    </div>
  {:else}
    <!-- Genuine empty — no error, no verse. Means the user has no
         scriptures registered. Guide them to /scripture so the
         empty state is a path, not a dead end. -->
    <p class="text-xs text-dim">
      No verses added yet. <a href="/scripture" class="text-secondary hover:underline">Start your catalogue →</a>
    </p>
  {/if}
</section>
