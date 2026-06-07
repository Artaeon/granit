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
  import Button from '$lib/components/Button.svelte';

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
    onSwap,
    onToggleMaximize
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
    /** Called when the header is double-clicked — the parent should
     *  toggle the workspace's maximize state for this leaf. Optional;
     *  when omitted, double-click does nothing (mobile view). */
    onToggleMaximize?: () => void;
  } = $props();

  let entry = $derived(findPane(pane));

  // Drag-and-drop. VSCode-style: grab a pane's header, drop on
  // another pane. The drop region inside the target decides what
  // happens:
  //   center  → swap two leaves' contents (existing behaviour)
  //   right   → split the target horizontally; new right pane gets
  //             the dragged source's pane kind (source unchanged)
  //   bottom  → split the target vertically; new bottom pane gets
  //             the dragged source's pane kind (source unchanged)
  // Uses custom MIME types so external text/file drags don't light
  // up drop targets. Mobile clients fall through (touch doesn't fire
  // native drag events) and use the palette swap/split commands.
  const DRAG_MIME_LEAF = 'application/x-granit-pane-leaf';
  const DRAG_MIME_PANE = 'application/x-granit-pane-kind';
  // 25% margin on right + bottom edges — wide enough to hit
  // comfortably without crowding the center swap zone.
  const EDGE_THRESHOLD = 0.75;
  type DropRegion = 'center' | 'right' | 'bottom';
  let dropRegion = $state<DropRegion | null>(null);
  // Source-drag visual feedback. When true, the pane fades to ~40%
  // opacity so the user sees where the content is moving FROM.
  // Mirrors VSCode's drag-source treatment. Flips back on dragend
  // (always fires after drop/cancel, including drop outside).
  let dragging = $state(false);

  function onDragStart(e: DragEvent) {
    if (!leafId || !e.dataTransfer) return;
    e.dataTransfer.setData(DRAG_MIME_LEAF, leafId);
    e.dataTransfer.setData(DRAG_MIME_PANE, pane);
    e.dataTransfer.effectAllowed = 'move';
    dragging = true;
  }
  function onDragEnd() {
    dragging = false;
  }
  function regionFor(e: DragEvent, rect: DOMRect): DropRegion {
    const rightFraction = (e.clientX - rect.left) / rect.width;
    const bottomFraction = (e.clientY - rect.top) / rect.height;
    // Prefer the more "extreme" edge when both qualify (bottom-right
    // corner). Bias slightly to the right edge for ties since most
    // users drag horizontally and read right→left in dialect.
    const rightExcess = rightFraction - EDGE_THRESHOLD;
    const bottomExcess = bottomFraction - EDGE_THRESHOLD;
    if (rightExcess > 0 && rightExcess >= bottomExcess && onSplitH) return 'right';
    if (bottomExcess > 0 && onSplitV) return 'bottom';
    return 'center';
  }
  function onDragOver(e: DragEvent) {
    if (!leafId || !e.dataTransfer) return;
    if (!e.dataTransfer.types.includes(DRAG_MIME_LEAF)) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    dropRegion = regionFor(e, rect);
  }
  function onDragLeave(e: DragEvent) {
    // Only clear when leaving the outer slot — child elements can
    // trigger dragleave as the cursor crosses their boundaries.
    if (e.currentTarget === e.target) {
      dropRegion = null;
    }
  }
  function onDrop(e: DragEvent) {
    const region = dropRegion;
    dropRegion = null;
    if (!leafId || !e.dataTransfer) return;
    const source = e.dataTransfer.getData(DRAG_MIME_LEAF);
    const sourcePane = e.dataTransfer.getData(DRAG_MIME_PANE) as PaneKind;
    if (!source || source === leafId) return;
    e.preventDefault();
    if (region === 'right' && onSplitH && sourcePane) {
      onSplitH(sourcePane);
    } else if (region === 'bottom' && onSplitV && sourcePane) {
      onSplitV(sourcePane);
    } else if (onSwap) {
      onSwap(source);
    }
  }

  // When the user clicks the split buttons, default the new pane to
  // the FIRST pane type that isn't this one — saves a click vs.
  // "same as current" which is a useless split.
  let nextPaneCandidate = $derived(PANES.find((p) => p.id !== pane)?.id ?? pane);
</script>

<div
  onpointerdowncapture={() => onFocus?.()}
  ondragstart={onDragStart}
  ondragend={onDragEnd}
  ondragover={onDragOver}
  ondragleave={onDragLeave}
  ondrop={onDrop}
  class="relative flex flex-col w-full h-full min-w-0 min-h-0 border rounded overflow-hidden bg-base transition-all
    {focused ? 'border-primary' : 'border-surface1'}
    {dragging ? 'opacity-40' : ''}"
>
  <header
    draggable={leafId ? 'true' : undefined}
    ondblclick={(e) => {
      // Only react when the double-click hit the header background
      // — clicks on the picker / split / close buttons keep their own
      // single-click semantics and shouldn't accidentally maximize.
      if (e.target !== e.currentTarget) return;
      onToggleMaximize?.();
    }}
    title={onToggleMaximize ? 'Double-click to maximize / restore' : undefined}
    class="flex items-center gap-1.5 px-2 py-1 border-b text-xs flex-shrink-0 transition-colors
      {focused ? 'border-primary bg-surface0' : 'border-surface1 bg-surface0'}
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
      <Button
        variant="ghost"
        size="sm"
        iconOnly
        onclick={() => onSplitH?.(nextPaneCandidate)}
        title="Split horizontally — new pane appears to the right"
        aria-label="Split horizontally"
        class="hidden md:inline-flex font-mono"
      >|</Button>
    {/if}
    {#if onSplitV}
      <Button
        variant="ghost"
        size="sm"
        iconOnly
        onclick={() => onSplitV?.(nextPaneCandidate)}
        title="Split vertically — new pane appears below"
        aria-label="Split vertically"
        class="hidden md:inline-flex font-mono"
      >_</Button>
    {/if}
    {#if closable && onClose}
      <Button
        variant="ghost"
        size="sm"
        iconOnly
        onclick={onClose}
        title="Close this pane"
        aria-label="Close this pane"
        class="tap-target text-base hover:text-error"
      >×</Button>
    {/if}
  </header>
  <div class="flex-1 min-h-0 overflow-auto">
    {#if entry}
      {@const PaneComponent = entry.component}
      <PaneComponent />
    {/if}
  </div>
  <!-- Drop preview overlay. Shows during dragover; pointer-events-none
       so the underlying pane still receives the dragover/leave/drop
       events that drive the region calculation. -->
  {#if dropRegion}
    <div class="absolute inset-0 pointer-events-none z-10">
      {#if dropRegion === 'right'}
        <div class="absolute top-0 bottom-0 right-0 w-1/2 bg-primary/15 border-l-2 border-primary"></div>
      {:else if dropRegion === 'bottom'}
        <div class="absolute left-0 right-0 bottom-0 h-1/2 bg-primary/15 border-t-2 border-primary"></div>
      {:else}
        <div class="absolute inset-0 ring-2 ring-inset ring-primary bg-primary/5"></div>
      {/if}
    </div>
  {/if}
</div>
