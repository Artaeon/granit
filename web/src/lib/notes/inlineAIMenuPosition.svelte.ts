// Menu-positioning controller + dismiss-side-effects installer for
// InlineAIMenu.
//
// Two concerns live here because they share lifetimes (mount/teardown)
// and both reach for window/document:
//
//   1. createMenuPositionController — owns the viewport-clamped
//      {left, top} the menu's fixed-position wrapper consumes. The
//      caller passes a getter for the menu's DOM element and the
//      anchor coords (the trigger event's x/y in viewport space).
//      clamp() reads the current rect, falls back to the anchor when
//      the rect is unmeasurable, and flips the menu above the anchor
//      when there isn't room below. The async library load and the
//      user's typed query both grow/shrink the menu after mount, so
//      the caller is expected to re-call clamp() from a tick()-deferred
//      $effect that watches the content shape.
//
//   2. installMenuDismissHandlers — wires the global listeners that
//      close the menu on resize OR an outside primary-button click.
//      Returns a teardown function for onMount-style use. Only the
//      primary button (e.button === 0) dismisses on outside-click so
//      a right-click in the editor still opens the OS context menu
//      against the current selection without first eating the inline
//      menu state.

export interface MenuPosition {
  left: number;
  top: number;
}

export interface MenuPositionOpts {
  /** Anchor point in viewport space — the trigger event's x/y. */
  getAnchor: () => { x: number; y: number };
  /** Returns the menu's outer element, or undefined before mount. */
  getMenuEl: () => HTMLElement | undefined;
}

export interface MenuPositionController {
  /** Reactive position the menu's fixed wrapper reads. */
  readonly pos: MenuPosition;
  /** Measure the menu against the viewport and update `pos`. Safe
   *  to call before the element is mounted (no-op). Should be invoked
   *  from a tick()-deferred $effect that watches whatever can change
   *  the menu's height (library fetch, query filter). */
  clamp(): void;
}

export function createMenuPositionController(
  opts: MenuPositionOpts
): MenuPositionController {
  let pos = $state<MenuPosition>({ left: 0, top: 0 });

  function clamp() {
    const el = opts.getMenuEl();
    if (!el) return;
    const rect = el.getBoundingClientRect();
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    const margin = 8;
    const anchor = opts.getAnchor();
    let left = anchor.x;
    let top = anchor.y;
    if (left + rect.width > vw - margin) left = vw - margin - rect.width;
    if (left < margin) left = margin;
    if (top + rect.height > vh - margin) {
      // Flip above the trigger anchor when there's no room below.
      top = Math.max(margin, anchor.y - rect.height - 28);
    }
    pos = { left, top };
  }

  return {
    get pos() {
      return pos;
    },
    clamp
  };
}

export interface MenuDismissOpts {
  /** Returns the menu's outer element. Clicks INSIDE this element
   *  don't dismiss. */
  getMenuEl: () => HTMLElement | undefined;
  /** Called on outside primary-button click and on window resize.
   *  Resize as a dismiss target is debatable, but keeps the menu's
   *  measured position from going stale during a viewport flip. */
  onResize: () => void;
  /** Called on outside primary-button click only. The menu's own
   *  parent typically wires this to `onClose`. */
  onOutsideClick: () => void;
}

/** Install resize + outside-click handlers. Returns a teardown
 *  function — call from an $effect or stash and run on destroy. */
export function installMenuDismissHandlers(opts: MenuDismissOpts): () => void {
  const onResize = () => opts.onResize();
  const onDocClick = (e: MouseEvent) => {
    const el = opts.getMenuEl();
    if (!el) return;
    // Only the primary button closes the menu on outside-click.
    // Right-click in the editor would otherwise dismiss the menu
    // before the OS spell-check menu opened, eating the chance to
    // act on the menu's current state.
    if (e.button !== 0) return;
    if (e.target instanceof Node && el.contains(e.target)) return;
    opts.onOutsideClick();
  };
  window.addEventListener('resize', onResize);
  document.addEventListener('mousedown', onDocClick);
  return () => {
    window.removeEventListener('resize', onResize);
    document.removeEventListener('mousedown', onDocClick);
  };
}
