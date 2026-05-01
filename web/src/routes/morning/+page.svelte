<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { auth } from '$lib/stores/auth';
  import { api, type Task, type HabitInfo } from '$lib/api';
  import { scriptures, scriptureOfTheDay } from '$lib/morning/scriptures';
  import { inlineMd } from '$lib/util/inlineMd';
  import { toast } from '$lib/components/toast';

  type Step = 'scripture' | 'goal' | 'tasks' | 'habits' | 'thoughts' | 'review';
  const order: Step[] = ['scripture', 'goal', 'tasks', 'habits', 'thoughts', 'review'];

  let step = $state<Step>('scripture');

  // Step 1: Scripture
  let scripture = $state(scriptureOfTheDay());
  let customScripture = $state('');
  let customSource = $state('');

  // Step 2: Goal
  let goal = $state('');

  // Step 3: Tasks
  let openTasks = $state<Task[]>([]);
  let pickedTasks = $state<Set<string>>(new Set());

  // Step 4: Habits
  let knownHabits = $state<HabitInfo[]>([]);
  let pickedHabits = $state<Set<string>>(new Set());
  let newHabit = $state('');

  // Step 5: Thoughts
  let thoughts = $state('');

  // Submission
  let saving = $state(false);
  let error = $state('');

  // Persist progress through the wizard so a closed tab / phone lock
  // doesn't lose what the user already picked. Keyed per-day so yesterday's
  // half-completed wizard doesn't bleed into today.
  const today = new Date().toISOString().slice(0, 10);
  const STORAGE_KEY = `granit.morning.${today}`;

  interface Snapshot {
    step: Step;
    scriptureSource: string;
    customScripture: string;
    customSource: string;
    goal: string;
    pickedTasks: string[];
    pickedHabits: string[];
    newHabit: string;
    thoughts: string;
  }

  function snapshot(): Snapshot {
    return {
      step,
      scriptureSource: scripture.source,
      customScripture,
      customSource,
      goal,
      pickedTasks: [...pickedTasks],
      pickedHabits: [...pickedHabits],
      newHabit,
      thoughts
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
      pickedTasks = new Set(s.pickedTasks ?? []);
      pickedHabits = new Set(s.pickedHabits ?? []);
      newHabit = s.newHabit ?? '';
      thoughts = s.thoughts ?? '';
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
    void pickedTasks;
    void pickedHabits;
    void newHabit;
    void thoughts;
    persist();
  });

  async function load() {
    if (!$auth) return;
    try {
      const [t, h] = await Promise.all([api.listTasks({ status: 'open' }), api.listHabits()]);
      openTasks = t.tasks;
      knownHabits = h.habits;
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

  async function save() {
    saving = true;
    error = '';
    try {
      await api.saveMorning({
        scripture: activeScripture.text ? activeScripture : undefined,
        goal: goal.trim() || undefined,
        tasks: pickedTaskTexts,
        habits: Array.from(pickedHabits),
        thoughts: thoughts.trim() || undefined
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

  // Sort tasks: priority first, then due-soon, then by note path
  let sortedTasks = $derived.by(() => {
    const today = new Date().toISOString().slice(0, 10);
    return [...openTasks].sort((a, b) => {
      if (a.priority !== b.priority) return (a.priority || 99) - (b.priority || 99);
      const ad = a.dueDate ?? '~';
      const bd = b.dueDate ?? '~';
      if (ad !== bd) return ad < bd ? -1 : 1;
      return a.notePath.localeCompare(b.notePath);
    }).slice(0, 60);
  });

  const stepLabels: Record<Step, string> = {
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
        Plan today in 5 quick steps. Saves a <code class="text-xs">## Daily Plan</code> block to your daily note.
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
      {#if step === 'scripture'}
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
      {:else if step === 'tasks'}
        <h2 class="text-lg font-medium text-text mb-1">Tasks for today</h2>
        <p class="text-sm text-dim mb-4">{pickedTasks.size} selected · pick what you'll actually commit to.</p>
        {#if openTasks.length === 0}
          <div class="text-sm text-dim italic">no open tasks — add some first or skip this step</div>
        {:else}
          <ul class="space-y-1 max-h-[24rem] overflow-y-auto">
            {#each sortedTasks as t (t.id)}
              {@const sel = pickedTasks.has(t.id)}
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
                  {#if t.priority > 0}
                    <span class="text-[10px] font-mono px-1 rounded
                      {t.priority === 1 ? 'bg-error/20 text-error' : ''}
                      {t.priority === 2 ? 'bg-warning/20 text-warning' : ''}
                      {t.priority === 3 ? 'bg-info/20 text-info' : ''}">P{t.priority}</span>
                  {/if}
                  <span class="flex-1 text-sm text-text">{@html inlineMd(t.text)}</span>
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

        <div class="bg-mantle border border-surface1 rounded p-4 space-y-3 text-sm">
          {#if activeScripture.text}
            <blockquote class="border-l-2 border-primary pl-3 italic text-subtext">
              "{activeScripture.text}"
              {#if activeScripture.source}<div class="text-xs text-dim mt-1 not-italic">— {activeScripture.source}</div>{/if}
            </blockquote>
          {/if}
          {#if goal.trim()}
            <div>
              <div class="text-xs uppercase tracking-wider text-dim mb-1">Today's Goal</div>
              <div class="font-medium text-text">{goal}</div>
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
        disabled={step === 'scripture'}
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
