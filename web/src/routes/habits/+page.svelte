<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, type HabitInfo, type HabitsResponse , todayISO } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import Heatmap from '$lib/components/Heatmap.svelte';
  import { habitTargets, setHabitTarget } from '$lib/habits/targets';
  import { loadStoredString, saveStoredString } from '$lib/util/storage';

  // /habits — three view modes for the same data:
  //   • Today: large quick-tick cards, the morning/evening rhythm view
  //   • Week:  7-column grid showing the last seven days at a glance
  //   • List:  full 90-day GitHub-style heatmap with streak stats
  // Sort + insight (best day of week) work across all three views so
  // the user's preference sticks regardless of which lens they pick.

  type View = 'today' | 'week' | 'list' | 'heatmap';
  type Sort = 'streak' | 'completion' | 'alpha' | 'behind';

  // Persist per-device so a user who lives in Week view doesn't have
  // to toggle every time. List remains the default for first-time
  // users — the 90-day grid is the most informative entry point.
  const VIEW_KEY = 'granit.habits.view';
  const SORT_KEY = 'granit.habits.sort';

  let view = $state<View>(loadStoredString(VIEW_KEY, 'list') as View);
  let sortBy = $state<Sort>(loadStoredString(SORT_KEY, 'streak') as Sort);

  $effect(() => saveStoredString(VIEW_KEY, view));
  $effect(() => saveStoredString(SORT_KEY, sortBy));

  let data = $state<HabitsResponse | null>(null);
  let loading = $state(false);
  let busy = $state<string | null>(null);

  // ─── AI insight ──────────────────────────────────────────────────
  // Reads each habit's last 30 days and produces 2-3 short
  // observations: weekday patterns (consistent Wed misses), streak
  // risks (no-doomscrolling decayed Mon → Sun), or wins (longest
  // streak ever this week). Streamed via the audit-gated chat
  // pipeline, no separate Tier 1 endpoint — it's a chat with a
  // structured prompt, same as people / examen.
  let aiBusy = $state(false);
  let aiAbort: AbortController | null = null;
  let aiInsights = $state<string[]>([]);
  let aiError = $state('');

  function buildHabitsSeed(): string {
    if (!data) return '';
    const today = data.today;
    const habits = data.habits.slice(0, 12).map((h) => {
      // Compact day grid — a string of 30 chars where 1 = done,
      // 0 = missed, . = before-tracking. Cheaper than 30 objects
      // and the model reads patterns from it just fine.
      const grid = h.days.slice(-30).map((d) => (d.done ? '1' : '0')).join('');
      return {
        name: h.name,
        currentStreak: h.currentStreak,
        longestStreak: h.longestStreak,
        last7Pct: h.last7Pct,
        last30Pct: h.last30Pct,
        last30: grid,
        doneToday: h.doneToday
      };
    });
    return JSON.stringify({ today, habits }, null, 2);
  }

  async function aiInsight() {
    if (aiBusy || !data) return;
    aiAbort?.abort();
    aiAbort = new AbortController();
    aiBusy = true;
    aiError = '';
    aiInsights = [];
    const seed = buildHabitsSeed();
    const system = 'You analyse the user\'s habit data and surface 2-3 short, specific observations. Examples of good observations: a weekday that consistently fails ("you miss Wednesdays consistently this month"), a streak risk ("no-doomscrolling decayed from 80% week 1 to 30% week 4"), a win ("morning-movement is at the longest streak it\'s ever been"). Examples of BAD observations: generic praise, vague advice, "keep going!". Each observation on its own line. No preamble, no numbering, no bullets. Under 22 words each. The grid string is 30 days oldest-to-newest; 1 = done, 0 = missed.';
    const user = `Today is ${data.today}. Here are the habits:\n\n\`\`\`json\n${seed}\n\`\`\`\n\nGive me 2-3 sharp observations.`;
    let buf = '';
    try {
      await api.chatStream(
        [
          { role: 'system', content: system },
          { role: 'user', content: user }
        ],
        undefined,
        {
          onChunk: (c) => {
            buf += c;
            aiInsights = buf.split(/\n+/).map((l) => l.trim()).filter((l) => l.length > 0);
          },
          onDone: () => {},
          onError: (err) => { aiError = err.message; }
        },
        aiAbort.signal
      );
    } finally {
      aiBusy = false;
      aiAbort = null;
    }
  }
  function dismissInsight() {
    aiAbort?.abort();
    aiBusy = false;
    aiInsights = [];
    aiError = '';
  }

  // Add-habit-from-web. The existing toggleHabit endpoint already
  // auto-creates the `- [ ] habit` line when the supplied name
  // doesn't match anything in today's `## Habits` section (and
  // creates the section + minimal frontmatter when the daily note
  // doesn't exist yet). So "add a habit" is just a toggle call with
  // done=false on a fresh name.
  let addOpen = $state(false);
  let addName = $state('');
  let addBusy = $state(false);

  async function addHabit(e?: Event) {
    e?.preventDefault();
    const name = addName.trim();
    if (!name || addBusy) return;
    addBusy = true;
    try {
      // done=false for a fresh "track this" intent (the user hasn't
      // done it today yet, just wants the habit in the list).
      await api.toggleHabit(name, data?.today ?? todayISO(), false);
      addName = '';
      addOpen = false;
      await load();
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      (await import('$lib/components/toast')).toast.error(`couldn't add habit: ${msg}`);
    } finally {
      addBusy = false;
    }
  }

  async function load() {
    if (!$auth) return;
    loading = true;
    try {
      data = await api.listHabits();
    } finally {
      loading = false;
    }
  }
  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') load();
    });
  });

  async function toggleToday(h: HabitInfo) {
    await toggleOnDate(h, data?.today ?? todayISO(), !h.doneToday);
  }

  // Click-on-dot retro-toggle. Works for any date, including future
  // ones (so the user can plan-log) — server creates the daily file
  // for that date if it doesn't exist yet. The optimistic flip keeps
  // the UI snappy on a slow link; load() at the end reconciles.
  async function toggleOnDate(h: HabitInfo, date: string, want: boolean) {
    busy = `${h.name}|${date}`;
    if (data) {
      const habit = data.habits.find((x) => x.name === h.name);
      const day = habit?.days.find((d) => d.date === date);
      if (day) day.done = want;
      if (habit && date === data.today) habit.doneToday = want;
      data = { ...data };
    }
    try {
      await api.toggleHabit(h.name, date, want);
      await load();
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      (await import('$lib/components/toast')).toast.error(`couldn't toggle: ${msg}`);
      await load(); // restore truth
    } finally {
      busy = null;
    }
  }

  // ----- Sorting -----
  // Each sort key maps to a comparator. "behind" surfaces struggling
  // habits at the top so a Sunday review naturally shows what needs
  // attention without scrolling.
  let sortedHabits = $derived.by(() => {
    if (!data) return [] as HabitInfo[];
    const list = [...data.habits];
    switch (sortBy) {
      case 'streak':
        return list.sort((a, b) => b.currentStreak - a.currentStreak);
      case 'completion':
        return list.sort((a, b) => b.last30Pct - a.last30Pct);
      case 'behind':
        return list.sort((a, b) => a.last7Pct - b.last7Pct);
      case 'alpha':
        return list.sort((a, b) => a.name.localeCompare(b.name));
    }
  });

  // ----- Insight: best day of week -----
  // Group the 90-day window by weekday and compute per-day completion
  // percent. Returns the day-of-week with the highest pct, or null
  // when no day has any logged occurrences (a fresh habit). Pure
  // derivation — no extra API surface.
  const DOW_LABELS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
  function bestDay(h: HabitInfo): { label: string; pct: number } | null {
    const buckets = [0, 0, 0, 0, 0, 0, 0];
    const counts = [0, 0, 0, 0, 0, 0, 0];
    for (const d of h.days) {
      // YYYY-MM-DD parsed in local time. Using new Date(s) directly
      // would parse as UTC and shift the weekday by one for users
      // east of UTC; explicit local construction avoids that.
      const [y, m, day] = d.date.split('-').map(Number);
      const dow = new Date(y, m - 1, day).getDay();
      counts[dow]++;
      if (d.done) buckets[dow]++;
    }
    let bestDow = -1;
    let bestPct = 0;
    for (let i = 0; i < 7; i++) {
      if (counts[i] === 0) continue;
      const pct = buckets[i] / counts[i];
      if (pct > bestPct) {
        bestPct = pct;
        bestDow = i;
      }
    }
    if (bestDow === -1 || bestPct === 0) return null;
    return { label: DOW_LABELS[bestDow], pct: Math.round(bestPct * 100) };
  }

  // Habits remaining today — the Today header surfaces "X / Y done"
  // so the user reads progress at a glance.
  let todayDone = $derived(data ? data.habits.filter((h) => h.doneToday).length : 0);
  let todayTotal = $derived(data ? data.habits.length : 0);
  let undoneToday = $derived(data ? data.habits.filter((h) => !h.doneToday && h.taskIdToday) : []);

  // ----- Bulk tick all -----
  // Power-user shortcut for the morning rhythm: a single click ticks
  // every habit not yet done today. Only enabled when at least one
  // habit can be toggled (some require the daily note's `## Habits`
  // section to exist first — those are skipped). Optimistic flip on
  // each, then a single load() reconciles. Errors are toasted but
  // we keep going for the rest so a single bad row doesn't block the
  // bulk action.
  let bulkBusy = $state(false);
  async function tickAllToday() {
    if (!data || bulkBusy) return;
    const targets = data.habits.filter((h) => !h.doneToday && h.taskIdToday);
    if (targets.length === 0) return;
    bulkBusy = true;
    const today = data.today;
    // Optimistic: flip everything in one pass before the network round-trips
    for (const h of targets) {
      const habit = data.habits.find((x) => x.name === h.name);
      const day = habit?.days.find((d) => d.date === today);
      if (day) day.done = true;
      if (habit) habit.doneToday = true;
    }
    data = { ...data };
    const failed: string[] = [];
    await Promise.all(
      targets.map(async (h) => {
        try {
          await api.toggleHabit(h.name, today, true);
        } catch {
          failed.push(h.name);
        }
      })
    );
    bulkBusy = false;
    await load();
    if (failed.length > 0) {
      const { toast } = await import('$lib/components/toast');
      toast.error(`couldn't tick: ${failed.join(', ')}`);
    }
  }

  // ----- Per-habit weekly target -----
  // last 7 days done count; mapped against the user's target for a
  // simple "3/5 this week" chip. Targets live in localStorage so
  // changing one doesn't round-trip the server. Pure derivation.
  function last7Done(h: HabitInfo): number {
    return h.days.slice(-7).filter((d) => d.done).length;
  }
  function targetState(h: HabitInfo): { target: number; done: number; pct: number } | null {
    const target = $habitTargets[h.name];
    if (!target) return null;
    const done = last7Done(h);
    return { target, done, pct: Math.min(1, done / target) };
  }
  // Edit-target popover — single open at a time, name keys it.
  let editingTarget = $state<string | null>(null);
  function bumpTarget(name: string, delta: number) {
    const cur = $habitTargets[name] ?? 7;
    const next = Math.max(1, Math.min(7, cur + delta));
    setHabitTarget(name, next);
  }
  function clearTarget(name: string) {
    setHabitTarget(name, null);
    editingTarget = null;
  }

  // ----- Week view helpers -----
  // The server returns 90 days oldest→newest. We want the last 7 in
  // chronological order so columns read left=oldest right=today.
  function weekDays(h: HabitInfo): HabitInfo['days'] {
    return h.days.slice(-7);
  }
  function shortDow(date: string): string {
    const [y, m, d] = date.split('-').map(Number);
    return DOW_LABELS[new Date(y, m - 1, d).getDay()];
  }
  function shortDate(date: string): string {
    const [, m, d] = date.split('-').map(Number);
    return `${m}/${d}`;
  }
