// Dispatcher for the `granit-action` chips the assistant emits.
// Extracted out of AIOverlay.svelte so the per-type field shaping
// (task default notePath, event time-string splitting, note slug,
// memory tags) lives next to the parser that produced ParsedAction
// instead of buried inside the overlay's send/render loop.
//
// The dispatcher is intentionally side-effecting — it calls api.*
// directly because the work IS the API call. The host component
// passes in (a) the api object, (b) a toast surface, (c) page-aware
// context for default folders / notePaths. Anything more abstract
// would just push the same plumbing one layer up without simplifying
// it.

import { type ParsedAction } from '$lib/chat/actionParser';
import { errorMessage } from '$lib/util/errorMessage';

export interface CommitActionDeps {
  api: {
    createTask: (body: {
      notePath: string;
      text: string;
      dueDate?: string;
      priority?: number;
      section?: string;
    }) => Promise<unknown>;
    createEvent: (ev: {
      title: string;
      date: string;
      start_time?: string;
      end_time?: string;
      location?: string;
    }) => Promise<unknown>;
    createNote: (body: {
      path: string;
      frontmatter?: Record<string, unknown>;
      body: string;
    }) => Promise<unknown>;
    addAIMemory: (content: string, tags?: string[]) => Promise<unknown>;
  };
  toast: {
    success: (msg: string, opts?: { action?: { label: string; href: string } }) => void;
    error: (msg: string) => void;
  };
  /** Used to fill task.notePath when the assistant didn't supply
   *  one and the user is currently looking at a note. Empty string
   *  means "no current note" and the dispatcher falls back to the
   *  daily note path the caller supplies. */
  currentNotePath: string;
  /** Path of today's daily note — caller passes `Daily/${todayISO()}.md`.
   *  Splitting this out so the dispatcher stays date-aware without
   *  reaching for todayISO() itself (the test surface would have
   *  to mock the clock otherwise). */
  defaultDailyNotePath: string;
  /** Optional hook fired after a `remember` action commits, so the
   *  caller can refresh its in-memory AIMemoryFact list. */
  onMemoryAdded?: () => void | Promise<void>;
}

/**
 * Commit a single ParsedAction. Returns true on success, false on
 * failure (the caller flips its `committed` map only on true so a
 * failed click can be retried).
 */
export async function commitParsedAction(
  a: ParsedAction,
  deps: CommitActionDeps
): Promise<boolean> {
  try {
    if (a.type === 'task') {
      // Default notePath: the user's current page when it's a note
      // (so the task lives where they're working), else today's
      // daily. The create-task endpoint auto-materialises the
      // daily note when it doesn't exist yet.
      const np = a.notePath || (deps.currentNotePath || deps.defaultDailyNotePath);
      await deps.api.createTask({
        notePath: np,
        text: a.text,
        dueDate: a.dueDate || undefined,
        priority: a.priority,
        section: '## Tasks'
      });
      deps.toast.success(`Task added · ${a.text.slice(0, 50)}`);
      return true;
    }
    if (a.type === 'event') {
      // Floating ISO ("2026-05-12T13:00:00") parses cleanly as
      // local; if the model emitted Z we strip the offset so we
      // store the wall-clock the user typed, matching how native
      // events round-trip.
      const startStr = a.start.replace(/Z$/, '').replace(/[+-]\d{2}:\d{2}$/, '');
      const startDate = startStr.slice(0, 10);
      const startTime = startStr.slice(11, 16);
      const endStr = (a.end ?? '').replace(/Z$/, '').replace(/[+-]\d{2}:\d{2}$/, '');
      const endTime = endStr.slice(11, 16);
      await deps.api.createEvent({
        title: a.title,
        date: startDate,
        start_time: startTime,
        end_time: endTime || undefined,
        location: a.location || undefined
      });
      deps.toast.success(`Event added · ${a.title}`);
      return true;
    }
    if (a.type === 'note') {
      const folder = (a.folder ?? '').trim();
      const slug = a.title.replace(/[^\w\s-]/g, '').replace(/\s+/g, '-').toLowerCase() || 'note';
      const path = folder ? `${folder.replace(/\/+$/, '')}/${slug}.md` : `${slug}.md`;
      await deps.api.createNote({
        path,
        frontmatter: { title: a.title },
        body: a.body
      });
      deps.toast.success(`Note created · ${path}`, {
        action: { label: 'Open', href: `/notes/${encodeURIComponent(path)}` }
      });
      return true;
    }
    if (a.type === 'remember') {
      await deps.api.addAIMemory(a.content, a.tags);
      deps.toast.success('Saved to memory');
      await deps.onMemoryAdded?.();
      return true;
    }
    return false;
  } catch (err) {
    deps.toast.error('Action failed: ' + errorMessage(err));
    return false;
  }
}
