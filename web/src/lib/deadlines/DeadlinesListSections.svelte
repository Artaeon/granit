<script lang="ts">
  // Sectioned list for /deadlines. Stream BB. Mirrors the tasks
  // SectionList pattern: each bucket renders as a section with a
  // tone-tinted header, count, and optional left border + tint when
  // the bucket carries urgency (overdue / this_week / missed).
  //
  // Collapse state persists at the parent level so it survives
  // reloads. Empty buckets render as a single muted header line so
  // the page doesn't go blank when filters narrow hard.
  import type { Deadline } from '$lib/api';
  import DeadlineCard from '$lib/deadlines/DeadlineCard.svelte';

  type Bucket = string;
  type GroupBy = 'urgency' | 'status' | 'month';

  type Props = {
    grouped: Map<Bucket, Deadline[]>;
    groupBy: GroupBy;
    bucketTitle: (b: Bucket) => string;
    bucketTone: (b: Bucket) => string;
    countdown: (d: Deadline) => string;
    goalTitle: (id?: string) => string;
    projectHref: (name: string) => string;
    goalHref: (id: string) => string;
    ventureHref: (name: string) => string;
    collapsedSections: Record<string, boolean>;
    onToggleSection: (key: string) => void;
    onOpen: (d: Deadline) => void;
    onMarkMet: (d: Deadline, e: MouseEvent) => void;
    onSnooze: (d: Deadline, days: number, e: MouseEvent) => void;
    onReopen: (d: Deadline, e: MouseEvent) => void;
  };

  let {
    grouped,
    groupBy,
    bucketTitle,
    bucketTone,
    countdown,
    goalTitle,
    projectHref,
    goalHref,
    ventureHref,
    collapsedSections,
    onToggleSection,
    onOpen,
    onMarkMet,
    onSnooze,
    onReopen
  }: Props = $props();

  // Per-bucket-tone visual signature — keeps a single style table so
  // tweaks land everywhere. `border` is only applied to buckets that
  // carry urgency so a calm month-bucket reads as neutral.
  function toneStyle(tone: string, key: string): {
    dot: string;
    label: string;
    tint: string;
    border: string;
    pulse: boolean;
  } {
    // Special case: 'overdue' under urgency grouping deserves the
    // loudest treatment in the page — same pattern tasks uses.
    if (groupBy === 'urgency' && key === 'overdue') {
      return {
        dot: 'bg-error',
        label: 'text-error',
        tint: 'bg-error/[0.06]',
        border: 'border-l-2 border-error pl-2',
        pulse: true
      };
    }
    switch (tone) {
      case 'error':
        return { dot: 'bg-error', label: 'text-error', tint: 'bg-error/[0.05]', border: 'border-l-2 border-error pl-2', pulse: false };
      case 'warning':
        return { dot: 'bg-warning', label: 'text-warning', tint: 'bg-warning/[0.04]', border: 'border-l-2 border-warning pl-2', pulse: false };
      case 'info':
        return { dot: 'bg-info', label: 'text-info', tint: '', border: '', pulse: false };
      case 'success':
        return { dot: 'bg-success', label: 'text-success', tint: '', border: '', pulse: false };
      case 'secondary':
        return { dot: 'bg-secondary', label: 'text-text', tint: '', border: '', pulse: false };
      default:
        return { dot: 'bg-surface2', label: 'text-dim', tint: '', border: '', pulse: false };
    }
  }

  // Default collapse for tail buckets (met / cancelled / dim months).
  // Explicit user toggles in `collapsedSections` override; absence
  // falls through to "open" for live work, "collapsed" for archive
  // buckets so the user sees what matters first.
  function defaultCollapsed(b: Bucket, tone: string): boolean {
    if (groupBy === 'urgency') return b === 'met' || b === 'cancelled';
    if (groupBy === 'status') return b === 'cancelled';
    // month — collapse past months by default; tone is 'dim' for them.
    return tone === 'dim';
  }

  function isCollapsed(b: Bucket, tone: string): boolean {
    const explicit = collapsedSections[b];
    if (explicit === true) return true;
    if (explicit === false) return false;
    return defaultCollapsed(b, tone);
  }
</script>

<div class="space-y-3">
  {#each Array.from(grouped.entries()) as [b, rows] (b)}
    {#if rows.length > 0}
      {@const tone = bucketTone(b)}
      {@const s = toneStyle(tone, b)}
      {@const collapsed = isCollapsed(b, tone)}
      <section class="rounded {s.tint} {s.border}">
        <header class="flex items-center gap-2 py-1.5">
          <button
            type="button"
            onclick={() => onToggleSection(b)}
            class="inline-flex items-center gap-2 group flex-1 min-w-0 text-left"
            aria-expanded={!collapsed}
            aria-controls={`dl-sect-${b}`}
            title={collapsed ? 'Expand section' : 'Collapse section'}
          >
            <svg
              viewBox="0 0 24 24"
              class="w-3 h-3 text-dim flex-shrink-0 transition-transform {collapsed ? '' : 'rotate-90'}"
              fill="none"
              stroke="currentColor"
              stroke-width="2.5"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <path d="M9 6l6 6-6 6" />
            </svg>
            <span class="w-2 h-2 rounded-full flex-shrink-0 {s.dot} {s.pulse ? 'animate-pulse' : ''}" aria-hidden="true"></span>
            <h2 class="text-sm font-semibold {s.label} truncate">{bucketTitle(b)}</h2>
            <span class="text-xs text-dim font-mono tabular-nums flex-shrink-0">{rows.length}</span>
            {#if groupBy === 'urgency' && b === 'overdue'}
              <span class="ml-0.5 px-1 py-0 bg-error/10 text-error text-[9px] tracking-wider rounded uppercase font-bold flex-shrink-0" title="Past their due date">
                past due
              </span>
            {/if}
          </button>
        </header>

        {#if !collapsed}
          <ul id={`dl-sect-${b}`} class="space-y-1.5 pb-1">
            {#each rows as d (d.id)}
              <li>
                <DeadlineCard
                  {d}
                  countdown={countdown(d)}
                  {goalTitle}
                  {projectHref}
                  {goalHref}
                  {ventureHref}
                  onOpen={onOpen}
                  onMarkMet={onMarkMet}
                  onSnooze={onSnooze}
                  onReopen={onReopen}
                />
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    {/if}
  {/each}
</div>

