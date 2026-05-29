<script lang="ts">
  // Single deadline row used by the /deadlines list view. Stream BB.
  // Carries the visual hierarchy for the page:
  //   Critical → left border error + dot + faint error tint
  //   High     → left border warning + dot + faint warning tint
  //   Normal   → left border surface1 + dot, no tint
  //   Met / Cancelled → opacity-60 + line-through title
  //
  // Quick-actions (mark met / snooze / reopen) reveal on hover or
  // keyboard focus so the row stays calm on first paint but the
  // affordance is one click away. The card is itself the clickable
  // surface (role=button + Enter/Space handler) so the whole row
  // opens the edit drawer; chip links + quick-action buttons
  // stopPropagation so they don't double-trigger the open.
  import type { Deadline, DeadlineStatus } from '$lib/api';
  import DeadlinePill from '$lib/deadlines/DeadlinePill.svelte';

  type Props = {
    d: Deadline;
    countdown: string;
    goalTitle: (id?: string) => string;
    projectHref: (name: string) => string;
    goalHref: (id: string) => string;
    ventureHref: (name: string) => string;
    onOpen: (d: Deadline) => void;
    onMarkMet: (d: Deadline, e: MouseEvent) => void;
    onSnooze: (d: Deadline, days: number, e: MouseEvent) => void;
    onReopen: (d: Deadline, e: MouseEvent) => void;
  };

  let {
    d,
    countdown,
    goalTitle,
    projectHref,
    goalHref,
    ventureHref,
    onOpen,
    onMarkMet,
    onSnooze,
    onReopen
  }: Props = $props();

  let isMet = $derived(d.status === 'met');
  let isCancelled = $derived(d.status === 'cancelled');
  let isDone = $derived(isMet || isCancelled);

  // Tier-driven visual signature. Done rows wash out regardless of
  // tier — past wins shouldn't keep shouting in red.
  type TierStyle = { dot: string; border: string; tint: string };
  let tier = $derived<TierStyle>(
    isDone
      ? { dot: 'bg-surface2', border: 'border-l-surface1', tint: '' }
      : d.importance === 'critical'
        ? { dot: 'bg-error', border: 'border-l-error', tint: 'bg-error/[0.06]' }
        : d.importance === 'high'
          ? { dot: 'bg-warning', border: 'border-l-warning', tint: 'bg-warning/[0.04]' }
          : { dot: 'bg-secondary', border: 'border-l-surface1', tint: '' }
  );

  function statusTone(s: DeadlineStatus | string | undefined): string {
    switch (s) {
      case 'met':
        return 'success';
      case 'missed':
        return 'error';
      case 'cancelled':
        return 'subtext';
      default:
        return 'info';
    }
  }
  let st = $derived(statusTone(d.status));
</script>

<div
  role="button"
  tabindex="0"
  onclick={() => onOpen(d)}
  onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onOpen(d); } }}
  class="group w-full text-left border border-surface1 border-l-2 {tier.border} {tier.tint || 'bg-surface0'} rounded-lg p-3 hover:border-primary transition-colors flex flex-col gap-1.5 cursor-pointer {isDone ? 'opacity-60' : ''}"
>
  <!-- Title row — colored dot anchors the importance tier in addition
       to the left border, so the row reads as critical/high at a
       glance even when scanning vertically. -->
  <div class="flex items-baseline gap-2">
    <span class="w-2 h-2 mt-1 rounded-full flex-shrink-0 self-center {tier.dot}" aria-hidden="true"></span>
    <span class="text-base text-text font-medium flex-1 min-w-0 truncate {isMet ? 'line-through' : ''}">
      {d.title}
    </span>
    <div class="flex items-center gap-1.5 flex-shrink-0">
      <DeadlinePill variant="importance" importance={d.importance} />
      {#if d.status && d.status !== 'active'}
        <span
          class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded whitespace-nowrap"
          style="background: var(--color-{st}); color: #ffffff;"
        >{d.status}</span>
      {/if}
    </div>
  </div>

  <!-- Meta row — date, countdown, parent-entity chips, hover-revealed
       quick-actions. -->
  <div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-dim">
    <span class="font-mono tabular-nums text-subtext">{d.date}</span>
    <span class="text-subtext">· {countdown}</span>
    {#if d.venture}
      <a
        href={ventureHref(d.venture)}
        onclick={(e) => e.stopPropagation()}
        class="text-secondary hover:underline"
      >🏢 {d.venture}</a>
    {/if}
    {#if d.goal_id}
      <a
        href={goalHref(d.goal_id)}
        onclick={(e) => e.stopPropagation()}
        class="text-secondary hover:underline"
      >🎯 {goalTitle(d.goal_id)}</a>
    {/if}
    {#if d.project}
      <a
        href={projectHref(d.project)}
        onclick={(e) => e.stopPropagation()}
        class="text-secondary hover:underline"
      >📁 {d.project}</a>
    {/if}
    {#if d.task_ids && d.task_ids.length > 0}
      <span>🔗 {d.task_ids.length} task{d.task_ids.length === 1 ? '' : 's'}</span>
    {/if}

    <span class="ml-auto flex items-center gap-1 opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 transition-opacity">
      {#if !isDone}
        <button
          type="button"
          onclick={(e) => onMarkMet(d, e)}
          class="px-1.5 py-0.5 text-success hover:bg-surface0 rounded"
          title="Mark met"
          aria-label="Mark {d.title} as met"
        >✓</button>
        <button
          type="button"
          onclick={(e) => onSnooze(d, 1, e)}
          class="px-1.5 py-0.5 text-info hover:bg-surface0 rounded"
          title="Snooze 1 day"
          aria-label="Snooze {d.title} 1 day"
        >+1d</button>
        <button
          type="button"
          onclick={(e) => onSnooze(d, 7, e)}
          class="px-1.5 py-0.5 text-info hover:bg-surface0 rounded"
          title="Snooze 1 week"
          aria-label="Snooze {d.title} 1 week"
        >+7d</button>
      {:else}
        <button
          type="button"
          onclick={(e) => onReopen(d, e)}
          class="px-1.5 py-0.5 text-warning hover:bg-surface0 rounded"
          title="Reopen"
          aria-label="Reopen {d.title}"
        >↺</button>
      {/if}
    </span>
  </div>

  {#if d.description}
    <p class="text-sm text-subtext line-clamp-2">{d.description}</p>
  {/if}
</div>

