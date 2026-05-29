// Overlay chrome layer for AIOverlay — desktop resize + mobile sheet
// snap/drag + iOS keyboard offset + body-scroll lock. Extracted from
// AIOverlay.svelte because none of this is "AI logic"; it's pure UI
// chrome that any future variant of the overlay (or a sibling chat
// surface that wants the same panel ergonomics) can reuse.
//
// Geometry constants and math (clamp, snap, persist) still live in
// ../ai-overlay-geometry.ts — this module is just the stateful
// orchestrator on top of those helpers.
//
// Why a `.svelte.ts` factory and not a Svelte component:
//   - There is no rendered DOM here. Every concern is state + side
//     effect (visualViewport listeners, body-position locks, CSS var
//     mirrors) plus three pointer handlers.
//   - The parent template still owns the panel wrapper element so it
//     can compose the chrome class hooks with its own transition /
//     pinned-state / sabbath gating without prop drilling.
//   - The composable shape matches the repo's existing precedents
//     (voiceDictation.svelte.ts, useResizeHandle.svelte.ts).
//
// Effect ownership: every $effect below attaches to the component
// scope of whoever calls createOverlayChrome() during its own
// initialization. Cleanups (return functions) fire on unmount and
// on dependency changes per Svelte 5 semantics.

import {
  type SheetSnap,
  SHEET_SNAP_KEY,
  clampPanelWidth,
  loadPanelWidth,
  persistPanelWidth,
  nextPanelWidthForKey,
  loadSheetSnap,
  snapHeightPx,
  clampSheetHeight,
  nearestSnap
} from '../ai-overlay-geometry';
import { saveStoredString } from '$lib/util/storage';

export interface OverlayChromeOptions {
  /** Reactive getter: is the overlay panel currently visible? */
  isOpen: () => boolean;
  /** Reactive getter: is the desktop pinned-permanent mode active? */
  isPinned: () => boolean;
  /** Reactive getter: are we below the mobile breakpoint? */
  isMobileView: () => boolean;
  /** DOM ref getter: outer panel element. Read by the mobile sheet
   *  drag handler for getBoundingClientRect at drag-start, so the
   *  parent keeps `bind:this={panelEl}` on its wrapper. */
  getPanelEl: () => HTMLElement | undefined;
}

export interface OverlayChrome {
  /** Current panel width in px (desktop resizable). */
  readonly panelWidth: number;
  /** True while a desktop resize drag is in flight. */
  readonly resizing: boolean;
  /** Mobile sheet snap label. */
  readonly sheetSnap: SheetSnap;
  /** True while a mobile sheet drag is in flight. */
  readonly sheetDragging: boolean;
  /** iOS keyboard obscured-height in px (0 when keyboard closed). */
  readonly keyboardOffset: number;
  /** True when the iOS keyboard is currently open. */
  readonly keyboardOpen: boolean;
  /** Final mobile sheet height as a CSS px string. Honours drag-in-flight. */
  readonly mobileSheetHeight: string;
  /** Cycle peek → mid → full. Bound to the mobile drag-handle click. */
  cycleSheetSnap(): void;
  /** PointerDown on the desktop left-edge resize handle. */
  onResizeStart(e: PointerEvent): void;
  /** KeyDown on the desktop resize handle (accessibility). */
  onResizeKey(e: KeyboardEvent): void;
  /** PointerDown on the mobile drag-handle bar. */
  onSheetHandleDown(e: PointerEvent): void;
}