</script>

<div class="h-full overflow-y-auto">
  <div class="p-4 sm:p-6 lg:p-8 max-w-5xl mx-auto">
    <header class="mb-5 flex flex-col sm:flex-row sm:items-baseline sm:justify-between gap-3">
      <div class="min-w-0">
        <h1 class="text-2xl sm:text-3xl font-semibold text-text">Habits</h1>
        <p class="text-sm text-dim mt-1">
          derived from <code class="text-xs">## Habits</code> in each daily note
          {#if data}
            <span class="ml-1">· {todayDone}/{todayTotal} done today</span>
          {/if}
        </p>
      </div>
      <div class="flex items-center gap-2 self-start">
        {#if undoneToday.length > 0}
          <button
            type="button"
            onclick={tickAllToday}
            disabled={bulkBusy}
            title="mark every undone habit as done today"
            class="px-3 py-1.5 bg-surface0 text-success border border-success rounded text-sm font-medium hover:bg-surface1 disabled:opacity-50"
          >
            {bulkBusy ? '…' : `Tick all (${undoneToday.length})`}
          </button>
        {/if}
        <button
          type="button"
          onclick={() => (addOpen = !addOpen)}
          class="px-3 py-1.5 bg-primary text-on-primary rounded text-sm font-medium hover:opacity-90"
        >{addOpen ? 'cancel' : '+ Add habit'}</button>
      </div>
    </header>

    {#if addOpen}
      <!-- Add-habit form. Single field — name only. Adds an unticked
           checkbox to today's daily note's ## Habits section (the
           server creates the section / file if needed). The user can
           then toggle it done from the same page or set per-day
           later. Keeps capture friction at a single keystroke beyond
           "where do I click". -->
      <form onsubmit={addHabit} class="bg-surface0 border border-surface1 rounded-lg p-3 mb-4 flex flex-wrap gap-2 items-center">
        <input
          bind:value={addName}
          required
          autofocus
          placeholder="habit name (e.g. morning movement, no doomscrolling)…"
          class="flex-1 min-w-[12rem] px-3 py-2 bg-mantle border border-surface1 rounded text-base sm:text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
        />
        <button
          type="submit"
          disabled={!addName.trim() || addBusy}
          class="px-4 py-2 bg-primary text-on-primary rounded text-sm font-medium disabled:opacity-50"
        >{addBusy ? '…' : 'add to today'}</button>
      </form>
    {/if}

    {#if data && data.habits.length >= 2 && (aiInsights.length > 0 || aiBusy || aiError)}
      <section class="mb-4 p-3 bg-surface1 border border-surface2 rounded-lg">
        <div class="flex items-baseline gap-2 mb-2">
          <h3 class="text-xs uppercase tracking-wider text-primary font-medium">Pattern insight</h3>
          <span class="flex-1"></span>
          {#if aiBusy}
            <span class="text-[11px] text-dim italic">analyzing…</span>
          {:else}
            <button onclick={aiInsight} class="text-[11px] text-secondary hover:underline">regenerate</button>
          {/if}
          <button onclick={dismissInsight} class="text-[11px] text-dim hover:text-text">dismiss</button>
        </div>
        {#if aiError}
          <p class="text-xs text-error">{aiError}</p>
        {:else}
          <ul class="space-y-1">
            {#each aiInsights as line}
              {@const cleaned = line.replace(/^[-•*\d.\s]+/, '').trim()}
              {#if cleaned}
                <li class="text-sm text-text leading-relaxed">{cleaned}</li>
              {/if}
            {/each}
          </ul>
        {/if}
      </section>
    {/if}

    <!-- View + sort controls. Both rows wrap on narrow screens; the
         view toggle uses a segmented pill, the sort uses a select to
         keep horizontal space tight on phones. -->
    {#if data && data.habits.length > 0}
      <div class="flex flex-wrap items-center gap-2 mb-5">
        <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-sm">
          {#each [
            { v: 'today', label: 'Today' },
            { v: 'week', label: 'Week' },
            { v: 'list', label: 'List' },
            { v: 'heatmap', label: 'Heatmap' }
          ] as o}
            <button
              type="button"
              class="px-3 py-1.5 capitalize transition-colors {view === o.v ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
              onclick={() => (view = o.v as View)}
            >{o.label}</button>
          {/each}
        </div>
        <span class="flex-1"></span>
        {#if data && data.habits.length >= 2 && aiInsights.length === 0 && !aiBusy && !aiError}
          <button
            type="button"
            onclick={aiInsight}
            class="text-[11px] px-2 py-1 rounded inline-flex items-center gap-1 bg-surface1 text-primary border border-surface2 hover:bg-surface2"
            title="Ask AI for pattern observations"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-3 h-3">
              <path d="M12 3l1.2 4.2L17 9l-3.8 1.8L12 15l-1.2-4.2L7 9l3.8-1.8L12 3z" stroke-linejoin="round"/>
            </svg>
            Insight
          </button>
        {/if}
        <label class="text-xs text-dim flex items-center gap-1.5">
          sort
          <select
            bind:value={sortBy}
            class="text-xs bg-surface0 border border-surface1 rounded px-2 py-1 text-text"
            aria-label="sort habits"
          >
            <option value="streak">by streak</option>
            <option value="completion">by 30-day completion</option>
            <option value="behind">behind first</option>
            <option value="alpha">alphabetical</option>
          </select>
        </label>
      </div>
    {/if}

    {#if loading && !data}
      <div class="space-y-4">
        {#each Array(3) as _}
          <div class="bg-surface0 border border-surface1 rounded-lg p-3 space-y-3">
            <div class="flex items-start gap-3">
              <Skeleton class="h-6 w-6 rounded flex-shrink-0" />
              <div class="flex-1 space-y-2">
                <Skeleton class="h-5 w-1/2" />
                <Skeleton class="h-3 w-2/3" />
              </div>
            </div>
            <Skeleton class="h-12 w-full" />
          </div>
        {/each}
      </div>
    {:else if data && data.habits.length === 0}
      <div class="bg-surface0 border border-surface1 rounded-lg p-5 sm:p-6 leading-relaxed">
        <svg viewBox="0 0 24 24" class="w-10 h-10 mb-3 text-dim" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
          <path d="M9 11l3 3L22 4"/>
          <path d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11"/>
        </svg>
        <h2 class="text-base font-medium text-text">Track habits in your daily notes</h2>
        <p class="text-sm text-dim mt-2 max-w-prose">
          Add a <code class="text-xs">## Habits</code> section to any daily note with checkbox lines.
          The web dashboard scans the last 90 days, computes streaks, and shows a dot grid like GitHub contributions.
        </p>
        <pre class="mt-4 p-3 bg-mantle rounded text-xs text-secondary font-mono overflow-x-auto">## Habits

- [ ] morning movement
- [ ] read 20 pages
- [ ] no doomscrolling</pre>
        <p class="mt-3 text-xs text-dim">
          The same checkboxes show up as tasks in the TUI — both views stay in sync via the markdown.
        </p>
      </div>
    {:else if data && view === 'today'}
      <!-- ===== TODAY VIEW ===== -->
      <!-- Large quick-tick cards focused on today. The big checkbox
           is the one the user wants to hit fast in the morning or
           evening — every other element is supporting context. -->
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
        {#each sortedHabits as h (h.name)}
          {@const insight = bestDay(h)}
          {@const tgt = targetState(h)}
          <div
            class="relative text-left p-4 bg-surface0 border rounded-lg transition-colors flex items-start gap-3
              {h.doneToday
                ? 'border-success bg-surface0'
                : 'border-surface1 hover:border-primary'}"
          >
            <button
              type="button"
              onclick={() => toggleToday(h)}
              disabled={busy === h.name || !h.taskIdToday}
              title={h.taskIdToday ? '' : 'add this habit to today\'s daily note first'}
              class="w-10 h-10 rounded flex-shrink-0 flex items-center justify-center transition-colors disabled:opacity-50
                {h.doneToday ? 'bg-success border-2 border-success' : 'border-2 border-surface2 hover:border-primary'}"
              aria-label="toggle today"
            >
              {#if h.doneToday}
                <svg viewBox="0 0 12 12" class="w-5 h-5 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
              {/if}
            </button>
            <div class="flex-1 min-w-0">
              <h2 class="text-base font-medium text-text break-words">{h.name}</h2>
              <div class="flex flex-wrap items-baseline gap-x-3 gap-y-0.5 text-xs text-dim mt-1">
                <span title="current streak">🔥 {h.currentStreak}d</span>
                <span title="last 7 days">7d: {h.last7Pct}%</span>
                <span title="last 30 days">30d: {h.last30Pct}%</span>
                {#if insight}
                  <span class="text-secondary" title="best day of week from last 90 days">
                    best: {insight.label} ({insight.pct}%)
                  </span>
                {/if}
                {#if tgt}
                  {@const hit = tgt.done >= tgt.target}
                  <button
                    type="button"
                    onclick={() => editingTarget = editingTarget === h.name ? null : h.name}
                    class="px-1.5 py-0.5 rounded text-[10px] uppercase tracking-wider border transition-colors
                      {hit
                        ? 'bg-surface0 text-success border-success hover:bg-surface1'
                        : 'bg-surface0 text-warning border-warning hover:bg-surface1'}"
                    title="weekly target — click to edit"
                  >🎯 {tgt.done}/{tgt.target}/wk</button>
                {:else}
                  <button
                    type="button"
                    onclick={() => editingTarget = editingTarget === h.name ? null : h.name}
                    class="px-1.5 py-0.5 rounded text-[10px] uppercase tracking-wider border bg-surface1 text-dim border-surface2 hover:text-text"
                    title="set a weekly target"
                  >+ target</button>
                {/if}
              </div>
              {#if editingTarget === h.name}
                <div class="mt-2 flex items-center gap-1.5 text-[11px]">
                  <span class="text-dim">target / week:</span>
                  <button class="px-1.5 py-0.5 bg-surface1 hover:bg-surface2 rounded text-text" onclick={() => bumpTarget(h.name, -1)}>−</button>
                  <span class="font-mono text-text w-4 text-center">{$habitTargets[h.name] ?? 5}</span>
                  <button class="px-1.5 py-0.5 bg-surface1 hover:bg-surface2 rounded text-text" onclick={() => bumpTarget(h.name, 1)}>+</button>
                  <button class="ml-1 px-1.5 py-0.5 text-dim hover:text-text underline" onclick={() => clearTarget(h.name)}>clear</button>
                  <button class="ml-auto px-1.5 py-0.5 text-dim hover:text-text" onclick={() => (editingTarget = null)}>done</button>
                </div>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    {:else if data && view === 'week'}
      <!-- ===== WEEK VIEW ===== -->
      <!-- 7-column grid: each row a habit, each column a day. Header
           row labels the day-of-week + date (M/D). Last column is
           today and gets a primary outline. Click any cell to toggle. -->
      {@const days = sortedHabits.length > 0 ? weekDays(sortedHabits[0]) : []}
      <!-- Mobile: the table doesn't fit in 7 columns at touch-target
           size, so it's allowed to scroll horizontally. min-w on the
           table forces 44px-ish day cells; the inner aspect-square
           keeps cells visually balanced once the layout has room. -->
      <div class="bg-surface0 border border-surface1 rounded-lg p-3 sm:p-4 overflow-x-auto">
        <table class="w-full border-separate border-spacing-1 min-w-[28rem]">
          <thead>
            <tr>
              <th class="w-32 sm:w-1/4 text-left text-[11px] uppercase tracking-wider text-dim font-medium pb-2">Habit</th>
              {#each days as d (d.date)}
                {@const isToday = d.date === data.today}
                <th
                  class="text-center text-[11px] font-medium pb-2 {isToday ? 'text-primary' : 'text-dim'}"
                >
                  <div>{shortDow(d.date)}</div>
                  <div class="font-mono text-[10px] mt-0.5">{shortDate(d.date)}</div>
                </th>
              {/each}
            </tr>
          </thead>
          <tbody>
            {#each sortedHabits as h (h.name)}
              <tr>
                <td class="text-sm text-text break-words py-1 pr-2">
                  {h.name}
                  <div class="text-[11px] text-dim">
                    🔥 {h.currentStreak}d · {h.last7Pct}% / 7d
                  </div>
                </td>
                {#each weekDays(h) as d (d.date)}
                  {@const isToday = d.date === data.today}
                  {@const cellBusy = busy === `${h.name}|${d.date}`}
                  <td class="p-0">
                    <button
                      type="button"
                      onclick={() => toggleOnDate(h, d.date, !d.done)}
                      disabled={cellBusy}
                      class="w-full aspect-square min-h-9 rounded transition-colors hover:opacity-80 disabled:opacity-40
                        {d.done ? 'bg-success' : 'bg-surface1 hover:bg-surface2'}"
                      class:ring-1={isToday}
                      class:ring-primary={isToday}
                      title={`${d.date}${d.done ? ' · done · click to undo' : ' · click to mark done'}`}
                      aria-label={`toggle ${h.name} on ${d.date}`}
                    ></button>
                  </td>
                {/each}
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else if data}
      <!-- ===== LIST VIEW ===== (was the only view before) -->
      <div class="space-y-4">
        {#each sortedHabits as h (h.name)}
          {@const insight = bestDay(h)}
          {@const tgt = targetState(h)}
          <article class="bg-surface0 border border-surface1 rounded-lg p-3">
            <div class="flex items-start gap-3 mb-3">
              <button
                onclick={() => toggleToday(h)}
                disabled={busy === h.name || !h.taskIdToday}
                title={h.taskIdToday ? (h.doneToday ? 'mark not done today' : 'mark done today') : 'open daily note to add this habit'}
                class="w-6 h-6 mt-0.5 rounded border flex-shrink-0 flex items-center justify-center transition-colors disabled:opacity-50
                  {h.doneToday ? 'bg-success border-success' : 'border-surface2 hover:border-primary'}"
                aria-label="toggle today"
              >
                {#if h.doneToday}
                  <svg viewBox="0 0 12 12" class="w-4 h-4 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
                {/if}
              </button>
              <div class="flex-1 min-w-0">
                <h2 class="text-base font-medium text-text break-words">{h.name}</h2>
                <div class="flex flex-wrap items-baseline gap-x-4 gap-y-0.5 text-xs text-dim mt-0.5">
                  <span>🔥 {h.currentStreak}-day streak</span>
                  <span>longest: {h.longestStreak}</span>
                  <span>last 7: {h.last7Pct}%</span>
                  <span>last 30: {h.last30Pct}%</span>
                  {#if insight}
                    <span class="text-secondary" title="best day of week from last 90 days">
                      best: {insight.label} ({insight.pct}%)
                    </span>
                  {/if}
                  {#if tgt}
                    {@const hit = tgt.done >= tgt.target}
                    <button
                      type="button"
                      onclick={() => editingTarget = editingTarget === h.name ? null : h.name}
                      class="px-1.5 py-0.5 rounded text-[10px] uppercase tracking-wider border transition-colors
                        {hit
                          ? 'bg-surface0 text-success border-success hover:bg-surface1'
                          : 'bg-surface0 text-warning border-warning hover:bg-surface1'}"
                      title="weekly target — click to edit"
                    >🎯 {tgt.done}/{tgt.target}/wk</button>
                  {:else}
                    <button
                      type="button"
                      onclick={() => editingTarget = editingTarget === h.name ? null : h.name}
                      class="px-1.5 py-0.5 rounded text-[10px] uppercase tracking-wider border bg-surface1 text-dim border-surface2 hover:text-text"
                    >+ target</button>
                  {/if}
                </div>
                {#if editingTarget === h.name}
                  <div class="mt-1.5 flex items-center gap-1.5 text-[11px]">
                    <span class="text-dim">target / week:</span>
                    <button class="px-1.5 py-0.5 bg-surface1 hover:bg-surface2 rounded text-text" onclick={() => bumpTarget(h.name, -1)}>−</button>
                    <span class="font-mono text-text w-4 text-center">{$habitTargets[h.name] ?? 5}</span>
                    <button class="px-1.5 py-0.5 bg-surface1 hover:bg-surface2 rounded text-text" onclick={() => bumpTarget(h.name, 1)}>+</button>
                    <button class="ml-1 px-1.5 py-0.5 text-dim hover:text-text underline" onclick={() => clearTarget(h.name)}>clear</button>
                    <button class="ml-auto px-1.5 py-0.5 text-dim hover:text-text" onclick={() => (editingTarget = null)}>done</button>
                  </div>
                {/if}
              </div>
            </div>

            <!-- Dot grid: 90 days, oldest→newest. Each dot is now a
                 button — click to toggle done/undone for that date.
                 Future-dated dots stay clickable too so the user can
                 plan-log (e.g. mark a workout planned for tomorrow). -->
            <div class="grid grid-flow-col grid-rows-7 gap-0.5" style="grid-auto-columns: minmax(0, 1fr);">
              {#each h.days as d (d.date)}
                {@const isToday = d.date === data.today}
                {@const cellBusy = busy === `${h.name}|${d.date}`}
                <button
                  type="button"
                  onclick={() => toggleOnDate(h, d.date, !d.done)}
                  disabled={cellBusy}
                  class="aspect-square rounded-[2px] transition-colors hover:opacity-70 disabled:opacity-40
                    {d.done ? 'bg-success' : 'bg-surface1 hover:bg-surface2'}"
                  class:ring-1={isToday}
                  class:ring-primary={isToday}
                  title={`${d.date}${d.done ? ' · done · click to undo' : ' · click to mark done'}`}
                  aria-label={`toggle ${h.name} on ${d.date}`}
                ></button>
              {/each}
            </div>
            <p class="text-[11px] text-dim mt-2">click any past day to mark / unmark — server updates that day's daily note</p>
          </article>
        {/each}
      </div>
    {:else if data && view === 'heatmap'}
      <!-- Year-at-a-glance per habit. The Heatmap component handles
           layout + tooltips; we just feed it {date, value} pairs.
           value = 1 when done, 0 otherwise — binary maxes the
           color scale at the brightest tone, which reads as a
           clean "completed" green. -->
      <div class="space-y-6">
        {#each sortedHabits as h (h.name)}
          <article class="bg-surface0 border border-surface1 rounded-lg p-3">
            <header class="flex items-baseline gap-2 mb-3">
              <h3 class="text-sm font-medium text-text">{h.name}</h3>
              <span class="text-[11px] text-dim font-mono tabular-nums">
                {h.currentStreak}d streak · best {h.longestStreak}d
              </span>
              <span class="text-[11px] text-dim font-mono tabular-nums ml-auto">
                {h.last30Pct}% / 30d
              </span>
            </header>
            <Heatmap
              cells={h.days.map((d) => ({ date: d.date, value: d.done ? 1 : 0 }))}
              maxValue={1}
              legendLabels={['none', '', '', '', 'done']}
            />
          </article>
        {/each}
      </div>
    {/if}
  </div>
</div>
