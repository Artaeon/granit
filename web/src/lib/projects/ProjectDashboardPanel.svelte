<script lang="ts">
  // ProjectDashboardPanel — full-screen overlay surfacing the project's
  // full operating picture in a card grid. NOT a chat surface; this is
  // the visual companion to the chat-mode "Project Manager" prelude.
  //
  // The data shape is the ProjectContextBundle that powers the chat's
  // PM prelude (see web/src/lib/ai/projectManagerContext.ts). Sharing
  // that loader means the dashboard's "12 open tasks" matches the
  // number the chat quotes when the user asks — one source of truth,
  // two surfaces.
  //
  // Layout:
  //   - 1 col on mobile, 2 cols at md, 3 cols at xl
  //   - Hero card spans the full row at every breakpoint so the
  //     identity is always the first thing you see
  //   - Each subsequent card is a self-contained surface0 box with
  //     surface1 border; tap targets ≥40px on mobile.
  import { onMount } from 'svelte';
  import { api, type Project } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import {
    loadProjectContext,
    type ProjectContextBundle
  } from '$lib/ai/projectManagerContext';
  import { computeMomentum, MOMENTUM_WEEKS } from './dashboardMomentum';

  let { project, onClose }: {
    project: Project;
    onClose: () => void;
  } = $props();

  let bundle = $state<ProjectContextBundle | null>(null);
  let loading = $state(true);
  let loadError = $state('');

  async function load() {
    loading = true;
    loadError = '';
    try {
      // Same shim shape as AIOverlay.svelte's project prelude so the
      // dashboard and the chat agree on what a "linked goal" or
      // "linked note" is — no drift between the surfaces.
      const b = await loadProjectContext(project.name, {
        getProject: (n) => api.getProject(n),
        listTasksForProject: async (n, s) => {
          const r = await api.listTasks({ project: n, status: s });
          return r.tasks;
        },
        listGoalsForProject: async (n) => {
          const r = await api.listGoals();
          return r.goals.filter((g) => g.project === n);
        },
        listNotesInFolder: async (folder) => {
          const r = await api.listNotes({ folder, limit: 50 });
          return r.notes;
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
    // Escape closes the overlay — mirrors the modal contract used
    // elsewhere (AIOverlay, ProjectCreate). Mouse users hit the X;
    // keyboard users hit Esc; no surprise either way.
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
  // Reuse the same priority labels as ProjectDetail so the same
  // value renders the same word everywhere.
  const priorityLabels = ['none', 'low', 'medium', 'high', 'highest'];

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

  function goalStatusTone(s?: string): string {
    if (s === 'active') return 'success';
    if (s === 'completed') return 'info';
    if (s === 'paused') return 'warning';
    if (s === 'archived') return 'subtext';
    return 'subtext';
  }

  // Momentum bars (computed in a tested pure helper).
  const momentum = $derived(bundle ? computeMomentum(bundle.doneTasks, new Date()) : []);
  const momentumMax = $derived(momentum.reduce((m, b) => Math.max(m, b.count), 0));
  const momentumTotal = $derived(momentum.reduce((s, b) => s + b.count, 0));

  // For the open-tasks card: show first 10, link to the full list.
  const OPEN_VISIBLE = 10;
  const openVisible = $derived(bundle ? bundle.openTasks.slice(0, OPEN_VISIBLE) : []);
  const openTotal = $derived(
    bundle ? (bundle.totals?.openTasks ?? bundle.openTasks.length) : 0
  );
  const openMore = $derived(Math.max(0, openTotal - openVisible.length));
</script>

<!-- Full-page overlay. Sits above the projects layout (sidebar + detail
     drawer) via z-50, with a backdrop that absorbs clicks so a stray
     click outside the panel doesn't dismiss it (the X button + Esc
     are the explicit exits). Hides body scroll while open. -->
<div class="fixed inset-0 z-50 bg-mantle flex flex-col" role="dialog" aria-modal="true" aria-label="Project dashboard">
  <!-- Header bar — project identity + close. Sticky so a long scroll
       through cards still has the close affordance reachable. -->
  <header class="flex-shrink-0 border-b border-surface1 bg-base px-3 sm:px-6 py-3 flex items-center gap-3">
    <span class="w-3 h-3 rounded-full flex-shrink-0" style="background: {colorVar(project.color)}"></span>
    <div class="flex-1 min-w-0">
      <div class="flex items-baseline gap-2">
        <h1 class="text-base sm:text-lg font-semibold text-text truncate">{project.name}</h1>
        <span class="text-[10px] uppercase tracking-wider text-dim flex-shrink-0">Dashboard</span>
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
        <!-- Skeleton grid — same grid template as the real layout so
             the page doesn't jump on load. Six pulse cards matches the
             six real cards below. -->
        <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3 sm:gap-4">
          <div class="md:col-span-2 xl:col-span-3 bg-surface0 border border-surface1 rounded-lg p-4 animate-pulse h-40"></div>
          {#each [0, 1, 2, 3, 4] as i (i)}
            <div class="bg-surface0 border border-surface1 rounded-lg p-4 animate-pulse h-48"></div>
          {/each}
        </div>
      {:else if loadError && !bundle}
        <div class="text-sm text-error border border-error/30 bg-error/5 rounded px-4 py-3">
          Could not load dashboard: {loadError}
          <button onclick={() => void load()} class="ml-3 underline text-secondary">retry</button>
        </div>
      {:else if bundle}
        {@const p = bundle.project}
        <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3 sm:gap-4">
          <!-- ── 1. Identity / Hero ──────────────────────────────────
               Spans every column at every breakpoint so the project
               identity is the first thing the user reads. Tags row
               (status / kind / venture / priority / due) mirrors the
               renderProjectContext markdown header so the dashboard
               surfaces exactly what the AI sees. -->
          <section class="md:col-span-2 xl:col-span-3 bg-surface0 border border-surface1 rounded-lg p-4 sm:p-5">
            <div class="flex items-baseline gap-2 flex-wrap mb-2">
              <h2 class="text-lg sm:text-xl font-semibold text-text">{p.name}</h2>
              {#if p.status}
                <span
                  class="px-2 py-0.5 rounded text-[10px] uppercase tracking-wider font-medium"
                  style="background: var(--color-{statusTone(p.status)}); color: var(--color-base);"
                >{p.status}</span>
              {/if}
              {#if p.kind}
                <span class="px-2 py-0.5 rounded bg-primary/15 text-primary uppercase tracking-wider text-[10px] font-medium">{p.kind}</span>
              {/if}
              {#if p.venture}
                <a
                  href={`/projects?venture=${encodeURIComponent(p.venture)}`}
                  class="px-2 py-0.5 rounded bg-secondary/15 text-secondary hover:bg-secondary/25 text-[11px]"
                >🏢 {p.venture}</a>
              {/if}
              {#if typeof p.priority === 'number' && p.priority > 0}
                <span class="px-2 py-0.5 rounded bg-warning/15 text-warning text-[10px] uppercase tracking-wider">P{p.priority} · {priorityLabels[p.priority] ?? ''}</span>
              {/if}
              {#if p.due_date}
                <span class="text-[11px] text-dim font-mono">due {p.due_date}</span>
              {/if}
            </div>

            {#if p.description && p.description.trim()}
              <p class="text-sm text-subtext leading-relaxed mb-3 whitespace-pre-wrap">{p.description}</p>
            {/if}

            {#if p.next_action && p.next_action.trim()}
              <!-- Next action gets the highlight chip styling used in
                   ProjectDetail — same visual cue across surfaces. -->
              <div>
                <div class="text-[10px] uppercase tracking-wider text-dim mb-1">Next action</div>
                <div class="px-3 py-2.5 rounded text-sm border border-warning/30 bg-warning/10 text-warning font-medium">
                  → {p.next_action}
                </div>
              </div>
            {/if}

            <!-- Progress strip below the next action so the user has
                 a quick "how far?" reading without scrolling. -->
            {#if typeof p.progress === 'number'}
              {@const pct = Math.round((p.progress ?? 0) * 100)}
              <div class="mt-3">
                <div class="flex items-baseline gap-2 mb-1 text-[11px]">
                  <span class="text-dim uppercase tracking-wider">Progress</span>
                  <span class="text-subtext font-mono">{pct}%</span>
                  {#if p.tasksTotal != null && p.tasksTotal > 0}
                    <span class="text-dim font-mono">· {p.tasksDone}/{p.tasksTotal} tasks</span>
                  {/if}
                </div>
                <div class="h-1.5 rounded-full bg-mantle overflow-hidden">
                  <div class="h-full" style="width: {pct}%; background: {colorVar(p.color)}"></div>
                </div>
              </div>
            {/if}
          </section>

          <!-- ── 2. Linked goals ────────────────────────────────────── -->
          <section class="bg-surface0 border border-surface1 rounded-lg p-4">
            <div class="flex items-baseline gap-2 mb-3">
              <h3 class="text-xs uppercase tracking-wider text-dim font-medium flex-1">Linked goals</h3>
              <span class="text-[11px] text-dim font-mono">{bundle.goals.length}</span>
            </div>
            {#if bundle.goals.length === 0}
              <p class="text-xs text-dim italic">No goals linked to this project yet.</p>
            {:else}
              <ul class="space-y-1.5">
                {#each bundle.goals as g (g.id)}
                  <li>
                    <a
                      href={`/goals?focus=${encodeURIComponent(g.id)}`}
                      class="block px-3 py-2 min-h-[40px] bg-mantle hover:bg-surface1 rounded text-sm transition-colors"
                    >
                      <div class="flex items-baseline gap-2">
                        <span class="text-text truncate flex-1">{g.title}</span>
                        {#if g.status}
                          <span
                            class="px-1.5 py-0.5 rounded text-[9px] uppercase tracking-wider font-medium flex-shrink-0"
                            style="background: var(--color-{goalStatusTone(g.status)}); color: var(--color-base);"
                          >{g.status}</span>
                        {/if}
                      </div>
                      {#if g.target_date}
                        <div class="text-[11px] text-dim font-mono mt-0.5">by {g.target_date}</div>
                      {/if}
                    </a>
                  </li>
                {/each}
              </ul>
            {/if}
          </section>

          <!-- ── 3. Open tasks ──────────────────────────────────────── -->
          <section class="bg-surface0 border border-surface1 rounded-lg p-4">
            <div class="flex items-baseline gap-2 mb-3">
              <h3 class="text-xs uppercase tracking-wider text-dim font-medium flex-1">Open tasks</h3>
              <span class="text-[11px] text-dim font-mono">{openTotal}</span>
            </div>
            {#if openVisible.length === 0}
              <p class="text-xs text-dim italic">Nothing open — either you're shipped or nothing has landed in this project yet.</p>
            {:else}
              <ul class="space-y-1">
                {#each openVisible as t (t.id)}
                  <li class="px-2 py-1.5 min-h-[40px] flex items-baseline gap-2 rounded hover:bg-mantle">
                    <span class="w-1.5 h-1.5 rounded-full bg-secondary flex-shrink-0 mt-1.5"></span>
                    <span class="text-sm text-text flex-1 leading-snug">{t.text}</span>
                    <span class="flex items-baseline gap-1 flex-shrink-0">
                      {#if t.priority && t.priority > 0}
                        <span class="text-[10px] px-1 py-0.5 rounded bg-warning/15 text-warning font-mono">P{t.priority}</span>
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
                  href={`/tasks?project=${encodeURIComponent(p.name)}`}
                  class="block mt-2 text-xs text-secondary hover:underline"
                >+ {openMore} more in /tasks →</a>
              {/if}
            {/if}
          </section>

          <!-- ── 4. Recently done ───────────────────────────────────── -->
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

          <!-- ── 5. Linked notes ────────────────────────────────────── -->
          <section class="bg-surface0 border border-surface1 rounded-lg p-4">
            <div class="flex items-baseline gap-2 mb-3">
              <h3 class="text-xs uppercase tracking-wider text-dim font-medium flex-1">Linked notes</h3>
              <span class="text-[11px] text-dim font-mono">{bundle.totals?.notes ?? bundle.notes.length}</span>
            </div>
            {#if !p.folder}
              <p class="text-xs text-dim italic">No folder set on this project — set one in the detail panel to surface notes here.</p>
            {:else if bundle.notes.length === 0}
              <p class="text-xs text-dim italic">No notes under <code class="text-secondary">{p.folder}</code> yet.</p>
            {:else}
              <ul class="space-y-1">
                {#each bundle.notes as n (n.path)}
                  <li>
                    <a
                      href={`/notes/${encodeURIComponent(n.path)}`}
                      class="block px-2 py-1.5 min-h-[40px] rounded hover:bg-mantle"
                    >
                      <div class="text-sm text-text truncate">{n.title || n.path}</div>
                      <div class="text-[10px] text-dim font-mono truncate">{n.path}</div>
                    </a>
                  </li>
                {/each}
              </ul>
            {/if}
          </section>

          <!-- ── 6. Momentum (4-week bar chart) ─────────────────────── -->
          <section class="bg-surface0 border border-surface1 rounded-lg p-4">
            <div class="flex items-baseline gap-2 mb-3">
              <h3 class="text-xs uppercase tracking-wider text-dim font-medium flex-1">Momentum · last {MOMENTUM_WEEKS}w</h3>
              <span class="text-[11px] text-dim font-mono">{momentumTotal} done</span>
            </div>
            {#if momentumTotal === 0}
              <p class="text-xs text-dim italic">No completions in the last {MOMENTUM_WEEKS} weeks.</p>
            {:else}
              <!-- Plain divs as bars — no chart library. Bars share a
                   row, height is percent of the row's max bar so the
                   chart adapts to both light and busy weeks. The
                   current-week bar is primary-coloured; past weeks
                   are secondary-tinted so the eye locks onto "where
                   we are now". -->
              <div class="flex items-end gap-1.5 h-24" aria-label="tasks completed per week">
                {#each momentum as b (b.label)}
                  {@const pct = momentumMax === 0 ? 0 : Math.max(3, Math.round((b.count / momentumMax) * 100))}
                  <div class="flex-1 flex flex-col items-center justify-end gap-1" title="{b.label}: {b.count} completed">
                    <div class="text-[10px] text-subtext font-mono leading-none">{b.count}</div>
                    <div
                      class="w-full rounded-t {b.isThisWeek ? 'bg-primary' : 'bg-secondary/40'} transition-all"
                      style="height: {pct}%"
                    ></div>
                    <div class="text-[10px] {b.isThisWeek ? 'text-primary' : 'text-dim'} font-mono leading-none">{b.label}</div>
                  </div>
                {/each}
              </div>
            {/if}
          </section>
        </div>
      {/if}
    </div>
  </div>
</div>
