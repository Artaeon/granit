<script lang="ts">
  import type { Project, Task } from '$lib/api';

  // Lightweight Gantt-ish timeline. One row per project, a coloured bar
  // spanning created_at → due_date (or to "now" if no due). Pure
  // CSS+SVG, no chart library — the bar is an absolutely-positioned
  // div inside a percentage-tracking row container, and the date axis
  // is rendered as labelled tick marks.
  //
  // Bars are status-coloured (active = project's color; paused =
  // warning; completed = success; archived = dim). Overdue projects
  // (due_date < today and status != completed/archived) render a red
  // outline so the eye picks them out at a glance. Today is drawn as
  // a vertical line so "where are we relative to the plan" is obvious.

  let {
    projects,
    onSelect,
    colorVar
  }: {
    projects: Project[];
    /** Accepted but unused — kept available so future extensions
     *  (e.g. task-density tinting along the bar) can read tasks
     *  without a parent contract change. */
    tasks?: Task[];
    onSelect: (name: string) => void;
    colorVar: (c?: string) => string;
    /** Accepted but unused — same forward-compat reasoning as tasks. */
    statusTone?: (s: string) => string;
  } = $props();

  // Range = [earliest project start … max(latest due, today + 14d)].
  // Padding the right edge means projects with no due_date still
  // render a bar that doesn't kiss the edge of the chart.
  const range = $derived.by(() => {
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    let minMs = today.getTime();
    let maxMs = today.getTime() + 14 * 86400000;
    for (const p of projects) {
      const start = p.created_at ? new Date(p.created_at) : null;
      const end = p.due_date ? new Date(p.due_date) : null;
      if (start && !Number.isNaN(start.getTime()) && start.getTime() < minMs) minMs = start.getTime();
      if (end && !Number.isNaN(end.getTime()) && end.getTime() > maxMs) maxMs = end.getTime();
    }
    // Round bounds to month edges so the axis ticks land on round
    // dates instead of drifting arbitrary days.
    const min = new Date(minMs);
    min.setDate(1);
    min.setHours(0, 0, 0, 0);
    const max = new Date(maxMs);
    max.setMonth(max.getMonth() + 1, 1);
    max.setHours(0, 0, 0, 0);
    return { min, max, span: Math.max(1, max.getTime() - min.getTime()) };
  });

  // Month tick marks across the visible range. Sparse-but-readable —
  // a 3-month range gets 3 ticks, a 12-month range gets 12.
  const ticks = $derived.by(() => {
    const out: { label: string; pct: number }[] = [];
    const cursor = new Date(range.min);
    while (cursor.getTime() < range.max.getTime()) {
      const pct = ((cursor.getTime() - range.min.getTime()) / range.span) * 100;
      const label = cursor.toLocaleString(undefined, { month: 'short', year: '2-digit' });
      out.push({ label, pct });
      cursor.setMonth(cursor.getMonth() + 1);
    }
    return out;
  });

  const todayPct = $derived.by(() => {
    const t = Date.now();
    if (t < range.min.getTime() || t > range.max.getTime()) return -1;
    return ((t - range.min.getTime()) / range.span) * 100;
  });

  function statusBarColor(p: Project): string {
    const s = p.status ?? 'active';
    if (s === 'paused') return 'var(--color-warning)';
    if (s === 'completed') return 'var(--color-success)';
    if (s === 'archived') return 'var(--color-subtext)';
    return colorVar(p.color);
  }

  function rowBar(p: Project): { leftPct: number; widthPct: number; isOpen: boolean; overdue: boolean } | null {
    const start = p.created_at ? new Date(p.created_at) : null;
    if (!start || Number.isNaN(start.getTime())) return null;
    const end = p.due_date ? new Date(p.due_date) : new Date();
    const isOpen = !p.due_date;
    const startMs = Math.max(start.getTime(), range.min.getTime());
    const endMs = Math.min(end.getTime(), range.max.getTime());
    if (endMs < startMs) return null;
    const leftPct = ((startMs - range.min.getTime()) / range.span) * 100;
    const widthPct = Math.max(0.6, ((endMs - startMs) / range.span) * 100);
    const overdue =
      !!p.due_date &&
      end.getTime() < Date.now() &&
      (p.status ?? 'active') !== 'completed' &&
      (p.status ?? 'active') !== 'archived';
    return { leftPct, widthPct, isOpen, overdue };
  }

  function progress(p: Project): number {
    return Math.max(0, Math.min(1, p.progress ?? 0));
  }
</script>

