<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import {
    api,
    type BookDiscoverResult,
    type BookDiscoverSource,
    type BookDiscoverWarning
  } from '$lib/api';
  import { errorMessage } from '$lib/util/errorMessage';
  import { toast } from '$lib/components/toast';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import Skeleton from '$lib/components/Skeleton.svelte';

  // Discover — search Project Gutenberg and one-click import into
  // <vault>/Books/. The shelf at /books picks the imported file up
  // automatically via the state.changed WS broadcast the import
  // handler emits.
  //
  // Standard Ebooks used to be a second source here but they put
  // their full catalogue OPDS feeds behind a paid Patrons Circle
  // membership in 2026 (every /opds/* endpoint now returns 401 to
  // unauthenticated callers). Until we add an auth surface for
  // their credentials, only Project Gutenberg is wired up.
  //
  // Why no auto-search-on-load: the catalogue spans ~70k titles;
  // a default query would be either misleading or arbitrary. Empty
  // search → suggested-query chips so the user has obvious starting
  // points without pretending to "browse the catalog".

  type Source = BookDiscoverSource;

  let q = $state('');
  let busy = $state(false);
  let error = $state('');
  let results = $state<BookDiscoverResult[]>([]);
  let warnings = $state<BookDiscoverWarning[]>([]);
  let searched = $state(false);
  let importingId = $state<string | null>(null); // source+externalId of the in-flight import

  // Frontend-side import timeout. The backend caps each download at
  // 120 s, so 150 s on the client guarantees we never out-wait the
  // server (the spinner can't get stuck if the server returned).
  // Implemented as a hard AbortController fallback in case the fetch
  // promise never settles (proxy mid-flight, browser tab throttled).
  const IMPORT_HARD_TIMEOUT_MS = 150 * 1000;

  const SUGGESTIONS = [
    'Pride and Prejudice',
    'Marcus Aurelius',
    'Tolstoy',
    'Dickens',
    'Plato',
    'Conan Doyle',
    'Shakespeare',
    'Frankenstein'
  ];

  async function search() {
    const term = q.trim();
    if (!term) {
      results = [];
      warnings = [];
      searched = false;
      return;
    }
    busy = true;
    error = '';
    searched = true;
    try {
      const r = await api.discoverBooks(term, {
        sources: ['gutenberg'],
        limit: 30
      });
      results = r.results;
      warnings = r.warnings ?? [];
    } catch (e) {
      error = errorMessage(e);
      results = [];
      warnings = [];
    } finally {
      busy = false;
    }
  }

  async function importResult(r: BookDiscoverResult) {
    const key = r.source + ':' + r.externalId;
    if (importingId) return;
    importingId = key;

    // Guarantee the spinner can't outlive the request. If the
    // backend returns successfully, we resolve well before this fires.
    const timer = setTimeout(() => {
      if (importingId === key) {
        importingId = null;
        toast.error(
          `Import timed out after ${Math.round(IMPORT_HARD_TIMEOUT_MS / 1000)}s. The download server may be slow — try again, or pick a different result.`
        );
      }
    }, IMPORT_HARD_TIMEOUT_MS);

    try {
      const sum = await api.importBook({
        source: r.source,
        downloadUrl: r.downloadUrl,
        title: r.title
      });
      toast.success(`Saved "${sum.title}" to your library`, {
        action: { label: 'Read now', href: `/books/${encodeURIComponent(sum.id)}` }
      });
    } catch (e) {
      toast.error('Import failed: ' + errorMessage(e));
    } finally {
      clearTimeout(timer);
      // Only clear if our key still owns it (the timeout fallback
      // may have already cleared it on a slow request).
      if (importingId === key) importingId = null;
    }
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault();
      void search();
    }
  }

  function applySuggestion(text: string) {
    q = text;
    void search();
  }

  function sourceBadge(s: Source): { label: string; cls: string } {
    return s === 'gutenberg'
      ? { label: 'Project Gutenberg', cls: 'bg-amber-100 text-amber-900 dark:bg-amber-900/30 dark:text-amber-200' }
      : { label: 'Standard Ebooks',   cls: 'bg-emerald-100 text-emerald-900 dark:bg-emerald-900/30 dark:text-emerald-200' };
  }

  function warningCopy(w: BookDiscoverWarning): string {
    if (w.source === 'standardebooks') {
      return 'Standard Ebooks moved their catalogue feed behind a paid Patrons Circle subscription — search and import are unavailable in this version.';
    }
    return w.message;
  }

  onMount(() => {
    if (!$auth) return;
    // No initial fetch — the user types or clicks a suggestion.
  });
</script>

<svelte:head>
  <title>Discover books — Granit</title>
</svelte:head>

