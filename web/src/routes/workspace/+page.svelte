<!--
  Workspace shell — Phase 2 of the granit vision (VSCode-for-life).
  Each named workspace owns a recursive split-tree layout. Any pane
  can split horizontally or vertically, any pane can close (replaced
  by its sibling subtree). Pick pane types from each leaf's header.

  Workspace switching/rename/create/delete lives in the StatusBar's
  WorkspacePills — that's the single switcher across the app, so
  /workspace doesn't ship its own tray.

  Mobile (< md): the recursive tree collapses to a tabbed
  single-leaf view. A pill row above the pane area lists every leaf
  as a tab — tap to switch which leaf is shown full-screen. Splits +
  closes still work from the leaf header.
-->
<script lang="ts">
  import SplitView from '$lib/workspace/SplitView.svelte';
  import PaneSlot from '$lib/workspace/PaneSlot.svelte';
  import { workspaceStoreSingleton } from '$lib/workspace/workspaceStore.svelte';
  import { leaves } from '$lib/workspace/splitTree';
  import { findPane } from '$lib/workspace/paneRegistry';

  // Shared module singleton so the StatusBar's workspace pills + this
  // shell read/write the same state.
  const store = workspaceStoreSingleton();

  let activeLeaves = $derived(leaves(store.active.layout));
  let canClose = $derived(activeLeaves.length > 1);

  // Mobile: which leaf is currently visible. Defaults to the first
  // leaf; tap a tab to switch. Resets when the active workspace
  // changes (new workspace → first leaf wins).
  let mobileLeafId = $state<string | null>(null);
  let mobileLeaf = $derived(
    activeLeaves.find((l) => l.id === mobileLeafId) ?? activeLeaves[0]
  );
  $effect(() => {
    // Reset when the workspace switches so the user lands on the
    // first leaf of the new workspace, not a stale id from the
    // previous one.
    void store.activeId;
    mobileLeafId = null;
  });
</script>

<div class="flex flex-col h-screen w-full overflow-hidden bg-base">
  <!-- Mobile leaf-tabs: shows every leaf as a tab. Visible only on
       mobile (< md); desktop renders the recursive split-tree
       directly. -->
  {#if activeLeaves.length > 1}
    <div
      class="flex md:hidden items-center gap-1.5 px-2 py-1.5 border-b border-surface1 bg-mantle overflow-x-auto flex-shrink-0 scrollbar-thin"
      role="tablist"
      aria-label="Workspace panes"
    >
      <span class="text-dim font-mono uppercase tracking-wider text-[10px] flex-shrink-0">panes</span>
      {#each activeLeaves as leaf, i (leaf.id)}
        {@const entry = findPane(leaf.pane)}
        {@const active = mobileLeaf?.id === leaf.id}
        <button
          type="button"
          role="tab"
          aria-selected={active}
          onclick={() => (mobileLeafId = leaf.id)}
          class="px-2 py-0.5 rounded text-xs font-medium border whitespace-nowrap
            {active ? 'bg-primary text-on-primary border-primary' : 'bg-surface0 text-subtext border-surface1 hover:border-primary'}"
        >{entry?.label ?? leaf.pane} <span class="opacity-60 ml-0.5">{i + 1}</span></button>
      {/each}
    </div>
  {/if}

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

  <!-- Mobile: show whichever leaf the tabs picked. Splits + closes
       still operate against the tree — they just don't render
       side-by-side on small screens. -->
  <div class="flex md:hidden flex-1 min-h-0 w-full overflow-hidden">
    {#if mobileLeaf}
      <PaneSlot
        pane={mobileLeaf.pane}
        onChange={(p) => store.setPane(mobileLeaf.id, p)}
        onSplitH={(p) => store.split(mobileLeaf.id, 'h', p)}
        onSplitV={(p) => store.split(mobileLeaf.id, 'v', p)}
        closable={canClose}
        onClose={() => store.close(mobileLeaf.id)}
      />
    {/if}
  </div>
</div>
