<script lang="ts">
  // Today view — large quick-tick cards focused on today. The big
  // checkbox is the one the user wants to hit fast morning/evening;
  // everything else is supporting context. Extracted verbatim from the
  // /habits route. Reads toggle/busy off dataCtl, the rename inline
  // editor off renameCtl, the weekly-target editor off targetsCtl.
  import { focusOnMount } from '$lib/util/focusOnMount';
  import { habitTargets } from '$lib/habits/targets';
  import { bestDay } from '$lib/habits/habitsDerives';
  import type { HabitInfo } from '$lib/api';
  import type { HabitsDataController } from '$lib/habits/habitsData.svelte';
  import type { HabitsRenameController } from '$lib/habits/habitsRename.svelte';
  import type { HabitsTargetsEditController } from '$lib/habits/habitsTargetsEdit.svelte';

  let {
    sortedHabits,
    dataCtl,
    renameCtl,
    targetsCtl
  }: {
    sortedHabits: HabitInfo[];
    dataCtl: HabitsDataController;
    renameCtl: HabitsRenameController;
    targetsCtl: HabitsTargetsEditController;
  } = $props();

  const busy = $derived(dataCtl.busy);
  const editingName = $derived(renameCtl.editingName);
  const editingTarget = $derived(targetsCtl.editingTarget);
</script>

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
