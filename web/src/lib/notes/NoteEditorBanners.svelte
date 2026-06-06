<script lang="ts">
  // Three save-related affordances rendered between the deadline
  // strip and the reading-progress bar on the notes editor:
  //
  //   1. draft-restored — persistent affordance while editing on
  //      top of a localStorage draft. Previously the user only saw
  //      a 3 s toast, then nothing — they couldn't tell "why are
  //      my changes here?" when revisiting a recovered note.
  //      Clears on next successful save.
  //
  //   2. conflict — the file on disk moved forward since we loaded
  //      it (412 from putNote's If-Match guard). Two explicit
  //      choices, never silent overwrite: Reload server version
  //      (discard local for the server's body), or Overwrite
  //      anyway (skip If-Match on the next save). Lives above the
  //      transient-failure banner so it can't be hidden behind it.
  //
  //   3. autosave-failing — goes sticky after the 2nd consecutive
  //      failure; earlier failures are surfaced via per-failure
  //      toast. The threshold avoids alarming on a one-off blip
  //      while still making prolonged outages obvious.
  //
  // The page hands every reactive datum + every callback so the
  // banners stay presentational. Renders nothing on a fresh,
  // conflict-free page.

  interface Props {
    draftRestored: boolean;
    conflictDetected: boolean;
    saveFailCount: number;
    lastSaveError: string;
    saving: boolean;
    onDismissDraftBadge: () => void;
    onReload: () => void;
    onOverwrite: () => void;
    onRetry: () => void;
  }

  const {
    draftRestored,
    conflictDetected,
    saveFailCount,
    lastSaveError,
    saving,
    onDismissDraftBadge,
    onReload,
    onOverwrite,
    onRetry
  }: Props = $props();
</script>

{#if draftRestored}
  <div class="px-3 py-1.5 text-xs flex items-center gap-2 bg-warning/15 border-b border-warning/30 text-text">
    <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 flex-shrink-0 text-warning" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
      <path d="M21 12.79A9 9 0 1 1 11.21 3a7 7 0 0 0 9.79 9.79z"/>
    </svg>
    <span class="flex-1">Editing a restored draft from this browser — saves on disk reflect what's typed here.</span>
    <button
      onclick={onDismissDraftBadge}
      class="px-1.5 py-0.5 text-[10px] text-dim hover:text-text"
      aria-label="dismiss"
    >dismiss</button>
  </div>
{/if}

{#if conflictDetected}
  <div
    role="status"
    class="px-3 sm:px-4 py-2 border-b border-warning bg-warning/10 text-text text-xs sm:text-sm flex items-center gap-3"
  >
    <span class="flex-shrink-0 text-warning" aria-hidden="true">⚠</span>
    <span class="flex-1 min-w-0">
      <strong class="font-semibold">Conflict</strong> — this note was changed elsewhere (another tab, TUI, or sync) since you opened it. Your local edits are safe; choose how to resolve.
    </span>
    <button
      type="button"
      onclick={onReload}
      class="px-2.5 py-1 rounded bg-surface0 hover:bg-surface1 text-text font-medium flex-shrink-0"
    >
      Reload server version
    </button>
    <button
      type="button"
      onclick={onOverwrite}
      disabled={saving}
      class="px-2.5 py-1 rounded bg-warning/30 hover:bg-warning/40 text-text font-medium flex-shrink-0 disabled:opacity-50"
    >
      {saving ? 'overwriting…' : 'Overwrite anyway'}
    </button>
  </div>
{/if}

{#if saveFailCount >= 2 && !conflictDetected}
  <div
    role="status"
    class="px-3 sm:px-4 py-2 border-b border-error bg-surface0 text-error text-xs sm:text-sm flex items-center gap-3"
  >
    <span class="flex-shrink-0" aria-hidden="true">⚠</span>
    <span class="flex-1 min-w-0">
      <strong class="font-semibold">Autosave failing</strong> ({saveFailCount} attempt{saveFailCount === 1 ? '' : 's'})
      {#if lastSaveError}<span class="text-error/80"> — {lastSaveError}</span>{/if}.
      Your edits are saved locally and will sync when the server is reachable.
    </span>
    <button
      type="button"
      onclick={onRetry}
      disabled={saving}
      class="px-2.5 py-1 rounded bg-surface0 hover:bg-surface1 text-error font-medium flex-shrink-0 disabled:opacity-50"
    >
      {saving ? 'retrying…' : 'retry now'}
    </button>
  </div>
{/if}
