<script lang="ts">
  import type { Project, Task } from '$lib/api';
  import { startOfIsoWeek } from '$lib/util/isoWeek';
  import { fmtDateISO as ymd } from '$lib/util/date';

  // Tiny stacked horizontal bar showing this project's task breakdown
  // by status: open + scheduled-this-week + done. The list cards
  // already show overall progress (done/total) but the split is more
  // useful — it answers "is the open work being scheduled?" which is
  // the load-bearing question for momentum.

  let {
    project,
    tasks
  }: {
    project: Project;
    tasks: Task[];
  } = $props();

  // Filter tasks to this project using the same matching rule the
  // rest of the page uses. Memoised via $derived so list-page renders
  // don't redo the scan for every card on every keystroke.
  const split = $derived.by(() => {
    const today = ymd(new Date());
    const monStart = ymd(startOfIsoWeek(new Date()));
    const folder = (project.folder ?? '').replace(/\/$/, '');
    let open = 0;
    let scheduled = 0;
    let done = 0;
    for (const t of tasks) {
      const matches = t.projectId === project.name || (folder && t.notePath.startsWith(folder + '/'));
      if (!matches) continue;
      if (t.done) done++;
      else if (t.scheduledStart) {
        const day = t.scheduledStart.slice(0, 10);
        if (day >= monStart && day <= today) scheduled++;
        else open++;
      } else open++;
    }
    return { open, scheduled, done, total: open + scheduled + done };
  });
</script>

{#if split.total > 0}
  <!-- Three-segment stacked bar: open (subtext), scheduled-this-week
       (secondary, the "queued for action" signal), done (success).
       Total < 1px segments are clamped to 6% so a single open/done
       task in a long project is still visible. -->
  <div class="flex items-center gap-2 mt-1.5 text-[10px]" title="open · scheduled this week · done">
    <div class="flex-1 h-1.5 rounded-full bg-surface0 overflow-hidden flex">
      {#if split.open > 0}
        {@const pct = Math.max(6, Math.round((split.open / split.total) * 100))}
        <div class="h-full bg-subtext/50" style="width: {pct}%" title="{split.open} open"></div>
      {/if}
      {#if split.scheduled > 0}
        {@const pct = Math.max(6, Math.round((split.scheduled / split.total) * 100))}
        <div class="h-full bg-secondary" style="width: {pct}%" title="{split.scheduled} scheduled this week"></div>
      {/if}
      {#if split.done > 0}
        {@const pct = Math.max(6, Math.round((split.done / split.total) * 100))}
        <div class="h-full bg-success" style="width: {pct}%" title="{split.done} done"></div>
      {/if}
    </div>
    <span class="font-mono text-dim flex-shrink-0 tabular-nums">
      <span class="text-subtext">{split.open}</span>
      {#if split.scheduled > 0}<span class="text-secondary"> · {split.scheduled}</span>{/if}
      <span class="text-success"> · {split.done}</span>
    </span>
  </div>
{/if}
