<script lang="ts">
  // "Coming up" hero strip for /deadlines. Stream BB. Three
  // most-urgent active rows (critical → high → normal, then earliest
  // date). Replaces the single-hero countdown card with three side-
  // by-side cards because the user often has multiple critical
  // things stacked and showing only one hides the next-on-deck.
  //
  // Border + urgency-pill colour are driven by days-until (NOT
  // importance) — the strip's job is "what's about to happen next",
  // so a critical-but-distant deadline shouldn't burn red while a
  // normal-tomorrow does. Importance still tints the title's left
  // border on the list rows below.
  import type { Deadline } from '$lib/api';
  import { daysUntil } from '$lib/deadlines/util';

  type Props = {
    rows: Deadline[];
    goalTitle: (id?: string) => string;
    onOpen: (d: Deadline) => void;
  };

  let { rows, goalTitle, onOpen }: Props = $props();
</script>

{#if rows.length > 0}
  <div class="mb-5">
    <div class="text-[11px] uppercase tracking-wider text-dim mb-2">Coming up</div>
    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
      {#each rows as h (h.id)}
        {@const days = daysUntil(h.date)}
        {@const urgencyBg = days < 0 || days <= 3
          ? '#ff453a'
          : days <= 7
            ? '#ff9f0a'
            : days <= 14
              ? '#0a84ff'
              : 'var(--color-secondary)'}
        {@const urgencyLabel = days < 0
          ? `${Math.abs(days)}d overdue`
          : days === 0
            ? 'Today'
            : days === 1
              ? 'Tomorrow'
              : `in ${days} days`}
        <button
          type="button"
          onclick={() => onOpen(h)}
          class="text-left block p-3 sm:p-4 bg-surface0 border-l-4 rounded-lg hover:bg-surface1 transition-colors"
          style="border-left-color: {urgencyBg};"
        >
          <div class="flex items-start gap-2.5">
            <span
              class="w-3 h-3 mt-1.5 rounded-full flex-shrink-0"
              style="background: {urgencyBg};"
              aria-hidden="true"
            ></span>
            <div class="flex-1 min-w-0">
              <div class="text-base font-semibold text-text leading-tight truncate" title={h.title}>
                {h.title}
              </div>
              <div class="mt-1.5 flex items-center gap-2 text-xs">
                <span
                  class="inline-flex px-2 py-0.5 rounded font-semibold uppercase tracking-wide"
                  style="background: {urgencyBg}; color: #ffffff;"
                >{urgencyLabel}</span>
                <span class="text-subtext font-mono">{h.date}</span>
              </div>
              {#if h.project || h.goal_id || h.venture}
                <div class="flex flex-wrap items-center gap-x-3 gap-y-0.5 mt-2 text-[11px] text-subtext">
                  {#if h.venture}<span class="truncate">{h.venture}</span>{/if}
                  {#if h.goal_id}<span class="truncate">{goalTitle(h.goal_id)}</span>{/if}
                  {#if h.project}<span class="truncate">{h.project}</span>{/if}
                </div>
              {/if}
            </div>
          </div>
        </button>
      {/each}
    </div>
  </div>
{/if}
