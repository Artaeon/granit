<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { auth } from '$lib/stores/auth';
  import { api, fmtDateISO, type Note } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { rafThrottle } from '$lib/util/streamThrottle';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import { toast } from '$lib/components/toast';
  import JotsPageHeader from '$lib/jots/JotsPageHeader.svelte';
  import JotsToolbar from '$lib/jots/JotsToolbar.svelte';
  import JotsAIPanel from '$lib/jots/JotsAIPanel.svelte';
  import JotsQuickFilters from '$lib/jots/JotsQuickFilters.svelte';
  import JotsComposer from '$lib/jots/JotsComposer.svelte';
  import JotItem from '$lib/jots/JotItem.svelte';
  import JotsShortcutsOverlay from '$lib/jots/JotsShortcutsOverlay.svelte';
  import { createJotsFeedData } from '$lib/jots/jotsFeedData.svelte';
  import { createJotsFilters } from '$lib/jots/jotsFilters.svelte';
  import { createJotsAI } from '$lib/jots/jotsAI.svelte';

  // Amplenote-style infinite-scroll feed of every daily note. The page
  // talks to /api/v1/jots which paginates server-side — fetching N
  // dailies one-by-one would round-trip N times per page; the dedicated
  // endpoint keeps it to one round-trip per page no matter how many
  // years of dailies the user has accumulated.

  const feed = createJotsFeedData({
    isAuthed: () => !!$auth
  });
  // Bind read-side aliases via $derived so the rest of the page reads
  // these names unchanged after the controller hand-off.
  let jots = $derived(feed.jots);
  let loading = $derived(feed.loading);
  let done = $derived(feed.done);
  let error = $derived(feed.error);
  let dailyFolder = $derived(feed.dailyFolder);
  let dayActivityCache = $derived(feed.dayActivityCache);
  let dayActivityLoading = $derived(feed.dayActivityLoading);

  // Inline search state
  let searchText = $state('');
  let searchResults = $state<Note[]>([]);
  let searching = $state(false);
  let searchEl = $state<HTMLInputElement | undefined>();

  // Keyboard navigation
  let currentJotIdx = $state(-1);
  let showShortcuts = $state(false);

  // Hashtag + quick-filter state lives in jotsFilters; the page binds
  // through to allTags / visibleJots / hasAnyFilter so the chip rail
  // and filtered list track changes reactively.
  const filtersCtl = createJotsFilters({
    getJots: () => feed.jots,
    getToday: () => today
  });
  let activeTags = $derived(filtersCtl.activeTags);
  let filterOpenTasks = $derived(filtersCtl.filterOpenTasks);
  let filterTimeframe = $derived(filtersCtl.filterTimeframe);
  let hasAnyFilter = $derived(filtersCtl.hasAnyFilter);
  let allTags = $derived(filtersCtl.allTags);
  let visibleJots = $derived(filtersCtl.visibleJots);
  const toggleTag = (t: string) => filtersCtl.toggleTag(t);
  const clearAllFilters = () => filtersCtl.clearAllFilters();

  // Sentinel + observer for infinite scroll.
  let sentinel: HTMLDivElement | undefined = $state();
  let observer: IntersectionObserver | null = null;

  // ─── AI: multi-mode panel ────────────────────────────────────────
  // One panel below the toolbar that switches between three modes
  // (themes / ask / digest). The controller guarantees only one
  // stream runs at a time — switching mode dismisses the previous
  // result and aborts the in-flight stream.
  const aiCtl = createJotsAI({
    getJots: () => feed.jots,
    getToday: () => today,
    getDailyFolder: () => feed.dailyFolder,
    applyThemeSearch: (q) => { searchText = q; runSearch(); },
    toastInfo: (m) => toast.info(m),
    toastSuccess: (m) => toast.success(m),
    toastError: (m) => toast.error(m),
    navigate: (p) => goto(p)
  });

  // Midnight today, recomputed reactively via $derived.by — used as the
  // anchor for relative-date labels ("Today" / "Yesterday" / etc).
  let today = $derived.by(() => {
    const d = new Date();
    return new Date(d.getFullYear(), d.getMonth(), d.getDate());
  });

  // ── header stats: streak + loaded counters ────────────────────────
  // All derived from the loaded `jots` array — no extra round-trips.
  // The streak window is bounded by what's currently loaded; if the
  // user scrolls past the streak edge it extends naturally as more
  // pages arrive.

  // Current daily-note streak: consecutive calendar days ending today
  // (or yesterday — Amplenote-style grace so you don't lose a streak
  // before today's daily is written) that have a loaded jot.
  let streakDays = $derived.by(() => {
    if (jots.length === 0) return 0;
    const have = new Set(jots.map((j) => j.date));
    // Walk back from today. Allow today to be missing as long as
    // yesterday is present, so the badge keeps the previous count
    // through the morning before today's note exists.
    const anchor = new Date(today);
    const todayISO = fmtDateISO(anchor);
    let cur = new Date(anchor);
    if (!have.has(todayISO)) {
      cur.setDate(cur.getDate() - 1);
      if (!have.has(fmtDateISO(cur))) return 0;
    }
    let count = 0;
    while (have.has(fmtDateISO(cur))) {
      count += 1;
      cur.setDate(cur.getDate() - 1);
    }
    return count;
  });

  // Total word count across all loaded jot bodies. Whitespace split is
  // good enough — this is a glanceable metric, not an editor stat.
  let loadedWords = $derived.by(() => {
    let n = 0;
    for (const j of jots) {
      const body = j.body ?? '';
      if (!body) continue;
      const matches = body.match(/\S+/g);
      if (matches) n += matches.length;
    }
    return n;
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

  // ── composer AI-expand ──────────────────────────────────────────
  // Toggle next to the Add button. When ON, hitting Enter doesn't
  // save directly — it routes through the AI to expand a terse note
  // into a fuller entry, with a streaming preview + Keep/Discard
  // before commitment. Persisted to localStorage so the user's
  // preference survives reloads.
  const EXPAND_KEY = 'granit.jots.composerExpand';
  let composerExpand = $state<boolean>(
    typeof window !== 'undefined' && window.localStorage.getItem(EXPAND_KEY) === '1'
  );
  let expanding = $state(false);
  let expandedText = $state('');
  let expandAbort: AbortController | null = null;
  $effect(() => {
    if (typeof window === 'undefined') return;
    try { window.localStorage.setItem(EXPAND_KEY, composerExpand ? '1' : '0'); } catch {}
  });

  async function runExpand() {
    const raw = composerText.trim();
    if (!raw || expanding) return;
    expandAbort?.abort();
    expandAbort = new AbortController();
    expanding = true;
    expandedText = '';
    const system =
      'You expand a user\'s terse journal note into a richer entry suitable for a daily ' +
      'log. Preserve every fact and feeling the user wrote — don\'t invent details or ' +
      'embellish. Add gentle scaffolding: link related ideas the user mentioned, expand ' +
      'shorthand, write in the user\'s voice. Return the expanded entry as markdown. ' +
      'Aim for 2-4 short paragraphs or a bullet list, depending on what fits. No preamble.';
    const user = 'Terse note:\n```\n' + raw + '\n```';
    try {
      const t = rafThrottle((full) => { expandedText = full; });
      await api.chatStream(
        [{ role: 'system', content: system }, { role: 'user', content: user }],
        undefined,
        {
          onChunk: t.onChunk,
          onDone: () => { t.flush(); },
          onError: (err) => { t.flush(); toast.error('expand failed: ' + err.message); expandedText = ''; }
        },
        expandAbort.signal
      );
    } finally {
      expanding = false;
      expandAbort = null;
    }
  }

  function discardExpand() {
    expandAbort?.abort();
    expandAbort = null;
    expanding = false;
    expandedText = '';
    composerEl?.focus();
  }

  async function keepExpand() {
    if (!expandedText.trim()) return;
    // Replace the raw composer text with the expanded version and
    // commit through the normal submit path. Saves us from duplicating
    // the appendUnderJotsSection / putNote / WS-refetch logic.
    composerText = expandedText.trim();
    expandedText = '';
    await submitJot({ skipExpand: true });
  }

  async function submitJot(opts: { skipExpand?: boolean } = {}) {
    const text = composerText.trim();
    if (!text || composerBusy) return;
    // If expand is on and we haven't yet expanded this draft, kick off
    // the AI and STOP — the user gets a preview to review before any
    // save hits the daily note.
    if (composerExpand && !opts.skipExpand) {
      runExpand();
      return;
    }
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
      feed.scheduleRefetch(today);
      composerEl?.focus();
    } catch (e) {
      toast.error('failed to add jot: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      composerBusy = false;
    }
  }

  // ── keyboard shortcuts ────────────────────────────────────────────
  // Amplenote-style single-key navigation. Active only when no input
  // has focus (otherwise typing "j" into the composer would scroll
  // instead of insert). Esc remains active inside inputs as a way out.
  function isTypingTarget(t: EventTarget | null): boolean {
    if (!t) return false;
    const el = t as HTMLElement;
    const tag = el.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true;
    return el.isContentEditable;
  }

  function scrollToJot(idx: number) {
    if (typeof document === 'undefined') return;
    const cards = document.querySelectorAll<HTMLElement>('[data-jot-date]');
    if (!cards.length) return;
    const clamped = Math.max(0, Math.min(idx, cards.length - 1));
    currentJotIdx = clamped;
    // block:start lands the header just under the sticky toolbar; the
    // browser's smooth scroll handles the rest. Card's data-jot-date
    // attribute is set in the template above so this lookup stays
    // independent of class names.
    cards[clamped].scrollIntoView({ behavior: 'smooth', block: 'start' });
  }

  function onShortcutKey(e: KeyboardEvent) {
    // Esc always honored, even inside inputs — it's the universal "back out".
    if (e.key === 'Escape') {
      if (showShortcuts) {
        showShortcuts = false;
        e.preventDefault();
        return;
      }
      if (isTypingTarget(e.target)) {
        (e.target as HTMLElement).blur();
        return;
      }
      if (hasAnyFilter) {
        clearAllFilters();
        e.preventDefault();
      } else if (searchText) {
        clearSearch();
        e.preventDefault();
      }
      return;
    }
    if (isTypingTarget(e.target)) return;
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    switch (e.key) {
      case '?':
        e.preventDefault();
        showShortcuts = !showShortcuts;
        return;
      case '/':
        e.preventDefault();
        searchEl?.focus();
        return;
      case 'c':
        e.preventDefault();
        composerEl?.focus();
        return;
      case 'j':
        e.preventDefault();
        scrollToJot(currentJotIdx + 1);
        return;
      case 'k':
        e.preventDefault();
        scrollToJot(Math.max(0, currentJotIdx - 1));
        return;
      case 'g':
        e.preventDefault();
        currentJotIdx = -1;
        document.getElementById('jots-scroll')?.scrollTo({ top: 0, behavior: 'smooth' });
        return;
      case 'G':
        e.preventDefault();
        // End-of-feed: load another page first so the user sees motion
        // instead of an abrupt stop, then scroll to the bottom of
        // what's currently rendered.
        feed.loadMore();
        document.getElementById('jots-scroll')?.scrollTo({
          top: document.getElementById('jots-scroll')?.scrollHeight ?? 0,
          behavior: 'smooth'
        });
        return;
    }
  }

  // ── lifecycle ─────────────────────────────────────────────────────

  onMount(() => {
    // Resolve the daily folder so jump-to-day + WS scoping work.
    feed.loadConfig();
    feed.loadMore();

    // Hook up the IntersectionObserver once the sentinel is in the DOM.
    const setupObserver = () => {
      if (!sentinel) return;
      observer = new IntersectionObserver(
        (entries) => {
          for (const e of entries) {
            if (e.isIntersecting) feed.loadMore();
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
      const m = feed.jotMatches(ev.path);
      if (!m) return;
      feed.scheduleRefetch(m.date);
    });

    window.addEventListener('keydown', onShortcutKey);

    return () => {
      observer?.disconnect();
      offWs();
      window.removeEventListener('keydown', onShortcutKey);
      feed.dispose();
    };
  });
</script>

<div class="h-full overflow-y-auto" id="jots-scroll">
  <div class="max-w-3xl mx-auto px-3 sm:px-5 lg:px-6 pt-2 pb-6">
    <JotsPageHeader
      streakDays={streakDays}
      jotsCount={jots.length}
      tagsCount={allTags.length}
      loadedWords={loadedWords}
      onToggleHelp={() => (showShortcuts = !showShortcuts)}
    />

    <JotsToolbar
      bind:searchEl
      bind:searchText
      searching={searching}
      searchResults={searchResults}
      aiMode={aiCtl.mode}
      aiBusy={aiCtl.busy}
      jotsCount={jots.length}
      onJumpToDate={jumpToDate}
      onSearchEnter={runSearch}
      onClearSearch={clearSearch}
      onDetectThemes={aiCtl.detectThemes}
      onStartAsk={aiCtl.startAsk}
      onBuildDigest={aiCtl.buildDigest}
      onOpenToday={openToday}
    >
      {#snippet aiPanel()}
        {#if aiCtl.mode !== 'none'}
          <JotsAIPanel
            mode={aiCtl.mode}
            busy={aiCtl.busy}
            error={aiCtl.error}
            themes={aiCtl.themes}
            bind:askQuestion={aiCtl.askQuestion}
            askAnswer={aiCtl.askAnswer}
            bind:askInputEl={aiCtl.askInputEl}
            digestAnswer={aiCtl.digestAnswer}
            onStop={aiCtl.cancel}
            onRegenerateThemes={aiCtl.detectThemes}
            onApplyTheme={aiCtl.applyTheme}
            onSubmitAsk={aiCtl.submitAsk}
            onCopy={aiCtl.copyToClipboard}
            onRegenerateDigest={aiCtl.buildDigest}
            onSaveDigestAsNote={aiCtl.saveDigestAsNote}
            onDismiss={aiCtl.dismiss}
            onWikilink={handleWikilink}
          />
        {/if}
      {/snippet}
      {#snippet quickFilters()}
        <JotsQuickFilters
          activeTags={activeTags}
          allTags={allTags}
          filterOpenTasks={filterOpenTasks}
          filterTimeframe={filterTimeframe}
          hasAnyFilter={hasAnyFilter}
          visibleCount={visibleJots.length}
          totalCount={jots.length}
          onToggleOpenTasks={() => (filtersCtl.filterOpenTasks = !filtersCtl.filterOpenTasks)}
          onSetTimeframe={(tf) => (filtersCtl.filterTimeframe = tf)}
          onToggleTag={toggleTag}
          onClearAll={clearAllFilters}
        />
      {/snippet}
    </JotsToolbar>

    <JotsComposer
      bind:text={composerText}
      bind:composerEl
      busy={composerBusy}
      expand={composerExpand}
      expanding={expanding}
      expandedText={expandedText}
      onSubmit={() => submitJot()}
      onToggleExpand={() => (composerExpand = !composerExpand)}
      onDiscardExpand={discardExpand}
      onKeepExpand={keepExpand}
      onRunExpand={runExpand}
      onWikilink={handleWikilink}
    />

    {#if error}
      <div class="text-sm text-error mb-4 p-3 bg-surface0 border border-error rounded">
        {error}
      </div>
    {/if}

    <!-- First-paint skeleton: 3 placeholder cards while the first page lands. -->
    {#if jots.length === 0 && loading}
      <div class="space-y-3">
        {#each [0, 1, 2] as _}
          <div class="bg-surface0 border border-surface1 rounded p-2.5">
            <Skeleton class="h-4 w-36 mb-2" />
            <Skeleton class="h-4 w-full mb-1" />
            <Skeleton class="h-4 w-5/6 mb-1" />
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
      {#if hasAnyFilter && visibleJots.length === 0}
        <p class="text-xs text-dim italic mb-3">
          No jots match the active filter{activeTags.length + (filterOpenTasks ? 1 : 0) + (filterTimeframe !== 'all' ? 1 : 0) === 1 ? '' : 's'}.
          {#if activeTags.length > 0}
            Tags: {#each activeTags as t, i}<span class="text-text">#{t}</span>{i < activeTags.length - 1 ? ', ' : ''}{/each}.
          {/if}
          Keep scrolling to load older dailies, or
          <button type="button" onclick={clearAllFilters} class="underline hover:text-text">clear filters</button>.
        </p>
      {/if}
      <ul class="space-y-3">
        {#each visibleJots as jot (jot.path)}
          <li data-jot-date={jot.date}>
            <JotItem
              jot={jot}
              today={today}
              activity={dayActivityCache[jot.date]}
              activityLoading={!!dayActivityLoading[jot.date]}
              onWikilink={handleWikilink}
              onExpandActivity={feed.loadDayActivity}
            />
          </li>
        {/each}
      </ul>

      <!-- Sentinel: when this enters the viewport, load the next page. -->
      <div bind:this={sentinel} class="h-12 mt-4 flex items-center justify-center text-xs text-dim">
        {#if loading}
          loading more…
        {:else if done}
          {jots.length} jot{jots.length === 1 ? '' : 's'} · end of feed
        {/if}
      </div>
    {/if}
  </div>
</div>

{#if showShortcuts}
  <JotsShortcutsOverlay onClose={() => (showShortcuts = false)} />
{/if}
