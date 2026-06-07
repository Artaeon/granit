<script lang="ts">
  // List view — one card per habit with stats, the weekly-target +
  // habit-stack inline editors, and a compact 90-day contribution grid.
  // The richest of the four views; extracted verbatim from the /habits
  // route. Reads toggles/busy/anchors off dataCtl and the three inline
  // editors off rename / targets / stack controllers.
  import { focusOnMount } from '$lib/util/focusOnMount';
  import { habitTargets } from '$lib/habits/targets';
  import { bestDay } from '$lib/habits/habitsDerives';
  import type { HabitInfo, HabitsResponse } from '$lib/api';
  import type { HabitsDataController } from '$lib/habits/habitsData.svelte';
  import type { HabitsRenameController } from '$lib/habits/habitsRename.svelte';
  import type { HabitsTargetsEditController } from '$lib/habits/habitsTargetsEdit.svelte';
  import type { HabitsStackEditController } from '$lib/habits/habitsStackEdit.svelte';

  let {
    data,
    sortedHabits,
    dataCtl,
    renameCtl,
    targetsCtl,
    stackCtl
  }: {
    data: HabitsResponse;
    sortedHabits: HabitInfo[];
    dataCtl: HabitsDataController;
    renameCtl: HabitsRenameController;
    targetsCtl: HabitsTargetsEditController;
    stackCtl: HabitsStackEditController;
  } = $props();

  const busy = $derived(dataCtl.busy);
  const anchorsFor = $derived(dataCtl.anchorsFor);
  const editingName = $derived(renameCtl.editingName);
  const editingTarget = $derived(targetsCtl.editingTarget);
  const editingStack = $derived(stackCtl.editingStack);
</script>

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
                class="px-1.5 py-0.5 rounded text-[10px] border transition-colors
                  {hit
                    ? 'bg-surface0 text-success border-success hover:bg-surface1'
                    : 'bg-surface0 text-warning border-warning hover:bg-surface1'}"
                title="weekly target — click to edit"
              >🎯 {tgt.done}/{tgt.target}/wk</button>
            {:else}
              <button
                type="button"
                onclick={() => targetsCtl.editingTarget = editingTarget === h.name ? null : h.name}
                class="px-1.5 py-0.5 rounded text-[10px] border bg-surface1 text-dim border-surface2 hover:text-text"
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
          <!-- Stack anchor — chain badge when set, "+ stack" button when
               not. Edit pops a small inline form with a select of every
               other (non-self) habit. Anchoring beats willpower for
               habit consistency. -->
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
                class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] border border-secondary/40 bg-surface0 text-secondary hover:bg-surface1"
                title="stack anchor — click to edit or clear"
              >🔗 after {h.stackAfter}</button>
              {#if anchorsFor[h.name]?.length}
                <span
                  class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] bg-surface1 text-dim"
                  title="other habits anchored to this one"
                >→ triggers {anchorsFor[h.name].join(', ')}</span>
              {/if}
            </div>
          {:else}
            <div class="mt-1.5 flex items-center gap-1.5 text-[11px] flex-wrap">
              <button
                type="button"
                onclick={() => stackCtl.startStackEdit(h)}
                class="px-1.5 py-0.5 rounded text-[10px] border bg-surface1 text-dim border-surface2 hover:text-text"
                title="anchor this habit to another habit you already do"
              >+ stack after…</button>
              {#if anchorsFor[h.name]?.length}
                <span
                  class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] bg-surface1 text-dim"
                  title="other habits anchored to this one"
                >→ triggers {anchorsFor[h.name].join(', ')}</span>
              {/if}
            </div>
          {/if}
        </div>
      </div>

      <!-- Compact GitHub-contribution-style grid: fixed ~13px cells
           instead of stretching each of the 13 columns to fill the row
           (which made one habit ~600px tall). w-fit keeps the grid at
           its natural width. Click any dot to toggle that date. -->
      <div class="grid grid-flow-col grid-rows-7 gap-[3px] w-fit" style="grid-auto-columns: 0.85rem;">
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
