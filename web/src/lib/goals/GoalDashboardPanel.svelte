<script lang="ts">
  // GoalDashboardPanel — full-screen overlay surfacing the goal's
  // full operating picture in a card grid. Visual companion to the
  // chat-mode "Goal Manager" prelude; the data shape is the
  // GoalContextBundle that powers that prelude
  // (web/src/lib/ai/goalManagerContext.ts), so the dashboard's
  // "12 open tasks" matches the number the chat quotes when the
  // user asks. One source of truth, two surfaces.
  //
  // Layout:
  //   - 1 col on mobile, 2 cols at md, 3 cols at xl
  //   - Hero card spans the full row so the goal's identity is
  //     always the first thing on the page
  //   - Each subsequent card is a self-contained surface0 box with
  //     surface1 border; tap targets ≥40px on mobile.
  import { onMount } from 'svelte';
  import { api, type Goal, type Milestone } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import {
    loadGoalContext,
    type GoalContextBundle
  } from '$lib/ai/goalManagerContext';

  let { goal, onClose }: {
    goal: Goal;
    onClose: () => void;
  } = $props();

  let bundle = $state<GoalContextBundle | null>(null);
  let loading = $state(true);
  let loadError = $state('');

  async function load() {
    loading = true;
    loadError = '';
    try {
      // Same shim shape as AIOverlay.svelte's goal prelude so the
      // dashboard and the chat agree on "what tasks live under this
      // goal". listGoals + find by id matches the AIOverlay loader.
      const b = await loadGoalContext(goal.id, {
        getGoal: async (id) => {
          const r = await api.listGoals();
          const g = r.goals.find((x) => x.id === id);
          if (!g) throw new Error('goal not found');
          return g;
        },
        listTasksForGoal: async (id, s) => {
          const r = await api.listTasks({ goal: id, status: s });
          return r.tasks;
        }
      });
      bundle = b;
    } catch (e) {
      loadError = e instanceof Error ? e.message : String(e);
      toast.error('dashboard failed to load: ' + loadError);
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    void load();
    // Esc closes — mirrors AIOverlay and ProjectDashboardPanel.
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        onClose();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  // ── Derived chips / labels ──────────────────────────────────────
  function statusTone(s?: string): string {
    if (s === 'active') return 'success';
    if (s === 'paused') return 'warning';
    if (s === 'completed') return 'info';
    if (s === 'archived') return 'subtext';
    return 'subtext';
  }

  function colorVar(c?: string): string {
    const map: Record<string, string> = {
      red: 'error', yellow: 'warning', orange: 'accent', green: 'success',
      blue: 'secondary', purple: 'primary', cyan: 'info', mauve: 'primary',
      peach: 'accent', teal: 'info', sapphire: 'secondary', pink: 'accent',
      lavender: 'primary', flamingo: 'error'
    };
    return `var(--color-${map[c ?? ''] ?? 'secondary'})`;
  }

  // ── Review cadence ──────────────────────────────────────────────
  // Compute "review due" by adding the cadence interval to the
  // last review timestamp (or to created_at if never reviewed).
  // A goal with cadence set but no log line lights up red — that's
  // the case where the user committed to a review rhythm and then
  // never started.
  function freqDays(f?: string): number {
    if (f === 'weekly') return 7;
    if (f === 'monthly') return 30;
    if (f === 'quarterly') return 90;
    return 0;
  }

  function parseDate(s?: string): Date | null {
    if (!s) return null;
    const d = new Date(s);
    if (Number.isNaN(d.getTime())) return null;
    return d;
  }

  function fmtDays(n: number): string {
    if (n === 0) return 'today';
    if (n === 1) return 'in 1 day';
    if (n > 0) return `in ${n} days`;
    if (n === -1) return '1 day overdue';
    return `${-n} days overdue`;
  }

  const reviewDue = $derived.by(() => {
    if (!goal.review_frequency) return null;
    const days = freqDays(goal.review_frequency);
    if (days <= 0) return null;
    const anchor = parseDate(goal.last_reviewed) ?? parseDate(goal.created_at);
    if (!anchor) return null;
    const due = new Date(anchor);
    due.setDate(due.getDate() + days);
    const now = new Date();
    const diff = Math.round((due.getTime() - now.getTime()) / (1000 * 60 * 60 * 24));
    return {
      dueDate: due.toISOString().slice(0, 10),
      diffDays: diff,
      overdue: diff < 0,
      label: fmtDays(diff)
    };
  });

  // ── Milestones ──────────────────────────────────────────────────
  const milestones = $derived<Milestone[]>(
    Array.isArray(goal.milestones) ? goal.milestones : []
  );
  const msDone = $derived(milestones.filter((m) => m.done).length);
  const msTotal = $derived(milestones.length);

  // ── Open / done task slices ─────────────────────────────────────
  // The bundle's caps are already token-conscious (GOAL_TASK_CAP=20,
  // GOAL_DONE_TASK_CAP=8). We cap display at 10 open / 8 done to
  // keep the cards scannable; the "+N more" link punts the rest to
  // /tasks where the user can filter properly.
  const OPEN_VISIBLE = 10;
  const openVisible = $derived(bundle ? bundle.openTasks.slice(0, OPEN_VISIBLE) : []);
  const openTotal = $derived(
    bundle ? (bundle.totals?.openTasks ?? bundle.openTasks.length) : 0
  );
  const openMore = $derived(Math.max(0, openTotal - openVisible.length));

  // ── Review history ──────────────────────────────────────────────
  // Sorted newest-first; the bundle doesn't sort review_log so we
  // do it here. Cap at 5 — the dashboard is a snapshot, not an
  // archive.
  const recentReviews = $derived.by(() => {
    const log = goal.review_log ?? [];
    return [...log]
      .sort((a, b) => (b.date ?? '').localeCompare(a.date ?? ''))
      .slice(0, 5);
  });

  // ── Snapshot card numbers ───────────────────────────────────────
  // Computed here rather than via {@const} in the template so the
  // section markup doesn't need a snippet wrapper. Progress is the
  // share of milestones done; for a goal with no milestones we fall
  // back to "completed" status = 100%.
  const totalTasks = $derived(bundle ? bundle.openTasks.length + bundle.doneTasks.length : 0);
  const msPct = $derived(
    msTotal === 0
      ? goal.status === 'completed'
        ? 100
        : 0
      : Math.round((msDone / msTotal) * 100)
  );
</script>

<!-- Full-page overlay above the goals page. Backdrop absorbs clicks
     so a stray click outside the cards doesn't dismiss; explicit
     exits are the X button + Esc. Hides body scroll while open. -->
<div class="fixed inset-0 z-50 bg-mantle flex flex-col" role="dialog" aria-modal="true" aria-label="Goal dashboard">
  <header class="flex-shrink-0 border-b border-surface1 bg-base px-3 sm:px-6 py-3 flex items-center gap-3">
    <span class="w-3 h-3 rounded-full flex-shrink-0" style="background: {colorVar(goal.color)}"></span>
    <div class="flex-1 min-w-0">
      <div class="flex items-baseline gap-2">
        <h1 class="text-base sm:text-lg font-semibold text-text truncate">{goal.title}</h1>
        <span class="text-[10px] uppercase tracking-wider text-dim flex-shrink-0">Goal · Dashboard</span>
      </div>
    </div>
    <button
      onclick={onClose}
      aria-label="close dashboard"
      class="min-w-[40px] min-h-[40px] flex items-center justify-center rounded text-subtext hover:text-error hover:bg-surface0"
      title="close (Esc)"
    >
      <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
        <path d="M6 6l12 12M6 18L18 6" />
      </svg>
    </button>
  </header>

  <div class="flex-1 overflow-y-auto">
    <div class="max-w-7xl mx-auto p-3 sm:p-6">
      {#if loading && !bundle}
        <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3 sm:gap-4">
          <div class="md:col-span-2 xl:col-span-3 bg-surface0 border border-surface1 rounded-lg p-4 animate-pulse h-40"></div>
          {#each [0, 1, 2, 3, 4] as i (i)}
            <div class="bg-surface0 border border-surface1 rounded-lg p-4 animate-pulse h-48"></div>
          {/each}
        </div>
      {:else if loadError && !bundle}
        <div class="text-sm text-error border border-error bg-surface0 rounded px-4 py-3">
          Could not load dashboard: {loadError}
          <button onclick={() => void load()} class="ml-3 underline text-secondary">retry</button>
        </div>
      {:else if bundle}
        {@const g = bundle.goal}
        <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3 sm:gap-4">
          <!-- ── 1. Identity / Hero ─────────────────────────────────
               Spans every column at every breakpoint so the goal's
               identity is the first thing the user reads. Tags row
               mirrors the renderGoalContext markdown header so the
               dashboard surfaces exactly what the AI sees. Review-
               due indicator surfaces when a cadence is set + an
               anchor (last_reviewed / created_at) is available. -->
          <section class="md:col-span-2 xl:col-span-3 bg-surface0 border border-surface1 rounded-lg p-4 sm:p-5">
            <div class="flex items-baseline gap-2 flex-wrap mb-2">
              <h2 class="text-lg sm:text-xl font-semibold text-text">{g.title}</h2>
              {#if g.status}
                <span
                  class="px-2 py-0.5 rounded text-[10px] uppercase tracking-wider font-medium"
                  style="background: var(--color-{statusTone(g.status)}); color: var(--color-base);"
                >{g.status}</span>
              {/if}
              {#if g.category}
                <span class="px-2 py-0.5 rounded bg-surface1 text-primary uppercase tracking-wider text-[10px] font-medium">{g.category}</span>
              {/if}
              {#if g.venture}
                <a
                  href={`/goals?venture=${encodeURIComponent(g.venture)}`}
                  class="px-2 py-0.5 rounded bg-surface1 text-secondary hover:bg-surface2 text-[11px]"
                >🏢 {g.venture}</a>
              {/if}
              {#if g.target_date}
                <span class="text-[11px] text-dim font-mono">target {g.target_date}</span>
              {/if}
              {#if g.review_frequency}
                <span class="text-[11px] text-dim font-mono">review · {g.review_frequency}</span>
              {/if}
            </div>

            {#if g.description && g.description.trim()}
              <p class="text-sm text-subtext leading-relaxed mb-3 whitespace-pre-wrap">{g.description}</p>
            {/if}

            {#if reviewDue}
              <!-- Review-due indicator. Tone is warning for "due
                   today/this week", error when overdue, dim
                   otherwise — same visual grammar as the deadlines
                   surface. -->
              {@const tone = reviewDue.overdue
                ? 'error'
                : reviewDue.diffDays <= 7
                  ? 'warning'
                  : 'subtext'}
              <div class="px-3 py-2.5 rounded text-sm border" style="border-color: color-mix(in srgb, var(--color-{tone}) 30%, transparent); background: color-mix(in srgb, var(--color-{tone}) 10%, transparent); color: var(--color-{tone});">
                <span class="font-medium">Next review</span>
                <span class="font-mono ml-1">{reviewDue.dueDate}</span>
                <span class="ml-2">· {reviewDue.label}</span>
                {#if !g.last_reviewed}
                  <span class="ml-2 text-[11px] opacity-80">(no reviews logged yet)</span>
                {/if}
              </div>
            {/if}

            {#if g.project && g.project.trim()}
              <!-- Linked project chip — clickable to the project
                   page. Bidirectional surface: ProjectDashboardPanel
                   already lists "linked goals", this is the goal-side
                   mirror so the user can hop project → goal → project
                   without breadcrumbs. -->
              <div class="mt-3 flex items-center gap-2 text-[11px]">
                <span class="text-[10px] uppercase tracking-wider text-dim">Linked project</span>
                <a
                  href={`/projects?p=${encodeURIComponent(g.project)}`}
                  class="px-2 py-0.5 rounded bg-surface1 text-primary hover:bg-primary hover:text-on-primary inline-flex items-center gap-1"
                >📁 {g.project}</a>
              </div>
            {/if}
          </section>

          <!-- ── 2. Milestones ──────────────────────────────────────
               Milestone list with done/pending icons, target dates,
               and a "X of Y done" badge. When no milestones exist
               we surface a one-line hint pointing the user back to
               GoalDetail to add some. -->
          <section class="bg-surface0 border border-surface1 rounded-lg p-4">
            <div class="flex items-baseline gap-2 mb-3">
              <h3 class="text-xs uppercase tracking-wider text-dim font-medium flex-1">Milestones</h3>
              {#if msTotal > 0}
                <span class="text-[11px] text-dim font-mono">{msDone} of {msTotal} done</span>
              {/if}
            </div>
            {#if msTotal === 0}
              <p class="text-xs text-dim italic">No milestones yet — break the goal into 2-5 checkpoints in the detail drawer.</p>
            {:else}
              <ul class="space-y-1">
                {#each milestones as m, idx (idx)}
                  <li class="px-2 py-1.5 min-h-[40px] flex items-baseline gap-2 rounded">
                    <span class="flex-shrink-0 mt-0.5 {m.done ? 'text-success' : 'text-dim'}">
                      {m.done ? '✓' : '○'}
                    </span>
                    <span class="text-sm flex-1 leading-snug {m.done ? 'text-subtext line-through decoration-dim' : 'text-text'}">{m.text}</span>
                    {#if m.due_date}
                      <span class="text-[10px] text-dim font-mono flex-shrink-0">{m.due_date}</span>
                    {/if}
                  </li>
                {/each}
              </ul>
            {/if}
          </section>

          <!-- ── 3. Open tasks ──────────────────────────────────────
               Cap visible at 10; punt to /tasks?goal=<id> for the
               rest. Empty state is honest — "nothing tagged with
               this goal yet" — and points the user at the action
               that would change it. -->
          <section class="bg-surface0 border border-surface1 rounded-lg p-4">
            <div class="flex items-baseline gap-2 mb-3">
              <h3 class="text-xs uppercase tracking-wider text-dim font-medium flex-1">Open tasks</h3>
              <span class="text-[11px] text-dim font-mono">{openTotal}</span>
            </div>
            {#if openVisible.length === 0}
              <p class="text-xs text-dim italic">Nothing open tagged with this goal. Add <code class="text-secondary">goal:{g.id}</code> to any task line.</p>
            {:else}
              <ul class="space-y-1">
                {#each openVisible as t (t.id)}
                  <li class="px-2 py-1.5 min-h-[40px] flex items-baseline gap-2 rounded hover:bg-mantle">
                    <span class="w-1.5 h-1.5 rounded-full bg-secondary flex-shrink-0 mt-1.5"></span>
                    <span class="text-sm text-text flex-1 leading-snug">{t.text}</span>
                    <span class="flex items-baseline gap-1 flex-shrink-0">
                      {#if t.priority && t.priority > 0}
                        <span class="text-[10px] px-1 py-0.5 rounded bg-surface0 text-warning font-mono">P{t.priority}</span>
                      {/if}
                      {#if t.dueDate}
                        <span class="text-[10px] text-dim font-mono">{t.dueDate}</span>
                      {/if}
                    </span>
                  </li>
                {/each}
              </ul>
              {#if openMore > 0}
                <a
                  href={`/tasks?goal=${encodeURIComponent(g.id)}`}
                  class="block mt-2 text-xs text-secondary hover:underline"
                >+ {openMore} more in /tasks →</a>
              {/if}
            {/if}
          </section>

          <!-- ── 4. Recently done ───────────────────────────────────
               The bundle's loader already sorts done tasks newest-
               first (completedAt desc, fallback updatedAt) and caps
               at GOAL_DONE_TASK_CAP=8 — we trust that ordering and
               just render. -->
          <section class="bg-surface0 border border-surface1 rounded-lg p-4">
            <div class="flex items-baseline gap-2 mb-3">
              <h3 class="text-xs uppercase tracking-wider text-dim font-medium flex-1">Recently done</h3>
              <span class="text-[11px] text-dim font-mono">{bundle.totals?.doneTasks ?? bundle.doneTasks.length}</span>
            </div>
            {#if bundle.doneTasks.length === 0}
              <p class="text-xs text-dim italic">No completions on record yet.</p>
            {:else}
              <ul class="space-y-1">
                {#each bundle.doneTasks as t (t.id)}
                  <li class="px-2 py-1.5 min-h-[40px] flex items-baseline gap-2 rounded">
                    <span class="text-success flex-shrink-0 mt-0.5">✓</span>
                    <span class="text-sm text-subtext line-through decoration-dim flex-1 leading-snug">{t.text}</span>
                    {#if t.completedAt}
                      <span class="text-[10px] text-dim font-mono flex-shrink-0">{t.completedAt.slice(0, 10)}</span>
                    {/if}
                  </li>
                {/each}
              </ul>
            {/if}
          </section>

          <!-- ── 5. Review history ──────────────────────────────────
               Last 3-5 review_log entries. Each shows date + the
               progress integer the TUI captured + the note text.
               No editing here — that lives in GoalDetail. -->
          <section class="bg-surface0 border border-surface1 rounded-lg p-4">
            <div class="flex items-baseline gap-2 mb-3">
              <h3 class="text-xs uppercase tracking-wider text-dim font-medium flex-1">Review history</h3>
              <span class="text-[11px] text-dim font-mono">{(g.review_log ?? []).length}</span>
            </div>
            {#if recentReviews.length === 0}
              <p class="text-xs text-dim italic">No reviews logged yet. Log one from the detail drawer to start a cadence.</p>
            {:else}
              <ul class="space-y-2">
                {#each recentReviews as r, idx (idx + (r.date ?? ''))}
                  <li class="px-2 py-1.5 rounded border-l-2 border-surface2 bg-mantle">
                    <div class="flex items-baseline gap-2">
                      <span class="text-[11px] text-dim font-mono">{r.date}</span>
                      {#if typeof r.progress === 'number' && r.progress > 0}
                        <span class="text-[10px] px-1 py-0.5 rounded bg-surface0 text-success font-mono">{r.progress}%</span>
                      {/if}
                    </div>
                    {#if r.note && r.note.trim()}
                      <p class="text-xs text-subtext mt-1 leading-snug whitespace-pre-wrap">{r.note}</p>
                    {/if}
                  </li>
                {/each}
              </ul>
            {/if}
          </section>

          <!-- ── 6. Quick stats ─────────────────────────────────────
               Compact roll-up card. Three numbers + a progress bar
               so the user can read momentum at a glance without
               flipping back to GoalDetail. Progress is the share of
               milestones done; for a goal with no milestones we
               fall back to "completed" status = 100%. -->
          <section class="bg-surface0 border border-surface1 rounded-lg p-4">
            <div class="flex items-baseline gap-2 mb-3">
              <h3 class="text-xs uppercase tracking-wider text-dim font-medium flex-1">Snapshot</h3>
            </div>
            <div class="grid grid-cols-3 gap-2 mb-3">
              <div class="text-center">
                <div class="text-lg font-semibold text-text font-mono">{openTotal}</div>
                <div class="text-[10px] uppercase tracking-wider text-dim">open</div>
              </div>
              <div class="text-center">
                <div class="text-lg font-semibold text-text font-mono">{bundle.totals?.doneTasks ?? bundle.doneTasks.length}</div>
                <div class="text-[10px] uppercase tracking-wider text-dim">done</div>
              </div>
              <div class="text-center">
                <div class="text-lg font-semibold text-text font-mono">{totalTasks}</div>
                <div class="text-[10px] uppercase tracking-wider text-dim">total</div>
              </div>
            </div>
            <div class="flex items-baseline gap-2 mb-1 text-[11px]">
              <span class="text-dim uppercase tracking-wider">Milestones</span>
              <span class="text-subtext font-mono">{msPct}%</span>
            </div>
            <div class="h-1.5 rounded-full bg-mantle overflow-hidden">
              <div class="h-full" style="width: {msPct}%; background: {colorVar(g.color)}"></div>
            </div>
          </section>
        </div>
      {/if}
    </div>
  </div>
</div>
