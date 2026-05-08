<!--
  ReferenceNotePanel — pin any note to read alongside the current
  one. Lives in the right info rail of the note editor; the user
  picks a note via fuzzy search, the body renders read-only as a
  scrollable preview. Pick persists per-current-note so reopening
  the editor on the same note shows the same reference.

  Why not the existing split view: split is the SAME note in two
  panes (edit + preview). This is two DIFFERENT notes — the user is
  writing about something they're reading. Different muscle.

  Performance: notes list is fetched once on mount + on WS note.*
  events. Body is fetched on-demand when picked. Memoised by path
  in-component so toggling between two recently-picked references
  doesn't re-fetch.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Note } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';
  import { toast } from '$lib/components/toast';

  let {
    currentPath = '',
    currentBody = '',
    currentTitle = ''
  }: { currentPath?: string; currentBody?: string; currentTitle?: string } = $props();

  const STORAGE_PREFIX = 'granit.note.ref:';
  function refKey(path: string): string {
    return STORAGE_PREFIX + path;
  }

  let allNotes = $state<{ path: string; title: string }[]>([]);
  let allNotesLoaded = $state(false);
  let pickerOpen = $state(false);
  let q = $state('');
  let activePath = $state<string | null>(null);
  let activeBody = $state<string>('');
  let activeTitle = $state<string>('');
  let activeLoading = $state(false);
  let activeError = $state<string>('');
  let activeCollapsed = $state(false);

  // Body cache — keep the last few fetched references in memory so
  // toggling between two recent picks doesn't re-fetch. LRU shape:
  // map insertion order is preserved, we evict the oldest when the
  // map grows past 5.
  const bodyCache = new Map<string, { title: string; body: string }>();

  async function loadNotes() {
    try {
      const r = await api.listNotes({ limit: 5000 });
      allNotes = r.notes.map((n) => ({ path: n.path, title: n.title }));
    } finally {
      allNotesLoaded = true;
    }
  }

  onMount(() => {
    loadNotes();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') {
        loadNotes();
        // If our currently-pinned reference is the one that changed,
        // re-fetch its body so the preview stays accurate.
        if (activePath && ev.type === 'note.changed' && ev.path === activePath) {
          bodyCache.delete(activePath);
          void pickReference(activePath, /*persist*/ false);
        }
      }
    });
  });

  // Restore previously-pinned reference on currentPath change. If
  // we change to a new note that doesn't have a saved ref, clear
  // the panel rather than carry over the old one.
  $effect(() => {
    if (!currentPath) return;
    try {
      const saved = localStorage.getItem(refKey(currentPath));
      if (saved && saved !== currentPath) {
        void pickReference(saved, /*persist*/ false);
      } else {
        activePath = null;
        activeBody = '';
        activeTitle = '';
      }
    } catch {}
  });

  // Filtered picker results — basename + title fuzzy contains.
  // Fuzzy enough to find "stoic" → "Stoicism — Marcus.md" without
  // a real fuzzy lib; we cap at 30 to keep render cheap.
  let results = $derived.by(() => {
    const term = q.trim().toLowerCase();
    if (!term) {
      // Recent / closest notes: just the first 30 from the index
      // (already sorted server-side by mod time desc).
      return allNotes.filter((n) => n.path !== currentPath).slice(0, 30);
    }
    const out = allNotes.filter((n) => {
      if (n.path === currentPath) return false;
      const hay = (n.title + ' ' + n.path).toLowerCase();
      return hay.includes(term);
    });
    return out.slice(0, 30);
  });

  async function pickReference(path: string, persist = true) {
    if (!path) return;
    activePath = path;
    activeError = '';
    if (persist && currentPath) {
      try { localStorage.setItem(refKey(currentPath), path); } catch {}
    }
    pickerOpen = false;
    q = '';
    // Cache hit?
    const cached = bodyCache.get(path);
    if (cached) {
      activeBody = cached.body;
      activeTitle = cached.title;
      return;
    }
    activeLoading = true;
    activeBody = '';
    activeTitle = '';
    try {
      const note = await api.getNote(path);
      activeBody = note.body ?? '';
      activeTitle = note.title || path.replace(/\.md$/, '');
      // LRU insert — evict oldest if > 5.
      bodyCache.set(path, { body: activeBody, title: activeTitle });
      if (bodyCache.size > 5) {
        const oldest = bodyCache.keys().next().value;
        if (oldest) bodyCache.delete(oldest);
      }
    } catch (e) {
      activeError = e instanceof Error ? e.message : String(e);
    } finally {
      activeLoading = false;
    }
  }

  function clearReference() {
    activePath = null;
    activeBody = '';
    activeTitle = '';
    activeError = '';
    if (currentPath) {
      try { localStorage.removeItem(refKey(currentPath)); } catch {}
    }
  }

  function navigateWikilink(target: string) {
    // Basic wikilink navigation: try to match by title or path.
    const t = target.trim();
    const match = allNotes.find(
      (n) => n.path === t || n.path.replace(/\.md$/, '') === t || n.title === t
    );
    if (match) {
      void pickReference(match.path);
    }
  }

  // ─── AI compare: current note vs reference ──────────────────────
  // Streams a structured comparison: shared themes, where they
  // disagree, and what each says that the other doesn't. Lives
  // here (not the menu) because the comparison is meaningless
  // without a pinned reference, and the panel is the home of that
  // pin. Audit-gated chat pipeline.
  let cmpBusy = $state(false);
  let cmpAbort: AbortController | null = null;
  let cmpResponse = $state('');
  let cmpError = $state('');
  let cmpOpen = $state(false);

  function clip(text: string, max = 6000): string {
    const t = text.trim();
    return t.length > max ? t.slice(0, max) + '\n…(truncated)' : t;
  }

  async function compareNotes() {
    if (cmpBusy) return;
    if (!activePath || !activeBody.trim() || !currentBody.trim()) {
      toast.info('Pick a reference first.');
      return;
    }
    cmpAbort?.abort();
    cmpAbort = new AbortController();
    cmpBusy = true;
    cmpError = '';
    cmpResponse = '';
    cmpOpen = true;
    let buf = '';
    try {
      await api.chatStream(
        [
          {
            role: 'system',
            content:
              'You compare two notes from the user\'s vault. Return markdown with three short sections, in this order:\n## Shared\n2-3 bullets — themes/claims both notes agree on or both treat as relevant.\n## They diverge\n2-3 bullets — where the two notes say different (or contradictory) things, naming each side. Format like "**A** says … but **B** says …".\n## Only in A · Only in B\nTwo short sub-lists, each 2-3 bullets, of points that appear in one note and not the other.\nBe concrete. Quote a phrase rather than paraphrase generically. Skip a section if there\'s nothing real to put in it. No preamble.'
          },
          {
            role: 'user',
            content:
              `**A: ${currentTitle || currentPath}**\n\n${clip(currentBody)}\n\n---\n\n**B: ${activeTitle || activePath}**\n\n${clip(activeBody)}`
          }
        ],
        undefined,
        {
          onChunk: (c) => { buf += c; cmpResponse = buf; },
          onDone: () => {},
          onError: (err) => { cmpError = err.message; }
        },
        cmpAbort.signal
      );
    } finally {
      cmpBusy = false;
      cmpAbort = null;
    }
  }
  function dismissCompare() {
    cmpAbort?.abort();
    cmpBusy = false;
    cmpResponse = '';
    cmpError = '';
    cmpOpen = false;
  }
