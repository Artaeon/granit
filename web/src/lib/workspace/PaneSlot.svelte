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
  import AddPaneMenu from './AddPaneMenu.svelte';

  let {
    pane,
    leafId,
    onChange,
    onSplitH,
    onSplitV,
    closable = false,
    onClose,
    focused = false,
    onFocus,
    onSwap
  }: {
    pane: PaneKind;
    /** Id of the leaf this slot renders. Used to label drag payloads
     *  for the header drag-and-drop swap. Optional — when omitted,
     *  drag-and-drop is disabled (e.g. mobile single-leaf view). */
    leafId?: string;
    onChange: (next: PaneKind) => void;
    /** Split this pane horizontally — new pane appears to the right. */
    onSplitH?: (newPane: PaneKind) => void;
    /** Split this pane vertically — new pane appears below. */
    onSplitV?: (newPane: PaneKind) => void;
    closable?: boolean;
    onClose?: () => void;
    /** True when this leaf is the active workspace's focused leaf —
     *  tints the header border + adds a primary outline so the user
     *  sees where keyboard commands (g w, future splits) will land. */
    focused?: boolean;
    /** Called on pointerdown inside the slot so a click anywhere
     *  inside the pane focuses it. Capture-phase to fire before any
     *  child handler. */
    onFocus?: () => void;
    /** Called when a header from another PaneSlot is dropped onto
     *  this one — the parent should swap pane kinds between the two
     *  leaves. The argument is the SOURCE leaf id (this leaf's id is
     *  already known to the caller). */
    onSwap?: (sourceLeafId: string) => void;
  } = $props();

  let entry = $derived(findPane(pane));

  // Drag-and-drop header swap. VSCode-style: grab a pane's header,
  // drag it onto another pane's header, the two contents swap. Uses
  // a custom MIME type so the browser only treats granit headers as
  // valid drop targets — text dragged from outside the app won't
  // light up a target. Mobile clients fall through (touch doesn't
  // fire native drag events) and use the palette swap commands.
  const DRAG_MIME = 'application/x-granit-pane-leaf';
  let dragOver = $state(false);
  function onDragStart(e: DragEvent) {
    if (!leafId || !e.dataTransfer) return;
    e.dataTransfer.setData(DRAG_MIME, leafId);
    e.dataTransfer.effectAllowed = 'move';
  }
  function onDragOver(e: DragEvent) {
    if (!leafId || !onSwap || !e.dataTransfer) return;
    if (!e.dataTransfer.types.includes(DRAG_MIME)) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    dragOver = true;
  }
  function onDragLeave() {
    dragOver = false;
  }
  function onDrop(e: DragEvent) {
    dragOver = false;
    if (!leafId || !onSwap || !e.dataTransfer) return;
    const source = e.dataTransfer.getData(DRAG_MIME);
    if (!source || source === leafId) return;
    e.preventDefault();
    onSwap(source);
  }

  // When the user clicks the split buttons, default the new pane to
  // the FIRST pane type that isn't this one — saves a click vs.
  // "same as current" which is a useless split.
  let nextPaneCandidate = $derived(PANES.find((p) => p.id !== pane)?.id ?? pane);
</script>

<div
  onpointerdowncapture={() => onFocus?.()}
  class="flex flex-col h-full min-h-0 border rounded overflow-hidden bg-base transition-colors {focused ? 'border-primary' : 'border-surface1'}"
>
  <header
    draggable={leafId ? 'true' : undefined}
    ondragstart={onDragStart}
    ondragover={onDragOver}
    ondragleave={onDragLeave}
    ondrop={onDrop}
    class="flex items-center gap-1.5 px-2 py-1 border-b text-xs flex-shrink-0 transition-colors
      {focused ? 'border-primary bg-surface0' : 'border-surface1 bg-surface0'}
      {dragOver ? 'ring-2 ring-inset ring-primary' : ''}
      {leafId ? 'cursor-grab active:cursor-grabbing' : ''}"
  >
    <PaneTypePicker {pane} {onChange} />
    <span class="flex-1"></span>
    {#if onSplitH}
      <!-- Mobile: tap "+ pane" → pick a pane type from the popover →
           that becomes the new leaf. Mobile shows one leaf at a time,
           so a silent default-split was a footgun — user kept getting
           an unrelated pane and having to swap it. Now the menu makes
           the choice explicit. -->
      <span class="md:hidden">
        <AddPaneMenu
          excludePane={pane}
          onPick={(p) => onSplitH?.(p)}
        />
      </span>
      <!-- Desktop keeps the two axes so a user designing a side-by-
           side layout can pick where the new pane lands. -->
      <button
        type="button"
        onclick={() => onSplitH?.(nextPaneCandidate)}
        title="Split horizontally — new pane appears to the right"
        aria-label="Split horizontally"
        class="hidden md:inline-block px-1.5 py-0.5 text-dim hover:text-primary border border-surface1 hover:border-primary rounded font-mono text-[11px] leading-none"
      >|</button>
    {/if}
    {#if onSplitV}
      <button
        type="button"
        onclick={() => onSplitV?.(nextPaneCandidate)}
        title="Split vertically — new pane appears below"
        aria-label="Split vertically"
        class="hidden md:inline-block px-1.5 py-0.5 text-dim hover:text-primary border border-surface1 hover:border-primary rounded font-mono text-[11px] leading-none"
      >_</button>
    {/if}
    {#if closable && onClose}
      <button
        type="button"
        onclick={onClose}
        title="Close this pane"
        aria-label="Close this pane"
        class="tap-target text-dim hover:text-error text-base leading-none px-2 md:px-1"
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
