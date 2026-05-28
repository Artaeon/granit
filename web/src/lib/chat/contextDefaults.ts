// Page-aware suggested-mode rule for the AI overlay.
//
// When the overlay opens for the first time on a route, we nudge the
// agent mode to the one that best fits the page (Tasks/Calendar →
// Analyst, Goals/Ventures → Coach, etc.). Only fires when the user
// hasn't actively chosen something away from "general" — see
// applyContextDefaults in AIOverlay.svelte for the gating.
//
// Lives here so the route → mode map is a single named table rather
// than buried inline. Adding new routes / changing the mapping is now
// a one-line edit in one file.

/** Returns the suggested mode id for a given pathname, or null when
 *  no rule applies. Conservative: only flips on routes where the
 *  mode shift is obviously useful. /notes/X already has its own
 *  attach-current-note logic; the rest live here. */
export function suggestedModeForPath(pathname: string): string | null {
  if (pathname.startsWith('/tasks')) return 'analyst';
  if (pathname.startsWith('/calendar')) return 'analyst';
  if (pathname.startsWith('/goals') || pathname.startsWith('/ventures')) return 'coach';
  if (pathname.startsWith('/projects')) return 'architect';
  if (pathname.startsWith('/examen')) return 'examen';
  if (
    pathname.startsWith('/prayer') ||
    pathname.startsWith('/bible') ||
    pathname.startsWith('/scripture')
  ) {
    return 'aurelius';
  }
  return null;
}
