<!--
  SplitView — recursive renderer for the workspace split-tree.

  A node is either a leaf (a pane) or a split (two children + a
  gutter). The component renders itself via <svelte:self/> for each
  child, so any depth of nesting works without changes: a 2x2 grid
  is just a horizontal split where each child is a vertical split.
  (svelte:self avoids the self-import circular dep — see the
  ReferenceError fix that drove this change.)

  Owns the per-split gutter drag. Pane swap / split / close affordances
  live in PaneSlot — this component just plumbs them through.
-->
<script lang="ts">
  import PaneSlot from './PaneSlot.svelte';
  import type { TreeNode } from './splitTree';
  import type { PaneKind } from './paneRegistry';

  let {
    node,
    onSetPane,
    onSetRatio,
    onSplit,
    onClose,
    onSwap,
    onToggleMaximize,
    canClose,
    focusedLeafId,
    onFocus
  }: {
    node: TreeNode;
    onSetPane: (leafId: string, pane: PaneKind) => void;
    onSetRatio: (splitId: string, ratio: number) => void;
    onSplit: (leafId: string, direction: 'h' | 'v', newPane: PaneKind) => void;
    onClose: (leafId: string) => void;
    /** Swap two leaves' pane kinds. Wired by /workspace to store.swap.
     *  Drives the header drag-and-drop in PaneSlot. */
    onSwap: (leafIdA: string, leafIdB: string) => void;
    /** Toggle maximize for a leaf. Wired by /workspace to
     *  store.toggleMaximize. Drives PaneSlot's double-click-header
     *  affordance. */
    onToggleMaximize: (leafId: string) => void;
    /** True when the workspace has more than one leaf — drives
     *  PaneSlot's close-button visibility. The store guarantees at
     *  least one leaf survives at all times. */
    canClose: boolean;
    /** Id of the focused leaf in the active workspace. The matching
     *  PaneSlot renders a primary border + accent. */
    focusedLeafId: string;
    /** Called when the user clicks inside a leaf. The /workspace
     *  shell forwards this to store.focus(). */
    onFocus: (leafId: string) => void;
  } = $props();

  // Per-split drag state. Kept local to each SplitView instance so
  // dragging one gutter doesn't leak into siblings.
  let containerEl: HTMLElement | null = $state(null);
  let dragging = $state(false);

  function onPointerDown(e: PointerEvent) {
    if (node.kind !== 'split') return;
    dragging = true;
    // Pointer capture keeps the gutter receiving move/up events even
    // when the cursor strays outside the gutter rectangle mid-drag.
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
  }
  function onPointerMove(e: PointerEvent) {
    if (!dragging || !containerEl || node.kind !== 'split') return;
    const rect = containerEl.getBoundingClientRect();
    const ratio =
      node.direction === 'h'
        ? (e.clientX - rect.left) / rect.width
        : (e.clientY - rect.top) / rect.height;
    onSetRatio(node.id, ratio);
  }
  function onPointerUp(e: PointerEvent) {
    if (!dragging) return;
    dragging = false;
    (e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
  }

  // Keyboard resize: arrow keys nudge the ratio along the split axis.
  // Shift = 10% jumps (VSCode-style coarse step); Home = recenter at 50%.
  // Off-axis arrows are ignored so a vertical gutter doesn't jump on Up/Down.
  function onKeyDown(e: KeyboardEvent) {
    if (node.kind !== 'split') return;
    const step = e.shiftKey ? 0.1 : 0.02;
    const isH = node.direction === 'h';
    const decKey = isH ? 'ArrowLeft' : 'ArrowUp';
    const incKey = isH ? 'ArrowRight' : 'ArrowDown';
    if (e.key === decKey) {
      e.preventDefault();
      onSetRatio(node.id, Math.max(0.1, node.ratio - step));
    } else if (e.key === incKey) {
      e.preventDefault();
      onSetRatio(node.id, Math.min(0.9, node.ratio + step));
    } else if (e.key === 'Home') {
      e.preventDefault();
      onSetRatio(node.id, 0.5);
    }
  }
</script>

{#if node.kind === 'leaf'}
  <PaneSlot
    pane={node.pane}
    leafId={node.id}
    onChange={(p) => onSetPane(node.id, p)}
    onSplitH={(p) => onSplit(node.id, 'h', p)}
    onSplitV={(p) => onSplit(node.id, 'v', p)}
    closable={canClose}
    onClose={() => onClose(node.id)}
    focused={focusedLeafId === node.id}
    onFocus={() => onFocus(node.id)}
    onSwap={(sourceLeafId) => onSwap(sourceLeafId, node.id)}
    onToggleMaximize={() => onToggleMaximize(node.id)}
  />
{:else}
  <div
    bind:this={containerEl}
    class="flex w-full h-full min-w-0 min-h-0 overflow-hidden"
    class:flex-row={node.direction === 'h'}
    class:flex-col={node.direction === 'v'}
    class:select-none={dragging}
  >
    <div
      class="min-w-0 min-h-0 overflow-hidden"
      style:flex-basis={`${node.ratio * 100}%`}
      style:flex-grow="0"
      style:flex-shrink="0"
    >
      <svelte:self
        node={node.first}
        {onSetPane}
        {onSetRatio}
        {onSplit}
        {onClose}
        {onSwap}
        {onToggleMaximize}
        {canClose}
        {focusedLeafId}
        {onFocus}
      />
    </div>
    <!--
      Outer = 6px hit-zone so the cursor catches even when slightly off
      the visible line. Inner = the 1px line the user actually sees.
      group-hover propagates the hover-colour from outer to inner so
      the hit-zone hover behaviour matches the pre-polish version.
    -->
    <div
      role="separator"
      aria-orientation={node.direction === 'h' ? 'vertical' : 'horizontal'}
      aria-valuenow={Math.round(node.ratio * 100)}
      aria-label="Resize panes"
      tabindex="0"
      onpointerdown={onPointerDown}
      onpointermove={onPointerMove}
      onpointerup={onPointerUp}
      onkeydown={onKeyDown}
      class="group flex-shrink-0 flex items-center justify-center touch-none"
      class:w-1.5={node.direction === 'h'}
      class:h-1.5={node.direction === 'v'}
      class:cursor-col-resize={node.direction === 'h'}
      class:cursor-row-resize={node.direction === 'v'}
    >
      <div
        class="bg-surface1 group-hover:bg-primary transition-colors"
        class:w-px={node.direction === 'h'}
        class:h-full={node.direction === 'h'}
        class:h-px={node.direction === 'v'}
        class:w-full={node.direction === 'v'}
      ></div>
    </div>
    <div class="flex-1 min-w-0 min-h-0 overflow-hidden">
      <svelte:self
        node={node.second}
        {onSetPane}
        {onSetRatio}
        {onSplit}
        {onClose}
        {onSwap}
        {onToggleMaximize}
        {canClose}
        {focusedLeafId}
        {onFocus}
      />
    </div>
  </div>
{/if}
