<script lang="ts">
  import type { DeadlineImportance, DeadlineStatus } from '$lib/api';

  // DeadlinePill — single source of truth for how a deadline's
  // urgency + importance render across the app. Used by:
  //   - dashboard's TopDeadlinesWidget (compact row pill)
  //   - /deadlines page (list rows + countdown card)
  //   - morning routine's anchors deadline strip
  //   - note-page's project/goal deadline strip
  //
  // Two display modes:
  //   variant="countdown"  — shows "in 12d" / "today" / "3d overdue"
  //                          colored by urgency (red ≤3, orange ≤7,
  //                          neutral later, error past).
  //   variant="importance" — shows the importance icon + label, colored
  //                          by importance tone.
  //
  // Pass either `days` (precomputed) OR `iso` (we'll compute days from
  // today). For importance we accept the raw value off the deadline.

  interface Props {
    /** Render mode. */
    variant?: 'countdown' | 'importance' | 'icon';
    /** Days until the deadline; negative = overdue. Required for countdown. */
    days?: number;
    /** Importance level (critical / high / normal). Required for importance. */
    importance?: DeadlineImportance;
    /** Deadline status — `met` / `cancelled` short-circuits countdown. */
    status?: DeadlineStatus;
    /** Compact = smaller text + tighter padding (dashboard rows). */
    size?: 'sm' | 'md';
  }

  let {
    variant = 'countdown',
    days,
    importance = 'normal',
    status,
    size = 'sm'
  }: Props = $props();

  // Urgency tone (countdown variant).
  function urgencyTone(d: number): string {
    if (d < 0) return 'error';
    if (d <= 3) return 'error';
    if (d <= 7) return 'warning';
    if (d <= 14) return 'info';
    return 'subtext';
  }

  function importanceTone(i: DeadlineImportance): string {
    switch (i) {
      case 'critical':
        return 'error';
      case 'high':
        return 'warning';
      default:
        return 'secondary';
    }
  }

  // Coloured circle that doubles as a status icon for the row title.
  // Reused by widget rows + countdown card. We use plain emoji
  // (matches the prompt) because they render consistently across iOS /
  // Android / desktop browsers without an icon font dependency.
  export function importanceIcon(i: DeadlineImportance): string {
    switch (i) {
      case 'critical':
        return '🔴';
      case 'high':
        return '🟠';
      default:
        return '🟢';
    }
  }

  function relLabel(d: number): string {
    if (d < 0) return `${Math.abs(d)}d overdue`;
    if (d === 0) return 'today';
    if (d === 1) return 'tomorrow';
    if (d < 14) return `in ${d}d`;
    if (d < 60) return `in ${Math.round(d / 7)}w`;
    return `in ${Math.round(d / 30)}mo`;
  }

  let tone = $derived.by(() => {
    if (variant === 'importance') return importanceTone(importance);
    if (status === 'met') return 'success';
    if (status === 'cancelled') return 'subtext';
    return urgencyTone(days ?? 0);
  });

  let label = $derived.by(() => {
    if (variant === 'importance') return importance;
    if (status === 'met') return 'met';
    if (status === 'cancelled') return 'cancelled';
    return relLabel(days ?? 0);
  });

  let pad = $derived(size === 'sm' ? 'px-1.5 py-0.5 text-[10px]' : 'px-2 py-1 text-xs');
</script>

{#if variant === 'icon'}
  <span class="inline-block flex-shrink-0" aria-label={importance} title={importance}>{importanceIcon(importance)}</span>
{:else}
  <span
    class="inline-flex items-center {pad} rounded font-medium tabular-nums whitespace-nowrap uppercase tracking-wider"
    style="background: color-mix(in srgb, var(--color-{tone}) 14%, transparent); color: var(--color-{tone}); border: 1px solid color-mix(in srgb, var(--color-{tone}) 30%, transparent);"
  >
    {label}
  </span>
{/if}
