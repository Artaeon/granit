<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Goal, type BookShelfRow, type HabitInfo } from '$lib/api';

  // RootsWidget — one-line-per-domain snapshot mirroring the /roots
  // dashboard at glanceable density. Spirit · Mind · Body · Vocation,
  // with the single most-load-bearing metric per domain.
  //
  // Spirit   — Bible streak (current days)
  // Mind     — books currently reading (count)
  // Body     — top habit by current streak
  // Vocation — top open goal title
  //
  // Pulls in parallel; no source failure blanks the whole tile.

  let bibleStreak = $state(0);
  let booksReading = $state(0);
  let topHabit = $state<HabitInfo | null>(null);
  let topGoal = $state<Goal | null>(null);
  let loading = $state(true);

  onMount(() => { void load(); });

  async function load() {
    loading = true;
    const [bs, hs, bks, gs] = await Promise.allSettled([
      api.bibleStreak(),
      api.listHabits(),
      api.listBooks(),
      api.listGoals()
    ]);
    if (bs.status === 'fulfilled') bibleStreak = bs.value.current;
    if (hs.status === 'fulfilled') {
      // Sort by current streak desc and pick the highest. Ignore
      // habits with no streak so the line doesn't read "Habit: 0d".
      const ranked = hs.value.habits.slice().sort((a, b) => b.currentStreak - a.currentStreak);
      topHabit = ranked.find((h) => h.currentStreak > 0) ?? null;
    }
    if (bks.status === 'fulfilled') {
      booksReading = bks.value.books.filter((b: BookShelfRow) => {
        const s = (b as unknown as { status?: string }).status;
        return s === 'reading' || s === 'in_progress';
      }).length;
    }
    if (gs.status === 'fulfilled') {
      const active = gs.value.goals.filter((g) => g.status !== 'completed' && g.status !== 'archived');
      // Prefer the goal with the soonest target date, falling back to
      // the most-recently-updated active goal if no dates are set.
      const dated = active.filter((g) => g.target_date).sort((a, b) =>
        (a.target_date ?? '').localeCompare(b.target_date ?? '')
      );
      topGoal = dated[0] ?? active[0] ?? null;
    }
    loading = false;
  }
</script>

<section class="bg-surface0 border border-surface1 rounded-lg shadow-sm p-3 flex flex-col h-full">
  <div class="flex items-baseline justify-between mb-2">
    <h2 class="text-xs text-dim font-semibold">Roots</h2>
    <a href="/roots" class="text-xs text-secondary hover:underline">open →</a>
  </div>

  {#if loading}
    <p class="text-sm text-dim italic">loading…</p>
  {:else}
    <ul class="space-y-1.5 text-xs flex-1">
      <li class="flex items-baseline gap-2">
        <span class="w-1.5 h-1.5 rounded-full bg-yellow flex-shrink-0"></span>
        <a href="/scripture" class="text-dim hover:text-text">Spirit</a>
        <span class="ml-auto text-text font-mono tabular-nums">
          {bibleStreak > 0 ? bibleStreak + 'd' : '—'}
        </span>
      </li>
      <li class="flex items-baseline gap-2">
        <span class="w-1.5 h-1.5 rounded-full bg-blue flex-shrink-0"></span>
        <a href="/books" class="text-dim hover:text-text">Mind</a>
        <span class="ml-auto text-text font-mono tabular-nums">
          {booksReading > 0 ? booksReading + ' reading' : '—'}
        </span>
      </li>
      <li class="flex items-baseline gap-2">
        <span class="w-1.5 h-1.5 rounded-full bg-green flex-shrink-0"></span>
        <a href="/habits" class="text-dim hover:text-text">Body</a>
        <span class="ml-auto text-text truncate max-w-[60%] text-right" title={topHabit?.name ?? ''}>
          {#if topHabit}
            <span class="text-text">{topHabit.name}</span> <span class="text-dim font-mono">{topHabit.currentStreak}d</span>
          {:else}
            <span class="text-dim">—</span>
          {/if}
        </span>
      </li>
      <li class="flex items-baseline gap-2">
        <span class="w-1.5 h-1.5 rounded-full bg-mauve flex-shrink-0"></span>
        <a href="/goals" class="text-dim hover:text-text">Vocation</a>
        <span class="ml-auto text-text truncate max-w-[60%] text-right" title={topGoal?.title ?? ''}>
          {#if topGoal}
            {topGoal.title}{#if topGoal.target_date} <span class="text-dim font-mono text-[10px]">→ {topGoal.target_date}</span>{/if}
          {:else}
            <span class="text-dim">—</span>
          {/if}
        </span>
      </li>
    </ul>
  {/if}
</section>
