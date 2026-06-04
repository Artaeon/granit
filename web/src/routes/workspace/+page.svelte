<!--
  Workspace shell — the first tangible step toward the granit vision
  (VSCode-for-life). Two horizontal slots, each holding any pane from
  the registry (Tasks / Calendar / Goals). The user picks pane types
  from the slot headers; the chosen layout persists to localStorage
  so a refresh / shared-link returns to the same shape.

  Resize via mouse drag on the gutter. Mobile shows only the left
  slot — Phase 3 of the vision will add swipe-between-panes; this v1
  keeps the small screen single-pane so the slot picker still works
  one-handed.

  Deliberately minimal — no recursive splits, no multi-workspace
  tray, no cross-pane context bus. Each of those is a separate
  follow-up that lands on top of this foundation.
-->
<script lang="ts">
  import PaneSlot from '$lib/workspace/PaneSlot.svelte';
  import type { PaneKind } from '$lib/workspace/paneRegistry';
  import { loadStored, saveStored } from '$lib/util/storage';

  const LAYOUT_KEY = 'granit.workspace.layout';

  type Layout = {
    left: PaneKind;
    right: PaneKind;
    ratio: number; // 0.1 .. 0.9 — left-pane width fraction
  };

  function loadLayout(): Layout {
    const saved = loadStored<Layout | null>(LAYOUT_KEY, null);
    if (
      saved &&
      typeof saved === 'object' &&
      'left' in saved &&
      'right' in saved &&
      'ratio' in saved
    ) {
      return {
        left: saved.left,
        right: saved.right,
        ratio: Math.min(0.9, Math.max(0.1, saved.ratio ?? 0.5))
      };
    }
    return { left: 'tasks', right: 'calendar', ratio: 0.5 };
  }

  let layout = $state<Layout>(loadLayout());

  $effect(() => saveStored(LAYOUT_KEY, layout));

  // Resize logic — pointer drag on the gutter changes the left-pane
  // width ratio. We track ratio (not pixels) so a window resize
  // keeps the split proportional.
  let containerEl: HTMLElement | null = $state(null);
  let dragging = $state(false);

  function onPointerDown() {
    dragging = true;
  }
  function onPointerMove(e: PointerEvent) {
    if (!dragging || !containerEl) return;
    const rect = containerEl.getBoundingClientRect();
    const ratio = (e.clientX - rect.left) / rect.width;
    layout.ratio = Math.min(0.9, Math.max(0.1, ratio));
  }
  function onPointerUp() {
    dragging = false;
  }

  function setLeft(p: PaneKind) {
    layout = { ...layout, left: p };
  }
  function setRight(p: PaneKind) {
    layout = { ...layout, right: p };
  }
</script>

<svelte:window onpointermove={onPointerMove} onpointerup={onPointerUp} />

<div
  bind:this={containerEl}
  class="flex h-screen w-full overflow-hidden bg-base"
  class:select-none={dragging}
>
  <!-- Left slot — width = ratio% on desktop, full width on mobile
       (right slot + gutter hidden via md:flex). -->
  <div
    class="flex-shrink-0 min-h-0 h-full w-full md:w-[var(--left-w)]"
    style="--left-w: {layout.ratio * 100}%"
  >
    <PaneSlot pane={layout.left} onChange={setLeft} />
  </div>

  <!-- Gutter — pointer-drag handle on desktop only. -->
  <div
    role="separator"
    aria-orientation="vertical"
    aria-valuenow={Math.round(layout.ratio * 100)}
    aria-label="Resize panes"
    tabindex="0"
    onpointerdown={onPointerDown}
    class="hidden md:flex flex-shrink-0 w-1 cursor-col-resize bg-surface1 hover:bg-primary transition-colors"
  ></div>

  <!-- Right slot — fills the rest. Hidden on mobile (Phase 3 will
       add swipe-between-panes). -->
  <div class="hidden md:flex flex-1 min-h-0 h-full">
    <div class="flex-1 min-h-0">
      <PaneSlot pane={layout.right} onChange={setRight} />
    </div>
  </div>
</div>
