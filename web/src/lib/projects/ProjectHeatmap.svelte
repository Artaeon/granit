<script lang="ts">
  import type { Project, Task } from '$lib/api';

  // Workload heatmap — projects (rows) × ISO weeks (columns) where
  // each cell encodes that project's task-completion volume in that
  // week. Cell intensity = count / global max so a single hot week
  // doesn't make every other cell look dead. Empty cells are visibly
  // empty (not just dim) so a "this project went dark for 3 weeks"
  // pattern jumps out.
  //
  // Cells are clickable into the project detail. The current week is
  // bordered so 'now' is anchored. Pure CSS grid — no chart library.

  let {
    projects,
    tasks,
    onSelect,
    colorVar
  }: {
    projects: Project[];
    tasks: Task[];
    onSelect: (name: string) => void;
    colorVar: (c?: string) => string;
  } = $props();

  // 12 weeks of history. Long enough to spot a multi-week dip,
  // short enough to fit on a phone in landscape. Could be made
  // user-configurable later, but 12 is a sensible default that
  // matches "a quarter".
  const WEEKS = 12;

  function isoWeekKey(d: Date): string {
    const t = new Date(Date.UTC(d.getFullYear(), d.getMonth(), d.getDate()));
    const day = (t.getUTCDay() + 6) % 7;
    t.setUTCDate(t.getUTCDate() - day + 3);
    const firstThu = new Date(Date.UTC(t.getUTCFullYear(), 0, 4));
    const week = 1 + Math.round((t.getTime() - firstThu.getTime()) / (7 * 24 * 60 * 60 * 1000));
    return `${t.getUTCFullYear()}-W${String(week).padStart(2, '0')}`;
  }
  function startOfIsoWeek(d: Date): Date {
    const t = new Date(d);
    const day = (t.getDay() + 6) % 7;
    t.setDate(t.getDate() - day);
    t.setHours(0, 0, 0, 0);
    return t;
  }

  const weekOrder = $derived.by(() => {
    const start = startOfIsoWeek(new Date());
    const out: { key: string; label: string; isThisWeek: boolean }[] = [];
    const thisKey = isoWeekKey(new Date());
    for (let i = WEEKS - 1; i >= 0; i--) {
      const d = new Date(start);
      d.setDate(d.getDate() - i * 7);
      const k = isoWeekKey(d);
      out.push({
        key: k,
        // "W14" — short. The full year+week label is on the cell title.
        label: 'W' + k.split('W')[1],
        isThisWeek: k === thisKey
      });
    }
    return out;
  });

  // Map: projectName -> Map<weekKey, count>
  const cells = $derived.by(() => {
    const out = new Map<string, Map<string, number>>();
    const validKeys = new Set(weekOrder.map((w) => w.key));
    for (const p of projects) out.set(p.name, new Map());
    for (const t of tasks) {
      if (!t.done || !t.completedAt) continue;
      const k = isoWeekKey(new Date(t.completedAt));
      if (!validKeys.has(k)) continue;
      // Match the same project-membership rule the rest of the page
      // uses (explicit projectId OR notePath under folder) so cells
      // and the detail page agree exactly.
      for (const p of projects) {
        const folder = (p.folder ?? '').replace(/\/$/, '');
        if (t.projectId === p.name || (folder && t.notePath.startsWith(folder + '/'))) {
          const inner = out.get(p.name);
          if (inner) inner.set(k, (inner.get(k) ?? 0) + 1);
        }
      }
    }
    return out;
  });

  // Global max across all cells — the cell-intensity scale. Without
  // this, every project's hottest week looks the same intensity, so
  // you can't compare project A against project B at a glance.
  const globalMax = $derived.by(() => {
    let m = 0;
    for (const inner of cells.values()) {
      for (const v of inner.values()) if (v > m) m = v;
    }
    return Math.max(1, m);
  });

  // Per-row totals so each project shows "32 done · 12w" alongside
  // the cells. The number complements the visual: cells show
  // distribution, total shows magnitude.
  const rowTotals = $derived.by(() => {
    const out = new Map<string, number>();
    for (const [name, inner] of cells) {
      let s = 0;
      for (const v of inner.values()) s += v;
      out.set(name, s);
    }
    return out;
  });

  function cellOpacity(count: number): number {
    if (count === 0) return 0;
    // Floor at 0.18 so a 1-task week is still visibly different
    // from an empty week. Otherwise low-volume projects look dead.
    return 0.18 + 0.82 * (count / globalMax);
  }
</script>

