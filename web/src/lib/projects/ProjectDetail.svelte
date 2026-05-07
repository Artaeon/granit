<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Goal, type Project, type ProjectGoal, type Task } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import GoalEditor from './GoalEditor.svelte';
  import TaskRow from '$lib/components/TaskRow.svelte';
  import EntityDeadlines from '$lib/deadlines/EntityDeadlines.svelte';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';

  let { project, onClose, onUpdated, onDeleted }: {
    project: Project;
    onClose: () => void;
    onUpdated: () => void | Promise<void>;
    onDeleted: (name: string) => void | Promise<void>;
  } = $props();

  // Local edit buffer — committed via patch on blur or save.
  let editingDescription = $state(false);
  let descBuf = $state('');
  let editingNextAction = $state(false);
  let nextActionBuf = $state('');
  let editingName = $state(false);
  let nameBuf = $state('');

  let projectTasks = $state<Task[]>([]);
  let loadingTasks = $state(false);
  let showCompletedTasks = $state(false);

  // Top-level goals (.granit/goals.json) linked to this project via the
  // goal's `project` field. Read-only here — the goals page is where
  // those get edited. We render a compact list as a quick context cue
  // so the project detail surface answers "what are we working towards?".
  let linkedGoals = $state<Goal[]>([]);

  async function loadTasks() {
    loadingTasks = true;
    try {
      // Pull ALL tasks; project membership = matching project field OR
      // notePath under project's folder. Server already does this matching
      // for the projectView decoration so we mirror the same logic here.
      const r = await api.listTasks({});
      const folder = (project.folder ?? '').replace(/\/$/, '');
      projectTasks = r.tasks.filter((t) => {
        if (t.projectId === project.name) return true;
        if (folder && t.notePath.startsWith(folder + '/')) return true;
        return false;
      });
    } catch (e) {
      console.error(e);
    } finally {
      loadingTasks = false;
    }
  }

  async function loadLinkedGoals() {
    try {
      const r = await api.listGoals();
      linkedGoals = r.goals.filter((g) => g.project === project.name);
    } catch (e) {
      // Non-fatal — goals endpoint failure shouldn't break the project
      // page; just leave the section empty.
      console.error('listGoals', e);
    }
  }

  $effect(() => {
    void project.name;
    loadTasks();
    loadLinkedGoals();
  });

  async function patch(p: Partial<Project>): Promise<boolean> {
    try {
      await api.patchProject(project.name, p);
      await onUpdated();
      return true;
    } catch (e) {
      toast.error('save failed: ' + (e instanceof Error ? e.message : String(e)));
      return false;
    }
  }

  async function commitDescription() {
    editingDescription = false;
    if (descBuf !== (project.description ?? '')) await patch({ description: descBuf });
  }
  async function commitNextAction() {
    editingNextAction = false;
    if (nextActionBuf !== (project.next_action ?? '')) await patch({ next_action: nextActionBuf });
  }
  async function commitName() {
    editingName = false;
    if (nameBuf && nameBuf !== project.name) await patch({ name: nameBuf });
  }

  async function setStatus(status: string) {
    await patch({ status });
  }

  async function setColor(color: string) {
    await patch({ color });
  }

  async function setPriority(priority: number) {
    await patch({ priority });
  }

  async function setDueDate(due_date: string) {
    await patch({ due_date });
  }

  async function setTags(raw: string) {
    const tags = raw.split(',').map((t) => t.trim()).filter(Boolean);
    await patch({ tags });
  }

  async function setFolder(folder: string) {
    await patch({ folder });
  }

  async function updateGoals(goals: ProjectGoal[]) {
    await patch({ goals });
  }

  async function deleteProject() {
    if (!confirm(`Delete project "${project.name}"? Tasks won't be removed.`)) return;
    try {
      await api.deleteProject(project.name);
      await onDeleted(project.name);
    } catch (e) {
      toast.error('delete failed: ' + (e instanceof Error ? e.message : String(e)));
    }
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

  const colorOptions = ['blue', 'green', 'mauve', 'peach', 'red', 'yellow', 'pink', 'lavender', 'teal', 'sapphire', 'flamingo'];
  const categoryOptions = ['development', 'social-media', 'personal', 'business', 'writing', 'research', 'health', 'finance', 'other'];
  const kindOptions = ['software', 'content', 'research', 'business', 'creative', 'client', 'personal', 'other'];
  const statusOptions = ['active', 'paused', 'completed', 'archived'];
  const priorityLabels = ['none', 'low', 'medium', 'high', 'highest'];

  let progressPct = $derived(Math.round((project.progress ?? 0) * 100));

  let openTasks = $derived(projectTasks.filter((t) => !t.done));
  let doneTasks = $derived(projectTasks.filter((t) => t.done));

  // Per-goal task tallies — for the linked-goals section, surface
  // not just milestone progress but actual task velocity so the
  // user sees which goal is being actively worked on. Project
  // tasks already loaded; this is just a bucket-by-goalId
  // derivation, no extra wire calls.
  const tasksByGoal = $derived.by(() => {
    const m = new Map<string, { open: number; done: number }>();
    for (const t of projectTasks) {
      if (!t.goalId) continue;
      const b = m.get(t.goalId) ?? { open: 0, done: 0 };
      if (t.done) b.done++;
      else b.open++;
      m.set(t.goalId, b);
    }
    return m;
  });

  // ── Burn-up: weekly completion buckets for this project ──────────
  // Same ISO-week scheme as TaskVelocityWidget so a "W19" tally
  // matches what the dashboard shows. Scoped to projectTasks so
  // each project's chart only counts its own work.
  const BURNUP_WEEKS = 8;
  function weekKey(d: Date): string {
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
  const burnup = $derived.by(() => {
    const now = new Date();
    const weekStart = startOfIsoWeek(now);
    const thisKey = weekKey(now);
    const order: string[] = [];
    const labels = new Map<string, string>();
    for (let i = BURNUP_WEEKS - 1; i >= 0; i--) {
      const d = new Date(weekStart);
      d.setDate(d.getDate() - i * 7);
      const k = weekKey(d);
      order.push(k);
      labels.set(k, k === thisKey ? 'Now' : k.split('W')[1]);
    }
    const counts = new Map<string, number>();
    for (const t of doneTasks) {
      if (!t.completedAt) continue;
      const d = new Date(t.completedAt);
      if (Number.isNaN(d.getTime())) continue;
      const k = weekKey(d);
      if (!order.includes(k)) continue;
      counts.set(k, (counts.get(k) ?? 0) + 1);
    }
    return order.map((k) => ({
      label: labels.get(k) ?? k,
      count: counts.get(k) ?? 0,
      isThisWeek: k === thisKey
    }));
  });
  const burnupMax = $derived(burnup.reduce((m, b) => Math.max(m, b.count), 0));
  const burnupTotal = $derived(burnup.reduce((s, b) => s + b.count, 0));

  // ── AI project summary ───────────────────────────────────────────
  // Fires /chat with a focused prompt that bundles the project's
  // state — open vs done tasks, linked goals, deadlines context —
  // and asks for a 3-bullet status summary. Goes through the same
  // gate as the global chat (Sabbath / consent / redaction / audit)
  // so this isn't a side-channel that bypasses the AI foundation.
  let aiSummaryOpen = $state(false);
  let aiSummary = $state('');
  let aiSummaryBusy = $state(false);
  let aiSummaryError = $state('');
  let aiSummaryAbort: AbortController | null = null;

  async function runAISummary() {
    if (aiSummaryBusy) return;
    aiSummaryBusy = true;
    aiSummaryError = '';
    aiSummary = '';
    aiSummaryOpen = true;
    aiSummaryAbort = new AbortController();
    // Compose a structured context block the model can reason
    // over without us having to rely on an aicontext snapshot.
    // Keep it under ~3KB so token cost stays predictable.
    const ctx = [
      `Project: ${project.name}`,
      project.description ? `Description: ${project.description}` : '',
      project.next_action ? `Next action: ${project.next_action}` : '',
      `Tasks: ${openTasks.length} open / ${doneTasks.length} done`,
      openTasks.length > 0
        ? `Open tasks:\n${openTasks.slice(0, 15).map((t) => `- ${t.text}${t.dueDate ? ` (due ${t.dueDate})` : ''}`).join('\n')}`
        : '',
      linkedGoals.length > 0
        ? `Linked goals:\n${linkedGoals.map((g) => `- ${g.title}`).join('\n')}`
        : '',
      doneTasks.length > 0
        ? `Recent completions:\n${doneTasks.slice(-8).map((t) => `- ${t.text}`).join('\n')}`
        : ''
    ].filter(Boolean).join('\n\n');
    const userMessage =
      'Give me a concise status summary of this project. ' +
      'Use this format:\n' +
      '- **Where it stands:** one sentence on momentum / blockers\n' +
      '- **Next move:** one concrete action to keep things moving\n' +
      '- **Risks:** anything that looks stuck or overdue (or "none" if clean)\n\n' +
      'Project context:\n\n' + ctx;
    try {
      await api.chatStream(
        [{ role: 'user', content: userMessage }],
        undefined,
        {
          onChunk: (c) => { aiSummary += c; },
          onError: (err) => { aiSummaryError = err.message; }
        },
        aiSummaryAbort.signal
      );
    } finally {
      aiSummaryBusy = false;
      aiSummaryAbort = null;
    }
  }
  function cancelAISummary() { aiSummaryAbort?.abort(); }
</script>

<div class="h-full flex flex-col overflow-hidden">
  <!-- Header -->
  <header class="px-4 py-3 border-b border-surface1 flex-shrink-0 flex items-center gap-2">
    <button
      onclick={onClose}
      aria-label="back"
      class="md:hidden w-9 h-9 -ml-2 flex items-center justify-center text-subtext hover:text-primary"
    >
      <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M15 18l-6-6 6-6" stroke-linecap="round" stroke-linejoin="round" />
      </svg>
    </button>
    <span class="w-3 h-3 rounded-full flex-shrink-0" style="background: {colorVar(project.color)}"></span>
    {#if editingName}
      <input
        bind:value={nameBuf}
        onblur={commitName}
        onkeydown={(e) => { if (e.key === 'Enter') commitName(); else if (e.key === 'Escape') editingName = false; }}
        autofocus
        class="text-base sm:text-lg font-semibold flex-1 px-1 -mx-1 bg-surface0 border border-primary rounded text-text outline-none"
      />
    {:else}
      <button
        onclick={() => { nameBuf = project.name; editingName = true; }}
        class="text-base sm:text-lg font-semibold text-text truncate flex-1 text-left hover:text-primary"
        title="click to rename"
      >{project.name}</button>
    {/if}
    <select
      value={project.status ?? 'active'}
      onchange={(e) => setStatus((e.target as HTMLSelectElement).value)}
      class="text-xs px-2 py-1 bg-surface0 border border-surface1 rounded text-subtext hover:border-primary"
    >
      {#each statusOptions as s}<option value={s}>{s}</option>{/each}
    </select>
    <!-- "Pray for this" — opens /prayer with the project pre-linked.
         Lets a moment of clarity in the project view become an
         intention in one click. -->
    <a
      href={`/prayer?project=${encodeURIComponent(project.name)}`}
      title="add a prayer intention for this project"
      aria-label="pray for this project"
      class="w-9 h-9 flex items-center justify-center text-dim hover:text-primary rounded text-base"
    >🙏</a>
    <button
      onclick={deleteProject}
      title="delete project"
      class="w-9 h-9 flex items-center justify-center text-dim hover:text-error rounded"
      aria-label="delete"
    >
      <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
        <path d="M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/>
      </svg>
    </button>
  </header>

  <div class="flex-1 overflow-y-auto">
    <div class="max-w-3xl mx-auto p-4 sm:p-6 space-y-6">
      <!-- Classification strip — kind + venture at a glance. Only renders
           if at least one is set, so older projects don't get an empty
           row. The repo link doubles as a quick-launcher. -->
      {#if project.kind || project.venture || (project.kind === 'software' && project.repo_url)}
        <div class="flex flex-wrap items-center gap-2 -mt-1 text-xs">
          {#if project.kind}
            <span class="px-2 py-0.5 rounded bg-primary/15 text-primary uppercase tracking-wider text-[10px] font-medium">{project.kind}</span>
          {/if}
          {#if project.venture}
            <a
              href={`/projects?venture=${encodeURIComponent(project.venture)}`}
              class="px-2 py-0.5 rounded bg-secondary/15 text-secondary hover:bg-secondary/25"
              title="show all projects in this venture"
            >🏢 {project.venture}</a>
          {/if}
          {#if project.kind === 'software' && project.repo_url}
            <a
              href={project.repo_url}
              target="_blank"
              rel="noopener noreferrer"
              class="px-2 py-0.5 rounded bg-surface0 text-subtext border border-surface1 hover:border-primary hover:text-primary font-mono"
            >↗ repo</a>
          {/if}
        </div>
      {/if}
      <!-- Progress bar -->
      <section>
        <div class="flex items-baseline justify-between mb-1.5">
          <h3 class="text-xs uppercase tracking-wider text-dim font-medium">Progress</h3>
          <span class="text-xs text-subtext font-mono">
            {progressPct}%
            {#if project.tasksTotal != null && project.tasksTotal > 0}
              · <span class="text-dim">{project.tasksDone}/{project.tasksTotal} tasks</span>
            {/if}
          </span>
        </div>
        <div class="h-2 rounded-full bg-surface0 overflow-hidden">
          <div
            class="h-full transition-all"
            style="width: {progressPct}%; background: {colorVar(project.color)}"
          ></div>
        </div>

        {#if burnupTotal > 0}
          <!-- Burn-up — last 8 weeks of completion velocity for
               THIS project. Same ISO-week scheme as the dashboard
               TaskVelocityWidget so a "W19" tally matches across
               surfaces. Hidden when there's no completion history
               yet to avoid a row of empty bars. -->
          <div class="mt-3">
            <div class="flex items-baseline gap-2 mb-1.5">
              <span class="text-[10px] uppercase tracking-wider text-dim">8-week burn-up</span>
              <span class="flex-1"></span>
              <span class="text-[10px] text-dim font-mono">{burnupTotal} done</span>
            </div>
            <div class="flex items-end gap-1 h-10">
              {#each burnup as b (b.label)}
                {@const pct = burnupMax === 0 ? 0 : Math.max(2, Math.round((b.count / burnupMax) * 100))}
                <div class="flex-1 flex flex-col items-center justify-end gap-0.5" title="{b.label}: {b.count}">
                  <div
                    class="w-full rounded-t {b.isThisWeek ? 'bg-primary' : 'bg-secondary/40'} transition-all"
                    style="height: {pct}%"
                  ></div>
                  <div class="text-[9px] text-dim font-mono leading-none">{b.label}</div>
                </div>
              {/each}
            </div>
          </div>
        {/if}
      </section>

      <!-- AI summary — fires /chat with a project-context blob and
           asks for a 3-bullet status. Goes through the same gate
           as the global chat (Sabbath / consent / redaction /
           audit). Collapsible so the panel stays out of the way
           until the user asks for it. -->
      <section>
        <div class="flex items-baseline gap-2 mb-1.5">
          <h3 class="text-xs uppercase tracking-wider text-dim font-medium flex-1">AI summary</h3>
          {#if aiSummaryBusy}
            <button onclick={cancelAISummary} class="text-[11px] text-warning hover:underline">cancel</button>
          {:else if aiSummary}
            <button
              onclick={() => { aiSummary = ''; aiSummaryError = ''; aiSummaryOpen = false; }}
              class="text-[11px] text-dim hover:text-error"
            >clear</button>
          {/if}
          <button
            onclick={() => void runAISummary()}
            disabled={aiSummaryBusy || projectTasks.length === 0}
            class="text-[11px] px-2 py-0.5 rounded bg-surface0 border border-surface1 text-subtext hover:border-primary disabled:opacity-50"
            title="Ask the AI to summarise this project's state"
          >{aiSummaryBusy ? '✨ thinking…' : aiSummary ? '✨ regenerate' : '✨ summarise'}</button>
        </div>
        {#if aiSummaryError}
          <div class="text-xs text-error border border-error/30 bg-error/5 rounded px-3 py-2">{aiSummaryError}</div>
        {:else if aiSummary || aiSummaryBusy}
          <div class="bg-surface0 border border-surface1 rounded px-3 py-2 text-sm text-text break-words">
            <div class="prose prose-sm max-w-none">
              <MarkdownRenderer body={aiSummary || '_…_'} />
            </div>
          </div>
        {/if}
      </section>

      <!-- Description -->
      <section>
        <h3 class="text-xs uppercase tracking-wider text-dim font-medium mb-1.5">Description</h3>
        {#if editingDescription}
          <textarea
            bind:value={descBuf}
            onblur={commitDescription}
            onkeydown={(e) => { if (e.key === 'Escape') editingDescription = false; }}
            autofocus
            rows="3"
            class="w-full px-3 py-2 bg-surface0 border border-primary rounded text-sm text-text outline-none"
          ></textarea>
        {:else}
          <button
            onclick={() => { descBuf = project.description ?? ''; editingDescription = true; }}
            class="w-full text-left px-3 py-2 text-sm rounded hover:bg-surface0 {project.description ? 'text-text' : 'text-dim italic'}"
          >{project.description || 'click to add a description…'}</button>
        {/if}
      </section>

      <!-- Next Action (highlight chip) -->
      <section>
        <div class="flex items-baseline justify-between mb-1.5">
          <h3 class="text-xs uppercase tracking-wider text-dim font-medium">Next action</h3>
          <a
            href={`/calendar?plan=1&project=${encodeURIComponent(project.name)}`}
            class="text-xs text-secondary hover:underline"
            title="open the calendar in plan mode to drag tasks onto the grid"
          >schedule →</a>
        </div>
        {#if editingNextAction}
          <input
            bind:value={nextActionBuf}
            onblur={commitNextAction}
            onkeydown={(e) => { if (e.key === 'Enter') commitNextAction(); else if (e.key === 'Escape') editingNextAction = false; }}
            autofocus
            class="w-full px-3 py-2 bg-surface0 border border-primary rounded text-sm text-text outline-none"
          />
        {:else}
          <button
            onclick={() => { nextActionBuf = project.next_action ?? ''; editingNextAction = true; }}
            class="w-full text-left px-3 py-2.5 rounded text-sm border border-warning/30 bg-warning/10 text-warning hover:bg-warning/20 {!project.next_action ? 'italic opacity-70' : 'font-medium'}"
          >→ {project.next_action || 'what\'s the next concrete step?'}</button>
        {/if}
      </section>

      <!-- Deadlines linked to this project. Free-standing component
           so the same panel renders on goals + ventures with the same
           visual language. Quick-add jumps to /deadlines with project
           pre-set; full editing still happens on the deadlines page. -->
      <EntityDeadlines scope={{ kind: 'project', name: project.name }} />

      <!-- Goals + milestones -->
      <section>
        <h3 class="text-xs uppercase tracking-wider text-dim font-medium mb-2">Goals & milestones</h3>
        <GoalEditor goals={project.goals ?? []} onChange={updateGoals} />
      </section>

      <!-- Linked top-level goals (.granit/goals.json) -->
      {#if linkedGoals.length > 0}
        <section>
          <div class="flex items-baseline justify-between mb-2">
            <h3 class="text-xs uppercase tracking-wider text-dim font-medium">Linked goals · {linkedGoals.length}</h3>
            <a href="/goals" class="text-xs text-secondary hover:underline">open /goals →</a>
          </div>
          <ul class="space-y-1.5">
            {#each linkedGoals as g (g.id)}
              {@const ms = g.milestones ?? []}
              {@const total = ms.length}
              {@const done = ms.filter((m) => m.done).length}
              {@const pct = total === 0 ? (g.status === 'completed' ? 100 : 0) : Math.round((done / total) * 100)}
              {@const taskCounts = tasksByGoal.get(g.id) ?? { open: 0, done: 0 }}
              {@const goalTaskTotal = taskCounts.open + taskCounts.done}
              <!-- Each row is now a clickable link to the goal's
                   detail drawer (?focus=<id> auto-opens it) with
                   milestone progress AND task tally surfaced
                   side-by-side. The two metrics complement: the
                   milestone bar shows planned-vs-done, the task
                   counts show ongoing momentum. -->
              <li>
                <a
                  href="/goals?focus={encodeURIComponent(g.id)}"
                  class="block px-3 py-2 bg-surface0 hover:bg-surface1 rounded text-sm transition-colors"
                >
                  <div class="flex items-baseline justify-between gap-2">
                    <span class="text-text truncate">{g.title}</span>
                    <span class="text-[11px] text-dim flex-shrink-0">
                      {pct}%{#if total > 0} · {done}/{total}{/if}
                      {#if goalTaskTotal > 0}
                        <span class="text-secondary ml-1" title="open / done tasks linked to this goal">{taskCounts.open}/{goalTaskTotal} ✓</span>
                      {/if}
                    </span>
                  </div>
                  {#if total > 0}
                    <div class="mt-1 h-1 bg-mantle rounded-full overflow-hidden">
                      <div class="h-full bg-primary" style="width: {pct}%"></div>
                    </div>
                  {/if}
                </a>
              </li>
            {/each}
          </ul>
        </section>
      {/if}

      <!-- Time spent -->
      {#if (project.time_spent ?? 0) > 0}
        <section>
          <h3 class="text-xs uppercase tracking-wider text-dim font-medium mb-1.5">Time spent</h3>
          <p class="text-sm text-text">
            {Math.floor((project.time_spent ?? 0) / 60)}h {(project.time_spent ?? 0) % 60}m
            <span class="text-dim text-xs">tracked</span>
          </p>
        </section>
      {/if}

      <!-- Linked tasks -->
      <section>
        <div class="flex items-baseline justify-between mb-2">
          <h3 class="text-xs uppercase tracking-wider text-dim font-medium">Tasks · {projectTasks.length}</h3>
          <a
            href="/tasks?project={encodeURIComponent(project.name)}"
            class="text-xs text-secondary hover:underline"
          >open in /tasks →</a>
        </div>
        {#if loadingTasks && projectTasks.length === 0}
          <div class="text-xs text-dim">loading…</div>
        {:else if projectTasks.length === 0}
          <div class="text-xs text-dim italic">No tasks linked. Tag a task with <code class="text-secondary">project:{project.name}</code> or place it under <code class="text-secondary">{project.folder || '<no folder>'}</code>.</div>
        {:else}
          <div class="space-y-px">
            {#each openTasks.slice(0, 25) as t (t.id)}
              <TaskRow task={t} onChanged={loadTasks} />
            {/each}
          </div>
          {#if doneTasks.length > 0}
            <button
              onclick={() => (showCompletedTasks = !showCompletedTasks)}
              class="mt-2 text-[11px] text-dim hover:text-text"
            >{showCompletedTasks ? '▾' : '▸'} {doneTasks.length} completed</button>
            {#if showCompletedTasks}
              <div class="space-y-px mt-1 opacity-70">
                {#each doneTasks.slice(0, 25) as t (t.id)}
                  <TaskRow task={t} onChanged={loadTasks} />
                {/each}
              </div>
            {/if}
          {/if}
        {/if}
      </section>

      <!-- Metadata grid -->
      <section class="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-3 pt-4 border-t border-surface1">
        <div>
          <label for="prj-kind" class="text-[11px] uppercase tracking-wider text-dim block mb-1">Kind</label>
          <select
            id="prj-kind"
            value={project.kind ?? ''}
            onchange={(e) => patch({ kind: (e.target as HTMLSelectElement).value })}
            class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text"
          >
            <option value="">—</option>
            {#each kindOptions as k}<option value={k}>{k}</option>{/each}
          </select>
        </div>
        <div>
          <label for="prj-venture" class="text-[11px] uppercase tracking-wider text-dim block mb-1">Venture / Company</label>
          <input
            id="prj-venture"
            value={project.venture ?? ''}
            onblur={(e) => patch({ venture: (e.target as HTMLInputElement).value })}
            placeholder="e.g. Stoicera"
            class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text"
          />
        </div>
        {#if (project.kind ?? '') === 'software'}
          <div class="sm:col-span-2">
            <label for="prj-repo" class="text-[11px] uppercase tracking-wider text-dim block mb-1">Repo URL</label>
            <div class="flex gap-2">
              <input
                id="prj-repo"
                type="url"
                value={project.repo_url ?? ''}
                onblur={(e) => patch({ repo_url: (e.target as HTMLInputElement).value })}
                placeholder="https://github.com/you/repo"
                class="flex-1 px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text font-mono"
              />
              {#if project.repo_url}
                <a
                  href={project.repo_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  class="px-2.5 py-1 text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary hover:text-primary"
                  title="open repo"
                >open ↗</a>
              {/if}
            </div>
          </div>
        {/if}
        <div>
          <label for="prj-folder" class="text-[11px] uppercase tracking-wider text-dim block mb-1">Folder</label>
          <input
            id="prj-folder"
            value={project.folder ?? ''}
            onblur={(e) => setFolder((e.target as HTMLInputElement).value)}
            placeholder="e.g. Projects/foo"
            class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text"
          />
        </div>
        <div>
          <label for="prj-due" class="text-[11px] uppercase tracking-wider text-dim block mb-1">Due date</label>
          <input
            id="prj-due"
            type="date"
            value={project.due_date ?? ''}
            onchange={(e) => setDueDate((e.target as HTMLInputElement).value)}
            class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text"
          />
        </div>
        <div>
          <label for="prj-tags" class="text-[11px] uppercase tracking-wider text-dim block mb-1">Tags</label>
          <input
            id="prj-tags"
            value={(project.tags ?? []).join(', ')}
            onblur={(e) => setTags((e.target as HTMLInputElement).value)}
            placeholder="comma, separated"
            class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text"
          />
        </div>
        <div>
          <label for="prj-cat" class="text-[11px] uppercase tracking-wider text-dim block mb-1">Category</label>
          <select
            id="prj-cat"
            value={project.category ?? ''}
            onchange={(e) => patch({ category: (e.target as HTMLSelectElement).value })}
            class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text"
          >
            <option value="">—</option>
            {#each categoryOptions as c}<option value={c}>{c}</option>{/each}
          </select>
        </div>
        <div>
          <span class="text-[11px] uppercase tracking-wider text-dim block mb-1">Priority</span>
          <div class="flex gap-1">
            {#each priorityLabels as label, i}
              <button
                onclick={() => setPriority(i)}
                class="flex-1 px-1 py-1 text-[11px] rounded {project.priority === i ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1'}"
              >{label}</button>
            {/each}
          </div>
        </div>
        <div>
          <span class="text-[11px] uppercase tracking-wider text-dim block mb-1">Color</span>
          <div class="flex gap-1.5 flex-wrap">
            {#each colorOptions as c}
              <button
                onclick={() => setColor(c)}
                aria-label="color {c}"
                class="w-6 h-6 rounded-full border-2 {project.color === c ? 'border-text' : 'border-surface1'}"
                style="background: {colorVar(c)}"
              ></button>
            {/each}
          </div>
        </div>
      </section>

      <footer class="text-[11px] text-dim pt-2 border-t border-surface1 flex justify-between">
        <span>created {project.created_at || '—'}</span>
        <span>updated {project.updated_at || '—'}</span>
      </footer>
    </div>
  </div>
</div>
