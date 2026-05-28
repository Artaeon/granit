<script lang="ts">
  // RightPane shell — the companion sidebar that hosts a secondary
  // view alongside the main route. Phase 1.5 expands Phase 1's five
  // content options to ten and replaces the horizontal icon-row
  // picker with a dropdown (10 buttons don't fit a slim header). It
  // also adds a mobile bottom-sheet variant — the pane no longer
  // hides on small viewports; instead it slides up from the bottom
  // with a drag-handle for height resize.
  //
  // Width on desktop is driven by the rightPaneStore (persisted,
  // clamped 280..640). Height on mobile is driven by mobileHeight
  // (persisted, clamped 30..90 vh). The drag handle is on the LEFT
  // edge on desktop, top center on mobile.

  import { onMount, tick } from 'svelte';
  import {
    rightPaneStore,
    setRightPaneContent,
    setRightPaneWidth,
    setRightPaneMobileHeight,
    closeRightPane,
    type RightPaneContent
  } from '$lib/stores/rightPane';
  import { mediaQuery } from '$lib/util/mediaQuery';
  import { findBinding } from '$lib/keybindings/registry';
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

  // Desktop horizontal-resize state. The handle is a wider hit-target
  // (8px) with a 2px visible line (4px on hover/active) so users
  // actually see it and confidently click. cursor: col-resize +
  // body-level userSelect lock during the drag so the cursor doesn't
  // flicker through selectable text in the main pane.
  let dragging = $state(false);

  function startDrag(e: MouseEvent) {
    e.preventDefault();
    dragging = true;
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

  // Mobile bottom-sheet drag. Touch handlers translate the y-delta
  // into a vh percentage. Drag up = bigger (cap 90); drag down past
  // 30 → close instead of clamping at the floor, so users can swipe-
  // down to dismiss like other bottom sheets in the app.
  let mobileDragging = $state(false);
  let dragStartY = 0;
  let dragStartHeight = 0;

  function onSheetTouchStart(e: TouchEvent) {
    if (e.touches.length !== 1) return;
    mobileDragging = true;
    dragStartY = e.touches[0].clientY;
    dragStartHeight = $rightPaneStore.mobileHeight;
  }
  function onSheetTouchMove(e: TouchEvent) {
    if (!mobileDragging || e.touches.length !== 1) return;
    e.preventDefault();
    const dy = e.touches[0].clientY - dragStartY;
    const vh = window.innerHeight;
    if (vh <= 0) return;
    // Drag down (positive dy) shrinks; drag up (negative dy) grows.
    const next = dragStartHeight - (dy / vh) * 100;
    // Allow values below the clamp floor while dragging so the user
    // can swipe-down to dismiss; the close threshold sits at 35.
    if (next < 20) {
      // Stop tracking — touch-end will close the pane.
      return;
    }
    setRightPaneMobileHeight(next);
  }
  function onSheetTouchEnd() {
    if (!mobileDragging) return;
    mobileDragging = false;
    // Below 35 (close threshold) means the user pulled the sheet
    // down past the comfortable minimum — interpret as "dismiss".
    if ($rightPaneStore.mobileHeight < 35) {
      // Snap back to 60 (default) before closing so a subsequent
      // re-open lands at a usable height, not the floor.
      setRightPaneMobileHeight(60);
      closeRightPane();
    }
  }

  // Picker dropdown. Click toggles the menu; outside click + Escape
  // close it. Open state is component-local (not store-persisted) —
  // a dropdown is a transient affordance.
  let pickerOpen = $state(false);
  let pickerButtonEl: HTMLButtonElement | undefined = $state();
  let pickerMenuEl: HTMLDivElement | undefined = $state();

  function togglePicker() {
    pickerOpen = !pickerOpen;
  }
  function closePicker() {
    pickerOpen = false;
  }
  async function pick(c: RightPaneContent) {
    setRightPaneContent(c);
    closePicker();
    // Wait a tick so the dropdown collapse animation doesn't compete
    // with focus restoration on the trigger button.
    await tick();
    pickerButtonEl?.focus();
  }

  $effect(() => {
    if (!pickerOpen) return;
    function onDoc(ev: MouseEvent) {
      const t = ev.target as Node | null;
      if (!t) return;
      if (pickerMenuEl && pickerMenuEl.contains(t)) return;
      if (pickerButtonEl && pickerButtonEl.contains(t)) return;
      pickerOpen = false;
    }
    function onKey(ev: KeyboardEvent) {
      if (ev.key === 'Escape') {
        pickerOpen = false;
        pickerButtonEl?.focus();
      }
    }
    document.addEventListener('mousedown', onDoc);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onDoc);
      document.removeEventListener('keydown', onKey);
    };
  });

  // Mod glyph for chord display in the dropdown. Same convention the
  // shortcuts cheatsheet uses — ⌘ on macOS, Ctrl elsewhere — so the
  // chord shown matches what the user actually presses.
  let isMac = $state(false);
  onMount(() => {
    isMac =
      typeof navigator !== 'undefined' &&
      /Mac|iPhone|iPad/i.test(navigator.platform || navigator.userAgent);
  });
  function displayChord(keys: string): string {
    if (!keys) return '';
    const modGlyph = isMac ? '⌘' : 'Ctrl';
    const shiftGlyph = isMac ? '⇧' : 'Shift';
    return keys
      .replace(/\bMod\b/g, modGlyph)
      .replace(/\bShift\b/g, shiftGlyph)
      .replace(/\+/g, isMac ? '' : '+');
  }

  interface ContentOption {
    id: RightPaneContent;
    label: string;
    title: string;
    bindingId: string;
    /** SVG path snippet for an outline icon. Drawn at 16x16. */
    iconPath: string;
  }

  const OPTIONS: ContentOption[] = [
    {
      id: 'calendar',
      label: 'Calendar',
      title: 'Today + tomorrow events',
      bindingId: 'right-pane-calendar',
      iconPath: '<rect x="3" y="4" width="18" height="18" rx="2"/><line x1="16" y1="2" x2="16" y2="6"/><line x1="8" y1="2" x2="8" y2="6"/><line x1="3" y1="10" x2="21" y2="10"/>'
    },
    {
      id: 'notes',
      label: 'Recent notes',
      title: '15 most-recent notes',
      bindingId: 'right-pane-notes',
      iconPath: '<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="8" y1="13" x2="16" y2="13"/><line x1="8" y1="17" x2="14" y2="17"/>'
    },
    {
      id: 'ai',
      label: 'AI',
      title: 'AI launcher',
      bindingId: 'right-pane-ai',
      iconPath: '<path d="M12 3v3M12 18v3M5.6 5.6l2.1 2.1M16.3 16.3l2.1 2.1M3 12h3M18 12h3M5.6 18.4l2.1-2.1M16.3 7.7l2.1-2.1"/><circle cx="12" cy="12" r="3.5"/>'
    },
    {
      id: 'vision',
      label: 'Vision',
      title: 'Pinned vision doc',
      bindingId: 'right-pane-vision',
      iconPath: '<circle cx="12" cy="12" r="3"/><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z"/>'
    },
    {
      id: 'widgets',
      label: 'Widgets',
      title: 'Slim widget strip (3)',
      bindingId: 'right-pane-widgets',
      iconPath: '<rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/>'
    },
    {
      id: 'tasks',
      label: 'Tasks',
      title: "Today's tasks",
      bindingId: 'right-pane-tasks',
      iconPath: '<polyline points="9 11 12 14 22 4"/><path d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11"/>'
    },
    {
      id: 'today',
      label: 'Today',
      title: 'Daily note + tasks combo',
      bindingId: 'right-pane-today',
      iconPath: '<circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/>'
    },
    {
      id: 'goals',
      label: 'Goals',
      title: 'Active goals + progress',
      bindingId: 'right-pane-goals',
      iconPath: '<circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="6"/><circle cx="12" cy="12" r="2"/>'
    },
    {
      id: 'habits',
      label: 'Habits',
      title: "Today's habit check-ins",
      bindingId: 'right-pane-habits',
      iconPath: '<path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z"/>'
    },
    {
      id: 'dashboard',
      label: 'Dashboard',
      title: 'Expanded widget column (6)',
      bindingId: 'right-pane-dashboard',
      iconPath: '<rect x="3" y="3" width="18" height="18" rx="2"/><line x1="3" y1="9" x2="21" y2="9"/><line x1="9" y1="21" x2="9" y2="9"/>'
    }
  ];

  let currentOption = $derived.by(
    () => OPTIONS.find((o) => o.id === $rightPaneStore.content) ?? OPTIONS[0]
  );

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
      onmousedown={startDrag}
      aria-label="Resize right pane"
    >
      <span
        class="my-auto h-12 transition-all
          {dragging
            ? 'w-1 bg-primary'
            : 'w-0.5 bg-surface2 group-hover:w-1 group-hover:bg-primary'}"
        aria-hidden="true"
      ></span>
    </button>

    <!-- Header: dropdown picker + close. -->
    <header class="flex items-center gap-2 px-2 py-1.5 border-b border-surface1 flex-shrink-0">
      <div class="relative">
        <button
          bind:this={pickerButtonEl}
          type="button"
          onclick={togglePicker}
          aria-haspopup="menu"
          aria-expanded={pickerOpen}
          title={currentOption.title}
          class="flex items-center gap-1.5 px-2 py-1 rounded text-sm text-text hover:bg-surface0 transition-colors"
        >
          <svg viewBox="0 0 24 24" class="w-4 h-4 text-primary" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            {@html currentOption.iconPath}
          </svg>
          <span>{currentOption.label}</span>
          <svg viewBox="0 0 24 24" class="w-3 h-3 text-dim" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <polyline points="6 9 12 15 18 9"/>
          </svg>
        </button>

        {#if pickerOpen}
          <div
            bind:this={pickerMenuEl}
            role="menu"
            class="absolute left-0 top-full mt-1 z-20 min-w-[14rem] bg-surface0 border border-surface1 rounded shadow-lg py-1"
          >
            {#each OPTIONS as opt (opt.id)}
              {@const binding = findBinding(opt.bindingId)}
              {@const active = $rightPaneStore.content === opt.id}
              <button
                role="menuitem"
                type="button"
                onclick={() => pick(opt.id)}
                title={opt.title}
                class="flex items-center gap-2 w-full px-2 py-1.5 text-left text-sm transition-colors
                  {active ? 'bg-surface1 text-primary' : 'text-text hover:bg-surface1'}"
              >
                <svg viewBox="0 0 24 24" class="w-4 h-4 flex-shrink-0 {active ? 'text-primary' : 'text-dim'}" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                  {@html opt.iconPath}
                </svg>
                <span class="flex-1 truncate">{opt.label}</span>
                {#if binding}
                  <span class="text-[10px] text-dim tabular-nums flex-shrink-0">{displayChord(binding.keys)}</span>
                {/if}
              </button>
            {/each}
          </div>
        {/if}
      </div>

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
       a pull-down past 30vh closes the sheet. The backdrop is
       deliberately omitted — users keep their main view visible
       while the pane sits docked. -->
  <div
    class="md:hidden fixed left-0 right-0 bottom-0 z-30 bg-mantle border-t border-surface1 rounded-t-xl shadow-2xl flex flex-col"
    style="height: {$rightPaneStore.mobileHeight}vh; transition: {mobileDragging ? 'none' : 'height 150ms ease'};"
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
      ontouchstart={onSheetTouchStart}
      ontouchmove={onSheetTouchMove}
      ontouchend={onSheetTouchEnd}
      ontouchcancel={onSheetTouchEnd}
      aria-label="Resize right pane (drag) or pull down to close"
    >
      <span class="block h-1 w-10 rounded-full bg-surface2"></span>
    </button>

    <!-- Header: dropdown picker + close. Same picker as desktop. -->
    <header class="flex items-center gap-2 px-3 pb-2 border-b border-surface1 flex-shrink-0">
      <div class="relative">
        <button
          bind:this={pickerButtonEl}
          type="button"
          onclick={togglePicker}
          aria-haspopup="menu"
          aria-expanded={pickerOpen}
          title={currentOption.title}
          class="flex items-center gap-1.5 px-2 py-1 rounded text-sm text-text hover:bg-surface0 transition-colors"
        >
          <svg viewBox="0 0 24 24" class="w-4 h-4 text-primary" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            {@html currentOption.iconPath}
          </svg>
          <span>{currentOption.label}</span>
          <svg viewBox="0 0 24 24" class="w-3 h-3 text-dim" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <polyline points="6 9 12 15 18 9"/>
          </svg>
        </button>

        {#if pickerOpen}
          <div
            bind:this={pickerMenuEl}
            role="menu"
            class="absolute left-0 top-full mt-1 z-20 min-w-[14rem] bg-surface0 border border-surface1 rounded shadow-lg py-1"
          >
            {#each OPTIONS as opt (opt.id)}
              {@const binding = findBinding(opt.bindingId)}
              {@const active = $rightPaneStore.content === opt.id}
              <button
                role="menuitem"
                type="button"
                onclick={() => pick(opt.id)}
                title={opt.title}
                class="flex items-center gap-2 w-full px-2 py-1.5 text-left text-sm transition-colors
                  {active ? 'bg-surface1 text-primary' : 'text-text hover:bg-surface1'}"
              >
                <svg viewBox="0 0 24 24" class="w-4 h-4 flex-shrink-0 {active ? 'text-primary' : 'text-dim'}" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                  {@html opt.iconPath}
                </svg>
                <span class="flex-1 truncate">{opt.label}</span>
                {#if binding}
                  <span class="text-[10px] text-dim tabular-nums flex-shrink-0">{displayChord(binding.keys)}</span>
                {/if}
              </button>
            {/each}
          </div>
        {/if}
      </div>

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
