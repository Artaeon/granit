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
  import Skeleton from '$lib/components/Skeleton.svelte';

  // Notes hub. View modes covering the full surface area: tree (the
  // classic hierarchy), recent (sorted by mod time), pinned (the user's
  // anchored set), all (flat list with sort options), alpha (A–Z with
  // letter dividers — useful when the user remembers a title but not
  // its folder), tags (grouped by primary tag), folders (top-level
  // folder cards as a navigation overview), search (full-text via the
  // search index). The TUI's notes overlay does most of the same; the
  // web matches.
  type View = 'recent' | 'tree' | 'pinned' | 'all' | 'alpha' | 'tags' | 'folders' | 'search';
  const VIEW_KEY = 'granit.notes.view';
  // Validate the persisted value before trusting it — an older build
  // could have stored a string that's no longer a valid view.
  const VALID_VIEWS: ReadonlySet<View> = new Set(['recent', 'tree', 'pinned', 'all', 'alpha', 'tags', 'folders', 'search']);
  function loadInitialView(): View {
    if (typeof localStorage === 'undefined') return 'recent';
    const stored = localStorage.getItem(VIEW_KEY);
    return stored && VALID_VIEWS.has(stored as View) ? (stored as View) : 'recent';
  }
  let view = $state<View>(loadInitialView());
  $effect(() => {
    if (typeof localStorage !== 'undefined') {
      try { localStorage.setItem(VIEW_KEY, view); } catch {}
    }
  });

  type SortKey = 'modified' | 'created' | 'name' | 'size';
  let sortKey = $state<SortKey>('modified');

  // Folder filter — set by clicking a card in the folders view. The
  // page swaps to a flat list of just that folder's notes. '' means
  // unfiltered; '__root__' isolates vault-root files; any other value
  // is a folder prefix.
  let folderFilter = $state('');

  let notes = $state<Note[]>([]);
  let pinned = $state<Set<string>>(new Set());
  let loading = $state(false);

  // Search state — debounced via $effect that wakes when q changes.
  let q = $state('');
  let searchResults = $state<Note[]>([]);
  let searching = $state(false);
  let searchTimer: ReturnType<typeof setTimeout> | null = null;

  // Quick-create form state. Default folder = current view's
  // contextual root (if user is in a folder, that becomes the create
  // target). Falls back to vault root when none.
  let createOpen = $state(false);
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
    return () => {
      unsub();
      document.removeEventListener('visibilitychange', onVisible);
      window.removeEventListener('focus', onVisible);
      reload.cancel();
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

  let allSorted = $derived.by(() => {
    if (view !== 'all') return [];
    const arr = notes.filter(passesFolderFilter);
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

  async function quickCreate() {
    const title = createTitle.trim();
    if (!title) return;
    creating = true;
    try {
      // Slug-ify the title for the filename. Spaces → hyphens; strip
      // anything outside [A-Za-z0-9-]. The user's title remains in the
      // body's H1 so the prettier name is preserved for display.
      const slug = title.replace(/[^\w\s-]/g, '').trim().replace(/\s+/g, '-');
      const folder = createFolder.trim().replace(/^\/+|\/+$/g, '');
      const path = (folder ? folder + '/' : '') + slug + '.md';
      const body = `# ${title}\n\n`;
      await api.createNote({ path, body });
      createOpen = false;
      createTitle = '';
      // Don't reset folder — common workflow is "create another in the
      // same folder". User can clear manually.
      goto(`/notes/${encodeURIComponent(path)}`);
    } catch (e) {
      toast.error('create failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      creating = false;
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
  <header class="px-3 sm:px-4 py-3 border-b border-surface1 flex-shrink-0 sticky top-0 z-20 bg-mantle/85 supports-[backdrop-filter]:bg-mantle/60 supports-[backdrop-filter]:backdrop-blur-md">
    <div class="flex items-center justify-between gap-3 mb-3">
      <div class="min-w-0">
        <h1 class="text-xl sm:text-2xl font-semibold text-text truncate">Notes</h1>
        <p class="text-xs text-dim mt-0.5">{notes.length} notes · {pinnedCount} pinned</p>
      </div>
      <button
        onclick={() => (createOpen = true)}
        class="px-3 py-1.5 bg-primary text-on-primary rounded text-sm font-medium hover:opacity-90 flex items-center gap-1.5 flex-shrink-0"
      >
        <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round">
          <path d="M12 5v14M5 12h14"/>
        </svg>
        <span class="hidden sm:inline">New note</span>
        <span class="sm:hidden">New</span>
      </button>
    </div>

    <!-- Search bar — always visible. Typing here flips view to
         'search' so the user gets the body-aware index, not just the
         tree filter. -->
    <input
      bind:value={q}
      onfocus={() => { if (q.trim()) view = 'search'; }}
      oninput={() => { if (q.trim()) view = 'search'; }}
      placeholder="Search notes (full-text)…"
      class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm sm:text-base text-text placeholder-dim focus:outline-none focus:border-primary"
    />

    <!-- View tabs. Persisted to localStorage. The strip is wrapped
         in a relative shell so we can paint a thin gradient on the
         right edge — a visual cue that more tabs sit beyond the
         visible area on phones. The shell is scroll-padded + uses
         scroll-snap so a thumbing user lands cleanly on a tab
         instead of mid-button. -->
    <div class="mt-3 relative">
      <nav
        class="flex gap-1 overflow-x-auto text-xs notes-view-strip"
        aria-label="view"
      >
        {#each [
          // Counts are cheap O(n) approximations — they intentionally
          // do NOT consume the heavy view derivations (recent /
          // pinnedList / alphaSections / tagSections / folderCards).
          // Those only run when the user actually opens that view.
          // 'Recent' caps at 30, so notes.length<30 we show n, else 30.
          { id: 'recent' as View, label: 'Recent', count: Math.min(30, notes.length) },
          { id: 'pinned' as View, label: 'Pinned', count: pinnedCount },
          { id: 'tree' as View, label: 'Tree', count: notes.length },
          { id: 'all' as View, label: 'All', count: notes.length },
          { id: 'alpha' as View, label: 'A–Z', count: notes.length },
          { id: 'tags' as View, label: 'Tags', count: tagCount },
          { id: 'folders' as View, label: 'Folders', count: folderCount },
          ...(q.trim() ? [{ id: 'search' as View, label: 'Search', count: searchResults.length }] : [])
        ] as t}
          <button
            onclick={() => {
              // Clicking the All tab directly clears any folder filter
              // — folder filtering is only set via the Folders cards;
              // hitting the tab on its own should mean "show everything".
              if (t.id === 'all' && view !== 'all') folderFilter = '';
              view = t.id;
            }}
            class="px-3 py-1.5 rounded transition-colors flex-shrink-0 snap-start
              {view === t.id ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1 border border-surface1'}"
          >
            {t.label} <span class="opacity-70 ml-0.5">{t.count}</span>
          </button>
        {/each}
        {#if view === 'all' && folderFilter}
          <button
            type="button"
            onclick={() => (folderFilter = '')}
            class="ml-1 inline-flex items-center gap-1 px-2 py-1 rounded bg-warning/10 text-warning hover:bg-warning/20 flex-shrink-0"
            title="Clear folder filter"
          >
            <span>📁</span>
            <span class="font-medium">{folderFilter === '__root__' ? '/' : folderFilter}</span>
            <span aria-hidden="true">×</span>
          </button>
        {/if}
        {#if view === 'all'}
          <!-- Sort options ride alongside the tabs in the same scroll
               strip on mobile so they don't wrap to a second line and
               push the list down; on sm+ they pin to the right side. -->
          <span class="ml-auto flex items-center gap-1 text-dim flex-shrink-0">
            <span class="hidden sm:inline">sort by</span>
            <span class="sm:hidden text-[10px] uppercase tracking-wider">sort</span>
            {#each [
              { id: 'modified' as SortKey, label: 'modified' },
              { id: 'created' as SortKey, label: 'created' },
              { id: 'name' as SortKey, label: 'name' },
              { id: 'size' as SortKey, label: 'size' }
            ] as s}
              <button
                onclick={() => (sortKey = s.id)}
                class="px-2 py-1 rounded flex-shrink-0 {sortKey === s.id ? 'text-primary font-medium' : 'hover:text-text'}"
              >{s.label}</button>
            {/each}
          </span>
        {/if}
      </nav>
      <!-- Right-edge fade — hints at off-screen tabs on the small
           viewports where the strip overflows. Hidden on sm+ where
           the strip typically fits. The fade uses color-mix on the
           current theme's mantle so it works in light + dark
           palettes without per-theme overrides. -->
      <div
        class="pointer-events-none absolute top-0 right-0 h-full w-6 sm:hidden"
        style="background: linear-gradient(to left, var(--color-mantle), color-mix(in srgb, var(--color-mantle) 0%, transparent));"
        aria-hidden="true"
      ></div>
    </div>
  </header>

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
    {:else if view === 'alpha'}
      {#if notes.length === 0}
        <div class="p-8 text-center text-sm text-dim">No notes in your vault.</div>
      {:else}
        <div class="overflow-y-auto h-full">
          {#each alphaSections as sec (sec.letter)}
            <div class="sticky top-0 z-10 bg-mantle/95 backdrop-blur px-3 sm:px-4 py-1 text-[11px] uppercase tracking-wider text-dim border-b border-surface1/60">
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
            <div class="sticky top-0 z-10 bg-mantle/95 backdrop-blur px-3 sm:px-4 py-1.5 border-b border-surface1/60 flex items-center gap-2">
              {#if sec.untagged}
                <span class="text-[11px] uppercase tracking-wider text-dim italic">untagged</span>
              {:else}
                <span class="text-xs px-1.5 py-0.5 rounded bg-secondary/15 text-secondary">#{sec.tag}</span>
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
    {:else if view === 'folders'}
      {#if folderCards.length === 0}
        <div class="p-8 text-center text-sm text-dim">No folders yet — create a note with a path like <code class="text-text">Notes/Ideas/foo.md</code> to get started.</div>
      {:else}
        <div class="overflow-y-auto h-full p-3 sm:p-4">
          <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-2 sm:gap-3">
            {#each folderCards as card (card.name)}
              <button
                type="button"
                onclick={() => { folderFilter = card.isRoot ? '__root__' : card.name; view = 'all'; }}
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

{#snippet row(n: Note)}
  {@const isPinned = pinned.has(n.path)}
  <li class="group hover:bg-surface0/60 transition-colors">
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
                <span class="px-1 rounded bg-secondary/10 text-secondary">#{tag}</span>
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
    onclick={(e) => { if (e.target === e.currentTarget) createOpen = false; }}
    onkeydown={(e) => { if (e.key === 'Escape') createOpen = false; }}
    tabindex="-1"
  >
    <div class="bg-mantle border border-surface1 rounded-lg w-full max-w-md p-5 shadow-xl">
      <h2 id="newnote-title" class="text-base font-semibold text-text mb-3">New note</h2>
      <form
        onsubmit={(e) => { e.preventDefault(); quickCreate(); }}
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
          <p class="text-[11px] text-dim mt-1">Leave empty for vault root.</p>
        </div>
        <div class="flex justify-end gap-2 pt-1">
          <button
            type="button"
            onclick={() => (createOpen = false)}
            class="px-3 py-1.5 text-sm text-dim hover:text-text"
          >cancel</button>
          <button
            type="submit"
            disabled={!createTitle.trim() || creating}
            class="px-4 py-1.5 bg-primary text-on-primary rounded text-sm font-medium disabled:opacity-50"
          >{creating ? 'creating…' : 'Create'}</button>
        </div>
      </form>
    </div>
  </div>
{/if}

<style>
  /* Horizontally-scrolling view-tab strip on mobile. Snap so a
     thumb-flick lands on a tab; hide the scrollbar so the right-
     edge gradient (drawn by the sibling div) reads as the only
     "more tabs" hint. Desktop with a wide viewport still gets the
     scrollable strip but the gradient is hidden via sm:hidden so
     it doesn't paint over a fully-visible toggle row. */
  .notes-view-strip {
    scroll-snap-type: x mandatory;
    scroll-padding-left: 0.75rem;
    scrollbar-width: none;
    -ms-overflow-style: none;
  }
  .notes-view-strip::-webkit-scrollbar {
    display: none;
  }
</style>
