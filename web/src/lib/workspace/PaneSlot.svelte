<!--
  PaneSlot — a single tileable cell in the workspace shell. Wraps a
  pane component with a slim header carrying:

    - PaneTypePicker (single dropdown showing the active pane name)
    - split affordances: horizontal | / vertical _
    - a "close" button when the slot is closable (the workspace
      shell hides this when only one leaf remains)

  Stays presentational. Layout, resize, persistence — all owned by
  the parent SplitView / workspace store.
-->
<script lang="ts">
  import { PANES, findPane, type PaneKind } from './paneRegistry';
  import PaneTypePicker from './PaneTypePicker.svelte';

  let {
    pane,
    onChange,
    onSplitH,
    onSplitV,
    closable = false,
    onClose
  }: {
    pane: PaneKind;
    onChange: (next: PaneKind) => void;
    /** Split this pane horizontally — new pane appears to the right. */
    onSplitH?: (newPane: PaneKind) => void;
    /** Split this pane vertically — new pane appears below. */
    onSplitV?: (newPane: PaneKind) => void;
    closable?: boolean;
    onClose?: () => void;
  } = $props();

  let entry = $derived(findPane(pane));

  // When the user clicks the split buttons, default the new pane to
  // the FIRST pane type that isn't this one — saves a click vs.
  // "same as current" which is a useless split.
  let nextPaneCandidate = $derived(PANES.find((p) => p.id !== pane)?.id ?? pane);
</script>

<div class="flex flex-col h-full min-h-0 border border-surface1 rounded overflow-hidden bg-base">
  <header
    class="flex items-center gap-1.5 px-2 py-1 border-b border-surface1 bg-surface0 text-xs flex-shrink-0"
  >
    <PaneTypePicker {pane} {onChange} />
    <span class="flex-1"></span>
    {#if onSplitH}
      <button
        type="button"
        onclick={() => onSplitH?.(nextPaneCandidate)}
        title="Split horizontally — new pane appears to the right"
        aria-label="Split horizontally"
        class="px-1.5 py-0.5 text-dim hover:text-primary border border-surface1 hover:border-primary rounded font-mono text-[11px] leading-none"
      >|</button>
    {/if}
    {#if onSplitV}
      <button
        type="button"
        onclick={() => onSplitV?.(nextPaneCandidate)}
        title="Split vertically — new pane appears below"
        aria-label="Split vertically"
        class="px-1.5 py-0.5 text-dim hover:text-primary border border-surface1 hover:border-primary rounded font-mono text-[11px] leading-none"
      >_</button>
    {/if}
    {#if closable && onClose}
      <button
        type="button"
        onclick={onClose}
        title="Close this pane"
        aria-label="Close this pane"
        class="text-dim hover:text-error text-base leading-none px-1"
      >×</button>
    {/if}
  </header>
  <div class="flex-1 min-h-0 overflow-auto">
    {#if entry}
      {@const PaneComponent = entry.component}
      <PaneComponent />
    {/if}
  </div>
</div>
