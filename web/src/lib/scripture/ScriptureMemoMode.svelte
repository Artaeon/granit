<!--
  ScriptureMemoMode — cloze-deletion memorization drill that used to
  live inline in /scripture as the {:else if mode === 'memo'} branch.

  Algorithm: hide ~25% of the significant (non-filler) words in the
  verse; the user fills in the blanks; check tells them which ones
  matched. Per-verse accuracy is stored in localStorage and used to
  weight pickWeak() so weak verses surface more often. This is
  spaced-repetition lite — accuracy alone, not interval-based.

  Contemplative carve-out reminder: this surface tracks accuracy
  (the only honest signal for memorization quality) but never
  exposes XP, streaks-as-performance, or "level up" framing. The
  accuracy ratio is descriptive feedback, not gamification.

  The optional "Schedule review" button drops a 5-session task
  series onto the user's calendar (day 1/3/7/14/30) — opt-in,
  visible only post-reveal so it doesn't conflate "I'm learning"
  with "I've shown I know it".
-->
<script lang="ts">
  import { type Scripture } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { loadStored, saveStored } from '$lib/util/storage';

  let {
    all,
    onScheduleReview
  }: {
    all: Scripture[];
    onScheduleReview: (verse: Scripture) => Promise<void>;
  } = $props();

  type Stats = Record<string, { tries: number; correct: number }>;
  const STATS_KEY = 'granit.scripture.stats';
  function loadStats(): Stats { return loadStored<Stats>(STATS_KEY, {}); }
  function saveStats(s: Stats) { saveStored(STATS_KEY, s); }

  let drill = $state<{ verse: Scripture; words: string[]; hidden: Set<number>; guesses: Record<number, string> } | null>(null);
  let revealed = $state(false);
  let schedulingReview = $state(false);

  // Pick a verse weighted by inverse accuracy. Never-practiced
  // verses get full weight; mastered ones still keep a 0.1 floor
  // so they can resurface occasionally.
  function pickWeak(): Scripture | null {
    if (all.length === 0) return null;
    const stats = loadStats();
    const weights = all.map((v) => {
      const s = stats[v.source ?? v.text];
      if (!s || s.tries === 0) return 1.0;
      const ratio = s.correct / s.tries;
      return Math.max(0.1, 1.0 - ratio);
    });
    const total = weights.reduce((a, b) => a + b, 0);
    let r = Math.random() * total;
    for (let i = 0; i < weights.length; i++) {
      r -= weights[i];
      if (r <= 0) return all[i];
    }
    return all[all.length - 1];
  }

  // Build a cloze drill. Skip-words list is intentionally tiny —
  // we want anchors but not a baby-game. ~25% target with floor 2
  // / ceiling 8 keeps even short verses meaningful and long
  // psalms manageable.
  function startDrill() {
    const v = pickWeak();
    if (!v) return;
    const words = v.text.split(/(\s+)/);
    const significantIdx: number[] = [];
    const skip = new Set(['a', 'an', 'the', 'to', 'of', 'in', 'on', 'and', 'or', 'but', 'is', 'it']);
    for (let i = 0; i < words.length; i++) {
      const w = words[i].replace(/[^\p{L}']/gu, '').toLowerCase();
      if (w && !skip.has(w) && w.length > 2) significantIdx.push(i);
    }
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

  // Auto-start the first drill when the user lands on memo mode
  // with content available — saves a click on the empty state.
  $effect(() => {
    if (all.length > 0 && !drill) {
      startDrill();
    }
  });

  let totalTries = $derived.by(() =>
    Object.values(loadStats()).reduce((sum, x) => sum + x.tries, 0)
  );
  let totalCorrect = $derived.by(() =>
    Object.values(loadStats()).reduce((sum, x) => sum + x.correct, 0)
  );

  async function handleScheduleReview() {
    if (!drill || schedulingReview) return;
    schedulingReview = true;
    try {
      await onScheduleReview(drill.verse);
    } finally {
      schedulingReview = false;
    }
  }

  // Exposed so the parent's "Next" header-button can trigger a
  // re-roll without reaching into private state.
  export function nextDrill() {
    startDrill();
  }
</script>

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
      <!-- Drop a 5-session review series onto the calendar at 1/3/
           7/14/30-day offsets. Visible only post-reveal so we don't
           conflate "I'm learning this" with "I've shown I know this". -->
      <button
        onclick={handleScheduleReview}
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
