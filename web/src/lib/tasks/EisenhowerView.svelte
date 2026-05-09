<script lang="ts">
  import type { Task } from '$lib/api';
  import { todayISO } from '$lib/api';
  import TaskCard from '$lib/tasks/TaskCard.svelte';

  // Eisenhower matrix view — the classic 2×2 of urgent × important
  // attributed to Dwight Eisenhower and popularised by Stephen Covey.
  // Maps cleanly onto granit's existing task fields:
  //
  //   - Important = priority 1 or 2 (P1 + P2 — the user explicitly
  //     marked these as mattering; everything else is treated as
  //     not-important for the purposes of THIS view, though it may
  //     be P3 or unprioritised in the rest of the app)
  //   - Urgent    = dueDate is today or overdue OR scheduledStart
  //     is today (the user has committed to doing it today, one
  //     way or another)
  //
  // Quadrants:
  //   Q1 — DO        | important & urgent  — fires of the day
  //   Q2 — PLAN      | important & not urgent — the deep-work pile
  //   Q3 — DELEGATE  | urgent & not important — interruptions / admin
  //   Q4 — DROP      | not urgent & not important — the silent drift
  //
  // Why this view earns space alongside list / kanban / today:
  //   - List sorts by one axis at a time. The matrix is two-axis
  //     by construction, which is the actual prioritisation
  //     question ("what should I work on now?").
  //   - Kanban groups by status. The matrix groups by *priority
  //     posture* — orthogonal to status, and surfaces the Q4 pile
  //     (low-priority + no due date) that kanban hides among its
  //     "to do" column.

  type Props = {
    tasks: Task[];
    onOpenDetail?: (t: Task) => void;
    onContextMenu?: (t: Task, x: number, y: number) => void;
    onChanged?: (t: Task) => void;
  };
  let { tasks, onOpenDetail, onContextMenu, onChanged }: Props = $props();

  function isImportant(t: Task): boolean {
    return t.priority === 1 || t.priority === 2;
  }

  function isUrgent(t: Task): boolean {
    const today = todayISO();
    if (t.dueDate && t.dueDate <= today) return true;
    if (t.scheduledStart && t.scheduledStart.slice(0, 10) === today) return true;
    return false;
  }

  type Quadrant = 'do' | 'plan' | 'delegate' | 'drop';

  function classify(t: Task): Quadrant {
    const imp = isImportant(t);
    const urg = isUrgent(t);
    if (imp && urg) return 'do';
    if (imp && !urg) return 'plan';
    if (!imp && urg) return 'delegate';
    return 'drop';
  }

  // Bucket the live `tasks` array into four quadrants. Each quadrant
  // is sorted by (priority asc, dueDate asc) so the most pressing
  // row in each lands at the top — matters especially for Q1 where
  // the user is racing.
  let quadrants = $derived.by(() => {
    const out: Record<Quadrant, Task[]> = { do: [], plan: [], delegate: [], drop: [] };
    for (const t of tasks) {
      if (t.done) continue;
      out[classify(t)].push(t);
    }
    const cmp = (a: Task, b: Task) => {
      const pa = a.priority || 99;
      const pb = b.priority || 99;
      if (pa !== pb) return pa - pb;
      const da = a.dueDate ?? 'zzz';
      const db = b.dueDate ?? 'zzz';
      return da.localeCompare(db);
    };
    for (const q of Object.keys(out) as Quadrant[]) out[q].sort(cmp);
    return out;
  });

  // Per-quadrant rendering metadata. Title is the verb the user
  // should associate with the bucket; subtitle names the underlying
  // shape so the legend doubles as a teaching surface for users
  // who haven't seen the matrix before.
  const QUADRANT_META: Record<Quadrant, {
    title: string;
    subtitle: string;
    accent: string;     // tailwind border color
    badge: string;      // bg + text for the count chip
  }> = {
    do: {
      title: 'Do',
      subtitle: 'Important · urgent',
      accent: 'border-error/40',
      badge: 'bg-error/15 text-error'
    },
    plan: {
      title: 'Plan',
      subtitle: 'Important · not urgent',
      accent: 'border-success/40',
      badge: 'bg-success/15 text-success'
    },
    delegate: {
      title: 'Delegate',
      subtitle: 'Urgent · not important',
      accent: 'border-warning/40',
      badge: 'bg-warning/15 text-warning'
    },
    drop: {
      title: 'Drop',
      subtitle: 'Not urgent · not important',
      accent: 'border-surface2',
      badge: 'bg-surface1 text-dim'
    }
  };

  // Render order is row-major: top row is "important", bottom row
  // is "not important". Left column is "urgent", right column is
  // "not urgent" — matches the Covey / classic textbook layout so
  // users coming from Getting Things Done find what they expect.
  const LAYOUT: Quadrant[] = ['do', 'plan', 'delegate', 'drop'];
</script>

<!-- Two columns × two rows on desktop; collapses to a single column
     stack on mobile because four side-by-side panels at <640px is
     unreadable. The 2x2 grid IS the value of this view; we don't
     try to fake it on small screens. -->
<div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
  {#each LAYOUT as q (q)}
    {@const meta = QUADRANT_META[q]}
    {@const list = quadrants[q]}
    <section class="bg-surface0 border-2 {meta.accent} rounded-lg flex flex-col min-h-[18rem]">
      <header class="flex items-baseline justify-between px-4 py-3 border-b border-surface1">
        <div>
          <h3 class="text-sm font-semibold text-text">{meta.title}</h3>
          <p class="text-[11px] text-dim mt-0.5">{meta.subtitle}</p>
        </div>
        <span class="text-xs px-2 py-0.5 rounded-full {meta.badge} tabular-nums">
          {list.length}
        </span>
      </header>
      <div class="flex-1 overflow-y-auto p-3 space-y-1.5">
        {#if list.length === 0}
          <p class="text-xs text-dim italic text-center py-6 leading-relaxed">
            {#if q === 'do'}
              Nothing on fire — enjoy it.
            {:else if q === 'plan'}
              No deep-work commitments yet. Add a P1 / P2 with no due date and it'll land here.
            {:else if q === 'delegate'}
              No interruptions queued. The day's clean.
            {:else}
              Empty. Things tend to drift here when a P3 / P4 misses its deadline — review periodically.
            {/if}
          </p>
        {:else}
          {#each list as t (t.id)}
            <TaskCard
              task={t}
              compact
              onOpenDetail={onOpenDetail}
              onContextMenu={onContextMenu}
              onChanged={onChanged}
            />
          {/each}
        {/if}
      </div>
    </section>
  {/each}
</div>
