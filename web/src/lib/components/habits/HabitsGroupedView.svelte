<!--
  HabitsGroupedView — sixth view mode of the habits page.

  Reads a GroupingController and renders one section per category,
  in the order the controller emits (alphabetical, uncategorised
  last). Each section's body is the same card the list view renders;
  the route hands that down as a Snippet so we keep one card layout
  in one place and stay strictly additive.

  Empty buckets are filtered upstream by the controller, so there's
  never an empty header. The "Uncategorized" header gets a dimmer
  treatment than real categories so the user reads it as a hint to
  go assign categories, not as a peer bucket.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HabitInfo } from '$lib/api';
  import type { GroupingController } from '$lib/habits/habitsGrouping.svelte';

  type Props = {
    ctl: GroupingController;
    /** Per-habit card body. The route hands its existing card down
     *  so grouped-mode shares one card definition with the list view. */
    card: Snippet<[HabitInfo]>;
  };
  let { ctl, card }: Props = $props();
</script>

<div class="space-y-6">
  {#each ctl.grouped.order as key (key)}
    {@const bucket = ctl.grouped.byCategory.get(key) ?? []}
    {@const uncat = ctl.isUncategorized(key)}
    <section>
      <header class="flex items-baseline gap-2 mb-2 sticky top-0 bg-base/80 backdrop-blur-sm py-1 z-[1]">
        <h2
          class="text-xs uppercase tracking-wider font-medium {uncat ? 'text-dim' : 'text-primary'}"
        >{ctl.labelOf(key)}</h2>
        <span class="text-[11px] text-dim font-mono tabular-nums">
          {bucket.length}
        </span>
        <span class="flex-1 border-b border-surface1"></span>
      </header>
      <div class="space-y-3">
        {#each bucket as h (h.name)}
          {@render card(h)}
        {/each}
      </div>
    </section>
  {/each}
</div>
