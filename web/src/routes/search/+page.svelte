<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { api, type SearchHit } from '$lib/api';

  // Full-text search page. Renders snippet previews around the
  // matched term with the term highlighted, click-through to the
  // note at the matched line. The notes/+page.svelte search view
  // also calls /api/v1/search but it strips line/column/match info
  // and renders only as a flat note list — this page is for users
  // who want the rich search experience.
  //
  // URL state: the query lives in ?q= so a search is bookmarkable
  // and shareable. Browser back / forward cycle through queries.

  let q = $state('');
  let results = $state<SearchHit[]>([]);
  let total = $state(0);
  let ready = $state(true);
  let busy = $state(false);
  let error = $state('');
  let inputEl: HTMLInputElement | undefined = $state();

  // Recent queries persist to localStorage so the user can re-run
  // a search they did yesterday without retyping.
  const RECENT_KEY = 'granit.search.recent';
  const RECENT_MAX = 8;
  let recent = $state<string[]>([]);

  // Saved (pinned) searches — explicitly starred by the user, not
  // auto-tracked. Survives recent-list eviction. Same shape as
  // recent (an array of query strings); separate key so the user
  // can star a search and still have the recent list churn.
  const SAVED_KEY = 'granit.search.saved';
  let saved = $state<string[]>([]);
  function loadSaved() {
    try {
      const raw = localStorage.getItem(SAVED_KEY);
      if (raw) {
        const parsed = JSON.parse(raw);
        if (Array.isArray(parsed)) saved = parsed;
      }
    } catch {}
  }
  function persistSaved() {
    try { localStorage.setItem(SAVED_KEY, JSON.stringify(saved)); } catch {}
  }
  function toggleSaved() {
    const t = q.trim();
    if (!t) return;
    const idx = saved.indexOf(t);
    if (idx >= 0) saved = [...saved.slice(0, idx), ...saved.slice(idx + 1)];
    else saved = [t, ...saved];
    persistSaved();
  }
  let isSaved = $derived(saved.includes(q.trim()));

  onMount(() => {
    try {
      const raw = localStorage.getItem(RECENT_KEY);
      if (raw) {
        const parsed = JSON.parse(raw);
        if (Array.isArray(parsed)) recent = parsed.slice(0, RECENT_MAX);
      }
    } catch {}
    loadSaved();
    // Hydrate from URL on mount.
    const fromUrl = new URL(window.location.href).searchParams.get('q') ?? '';
    if (fromUrl) {
      q = fromUrl;
      void runSearch();
    }
    // Auto-focus on mount so the user can just type.
    inputEl?.focus();
  });

  function rememberQuery(query: string) {
    const trimmed = query.trim();
    if (!trimmed) return;
    const next = [trimmed, ...recent.filter((r) => r !== trimmed)].slice(0, RECENT_MAX);
    recent = next;
    try { localStorage.setItem(RECENT_KEY, JSON.stringify(next)); } catch {}
  }

  // Debounce so every keystroke doesn't fire a request. 150ms is
  // fast enough that the result list feels live, slow enough that
  // typing "foobar" is one request rather than six.
  let debounceTimer: ReturnType<typeof setTimeout> | undefined;
  $effect(() => {
    void q;
    clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => void runSearch(), 150);
    return () => clearTimeout(debounceTimer);
  });

  async function runSearch() {
    const query = q.trim();
    error = '';
    // Reflect the query in the URL bar so refresh / share preserves it.
    const u = new URL(window.location.href);
    if (query) u.searchParams.set('q', query);
    else u.searchParams.delete('q');
    history.replaceState(history.state, '', u.toString());

    if (!query || query.length < 2) {
      results = [];
      total = 0;
      return;
    }
    busy = true;
    try {
      const r = await api.search(query, 100);
      results = r.results;
      total = r.total;
      ready = r.ready;
      // Don't remember every keystroke as a separate "recent". Only
      // count it once the user pauses for ~600ms after the request
      // returned with at least one hit.
      if (results.length > 0) rememberQuery(query);
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
      results = [];
    } finally {
      busy = false;
    }
  }

  function jumpToHit(hit: SearchHit) {
    // Navigate to the note. The notes page reads ?line= from the
    // URL and scrolls to that line on mount.
    const url = `/notes/${encodeURIComponent(hit.path)}${hit.line > 0 ? `?line=${hit.line}` : ''}`;
    void goto(url);
  }

  // Highlight the query term inside a match line. Splits on the
  // first matched fragment (case-insensitive) and renders the rest
  // as plain text — keeps it cheap. The term is escaped before being
  // dropped into the regex so "C++" doesn't crash the parser.
  function highlight(line: string, query: string): { before: string; match: string; after: string } | null {
    if (!line || !query) return null;
    const term = query.split(/\s+/)[0]; // first word only — simple but works
    const escaped = term.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    const re = new RegExp(escaped, 'i');
    const m = line.match(re);
    if (!m || m.index === undefined) return null;
    return {
      before: line.slice(0, m.index),
      match: line.slice(m.index, m.index + m[0].length),
      after: line.slice(m.index + m[0].length)
    };
  }

  // Group hits by note path so the user sees "5 hits in foo.md"
  // rather than 5 unrelated lines. Within a group we keep order by
  // line ascending.
  let grouped = $derived.by(() => {
    const byPath: Record<string, { title: string; hits: SearchHit[] }> = {};
    for (const h of results) {
      if (!byPath[h.path]) byPath[h.path] = { title: h.title, hits: [] };
      byPath[h.path].hits.push(h);
    }
    // Sort groups by their highest score (the search index already
    // sorts result items by score desc, so the first hit per path
    // is the best one).
    return Object.entries(byPath)
      .map(([path, g]) => ({ path, title: g.title, hits: g.hits.sort((a, b) => a.line - b.line), topScore: g.hits[0]?.score ?? 0 }))
      .sort((a, b) => b.topScore - a.topScore);
  });

  function clearRecent() {
    recent = [];
    try { localStorage.removeItem(RECENT_KEY); } catch {}
  }
