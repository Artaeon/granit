<script lang="ts">
  // RightPane shell — the companion sidebar that hosts a secondary
  // view alongside the main route. Phase 1.5 expanded Phase 1's five
  // content options to ten and replaced the horizontal icon-row
  // picker with a dropdown (10 buttons don't fit a slim header). It
  // also added a mobile bottom-sheet variant — the pane no longer
  // hides on small viewports; instead it slides up from the bottom
  // with a drag-handle for height resize.
  //
  // Width on desktop is driven by the rightPaneStore (persisted,
  // clamped 280..640). Height on mobile is driven by mobileHeight
  // (persisted, clamped 30..90 vh). The drag handle is on the LEFT
  // edge on desktop, top centre on mobile.
  //
  // Decomposition: the dropdown picker lives in ContentPicker, the
  // drag math lives in useResizeHandle, and the 10-option meta lives
  // in rightPaneContentMeta. This file is now just the shell —
  // shared header/close, body switch, desktop vs mobile chrome.

  import { get } from 'svelte/store';
  import {
    rightPaneStore,
    setRightPaneContent,
    setRightPaneWidth,
    setRightPaneMobileHeight,
    closeRightPane
  } from '$lib/stores/rightPane';
  import { mediaQuery } from '$lib/util/mediaQuery';
  import ContentPicker from './ContentPicker.svelte';
  import { useResizeHandle } from './useResizeHandle.svelte';
  import RightPaneCalendar from './rightpane/RightPaneCalendar.svelte';
  import RightPaneNotes from './rightpane/RightPaneNotes.svelte';
  import RightPaneAI from './rightpane/RightPaneAI.svelte';
  import RightPaneVision from './rightpane/RightPaneVision.svelte';
  import RightPaneWidgets from './rightpane/RightPaneWidgets.svelte';
  import RightPaneTasks from './rightpane/RightPaneTasks.svelte';
  import RightPaneToday from './rightpane/RightPaneToday.svelte';
  import RightPaneGoals from './rightpane/RightPaneGoals.svelte';
  import RightPaneHabits from './rightpane/RightPaneHabits.svelte';
  import RightPaneDashboard from './rightpane/RightPaneDashboard.svelte';

  // Desktop horizontal resize. The handle is a wider hit-target (8px)
  // with a 2px visible line (4px on hover/active) so users actually
  // see it. cursor: col-resize + body-level userSelect lock during the
  // drag so the cursor doesn't flicker through selectable text in the
  // main pane.
  const desktopResize = useResizeHandle({
    axis: 'x',
    getStart: () => get(rightPaneStore).width,
    onResize: (n) => setRightPaneWidth(n)
  });

  // Mobile bottom-sheet drag. Touch handlers translate the y-delta
  // into a vh percentage. Drag up = bigger (cap 90); drag down past
  // 35 → close instead of clamping at the floor, so users can swipe-
  // down to dismiss like other bottom sheets in the app.
  const mobileResize = useResizeHandle({
    axis: 'y',
    getStart: () => get(rightPaneStore).mobileHeight,
    getCurrent: () => get(rightPaneStore).mobileHeight,
    onResize: (n) => setRightPaneMobileHeight(n),
    onPullClose: () => {
      // Snap back to 60 (default) before closing so a subsequent
      // re-open lands at a usable height, not the floor.
      setRightPaneMobileHeight(60);
      closeRightPane();
    }
  });

  // Viewport: mobile = below md breakpoint (Tailwind's 768px). The
  // store stays the same; only the layout flips. md+ renders the
  // pane as a flex sibling; below md renders it as a fixed bottom
  // sheet with a translate-y animation.
  const isDesktop = mediaQuery('(min-width: 768px)');
</script>