<div class="h-full flex flex-col">
  <!-- Header — count + tiny legend so the bar colours are legible
       without a tooltip. -->
  <div class="px-3 sm:px-4 py-2 border-b border-surface1 flex items-center gap-3 flex-shrink-0 text-xs">
    <span class="text-text font-medium">Timeline</span>
    <span class="text-dim">{projects.length} project{projects.length === 1 ? '' : 's'}</span>
    <span class="flex-1"></span>
    <span class="hidden sm:inline-flex items-center gap-1 text-dim">
      <span class="w-2 h-2 rounded-sm" style="background: var(--color-success)"></span> active/done
    </span>
    <span class="hidden sm:inline-flex items-center gap-1 text-dim">
      <span class="w-2 h-2 rounded-sm" style="background: var(--color-warning)"></span> paused
    </span>
    <span class="hidden sm:inline-flex items-center gap-1 text-dim">
      <span class="w-2 h-2 rounded-sm border border-error" style="background: transparent"></span> overdue
    </span>
  </div>

  {#if projects.length === 0}
    <div class="flex-1 flex items-center justify-center text-dim text-sm italic px-4 text-center">
      No projects match the current filter. Clear the status / venture chips above to see more, or create a new project with the + new button.
    </div>
  {:else}
    <div class="flex-1 overflow-auto">
      <!-- Outer two-column grid — left column is the project name
           label (sticky on horizontal scroll, so on mobile users can
           swipe the chart and the name stays anchored), right column
           is the chart area. -->
      <div class="grid timeline-grid min-w-[700px]">
        <!-- Axis row -->
        <div class="px-3 py-1.5 border-b border-surface1 bg-mantle/60 sticky left-0 z-10 text-[10px] text-dim uppercase tracking-wider font-medium">
          Project
        </div>
        <div class="relative border-b border-surface1 bg-mantle/60 h-7">
          {#each ticks as t (t.pct)}
            <div
              class="absolute top-0 bottom-0 border-l border-surface1/60 text-[9px] text-dim font-mono pl-1"
              style="left: {t.pct}%"
            >{t.label}</div>
          {/each}
          {#if todayPct >= 0}
            <div
              class="absolute top-0 bottom-0 border-l-2 border-primary z-20"
              style="left: {todayPct}%"
              title="today"
            ></div>
          {/if}
        </div>

        <!-- One row per project -->
        {#each projects as p (p.name)}
          {@const bar = rowBar(p)}
          {@const pct = progress(p)}
          <div class="px-3 py-1.5 border-b border-surface1/50 sticky left-0 bg-base z-[5] flex items-center gap-1.5 min-w-0">
            <span class="w-2 h-2 rounded-full flex-shrink-0" style="background: {colorVar(p.color)}"></span>
            <button
              onclick={() => onSelect(p.name)}
              class="text-xs text-text hover:text-primary truncate flex-1 text-left"
              title="open {p.name}"
            >{p.name}</button>
          </div>
          <div class="relative border-b border-surface1/50 h-8">
            <!-- Today line repeats per row so it's always visible at
                 any scroll position (otherwise it disappears as the
                 user scrolls vertically away from the axis). -->
            {#if todayPct >= 0}
              <div class="absolute top-0 bottom-0 border-l border-primary/30 z-[2]" style="left: {todayPct}%"></div>
            {/if}
            {#if bar}
              <button
                onclick={() => onSelect(p.name)}
                class="absolute top-1.5 bottom-1.5 rounded text-left overflow-hidden hover:ring-1 hover:ring-primary transition-all
                       {bar.overdue ? 'ring-1 ring-error' : ''}
                       {bar.isOpen ? 'opacity-80' : ''}"
                style="left: {bar.leftPct}%; width: {bar.widthPct}%; background: {statusBarColor(p)}; min-width: 6px;"
                title="{p.name} · {p.created_at?.slice(0, 10) ?? '—'} → {p.due_date?.slice(0, 10) ?? 'open'} · {Math.round(pct * 100)}%"
              >
                <!-- Inner progress fill — slightly darker overlay
                     showing how much of the project's tasks are done.
                     Renders inside the bar so progress + duration are
                     legible together. -->
                {#if pct > 0}
                  <div
                    class="absolute inset-y-0 left-0 bg-base/30"
                    style="width: {Math.round(pct * 100)}%"
                  ></div>
                {/if}
              </button>
            {/if}
          </div>
        {/each}
      </div>
    </div>
  {/if}
</div>

<style>
  /* CSS grid sized to the project-label column on the left + a flexible
     chart column on the right. Using grid (not flex) keeps every row's
     left column the same width, so the names line up vertically. */
  .timeline-grid {
    grid-template-columns: 12rem 1fr;
  }
  @media (min-width: 768px) {
    .timeline-grid {
      grid-template-columns: 16rem 1fr;
    }
  }
</style>
