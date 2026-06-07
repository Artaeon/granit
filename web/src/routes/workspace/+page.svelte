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
  import { onMount } from 'svelte';
  import SplitView from '$lib/workspace/SplitView.svelte';
  import PaneSlot from '$lib/workspace/PaneSlot.svelte';
  import NavIcon from '$lib/components/NavIcon.svelte';
  import { workspaceStoreSingleton } from '$lib/workspace/workspaceStore.svelte';
  import { leaves } from '$lib/workspace/splitTree';
  import { findPane } from '$lib/workspace/paneRegistry';
  import { isTypingTarget } from '$lib/util/isTypingTarget';

  // Shared module singleton so the StatusBar's workspace pills + this
  // shell read/write the same state.
  const store = workspaceStoreSingleton();

  // Route-scoped keyboard shortcut: bare 1..9 focuses leaf N. Mirrors
  // the index shown on each mobile leaf-tab. Skipped while the user is
  // typing inside a pane so a note editor or task input still gets
  // its own digits. No modifier so the chord is muscle-memory cheap;
  // the global Mod+1..9 still routes to the tab strip.
  onMount(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey || e.shiftKey) return;
      if (isTypingTarget(e.target)) return;
      if (e.key < '1' || e.key > '9') return;
      const idx = parseInt(e.key, 10) - 1;
      const leaf = leaves(store.active.layout)[idx];
      if (!leaf) return;
      e.preventDefault();
      store.focus(leaf.id);
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  let activeLeaves = $derived(leaves(store.active.layout));
  let canClose = $derived(activeLeaves.length > 1);

  // Maximized-leaf override. When the store says a leaf is maximized,
  // desktop renders just that PaneSlot full-height instead of the
  // SplitView. Splits still exist in the tree — we just hide them.
  // Falls back to null when the maximized id no longer points at a
  // real leaf (store self-heals; this is defensive).
  let maximizedLeaf = $derived(
    store.maximizedLeafId
      ? activeLeaves.find((l) => l.id === store.maximizedLeafId) ?? null
      : null
  );

  // Mobile picks the leaf to show from the store's focused leaf so
  // the same "which pane am I on" notion drives both: desktop's focus
  // ring AND mobile's full-screen leaf. Tapping a tab focuses; the
  // store auto-resets focus on workspace switch / layout change.
  let mobileLeaf = $derived(
    activeLeaves.find((l) => l.id === store.focusedLeafId) ?? activeLeaves[0]
  );
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
          onclick={() => store.focus(leaf.id)}
          class="inline-flex items-center gap-1.5 px-2 py-1 rounded text-xs font-medium border whitespace-nowrap tap-target
            {active ? 'bg-primary text-on-primary border-primary' : 'bg-surface0 text-subtext border-surface1 hover:border-primary'}"
        >
          {#if entry}
            <NavIcon name={entry.icon} class="w-3.5 h-3.5 {active ? 'text-on-primary' : 'text-dim'}" />
          {/if}
          <span>{entry?.label ?? leaf.pane}</span>
          <span class="opacity-60 text-[10px] font-mono">{i + 1}</span>
        </button>
      {/each}
    </div>
  {/if}

  <!-- Desktop: recursive split-tree fills the rest. When a leaf is
       maximized, short-circuit the SplitView and render just that
       PaneSlot full-height — the split tree is preserved underneath,
       toggling off restores the original layout. -->
  <div class="hidden md:flex flex-1 min-h-0 w-full overflow-hidden">
    {#if maximizedLeaf}
      <PaneSlot
        pane={maximizedLeaf.pane}
        leafId={maximizedLeaf.id}
        onChange={(p) => store.setPane(maximizedLeaf.id, p)}
        onSplitH={(p) => store.split(maximizedLeaf.id, 'h', p)}
        onSplitV={(p) => store.split(maximizedLeaf.id, 'v', p)}
        closable={canClose}
        onClose={() => store.close(maximizedLeaf.id)}
        focused={true}
        onFocus={() => store.focus(maximizedLeaf.id)}
        onSwap={(sourceLeafId) => store.swap(sourceLeafId, maximizedLeaf.id)}
      />
    {:else}
      <SplitView
        node={store.active.layout}
        onSetPane={store.setPane}
        onSetRatio={store.setRatio}
        onSplit={store.split}
        onClose={store.close}
        onSwap={store.swap}
        {canClose}
        focusedLeafId={store.focusedLeafId}
        onFocus={store.focus}
      />
    {/if}
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
        focused={mobileLeaf.id === store.focusedLeafId}
        onFocus={() => store.focus(mobileLeaf.id)}
      />
    {/if}
  </div>
</div>
