<script lang="ts">
  // One entry in the jots feed: sticky-header date strip + body
  // (rendered markdown) + collapsed "what happened that day" details
  // block. The page hands in the cached dayActivity items + per-date
  // loading flags so this component can render inline counts without
  // owning the fetch lifecycle.
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';
  import type { DayActivityItem, Jot } from '$lib/api';

  type Props = {
    jot: Jot;
    today: Date;
    activity: DayActivityItem[] | undefined;
    activityLoading: boolean;
    onWikilink: (target: string) => void;
    onExpandActivity: (date: string) => void;
  };

  let {
    jot,
    today,
    activity,
    activityLoading,
    onWikilink,
    onExpandActivity
  }: Props = $props();

  // ── activity bucketing ─────────────────────────────────────────────
  // Group + label the per-day items so the panel renders a labelled
  // bucket per Kind. Order is fixed so the layout stays stable across
  // re-renders even as items shift.
  type Bucket = { kind: string; label: string; items: DayActivityItem[] };
  const KIND_LABELS: Record<string, string> = {
    note_created: 'Notes created',
    task_created: 'Tasks created',
    task_completed: 'Tasks completed',
    event: 'Calendar',
    habit: 'Habits',
    prayer: 'Prayer',
    hub_item: 'Hub',
    jot: 'Jots'
  };
  const BUCKET_ORDER: string[] = [
    'event',
    'task_created',
    'task_completed',
    'note_created',
    'jot',
    'habit',
    'prayer',
    'hub_item'
  ];

  function bucketize(items: DayActivityItem[]): Bucket[] {
    const groups = new Map<string, DayActivityItem[]>();
    for (const it of items) {
      const arr = groups.get(it.kind) ?? [];
      arr.push(it);
      groups.set(it.kind, arr);
    }
    const out: Bucket[] = [];
    for (const k of BUCKET_ORDER) {
      const arr = groups.get(k);
      if (arr && arr.length > 0) {
        out.push({ kind: k, label: KIND_LABELS[k] ?? k, items: arr });
      }
    }
    for (const [k, arr] of groups) {
      if (BUCKET_ORDER.indexOf(k) === -1 && arr.length > 0) {
        out.push({ kind: k, label: KIND_LABELS[k] ?? k, items: arr });
      }
    }
    return out;
  }

  function activityHref(it: DayActivityItem): string {
    if (it.path) return `/notes/${encodeURIComponent(it.path)}`;
    if (it.kind === 'event') return '/calendar';
    if (it.kind === 'prayer') return '/prayer';
    if (it.kind === 'hub_item') return '/hub';
    if (it.kind === 'habit') return '/habits';
    if (it.kind === 'task_created' || it.kind === 'task_completed') return '/tasks';
    return '#';
  }

  function activityTime(at: string): string {
    const d = new Date(at);
    if (Number.isNaN(d.getTime())) return '';
    const hh = String(d.getHours()).padStart(2, '0');
    const mm = String(d.getMinutes()).padStart(2, '0');
    return `${hh}:${mm}`;
  }

  // Inline-header counts: at-a-glance summary by Kind. Four buckets
  // that matter most without expanding: events, tasks done, tasks
  // created, notes. Habits/prayer/hub stay inside the details panel.
  type ActivitySummary = {
    events: number;
    tasksCreated: number;
    tasksDone: number;
    notes: number;
    total: number;
  };
  function summarize(items: DayActivityItem[] | undefined): ActivitySummary {
    const s: ActivitySummary = { events: 0, tasksCreated: 0, tasksDone: 0, notes: 0, total: 0 };
    if (!items) return s;
    for (const it of items) {
      s.total += 1;
      switch (it.kind) {
        case 'event': s.events += 1; break;
        case 'task_created': s.tasksCreated += 1; break;
        case 'task_completed': s.tasksDone += 1; break;
        case 'note_created': s.notes += 1; break;
      }
    }
    return s;
  }

  function relativeLabel(date: string, today: Date): string {
    const d = new Date(date + 'T00:00:00');
    const diff = Math.round((d.getTime() - today.getTime()) / 86400000);
    if (diff === 0) return 'Today';
    if (diff === -1) return 'Yesterday';
    if (diff === 1) return 'Tomorrow';
    if (diff > -7 && diff < 7) {
      return d.toLocaleDateString(undefined, { weekday: 'long' });
    }
    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
  }

  let summary = $derived(summarize(activity));
