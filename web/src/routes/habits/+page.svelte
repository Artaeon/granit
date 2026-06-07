<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { onWsEvent } from '$lib/ws';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import Button from '$lib/components/Button.svelte';
  import { habitTargets } from '$lib/habits/targets';
  import { focusOnMount } from '$lib/util/focusOnMount';
  import { createHabitsViewState, type HabitsView } from '$lib/habits/habitsViewState.svelte';
  import { createHabitsAI } from '$lib/habits/habitsAI.svelte';
  import { createHabitsData } from '$lib/habits/habitsData.svelte';
  import { createHabitsRename } from '$lib/habits/habitsRename.svelte';
  import { createHabitsStackEdit } from '$lib/habits/habitsStackEdit.svelte';
  import { createHabitsAdd } from '$lib/habits/habitsAdd.svelte';
  import { createHabitsTargetsEdit } from '$lib/habits/habitsTargetsEdit.svelte';
  import HabitsTodayView from '$lib/habits/HabitsTodayView.svelte';
  import HabitsWeekView from '$lib/habits/HabitsWeekView.svelte';
  import HabitsHeatmapView from '$lib/habits/HabitsHeatmapView.svelte';
  import HabitsListView from '$lib/habits/HabitsListView.svelte';

  // /habits — three view modes (Today / Week / List / Heatmap) over
  // the same 90-day window. Sort + insight (best day of week) work
  // across all three views so the preference sticks across lenses.
  //
  // The page module is now a wiring layer: every cluster of state +
  // behaviour lives in a $lib/habits/* controller; the script body
  // just instantiates them, threads the deps, and re-exposes the
  // most-used reactive reads as locals for the template.

  // Shared toast hooks — every controller funnels through these.
  const toastError = async (msg: string) =>
    (await import('$lib/components/toast')).toast.error(msg);
  const toastSuccess = async (msg: string) =>
    (await import('$lib/components/toast')).toast.success(msg);

  const dataCtl = createHabitsData({ isAuthed: () => !!$auth, onError: toastError });
  const data = $derived(dataCtl.data);
  const loading = $derived(dataCtl.loading);
  const bulkBusy = $derived(dataCtl.bulkBusy);
  const todayDone = $derived(dataCtl.todayDone);
  const todayTotal = $derived(dataCtl.todayTotal);
  const undoneToday = $derived(dataCtl.undoneToday);

  const viewCtl = createHabitsViewState({ getData: () => dataCtl.data });
  const sortedHabits = $derived(viewCtl.sortedHabits);

  const addCtl = createHabitsAdd({
    getToday: () => dataCtl.data?.today,
    reload: () => dataCtl.load(),
    onError: toastError
  });
  const addBusy = $derived(addCtl.addBusy);

  // aiCtl.adopt() pre-fills addCtl.addName and runs the add pipeline,
  // so a "suggest from goals" pick routes through the same submit
  // path as a manual add.
  const aiCtl = createHabitsAI({
    getData: () => dataCtl.data,
    adopt: async (name: string) => {
      addCtl.addName = name;
      await addCtl.addHabit();
    }
  });
  const aiInsights = $derived(aiCtl.insightLines);
  const aiBusy = $derived(aiCtl.insightBusy);
  const aiError = $derived(aiCtl.insightError);
  const suggestedHabits = $derived(aiCtl.suggested);
  const suggestBusy = $derived(aiCtl.suggestBusy);
  const suggestError = $derived(aiCtl.suggestError);

  const targetsCtl = createHabitsTargetsEdit({ getTargets: () => $habitTargets });

  const stackCtl = createHabitsStackEdit({
    setBusy: (key) => { dataCtl.busy = key; },
    reload: () => dataCtl.load(),
    onError: toastError
  });

  const renameCtl = createHabitsRename({
    setBusy: (key) => { dataCtl.busy = key; },
    reload: () => dataCtl.load(),
    onSuccess: toastSuccess,
    onError: toastError
  });

  onMount(() => {
    dataCtl.load();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') dataCtl.load();
    });
  });
</script>

