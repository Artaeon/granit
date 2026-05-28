<script lang="ts">
  // Inline "this occurrence vs the entire series" picker for
  // recurring events. Replaces stacked native confirm() dialogs where
  // the safe path was OK and the destructive series-wide branch sat
  // behind Cancel — too easy to trigger by reflexively pressing Esc
  // to abort. Three explicit buttons; nothing fires until one is
  // clicked.
  //
  // The component is presentation-only. Callers handle the actual
  // skip / EXDATE / patch via the chosen scope. EventDetail uses it
  // for the recurring-delete confirm. The audit also flagged
  // +page.svelte's drag-move confirm() chains for replacement — that's
  // scoped to the next UX-polish stream.

  let {
    onChoose,
    onCancel,
    /** Optional event title rendered inline in the prompt. Plain
     *  text — wrapped in a font-medium span so it visually stands
     *  out without us having to inject HTML from callers. */
    eventTitle = '',
    /** Verb describing the action ('delete', 'change', 'move' …).
     *  Defaults to 'change' so the prompt reads naturally for any
     *  callsite. */
    action = 'change',
    /** Include the "All future occurrences" option. Off by default
     *  because the existing delete flow only offers this-vs-series.
     *  Callers that want the future split can opt in. */
    showFuture = false,
    /** Tone for the destructive Series button — most callers want
     *  the warning red surface; resize / move callers may prefer a
     *  softer neutral tone. */
    seriesTone = 'error',
    /** Caller-driven busy state — disables every button while a
     *  network call from the previous choice is still in-flight. */
    busy = false
  }: {
    onChoose: (scope: 'this' | 'future' | 'series') => void;
    onCancel: () => void;
    eventTitle?: string;
    action?: string;
    showFuture?: boolean;
    seriesTone?: 'error' | 'warning' | 'subtext';
    busy?: boolean;
  } = $props();

  let seriesClass = $derived(
    seriesTone === 'error'
      ? 'bg-surface1 text-error rounded hover:bg-surface2'
      : seriesTone === 'warning'
        ? 'bg-surface1 text-warning rounded hover:bg-surface2'
        : 'bg-surface1 text-subtext rounded hover:bg-surface2'
  );
</script>

<div class="pt-2 border-t border-surface1 space-y-2">
  {#if eventTitle}
    <div class="text-xs text-text">
      <span class="font-medium">"{eventTitle}"</span> is a recurring event. What do you want to {action}?
    </div>
  {/if}
  <div class="flex flex-wrap gap-2">
    <button
      type="button"
      onclick={() => onChoose('this')}
      disabled={busy}
      class="px-3 py-1.5 text-sm bg-surface1 text-warning rounded hover:bg-surface2 disabled:opacity-50"
      title="EXDATE just this date — every other instance stays"
    >Just this occurrence</button>
    {#if showFuture}
      <button
        type="button"
        onclick={() => onChoose('future')}
        disabled={busy}
        class="px-3 py-1.5 text-sm bg-surface1 text-warning rounded hover:bg-surface2 disabled:opacity-50"
        title="Apply to this occurrence and every future one — past stays"
      >This and future</button>
    {/if}
    <button
      type="button"
      onclick={() => onChoose('series')}
      disabled={busy}
      class="px-3 py-1.5 text-sm {seriesClass} disabled:opacity-50"
      title="Apply to every past + future instance"
    >Entire series</button>
    <button
      type="button"
      onclick={onCancel}
      disabled={busy}
      class="px-3 py-1.5 text-sm text-subtext hover:text-text"
    >Cancel</button>
  </div>
</div>
