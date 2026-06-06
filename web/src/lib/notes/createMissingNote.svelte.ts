// Empty / not-found "create note" action for the notes editor route.
//
// Surfaces the controller that backs the "Create note" button on the
// not-found state (see NoteEmptyState.svelte): clean the path (.md
// suffix), POST a new empty note, then force-reload to land on the
// fresh file. The busy gate keeps the button safe from double clicks.
//
// Also exposes `notFoundTitle` — the basename derived from the URL
// path, used as the header title above the create CTA.

import { api } from '$lib/api';
import { toast } from '$lib/components/toast';
import type { NotePipelineController } from '$lib/notes/notePipelineState.svelte';

export interface CreateMissingNoteCtl {
  readonly creatingNote: boolean;
  readonly notFoundTitle: string;
  create: () => Promise<void>;
}

export interface CreateMissingNoteOpts {
  pipe: NotePipelineController;
  /** Raw URL path (the route's $page.params.path, URI-encoded). */
  getRawPath: () => string;
  /** Reload the active note after the create. The page reuses its
   *  load() wrapper here so the post-create flow matches a fresh
   *  navigation (draft reconciliation, scroll restore, etc.). */
  load: (path: string, opts: { force?: boolean }) => Promise<unknown>;
}

export function createMissingNoteCtl(
  opts: CreateMissingNoteOpts
): CreateMissingNoteCtl {
  let creatingNote = $state(false);

  const notFoundTitle = $derived.by(() => {
    const path = opts.getRawPath();
    if (!path) return '';
    return decodeURIComponent(path).split('/').pop()?.replace(/\.md$/, '') ?? '';
  });

  async function create(): Promise<void> {
    const path = decodeURIComponent(opts.getRawPath());
    if (!path || creatingNote) return;
    const cleanPath = path.endsWith('.md') ? path : path + '.md';
    creatingNote = true;
    try {
      await api.createNote({ path: cleanPath, body: '' });
      opts.pipe.notFound = false;
      opts.pipe.lastLoadedPath = '';
      await opts.load(cleanPath, { force: true });
    } catch (e) {
      toast.error(
        `Couldn't create note: ${e instanceof Error ? e.message : String(e)}`
      );
    } finally {
      creatingNote = false;
    }
  }

  return {
    get creatingNote() { return creatingNote; },
    get notFoundTitle() { return notFoundTitle; },
    create
  };
}
