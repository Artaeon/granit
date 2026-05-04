<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, type HabitInfo, type HabitsResponse, type Virtue } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import Skeleton from '$lib/components/Skeleton.svelte';

  // /habits — three view modes for the same data:
  //   • Today: large quick-tick cards, the morning/evening rhythm view
  //   • Week:  7-column grid showing the last seven days at a glance
  //   • List:  full 90-day GitHub-style heatmap with streak stats
  // Sort + insight (best day of week) work across all three views so
  // the user's preference sticks regardless of which lens they pick.

  type View = 'today' | 'week' | 'list';
  type Sort = 'streak' | 'completion' | 'alpha' | 'behind';

  // Persist per-device so a user who lives in Week view doesn't have
  // to toggle every time. List remains the default for first-time
  // users — the 90-day grid is the most informative entry point.
  const VIEW_KEY = 'granit.habits.view';
  const SORT_KEY = 'granit.habits.sort';

  let view = $state<View>(
    (typeof localStorage !== 'undefined' && (localStorage.getItem(VIEW_KEY) as View)) || 'list'
  );
  let sortBy = $state<Sort>(
    (typeof localStorage !== 'undefined' && (localStorage.getItem(SORT_KEY) as Sort)) || 'streak'
  );

  $effect(() => {
    if (typeof localStorage === 'undefined') return;
    try { localStorage.setItem(VIEW_KEY, view); } catch {}
  });
  $effect(() => {
    if (typeof localStorage === 'undefined') return;
    try { localStorage.setItem(SORT_KEY, sortBy); } catch {}
  });

  let data = $state<HabitsResponse | null>(null);
  let loading = $state(false);
  let busy = $state<string | null>(null);

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
      await api.toggleHabit(name, data?.today ?? new Date().toISOString().slice(0, 10), false);
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

  // Virtue catalogue — used to compute the reverse linkage (which
  // virtues does each habit feed?). Best-effort: a missing virtues
  // module just leaves the linkage empty and the chip doesn't render.
  let virtues = $state<Virtue[]>([]);
  // Pre-computed habit-name (lowercased) → virtues that link it.
  // Recomputed when either side updates so the chips stay accurate.
  let virtuesByHabit = $derived.by(() => {
    const m = new Map<string, Virtue[]>();
    for (const v of virtues) {
      if ((v.status ?? 'active') !== 'active') continue;
      for (const h of v.linked_habits ?? []) {
        const k = h.toLowerCase();
        const arr = m.get(k) ?? [];
        arr.push(v);
        m.set(k, arr);
      }
    }
    return m;
  });

  async function load() {
    if (!$auth) return;
    loading = true;
    try {
      // Habits + virtues in parallel; virtues failure leaves the
      // reverse-linkage map empty (chip just doesn't render).
      const [d, v] = await Promise.all([
        api.listHabits(),
        api.listVirtues().catch(() => ({ virtues: [] as Virtue[], total: 0 }))
      ]);
      data = d;
      virtues = v.virtues;
    } finally {
      loading = false;
    }
  }
  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') load();
      if (ev.type === 'state.changed' && ev.path === '.granit/virtues.json') load();
    });
  });

  async function toggleToday(h: HabitInfo) {
    await toggleOnDate(h, data?.today ?? new Date().toISOString().slice(0, 10), !h.doneToday);
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
      <button
        type="button"
        onclick={() => (addOpen = !addOpen)}
        class="px-3 py-1.5 bg-primary text-on-primary rounded text-sm font-medium hover:opacity-90 self-start"
      >{addOpen ? 'cancel' : '+ Add habit'}</button>
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

    <!-- View + sort controls. Both rows wrap on narrow screens; the
         view toggle uses a segmented pill, the sort uses a select to
         keep horizontal space tight on phones. -->
    {#if data && data.habits.length > 0}
      <div class="flex flex-wrap items-center gap-2 mb-5">
        <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-sm">
          {#each [
            { v: 'today', label: 'Today' },
            { v: 'week', label: 'Week' },
            { v: 'list', label: 'List' }
          ] as o}
            <button
              type="button"
              class="px-3 py-1.5 capitalize transition-colors {view === o.v ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
              onclick={() => (view = o.v as View)}
            >{o.label}</button>
          {/each}
        </div>
        <span class="flex-1"></span>
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
          <div class="bg-surface0 border border-surface1 rounded-lg p-4 space-y-3">
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
        <div class="text-4xl mb-3 opacity-60">◈</div>
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
          {@const linkedVirtuesToday = virtuesByHabit.get(h.name.toLowerCase()) ?? []}
          <button
            type="button"
            onclick={() => toggleToday(h)}
            disabled={busy === h.name || !h.taskIdToday}
            title={h.taskIdToday ? '' : 'add this habit to today\'s daily note first'}
            class="text-left p-4 bg-surface0 border rounded-lg transition-colors flex items-start gap-3
              {h.doneToday
                ? 'border-success/40 bg-success/5'
                : 'border-surface1 hover:border-primary/40'}
              disabled:opacity-50"
          >
            <div
              class="w-10 h-10 rounded flex-shrink-0 flex items-center justify-center transition-colors
                {h.doneToday ? 'bg-success border-2 border-success' : 'border-2 border-surface2'}"
            >
              {#if h.doneToday}
                <svg viewBox="0 0 12 12" class="w-5 h-5 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
              {/if}
            </div>
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
              </div>
              <!-- Virtue chips — reverse linkage. "this habit feeds:" -->
              {#if linkedVirtuesToday.length > 0}
                <div class="flex flex-wrap gap-1 mt-1.5">
                  {#each linkedVirtuesToday as lv (lv.id)}
                    <a
                      href="/virtues"
                      onclick={(e) => e.stopPropagation()}
                      class="inline-flex items-baseline gap-1 px-1.5 py-0.5 rounded text-[10px] uppercase tracking-wider bg-secondary/15 text-secondary hover:bg-secondary/25"
                      title="feeds the {lv.name} virtue"
                    >🌱 {lv.name}</a>
                  {/each}
                </div>
              {/if}
            </div>
          </button>
        {/each}
      </div>
    {:else if data && view === 'week'}
      <!-- ===== WEEK VIEW ===== -->
      <!-- 7-column grid: each row a habit, each column a day. Header
           row labels the day-of-week + date (M/D). Last column is
           today and gets a primary outline. Click any cell to toggle. -->
      {@const days = sortedHabits.length > 0 ? weekDays(sortedHabits[0]) : []}
      <div class="bg-surface0 border border-surface1 rounded-lg p-3 sm:p-4 overflow-x-auto">
        <table class="w-full border-separate border-spacing-1">
          <thead>
            <tr>
              <th class="w-1/3 sm:w-1/4 text-left text-[11px] uppercase tracking-wider text-dim font-medium pb-2">Habit</th>
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
                      class="w-full aspect-square rounded transition-colors hover:opacity-80 disabled:opacity-40
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
          {@const linkedVirtuesList = virtuesByHabit.get(h.name.toLowerCase()) ?? []}
          <article class="bg-surface0 border border-surface1 rounded-lg p-4">
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
                </div>
                {#if linkedVirtuesList.length > 0}
                  <div class="flex flex-wrap gap-1 mt-1.5">
                    {#each linkedVirtuesList as lv (lv.id)}
                      <a
                        href="/virtues"
                        class="inline-flex items-baseline gap-1 px-1.5 py-0.5 rounded text-[10px] uppercase tracking-wider bg-secondary/15 text-secondary hover:bg-secondary/25"
                        title="feeds the {lv.name} virtue"
                      >🌱 {lv.name}</a>
                    {/each}
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
    {/if}
  </div>
</div>
