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
// Pulled out of +page.svelte so the route's reactive graph isn't
// holding both the editor side (body, save, autosave) AND the
// reader side (scroll, headings, visited) in the same script.
//
// The page keeps `previewContainer` as a `bind:this` lvalue since
// Svelte's binding requires it. Everything else moves here; the
// page wires two thin $effects (loadFor on note-path change,
// attach when the container ref appears).

import {
  loadVisitedMap,
  recordVisitedLine,
  clearVisitedFor
} from '$lib/notes/noteHistory';

export interface PreviewScrollTracker {
  readonly visitedHeadings: Set<number>;
  readonly previewProgress: number;
  /** Mark a single heading as visited and persist. No-op on
   *  unknown path or already-visited line. */
  markVisited(notePath: string | null, line: number): void;
  /** Drop the in-memory + on-disk visited set for `notePath`. */
  resetVisited(notePath: string | null): void;
  /** Load the visited set for `notePath` (or clear if null). Call
   *  inside a page-level $effect that tracks note?.path. */
  loadFor(notePath: string | null): void;
  /** Attach the scroll listener + run an initial tick so a fresh
   *  load with the user at the top still marks the first heading.
   *  Returns the cleanup the caller must return from its $effect. */
  attach(container: HTMLElement, getNotePath: () => string | null): () => void;
}

export function createPreviewScrollTracker(): PreviewScrollTracker {
  let visitedHeadings = $state<Set<number>>(new Set());
  let previewProgress = $state(0);

  function markVisited(notePath: string | null, line: number) {
    if (!notePath) return;
    if (visitedHeadings.has(line)) return;
    visitedHeadings = recordVisitedLine(notePath, line);
  }

  function resetVisited(notePath: string | null) {
    if (!notePath) return;
    visitedHeadings = new Set();
    clearVisitedFor(notePath);
  }

  function loadFor(notePath: string | null) {
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
        // Tick every heading whose top is above the viewport's top
        // quarter (matches Outline's active-heading bias). Two
        // layout-cost optimisations on top of the rAF coalescer:
        //   1. Skip headings already in visitedHeadings — avoids the
        //      getBoundingClientRect call for lines we've already
        //      recorded. On a long doc the visited set grows
        //      monotonically as the user reads down, so most scroll
        //      frames touch ZERO new reads.
        //   2. Break on the first heading below the cutoff. Headings
        //      are in document order so once we see one with top >
        //      cutoff, every later one is also below — no point
        //      forcing N-k more layout flushes.
        const cTop = container.getBoundingClientRect().top;
        const cutoff = cTop + container.clientHeight * 0.25;
        const notePath = getNotePath();
        const els = container.querySelectorAll<HTMLElement>('[data-heading-line]');
        for (const el of els) {
          const ln = parseInt(el.dataset.headingLine ?? '', 10);
          if (!Number.isFinite(ln)) continue;
          if (visitedHeadings.has(ln)) continue;
          const top = el.getBoundingClientRect().top;
          if (top > cutoff) break;
          markVisited(notePath, ln);
        }
      });
    };

    container.addEventListener('scroll', onScroll, { passive: true });
    onScroll(); // initial tick for an at-top load
    return () => {
      container.removeEventListener('scroll', onScroll);
      if (raf) cancelAnimationFrame(raf);
    };
  }

  return {
    get visitedHeadings() {
      return visitedHeadings;
    },
    get previewProgress() {
      return previewProgress;
    },
    markVisited,
    resetVisited,
    loadFor,
    attach
  };
}
