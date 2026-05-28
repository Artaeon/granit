// Right pane companion sidebar — Phase 1.5 of the multi-pane workspace.
//
// The right pane is a desktop/tablet column on the right edge of the
// shell that hosts a secondary view. Phase 1 shipped 5 content
// options (calendar / notes / ai / vision / widgets). Phase 1.5
// expands the picker to 10 (adds tasks / today / goals / habits /
// dashboard) and adds mobile support via a bottom-sheet variant
// driven by `mobileHeight`. On mobile (< md:) the pane slides up
// from the bottom; on desktop it sits as a flex sibling.
//
// State lives here (not inside the shell component) so any surface —
// nav-sidebar toggle button, command palette entry, keyboard shortcut
// — can flip the pane without prop-drilling. Pieces:
//
//   open         — boolean, default false. Persisted so the user's
//                  choice survives reload.
//   content      — which sub-view to render. One of ten content keys.
//                  Picker is a dropdown (was an icon row in Phase 1)
//                  because 10 items don't fit a horizontal strip.
//   width        — px, clamped 280..640 on every set. 360 default.
//                  Desktop only; the mobile sheet uses mobileHeight.
//   mobileHeight — vh percentage, clamped 30..90. 60 default. The
//                  drag-handle on the bottom-sheet variant updates
//                  this; below 35 closes the pane instead of
//                  resizing past the floor.
//
// Persistence keys are stable so a future settings UI ("right pane
// width…") can read them directly:
// granit.rightpane.{open,content,width,mobileHeight}.

import { writable } from 'svelte/store';
import { loadStored, loadStoredString, saveStored, saveStoredString } from '$lib/util/storage';

export type RightPaneContent =
  | 'calendar'
  | 'notes'
  | 'ai'
  | 'vision'
  | 'widgets'
  | 'tasks'
  | 'today'
  | 'goals'
  | 'habits'
  | 'dashboard';

export interface RightPaneState {
  open: boolean;
  content: RightPaneContent;
  /** Pane width in px. Clamped 280..640 on every set. Desktop only. */
  width: number;
  /** Mobile bottom-sheet height as a vh percentage. Clamped 30..90. */
  mobileHeight: number;
}

const OPEN_KEY = 'granit.rightpane.open';
const CONTENT_KEY = 'granit.rightpane.content';
const WIDTH_KEY = 'granit.rightpane.width';
const MOBILE_HEIGHT_KEY = 'granit.rightpane.mobileHeight';

const MIN_WIDTH = 280;
const MAX_WIDTH = 640;
const DEFAULT_WIDTH = 360;
const MIN_MOBILE_HEIGHT = 30;
const MAX_MOBILE_HEIGHT = 90;
const DEFAULT_MOBILE_HEIGHT = 60;
const DEFAULT_CONTENT: RightPaneContent = 'calendar';

const VALID_CONTENT: ReadonlySet<RightPaneContent> = new Set<RightPaneContent>([
  'calendar',
  'notes',
  'ai',
  'vision',
  'widgets',
  'tasks',
  'today',
  'goals',
  'habits',
  'dashboard'
]);

function clampWidth(w: number): number {
  if (!Number.isFinite(w)) return DEFAULT_WIDTH;
  return Math.max(MIN_WIDTH, Math.min(MAX_WIDTH, Math.round(w)));
}

function clampMobileHeight(h: number): number {
  if (!Number.isFinite(h)) return DEFAULT_MOBILE_HEIGHT;
  return Math.max(MIN_MOBILE_HEIGHT, Math.min(MAX_MOBILE_HEIGHT, Math.round(h)));
}

function loadInitial(): RightPaneState {
  const open = loadStoredString(OPEN_KEY, '0') === '1';
  const rawContent = loadStoredString(CONTENT_KEY, DEFAULT_CONTENT) as RightPaneContent;
  // Unknown values (e.g. a content key removed in a future build)
  // fall back to the default so the pane never renders a missing
  // sub-component.
  const content: RightPaneContent = VALID_CONTENT.has(rawContent) ? rawContent : DEFAULT_CONTENT;
  const width = clampWidth(loadStored<number>(WIDTH_KEY, DEFAULT_WIDTH));
  const mobileHeight = clampMobileHeight(
    loadStored<number>(MOBILE_HEIGHT_KEY, DEFAULT_MOBILE_HEIGHT)
  );
  return { open, content, width, mobileHeight };
}

export const rightPaneStore = writable<RightPaneState>(loadInitial());

// Persist on every change. Single subscriber so the writes coalesce
// naturally with svelte's diffing — content changes don't rewrite
// the width key, etc. (loadStoredString returns the prior raw value
// on a no-op set, so this is cheap).
rightPaneStore.subscribe((state) => {
  if (typeof localStorage === 'undefined') return;
  saveStoredString(OPEN_KEY, state.open ? '1' : '0');
  saveStoredString(CONTENT_KEY, state.content);
  saveStored(WIDTH_KEY, state.width);
  saveStored(MOBILE_HEIGHT_KEY, state.mobileHeight);
});

export function toggleRightPane(): void {
  rightPaneStore.update((s) => ({ ...s, open: !s.open }));
}

export function openRightPane(): void {
  rightPaneStore.update((s) => (s.open ? s : { ...s, open: true }));
}

export function closeRightPane(): void {
  rightPaneStore.update((s) => (s.open ? { ...s, open: false } : s));
}

export function setRightPaneContent(c: RightPaneContent): void {
  if (!VALID_CONTENT.has(c)) return;
  // Picking a content also opens the pane — switching to "notes" while
  // the pane is closed is the same intent as "show me notes".
  rightPaneStore.update((s) => ({ ...s, content: c, open: true }));
}

export function setRightPaneWidth(w: number): void {
  const clamped = clampWidth(w);
  rightPaneStore.update((s) => (s.width === clamped ? s : { ...s, width: clamped }));
}

export function setRightPaneMobileHeight(h: number): void {
  const clamped = clampMobileHeight(h);
  rightPaneStore.update((s) =>
    s.mobileHeight === clamped ? s : { ...s, mobileHeight: clamped }
  );
}

// Exported for tests / consumers that want to know the bounds.
export const RIGHT_PANE_MIN_WIDTH = MIN_WIDTH;
export const RIGHT_PANE_MAX_WIDTH = MAX_WIDTH;
export const RIGHT_PANE_MIN_MOBILE_HEIGHT = MIN_MOBILE_HEIGHT;
export const RIGHT_PANE_MAX_MOBILE_HEIGHT = MAX_MOBILE_HEIGHT;