export function createOverlayChrome(opts: OverlayChromeOptions): OverlayChrome {
  // ── Reactive state ──────────────────────────────────────────────
  let panelWidth = $state<number>(loadPanelWidth());
  let resizing = $state(false);
  let sheetSnap = $state<SheetSnap>(loadSheetSnap());
  let sheetDragHeight = $state<number | null>(null);
  let sheetDragging = $state(false);
  let keyboardOffset = $state(0);
  let keyboardOpen = $state(false);
  // Snap to restore when the keyboard closes. Null when no save is
  // pending so the restore branch doesn't fire spuriously.
  let savedSnapBeforeKeyboard: SheetSnap | null = $state(null);

  // The actual mobile sheet height we render at — drag value while a
  // drag is in flight, otherwise the snap target. visualViewport.height
  // (not innerHeight - keyboardOffset) is the snap base because iOS
  // Safari keeps innerHeight fixed when the keyboard opens while
  // Chrome Android already shrinks innerHeight — subtracting the
  // offset on Android would double-count and float the input above
  // the keyboard with a fat gap. visualViewport.height is the visible-
  // above-keyboard region on every modern mobile browser.
  let mobileSheetHeight = $derived.by(() => {
    if (typeof window === 'undefined') return `${snapHeightPx(sheetSnap, 800)}px`;
    if (sheetDragging && sheetDragHeight !== null) {
      return `${Math.round(sheetDragHeight)}px`;
    }
    const visibleH = window.visualViewport?.height ?? window.innerHeight;
    return `${snapHeightPx(sheetSnap, visibleH)}px`;
  });

  // ── Mobile body-scroll lock ──────────────────────────────────
  // When the sheet is open on mobile, lock the document body so iOS
  // Safari can't scroll the page to bring the focused composer above
  // the keyboard — that scroll is what historically dragged the
  // position: fixed panel up with it. Classic "save scrollY → fix
  // body → restore" recipe so the user lands back at the same scroll
  // position when the overlay closes. Pinned desktop panel doesn't
  // trigger this lock; mobile pinned isn't a thing.
  let savedScrollY = 0;
  $effect(() => {
    if (typeof window === 'undefined') return;
    if (opts.isOpen() && opts.isMobileView() && !opts.isPinned()) {
      savedScrollY = window.scrollY;
      const body = document.body;
      const html = document.documentElement;
      body.style.position = 'fixed';
      body.style.top = `-${savedScrollY}px`;
      body.style.left = '0';
      body.style.right = '0';
      body.style.width = '100%';
      // html overflow:hidden as second-stage defence against iOS
      // rubber-band scrolling that can leak through a fixed body in
      // some Safari builds.
      html.style.overflow = 'hidden';
      return () => {
        body.style.position = '';
        body.style.top = '';
        body.style.left = '';
        body.style.right = '';
        body.style.width = '';
        html.style.overflow = '';
        window.scrollTo(0, savedScrollY);
      };
    }
  });

  // ── --ai-pinned-w CSS variable ──────────────────────────────────
  // Pinned mode reserves a matching right gutter on <main> via this
  // variable, set on documentElement so +layout.svelte can read it.
  // Cleared (0px) when unpinned so content reclaims the space.
  $effect(() => {
    if (typeof document === 'undefined') return;
    if (opts.isPinned()) {
      document.documentElement.style.setProperty('--ai-pinned-w', `${panelWidth}px`);
    } else {
      document.documentElement.style.setProperty('--ai-pinned-w', '0px');
    }
  });

  // ── sheetSnap persistence ──────────────────────────────────
  $effect(() => saveStoredString(SHEET_SNAP_KEY, sheetSnap));

  // ── iOS keyboard offset ──────────────────────────────────
  // visualViewport shrinks when the keyboard opens; the delta against
  // window.innerHeight is the obscured strip height. We lift the
  // panel's `bottom` by that amount so the compose stays just above
  // the keyboard. ~120px threshold cleanly separates "keyboard" from
  // chrome resize (URL bar / floating UI can shrink VV by 40–80px in
  // normal scroll without the keyboard being involved).
  $effect(() => {
    if (typeof window === 'undefined') return;
    const vv = window.visualViewport;
    if (!vv) return;
    function update() {
      const obscured = Math.max(0, window.innerHeight - (vv?.height ?? window.innerHeight));
      keyboardOffset = obscured;
      keyboardOpen = obscured > 120;
    }
    vv.addEventListener('resize', update);
    vv.addEventListener('scroll', update);
    update();
    return () => {
      vv.removeEventListener('resize', update);
      vv.removeEventListener('scroll', update);
    };
  });

  // ── Snap to 'full' while keyboard is open ──────────────────────
  // Minimises moment-of-friction where the user taps in compose and
  // can barely see their own conversation history. Restores to the
  // previous snap once the keyboard closes.
  $effect(() => {
    if (keyboardOpen) {
      if (savedSnapBeforeKeyboard === null) {
        savedSnapBeforeKeyboard = sheetSnap;
        sheetSnap = 'full';
      }
    } else if (savedSnapBeforeKeyboard !== null) {
      sheetSnap = savedSnapBeforeKeyboard;
      savedSnapBeforeKeyboard = null;
    }
  });

  // ── Desktop resize ──────────────────────────────────────
  // Pointer events cover mouse + touch + pen. We capture so the user
  // can drag past the panel edge without losing the gesture if their
  // cursor strays into the chat content. Panel is right-anchored;
  // widening means pulling LEFT, which increases (innerWidth - clientX).
  function onResizeStart(e: PointerEvent) {
    e.preventDefault();
    resizing = true;
    const target = e.currentTarget as HTMLElement;
    target.setPointerCapture(e.pointerId);
    function onMove(ev: PointerEvent) {
      panelWidth = clampPanelWidth(window.innerWidth - ev.clientX);
    }
    function onUp() {
      resizing = false;
      target.releasePointerCapture(e.pointerId);
      target.removeEventListener('pointermove', onMove);
      target.removeEventListener('pointerup', onUp);
      target.removeEventListener('pointercancel', onUp);
      persistPanelWidth(panelWidth);
    }
    target.addEventListener('pointermove', onMove);
    target.addEventListener('pointerup', onUp);
    target.addEventListener('pointercancel', onUp);
  }

  // Keyboard fallback for the resize handle. ArrowLeft widens
  // (right-anchored panel), ArrowRight narrows; Home/End jump to
  // extremes. Rule lives in nextPanelWidthForKey so both surfaces
  // (mouse drag, keyboard) agree on bounds.
  function onResizeKey(e: KeyboardEvent) {
    const next = nextPanelWidthForKey(panelWidth, e.key);
    if (next === null) return;
    e.preventDefault();
    panelWidth = next;
    persistPanelWidth(next);
  }

  // ── Mobile sheet drag ──────────────────────────────────
  // Desktop has its own left-edge resize handle; ignore the mobile-
  // only drag-handle on >=md. Pulling UP grows the sheet; clientY
  // decreases as the finger moves up, so dy is negative and we
  // subtract. window.innerHeight read fresh on release in case a
  // soft keyboard shifted vh between drag start and end.
  function onSheetHandleDown(e: PointerEvent) {
    if (typeof window === 'undefined' || window.innerWidth >= 768) return;
    e.preventDefault();
    const startY = e.clientY;
    const viewportH = window.innerHeight;
    const startH =
      opts.getPanelEl()?.getBoundingClientRect().height ?? snapHeightPx(sheetSnap, viewportH);
    sheetDragging = true;
    sheetDragHeight = startH;
    const target = e.currentTarget as HTMLElement;
    target.setPointerCapture(e.pointerId);
    function move(ev: PointerEvent) {
      const dy = ev.clientY - startY;
      sheetDragHeight = clampSheetHeight(startH - dy, viewportH);
    }
    function up() {
      target.releasePointerCapture(e.pointerId);
      target.removeEventListener('pointermove', move);
      target.removeEventListener('pointerup', up);
      target.removeEventListener('pointercancel', up);
      const finalH = sheetDragHeight ?? startH;
      sheetSnap = nearestSnap(finalH, window.innerHeight);
      sheetDragHeight = null;
      sheetDragging = false;
    }
    target.addEventListener('pointermove', move);
    target.addEventListener('pointerup', up);
    target.addEventListener('pointercancel', up);
  }

  // Tap the drag-handle bar (not a drag) cycles through snaps.
  function cycleSheetSnap() {
    const order: SheetSnap[] = ['peek', 'mid', 'full'];
    const idx = order.indexOf(sheetSnap);
    sheetSnap = order[(idx + 1) % order.length];
  }

  return {
    get panelWidth() { return panelWidth; },
    get resizing() { return resizing; },
    get sheetSnap() { return sheetSnap; },
    get sheetDragging() { return sheetDragging; },
    get keyboardOffset() { return keyboardOffset; },
    get keyboardOpen() { return keyboardOpen; },
    get mobileSheetHeight() { return mobileSheetHeight; },
    cycleSheetSnap,
    onResizeStart,
    onResizeKey,
    onSheetHandleDown
  };
}
