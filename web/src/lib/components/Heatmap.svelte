<script lang="ts">
  // GitHub-contributions-style year heatmap. 53 weeks × 7 days,
  // each cell color-coded by `value` (0..maxValue). Reusable —
  // habits pass {date, done} pairs, virtues pass {date, score},
  // a future commit log could pass {date, count}.
  //
  // Layout: rows are weekdays (Mon at top, Sun at bottom — ISO
  // ordering, matches what feels natural in Europe; flip via prop
  // for a Sun-first audience). Columns are weeks, oldest left.
  // Date alignment: column 0 starts on the Monday of the week
  // containing 365 days ago, so today's cell always lands in the
  // rightmost column. Days outside the 365-day window are rendered
  // as empty cells so the grid stays a clean rectangle.
  //
  // No tooltip library; native title attributes on each cell.
  // Cheap; survives any browser; works offline. The prose-grade
  // tooltip (with month name etc.) is good enough for at-a-glance
  // review.

  type Cell = {
    date: string; // YYYY-MM-DD
    value: number;
  };

  let {
    cells,
    maxValue,
    title = '',
    weekStart = 'mon',
    days = 365,
    legendLabels = ['none', 'low', 'mid', 'high', 'full']
  }: {
    cells: Cell[];
    maxValue?: number;
    title?: string;
    weekStart?: 'mon' | 'sun';
    days?: number;
    legendLabels?: string[];
  } = $props();

  // Map for O(1) date lookup. Built once per cells change.
  let byDate = $derived.by(() => {
    const m = new Map<string, number>();
    for (const c of cells) m.set(c.date, c.value);
    return m;
  });

  let effectiveMax = $derived(maxValue ?? Math.max(1, ...cells.map((c) => c.value)));

  // Build the grid: weeks × 7-days, each cell carrying the date or
  // null (out of window). Always end on today.
  type GridCell = { date: string | null; value: number };
  let grid = $derived.by((): GridCell[][] => {
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    const start = new Date(today);
    start.setDate(start.getDate() - days + 1);
    // Snap start to the previous weekStart-day so the first column
    // is a full week.
    const targetWeekday = weekStart === 'mon' ? 1 : 0; // 0=Sun, 1=Mon
    while (start.getDay() !== targetWeekday) {
      start.setDate(start.getDate() - 1);
    }
    const cols: GridCell[][] = [];
    let cursor = new Date(start);
    while (cursor <= today) {
      const week: GridCell[] = [];
      for (let i = 0; i < 7; i++) {
        if (cursor > today) {
          week.push({ date: null, value: 0 });
        } else {
          const iso = `${cursor.getFullYear()}-${String(cursor.getMonth() + 1).padStart(2, '0')}-${String(cursor.getDate()).padStart(2, '0')}`;
          // Only inside the requested window; before-window cells
          // stay empty so the snap-to-weekStart doesn't pollute the
          // visual.
          const inWindow = cursor >= new Date(today.getTime() - (days - 1) * 86400000);
          week.push({
            date: inWindow ? iso : null,
            value: inWindow ? (byDate.get(iso) ?? 0) : 0
          });
        }
        cursor.setDate(cursor.getDate() + 1);
      }
      cols.push(week);
    }
    return cols;
  });

  // Bucket a value into 0..4 for the 5-tone color scale. Empty
  // cells (no data) render as 0 (lightest); cells with the max
  // value render as 4 (darkest). Linear bucketing — fine for
  // habits (binary, max=1) and good enough for virtues (1-5).
  function level(v: number): number {
    if (v <= 0 || effectiveMax <= 0) return 0;
    const ratio = v / effectiveMax;
    if (ratio >= 0.999) return 4;
    if (ratio >= 0.66) return 3;
    if (ratio >= 0.33) return 2;
    return 1;
  }

  // Month-label row above the grid. We label a column when the
  // first-day-of-the-week shifts month — same logic GitHub uses.
  let monthLabels = $derived.by((): { col: number; label: string }[] => {
    const out: { col: number; label: string }[] = [];
    let prev = -1;
    for (let i = 0; i < grid.length; i++) {
      const firstDate = grid[i][0].date;
      if (!firstDate) continue;
      const mo = new Date(firstDate).getMonth();
      if (mo !== prev) {
        const label = new Date(firstDate).toLocaleDateString(undefined, { month: 'short' });
        out.push({ col: i, label });
        prev = mo;
      }
    }
    return out;
  });

  // Weekday labels — Mon, Wed, Fri (only every other row to keep
  // the gutter narrow). Drives `dayLabel(rowIndex)`.
  function dayLabel(i: number): string {
    if (weekStart === 'mon') {
      return ['Mon', '', 'Wed', '', 'Fri', '', ''][i] ?? '';
    }
    return ['Sun', '', 'Tue', '', 'Thu', '', ''][i] ?? '';
  }

  function formatTooltip(c: GridCell): string {
    if (!c.date) return '';
    const d = new Date(c.date);
    const dateStr = d.toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric', year: 'numeric' });
    return `${dateStr} — ${c.value}`;
  }
