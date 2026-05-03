<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { auth } from '$lib/stores/auth';
  import { api, todayISO, type Task, type HabitInfo, type Goal, type Deadline } from '$lib/api';
  import { scriptures, scriptureOfTheDay } from '$lib/morning/scriptures';
  import { inlineMd } from '$lib/util/inlineMd';
  import { toast } from '$lib/components/toast';
  import { classifyAiError } from '$lib/util/aiErrors';
  import DeadlinePill from '$lib/deadlines/DeadlinePill.svelte';

  // The wizard runs in 7 steps now. The new "anchors" step opens the
  // routine with a read-only review of what the user is currently
  // working towards (active goals, today's habits, near-term deadlines)
  // — so they enter the planning loop already grounded in the bigger
  // picture instead of staring at a blank scripture.
  type Step = 'anchors' | 'scripture' | 'goal' | 'tasks' | 'habits' | 'thoughts' | 'review';
  const order: Step[] = ['anchors', 'scripture', 'goal', 'tasks', 'habits', 'thoughts', 'review'];

  let step = $state<Step>('anchors');

  // Step: Anchors
  let activeGoals = $state<Goal[]>([]);
  let allGoalsById = $state<Record<string, string>>({});
  let upcomingDeadlines = $state<Deadline[] | null>(null);
  let deadlinesLoaded = $state(false);

  // Step 1: Scripture
  let scripture = $state(scriptureOfTheDay());
  let customScripture = $state('');
  let customSource = $state('');

  // Step 2: Goal
  let goal = $state('');
  let linkedGoalId = $state<string>(''); // wires today's goal to a granit Goal

  // Step 3: Tasks
  let openTasks = $state<Task[]>([]);
  let pickedTasks = $state<Set<string>>(new Set());

  // Step 4: Habits
  let knownHabits = $state<HabitInfo[]>([]);
  let pickedHabits = $state<Set<string>>(new Set());
  let newHabit = $state('');

  // Step 5: Thoughts
  let thoughts = $state('');

  // Review: the user's win-condition for the day. Persisted with the
  // snapshot and rendered in the saved Plan block.
  let winSentence = $state('');

  // Submission
  let saving = $state(false);
  let error = $state('');

  let suggesting = $state(false);
  let suggestion = $state('');

  // Persist progress through the wizard so a closed tab / phone lock
  // doesn't lose what the user already picked. Keyed per-day so yesterday's
  // half-completed wizard doesn't bleed into today.
  const today = todayISO();
  const STORAGE_KEY = `granit.morning.${today}`;

  interface Snapshot {
    step: Step;
    scriptureSource: string;
    customScripture: string;
    customSource: string;
    goal: string;
    linkedGoalId: string;
    pickedTasks: string[];
    pickedHabits: string[];
    newHabit: string;
    thoughts: string;
    winSentence: string;
  }

  function snapshot(): Snapshot {
    return {
      step,
      scriptureSource: scripture.source,
      customScripture,
      customSource,
      goal,
      linkedGoalId,
      pickedTasks: [...pickedTasks],
      pickedHabits: [...pickedHabits],
      newHabit,
      thoughts,
      winSentence
    };
  }

  function persist() {
    try { localStorage.setItem(STORAGE_KEY, JSON.stringify(snapshot())); } catch {}
  }

  function restore() {
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      if (!raw) return;
      const s = JSON.parse(raw) as Snapshot;
      if (s.step) step = s.step;
      if (s.scriptureSource) {
        const m = scriptures.find((x) => x.source === s.scriptureSource);
        if (m) scripture = m;
      }
      customScripture = s.customScripture ?? '';
      customSource = s.customSource ?? '';
      goal = s.goal ?? '';
      linkedGoalId = s.linkedGoalId ?? '';
      pickedTasks = new Set(s.pickedTasks ?? []);
      pickedHabits = new Set(s.pickedHabits ?? []);
      newHabit = s.newHabit ?? '';
      thoughts = s.thoughts ?? '';
      winSentence = s.winSentence ?? '';
    } catch {}
  }

  function clearPersisted() {
    try { localStorage.removeItem(STORAGE_KEY); } catch {}
  }

  // Auto-persist on every change to a tracked field.
  $effect(() => {
    void step;
    void scripture;
    void customScripture;
    void customSource;
    void goal;
    void linkedGoalId;
    void pickedTasks;
    void pickedHabits;
    void newHabit;
    void thoughts;
    void winSentence;
    persist();
  });

  async function load() {
    if (!$auth) return;
    try {
      const [t, h, g, d] = await Promise.all([
        api.listTasks({ status: 'open' }),
        api.listHabits(),
        api.listGoals().catch((): { goals: Goal[]; total: number } => ({ goals: [], total: 0 })),
        // tryListDeadlines never throws — returns null when unavailable.
        api.tryListDeadlines()
      ]);
      openTasks = t.tasks;
      knownHabits = h.habits;
      activeGoals = g.goals.filter((x) => (x.status ?? 'active') === 'active').slice(0, 3);
      const map: Record<string, string> = {};
      for (const goalEntry of g.goals) map[goalEntry.id] = goalEntry.title;
      allGoalsById = map;
      upcomingDeadlines = d;
      deadlinesLoaded = true;
      // Pre-tick today's habits that are usually done — but ONLY if we
      // haven't restored from a snapshot (otherwise we'd overwrite the
      // user's deliberate choices).
      const restored = !!localStorage.getItem(STORAGE_KEY);
      if (!restored) {
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

  function next() {
    const i = order.indexOf(step);
    if (i < order.length - 1) step = order[i + 1];
  }
  function back() {
    const i = order.indexOf(step);
    if (i > 0) step = order[i - 1];
  }
  function jumpTo(s: Step) {
    step = s;
  }

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

  // From the anchors step the user can mark a habit done immediately —
  // if it's already done, no-op. Server creates the daily file when
  // needed.
  let anchorBusy = $state<Record<string, boolean>>({});
  async function markHabitDoneNow(h: HabitInfo) {
    if (h.doneToday) return;
    anchorBusy[h.name] = true;
    try {
      await api.toggleHabit(h.name, today, true);
      // Optimistic local update — the WS event will reconcile.
      knownHabits = knownHabits.map((x) =>
        x.name === h.name
          ? { ...x, doneToday: true, currentStreak: (x.currentStreak ?? 0) + (x.doneToday ? 0 : 1) }
          : x
      );
      pickedHabits.add(h.name);
      pickedHabits = new Set(pickedHabits);
    } catch (e) {
      toast.error(`failed: ${e instanceof Error ? e.message : String(e)}`);
    } finally {
      anchorBusy[h.name] = false;
    }
  }

  let activeScripture = $derived.by(() => {
    if (customScripture.trim()) {
      return { text: customScripture.trim(), source: customSource.trim() };
    }
    return scripture;
  });

  let pickedTaskTexts = $derived.by(() => {
    const ts: string[] = [];
    for (const t of openTasks) if (pickedTasks.has(t.id)) ts.push(t.text);
    return ts;
  });

  // Build a compact context string from open tasks for the AI suggestion
  // call. Only the top-priority / soonest-due items — sending 60 task
  // lines blows the prompt without changing the answer materially.
  function focusContext(): string {
    const top = sortedTasks.slice(0, 12).map((t) => {
      const p = t.priority > 0 ? `P${t.priority} ` : '';
      const due = t.dueDate ? ` (due ${t.dueDate})` : '';
      return `- ${p}${t.text}${due}`;
    });
    return top.join('\n');
  }

  async function suggestFocus() {
    suggesting = true;
    suggestion = '';
    try {
      const ctx = focusContext();
      const today = todayISO();
      const userMsg = ctx
        ? `It's ${today}. My open tasks (top by priority + due date):\n\n${ctx}\n\n` +
          `If I only got ONE thing done today, what should it be? Reply with a single, action-oriented sentence ` +
          `(8–14 words). No preamble, no list, no quotes — just the sentence. Pick something concrete from the tasks above ` +
          `or, if nothing fits, propose a focused outcome.`
        : `It's ${today}. I haven't logged any open tasks. Suggest one focused outcome for today as a single ` +
          `action-oriented sentence (8–14 words). No preamble, no quotes.`;
      const r = await api.chat([{ role: 'user', content: userMsg }]);
      let s = r.message.content.trim();
      s = s.replace(/^["'`]+|["'`]+$/g, '').trim();
      suggestion = s;
    } catch (e) {
      const raw = e instanceof Error ? e.message : String(e);
      console.error('[morning] suggestFocus failed:', raw);
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
      // Prefill the daily goal with the goal's title if the input is
      // empty — the user can still rewrite it. This makes the chip act
      // as a "today contributes to" shortcut instead of an opaque tag.
      if (!goal.trim()) goal = g.title;
    }
  }

  async function save() {
    saving = true;
    error = '';
    try {
      // Compose a richer goal string when the user linked a goal — the
      // server save endpoint takes plain strings so we encode the link
      // inline. Format chosen so it's still readable as a daily-note
      // bullet without further parsing.
      const linked = activeGoals.find((g) => g.id === linkedGoalId);
      const goalText = goal.trim();
      const goalForSave = goalText
        ? linked
          ? `${goalText} — contributes to: ${linked.title}`
          : goalText
        : undefined;
      // The win sentence rides along in the thoughts block (server has
      // no dedicated field). Prepend it so it shows up first when the
      // user reopens the daily note.
      const winLine = winSentence.trim();
      const thoughtsBody = winLine
        ? `Today's win: ${winLine}${thoughts.trim() ? `\n\n${thoughts.trim()}` : ''}`
        : (thoughts.trim() || undefined);

      await api.saveMorning({
        scripture: activeScripture.text ? activeScripture : undefined,
        goal: goalForSave,
        tasks: pickedTaskTexts,
        habits: Array.from(pickedHabits),
        thoughts: thoughtsBody
      });
      clearPersisted();
      toast.success("daily plan saved");
      goto('/');
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      error = msg;
      toast.error(`save failed: ${msg}`);
    } finally {
      saving = false;
    }
  }

  // Sort tasks for the picker:
  //   1. overdue P1/P2 (most-overdue first)
  //   2. due today
  //   3. quick wins (no scheduled, ≤30 min estimate or short text)
  //   4. everything else by priority then due-soon
  // The user picks from the top — and the most pressing items are at
  // the top.
  let sortedTasks = $derived.by(() => {
    const todayStr = todayISO();
    type Bucketed = { task: Task; bucket: number; rank: number };
    const isOverdueImportant = (t: Task) =>
      !!t.dueDate && t.dueDate < todayStr && (t.priority === 1 || t.priority === 2);
    const isDueToday = (t: Task) => t.dueDate === todayStr;
    const isQuickWin = (t: Task) =>
      !t.scheduledStart &&
      (!t.dueDate || t.dueDate >= todayStr) &&
      (((t.estimatedMinutes ?? 0) > 0 && (t.estimatedMinutes ?? 0) <= 30) ||
        (t.text.length <= 60 && (t.priority === 0 || t.priority >= 3)));

    const bucketed: Bucketed[] = openTasks.map((t) => {
      let bucket = 9;
      if (isOverdueImportant(t)) bucket = 0;
      else if (isDueToday(t)) bucket = 1;
      else if (isQuickWin(t)) bucket = 2;
      const rank =
        bucket === 0
          ? -((todayStr.localeCompare(t.dueDate ?? '~')) || 0) // more overdue → smaller (more negative) rank
          : (t.priority || 99) * 100 + (t.dueDate ? Number(t.dueDate.replace(/-/g, '').slice(2)) : 999_999);
      return { task: t, bucket, rank };
    });

    return bucketed
      .sort((a, b) => (a.bucket !== b.bucket ? a.bucket - b.bucket : a.rank - b.rank))
      .slice(0, 60)
      .map((x) => x.task);
  });

  // Anchors-step helpers
  function daysUntil(iso: string): number {
    const [y, m, d] = iso.split('-').map(Number);
    const due = new Date(y, m - 1, d);
    const t = new Date();
    t.setHours(0, 0, 0, 0);
    return Math.round((due.getTime() - t.getTime()) / 86_400_000);
  }
  function goalProgress(g: Goal): number {
    const ms = g.milestones ?? [];
    if (!ms || ms.length === 0) return 0;
    return Math.round((ms.filter((m) => m.done).length / ms.length) * 100);
  }
  let upcomingNear = $derived.by(() => {
    if (!upcomingDeadlines) return [];
    return upcomingDeadlines
      .filter((d) => d.status !== 'cancelled' && d.status !== 'met')
      .map((d) => ({ d, days: daysUntil(d.date) }))
      .filter((x) => x.days <= 14)
      .sort((a, b) => a.days - b.days)
      .slice(0, 5);
  });

  const stepLabels: Record<Step, string> = {
    anchors: 'Anchors',
    scripture: 'Scripture',
    goal: 'Today\'s Goal',
    tasks: 'Tasks',
    habits: 'Habits',
    thoughts: 'Thoughts',
    review: 'Review'
  };
</script>

<div class="h-full overflow-y-auto">
  <div class="p-4 sm:p-6 lg:p-8 max-w-3xl mx-auto">
    <header class="mb-5 sm:mb-6">
      <h1 class="text-2xl sm:text-3xl font-semibold text-text">Morning routine</h1>
      <p class="text-sm text-dim mt-1">
        Plan today in {order.length} quick steps. Saves a <code class="text-xs">## Daily Plan</code> block to your daily note.
      </p>
    </header>

    <!-- Stepper -->
    <nav class="flex flex-wrap items-center gap-1.5 mb-6 text-xs">
      {#each order as s, i}
        {@const active = step === s}
        {@const past = order.indexOf(step) > i}
        <button
          onclick={() => jumpTo(s)}
          class="px-2.5 py-1 rounded transition-colors
            {active ? 'bg-primary text-mantle' : ''}
            {past ? 'text-success' : ''}
            {!active && !past ? 'text-dim hover:text-text bg-surface0' : ''}"
        >
          {i + 1}. {stepLabels[s]}
        </button>
        {#if i < order.length - 1}
          <span class="text-dim/50">·</span>
        {/if}
      {/each}
    </nav>

    {#if error}
      <div class="text-sm text-error mb-4 p-3 bg-error/10 border border-error/30 rounded">{error}</div>
    {/if}

    <main class="bg-surface0 border border-surface1 rounded-lg p-5 sm:p-6 min-h-[24rem]">
      {#if step === 'anchors'}
        <h2 class="text-lg font-medium text-text mb-1">Today's anchors</h2>
        <p class="text-sm text-dim mb-4">What you're working towards, what you've committed to, what's coming up.</p>

        <!-- Active goals -->
        <section class="mb-5">
          <div class="flex items-baseline justify-between mb-2">
            <h3 class="text-xs uppercase tracking-wider text-dim">Active goals</h3>
            <a href="/goals" class="text-xs text-secondary hover:underline">all →</a>
          </div>
          {#if activeGoals.length === 0}
            <p class="text-sm text-dim italic">No active goals. <a href="/goals" class="text-secondary hover:underline">Set one →</a></p>
          {:else}
            <ul class="space-y-2">
              {#each activeGoals as g (g.id)}
                {@const p = goalProgress(g)}
                <li class="bg-mantle/40 border border-surface1/60 rounded p-2.5">
                  <div class="flex items-baseline gap-2">
                    <span class="text-sm text-text flex-1 truncate">{@html inlineMd(g.title)}</span>
                    {#if g.target_date}
                      <span class="text-[10px] text-dim font-mono">🎯 {g.target_date}</span>
                    {/if}
                  </div>
                  {#if g.milestones && g.milestones.length > 0}
                    <div class="h-1.5 bg-surface1 rounded-full overflow-hidden mt-1.5">
                      <div class="h-full bg-primary" style="width: {p}%"></div>
                    </div>
                    <div class="text-[10px] text-dim mt-0.5">{p}% · {g.milestones.filter((m) => m.done).length}/{g.milestones.length}</div>
                  {/if}
                </li>
              {/each}
            </ul>
          {/if}
        </section>

        <!-- Habits checklist -->
        <section class="mb-5">
          <div class="flex items-baseline justify-between mb-2">
            <h3 class="text-xs uppercase tracking-wider text-dim">Today's habits</h3>
            <a href="/habits" class="text-xs text-secondary hover:underline">all →</a>
          </div>
          {#if knownHabits.length === 0}
            <p class="text-sm text-dim italic">No habits tracked. <a href="/habits" class="text-secondary hover:underline">Add one →</a></p>
          {:else}
            <ul class="space-y-1">
              {#each knownHabits.slice(0, 6) as h (h.name)}
                <li class="flex items-center gap-2 text-sm">
                  <button
                    onclick={() => markHabitDoneNow(h)}
                    disabled={anchorBusy[h.name] || h.doneToday}
                    aria-label={h.doneToday ? `${h.name} done` : `mark ${h.name} done`}
                    class="w-4 h-4 rounded border flex-shrink-0 flex items-center justify-center transition-colors
                      {h.doneToday ? 'bg-success border-success' : 'border-surface2 hover:border-primary'}"
                  >
                    {#if h.doneToday}
                      <svg viewBox="0 0 12 12" class="w-3 h-3 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
                    {/if}
                  </button>
                  <span class="flex-1 text-text truncate {h.doneToday ? 'opacity-60' : ''}">{h.name}</span>
                  {#if h.currentStreak > 0}<span class="text-xs text-warning">🔥 {h.currentStreak}</span>{/if}
                  <span class="text-[11px] text-dim tabular-nums">L7 {h.last7Pct}%</span>
                </li>
              {/each}
            </ul>
          {/if}
        </section>

        <!-- Deadlines (defensive — only renders if endpoint shipped) -->
        {#if deadlinesLoaded && upcomingDeadlines !== null && upcomingNear.length > 0}
          <section class="mb-2">
            <div class="flex items-baseline justify-between mb-2">
              <h3 class="text-xs uppercase tracking-wider text-dim">Upcoming deadlines · 14 days</h3>
              <a href="/goals" class="text-xs text-secondary hover:underline">all →</a>
            </div>
            <ul class="space-y-1.5">
              {#each upcomingNear as { d, days } (d.id)}
                <li class="flex items-baseline gap-2">
                  <DeadlinePill variant="countdown" {days} status={d.status} />
                  <DeadlinePill variant="icon" importance={d.importance} />
                  <span class="text-sm text-text flex-1 truncate">{d.title}</span>
                  {#if d.goal_id && allGoalsById[d.goal_id]}
                    <span class="text-[11px] text-dim truncate">🎯 {allGoalsById[d.goal_id]}</span>
                  {:else if d.project}
                    <span class="text-[11px] text-dim truncate">⏵ {d.project}</span>
                  {/if}
                </li>
              {/each}
            </ul>
          </section>
        {/if}

        <p class="text-[11px] text-dim mt-4 italic">Press <kbd class="text-[10px]">next →</kbd> when you're grounded.</p>
      {:else if step === 'scripture'}
        <h2 class="text-lg font-medium text-text mb-1">Scripture</h2>
        <p class="text-sm text-dim mb-4">A line to anchor your day.</p>

        <blockquote class="border-l-2 border-primary pl-4 py-2 my-4 italic text-subtext">
          "{activeScripture.text}"
          {#if activeScripture.source}
            <div class="not-italic text-xs text-dim mt-1">— {activeScripture.source}</div>
          {/if}
        </blockquote>

        <div class="text-xs text-dim mt-4 mb-2">Pick from rotation</div>
        <div class="flex flex-wrap gap-2 max-h-40 overflow-y-auto mb-4">
          {#each scriptures as s}
            <button
              onclick={() => { scripture = s; customScripture = ''; customSource = ''; }}
              class="text-xs px-2 py-1 rounded border
                {scripture === s && !customScripture ? 'border-primary text-primary' : 'border-surface1 text-subtext hover:border-primary/40'}"
            >
              {s.source}
            </button>
          {/each}
        </div>

        <div class="text-xs text-dim mt-2 mb-2">Or paste your own</div>
        <input bind:value={customScripture} placeholder="quote / verse text" class="w-full px-3 py-2 mb-2 bg-mantle border border-surface1 rounded text-base sm:text-sm text-text placeholder-dim focus:outline-none focus:border-primary" />
        <input bind:value={customSource} placeholder="source (e.g. Proverbs 27:17)" class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-base sm:text-sm text-text placeholder-dim focus:outline-none focus:border-primary" />
      {:else if step === 'goal'}
        <h2 class="text-lg font-medium text-text mb-1">Today's #1 goal</h2>
        <p class="text-sm text-dim mb-4">If you only got one thing done today, what should it be?</p>
        <input
          bind:value={goal}
          placeholder="ship the mealtime onboarding flow"
          class="w-full px-3 py-3 bg-mantle border border-surface1 rounded text-base text-text placeholder-dim focus:outline-none focus:border-primary"
        />
        <div class="mt-3 flex items-center gap-2">
          <button
            onclick={suggestFocus}
            disabled={suggesting}
            class="px-3 py-1.5 text-xs rounded border border-primary/40 text-primary hover:bg-primary/10 disabled:opacity-50"
          >
            {suggesting ? 'thinking…' : '✨ Suggest based on my tasks'}
          </button>
          <span class="text-[11px] text-dim italic">uses your AI provider · single short sentence</span>
        </div>
        {#if suggestion}
          <div class="mt-3 p-3 bg-primary/8 border-l-3 border-primary rounded">
            <div class="text-[10px] uppercase tracking-wider text-primary mb-1">Suggested</div>
            <p class="text-sm text-text mb-2">{suggestion}</p>
            <div class="flex items-center gap-2">
              <button
                onclick={acceptSuggestion}
                class="px-2.5 py-1 text-xs rounded bg-primary text-mantle font-medium"
              >use this</button>
              <button
                onclick={() => (suggestion = '')}
                class="px-2.5 py-1 text-xs rounded text-dim hover:text-text"
              >dismiss</button>
              <button
                onclick={suggestFocus}
                disabled={suggesting}
                class="ml-auto text-xs text-secondary hover:underline disabled:opacity-50"
              >try again</button>
            </div>
          </div>
        {/if}

        {#if activeGoals.length > 0}
          <div class="mt-5 pt-4 border-t border-surface1">
            <div class="text-xs uppercase tracking-wider text-dim mb-2">Today contributes to</div>
            <div class="flex flex-wrap gap-1.5">
              {#each activeGoals as g (g.id)}
                {@const sel = linkedGoalId === g.id}
                <button
                  onclick={() => pickGoalLink(g)}
                  class="px-2.5 py-1 text-xs rounded border transition-colors
                    {sel ? 'bg-primary/15 border-primary text-primary' : 'border-surface1 text-subtext hover:border-primary/40'}"
                >
                  {sel ? '✓ ' : '🎯 '}{g.title}
                </button>
              {/each}
              {#if linkedGoalId}
                <button
                  onclick={() => (linkedGoalId = '')}
                  class="px-2.5 py-1 text-xs text-dim hover:text-text"
                >clear link</button>
              {/if}
            </div>
            <p class="text-[11px] text-dim mt-2 italic">Saved as <code class="text-[10px]">— contributes to: …</code> on the goal line.</p>
          </div>
        {/if}
      {:else if step === 'tasks'}
        <h2 class="text-lg font-medium text-text mb-1">Tasks for today</h2>
        <p class="text-sm text-dim mb-4">{pickedTasks.size} selected · pick what you'll actually commit to. Sorted by urgency.</p>
        {#if openTasks.length === 0}
          <div class="text-sm text-dim italic">no open tasks — add some first or skip this step</div>
        {:else}
          <ul class="space-y-1 max-h-[24rem] overflow-y-auto">
            {#each sortedTasks as t (t.id)}
              {@const sel = pickedTasks.has(t.id)}
              {@const overdueImp = !!t.dueDate && t.dueDate < today && (t.priority === 1 || t.priority === 2)}
              {@const dueToday = t.dueDate === today}
              <li>
                <button
                  onclick={() => toggleTask(t.id)}
                  class="w-full text-left flex items-baseline gap-2 px-2 py-1.5 rounded hover:bg-surface1 group"
                >
                  <span class="w-4 h-4 mt-0.5 rounded border flex-shrink-0 flex items-center justify-center
                    {sel ? 'bg-primary border-primary' : 'border-surface2'}">
                    {#if sel}
                      <svg viewBox="0 0 12 12" class="w-3 h-3 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
                    {/if}
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
                  {#if t.dueDate}<span class="text-xs text-dim">{t.dueDate}</span>{/if}
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      {:else if step === 'habits'}
        <h2 class="text-lg font-medium text-text mb-1">Habits</h2>
        <p class="text-sm text-dim mb-4">Which ones are non-negotiable today?</p>

        {#if knownHabits.length > 0}
          <ul class="space-y-1 mb-4">
            {#each knownHabits as h (h.name)}
              {@const sel = pickedHabits.has(h.name)}
              <li>
                <button
                  onclick={() => toggleHabit(h.name)}
                  class="w-full text-left flex items-baseline gap-2 px-2 py-1.5 rounded hover:bg-surface1"
                >
                  <span class="w-4 h-4 mt-0.5 rounded border flex-shrink-0 flex items-center justify-center
                    {sel ? 'bg-primary border-primary' : 'border-surface2'}">
                    {#if sel}<svg viewBox="0 0 12 12" class="w-3 h-3 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>{/if}
                  </span>
                  <span class="flex-1 text-sm text-text">{h.name}</span>
                  {#if h.currentStreak > 0}<span class="text-xs text-warning">🔥 {h.currentStreak}</span>{/if}
                  <span class="text-xs text-dim">L7 {h.last7Pct}%</span>
                </button>
              </li>
            {/each}
          </ul>
        {/if}

        <form onsubmit={addCustomHabit} class="flex gap-2 mt-2">
          <input
            bind:value={newHabit}
            placeholder="add a new habit…"
            class="flex-1 px-2 py-2 bg-mantle border border-surface1 rounded text-base sm:text-sm text-text focus:outline-none focus:border-primary"
          />
          <button type="submit" disabled={!newHabit.trim()} class="px-3 py-2 bg-surface1 text-subtext rounded text-sm disabled:opacity-50">+ add</button>
        </form>
      {:else if step === 'thoughts'}
        <h2 class="text-lg font-medium text-text mb-1">Thoughts</h2>
        <p class="text-sm text-dim mb-4">Anything on your mind. Two sentences works.</p>
        <textarea
          bind:value={thoughts}
          rows="8"
          placeholder="grateful for… / wrestling with… / today's mood…"
          class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-base sm:text-sm text-text placeholder-dim focus:outline-none focus:border-primary leading-relaxed font-mono"
        ></textarea>
      {:else if step === 'review'}
        <h2 class="text-lg font-medium text-text mb-1">Review</h2>
        <p class="text-sm text-dim mb-4">This is what we'll save under <code class="text-xs">## Daily Plan</code>.</p>

        <!-- Win-condition prompt -->
        <div class="mb-4 p-3 bg-mantle/60 border border-primary/30 rounded">
          <label class="block text-xs uppercase tracking-wider text-primary mb-1.5" for="win-input">
            What would make today a win?
          </label>
          <input
            id="win-input"
            bind:value={winSentence}
            placeholder="one sentence — concrete, finishable today"
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
          />
          <p class="text-[11px] text-dim mt-1.5 italic">Saved with your thoughts so you can re-read it tonight.</p>
        </div>

        <div class="bg-mantle border border-surface1 rounded p-4 space-y-3 text-sm">
          {#if activeScripture.text}
            <blockquote class="border-l-2 border-primary pl-3 italic text-subtext">
              "{activeScripture.text}"
              {#if activeScripture.source}<div class="text-xs text-dim mt-1 not-italic">— {activeScripture.source}</div>{/if}
            </blockquote>
          {/if}
          {#if winSentence.trim()}
            <div>
              <div class="text-xs uppercase tracking-wider text-primary mb-1">Today's win</div>
              <div class="font-medium text-text">{winSentence}</div>
            </div>
          {/if}
          {#if goal.trim()}
            <div>
              <div class="text-xs uppercase tracking-wider text-dim mb-1">Today's Goal</div>
              <div class="font-medium text-text">{goal}</div>
              {#if linkedGoalId}
                {@const g = activeGoals.find((x) => x.id === linkedGoalId)}
                {#if g}
                  <div class="text-[11px] text-secondary mt-0.5">→ contributes to: {g.title}</div>
                {/if}
              {/if}
            </div>
          {/if}
          {#if pickedTaskTexts.length > 0}
            <div>
              <div class="text-xs uppercase tracking-wider text-dim mb-1">Tasks ({pickedTaskTexts.length})</div>
              <ul class="space-y-0.5 text-text">
                {#each pickedTaskTexts as t}<li>· {@html inlineMd(t)}</li>{/each}
              </ul>
            </div>
          {/if}
          {#if pickedHabits.size > 0}
            <div>
              <div class="text-xs uppercase tracking-wider text-dim mb-1">Habits ({pickedHabits.size})</div>
              <ul class="space-y-0.5 text-text">
                {#each [...pickedHabits] as h}<li>☐ {h}</li>{/each}
              </ul>
            </div>
          {/if}
          {#if thoughts.trim()}
            <div>
              <div class="text-xs uppercase tracking-wider text-dim mb-1">Thoughts</div>
              <div class="text-subtext whitespace-pre-wrap">{thoughts}</div>
            </div>
          {/if}
        </div>
      {/if}
    </main>

    <div class="flex items-center justify-between mt-5">
      <button
        onclick={back}
        disabled={step === 'anchors'}
        class="px-4 py-2 text-sm text-subtext disabled:opacity-30"
      >
        ← back
      </button>
      {#if step === 'review'}
        <button
          onclick={save}
          disabled={saving}
          class="px-5 py-2 bg-primary text-mantle rounded text-sm font-medium disabled:opacity-50"
        >
          {saving ? 'saving…' : 'save to today\'s daily note'}
        </button>
      {:else}
        <button
          onclick={next}
          class="px-4 py-2 bg-primary text-mantle rounded text-sm font-medium"
        >
          next →
        </button>
      {/if}
    </div>
  </div>
</div>
