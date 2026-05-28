<script lang="ts">
  // RightPaneAI — Phase 1 launcher card.
  //
  // The full AIOverlay carries ~3400 LOC of layout assumptions that
  // are tightly coupled to its full-overlay sheet shape (visualViewport
  // hacks, pinned-mode width var, sabbath gating, etc.). Embedding it
  // inside a 280-640px column would either duplicate state (two
  // threads, two persona pickers) or require a substantial refactor of
  // the overlay to accept a "dock" prop. Both are out of scope for
  // Phase 1.
  //
  // So this content option is a discoverability card: it explains the
  // pane will host an embedded chat in Phase 2, and surfaces a single
  // button that triggers the existing global overlay (the same one the
  // sidebar Ask-AI button opens). Single source of truth for AI state
  // stays in $lib/stores/ai-overlay.

  import { openAIOverlay } from '$lib/stores/ai-overlay';
  import { sabbath } from '$lib/stores/sabbath';
</script>

<div class="flex flex-col h-full text-sm p-4 gap-4">
  <div class="space-y-2">
    <h3 class="text-xs uppercase tracking-wider text-dim font-medium">AI</h3>
    <p class="text-xs text-dim leading-relaxed">
      The right pane will host an embedded chat dock in a later phase.
      For now, use the global overlay — same thread, same persona, opens
      anywhere in the app.
    </p>
  </div>

  <button
    type="button"
    onclick={() => openAIOverlay()}
    disabled={$sabbath}
    class="w-full flex items-center gap-3 px-3 py-2.5 rounded transition-colors {$sabbath ? 'bg-surface0 text-dim cursor-not-allowed' : 'bg-primary text-on-primary hover:opacity-90 font-medium'}"
    title={$sabbath ? 'AI paused during Sabbath' : 'Open the AI overlay (⌘J)'}
  >
    <svg viewBox="0 0 24 24" class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
      <path d="M12 3v3M12 18v3M5.6 5.6l2.1 2.1M16.3 16.3l2.1 2.1M3 12h3M18 12h3M5.6 18.4l2.1-2.1M16.3 7.7l2.1-2.1"/>
      <circle cx="12" cy="12" r="3.5" fill="currentColor"/>
    </svg>
    <span class="flex-1 text-left">{$sabbath ? 'AI paused — Sabbath' : 'Open AI overlay'}</span>
    {#if !$sabbath}
      <kbd class="text-[10px] font-mono px-1.5 py-0.5 rounded border border-on-primary opacity-70">⌘J</kbd>
    {/if}
  </button>

  <div class="text-[11px] text-dim leading-relaxed border-t border-surface1 pt-3">
    <p class="mb-1 font-medium text-subtext">Why a launcher and not a dock?</p>
    <p>
      The overlay holds the live thread and persona state. Embedding it
      twice would split that state. Phase 2 will refactor the chat surface
      so the dock and the overlay share one source of truth.
    </p>
  </div>
</div>
