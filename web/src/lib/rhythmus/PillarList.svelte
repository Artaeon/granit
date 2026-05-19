<script lang="ts">
  // The five pillar rows. One row per pillar with: icon, label,
  // current minimum text (from the rhythmus config in the user's
  // current mode), and a toggle. That's it.
  //
  // What this component refuses to render:
  //   - any pillar hidden in the current mode (the work pillar
  //     drops out in emergency mode by default, for example).
  //   - the next-action highlight as a *separate* row — the
  //     NextActionCard above already says what's next.
  //
  // Tone: ticked rows do NOT get a strikethrough or a "completed
  // 60%" bar. The minimum is binary: either today's minimum is
  // satisfied or it isn't. That's the whole pivot from streaks-as-
  // performance.

  import type { DayMode, PillarState } from './dayState';
  import { DEFAULT_PILLARS, PILLAR_ORDER, type PillarKey } from './pillars';
  import {
    rhythmusConfig,
    pillarLabel,
    pillarMinimumFor,
    pillarVisibleIn
  } from './minima';
  import { sabbath } from '$lib/stores/sabbath';

  type Props = {
    mode: DayMode;
    pillars: Record<PillarKey, PillarState>;
    onToggle: (key: PillarKey, next: boolean) => void;
  };

  let { mode, pillars, onToggle }: Props = $props();

  // Read the config reactively so a Rhythmus-tab edit reflects here
  // without a page reload. $rhythmusConfig is the store auto-subscribed.
  let cfg = $derived($rhythmusConfig);

  // On Sabbath the work pillar disappears — the day's rule is "no
  // work", not "less work" — but the other four stay because rest
  // doesn't mean skipping food, prayer, movement, or sleep prep.
  let visible = $derived(
    PILLAR_ORDER.filter((key) => {
      if ($sabbath && key === 'work') return false;
      return pillarVisibleIn(cfg, key, mode);
    })
  );
</script>

<section aria-label="Die fünf Säulen heute" class="bg-mantle border border-surface1 rounded-lg overflow-hidden">
  {#each visible as key (key)}
    {@const state = pillars[key]}
    {@const label = pillarLabel(cfg, key)}
    {@const icon = DEFAULT_PILLARS[key].icon}
    {@const minimum = pillarMinimumFor(cfg, key, mode)}
    <!-- Whole row is the tap target. The 24×24 checkbox alone is
         too small for thumb-driven mobile use (Apple HIG + Material
         both want 44×44 minimum); making the entire row a button
         widens the hit area without enlarging the visible chrome.
         Power-user keyboard parity stays via the button's tab focus
         and aria-pressed. -->
    <button
      type="button"
      onclick={() => onToggle(key, !state.done)}
      aria-pressed={state.done}
      aria-label="{label} — {state.done ? 'erledigt, klick zum Zurücksetzen' : 'als erledigt markieren'}"
      class="w-full flex items-center gap-3 px-4 py-3 text-left border-b border-surface1 last:border-b-0 hover:bg-surface0 transition-colors"
    >
      <span class="w-8 text-center text-lg" aria-hidden="true">{icon}</span>
      <div class="flex-1 min-w-0">
        <div class="text-sm text-text font-medium leading-tight">{label}</div>
        <div class="text-xs text-dim leading-snug">{minimum}</div>
      </div>
      <span
        class="w-6 h-6 rounded border flex items-center justify-center flex-shrink-0 transition-colors
          {state.done ? 'bg-success border-success' : 'border-surface2'}"
        aria-hidden="true"
      >
        {#if state.done}
          <svg viewBox="0 0 12 12" class="w-3 h-3 text-mantle">
            <path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z" />
          </svg>
        {/if}
      </span>
    </button>
  {/each}
</section>
