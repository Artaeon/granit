<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { onWsEvent } from '$lib/ws';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import Heatmap from '$lib/components/Heatmap.svelte';
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
  import { bestDay, weekDays, shortDow, shortDate } from '$lib/habits/habitsDerives';

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
  const busy = $derived(dataCtl.busy);
  const bulkBusy = $derived(dataCtl.bulkBusy);
  const anchorsFor = $derived(dataCtl.anchorsFor);
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
  const editingTarget = $derived(targetsCtl.editingTarget);

  const stackCtl = createHabitsStackEdit({
    setBusy: (key) => { dataCtl.busy = key; },
    reload: () => dataCtl.load(),
    onError: toastError
  });
  const editingStack = $derived(stackCtl.editingStack);

  const renameCtl = createHabitsRename({
    setBusy: (key) => { dataCtl.busy = key; },
    reload: () => dataCtl.load(),
    onSuccess: toastSuccess,
    onError: toastError
  });
  const editingName = $derived(renameCtl.editingName);

  onMount(() => {
    dataCtl.load();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') dataCtl.load();
    });
  });
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
          <h3 class="text-xs uppercase tracking-wider text-primary font-medium">Pattern insight</h3>
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
          <h3 class="text-xs uppercase tracking-wider text-secondary font-medium">Habits from your goals</h3>
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
      <!-- ===== TODAY VIEW ===== -->
      <!-- Large quick-tick cards focused on today. The big checkbox
           is the one the user wants to hit fast in the morning or
           evening — every other element is supporting context. -->
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
        {#each sortedHabits as h (h.name)}
          {@const insight = bestDay(h)}
          {@const tgt = targetsCtl.targetState(h)}
          <div
            class="relative text-left p-4 bg-surface0 border rounded-lg transition-colors flex items-start gap-3
              {h.doneToday
                ? 'border-success bg-surface0'
                : 'border-surface1 hover:border-primary'}"
          >
            <button
              type="button"
              onclick={() => dataCtl.toggleToday(h)}
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
              {#if editingName === h.name}
                <form
                  onsubmit={(e) => { e.preventDefault(); renameCtl.submitRename(h.name); }}
                  class="flex items-center gap-1.5"
                >
                  <input
                    bind:value={renameCtl.renameDraft}
                    use:focusOnMount
                    class="flex-1 px-2 py-1 bg-base border border-surface2 rounded text-text text-sm"
                    placeholder="new name"
                  />
                  <button type="submit" disabled={busy === h.name} class="px-2 py-1 bg-primary text-on-primary rounded text-xs disabled:opacity-50">save</button>
                  <button type="button" onclick={() => renameCtl.cancelRename()} class="px-2 py-1 text-dim hover:text-text text-xs">cancel</button>
                </form>
              {:else}
                <div class="flex items-start justify-between gap-2">
                  <h2 class="text-base font-medium text-text break-words flex-1 min-w-0">{h.name}</h2>
                  <div class="flex items-center gap-0.5 flex-shrink-0">
                    <button
                      type="button"
                      onclick={() => renameCtl.startRename(h)}
                      class="px-1.5 py-0.5 text-dim hover:text-text rounded text-[11px]"
                      title="rename habit"
                      aria-label="rename habit"
                    >✎</button>
                    <button
                      type="button"
                      onclick={() => renameCtl.deleteHabit(h.name)}
                      disabled={busy === h.name}
                      class="px-1.5 py-0.5 text-dim hover:text-error rounded text-[11px] disabled:opacity-50"
                      title="delete habit — strips matching lines from every daily note"
                      aria-label="delete habit"
                    >×</button>
                  </div>
                </div>
              {/if}
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
                    onclick={() => targetsCtl.editingTarget = editingTarget === h.name ? null : h.name}
                    class="px-1.5 py-0.5 rounded text-[10px] uppercase tracking-wider border transition-colors
                      {hit
                        ? 'bg-surface0 text-success border-success hover:bg-surface1'
                        : 'bg-surface0 text-warning border-warning hover:bg-surface1'}"
                    title="weekly target — click to edit"
                  >🎯 {tgt.done}/{tgt.target}/wk</button>
                {:else}
                  <button
                    type="button"
                    onclick={() => targetsCtl.editingTarget = editingTarget === h.name ? null : h.name}
                    class="px-1.5 py-0.5 rounded text-[10px] uppercase tracking-wider border bg-surface1 text-dim border-surface2 hover:text-text"
                    title="set a weekly target"
                  >+ target</button>
                {/if}
              </div>
              {#if editingTarget === h.name}
                <div class="mt-2 flex items-center gap-1.5 text-[11px]">
                  <span class="text-dim">target / week:</span>
                  <button class="px-1.5 py-0.5 bg-surface1 hover:bg-surface2 rounded text-text" onclick={() => targetsCtl.bumpTarget(h.name, -1)}>−</button>
                  <span class="font-mono text-text w-4 text-center">{$habitTargets[h.name] ?? 5}</span>
                  <button class="px-1.5 py-0.5 bg-surface1 hover:bg-surface2 rounded text-text" onclick={() => targetsCtl.bumpTarget(h.name, 1)}>+</button>
                  <button class="ml-1 px-1.5 py-0.5 text-dim hover:text-text underline" onclick={() => targetsCtl.clearTarget(h.name)}>clear</button>
                  <button class="ml-auto px-1.5 py-0.5 text-dim hover:text-text" onclick={() => (targetsCtl.editingTarget = null)}>done</button>
                </div>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    {:else if data && viewCtl.view === 'week'}
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
                      onclick={() => dataCtl.toggleOnDate(h, d.date, !d.done)}
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
          {@const tgt = targetsCtl.targetState(h)}
          <article class="bg-surface0 border border-surface1 rounded-lg p-3">
            <div class="flex items-start gap-3 mb-3">
              <button
                onclick={() => dataCtl.toggleToday(h)}
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
                {#if editingName === h.name}
                  <form
                    onsubmit={(e) => { e.preventDefault(); renameCtl.submitRename(h.name); }}
                    class="flex items-center gap-1.5"
                  >
                    <input
                      bind:value={renameCtl.renameDraft}
                      use:focusOnMount
                      class="flex-1 px-2 py-1 bg-base border border-surface2 rounded text-text text-sm"
                      placeholder="new name"
                    />
                    <button type="submit" disabled={busy === h.name} class="px-2 py-1 bg-primary text-on-primary rounded text-xs disabled:opacity-50">save</button>
                    <button type="button" onclick={() => renameCtl.cancelRename()} class="px-2 py-1 text-dim hover:text-text text-xs">cancel</button>
                  </form>
                {:else}
                  <div class="flex items-start justify-between gap-2">
                    <h2 class="text-base font-medium text-text break-words flex-1 min-w-0">{h.name}</h2>
                    <div class="flex items-center gap-0.5 flex-shrink-0">
                      <button
                        type="button"
                        onclick={() => renameCtl.startRename(h)}
                        class="px-1.5 py-0.5 text-dim hover:text-text rounded text-[11px]"
                        title="rename habit"
                        aria-label="rename habit"
                      >✎</button>
                      <button
                        type="button"
                        onclick={() => renameCtl.deleteHabit(h.name)}
                        disabled={busy === h.name}
                        class="px-1.5 py-0.5 text-dim hover:text-error rounded text-[11px] disabled:opacity-50"
                        title="delete habit — strips matching lines from every daily note"
                        aria-label="delete habit"
                      >×</button>
                    </div>
                  </div>
                {/if}
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
                      onclick={() => targetsCtl.editingTarget = editingTarget === h.name ? null : h.name}
                      class="px-1.5 py-0.5 rounded text-[10px] uppercase tracking-wider border transition-colors
                        {hit
                          ? 'bg-surface0 text-success border-success hover:bg-surface1'
                          : 'bg-surface0 text-warning border-warning hover:bg-surface1'}"
                      title="weekly target — click to edit"
                    >🎯 {tgt.done}/{tgt.target}/wk</button>
                  {:else}
                    <button
                      type="button"
                      onclick={() => targetsCtl.editingTarget = editingTarget === h.name ? null : h.name}
                      class="px-1.5 py-0.5 rounded text-[10px] uppercase tracking-wider border bg-surface1 text-dim border-surface2 hover:text-text"
                    >+ target</button>
                  {/if}
                </div>
                {#if editingTarget === h.name}
                  <div class="mt-1.5 flex items-center gap-1.5 text-[11px]">
                    <span class="text-dim">target / week:</span>
                    <button class="px-1.5 py-0.5 bg-surface1 hover:bg-surface2 rounded text-text" onclick={() => targetsCtl.bumpTarget(h.name, -1)}>−</button>
                    <span class="font-mono text-text w-4 text-center">{$habitTargets[h.name] ?? 5}</span>
                    <button class="px-1.5 py-0.5 bg-surface1 hover:bg-surface2 rounded text-text" onclick={() => targetsCtl.bumpTarget(h.name, 1)}>+</button>
                    <button class="ml-1 px-1.5 py-0.5 text-dim hover:text-text underline" onclick={() => targetsCtl.clearTarget(h.name)}>clear</button>
                    <button class="ml-auto px-1.5 py-0.5 text-dim hover:text-text" onclick={() => (targetsCtl.editingTarget = null)}>done</button>
                  </div>
                {/if}
                <!-- Stack anchor — chain badge when set, "+ stack"
                     button when not. Edit pops a small inline form
                     with a select of every other (non-self) habit.
                     Behavioural-science play: anchoring beats
                     willpower for habit consistency. -->
                {#if editingStack === h.name}
                  <div class="mt-1.5 flex items-center gap-1.5 text-[11px] flex-wrap">
                    <span class="text-dim">after</span>
                    <select
                      bind:value={stackCtl.stackDraft}
                      class="px-1.5 py-0.5 bg-surface1 border border-surface2 rounded text-text text-[11px]"
                    >
                      <option value="">(none — clear anchor)</option>
                      {#each data?.habits.filter((other) => other.name !== h.name) ?? [] as other (other.name)}
                        <option value={other.name}>{other.name}</option>
                      {/each}
                    </select>
                    <button
                      class="px-1.5 py-0.5 bg-primary text-on-primary rounded text-[11px] disabled:opacity-50"
                      disabled={busy === h.name}
                      onclick={() => stackCtl.submitStackEdit(h.name)}
                    >save</button>
                    <button class="px-1.5 py-0.5 text-dim hover:text-text" onclick={() => stackCtl.cancelStackEdit()}>cancel</button>
                  </div>
                {:else if h.stackAfter}
                  <div class="mt-1.5 flex items-center gap-1.5 text-[11px] flex-wrap">
                    <button
                      type="button"
                      onclick={() => stackCtl.startStackEdit(h)}
                      class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] uppercase tracking-wider border border-secondary/40 bg-surface0 text-secondary hover:bg-surface1"
                      title="stack anchor — click to edit or clear"
                    >🔗 after {h.stackAfter}</button>
                    {#if anchorsFor[h.name]?.length}
                      <span
                        class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] uppercase tracking-wider bg-surface1 text-dim"
                        title="other habits anchored to this one"
                      >→ triggers {anchorsFor[h.name].join(', ')}</span>
                    {/if}
                  </div>
                {:else}
                  <div class="mt-1.5 flex items-center gap-1.5 text-[11px] flex-wrap">
                    <button
                      type="button"
                      onclick={() => stackCtl.startStackEdit(h)}
                      class="px-1.5 py-0.5 rounded text-[10px] uppercase tracking-wider border bg-surface1 text-dim border-surface2 hover:text-text"
                      title="anchor this habit to another habit you already do"
                    >+ stack after…</button>
                    {#if anchorsFor[h.name]?.length}
                      <span
                        class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] uppercase tracking-wider bg-surface1 text-dim"
                        title="other habits anchored to this one"
                      >→ triggers {anchorsFor[h.name].join(', ')}</span>
                    {/if}
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
                  onclick={() => dataCtl.toggleOnDate(h, d.date, !d.done)}
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
    {:else if data && viewCtl.view === 'heatmap'}
      <!-- Year-at-a-glance per habit. The Heatmap component handles
           layout + tooltips; we just feed it {date, value} pairs.
           value = 1 when done, 0 otherwise — binary maxes the
           color scale at the brightest tone, which reads as a
           clean "completed" green. -->
      <div class="space-y-4">
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
