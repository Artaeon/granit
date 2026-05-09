<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, type BookDiscoverResult, type BookDiscoverSource } from '$lib/api';
  import { errorMessage } from '$lib/util/errorMessage';
  import { toast } from '$lib/components/toast';
  import { loadStoredString, saveStoredString } from '$lib/util/storage';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import Skeleton from '$lib/components/Skeleton.svelte';

  // Discover — search Project Gutenberg + Standard Ebooks (legal,
  // public-domain or hand-typeset open content) and one-click
  // import into <vault>/Books/. The shelf at /books picks the
  // imported file up automatically via the WS state.changed
  // broadcast the import handler emits.
  //
  // Why no auto-search-on-load: the catalogues span ~70k titles;
  // a default query would be either misleading or arbitrary. Empty
  // search → suggested-query chips ("Marcus Aurelius", "Austen",
  // "Tolstoy") so the user has obvious starting points without
  // pretending to "browse the catalog".

  type Source = BookDiscoverSource;

  // Persist the user's source filter so a return visit reopens
  // with their preferred subset (default: both sources on).
  const SOURCE_KEY = 'granit.books.discover.sources';
  const DEFAULT_ENABLED: Set<Source> = new Set(['gutenberg', 'standardebooks']);
  function loadSources(): Set<Source> {
    const raw = loadStoredString(SOURCE_KEY, '');
    if (!raw) return new Set(DEFAULT_ENABLED);
    const list = raw.split(',').filter((s): s is Source =>
      s === 'gutenberg' || s === 'standardebooks'
    );
    return list.length > 0 ? new Set(list) : new Set(DEFAULT_ENABLED);
  }
  let enabled = $state<Set<Source>>(loadSources());
  $effect(() => saveStoredString(SOURCE_KEY, [...enabled].join(',')));

  let q = $state('');
  let busy = $state(false);
  let error = $state('');
  let results = $state<BookDiscoverResult[]>([]);
  let searched = $state(false);
  let importingId = $state<string | null>(null); // source+externalId of the in-flight import

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
      searched = false;
      return;
    }
    if (enabled.size === 0) {
      error = 'Pick at least one source';
      return;
    }
    busy = true;
    error = '';
    searched = true;
    try {
      const r = await api.discoverBooks(term, {
        sources: [...enabled],
        limit: 30
      });
      results = r.results;
    } catch (e) {
      error = errorMessage(e);
      results = [];
    } finally {
      busy = false;
    }
  }

  async function importResult(r: BookDiscoverResult) {
    const key = r.source + ':' + r.externalId;
    if (importingId) return;
    importingId = key;
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
      importingId = null;
    }
  }

  function toggleSource(s: Source) {
    const next = new Set(enabled);
    if (next.has(s)) next.delete(s);
    else next.add(s);
    enabled = next;
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

  // Group results by source so the user sees clusters (and a
  // header with the source name) rather than a scrambled grid.
  let grouped = $derived.by(() => {
    const map: Record<Source, BookDiscoverResult[]> = {
      gutenberg: [],
      standardebooks: []
    };
    for (const r of results) map[r.source].push(r);
    return map;
  });

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
    subtitle="Search Project Gutenberg and Standard Ebooks — public-domain classics, free to read"
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
  <div class="mt-2 flex flex-col sm:flex-row gap-3">
    <div class="flex-1 flex gap-2">
      <input
        type="search"
        bind:value={q}
        onkeydown={onKey}
        placeholder="Search by title, author, subject…"
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
    <div class="flex gap-2 flex-wrap">
      {#each [{ id: 'gutenberg' as Source, label: 'Gutenberg' }, { id: 'standardebooks' as Source, label: 'Standard Ebooks' }] as s (s.id)}
        <label class="flex items-center gap-1.5 text-sm px-3 py-2 bg-surface0 border border-surface1 rounded cursor-pointer hover:border-primary {enabled.has(s.id) ? 'border-primary text-text' : 'text-dim'}">
          <input
            type="checkbox"
            checked={enabled.has(s.id)}
            onchange={() => toggleSource(s.id)}
            class="accent-primary"
          />
          {s.label}
        </label>
      {/each}
    </div>
  </div>

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
        <p><strong class="text-text">Project Gutenberg</strong> — ~70 000 titles, mostly classics in the public domain. Format quality varies, but coverage is unmatched.</p>
        <p><strong class="text-text">Standard Ebooks</strong> — ~600 carefully typeset editions of public-domain works. The reading experience matches a hardcover. <a href="https://standardebooks.org/about" target="_blank" class="underline hover:text-text">Learn more →</a></p>
        <p class="pt-3 text-xs">Imported books land in <code class="text-text bg-surface0 px-1 py-0.5 rounded">&lt;vault&gt;/Books/</code> and appear on your shelf immediately.</p>
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
    <div class="mt-6 space-y-8">
      {#each (['gutenberg', 'standardebooks'] as Source[]) as src (src)}
        {#if grouped[src].length > 0}
          <section>
            <header class="flex items-center gap-2 mb-3">
              <h2 class="text-sm font-medium text-text">
                {src === 'gutenberg' ? 'Project Gutenberg' : 'Standard Ebooks'}
              </h2>
              <span class="text-xs text-dim">{grouped[src].length} result{grouped[src].length === 1 ? '' : 's'}</span>
            </header>
            <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
              {#each grouped[src] as r (r.source + r.externalId)}
                <article class="flex gap-3 p-3 border border-surface1 rounded-lg bg-surface0/50 hover:bg-surface0 transition-colors">
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
                      <p class="text-xs text-subtext line-clamp-1 mt-0.5">{r.authors.join(', ')}</p>
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
          </section>
        {/if}
      {/each}
    </div>

    <!-- License clarity — public-domain doesn't mean "use however"
         (some Standard Ebooks editions add typeset value the user
         should attribute on republication). Quiet footer reminder. -->
    <p class="text-[11px] text-dim mt-12 mb-8 max-w-2xl leading-relaxed">
      Project Gutenberg titles are in the US public domain.
      Standard Ebooks titles are public domain in the US and licensed under
      <a href="https://standardebooks.org/about" target="_blank" rel="noopener" class="underline hover:text-text">CC0</a>
      — read freely; check their licence page if you plan to republish.
    </p>
  {/if}
</div>
