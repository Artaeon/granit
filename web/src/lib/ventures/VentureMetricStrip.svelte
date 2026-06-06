<script lang="ts">
  // Above-tabs metric strip for the venture detail page.
  //
  // Fifth extraction step out of routes/ventures/[name]/+page.svelte.
  // Folds three coherent display blocks that sit between the hero and
  // the sub-nav into one component:
  //   1. Aggregate row — four metric tiles (active projects, active
  //      goals, next deadline tone-coded to urgency, prayer count).
  //   2. Overall progress bar — mean progress across active projects,
  //      with `done/total tasks` alongside the percentage.
  //   3. AI summary panel — streaming-aware tri-state (loading / text
  //      / error), with × dismiss button.
  //
  // All three share the same "what's the current state, at a glance"
  // intent and sit in the same render position, so they extract as
  // one component rather than three. Pure presentation — no $state.
  import { type Venture, type Deadline } from '$lib/api';
  import { colorVar, countdown, deadlineTone } from './venturesDetailHelpers';

  interface Props {
    venture: Venture;
    activeProjectsCount: number;
    activeGoalsCount: number;
    activeIntentionsCount: number;
    nextDeadline: Deadline | null;
    aggregateProgress: number;
    aggregateTasksOpen: number;
    aggregateTasksDone: number;
    showProgressBar: boolean;
    aiBusy: boolean;
    aiText: string;
    aiError: string;
    onDismissAI: () => void;
  }
  let {
    venture,
    activeProjectsCount,
    activeGoalsCount,
    activeIntentionsCount,
    nextDeadline,
    aggregateProgress,
    aggregateTasksOpen,
    aggregateTasksDone,
    showProgressBar,
    aiBusy,
    aiText,
    aiError,
    onDismissAI
  }: Props = $props();
</script>

<!-- Aggregate row — at-a-glance momentum signal. Active projects,
     goals, next deadline tile (tone-coded to urgency), prayer count. -->
<section class="grid grid-cols-2 sm:grid-cols-4 gap-2 mb-4">
  <a
    href={`/projects?venture=${encodeURIComponent(venture.name)}`}
    class="block px-3 py-3 bg-surface0 border border-surface1 rounded hover:border-primary transition-colors"
  >
    <div class="text-2xl font-semibold text-text tabular-nums leading-none">{activeProjectsCount}</div>
    <div class="text-[11px] text-dim mt-1">Active projects</div>
  </a>
  <a
    href="/goals"
    class="block px-3 py-3 bg-surface0 border border-surface1 rounded hover:border-primary transition-colors"
  >
    <div class="text-2xl font-semibold text-text tabular-nums leading-none">{activeGoalsCount}</div>
    <div class="text-[11px] text-dim mt-1">Active goals</div>
  </a>
  {#if nextDeadline}
    {@const tone = deadlineTone(nextDeadline)}
    <a
      href={`/deadlines?venture=${encodeURIComponent(venture.name)}`}
      class="block px-3 py-3 bg-surface0 border border-surface1 rounded hover:border-primary transition-colors"
      title={nextDeadline.title}
    >
      <div
        class="text-2xl font-semibold tabular-nums leading-none"
        style="color: var(--color-{tone});"
      >{countdown(nextDeadline)}</div>
      <div class="text-[11px] text-dim mt-1 truncate">Next: {nextDeadline.title}</div>
    </a>
  {:else}
    <a
      href={`/deadlines?venture=${encodeURIComponent(venture.name)}&new=1`}
      class="block px-3 py-3 bg-surface0 border border-surface1 rounded hover:border-primary transition-colors"
    >
      <div class="text-2xl font-semibold text-text tabular-nums leading-none">—</div>
      <div class="text-[11px] text-dim mt-1">No deadlines</div>
    </a>
  {/if}
  <a
    href={`/prayer?venture=${encodeURIComponent(venture.name)}`}
    class="block px-3 py-3 bg-surface0 border border-surface1 rounded hover:border-primary transition-colors"
  >
    <div
      class="text-2xl font-semibold tabular-nums leading-none"
      style="color: {activeIntentionsCount > 0 ? 'var(--color-secondary)' : 'var(--color-text)'};"
    >{activeIntentionsCount}</div>
    <div class="text-[11px] text-dim mt-1">Praying for</div>
  </a>
</section>

<!-- Progress bar — averaged across active projects. Single
     anchor for "how are we doing on this venture". -->
{#if showProgressBar}
  <section class="mb-4">
    <div class="flex items-baseline justify-between mb-1.5">
      <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Overall progress</h2>
      <span class="text-xs text-subtext font-mono">
        {Math.round(aggregateProgress * 100)}%
        {#if aggregateTasksOpen + aggregateTasksDone > 0}
          · <span class="text-dim">{aggregateTasksDone}/{aggregateTasksDone + aggregateTasksOpen} tasks</span>
        {/if}
      </span>
    </div>
    <div class="h-2 rounded-full bg-surface0 overflow-hidden">
      <div
        class="h-full transition-all"
        style="width: {Math.round(aggregateProgress * 100)}%; background: {colorVar(venture.color)}"
      ></div>
    </div>
  </section>
{/if}

<!-- AI summary panel — appears below the progress bar. Streams
     tokens as they arrive; the user can dismiss with the × or
     regenerate via the trigger button. We intentionally render
     plain prose (no markdown) — the prompt asks for it, and
     skipping a markdown lib keeps the page small. -->
{#if aiText || aiBusy || aiError}
  <section
    class="mb-4 rounded-lg p-4 border"
    style="border-color: color-mix(in srgb, var(--color-primary) 30%, transparent); background: color-mix(in srgb, var(--color-primary) 4%, transparent);"
  >
    <div class="flex items-baseline justify-between gap-2 mb-2">
      <h2 class="text-xs uppercase tracking-wider text-primary font-medium flex items-center gap-1.5">
        <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83" stroke-linecap="round" />
        </svg>
        <span>AI status summary</span>
      </h2>
      <button
        onclick={onDismissAI}
        class="text-dim hover:text-text text-sm leading-none"
        aria-label="dismiss summary"
      >×</button>
    </div>
    {#if aiError}
      <p class="text-sm text-error">{aiError}</p>
    {:else if aiText}
      <p class="text-sm text-subtext leading-relaxed whitespace-pre-wrap break-words">{aiText}{#if aiBusy}<span class="inline-block w-1.5 h-3.5 bg-primary/60 align-middle ml-0.5 animate-pulse"></span>{/if}</p>
    {:else}
      <p class="text-sm text-dim italic">analyzing venture state…</p>
    {/if}
  </section>
{/if}
