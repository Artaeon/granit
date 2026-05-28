<script lang="ts">
  // RightPane shell — the companion right sidebar that hosts a
  // secondary view (calendar / notes / AI / vision / widgets) while
  // the main route stays in the middle pane. Hidden on mobile via the
  // `hidden md:flex` class on the <aside> — the soft keyboard + bottom
  // nav already crowd small viewports, and the pane's value (a
  // persistent reference column alongside content) only makes sense
  // at desktop/tablet widths.
  //
  // Width is driven by the rightPaneStore (persisted, clamped 280..640).
  // The left edge of the pane is a drag handle; mousedown begins
  // resize, mousemove updates the store (which re-clamps + persists),
  // mouseup tears down the listeners. We compute new width as
  // (windowWidth - clientX) so the pane is right-anchored — dragging
  // LEFT widens it, RIGHT narrows it.
  //
  // The header content-picker is a tiny icon row: five small buttons
  // (calendar / notes / AI / vision / widgets) plus a close ×. Click =
  // setRightPaneContent (which also opens the pane if it was closed —
  // a defensive default since this component only renders when open is
  // true anyway, but it keeps the helper consistent with keyboard
  // shortcuts that may fire while the pane is hidden).

  import {
    rightPaneStore,
    setRightPaneContent,
    setRightPaneWidth,
    closeRightPane,
    type RightPaneContent
  } from '$lib/stores/rightPane';
  import RightPaneCalendar from './rightpane/RightPaneCalendar.svelte';
  import RightPaneNotes from './rightpane/RightPaneNotes.svelte';
  import RightPaneAI from './rightpane/RightPaneAI.svelte';
  import RightPaneVision from './rightpane/RightPaneVision.svelte';
  import RightPaneWidgets from './rightpane/RightPaneWidgets.svelte';

  // Drag-resize state. Closure-local refs (not $state) since this
  // listener doesn't render anything — it just mutates the store.
  let dragging = $state(false);

  function startDrag(e: MouseEvent) {
    e.preventDefault();
    dragging = true;
    // Body-level cursor + user-select lock so the cursor doesn't
    // flicker between col-resize and the text-cursor as the user
    // drags across the main pane's selectable text.
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';

    function onMove(ev: MouseEvent) {
      const next = window.innerWidth - ev.clientX;
      setRightPaneWidth(next);
    }
    function onUp() {
      dragging = false;
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
      window.removeEventListener('mousemove', onMove);
      window.removeEventListener('mouseup', onUp);
    }
    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
  }

  interface ContentOption {
    id: RightPaneContent;
    label: string;
    title: string;
    // SVG path snippet for an outline icon. Drawn at 16x16; uses
    // currentColor + stroke-width 1.8 for parity with the nav rail.
    iconPath: string;
  }

  const OPTIONS: ContentOption[] = [
    {
      id: 'calendar',
      label: 'Calendar',
      title: 'Calendar — today + tomorrow (⌘⇧1)',
      iconPath: '<rect x="3" y="4" width="18" height="18" rx="2"/><line x1="16" y1="2" x2="16" y2="6"/><line x1="8" y1="2" x2="8" y2="6"/><line x1="3" y1="10" x2="21" y2="10"/>'
    },
    {
      id: 'notes',
      label: 'Notes',
      title: 'Notes — recent (⌘⇧2)',
      iconPath: '<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="8" y1="13" x2="16" y2="13"/><line x1="8" y1="17" x2="14" y2="17"/>'
    },
    {
      id: 'ai',
      label: 'AI',
      title: 'AI — launcher (⌘⇧3)',
      iconPath: '<path d="M12 3v3M12 18v3M5.6 5.6l2.1 2.1M16.3 16.3l2.1 2.1M3 12h3M18 12h3M5.6 18.4l2.1-2.1M16.3 7.7l2.1-2.1"/><circle cx="12" cy="12" r="3.5"/>'
    },
    {
      id: 'vision',
      label: 'Vision',
      title: 'Vision — pinned doc (⌘⇧4)',
      iconPath: '<circle cx="12" cy="12" r="3"/><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z"/>'
    },
    {
      id: 'widgets',
      label: 'Widgets',
      title: 'Widgets — strip (⌘⇧5)',
      iconPath: '<rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/>'
    }
  ];
</script>

<aside
  class="hidden md:flex flex-col flex-shrink-0 bg-mantle border-l border-surface1 relative h-full min-h-0"
  style="width: {$rightPaneStore.width}px"
  aria-label="Right pane"
>
  <!-- Drag handle. 4px wide hit-target on the LEFT edge with a 1px
       visible line; cursor flips to col-resize on hover. Stays
       below the header so the user can drag from anywhere along
       the left edge. -->
  <button
    type="button"
    class="absolute top-0 left-0 bottom-0 w-1 z-10 group {dragging ? 'bg-primary/40' : 'hover:bg-primary/30'} transition-colors"
    style="cursor: col-resize"
    onmousedown={startDrag}
    aria-label="Resize right pane"
  ></button>

  <!-- Header: content picker + close. -->
  <header class="flex items-center gap-0.5 px-2 py-1.5 border-b border-surface1 flex-shrink-0">
    {#each OPTIONS as opt (opt.id)}
      <button
        type="button"
        onclick={() => setRightPaneContent(opt.id)}
        title={opt.title}
        aria-label={opt.label}
        aria-pressed={$rightPaneStore.content === opt.id}
        class="w-7 h-7 flex items-center justify-center rounded transition-colors {$rightPaneStore.content === opt.id ? 'bg-surface1 text-primary' : 'text-dim hover:bg-surface0 hover:text-text'}"
      >
        <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          {@html opt.iconPath}
        </svg>
      </button>
    {/each}
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
    {/if}
  </div>
</aside>
