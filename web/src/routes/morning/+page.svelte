<script lang="ts">
  // The Calm Morning — single-page redesign.
  //
  // The previous iteration was an 8-step wizard (anchors → scripture
  // → prayer → goal → tasks → habits → thoughts → review). Even with
  // per-day persistence it felt like a slog: eight clicks before
  // "lock in", every step screen-sized, formal headers everywhere.
  //
  // The new shape is a single scroll. Sections live next to each other,
  // each one optional — leave it blank and it's silently skipped on
  // save. One "Lock in" CTA at the bottom commits the whole plan to
  // today's daily note. A "skip ritual" link at the top jumps straight
  // to the dashboard for days when the user just wants to get going.
  //
  // The API contract is preserved: saveMorning still takes the same
  // {scripture, goal, tasks, habits, thoughts} shape so existing
  // daily notes stay round-trippable. Prayer intentions still ride
  // along in the thoughts block under a `Praying for:` heading.

  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { auth } from '$lib/stores/auth';
  import { api, todayISO, type Task, type Goal } from '$lib/api';
  import { scriptures } from '$lib/morning/scriptures';
  import { createMorningScripture } from '$lib/morning/morningScripture.svelte';
  import { createMorningPicks } from '$lib/morning/morningPicks.svelte';
  import { createMorningData } from '$lib/morning/morningData.svelte';
  import { createMorningBriefing } from '$lib/morning/morningBriefing.svelte';
  import { createMorningFocus } from '$lib/morning/morningFocus.svelte';
  import { installMorningPersistence } from '$lib/morning/morningPersistence.svelte';
  import { inlineMd } from '$lib/util/inlineMd';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import DeadlinePill from '$lib/deadlines/DeadlinePill.svelte';

  // ─── Persistence key (today) ──────────────────────────────────────
  // Hoisted above the controller construction so dataCtl can read it.
  const today = todayISO();
  const STORAGE_KEY = `granit.morning.${today}`;

  // ─── Data ─────────────────────────────────────────────────────────
  const dataCtl = createMorningData({
    todayISO: today,
    listTasks: (o) => api.listTasks(o),
    listHabits: () => api.listHabits(),
    listGoals: () => api.listGoals(),
    tryListDeadlines: () => api.tryListDeadlines(),
    listPrayer: () => api.listPrayer(),
    calendar: (a, b) => api.calendar(a, b)
  });
  const activeGoals = $derived(dataCtl.activeGoals);
  const upcomingDeadlines = $derived(dataCtl.upcomingDeadlines);
  const openTasks = $derived(dataCtl.openTasks);
  const knownHabits = $derived(dataCtl.knownHabits);
  const activeIntentions = $derived(dataCtl.activeIntentions);
  const todayEvents = $derived(dataCtl.todayEvents);

  // ─── Form state ───────────────────────────────────────────────────
  const scriptureCtl = createMorningScripture();

  const focusCtl = createMorningFocus({
    todayISO: today,
    getSortedTasks: () => sortedTasks,
    chat: (m) => api.chat(m)
  });
  const picksCtl = createMorningPicks({
    appendKnownHabit: (h) => dataCtl.appendKnownHabit(h),
    prependActiveIntention: (p) => dataCtl.prependActiveIntention(p),
    createPrayer: (args) => api.createPrayer(args)
  });
  let thoughts = $state('');

  let saving = $state(false);
  // Local save-path error. Load errors live on dataCtl.error and are
  // merged into the banner via the `error` derived below.
  let saveError = $state('');
  const error = $derived(saveError || dataCtl.error);

  // AI morning briefing — a 60-100 word read of "what today looks
  // like and where to focus". Distinct from the single-sentence
  // focus suggestion (which fills the #1 focus field): this is a
  // narrative orient that the user reads ONCE and then dismisses,
  // analogous to a personal-assistant note dropped on the desk.
  // Pulls calendar events for today + upcoming deadlines + a slice
  // of urgent open tasks + active goals.
  const briefingCtl = createMorningBriefing({
    todayISO: today,
    getEvents: () => dataCtl.todayEvents,
    getTasks: () => sortedTasks,
    getGoals: () => dataCtl.activeGoals,
    getDeadlines: () => upcomingNear,
    chatStream: (m, n, h, s) => api.chatStream(m, n, h, s)
  });

  // ─── Persistence ──────────────────────────────────────────────────
  // Per-day localStorage so a closed tab doesn't lose progress, but
  // yesterday's half-finished morning doesn't bleed into today.
  const persistenceCtl = installMorningPersistence({
    storageKey: STORAGE_KEY,
    scriptureCtl,
    focusCtl,
    picksCtl,
    getThoughts: () => thoughts,
    setThoughts: (v) => { thoughts = v; }
  });

  // ─── Load ─────────────────────────────────────────────────────────
  async function load() {
    if (!$auth) return;
    await dataCtl.load();
    // Restore the per-day brief-dismissed flag.
    briefingCtl.hydrateDismissed();
    // Pre-tick today's habits (only if no restored snapshot — don't
    // clobber the user's deliberate choices).
    if (!persistenceCtl.hasSnapshot()) picksCtl.pretickWarmHabits(dataCtl.knownHabits);
  }
  onMount(() => {
    persistenceCtl.restore();
    load();
  });

  // ─── Derived ──────────────────────────────────────────────────────
  const activeScripture = $derived(scriptureCtl.active);
  const greeting = $derived.by(() => {
    const h = new Date().getHours();
    if (h < 5) return 'Late night';
    if (h < 12) return 'Good morning';
    if (h < 17) return 'Good afternoon';
    return 'Good evening';
  });
  const dateLine = $derived(new Date().toLocaleDateString(undefined, {
    weekday: 'long', month: 'long', day: 'numeric'
  }));

  // Tasks sorted by urgency. Same algorithm as the previous version —
  // overdue+important → due today → quick wins → priority/date.
  const sortedTasks = $derived.by(() => {
    type Bucketed = { task: Task; bucket: number; rank: number };
    const isOverdueImportant = (t: Task) =>
      !!t.dueDate && t.dueDate < today && (t.priority === 1 || t.priority === 2);
    const isDueToday = (t: Task) => t.dueDate === today;
    const isQuickWin = (t: Task) =>
      !t.scheduledStart &&
      (!t.dueDate || t.dueDate >= today) &&
      (((t.estimatedMinutes ?? 0) > 0 && (t.estimatedMinutes ?? 0) <= 30) ||
        (t.text.length <= 60 && (t.priority === 0 || t.priority >= 3)));
    const bucketed: Bucketed[] = openTasks.map((t) => {
      let bucket = 9;
      if (isOverdueImportant(t)) bucket = 0;
      else if (isDueToday(t)) bucket = 1;
      else if (isQuickWin(t)) bucket = 2;
      const rank =
        bucket === 0
          ? -((today.localeCompare(t.dueDate ?? '~')) || 0)
          : (t.priority || 99) * 100 + (t.dueDate ? Number(t.dueDate.replace(/-/g, '').slice(2)) : 999_999);
      return { task: t, bucket, rank };
    });
    return bucketed
      .sort((a, b) => (a.bucket !== b.bucket ? a.bucket - b.bucket : a.rank - b.rank))
      .slice(0, 60)
      .map((x) => x.task);
  });
  const taskPreview = $derived(sortedTasks.slice(0, 8));
  const taskOverflow = $derived(sortedTasks.length - taskPreview.length);

  const sortedIntentions = $derived.by(() => {
    const tied = activeIntentions.filter((p) => p.venture || p.project || p.goal);
    const persons = activeIntentions.filter((p) => p.person && !(p.venture || p.project || p.goal));
    const general = activeIntentions.filter((p) => !p.person && !(p.venture || p.project || p.goal));
    return [...tied, ...persons, ...general];
  });

  const pickedTaskTexts = $derived.by(() => {
    const ts: string[] = [];
    for (const t of openTasks) if (picksCtl.pickedTasks.has(t.id)) ts.push(t.text);
    return ts;
  });

  function daysUntil(iso: string): number {
    const [y, m, d] = iso.split('-').map(Number);
    const due = new Date(y, m - 1, d);
    const t = new Date();
    t.setHours(0, 0, 0, 0);
    return Math.round((due.getTime() - t.getTime()) / 86_400_000);
  }
  const upcomingNear = $derived.by(() => {
    if (!upcomingDeadlines) return [];
    return upcomingDeadlines
      .filter((d) => d.status !== 'cancelled' && d.status !== 'met')
      .map((d) => ({ d, days: daysUntil(d.date) }))
      .filter((x) => x.days <= 7)
      .sort((a, b) => a.days - b.days)
      .slice(0, 3);
  });

  // ─── Actions ──────────────────────────────────────────────────────
  // Morning stat row — quick at-a-glance numbers the user sees
  // before they decide what to plan. Derived from the same data
  // arrays the rest of the page reads.
  const stats = $derived.by(() => {
    let overdue = 0;
    let dueToday = 0;
    for (const t of openTasks) {
      if (!t.dueDate) continue;
      if (t.dueDate < today) overdue++;
      else if (t.dueDate === today) dueToday++;
    }
    return {
      openTasks: openTasks.length,
      overdue,
      dueToday,
      events: todayEvents.length,
      deadlinesThisWeek: upcomingNear.length
    };
  });
  async function lockIn() {
    saving = true;
    saveError = '';
    try {
      const linked = activeGoals.find((g) => g.id === focusCtl.linkedGoalId);
      const goalText = focusCtl.goal.trim();
      const goalForSave = goalText
        ? linked ? `${goalText} — contributes to: ${linked.title}` : goalText
        : undefined;

      // Prayer intentions ride along in the thoughts block under
      // 'Praying for:' (server has no dedicated prayer field — keeps
      // the daily note self-contained without a schema change).
      const prayerLines: string[] = [];
      for (const id of picksCtl.pickedIntentions) {
        const intent = activeIntentions.find((x) => x.id === id);
        if (!intent) continue;
        let line = `- ${intent.text}`;
        if (intent.venture) line += ` (🏢 ${intent.venture})`;
        else if (intent.project) line += ` (📁 ${intent.project})`;
        else if (intent.person) line += ` (👤 ${intent.person})`;
        if (intent.passage_ref) line += ` — ${intent.passage_ref}`;
        prayerLines.push(line);
      }
      const prayerBlock = prayerLines.length > 0 ? `Praying for:\n${prayerLines.join('\n')}` : '';
      const winLine = focusCtl.winSentence.trim();
      const winPart = winLine ? `Today's win: ${winLine}` : '';
      const thoughtsRaw = thoughts.trim();
      const thoughtsBody = [winPart, prayerBlock, thoughtsRaw]
        .filter((s) => s.length > 0)
        .join('\n\n') || undefined;

      await api.saveMorning({
        scripture: activeScripture.text ? activeScripture : undefined,
        goal: goalForSave,
        tasks: pickedTaskTexts,
        habits: Array.from(picksCtl.pickedHabits),
        thoughts: thoughtsBody
      });
      persistenceCtl.clear();
      toast.success('today is locked in');
      goto('/');
    } catch (e) {
      const msg = errorMessage(e);
      saveError = msg;
      toast.error(`save failed: ${msg}`);
    } finally {
      saving = false;
    }
  }

  // Counts the user can see on the lock-in button.
  const filledCount = $derived.by(() => {
    let n = 0;
    if (focusCtl.winSentence.trim()) n++;
    if (focusCtl.goal.trim()) n++;
    if (picksCtl.pickedTasks.size > 0) n++;
    if (picksCtl.pickedHabits.size > 0) n++;
    if (thoughts.trim() || picksCtl.pickedIntentions.size > 0) n++;
    return n;
  });
