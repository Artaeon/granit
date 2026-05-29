<!--
  ScriptureBibleMode — the full embedded WEB bible reader. Used to
  live inline in /scripture as the {:else if mode === 'bible'} branch.

  Owns:
    - lazy book index load
    - random-passage controls
    - server-side search box (debounced)
    - chapter / passage view + bookmark / reflect actions
    - translation-compare + Strong's word-study toggles
    - OT / NT book picker with chapter grid
    - reading-recent-chapters trail (localStorage)
    - reading-streak counter (localStorage)

  Parent owns:
    - the actual mode switch + `current` slot (so "Use as today's
      verse" can promote back into Read mode)
    - the bookmark CREATE callbacks (so other modes can bookmark
      from the read surface too)
    - the AgentRunPanel (Reflect/AI-reflect actions still resolve
      through the page-level panel)

  External entry points (cross-ref tap from Read, open-bookmark
  from Bookmarks) drive this component through the bindable
  `passage` / `chapter` props the parent sets via the exported
  `loadChapter()` method.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import {
    api,
    type Scripture,
    type BibleBookSummary,
    type BiblePassage,
    type BibleVerse,
    type BibleSearchHit
  } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { loadStored, saveStored } from '$lib/util/storage';
  import TranslationDiff from '$lib/scripture/TranslationDiff.svelte';
  import WordStudy from '$lib/scripture/WordStudy.svelte';
  import TaggedVerse from '$lib/scripture/TaggedVerse.svelte';

  let {
    aiLoading,
    onUseAsTodayVerse,
    onBookmarkPassage,
    onBookmarkVerse,
    onReflectPassage,
    onAIReflectPassage
  }: {
    aiLoading: boolean;
    onUseAsTodayVerse: (s: Scripture) => void;
    onBookmarkPassage: (p: BiblePassage) => void | Promise<void>;
    onBookmarkVerse: (bookCode: string, book: string, chapter: number, v: BibleVerse) => void | Promise<void>;
    onReflectPassage: (p: BiblePassage) => void | Promise<void>;
    onAIReflectPassage: (p: BiblePassage) => void | Promise<void>;
  } = $props();

  // ── Book index + active view ─────────────────────────────────────
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
  let pickerOpenBook = $state<string | null>(null);
  let pickerShowOT = $state(true);
  let pickerShowNT = $state(true);

  // ── Translation comparison + Strong's word study ─────────────────
  let compareOpen = $state(false);
  let wordStudyMode = $state(false);
  let selectedStrong = $state<string | null>(null);
  let strongsAvailable = $state<{ lexicon: boolean; tagged: boolean } | null>(null);
  async function ensureStrongsStatus() {
    if (strongsAvailable !== null) return;
    try {
      strongsAvailable = await api.strongsStatus();
    } catch {
      strongsAvailable = { lexicon: false, tagged: false };
    }
  }

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

  // First mount loads the index (lazy load: the parent only
  // instantiates this component when mode === 'bible').
  onMount(() => {
    void ensureBibleIndex();
  });

  async function bibleRandom() {
    try {
      const opts: { length?: number; testament?: 'OT' | 'NT' } = { length: bibleLengthFilter };
      if (bibleTestamentFilter) opts.testament = bibleTestamentFilter;
      biblePassage = await api.bibleRandom(opts);
      bibleChapter = null;
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // ── Reading history + streak (localStorage) ──────────────────────
  type RecentChapter = { bookCode: string; book: string; chapter: number; at: number };
  const RECENT_KEY = 'granit.bible.recent';
  const RECENT_MAX = 8;
  const STREAK_KEY = 'granit.bible.streak';
  type StreakState = { lastDay: string; streak: number };
  function loadStreak(): StreakState {
    const s = loadStored<StreakState | null>(STREAK_KEY, null);
    return s ? { lastDay: s.lastDay ?? '', streak: s.streak ?? 0 } : { lastDay: '', streak: 0 };
  }
  function saveStreak(s: StreakState): void { saveStored(STREAK_KEY, s); }
  let streak = $state<StreakState>(loadStreak());

  function todayKey(): string {
    const d = new Date();
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  }
  function bumpStreak() {
    const today = todayKey();
    if (streak.lastDay === today) return;
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
    const arr = loadStored<RecentChapter[]>(RECENT_KEY, []);
    return Array.isArray(arr) ? arr : [];
  }
  function saveRecent(list: RecentChapter[]): void { saveStored(RECENT_KEY, list); }
  let recent = $state<RecentChapter[]>(loadRecent());

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
      ensureStrongsStatus();
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

  function biblePassageToScripture(p: BiblePassage): Scripture {
    return {
      text: p.verses.map((v) => v.text).join(' '),
      source: `${p.reference} (WEB)`
    };
  }

  // Debounced server-side search.
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
    void loadBibleChapter(h.bookCode, h.chapter);
    setTimeout(() => {
      const el = document.getElementById(`bible-v-${h.verse}`);
      if (el) el.scrollIntoView({ behavior: 'smooth', block: 'center' });
    }, 100);
  }

  let bibleBooksOT = $derived(bibleBooks.filter((b) => b.testament === 'OT'));
  let bibleBooksNT = $derived(bibleBooks.filter((b) => b.testament === 'NT'));

  // Exposed for parent — cross-ref tap from Read mode + open-bookmark
  // from Bookmarks both need to drive a chapter load + optional
  // verse scroll. Kept as exported async fns so the parent can await
  // before scrolling.
  export async function openAt(bookCode: string, chapter: number, verse?: number) {
    await ensureBibleIndex();
    await loadBibleChapter(bookCode, chapter);
    if (verse) {
      setTimeout(() => {
        const el = document.getElementById(`bible-v-${verse}`);
        if (el) el.scrollIntoView({ behavior: 'smooth', block: 'center' });
      }, 100);
    }
  }

  // Header "Next" button → re-roll a random passage. Exposed so the
  // page can wire its quick-capture button mode-aware.
  export function rerollRandom() {
    void bibleRandom();
  }
</script>

{#if bibleLoading && bibleBooks.length === 0}
  <div class="text-sm text-dim">loading bible…</div>
{:else}
  <!-- Continue reading — surfaces the last chapter so the user can
       jump back into sequential reading without scrolling the picker. -->
  {#if recent.length > 0}
    <div class="bg-surface0 border border-surface1 rounded-lg p-3 mb-2.5">
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

  <!-- Random passage controls. -->
  <div class="bg-surface0 border border-surface1 rounded-lg p-3 mb-2.5">
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

  <!-- Server-side search. -->
  <div class="bg-surface0 border border-surface1 rounded-lg p-3 mb-2.5">
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
      <div class="flex gap-2 mt-4 flex-wrap">
        <button
          onclick={bibleRandom}
          class="px-3 py-1.5 text-sm bg-surface0 border border-surface1 rounded hover:border-primary"
        >Another passage</button>
        <button
          onclick={() => loadBibleChapter(biblePassage!.bookCode, biblePassage!.chapter)}
          class="px-3 py-1.5 text-sm bg-surface0 border border-surface1 rounded hover:border-primary"
        >Read full chapter →</button>
        <button
          onclick={() => onUseAsTodayVerse(biblePassageToScripture(biblePassage!))}
          class="px-3 py-1.5 text-sm bg-surface0 border border-surface1 rounded hover:border-primary"
        >Use as today's verse</button>
        <button
          onclick={() => onBookmarkPassage(biblePassage!)}
          class="px-3 py-1.5 text-sm bg-surface0 border border-surface1 rounded hover:border-primary"
          title="Save this passage to your bookmarks"
        >★ Bookmark</button>
        <button
          onclick={() => onReflectPassage(biblePassage!)}
          class="px-3 py-1.5 text-sm bg-surface0 border border-surface1 rounded hover:border-primary"
        >Reflect →</button>
        <button
          onclick={() => onAIReflectPassage(biblePassage!)}
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
              onclick={() => onBookmarkVerse(bibleChapter!.bookCode, bibleChapter!.book, bibleChapter!.chapter, v)}
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

    <!-- Translation + word-study toggles — opt-in panels riding on
         top of the chapter view. The components handle data-not-
         bundled cases themselves so the buttons stay discoverable. -->
    <div class="mt-4 flex flex-wrap items-center gap-2">
      <button
        type="button"
        onclick={() => (compareOpen = !compareOpen)}
        class="text-xs px-2.5 py-1 rounded border transition-colors {compareOpen ? 'bg-primary text-on-primary border-primary' : 'bg-mantle border-surface1 text-subtext hover:border-primary hover:text-text'}"
        title="Compare this chapter across the bundled translations"
      >{compareOpen ? 'Hide translations' : 'Compare translations'}</button>
      <button
        type="button"
        onclick={() => (wordStudyMode = !wordStudyMode)}
        class="text-xs px-2.5 py-1 rounded border transition-colors {wordStudyMode ? 'bg-primary text-on-primary border-primary' : 'bg-mantle border-surface1 text-subtext hover:border-primary hover:text-text'}"
        title="Toggle Strong's word-study mode — tap any word to see its lexicon entry"
      >{wordStudyMode ? 'Exit word study' : 'Word study mode'}{#if strongsAvailable && !strongsAvailable.tagged}<span class="opacity-70"> · not bundled</span>{/if}</button>
    </div>

    {#if compareOpen}
      <div class="mt-3">
        <TranslationDiff
          bookCode={bibleChapter.bookCode}
          chapter={bibleChapter.chapter}
        />
      </div>
    {/if}

    {#if wordStudyMode}
      <div class="mt-4 bg-surface0 border border-surface1 rounded-lg p-3 sm:p-4">
        <h3 class="text-xs uppercase tracking-wider text-dim font-medium mb-2">Word study</h3>
        <TaggedVerse
          bookCode={bibleChapter.bookCode}
          chapter={bibleChapter.chapter}
          onSelectStrong={(code) => (selectedStrong = code)}
        />
        {#if selectedStrong}
          <div class="mt-3 pt-3 border-t border-surface1">
            <WordStudy
              strongsCode={selectedStrong}
              onClose={() => (selectedStrong = null)}
            />
          </div>
        {/if}
      </div>
    {/if}
  {/if}

  <!-- Book / chapter picker — OT + NT in two collapsible cards. -->
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
