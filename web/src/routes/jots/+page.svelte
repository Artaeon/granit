<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { auth } from '$lib/stores/auth';
  import { api, type Jot, type Note } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import { toast } from '$lib/components/toast';

  // Amplenote-style infinite-scroll feed of every daily note. The page
  // talks to /api/v1/jots which paginates server-side — fetching N
  // dailies one-by-one would round-trip N times per page; the dedicated
  // endpoint keeps it to one round-trip per page no matter how many
  // years of dailies the user has accumulated.

  let jots = $state<Jot[]>([]);
  let cursor = $state<string | null>(null);
  let loading = $state(false);
  let done = $state(false);
  let error = $state('');

  // Daily folder, pulled from the user's config so jump-to-day knows
  // where to navigate. Read once on mount; settings changes mid-session
  // are rare and a refresh recovers cleanly.
  let dailyFolder = $state('');

  // Inline search state
  let searchText = $state('');
  let searchResults = $state<Note[]>([]);
  let searching = $state(false);

  // Sentinel + observer for infinite scroll.
  let sentinel: HTMLDivElement | undefined = $state();
  let observer: IntersectionObserver | null = null;

  // ── jot path / regex ──────────────────────────────────────────────
  // Mirrors the server's filter — a vault-relative path is a daily note
  // iff it's `<folder>/YYYY-MM-DD.md` or just `YYYY-MM-DD.md` (when no
  // folder is configured). Used to scope WS-driven refetches.
  function jotMatches(path: string): { date: string; folder: string } | null {
    const m = path.match(/^(?:(.+)\/)?(\d{4}-\d{2}-\d{2})\.md$/);
    if (!m) return null;
    const folder = m[1] ?? '';
    if (folder !== dailyFolder) return null;
    return { date: m[2], folder };
  }

  async function loadMore() {
    if (loading || done || !$auth) return;
    loading = true;
    error = '';
    try {
      const params: { before?: string; limit: number } = { limit: 20 };
      if (cursor) params.before = cursor;
      const r = await api.listJots(params);
      jots = [...jots, ...r.jots];
      cursor = r.nextBefore;
      if (!r.hasMore) done = true;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      loading = false;
    }
  }

  // Refetch a single jot by date and patch it into the array (or
  // prepend if it didn't exist before — i.e. today's daily was just
  // created).
  async function refetchJot(date: string) {
    try {
      // /jots is sort-desc + cursor-based; to grab a single date we ask
      // for the page just-after it (before = date+1day) limited to 1.
      const next = nextDateISO(date);
      const r = await api.listJots({ before: next, limit: 1 });
      const fresh = r.jots.find((j) => j.date === date);
      if (!fresh) {
        // The jot was deleted (or never existed) — drop it from the
        // list if it's there.
        jots = jots.filter((j) => j.date !== date);
        return;
      }
      const idx = jots.findIndex((j) => j.date === date);
      if (idx >= 0) {
        jots = [...jots.slice(0, idx), fresh, ...jots.slice(idx + 1)];
      } else {
        // New (today's) daily — prepend, keeping desc order intact.
        const insertAt = jots.findIndex((j) => j.date < date);
        if (insertAt < 0) jots = [...jots, fresh];
        else jots = [...jots.slice(0, insertAt), fresh, ...jots.slice(insertAt)];
      }
    } catch {
      // Soft-fail: a refetch error shouldn't blow up the page.
    }
  }

  function nextDateISO(d: string): string {
    const dt = new Date(d + 'T00:00:00');
    dt.setUTCDate(dt.getUTCDate() + 1);
    return dt.toISOString().slice(0, 10);
  }

  // ── header date formatting ────────────────────────────────────────
  // "Today" / "Yesterday" / weekday for ±6 days / full date otherwise.
  function relativeLabel(date: string, today: Date): string {
    const d = new Date(date + 'T00:00:00');
    const diff = Math.round((d.getTime() - today.getTime()) / 86400000);
    if (diff === 0) return 'Today';
    if (diff === -1) return 'Yesterday';
    if (diff === 1) return 'Tomorrow';
    if (diff > -7 && diff < 7) {
      return d.toLocaleDateString(undefined, { weekday: 'long' });
    }
    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
  }

  function fullLabel(date: string): string {
    const d = new Date(date + 'T00:00:00');
    return d.toLocaleDateString(undefined, {
      weekday: 'long',
      year: 'numeric',
      month: 'long',
      day: 'numeric'
    });
  }

  // Midnight today, recomputed reactively via $derived.by — used as the
  // anchor for relative-date labels ("Today" / "Yesterday" / etc).
  let today = $derived.by(() => {
    const d = new Date();
    return new Date(d.getFullYear(), d.getMonth(), d.getDate());
  });

  // ── handlers ──────────────────────────────────────────────────────
  function handleWikilink(target: string) {
    // Naive: same logic as note-detail page — try as-is, else treat as
    // a title and append .md. The server will 404 for missing notes;
    // the user lands on whatever the route can resolve.
    const path = target.endsWith('.md') ? target : target + '.md';
    goto(`/notes/${encodeURIComponent(path)}`);
  }

  function jumpToDate(e: Event) {
    const v = (e.target as HTMLInputElement).value;
    if (!v) return;
    const path = dailyFolder ? `${dailyFolder}/${v}.md` : `${v}.md`;
    goto(`/notes/${encodeURIComponent(path)}`);
  }

  async function runSearch() {
    const q = searchText.trim();
    if (!q) {
      searchResults = [];
      return;
    }
    searching = true;
    try {
      const r = await api.listNotes({ folder: dailyFolder, q, limit: 10 });
      searchResults = r.notes;
    } catch (e) {
      toast.error('search failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      searching = false;
    }
  }

  function clearSearch() {
    searchText = '';
    searchResults = [];
  }

  function openToday() {
    const t = new Date();
    const ds = `${t.getFullYear()}-${String(t.getMonth() + 1).padStart(2, '0')}-${String(t.getDate()).padStart(2, '0')}`;
    const path = dailyFolder ? `${dailyFolder}/${ds}.md` : `${ds}.md`;
    goto(`/notes/${encodeURIComponent(path)}`);
  }

  // Quick-jot composer — Amplenote-style "fire a thought into today"
  // without leaving the feed. Appends a timestamped line under a
  // `## Jots` section in today's daily, creating the section on first
  // use. The WS note.changed event then re-fetches today's jot in
  // the feed automatically.
  let composerText = $state('');
  let composerBusy = $state(false);
  let composerEl = $state<HTMLTextAreaElement | undefined>();

  function appendUnderJotsSection(body: string, line: string): string {
    // Find the `## Jots` heading; if present, splice the line in just
    // below it (after any existing list items the user has there). If
    // missing, append the section to the end of the document.
    const lines = body.split('\n');
    const idx = lines.findIndex((l) => /^##\s+Jots\b/i.test(l.trim()));
    if (idx === -1) {
      const sep = body.endsWith('\n') ? '' : '\n';
      return body + `${sep}\n## Jots\n${line}\n`;
    }
    // Walk past the heading to the end of the section (next `## ` or EOF).
    let end = lines.length;
    for (let i = idx + 1; i < lines.length; i++) {
      if (/^##\s+/.test(lines[i].trim())) {
        end = i;
        break;
      }
    }
    // Insert before `end`, trimming trailing empty lines so the new
    // line sits flush with the section content.
    let insertAt = end;
    while (insertAt > idx + 1 && lines[insertAt - 1].trim() === '') insertAt--;
    lines.splice(insertAt, 0, line);
    return lines.join('\n');
  }

  async function submitJot() {
    const text = composerText.trim();
    if (!text || composerBusy) return;
    composerBusy = true;
    try {
      const note = await api.daily('today');
      const t = new Date();
      const hh = String(t.getHours()).padStart(2, '0');
      const mm = String(t.getMinutes()).padStart(2, '0');
      // Multi-line input collapses to "; " separators so the appended
      // line stays a single bullet. Original line breaks are preserved
      // by markdown viewers since the line ends with a bullet.
      const flat = text.replace(/\n+/g, '; ');
      const newBody = appendUnderJotsSection(note.body ?? '', `- ${hh}:${mm} — ${flat}`);
      await api.putNote(note.path, {
        frontmatter: note.frontmatter ?? undefined,
        body: newBody
      });
      composerText = '';
      toast.success('jot saved');
      // WS will re-fetch; queue an immediate optimistic refetch too in
      // case the WS round-trip lags.
      const today = `${t.getFullYear()}-${String(t.getMonth() + 1).padStart(2, '0')}-${String(t.getDate()).padStart(2, '0')}`;
      scheduleRefetch(today);
      composerEl?.focus();
    } catch (e) {
      toast.error('failed to add jot: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      composerBusy = false;
    }
  }

  // ── lifecycle ─────────────────────────────────────────────────────
  // Debounce WS-driven refetches per-date — a flurry of writes (the
  // user typing into a daily) shouldn't trigger a refetch per
  // keystroke. 500ms feels live without thrashing.
  const pendingRefetch = new Map<string, ReturnType<typeof setTimeout>>();
  function scheduleRefetch(date: string) {
    const existing = pendingRefetch.get(date);
    if (existing) clearTimeout(existing);
    const t = setTimeout(() => {
      pendingRefetch.delete(date);
      refetchJot(date);
    }, 500);
    pendingRefetch.set(date, t);
  }

  onMount(() => {
    // Resolve the daily folder so jump-to-day + WS scoping work.
    api
      .getConfig()
      .then((c) => {
        dailyFolder = (c.daily_notes_folder ?? '').replace(/\/+$/, '');
      })
      .catch(() => {
        // No config endpoint or failure → assume vault root, which is
        // the default-config behavior anyway.
        dailyFolder = '';
      });

    loadMore();

    // Hook up the IntersectionObserver once the sentinel is in the DOM.
    const setupObserver = () => {
      if (!sentinel) return;
      observer = new IntersectionObserver(
        (entries) => {
          for (const e of entries) {
            if (e.isIntersecting) loadMore();
          }
        },
        { rootMargin: '400px' }
      );
      observer.observe(sentinel);
    };
    // microtask delay so the bind:this has resolved
    queueMicrotask(setupObserver);

    const offWs = onWsEvent((ev) => {
      if (ev.type !== 'note.changed' && ev.type !== 'note.removed') return;
      const m = jotMatches(ev.path);
      if (!m) return;
      scheduleRefetch(m.date);
    });

    return () => {
      observer?.disconnect();
      offWs();
      for (const t of pendingRefetch.values()) clearTimeout(t);
      pendingRefetch.clear();
    };
  });
</script>

<div class="h-full overflow-y-auto" id="jots-scroll">
  <div class="max-w-3xl mx-auto p-4 sm:p-6 lg:p-8">
    <header class="mb-4">
      <h1 class="text-2xl sm:text-3xl font-semibold text-text">Jots</h1>
      <p class="text-sm text-dim mt-1">Every daily note, newest first. Scroll to keep going.</p>
    </header>

    <!-- Toolbar (sticky) -->
    <div
      class="sticky top-0 z-20 -mx-4 sm:-mx-6 lg:-mx-8 px-4 sm:px-6 lg:px-8 py-2.5 mb-4 bg-base/95 backdrop-blur border-b border-surface1"
    >
      <div class="flex flex-wrap items-center gap-2">
        <label class="flex items-center gap-2 text-xs text-dim">
          <span>jump to</span>
          <input
            type="date"
            onchange={jumpToDate}
            class="bg-mantle border border-surface1 rounded px-2 py-1 text-xs text-text focus:outline-none focus:border-primary"
          />
        </label>
        <div class="flex-1 min-w-[12rem] flex items-center gap-1">
          <input
            type="text"
            bind:value={searchText}
            onkeydown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                runSearch();
              } else if (e.key === 'Escape') {
                clearSearch();
              }
            }}
            placeholder="search jots…"
            class="flex-1 bg-mantle border border-surface1 rounded px-2 py-1 text-xs text-text placeholder-dim focus:outline-none focus:border-primary"
          />
          {#if searchText}
            <button
              type="button"
              onclick={clearSearch}
              aria-label="clear search"
              class="text-xs text-dim hover:text-text px-1.5"
            >×</button>
          {/if}
        </div>
        <button
          type="button"
          onclick={openToday}
          class="text-xs px-2 py-1 rounded bg-surface0 text-subtext hover:bg-surface1"
        >Today</button>
      </div>

      {#if searchResults.length > 0}
        <div class="mt-2 bg-mantle border border-surface1 rounded p-2 max-h-64 overflow-y-auto">
          <div class="text-[10px] uppercase tracking-wider text-dim mb-1.5 px-1">
            {searchResults.length} match{searchResults.length === 1 ? '' : 'es'}
          </div>
          <ul class="space-y-0.5">
            {#each searchResults as n (n.path)}
              <li>
                <a
                  href="/notes/{encodeURIComponent(n.path)}"
                  class="block px-2 py-1 rounded text-sm text-text hover:bg-surface0"
                >
                  <span class="font-medium">{n.title}</span>
                  <span class="text-xs text-dim ml-2">{n.path}</span>
                </a>
              </li>
            {/each}
          </ul>
        </div>
      {:else if searchText && !searching}
        <div class="mt-2 text-xs text-dim italic px-1">no matches — press Enter to search</div>
      {/if}
    </div>

    <!-- Quick-jot composer — Amplenote-style fire-and-forget input
         that appends a timestamped line to today's daily. The user
         doesn't navigate; the new content lands in the feed below. -->
    <div class="mb-5 bg-surface0 border border-surface1 rounded-lg focus-within:border-primary transition-colors">
      <textarea
        bind:this={composerEl}
        bind:value={composerText}
        onkeydown={(e) => {
          // Enter (without shift) submits; Shift+Enter inserts a newline
          if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            submitJot();
          }
        }}
        placeholder="What's on your mind? (Enter to save, Shift+Enter for newline)"
        rows="2"
        disabled={composerBusy}
        class="w-full bg-transparent px-3 py-2 text-sm text-text placeholder-dim focus:outline-none resize-y disabled:opacity-50"
      ></textarea>
      <div class="flex items-center justify-between px-3 py-1.5 border-t border-surface1/50">
        <p class="text-[10px] text-dim">Appends under <code>## Jots</code> in today's daily</p>
        <button
          type="button"
          onclick={submitJot}
          disabled={composerBusy || !composerText.trim()}
          class="text-xs px-3 py-1 rounded bg-primary text-on-primary font-medium hover:opacity-90 disabled:opacity-50"
        >{composerBusy ? '…' : 'Add jot'}</button>
      </div>
    </div>

    {#if error}
      <div class="text-sm text-error mb-4 p-3 bg-error/10 border border-error/30 rounded">
        {error}
      </div>
    {/if}

    <!-- First-paint skeleton: 3 placeholder cards while the first page lands. -->
    {#if jots.length === 0 && loading}
      <div class="space-y-4">
        {#each [0, 1, 2] as _}
          <div class="bg-surface0 border border-surface1 rounded p-4">
            <Skeleton class="h-5 w-40 mb-3" />
            <Skeleton class="h-4 w-full mb-1.5" />
            <Skeleton class="h-4 w-5/6 mb-1.5" />
            <Skeleton class="h-4 w-3/4" />
          </div>
        {/each}
      </div>
    {:else if jots.length === 0 && done}
      <EmptyState
        icon="📓"
        title="No daily notes yet"
        description="Once you start writing dailies, they show up here — newest at the top, infinite scroll all the way back."
      >
        {#snippet action()}
          <button
            onclick={openToday}
            class="px-4 py-2 bg-primary text-on-primary rounded text-sm font-medium"
          >
            Open today's daily
          </button>
        {/snippet}
      </EmptyState>
    {:else}
      <ul class="space-y-5">
        {#each jots as jot (jot.path)}
          <li>
            <article>
              <header
                class="sticky top-[3.25rem] z-10 -mx-1 px-1 py-1.5 bg-base/95 backdrop-blur flex items-baseline gap-2 mb-2"
              >
                <h2 class="text-base sm:text-lg font-semibold text-text">
                  {relativeLabel(jot.date, today)}
                </h2>
                <span class="text-xs text-dim hidden sm:inline">{fullLabel(jot.date)}</span>
                <span class="text-xs text-dim sm:hidden">{jot.date}</span>
                {#if jot.openTasks > 0}
                  <span
                    class="text-[10px] px-1.5 py-0.5 rounded bg-warning/15 text-warning font-medium"
                  >
                    {jot.openTasks} open task{jot.openTasks === 1 ? '' : 's'}
                  </span>
                {/if}
                <a
                  href="/notes/{encodeURIComponent(jot.path)}"
                  class="ml-auto text-xs text-primary hover:underline"
                >
                  Open →
                </a>
              </header>
              <div class="bg-surface0 border border-surface1 rounded p-4">
                {#if jot.body.trim()}
                  <MarkdownRenderer body={jot.body} onWikilink={handleWikilink} />
                {:else}
                  <p class="text-sm text-dim italic">empty</p>
                {/if}
              </div>
            </article>
          </li>
        {/each}
      </ul>

      <!-- Sentinel: when this enters the viewport, load the next page. -->
      <div bind:this={sentinel} class="h-12 mt-6 flex items-center justify-center text-xs text-dim">
        {#if loading}
          loading more…
        {:else if done}
          {jots.length} jot{jots.length === 1 ? '' : 's'} · end of feed
        {/if}
      </div>
    {/if}
  </div>
</div>