</script>

<div class="heatmap">
  {#if title}
    <h3 class="text-xs uppercase tracking-wider text-dim font-semibold mb-2">{title}</h3>
  {/if}
  <div class="hm-scroll overflow-x-auto">
    <div class="hm-grid inline-block min-w-full">
      <!-- Month labels above the grid. Columns line up with the
           first-week column so the labels read naturally. -->
      <div class="hm-months grid" style="grid-template-columns: 2rem repeat({grid.length}, 0.6rem);">
        <div></div>
        {#each grid as _, i}
          {@const m = monthLabels.find((ml) => ml.col === i)}
          <div class="text-[9px] text-dim font-mono leading-tight pb-0.5">
            {m?.label ?? ''}
          </div>
        {/each}
      </div>
      <!-- Body: 7 rows, leftmost weekday-label column + per-week
           cell columns. Inline grid-template-columns so the cell
           size is in one place; CSS variables would be cleaner but
           this is two lines. -->
      <div class="hm-body grid gap-y-px gap-x-px" style="grid-template-columns: 2rem repeat({grid.length}, 0.6rem); grid-template-rows: repeat(7, 0.6rem);">
        {#each Array.from({ length: 7 }, (_, i) => i) as row}
          <div class="text-[9px] text-dim font-mono leading-none pr-1 self-center">{dayLabel(row)}</div>
          {#each grid as week (week)}
            {@const cell = week[row]}
            {@const lvl = level(cell.value)}
            <div
              class="hm-cell rounded-[1px]"
              data-level={cell.date ? lvl : 'none'}
              title={formatTooltip(cell)}
            ></div>
          {/each}
        {/each}
      </div>
    </div>
  </div>
  <!-- Legend — 5-tone scale + textual hint. Aligned right so the
       grid above stays the focal point. -->
  <div class="hm-legend flex items-center justify-end gap-1 mt-1.5 text-[9px] text-dim font-mono">
    <span>{legendLabels[0] ?? 'less'}</span>
    {#each [0, 1, 2, 3, 4] as l}
      <div class="hm-cell rounded-[1px] w-2 h-2" data-level={l}></div>
    {/each}
    <span>{legendLabels[4] ?? 'more'}</span>
  </div>
</div>

<style>
  /* The five-tone scale uses the success palette so the heatmap
     reads as "good things accumulating" without needing per-feature
     color choices. data-level=none renders blank for out-of-window
     cells so the snap-to-week-start doesn't show ghost colors. */
  .hm-cell {
    width: 0.6rem;
    height: 0.6rem;
  }
  .hm-cell[data-level='none'] {
    background: transparent;
  }
  .hm-cell[data-level='0'] {
    background: var(--color-surface1);
  }
  .hm-cell[data-level='1'] {
    background: color-mix(in srgb, var(--color-success) 28%, var(--color-surface1));
  }
  .hm-cell[data-level='2'] {
    background: color-mix(in srgb, var(--color-success) 50%, var(--color-surface1));
  }
  .hm-cell[data-level='3'] {
    background: color-mix(in srgb, var(--color-success) 72%, var(--color-surface1));
  }
  .hm-cell[data-level='4'] {
    background: var(--color-success);
  }
</style>
