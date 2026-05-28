<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { auth } from '$lib/stores/auth';
  import { api, type Note } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { createCoalescedReload } from '$lib/util/coalesce';
  import { relativeTime } from '$lib/util/relativeTime';
  import { toast } from '$lib/components/toast';
  import NotesTree from '$lib/notes/NotesTree.svelte';
  import NotesPageHeader from '$lib/notes/NotesPageHeader.svelte';
  import NotesQuickFilters from '$lib/notes/NotesQuickFilters.svelte';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import { loadStored, loadStoredString, saveStored, saveStoredString } from '$lib/util/storage';
  import { rafThrottle } from '$lib/util/streamThrottle';

  // Notes hub. View modes covering the full surface area:
  //   stream      — default. Reverse-chrono buckets (Today / Yesterday /
  //                 This week / Earlier this month / Older). "Less
  //                 organised, more like the mind" — the user lands on
  //                 their recent thinking, not a folder tree.
  //   tree        — the classic hierarchy
  //   recent      — top-30 by modTime (denser than 'stream'; kept as a
  //                 sub-option for when the user wants the flat cap)
  //   pinned      — the user's anchored set
  //   all         — flat list with sort options
  //   alpha       — A–Z with letter dividers (useful when the title is
  //                 remembered but not the folder)
  //   tags        — grouped by primary tag
  //   collections — saved virtual folders (localStorage filter recipes)
  //   folders     — top-level folder cards
  //   search      — full-text via the search index
  type View = 'stream' | 'recent' | 'tree' | 'pinned' | 'all' | 'alpha' | 'tags' | 'collections' | 'folders' | 'search';
  const VIEW_KEY = 'granit.notes.view';
  // Validate the persisted value before trusting it — an older build
  // could have stored a string that's no longer a valid view.
  const VALID_VIEWS: ReadonlySet<View> = new Set([
    'stream', 'recent', 'tree', 'pinned', 'all', 'alpha', 'tags', 'collections', 'folders', 'search'
  ]);
  function loadInitialView(): View {
    const stored = loadStoredString(VIEW_KEY, 'stream');
    return VALID_VIEWS.has(stored as View) ? (stored as View) : 'stream';
  }
  let view = $state<View>(loadInitialView());
  $effect(() => saveStoredString(VIEW_KEY, view));

  // Slim-header overflow menu (mirrors /tasks). Only the 4 less-used
  // views live in the dropdown — primary 5 sit in the segmented
  // control. activeOverflowLabel surfaces the current overflow view
  // back in the More button so the user has a breadcrumb without
  // opening the menu.
  const OVERFLOW_KEYS: ReadonlySet<View> = new Set(['alpha', 'tags', 'folders', 'collections']);
  const OVERFLOW_LABELS: Record<string, string> = {
    alpha: 'A–Z',
    tags: 'Tags',
    folders: 'Folders',
    collections: 'Collections'
  };
  let moreViewsOpen = $state(false);
  let activeOverflowLabel = $derived(OVERFLOW_KEYS.has(view) ? (OVERFLOW_LABELS[view] ?? '') : '');
  function selectView(v: View) {
    // Clicking the All tab directly clears any folder/tag filter —
    // those are only set via Folders cards or a Collection; hitting
    // the segmented "All" on its own should mean "show everything".
    if (v === 'all' && view !== 'all') { folderFilter = ''; tagFilter = ''; }
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

  type SortKey = 'modified' | 'created' | 'name' | 'size';
  let sortKey = $state<SortKey>('modified');

  // Folder filter — set by clicking a card in the folders view. The
  // page swaps to a flat list of just that folder's notes. '' means
  // unfiltered; '__root__' isolates vault-root files; any other value
  // is a folder prefix.
  let folderFilter = $state('');

  // Tag filter — set by a collection that pins a specific tag. Empty
  // means unfiltered. Applied alongside the folder filter in 'all'.
  let tagFilter = $state('');

  let notes = $state<Note[]>([]);
  let pinned = $state<Set<string>>(new Set());
  let loading = $state(false);

  // Search state — debounced via $effect that wakes when q changes.
  let q = $state('');
  let searchResults = $state<Note[]>([]);
  let searching = $state(false);
  let searchTimer: ReturnType<typeof setTimeout> | null = null;

  // ---- Smart collections ---------------------------------------------------
  // A collection is a saved filter recipe. We persist them in
  // localStorage (no backend) under a single key. Activating a
  // collection sets the folder/tag/sort/search filters and switches to
  // 'all' (or 'search' if a free-text query is set) so the existing
  // filter mechanics render the result list.
  interface Collection {
    id: string;
    name: string;
    query: string;
    tag?: string;
    folder?: string;
    sort: SortKey;
  }
  const COLLECTIONS_KEY = 'granit.notes.collections';
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
  let collections = $state<Collection[]>(loadStored<Collection[]>(COLLECTIONS_KEY, [], validateCollections));
  $effect(() => saveStored(COLLECTIONS_KEY, collections));

  function applyCollection(c: Collection) {
    folderFilter = c.folder ?? '';
    tagFilter = c.tag ?? '';
    sortKey = c.sort;
    if (c.query) {
      q = c.query;
      view = 'search';
    } else {
      q = '';
      view = 'all';
    }
  }
  function saveCurrentAsCollection() {
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

  // Quick-create / quick-capture dialog state.
  //
  // The dialog flow has three modes:
  //   'capture' — single big textarea; AI parses + populates staging
  //   'staging' — editable AI suggestions (title / tags / folder / links)
  //   'manual'  — legacy two-field (title + folder) fallback when the
  //               user clicks "Skip AI" or AI fails
  type CaptureMode = 'capture' | 'staging' | 'manual';
  let createOpen = $state(false);
  let captureMode = $state<CaptureMode>('capture');
  let captureText = $state('');
  let captureBusy = $state(false);
  let captureAbort: AbortController | null = null;
  let captureRaw = $state(''); // streaming token buffer (for the user's "AI is thinking…" hint)

  // Staging fields (populated from AI JSON, then editable).
  let stageTitle = $state('');
  let stageTags = $state<string[]>([]);
  let stageTagInput = $state('');
  let stageFolder = $state('Inbox');
  let stageWikilinkCandidates = $state<string[]>([]);
  let stageWikilinksChosen = $state<Set<string>>(new Set());

  // Manual-mode fields (kept around for the "Skip AI" fallback).
  let createTitle = $state('');
  let createFolder = $state('');
  let creating = $state(false);

  async function loadAll() {
    if (!$auth) return;
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

  onMount(() => {
    loadAll();
    const unsub = onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') reload.trigger();
    });
    // Mobile browsers (and any backgrounded tab) suspend the WS, so
    // notes created/edited on another device while we were away never
    // make it through. Refetch on the visibility flip so a returning
    // tab catches up without the user having to pull-to-refresh. We
    // call loadAll directly (bypassing the coalesce window) so the
    // user sees the fresh list as soon as the tab returns.
    const onVisible = () => {
      if (document.visibilityState === 'visible') loadAll();
    };
    document.addEventListener('visibilitychange', onVisible);
    window.addEventListener('focus', onVisible);
    // ⌘N / Ctrl+N opens quick-capture. We use keydown on the window so
    // it works regardless of focus location. Ignore when an input or
    // textarea already owns the cursor — the user is mid-type and
    // doesn't want us to hijack their keystroke.
    const onKey = (e: KeyboardEvent) => {
      if (!(e.key === 'n' || e.key === 'N')) return;
      if (!(e.metaKey || e.ctrlKey)) return;
      const t = e.target as HTMLElement | null;
      if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return;
      e.preventDefault();
      openCapture();
    };
    window.addEventListener('keydown', onKey);
    // Deep-link from the PWA shortcut: /notes?capture=1 auto-opens
    // the quick-capture dialog. We only honour the flag on first
    // mount so a back-nav onto /notes doesn't re-pop the dialog at
    // the user. The history.replaceState rinses the flag so a
    // subsequent reload also stays clean.
    if (typeof window !== 'undefined') {
      const sp = new URLSearchParams(window.location.search);
      const want = sp.get('capture') === '1';
      // Web Share Target — manifest registers /notes as the share
      // sink so the OS share sheet pipes title/text/url here. Treat
      // any of them as an implicit capture intent.
      const shTitle = sp.get('title') ?? '';
      const shText = sp.get('text') ?? '';
      const shUrl = sp.get('url') ?? '';
      const shared = shTitle || shText || shUrl;
      if (want || shared) {
        openCapture();
        if (shared) {
          // Build a friendly pre-fill: title on its own line, then
          // body text, then url (skip url if it's already inside the
          // text — some clients duplicate it). openCapture() resets
          // captureText to '', so this assignment must come AFTER.
          const parts: string[] = [];
          if (shTitle) parts.push(shTitle);
          if (shText) parts.push(shText);
          if (shUrl && !shText.includes(shUrl)) parts.push(shUrl);
          captureText = parts.join('\n\n');
        }
        const u = new URL(window.location.href);
        u.searchParams.delete('capture');
        u.searchParams.delete('title');
        u.searchParams.delete('text');
        u.searchParams.delete('url');
        window.history.replaceState({}, '', u.pathname + (u.search || '') + (u.hash || ''));
      }
    }
    return () => {
      unsub();
      document.removeEventListener('visibilitychange', onVisible);
      window.removeEventListener('focus', onVisible);
      window.removeEventListener('keydown', onKey);
      reload.cancel();
      captureAbort?.abort();
    };
  });

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
  let pinnedCount = $derived.by(() => {
    let c = 0;
    for (const n of notes) if (pinned.has(n.path)) c++;
    return c;
  });
  let pinnedList = $derived.by(() => {
    if (view !== 'pinned') return [];
    return notes.filter((n) => pinned.has(n.path));
  });

  let recent = $derived.by(() => {
    if (view !== 'recent') return [];
    return [...notes]
      .sort((a, b) => (a.modTime > b.modTime ? -1 : 1))
      .slice(0, 30);
  });

  // Helper — apply the folder card filter to a note list. '__root__'
  // matches notes with no slash in their path; any other value is a
  // top-level folder prefix.
  function passesFolderFilter(n: Note): boolean {
    if (!folderFilter) return true;
    if (folderFilter === '__root__') return n.path.indexOf('/') === -1;
    return n.path.startsWith(folderFilter + '/');
  }
  function passesTagFilter(n: Note): boolean {
    if (!tagFilter) return true;
    return !!n.tags && n.tags.indexOf(tagFilter) !== -1;
  }

  let allSorted = $derived.by(() => {
    if (view !== 'all') return [];
    const arr = notes.filter((n) => passesFolderFilter(n) && passesTagFilter(n));
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
  interface StreamSection { id: string; label: string; notes: Note[] }
  let streamSections = $derived.by<StreamSection[]>(() => {
    if (view !== 'stream') return [];
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
  interface AlphaSection { letter: string; notes: Note[] }
  let alphaSections = $derived.by<AlphaSection[]>(() => {
    if (view !== 'alpha') return [];
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
  interface FolderCard { name: string; count: number; recentTitle: string; recentModTime: string; isRoot: boolean }
  let folderCards = $derived.by<FolderCard[]>(() => {
    if (view !== 'folders') return [];
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
  let folderCount = $derived.by(() => {
    const seen = new Set<string>();
    for (const n of notes) {
      const slash = n.path.indexOf('/');
      seen.add(slash === -1 ? '__root__' : n.path.slice(0, slash));
    }
    return seen.size;
  });
  let tagCount = $derived.by(() => {
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
  let topTags = $derived.by<string[]>(() => {
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
  interface TagSection { tag: string; notes: Note[]; untagged: boolean }
  let tagSections = $derived.by<TagSection[]>(() => {
    if (view !== 'tags') return [];
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

  // ---- actions ----

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

  // ---- quick-capture (AI) ----

  function slugify(s: string): string {
    return s.replace(/[^\w\s-]/g, '').trim().replace(/\s+/g, '-') || 'untitled';
  }
  function sanitizeFolder(s: string): string {
    return s.trim().replace(/^\/+|\/+$/g, '');
  }

  function openCapture() {
    captureAbort?.abort();
    captureAbort = null;
    captureMode = 'capture';
    captureText = '';
    captureBusy = false;
    captureRaw = '';
    stageTitle = '';
    stageTags = [];
    stageTagInput = '';
    stageFolder = 'Inbox';
    stageWikilinkCandidates = [];
    stageWikilinksChosen = new Set();
    createTitle = '';
    createFolder = '';
    creating = false;
    createOpen = true;
  }
  function closeCapture() {
    captureAbort?.abort();
    captureAbort = null;
    captureBusy = false;
    creating = false;
    createOpen = false;
  }

  // The AI prompt is built fresh on each capture so the existing-tag
  // list and sampled titles reflect the *current* vault. Sampling: we
  // take a random slice of titles so the AI sees variety without us
  // shipping the entire vault in the prompt — the prompt is bounded
  // even on huge vaults.
  function sampleTitles(max: number): string[] {
    if (notes.length <= max) return notes.map((n) => n.title);
    // Reservoir-style — cheap, no shuffle of the whole array.
    const out: string[] = [];
    for (let i = 0; i < notes.length; i++) {
      if (out.length < max) out.push(notes[i].title);
      else {
        const j = Math.floor(Math.random() * (i + 1));
        if (j < max) out[j] = notes[i].title;
      }
    }
    return out;
  }

  interface AiCapture {
    title: string;
    tags: string[];
    folder: string;
    wikilinkCandidates: string[];
  }
  function parseAiJson(raw: string): AiCapture | null {
    let cleaned = raw.trim();
    if (cleaned.startsWith('```')) {
      cleaned = cleaned.replace(/^```json\s*/i, '').replace(/^```\s*/, '').replace(/```\s*$/, '').trim();
    }
    // Some models wrap with extra prose — try to grab the first
    // {...} block as a fallback.
    if (!cleaned.startsWith('{')) {
      const m = cleaned.match(/\{[\s\S]*\}/);
      if (m) cleaned = m[0];
    }
    try {
      const parsed = JSON.parse(cleaned) as Record<string, unknown>;
      const title = typeof parsed.title === 'string' ? parsed.title.trim() : '';
      const folder = typeof parsed.folder === 'string' ? parsed.folder.trim() : '';
      const tagsRaw = Array.isArray(parsed.tags) ? parsed.tags : [];
      const tags = tagsRaw
        .filter((t): t is string => typeof t === 'string' && !!t.trim())
        .map((t) => t.trim().replace(/^#/, ''));
      const wlRaw = Array.isArray(parsed.wikilinkCandidates) ? parsed.wikilinkCandidates : [];
      const wl = wlRaw.filter((t): t is string => typeof t === 'string' && !!t.trim()).map((t) => t.trim());
      if (!title) return null;
      return { title, tags, folder: folder || 'Inbox', wikilinkCandidates: wl };
    } catch {
      return null;
    }
  }

  async function runAiCapture() {
    const text = captureText.trim();
    if (!text || captureBusy) return;
    captureBusy = true;
    captureRaw = '';
    captureAbort = new AbortController();
    // rAF coalescer — keeps "AI is thinking…" hint fluid without
    // spamming reactive writes.
    const throttle = rafThrottle((acc) => { captureRaw = acc; });

    const tagsHint = topTags.length > 0 ? topTags.join(', ') : '(none yet)';
    const titlesHint = sampleTitles(40).join('\n');
    const system =
      'You read a freeform note capture from the user and produce a STRICT JSON ' +
      'object (no fences, no prose) of the shape: ' +
      '{"title": "<short, human-readable title — Title Case, no trailing punctuation, no filename>", ' +
      '"tags": ["tag1", "tag2"], ' +
      '"folder": "<folder path inside the vault, default \\"Inbox\\" if unsure>", ' +
      '"wikilinkCandidates": ["Existing Note Title", "..."]}. ' +
      'Prefer reusing tags from this list of existing tags (lowercase, no leading #): ' + tagsHint + '. ' +
      'wikilinkCandidates MUST be picked verbatim from this sample of existing note titles ' +
      '(omit the field or use [] if none seem related): \n' + titlesHint;
    try {
      await api.chatStream(
        [
          { role: 'system', content: system },
          { role: 'user', content: text }
        ],
        undefined,
        {
          onChunk: throttle.onChunk,
          onDone: () => {
            throttle.flush();
            const parsed = parseAiJson(throttle.value());
            if (!parsed) {
              toast.error('AI returned bad JSON — switching to manual.');
              // Pre-fill the manual title with the first line so the
              // user doesn't lose the typed body.
              const first = text.split(/\n/)[0]?.trim() ?? '';
              createTitle = first.slice(0, 80);
              captureMode = 'manual';
              return;
            }
            stageTitle = parsed.title;
            stageTags = parsed.tags;
            stageFolder = parsed.folder || 'Inbox';
            stageWikilinkCandidates = parsed.wikilinkCandidates.filter(
              (t) => notes.some((n) => n.title === t)
            );
            stageWikilinksChosen = new Set();
            captureMode = 'staging';
          },
          onError: (err) => {
            throttle.flush();
            toast.error('AI failed: ' + err.message + ' — switching to manual.');
            const first = text.split(/\n/)[0]?.trim() ?? '';
            createTitle = first.slice(0, 80);
            captureMode = 'manual';
          }
        },
        captureAbort.signal
      );
    } finally {
      captureBusy = false;
      captureAbort = null;
    }
  }

  // ⌘Enter without waiting for AI — drop the captured text straight
  // into Inbox/{first-line}.md with empty frontmatter. The user did
  // not gate this on AI; we honour the intent.
  async function fastCapture() {
    const text = captureText.trim();
    if (!text || creating) return;
    const first = text.split(/\n/)[0]?.trim() || 'untitled';
    const title = first.slice(0, 80);
    const path = `Inbox/${slugify(title)}.md`;
    creating = true;
    try {
      await api.createNote({ path, frontmatter: {}, body: text });
      closeCapture();
      goto(`/notes/${encodeURIComponent(path)}`);
    } catch (e) {
      toast.error('create failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      creating = false;
    }
  }

  function addStageTag() {
    const t = stageTagInput.trim().replace(/^#/, '');
    if (!t) return;
    if (!stageTags.includes(t)) stageTags = [...stageTags, t];
    stageTagInput = '';
  }
  function removeStageTag(t: string) {
    stageTags = stageTags.filter((x) => x !== t);
  }
  function toggleWikilink(title: string) {
    const next = new Set(stageWikilinksChosen);
    if (next.has(title)) next.delete(title);
    else next.add(title);
    stageWikilinksChosen = next;
  }

  async function saveStaged() {
    const title = stageTitle.trim();
    if (!title || creating) return;
    const folder = sanitizeFolder(stageFolder || 'Inbox');
    const path = (folder ? folder + '/' : '') + slugify(title) + '.md';
    let body = captureText.trim();
    if (stageWikilinksChosen.size > 0) {
      const links = [...stageWikilinksChosen].map((t) => `[[${t}]]`).join(', ');
      body = body + '\n\nRelated: ' + links + '\n';
    }
    const frontmatter: Record<string, unknown> = {};
    if (stageTags.length > 0) frontmatter.tags = stageTags;
    creating = true;
    try {
      await api.createNote({ path, frontmatter, body });
      closeCapture();
      goto(`/notes/${encodeURIComponent(path)}`);
    } catch (e) {
      toast.error('create failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      creating = false;
    }
  }

  // Manual-mode fallback (legacy two-field create). Kept for the user
  // who deliberately bypasses the AI flow with "Skip AI".
  async function manualCreate() {
    const title = createTitle.trim();
    if (!title) return;
    creating = true;
    try {
      const folder = sanitizeFolder(createFolder);
      const path = (folder ? folder + '/' : '') + slugify(title) + '.md';
      const body = captureText.trim() || `# ${title}\n\n`;
      await api.createNote({ path, body });
      closeCapture();
      goto(`/notes/${encodeURIComponent(path)}`);
    } catch (e) {
      toast.error('create failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      creating = false;
    }
  }

  // Capture dialog keyboard handler.
  function onCaptureKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault();
      closeCapture();
      return;
    }
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      e.preventDefault();
      if (captureMode === 'capture') {
        if (!captureBusy && captureText.trim()) fastCapture();
      } else if (captureMode === 'staging') {
        saveStaged();
      } else if (captureMode === 'manual') {
        manualCreate();
      }
    }
  }

  // Falls back to a calendar date past 30 days — past that, "5w
  // ago" reads less well than "Apr 12".
  const fmtRelative = (iso: string) => relativeTime(iso, { dateThresholdDays: 30 });

  function fmtSize(bytes?: number): string {
    if (!bytes) return '';
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${Math.round(bytes / 102.4) / 10} KB`;
    return `${Math.round(bytes / 1024 / 102.4) / 10} MB`;
  }

  // What list the right pane should render based on the active view.
  let activeList = $derived.by(() => {
    if (view === 'search') return searchResults;
    if (view === 'recent') return recent;
    if (view === 'pinned') return pinnedList;
    if (view === 'all') return allSorted;
    return [];
  });
</script>

<div class="h-full flex flex-col">
  <!-- Stream T — slim single-row page header. Title + count on the
       left, search · view-switcher · More-views · New on the right.
       Active filter pills + sort segmented sit in the QuickFilters
       row below so the chrome stays mute. Saves ~60-70px of vertical
       space vs the previous three-row layout. -->
  <NotesPageHeader
    {view}
    bind:q
    notesCount={notes.length}
    pinnedCount={pinnedCount}
    searchResultsCount={searchResults.length}
    moreViewsOpen={moreViewsOpen}
    activeOverflowLabel={activeOverflowLabel}
    onSelectView={selectView}
    onToggleMoreViews={() => (moreViewsOpen = !moreViewsOpen)}
    onPickOverflowView={pickOverflowView}
    onMoreViewsKey={onMoreViewsKey}
    onQuickCapture={openCapture}
    onSearchInput={(v) => { if (v.trim()) view = 'search'; }}
    onSearchFocus={() => { if (q.trim()) view = 'search'; }}
  />

  <!-- Quick-filter row. Renders only on 'all' (sort segmented +
       folder/tag clear pills) and 'search' with an active query
       (Save-as-collection). Self-hides on every other view. -->
  <NotesQuickFilters
    {view}
    {folderFilter}
    {tagFilter}
    {sortKey}
    searchActive={!!q.trim()}
    onClearFolder={() => (folderFilter = '')}
    onClearTag={() => (tagFilter = '')}
    onPickSort={(s) => (sortKey = s)}
    onSaveCollection={saveCurrentAsCollection}
  />

  <div class="flex-1 min-h-0 overflow-hidden">
    {#if view === 'tree'}
      <NotesTree />
    {:else if loading && notes.length === 0}
      <div class="p-3 space-y-2">
        {#each Array(8) as _}
          <Skeleton class="h-12 w-full" />
        {/each}
      </div>
    {:else if view === 'search' && q.trim() && !searching && searchResults.length === 0}
      <div class="p-8 text-center text-sm text-dim">No notes match <code class="text-text">{q}</code></div>
    {:else if view === 'stream'}
      {#if notes.length === 0}
        <div class="p-8 text-center text-sm text-dim">No notes yet — hit <kbd class="px-1 rounded bg-surface1 text-text">⌘N</kbd> to capture your first thought.</div>
      {:else}
        <div class="overflow-y-auto h-full">
          {#each streamSections as sec (sec.id)}
            <div class="sticky top-0 z-10 bg-mantle px-3 sm:px-4 py-1 text-[11px] uppercase tracking-wider text-dim border-b border-surface1">
              {sec.label} <span class="opacity-60 ml-1">{sec.notes.length}</span>
            </div>
            <ul class="divide-y divide-surface1/50">
              {#each sec.notes as n (n.path)}
                {@render streamRow(n)}
              {/each}
            </ul>
          {/each}
        </div>
      {/if}
    {:else if view === 'alpha'}
      {#if notes.length === 0}
        <div class="p-8 text-center text-sm text-dim">No notes in your vault.</div>
      {:else}
        <div class="overflow-y-auto h-full">
          {#each alphaSections as sec (sec.letter)}
            <div class="sticky top-0 z-10 bg-mantle px-3 sm:px-4 py-1 text-[11px] uppercase tracking-wider text-dim border-b border-surface1">
              {sec.letter} <span class="opacity-60 ml-1">{sec.notes.length}</span>
            </div>
            <ul class="divide-y divide-surface1/50">
              {#each sec.notes as n (n.path)}
                {@render row(n)}
              {/each}
            </ul>
          {/each}
        </div>
      {/if}
    {:else if view === 'tags'}
      {#if tagSections.length === 0}
        <div class="p-8 text-center text-sm text-dim">No tagged notes yet. Add a <code class="text-text">tags:</code> field in frontmatter or use <code class="text-text">#tag</code> in the body.</div>
      {:else}
        <div class="overflow-y-auto h-full">
          {#each tagSections as sec (sec.tag)}
            <div class="sticky top-0 z-10 bg-mantle px-3 sm:px-4 py-1.5 border-b border-surface1 flex items-center gap-2">
              {#if sec.untagged}
                <span class="text-[11px] uppercase tracking-wider text-dim italic">untagged</span>
              {:else}
                <span class="text-xs px-1.5 py-0.5 rounded bg-surface1 text-secondary">#{sec.tag}</span>
              {/if}
              <span class="text-[11px] text-dim">{sec.notes.length}</span>
            </div>
            <ul class="divide-y divide-surface1/50">
              {#each sec.notes as n (n.path)}
                {@render row(n)}
              {/each}
            </ul>
          {/each}
        </div>
      {/if}
    {:else if view === 'collections'}
      <div class="overflow-y-auto h-full p-3 sm:p-4 space-y-2">
        {#if collections.length === 0}
          <div class="p-8 text-center text-sm text-dim">
            No collections yet. Run a search and click <span class="text-text">Save as collection…</span> to pin it here.
          </div>
        {:else}
          <ul class="divide-y divide-surface1/50">
            {#each collections as c (c.id)}
              <li class="group hover:bg-surface0 transition-colors">
                <div class="flex items-center gap-3 px-3 sm:px-4 py-2.5">
                  <button
                    type="button"
                    onclick={() => applyCollection(c)}
                    class="flex-1 min-w-0 text-left"
                  >
                    <div class="flex items-baseline gap-2 min-w-0">
                      <span class="text-sm text-text truncate">{c.name}</span>
                    </div>
                    <div class="flex items-center gap-2 mt-0.5 text-[11px] text-dim flex-wrap">
                      {#if c.query}<span>q: <code class="text-text">{c.query}</code></span>{/if}
                      {#if c.folder}<span>·</span><span>📁 {c.folder === '__root__' ? '/' : c.folder}</span>{/if}
                      {#if c.tag}<span>·</span><span class="text-secondary">#{c.tag}</span>{/if}
                      <span>·</span><span>sort: {c.sort}</span>
                    </div>
                  </button>
                  <button
                    onclick={() => deleteCollection(c.id)}
                    aria-label="delete collection"
                    class="w-8 h-8 flex items-center justify-center text-dim hover:text-error rounded opacity-100 sm:opacity-0 sm:group-hover:opacity-100 transition-opacity"
                    title="Delete"
                  >
                    <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
                      <path d="M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2m3 0v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/>
                    </svg>
                  </button>
                </div>
              </li>
            {/each}
          </ul>
        {/if}
      </div>
    {:else if view === 'folders'}
      {#if folderCards.length === 0}
        <div class="p-8 text-center text-sm text-dim">No folders yet — create a note with a path like <code class="text-text">Notes/Ideas/foo.md</code> to get started.</div>
      {:else}
        <div class="overflow-y-auto h-full p-3 sm:p-4">
          <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-2 sm:gap-3">
            {#each folderCards as card (card.name)}
              <button
                type="button"
                onclick={() => { folderFilter = card.isRoot ? '__root__' : card.name; tagFilter = ''; view = 'all'; }}
                class="text-left p-3 bg-surface0 hover:bg-surface1 border border-surface1 rounded transition-colors min-h-[5rem]"
              >
                <div class="flex items-baseline gap-2 mb-1">
                  <span class="text-warning text-base flex-shrink-0">{card.isRoot ? '🏠' : '📁'}</span>
                  <span class="text-sm font-medium text-text truncate flex-1">{card.name}</span>
                  <span class="text-[11px] text-dim flex-shrink-0">{card.count}</span>
                </div>
                {#if card.recentTitle}
                  <div class="text-[11px] text-dim truncate" title={card.recentTitle}>
                    {card.recentTitle}
                  </div>
                  <div class="text-[10px] text-dim/80 mt-0.5">{fmtRelative(card.recentModTime)}</div>
                {/if}
              </button>
            {/each}
          </div>
        </div>
      {/if}
    {:else if activeList.length === 0}
      <div class="p-8 text-center text-sm text-dim">
        {#if view === 'pinned'}No pinned notes yet. Click the ★ icon on any note to pin it.
        {:else if view === 'recent'}No notes in your vault.
        {:else}Empty.{/if}
      </div>
    {:else}
      <ul class="overflow-y-auto h-full divide-y divide-surface1/50">
        {#each activeList as n (n.path)}
          {@render row(n)}
        {/each}
      </ul>
    {/if}
  </div>
</div>

<!-- Dense single-line row variant used by the Stream view. Title +
     tag chips + relative date all on one line, with the action
     buttons revealed on hover. Matches the power-UI density the
     user asked for; the legacy multi-line `row` snippet below is
     still used by alpha/tags/all/recent/pinned. -->
{#snippet streamRow(n: Note)}
  {@const isPinned = pinned.has(n.path)}
  <li class="group hover:bg-surface0 transition-colors">
    <div class="flex items-center gap-2 px-3 sm:px-4 py-1.5">
      <button
        type="button"
        onclick={() => open(n)}
        class="flex-1 min-w-0 text-left flex items-center gap-2"
      >
        {#if isPinned}<span class="text-warning text-xs flex-shrink-0">★</span>{/if}
        <span class="text-sm text-text truncate flex-shrink min-w-0">{n.title}</span>
        {#if n.tags && n.tags.length > 0}
          <span class="hidden sm:flex flex-wrap gap-1 flex-shrink-0">
            {#each n.tags.slice(0, 3) as tag}
              <span class="text-[10px] px-1 rounded bg-surface1 text-secondary">#{tag}</span>
            {/each}
          </span>
        {/if}
        <span class="ml-auto text-[11px] text-dim flex-shrink-0 tabular-nums">{fmtRelative(n.modTime)}</span>
      </button>
      <div class="flex items-center gap-0.5 opacity-100 sm:opacity-0 sm:group-hover:opacity-100 transition-opacity">
        <button
          onclick={() => togglePin(n)}
          aria-label={isPinned ? 'unpin' : 'pin'}
          class="w-7 h-7 flex items-center justify-center text-dim hover:text-warning rounded"
          title={isPinned ? 'Unpin' : 'Pin'}
        >★</button>
        <button
          onclick={() => del(n)}
          aria-label="delete"
          class="w-7 h-7 flex items-center justify-center text-dim hover:text-error rounded"
          title="Delete"
        >
          <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
            <path d="M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2m3 0v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/>
          </svg>
        </button>
      </div>
    </div>
  </li>
{/snippet}

{#snippet row(n: Note)}
  {@const isPinned = pinned.has(n.path)}
  <li class="group hover:bg-surface0 transition-colors">
    <div class="flex items-center gap-3 px-3 sm:px-4 py-2.5">
      <button
        type="button"
        onclick={() => open(n)}
        class="flex-1 min-w-0 text-left"
      >
        <div class="flex items-baseline gap-2 min-w-0">
          {#if isPinned}<span class="text-warning text-xs flex-shrink-0">★</span>{/if}
          <span class="text-sm text-text truncate">{n.title}</span>
          <span class="text-[11px] text-dim truncate">{n.path}</span>
        </div>
        <div class="flex items-center gap-2 mt-0.5 text-[11px] text-dim">
          <span>{fmtRelative(n.modTime)}</span>
          {#if n.size}<span>·</span><span>{fmtSize(n.size)}</span>{/if}
          {#if n.tags && n.tags.length > 0}
            <span>·</span>
            <span class="flex flex-wrap gap-1">
              {#each n.tags.slice(0, 3) as tag}
                <span class="px-1 rounded bg-surface1 text-secondary">#{tag}</span>
              {/each}
            </span>
          {/if}
        </div>
      </button>
      <!-- Hover-revealed action buttons. Tap-friendly on mobile
           (always visible) since :hover doesn't fire on touch. -->
      <div class="flex items-center gap-0.5 opacity-100 sm:opacity-0 sm:group-hover:opacity-100 transition-opacity">
        <button
          onclick={() => togglePin(n)}
          aria-label={isPinned ? 'unpin' : 'pin'}
          class="w-8 h-8 flex items-center justify-center text-dim hover:text-warning rounded"
          title={isPinned ? 'Unpin' : 'Pin'}
        >★</button>
        <button
          onclick={() => rename(n)}
          aria-label="rename"
          class="w-8 h-8 flex items-center justify-center text-dim hover:text-secondary rounded"
          title="Rename or move"
        >
          <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
            <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/>
            <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/>
          </svg>
        </button>
        <button
          onclick={() => del(n)}
          aria-label="delete"
          class="w-8 h-8 flex items-center justify-center text-dim hover:text-error rounded"
          title="Delete"
        >
          <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
            <path d="M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2m3 0v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/>
          </svg>
        </button>
      </div>
    </div>
  </li>
{/snippet}

{#if createOpen}
  <!-- Click-outside to close. Inner div stops propagation. -->
  <div
    role="dialog"
    aria-modal="true"
    aria-labelledby="newnote-title"
    class="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4"
    onclick={(e) => { if (e.target === e.currentTarget) closeCapture(); }}
    onkeydown={onCaptureKey}
    tabindex="-1"
  >
    <div class="bg-mantle border border-surface1 rounded-lg w-full max-w-xl p-5 shadow-xl">
      {#if captureMode === 'capture'}
        <div class="flex items-center justify-between mb-3">
          <h2 id="newnote-title" class="text-base font-semibold text-text">Quick capture</h2>
          <button
            type="button"
            onclick={() => { captureMode = 'manual'; }}
            class="text-[11px] text-dim hover:text-text"
            title="Skip AI and enter title + folder manually"
          >Skip AI →</button>
        </div>
        <textarea
          bind:value={captureText}
          placeholder="Capture anything — title, tags, folder, and links will be suggested."
          autofocus
          rows="6"
          disabled={captureBusy}
          class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary resize-y disabled:opacity-60"
        ></textarea>
        {#if captureBusy}
          <div class="mt-2 text-[11px] text-dim flex items-center gap-2">
            <span class="inline-block w-2 h-2 rounded-full bg-primary animate-pulse"></span>
            <span>AI is reading… {captureRaw.length > 0 ? `(${captureRaw.length} chars)` : ''}</span>
          </div>
        {:else}
          <div class="mt-2 text-[11px] text-dim">
            <kbd class="px-1 rounded bg-surface1 text-text">⌘Enter</kbd> to save straight to Inbox without AI.
          </div>
        {/if}
        <div class="flex justify-end gap-2 pt-3">
          <button
            type="button"
            onclick={closeCapture}
            class="px-3 py-1.5 text-sm text-dim hover:text-text"
          >cancel</button>
          <button
            type="button"
            onclick={runAiCapture}
            disabled={!captureText.trim() || captureBusy}
            class="px-4 py-1.5 bg-primary text-on-primary rounded text-sm font-medium disabled:opacity-50"
          >{captureBusy ? 'thinking…' : 'Capture'}</button>
        </div>
      {:else if captureMode === 'staging'}
        <div class="flex items-center justify-between mb-3">
          <h2 id="newnote-title" class="text-base font-semibold text-text">Review</h2>
          <button
            type="button"
            onclick={() => { captureMode = 'capture'; }}
            class="text-[11px] text-dim hover:text-text"
          >← back</button>
        </div>
        <div class="space-y-3">
          <div>
            <label for="stage-title" class="block text-[11px] uppercase tracking-wider text-dim mb-1">Title</label>
            <input
              id="stage-title"
              bind:value={stageTitle}
              class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
            />
          </div>
          <div>
            <!-- Not a real form-label — the field below is a chip-grid
                 + free-text input, no single control to bind to. -->
            <span class="block text-[11px] uppercase tracking-wider text-dim mb-1">Tags</span>
            <div class="flex flex-wrap items-center gap-1 px-2 py-1.5 bg-surface0 border border-surface1 rounded">
              {#each stageTags as tag (tag)}
                <span class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-surface1 text-secondary text-xs">
                  #{tag}
                  <button
                    type="button"
                    onclick={() => removeStageTag(tag)}
                    aria-label="remove tag"
                    class="text-dim hover:text-error"
                  >×</button>
                </span>
              {/each}
              <input
                bind:value={stageTagInput}
                onkeydown={(e) => {
                  if (e.key === 'Enter' || e.key === ',') { e.preventDefault(); addStageTag(); }
                  else if (e.key === 'Backspace' && !stageTagInput && stageTags.length > 0) {
                    e.preventDefault();
                    stageTags = stageTags.slice(0, -1);
                  }
                }}
                placeholder="add tag…"
                class="flex-1 min-w-[6rem] bg-transparent text-xs text-text placeholder-dim focus:outline-none"
              />
            </div>
          </div>
          <div>
            <label for="stage-folder" class="block text-[11px] uppercase tracking-wider text-dim mb-1">Folder</label>
            <input
              id="stage-folder"
              bind:value={stageFolder}
              placeholder="Inbox"
              class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-text font-mono text-xs focus:outline-none focus:border-primary"
            />
          </div>
          {#if stageWikilinkCandidates.length > 0}
            <div>
              <!-- Group heading for a row of toggle buttons; not
                   bound to a single input. -->
              <span class="block text-[11px] uppercase tracking-wider text-dim mb-1">Related notes <span class="text-dim/70 normal-case">(toggle to insert as <code class="text-text">[[wikilinks]]</code>)</span></span>
              <div class="flex flex-wrap gap-1">
                {#each stageWikilinkCandidates as cand (cand)}
                  {@const on = stageWikilinksChosen.has(cand)}
                  <button
                    type="button"
                    onclick={() => toggleWikilink(cand)}
                    class="text-xs px-1.5 py-0.5 rounded border transition-colors
                      {on ? 'bg-primary text-on-primary border-primary' : 'bg-surface0 border-surface1 text-subtext hover:bg-surface1'}"
                  >[[{cand}]]</button>
                {/each}
              </div>
            </div>
          {/if}
          <details class="text-[11px] text-dim">
            <summary class="cursor-pointer hover:text-text">show body ({captureText.length} chars)</summary>
            <pre class="mt-1 p-2 bg-surface0 border border-surface1 rounded text-text whitespace-pre-wrap font-mono text-[11px] max-h-32 overflow-y-auto">{captureText}</pre>
          </details>
        </div>
        <div class="flex justify-end gap-2 pt-4">
          <button
            type="button"
            onclick={closeCapture}
            class="px-3 py-1.5 text-sm text-dim hover:text-text"
          >cancel</button>
          <button
            type="button"
            onclick={saveStaged}
            disabled={!stageTitle.trim() || creating}
            class="px-4 py-1.5 bg-primary text-on-primary rounded text-sm font-medium disabled:opacity-50"
          >{creating ? 'saving…' : 'Save'}</button>
        </div>
      {:else}
        <!-- manual fallback -->
        <div class="flex items-center justify-between mb-3">
          <h2 id="newnote-title" class="text-base font-semibold text-text">New note</h2>
          <button
            type="button"
            onclick={() => { captureMode = 'capture'; }}
            class="text-[11px] text-dim hover:text-text"
          >← try AI</button>
        </div>
        <form
          onsubmit={(e) => { e.preventDefault(); manualCreate(); }}
          class="space-y-3"
        >
          <div>
            <label for="nn-title" class="block text-[11px] uppercase tracking-wider text-dim mb-1">Title</label>
            <input
              id="nn-title"
              bind:value={createTitle}
              placeholder="My brilliant idea"
              required
              class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-text focus:outline-none focus:border-primary"
            />
          </div>
          <div>
            <label for="nn-folder" class="block text-[11px] uppercase tracking-wider text-dim mb-1">Folder <span class="text-dim/70 normal-case">(optional)</span></label>
            <input
              id="nn-folder"
              bind:value={createFolder}
              placeholder="Notes/Ideas"
              class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-text font-mono text-xs focus:outline-none focus:border-primary"
            />
            <p class="text-[11px] text-dim mt-1">Leave empty for vault root. The captured body is preserved.</p>
          </div>
          <div class="flex justify-end gap-2 pt-1">
            <button
              type="button"
              onclick={closeCapture}
              class="px-3 py-1.5 text-sm text-dim hover:text-text"
            >cancel</button>
            <button
              type="submit"
              disabled={!createTitle.trim() || creating}
              class="px-4 py-1.5 bg-primary text-on-primary rounded text-sm font-medium disabled:opacity-50"
            >{creating ? 'creating…' : 'Create'}</button>
          </div>
        </form>
      {/if}
    </div>
  </div>
{/if}

