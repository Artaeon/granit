// Deep-link `?focus=<task-id>` handler. The dashboard's TodayStream
// widget links here so a click on a scheduled / due task lands
// directly on its detail drawer instead of the user having to
// scroll-and-find.
//
// Single-fire-per-change guard: only opens the drawer when the
// focus id, the task list, or the open detail target combine into
// a state we haven't already serviced. Without it a re-rendered
// task list would re-open the drawer on every load — annoying for
// the user who already dismissed it.

import type { Task } from '$lib/api';

export interface TasksFocusDeepLinkOptions {
  /** Current ?focus=<id> value from the URL. Pass a getter so the
   *  installer can re-read it without becoming reactive on
   *  unrelated $page.url changes. */
  getFocusId: () => string | null;
  /** Live task list. Reading it via a getter keeps the effect's
   *  dep set tight — we only retrigger on length / contents
   *  changes, not on $page subscription. */
  getTasks: () => Task[];
  /** Open the drawer. Same callback the pane uses for in-list
   *  clicks. */
  openDetail: (t: Task) => void;
}

export function installTasksFocusDeepLink(opts: TasksFocusDeepLinkOptions): void {
  let lastFocusedFromUrl = $state<string | null>(null);
  $effect(() => {
    const focusId = opts.getFocusId();
    const tasks = opts.getTasks();
    if (!focusId || tasks.length === 0) return;
    if (focusId === lastFocusedFromUrl) return;
    const t = tasks.find((x) => x.id === focusId);
    if (t) {
      opts.openDetail(t);
      lastFocusedFromUrl = focusId;
    }
  });
}