</script>

<div class="text-sm">
  {#if activePath}
    <!-- Header row: title + collapse + close. The reference body
         is collapsed by default if it's a long doc so the rail
         doesn't stretch — user expands explicitly. -->
    <div class="flex items-baseline gap-1 mb-1.5">
      <button
        type="button"
        onclick={() => (activeCollapsed = !activeCollapsed)}
        aria-expanded={!activeCollapsed}
        class="flex-1 text-left text-xs text-text hover:text-primary truncate flex items-baseline gap-1"
        title={activePath}
      >
        <span class="text-dim text-[10px]">{activeCollapsed ? '▸' : '▾'}</span>
        <span class="truncate">{activeTitle || activePath}</span>
      </button>
      <button
        type="button"
        onclick={compareNotes}
        disabled={cmpBusy}
        class="text-[10px] text-primary hover:underline disabled:opacity-50 px-1 inline-flex items-center gap-0.5"
        title="AI compare current note vs this reference"
      >
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-2.5 h-2.5">
          <path d="M12 3l1.2 4.2L17 9l-3.8 1.8L12 15l-1.2-4.2L7 9l3.8-1.8L12 3z" stroke-linejoin="round"/>
        </svg>
        {cmpBusy ? '…' : 'compare'}
      </button>
      <a
        href="/notes/{encodeURIComponent(activePath)}"
        class="text-[10px] text-dim hover:text-primary px-1"
        title="open the reference in the main editor"
      >open ↗</a>
      <button
        type="button"
        onclick={() => (pickerOpen = true)}
        class="text-[10px] text-dim hover:text-text px-1"
        title="swap reference"
      >swap</button>
      <button
        type="button"
        onclick={clearReference}
        aria-label="unpin reference"
        class="text-[10px] text-dim hover:text-error px-1"
      >×</button>
    </div>
    {#if cmpOpen}
      <div class="mb-2 p-2 bg-primary/5 border border-primary/20 rounded">
        <div class="flex items-baseline gap-2 mb-1.5">
          <span class="text-[10px] uppercase tracking-wider text-primary font-medium">comparison</span>
          <span class="flex-1"></span>
          {#if cmpBusy}
            <button onclick={() => cmpAbort?.abort()} class="text-[10px] text-warning hover:underline">cancel</button>
          {:else if cmpResponse}
            <button onclick={compareNotes} class="text-[10px] text-secondary hover:underline">regenerate</button>
          {/if}
          <button onclick={dismissCompare} class="text-[10px] text-dim hover:text-text">dismiss</button>
        </div>
        {#if cmpError}
          <p class="text-[11px] text-error">{cmpError}</p>
        {:else if cmpBusy && !cmpResponse}
          <p class="text-[11px] text-dim italic">comparing…</p>
        {:else}
          <div class="reference-prose text-[12px]">
            <MarkdownRenderer body={cmpResponse} />
          </div>
        {/if}
      </div>
    {/if}
    {#if !activeCollapsed}
      {#if activeLoading}
        <div class="text-[11px] text-dim italic">loading…</div>
      {:else if activeError}
        <div class="text-[11px] text-error bg-error/10 rounded px-2 py-1">{activeError}</div>
      {:else}
        <!-- Read-only preview. Constrained max-height so a long
             reference doesn't push the rest of the rail off-screen.
             Wikilinks inside the reference navigate within the
             panel rather than the main editor — keeping the rail's
             role as a research surface, not a navigation hijack. -->
        <div class="max-h-96 overflow-y-auto bg-mantle/30 border border-surface1 rounded px-3 py-2 text-[12px] reference-prose">
          <MarkdownRenderer body={activeBody} onWikilink={navigateWikilink} />
        </div>
      {/if}
    {/if}
  {:else if pickerOpen}
    <div>
      <input
        bind:value={q}
        placeholder="filter notes…"
        class="w-full px-2 py-1 mb-1 bg-surface0 border border-surface1 rounded text-xs text-text placeholder-dim focus:outline-none focus:border-primary"
      />
      {#if !allNotesLoaded}
        <div class="text-[11px] text-dim italic px-2">loading…</div>
      {:else if results.length === 0}
        <div class="text-[11px] text-dim italic px-2">no matches</div>
      {:else}
        <ul class="max-h-72 overflow-y-auto space-y-px">
          {#each results as n (n.path)}
            <li>
              <button
                type="button"
                onclick={() => pickReference(n.path)}
                class="w-full text-left px-2 py-1 rounded text-xs hover:bg-surface0 truncate text-text hover:text-primary"
                title={n.path}
              >
                {n.title || n.path.replace(/\.md$/, '')}
              </button>
            </li>
          {/each}
        </ul>
      {/if}
      <button
        type="button"
        onclick={() => (pickerOpen = false)}
        class="text-[11px] text-dim hover:text-text mt-1"
      >cancel</button>
    </div>
  {:else}
    <button
      type="button"
      onclick={() => (pickerOpen = true)}
      class="w-full text-left text-xs text-dim hover:text-primary px-2 py-1.5 rounded border border-dashed border-surface1 hover:border-primary/40"
      title="pick a note to read alongside this one"
    >
      + pin a reference note
    </button>
  {/if}
</div>

<style>
  /* Slightly tighter prose inside the reference panel — the rail is
     narrower than the main pane and a normal max-w-prose lays out
     awkwardly at this width. */
  .reference-prose :global(h1) { font-size: 1rem; margin-top: 0.5rem; }
  .reference-prose :global(h2) { font-size: 0.95rem; margin-top: 0.5rem; }
  .reference-prose :global(h3) { font-size: 0.9rem; }
  .reference-prose :global(p) { margin: 0.4rem 0; }
  .reference-prose :global(ul),
  .reference-prose :global(ol) { margin: 0.4rem 0; padding-left: 1.25rem; }
</style>
