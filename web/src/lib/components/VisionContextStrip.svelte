<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Vision } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { PILLAR_ORDER, type PillarKey } from '$lib/rhythmus/pillars';
  import { rhythmusConfig, pillarLabel } from '$lib/rhythmus/minima';

  // VisionContextStrip is the small "why" banner that sits at the
  // top of planning surfaces (/goals, /projects, /deadlines,
  // /ventures, /plans/week, /review) so the user's anchor stays in
  // peripheral vision while they drill into tactics. Renders nothing
  // when no vision is set — these pages already work without it;
  // the strip is additive context, not a dependency.
  //
  // Content rules (in order):
  //   1. If any identity is set, show the first non-empty one. Rotates
  //      across pillars over time as the user fills them in. One row,
  //      not five — a strip is for peripheral vision, not the full
  //      anchor; clicking opens /vision for the rest.
  //   2. Otherwise, if a legacy season_focus is set, show that. Keeps
  //      pre-pivot users covered until they migrate.
  //   3. Otherwise, if mission is set, show that.
  //   4. Else render nothing.
  //
  // Single fetch on mount + refresh via the existing vision WS path.

  let vision = $state<Vision | null>(null);
  let cfg = $derived($rhythmusConfig);

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

  type Display =
    | { kind: 'identity'; pillar: PillarKey; label: string; text: string }
    | { kind: 'season'; text: string }
    | { kind: 'mission'; text: string };

  function firstIdentity(v: Vision): { key: PillarKey; text: string } | null {
    if (!v.identities) return null;
    for (const key of PILLAR_ORDER) {
      const t = v.identities[key];
      if (typeof t === 'string' && t.trim()) return { key, text: t.trim() };
    }
    return null;
  }

  let display = $derived.by<Display | null>(() => {
    if (!vision) return null;
    const id = firstIdentity(vision);
    if (id) return { kind: 'identity', pillar: id.key, label: pillarLabel(cfg, id.key), text: id.text };
    if (vision.season_focus && vision.season_focus.trim()) {
      return { kind: 'season', text: vision.season_focus.trim() };
    }
    if (vision.mission && vision.mission.trim()) {
      return { kind: 'mission', text: vision.mission.trim() };
    }
    return null;
  });
</script>

{#if display}
  <a
    href="/vision"
    class="block mb-4 px-3 py-2 bg-surface0 border border-surface1 rounded text-xs hover:border-primary transition-colors group"
    title="Open vision"
  >
    <div class="flex items-baseline gap-3 flex-wrap">
      {#if display.kind === 'identity'}
        <span class="text-dim uppercase tracking-wider">{display.label}</span>
        <span class="text-text font-medium font-serif italic">{display.text}</span>
      {:else if display.kind === 'season'}
        <span class="text-dim uppercase tracking-wider">Season focus:</span>
        <span class="text-text font-medium">{display.text}</span>
        {#if vision?.season_day && vision.season_total}
          <span class="text-dim">· day {vision.season_day} of {vision.season_total}</span>
        {/if}
      {:else}
        <span class="text-dim uppercase tracking-wider">Mission:</span>
        <span class="text-text font-medium italic font-serif">{display.text}</span>
      {/if}
      <span class="flex-1"></span>
      <span class="text-dim group-hover:text-primary transition-colors">vision →</span>
    </div>
  </a>
{/if}
