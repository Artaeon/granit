// Drag-to-resize composable for the right pane. Used by the desktop
// flex-sibling (horizontal drag → width in px) and the mobile bottom
// sheet (vertical drag → height in vh percentage). Both surfaces
// share the same gesture skeleton (start → move → end) but invert
// which axis and which screen-edge maps to "bigger"; this composable
// hides the math behind two callbacks.
//
// Caller wires:
//   - axis:        'x' for desktop horizontal, 'y' for mobile vertical
//   - onResize(n): the new value the caller should clamp + persist
//   - onPullClose: optional — receives a threshold-cross signal so the
//                  mobile sheet can dismiss when the user drags past
//                  the floor. Desktop passes nothing.
//
// Return shape: mousedown for desktop, touchstart/move/end for mobile.
// Internally tracks its own listeners + body-cursor lock so the page
// doesn't get into a half-dragged state if the mouseup fires off a
// stale element.

export type ResizeAxis = 'x' | 'y';

export interface ResizeHandleOptions {
  axis: ResizeAxis;
  /**
   * Snapshot of the current size at drag-start. For axis='x' this is
   * pixels; for axis='y' it's vh percentage. The composable doesn't
   * read or write it after start — caller owns the live value.
   */
  getStart: () => number;
  /**
   * Called with each move event. `next` is the proposed size in the
   * caller's chosen unit (px for x, vh-percent for y). Caller is
   * responsible for clamping + persisting.
   */
  onResize: (next: number) => void;
  /**
   * Mobile-only optional hook. Called once on touchend when the live
   * size at end is below `closeThreshold` (vh percent). Desktop omits.
   */
  onPullClose?: () => void;
  /**
   * Mobile floor below which the move handler stops tracking — the
   * caller's onResize is not called for sub-floor values, leaving
   * pull-to-dismiss as the only way past it. Defaults to 20 (vh).
   */
  pullFloor?: number;
  /**
   * Mobile threshold for "the user wants to dismiss" — checked at
   * touchend against the live size from getCurrent(). Defaults to 35.
   */
  closeThreshold?: number;
  /**
   * Read the live size at touchend so onPullClose can be decided.
   * Required for axis='y' when onPullClose is set.
   */
  getCurrent?: () => number;
}

export interface ResizeHandle {
  /** True while a drag is active. Read in the markup for visual state. */
  readonly dragging: boolean;
  /** Mousedown handler for the desktop horizontal handle. */
  startMouseDrag: (e: MouseEvent) => void;
  /** Touchstart handler for the mobile vertical handle. */
  startTouchDrag: (e: TouchEvent) => void;
  /** Touchmove handler for the mobile vertical handle. */
  onTouchMove: (e: TouchEvent) => void;
  /** Touchend/cancel handler for the mobile vertical handle. */
  onTouchEnd: () => void;
}

/**
 * Drag-to-resize composable. Returns mouse/touch handlers and a live
 * `dragging` flag. Uses Svelte 5 runes ($state) so the flag is reactive
 * inside the consuming component without an extra store.
 */
export function useResizeHandle(opts: ResizeHandleOptions): ResizeHandle {
  let dragging = $state(false);
  let startCoord = 0;
  let startSize = 0;

  function startMouseDrag(e: MouseEvent) {
    if (opts.axis !== 'x') return;
    e.preventDefault();
    dragging = true;
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';

    function onMove(ev: MouseEvent) {
      // Right-edge handle: new width = distance from right edge of
      // window to the pointer. Caller clamps + persists.
      const next = window.innerWidth - ev.clientX;
      opts.onResize(next);
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

  function startTouchDrag(e: TouchEvent) {
    if (opts.axis !== 'y') return;
    if (e.touches.length !== 1) return;
    dragging = true;
    startCoord = e.touches[0].clientY;
    startSize = opts.getStart();
  }

  function onTouchMove(e: TouchEvent) {
    if (opts.axis !== 'y') return;
    if (!dragging || e.touches.length !== 1) return;
    e.preventDefault();
    const dy = e.touches[0].clientY - startCoord;
    const vh = window.innerHeight;
    if (vh <= 0) return;
    // Drag down (positive dy) shrinks; drag up (negative dy) grows.
    const next = startSize - (dy / vh) * 100;
    const floor = opts.pullFloor ?? 20;
    if (next < floor) {
      // Stop tracking — touchend decides dismiss vs snap.
      return;
    }
    opts.onResize(next);
  }

  function onTouchEnd() {
    if (opts.axis !== 'y') return;
    if (!dragging) return;
    dragging = false;
    if (opts.onPullClose && opts.getCurrent) {
      const threshold = opts.closeThreshold ?? 35;
      if (opts.getCurrent() < threshold) opts.onPullClose();
    }
  }

  return {
    get dragging() {
      return dragging;
    },
    startMouseDrag,
    startTouchDrag,
    onTouchMove,
    onTouchEnd
  };
}
