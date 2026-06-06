<!--
  TasksEmptyStates — the empty-cell branches for the dataCtl view body.
  Rendered by the parent only when `filterCtl.filtered.length === 0`
  (and not still loading). Internal switch on `view` picks the right
  tone:

    • today      — "Today is clear" (calm-celebratory)
    • review     — "No tasks completed in the last 7 days"
    • inbox      — "Inbox empty"
    • stale      — "No stale tasks"
    • quickwins  — "No quick wins available"
    • totalTasks === 0 — onboarding hint at the quick-add bar
    • else       — filter-masked "No tasks here" with subtitle + CTAs

  Order matters: the view-specific branches win over the generic
  totalTasks-zero branch, so a fresh user landing on the inbox view
  sees "inbox empty" not the onboarding card.
-->
<script lang="ts">
  import type { View } from './tasksHelpers';

  type Props = {
    view: View;
    /** dataCtl.tasks.length — used to distinguish onboarding (no tasks
     *  at all) from filter-masked (tasks exist, none match). */
    totalTasks: number;
    /** Adaptive subtitle for the filter-masked branch, derived in the
     *  parent against the active filter set. */
    emptyStateSubtitle: string;
    onSwitchView: (v: View) => void;
    onClearAll: () => void;
    onQuickCapture: () => void;
  };

  let {
    view,
    totalTasks,
    emptyStateSubtitle,
    onSwitchView,
    onClearAll,
    onQuickCapture
  }: Props = $props();
</script>

{#if view === 'today'}
  <!-- Today view inbox-zero message. Different from a true empty
       state — the user has tasks, just none for today. The
       tone is calm-celebratory rather than the cobwebbed
       "get to work" used by the Review view. -->
  <div class="max-w-md mx-auto py-6 text-center">
    <div class="text-4xl mb-3 opacity-50">🌤</div>
    <h2 class="text-base font-medium text-text mb-1">Today is clear</h2>
    <p class="text-sm text-dim">
      Nothing overdue, nothing due today, nothing scheduled. Take the open space — or pick something from
      <button class="text-primary hover:underline" onclick={() => onSwitchView('list')}>the full list</button>.
    </p>
  </div>
{:else if view === 'review'}
  <div class="bg-surface0 border border-surface1 rounded-lg p-5 text-center">
    <p class="text-sm text-text mb-1">No tasks completed in the last 7 days.</p>
    <p class="text-xs text-dim mb-3">The review tab shows what you've finished — once a few tasks roll through, this is where you'll spot patterns.</p>
    <button
      type="button"
      onclick={() => onSwitchView('list')}
      class="text-xs px-3 py-1.5 bg-primary text-on-primary rounded font-medium hover:opacity-90"
    >Open task list →</button>
  </div>
{:else if view === 'inbox'}
  <div class="bg-surface0 border border-surface1 rounded-lg p-5 text-center">
    <p class="text-sm text-success mb-1">Inbox empty.</p>
    <p class="text-xs text-dim mb-3">Nothing waiting to be triaged. Captured tasks land here for sorting before they hit the main list.</p>
    <button
      type="button"
      onclick={() => onSwitchView('list')}
      class="text-xs px-3 py-1.5 bg-surface1 border border-surface2 text-text rounded font-medium hover:border-primary"
    >Open task list →</button>
  </div>
{:else if view === 'stale'}
  <div class="bg-surface0 border border-surface1 rounded-lg p-5 text-center">
    <p class="text-sm text-success mb-1">No stale tasks.</p>
    <p class="text-xs text-dim">Everything's been touched in the last week — nothing rotting in the backlog.</p>
  </div>
{:else if view === 'quickwins'}
  <p class="text-sm text-dim italic">No quick wins available. Add an estimate (e.g. <code class="text-secondary">est:30m</code>) to high-priority tasks.</p>
{:else if totalTasks === 0}
  <!-- True empty: no tasks anywhere. Onboarding-style hint
       pointing at the quick-add bar. -->
  <div class="max-w-md mx-auto py-6 text-center">
    <div class="text-5xl mb-3 opacity-30">✓</div>
    <h2 class="text-lg font-semibold text-text mb-2">No tasks yet</h2>
    <p class="text-sm text-dim mb-1">
      Type your first task in the bar above. Examples:
    </p>
    <ul class="text-sm text-subtext font-mono mt-3 space-y-1.5 inline-block text-left">
      <li>fix login bug <span class="text-error">!1</span> <span class="text-secondary">due:tomorrow</span></li>
      <li>buy groceries <span class="text-info">#errands</span></li>
      <li>review PR <span class="text-warning">!2</span> <span class="text-secondary">due:fri</span></li>
    </ul>
  </div>
{:else}
  <!-- Tasks exist but the active filter masks them all. The
       subtitle adapts to which filter is the dominant signal
       (tag / project / goal / priority / search) so the user
       reads "No tasks tagged #X" instead of a generic "no
       matches". Two CTAs: Quick capture for fast entry and
       Clear filters for the reset path. min-w-0 keeps the
       card from overrunning the sidebar on narrow viewports. -->
  <div class="min-w-0">
    <div class="max-w-md mx-auto py-6 text-center">
      <div class="text-4xl mb-3 opacity-30">🔍</div>
      <h2 class="text-base font-medium text-text mb-2">No tasks here</h2>
      <p class="text-sm text-dim mb-1">{emptyStateSubtitle}</p>
      <p class="text-xs text-dim mb-4">
        {totalTasks} {totalTasks === 1 ? 'task is' : 'tasks are'} hidden by the current filters.
      </p>
      <div class="flex items-center justify-center gap-2 flex-wrap">
        <button
          type="button"
          onclick={onQuickCapture}
          class="px-3 py-1.5 bg-primary text-on-primary rounded text-sm font-medium hover:opacity-90 inline-flex items-center gap-1.5"
          title="Open the global capture modal (Cmd-Shift-N)"
        >
          <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M12 5v14M5 12h14"/></svg>
          Quick capture
        </button>
        <button
          type="button"
          onclick={onClearAll}
          class="px-3 py-1.5 bg-surface0 border border-surface1 hover:border-primary rounded text-sm text-subtext"
        >Clear filters</button>
      </div>
    </div>
  </div>
{/if}
