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
  import { api, todayISO, type Task, type HabitInfo, type Goal, type Deadline, type PrayerIntention } from '$lib/api';
  import { scriptures, scriptureOfTheDay } from '$lib/morning/scriptures';
  import { inlineMd } from '$lib/util/inlineMd';
  import { toast } from '$lib/components/toast';
  import { classifyAiError } from '$lib/util/aiErrors';
  import DeadlinePill from '$lib/deadlines/DeadlinePill.svelte';

  // ─── Data ─────────────────────────────────────────────────────────
  let activeGoals = $state<Goal[]>([]);
  let allGoalsById = $state<Record<string, string>>({});
  let upcomingDeadlines = $state<Deadline[] | null>(null);
  let openTasks = $state<Task[]>([]);
  let knownHabits = $state<HabitInfo[]>([]);
  let activeIntentions = $state<PrayerIntention[]>([]);

  // ─── Form state ───────────────────────────────────────────────────
  let scripture = $state(scriptureOfTheDay());
  let customScripture = $state('');
  let customSource = $state('');
  let scripturePickerOpen = $state(false);

  let winSentence = $state('');
  let goal = $state('');
  let linkedGoalId = $state<string>('');
  let pickedTasks = $state<Set<string>>(new Set());
  let showAllTasks = $state(false);
  let pickedHabits = $state<Set<string>>(new Set());
  let pickedIntentions = $state<Set<string>>(new Set());
  let prayerPickerOpen = $state(false);
  let thoughts = $state('');
  let newPrayerText = $state('');
  let addingPrayer = $state(false);
  let newHabit = $state('');

  let saving = $state(false);
  let suggesting = $state(false);
  let suggestion = $state('');
  let error = $state('');

  // ─── Persistence ──────────────────────────────────────────────────
  // Per-day localStorage so a closed tab doesn't lose progress, but
  // yesterday's half-finished morning doesn't bleed into today.
  const today = todayISO();
  const STORAGE_KEY = `granit.morning.${today}`;
  interface Snapshot {
    scriptureSource: string;
    customScripture: string;
    customSource: string;
    winSentence: string;
    goal: string;
    linkedGoalId: string;
    pickedTasks: string[];
    pickedHabits: string[];
    pickedIntentions: string[];
    thoughts: string;
    newHabit: string;
  }
  function persist() {
    const s: Snapshot = {
      scriptureSource: scripture.source,
      customScripture,
      customSource,
      winSentence,
      goal,
      linkedGoalId,
      pickedTasks: [...pickedTasks],
      pickedHabits: [...pickedHabits],
      pickedIntentions: [...pickedIntentions],
      thoughts,
      newHabit
    };
    try { localStorage.setItem(STORAGE_KEY, JSON.stringify(s)); } catch {}
  }
  function restore() {
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      if (!raw) return false;
      const s = JSON.parse(raw) as Snapshot;
      if (s.scriptureSource) {
        const m = scriptures.find((x) => x.source === s.scriptureSource);
        if (m) scripture = m;
      }
      customScripture = s.customScripture ?? '';
      customSource = s.customSource ?? '';
      winSentence = s.winSentence ?? '';
      goal = s.goal ?? '';
      linkedGoalId = s.linkedGoalId ?? '';
      pickedTasks = new Set(s.pickedTasks ?? []);
      pickedHabits = new Set(s.pickedHabits ?? []);
      pickedIntentions = new Set(s.pickedIntentions ?? []);
      thoughts = s.thoughts ?? '';
      newHabit = s.newHabit ?? '';
      return true;
    } catch {
      return false;
    }
  }
  function clearPersisted() {
    try { localStorage.removeItem(STORAGE_KEY); } catch {}
  }
  $effect(() => {
    void scripture; void customScripture; void customSource;
    void winSentence; void goal; void linkedGoalId;
    void pickedTasks; void pickedHabits; void pickedIntentions;
    void thoughts; void newHabit;
    persist();
  });

  // ─── Load ─────────────────────────────────────────────────────────
  async function load() {
    if (!$auth) return;
    try {
      const [t, h, g, d, p] = await Promise.all([
        api.listTasks({ status: 'open' }),
        api.listHabits(),
        api.listGoals().catch((): { goals: Goal[]; total: number } => ({ goals: [], total: 0 })),
        api.tryListDeadlines(),
        api.listPrayer().catch(() => ({ intentions: [] as PrayerIntention[], total: 0 }))
      ]);
      openTasks = t.tasks;
      knownHabits = h.habits;
      activeIntentions = p.intentions.filter((x) => x.status === 'praying');
      activeGoals = g.goals.filter((x) => (x.status ?? 'active') === 'active').slice(0, 3);
      const map: Record<string, string> = {};
      for (const ge of g.goals) map[ge.id] = ge.title;
      allGoalsById = map;
      upcomingDeadlines = d;
      // Pre-tick today's habits (only if no restored snapshot — don't
      // clobber the user's deliberate choices).
      const hadSnapshot = !!localStorage.getItem(STORAGE_KEY);
      if (!hadSnapshot) {
        for (const k of knownHabits) {
          if (k.last7Pct >= 50) pickedHabits.add(k.name);
        }
        pickedHabits = new Set(pickedHabits);
      }
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }
  onMount(() => {
    restore();
    load();
  });

  // ─── Derived ──────────────────────────────────────────────────────
  const activeScripture = $derived.by(() => {
    if (customScripture.trim()) return { text: customScripture.trim(), source: customSource.trim() };
    return scripture;
  });
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
    for (const t of openTasks) if (pickedTasks.has(t.id)) ts.push(t.text);
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
  function toggleTask(id: string) {
    if (pickedTasks.has(id)) pickedTasks.delete(id);
    else pickedTasks.add(id);
    pickedTasks = new Set(pickedTasks);
  }
  function toggleHabit(name: string) {
    if (pickedHabits.has(name)) pickedHabits.delete(name);
    else pickedHabits.add(name);
    pickedHabits = new Set(pickedHabits);
  }
  function toggleIntention(id: string) {
    if (pickedIntentions.has(id)) pickedIntentions.delete(id);
    else pickedIntentions.add(id);
    pickedIntentions = new Set(pickedIntentions);
  }
  function addCustomHabit(e: Event) {
    e.preventDefault();
    const n = newHabit.trim();
    if (!n) return;
    pickedHabits.add(n);
    pickedHabits = new Set(pickedHabits);
    knownHabits = [...knownHabits, {
      name: n, days: [], currentStreak: 0, longestStreak: 0,
      last7Pct: 0, last30Pct: 0, doneToday: false
    }];
    newHabit = '';
  }
  async function addNewPrayer(e: Event) {
    e.preventDefault();
    const text = newPrayerText.trim();
    if (!text || addingPrayer) return;
    addingPrayer = true;
    try {
      const created = await api.createPrayer({ text, status: 'praying' });
      activeIntentions = [created, ...activeIntentions];
      pickedIntentions.add(created.id);
      pickedIntentions = new Set(pickedIntentions);
      newPrayerText = '';
    } catch (err) {
      toast.error(`failed: ${err instanceof Error ? err.message : String(err)}`);
    } finally {
      addingPrayer = false;
    }
  }

  function focusContext(): string {
    return sortedTasks.slice(0, 12).map((t) => {
      const p = t.priority > 0 ? `P${t.priority} ` : '';
      const due = t.dueDate ? ` (due ${t.dueDate})` : '';
      return `- ${p}${t.text}${due}`;
    }).join('\n');
  }
  async function suggestFocus() {
    suggesting = true;
    suggestion = '';
    try {
      const ctx = focusContext();
      const userMsg = ctx
        ? `It's ${today}. My open tasks (top by priority + due date):\n\n${ctx}\n\nIf I only got ONE thing done today, what should it be? Reply with a single, action-oriented sentence (8–14 words). No preamble, no list, no quotes — just the sentence. Pick something concrete from the tasks above or, if nothing fits, propose a focused outcome.`
        : `It's ${today}. I haven't logged any open tasks. Suggest one focused outcome for today as a single action-oriented sentence (8–14 words). No preamble, no quotes.`;
      const r = await api.chat([{ role: 'user', content: userMsg }]);
      suggestion = r.message.content.trim().replace(/^["'`]+|["'`]+$/g, '').trim();
    } catch (e) {
      const raw = e instanceof Error ? e.message : String(e);
      const hint = classifyAiError(raw);
      toast.error(hint.headline, { action: hint.cta, details: hint.raw });
    } finally {
      suggesting = false;
    }
  }
  function acceptSuggestion() {
    if (suggestion) goal = suggestion;
    suggestion = '';
  }
  function pickGoalLink(g: Goal) {
    if (linkedGoalId === g.id) {
      linkedGoalId = '';
    } else {
      linkedGoalId = g.id;
      if (!goal.trim()) goal = g.title;
    }
  }

  async function lockIn() {
    saving = true;
    error = '';
    try {
      const linked = activeGoals.find((g) => g.id === linkedGoalId);
      const goalText = goal.trim();
      const goalForSave = goalText
        ? linked ? `${goalText} — contributes to: ${linked.title}` : goalText
        : undefined;

      // Prayer intentions ride along in the thoughts block under
      // 'Praying for:' (server has no dedicated prayer field — keeps
      // the daily note self-contained without a schema change).
      const prayerLines: string[] = [];
      for (const id of pickedIntentions) {
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
      const winLine = winSentence.trim();
      const winPart = winLine ? `Today's win: ${winLine}` : '';
      const thoughtsRaw = thoughts.trim();
      const thoughtsBody = [winPart, prayerBlock, thoughtsRaw]
        .filter((s) => s.length > 0)
        .join('\n\n') || undefined;

      await api.saveMorning({
        scripture: activeScripture.text ? activeScripture : undefined,
        goal: goalForSave,
        tasks: pickedTaskTexts,
        habits: Array.from(pickedHabits),
        thoughts: thoughtsBody
      });
      clearPersisted();
      toast.success('today is locked in');
      goto('/');
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      error = msg;
      toast.error(`save failed: ${msg}`);
    } finally {
      saving = false;
    }
  }

  // Counts the user can see on the lock-in button.
  const filledCount = $derived.by(() => {
    let n = 0;
    if (winSentence.trim()) n++;
    if (goal.trim()) n++;
    if (pickedTasks.size > 0) n++;
    if (pickedHabits.size > 0) n++;
    if (thoughts.trim() || pickedIntentions.size > 0) n++;
    return n;
  });
</script>

<div class="h-full overflow-y-auto bg-base flex flex-col">
  <div class="p-4 sm:p-6 lg:p-8 max-w-2xl w-full mx-auto flex-1">
    <!-- Header: greeting + scripture + date. The whole point of
         scripture being inline (not its own step) is that it's
         passive — read once, anchor the day, move on. -->
    <header class="mb-6 sm:mb-8">
      <div class="flex items-baseline justify-between gap-3 mb-3">
        <h1 class="text-2xl sm:text-3xl font-semibold text-text">{greeting}</h1>
        <a href="/" class="text-xs text-dim hover:text-text">skip ritual →</a>
      </div>
      <div class="text-xs text-dim">{dateLine}</div>

      {#if activeScripture.text}
        <blockquote class="mt-4 border-l-2 border-primary/60 pl-3 py-1.5 italic text-subtext text-sm">
          "{activeScripture.text}"
          {#if activeScripture.source}
            <span class="not-italic text-[11px] text-dim ml-2">— {activeScripture.source}</span>
          {/if}
        </blockquote>
        <button
          type="button"
          onclick={() => (scripturePickerOpen = !scripturePickerOpen)}
          class="text-[11px] text-dim hover:text-text mt-1"
        >
          {scripturePickerOpen ? 'hide picker' : 'change verse'}
        </button>
      {/if}

      {#if scripturePickerOpen}
        <div class="mt-3 p-3 bg-mantle/60 border border-surface1 rounded space-y-3">
          <div>
            <div class="text-[10px] uppercase tracking-wider text-dim mb-1.5">From rotation</div>
            <div class="flex flex-wrap gap-1.5 max-h-32 overflow-y-auto">
              {#each scriptures as s}
                <button
                  type="button"
                  onclick={() => { scripture = s; customScripture = ''; customSource = ''; }}
                  class="text-[11px] px-2 py-0.5 rounded border
                    {scripture === s && !customScripture ? 'border-primary text-primary' : 'border-surface1 text-subtext hover:border-primary/40'}"
                >{s.source}</button>
              {/each}
            </div>
          </div>
          <div>
            <div class="text-[10px] uppercase tracking-wider text-dim mb-1.5">Or paste your own</div>
            <input bind:value={customScripture} placeholder="quote / verse text"
              class="w-full px-2 py-1.5 mb-1.5 bg-surface0 border border-surface1 rounded text-sm" />
            <input bind:value={customSource} placeholder="source"
              class="w-full px-2 py-1.5 bg-surface0 border border-surface1 rounded text-sm" />
          </div>
        </div>
      {/if}
    </header>

    {#if error}
      <div class="mb-5 text-sm text-error p-3 bg-error/10 border border-error/30 rounded">{error}</div>
    {/if}

    <!-- Anchors strip — passive context. Active goals + nearest
         deadlines so the user starts grounded in what they're
         actually working towards before picking today's specifics. -->
    {#if activeGoals.length > 0 || upcomingNear.length > 0}
      <section class="mb-7 p-3 sm:p-4 rounded-lg bg-mantle/50 border border-surface1/60">
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
          <ul class="space-y-1.5 {activeGoals.length > 0 ? 'mt-2 pt-2 border-t border-surface1/60' : ''}">
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
        {#if winSentence.trim()}<span class="text-[10px] text-success">✓</span>{/if}
      </div>
      <input
        bind:value={winSentence}
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
          onclick={suggestFocus}
          disabled={suggesting}
          class="text-[11px] text-primary hover:underline disabled:opacity-50 inline-flex items-center gap-1"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-3 h-3">
            <path d="M12 3l1.2 4.2L17 9l-3.8 1.8L12 15l-1.2-4.2L7 9l3.8-1.8L12 3z" stroke-linejoin="round"/>
          </svg>
          {suggesting ? 'thinking…' : 'suggest from tasks'}
        </button>
      </div>
      <input
        bind:value={goal}
        placeholder="if you got one thing done…"
        class="w-full px-3 py-2.5 bg-surface0 border border-surface1 rounded text-text placeholder-dim focus:outline-none focus:border-primary"
      />
      {#if suggestion}
        <div class="mt-2 p-2.5 bg-primary/8 border-l-2 border-primary rounded text-sm">
          <div class="text-text mb-1.5">{suggestion}</div>
          <div class="flex items-center gap-2">
            <button onclick={acceptSuggestion} class="px-2 py-0.5 text-[11px] rounded bg-primary text-on-primary font-medium">use this</button>
            <button onclick={() => (suggestion = '')} class="px-2 py-0.5 text-[11px] text-dim hover:text-text">dismiss</button>
            <button onclick={suggestFocus} disabled={suggesting} class="ml-auto text-[11px] text-secondary hover:underline disabled:opacity-50">try again</button>
          </div>
        </div>
      {/if}
      {#if activeGoals.length > 0}
        <div class="flex flex-wrap items-center gap-1.5 mt-2.5">
          <span class="text-[11px] text-dim">contributes to:</span>
          {#each activeGoals as g (g.id)}
            {@const sel = linkedGoalId === g.id}
            <button
              type="button"
              onclick={() => pickGoalLink(g)}
              class="px-2 py-0.5 text-[11px] rounded-full border transition-colors
                {sel ? 'bg-primary/15 border-primary text-primary' : 'border-surface1 text-subtext hover:border-primary/40'}"
            >
              {sel ? '✓ ' : ''}{g.title}
            </button>
          {/each}
          {#if linkedGoalId}
            <button onclick={() => (linkedGoalId = '')} class="text-[11px] text-dim hover:text-text">clear</button>
          {/if}
        </div>
      {/if}
    </section>

    <!-- 3 · Tasks — compact picker, urgency-sorted -->
    <section class="mb-7">
      <div class="flex items-baseline justify-between mb-2">
        <h2 class="text-sm font-semibold text-text uppercase tracking-wider">Pick your tasks</h2>
        <span class="text-[11px] text-dim">{pickedTasks.size} picked</span>
      </div>
      {#if openTasks.length === 0}
        <p class="text-sm text-dim italic">no open tasks — <a href="/tasks" class="text-secondary hover:underline">add some</a></p>
      {:else}
        <ul class="space-y-0.5">
          {#each (showAllTasks ? sortedTasks : taskPreview) as t (t.id)}
            {@const sel = pickedTasks.has(t.id)}
            {@const overdueImp = !!t.dueDate && t.dueDate < today && (t.priority === 1 || t.priority === 2)}
            {@const dueToday = t.dueDate === today}
            <li>
              <button
                type="button"
                onclick={() => toggleTask(t.id)}
                class="w-full text-left flex items-baseline gap-2 px-2 py-1.5 rounded hover:bg-surface0 group"
              >
                <span class="w-4 h-4 mt-0.5 rounded border flex-shrink-0 flex items-center justify-center
                  {sel ? 'bg-primary border-primary' : 'border-surface2'}">
                  {#if sel}<svg viewBox="0 0 12 12" class="w-3 h-3 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>{/if}
                </span>
                {#if overdueImp}
                  <span class="text-[10px] font-mono px-1 rounded bg-error/20 text-error">!OVERDUE</span>
                {:else if dueToday}
                  <span class="text-[10px] font-mono px-1 rounded bg-primary/20 text-primary">TODAY</span>
                {/if}
                {#if t.priority > 0}
                  <span class="text-[10px] font-mono px-1 rounded
                    {t.priority === 1 ? 'bg-error/20 text-error' : ''}
                    {t.priority === 2 ? 'bg-warning/20 text-warning' : ''}
                    {t.priority === 3 ? 'bg-info/20 text-info' : ''}">P{t.priority}</span>
                {/if}
                <span class="flex-1 text-sm text-text">{@html inlineMd(t.text)}</span>
                {#if t.estimatedMinutes}<span class="text-[10px] text-dim">⏱ {t.estimatedMinutes}m</span>{/if}
                {#if t.dueDate}<span class="text-[11px] text-dim font-mono">{t.dueDate}</span>{/if}
              </button>
            </li>
          {/each}
        </ul>
        {#if !showAllTasks && taskOverflow > 0}
          <button
            type="button"
            onclick={() => (showAllTasks = true)}
            class="mt-1.5 text-[11px] text-secondary hover:underline"
          >show {taskOverflow} more</button>
        {:else if showAllTasks && sortedTasks.length > 8}
          <button
            type="button"
            onclick={() => (showAllTasks = false)}
            class="mt-1.5 text-[11px] text-dim hover:text-text"
          >collapse</button>
        {/if}
      {/if}
    </section>

    <!-- 4 · Habits — compact horizontal grid -->
    {#if knownHabits.length > 0 || newHabit.length === 0}
      <section class="mb-7">
        <div class="flex items-baseline justify-between mb-2">
          <h2 class="text-sm font-semibold text-text uppercase tracking-wider">Habits</h2>
          <span class="text-[11px] text-dim">{pickedHabits.size} committed</span>
        </div>
        {#if knownHabits.length > 0}
          <div class="flex flex-wrap gap-1.5 mb-2">
            {#each knownHabits as h (h.name)}
              {@const sel = pickedHabits.has(h.name)}
              <button
                type="button"
                onclick={() => toggleHabit(h.name)}
                class="px-2.5 py-1 text-[11px] rounded-full border transition-colors inline-flex items-center gap-1.5
                  {sel ? 'bg-primary/15 border-primary text-primary' : 'border-surface1 text-subtext hover:border-primary/40'}"
              >
                <span>{sel ? '☑' : '☐'}</span>
                <span>{h.name}</span>
                {#if h.currentStreak > 0}<span class="text-warning">🔥{h.currentStreak}</span>{/if}
              </button>
            {/each}
          </div>
        {/if}
        <form onsubmit={addCustomHabit} class="flex gap-2">
          <input
            bind:value={newHabit}
            placeholder="add a habit…"
            class="flex-1 px-2.5 py-1.5 bg-surface0 border border-surface1 rounded text-sm"
          />
          <button type="submit" disabled={!newHabit.trim()} class="px-2.5 py-1.5 bg-surface1 text-subtext rounded text-sm disabled:opacity-50">+ add</button>
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
            onclick={() => (prayerPickerOpen = !prayerPickerOpen)}
            class="text-[11px] text-secondary hover:underline"
          >
            {prayerPickerOpen ? 'hide intentions' : `${pickedIntentions.size} of ${activeIntentions.length} intentions`}
          </button>
        {/if}
      </div>
      <textarea
        bind:value={thoughts}
        rows="3"
        placeholder="grateful for · wrestling with · today's mood · what to bring before God"
        class="w-full px-3 py-2.5 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary leading-relaxed"
      ></textarea>
      {#if prayerPickerOpen}
        <div class="mt-3 p-3 bg-mantle/40 border border-surface1 rounded space-y-2">
          {#if sortedIntentions.length > 0}
            <ul class="space-y-1 max-h-48 overflow-y-auto">
              {#each sortedIntentions as p (p.id)}
                {@const picked = pickedIntentions.has(p.id)}
                <li>
                  <button
                    type="button"
                    onclick={() => toggleIntention(p.id)}
                    class="w-full text-left flex items-start gap-2 px-2 py-1.5 rounded
                      {picked ? 'bg-primary/10' : 'hover:bg-surface0'}"
                  >
                    <span class="w-3.5 h-3.5 mt-0.5 rounded border flex-shrink-0 flex items-center justify-center text-[9px]
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
          <form onsubmit={addNewPrayer} class="flex gap-2">
            <input
              bind:value={newPrayerText}
              placeholder="add an intention…"
              class="flex-1 px-2 py-1.5 bg-surface0 border border-surface1 rounded text-sm"
            />
            <button type="submit" disabled={!newPrayerText.trim() || addingPrayer} class="px-2.5 py-1.5 bg-primary text-on-primary rounded text-sm disabled:opacity-50">
              {addingPrayer ? '…' : '+ add'}
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
  <footer class="sticky bottom-0 z-20 border-t border-surface1 bg-mantle/95 backdrop-blur px-4 py-3 pb-[calc(0.75rem+env(safe-area-inset-bottom,0px))]">
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
