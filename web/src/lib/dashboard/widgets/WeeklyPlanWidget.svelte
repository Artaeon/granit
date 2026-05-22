<script lang="ts">
  // WeeklyPlanWidget — surfaces this week's committed work, grouped
  // by venture, with a done/total count per group. Reads tasks whose
  // notePath matches the current week's plan note (Plans/<weekISO>.md).
  // If no plan exists yet for this week, shows a quiet CTA to /plans/week.
  //
  // This is the bridge between the weekly planning ritual and the
  // daily view: once the user has done their Sunday brain-dump and
  // committed it, the today surface shows what they actually
  // committed to — not just whatever's due.
  //
  // Section parsing: tasks live under "### <Venture>" headings in the
  // plan note. We re-derive the venture from each task's heading by
  // scanning the note body and matching task line numbers to the
  // most recent "### " heading above them. Cheap, no new endpoint.

  import { onMount, onDestroy } from 'svelte';
  import { api, type Task } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { isoWeekString, planNotePath } from '$lib/util/isoWeek';
  import { createCoalescedReload } from '$lib/util/coalesce';

  const weekISO = isoWeekString();
  const planPath = planNotePath();

  let tasks = $state<Task[]>([]);
  let buckets = $state<{ venture: string; total: number; done: number }[]>([]);
  let loading = $state(true);
  let planExists = $state(false);

  async function load() {
    loading = true;
    try {
      // Fetch the plan note body so we can map task lines to their
      // venture section heading. If the note doesn't exist, the user
      // hasn't planned this week yet — show the CTA.
      let body = '';
      try {
        const note = await api.getNote(planPath);
        body = note.body ?? '';
        planExists = true;
      } catch {
        planExists = false;
        tasks = [];
        buckets = [];
        return;
      }
      // Server-side filter by notePath — the listTasks endpoint
      // already supports it, so we don't pull the whole vault's
      // task list just to drop 99% of it.
      const res = await api.listTasks({ note: planPath, includeArchived: false });
      tasks = res.tasks;
      buckets = computeBuckets(body, tasks);
    } finally {
      loading = false;
    }
  }

  // Walk the plan note body line-by-line, tracking the most recent
  // "### <Venture>" heading. For each task, look up the venture by
  // its lineNum.
  //
  // Important — only count tasks that actually sit UNDER a "### "
  // subheading. If the user wrote a checkbox in the freeform "## Plan"
  // body (above any subheading), TaskStore still picks it up, but
  // it's not a committed item — it's an unstructured thought.
  // Surfacing it as 'Personal' would over-report committed work.
  function computeBuckets(body: string, ts: Task[]): { venture: string; total: number; done: number }[] {
    const lines = body.split('\n');
    const headingByLine: string[] = new Array(lines.length + 1).fill('');
    let cur = '';
    for (let i = 0; i < lines.length; i++) {
      const m = lines[i].match(/^###\s+(.+)$/);
      if (m) cur = m[1].trim();
      // line numbers in Task are 1-indexed
      headingByLine[i + 1] = cur;
    }
    const map = new Map<string, { total: number; done: number }>();
    for (const t of ts) {
      const venture = headingByLine[t.lineNum];
      if (!venture) continue; // pre-heading body checkbox → not a commitment
      const cur = map.get(venture) ?? { total: 0, done: 0 };
      cur.total += 1;
      if (t.done) cur.done += 1;
      map.set(venture, cur);
    }
    return [...map.entries()]
      .map(([venture, c]) => ({ venture, ...c }))
      .sort((a, b) => {
        // Personal last, others alphabetical.
        if (a.venture === 'Personal' && b.venture !== 'Personal') return 1;
        if (b.venture === 'Personal' && a.venture !== 'Personal') return -1;
        return a.venture.localeCompare(b.venture);
      });
  }

  const reload = createCoalescedReload(load, 600);
  onMount(() => {
    void load();
    // Reload on task changes targeting this plan note (someone ticked
    // a task elsewhere) or on note edits to the plan itself.
    return onWsEvent((ev) => {
      if (ev.type === 'task.changed' || ev.type === 'note.changed') {
        reload.trigger();
      }
    });
  });
  onDestroy(() => reload.cancel());

  let totalDone = $derived(buckets.reduce((s, b) => s + b.done, 0));
  let totalTotal = $derived(buckets.reduce((s, b) => s + b.total, 0));
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-3 flex flex-col h-full">
  <div class="flex items-baseline justify-between mb-2">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">This week's commitments</h2>
    <a href="/plans/week" class="text-xs text-secondary hover:underline">open →</a>
  </div>

  {#if loading}
    <p class="text-sm text-dim italic">loading…</p>
  {:else if !planExists}
    <!-- No plan note for this ISO week yet. Quiet CTA — never nags
         on Monday morning, just informs. -->
    <p class="text-sm text-subtext mb-2">No plan for <span class="font-mono text-dim">{weekISO}</span> yet.</p>
    <a href="/plans/week" class="self-start text-xs px-2.5 py-1 rounded border border-surface1 bg-surface0 hover:bg-surface1 text-subtext">
      brain-dump the week →
    </a>
  {:else if buckets.length === 0}
    <p class="text-sm text-dim italic">Plan exists but no committed tasks. <a href="/plans/week" class="text-secondary hover:underline">commit some →</a></p>
  {:else}
    <p class="text-[11px] text-dim mb-2 font-mono">{totalDone}/{totalTotal} done · {weekISO}</p>
    <ul class="space-y-1 text-sm flex-1">
      {#each buckets as b (b.venture)}
        <li class="flex items-baseline justify-between gap-2">
          <a
            href="/tasks?file={encodeURIComponent(planPath)}"
            class="text-text hover:underline truncate"
            title="see all {b.venture} tasks from this week"
          >{b.venture}</a>
          <span class="text-dim font-mono text-[11px] tabular-nums shrink-0">
            {b.done}/{b.total}
          </span>
        </li>
      {/each}
    </ul>
  {/if}
</section>
