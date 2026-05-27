// Swipe-to-action + long-press handler extracted from TaskCard. Tracks
// horizontal drag distance and surfaces a colored backing layer behind
// the card showing what will fire if the user releases now. Vertical
// movement (scroll intent) cancels the swipe so list-scrolling isn't
// accidentally hijacked.
//
// The .svelte.ts extension tells the Svelte compiler to process runes
// in this plain TS file so we can return reactive state from a regular
// function (no $state/$derived without the svelte preprocessor).

export interface SwipeGestureOptions {
  /** Pixel offset past which the action fires on release. Default 80. */
  threshold?: number;
  /** Fires when the user releases past +threshold (rightward swipe). */
  onSwipeRight?: () => void;
  /** Fires when the user releases past -threshold (leftward swipe). */
  onSwipeLeft?: () => void;
  /** Fires after holding without moving for longPressMs. Receives the
   *  touch's clientX/clientY so the caller can position a context menu. */
  onLongPress?: (x: number, y: number) => void;
  /** Long-press hold time in ms. Default 500 (matches platform convention). */
  longPressMs?: number;
}

export interface SwipeGestureHandlers {
  onTouchStart(e: TouchEvent): void;
  onTouchMove(e: TouchEvent): void;
  onTouchEnd(e: TouchEvent): void;
  /** Reactive — the current X-offset for the visual transform. */
  readonly offset: number;
  /** Reactive — true while a horizontal swipe is being tracked. */
  readonly active: boolean;
  /** The threshold the caller configured, exposed for label/visual logic. */
  readonly threshold: number;
}

export function useSwipeGesture(opts: SwipeGestureOptions = {}): SwipeGestureHandlers {
  const threshold = opts.threshold ?? 80;
  const longPressMs = opts.longPressMs ?? 500;

  let offset = $state(0);
  let active = $state(false);

  let startX = 0;
  let startY = 0;
  let longPressTimer: ReturnType<typeof setTimeout> | null = null;

  function onTouchStart(e: TouchEvent) {
    const t0 = e.touches[0];
    if (!t0) return;
    startX = t0.clientX;
    startY = t0.clientY;
    offset = 0;
    active = false;
    if (opts.onLongPress) {
      longPressTimer = setTimeout(() => {
        opts.onLongPress?.(t0.clientX, t0.clientY);
        longPressTimer = null;
      }, longPressMs);
    }
  }

  function onTouchMove(e: TouchEvent) {
    const t0 = e.touches[0];
    if (!t0) return;
    const dx = t0.clientX - startX;
    const dy = t0.clientY - startY;
    // Once the user has moved ~10px, decide whether this is a swipe
    // (horizontal) or a scroll (vertical) and lock in. If vertical
    // wins we cancel the swipe and let the list scroll naturally.
    if (!active && (Math.abs(dx) > 10 || Math.abs(dy) > 10)) {
      if (Math.abs(dx) > Math.abs(dy)) {
        active = true;
        // Cancel long-press; user is swiping, not holding.
        if (longPressTimer) { clearTimeout(longPressTimer); longPressTimer = null; }
      } else {
        // Vertical scroll — abort the swipe entirely.
        startX = NaN;
        return;
      }
    }
    if (!active || Number.isNaN(startX)) return;
    // Cap the visual offset at 1.5x threshold so the card doesn't
    // fly off-screen on a vigorous swipe — the action is committed
    // at threshold either way.
    offset = Math.max(-threshold * 1.5, Math.min(threshold * 1.5, dx));
    // Don't preventDefault on the touchmove event itself — the user
    // may still pan vertically after the lock-in moment if their
    // gesture changes direction.
  }

  function onTouchEnd(e: TouchEvent) {
    if (longPressTimer) {
      clearTimeout(longPressTimer);
      longPressTimer = null;
    }
    if (!active) return;
    const finalOffset = offset;
    offset = 0;
    active = false;
    // Commit the action if past threshold.
    if (finalOffset > threshold) {
      e.preventDefault();
      opts.onSwipeRight?.();
    } else if (finalOffset < -threshold) {
      e.preventDefault();
      opts.onSwipeLeft?.();
    }
  }

  return {
    onTouchStart,
    onTouchMove,
    onTouchEnd,
    get offset() { return offset; },
    get active() { return active; },
    get threshold() { return threshold; }
  };
}
