<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { type Note } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { relativeTime } from '$lib/util/relativeTime';
  import NotesTree from '$lib/notes/NotesTree.svelte';
  import NotesPageHeader from '$lib/notes/NotesPageHeader.svelte';
  import NotesQuickFilters from '$lib/notes/NotesQuickFilters.svelte';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import { createNotesListViewState } from '$lib/notes/notesListViewState.svelte';
  import { createNotesListData } from '$lib/notes/notesListData.svelte';
  import { createNotesListCapture } from '$lib/notes/notesListCapture.svelte';

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
  //
  // View / sort / filter / overflow / collection state lives in
  // notesListViewState — the page reaches it through viewCtl. Loaded
  // notes, the debounced search, every view-shape derivation, and the
  // row-level mutations live in notesListData (dataCtl) and surface
  // the free-text query the viewCtl's collections need to read.
  const viewCtl = createNotesListViewState({
    getQ: () => dataCtl.q,
    setQ: (v) => { dataCtl.q = v; }
  });
  const dataCtl = createNotesListData({
    isAuthed: () => !!$auth,
    getView: () => viewCtl.view,
    getFolderFilter: () => viewCtl.folderFilter,
    getTagFilter: () => viewCtl.tagFilter,
    getSortKey: () => viewCtl.sortKey
  });
  // Quick-capture dialog flow (three modes: capture textarea → AI-
  // staged review → manual fallback). State + every action live in
  // notesListCapture; the page mounts the dialog template and wires
  // the ⌘N / deep-link / Web Share Target install paths in onMount.
  const captureCtl = createNotesListCapture({
    getNotes: () => dataCtl.notes,
    getTopTags: () => dataCtl.topTags
  });

  onMount(() => {
    dataCtl.loadAll();
    const unsub = onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') dataCtl.reload.trigger();
    });
    // Mobile browsers (and any backgrounded tab) suspend the WS, so
    // notes created/edited on another device while we were away never
    // make it through. Refetch on the visibility flip so a returning
    // tab catches up without the user having to pull-to-refresh. We
    // call loadAll directly (bypassing the coalesce window) so the
    // user sees the fresh list as soon as the tab returns.
    const onVisible = () => {
      if (document.visibilityState === 'visible') dataCtl.loadAll();
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
      captureCtl.openCapture();
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
        captureCtl.openCapture();
        if (shared) {
          // Build a friendly pre-fill: title on its own line, then
          // body text, then url (skip url if it's already inside the
          // text — some clients duplicate it). captureCtl.openCapture() resets
          // captureCtl.captureText to '', so this assignment must come AFTER.
          const parts: string[] = [];
          if (shTitle) parts.push(shTitle);
          if (shText) parts.push(shText);
          if (shUrl && !shText.includes(shUrl)) parts.push(shUrl);
          captureCtl.captureText = parts.join('\n\n');
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
      dataCtl.reload.cancel();
      captureCtl.dispose();
    };
  });

  // Falls back to a calendar date past 30 days — past that, "5w
  // ago" reads less well than "Apr 12".
  const fmtRelative = (iso: string) => relativeTime(iso, { dateThresholdDays: 30 });

  function fmtSize(bytes?: number): string {
    if (!bytes) return '';
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${Math.round(bytes / 102.4) / 10} KB`;
    return `${Math.round(bytes / 1024 / 102.4) / 10} MB`;
  }

</script>

<div class="h-full flex flex-col">
  <!-- Stream T — slim single-row page header. Title + count on the
       left, search · view-switcher · More-views · New on the right.
       Active filter pills + sort segmented sit in the QuickFilters
       row below so the chrome stays mute. Saves ~60-70px of vertical
       space vs the previous three-row layout. -->
  <NotesPageHeader
    view={viewCtl.view}
    bind:q={dataCtl.q}
    notesCount={dataCtl.notes.length}
    pinnedCount={dataCtl.pinnedCount}
    searchResultsCount={dataCtl.searchResults.length}
    moreViewsOpen={viewCtl.moreViewsOpen}
    activeOverflowLabel={viewCtl.activeOverflowLabel}
    onSelectView={viewCtl.selectView}
    onToggleMoreViews={() => (viewCtl.moreViewsOpen = !viewCtl.moreViewsOpen)}
    onPickOverflowView={viewCtl.pickOverflowView}
    onMoreViewsKey={viewCtl.onMoreViewsKey}
    onQuickCapture={captureCtl.openCapture}
    onSearchInput={(v) => { if (v.trim()) viewCtl.view = 'search'; }}
    onSearchFocus={() => { if (dataCtl.q.trim()) viewCtl.view = 'search'; }}
  />

  <!-- Quick-filter row. Renders only on 'all' (sort segmented +
       folder/tag clear pills) and 'search' with an active query
       (Save-as-collection). Self-hides on every other view. -->
  <NotesQuickFilters
    view={viewCtl.view}
    folderFilter={viewCtl.folderFilter}
    tagFilter={viewCtl.tagFilter}
    sortKey={viewCtl.sortKey}
    searchActive={!!dataCtl.q.trim()}
    onClearFolder={() => (viewCtl.folderFilter = '')}
    onClearTag={() => (viewCtl.tagFilter = '')}
    onPickSort={(s) => (viewCtl.sortKey = s)}
    onSaveCollection={viewCtl.saveCurrentAsCollection}
  />

  <div class="flex-1 min-h-0 overflow-hidden">
    {#if viewCtl.view === 'tree'}
      <NotesTree />
    {:else if dataCtl.loading && dataCtl.notes.length === 0}
      <div class="p-3 space-y-2">
        {#each Array(8) as _}
          <Skeleton class="h-12 w-full" />
        {/each}
      </div>
    {:else if viewCtl.view === 'search' && dataCtl.q.trim() && !dataCtl.searching && dataCtl.searchResults.length === 0}
      <div class="p-8 text-center text-sm text-dim">No notes match <code class="text-text">{dataCtl.q}</code></div>
    {:else if viewCtl.view === 'stream'}
      {#if dataCtl.notes.length === 0}
        <div class="p-8 text-center text-sm text-dim">No notes yet — hit <kbd class="px-1 rounded bg-surface1 text-text">⌘N</kbd> to capture your first thought.</div>
      {:else}
        <div class="overflow-y-auto h-full">
          {#each dataCtl.streamSections as sec (sec.id)}
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
    {:else if viewCtl.view === 'alpha'}
      {#if dataCtl.notes.length === 0}
        <div class="p-8 text-center text-sm text-dim">No notes in your vault.</div>
      {:else}
        <div class="overflow-y-auto h-full">
          {#each dataCtl.alphaSections as sec (sec.letter)}
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
    {:else if viewCtl.view === 'tags'}
      {#if dataCtl.tagSections.length === 0}
        <div class="p-8 text-center text-sm text-dim">No tagged notes yet. Add a <code class="text-text">tags:</code> field in frontmatter or use <code class="text-text">#tag</code> in the body.</div>
      {:else}
        <div class="overflow-y-auto h-full">
          {#each dataCtl.tagSections as sec (sec.tag)}
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
    {:else if viewCtl.view === 'collections'}
      <div class="overflow-y-auto h-full p-3 sm:p-4 space-y-2">
        {#if viewCtl.collections.length === 0}
          <div class="p-8 text-center text-sm text-dim">
            No viewCtl.collections yet. Run a search and click <span class="text-text">Save as collection…</span> to pin it here.
          </div>
        {:else}
          <ul class="divide-y divide-surface1/50">
            {#each viewCtl.collections as c (c.id)}
              <li class="group hover:bg-surface0 transition-colors">
                <div class="flex items-center gap-3 px-3 sm:px-4 py-2.5">
                  <button
                    type="button"
                    onclick={() => viewCtl.applyCollection(c)}
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
                    onclick={() => viewCtl.deleteCollection(c.id)}
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
    {:else if viewCtl.view === 'folders'}
      {#if dataCtl.folderCards.length === 0}
        <div class="p-8 text-center text-sm text-dim">No folders yet — create a note with a path like <code class="text-text">Notes/Ideas/foo.md</code> to get started.</div>
      {:else}
        <div class="overflow-y-auto h-full p-3 sm:p-4">
          <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-2 sm:gap-3">
            {#each dataCtl.folderCards as card (card.name)}
              <button
                type="button"
                onclick={() => { viewCtl.folderFilter = card.isRoot ? '__root__' : card.name; viewCtl.tagFilter = ''; viewCtl.view = 'all'; }}
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
    {:else if dataCtl.activeList.length === 0}
      <div class="p-8 text-center text-sm text-dim">
        {#if viewCtl.view === 'pinned'}No pinned notes yet. Click the ★ icon on any note to pin it.
        {:else if viewCtl.view === 'recent'}No notes in your vault.
        {:else}Empty.{/if}
      </div>
    {:else}
      <ul class="overflow-y-auto h-full divide-y divide-surface1/50">
        {#each dataCtl.activeList as n (n.path)}
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
  {@const isPinned = dataCtl.pinned.has(n.path)}
  <li class="group hover:bg-surface0 transition-colors">
    <div class="flex items-center gap-2 px-3 sm:px-4 py-1.5">
      <button
        type="button"
        onclick={() => dataCtl.open(n)}
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
          onclick={() => dataCtl.togglePin(n)}
          aria-label={isPinned ? 'unpin' : 'pin'}
          class="w-7 h-7 flex items-center justify-center text-dim hover:text-warning rounded"
          title={isPinned ? 'Unpin' : 'Pin'}
        >★</button>
        <button
          onclick={() => dataCtl.del(n)}
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
  {@const isPinned = dataCtl.pinned.has(n.path)}
  <li class="group hover:bg-surface0 transition-colors">
    <div class="flex items-center gap-3 px-3 sm:px-4 py-2.5">
      <button
        type="button"
        onclick={() => dataCtl.open(n)}
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
          onclick={() => dataCtl.togglePin(n)}
          aria-label={isPinned ? 'unpin' : 'pin'}
          class="w-8 h-8 flex items-center justify-center text-dim hover:text-warning rounded"
          title={isPinned ? 'Unpin' : 'Pin'}
        >★</button>
        <button
          onclick={() => dataCtl.rename(n)}
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
          onclick={() => dataCtl.del(n)}
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

{#if captureCtl.createOpen}
  <!-- Click-outside to close. Inner div stops propagation. -->
  <div
    role="dialog"
    aria-modal="true"
    aria-labelledby="newnote-title"
    class="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4"
    onclick={(e) => { if (e.target === e.currentTarget) captureCtl.closeCapture(); }}
    onkeydown={captureCtl.onCaptureKey}
    tabindex="-1"
  >
    <div class="bg-mantle border border-surface1 rounded-lg w-full max-w-xl p-5 shadow-xl">
      {#if captureCtl.captureMode === 'capture'}
        <div class="flex items-center justify-between mb-3">
          <h2 id="newnote-title" class="text-base font-semibold text-text">Quick capture</h2>
          <button
            type="button"
            onclick={() => { captureCtl.captureMode = 'manual'; }}
            class="text-[11px] text-dim hover:text-text"
            title="Skip AI and enter title + folder manually"
          >Skip AI →</button>
        </div>
        <textarea
          bind:value={captureCtl.captureText}
          placeholder="Capture anything — title, tags, folder, and links will be suggested."
          autofocus
          rows="6"
          disabled={captureCtl.captureBusy}
          class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary resize-y disabled:opacity-60"
        ></textarea>
        {#if captureCtl.captureBusy}
          <div class="mt-2 text-[11px] text-dim flex items-center gap-2">
            <span class="inline-block w-2 h-2 rounded-full bg-primary animate-pulse"></span>
            <span>AI is reading… {captureCtl.captureRaw.length > 0 ? `(${captureCtl.captureRaw.length} chars)` : ''}</span>
          </div>
        {:else}
          <div class="mt-2 text-[11px] text-dim">
            <kbd class="px-1 rounded bg-surface1 text-text">⌘Enter</kbd> to save straight to Inbox without AI.
          </div>
        {/if}
        <div class="flex justify-end gap-2 pt-3">
          <button
            type="button"
            onclick={captureCtl.closeCapture}
            class="px-3 py-1.5 text-sm text-dim hover:text-text"
          >cancel</button>
          <button
            type="button"
            onclick={captureCtl.runAiCapture}
            disabled={!captureCtl.captureText.trim() || captureCtl.captureBusy}
            class="px-4 py-1.5 bg-primary text-on-primary rounded text-sm font-medium disabled:opacity-50"
          >{captureCtl.captureBusy ? 'thinking…' : 'Capture'}</button>
        </div>
      {:else if captureCtl.captureMode === 'staging'}
        <div class="flex items-center justify-between mb-3">
          <h2 id="newnote-title" class="text-base font-semibold text-text">Review</h2>
          <button
            type="button"
            onclick={() => { captureCtl.captureMode = 'capture'; }}
            class="text-[11px] text-dim hover:text-text"
          >← back</button>
        </div>
        <div class="space-y-3">
          <div>
            <label for="stage-title" class="block text-[11px] uppercase tracking-wider text-dim mb-1">Title</label>
            <input
              id="stage-title"
              bind:value={captureCtl.stageTitle}
              class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
            />
          </div>
          <div>
            <!-- Not a real form-label — the field below is a chip-grid
                 + free-text input, no single control to bind to. -->
            <span class="block text-[11px] uppercase tracking-wider text-dim mb-1">Tags</span>
            <div class="flex flex-wrap items-center gap-1 px-2 py-1.5 bg-surface0 border border-surface1 rounded">
              {#each captureCtl.stageTags as tag (tag)}
                <span class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-surface1 text-secondary text-xs">
                  #{tag}
                  <button
                    type="button"
                    onclick={() => captureCtl.removeStageTag(tag)}
                    aria-label="remove tag"
                    class="text-dim hover:text-error"
                  >×</button>
                </span>
              {/each}
              <input
                bind:value={captureCtl.stageTagInput}
                onkeydown={(e) => {
                  if (e.key === 'Enter' || e.key === ',') { e.preventDefault(); captureCtl.addStageTag(); }
                  else if (e.key === 'Backspace' && !captureCtl.stageTagInput && captureCtl.stageTags.length > 0) {
                    e.preventDefault();
                    captureCtl.stageTags = captureCtl.stageTags.slice(0, -1);
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
              bind:value={captureCtl.stageFolder}
              placeholder="Inbox"
              class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-text font-mono text-xs focus:outline-none focus:border-primary"
            />
          </div>
          {#if captureCtl.stageWikilinkCandidates.length > 0}
            <div>
              <!-- Group heading for a row of toggle buttons; not
                   bound to a single input. -->
              <span class="block text-[11px] uppercase tracking-wider text-dim mb-1">Related notes <span class="text-dim/70 normal-case">(toggle to insert as <code class="text-text">[[wikilinks]]</code>)</span></span>
              <div class="flex flex-wrap gap-1">
                {#each captureCtl.stageWikilinkCandidates as cand (cand)}
                  {@const on = captureCtl.stageWikilinksChosen.has(cand)}
                  <button
                    type="button"
                    onclick={() => captureCtl.toggleWikilink(cand)}
                    class="text-xs px-1.5 py-0.5 rounded border transition-colors
                      {on ? 'bg-primary text-on-primary border-primary' : 'bg-surface0 border-surface1 text-subtext hover:bg-surface1'}"
                  >[[{cand}]]</button>
                {/each}
              </div>
            </div>
          {/if}
          <details class="text-[11px] text-dim">
            <summary class="cursor-pointer hover:text-text">show body ({captureCtl.captureText.length} chars)</summary>
            <pre class="mt-1 p-2 bg-surface0 border border-surface1 rounded text-text whitespace-pre-wrap font-mono text-[11px] max-h-32 overflow-y-auto">{captureCtl.captureText}</pre>
          </details>
        </div>
        <div class="flex justify-end gap-2 pt-4">
          <button
            type="button"
            onclick={captureCtl.closeCapture}
            class="px-3 py-1.5 text-sm text-dim hover:text-text"
          >cancel</button>
          <button
            type="button"
            onclick={captureCtl.saveStaged}
            disabled={!captureCtl.stageTitle.trim() || captureCtl.creating}
            class="px-4 py-1.5 bg-primary text-on-primary rounded text-sm font-medium disabled:opacity-50"
          >{captureCtl.creating ? 'saving…' : 'Save'}</button>
        </div>
      {:else}
        <!-- manual fallback -->
        <div class="flex items-center justify-between mb-3">
          <h2 id="newnote-title" class="text-base font-semibold text-text">New note</h2>
          <button
            type="button"
            onclick={() => { captureCtl.captureMode = 'capture'; }}
            class="text-[11px] text-dim hover:text-text"
          >← try AI</button>
        </div>
        <form
          onsubmit={(e) => { e.preventDefault(); captureCtl.manualCreate(); }}
          class="space-y-3"
        >
          <div>
            <label for="nn-title" class="block text-[11px] uppercase tracking-wider text-dim mb-1">Title</label>
            <input
              id="nn-title"
              bind:value={captureCtl.createTitle}
              placeholder="My brilliant idea"
              required
              class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-text focus:outline-none focus:border-primary"
            />
          </div>
          <div>
            <label for="nn-folder" class="block text-[11px] uppercase tracking-wider text-dim mb-1">Folder <span class="text-dim/70 normal-case">(optional)</span></label>
            <input
              id="nn-folder"
              bind:value={captureCtl.createFolder}
              placeholder="Notes/Ideas"
              class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-text font-mono text-xs focus:outline-none focus:border-primary"
            />
            <p class="text-[11px] text-dim mt-1">Leave empty for vault root. The captured body is preserved.</p>
          </div>
          <div class="flex justify-end gap-2 pt-1">
            <button
              type="button"
              onclick={captureCtl.closeCapture}
              class="px-3 py-1.5 text-sm text-dim hover:text-text"
            >cancel</button>
            <button
              type="submit"
              disabled={!captureCtl.createTitle.trim() || captureCtl.creating}
              class="px-4 py-1.5 bg-primary text-on-primary rounded text-sm font-medium disabled:opacity-50"
            >{captureCtl.creating ? 'creating…' : 'Create'}</button>
          </div>
        </form>
      {/if}
    </div>
  </div>
{/if}

