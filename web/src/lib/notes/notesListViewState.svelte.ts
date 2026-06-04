// View + filter state for the notes list surface.
//
// First extraction step out of routes/notes/+page.svelte. Owns the
// view mode persistence + validation, the overflow-menu open flag +
// click-outside install, the folder/tag filters + sort key, and the
// localStorage-backed collection recipes (apply / save / delete).
//
// The free-text search query `q` stays page-local — collections need
// to read/write it (applyCollection sets a query, saveCurrentAsCollection
// snapshots it), so it threads through deps rather than living in this
// controller alongside the heavier search results state (which the
// data controller owns).
//
// Same shape as the goals/tasks/calendar controllers: getter/setter
// pairs for bindable state, a single deps bundle for cross-controller
// reads, getter-only derivations, methods for actions.

import { loadStored, loadStoredString, saveStored, saveStoredString } from '$lib/util/storage';
import { toast } from '$lib/components/toast';

export type View =
  | 'stream'
  | 'recent'
  | 'tree'
  | 'pinned'
  | 'all'
  | 'alpha'
  | 'tags'
  | 'collections'
  | 'folders'
  | 'search';

export type SortKey = 'modified' | 'created' | 'name' | 'size';

export interface Collection {
  id: string;
  name: string;
  query: string;
  tag?: string;
  folder?: string;
  sort: SortKey;
}

const VIEW_KEY = 'granit.notes.view';
const COLLECTIONS_KEY = 'granit.notes.collections';

const VALID_VIEWS: ReadonlySet<View> = new Set([
  'stream', 'recent', 'tree', 'pinned', 'all', 'alpha', 'tags', 'collections', 'folders', 'search'
]);

// Slim-header overflow menu (mirrors /tasks). Only the 4 less-used
// views live in the dropdown — primary 5 sit in the segmented
// control.
const OVERFLOW_KEYS: ReadonlySet<View> = new Set(['alpha', 'tags', 'folders', 'collections']);
const OVERFLOW_LABELS: Record<string, string> = {
  alpha: 'A–Z',
  tags: 'Tags',
  folders: 'Folders',
  collections: 'Collections'
};

function loadInitialView(): View {
  const stored = loadStoredString(VIEW_KEY, 'stream');
  return VALID_VIEWS.has(stored as View) ? (stored as View) : 'stream';
}

function isValidSort(s: unknown): s is SortKey {
  return s === 'modified' || s === 'created' || s === 'name' || s === 'size';
}

function validateCollections(raw: unknown): Collection[] {
  if (!Array.isArray(raw)) return [];
  const out: Collection[] = [];
  for (const it of raw) {
    if (!it || typeof it !== 'object') continue;
    const r = it as Record<string, unknown>;
    if (typeof r.id !== 'string' || typeof r.name !== 'string') continue;
    out.push({
      id: r.id,
      name: r.name,
      query: typeof r.query === 'string' ? r.query : '',
      tag: typeof r.tag === 'string' && r.tag ? r.tag : undefined,
      folder: typeof r.folder === 'string' && r.folder ? r.folder : undefined,
      sort: isValidSort(r.sort) ? r.sort : 'modified'
    });
  }
  return out;
}

export interface NotesListViewStateDeps {
  /** Free-text search query — owned by the page (and shared with the
   *  data controller's debounced search). applyCollection writes it;
   *  saveCurrentAsCollection reads it. */
  getQ: () => string;
  setQ: (v: string) => void;
}

export interface NotesListViewStateController {
  view: View;
  sortKey: SortKey;
  folderFilter: string;
  tagFilter: string;
  moreViewsOpen: boolean;
  collections: Collection[];

  readonly activeOverflowLabel: string;

  selectView(v: View): void;
  pickOverflowView(v: View): void;
  onMoreViewsKey(e: KeyboardEvent): void;
  applyCollection(c: Collection): void;
  saveCurrentAsCollection(): void;
  deleteCollection(id: string): void;
}

