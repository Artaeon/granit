<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { goto } from '$app/navigation';
  import {
    api,
    type Scripture,
    type ScriptureTopic,
    type BibleBookSummary,
    type BiblePassage,
    type BibleVerse,
    type BibleSearchHit,
    type BibleBookmark
  } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import StreakBadge from '$lib/notes/StreakBadge.svelte';
  import AgentRunPanel from '$lib/agents/AgentRunPanel.svelte';
  import type { AgentPreset } from '$lib/api';
  import { loadStored, loadStoredString, saveStored, saveStoredString } from '$lib/util/storage';
  import TranslationDiff from '$lib/scripture/TranslationDiff.svelte';
  import WordStudy from '$lib/scripture/WordStudy.svelte';
  import TaggedVerse from '$lib/scripture/TaggedVerse.svelte';
  import BibleBookmarksMode from '$lib/scripture/BibleBookmarksMode.svelte';
  import PrayerIntentionsMode from '$lib/scripture/PrayerIntentionsMode.svelte';

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
  type Mode = 'read' | 'memo' | 'browse' | 'bible' | 'bookmarks' | 'intentions';

  let mode = $state<Mode>('read');
  let today = $state<Scripture | null>(null);
  let current = $state<Scripture | null>(null); // verse currently being viewed/drilled
  let all = $state<Scripture[]>([]);
  let topics = $state<ScriptureTopic[]>([]); // theme-tag chips for /browse
  let activeTopic = $state<string>(''); // active theme filter; '' = no filter
  let loading = $state(false);
  let q = $state('');

  // Reading typography size — three steps so the verse-of-the-day
  // surface accommodates both close-reading on a phone and a dim
  // fallback on a wall-mounted display. Stored in localStorage so the
  // preference sticks; per-device on purpose (a phone often wants a
  // different size than a desktop).
  type ReadSize = 'sm' | 'md' | 'lg';
  const READ_SIZE_KEY = 'granit.scripture.readSize';
  function loadReadSize(): ReadSize {
    const v = loadStoredString(READ_SIZE_KEY, 'md');
    return v === 'sm' || v === 'lg' ? v : 'md';
  }
  let readSize = $state<ReadSize>(loadReadSize());
  function setReadSize(s: ReadSize) {
    readSize = s;
    saveStoredString(READ_SIZE_KEY, s);
  }
  // Tailwind class triplet keyed by size — kept here so the three
  // sizes are visible at a glance and stay coordinated with line
  // height / cite-margin tweaks.
  let readVerseClass = $derived(
    readSize === 'sm'
      ? 'text-lg sm:text-xl leading-relaxed'
      : readSize === 'lg'
        ? 'text-2xl sm:text-3xl leading-loose'
        : 'text-xl sm:text-2xl leading-relaxed'
  );

  // Memorization state — see drillVerse() for the algorithm.
  let drill = $state<{ verse: Scripture; words: string[]; hidden: Set<number>; guesses: Record<number, string> } | null>(null);
  let revealed = $state(false);

  // Per-verse accuracy in localStorage, keyed by source. Trial count +
  // success count → ratio. Lower-ratio verses are picked more often in
  // memo mode so weak spots get more practice.
  type Stats = Record<string, { tries: number; correct: number }>;
  const STATS_KEY = 'granit.scripture.stats';

  function loadStats(): Stats { return loadStored<Stats>(STATS_KEY, {}); }
  function saveStats(s: Stats) { saveStored(STATS_KEY, s); }

  async function load() {
    loading = true;
    // Settled rather than Promise.all so a 500 on listScriptures
    // doesn't kill the verse-of-the-day surface (or vice versa) —
    // the two endpoints are independent reads and should degrade
    // independently.
    const [tRes, listRes] = await Promise.allSettled([
      api.todayScripture(),
      api.listScriptures()
    ]);
    if (tRes.status === 'fulfilled') {
      today = tRes.value;
      current = tRes.value;
    } else {
      toast.error('failed to load verse of the day: ' + (tRes.reason instanceof Error ? tRes.reason.message : String(tRes.reason)));
    }
    if (listRes.status === 'fulfilled') {
      all = listRes.value.scriptures;
      topics = listRes.value.topics ?? [];
    } else {
      toast.error('failed to load scriptures: ' + (listRes.reason instanceof Error ? listRes.reason.message : String(listRes.reason)));
    }
    loading = false;
  }

  // Switch the active topic filter. '' clears it. Topic state is held
  // separately from the loaded list — we re-pull from the server so the
  // server's case-insensitive matching is the source of truth (the
  // client used to filter locally, but with topical metadata only on
  // bundled defaults, server-side scoping is more honest about what's
  // available in the user's vault).
  async function selectTopic(topic: string) {
    activeTopic = topic;
    try {
      const list = await api.listScriptures(topic || undefined);
      all = list.scriptures;
      // Topics only refresh when the catalogue itself changes, but the
      // server returns the full list every call, so we keep ours in
      // sync cheaply.
      topics = list.topics ?? topics;
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // Semantic search in browse mode. The existing `q` filter is a
  // local substring match; this is the AI sibling — "verses about
  // waiting on God" pattern. The button next to the filter input
  // fires this with `q` as the query. Results render in a separate
  // AI-matches block above the catalogue list so the user can
  // compare both.
  let aiSearching = $state(false);
  let aiSearchResults = $state<Scripture[]>([]);
  let aiSearchTopics = $state<string[]>([]);
  let aiSearchQuery = $state(''); // echoed back so we can show "AI matches for <query>"

  async function runSemanticSearch() {
    const query = q.trim();
    if (!query || aiSearching) return;
    aiSearching = true;
    try {
      const r = await api.scriptureSemanticSearch({ query });
      aiSearchResults = r.scriptures;
      aiSearchTopics = r.topics;
      aiSearchQuery = r.query;
      if (r.scriptures.length === 0 && r.topics.length === 0) {
        toast.info(topics.length === 0
          ? 'Topical search needs the bundled catalogue — add topics to your scriptures.md or revert to defaults.'
          : 'No matching topics — try a different query.');
      }
    } catch (e) {
      toast.error('AI search failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      aiSearching = false;
    }
  }

  function clearSemanticSearch() {
    aiSearchResults = [];
    aiSearchTopics = [];
    aiSearchQuery = '';
  }

  // The bookmarks list (.granit/bible-bookmarks.json) lives inside
  // $lib/scripture/BibleBookmarksMode.svelte — that component owns
  // the lazy load + delete + save-note + its own WS subscription.
  // The parent still owns the CREATE path (bookmarkPassage /
  // bookmarkVerse below) because those are triggered from the
  // bible-reader buttons; the panel auto-refreshes via WS when the
  // server emits state.changed on the bookmark file.

  onMount(() => {
    load();
    // Reading-habit streak: fire-and-forget on every page mount.
    // Idempotent server-side (RecordRead dedupes by date), so the
    // 50ms call is harmless on a re-visit and only matters the
    // first time the user opens scripture each calendar day. We
    // don't show a toast — the StreakBadge re-fetch via the
    // .granit/bible-reading-log.json WS broadcast surfaces the
    // updated count instead.
    void api.recordBibleRead().catch(() => {
      // Silent: a streak-tracking failure must never block the
      // user from reading scripture.
    });
    // Both bookmark + intention data files are now owned by their
    // own mode components (BibleBookmarksMode / PrayerIntentionsMode)
    // which carry their own WS subscriptions. No path-level handling
    // here anymore.
    return () => {};
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
      // No explicit reload — the server emits state.changed on
      // .granit/bible-bookmarks.json, and BibleBookmarksMode picks
      // it up to refresh its list. Skips a redundant fetch when the
      // panel isn't open yet.
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
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // Open a bookmark in the bible reader: load its chapter, scroll to
  // the first verse. The user can copy the saved snippet but the
  // chapter view shows the canonical translation alongside. Called
  // by BibleBookmarksMode via the onOpenBookmark prop.
  async function openBookmark(b: BibleBookmark) {
    mode = 'bible';
    await ensureBibleIndex();
    await loadBibleChapter(b.bookCode, b.chapter);
    setTimeout(() => {
      const el = document.getElementById(`bible-v-${b.verseFrom}`);
      if (el) el.scrollIntoView({ behavior: 'smooth', block: 'center' });
    }, 100);
  }

  // ── Prayer intentions ────────────────────────────────────────────
  // The prayer-list (.granit/prayer/intentions.json) lives inside
  // $lib/scripture/PrayerIntentionsMode.svelte — own lazy load, own
  // WS subscription, own CRUD. The bindable `prayingCount` below
  // mirrors the component's "currently praying" count up so the tab
  // badge stays live.
  let prayingCount = $state(0);

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

  // AI commentary — short in-page gloss streamed via /chat/stream.
  // Distinct from "AI reflection" (which spawns a full devotional note
  // via the agent preset): commentary is a 3-paragraph contextual
  // explainer rendered right under the verse so the user can ask a
  // quick question without leaving the read view. Five flavours:
  //   - context:     narrative / literary background (who/what/where in the text)
  //   - historical:  material + cultural background (customs, geography, audience)
  //   - cross-ref:   thematic parallels elsewhere in scripture
  //   - application: how this might land for the reader today
  //   - genre:       genre-aware reading prompts keyed off the verse's book
  // Each is a one-shot chatStream call; results land in commentaryText
  // as tokens arrive. Audit-gated through /chat/stream like every
  // other AI surface in granit.
  type CommentaryMode = 'context' | 'historical' | 'cross-ref' | 'application' | 'genre';
  let commentaryText = $state('');
  let commentaryMode = $state<CommentaryMode | null>(null);
  let commentaryStreaming = $state(false);
  let commentaryError = $state('');
  let commentaryAbort: AbortController | null = $state(null);

  // Bible-book genre buckets — used by the `genre` commentary mode to
  // pick a tailored prompt and by the chip strip to colour-code refs.
  // Bucketing follows the conventional Protestant divisions; Acts is
  // pulled out separately because narrative-of-the-church reads
  // differently from gospels-narrative or torah-narrative.
  type Genre = 'Torah' | 'History' | 'Wisdom' | 'Prophets' | 'Gospels' | 'Acts' | 'Epistles' | 'Apocalyptic' | 'Unknown';

  // Display-name → USFM code. Includes full names plus common
  // abbreviations encountered in AI prose. Keys are normalized via
  // normalizeRefKey() at lookup time so casing / spacing / trailing
  // dots ("Rom.") all collapse to the same entry.
  const BOOK_TO_USFM: Record<string, string> = {
    // OT
    genesis: 'GEN', gen: 'GEN', gn: 'GEN',
    exodus: 'EXO', exo: 'EXO', ex: 'EXO',
    leviticus: 'LEV', lev: 'LEV', lv: 'LEV',
    numbers: 'NUM', num: 'NUM', nm: 'NUM',
    deuteronomy: 'DEU', deut: 'DEU', dt: 'DEU',
    joshua: 'JOS', josh: 'JOS', jos: 'JOS',
    judges: 'JDG', judg: 'JDG', jdg: 'JDG',
    ruth: 'RUT', ru: 'RUT',
    '1samuel': '1SA', '1sam': '1SA', '1sa': '1SA',
    '2samuel': '2SA', '2sam': '2SA', '2sa': '2SA',
    '1kings': '1KI', '1kgs': '1KI', '1ki': '1KI',
    '2kings': '2KI', '2kgs': '2KI', '2ki': '2KI',
    '1chronicles': '1CH', '1chron': '1CH', '1chr': '1CH', '1ch': '1CH',
    '2chronicles': '2CH', '2chron': '2CH', '2chr': '2CH', '2ch': '2CH',
    ezra: 'EZR', ezr: 'EZR',
    nehemiah: 'NEH', neh: 'NEH',
    esther: 'EST', est: 'EST', esth: 'EST',
    job: 'JOB',
    psalms: 'PSA', psalm: 'PSA', ps: 'PSA', psa: 'PSA',
    proverbs: 'PRO', prov: 'PRO', pr: 'PRO', pro: 'PRO',
    ecclesiastes: 'ECC', eccl: 'ECC', ecc: 'ECC', qoh: 'ECC',
    songofsolomon: 'SNG', songofsongs: 'SNG', song: 'SNG', sng: 'SNG', canticles: 'SNG',
    isaiah: 'ISA', isa: 'ISA', is: 'ISA',
    jeremiah: 'JER', jer: 'JER',
    lamentations: 'LAM', lam: 'LAM',
    ezekiel: 'EZK', ezek: 'EZK', ezk: 'EZK',
    daniel: 'DAN', dan: 'DAN', dn: 'DAN',
    hosea: 'HOS', hos: 'HOS',
    joel: 'JOL', jl: 'JOL', jol: 'JOL',
    amos: 'AMO', am: 'AMO',
    obadiah: 'OBA', obad: 'OBA', oba: 'OBA', ob: 'OBA',
    jonah: 'JON', jon: 'JON',
    micah: 'MIC', mic: 'MIC',
    nahum: 'NAM', nah: 'NAM', nam: 'NAM',
    habakkuk: 'HAB', hab: 'HAB',
    zephaniah: 'ZEP', zeph: 'ZEP', zep: 'ZEP',
    haggai: 'HAG', hag: 'HAG',
    zechariah: 'ZEC', zech: 'ZEC', zec: 'ZEC',
    malachi: 'MAL', mal: 'MAL',
    // NT
    matthew: 'MAT', matt: 'MAT', mat: 'MAT', mt: 'MAT',
    mark: 'MRK', mrk: 'MRK', mk: 'MRK',
    luke: 'LUK', luk: 'LUK', lk: 'LUK',
    john: 'JHN', jhn: 'JHN', jn: 'JHN',
    acts: 'ACT', act: 'ACT',
    romans: 'ROM', rom: 'ROM', rm: 'ROM',
    '1corinthians': '1CO', '1cor': '1CO', '1co': '1CO',
    '2corinthians': '2CO', '2cor': '2CO', '2co': '2CO',
    galatians: 'GAL', gal: 'GAL',
    ephesians: 'EPH', eph: 'EPH',
    philippians: 'PHP', phil: 'PHP', php: 'PHP', phlp: 'PHP',
    colossians: 'COL', col: 'COL',
    '1thessalonians': '1TH', '1thess': '1TH', '1thes': '1TH', '1th': '1TH',
    '2thessalonians': '2TH', '2thess': '2TH', '2thes': '2TH', '2th': '2TH',
    '1timothy': '1TI', '1tim': '1TI', '1ti': '1TI',
    '2timothy': '2TI', '2tim': '2TI', '2ti': '2TI',
    titus: 'TIT', tit: 'TIT',
    philemon: 'PHM', philem: 'PHM', phm: 'PHM', phlm: 'PHM',
    hebrews: 'HEB', heb: 'HEB',
    james: 'JAS', jas: 'JAS', jm: 'JAS',
    '1peter': '1PE', '1pet': '1PE', '1pe': '1PE', '1pt': '1PE',
    '2peter': '2PE', '2pet': '2PE', '2pe': '2PE', '2pt': '2PE',
    '1john': '1JN', '1jn': '1JN', '1jhn': '1JN',
    '2john': '2JN', '2jn': '2JN', '2jhn': '2JN',
    '3john': '3JN', '3jn': '3JN', '3jhn': '3JN',
    jude: 'JUD', jud: 'JUD',
    revelation: 'REV', rev: 'REV', revelations: 'REV', apocalypse: 'REV'
  };

  function normalizeRefKey(s: string): string {
    let out = '';
    for (const ch of s.toLowerCase()) {
      if ((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')) out += ch;
    }
    return out;
  }

  // USFM code → genre. Kept as a flat lookup so adding/correcting a
  // single book is a one-line change.
  const USFM_TO_GENRE: Record<string, Genre> = {
    GEN: 'Torah', EXO: 'Torah', LEV: 'Torah', NUM: 'Torah', DEU: 'Torah',
    JOS: 'History', JDG: 'History', RUT: 'History',
    '1SA': 'History', '2SA': 'History', '1KI': 'History', '2KI': 'History',
    '1CH': 'History', '2CH': 'History', EZR: 'History', NEH: 'History', EST: 'History',
    JOB: 'Wisdom', PSA: 'Wisdom', PRO: 'Wisdom', ECC: 'Wisdom', SNG: 'Wisdom',
    ISA: 'Prophets', JER: 'Prophets', LAM: 'Prophets', EZK: 'Prophets',
    HOS: 'Prophets', JOL: 'Prophets', AMO: 'Prophets', OBA: 'Prophets',
    JON: 'Prophets', MIC: 'Prophets', NAM: 'Prophets', HAB: 'Prophets',
    ZEP: 'Prophets', HAG: 'Prophets', ZEC: 'Prophets', MAL: 'Prophets',
    DAN: 'Apocalyptic',
    MAT: 'Gospels', MRK: 'Gospels', LUK: 'Gospels', JHN: 'Gospels',
    ACT: 'Acts',
    ROM: 'Epistles', '1CO': 'Epistles', '2CO': 'Epistles', GAL: 'Epistles',
    EPH: 'Epistles', PHP: 'Epistles', COL: 'Epistles',
    '1TH': 'Epistles', '2TH': 'Epistles', '1TI': 'Epistles', '2TI': 'Epistles',
    TIT: 'Epistles', PHM: 'Epistles', HEB: 'Epistles', JAS: 'Epistles',
    '1PE': 'Epistles', '2PE': 'Epistles',
    '1JN': 'Epistles', '2JN': 'Epistles', '3JN': 'Epistles', JUD: 'Epistles',
    REV: 'Apocalyptic'
  };

  // Pauline corpus is a strict subset of Epistles — needed because the
  // genre prompt for Paul (command + indicative + audience) differs from
  // the Catholic / Johannine epistles. Heuristic only; Hebrews is left
  // out (authorship contested) but slots into the general Epistles
  // bucket which is fine for this surface.
  const PAULINE: ReadonlySet<string> = new Set([
    'ROM', '1CO', '2CO', 'GAL', 'EPH', 'PHP', 'COL',
    '1TH', '2TH', '1TI', '2TI', 'TIT', 'PHM'
  ]);

  // Pull the leading book token out of "Romans 8:28" / "1 Cor. 13:4" /
  // "Psalms 23:1". Returns the USFM code or null. The verse parsing
  // proper is in parseRefs() — this is the front-half lookup, factored
  // out so bookGenre() can reuse it.
  function sourceToUSFM(source: string): string | null {
    if (!source) return null;
    // Match: optional leading numeral (1/2/3), then alphabetic book
    // tokens (possibly multiple, e.g. "Song of Solomon"). We accept
    // letters + spaces + dots, then stop at the first digit (chapter).
    const m = source.match(/^\s*((?:[123]\s*)?[A-Za-z][A-Za-z.\s]*?)\s+\d/);
    if (!m) return null;
    const key = normalizeRefKey(m[1]);
    return BOOK_TO_USFM[key] ?? null;
  }

  function bookGenre(source: string): Genre {
    const code = sourceToUSFM(source);
    if (!code) return 'Unknown';
    return USFM_TO_GENRE[code] ?? 'Unknown';
  }

  // Per-genre prompt body. The verse line is appended by the caller.
  const GENRE_PROMPTS: Record<Genre, string> = {
    Wisdom:
      `This is wisdom / Psalm-style literature. In two short paragraphs: ` +
      `What emotion is named or evoked? What posture toward God does this passage ` +
      `model — petition, lament, praise, trust, perplexity? How might a reader ` +
      `borrow that posture in their own circumstances?`,
    Torah:
      `This is Torah / narrative. In two short paragraphs: ` +
      `What does this scene reveal about God's character or the human condition? ` +
      `What's the turning point — the moment things shift? Stick to what the text ` +
      `actually shows; resist allegorizing.`,
    History:
      `This is Old Testament historical narrative. In two short paragraphs: ` +
      `What does this scene reveal about God's character or the human condition? ` +
      `What's the turning point? How does the narrator's framing (what's named, ` +
      `what's omitted) shape the meaning?`,
    Gospels:
      `This is gospel narrative. In two short paragraphs: ` +
      `What does this scene reveal about Jesus' character or the human condition ` +
      `he addresses? What's the turning point — the question, healing, saying, ` +
      `or rebuke that changes the scene's center of gravity?`,
    Acts:
      `This is narrative of the early church. In two short paragraphs: ` +
      `What does this scene reveal about how the Spirit is moving and how the ` +
      `church is responding (or failing to)? What's the turning point? What ` +
      `pattern of mission or community emerges?`,
    Prophets:
      `This is prophetic literature. In two short paragraphs: ` +
      `What is God indicting or promising? To whom — which audience, under what ` +
      `covenant frame? What does the prophet expect that audience to do, feel, ` +
      `or hope for in response?`,
    Apocalyptic:
      `This is apocalyptic literature — symbol-heavy and addressed to readers ` +
      `under pressure. In two short paragraphs: What symbol stands for what here? ` +
      `What hope is being offered to suffering readers, and what posture of ` +
      `endurance does the passage call for?`,
    Epistles:
      `This is an epistle. In two short paragraphs: ` +
      `What command or exhortation is given? What indicative truth (about God, ` +
      `Christ, or the gospel) grounds that command? Who is the original audience ` +
      `and what situation are they being addressed in?`,
    Unknown:
      `Read this passage closely. In two short paragraphs: ` +
      `What is the central claim or movement? How does its place in the broader ` +
      `biblical narrative shape its meaning?`
  };

  function commentaryPrompt(verse: Scripture, kind: CommentaryMode): string {
    const ref = verse.source ? `${verse.source}: ` : '';
    const verseLine = `${ref}"${verse.text}"`;
    switch (kind) {
      case 'context':
        return (
          `Provide concise historical and literary context for this passage. ` +
          `Two short paragraphs: who wrote it, who it was originally addressed to, ` +
          `what was happening, and how the surrounding chapter shapes its meaning. ` +
          `Stick to scholarly consensus; mark anything contested as such.\n\n${verseLine}`
        );
      case 'historical':
        return (
          `Describe the ancient cultural and material world behind this passage. ` +
          `Two short paragraphs: relevant customs, geography, social structures, ` +
          `who said what to whom and why that mattered in their setting, and any ` +
          `material details (clothing, money, agriculture, ritual) a modern reader ` +
          `would otherwise miss. Focus on what's distinctive to the ancient ` +
          `context — not narrative/literary moves (that's a separate lens).` +
          `\n\n${verseLine}`
        );
      case 'cross-ref':
        return (
          `List 3-5 cross-references — other passages of scripture that echo or ` +
          `expand on this one. For each, give the citation and one sentence on ` +
          `the connection. Plain text list, no preamble.\n\n${verseLine}`
        );
      case 'application':
        return (
          `Offer a thoughtful, non-trite personal application of this passage for ` +
          `a contemporary reader. Two paragraphs. Avoid platitudes; be specific ` +
          `about the kinds of situations where this verse might land. End with ` +
          `one open-ended question for reflection.\n\n${verseLine}`
        );
      case 'genre': {
        const g = bookGenre(verse.source ?? '');
        // Pauline gets a Paul-specific override (command + indicative +
        // audience); everything else uses the genre default.
        const code = sourceToUSFM(verse.source ?? '');
        const body =
          code && PAULINE.has(code)
            ? `This is a Pauline epistle. In two short paragraphs: ` +
              `What command or exhortation does Paul give? What indicative truth ` +
              `(about God, Christ, or the gospel) grounds that command? Who is ` +
              `the original audience and what situation are they being addressed in?`
            : GENRE_PROMPTS[g];
        return `${body}\n\n${verseLine}`;
      }
    }
  }

  // Parse canonical bible refs out of free-form prose. Matches:
  //   - "John 3:16", "John 3:16-17"
  //   - "1 Corinthians 13:4", "1 Cor. 13:4-7"
  //   - "Ps. 23:1", "Rom. 8:28"
  // Returns deduped refs in order of first appearance. Verse is
  // optional (some refs are chapter-only — "Romans 8"); we capture it
  // when present so the click handler can scroll to the right anchor.
  //
  // Strategy: we anchor on the chapter+verse "N:N" / "N" digit-tail and
  // walk *backwards* to identify the book token. This avoids the
  // greedy-cap-word trap (a leading "See John 3:16" would otherwise
  // swallow "See" as part of a two-word book name and skip past the
  // real "John" match). Books are matched against BOOK_TO_USFM directly,
  // so anything not in the dictionary simply isn't a ref.
  type ParsedRef = { label: string; bookCode: string; chapter: number; verse?: number };
  function parseRefs(text: string): ParsedRef[] {
    if (!text) return [];
    // Anchor: capture the digit-tail. The book token sits before the
    // gap of whitespace. We accept up to 3 capitalised words before
    // the digits — that covers "Song of Solomon" and "1 Corinthians".
    // The optional leading numeral is folded into the same backtrack.
    const re = /\b((?:[123]\s+)?[A-Z][A-Za-z]+(?:\.?\s+(?:of\s+)?[A-Z]?[A-Za-z]+)?)\.?\s+(\d{1,3})(?::(\d{1,3})(?:-(\d{1,3}))?)?\b/g;
    const out: ParsedRef[] = [];
    const seen = new Set<string>();
    let m: RegExpExecArray | null;
    while ((m = re.exec(text)) !== null) {
      const bookRaw = m[1];
      const chapter = parseInt(m[2], 10);
      const verseStr = m[3];
      const verseEndStr = m[4];
      const verse = verseStr ? parseInt(verseStr, 10) : undefined;
      if (!Number.isFinite(chapter) || chapter < 1) continue;
      // Try the full captured book token first; if that misses (e.g.
      // "See John" or "John from" — extra adjoining word), peel one
      // word off either end and retry. This lets us recover from
      // mid-sentence captures without rewriting the regex engine. We
      // only try one-word peels because two-word real book names
      // ("Song of Solomon", "1 John") need to keep both tokens.
      let code = BOOK_TO_USFM[normalizeRefKey(bookRaw)];
      let usedBook = bookRaw;
      if (!code) {
        const parts = bookRaw.split(/\s+/);
        if (parts.length >= 2) {
          // Try dropping the leading word — handles "See John 3:16".
          const tail = parts.slice(1).join(' ');
          code = BOOK_TO_USFM[normalizeRefKey(tail)];
          if (code) usedBook = tail;
          else {
            // Try dropping the trailing word — handles "John from 3:16".
            const head = parts.slice(0, -1).join(' ');
            code = BOOK_TO_USFM[normalizeRefKey(head)];
            if (code) usedBook = head;
          }
        }
      }
      if (!code) continue;
      // Build a chip label that mirrors the surface form (sans trailing
      // dot) so the chip visually matches the prose.
      let label = `${usedBook.replace(/\.$/, '')} ${chapter}`;
      if (verse) label += `:${verse}${verseEndStr ? `-${verseEndStr}` : ''}`;
      const dedupKey = `${code}|${chapter}|${verse ?? ''}`;
      if (seen.has(dedupKey)) continue;
      seen.add(dedupKey);
      out.push({ label, bookCode: code, chapter, verse });
    }
    return out;
  }

  // Derive chips from the streamed commentary text. Only meaningful in
  // cross-ref mode, but the derivation is cheap and re-runs on every
  // token while streaming so the strip "builds up" alongside the prose.
  let crossRefChips = $derived.by(() => {
    if (commentaryMode !== 'cross-ref') return [];
    return parseRefs(commentaryText);
  });

  // Click handler for a cross-ref chip — jump into the Bible reader,
  // open the chapter, scroll to the verse anchor if we have one.
  // Mirrors the existing openBookmark() / openHit() pattern (setTimeout
  // 100 to give the chapter DOM time to render after the async load).
  async function gotoRef(r: ParsedRef) {
    mode = 'bible';
    await ensureBibleIndex();
    await loadBibleChapter(r.bookCode, r.chapter);
    if (r.verse) {
      setTimeout(() => {
        const el = document.getElementById(`bible-v-${r.verse}`);
        if (el) el.scrollIntoView({ behavior: 'smooth', block: 'center' });
      }, 100);
    }
  }

  async function runCommentary(kind: CommentaryMode) {
    if (!current) return;
    // If a stream is in flight from a previous click, cancel it before
    // starting a new one — otherwise tokens from both calls interleave
    // into the same buffer.
    if (commentaryAbort) {
      commentaryAbort.abort();
      commentaryAbort = null;
    }
    commentaryMode = kind;
    commentaryText = '';
    commentaryError = '';
    commentaryStreaming = true;
    const ctl = new AbortController();
    commentaryAbort = ctl;
    await api.chatStream(
      [{ role: 'user', content: commentaryPrompt(current, kind) }],
      undefined,
      {
        onChunk: (chunk) => {
          commentaryText += chunk;
        },
        onDone: () => {
          commentaryStreaming = false;
          commentaryAbort = null;
          if (!commentaryText) commentaryError = 'AI returned an empty response.';
        },
        onError: (err) => {
          commentaryStreaming = false;
          commentaryAbort = null;
          commentaryError = err.message;
        }
      },
      ctl.signal
    );
  }

  function stopCommentary() {
    if (commentaryAbort) {
      commentaryAbort.abort();
      commentaryAbort = null;
    }
    commentaryStreaming = false;
  }

  function clearCommentary() {
    stopCommentary();
    commentaryText = '';
    commentaryMode = null;
    commentaryError = '';
  }

  // Copy the visible verse to clipboard. Falls back to a manual prompt
  // if the Clipboard API isn't available (Safari < 13 / non-secure
  // contexts). The caller passes either the curated verse or a bible
  // passage; we render a "TEXT — SOURCE" string either way so paste
  // targets get a self-contained quote.
  async function copyVerseToClipboard(s: Scripture) {
    const out = s.source ? `"${s.text}" — ${s.source}` : `"${s.text}"`;
    try {
      await navigator.clipboard.writeText(out);
      toast.success('copied');
    } catch {
      toast.error('clipboard unavailable');
    }
  }

  // Append the visible verse to today's jot (the user's daily note)
  // as a markdown blockquote. Distinct from "Reflect on this", which
  // creates a fresh Devotionals/ note — this is for "I want to keep a
  // running log of verses I noticed today" alongside the rest of
  // today's journaling. Idempotent on duplicate clicks (we re-fetch
  // the note body each time, so concurrent edits aren't lost).
  async function saveToTodaysJot(s: Scripture) {
    try {
      const note = await api.daily('today');
      const block =
        '\n\n## Scripture\n\n' +
        `> ${s.text}\n` +
        (s.source ? `> — ${s.source}\n` : '');
      const next = (note.body ?? '') + block;
      await api.putNote(
        note.path,
        { frontmatter: note.frontmatter, body: next },
        // No etag — we just fetched the note ourselves; if a write
        // sneaks in between fetch and save the user gets a 412 toast
        // and can retry.
        undefined
      );
      toast.success('added to today\'s jot');
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // Reset commentary whenever the verse changes — stale commentary
  // under a different verse is worse than no commentary. We track
  // identity via a string key (text + source) so derived reads of
  // `current` itself don't tangle dependencies; the cleanup is wrapped
  // in untrack so any state writes inside don't loop the effect.
  $effect(() => {
    // Tracked: identity key. Untracked: the actual reset.
    const _key = current ? `${current.text}|${current.source ?? ''}` : '';
    untrack(() => {
      void _key;
      clearCommentary();
    });
  });

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

  // ─── Spaced-repetition scheduling ─────────────────────────────────
  // After a drill the user can schedule a 5-step review series on the
  // calendar: day 1, 3, 7, 14, 30 from today. Each step is a scheduled
  // task anchored to today's daily note (we can't predict a future
  // daily note's existence, and the calendar surfaces task_scheduled
  // rows by scheduledStart, not by notePath). The user lands on a
  // calendar row at 09:00 on each offset day with a one-line task to
  // review the verse — text body carries the verse so the calendar
  // hover already shows what to recite.
  const REVIEW_INTERVALS = [1, 3, 7, 14, 30];

  // Scheduling can be busy for a moment (5 sequential POSTs). Track
  // state so the button can disable + show progress without firing
  // duplicate series.
  let schedulingReview = $state(false);

  async function scheduleVerseReview(verse: Scripture) {
    if (schedulingReview) return;
    schedulingReview = true;
    try {
      // Daily note ensures today's daily exists — the task store
      // requires a real notePath. Failing to ensure → no anchor →
      // no series.
      const today = await api.daily('today');
      const baseDate = new Date();
      baseDate.setHours(9, 0, 0, 0);
      const label = verse.source ? verse.source : verse.text.slice(0, 40) + '…';
      const verseBody = verse.source
        ? `${verse.source}: "${verse.text}"`
        : `"${verse.text}"`;
      let scheduled = 0;
      for (const days of REVIEW_INTERVALS) {
        const at = new Date(baseDate.getTime() + days * 86_400_000);
        try {
          await api.createTask({
            notePath: today.path,
            text: `Review verse · ${label} (day ${days})\n${verseBody}`,
            scheduledStart: at.toISOString(),
            durationMinutes: 5,
            tags: ['memory-verse']
          });
          scheduled++;
        } catch (e) {
          // Surface the failure but keep going — partial schedule is
          // better than dropping the whole series. The toast at the
          // end summarises the outcome either way.
          console.error('Failed to schedule review day', days, e);
        }
      }
      if (scheduled === REVIEW_INTERVALS.length) {
        toast.success(`Review scheduled — ${scheduled} sessions over the next 30 days.`);
      } else if (scheduled > 0) {
        toast.info(`Scheduled ${scheduled} of ${REVIEW_INTERVALS.length} reviews — some failed.`);
      } else {
        toast.error('Failed to schedule review series.');
      }
    } catch (e) {
      toast.error('schedule failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      schedulingReview = false;
    }
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
  let totalTries = $derived.by(() =>
    Object.values(loadStats()).reduce((sum, x) => sum + x.tries, 0)
  );
  let totalCorrect = $derived.by(() =>
    Object.values(loadStats()).reduce((sum, x) => sum + x.correct, 0)
  );

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

  // ─── Translation comparison + Strong's word study toggles ────────
  // Both are opt-in panels that ride on top of the existing chapter
  // view. The components themselves handle the "data not bundled"
  // case gracefully, so a vanilla install just shows a single
  // translation + no Strong's data without ceremony.
  let compareOpen = $state(false);
  let wordStudyMode = $state(false);
  let selectedStrong = $state<string | null>(null);
  // Lazy-fetched once; null until we know. Lets the toggle button
  // render disabled+"not bundled" cleanly instead of flashing on/off.
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
      // Lazy-fetch Strong's status the first time the user lands in a
      // chapter — drives the "Word study" toggle's disabled/enabled
      // state. Fires once per session, cached afterward.
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
    <div class="flex flex-wrap items-baseline gap-3 mb-4">
      <div class="flex-1 min-w-0">
        <PageHeader title="Scripture" subtitle="Verse of the day, memorization drill, full bible (WEB)" />
      </div>
      <!-- Vault-backed reading streak. Same component the notes
           editor uses, parameterised on source so a fresh open
           hits /bible/streak (server-side, durable across devices)
           instead of the localStorage-only counter the inline
           span used to read. -->
      <div class="flex-shrink-0">
        <StreakBadge source="bible" />
      </div>
    </div>

    <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-sm mb-4 flex-wrap">
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
        onclick={() => (mode = 'bookmarks')}
      >Bookmarks</button>
      <button
        class="px-4 py-2 {mode === 'intentions' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
        onclick={() => (mode = 'intentions')}
      >Prayer {#if prayingCount > 0}<span class="text-[10px] opacity-70">{prayingCount}</span>{/if}</button>
    </div>

    {#if loading && all.length === 0}
      <div class="text-sm text-dim">loading…</div>
    {:else if mode === 'read'}
      {#if current}
        <!-- Reading-size control — three-step. Hidden in a top-right
             affordance so it doesn't compete with the verse for
             attention. Sticks per-device via localStorage. -->
        <div class="flex justify-end mb-2">
          <div class="inline-flex bg-surface0 border border-surface1 rounded text-[11px] overflow-hidden">
            <button
              type="button"
              onclick={() => setReadSize('sm')}
              class="px-2 py-0.5 {readSize === 'sm' ? 'bg-primary text-on-primary' : 'text-dim hover:bg-surface1'}"
              title="Smaller text"
              aria-label="Smaller text"
            >A</button>
            <button
              type="button"
              onclick={() => setReadSize('md')}
              class="px-2 py-0.5 {readSize === 'md' ? 'bg-primary text-on-primary' : 'text-dim hover:bg-surface1'}"
              title="Medium text"
              aria-label="Medium text"
            >A</button>
            <button
              type="button"
              onclick={() => setReadSize('lg')}
              class="px-2 py-0.5 {readSize === 'lg' ? 'bg-primary text-on-primary' : 'text-dim hover:bg-surface1'}"
              title="Larger text"
              aria-label="Larger text"
            >A</button>
          </div>
        </div>
        <article class="bg-surface0 border border-surface1 rounded-lg p-6 sm:p-8 text-center">
          <blockquote class="{readVerseClass} text-text font-serif italic">
            "{current.text}"
          </blockquote>
          {#if current.source}
            <cite class="text-sm text-subtext mt-4 block not-italic">— {current.source}</cite>
          {/if}
          {#if current.topics && current.topics.length > 0}
            <!-- Tag strip: clicking a topic flips into Browse mode
                 scoped to that theme — adjacent verses sharing the
                 same theme become discoverable in one click. -->
            <div class="flex flex-wrap gap-1.5 justify-center mt-4">
              {#each current.topics as tag (tag)}
                <button
                  type="button"
                  onclick={() => { mode = 'browse'; selectTopic(tag); }}
                  class="text-[11px] px-2 py-0.5 rounded-full bg-mantle border border-surface1 text-dim hover:border-primary hover:text-text"
                  title="Browse verses tagged {tag}"
                >{tag}</button>
              {/each}
            </div>
          {/if}
        </article>
        <div class="flex gap-2 justify-center mt-4 flex-wrap">
          <button
            onclick={anotherOne}
            class="px-4 py-2 text-sm bg-surface0 border border-surface1 rounded hover:border-primary"
          >Another verse</button>
          <button
            onclick={() => copyVerseToClipboard(current!)}
            class="px-4 py-2 text-sm bg-surface0 border border-surface1 rounded hover:border-primary"
            title="Copy verse + citation to clipboard"
          >Copy</button>
          <button
            onclick={() => saveToTodaysJot(current!)}
            class="px-4 py-2 text-sm bg-surface0 border border-surface1 rounded hover:border-primary"
            title="Append the verse to today's jot as a blockquote"
          >Save to today's jot</button>
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

        <!-- AI commentary — five quick lenses, streamed in-page. The
             gloss is intentionally short and ephemeral; the user can
             always fall through to "Reflect on this" or "AI reflection"
             when they want a saved devotional note. Cross-references
             additionally render a chip strip below the prose for
             one-click jumps into the bible reader. -->
        <div class="bg-surface0 border border-surface1 rounded-lg p-3 mt-4">
          <div class="flex items-baseline gap-2 flex-wrap">
            <span class="text-xs uppercase tracking-wider text-dim font-medium">Ask AI</span>
            <button
              type="button"
              onclick={() => runCommentary('context')}
              disabled={commentaryStreaming}
              class="text-xs px-2.5 py-1 rounded border transition-colors disabled:opacity-50 {commentaryMode === 'context' ? 'bg-primary text-on-primary border-primary' : 'bg-mantle border-surface1 text-subtext hover:border-primary hover:text-text'}"
              title="Narrative and literary background"
            >Context</button>
            <button
              type="button"
              onclick={() => runCommentary('historical')}
              disabled={commentaryStreaming}
              class="text-xs px-2.5 py-1 rounded border transition-colors disabled:opacity-50 {commentaryMode === 'historical' ? 'bg-primary text-on-primary border-primary' : 'bg-mantle border-surface1 text-subtext hover:border-primary hover:text-text'}"
              title="Ancient customs, geography, audience, material culture"
            >Historical</button>
            <button
              type="button"
              onclick={() => runCommentary('cross-ref')}
              disabled={commentaryStreaming}
              class="text-xs px-2.5 py-1 rounded border transition-colors disabled:opacity-50 {commentaryMode === 'cross-ref' ? 'bg-primary text-on-primary border-primary' : 'bg-mantle border-surface1 text-subtext hover:border-primary hover:text-text'}"
              title="Other passages that echo this one"
            >Cross-references</button>
            <button
              type="button"
              onclick={() => runCommentary('application')}
              disabled={commentaryStreaming}
              class="text-xs px-2.5 py-1 rounded border transition-colors disabled:opacity-50 {commentaryMode === 'application' ? 'bg-primary text-on-primary border-primary' : 'bg-mantle border-surface1 text-subtext hover:border-primary hover:text-text'}"
              title="How this might land for you today"
            >Application</button>
            <button
              type="button"
              onclick={() => runCommentary('genre')}
              disabled={commentaryStreaming}
              class="text-xs px-2.5 py-1 rounded border transition-colors disabled:opacity-50 {commentaryMode === 'genre' ? 'bg-primary text-on-primary border-primary' : 'bg-mantle border-surface1 text-subtext hover:border-primary hover:text-text'}"
              title="Genre-aware reading prompts ({current?.source ? bookGenre(current.source) : 'auto-detected from book'})"
            >Genre {#if current?.source}<span class="opacity-70">· {bookGenre(current.source)}</span>{/if}</button>
            <span class="flex-1"></span>
            {#if commentaryStreaming}
              <button
                type="button"
                onclick={stopCommentary}
                class="text-xs text-dim hover:text-error"
              >stop</button>
            {:else if commentaryText}
              <button
                type="button"
                onclick={clearCommentary}
                class="text-xs text-dim hover:text-text"
              >clear</button>
            {/if}
          </div>
          {#if commentaryText || commentaryStreaming || commentaryError}
            <div class="mt-3 pt-3 border-t border-surface1">
              {#if commentaryError}
                <p class="text-xs text-error">{commentaryError}</p>
              {:else}
                <p class="text-sm text-text leading-relaxed whitespace-pre-wrap">{commentaryText}{#if commentaryStreaming}<span class="inline-block w-2 h-3.5 align-middle bg-primary/70 ml-0.5 animate-pulse"></span>{/if}</p>
                {#if commentaryMode === 'cross-ref' && crossRefChips.length > 0}
                  <!-- Chip strip beneath cross-ref prose. One-click jump
                       into the bible reader at the cited chapter (and
                       verse, when one was parsed). Built incrementally
                       from the streamed text — chips appear as the AI
                       names refs. -->
                  <div class="mt-3 flex flex-wrap items-baseline gap-1.5">
                    <span class="text-[11px] uppercase tracking-wider text-dim font-medium">Jump to:</span>
                    {#each crossRefChips as r (r.bookCode + '|' + r.chapter + '|' + (r.verse ?? ''))}
                      <button
                        type="button"
                        onclick={() => gotoRef(r)}
                        class="text-[11px] px-2 py-0.5 rounded bg-mantle border border-surface1 text-subtext hover:border-primary hover:text-text font-mono"
                        title="Open {r.label} in the Bible reader"
                      >{r.label}</button>
                    {/each}
                  </div>
                {/if}
              {/if}
            </div>
          {/if}
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

        <div class="flex gap-2 mt-4 flex-wrap">
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
            <!-- Drop a 5-session review series on the calendar at 1/3/
                 7/14/30-day offsets. Visible only post-reveal because
                 scheduling before checking the drill conflates
                 "I'm learning this" with "I've shown I know this". -->
            <button
              onclick={() => drill && scheduleVerseReview(drill.verse)}
              disabled={schedulingReview || !drill}
              class="px-4 py-2 text-sm bg-surface0 border border-surface1 rounded hover:border-secondary text-subtext hover:text-text disabled:opacity-50"
              title="Schedule review on day 1, 3, 7, 14, and 30 from today"
            >{schedulingReview ? 'scheduling…' : 'Schedule review ↻'}</button>
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
      <!-- Filter input + Ask AI sibling. The input drives the existing
           local substring filter; Ask AI takes the same string and
           hands it to the semantic-search endpoint, which picks 1-3
           catalogue topics and returns their verses. Both surfaces
           share the input so a user who types a sentence-shaped query
           and gets no substring matches can pivot in one click. -->
      <div class="flex items-center gap-2 mb-3">
        <input
          bind:value={q}
          onkeydown={(e) => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); runSemanticSearch(); } }}
          placeholder="filter substring, or describe what you want and click Ask AI…"
          class="flex-1 px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
        />
        <button
          type="button"
          onclick={runSemanticSearch}
          disabled={!q.trim() || aiSearching}
          class="px-3 py-2 text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary hover:text-text disabled:opacity-50 flex-shrink-0"
          title="Find verses by meaning (Cmd-Enter) — AI maps your query to catalogue topics"
        >{aiSearching ? 'asking…' : 'Ask AI'}</button>
      </div>

      {#if aiSearchResults.length > 0 || aiSearchTopics.length > 0}
        <!-- AI matches block sits above the substring-filtered
             catalogue list so the user sees both. Topics chip strip
             surfaces what the model picked, so the user can refine
             into a single-topic view by clicking. -->
        <div class="mb-4 bg-surface0 border border-primary/40 rounded-lg p-3">
          <div class="flex items-baseline justify-between mb-2 gap-2">
            <h3 class="text-xs uppercase tracking-wider text-primary font-medium">AI matches for "{aiSearchQuery}"</h3>
            <button
              type="button"
              onclick={clearSemanticSearch}
              class="text-[11px] text-dim hover:text-text flex-shrink-0"
            >clear</button>
          </div>
          {#if aiSearchTopics.length > 0}
            <div class="flex flex-wrap gap-1 mb-2">
              {#each aiSearchTopics as t (t)}
                <button
                  type="button"
                  onclick={() => { clearSemanticSearch(); selectTopic(t); }}
                  class="text-[11px] px-2 py-0.5 rounded-full border border-primary/40 bg-mantle text-subtext hover:border-primary hover:text-text"
                  title="Focus catalogue list on this topic"
                >{t}</button>
              {/each}
            </div>
          {/if}
          {#if aiSearchResults.length > 0}
            <ul class="space-y-2">
              {#each aiSearchResults as v}
                <li>
                  <p class="text-sm text-text font-serif italic leading-relaxed">"{v.text}"</p>
                  {#if v.source}
                    <p class="text-xs text-subtext mt-0.5">— {v.source}</p>
                  {/if}
                </li>
              {/each}
            </ul>
          {/if}
        </div>
      {/if}

      {#if topics.length > 0}
        <!-- Topical chip strip — click a theme to scope the list to
             verses tagged with it. "All" clears the filter. Counts show
             at a glance which themes have the deepest coverage. -->
        <div class="flex flex-wrap gap-1.5 mb-4">
          <button
            type="button"
            onclick={() => selectTopic('')}
            class="text-xs px-2 py-1 rounded-full border transition-colors {activeTopic === '' ? 'bg-primary text-on-primary border-primary' : 'bg-mantle border-surface1 text-subtext hover:border-primary hover:text-text'}"
          >All <span class="opacity-70">{all.length === 0 ? '' : ''}</span></button>
          {#each topics as t (t.topic)}
            <button
              type="button"
              onclick={() => selectTopic(t.topic)}
              class="text-xs px-2 py-1 rounded-full border transition-colors {activeTopic === t.topic ? 'bg-primary text-on-primary border-primary' : 'bg-mantle border-surface1 text-subtext hover:border-primary hover:text-text'}"
              title="{t.count} verses"
            >{t.topic} <span class="opacity-70">{t.count}</span></button>
          {/each}
        </div>
      {/if}
      <ul class="divide-y divide-surface1 bg-surface0 border border-surface1 rounded-lg">
        {#each filteredAll as v}
          <li class="px-4 py-3 group">
            <p class="text-sm text-text font-serif italic leading-relaxed">"{v.text}"</p>
            <div class="flex items-baseline gap-2 mt-1.5 flex-wrap">
              {#if v.source}
                <p class="text-xs text-subtext">— {v.source}</p>
              {/if}
              {#if v.topics && v.topics.length > 0}
                <div class="flex flex-wrap gap-1">
                  {#each v.topics as tag (tag)}
                    <button
                      type="button"
                      onclick={() => selectTopic(tag)}
                      class="text-[10px] px-1.5 py-0.5 rounded bg-mantle border border-surface1 text-dim hover:border-primary hover:text-text"
                      title="Filter by {tag}"
                    >{tag}</button>
                  {/each}
                </div>
              {/if}
              <span class="flex-1"></span>
              <button
                type="button"
                onclick={() => useAsTodayVerse(v)}
                class="text-[11px] text-dim hover:text-primary opacity-0 group-hover:opacity-100 transition-opacity"
                title="Promote to verse view"
              >read →</button>
            </div>
          </li>
        {/each}
      </ul>
      {#if filteredAll.length === 0}
        <p class="text-sm text-dim italic mt-4 text-center">
          {activeTopic ? `No verses tagged "${activeTopic}".` : 'No matches.'}
        </p>
      {/if}
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

        <!-- Random passage controls — primary action up top -->
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

        <!-- Search box -->
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

          <!-- Translation + word-study toggles. Both are opt-in panels
               that ride on top of the chapter view. The components
               themselves handle the data-not-bundled case gracefully
               (TranslationDiff shows a "drop more JSONs alongside
               web.json" hint; TaggedVerse / WordStudy show a "run
               scripts/fetch-strongs.sh" hint). Buttons stay enabled
               either way so the user can discover the surface and
               find out how to unlock it. -->
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
            <!-- Word-study panel sits in-flow under the chapter so the
                 user keeps their place. The TaggedVerse component
                 renders the whole chapter with each word tappable;
                 onSelectStrong populates `selectedStrong`, which mounts
                 the lexicon card beneath. -->
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
      <BibleBookmarksMode
        active={mode === 'bookmarks'}
        onOpenBookmark={openBookmark}
      />
    {:else if mode === 'intentions'}
      <PrayerIntentionsMode
        active={mode === 'intentions'}
        bind:prayingCount
      />
    {/if}
  </div>
</div>

<!-- AgentRunPanel doesn't accept a pre-filled goal directly, but the
     panel's textarea is bound to a local state — we skip the goal
     input by writing aiGoal into the panel via the same mechanism the
     daily-note button uses. The agent reads the verse from the goal
     prompt in either case (devotional preset asks for "Verse: X / Source: Y"). -->
<AgentRunPanel bind:open={aiOpen} preset={devotionalPreset} initialGoal={aiGoal} />
