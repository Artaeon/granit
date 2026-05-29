<script lang="ts">
  // /scripture — verse-of-the-day surface + memorization drill +
  // catalogue browser + full WEB bible + bookmarks + prayer
  // intentions. Stream Z slimmed this page from 1896 LOC down to
  // ~500 by:
  //   - replacing the two-row PageHeader + 6-tab pill row with the
  //     slim ScripturePageHeader component (single 44px row)
  //   - extracting each mode (read/memo/browse/bible) into its own
  //     component (bookmarks/intentions were already extracted)
  //   - moving the AI-commentary genre lookup tables + ref parser
  //     into ScriptureReadMode (the only consumer)
  //
  // Contemplative carve-out reminder: scripture is not a /roots
  // gamified surface. No XP, no streak-as-performance, no scoring
  // appears anywhere in the page chrome. The StreakBadge that
  // survives is descriptive ("you've opened the bible N days in a
  // row") — a record, not a leaderboard.
  //
  // What the page itself still owns:
  //   - mode state + the bound `current` Scripture slot
  //   - the canonical catalogue (`all`, `topics`, `activeTopic`)
  //   - all CREATE / WRITE actions that span modes:
  //       anotherOne / reflectOnThis / aiReflect / copy /
  //       saveToTodaysJot / scheduleVerseReview /
  //       bookmarkPassage / bookmarkVerse / reflectOnPassage /
  //       aiReflectOnPassage
  //   - the AgentRunPanel (one panel for the page, shared across
  //     modes)
  //   - cross-mode handoffs:
  //       Read topic-tap → Browse scoped to topic
  //       Read cross-ref-tap → Bible at chapter+verse
  //       Bookmarks open-bookmark → Bible at chapter+verse
  //       Browse / Bible "use as today's verse" → Read
  import { onMount, tick } from 'svelte';
  import { goto } from '$app/navigation';
  import {
    api,
    type Scripture,
    type ScriptureTopic,
    type BiblePassage,
    type BibleVerse,
    type BibleBookmark
  } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import StreakBadge from '$lib/notes/StreakBadge.svelte';
  import AgentRunPanel from '$lib/agents/AgentRunPanel.svelte';
  import type { AgentPreset } from '$lib/api';
  import ScripturePageHeader from '$lib/scripture/ScripturePageHeader.svelte';
  import ScriptureReadMode from '$lib/scripture/ScriptureReadMode.svelte';
  import ScriptureMemoMode from '$lib/scripture/ScriptureMemoMode.svelte';
  import ScriptureBrowseMode from '$lib/scripture/ScriptureBrowseMode.svelte';
  import ScriptureBibleMode from '$lib/scripture/ScriptureBibleMode.svelte';
  import BibleBookmarksMode from '$lib/scripture/BibleBookmarksMode.svelte';
  import PrayerIntentionsMode from '$lib/scripture/PrayerIntentionsMode.svelte';

  type Mode = 'read' | 'memo' | 'browse' | 'bible' | 'bookmarks' | 'intentions';

  let mode = $state<Mode>('read');
  let today = $state<Scripture | null>(null);
  let current = $state<Scripture | null>(null); // verse currently being viewed
  let all = $state<Scripture[]>([]);
  let topics = $state<ScriptureTopic[]>([]);
  let activeTopic = $state<string>(''); // active theme filter; '' = no filter
  let loading = $state(false);

  // Bible component handle — used so cross-ref / open-bookmark can
  // drive into a chapter from outside the bible mode. We bind:this
  // and call .openAt() after a tick so the component is mounted.
  let bibleModeRef = $state<ScriptureBibleMode | undefined>(undefined);
  let memoModeRef = $state<ScriptureMemoMode | undefined>(undefined);

  async function load() {
    loading = true;
    // Promise.allSettled so a 500 on listScriptures doesn't kill the
    // VotD surface (or vice versa) — independent reads, independent
    // failures.
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

  // Switch the active topic filter. '' clears it. The server's
  // case-insensitive match is the source of truth so we re-pull
  // instead of filtering locally.
  async function selectTopic(topic: string) {
    activeTopic = topic;
    try {
      const list = await api.listScriptures(topic || undefined);
      all = list.scriptures;
      topics = list.topics ?? topics;
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // Read mode topic-tap → flip into Browse scoped to the topic.
  async function onTopicTap(topic: string) {
    mode = 'browse';
    await selectTopic(topic);
  }

  // Read mode cross-ref-tap → flip into Bible at chapter+verse. The
  // Bible component lazy-mounts; we await a tick so the bind:this
  // ref is populated before calling .openAt().
  async function onCrossRef(bookCode: string, chapter: number, verse?: number) {
    mode = 'bible';
    await tick();
    if (bibleModeRef) {
      await bibleModeRef.openAt(bookCode, chapter, verse);
    }
  }

  // Prayer count surfaces up from the PrayerIntentionsMode child
  // so the header tab badge stays live.
  let prayingCount = $state(0);

  onMount(() => {
    load();
    // Reading-habit streak: fire-and-forget. Idempotent server-side
    // (RecordRead dedupes by date). Silent on failure — a streak
    // hiccup must never block scripture reading.
    void api.recordBibleRead().catch(() => {});
    return () => {};
  });

  // ── Verse rotation + AI reflection (shared across modes) ─────────

  async function anotherOne() {
    try {
      current = await api.randomScripture();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // Devotional creator — saves a fresh Devotionals/ note pre-seeded
  // with the verse and navigates to it for editing.
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

  // AI reflection — pops the devotional preset's run panel with the
  // verse pre-filled. The agent writes 200-300 words into
  // Devotionals/{date}-{slug}.md.
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

  // Copy verse + citation to clipboard. Falls back to a toast if
  // the Clipboard API isn't available (non-secure / old browsers).
  async function copyVerseToClipboard(s: Scripture) {
    const out = s.source ? `"${s.text}" — ${s.source}` : `"${s.text}"`;
    try {
      await navigator.clipboard.writeText(out);
      toast.success('copied');
    } catch {
      toast.error('clipboard unavailable');
    }
  }

  // Append verse to today's jot as a markdown blockquote. Distinct
  // from "Reflect on this" — that creates a fresh Devotionals/
  // note, this rides alongside the rest of today's journaling.
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
        undefined
      );
      toast.success('added to today\'s jot');
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // ── Spaced-repetition review scheduling ──────────────────────────
  // After a memo drill the user can schedule a 5-step review series
  // on the calendar: day 1, 3, 7, 14, 30 from today. Each step is a
  // scheduled task anchored to today's daily note (the task store
  // requires a real notePath) at 09:00, with the verse text in the
  // body so the calendar hover already shows what to recite.
  const REVIEW_INTERVALS = [1, 3, 7, 14, 30];

  async function scheduleVerseReview(verse: Scripture) {
    try {
      const todayNote = await api.daily('today');
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
            notePath: todayNote.path,
            text: `Review verse · ${label} (day ${days})\n${verseBody}`,
            scheduledStart: at.toISOString(),
            durationMinutes: 5,
            tags: ['memory-verse']
          });
          scheduled++;
        } catch (e) {
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
    }
  }

  // ── Bookmark create (driven from Bible reader buttons) ───────────
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
      // No explicit reload — BibleBookmarksMode listens to the WS
      // state.changed broadcast on the bookmark file.
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

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

  // Bookmark → Bible chapter at first verse. Same pattern as the
  // cross-ref handoff: switch mode, await tick, call openAt.
  async function openBookmark(b: BibleBookmark) {
    mode = 'bible';
    await tick();
    if (bibleModeRef) {
      await bibleModeRef.openAt(b.bookCode, b.chapter, b.verseFrom);
    }
  }

  function biblePassageToScripture(p: BiblePassage): Scripture {
    return {
      text: p.verses.map((v) => v.text).join(' '),
      source: `${p.reference} (WEB)`
    };
  }

  function useAsTodayVerse(s: Scripture) {
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

  // Header quick-capture: re-roll the current mode's primary
  // action. Read → another verse, Memo → next drill, Browse →
  // promote VotD back into Read, Bible → random passage, others
  // → no-op (the mode's own content is the action).
  function onQuickCapture() {
    if (mode === 'read') {
      void anotherOne();
    } else if (mode === 'memo') {
      memoModeRef?.nextDrill();
    } else if (mode === 'browse') {
      mode = 'read';
      void anotherOne();
    } else if (mode === 'bible') {
      bibleModeRef?.rerollRandom();
    }
  }
</script>

<div class="flex flex-col h-full overflow-hidden">
  <ScripturePageHeader
    {mode}
    catalogueCount={all.length}
    {prayingCount}
    onSelectMode={(m) => (mode = m)}
    {onQuickCapture}
  />

  <div class="flex-1 overflow-y-auto">
    <div class="max-w-3xl mx-auto p-4 sm:p-6 lg:p-8">
      <!-- Reading-habit streak badge. Vault-backed (server-side, durable
           across devices). Sits on the page surface, not in the chrome,
           because it's descriptive — a record of opening the bible, not
           a score competing with other surfaces. -->
      <div class="flex justify-end mb-3">
        <StreakBadge source="bible" />
      </div>

      {#if loading && all.length === 0}
        <div class="text-sm text-dim">loading…</div>
      {:else if mode === 'read'}
        {#if current}
          <ScriptureReadMode
            {current}
            {today}
            {aiLoading}
            onAnother={anotherOne}
            onCopy={copyVerseToClipboard}
            onSaveToJot={saveToTodaysJot}
            onReflect={reflectOnThis}
            onAIReflect={aiReflect}
            {onTopicTap}
            {onCrossRef}
          />
        {/if}
      {:else if mode === 'memo'}
        <ScriptureMemoMode
          bind:this={memoModeRef}
          {all}
          onScheduleReview={scheduleVerseReview}
        />
      {:else if mode === 'browse'}
        <ScriptureBrowseMode
          {all}
          {topics}
          {activeTopic}
          onSelectTopic={selectTopic}
          onUseAsTodayVerse={useAsTodayVerse}
        />
      {:else if mode === 'bible'}
        <ScriptureBibleMode
          bind:this={bibleModeRef}
          {aiLoading}
          onUseAsTodayVerse={useAsTodayVerse}
          onBookmarkPassage={bookmarkPassage}
          onBookmarkVerse={bookmarkVerse}
          onReflectPassage={reflectOnPassage}
          onAIReflectPassage={aiReflectOnPassage}
        />
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
</div>

<!-- AgentRunPanel — single instance for the page, shared across
     modes. Reflect/AI-reflect actions from Read and Bible both
     pop this panel with the right preset + pre-filled goal. -->
<AgentRunPanel bind:open={aiOpen} preset={devotionalPreset} initialGoal={aiGoal} />