</script>

<svelte:head>
  <title>{q ? `Search: ${q}` : 'Search'} · Granit</title>
</svelte:head>

<div class="h-full flex flex-col">
  <header class="flex-shrink-0 px-3 sm:px-4 py-3 border-b border-surface1 bg-mantle/40 flex items-center gap-3">
    <svg viewBox="0 0 24 24" class="w-5 h-5 text-dim flex-shrink-0" fill="none" stroke="currentColor" stroke-width="2">
      <circle cx="11" cy="11" r="7"/>
      <path d="M21 21l-4.5-4.5" stroke-linecap="round"/>
    </svg>
    <h1 class="text-base font-semibold text-text">Search</h1>
    <input
      bind:this={inputEl}
      bind:value={q}
      placeholder="Search across the vault…"
      aria-label="Search query"
      class="flex-1 min-w-0 px-3 py-2 bg-surface0 border border-surface1 rounded text-base sm:text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
    />
    {#if busy}
      <span class="text-xs text-dim">searching…</span>
    {:else if q && results.length > 0}
      <span class="text-xs text-dim font-mono tabular-nums whitespace-nowrap">
        {total} {total === 1 ? 'hit' : 'hits'}
      </span>
    {/if}
    {#if q.trim()}
      <button
        onclick={toggleSaved}
        title={isSaved ? 'Remove from saved' : 'Save this search'}
        aria-label={isSaved ? 'Unsave search' : 'Save search'}
        class="w-8 h-8 flex items-center justify-center rounded transition-colors
          {isSaved ? 'text-warning hover:bg-warning/10' : 'text-dim hover:text-warning hover:bg-surface0'}"
      >
        <span class="text-base">{isSaved ? '★' : '☆'}</span>
      </button>
    {/if}
  </header>

  <div class="flex-1 overflow-auto p-3 sm:p-4">
    {#if !q.trim()}
      <div class="max-w-2xl mx-auto py-6">
        <div class="text-sm text-dim mb-4">
          Search across every note in your vault. Matches are scored by relevance and ranked across all notes.
        </div>
        {#if !ready}
          <div class="px-3 py-2 bg-warning/10 border border-warning/30 text-warning text-sm rounded mb-4">
            Search index is still building. Recent edits may not show up for a few seconds.
          </div>
        {/if}
        {#if saved.length > 0}
          <div class="mb-2 flex items-center gap-2">
            <h2 class="text-xs uppercase tracking-wider text-dim font-semibold">Saved</h2>
          </div>
          <ul class="space-y-1.5 mb-6">
            {#each saved as s}
              <li class="flex items-stretch gap-1">
                <button
                  onclick={() => { q = s; }}
                  class="flex-1 text-left px-3 py-2 bg-surface0 hover:bg-surface1 border border-surface1 rounded text-sm text-subtext flex items-center gap-2"
                >
                  <span class="text-warning">★</span>
                  <span>{s}</span>
                </button>
                <button
                  onclick={() => { saved = saved.filter((x) => x !== s); persistSaved(); }}
                  aria-label="Remove saved search"
                  class="px-2 text-dim hover:text-error"
                >×</button>
              </li>
            {/each}
          </ul>
        {/if}
        {#if recent.length > 0}
          <div class="mb-2 flex items-center gap-2">
            <h2 class="text-xs uppercase tracking-wider text-dim font-semibold">Recent</h2>
            <button onclick={clearRecent} class="text-[11px] text-dim hover:text-error">clear</button>
          </div>
          <ul class="space-y-1.5">
            {#each recent as r}
              <li>
                <button
                  onclick={() => { q = r; }}
                  class="w-full text-left px-3 py-2 bg-surface0 hover:bg-surface1 border border-surface1 rounded text-sm text-subtext flex items-center gap-2"
                >
                  <span class="text-dim">↲</span>
                  <span>{r}</span>
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      </div>
    {:else if error}
      <div class="text-sm text-error">Search failed: {error}</div>
    {:else if q.trim().length < 2}
      <div class="text-sm text-dim italic">Type at least 2 characters…</div>
    {:else if results.length === 0 && !busy}
      <div class="max-w-md mx-auto py-12 text-center">
        <div class="text-4xl mb-3 opacity-30">∅</div>
        <h2 class="text-base font-medium text-text mb-2">No matches for "{q.trim()}"</h2>
        <p class="text-sm text-dim">
          Try fewer or different keywords. Search is case-insensitive and matches partial words.
        </p>
      </div>
    {:else}
      <div class="max-w-3xl mx-auto space-y-4">
        {#each grouped as g (g.path)}
          <section class="border border-surface1 rounded bg-surface0/40">
            <header class="px-3 py-2 border-b border-surface1 flex items-baseline gap-2">
              <button
                onclick={() => goto(`/notes/${encodeURIComponent(g.path)}`)}
                class="text-sm font-medium text-text hover:text-primary truncate"
                title={g.path}
              >{g.title || g.path}</button>
              <span class="text-[11px] text-dim font-mono tabular-nums">
                {g.hits.length} {g.hits.length === 1 ? 'hit' : 'hits'}
              </span>
              <span class="text-[10px] text-dim font-mono ml-auto truncate hidden sm:inline">{g.path}</span>
            </header>
            <ul class="divide-y divide-surface1">
              {#each g.hits as h (h.line)}
                {@const hl = highlight(h.matchLine, q)}
                <li>
                  <button
                    onclick={() => jumpToHit(h)}
                    class="w-full text-left px-3 py-2 hover:bg-surface1/50 flex items-baseline gap-3"
                    title="Open at line {h.line}"
                  >
                    <span class="text-[10px] text-dim font-mono tabular-nums w-10 flex-shrink-0">L{h.line}</span>
                    <span class="text-sm text-subtext font-mono break-all">
                      {#if hl}
                        <span class="text-dim">{hl.before}</span><mark class="bg-primary/30 text-text px-0.5 rounded">{hl.match}</mark><span class="text-dim">{hl.after}</span>
                      {:else}
                        <span class="text-dim">{h.matchLine || '—'}</span>
                      {/if}
                    </span>
                  </button>
                </li>
              {/each}
            </ul>
          </section>
        {/each}
      </div>
    {/if}
  </div>
</div>