</script>

<div class="h-full overflow-y-auto bg-base flex flex-col">
  <div class="p-4 sm:p-6 lg:p-8 max-w-2xl w-full mx-auto flex-1">
    <!-- Header: greeting + scripture + date. The whole point of
         scripture being inline (not its own step) is that it's
         passive — read once, anchor the day, move on. -->
    <header class="mb-4 sm:mb-8">
      <div class="flex items-baseline justify-between gap-3 mb-3">
        <h1 class="text-2xl sm:text-3xl font-semibold text-text">{greeting}</h1>
        <a href="/" class="text-xs text-dim hover:text-text">skip ritual →</a>
      </div>
      <div class="text-xs text-dim">{dateLine}</div>

      {#if activeScripture.text}
        <blockquote class="mt-4 border-l-2 border-primary pl-3 py-1.5 italic text-subtext text-sm">
          "{activeScripture.text}"
          {#if activeScripture.source}
            <span class="not-italic text-[11px] text-dim ml-2">— {activeScripture.source}</span>
          {/if}
        </blockquote>
        <button
          type="button"
          onclick={() => (scriptureCtl.pickerOpen = !scriptureCtl.pickerOpen)}
          class="text-[11px] text-dim hover:text-text mt-1"
        >
          {scriptureCtl.pickerOpen ? 'hide picker' : 'change verse'}
        </button>
      {/if}

      {#if scriptureCtl.pickerOpen}
        <div class="mt-3 p-3 bg-mantle border border-surface1 rounded space-y-3">
          <div>
            <div class="text-[10px] uppercase tracking-wider text-dim mb-1.5">From rotation</div>
            <div class="flex flex-wrap gap-1.5 max-h-32 overflow-y-auto">
              {#each scriptures as s}
                <button
                  type="button"
                  onclick={() => scriptureCtl.pick(s)}
                  class="text-xs px-2.5 py-1.5 rounded border
                    sm:text-[11px] sm:px-2 sm:py-0.5
                    {scriptureCtl.scripture === s && !scriptureCtl.customScripture ? 'border-primary text-primary' : 'border-surface1 text-subtext hover:border-primary'}"
                >{s.source}</button>
              {/each}
            </div>
          </div>
          <div>
            <div class="text-[10px] uppercase tracking-wider text-dim mb-1.5">Or paste your own</div>
            <input bind:value={scriptureCtl.customScripture} placeholder="quote / verse text"
              class="w-full px-2 py-1.5 mb-1.5 bg-surface0 border border-surface1 rounded text-sm" />
            <input bind:value={scriptureCtl.customSource} placeholder="source"
              class="w-full px-2 py-1.5 bg-surface0 border border-surface1 rounded text-sm" />
          </div>
        </div>
      {/if}
    </header>

    {#if error}
      <div class="mb-5 text-sm text-error p-3 bg-surface0 border border-error rounded">{error}</div>
    {/if}

    <!-- Stat strip — quick read of what today looks like before any
         AI call. Numbers come from the same data arrays the rest of
         the page reads, so they're consistent with the lower
         sections. Hidden when everything is zero — a brand-new
         vault doesn't need this row taking space. -->
    {#if stats.openTasks + stats.events + stats.deadlinesThisWeek > 0}
      <section class="mb-4 flex flex-wrap items-center gap-1.5 text-xs">
        <span class="px-2 py-1 rounded bg-surface0 text-subtext font-mono tabular-nums">
          <span class="text-text font-semibold">{stats.openTasks}</span> open
        </span>
        {#if stats.overdue > 0}
          <span class="px-2 py-1 rounded bg-surface0 text-error font-mono tabular-nums" title="Past their due date">
            <span class="font-semibold">{stats.overdue}</span> overdue
          </span>
        {/if}
        {#if stats.dueToday > 0}
          <span class="px-2 py-1 rounded bg-surface0 text-warning font-mono tabular-nums" title="Tasks due today">
            <span class="font-semibold">{stats.dueToday}</span> due today
          </span>
        {/if}
        {#if stats.events > 0}
          <span class="px-2 py-1 rounded bg-surface0 text-info font-mono tabular-nums" title="Events scheduled today">
            <span class="font-semibold">{stats.events}</span> event{stats.events === 1 ? '' : 's'}
          </span>
        {/if}
        {#if stats.deadlinesThisWeek > 0}
          <span class="px-2 py-1 rounded bg-surface0 text-secondary font-mono tabular-nums" title="Deadlines within 7 days">
            <span class="font-semibold">{stats.deadlinesThisWeek}</span> deadline{stats.deadlinesThisWeek === 1 ? '' : 's'}
          </span>
        {/if}
      </section>
    {/if}

    <!-- AI morning brief — a 60-110 word read of "what today looks
         like and where to focus". Distinct from the single-sentence
         #1 focus suggester below: the brief is narrative orientation
         the user reads once and then dismisses (the dismissed flag
         is per-day so tomorrow starts clean). Streamed via
         api.chatStream + rafThrottle so a fast model doesn't choke
         the page. Sabbath / consent gates fire server-side; the
         error toast surfaces classifyAiError's headline. -->
    {#if !briefingCtl.dismissed}
      <section class="mb-7 p-3 sm:p-4 bg-mantle border border-surface1 rounded">
        <div class="flex items-baseline gap-2 mb-2">
          <span class="text-[10px] uppercase tracking-wider text-dim">AI brief</span>
          {#if briefingCtl.busy}
            <span class="text-[10px] text-secondary">streaming…</span>
          {/if}
          <span class="flex-1"></span>
          {#if briefingCtl.busy}
            <button
              type="button"
              onclick={briefingCtl.cancel}
              class="text-[11px] text-warning hover:text-error"
            >cancel</button>
          {:else if briefingCtl.text.trim()}
            <button
              type="button"
              onclick={briefingCtl.run}
              class="text-[11px] text-secondary hover:underline"
              title="Re-run the brief with fresh context"
            >↻ regenerate</button>
            <button
              type="button"
              onclick={briefingCtl.dismiss}
              class="text-[11px] text-dim hover:text-error"
              title="Hide for the rest of today"
            >dismiss</button>
          {/if}
        </div>
        {#if briefingCtl.error}
          <div class="text-sm text-error">{briefingCtl.error}</div>
        {:else if briefingCtl.text.trim()}
          <!-- Render as paragraph-split prose so the three-paragraph
               structure the prompt asks for reads correctly. -->
          <div class="text-sm text-text leading-relaxed space-y-2">
            {#each briefingCtl.text.trim().split(/\n{2,}/) as para}
              {#if para.trim()}<p>{para.trim()}</p>{/if}
            {/each}
          </div>
        {:else if briefingCtl.busy && briefingCtl.prev.trim()}
          <!-- Regenerate in flight — keep the previous brief on screen
               (dimmed) so the user has something to read until the new
               one streams in. -->
          <div class="text-sm text-dim leading-relaxed space-y-2 opacity-60">
            {#each briefingCtl.prev.trim().split(/\n{2,}/) as para}
              {#if para.trim()}<p>{para.trim()}</p>{/if}
            {/each}
          </div>
        {:else if briefingCtl.busy}
          <p class="text-sm text-dim italic">Reading the calendar + tasks…</p>
        {:else}
          <p class="text-sm text-dim mb-2">A 60-110 word read of how today looks and where to focus, grounded in your calendar + open tasks.</p>
          <button
            type="button"
            onclick={briefingCtl.run}
            class="text-xs px-2 py-1 bg-surface0 hover:bg-surface1 text-secondary border border-secondary"
            title="Generate a morning brief"
          >Generate brief</button>
          <button
            type="button"
            onclick={briefingCtl.dismiss}
            class="ml-1 text-[11px] text-dim hover:text-text px-1.5 py-1"
            title="Hide the brief section for today"
          >not today</button>
        {/if}
      </section>
    {/if}

    <!-- Anchors strip — passive context. Active goals + nearest
         deadlines so the user starts grounded in what they're
         actually working towards before picking today's specifics. -->
    {#if activeGoals.length > 0 || upcomingNear.length > 0}
      <section class="mb-7 p-3 sm:p-4 rounded-lg bg-mantle border border-surface1">
        <div class="text-[10px] uppercase tracking-wider text-dim mb-2">Working towards</div>
        {#if activeGoals.length > 0}
          <ul class="space-y-1.5">
            {#each activeGoals as g (g.id)}
              <li class="flex items-baseline gap-2 text-sm">
                <span class="text-dim">🎯</span>
                <span class="flex-1 text-text truncate">{@html inlineMd(g.title)}</span>
                {#if g.target_date}<span class="text-[11px] text-dim font-mono">{g.target_date}</span>{/if}
              </li>
            {/each}
          </ul>
        {/if}
        {#if upcomingNear.length > 0}
          <ul class="space-y-1.5 {activeGoals.length > 0 ? 'mt-2 pt-2 border-t border-surface1' : ''}">
            {#each upcomingNear as { d, days } (d.id)}
              <li class="flex items-baseline gap-2 text-sm">
                <DeadlinePill variant="countdown" {days} status={d.status} />
                <span class="flex-1 text-text truncate">{d.title}</span>
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    {/if}

    <!-- 1 · Today's win — the single most important sentence. Big
         input, no chrome. -->
    <section class="mb-7">
      <div class="flex items-baseline justify-between mb-2">
        <h2 class="text-sm font-semibold text-text uppercase tracking-wider">What would make today a win?</h2>
        {#if focusCtl.winSentence.trim()}<span class="text-[10px] text-success">✓</span>{/if}
      </div>
      <input
        bind:value={focusCtl.winSentence}
        placeholder="one concrete sentence — finishable today"
        class="w-full px-3 py-3 text-base bg-surface0 border border-surface1 rounded-lg text-text placeholder-dim focus:outline-none focus:border-primary"
      />
    </section>

    <!-- 2 · Today's #1 focus — with AI suggest -->
    <section class="mb-7">
      <div class="flex items-baseline justify-between mb-2">
        <h2 class="text-sm font-semibold text-text uppercase tracking-wider">Today's #1 focus</h2>
        <button
          type="button"
          onclick={focusCtl.suggestFocus}
          disabled={focusCtl.suggesting}
          class="text-[11px] text-primary hover:underline disabled:opacity-50 inline-flex items-center gap-1"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-3 h-3">
            <path d="M12 3l1.2 4.2L17 9l-3.8 1.8L12 15l-1.2-4.2L7 9l3.8-1.8L12 3z" stroke-linejoin="round"/>
          </svg>
          {focusCtl.suggesting ? 'thinking…' : 'suggest from tasks'}
        </button>
      </div>
      <input
        bind:value={focusCtl.goal}
        placeholder="if you got one thing done…"
        class="w-full px-3 py-2.5 bg-surface0 border border-surface1 rounded text-text placeholder-dim focus:outline-none focus:border-primary"
      />
      {#if focusCtl.suggestion}
        <div class="mt-2 p-2.5 bg-surface1 border-l-2 border-primary rounded text-sm">
          <div class="text-text mb-1.5">{focusCtl.suggestion}</div>
          <div class="flex items-center gap-2">
            <button onclick={focusCtl.acceptSuggestion} class="px-2 py-0.5 text-[11px] rounded bg-primary text-on-primary font-medium">use this</button>
            <button onclick={focusCtl.dismissSuggestion} class="px-2 py-0.5 text-[11px] text-dim hover:text-text">dismiss</button>
            <button onclick={focusCtl.suggestFocus} disabled={focusCtl.suggesting} class="ml-auto text-[11px] text-secondary hover:underline disabled:opacity-50">try again</button>
          </div>
        </div>
      {/if}
      {#if activeGoals.length > 0}
        <div class="flex flex-wrap items-center gap-1.5 mt-2.5">
          <span class="text-[11px] text-dim">contributes to:</span>
          {#each activeGoals as g (g.id)}
            {@const sel = focusCtl.linkedGoalId === g.id}
            <button
              type="button"
              onclick={() => focusCtl.pickGoalLink(g)}
              class="px-2 py-0.5 text-[11px] rounded-full border transition-colors
                {sel ? 'bg-surface1 border-primary text-primary' : 'border-surface1 text-subtext hover:border-primary'}"
            >
              {sel ? '✓ ' : ''}{g.title}
            </button>
          {/each}
          {#if focusCtl.linkedGoalId}
            <button onclick={focusCtl.clearGoalLink} class="text-[11px] text-dim hover:text-text">clear</button>
          {/if}
        </div>
      {/if}
    </section>

    <!-- 3 · Tasks — compact picker, urgency-sorted -->
    <section class="mb-7">
      <div class="flex items-baseline justify-between mb-2">
        <h2 class="text-sm font-semibold text-text uppercase tracking-wider">Pick your tasks</h2>
        <span class="text-[11px] text-dim">{picksCtl.pickedTasks.size} picked</span>
      </div>
      {#if openTasks.length === 0}
        <p class="text-sm text-dim italic">no open tasks — <a href="/tasks" class="text-secondary hover:underline">add some</a></p>
      {:else}
        <ul class="space-y-0.5">
          {#each (picksCtl.showAllTasks ? sortedTasks : taskPreview) as t (t.id)}
            {@const sel = picksCtl.pickedTasks.has(t.id)}
            {@const overdueImp = !!t.dueDate && t.dueDate < today && (t.priority === 1 || t.priority === 2)}
            {@const dueToday = t.dueDate === today}
            <li>
              <button
                type="button"
                onclick={() => picksCtl.toggleTask(t.id)}
                class="w-full text-left flex items-baseline gap-2 px-2 py-1.5 rounded hover:bg-surface0 group"
              >
                <span class="w-4 h-4 mt-0.5 rounded border flex-shrink-0 flex items-center justify-center
                  {sel ? 'bg-primary border-primary' : 'border-surface2'}">
                  {#if sel}<svg viewBox="0 0 12 12" class="w-3 h-3 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>{/if}
                </span>
                {#if overdueImp}
                  <span class="text-[10px] font-mono px-1 rounded bg-surface0 text-error">!OVERDUE</span>
                {:else if dueToday}
                  <span class="text-[10px] font-mono px-1 rounded bg-surface1 text-primary">TODAY</span>
                {/if}
                {#if t.priority > 0}
                  <span class="text-[10px] font-mono px-1 rounded
                    {t.priority === 1 ? 'bg-surface0 text-error' : ''}
                    {t.priority === 2 ? 'bg-surface0 text-warning' : ''}
                    {t.priority === 3 ? 'bg-surface0 text-info' : ''}">P{t.priority}</span>
                {/if}
                <span class="flex-1 text-sm text-text">{@html inlineMd(t.text)}</span>
                {#if t.estimatedMinutes}<span class="text-[10px] text-dim">⏱ {t.estimatedMinutes}m</span>{/if}
                {#if t.dueDate}<span class="text-[11px] text-dim font-mono">{t.dueDate}</span>{/if}
              </button>
            </li>
          {/each}
        </ul>
        {#if !picksCtl.showAllTasks && taskOverflow > 0}
          <button
            type="button"
            onclick={() => (picksCtl.showAllTasks = true)}
            class="mt-1.5 text-[11px] text-secondary hover:underline"
          >show {taskOverflow} more</button>
        {:else if picksCtl.showAllTasks && sortedTasks.length > 8}
          <button
            type="button"
            onclick={() => (picksCtl.showAllTasks = false)}
            class="mt-1.5 text-[11px] text-dim hover:text-text"
          >collapse</button>
        {/if}
      {/if}
    </section>

    <!-- 4 · Habits — compact horizontal grid -->
    {#if knownHabits.length > 0 || picksCtl.newHabit.length === 0}
      <section class="mb-7">
        <div class="flex items-baseline justify-between mb-2">
          <h2 class="text-sm font-semibold text-text uppercase tracking-wider">Habits</h2>
          <span class="text-[11px] text-dim">{picksCtl.pickedHabits.size} committed</span>
        </div>
        {#if knownHabits.length > 0}
          <div class="flex flex-wrap gap-1.5 mb-2">
            {#each knownHabits as h (h.name)}
              {@const sel = picksCtl.pickedHabits.has(h.name)}
              <button
                type="button"
                onclick={() => picksCtl.toggleHabit(h.name)}
                class="px-3 py-2 text-xs rounded-full border transition-colors inline-flex items-center gap-1.5
                  sm:px-2.5 sm:py-1 sm:text-[11px]
                  {sel ? 'bg-surface1 border-primary text-primary' : 'border-surface1 text-subtext hover:border-primary'}"
              >
                <span>{sel ? '☑' : '☐'}</span>
                <span>{h.name}</span>
                {#if h.currentStreak > 0}<span class="text-warning">🔥{h.currentStreak}</span>{/if}
              </button>
            {/each}
          </div>
        {/if}
        <form onsubmit={picksCtl.addCustomHabit} class="flex gap-2">
          <input
            bind:value={picksCtl.newHabit}
            placeholder="add a habit…"
            class="flex-1 px-2.5 py-1.5 bg-surface0 border border-surface1 rounded text-sm"
          />
          <button type="submit" disabled={!picksCtl.newHabit.trim()} class="px-2.5 py-1.5 bg-surface1 text-subtext rounded text-sm disabled:opacity-50">+ add</button>
        </form>
      </section>
    {/if}

    <!-- 5 · Bring forward — combined prayer + thoughts. The previous
         design split these into two ceremonial steps; merging keeps
         the meditative quality without the click overhead. The
         user's input lands in the daily note's thoughts section;
         picked prayer chips ride along under "Praying for:". -->
    <section class="mb-7">
      <div class="flex items-baseline justify-between mb-2">
        <h2 class="text-sm font-semibold text-text uppercase tracking-wider">Bring forward</h2>
        {#if activeIntentions.length > 0}
          <button
            type="button"
            onclick={() => (picksCtl.prayerPickerOpen = !picksCtl.prayerPickerOpen)}
            class="text-[11px] text-secondary hover:underline"
          >
            {picksCtl.prayerPickerOpen ? 'hide intentions' : `${picksCtl.pickedIntentions.size} of ${activeIntentions.length} intentions`}
          </button>
        {/if}
      </div>
      <textarea
        bind:value={thoughts}
        rows="3"
        placeholder="grateful for · wrestling with · today's mood · what to bring before God"
        class="w-full px-3 py-2.5 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary leading-relaxed"
      ></textarea>
      {#if picksCtl.prayerPickerOpen}
        <div class="mt-3 p-3 bg-mantle border border-surface1 rounded space-y-2">
          {#if sortedIntentions.length > 0}
            <ul class="space-y-1 max-h-48 overflow-y-auto">
              {#each sortedIntentions as p (p.id)}
                {@const picked = picksCtl.pickedIntentions.has(p.id)}
                <li>
                  <button
                    type="button"
                    onclick={() => picksCtl.toggleIntention(p.id)}
                    class="w-full text-left flex items-start gap-2 px-2 py-2.5 rounded
                      sm:py-1.5
                      {picked ? 'bg-primary/10' : 'hover:bg-surface0'}"
                  >
                    <span class="w-4 h-4 mt-0.5 rounded border flex-shrink-0 flex items-center justify-center text-[10px]
                      sm:w-3.5 sm:h-3.5 sm:text-[9px]
                      {picked ? 'bg-primary border-primary text-on-primary' : 'border-surface2'}">{picked ? '✓' : ''}</span>
                    <div class="flex-1 min-w-0">
                      <div class="text-sm text-text">{p.text}</div>
                      {#if p.venture || p.project || p.person}
                        <div class="text-[11px] text-dim">
                          {#if p.venture}🏢 {p.venture}
                          {:else if p.project}📁 {p.project}
                          {:else if p.person}👤 {p.person}{/if}
                        </div>
                      {/if}
                    </div>
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
          <form onsubmit={picksCtl.addNewPrayer} class="flex gap-2">
            <input
              bind:value={picksCtl.newPrayerText}
              placeholder="add an intention…"
              class="flex-1 px-2 py-1.5 bg-surface0 border border-surface1 rounded text-sm"
            />
            <button type="submit" disabled={!picksCtl.newPrayerText.trim() || picksCtl.addingPrayer} class="px-2.5 py-1.5 bg-primary text-on-primary rounded text-sm disabled:opacity-50">
              {picksCtl.addingPrayer ? '…' : '+ add'}
            </button>
          </form>
        </div>
      {/if}
    </section>
  </div>

  <!-- Sticky Lock-in footer. Lives inside the scroll container so
       sticky bottom-0 pins to the bottom of the main area without
       caring about sidebar width / compact mode (the previous fixed-
       position version had hardcoded left offsets that broke when
       the sidebar collapsed to compact). The bottom padding adds the
       iOS home-indicator inset so the Lock-in button stays reachable
       above the gesture area on phones. -->
  <footer class="sticky bottom-0 z-20 border-t border-surface1 bg-mantle px-4 py-3 pb-[calc(0.75rem+env(safe-area-inset-bottom,0px))]">
    <div class="max-w-2xl mx-auto flex items-center gap-3">
      <span class="text-[11px] text-dim flex-1">
        {#if filledCount === 0}
          Fill anything you want — empty sections are silently skipped.
        {:else}
          {filledCount} section{filledCount === 1 ? '' : 's'} filled · saves to today's daily note
        {/if}
      </span>
      <button
        onclick={lockIn}
        disabled={saving || filledCount === 0}
        class="px-5 py-2 rounded text-sm font-semibold bg-primary text-on-primary disabled:opacity-50 disabled:cursor-not-allowed"
      >
        {saving ? 'saving…' : 'Lock in →'}
      </button>
    </div>
  </footer>
</div>
