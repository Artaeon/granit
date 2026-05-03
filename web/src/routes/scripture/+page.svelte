<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import {
    api,
    type Scripture,
    type BibleBookSummary,
    type BiblePassage,
    type BibleVerse,
    type BibleSearchHit,
    type BibleBookmark
  } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import AgentRunPanel from '$lib/agents/AgentRunPanel.svelte';
  import type { AgentPreset } from '$lib/api';

  // Four modes:
  //   read   — verse-of-the-day in big type, "another one" button,
  //            reflect-on-this saves a Devotionals/ note
  //   memo   — cloze-deletion drill: hide every Nth significant word,
  //            you fill them in and the page tells you accuracy.
  //            Tracks per-verse stats in localStorage so weak verses
  //            surface preferentially (SM-2-style spaced repetition,
  //            simplified — accuracy alone, not interval-based)
  //   browse — paginated full list, search, click-to-copy
  //   bible  — full embedded WEB bible: random passage, book/chapter
  //            picker, full-text search. Distinct from `read` because
  //            the curated daily-rotation set stays small and stable
  //            for spaced-repetition; the bible tab is for free-form
  //            exploration.
  type Mode = 'read' | 'memo' | 'browse' | 'bible' | 'bookmarks';

  let mode = $state<Mode>('read');
  let today = $state<Scripture | null>(null);
  let current = $state<Scripture | null>(null); // verse currently being viewed/drilled
  let all = $state<Scripture[]>([]);
  let loading = $state(false);
  let q = $state('');

  // Memorization state — see drillVerse() for the algorithm.
  let drill = $state<{ verse: Scripture; words: string[]; hidden: Set<number>; guesses: Record<number, string> } | null>(null);
  let revealed = $state(false);

  // Per-verse accuracy in localStorage, keyed by source. Trial count +
  // success count → ratio. Lower-ratio verses are picked more often in
  // memo mode so weak spots get more practice.
  type Stats = Record<string, { tries: number; correct: number }>;
  const STATS_KEY = 'granit.scripture.stats';

  function loadStats(): Stats {
    try {
      const raw = localStorage.getItem(STATS_KEY);
      if (!raw) return {};
      return JSON.parse(raw);
    } catch {
      return {};
    }
  }
  function saveStats(s: Stats) {
    try { localStorage.setItem(STATS_KEY, JSON.stringify(s)); } catch {}
  }

  async function load() {
    loading = true;
    try {
      const [t, list] = await Promise.all([api.todayScripture(), api.listScriptures()]);
      today = t;
      current = t;
      all = list.scriptures;
    } catch (e) {
      toast.error('failed to load scriptures: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      loading = false;
    }
  }

  // Bookmarks — saved bible passages, .granit/bible-bookmarks.json.
  // Loaded lazily on first visit to the bookmarks tab; live-updates
  // via WS state.changed (TUI bookmark UI lands later, same file).
  let bookmarks = $state<BibleBookmark[]>([]);
  let bookmarksLoaded = $state(false);

  async function loadBookmarks() {
    try {
      const r = await api.listBibleBookmarks();
      bookmarks = r.bookmarks;
      bookmarksLoaded = true;
    } catch (e) {
      toast.error('failed to load bookmarks: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/bible-bookmarks.json') {
        if (bookmarksLoaded) loadBookmarks();
      }
    });
  });

  // Save the visible passage as a bookmark. Idempotent at the UX level
  // — a duplicate bookmark is allowed (user might want a second copy
  // with a different note); we don't dedupe here.
  async function bookmarkPassage(p: BiblePassage, note = '') {
    try {
      await api.createBibleBookmark({
        bookCode: p.bookCode,
        book: p.book,
        chapter: p.chapter,
        verseFrom: p.verses[0]?.n ?? 1,
        verseTo: p.verses[p.verses.length - 1]?.n ?? 1,
        text: p.verses.map((v) => v.text).join(' '),
        note: note || undefined
      });
      toast.success('bookmark saved');
      if (bookmarksLoaded) await loadBookmarks();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // Save a single verse from a chapter view. Snapshot + reference are
  // built from the verse number + the current chapter's book.
  async function bookmarkVerse(bookCode: string, book: string, chapter: number, v: BibleVerse) {
    try {
      await api.createBibleBookmark({
        bookCode,
        book,
        chapter,
        verseFrom: v.n,
        verseTo: v.n,
        text: v.text
      });
      toast.success('verse bookmarked');
      if (bookmarksLoaded) await loadBookmarks();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function deleteBookmark(b: BibleBookmark) {
    if (!confirm(`Remove bookmark "${b.reference}"?`)) return;
    try {
      await api.deleteBibleBookmark(b.id);
      toast.success('bookmark removed');
      await loadBookmarks();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // Note-edit happens inline; this commits the change.
  async function saveBookmarkNote(b: BibleBookmark, note: string) {
    try {
      await api.patchBibleBookmark(b.id, { note });
      toast.success('note saved');
      await loadBookmarks();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // Open a bookmark in the bible reader: load its chapter, scroll to
  // the first verse. The user can copy the saved snippet but the
  // chapter view shows the canonical translation alongside.
  async function openBookmark(b: BibleBookmark) {
    mode = 'bible';
    await ensureBibleIndex();
    await loadBibleChapter(b.bookCode, b.chapter);
    setTimeout(() => {
      const el = document.getElementById(`bible-v-${b.verseFrom}`);
      if (el) el.scrollIntoView({ behavior: 'smooth', block: 'center' });
    }, 100);
  }

  async function anotherOne() {
    try {
      current = await api.randomScripture();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // Devotional creator — saves a fresh Devotionals/ note pre-seeded
  // with the verse and routes the user to it for editing.
  async function reflectOnThis() {
    if (!current) return;
    try {
      const r = await api.createDevotional({ verse: current.text, source: current.source });
      toast.success('devotional created');
      goto(`/notes/${encodeURIComponent(r.path)}`);
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // AI reflection — opens the devotional preset's run panel with the
  // verse + citation pre-filled as the goal. The agent writes a
  // 200-300 word reflection into Devotionals/{date}-{slug}.md.
  let aiOpen = $state(false);
  let devotionalPreset = $state<AgentPreset | null>(null);
  let aiLoading = $state(false);
  let aiGoal = $state('');

  async function aiReflect() {
    if (!current) return;
    aiLoading = true;
    try {
      if (!devotionalPreset) {
        const r = await api.listAgentPresets();
        devotionalPreset = r.presets.find((p) => p.id === 'devotional') ?? null;
      }
      if (!devotionalPreset) {
        toast.error('devotional preset not found');
        return;
      }
      aiGoal = `Verse: "${current.text}"${current.source ? `\nSource: ${current.source}` : ''}`;
      aiOpen = true;
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      aiLoading = false;
    }
  }

  // Pick a verse for memorization, weighted toward verses with lower
  // accuracy. Returns a uniform random pick when nothing has been
  // drilled yet.
  function pickWeak(): Scripture | null {
    if (all.length === 0) return null;
    const stats = loadStats();
    const weights = all.map((v) => {
      const s = stats[v.source ?? v.text];
      if (!s || s.tries === 0) return 1.0; // never practiced — full weight
      const ratio = s.correct / s.tries;
      return Math.max(0.1, 1.0 - ratio); // floor so even mastered verses aren't impossible
    });
    const total = weights.reduce((a, b) => a + b, 0);
    let r = Math.random() * total;
    for (let i = 0; i < weights.length; i++) {
      r -= weights[i];
      if (r <= 0) return all[i];
    }
    return all[all.length - 1];
  }

  // Build a cloze drill: hide ~every 4th non-trivial word. Punctuation
  // stays put so the user has anchor points; small filler words like
  // "a", "to", "of" are skipped (too easy / not the point of memo).
  function startDrill() {
    const v = pickWeak();
    if (!v) return;
    const words = v.text.split(/(\s+)/); // keep whitespace tokens
    const significantIdx: number[] = [];
    const skip = new Set(['a', 'an', 'the', 'to', 'of', 'in', 'on', 'and', 'or', 'but', 'is', 'it']);
    for (let i = 0; i < words.length; i++) {
      const w = words[i].replace(/[^\p{L}']/gu, '').toLowerCase();
      if (w && !skip.has(w) && w.length > 2) significantIdx.push(i);
    }
    // Hide ~25% of significant words, minimum 2, maximum 8.
    const target = Math.min(8, Math.max(2, Math.round(significantIdx.length * 0.25)));
    const hidden = new Set<number>();
    while (hidden.size < target && hidden.size < significantIdx.length) {
      hidden.add(significantIdx[Math.floor(Math.random() * significantIdx.length)]);
    }
    drill = { verse: v, words, hidden, guesses: {} };
    revealed = false;
  }

  function checkDrill() {
    if (!drill) return;
    let correct = 0;
    for (const i of drill.hidden) {
      const want = drill.words[i].replace(/[^\p{L}']/gu, '').toLowerCase();
      const got = (drill.guesses[i] ?? '').replace(/[^\p{L}']/gu, '').toLowerCase();
      if (want === got) correct++;
    }
    revealed = true;
    const stats = loadStats();
    const key = drill.verse.source ?? drill.verse.text;
    const cur = stats[key] ?? { tries: 0, correct: 0 };
    cur.tries++;
    if (correct === drill.hidden.size) cur.correct++;
    stats[key] = cur;
    saveStats(stats);
    if (correct === drill.hidden.size) toast.success('perfect — all blanks correct!');
    else toast.info(`${correct} / ${drill.hidden.size} correct`);
  }

  let filteredAll = $derived.by(() => {
    const term = q.trim().toLowerCase();
    if (!term) return all;
    return all.filter((v) =>
      v.text.toLowerCase().includes(term) ||
      (v.source ?? '').toLowerCase().includes(term)
    );
  });

  // For overall stats display in the memo header.
  let totalTries = $derived.by(() => {
    if (typeof localStorage === 'undefined') return 0;
    const s = loadStats();
    return Object.values(s).reduce((sum, x) => sum + x.tries, 0);
  });
  let totalCorrect = $derived.by(() => {
    if (typeof localStorage === 'undefined') return 0;
    const s = loadStats();
    return Object.values(s).reduce((sum, x) => sum + x.correct, 0);
  });

  // ─── Bible mode ──────────────────────────────────────────────────────
  // Lazy-loaded so /scripture stays fast to first paint; we only fetch
  // the book index when the user actually clicks the Bible tab.
  let bibleBooks = $state<BibleBookSummary[]>([]);
  let bibleMeta = $state<{ name: string; abbreviation: string; license: string } | null>(null);
  let bibleLoading = $state(false);
  let biblePassage = $state<BiblePassage | null>(null);
  let bibleChapter = $state<{ book: string; bookCode: string; chapter: number; verses: BibleVerse[]; chapters: number } | null>(null);
  let bibleSearchQ = $state('');
  let bibleSearchHits = $state<BibleSearchHit[]>([]);
  let bibleSearchBusy = $state(false);
  let bibleLengthFilter = $state(4);
  let bibleTestamentFilter = $state<'' | 'OT' | 'NT'>('');
  // Picker state: which book is expanded to show its chapter grid.
  let pickerOpenBook = $state<string | null>(null);
  // Picker state: collapsible OT / NT sections.
  let pickerShowOT = $state(true);
  let pickerShowNT = $state(true);

  async function ensureBibleIndex() {
    if (bibleBooks.length > 0 || bibleLoading) return;
    bibleLoading = true;
    try {
      const r = await api.bibleBooks();
      bibleBooks = r.books;
      bibleMeta = r.meta;
    } catch (e) {
      toast.error('failed to load bible: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      bibleLoading = false;
    }
  }

  async function bibleRandom() {
    try {
      const opts: { length?: number; testament?: 'OT' | 'NT' } = { length: bibleLengthFilter };
      if (bibleTestamentFilter) opts.testament = bibleTestamentFilter;
      biblePassage = await api.bibleRandom(opts);
      bibleChapter = null; // mutually exclusive views — passage replaces chapter view
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // Reading history — last N chapters viewed, kept in localStorage.
  // Used to render a "Continue reading" pointer at the top of the
  // bible tab and a "Recent" strip for one-click jump-back. Per-device
  // (not synced) on purpose — reading position is much more about how
  // *this* device is positioned than something to round-trip through
  // the vault.
  type RecentChapter = { bookCode: string; book: string; chapter: number; at: number };
  const RECENT_KEY = 'granit.bible.recent';
  const RECENT_MAX = 8;

  // Reading streak — count of consecutive distinct days the user has
  // opened a chapter. Each loadBibleChapter call records today's
  // YYYY-MM-DD; the streak resets when a day is skipped. Stored in
  // localStorage as { lastDay, streak } so a single field captures
  // both the trail end and the current run.
  const STREAK_KEY = 'granit.bible.streak';
  type StreakState = { lastDay: string; streak: number };
  function loadStreak(): StreakState {
    if (typeof localStorage === 'undefined') return { lastDay: '', streak: 0 };
    try {
      const raw = localStorage.getItem(STREAK_KEY);
      if (!raw) return { lastDay: '', streak: 0 };
      const s = JSON.parse(raw) as StreakState;
      return { lastDay: s.lastDay ?? '', streak: s.streak ?? 0 };
    } catch {
      return { lastDay: '', streak: 0 };
    }
  }
  function saveStreak(s: StreakState): void {
    try { localStorage.setItem(STREAK_KEY, JSON.stringify(s)); } catch {}
  }
  let streak = $state<StreakState>(loadStreak());

  function todayKey(): string {
    const d = new Date();
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  }
  function bumpStreak() {
    const today = todayKey();
    if (streak.lastDay === today) return; // already counted today
    // Yesterday → continue streak; older → reset to 1; empty → start at 1.
    const yest = new Date();
    yest.setDate(yest.getDate() - 1);
    const yKey = `${yest.getFullYear()}-${String(yest.getMonth() + 1).padStart(2, '0')}-${String(yest.getDate()).padStart(2, '0')}`;
    const next: StreakState = {
      lastDay: today,
      streak: streak.lastDay === yKey ? streak.streak + 1 : 1
    };
    streak = next;
    saveStreak(next);
  }

  function loadRecent(): RecentChapter[] {
    if (typeof localStorage === 'undefined') return [];
    try {
      const raw = localStorage.getItem(RECENT_KEY);
      if (!raw) return [];
      const arr = JSON.parse(raw) as RecentChapter[];
      return Array.isArray(arr) ? arr : [];
    } catch {
      return [];
    }
  }
  function saveRecent(list: RecentChapter[]): void {
    try { localStorage.setItem(RECENT_KEY, JSON.stringify(list)); } catch {}
  }
  let recent = $state<RecentChapter[]>(typeof localStorage === 'undefined' ? [] : loadRecent());

  function pushRecent(book: string, bookCode: string, chapter: number) {
    const next: RecentChapter[] = [
      { bookCode, book, chapter, at: Date.now() },
      ...recent.filter((r) => !(r.bookCode === bookCode && r.chapter === chapter))
    ].slice(0, RECENT_MAX);
    recent = next;
    saveRecent(next);
  }

  async function loadBibleChapter(book: string, chapter: number) {
    try {
      bibleChapter = await api.bibleChapter(book, chapter);
      biblePassage = null;
      pushRecent(bibleChapter.book, bibleChapter.bookCode, bibleChapter.chapter);
      bumpStreak();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function bibleNextChapter(delta: number) {
    if (!bibleChapter) return;
    const next = bibleChapter.chapter + delta;
    if (next < 1 || next > bibleChapter.chapters) return;
    await loadBibleChapter(bibleChapter.bookCode, next);
  }

  // Pull the visible passage into the curated `current` slot so
  // the "use as today's verse" + devotional + AI reflection flows can
  // operate on a bible passage without reimplementing them.
  function biblePassageToScripture(p: BiblePassage): Scripture {
    return {
      text: p.verses.map((v) => v.text).join(' '),
      source: `${p.reference} (WEB)`
    };
  }

  function useAsTodayVerse(s: Scripture) {
    // Promote into the curated `current` slot — the existing read-tab
    // controls then operate on it (Reflect / AI Reflection / etc.).
    current = s;
    mode = 'read';
    toast.success('promoted to verse view');
  }

  async function reflectOnPassage(p: BiblePassage) {
    const s = biblePassageToScripture(p);
    try {
      const r = await api.createDevotional({ verse: s.text, source: s.source });
      toast.success('devotional created');
      goto(`/notes/${encodeURIComponent(r.path)}`);
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function aiReflectOnPassage(p: BiblePassage) {
    aiLoading = true;
    try {
      if (!devotionalPreset) {
        const r = await api.listAgentPresets();
        devotionalPreset = r.presets.find((pp) => pp.id === 'devotional') ?? null;
      }
      if (!devotionalPreset) {
        toast.error('devotional preset not found');
        return;
      }
      const verse = p.verses.map((v) => `${v.n}. ${v.text}`).join('\n');
      aiGoal = `Passage: ${p.reference} (WEB)\n\n${verse}`;
      aiOpen = true;
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      aiLoading = false;
    }
  }

  // Debounced search — keeps the UI snappy even though the server scan
  // is fast; no point firing a request on every keystroke.
  let searchTimer: ReturnType<typeof setTimeout> | null = null;
  function onSearchInput() {
    if (searchTimer) clearTimeout(searchTimer);
    const q = bibleSearchQ.trim();
    if (q.length < 2) {
      bibleSearchHits = [];
      return;
    }
    searchTimer = setTimeout(async () => {
      bibleSearchBusy = true;
      try {
        const r = await api.bibleSearch(q, 50);
        bibleSearchHits = r.hits;
      } catch (e) {
        toast.error('search failed: ' + (e instanceof Error ? e.message : String(e)));
      } finally {
        bibleSearchBusy = false;
      }
    }, 250);
  }

  function openHit(h: BibleSearchHit) {
    loadBibleChapter(h.bookCode, h.chapter);
    // Defer scroll so the DOM has rendered the chapter.
    setTimeout(() => {
      const el = document.getElementById(`bible-v-${h.verse}`);
      if (el) el.scrollIntoView({ behavior: 'smooth', block: 'center' });
    }, 100);
  }

  // Books grouped by testament, for the collapsible picker.
  let bibleBooksOT = $derived(bibleBooks.filter((b) => b.testament === 'OT'));
  let bibleBooksNT = $derived(bibleBooks.filter((b) => b.testament === 'NT'));
</script>

<div class="h-full overflow-y-auto">
  <div class="max-w-3xl mx-auto p-4 sm:p-6 lg:p-8">
    <div class="flex flex-wrap items-baseline gap-3 mb-6">
      <div class="flex-1 min-w-0">
        <PageHeader title="Scripture" subtitle="Verse of the day, memorization drill, full bible (WEB)" />
      </div>
      {#if streak.streak > 0}
        <span
          class="text-xs px-2 py-1 rounded bg-success/15 text-success font-medium flex-shrink-0"
          title="Days in a row you've opened a chapter (per device)"
        >🔥 {streak.streak}-day streak</span>
      {/if}
    </div>

    <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-sm mb-6 flex-wrap">
      <button
        class="px-4 py-2 {mode === 'read' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
        onclick={() => (mode = 'read')}
      >Read</button>
      <button
        class="px-4 py-2 {mode === 'memo' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
        onclick={() => { mode = 'memo'; if (!drill) startDrill(); }}
      >Memorize</button>
      <button
        class="px-4 py-2 {mode === 'browse' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
        onclick={() => (mode = 'browse')}
      >Browse <span class="text-[10px] opacity-70">{all.length}</span></button>
      <button
        class="px-4 py-2 {mode === 'bible' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
        onclick={() => { mode = 'bible'; ensureBibleIndex(); }}
      >Bible</button>
      <button
        class="px-4 py-2 {mode === 'bookmarks' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
        onclick={() => { mode = 'bookmarks'; if (!bookmarksLoaded) loadBookmarks(); }}
      >Bookmarks {#if bookmarks.length > 0}<span class="text-[10px] opacity-70">{bookmarks.length}</span>{/if}</button>
    </div>

    {#if loading && all.length === 0}
      <div class="text-sm text-dim">loading…</div>
    {:else if mode === 'read'}
      {#if current}
        <article class="bg-surface0 border border-surface1 rounded-lg p-6 sm:p-8 text-center">
          <blockquote class="text-xl sm:text-2xl text-text leading-relaxed font-serif italic">
            "{current.text}"
          </blockquote>
          {#if current.source}
            <cite class="text-sm text-subtext mt-4 block not-italic">— {current.source}</cite>
          {/if}
        </article>
        <div class="flex gap-2 justify-center mt-4 flex-wrap">
          <button
            onclick={anotherOne}
            class="px-4 py-2 text-sm bg-surface0 border border-surface1 rounded hover:border-primary"
          >Another verse</button>
          <button
            onclick={reflectOnThis}
            class="px-4 py-2 text-sm bg-surface0 border border-surface1 rounded hover:border-primary"
          >Reflect on this →</button>
          <button
            onclick={aiReflect}
            disabled={aiLoading}
            class="px-4 py-2 text-sm bg-primary text-on-primary rounded hover:opacity-90 disabled:opacity-50"
            title="AI writes a 200-300 word reflection into Devotionals/"
          >{aiLoading ? '…' : 'AI reflection ✨'}</button>
        </div>
        {#if today && current === today}
          <p class="text-[11px] text-dim text-center mt-4 italic">Verse of the day — same on every device, rotates at midnight.</p>
        {/if}
      {/if}
    {:else if mode === 'memo'}
      <div class="bg-surface0 border border-surface1 rounded-lg p-6 sm:p-8">
        {#if !drill}
          <div class="text-sm text-dim italic">Pick a verse to drill — click "Start drill" below.</div>
        {:else}
          <p class="text-xs text-dim mb-4">Fill in the blanks. The page picks weaker verses more often.</p>
          <div class="text-lg sm:text-xl text-text leading-relaxed font-serif">
            {#each drill.words as w, i}
              {#if drill.hidden.has(i)}
                {@const want = w.replace(/[^\p{L}']/gu, '').toLowerCase()}
                {@const got = (drill.guesses[i] ?? '').replace(/[^\p{L}']/gu, '').toLowerCase()}
                {#if revealed}
                  <span
                    class="inline-block px-1 mx-0.5 rounded text-base"
                    style="background: color-mix(in srgb, var(--color-{got === want ? 'success' : 'error'}) 18%, transparent); color: var(--color-{got === want ? 'success' : 'error'});"
                  >{w}</span>
                {:else}
                  <input
                    type="text"
                    bind:value={drill.guesses[i]}
                    placeholder="___"
                    class="inline-block w-24 px-1 mx-0.5 bg-mantle border-b border-primary text-text text-base focus:outline-none focus:border-primary"
                  />
                {/if}
              {:else}
                <span>{w}</span>
              {/if}
            {/each}
          </div>
          {#if drill.verse.source}
            <cite class="text-sm text-subtext mt-4 block not-italic">— {drill.verse.source}</cite>
          {/if}
        {/if}

        <div class="flex gap-2 mt-6 flex-wrap">
          {#if !revealed}
            <button
              onclick={checkDrill}
              disabled={!drill}
              class="px-4 py-2 text-sm bg-primary text-on-primary rounded disabled:opacity-50"
            >Check</button>
          {:else}
            <button
              onclick={startDrill}
              class="px-4 py-2 text-sm bg-primary text-on-primary rounded"
            >Next verse →</button>
          {/if}
          <button
            onclick={startDrill}
            class="px-4 py-2 text-sm bg-surface0 border border-surface1 rounded hover:border-primary"
          >Skip</button>
          <span class="flex-1"></span>
          {#if totalTries > 0}
            <span class="text-xs text-dim self-center">
              accuracy: <span class="text-text font-mono">{Math.round((totalCorrect / totalTries) * 100)}%</span>
              ({totalCorrect}/{totalTries})
            </span>
          {/if}
        </div>
      </div>
    {:else if mode === 'browse'}
      <input
        bind:value={q}
        placeholder="filter…"
        class="w-full px-3 py-2 mb-4 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
      />
      <ul class="divide-y divide-surface1 bg-surface0/40 border border-surface1 rounded-lg">
        {#each filteredAll as v}
          <li class="px-4 py-3">
            <p class="text-sm text-text font-serif italic">"{v.text}"</p>
            {#if v.source}
              <p class="text-xs text-subtext mt-1">— {v.source}</p>
            {/if}
          </li>
        {/each}
      </ul>
      <p class="text-[11px] text-dim italic mt-3">
        Edit <code>.granit/scriptures.md</code> to add your own — same file the granit TUI reads.
      </p>
    {:else if mode === 'bible'}
      {#if bibleLoading && bibleBooks.length === 0}
        <div class="text-sm text-dim">loading bible…</div>
      {:else}
        <!-- Continue reading — surfaces the last chapter viewed so the
             user can jump back into a sequential reading flow without
             scrolling the picker. Hidden until something's been read. -->
        {#if recent.length > 0}
          <div class="bg-surface0 border border-surface1 rounded-lg p-4 mb-4">
            <div class="flex items-baseline gap-3 mb-2">
              <h3 class="text-xs uppercase tracking-wider text-dim font-medium">Continue reading</h3>
              <button
                type="button"
                onclick={() => loadBibleChapter(recent[0].bookCode, recent[0].chapter)}
                class="text-sm text-primary hover:underline font-medium"
              >{recent[0].book} {recent[0].chapter} →</button>
            </div>
            {#if recent.length > 1}
              <div class="flex flex-wrap gap-1.5">
                <span class="text-[11px] text-dim self-center">recent:</span>
                {#each recent.slice(1) as r (r.bookCode + r.chapter)}
                  <button
                    type="button"
                    onclick={() => loadBibleChapter(r.bookCode, r.chapter)}
                    class="text-[11px] px-2 py-0.5 rounded bg-mantle border border-surface1 text-subtext hover:border-primary hover:text-text"
                  >{r.book} {r.chapter}</button>
                {/each}
              </div>
            {/if}
          </div>
        {/if}

        <!-- Random passage controls — primary action up top -->
        <div class="bg-surface0 border border-surface1 rounded-lg p-4 mb-4">
          <div class="flex flex-wrap gap-2 items-center">
            <button
              onclick={bibleRandom}
              class="px-4 py-2 text-sm bg-primary text-on-primary rounded hover:opacity-90"
            >Random passage</button>
            <label class="text-xs text-subtext">
              length
              <select
                bind:value={bibleLengthFilter}
                class="ml-1 bg-mantle border border-surface1 rounded px-2 py-1 text-text"
              >
                {#each [1, 2, 3, 4, 5, 6, 8, 10] as n}
                  <option value={n}>{n}</option>
                {/each}
              </select>
            </label>
            <label class="text-xs text-subtext">
              from
              <select
                bind:value={bibleTestamentFilter}
                class="ml-1 bg-mantle border border-surface1 rounded px-2 py-1 text-text"
              >
                <option value="">whole bible</option>
                <option value="OT">Old Testament</option>
                <option value="NT">New Testament</option>
              </select>
            </label>
            {#if bibleMeta}
              <span class="ml-auto text-[11px] text-dim italic">
                {bibleMeta.name} ({bibleMeta.abbreviation}) · {bibleMeta.license}
              </span>
            {/if}
          </div>
        </div>

        <!-- Search box -->
        <div class="bg-surface0 border border-surface1 rounded-lg p-4 mb-4">
          <div class="flex gap-2 items-center">
            <input
              bind:value={bibleSearchQ}
              oninput={onSearchInput}
              placeholder="search the bible (e.g. 'love your enemies')…"
              class="flex-1 px-3 py-2 bg-mantle border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
            />
            {#if bibleSearchBusy}
              <span class="text-xs text-dim">searching…</span>
            {/if}
          </div>
          {#if bibleSearchHits.length > 0}
            <ul class="mt-3 max-h-64 overflow-y-auto divide-y divide-surface1 border border-surface1 rounded">
              {#each bibleSearchHits as h}
                <li>
                  <button
                    type="button"
                    onclick={() => openHit(h)}
                    class="w-full text-left px-3 py-2 hover:bg-surface1"
                  >
                    <p class="text-xs text-primary font-mono">{h.reference}</p>
                    <p class="text-sm text-text font-serif italic mt-0.5">"{h.text}"</p>
                  </button>
                </li>
              {/each}
            </ul>
            <p class="text-[11px] text-dim italic mt-2">
              showing {bibleSearchHits.length} {bibleSearchHits.length === 50 ? '(capped)' : ''}
            </p>
          {:else if bibleSearchQ.trim().length >= 2 && !bibleSearchBusy}
            <p class="text-[11px] text-dim italic mt-2">no matches</p>
          {/if}
        </div>

        <!-- Active passage / chapter view -->
        {#if biblePassage}
          <article class="bg-surface0 border border-surface1 rounded-lg p-6 sm:p-8">
            <p class="text-xs text-primary font-mono mb-3">{biblePassage.reference} (WEB)</p>
            <div class="text-lg sm:text-xl text-text leading-relaxed font-serif">
              {#each biblePassage.verses as v}
                <span class="text-xs align-super text-dim mr-1 font-sans not-italic">{v.n}</span><span>{v.text}</span>{' '}
              {/each}
            </div>
            <div class="flex gap-2 mt-6 flex-wrap">
              <button
                onclick={bibleRandom}
                class="px-3 py-1.5 text-sm bg-surface0 border border-surface1 rounded hover:border-primary"
              >Another passage</button>
              <button
                onclick={() => loadBibleChapter(biblePassage!.bookCode, biblePassage!.chapter)}
                class="px-3 py-1.5 text-sm bg-surface0 border border-surface1 rounded hover:border-primary"
              >Read full chapter →</button>
              <button
                onclick={() => useAsTodayVerse(biblePassageToScripture(biblePassage!))}
                class="px-3 py-1.5 text-sm bg-surface0 border border-surface1 rounded hover:border-primary"
              >Use as today's verse</button>
              <button
                onclick={() => bookmarkPassage(biblePassage!)}
                class="px-3 py-1.5 text-sm bg-surface0 border border-surface1 rounded hover:border-primary"
                title="Save this passage to your bookmarks"
              >★ Bookmark</button>
              <button
                onclick={() => reflectOnPassage(biblePassage!)}
                class="px-3 py-1.5 text-sm bg-surface0 border border-surface1 rounded hover:border-primary"
              >Reflect →</button>
              <button
                onclick={() => aiReflectOnPassage(biblePassage!)}
                disabled={aiLoading}
                class="px-3 py-1.5 text-sm bg-primary text-on-primary rounded hover:opacity-90 disabled:opacity-50"
              >{aiLoading ? '…' : 'AI reflection ✨'}</button>
            </div>
          </article>
        {:else if bibleChapter}
          <article class="bg-surface0 border border-surface1 rounded-lg p-6 sm:p-8">
            <div class="flex items-center justify-between mb-4">
              <h2 class="text-lg font-semibold text-text">
                {bibleChapter.book} {bibleChapter.chapter}
              </h2>
              <div class="flex gap-1">
                <button
                  disabled={bibleChapter.chapter <= 1}
                  onclick={() => bibleNextChapter(-1)}
                  class="px-2 py-1 text-sm bg-mantle border border-surface1 rounded hover:border-primary disabled:opacity-30"
                >‹ prev</button>
                <button
                  disabled={bibleChapter.chapter >= bibleChapter.chapters}
                  onclick={() => bibleNextChapter(1)}
                  class="px-2 py-1 text-sm bg-mantle border border-surface1 rounded hover:border-primary disabled:opacity-30"
                >next ›</button>
              </div>
            </div>
            <div class="text-base sm:text-lg text-text leading-relaxed font-serif space-y-1">
              {#each bibleChapter.verses as v}
                <p id={`bible-v-${v.n}`} class="group flex items-baseline gap-2">
                  <span class="text-xs align-super text-dim font-sans not-italic">{v.n}</span>
                  <span class="flex-1">{v.text}</span>
                  <button
                    type="button"
                    onclick={() => bookmarkVerse(bibleChapter!.bookCode, bibleChapter!.book, bibleChapter!.chapter, v)}
                    class="text-dim hover:text-primary opacity-0 group-hover:opacity-100 transition-opacity text-xs font-sans not-italic"
                    title="Bookmark this verse"
                    aria-label="Bookmark verse {v.n}"
                  >★</button>
                </p>
              {/each}
            </div>
            <p class="text-[11px] text-dim italic mt-4">
              {bibleChapter.book} · chapter {bibleChapter.chapter} of {bibleChapter.chapters} · WEB (Public Domain)
            </p>
          </article>
        {/if}

        <!-- Book / chapter picker -->
        <div class="mt-4 bg-surface0 border border-surface1 rounded-lg overflow-hidden">
          <button
            class="w-full text-left px-4 py-3 flex items-center justify-between hover:bg-surface1"
            onclick={() => (pickerShowOT = !pickerShowOT)}
          >
            <span class="text-sm font-semibold text-text">Old Testament</span>
            <span class="text-xs text-dim">{bibleBooksOT.length} books · {pickerShowOT ? '▾' : '▸'}</span>
          </button>
          {#if pickerShowOT}
            <ul class="divide-y divide-surface1 border-t border-surface1">
              {#each bibleBooksOT as bk}
                <li>
                  <button
                    class="w-full text-left px-4 py-2 flex items-center justify-between hover:bg-surface1"
                    onclick={() => (pickerOpenBook = pickerOpenBook === bk.code ? null : bk.code)}
                  >
                    <span class="text-sm text-text">{bk.name}</span>
                    <span class="text-[11px] text-dim">{bk.chapters} ch · {pickerOpenBook === bk.code ? '▾' : '▸'}</span>
                  </button>
                  {#if pickerOpenBook === bk.code}
                    <div class="px-3 pb-3 flex flex-wrap gap-1">
                      {#each Array.from({ length: bk.chapters }, (_, i) => i + 1) as ch}
                        <button
                          onclick={() => loadBibleChapter(bk.code, ch)}
                          class="px-2 py-1 text-xs bg-mantle border border-surface1 rounded hover:border-primary text-text font-mono"
                        >{ch}</button>
                      {/each}
                    </div>
                  {/if}
                </li>
              {/each}
            </ul>
          {/if}
        </div>

        <div class="mt-2 bg-surface0 border border-surface1 rounded-lg overflow-hidden">
          <button
            class="w-full text-left px-4 py-3 flex items-center justify-between hover:bg-surface1"
            onclick={() => (pickerShowNT = !pickerShowNT)}
          >
            <span class="text-sm font-semibold text-text">New Testament</span>
            <span class="text-xs text-dim">{bibleBooksNT.length} books · {pickerShowNT ? '▾' : '▸'}</span>
          </button>
          {#if pickerShowNT}
            <ul class="divide-y divide-surface1 border-t border-surface1">
              {#each bibleBooksNT as bk}
                <li>
                  <button
                    class="w-full text-left px-4 py-2 flex items-center justify-between hover:bg-surface1"
                    onclick={() => (pickerOpenBook = pickerOpenBook === bk.code ? null : bk.code)}
                  >
                    <span class="text-sm text-text">{bk.name}</span>
                    <span class="text-[11px] text-dim">{bk.chapters} ch · {pickerOpenBook === bk.code ? '▾' : '▸'}</span>
                  </button>
                  {#if pickerOpenBook === bk.code}
                    <div class="px-3 pb-3 flex flex-wrap gap-1">
                      {#each Array.from({ length: bk.chapters }, (_, i) => i + 1) as ch}
                        <button
                          onclick={() => loadBibleChapter(bk.code, ch)}
                          class="px-2 py-1 text-xs bg-mantle border border-surface1 rounded hover:border-primary text-text font-mono"
                        >{ch}</button>
                      {/each}
                    </div>
                  {/if}
                </li>
              {/each}
            </ul>
          {/if}
        </div>
      {/if}
    {:else if mode === 'bookmarks'}
      {#if !bookmarksLoaded}
        <div class="text-sm text-dim">loading bookmarks…</div>
      {:else if bookmarks.length === 0}
        <div class="bg-surface0 border border-surface1 rounded-lg p-6 text-center">
          <p class="text-sm text-dim">
            No bookmarks yet. Open a passage in the Bible tab and click <span class="text-primary">★ Bookmark</span> to save it.
          </p>
        </div>
      {:else}
        <ul class="space-y-3">
          {#each bookmarks as b (b.id)}
            <li class="bg-surface0 border border-surface1 rounded-lg p-4">
              <div class="flex items-baseline gap-2 mb-2">
                <button
                  type="button"
                  onclick={() => openBookmark(b)}
                  class="text-sm text-primary font-mono hover:underline"
                >{b.reference}</button>
                <span class="flex-1"></span>
                <button
                  type="button"
                  onclick={() => deleteBookmark(b)}
                  class="text-xs text-dim hover:text-error"
                  aria-label="Remove bookmark"
                >remove</button>
              </div>
              <p class="text-sm text-text font-serif italic leading-relaxed">"{b.text}"</p>
              <textarea
                value={b.note ?? ''}
                placeholder="Add a personal note…"
                onblur={(e) => {
                  const v = (e.currentTarget as HTMLTextAreaElement).value;
                  if (v !== (b.note ?? '')) saveBookmarkNote(b, v);
                }}
                class="w-full mt-3 px-2 py-1.5 bg-mantle border border-surface1 rounded text-xs text-text placeholder-dim focus:outline-none focus:border-primary resize-y"
                rows="2"
              ></textarea>
            </li>
          {/each}
        </ul>
        <p class="text-[11px] text-dim italic mt-3">
          Synced via <code>.granit/bible-bookmarks.json</code> — same file the granit TUI reads.
        </p>
      {/if}
    {/if}
  </div>
</div>

<!-- AgentRunPanel doesn't accept a pre-filled goal directly, but the
     panel's textarea is bound to a local state — we skip the goal
     input by writing aiGoal into the panel via the same mechanism the
     daily-note button uses. The agent reads the verse from the goal
     prompt in either case (devotional preset asks for "Verse: X / Source: Y"). -->
<AgentRunPanel bind:open={aiOpen} preset={devotionalPreset} initialGoal={aiGoal} />
