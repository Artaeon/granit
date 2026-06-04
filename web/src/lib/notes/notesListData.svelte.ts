// Loaded data + derivations for the notes list surface.
//
// Second extraction step out of routes/notes/+page.svelte. Owns the
// three loaded buckets (notes / pinned / loading flag), the debounced
// free-text search (q + searchResults + searching), every view-shape
// derivation, and the row-level mutations (open / togglePin / del /
// rename).
//
// The page still owns the onMount install ordering (WS subscription,
// visibility refresh, ⌘N shortcut), the deep-link / share-target URL
// handling, and the capture flow's reload coupling. Each one calls
// into dataCtl.loadAll() or dataCtl.reload — same shape every other
// route follows.
//
// Derivations that operate over a specific view-shape (alpha / tags
// / folders / all / stream) are gated on `view === ...` so a single
// WS-driven reload doesn't synchronously rebuild every bucket — only
// the visible one runs. Cheap O(n) counts power the tab strip.

import { goto } from '$app/navigation';
import { api, type Note } from '$lib/api';
import { createCoalescedReload } from '$lib/util/coalesce';
import { toast } from '$lib/components/toast';
import type { SortKey, View } from './notesListViewState.svelte';

export interface StreamSection {
  id: string;
  label: string;
  notes: Note[];
}

export interface AlphaSection {
  letter: string;
  notes: Note[];
}

export interface FolderCard {
  name: string;
  count: number;
  recentTitle: string;
  recentModTime: string;
  isRoot: boolean;
}

export interface TagSection {
  tag: string;
  notes: Note[];
  untagged: boolean;
}

export interface NotesListDataDeps {
  /** Boolean snapshot of the auth store — guards loadAll(). The page
   *  passes () => !!$auth so the read stays reactive in the calling
   *  context. */
  isAuthed: () => boolean;
  /** Read the active view mode (drives view-gated derivations). */
  getView: () => View;
  /** Read the folder filter (applied in 'all' view). */
  getFolderFilter: () => string;
  /** Read the tag filter (applied in 'all' view). */
  getTagFilter: () => string;
  /** Read the active sort key (drives the 'all' view ordering). */
  getSortKey: () => SortKey;
}

export interface NotesListDataController {
  notes: Note[];
  pinned: Set<string>;
  loading: boolean;
  q: string;
  searchResults: Note[];
  searching: boolean;

  readonly pinnedCount: number;
  readonly pinnedList: Note[];
  readonly recent: Note[];
  readonly allSorted: Note[];
  readonly streamSections: StreamSection[];
  readonly alphaSections: AlphaSection[];
  readonly folderCards: FolderCard[];
  readonly tagSections: TagSection[];
  readonly topTags: string[];
  readonly folderCount: number;
  readonly tagCount: number;
  readonly activeList: Note[];

  /** Fetch notes + pinned set in parallel. Failures of the pinned
   *  sidecar fall back to an empty set so the main list stays usable. */
  loadAll(): Promise<void>;
  /** Coalesced reload wrapper — used by the page's WS subscription. */
  reload: { trigger: () => void; flush: () => void; cancel: () => void };

  open(n: Note): void;
  togglePin(n: Note): Promise<void>;
  del(n: Note): Promise<void>;
  rename(n: Note): Promise<void>;
}

// Helper — apply the folder card filter to a note list. '__root__'
// matches notes with no slash in their path; any other value is a
// top-level folder prefix.
function passesFolderFilter(n: Note, folderFilter: string): boolean {
  if (!folderFilter) return true;
  if (folderFilter === '__root__') return n.path.indexOf('/') === -1;
  return n.path.startsWith(folderFilter + '/');
}
function passesTagFilter(n: Note, tagFilter: string): boolean {
  if (!tagFilter) return true;
  return !!n.tags && n.tags.indexOf(tagFilter) !== -1;
}

