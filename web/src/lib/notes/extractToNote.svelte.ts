// Extract-to-note controller. Hosts the `ExtractRequest` state
// machine plus the create-then-replace flow:
//
//   1. The Editor's onExtract event fires with the selected text +
//      an apply() callback. We capture the request as $state so the
//      ExtractToNoteDialog mounts.
//   2. The dialog collects title/path/tags from the user and calls
//      confirm(). We POST the new note FIRST; only if the API call
//      succeeds do we call apply(title), which mutates the source
//      buffer to replace the selection with `[[title]]`. A failed
//      create therefore can't leave the source pointing at a note
//      that doesn't exist.
//   3. confirm() finishes with a silent save() of the source so the
//      new wikilink hits disk in the same flush as the extraction
//      provenance footer.
//
// Lives in $lib/notes (next to the dialog component) rather than
// inside the notes route because the controller is pure glue — no
// route-local state, no DOM. The factory needs only `getNote`
// (to access the source title/path) and `save` (to flush after
// successful extraction), both injected by the page.

import { api, todayISO, type Note } from '$lib/api';
import { toast } from '$lib/components/toast';
import type { ExtractRequest } from '$lib/editor/extract-note';

export interface ExtractControllerOpts {
  /** Returns the currently-loaded note, or null while a navigation
   *  is in flight. Read on confirm so we capture the latest. */
  getNote: () => Note | null;
  /** Triggered after a successful create + apply so the source's
   *  wikilink edit reaches disk in the same flush as the new note. */
  save: (opts?: { silent?: boolean }) => Promise<void>;
}

export interface ExtractController {
  /** Reactive — null when no extraction is in flight. Bound to the
   *  dialog's `request` prop. */
  readonly request: ExtractRequest | null;
  /** Wire as `onExtract` on Editor components. */
  handleExtract: (req: ExtractRequest) => void;
  /** Dialog onConfirm. Throws on api.createNote failure (the
   *  dialog surfaces the error and stays open). */
  confirm: (args: { title: string; path: string; tags: string[] }) => Promise<void>;
  /** Dialog onDismiss. Cancels the editor-side request so a future
   *  selection doesn't see stale state. */
  dismiss: () => void;
}

export function createExtractController(opts: ExtractControllerOpts): ExtractController {
  let request = $state<ExtractRequest | null>(null);

  function handleExtract(req: ExtractRequest) {
    request = req;
  }

  function dismiss() {
    request?.cancel();
    request = null;
  }

  async function confirm(args: { title: string; path: string; tags: string[] }) {
    const note = opts.getNote();
    if (!request || !note) return;
    const { title, path, tags } = args;
    const sourceTitle = note.title || note.path;
    const body = `${request.text.trim()}

---
*Extracted from [[${sourceTitle}]] on ${todayISO()}*
`;
    // Frontmatter: title + extraction provenance + optional tags.
    // Tags are written only when present so a no-tag extract doesn't
    // get a `tags: []` line cluttering the file.
    const frontmatter: Record<string, unknown> = {
      title,
      extracted_from: note.path,
      extracted_at: new Date().toISOString()
    };
    if (tags.length > 0) frontmatter.tags = tags;
    await api.createNote({ path, frontmatter, body });
    request.apply(title);
    request = null;
    await opts.save({ silent: true });
    toast.success(`Extracted to [[${title}]]`);
  }

  return {
    get request() {
      return request;
    },
    handleExtract,
    confirm,
    dismiss
  };
}
