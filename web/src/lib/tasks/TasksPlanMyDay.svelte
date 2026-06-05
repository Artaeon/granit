<!--
  TasksPlanMyDay — the AI plan-my-day panel that appears above the
  list / kanban / today views. Distinct from triage + deadline-detect
  (which operate on UNTRIAGED tasks): this looks across ALL open
  tasks and produces a sequenced 3-7-task plan budgeted to the
  user's stated focus hours.

  Returns strict JSON so each row gets its own accept/skip controls
  — accepting pins the task into a back-to-back time slot via
  scheduledStart. Falls back to streamed prose if JSON parse fails.

  Parent owns visibility — this component renders nothing unless
  there's actually plan state worth showing (busy / response /
  error / plan rows).
-->
<script lang="ts">
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';
  import type { FocusPlanStore } from './aiAgentStore';
  import type { TasksDataController } from './tasksData.svelte';

  type Props = {
    focusPlan: FocusPlanStore;
    dataCtl: TasksDataController;
    aiFocusHours: number;
    load: () => void | Promise<void>;
  };

  let { focusPlan, dataCtl, aiFocusHours, load }: Props = $props();

  // Show nothing until the user has actually fired Plan-my-day at
  // least once and there's state worth rendering.
  let hasState = $derived(
    $focusPlan.busy || $focusPlan.response !== '' || $focusPlan.error !== '' || $focusPlan.plan.length > 0
  );
</script>

{#if hasState}
  <div class="px-3 py-3 border-b border-surface1 flex-shrink-0 bg-surface1">
    <div class="flex items-baseline gap-2 mb-2 flex-wrap">
      <span class="text-xs uppercase tracking-wider text-secondary font-semibold">Plan my day</span>
      {#if $focusPlan.plan.length > 0 && !$focusPlan.busy}
        {@const totalEst = $focusPlan.plan.reduce((s, p) => s + Math.max(15, p.estimateMinutes || 30), 0)}
        <span class="text-[11px] text-dim font-mono tabular-nums">{$focusPlan.plan.length} task{$focusPlan.plan.length === 1 ? '' : 's'} · {totalEst}m</span>
      {/if}
      <span class="flex-1"></span>
      {#if $focusPlan.busy}
        <button onclick={() => focusPlan.cancel()} class="text-[11px] text-warning hover:underline">cancel</button>
      {:else}
        {#if $focusPlan.plan.length > 0}
          <button
            onclick={() => void focusPlan.acceptAll(dataCtl.tasks, load)}
            class="text-[11px] text-success hover:underline"
            title="Pin every remaining plan item back-to-back starting now"
          >accept all</button>
        {/if}
        <button
          onclick={() => void focusPlan.run(dataCtl.tasks, aiFocusHours)}
          class="text-[11px] text-secondary hover:underline"
        >↻ regenerate</button>
        <button
          onclick={() => focusPlan.dismiss()}
          class="text-[11px] text-dim hover:text-error"
        >dismiss</button>
      {/if}
    </div>
    {#if $focusPlan.error}
      <div class="text-xs text-error">{$focusPlan.error}</div>
    {:else if $focusPlan.plan.length > 0}
      <!-- Structured plan view. Each row has its own accept/skip so
           the user can take 4 of 5 suggestions without burning the
           whole call. -->
      <ol class="space-y-1.5">
        {#each $focusPlan.plan as p (p.taskId)}
          {@const t = dataCtl.tasks.find((x) => x.id === p.taskId)}
          {#if t}
            <li class="flex items-start gap-2 text-xs">
              <span class="font-mono text-secondary tabular-nums w-5 flex-shrink-0 mt-0.5">{p.order}.</span>
              <div class="flex-1 min-w-0">
                <div class="text-text">
                  <span class="font-medium">{t.text}</span>
                  <span class="text-dim ml-2 font-mono tabular-nums">{Math.max(15, p.estimateMinutes || 30)}m</span>
                </div>
                {#if p.rationale}
                  <div class="text-dim mt-0.5 italic">{p.rationale}</div>
                {/if}
              </div>
              <button
                onclick={() => void focusPlan.acceptItem(p, dataCtl.tasks, load)}
                class="px-2 py-0.5 bg-surface0 text-success rounded hover:bg-surface1 flex-shrink-0"
                title="Pin this task into a time slot today"
              >accept</button>
              <button
                onclick={() => focusPlan.skipItem(p.taskId)}
                class="px-2 py-0.5 text-dim hover:text-text flex-shrink-0"
              >skip</button>
            </li>
          {/if}
        {/each}
      </ol>
      {#if $focusPlan.skipped}
        <p class="text-[11px] text-dim italic mt-2 pt-2 border-t border-surface1">Skipped: {$focusPlan.skipped}</p>
      {/if}
      <p class="text-[10px] text-dim mt-2">Context: {dataCtl.tasks.filter((t) => !t.done).slice(0, 30).length} open tasks shown · {aiFocusHours}h focus budget</p>
    {:else}
      <!-- Streaming/fallback view: show the raw model output while
           we wait for the JSON to close, OR if parsing fails. -->
      <div class="prose prose-sm max-w-none text-sm">
        <MarkdownRenderer body={$focusPlan.response || '_thinking…_'} />
      </div>
    {/if}
  </div>
{/if}
