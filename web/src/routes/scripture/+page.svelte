<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { api, type Scripture } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import PageHeader from '$lib/components/PageHeader.svelte';

  // Three modes:
  //   read   — verse-of-the-day in big type, "another one" button,
  //            reflect-on-this saves a Devotionals/ note
  //   memo   — cloze-deletion drill: hide every Nth significant word,
  //            you fill them in and the page tells you accuracy.
  //            Tracks per-verse stats in localStorage so weak verses
  //            surface preferentially (SM-2-style spaced repetition,
  //            simplified — accuracy alone, not interval-based)
  //   browse — paginated full list, search, click-to-copy
  type Mode = 'read' | 'memo' | 'browse';

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

  onMount(load);

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
</script>

<div class="h-full overflow-y-auto">
  <div class="max-w-3xl mx-auto p-4 sm:p-6 lg:p-8">
    <PageHeader title="Scripture" subtitle="Verse of the day, memorization drill, full library" />

    <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-sm mb-6 w-fit">
      <button
        class="px-4 py-2 {mode === 'read' ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}"
        onclick={() => (mode = 'read')}
      >Read</button>
      <button
        class="px-4 py-2 {mode === 'memo' ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}"
        onclick={() => { mode = 'memo'; if (!drill) startDrill(); }}
      >Memorize</button>
      <button
        class="px-4 py-2 {mode === 'browse' ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}"
        onclick={() => (mode = 'browse')}
      >Browse <span class="text-[10px] opacity-70">{all.length}</span></button>
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
            class="px-4 py-2 text-sm bg-primary text-mantle rounded hover:opacity-90"
          >Reflect on this →</button>
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
              class="px-4 py-2 text-sm bg-primary text-mantle rounded disabled:opacity-50"
            >Check</button>
          {:else}
            <button
              onclick={startDrill}
              class="px-4 py-2 text-sm bg-primary text-mantle rounded"
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
    {:else}
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
    {/if}
  </div>
</div>
