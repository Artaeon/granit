<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { auth } from '$lib/stores/auth';
  import { api, type Vision, type Note, type AgentPreset } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import AgentRunPanel from '$lib/agents/AgentRunPanel.svelte';

  // /review is the weekly examination ritual — five questions, saved
  // to a markdown note in Reviews/YYYY-Www.md. The page deliberately
  // re-uses the notes API (createNote/getNote/putNote) rather than
  // adding a new Go package, because the right shape for "guided
  // questions saved as a journal entry" is a regular note with
  // structured headings; the user can read past reviews from /notes
  // without a special viewer, and the TUI gets parity for free.

  // Five-question shape, keyed by canonical slug. Order is the
  // order the user works through them. The labels + prompts are
  // the actual journaling cues, designed to follow the biblical
  // examen (consciousness exam) tradition: gratitude → wins →
  // setbacks → relationships → next week's one focus.
  const QUESTIONS: { slug: string; heading: string; prompt: string }[] = [
    {
      slug: 'vision',
      heading: 'Vision check',
      prompt: 'Did this week move me toward my season focus? Yes, no, partially — and why?'
    },
    {
      slug: 'wins',
      heading: 'Wins',
      prompt: 'What went well? Where did I see God at work — in my own life, in others, in circumstances?'
    },
    {
      slug: 'setbacks',
      heading: 'Setbacks',
      prompt: "What didn't go well? What did I learn? What do I need to repent of, ask forgiveness for, or simply let go?"
    },
    {
      slug: 'people',
      heading: 'People',
      prompt: 'Who do I owe a thank-you, an apology, or a prayer for? Who needs my time next week?'
    },
    {
      slug: 'one_thing',
      heading: "Next week's one thing",
      prompt: 'If I could only do one thing next week, what would it be? Concrete, finishable.'
    }
  ];

  // ── Week ID + folder ──────────────────────────────────────────────
  // ISO week numbering: weeks start Monday, week containing Jan 4 is
  // week 1. Mirrors the Go time package (which uses ISO 8601 weeks)
  // so a future TUI surface that uses time.Time.ISOWeek() agrees on
  // file paths.
  function isoWeek(d: Date): { year: number; week: number } {
    // Copy + set to Thursday of that week (ISO definition).
    const t = new Date(Date.UTC(d.getFullYear(), d.getMonth(), d.getDate()));
    const day = t.getUTCDay() || 7;
    t.setUTCDate(t.getUTCDate() + 4 - day);
    const yearStart = new Date(Date.UTC(t.getUTCFullYear(), 0, 1));
    const week = Math.ceil(((t.getTime() - yearStart.getTime()) / 86400000 + 1) / 7);
    return { year: t.getUTCFullYear(), week };
  }
  function weekId(d: Date): string {
    const { year, week } = isoWeek(d);
    return `${year}-W${String(week).padStart(2, '0')}`;
  }
  function reviewPath(d: Date): string {
    return `Reviews/${weekId(d)}.md`;
  }

  // ── State ──────────────────────────────────────────────────────────
  let cursor = $state(new Date()); // which week the user is reviewing
  let answers = $state<Record<string, string>>(
    Object.fromEntries(QUESTIONS.map((q) => [q.slug, '']))
  );
  let busy = $state(false);
  let isExisting = $state(false); // true when this week's review already exists on disk
  let lastSavedAt = $state<string>(''); // ISO from frontmatter or note mtime
  let pastReviews = $state<Note[]>([]);
  let vision = $state<Vision | null>(null);

  // Cursor labels — "this week" / "last week" / "N weeks ago" so
  // navigating back through reviews feels concrete.
  function cursorLabel(d: Date): string {
    const todayWk = isoWeek(new Date());
    const cursorWk = isoWeek(d);
    if (todayWk.year === cursorWk.year && todayWk.week === cursorWk.week) return 'This week';
    // Last week is +1 week behind today, regardless of year boundary,
    // so compute by days rather than week-arithmetic to avoid
    // off-by-one across year wraps.
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    const c = new Date(d);
    c.setHours(0, 0, 0, 0);
    const weeks = Math.round((today.getTime() - c.getTime()) / (7 * 86400000));
    if (weeks === 1) return 'Last week';
    if (weeks > 1) return `${weeks} weeks ago`;
    if (weeks === -1) return 'Next week';
    return `${-weeks} weeks ahead`;
  }

  // ── Markdown encode/decode ────────────────────────────────────────
  // Body shape:
  //
  //   ---
  //   type: weekly-review
  //   week_iso: YYYY-Www
  //   ---
  //   ## Vision check
  //   <answer>
  //
  //   ## Wins
  //   <answer>
  //
  //   ...
  //
  // Encoding is straightforward; decoding looks for `^## <heading>$`
  // and grabs everything until the next `## ` or EOF. Simple
  // line-walk parser, no markdown-AST dependency.
  function encodeBody(): string {
    const parts: string[] = [];
    for (const q of QUESTIONS) {
      const val = (answers[q.slug] ?? '').trim();
      parts.push(`## ${q.heading}`);
      parts.push(val ? val : '_(empty)_');
      parts.push('');
    }
    return parts.join('\n').trimEnd() + '\n';
  }
  function decodeBody(body: string) {
    const lines = body.split('\n');
    const next: Record<string, string> = {};
    let currentSlug: string | null = null;
    let buf: string[] = [];
    const flush = () => {
      if (currentSlug !== null) {
        const text = buf.join('\n').trim();
        next[currentSlug] = text === '_(empty)_' ? '' : text;
      }
      buf = [];
    };
    for (const line of lines) {
      const m = line.match(/^##\s+(.+?)\s*$/);
      if (m) {
        flush();
        const q = QUESTIONS.find((q) => q.heading === m[1].trim());
        currentSlug = q?.slug ?? null;
        continue;
      }
      if (currentSlug !== null) buf.push(line);
    }
    flush();
    answers = { ...answers, ...next };
  }

  // ── Load / save ───────────────────────────────────────────────────
  async function load() {
    if (!$auth) return;
    busy = true;
    try {
      // Try to fetch this week's review. The notes API 404s for
      // missing — we treat that as "fresh review, empty form" and
      // don't toast on it.
      try {
        const note = await api.getNote(reviewPath(cursor));
        decodeBody(note.body ?? '');
        isExisting = true;
        lastSavedAt = note.modTime ?? '';
      } catch (e) {
        // 404 → reset to empty answers for the cursored week
        answers = Object.fromEntries(QUESTIONS.map((q) => [q.slug, '']));
        isExisting = false;
        lastSavedAt = '';
      }
      // Past reviews list — most recent 12, excluding the cursored
      // one. listNotes scoped to Reviews/ keeps the list tight.
      const list = await api.listNotes({ folder: 'Reviews', limit: 12 });
      pastReviews = list.notes.filter((n) => n.path !== reviewPath(cursor));
      // Pull vision so the page can show the user's current season
      // focus inline — pre-fills context for the vision-check
      // question without forcing the user to alt-tab.
      try {
        vision = await api.getVision();
      } catch {
        vision = null;
      }
    } finally {
      busy = false;
    }
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' && ev.path?.startsWith('Reviews/')) load();
      if (ev.type === 'note.removed' && ev.path?.startsWith('Reviews/')) load();
    });
  });

  // Re-load whenever the cursor changes (user navigates back/forward
  // through past weeks).
  $effect(() => {
    void cursor;
    load();
  });

  async function save() {
    if (!$auth || busy) return;
    busy = true;
    try {
      const path = reviewPath(cursor);
      const wId = weekId(cursor);
      const body = encodeBody();
      if (isExisting) {
        await api.putNote(path, {
          frontmatter: { type: 'weekly-review', week_iso: wId },
          body
        });
      } else {
        await api.createNote({
          path,
          frontmatter: { type: 'weekly-review', week_iso: wId },
          body
        });
        isExisting = true;
      }
      lastSavedAt = new Date().toISOString();
      toast.success('review saved');
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      busy = false;
    }
  }

  function prevWeek() {
    const d = new Date(cursor);
    d.setDate(d.getDate() - 7);
    cursor = d;
  }
  function nextWeek() {
    const d = new Date(cursor);
    d.setDate(d.getDate() + 7);
    cursor = d;
  }
  function thisWeek() {
    cursor = new Date();
  }

  // ── AI draft ──────────────────────────────────────────────────────
  // Opens the weekly-review-draft preset in the AgentRunPanel with
  // the user's vision + week ID pre-filled into the goal. The preset
  // refuses to overwrite an existing review, so calling it on a
  // populated week is a safe no-op (the agent's Final Answer tells
  // the user). After a successful run, the WS subscription on
  // Reviews/ already handles the refetch — the form populates with
  // the agent's draft automatically.
  let aiOpen = $state(false);
  let weeklyReviewPreset = $state<AgentPreset | null>(null);
  let aiGoal = $state('');
  let aiBusy = $state(false);

  async function openAIDraft() {
    if (!$auth || aiBusy) return;
    if (isExisting && Object.values(answers).some((v) => v.trim())) {
      if (!confirm("This week's review already has answers. The AI draft will refuse to overwrite — open the run panel anyway?")) return;
    }
    aiBusy = true;
    try {
      // Lazy-load the preset on first use, cache afterwards. Saves
      // a /agents/presets round-trip on subsequent clicks.
      if (!weeklyReviewPreset) {
        const r = await api.listAgentPresets();
        weeklyReviewPreset = r.presets.find((p) => p.id === 'weekly-review-draft') ?? null;
      }
      if (!weeklyReviewPreset) {
        toast.error('weekly-review-draft preset not found');
        return;
      }
      // Pre-fill goal with everything the agent needs without a
      // tool call: the ISO week, the vision context, and the daily-
      // notes folder if it's anything other than the default.
      const wId = weekId(cursor);
      const lines = [
        `Draft this week's review.`,
        `ISO week: ${wId}`,
      ];
      if (vision?.mission) lines.push(`Mission: ${vision.mission}`);
      if (vision?.season_focus) {
        const day = vision.season_day ? ` (day ${vision.season_day} of ${vision.season_total ?? 90})` : '';
        lines.push(`Season focus: ${vision.season_focus}${day}`);
      }
      if (vision?.values && vision.values.length > 0) {
        lines.push(`Core values: ${vision.values.join(', ')}`);
      }
      aiGoal = lines.join('\n');
      aiOpen = true;
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      aiBusy = false;
    }
  }

  // Friday/Sunday hint — surfaces a small note when it's actually a
  // good day to do the review. Doesn't gate anything; just nudges.
  let dayHint = $derived.by(() => {
    const d = new Date().getDay(); // 0=Sun, 5=Fri, 6=Sat
    if (d === 5) return 'Friday — a good day to look back on the week.';
    if (d === 0) return "Sunday — sabbath review fits well today.";
    return '';
  });

  // Past-review heuristics for the list — does the file have all five
  // sections filled? Helps the user spot weeks they only half-did.
  function reviewSummary(n: Note): { complete: boolean; preview: string } {
    const body = n.body ?? '';
    let filled = 0;
    let firstFill = '';
    for (const q of QUESTIONS) {
      const re = new RegExp(`^##\\s+${q.heading.replace(/[.*+?^${}()|[\\]\\\\]/g, '\\\\$&')}\\s*$([\\s\\S]*?)(?=\\n##\\s+|$)`, 'm');
      const m = body.match(re);
      if (m) {
        const text = m[1].trim();
        if (text && text !== '_(empty)_') {
          filled++;
          if (!firstFill) firstFill = text.split('\n')[0].slice(0, 80);
        }
      }
    }
    return {
      complete: filled === QUESTIONS.length,
      preview: firstFill || '(empty)'
    };
  }
