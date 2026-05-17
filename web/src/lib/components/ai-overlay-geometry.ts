// Pure-math helpers for the AIOverlay's resize / bottom-sheet
// gestures. Extracted out of AIOverlay.svelte because the file is
// 3200+ lines and the geometry is the cleanest separable piece:
// constants + load-from-localStorage + clamp + nearest-snap logic,
// all side-effect-free apart from the storage reads/writes that
// `loadStoredString` already abstracts. The component still owns
// the pointer event handlers (they mutate reactive $state vars in
// closure) — what comes here is the math those handlers reach for.
//
// Why a `.ts` module and not a sibling Svelte component: there's no
// rendered UI here, and the values are read from multiple places in
// AIOverlay (the resize edge, the keyboard handler, the bottom-sheet
// drag handler, the $effect that mirrors width to CSS var). A util
// module keeps every call site routed through the same definitions
// without dragging Svelte runtime into a no-render concern.

import { loadStoredString, saveStoredString } from '$lib/util/storage';

// ── Desktop resizable panel ──────────────────────────────────────

export const PANEL_WIDTH_KEY = 'granit.chat.overlay.width';
export const PANEL_WIDTH_MIN = 360;
export const PANEL_WIDTH_MAX = 720;
export const PANEL_WIDTH_DEFAULT = 420;

// clampPanelWidth bounds a raw width (px) into the allowed range.
// Exported so the pointer-drag handler in the component can produce
// a width and clamp it with the same rule as the boot-time load.
export function clampPanelWidth(n: number): number {
  return Math.min(PANEL_WIDTH_MAX, Math.max(PANEL_WIDTH_MIN, n));
}

// loadPanelWidth reads the persisted width and clamps. A missing /
// malformed entry falls back to PANEL_WIDTH_DEFAULT — the user
// experiences this as "first launch landed on the default."
export function loadPanelWidth(): number {
  const n = parseInt(loadStoredString(PANEL_WIDTH_KEY, ''), 10);
  if (!Number.isFinite(n)) return PANEL_WIDTH_DEFAULT;
  return clampPanelWidth(n);
}

export function persistPanelWidth(n: number): void {
  saveStoredString(PANEL_WIDTH_KEY, String(n));
}

// Keyboard accessibility on the resize handle. Returns the next
// panel-width given the current width and the pressed key, or null
// if the key isn't a resize key (so the caller can return early
// without calling preventDefault). Bound by the same clamp.
//
// Note the seemingly inverted L/R: pulling LEFT widens the panel
// (panel is right-anchored), so ArrowLeft → +16; ArrowRight → -16.
// Home → max, End → min.
export function nextPanelWidthForKey(current: number, key: string): number | null {
  switch (key) {
    case 'ArrowLeft':
      return clampPanelWidth(current + 16);
    case 'ArrowRight':
      return clampPanelWidth(current - 16);
    case 'Home':
      return PANEL_WIDTH_MAX;
    case 'End':
      return PANEL_WIDTH_MIN;
    default:
      return null;
  }
}

// ── Mobile bottom-sheet snap points ──────────────────────────────

export type SheetSnap = 'peek' | 'mid' | 'full';
export const SHEET_SNAP_KEY = 'granit.ai.sheet.snap';
export const SHEET_SNAP_PCT: Record<SheetSnap, number> = {
  peek: 35,
  mid: 65,
  full: 92
};
// Min / max drag heights as a fraction of viewport height. Outside
// this range the drag should clamp rather than free-form. 0.18 keeps
// the handle reachable; 0.95 leaves the user a sliver of the page
// underneath so they can dismiss with a tap.
export const SHEET_DRAG_MIN_VH = 0.18;
export const SHEET_DRAG_MAX_VH = 0.95;

export function loadSheetSnap(): SheetSnap {
  const v = loadStoredString(SHEET_SNAP_KEY, 'mid');
  return v === 'peek' || v === 'full' ? v : 'mid';
}

// snapHeightPx converts a snap label into the absolute pixel height
// the panel should occupy at the supplied viewport height. The
// caller (the component on mount / resize) reads window.innerHeight
// and feeds it here so the helper stays unit-testable without a DOM.
export function snapHeightPx(snap: SheetSnap, viewportH: number): number {
  return Math.round(viewportH * SHEET_SNAP_PCT[snap] / 100);
}

// clampSheetHeight bounds a free-drag height inside [min, max] of
// the supplied viewport. Same rule as the pointer-move handler used
// to inline.
export function clampSheetHeight(h: number, viewportH: number): number {
  const min = viewportH * SHEET_DRAG_MIN_VH;
  const max = viewportH * SHEET_DRAG_MAX_VH;
  if (h < min) return min;
  if (h > max) return max;
  return h;
}

// nearestSnap returns the snap label whose target percentage is
// closest to the supplied height/viewport ratio. Used on pointer
// release to "snap" the drag-finished height to one of the three
// stops. Stable in the face of viewport changes mid-drag (e.g. iOS
// keyboard shows / hides) because it works off the final ratio, not
// raw pixels.
export function nearestSnap(heightPx: number, viewportH: number): SheetSnap {
  if (viewportH <= 0) return 'mid';
  const ratio = (heightPx / viewportH) * 100;
  let best: SheetSnap = 'mid';
  let bestDist = Infinity;
  // Iterating an explicit list keeps the tie-break deterministic
  // (peek then mid then full) instead of relying on Object.keys order.
  for (const s of ['peek', 'mid', 'full'] as SheetSnap[]) {
    const d = Math.abs(SHEET_SNAP_PCT[s] - ratio);
    if (d < bestDist) {
      bestDist = d;
      best = s;
    }
  }
  return best;
}