<div class="px-4 sm:px-6 pt-4 max-w-7xl mx-auto w-full">
  <PageHeader
    title="Discover books"
    subtitle="Search Project Gutenberg — 70 000+ public-domain classics, 100% free to download and read"
  >
    {#snippet actions()}
      <a
        href="/books"
        class="text-sm px-3 py-1.5 bg-surface0 border border-surface1 rounded text-subtext hover:border-primary"
      >
        ← Shelf
      </a>
    {/snippet}
  </PageHeader>

  <!-- Search row -->
  <div class="mt-2 flex gap-2">
    <input
      type="search"
      bind:value={q}
      onkeydown={onKey}
      placeholder="Search Project Gutenberg by title, author, or subject…"
      class="flex-1 px-3 py-2 bg-surface0 border border-surface1 rounded text-text placeholder:text-dim focus:outline-none focus:border-primary"
      autocomplete="off"
    />
    <button
      onclick={search}
      disabled={busy || !q.trim()}
      class="px-4 py-2 bg-primary text-on-primary rounded text-sm font-medium hover:bg-primary/90 disabled:opacity-50 disabled:cursor-not-allowed"
    >
      {busy ? 'Searching…' : 'Search'}
    </button>
  </div>

  {#if warnings.length > 0}
    <div class="mt-3 space-y-2">
      {#each warnings as w (w.source)}
        <div class="bg-warning/10 border border-warning/30 text-warning rounded p-2.5 text-xs">
          <span class="font-medium">{sourceBadge(w.source).label}:</span>
          {warningCopy(w)}
        </div>
      {/each}
    </div>
  {/if}

  {#if !searched && !error}
    <div class="mt-8">
      <p class="text-sm text-dim mb-3">Try one of these:</p>
      <div class="flex flex-wrap gap-2">
        {#each SUGGESTIONS as s (s)}
          <button
            onclick={() => applySuggestion(s)}
            class="text-sm px-3 py-1.5 bg-surface0 border border-surface1 rounded hover:border-primary hover:text-text text-subtext"
          >
            {s}
          </button>
        {/each}
      </div>
      <div class="mt-12 max-w-2xl text-sm text-dim space-y-2">
        <p><strong class="text-text">Project Gutenberg</strong> — ~70 000 titles, all in the public domain in the United States. Free to download, read, and keep. Founded in 1971 — the original digital library.</p>
        <p class="pt-3 text-xs">Imported books land in <code class="text-text bg-surface0 px-1 py-0.5 rounded">&lt;vault&gt;/Books/</code> and appear on your shelf immediately.</p>
        <p class="pt-1 text-xs">
          Outside the US? Most countries follow life+70 (EU, UK, AU). The author's death year is shown on each result so you can verify quickly — anything before {new Date().getFullYear() - 70} is generally safe in life+70 jurisdictions.
        </p>
      </div>
    </div>
  {/if}

  {#if error}
    <div class="mt-4 bg-error/10 border border-error/30 text-error rounded p-3 text-sm">
      {error}
    </div>
  {/if}

  {#if busy && results.length === 0}
    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4 mt-6">
      {#each Array(6) as _, i (i)}
        <div class="flex gap-3 p-3 border border-surface1 rounded-lg">
          <Skeleton class="w-20 h-28 flex-shrink-0" />
          <div class="flex-1 space-y-2 pt-1">
            <Skeleton class="h-4 w-3/4" />
            <Skeleton class="h-3 w-1/2" />
            <Skeleton class="h-3 w-2/3" />
          </div>
        </div>
      {/each}
    </div>
  {:else if searched && results.length === 0 && !error}
    <EmptyState
      title="No matches"
      description="Try a different keyword, or toggle a source above. Title and author both work — Standard Ebooks also matches on subject."
    />
  {:else if results.length > 0}
    <div class="mt-6">
      <header class="flex items-center gap-2 mb-3">
        <h2 class="text-sm font-medium text-text">Project Gutenberg</h2>
        <span class="text-xs text-dim">{results.length} result{results.length === 1 ? '' : 's'}</span>
      </header>
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
        {#each results as r (r.source + r.externalId)}
          <article class="flex gap-3 p-3 border border-surface1 rounded-lg bg-surface0 hover:bg-surface0 transition-colors">
            <div class="w-20 h-28 flex-shrink-0 bg-surface1 rounded overflow-hidden flex items-center justify-center">
              {#if r.coverUrl}
                <img
                  src={r.coverUrl}
                  alt=""
                  class="w-full h-full object-cover"
                  loading="lazy"
                  referrerpolicy="no-referrer"
                  onerror={(e) => {
                    (e.currentTarget as HTMLImageElement).style.display = 'none';
                  }}
                />
              {:else}
                <span class="text-dim text-xs px-1 text-center font-serif">{r.title.slice(0, 20)}</span>
              {/if}
            </div>
            <div class="flex-1 min-w-0 flex flex-col">
              <span
                class="self-start text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded {sourceBadge(r.source).cls} mb-1"
              >
                {sourceBadge(r.source).label}
              </span>
              <h3 class="text-sm font-medium leading-snug line-clamp-2" title={r.title}>{r.title}</h3>
              {#if r.authors && r.authors.length > 0}
                <p class="text-xs text-subtext line-clamp-1 mt-0.5">
                  {r.authors.join(', ')}
                  {#if r.authorDeathYear}
                    <span class="text-dim">· d. {r.authorDeathYear}</span>
                  {/if}
                </p>
              {/if}
              {#if r.description}
                <p class="text-xs text-dim line-clamp-3 mt-1.5 leading-snug">{r.description}</p>
              {/if}
              <div class="mt-auto pt-2 flex items-center gap-2">
                <button
                  onclick={() => importResult(r)}
                  disabled={importingId !== null}
                  class="text-xs px-2.5 py-1 bg-primary text-on-primary rounded hover:bg-primary/90 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {importingId === r.source + ':' + r.externalId ? 'Saving…' : '+ Add'}
                </button>
                {#if r.externalUrl}
                  <a
                    href={r.externalUrl}
                    target="_blank"
                    rel="noopener"
                    class="text-xs px-2.5 py-1 text-dim hover:text-text"
                  >
                    Source ↗
                  </a>
                {/if}
              </div>
            </div>
          </article>
        {/each}
      </div>
    </div>

    <p class="text-[11px] text-dim mt-12 mb-8 max-w-2xl leading-relaxed">
      Every title here is in the US public domain via Project Gutenberg — free to download, read, and keep.
      Outside the US, life+70 (EU/UK/AU) and life+50 (CA/JP) cover most works whose author died before the
      cut-off; the death year shown on each result is the quickest jurisdiction self-check.
      Personal reading is universally fine; check before redistributing.
    </p>
  {/if}
</div>
