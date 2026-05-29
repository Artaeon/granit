<!--
  ScriptureReadMode — the contemplative "verse of the day" surface
  that used to live inline in /scripture as the {:else if mode === 'read'}
  branch. Carries:
    - the big verse card (with per-device A/A/A type-size control)
    - the action row (another / copy / save-to-jot / reflect / AI)
    - the AI commentary strip (5 lenses: context / historical /
      cross-ref / application / genre) with streamed gloss + chip
      strip for cross-references

  Contemplative carve-out: this is the calmest surface in the whole
  page. No XP, no score chrome — just the verse, the citation, and
  the optional AI lenses below.

  State boundary: the parent owns `current` (the actively-displayed
  scripture) and `mode` (the bound mode for the topic-chip handoff
  into Browse and cross-ref handoff into Bible). Everything else
  (commentary stream, type-size pref, ref parsing) lives here.
-->
<script lang="ts">
  import { untrack } from 'svelte';
  import { api, type Scripture } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { loadStoredString, saveStoredString } from '$lib/util/storage';

  type Mode = 'read' | 'memo' | 'browse' | 'bible' | 'bookmarks' | 'intentions';
  type CommentaryMode = 'context' | 'historical' | 'cross-ref' | 'application' | 'genre';

  // Parent props — `current` is the verse to display; `today` is the
  // server-curated VoTD so we can show the "rotates at midnight"
  // hint only when the user is on the canonical pick (a re-roll
  // breaks that contract). The handoff callbacks let the read view
  // pivot into Browse (topic-tap) or Bible (cross-ref-tap) without
  // owning either of those subsystems.
  let {
    current,
    today,
    aiLoading,
    onAnother,
    onCopy,
    onSaveToJot,
    onReflect,
    onAIReflect,
    onTopicTap,    // topic chip → Browse mode scoped to that topic
    onCrossRef    // cross-ref chip → Bible reader at chapter+verse
  }: {
    current: Scripture;
    today: Scripture | null;
    aiLoading: boolean;
    onAnother: () => void | Promise<void>;
    onCopy: (s: Scripture) => void | Promise<void>;
    onSaveToJot: (s: Scripture) => void | Promise<void>;
    onReflect: () => void | Promise<void>;
    onAIReflect: () => void | Promise<void>;
    onTopicTap: (topic: string) => void;
    onCrossRef: (bookCode: string, chapter: number, verse?: number) => void | Promise<void>;
  } = $props();

  // ── Type-size preference ─────────────────────────────────────────
  // Three-step A/A/A control. Persists per-device; a phone often
  // wants a different size than a desktop / wall display.
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
  let readVerseClass = $derived(
    readSize === 'sm'
      ? 'text-lg sm:text-xl leading-relaxed'
      : readSize === 'lg'
        ? 'text-2xl sm:text-3xl leading-loose'
        : 'text-xl sm:text-2xl leading-relaxed'
  );

  // ── AI commentary (short streamed gloss) ─────────────────────────
  let commentaryText = $state('');
  let commentaryMode = $state<CommentaryMode | null>(null);
  let commentaryStreaming = $state(false);
  let commentaryError = $state('');
  let commentaryAbort: AbortController | null = $state(null);

  // Bible-book genre buckets — used by the `genre` commentary mode
  // to pick a tailored prompt and by the chip strip to colour-code
  // refs. Same data the parent used to carry; lives here now since
  // commentary is the only consumer.
  type Genre = 'Torah' | 'History' | 'Wisdom' | 'Prophets' | 'Gospels' | 'Acts' | 'Epistles' | 'Apocalyptic' | 'Unknown';

  const BOOK_TO_USFM: Record<string, string> = {
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

  // Pauline corpus → genre prompt override for Paul-specific
  // command + indicative + audience framing.
  const PAULINE: ReadonlySet<string> = new Set([
    'ROM', '1CO', '2CO', 'GAL', 'EPH', 'PHP', 'COL',
    '1TH', '2TH', '1TI', '2TI', 'TIT', 'PHM'
  ]);

  function sourceToUSFM(source: string): string | null {
    if (!source) return null;
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

  // Parse canonical bible refs out of free-form prose for the
  // cross-ref chip strip. Walks backwards from the digit-tail so
  // mid-sentence captures ("See John 3:16") still recover the
  // right book token via one-word peel-and-retry.
  type ParsedRef = { label: string; bookCode: string; chapter: number; verse?: number };
  function parseRefs(text: string): ParsedRef[] {
    if (!text) return [];
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
      let code = BOOK_TO_USFM[normalizeRefKey(bookRaw)];
      let usedBook = bookRaw;
      if (!code) {
        const parts = bookRaw.split(/\s+/);
        if (parts.length >= 2) {
          const tail = parts.slice(1).join(' ');
          code = BOOK_TO_USFM[normalizeRefKey(tail)];
          if (code) usedBook = tail;
          else {
            const head = parts.slice(0, -1).join(' ');
            code = BOOK_TO_USFM[normalizeRefKey(head)];
            if (code) usedBook = head;
          }
        }
      }
      if (!code) continue;
      let label = `${usedBook.replace(/\.$/, '')} ${chapter}`;
      if (verse) label += `:${verse}${verseEndStr ? `-${verseEndStr}` : ''}`;
      const dedupKey = `${code}|${chapter}|${verse ?? ''}`;
      if (seen.has(dedupKey)) continue;
      seen.add(dedupKey);
      out.push({ label, bookCode: code, chapter, verse });
    }
    return out;
  }

  let crossRefChips = $derived.by(() => {
    if (commentaryMode !== 'cross-ref') return [];
    return parseRefs(commentaryText);
  });

  async function runCommentary(kind: CommentaryMode) {
    if (!current) return;
    // Cancel any in-flight stream so tokens from a previous lens
    // don't interleave with the new one.
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

  // Reset commentary whenever the verse changes — stale commentary
  // under a different verse is worse than no commentary. Tracked
  // via an identity-string key; the reset is wrapped in untrack so
  // state writes inside don't loop the effect.
  $effect(() => {
    const _key = current ? `${current.text}|${current.source ?? ''}` : '';
    untrack(() => {
      void _key;
      clearCommentary();
    });
  });

  function handleCrossRefClick(r: ParsedRef) {
    void onCrossRef(r.bookCode, r.chapter, r.verse);
  }

  // Expose toast as a no-op fallback if a parent passed nothing —
  // currently unused but kept here for future inline copy buttons.
  void toast;
</script>

<!-- Reading-size control — kept top-right of the verse card so it
     stays out of the visual centerline of the reading surface. -->
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
    <!-- Tag strip — click a topic to flip into Browse scoped to it. -->
    <div class="flex flex-wrap gap-1.5 justify-center mt-4">
      {#each current.topics as tag (tag)}
        <button
          type="button"
          onclick={() => onTopicTap(tag)}
          class="text-[11px] px-2 py-0.5 rounded-full bg-mantle border border-surface1 text-dim hover:border-primary hover:text-text"
          title="Browse verses tagged {tag}"
        >{tag}</button>
      {/each}
    </div>
  {/if}
</article>

<div class="flex gap-2 justify-center mt-4 flex-wrap">
  <button
    onclick={() => void onAnother()}
    class="px-4 py-2 text-sm bg-surface0 border border-surface1 rounded hover:border-primary"
  >Another verse</button>
  <button
    onclick={() => void onCopy(current)}
    class="px-4 py-2 text-sm bg-surface0 border border-surface1 rounded hover:border-primary"
    title="Copy verse + citation to clipboard"
  >Copy</button>
  <button
    onclick={() => void onSaveToJot(current)}
    class="px-4 py-2 text-sm bg-surface0 border border-surface1 rounded hover:border-primary"
    title="Append the verse to today's jot as a blockquote"
  >Save to today's jot</button>
  <button
    onclick={() => void onReflect()}
    class="px-4 py-2 text-sm bg-surface0 border border-surface1 rounded hover:border-primary"
  >Reflect on this →</button>
  <button
    onclick={() => void onAIReflect()}
    disabled={aiLoading}
    class="px-4 py-2 text-sm bg-primary text-on-primary rounded hover:opacity-90 disabled:opacity-50"
    title="AI writes a 200-300 word reflection into Devotionals/"
  >{aiLoading ? '…' : 'AI reflection ✨'}</button>
</div>

<!-- AI commentary strip — five lenses streamed in-page. Short and
     ephemeral; for a saved devotional the user falls through to
     Reflect / AI reflection above. -->
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
          <!-- Chip strip — built incrementally from the streaming
               text, one-click jump into the Bible reader. -->
          <div class="mt-3 flex flex-wrap items-baseline gap-1.5">
            <span class="text-[11px] uppercase tracking-wider text-dim font-medium">Jump to:</span>
            {#each crossRefChips as r (r.bookCode + '|' + r.chapter + '|' + (r.verse ?? ''))}
              <button
                type="button"
                onclick={() => handleCrossRefClick(r)}
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