</script>

<div class="h-full overflow-y-auto">
  <div class="max-w-2xl mx-auto p-4 sm:p-6 lg:p-8">
    <PageHeader title="Weekly review" subtitle="Five questions to close the loop between intention and execution" />

    <!-- Week navigator. Prev / this-week / next + cursor label. -->
    <div class="flex items-center gap-2 mb-3">
      <button onclick={prevWeek} class="text-xs px-2 py-1 rounded bg-surface0 border border-surface1 hover:border-primary text-subtext" aria-label="previous week">‹ prev</button>
      <button onclick={thisWeek} class="text-xs px-2 py-1 rounded bg-surface0 border border-surface1 hover:border-primary text-subtext">this week</button>
      <button onclick={nextWeek} class="text-xs px-2 py-1 rounded bg-surface0 border border-surface1 hover:border-primary text-subtext" aria-label="next week">next ›</button>
      <span class="flex-1"></span>
      <span class="text-xs text-dim font-mono">{weekId(cursor)} · {cursorLabel(cursor)}</span>
    </div>

    {#if dayHint && cursorLabel(cursor) === 'This week'}
      <p class="text-[11px] text-primary italic mb-4">{dayHint}</p>
    {/if}

    <!-- Vision context strip — shows the user's current season focus
         so the vision-check question doesn't require alt-tabbing. -->
    {#if vision?.season_focus}
      <div class="mb-5 px-3 py-2 bg-surface0/40 border border-surface1 rounded text-xs text-subtext">
        <span class="text-dim uppercase tracking-wider">Your season focus:</span>
        <span class="text-text font-medium ml-1">{vision.season_focus}</span>
        {#if vision.season_day && vision.season_total}
          <span class="text-dim ml-2">· day {vision.season_day} of {vision.season_total}</span>
        {/if}
      </div>
    {/if}

    {#if busy && !isExisting}
      <p class="text-sm text-dim">loading…</p>
    {:else}
      <form onsubmit={(e) => { e.preventDefault(); save(); }} class="space-y-6">
        {#each QUESTIONS as q}
          <section>
            <label for="rv-{q.slug}" class="block text-xs uppercase tracking-wider text-dim mb-1">{q.heading}</label>
            <p class="text-[11px] text-subtext italic mb-2">{q.prompt}</p>
            <textarea
              id="rv-{q.slug}"
              bind:value={answers[q.slug]}
              rows="4"
              class="w-full bg-surface0 border border-surface1 rounded px-3 py-2 text-sm text-text placeholder-dim focus:outline-none focus:border-primary resize-y"
            ></textarea>
          </section>
        {/each}

        <div class="flex items-center gap-3 pt-2 flex-wrap">
          {#if isExisting && lastSavedAt}
            <span class="text-[11px] text-dim">last saved {new Date(lastSavedAt).toLocaleString()}</span>
          {/if}
          <span class="flex-1"></span>
          <button
            type="button"
            onclick={openAIDraft}
            disabled={aiBusy}
            class="text-xs px-3 py-1.5 rounded bg-surface0 border border-surface1 text-subtext hover:border-primary disabled:opacity-50"
            title="Generate a draft from this week's jots + completed tasks"
          >{aiBusy ? '…' : '✨ AI draft'}</button>
          <button
            type="button"
            onclick={() => goto(`/notes/${encodeURIComponent(reviewPath(cursor))}`)}
            class="text-xs text-dim hover:text-text"
            disabled={!isExisting}
          >open as note ↗</button>
          <button
            type="submit"
            disabled={busy}
            class="text-sm px-4 py-2 rounded bg-primary text-on-primary font-medium hover:opacity-90 disabled:opacity-50"
          >{busy ? '…' : isExisting ? 'Update review' : 'Save review'}</button>
        </div>
      </form>
    {/if}

    <!-- Past reviews. Surfaces complete-vs-partial badges so the user
         can spot weeks they only half-finished. Click → cursor jumps
         to that week. -->
    {#if pastReviews.length > 0}
      <section class="mt-10 pt-6 border-t border-surface1">
        <h3 class="text-xs uppercase tracking-wider text-dim mb-3">Past reviews</h3>
        <ul class="space-y-1.5">
          {#each pastReviews as n (n.path)}
            {@const summary = reviewSummary(n)}
            {@const wkSlug = n.path.match(/(\d{4}-W\d{2})\.md$/)?.[1] ?? n.title}
            <li class="flex items-baseline gap-3 px-2 py-1.5 hover:bg-surface0 rounded">
              <button
                type="button"
                onclick={() => {
                  // Reverse-engineer a Date from the YYYY-Www slug.
                  // Pick Thursday of that ISO week — the canonical
                  // anchor day. Approximation: year-Jan-4 + (week-1)*7 days,
                  // then snap to Thursday.
                  const m = wkSlug.match(/^(\d{4})-W(\d{2})$/);
                  if (!m) return;
                  const year = parseInt(m[1], 10);
                  const week = parseInt(m[2], 10);
                  const jan4 = new Date(Date.UTC(year, 0, 4));
                  const dayOfJan4 = jan4.getUTCDay() || 7;
                  const mondayOfWeek1 = new Date(jan4);
                  mondayOfWeek1.setUTCDate(jan4.getUTCDate() - dayOfJan4 + 1);
                  const target = new Date(mondayOfWeek1);
                  target.setUTCDate(mondayOfWeek1.getUTCDate() + (week - 1) * 7 + 3);
                  cursor = target;
                }}
                class="font-mono text-xs text-secondary hover:underline w-20 text-left"
              >{wkSlug}</button>
              <span class="text-sm text-text flex-1 min-w-0 truncate">{summary.preview}</span>
              {#if summary.complete}
                <span class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-success/15 text-success">complete</span>
              {:else}
                <span class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-warning/15 text-warning">partial</span>
              {/if}
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    <p class="text-[11px] text-dim italic mt-10">
      Saved as <code>Reviews/{weekId(cursor)}.md</code> with frontmatter <code>type: weekly-review</code> — same files the granit TUI reads.
    </p>
  </div>
</div>

<!-- AI draft run panel. Opens with the weekly-review-draft preset
     pre-filled to write to Reviews/<this-week>.md. The preset
     refuses to overwrite an existing review with non-empty answers,
     so this is safe to invoke on a half-filled week. After a run
     completes the WS subscription on Reviews/ above re-fetches the
     note and the form populates automatically. -->
<AgentRunPanel bind:open={aiOpen} preset={weeklyReviewPreset} initialGoal={aiGoal} />
