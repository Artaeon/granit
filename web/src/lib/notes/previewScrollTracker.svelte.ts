// Preview-pane scroll observation for the notes route page.
//
// Owns three coupled pieces:
//
//   visitedHeadings: per-note Set of line numbers the reader has
//                    scrolled past with the top edge. Persisted via
//                    noteHistory; the in-memory Set mirrors the
//                    on-disk slice for the current note.
//   previewProgress: 0..1 fraction of the preview's scroll position
//                    — drives the thin tinted bar at the top of the
//                    preview pane.
//   scroll listener: rAF-throttled handler that updates both above,
//                    plus the heading-checkpoint walk that ticks
//                    every heading that has passed above the
//                    viewport's top quarter.
//
// The page wires two thin $effects: loadFor on note-path change,
// attach when the container ref appears. Their order is not
// guaranteed across separate $effect blocks — see `loadedForPath`
// below for why that matters.

import {
  loadVisitedMap,
  recordVisitedLine,
  clearVisitedFor
} from '$lib/notes/noteHistory';

export interface PreviewScrollTracker {
  readonly visitedHeadings: Set<number>;
  readonly previewProgress: number;
  /** Drop the in-memory + on-disk visited set for `notePath`. */
  resetVisited(notePath: string | null): void;
  /** Load the visited set for `notePath` (or clear if null). Page
   *  calls inside a $effect that tracks note?.path. */
  loadFor(notePath: string | null): void;
  /** Attach the scroll listener + run an initial deferred tick.
   *  Returns the cleanup the caller must return from its $effect. */
  attach(container: HTMLElement, getNotePath: () => string | null): () => void;
}

export function createPreviewScrollTracker(): PreviewScrollTracker {
  let visitedHeadings = $state<Set<number>>(new Set());
  let previewProgress = $state(0);
  // The note-path whose visited set is currently loaded into
  // `visitedHeadings`. attach()'s scroll handler refuses to record
  // ticks unless the active container's notePath matches — without
  // this, the unguaranteed order of the loadFor + attach effects
  // on a note swap could fire the initial scroll tick against the
  // PREVIOUS note's visited set, writing phantom visit data into
  // the new note's storage.
  let loadedForPath: string | null = null;

  function resetVisited(notePath: string | null) {
    if (!notePath) return;
    visitedHeadings = new Set();
    clearVisitedFor(notePath);
  }

  function loadFor(notePath: string | null) {
    loadedForPath = notePath;
    if (!notePath) {
      visitedHeadings = new Set();
      return;
    }
    visitedHeadings = new Set(loadVisitedMap()[notePath] ?? []);
  }

  function attach(container: HTMLElement, getNotePath: () => string | null): () => void {
    let raf = 0;

    const onScroll = () => {
      if (raf) return;
      raf = requestAnimationFrame(() => {
        raf = 0;
        const denom = Math.max(1, container.scrollHeight - container.clientHeight);
        previewProgress = Math.max(0, Math.min(1, container.scrollTop / denom));
        // Only record visited ticks against the path whose set is
        // actually loaded. Drops the walk entirely on a not-yet-
        // matched path — cheap, and avoids any chance of writing
        // line numbers from a fresh DOM into a stale storage slot.
        const notePath = getNotePath();
        if (!notePath || notePath !== loadedForPath) return;
        // Tick every heading whose top is above the viewport's top
        // quarter (matches Outline's active-heading bias). Two
        // layout-cost optimisations:
        //   1. Skip headings already in visitedHeadings — avoids the
        //      getBoundingClientRect call for lines we've already
        //      recorded. On a long doc the visited set grows
        //      monotonically as the user reads down, so most scroll
        //      frames touch ZERO new reads.
        //   2. Break on the first heading below the cutoff. Headings
        //      are in document order so once we see one with top >
        //      cutoff, every later one is also below.
        const cTop = container.getBoundingClientRect().top;
        const cutoff = cTop + container.clientHeight * 0.25;
        const els = container.querySelectorAll<HTMLElement>('[data-heading-line]');
        for (const el of els) {
          const ln = parseInt(el.dataset.headingLine ?? '', 10);
          if (!Number.isFinite(ln)) continue;
          if (visitedHeadings.has(ln)) continue;
          const top = el.getBoundingClientRect().top;
          if (top > cutoff) break;
          visitedHeadings = recordVisitedLine(notePath, ln);
        }
      });
    };

    container.addEventListener('scroll', onScroll, { passive: true });
    // Defer the initial tick by one frame so layout has settled. The
    // immediate variant thrashed when re-toggling preview onto a
    // cache-hit body — html landed in the DOM in the same tick as
    // the getBoundingClientRect walk.
    const initRaf = requestAnimationFrame(onScroll);
    return () => {
      container.removeEventListener('scroll', onScroll);
      if (raf) cancelAnimationFrame(raf);
      cancelAnimationFrame(initRaf);
    };
  }

  return {
    get visitedHeadings() {
      return visitedHeadings;
    },
    get previewProgress() {
      return previewProgress;
    },
    resetVisited,
    loadFor,
    attach
  };
}
