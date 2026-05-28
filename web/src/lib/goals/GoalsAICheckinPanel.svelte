<script lang="ts">
  // Visual shell for the AI Weekly Check-in surface on /goals.
  // The page owns all the state (entries, busy, error, hidden ids,
  // abort controller) + the orchestration — this component is
  // purely the rendered chrome so the page can shed a few hundred
  // lines and the panel can be styled without touching the
  // chatStream wiring.
  import type { Goal } from '$lib/api';
  import { daysUntilTarget } from './util';

  export interface CheckinEntry {
    id: string;
    title: string;
    verdict: 'on-track' | 'drifting' | 'dead' | string;
    question: string;
  }

  type Rollup = { open: number; done: number };

  type Props = {
    scope: Goal[];
    entries: CheckinEntry[];
    hidden: Set<string>;
    busy: boolean;
    error: string;
    goals: Goal[];
    rollupFor: (g: Goal) => Rollup;
    recentDoneFor: (g: Goal) => number;
    onAbort: () => void;
    onRetry: () => void;
    onClose: () => void;
    onSaveOne: (e: CheckinEntry) => void;
    onSaveAll: () => void;
    onDismiss: (e: CheckinEntry) => void;
    onOpenGoal: (id: string) => void;
  };

  let {
    scope,
    entries,
    hidden,
    busy,
    error,
    goals,
    rollupFor,
    recentDoneFor,
    onAbort,
    onRetry,
    onClose,
    onSaveOne,
    onSaveAll,
    onDismiss,
    onOpenGoal
  }: Props = $props();

  let hasVisible = $derived(entries.some((e) => !hidden.has(e.id)));
</script>

<section class="mb-5 bg-surface0 border border-surface1 rounded-lg overflow-hidden">
  <header class="px-4 py-2.5 border-b border-surface1 flex items-center gap-2">
    <span class="text-sm font-medium text-text">Weekly check-in</span>
    <span class="text-[11px] text-dim">{scope.length} active/paused goal{scope.length === 1 ? '' : 's'}</span>
    <span class="flex-1"></span>
    {#if busy}
      <button
        type="button"
        onclick={onAbort}
        class="px-2 py-1 text-xs bg-surface1 text-subtext rounded hover:bg-surface2"
      >Stop</button>
    {:else}
      {#if entries.length > 0 && hasVisible}
        <button
          type="button"
          onclick={onSaveAll}
          class="px-2.5 py-1 text-xs bg-primary text-on-primary rounded hover:opacity-90"
          title="Append all visible entries to today's jot"
        >Save all to jot</button>
      {/if}
      <button
        type="button"
        onclick={onRetry}
        class="px-2 py-1 text-xs bg-surface1 text-subtext rounded hover:bg-surface2"
        title="re-roll the check-in"
      >↻ retry</button>
    {/if}
    <button
      type="button"
      onclick={onClose}
      class="text-xs text-dim hover:text-text px-1"
    >Dismiss</button>
  </header>

  {#if error}
    <div class="px-4 py-2 text-xs text-error bg-surface0 border-b border-error">{error}</div>
  {/if}

  <div class="p-3 space-y-2">
    {#if busy && entries.length === 0}
      <div class="text-xs text-dim italic px-2 py-3 flex items-center gap-2">
        <span class="inline-block w-1.5 h-3 bg-primary/60 animate-pulse rounded-sm"></span>
        reading {scope.length} goals — verdict + question coming…
      </div>
    {:else if entries.length === 0 && !error}
      <div class="text-xs text-dim italic px-2 py-3">No entries yet.</div>
    {:else}
      {#each entries as e (e.id + e.question)}
        {#if !hidden.has(e.id)}
          {@const verdictTone = e.verdict === 'on-track' ? 'success'
            : e.verdict === 'drifting' ? 'warning'
            : e.verdict === 'dead' ? 'error'
            : 'subtext'}
          <article class="p-3 bg-mantle border border-surface1 rounded">
            <div class="flex items-baseline gap-2 mb-1">
              <span class="text-sm font-medium text-text flex-1 min-w-0 break-words">{e.title}</span>
              <span
                class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-medium tabular-nums whitespace-nowrap"
                style="background: color-mix(in srgb, var(--color-{verdictTone}) 14%, transparent); color: var(--color-{verdictTone});"
              >{e.verdict}</span>
            </div>
            <p class="text-sm text-subtext italic leading-snug">"{e.question}"</p>
            <div class="flex items-center gap-2 mt-2 text-[11px]">
              <button
                type="button"
                onclick={() => onSaveOne(e)}
                class="px-2 py-0.5 bg-surface1 text-primary rounded hover:bg-primary hover:text-on-primary"
                title="Append this entry to today's jot"
              >Save to jot</button>
              <button
                type="button"
                onclick={() => onDismiss(e)}
                class="text-dim hover:text-text"
              >Dismiss</button>
              {#if goals.some((g) => g.id === e.id)}
                <button
                  type="button"
                  onclick={() => onOpenGoal(e.id)}
                  class="ml-auto text-secondary hover:underline"
                >Open goal →</button>
              {/if}
            </div>
          </article>
        {/if}
      {/each}
    {/if}
  </div>

  <!-- "What the AI saw" disclosure — the page-wide rule is that
       the user can always inspect the AI's prompt context. -->
  {#if !busy && entries.length > 0}
    <details class="px-4 py-2 border-t border-surface1 text-[11px] text-dim">
      <summary class="cursor-pointer hover:text-text">What the AI saw</summary>
      <ul class="mt-1.5 space-y-0.5 list-disc list-inside">
        {#each scope as g (g.id)}
          {@const days = daysUntilTarget(g.target_date)}
          {@const roll = rollupFor(g)}
          {@const recent = recentDoneFor(g)}
          <li>
            <span class="text-subtext">{g.title}</span>
            · {g.status ?? 'active'}
            {#if days !== null}· {days < 0 ? `${Math.abs(days)}d past` : `${days}d left`}{/if}
            · {(g.milestones ?? []).filter((m) => m.done).length}/{(g.milestones ?? []).length} ms
            · {roll.open}↗ tasks · {recent} done in 4w
          </li>
        {/each}
      </ul>
    </details>
  {/if}
</section>