export function createNotesListData(deps: NotesListDataDeps): NotesListDataController {
  let notes = $state<Note[]>([]);
  let pinned = $state<Set<string>>(new Set());
  let loading = $state(false);

  // Search state — debounced via $effect that wakes when q changes.
  let q = $state('');
  let searchResults = $state<Note[]>([]);
  let searching = $state(false);
  let searchTimer: ReturnType<typeof setTimeout> | null = null;

  async function loadAll() {
    if (!deps.isAuthed()) return;
    loading = true;
    try {
      const [list, p] = await Promise.all([
        api.listNotes({ limit: 5000 }),
        api.listPinned().catch(() => ({ pinned: [] }))
      ]);
      notes = list.notes;
      pinned = new Set(p.pinned.map((x) => x.path));
    } finally {
      loading = false;
    }
  }

  // Coalesced reload — the editor's autosave can fire `note.changed`
  // every couple of seconds while a user types. A naive loadAll() per
  // event refetches up to 5000 notes + listPinned and rebuilds every
  // $derived view (recent / allSorted / pinnedList / activeList) on
  // every tick, freezing the page on mid-sized vaults. One trailing-
  // edge reload per window suffices: the user doesn't need sub-second
  // freshness on a list panel.
  // See $lib/util/coalesce for the canonical implementation.
  const reload = createCoalescedReload(() => loadAll(), 600);

  // Debounced search — fires 250ms after the user stops typing. We
  // re-fetch instead of filtering locally because /api/v1/search uses
  // the body-aware index, not just titles.
  $effect(() => {
    const query = q.trim();
    if (searchTimer) clearTimeout(searchTimer);
    if (!query) { searchResults = []; searching = false; return; }
    searching = true;
    searchTimer = setTimeout(async () => {
      try {
        const r = await api.search(query, 50);
        // Map hits back to notes by path so we can render uniformly.
        const byPath = new Map(notes.map((n) => [n.path, n]));
        searchResults = r.results
          .map((h) => byPath.get(h.path))
          .filter((n): n is Note => !!n);
      } catch {
        searchResults = [];
      } finally {
        searching = false;
      }
    }, 250);
  });

  // ---- derived lists per view ----
  //
  // Performance note: each heavy view (alpha / tags / folders / all)
  // walks all 5000+ notes and either sorts or buckets them. If we
  // computed every one of these unconditionally, a single WS-driven
  // loadAll() would re-run all five derivations synchronously — felt
  // like a UI freeze when typing in another tab. We now gate each
  // heavy derivation on `view === ...` so only the visible one
  // re-runs when notes change. Tab counts use cheap O(1) approximations
  // (notes.length / pinnedCount) instead of consuming the heavy
  // derivations — counts only need to be roughly right for the UI cue.

  // O(notes) but a single pass with no allocation — much cheaper
  // than the full filter+derived list when we only need the count
  // for the tab strip. The Set lookup is O(1).
  const pinnedCount = $derived.by(() => {
    let c = 0;
    for (const n of notes) if (pinned.has(n.path)) c++;
    return c;
  });
  const pinnedList = $derived.by(() => {
    if (deps.getView() !== 'pinned') return [];
    return notes.filter((n) => pinned.has(n.path));
  });

  const recent = $derived.by(() => {
    if (deps.getView() !== 'recent') return [];
    return [...notes]
      .sort((a, b) => (a.modTime > b.modTime ? -1 : 1))
      .slice(0, 30);
  });

  const allSorted = $derived.by(() => {
    if (deps.getView() !== 'all') return [];
    const folderFilter = deps.getFolderFilter();
    const tagFilter = deps.getTagFilter();
    const sortKey = deps.getSortKey();
    const arr = notes.filter(
      (n) => passesFolderFilter(n, folderFilter) && passesTagFilter(n, tagFilter)
    );
    arr.sort((a, b) => {
      switch (sortKey) {
        case 'modified': return a.modTime > b.modTime ? -1 : 1;
        case 'created': {
          const ac = (a.frontmatter?.created as string) || a.modTime;
          const bc = (b.frontmatter?.created as string) || b.modTime;
          return ac > bc ? -1 : 1;
        }
        case 'name': return a.title.localeCompare(b.title);
        case 'size': return (b.size ?? 0) - (a.size ?? 0);
      }
    });
    return arr;
  });

  // Stream view — buckets by recency window. "Today / Yesterday / This
  // week / Earlier this month / Older". Each bucket is reverse-chrono.
  // Cutoffs are computed once per derivation (cheap) — we don't
  // memoize against a clock, so the buckets are correct as of the
  // last render, which is good enough for a list view.
  const streamSections = $derived.by<StreamSection[]>(() => {
    if (deps.getView() !== 'stream') return [];
    const now = new Date();
    const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    const startOfYesterday = new Date(startOfToday.getTime() - 86_400_000);
    // "This week" = the rest of the current ISO week back to Monday,
    // not counting today/yesterday (which have their own buckets).
    // We treat Monday as the week boundary. dow 0=Sun, 1=Mon … 6=Sat.
    const dow = startOfToday.getDay();
    const daysSinceMonday = (dow + 6) % 7; // Mon→0, Sun→6
    const startOfWeek = new Date(startOfToday.getTime() - daysSinceMonday * 86_400_000);
    const startOfMonth = new Date(now.getFullYear(), now.getMonth(), 1);

    const today: Note[] = [];
    const yesterday: Note[] = [];
    const week: Note[] = [];
    const month: Note[] = [];
    const older: Note[] = [];

    for (const n of notes) {
      const t = Date.parse(n.modTime);
      if (Number.isNaN(t)) { older.push(n); continue; }
      if (t >= startOfToday.getTime()) today.push(n);
      else if (t >= startOfYesterday.getTime()) yesterday.push(n);
      else if (t >= startOfWeek.getTime()) week.push(n);
      else if (t >= startOfMonth.getTime()) month.push(n);
      else older.push(n);
    }
    const cmp = (a: Note, b: Note) => (a.modTime > b.modTime ? -1 : 1);
    today.sort(cmp); yesterday.sort(cmp); week.sort(cmp); month.sort(cmp); older.sort(cmp);
    const out: StreamSection[] = [];
    if (today.length) out.push({ id: 'today', label: 'Today', notes: today });
    if (yesterday.length) out.push({ id: 'yesterday', label: 'Yesterday', notes: yesterday });
    if (week.length) out.push({ id: 'week', label: 'This week', notes: week });
    if (month.length) out.push({ id: 'month', label: 'Earlier this month', notes: month });
    if (older.length) out.push({ id: 'older', label: 'Older', notes: older });
    return out;
  });

  // Alphabetical view — A–Z with letter dividers. Notes whose title
  // starts with a non-letter (numbers, emoji, punctuation) bucket into
  // a single "#" section so the alphabet stays clean. Useful when the
  // user remembers a title but not its folder, and "all → sort by
  // name" doesn't visually break the wall of titles into something
  // scan-friendly.
  const alphaSections = $derived.by<AlphaSection[]>(() => {
    if (deps.getView() !== 'alpha') return [];
    const buckets = new Map<string, Note[]>();
    for (const n of notes) {
      const first = (n.title || n.path).trim().charAt(0).toUpperCase();
      const letter = /[A-Z]/.test(first) ? first : '#';
      const bucket = buckets.get(letter);
      if (bucket) bucket.push(n);
      else buckets.set(letter, [n]);
    }
    const out: AlphaSection[] = [];
    for (const [letter, list] of buckets) {
      list.sort((a, b) => a.title.localeCompare(b.title));
      out.push({ letter, notes: list });
    }
    out.sort((a, b) => {
      // '#' floats to the end; letters sort A→Z.
      if (a.letter === '#') return 1;
      if (b.letter === '#') return -1;
      return a.letter.localeCompare(b.letter);
    });
    return out;
  });

  // Folder-card grid — top-level folders rendered as tappable cards
  // with note counts and the most-recent note title underneath. Acts
  // as a high-level navigation overview when the user wants to step
  // into a section without scrolling the full tree. Clicking a card
  // jumps to the tree view with that folder pre-expanded (via a
  // hash fragment we read on mount). Vault-root notes get their own
  // card so they aren't invisible.
  const folderCards = $derived.by<FolderCard[]>(() => {
    if (deps.getView() !== 'folders') return [];
    const buckets = new Map<string, { notes: Note[]; isRoot: boolean }>();
    for (const n of notes) {
      const slash = n.path.indexOf('/');
      const top = slash === -1 ? '' : n.path.slice(0, slash);
      const key = top || '__root__';
      const isRoot = top === '';
      const bucket = buckets.get(key);
      if (bucket) bucket.notes.push(n);
      else buckets.set(key, { notes: [n], isRoot });
    }
    const out: FolderCard[] = [];
    for (const [key, b] of buckets) {
      b.notes.sort((a, b) => (a.modTime > b.modTime ? -1 : 1));
      const top = b.notes[0];
      out.push({
        name: b.isRoot ? '/' : key,
        count: b.notes.length,
        recentTitle: top?.title ?? '',
        recentModTime: top?.modTime ?? '',
        isRoot: b.isRoot
      });
    }
    out.sort((a, b) => {
      // Root last, then by count desc, then alphabetical.
      if (a.isRoot !== b.isRoot) return a.isRoot ? 1 : -1;
      const dc = b.count - a.count;
      return dc !== 0 ? dc : a.name.localeCompare(b.name);
    });
    return out;
  });

  // Cheap O(n) counts for the tab strip — single pass, no allocation,
  // no sorting. These avoid forcing the heavy folderCards / tagSections
  // / alphaSections derivations to run when the user isn't viewing
  // them. The only state we actually need for the tab badge is the
  // unique-bucket count, which we get without materializing buckets.
  const folderCount = $derived.by(() => {
    const seen = new Set<string>();
    for (const n of notes) {
      const slash = n.path.indexOf('/');
      seen.add(slash === -1 ? '__root__' : n.path.slice(0, slash));
    }
    return seen.size;
  });
  const tagCount = $derived.by(() => {
    const seen = new Set<string>();
    let hasUntagged = false;
    for (const n of notes) {
      if (n.tags && n.tags.length > 0) seen.add(n.tags[0]);
      else hasUntagged = true;
    }
    return seen.size + (hasUntagged ? 1 : 0);
  });

  // Top tags (used to hint the AI; also a handy cheap stat). Counts
  // every tag (not just primary) so synonyms surface. Capped at 30
  // because the AI prompt is fed this list — more than that and we
  // burn context window for diminishing return.
  const topTags = $derived.by<string[]>(() => {
    const counts = new Map<string, number>();
    for (const n of notes) {
      if (!n.tags) continue;
      for (const t of n.tags) counts.set(t, (counts.get(t) ?? 0) + 1);
    }
    return [...counts.entries()]
      .sort((a, b) => b[1] - a[1])
      .slice(0, 30)
      .map(([t]) => t);
  });

  // Tag-grouped view — bucket each note under its primary tag (the
  // first entry in `note.tags`). Notes without tags collect under a
  // single "untagged" bucket that sorts last so the meaningful tags
  // surface first. The user typically curates tags as topics; this
  // view answers "show me everything tagged #idea" without typing a
  // search. Buckets sort by note count desc, then alphabetically — a
  // big tag jumps to the top, ties resolve predictably.
  const tagSections = $derived.by<TagSection[]>(() => {
    if (deps.getView() !== 'tags') return [];
    const buckets = new Map<string, Note[]>();
    let untagged: Note[] = [];
    for (const n of notes) {
      const primary = n.tags && n.tags.length > 0 ? n.tags[0] : null;
      if (!primary) {
        untagged.push(n);
        continue;
      }
      const bucket = buckets.get(primary);
      if (bucket) bucket.push(n);
      else buckets.set(primary, [n]);
    }
    const out: TagSection[] = [];
    for (const [tag, list] of buckets) {
      list.sort((a, b) => (a.modTime > b.modTime ? -1 : 1));
      out.push({ tag, notes: list, untagged: false });
    }
    out.sort((a, b) => {
      const dc = b.notes.length - a.notes.length;
      return dc !== 0 ? dc : a.tag.localeCompare(b.tag);
    });
    if (untagged.length > 0) {
      untagged.sort((a, b) => (a.modTime > b.modTime ? -1 : 1));
      out.push({ tag: 'untagged', notes: untagged, untagged: true });
    }
    return out;
  });

  // What list the right pane should render based on the active view.
  const activeList = $derived.by(() => {
    const v = deps.getView();
    if (v === 'search') return searchResults;
    if (v === 'recent') return recent;
    if (v === 'pinned') return pinnedList;
    if (v === 'all') return allSorted;
    return [];
  });

  // ---- row actions ----

  function open(n: Note) {
    goto(`/notes/${encodeURIComponent(n.path)}`);
  }

  async function togglePin(n: Note) {
    try {
      const want = !pinned.has(n.path);
      const r = await api.setPinned(n.path, want);
      pinned = new Set(r.pinned.map((p) => p.path));
    } catch (e) {
      toast.error('pin failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function del(n: Note) {
    if (!confirm(`Delete "${n.title}"? This cannot be undone.`)) return;
    try {
      await api.deleteNote(n.path);
      notes = notes.filter((x) => x.path !== n.path);
      pinned.delete(n.path);
      pinned = new Set(pinned);
      toast.success('deleted');
    } catch (e) {
      toast.error('delete failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function rename(n: Note) {
    const next = prompt('New path (relative to vault):', n.path);
    if (!next || next.trim() === n.path) return;
    try {
      await api.renameNote(n.path, next.trim());
      toast.success('renamed');
      await loadAll();
    } catch (e) {
      toast.error('rename failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  return {
    get notes() { return notes; },
    set notes(v) { notes = v; },
    get pinned() { return pinned; },
    set pinned(v) { pinned = v; },
    get loading() { return loading; },
    set loading(v) { loading = v; },
    get q() { return q; },
    set q(v) { q = v; },
    get searchResults() { return searchResults; },
    set searchResults(v) { searchResults = v; },
    get searching() { return searching; },
    set searching(v) { searching = v; },
    get pinnedCount() { return pinnedCount; },
    get pinnedList() { return pinnedList; },
    get recent() { return recent; },
    get allSorted() { return allSorted; },
    get streamSections() { return streamSections; },
    get alphaSections() { return alphaSections; },
    get folderCards() { return folderCards; },
    get tagSections() { return tagSections; },
    get topTags() { return topTags; },
    get folderCount() { return folderCount; },
    get tagCount() { return tagCount; },
    get activeList() { return activeList; },
    loadAll,
    reload,
    open,
    togglePin,
    del,
    rename
  };
}
