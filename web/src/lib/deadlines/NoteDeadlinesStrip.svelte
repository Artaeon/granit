<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Deadline } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { daysUntil } from '$lib/deadlines/util';

  // NoteDeadlinesStrip — inline 1-line strip rendered above the editor
  // when the open note's frontmatter ties it to a project or goal that
  // has active deadlines. Lets the user notice "2 deadlines for this
  // project, one's in 12 days" without leaving the note.
  //
  // Defensive design:
  //   - only renders when at least one matching active deadline exists
  //   - matching is exact-equality on `project` (string) or `goal_id`
  //   - silently no-op if /deadlines is unavailable (uses tryListDeadlines)
  //
  // Click → /deadlines?project=Foo (or ?goal_id=G123) — the deadlines
  // page reads the param and scopes its list.

  interface Props {
    /** The open note's frontmatter, if any. */
    frontmatter?: Record<string, unknown> | null;
  }

  let { frontmatter }: Props = $props();

  let all = $state<Deadline[] | null>(null);

  async function load() {
    all = await api.tryListDeadlines();
  }

  onMount(() => {
    load();
    // Refresh on the deadlines.json signal so toggling a deadline elsewhere
    // updates the strip without requiring a reload.
    return onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/deadlines.json') load();
    });
  });

  // Read project / goal_id off the frontmatter. We accept both
  // canonical Granit field names and a couple of common alternates so
  // an older vault doesn't silently miss the strip.
  let project = $derived.by(() => {
    if (!frontmatter) return '';
    const v = frontmatter['project'] ?? frontmatter['Project'];
    return typeof v === 'string' ? v : '';
  });

  let goalId = $derived.by(() => {
    if (!frontmatter) return '';
    const v = frontmatter['goal_id'] ?? frontmatter['goalId'] ?? frontmatter['goal'];
    return typeof v === 'string' ? v : '';
  });

  // Matching deadlines, sorted nearest-first, active only.
  let matching = $derived.by(() => {
    if (!all) return [];
    if (!project && !goalId) return [];
    return all
      .filter((d) => d.status !== 'met' && d.status !== 'cancelled')
      .filter((d) => {
        if (project && d.project === project) return true;
        if (goalId && d.goal_id === goalId) return true;
        return false;
      })
      .map((d) => ({ d, days: daysUntil(d.date) }))
      .sort((a, b) => a.days - b.days);
  });

  // Compact "12d" / "today" / "3d ago" label — saves horizontal space
  // since the strip lists titles inline.
  function tag(days: number): string {
    if (days < 0) return `${Math.abs(days)}d ago`;
    if (days === 0) return 'today';
    if (days === 1) return 'tomorrow';
    if (days < 14) return `${days}d`;
    if (days < 60) return `${Math.round(days / 7)}w`;
    return `${Math.round(days / 30)}mo`;
  }

  let scopeHref = $derived.by(() => {
    if (project) return `/deadlines?project=${encodeURIComponent(project)}`;
    if (goalId) return `/deadlines?goal_id=${encodeURIComponent(goalId)}`;
    return '/deadlines';
  });

  // Hex tone for the leading clock — tracks the most-urgent match so the
  // strip "feels" red when something's overdue, even before reading.
  let tone = $derived.by(() => {
    if (matching.length === 0) return 'subtext';
    const min = matching[0].days;
    if (min < 0 || min <= 3) return 'error';
    if (min <= 7) return 'warning';
    if (min <= 14) return 'info';
    return 'secondary';
  });
</script>

{#if matching.length > 0}
  <a
    href={scopeHref}
    class="flex items-center gap-2 px-3 py-1.5 border-b border-surface1 text-xs hover:bg-surface0 transition-colors"
    style="background: color-mix(in srgb, var(--color-{tone}) 6%, transparent);"
  >
    <span aria-hidden="true">⏰</span>
    <span class="text-dim flex-shrink-0">
      {matching.length} {matching.length === 1 ? 'deadline' : 'deadlines'} for this
      {project ? 'project' : 'goal'}:
    </span>
    <span class="flex-1 min-w-0 truncate text-text">
      {#each matching.slice(0, 3) as { d, days }, i}
        {#if i > 0}<span class="text-dim/60">, </span>{/if}
        <span class="font-medium">{d.title}</span>
        <span style="color: var(--color-{tone});" class="ml-0.5 tabular-nums">({tag(days)})</span>
      {/each}
      {#if matching.length > 3}
        <span class="text-dim/70"> +{matching.length - 3} more</span>
      {/if}
    </span>
    <span class="text-secondary flex-shrink-0">→</span>
  </a>
{/if}
