<!--
  Workspace shell — Phase 2 of the granit vision (VSCode-for-life).
  Named workspaces in a tray at the top; each workspace owns a
  recursive split-tree layout. Any pane can split horizontally or
  vertically, any pane can close (replaced by its sibling subtree).
  Pick pane types from each leaf's header.

  Mobile (< md) renders only the FIRST leaf — the recursive tree
  collapses gracefully on small screens. Phase 3 of the vision will
  add a leaf-picker for mobile so the user can navigate any tile.

  The shell stays tiny — most logic lives in the workspaceStore +
  splitTree primitives + the recursive SplitView. This file just
  wires the tray on top of the recursive view and falls back to a
  single-leaf flatten on mobile.
-->
<script lang="ts">
  import SplitView from '$lib/workspace/SplitView.svelte';
  import PaneSlot from '$lib/workspace/PaneSlot.svelte';
  import WorkspaceTray from '$lib/workspace/WorkspaceTray.svelte';
  import { createWorkspaceStore } from '$lib/workspace/workspaceStore.svelte';
  import { leaves } from '$lib/workspace/splitTree';

  const store = createWorkspaceStore();

  let firstLeaf = $derived(leaves(store.active.layout)[0]);
  let canClose = $derived(leaves(store.active.layout).length > 1);
</script>

<div class="flex flex-col h-screen w-full overflow-hidden bg-base">
  <WorkspaceTray {store} />

  <!-- Desktop: recursive split-tree fills the rest. -->
  <div class="hidden md:flex flex-1 min-h-0 w-full overflow-hidden">
    <SplitView
      node={store.active.layout}
      onSetPane={store.setPane}
      onSetRatio={store.setRatio}
      onSplit={store.split}
      onClose={store.close}
      {canClose}
    />
  </div>

  <!-- Mobile: show the first leaf only. Pane-type swap still works;
       splits + closes are hidden because there's nowhere to draw a
       second leaf at this width. -->
  <div class="flex md:hidden flex-1 min-h-0 w-full overflow-hidden">
    {#if firstLeaf}
      <PaneSlot
        pane={firstLeaf.pane}
        onChange={(p) => store.setPane(firstLeaf.id, p)}
      />
    {/if}
  </div>
</div>
