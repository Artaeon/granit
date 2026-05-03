<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { auth } from '$lib/stores/auth';
  import { api, type Note } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import NotesTree from '$lib/notes/NotesTree.svelte';
  import Skeleton from '$lib/components/Skeleton.svelte';

  // Notes hub. Five view modes covering the full surface area: tree
  // (the classic hierarchy), recent (sorted by mod time), pinned (the
  // user's anchored set), all (flat list with sort options), search
  // (full-text via the search index). The TUI's notes overlay does
  // most of the same; the web matches.
  type View = 'recent' | 'tree' | 'pinned' | 'all' | 'search';
  const VIEW_KEY = 'granit.notes.view';
  let view = $state<View>(
    (typeof localStorage !== 'undefined' && (localStorage.getItem(VIEW_KEY) as View)) || 'recent'
  );
  $effect(() => {
    if (typeof localStorage !== 'undefined') {
      try { localStorage.setItem(VIEW_KEY, view); } catch {}
    }
  });

  type SortKey = 'modified' | 'created' | 'name' | 'size';
  let sortKey = $state<SortKey>('modified');

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
  onMount(() => {
    loadAll();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') loadAll();
    });
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

  let pinnedList = $derived.by(() => notes.filter((n) => pinned.has(n.path)));

  let recent = $derived.by(() => {
    return [...notes]
      .sort((a, b) => (a.modTime > b.modTime ? -1 : 1))
      .slice(0, 30);
  });

  let allSorted = $derived.by(() => {
    const arr = [...notes];
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

  function fmtRelative(iso: string): string {
    const d = new Date(iso);
    if (isNaN(d.getTime())) return '';
    const mins = Math.round((Date.now() - d.getTime()) / 60_000);
    if (mins < 1) return 'just now';
    if (mins < 60) return `${mins}m ago`;
    if (mins < 24 * 60) return `${Math.round(mins / 60)}h ago`;
    const days = Math.round(mins / (24 * 60));
    if (days < 7) return `${days}d ago`;
    if (days < 30) return `${Math.round(days / 7)}w ago`;
    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
  }

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
  <header class="px-3 sm:px-4 py-3 border-b border-surface1 flex-shrink-0">
    <div class="flex items-center justify-between gap-3 mb-3">
      <div>
        <h1 class="text-xl sm:text-2xl font-semibold text-text">Notes</h1>
        <p class="text-xs text-dim mt-0.5">{notes.length} notes · {pinnedList.length} pinned</p>
      </div>
      <button
        onclick={() => (createOpen = true)}
        class="px-3 py-1.5 bg-primary text-mantle rounded text-sm font-medium hover:opacity-90 flex items-center gap-1.5"
      >
        <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round">
          <path d="M12 5v14M5 12h14"/>
        </svg>
        New note
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

    <!-- View tabs. Persisted to localStorage. -->
    <nav class="mt-3 flex gap-1 overflow-x-auto text-xs" aria-label="view">
      {#each [
        { id: 'recent' as View, label: 'Recent', count: recent.length },
        { id: 'pinned' as View, label: 'Pinned', count: pinnedList.length },
        { id: 'tree' as View, label: 'Tree', count: notes.length },
        { id: 'all' as View, label: 'All', count: notes.length },
        ...(q.trim() ? [{ id: 'search' as View, label: 'Search', count: searchResults.length }] : [])
      ] as t}
        <button
          onclick={() => (view = t.id)}
          class="px-3 py-1.5 rounded transition-colors flex-shrink-0
            {view === t.id ? 'bg-primary text-mantle' : 'bg-surface0 text-subtext hover:bg-surface1 border border-surface1'}"
        >
          {t.label} <span class="opacity-70 ml-0.5">{t.count}</span>
        </button>
      {/each}
      {#if view === 'all'}
        <span class="ml-auto flex items-center gap-1 text-dim flex-shrink-0">
          <span>sort by</span>
          {#each [
            { id: 'modified' as SortKey, label: 'modified' },
            { id: 'created' as SortKey, label: 'created' },
            { id: 'name' as SortKey, label: 'name' },
            { id: 'size' as SortKey, label: 'size' }
          ] as s}
            <button
              onclick={() => (sortKey = s.id)}
              class="px-2 py-1 rounded {sortKey === s.id ? 'text-primary font-medium' : 'hover:text-text'}"
            >{s.label}</button>
          {/each}
        </span>
      {/if}
    </nav>
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
    {:else if activeList.length === 0}
      <div class="p-8 text-center text-sm text-dim">
        {#if view === 'pinned'}No pinned notes yet. Click the ★ icon on any note to pin it.
        {:else if view === 'recent'}No notes in your vault.
        {:else}Empty.{/if}
      </div>
    {:else}
      <ul class="overflow-y-auto h-full divide-y divide-surface1/50">
        {#each activeList as n (n.path)}
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
        {/each}
      </ul>
    {/if}
  </div>
</div>

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
            class="px-4 py-1.5 bg-primary text-mantle rounded text-sm font-medium disabled:opacity-50"
          >{creating ? 'creating…' : 'Create'}</button>
        </div>
      </form>
    </div>
  </div>
{/if}