export function createNotesListViewState(
  deps: NotesListViewStateDeps
): NotesListViewStateController {
  let view = $state<View>(loadInitialView());
  $effect(() => saveStoredString(VIEW_KEY, view));

  let sortKey = $state<SortKey>('modified');

  // Folder filter — set by clicking a card in the folders view. The
  // page swaps to a flat list of just that folder's notes. '' means
  // unfiltered; '__root__' isolates vault-root files; any other value
  // is a folder prefix.
  let folderFilter = $state('');

  // Tag filter — set by a collection that pins a specific tag. Empty
  // means unfiltered. Applied alongside the folder filter in 'all'.
  let tagFilter = $state('');

  let moreViewsOpen = $state(false);

  // Click-outside dismiss for the overflow menu. Install only while
  // the menu is open so the rest of the page doesn't pay for it.
  $effect(() => {
    if (!moreViewsOpen) return;
    function onDocClick(e: MouseEvent) {
      const target = e.target as HTMLElement | null;
      if (target && target.closest('[data-more-views]')) return;
      moreViewsOpen = false;
    }
    window.addEventListener('mousedown', onDocClick);
    return () => window.removeEventListener('mousedown', onDocClick);
  });

  let collections = $state<Collection[]>(
    loadStored<Collection[]>(COLLECTIONS_KEY, [], validateCollections)
  );
  $effect(() => saveStored(COLLECTIONS_KEY, collections));

  function selectView(v: View) {
    // Clicking the All tab directly clears any folder/tag filter —
    // those are only set via Folders cards or a Collection; hitting
    // the segmented "All" on its own should mean "show everything".
    if (v === 'all' && view !== 'all') {
      folderFilter = '';
      tagFilter = '';
    }
    view = v;
  }

  function pickOverflowView(v: View) {
    view = v;
    moreViewsOpen = false;
  }

  function onMoreViewsKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      moreViewsOpen = false;
      e.stopPropagation();
    }
  }

  function applyCollection(c: Collection) {
    folderFilter = c.folder ?? '';
    tagFilter = c.tag ?? '';
    sortKey = c.sort;
    if (c.query) {
      deps.setQ(c.query);
      view = 'search';
    } else {
      deps.setQ('');
      view = 'all';
    }
  }

  function saveCurrentAsCollection() {
    const q = deps.getQ();
    const name = prompt('Name for this collection:', q.trim() || 'New collection');
    if (!name || !name.trim()) return;
    const c: Collection = {
      id: typeof crypto !== 'undefined' && 'randomUUID' in crypto
        ? crypto.randomUUID()
        : `c-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      name: name.trim(),
      query: q.trim(),
      tag: tagFilter || undefined,
      folder: folderFilter || undefined,
      sort: sortKey
    };
    collections = [...collections, c];
    toast.success('collection saved');
  }

  function deleteCollection(id: string) {
    const c = collections.find((x) => x.id === id);
    if (!c) return;
    if (!confirm(`Delete collection "${c.name}"?`)) return;
    collections = collections.filter((x) => x.id !== id);
  }

  return {
    get view() { return view; },
    set view(v) { view = v; },
    get sortKey() { return sortKey; },
    set sortKey(v) { sortKey = v; },
    get folderFilter() { return folderFilter; },
    set folderFilter(v) { folderFilter = v; },
    get tagFilter() { return tagFilter; },
    set tagFilter(v) { tagFilter = v; },
    get moreViewsOpen() { return moreViewsOpen; },
    set moreViewsOpen(v) { moreViewsOpen = v; },
    get collections() { return collections; },
    set collections(v) { collections = v; },
    get activeOverflowLabel() {
      return OVERFLOW_KEYS.has(view) ? (OVERFLOW_LABELS[view] ?? '') : '';
    },
    selectView,
    pickOverflowView,
    onMoreViewsKey,
    applyCollection,
    saveCurrentAsCollection,
    deleteCollection
  };
}