{#if $isDesktop}
  <!-- ── Desktop variant ──────────────────────────────────────────
       Flex-sibling column on the right. Width persisted; drag handle
       on the left edge. -->
  <aside
    class="hidden md:flex flex-col flex-shrink-0 bg-mantle border-l border-surface1 relative h-full min-h-0"
    style="width: {$rightPaneStore.width}px"
    aria-label="Right pane"
  >
    <!-- Drag handle. 8px hit-target with a 2px visible line by
         default; expands to 4px on hover/active. surface2 baseline,
         primary on hover/active so users actually SEE the handle. -->
    <button
      type="button"
      class="absolute top-0 left-0 bottom-0 w-2 z-10 group flex items-stretch justify-center cursor-col-resize"
      onmousedown={desktopResize.startMouseDrag}
      aria-label="Resize right pane"
    >
      <span
        class="my-auto h-12 transition-all
          {desktopResize.dragging
            ? 'w-1 bg-primary'
            : 'w-0.5 bg-surface2 group-hover:w-1 group-hover:bg-primary'}"
        aria-hidden="true"
      ></span>
    </button>

    <!-- Header: dropdown picker + close. -->
    <header class="flex items-center gap-2 px-2 py-1.5 border-b border-surface1 flex-shrink-0">
      <ContentPicker current={$rightPaneStore.content} onSelect={setRightPaneContent} />

      <span class="flex-1"></span>
      <button
        type="button"
        onclick={closeRightPane}
        title="Close right pane (⌘\)"
        aria-label="Close right pane"
        class="w-7 h-7 flex items-center justify-center rounded text-dim hover:bg-surface0 hover:text-text transition-colors"
      >
        <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <line x1="18" y1="6" x2="6" y2="18"/>
          <line x1="6" y1="6" x2="18" y2="18"/>
        </svg>
      </button>
    </header>

    <!-- Body. Each content sub-component owns its own scroll, header,
         and footer affordances so the shell here stays trivial. -->
    <div class="flex-1 min-h-0 overflow-hidden">
      {#if $rightPaneStore.content === 'calendar'}
        <RightPaneCalendar />
      {:else if $rightPaneStore.content === 'notes'}
        <RightPaneNotes />
      {:else if $rightPaneStore.content === 'ai'}
        <RightPaneAI />
      {:else if $rightPaneStore.content === 'vision'}
        <RightPaneVision />
      {:else if $rightPaneStore.content === 'widgets'}
        <RightPaneWidgets />
      {:else if $rightPaneStore.content === 'tasks'}
        <RightPaneTasks />
      {:else if $rightPaneStore.content === 'today'}
        <RightPaneToday />
      {:else if $rightPaneStore.content === 'goals'}
        <RightPaneGoals />
      {:else if $rightPaneStore.content === 'habits'}
        <RightPaneHabits />
      {:else if $rightPaneStore.content === 'dashboard'}
        <RightPaneDashboard />
      {/if}
    </div>
  </aside>
{:else}
  <!-- ── Mobile bottom-sheet variant ───────────────────────────────
       fixed inset-x-0 bottom-0 with a vh height + translate-y
       animation. Drag handle on the top centre resizes via touch;
       a pull-down past 35vh closes the sheet. The backdrop is
       deliberately omitted — users keep their main view visible
       while the pane sits docked. -->
  <div
    class="md:hidden fixed left-0 right-0 bottom-0 z-30 bg-mantle border-t border-surface1 rounded-t-xl shadow-2xl flex flex-col"
    style="height: {$rightPaneStore.mobileHeight}vh; transition: {mobileResize.dragging ? 'none' : 'height 150ms ease'};"
    aria-label="Right pane"
    role="dialog"
  >
    <!-- Drag handle bar — full-width tap surface, visible pill at
         the centre. touch-action: none keeps the browser from
         interpreting the drag as a scroll. -->
    <button
      type="button"
      class="flex-shrink-0 w-full py-2 flex items-center justify-center cursor-grab active:cursor-grabbing"
      style="touch-action: none;"
      ontouchstart={mobileResize.startTouchDrag}
      ontouchmove={mobileResize.onTouchMove}
      ontouchend={mobileResize.onTouchEnd}
      ontouchcancel={mobileResize.onTouchEnd}
      aria-label="Resize right pane (drag) or pull down to close"
    >
      <span class="block h-1 w-10 rounded-full bg-surface2"></span>
    </button>

    <!-- Header: dropdown picker + close. Same picker as desktop. -->
    <header class="flex items-center gap-2 px-3 pb-2 border-b border-surface1 flex-shrink-0">
      <ContentPicker current={$rightPaneStore.content} onSelect={setRightPaneContent} />

      <span class="flex-1"></span>
      <button
        type="button"
        onclick={closeRightPane}
        title="Close right pane"
        aria-label="Close right pane"
        class="w-8 h-8 flex items-center justify-center rounded text-dim hover:bg-surface0 hover:text-text transition-colors"
      >
        <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <line x1="18" y1="6" x2="6" y2="18"/>
          <line x1="6" y1="6" x2="18" y2="18"/>
        </svg>
      </button>
    </header>

    <div class="flex-1 min-h-0 overflow-hidden">
      {#if $rightPaneStore.content === 'calendar'}
        <RightPaneCalendar />
      {:else if $rightPaneStore.content === 'notes'}
        <RightPaneNotes />
      {:else if $rightPaneStore.content === 'ai'}
        <RightPaneAI />
      {:else if $rightPaneStore.content === 'vision'}
        <RightPaneVision />
      {:else if $rightPaneStore.content === 'widgets'}
        <RightPaneWidgets />
      {:else if $rightPaneStore.content === 'tasks'}
        <RightPaneTasks />
      {:else if $rightPaneStore.content === 'today'}
        <RightPaneToday />
      {:else if $rightPaneStore.content === 'goals'}
        <RightPaneGoals />
      {:else if $rightPaneStore.content === 'habits'}
        <RightPaneHabits />
      {:else if $rightPaneStore.content === 'dashboard'}
        <RightPaneDashboard />
      {/if}
    </div>
  </div>
{/if}