</script>

<article>
  <header
    class="sticky top-[2.5rem] z-10 -mx-1 px-1 py-1 bg-base flex items-baseline gap-2 mb-1.5 border-b border-surface1/60"
  >
    <h2 class="text-sm font-semibold text-text">
      {relativeLabel(jot.date, today)}
    </h2>
    <span class="text-[11px] text-dim hidden sm:inline font-mono">{jot.date}</span>
    {#if jot.openTasks > 0}
      <span
        class="text-[10px] px-1 py-0.5 rounded bg-surface1 text-text font-mono"
        title="{jot.openTasks} open task{jot.openTasks === 1 ? '' : 's'} in this daily"
      >{jot.openTasks}☐</span>
    {/if}
    {#if activity}
      {#if summary.total > 0}
        <span class="flex items-baseline gap-1 text-[10px] font-mono text-dim">
          {#if summary.events > 0}
            <span class="text-text" title="{summary.events} calendar event{summary.events === 1 ? '' : 's'}">{summary.events}cal</span>
          {/if}
          {#if summary.tasksDone > 0}
            <span class="text-text" title="{summary.tasksDone} task{summary.tasksDone === 1 ? '' : 's'} completed">{summary.tasksDone}✓</span>
          {/if}
          {#if summary.tasksCreated > 0}
            <span title="{summary.tasksCreated} task{summary.tasksCreated === 1 ? '' : 's'} created">+{summary.tasksCreated}</span>
          {/if}
          {#if summary.notes > 0}
            <span title="{summary.notes} note{summary.notes === 1 ? '' : 's'} created">{summary.notes}n</span>
          {/if}
        </span>
      {/if}
    {/if}
    <a
      href="/notes/{encodeURIComponent(jot.path)}"
      class="ml-auto text-[11px] text-text hover:underline opacity-70 hover:opacity-100"
    >open →</a>
  </header>
  <div class="bg-surface0 border border-surface1 rounded p-2.5">
    {#if jot.body.trim()}
      <MarkdownRenderer body={jot.body} onWikilink={onWikilink} />
    {:else}
      <p class="text-xs text-dim italic">empty</p>
    {/if}
  </div>

  <details
    class="mt-1 bg-surface0 border border-surface1 rounded text-sm"
    ontoggle={(e) => {
      if ((e.currentTarget as HTMLDetailsElement).open) {
        onExpandActivity(jot.date);
      }
    }}
  >
    <summary class="cursor-pointer px-2 py-1 text-[10px] uppercase tracking-[0.18em] text-dim hover:text-text select-none">
      what happened that day
    </summary>
    <div class="px-2.5 pb-2.5 pt-1">
      {#if activityLoading && activity === undefined}
        <p class="text-xs text-dim italic">loading…</p>
      {:else if (activity?.length ?? 0) === 0}
        {#if activity !== undefined}
          <p class="text-xs text-dim italic">No tracked activity on this day.</p>
        {/if}
      {:else}
        {#each bucketize(activity ?? []) as bucket (bucket.kind)}
          <div class="mb-2 last:mb-0">
            <h4 class="text-[10px] uppercase tracking-[0.18em] text-text font-medium mb-0.5">
              {bucket.label} <span class="text-dim font-normal">({bucket.items.length})</span>
            </h4>
            <ul>
              {#each bucket.items as it (it.kind + ':' + (it.target_id ?? it.path ?? it.title) + ':' + it.at)}
                <li class="flex items-baseline gap-1.5 text-[11px] leading-relaxed">
                  <span class="text-dim font-mono w-9 shrink-0">{activityTime(it.at)}</span>
                  <a
                    href={activityHref(it)}
                    class="text-text hover:underline truncate"
                  >{it.title}</a>
                  {#if it.detail}
                    <span class="text-dim truncate">· {it.detail}</span>
                  {/if}
                </li>
              {/each}
            </ul>
          </div>
        {/each}
      {/if}
    </div>
  </details>
</article>
