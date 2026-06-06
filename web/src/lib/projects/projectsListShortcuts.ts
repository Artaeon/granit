// Keyboard shortcuts + one-shot URL-param consumers for the projects
// LIST page.
//
// Sixth extraction step out of routes/projects/+page.svelte. Two
// orthogonal side-effects bundled into one install helper because
// they share a lifecycle (the page mounts once, both want a single
// cleanup):
//
//   1. consumeAgentLaunchParam — the chat sidebar's "Run Project
//      Agent" entry deep-links to /projects?agent=1. Without
//      consuming the param the agent would re-open on every navigate
//      back to the page; we strip ?agent=1 once so a reload won't
//      keep re-launching.
//
//   2. installProjectsListShortcuts — 'a' opens the Project Agent,
//      mirroring the /tasks hotkey contract. isTypingTarget guard
//      suppresses while the user is typing in inputs / textareas so
//      the letter still types literally inside any form field on
//      the page.
//
// Pure event wiring; both delegate to the page's existing setters
// via the deps bundle — same shape installNoteShortcuts uses on the
// notes route.

import { isTypingTarget } from '$lib/util/isTypingTarget';

export interface ProjectsListShortcutsDeps {
  /** Open the Project Agent overlay. Called by both the 'a' hotkey
   *  and the ?agent=1 URL deep-link. */
  openAgent: () => void;
}

/** Install the 'a' = open-Project-Agent hotkey. Returns a teardown
 *  that removes the listener — wire into onMount's cleanup. */
export function installProjectsListShortcuts(
  deps: ProjectsListShortcutsDeps
): () => void {
  const onKey = (e: KeyboardEvent) => {
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    if (isTypingTarget(e.target)) return;
    if (e.key === 'a') {
      deps.openAgent();
      e.preventDefault();
    }
  };
  window.addEventListener('keydown', onKey);
  return () => window.removeEventListener('keydown', onKey);
}

export interface ConsumeAgentLaunchParamDeps {
  /** Current URLSearchParams from $page.url. */
  getSearchParams: () => URLSearchParams;
  /** SvelteKit navigation, wrapped to keep this helper free of
   *  $app/navigation. */
  navigate: (url: string, opts?: { replaceState?: boolean; keepFocus?: boolean }) => void;
  /** Open the Project Agent overlay. */
  openAgent: () => void;
}

/**
 * If ?agent=1 is present, open the agent and strip the param so a
 * later reload / replaceState navigation doesn't re-trigger it.
 * Safe to call multiple times — the second call is a no-op since
 * the param is already gone.
 */
export function consumeAgentLaunchParam(deps: ConsumeAgentLaunchParamDeps) {
  if (deps.getSearchParams().get('agent') !== '1') return;
  deps.openAgent();
  const params = new URLSearchParams(deps.getSearchParams());
  params.delete('agent');
  deps.navigate(`/projects${params.toString() ? '?' + params : ''}`, {
    replaceState: true,
    keepFocus: true
  });
}
