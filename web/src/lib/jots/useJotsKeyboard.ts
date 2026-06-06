// JotsPane page-scoped keyboard handler.
//
// Owns the single window-level keydown listener for the jots feed.
// Amplenote-style single-key navigation: j/k step through cards,
// g/G jump to top/bottom (G also pre-loads the next page so the
// user sees motion at the end of the feed), `/` focuses the
// search input, `c` focuses the composer, `?` toggles the help
// overlay. Esc is the universal "back out" — closes help first,
// then blurs the typing target, then clears filters, then clears
// the search.
//
// Pattern follows useTasksKeyboard: plain TS, a refs object that
// reads parent state via getters and dispatches via callbacks.
// Lifecycle is one register/cleanup pair; call installJotsKeyboard
// from onMount and return the cleanup.

import { isTypingTarget } from '$lib/util/isTypingTarget';

export type JotsKeyboardRefs = {
  // ── Read / mutate the current-jot cursor ────────────────────────
  /** 0-based index into the rendered cards. -1 = no cursor. */
  getCursorIdx: () => number;
  setCursorIdx: (idx: number) => void;

  // ── Help overlay ───────────────────────────────────────────────
  isHelpOpen: () => boolean;
  setHelpOpen: (v: boolean) => void;

  // ── Read state for the Esc cascade ─────────────────────────────
  hasAnyFilter: () => boolean;
  hasSearchText: () => boolean;
  clearAllFilters: () => void;
  clearSearch: () => void;

  // ── Focus targets for `/` and `c` ──────────────────────────────
  focusSearch: () => void;
  focusComposer: () => void;

  // ── End-of-feed `G` pre-loads the next page ────────────────────
  loadMore: () => void;
};

/** Bring the cursor card into view via the data-jot-date attribute
 *  the template sets on each card. Clamps to the rendered range so
 *  j past the end / k past the start no-op gracefully. */
function scrollToCard(idx: number, setCursorIdx: (i: number) => void) {
  if (typeof document === 'undefined') return;
  const cards = document.querySelectorAll<HTMLElement>('[data-jot-date]');
  if (!cards.length) return;
  const clamped = Math.max(0, Math.min(idx, cards.length - 1));
  setCursorIdx(clamped);
  // block:start lands the header just under the sticky toolbar; the
  // browser's smooth scroll handles the rest.
  cards[clamped].scrollIntoView({ behavior: 'smooth', block: 'start' });
}

/** Install the window-level keydown listener. Call from onMount and
 *  return the result so the cleanup runs on unmount. */
export function installJotsKeyboard(refs: JotsKeyboardRefs): () => void {
  function onKey(e: KeyboardEvent) {
    // Esc always honored, even inside inputs — it's the universal "back out".
    if (e.key === 'Escape') {
      if (refs.isHelpOpen()) {
        refs.setHelpOpen(false);
        e.preventDefault();
        return;
      }
      if (isTypingTarget(e.target)) {
        (e.target as HTMLElement).blur();
        return;
      }
      if (refs.hasAnyFilter()) {
        refs.clearAllFilters();
        e.preventDefault();
      } else if (refs.hasSearchText()) {
        refs.clearSearch();
        e.preventDefault();
      }
      return;
    }
    if (isTypingTarget(e.target)) return;
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    const cursorIdx = refs.getCursorIdx();
    switch (e.key) {
      case '?':
        e.preventDefault();
        refs.setHelpOpen(!refs.isHelpOpen());
        return;
      case '/':
        e.preventDefault();
        refs.focusSearch();
        return;
      case 'c':
        e.preventDefault();
        refs.focusComposer();
        return;
      case 'j':
        e.preventDefault();
        scrollToCard(cursorIdx + 1, refs.setCursorIdx);
        return;
      case 'k':
        e.preventDefault();
        scrollToCard(Math.max(0, cursorIdx - 1), refs.setCursorIdx);
        return;
      case 'g':
        e.preventDefault();
        refs.setCursorIdx(-1);
        document.getElementById('jots-scroll')?.scrollTo({ top: 0, behavior: 'smooth' });
        return;
      case 'G':
        e.preventDefault();
        // End-of-feed: load another page first so the user sees motion
        // instead of an abrupt stop, then scroll to the bottom of
        // what's currently rendered.
        refs.loadMore();
        document.getElementById('jots-scroll')?.scrollTo({
          top: document.getElementById('jots-scroll')?.scrollHeight ?? 0,
          behavior: 'smooth'
        });
        return;
    }
  }
  window.addEventListener('keydown', onKey);
  return () => window.removeEventListener('keydown', onKey);
}