<div class="h-full overflow-y-auto">
  <div class="@container p-4 sm:p-6 lg:p-8 max-w-5xl mx-auto">
    <header class="mb-5 flex flex-col @md:flex-row @md:items-baseline @md:justify-between gap-3">
      <div class="min-w-0">
        <h1 class="text-2xl @md:text-3xl font-semibold text-text">Habits</h1>
        <p class="text-sm text-dim mt-1">
          derived from <code class="text-xs">## Habits</code> in each daily note
          {#if data}
            <span class="ml-1">· {todayDone}/{todayTotal} done today</span>
          {/if}
        </p>
      </div>
      <div class="flex items-center gap-2 self-start">
        {#if undoneToday.length > 0}
          <Button
            variant="secondary"
            onclick={() => dataCtl.tickAllToday()}
            disabled={bulkBusy}
            title="mark every undone habit as done today"
            class="px-3 py-1.5 text-sm !text-success !border-success hover:!text-success"
          >
            {bulkBusy ? '…' : `Tick all (${undoneToday.length})`}
          </Button>
        {/if}
        <Button
          variant="primary"
          onclick={() => (addCtl.addOpen = !addCtl.addOpen)}
          class="px-3 py-1.5 text-sm"
        >{addCtl.addOpen ? 'cancel' : '+ Add habit'}</Button>
      </div>
    </header>

    {#if addCtl.addOpen}
      <!-- Add-habit form. Single field — name only. Adds an unticked
           checkbox to today's daily note's ## Habits section (the
           server creates the section / file if needed). The user can
           then toggle it done from the same page or set per-day
           later. Keeps capture friction at a single keystroke beyond
           "where do I click". -->
      <form onsubmit={(e) => addCtl.addHabit(e)} class="bg-surface0 border border-surface1 rounded-lg p-3 mb-4 flex flex-wrap gap-2 items-center">
        <input
          bind:value={addCtl.addName}
          required
          use:focusOnMount
          placeholder="habit name (e.g. morning movement, no doomscrolling)…"
          class="flex-1 min-w-[12rem] px-3 py-2 bg-mantle border border-surface1 rounded text-base sm:text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
        />
        <Button
          type="submit"
          variant="primary"
          disabled={!addCtl.addName.trim() || addBusy}
          class="px-4 py-2 text-sm"
        >{addBusy ? '…' : 'add to today'}</Button>
      </form>
    {/if}

    {#if data && data.habits.length >= 2 && (aiInsights.length > 0 || aiBusy || aiError)}
      <section class="mb-4 p-3 bg-surface1 border border-surface2 rounded-lg">
        <div class="flex items-baseline gap-2 mb-2">
          <h3 class="text-xs text-primary font-medium">Pattern insight</h3>
          <span class="flex-1"></span>
          {#if aiBusy}
            <span class="text-[11px] text-dim italic">analyzing…</span>
          {:else}
            <button onclick={() => aiCtl.runInsight()} class="text-[11px] text-secondary hover:underline">regenerate</button>
          {/if}
          <button onclick={() => aiCtl.dismissInsight()} class="text-[11px] text-dim hover:text-text">dismiss</button>
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

    <!-- AI-suggested habits from goals. Distinct surface from "Pattern
         insight" above: that one observes existing data, this one
         generates new habits to add. Each suggestion is one-click
         adopt; rationale stays visible so the user knows WHY this
         habit was proposed for which goal. -->
    {#if suggestedHabits.length > 0 || suggestBusy || suggestError}
      <section class="mb-4 p-3 bg-surface1 border border-secondary/40 rounded-lg">
        <div class="flex items-baseline gap-2 mb-2">
          <h3 class="text-xs text-secondary font-medium">Habits from your goals</h3>
          <span class="flex-1"></span>
          {#if suggestBusy}
            <span class="text-[11px] text-dim italic">proposing…</span>
          {:else if suggestedHabits.length > 0}
            <button onclick={() => aiCtl.runSuggest()} class="text-[11px] text-secondary hover:underline">regenerate</button>
          {/if}
          <button onclick={() => aiCtl.dismissSuggest()} class="text-[11px] text-dim hover:text-text">dismiss</button>
        </div>
        {#if suggestError}
          <p class="text-xs text-error">{suggestError}</p>
        {:else if suggestedHabits.length === 0 && suggestBusy}
          <p class="text-xs text-dim italic">reading your goals…</p>
        {:else}
          <ul class="space-y-1.5">
            {#each suggestedHabits as h (h.name)}
              <li class="flex items-start gap-2">
                <button
                  type="button"
                  onclick={() => aiCtl.adoptSuggestion(h.name)}
                  disabled={addBusy}
                  class="text-[11px] px-2 py-0.5 rounded bg-secondary text-on-secondary hover:opacity-90 disabled:opacity-50 flex-shrink-0 mt-0.5"
                  title="Track this habit starting today"
                >+ add</button>
                <div class="min-w-0 flex-1">
                  <div class="text-sm text-text font-medium">{h.name}</div>
                  {#if h.rationale}
                    <div class="text-[11px] text-subtext leading-snug">{h.rationale}</div>
                  {/if}
                </div>
              </li>
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
              class="px-3 py-1.5 capitalize transition-colors {viewCtl.view === o.v ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
              onclick={() => (viewCtl.view = o.v as HabitsView)}
            >{o.label}</button>
          {/each}
        </div>
        <span class="flex-1"></span>
        {#if data && data.habits.length >= 2 && aiInsights.length === 0 && !aiBusy && !aiError}
          <button
            type="button"
            onclick={() => aiCtl.runInsight()}
            class="text-[11px] px-2 py-1 rounded inline-flex items-center gap-1 bg-surface1 text-primary border border-surface2 hover:bg-surface2"
            title="Ask AI for pattern observations"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-3 h-3">
              <path d="M12 3l1.2 4.2L17 9l-3.8 1.8L12 15l-1.2-4.2L7 9l3.8-1.8L12 3z" stroke-linejoin="round"/>
            </svg>
            Insight
          </button>
        {/if}
        {#if suggestedHabits.length === 0 && !suggestBusy && !suggestError}
          <button
            type="button"
            onclick={() => aiCtl.runSuggest()}
            class="text-[11px] px-2 py-1 rounded inline-flex items-center gap-1 bg-surface1 text-secondary border border-surface2 hover:bg-surface2"
            title="Propose habits that ladder toward your active goals"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-3 h-3" stroke-linecap="round" stroke-linejoin="round">
              <path d="M5 12l4 4 10-10"/>
              <path d="M12 21v-3"/>
            </svg>
            Suggest from goals
          </button>
        {/if}
        <label class="text-xs text-dim flex items-center gap-1.5">
          sort
          <select
            bind:value={viewCtl.sortBy}
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
    {:else if data && viewCtl.view === 'today'}
      <HabitsTodayView {sortedHabits} {dataCtl} {renameCtl} {targetsCtl} />
    {:else if data && viewCtl.view === 'week'}
      <HabitsWeekView {data} {sortedHabits} {dataCtl} />
    {:else if data && viewCtl.view === 'heatmap'}
      <HabitsHeatmapView {sortedHabits} />
    {:else if data}
      <HabitsListView {data} {sortedHabits} {dataCtl} {renameCtl} {targetsCtl} {stackCtl} />
    {/if}
  </div>
</div>
