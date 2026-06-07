<script lang="ts">
  // Bottom-of-page Archive section. Renders only when the user has
  // archived habits AND the "Show archived" toggle is on; otherwise
  // the page hides it entirely. Compact list — name + streak summary
  // + Restore button. Muted colours so it doesn't visually compete
  // with the active habits above.

  import type { HabitInfo } from '$lib/api';
  import type { ArchiveController } from '$lib/habits/habitsArchive.svelte';

  type Props = {
    habits: HabitInfo[];
    archive: ArchiveController;
  };

  let { habits, archive }: Props = $props();

  // Only the archived subset, alphabetised so the order is stable
  // across reloads (the API order follows streak which gets weird
  // when an archived habit's streak is frozen).
  const archived = $derived(
    [...habits.filter((h) => h.archived)].sort((a, b) => a.name.localeCompare(b.name))
  );
</script>

{#if archived.length > 0}
  <section class="mt-8 border-t border-surface1 pt-4">
    <header class="flex items-baseline gap-2 mb-3">
      <h2 class="text-xs uppercase tracking-wider text-dim font-medium">
        Archived
      </h2>
      <span class="text-[11px] text-dim">{archived.length}</span>
      <span class="flex-1"></span>
      <span class="text-[11px] text-dim italic">
        archived habits are hidden from every view by default
      </span>
    </header>

    <ul class="space-y-1.5">
      {#each archived as h (h.name)}
        <li
          class="flex items-center gap-3 px-3 py-2 bg-surface0/60 border border-surface1 rounded text-sm"
        >
          <div class="flex-1 min-w-0">
            <div class="text-text/80 truncate">{h.name}</div>
            <div class="text-[11px] text-dim flex flex-wrap gap-x-2">
              <span>frozen at {h.currentStreak}d streak</span>
              <span>longest {h.longestStreak}d</span>
              {#if h.category}
                <span class="text-secondary/70">{h.category}</span>
              {/if}
            </div>
          </div>
          <button
            type="button"
            onclick={() => archive.restore(h.name)}
            disabled={archive.busy === h.name}
            class="px-2 py-1 text-[11px] uppercase tracking-wider rounded border border-surface2 text-dim hover:text-text hover:border-primary disabled:opacity-50"
            title="restore — habit reappears in every view"
            aria-label={`restore ${h.name}`}
          >
            {archive.busy === h.name ? '…' : 'restore'}
          </button>
        </li>
      {/each}
    </ul>
  </section>
{/if}
