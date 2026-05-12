<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { goto } from '$app/navigation';
  import { auth } from '$lib/stores/auth';
  import { api, type BookShelfRow } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { errorMessage } from '$lib/util/errorMessage';
  import { relativeTime } from '$lib/util/relativeTime';
  import { loadStoredString, saveStoredString } from '$lib/util/storage';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import Skeleton from '$lib/components/Skeleton.svelte';

  // Books shelf — covers grid + reading-progress bar per row, with
  // a list-view toggle for users who want density. The shelf only
  // surfaces the bare minimum to render a card; the reader page
  // pulls the full spine + TOC on demand.
  //
  // Library lives at <vault>/Books/. The user adds books by dropping
  // .epub files into that folder via their preferred sync (rsync,
  // Syncthing, file manager, scp). Phase 2 will add an in-app
  // "Discover" surface that searches Project Gutenberg / Standard
  // Ebooks / Open Library and downloads into the same folder.

  type View = 'grid' | 'list';
  const VIEW_KEY = 'granit.books.view';
  let view = $state<View>(loadStoredString(VIEW_KEY, 'grid') as View);
  $effect(() => saveStoredString(VIEW_KEY, view));

  let books = $state<BookShelfRow[]>([]);
  let loading = $state(false);
  let error = $state('');
  let q = $state('');

  // Cover blob URLs are cached locally so we revoke them on
  // unmount (browsers leak object URLs otherwise). Map: bookId →
  // blob URL. Null entry means "we tried, no cover" — distinct
  // from "not yet loaded" (undefined) so the fallback renders
  // immediately on subsequent renders.
  const coverURLs = new Map<string, string | null>();
  let coverVersion = $state(0); // bump to re-render after a fill

  async function load() {
    if (!$auth) return;
    loading = true;
    error = '';
    try {
      const r = await api.listBooks();
      books = r.books;
      // Lazily fetch covers — kicks off in parallel but doesn't
      // block the initial render. Each fill bumps coverVersion so
      // the {#key} block redraws when its url settles.
      for (const b of books) {
        if (!b.hasCover || coverURLs.has(b.id)) continue;
        api
          .bookCoverBlobURL(b.id)
          .then((url) => {
            coverURLs.set(b.id, url);
            coverVersion++;
          })
          .catch(() => {
            coverURLs.set(b.id, null);
            coverVersion++;
          });
      }
    } catch (e) {
      error = errorMessage(e);
    } finally {
      loading = false;
    }
  }

  let filtered = $derived.by(() => {
    const term = q.trim().toLowerCase();
    if (!term) return books;
    return books.filter((b) => {
      const hay = (b.title + ' ' + (b.authors ?? []).join(' ')).toLowerCase();
      return hay.includes(term);
    });
  });

  // Currently-reading rail — books with a non-zero last-read-at,
  // sorted by recency. Pinned at the top of the shelf so the user
  // can resume in one tap.
  let resuming = $derived.by(() =>
    [...books]
      .filter((b) => b.lastReadAt && b.progressPct < 100)
      .sort((a, b) => (a.lastReadAt! < b.lastReadAt! ? 1 : -1))
      .slice(0, 4)
  );

  function fmtBytes(n: number): string {
    if (n < 1024) return `${n} B`;
    if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)} kB`;
    return `${(n / 1024 / 1024).toFixed(1)} MB`;
  }

  function open(b: BookShelfRow) {
    goto(`/books/${encodeURIComponent(b.id)}`);
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path?.startsWith('.granit/books/')) {
        load();
      }
    });
  });

  onDestroy(() => {
    for (const url of coverURLs.values()) {
      if (url) URL.revokeObjectURL(url);
    }
  });
</script>

<div class="px-4 sm:px-6 pt-4 max-w-7xl mx-auto w-full">
  <PageHeader title="Books" subtitle="Read EPUBs from your vault — progress and highlights stay with you">
    {#snippet actions()}
      <input
        type="search"
        bind:value={q}
        placeholder="Search title or author"
        class="text-sm px-3 py-1.5 bg-surface0 border border-surface1 rounded text-text placeholder:text-dim focus:outline-none focus:border-primary w-44 sm:w-56"
      />
      <div class="flex rounded border border-surface1 overflow-hidden">
        <button
          class="px-2.5 py-1.5 text-xs {view === 'grid'
            ? 'bg-primary text-on-primary'
            : 'bg-surface0 text-subtext hover:text-text'}"
          onclick={() => (view = 'grid')}
        >
          Grid
        </button>
        <button
          class="px-2.5 py-1.5 text-xs {view === 'list'
            ? 'bg-primary text-on-primary'
            : 'bg-surface0 text-subtext hover:text-text'}"
          onclick={() => (view = 'list')}
        >
          List
        </button>
      </div>
      <a
        href="/books/discover"
        class="text-sm px-3 py-1.5 bg-primary text-on-primary rounded hover:bg-primary/90 flex items-center gap-1"
        title="Search Project Gutenberg + Standard Ebooks"
      >
        + Discover
      </a>
    {/snippet}
  </PageHeader>

  {#if loading && books.length === 0}
    <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-4 mt-4">
      {#each Array(8) as _, i (i)}
        <Skeleton class="aspect-[2/3] rounded" />
      {/each}
    </div>
  {:else if error}
    <div class="bg-surface0 border border-error text-error rounded p-4 mt-4 text-sm">
      Couldn't load shelf: {error}
    </div>
  {:else if books.length === 0}
    <EmptyState
      title="No books yet"
      description="Use Discover to pull free public-domain classics, or drop your own .epub files into <vault>/Books/."
    />
  {:else}
    {#if resuming.length > 0 && !q}
      <section class="mt-2 mb-4">
        <h2 class="text-xs uppercase tracking-wider text-dim mb-2 px-1">Continue reading</h2>
        <div class="flex gap-3 overflow-x-auto pb-2 -mx-1 px-1">
          {#each resuming as b (b.id + '-resume')}
            <button
              onclick={() => open(b)}
              class="flex-shrink-0 w-32 text-left group"
            >
              {#key coverVersion}
                <div
                  class="aspect-[2/3] rounded bg-surface1 border border-surface1 group-hover:border-primary overflow-hidden flex items-center justify-center"
                >
                  {#if b.hasCover && coverURLs.get(b.id)}
                    <img src={coverURLs.get(b.id)!} alt="" class="w-full h-full object-cover" />
                  {:else}
                    <span class="text-dim text-2xl px-2 text-center font-serif">
                      {b.title.slice(0, 2)}
                    </span>
                  {/if}
                </div>
              {/key}
              <div class="mt-1 h-1 rounded bg-surface1 overflow-hidden">
                <div
                  class="h-full bg-primary"
                  style="width: {b.progressPct}%"
                ></div>
              </div>
              <div class="text-xs mt-1 truncate" title={b.title}>{b.title}</div>
              <div class="text-[11px] text-dim truncate">
                ch {b.furthestChapter + 1} of {b.totalChapters || '?'} · {b.lastReadAt ? relativeTime(b.lastReadAt) : ''}
              </div>
            </button>
          {/each}
        </div>
      </section>
    {/if}

    {#if view === 'grid'}
      <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-5 mt-2">
        {#each filtered as b (b.id)}
          <button onclick={() => open(b)} class="text-left group">
            {#key coverVersion}
              <div
                class="aspect-[2/3] rounded bg-surface1 border border-surface1 group-hover:border-primary overflow-hidden flex items-center justify-center transition-colors"
              >
                {#if b.hasCover && coverURLs.get(b.id)}
                  <img src={coverURLs.get(b.id)!} alt="" class="w-full h-full object-cover" />
                {:else}
                  <div class="text-center px-3">
                    <div class="font-serif text-text text-base leading-tight line-clamp-3">{b.title}</div>
                    {#if b.authors && b.authors.length > 0}
                      <div class="text-xs text-dim mt-2 line-clamp-2">{b.authors[0]}</div>
                    {/if}
                  </div>
                {/if}
              </div>
            {/key}
            {#if b.totalChapters > 0}
              <div class="mt-1.5 h-1 rounded bg-surface1 overflow-hidden">
                <div class="h-full bg-primary" style="width: {b.progressPct}%"></div>
              </div>
            {/if}
            <div class="text-sm font-medium mt-1.5 line-clamp-2 leading-tight" title={b.title}>{b.title}</div>
            {#if b.authors && b.authors.length > 0}
              <div class="text-xs text-dim line-clamp-1">{b.authors.join(', ')}</div>
            {/if}
            <div class="text-[11px] text-dim mt-0.5">{fmtBytes(b.bytes)}</div>
          </button>
        {/each}
      </div>
    {:else}
      <table class="w-full text-sm mt-2">
        <thead class="text-left text-xs uppercase tracking-wider text-dim border-b border-surface1">
          <tr>
            <th class="py-2 px-2">Title</th>
            <th class="py-2 px-2 hidden sm:table-cell">Authors</th>
            <th class="py-2 px-2 w-24 hidden md:table-cell">Size</th>
            <th class="py-2 px-2 w-32">Progress</th>
            <th class="py-2 px-2 w-32 hidden md:table-cell">Last read</th>
          </tr>
        </thead>
        <tbody>
          {#each filtered as b (b.id)}
            <tr
              onclick={() => open(b)}
              class="border-b border-surface1 hover:bg-surface0 cursor-pointer"
            >
              <td class="py-2 px-2 font-medium">{b.title}</td>
              <td class="py-2 px-2 text-subtext hidden sm:table-cell">{(b.authors ?? []).join(', ')}</td>
              <td class="py-2 px-2 text-dim hidden md:table-cell">{fmtBytes(b.bytes)}</td>
              <td class="py-2 px-2">
                <div class="h-1.5 rounded bg-surface1 overflow-hidden w-24">
                  <div class="h-full bg-primary" style="width: {b.progressPct}%"></div>
                </div>
                <div class="text-[11px] text-dim mt-0.5">{Math.round(b.progressPct)}%</div>
              </td>
              <td class="py-2 px-2 text-dim text-xs hidden md:table-cell">
                {b.lastReadAt ? relativeTime(b.lastReadAt) : '—'}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}

    <p class="text-xs text-dim mt-8 mb-12 max-w-2xl">
      Tip: Granit reads .epub files from your <code class="text-text bg-surface0 px-1 py-0.5 rounded">Books/</code>
      vault folder. Use <a href="/books/discover" class="underline hover:text-text">Discover</a> to pull
      free public-domain classics from Project Gutenberg, or drop your own DRM-free EPUBs in directly —
      they appear here on the next reload.
    </p>
  {/if}
</div>
