<script lang="ts">
  // Calendar view for /deadlines. Stream BB. Month grid with each
  // day cell showing up to 3 compact deadline rows + a "+N more"
  // overflow. Empty in-month cells show a faint "+" on hover so the
  // user can create-on-that-date.
  import type { Deadline } from '$lib/api';

  type Props = {
    filtered: Deadline[];
    todayISO: () => string;
    onOpen: (d: Deadline) => void;
    onCreateOn: (iso: string) => void;
  };

  let { filtered, todayISO, onOpen, onCreateOn }: Props = $props();

  // Cursor is the first-of-month being shown. ←/→ buttons step it.
  let calCursor = $state(new Date(new Date().getFullYear(), new Date().getMonth(), 1));

  function calStep(n: number) {
    const dt = new Date(calCursor);
    dt.setMonth(dt.getMonth() + n);
    calCursor = dt;
  }
  function calLabel(): string {
    return calCursor.toLocaleDateString(undefined, { month: 'long', year: 'numeric' });
  }

  let calCells = $derived.by(() => {
    // 6 weeks x 7 days, aligned to Monday-start. Each cell holds the
    // ISO date string + the deadlines (post-filter) due that day.
    const first = new Date(calCursor);
    // Monday-of-grid = the Monday on or before the 1st. JS Sunday=0 → shift.
    const dow = (first.getDay() + 6) % 7;
    const start = new Date(first);
    start.setDate(first.getDate() - dow);
    const cells: { iso: string; date: Date; rows: Deadline[]; inMonth: boolean }[] = [];
    const byDate = new Map<string, Deadline[]>();
    for (const d of filtered) {
      if (!byDate.has(d.date)) byDate.set(d.date, []);
      byDate.get(d.date)!.push(d);
    }
    for (let i = 0; i < 42; i++) {
      const dt = new Date(start);
      dt.setDate(start.getDate() + i);
      const iso = `${dt.getFullYear()}-${String(dt.getMonth() + 1).padStart(2, '0')}-${String(dt.getDate()).padStart(2, '0')}`;
      cells.push({
        iso,
        date: dt,
        rows: byDate.get(iso) ?? [],
        inMonth: dt.getMonth() === calCursor.getMonth()
      });
    }
    return cells;
  });

  const weekdayLabels = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];

  function isToday(iso: string): boolean {
    return iso === todayISO();
  }

  // Calendar cell row tone — driven by importance (with status
  // overrides). Distinct from the urgency tone the timeline uses
  // because in the grid layout the column already encodes the date.
  function calRowTone(d: Deadline): string {
    if (d.status === 'met') return 'success';
    if (d.status === 'cancelled') return 'dim';
    if (d.importance === 'critical') return 'error';
    if (d.importance === 'high') return 'warning';
    return 'info';
  }
</script>

<div class="flex items-center gap-2 mb-3">
  <button
    type="button"
    onclick={() => calStep(-1)}
    class="px-2 py-1 text-sm text-subtext hover:bg-surface1 rounded"
    aria-label="Previous month"
  >‹</button>
  <span class="text-sm font-medium text-text tabular-nums">{calLabel()}</span>
  <button
    type="button"
    onclick={() => calStep(1)}
    class="px-2 py-1 text-sm text-subtext hover:bg-surface1 rounded"
    aria-label="Next month"
  >›</button>
  <button
    type="button"
    onclick={() => (calCursor = new Date(new Date().getFullYear(), new Date().getMonth(), 1))}
    class="px-2 py-1 text-xs text-dim hover:text-text rounded"
  >Today</button>
</div>

<div class="grid grid-cols-7 gap-px bg-surface1 border border-surface1 rounded overflow-hidden">
  {#each weekdayLabels as w}
    <div class="bg-mantle text-[10px] uppercase tracking-wider text-dim font-medium py-1.5 text-center">{w}</div>
  {/each}
  {#each calCells as c (c.iso)}
    {@const today = isToday(c.iso)}
    <div
      class="bg-base min-h-[5rem] p-1.5 flex flex-col gap-1 {c.inMonth ? '' : 'opacity-40'}"
      style={today ? 'box-shadow: inset 0 0 0 2px var(--color-primary);' : ''}
    >
      <div class="text-[11px] text-dim tabular-nums flex items-center gap-1">
        <span class={today ? 'text-primary font-semibold' : ''}>{c.date.getDate()}</span>
        {#if c.rows.length === 0 && c.inMonth}
          <button
            type="button"
            onclick={() => onCreateOn(c.iso)}
            class="ml-auto opacity-0 hover:opacity-100 focus:opacity-100 text-dim hover:text-primary"
            aria-label="Create on {c.iso}"
            title="Create on {c.iso}"
          >+</button>
        {/if}
      </div>
      {#each c.rows.slice(0, 3) as d (d.id)}
        {@const tone = calRowTone(d)}
        {@const isMet = d.status === 'met'}
        <button
          type="button"
          onclick={() => onOpen(d)}
          class="text-left text-[11px] truncate px-1 py-0.5 rounded hover:opacity-80 {isMet ? 'line-through opacity-60' : ''}"
          style="background: var(--color-{tone}); color: #ffffff;"
          title={d.title}
        >{d.title}</button>
      {/each}
      {#if c.rows.length > 3}
        <span class="text-[10px] text-dim">+ {c.rows.length - 3} more</span>
      {/if}
    </div>
  {/each}
</div>
