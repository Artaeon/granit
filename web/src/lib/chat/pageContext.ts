// Page-context derivations for the AI overlay.
//
// The overlay needs to know "what is the user looking at?" to drive
// mode auto-switch, the chip strip, and the page-agent shortcut. The
// route-parsing rules are non-trivial — projects use a ?p= query
// param while a drawer is open, goals use ?focus=, calendar is path-
// prefix only, /notes paths are URL-decoded — so the rules lived
// inline in AIOverlay.svelte for years and accumulated comments.
//
// Extracted to pure functions so:
//   - the route-parsing logic stays out of the overlay god-file,
//   - each branch is unit-testable in isolation,
//   - consumers wire them into their own $derived blocks (the
//     reactive layer stays with the component, the logic doesn't).
//
// No Svelte runes here — pure (pathname, searchParams) → derived
// shape. Callers pass `$page.url.pathname` and `$page.url.searchParams`.

export interface PageAgent {
  path: string;
  label: string;
  glyph: string;
}

/** `/notes/<rel-path>` → the URL-decoded vault-relative path. Empty
 *  string on any other route. */
export function deriveCurrentNotePath(pathname: string): string {
  if (!pathname.startsWith('/notes/')) return '';
  return decodeURIComponent(pathname.slice('/notes/'.length));
}

/** `/projects` (list view, drawer open via `?p=<name>`) OR
 *  `/projects/<name>/...` (legacy deep link) → project name. Empty
 *  string otherwise. The drawer-via-query-param shape is the canonical
 *  "I'm looking at project X" signal because the project detail
 *  doesn't get a route segment of its own. */
export function deriveCurrentProjectName(pathname: string, searchParams: URLSearchParams): string {
  if (pathname === '/projects' || pathname.startsWith('/projects?')) {
    const q = searchParams.get('p');
    return q ? decodeURIComponent(q) : '';
  }
  if (pathname.startsWith('/projects/')) {
    const tail = pathname.slice('/projects/'.length);
    const name = tail.split('/')[0];
    if (name) return decodeURIComponent(name);
  }
  return '';
}

/** `/goals?focus=<id>` → the focused goal's id. Empty string on any
 *  non-goal route or when no goal is focused. The page uses ?focus
 *  rather than a path segment, so there's no pathname-tail to parse. */
export function deriveCurrentGoalId(pathname: string, searchParams: URLSearchParams): string {
  if (!pathname.startsWith('/goals')) return '';
  return searchParams.get('focus') ?? '';
}

/** True when the route is anywhere under `/calendar`. Presence alone
 *  is enough to enter Calendar Manager mode + inject the date-window
 *  prelude; no entity-id parse required. */
export function deriveOnCalendarPage(pathname: string): boolean {
  return pathname.startsWith('/calendar');
}

/** Page-agent shortcut shape — labels the "Run X Agent" button in
 *  the sidebar. Returns null on pages without an agent so the button
 *  hides cleanly. `currentProjectName` is woven into the project-page
 *  label so the chip reads "Project Agent · Foo" when a drawer is
 *  open. */
export function derivePageAgent(pathname: string, currentProjectName: string): PageAgent | null {
  if (pathname.startsWith('/tasks')) return { path: '/tasks', label: 'Task Agent', glyph: 'TA' };
  if (pathname === '/projects' || pathname.startsWith('/projects?') || pathname.startsWith('/projects/')) {
    return {
      path: '/projects',
      label: currentProjectName ? `Project Agent · ${currentProjectName}` : 'Project Agent',
      glyph: 'PA'
    };
  }
  if (pathname.startsWith('/goals')) return { path: '/goals', label: 'Goal Agent', glyph: 'GA' };
  if (pathname.startsWith('/calendar')) return { path: '/calendar', label: 'Calendar Agent', glyph: 'CA' };
  return null;
}

/** Build the goto() target that launches the page agent on the
 *  current route. Preserves existing search params so the agent
 *  opens at the user's current filter/selection state, then layers
 *  on `?agent=1` to trigger the embedded agent dialog. */
export function buildPageAgentTarget(agentPath: string, currentSearch: URLSearchParams): string {
  const params = new URLSearchParams(currentSearch);
  params.set('agent', '1');
  const qs = params.toString();
  return `${agentPath}${qs ? '?' + qs : ''}`;
}
