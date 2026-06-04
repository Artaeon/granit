<!--
  Workspace shell — Phase 2 of the granit vision (VSCode-for-life).
  Named workspaces in a tray at the top, each holding a two-slot
  horizontal split. The user picks pane types from the slot headers;
  the gutter resizes the left-pane width ratio. Every choice persists
  to localStorage so refresh / shared-link returns to the same shape.

  Mobile shows only the left slot — Phase 3 of the vision will add
  swipe-between-panes; this v1 keeps small screens single-pane so the
  slot picker still works one-handed.

  Layout: tray on top, two-slot split filling the rest. The split
  inherits its shape from the active workspace; switching workspaces
  flips the pane types + ratio in one paint. Multi-workspace state
  lives in $lib/workspace/workspaceStore.
-->
<script lang="ts">
  import PaneSlot from '$lib/workspace/PaneSlot.svelte';
  import WorkspaceTray from '$lib/workspace/WorkspaceTray.svelte';
  import { createWorkspaceStore } from '$lib/workspace/workspaceStore.svelte';
  import type { PaneKind } from '$lib/workspace/paneRegistry';

  const store = createWorkspaceStore();

  // Gutter-drag — pointer drag updates the active workspace's
  // ratio. We track ratio (not pixels) so a window resize keeps the
  // split proportional.
  let splitEl: HTMLElement | null = $state(null);
  let dragging = $state(false);

  function onPointerDown() {
    dragging = true;
  }
  function onPointerMove(e: PointerEvent) {
    if (!dragging || !splitEl) return;
    const rect = splitEl.getBoundingClientRect();
    const ratio = (e.clientX - rect.left) / rect.width;
    store.patchActiveLayout({ ratio: Math.min(0.9, Math.max(0.1, ratio)) });
  }
  function onPointerUp() {
    dragging = false;
  }

  function setLeft(p: PaneKind) {
    store.patchActiveLayout({ left: p });
  }
  function setRight(p: PaneKind) {
    store.patchActiveLayout({ right: p });
  }
</script>

<svelte:window onpointermove={onPointerMove} onpointerup={onPointerUp} />

<div class="flex flex-col h-screen w-full overflow-hidden bg-base" class:select-none={dragging}>
  <WorkspaceTray {store} />

  <div bind:this={splitEl} class="flex flex-1 min-h-0 w-full overflow-hidden">
    <!-- Left slot. Width is set via CSS variable so the gutter drag
         updates the layout in one paint. Mobile collapses to full
         width and the right slot is hidden (Phase 3 will add
         swipe-between-panes). -->
    <div
      class="flex-shrink-0 min-h-0 h-full w-full md:w-[var(--left-w)]"
      style="--left-w: {store.active.layout.ratio * 100}%"
    >
      <PaneSlot pane={store.active.layout.left} onChange={setLeft} />
    </div>

    <!-- Gutter — pointer-drag handle on desktop only. -->
    <div
      role="separator"
      aria-orientation="vertical"
      aria-valuenow={Math.round(store.active.layout.ratio * 100)}
      aria-label="Resize panes"
      tabindex="0"
      onpointerdown={onPointerDown}
      class="hidden md:flex flex-shrink-0 w-1 cursor-col-resize bg-surface1 hover:bg-primary transition-colors"
    ></div>

    <!-- Right slot — fills the rest, hidden on mobile. -->
    <div class="hidden md:flex flex-1 min-h-0 h-full">
      <div class="flex-1 min-h-0">
        <PaneSlot pane={store.active.layout.right} onChange={setRight} />
      </div>
    </div>
  </div>
</div>