<div class="h-full flex flex-col">
  <!-- Header — count + scale legend so the cell intensity is
       interpretable without hovering. -->
  <div class="px-3 sm:px-4 py-2 border-b border-surface1 flex items-center gap-3 flex-shrink-0 text-xs">
    <span class="text-text font-medium">Heatmap</span>
    <span class="text-dim">{projects.length} project{projects.length === 1 ? '' : 's'} · last {WEEKS}w</span>
    <span class="flex-1"></span>
    <span class="hidden sm:inline-flex items-center gap-1 text-dim">
      <span>less</span>
      <span class="flex gap-0.5">
        {#each [0.18, 0.4, 0.6, 0.8, 1] as a (a)}
          <span class="w-3 h-3 rounded-sm" style="background: var(--color-success); opacity: {a}"></span>
        {/each}
      </span>
      <span>more</span>
    </span>
  </div>

  {#if projects.length === 0}
    <div class="flex-1 flex items-center justify-center text-dim text-sm italic px-4 text-center">
      No projects to chart. Create a project, link tasks to it, and the
      heatmap will surface week-by-week activity.
    </div>
  {:else if globalMax === 1 && [...cells.values()].every((m) => m.size === 0)}
    <div class="flex-1 flex items-center justify-center text-dim text-sm italic px-4 text-center">
      No completed tasks in the last {WEEKS} weeks. Tag tasks with
      <code class="text-secondary">project:&lt;name&gt;</code> or place them under a project's folder, then check off some work.
    </div>
  {:else}
    <div class="flex-1 overflow-auto">
      <div class="grid heatmap-grid min-w-[640px]">
        <!-- Axis row — week labels -->
        <div class="px-3 py-1.5 border-b border-surface1 bg-mantle/60 sticky left-0 z-10 text-[10px] text-dim uppercase tracking-wider font-medium">
          Project
        </div>
        <div class="border-b border-surface1 bg-mantle/60 grid heatmap-cols">
          {#each weekOrder as w (w.key)}
            <div
              class="text-[9px] {w.isThisWeek ? 'text-primary font-medium' : 'text-dim'} font-mono text-center py-1.5"
              title={w.key}
            >{w.isThisWeek ? 'now' : w.label}</div>
          {/each}
        </div>
        <div class="px-2 py-1.5 border-b border-surface1 bg-mantle/60 text-[10px] text-dim uppercase tracking-wider font-medium text-right">
          12w
        </div>

        <!-- Rows -->
        {#each projects as p (p.name)}
          {@const inner = cells.get(p.name) ?? new Map()}
          {@const total = rowTotals.get(p.name) ?? 0}
          <div class="px-3 py-1 border-b border-surface1/50 sticky left-0 bg-base z-[5] flex items-center gap-1.5 min-w-0">
            <span class="w-2 h-2 rounded-full flex-shrink-0" style="background: {colorVar(p.color)}"></span>
            <button
              onclick={() => onSelect(p.name)}
              class="text-xs text-text hover:text-primary truncate flex-1 text-left"
              title="open {p.name}"
            >{p.name}</button>
          </div>
          <div class="border-b border-surface1/50 grid heatmap-cols">
            {#each weekOrder as w (w.key)}
              {@const count = inner.get(w.key) ?? 0}
              {@const op = cellOpacity(count)}
              <button
                onclick={() => onSelect(p.name)}
                class="h-7 mx-px rounded-sm transition-all hover:ring-1 hover:ring-primary {w.isThisWeek ? 'ring-1 ring-primary/40' : ''}"
                style="background: {count > 0 ? colorVar(p.color) : 'var(--color-surface0)'}; opacity: {count > 0 ? op : 1};"
                title="{p.name} · {w.key}: {count} done"
                aria-label="{count} tasks done in {w.key}"
              ></button>
            {/each}
          </div>
          <div class="px-2 py-1 border-b border-surface1/50 text-[10px] {total === 0 ? 'text-dim' : 'text-subtext'} font-mono text-right tabular-nums">
            {total}
          </div>
        {/each}
      </div>
    </div>
  {/if}
</div>

<style>
  /* Three-column grid: project label | weeks strip | total. The
     middle column is itself a sub-grid sized to fit the WEEKS count.
     Keeping the totals out as a third column means they line up
     vertically across rows independent of week count. */
  .heatmap-grid {
    grid-template-columns: 12rem 1fr 3rem;
  }
  @media (min-width: 768px) {
    .heatmap-grid {
      grid-template-columns: 16rem 1fr 3.5rem;
    }
  }
  .heatmap-cols {
    grid-template-columns: repeat(12, minmax(0, 1fr));
  }
</style>
