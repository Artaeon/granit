// Wires the previewScrollTracker into the notes editor route.
//
// The tracker itself (previewScrollTracker.svelte) owns the visited-
// headings $state + the rAF-throttled scroll listener. This thin
// installer owns the two $effects that bridge the tracker to the
// route's reactive state:
//
//   1. loadFor — refresh the visited set when the active note path
//      changes (or clear it when no note is loaded).
//   2. attach — install the scroll listener once the preview
//      container ref appears. attach() returns its own cleanup
//      which the $effect re-runs on container swap.
//
// resetVisited is forwarded as-is so the rail's "reset reading
// progress" button stays one click away.

import { createPreviewScrollTracker } from '$lib/notes/previewScrollTracker.svelte';

export interface PreviewScrollWiring {
  readonly visitedHeadings: Set<number>;
  readonly previewProgress: number;
  resetVisited: () => void;
}

export interface PreviewScrollWiringOpts {
  getNotePath: () => string | null;
  getContainer: () => HTMLElement | null;
}

export function installPreviewScrollWiring(
  opts: PreviewScrollWiringOpts
): PreviewScrollWiring {
  const tracker = createPreviewScrollTracker();

  $effect(() => {
    tracker.loadFor(opts.getNotePath());
  });
  $effect(() => {
    const c = opts.getContainer();
    if (!c) return;
    return tracker.attach(c, opts.getNotePath);
  });

  return {
    get visitedHeadings() { return tracker.visitedHeadings; },
    get previewProgress() { return tracker.previewProgress; },
    resetVisited: () => tracker.resetVisited(opts.getNotePath())
  };
}
