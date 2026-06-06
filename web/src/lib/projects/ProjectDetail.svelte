<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Project, type ProjectGoal } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import GoalEditor from './GoalEditor.svelte';
  import ProjectNotesTab from './ProjectNotesTab.svelte';
  import ProjectStarterPack from './ProjectStarterPack.svelte';
  import TaskRow from '$lib/components/TaskRow.svelte';
  import EntityDeadlines from '$lib/deadlines/EntityDeadlines.svelte';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';
  import { focusOnMount } from '$lib/util/focusOnMount';
  import { onWsEvent } from '$lib/ws';
  import {
    createProjectAIHealth,
    type HealthMomentum
  } from './projectAIHealth.svelte';
  import { createProjectAIBrief } from './projectAIBrief.svelte';
  import { createProjectDetailData } from './projectDetailData.svelte';
  import { createProjectInlineEdit } from './projectInlineEdit.svelte';
  import { createProjectStats } from './projectStats.svelte';
  import { askAIAboutProject, openProjectResearch } from './projectAISeed';

  let { project, onClose, onUpdated, onDeleted, onOpenDashboard }: {
    project: Project;
    onClose: () => void;
    onUpdated: () => void | Promise<void>;
    onDeleted: (name: string) => void | Promise<void>;
    /** Optional — when supplied, the header shows a Dashboard button
     *  that delegates to the parent /projects page. The parent owns
     *  the dashboard URL state (?dashboard=1) and renders the
     *  ProjectDashboardPanel overlay on top, so this component stays
     *  unaware of how the dashboard mounts. */
    onOpenDashboard?: () => void;
  } = $props();

  // Inline-edit + draft autosave for the three editable text fields
  // (description, next_action, name) live in projectInlineEdit. The
  // controller owns the buffer + cancelling sentinels + commit
  // wrappers; the parent only wires patch + reset.
  const editCtl = createProjectInlineEdit({
    getProject: () => project,
    patch: (p) => patch(p)
  });

  // Loaded data + loaders live in projectDetailData. Read via
  // dataCtl.projectTasks / linkedGoals / projectVision / loadingTasks
  // / projectVisionLoading / projectVisionCreating / projectVisionKey.
  const dataCtl = createProjectDetailData({ getProject: () => project });
  const loadTasks = dataCtl.loadTasks;
  const loadLinkedGoals = dataCtl.loadLinkedGoals;
  const loadProjectVision = dataCtl.loadProjectVision;
  const createProjectVision = dataCtl.createProjectVision;

  let showCompletedTasks = $state(false);

  // Local read aliases for the template — writes go through dataCtl
  // via the loader methods. Same pattern as aiOverlayState's aliases.
  const projectTasks = $derived(dataCtl.projectTasks);
  const linkedGoals = $derived(dataCtl.linkedGoals);
  const projectVision = $derived(dataCtl.projectVision);
  const loadingTasks = $derived(dataCtl.loadingTasks);
  const projectVisionLoading = $derived(dataCtl.projectVisionLoading);
  const projectVisionCreating = $derived(dataCtl.projectVisionCreating);
  const projectVisionKey = $derived(dataCtl.projectVisionKey);

  // Last project name we initialised buffers for. When the parent
  // swaps the project prop without unmounting (master-detail list-
  // click), we need to close any open inline editors — descBuf etc.
  // still hold the OLD project's text, and the draft-save $effects
  // would otherwise write that text under the NEW project's key.
  let lastProjectName = '';
  $effect(() => {
    void project.name;
    loadTasks();
    loadLinkedGoals();
    void loadProjectVision();
    // Project-switch reset: drop edit state + cancel pending draft
    // writers so the prior project's buffers don't bleed into the
    // new project's localStorage key.
    if (lastProjectName && lastProjectName !== project.name) {
      editCtl.reset();
    }
    lastProjectName = project.name;
  });

  // Reload the project's vision when the central catalogue changes
  // (user edited the vision on /vision and we're showing its
  // compact read view here).
  onMount(() => {
    return onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/visions.json') {
        void loadProjectVision();
      }
    });
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

  // commitDescription / commitNextAction / commitName live in
  // editCtl; aliases keep the template call sites terse.
  const commitDescription = editCtl.commitDescription;
  const commitNextAction = editCtl.commitNextAction;
  const commitName = editCtl.commitName;

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

  // AI overlay seeders — composing the project's context into a chat
  // seed lives in projectAISeed. Bound to one-liner aliases here so
  // the existing template onclick handlers stay terse.
  function askAIAboutThisProject() {
    askAIAboutProject({ project, projectTasks, linkedGoals });
  }
  function openResearchMode() {
    openProjectResearch({ project, projectTasks, linkedGoals });
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

  const progressPct = $derived(statsCtl.progressPct);

  // View-time derives — openTasks/doneTasks split, weekSchedule strip,
  // tasksByGoal map, burnup chart — live in projectStats.
  const statsCtl = createProjectStats({
    getProject: () => project,
    getProjectTasks: () => projectTasks
  });
  const openTasks = $derived(statsCtl.openTasks);
  const doneTasks = $derived(statsCtl.doneTasks);
  const weekSchedule = $derived(statsCtl.weekSchedule);
  const weekScheduleMax = $derived(statsCtl.weekScheduleMax);
  const weekScheduleTotal = $derived(statsCtl.weekScheduleTotal);
  const tasksByGoal = $derived(statsCtl.tasksByGoal);
  const burnup = $derived(statsCtl.burnup);
  const burnupMax = $derived(statsCtl.burnupMax);
  const burnupTotal = $derived(statsCtl.burnupTotal);

  // ── AI project health check ──────────────────────────────────────
  // Bundles the project's state — open/done tasks, recent completions,
  // last completion age, linked goals — and asks for a structured
  // 3-section verdict: momentum (alive/slowing/stalled/dead),
  // blockers (what's actually stuck), and the single next concrete
  // action. The model returns JSON so we can render the momentum
  // badge as a coloured pill instead of fishing prose for keywords;
  // a plain-prose fallback is rendered if parsing fails so a flaky
  // response still shows something useful.
  //
  // Goes through chatStream → /chat/stream so this remains audit/
  // Sabbath/redaction-gated — no side channel around the AI gate.
  // AI "project health verdict" controller. See projectAIHealth for
  // the prompt + JSON-schema details. The controller owns the state
  // + the streamed JSON parse; the parent reads aiHealthCtl.X.
  const aiHealthCtl = createProjectAIHealth({
    getProject: () => project,
    getOpenTasks: () => openTasks,
    getDoneTasks: () => doneTasks,
    getLinkedGoals: () => linkedGoals,
    getAllTasks: () => projectTasks
  });
  const runAIHealth = aiHealthCtl.run;
  const cancelAIHealth = aiHealthCtl.cancel;
  const clearAIHealth = aiHealthCtl.dismiss;

  function momentumTone(m: HealthMomentum): string {
    if (m === 'alive') return 'success';
    if (m === 'slowing') return 'warning';
    if (m === 'stalled') return 'accent';
    return 'error';
  }
  function momentumLabel(m: HealthMomentum): string {
    if (m === 'alive') return 'Alive';
    if (m === 'slowing') return 'Slowing';
    if (m === 'stalled') return 'Stalled';
    return 'Dead';
  }

  // ── AI-drafted project brief ─────────────────────────────────────
  // Only offered when the description field is empty AND the project
  // has at least some signal (a task or a linked goal). With nothing
  // to read, the model would just paraphrase the project name. The
  // draft sits in a review pane above the description editor — the
  // user accepts (saves to description) or dismisses; it never
  // silently overwrites.
  // AI "project brief" controller. See projectAIBrief.svelte for the
  // prompt + the why on no-bullets-no-preamble-no-corporate-sludge.
  const aiBriefCtl = createProjectAIBrief({
    getProject: () => project,
    getOpenTasks: () => openTasks,
    getDoneTasks: () => doneTasks,
    getLinkedGoals: () => linkedGoals,
    applyDescription: (text) => patch({ description: text })
  });
  const runAIBrief = aiBriefCtl.run;
  const cancelAIBrief = aiBriefCtl.cancel;
  const applyAIBrief = aiBriefCtl.apply;
  const dismissAIBrief = aiBriefCtl.dismiss;
</script>

<div class="h-full flex flex-col overflow-hidden">
  <!-- Header -->
  <header class="px-3 py-2 border-b border-surface1 flex-shrink-0 flex items-center gap-2">
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
    {#if editCtl.editingName}
      <input
        bind:value={editCtl.nameBuf}
        onblur={commitName}
        onkeydown={(e) => { if (e.key === 'Enter') commitName(); else if (e.key === 'Escape') editCtl.editingName = false; }}
        use:focusOnMount
        class="text-base sm:text-lg font-semibold flex-1 px-1 -mx-1 bg-surface0 border border-primary rounded text-text outline-none"
      />
    {:else}
      <button
        onclick={editCtl.startEditName}
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
    <!-- Dashboard — opens the full-screen ProjectDashboardPanel
         overlay above the projects layout. Lives behind a thin
         delegate so the detail drawer stays unaware of how the
         dashboard mounts; the parent /projects page owns the
         ?dashboard=1 URL state and renders the overlay. -->
    <!-- Ask AI about this project — opens the AIOverlay seeded
         with project name + status + open-task count + linked
         goals so the model is grounded before the first user
         message. send=false: prompt is pre-filled but not yet
         submitted; the user can edit before pressing Enter. -->
    <button
      onclick={askAIAboutThisProject}
      title="ask AI about this project"
      aria-label="ask ai about this project"
      class="px-2.5 py-1.5 min-h-[36px] text-xs rounded border border-surface1 bg-surface0 text-subtext hover:border-primary hover:text-primary inline-flex items-center gap-1"
    >
      <span aria-hidden="true">✨</span>
      <span class="hidden sm:inline">Ask AI</span>
    </button>
    <!-- Research Mode — pins the AI overlay as a side rail seeded
         with this project's context, framed as exploration rather
         than action. Stays visible while the user moves through
         notes / tasks / deadlines so the AI can be the running
         thinking partner instead of a one-shot Q&A. -->
    <button
      onclick={openResearchMode}
      title="open AI side-rail in research mode for this project"
      aria-label="open research mode"
      class="px-2.5 py-1.5 min-h-[36px] text-xs rounded border border-surface1 bg-surface0 text-subtext hover:border-primary hover:text-primary inline-flex items-center gap-1"
    >
      <span aria-hidden="true">🔬</span>
      <span class="hidden sm:inline">Research Mode</span>
    </button>
    {#if onOpenDashboard}
      <button
        onclick={onOpenDashboard}
        title="open project dashboard — visual operating picture"
        aria-label="open project dashboard"
        class="px-2.5 py-1.5 min-h-[36px] text-xs rounded border border-surface1 bg-surface0 text-subtext hover:border-primary hover:text-primary inline-flex items-center gap-1"
      >
        <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <rect x="3" y="3" width="7" height="9" rx="1" />
          <rect x="14" y="3" width="7" height="5" rx="1" />
          <rect x="14" y="12" width="7" height="9" rx="1" />
          <rect x="3" y="16" width="7" height="5" rx="1" />
        </svg>
        <span class="hidden sm:inline">Dashboard</span>
      </button>
    {/if}
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
    <div class="max-w-3xl mx-auto p-4 sm:p-6 space-y-4">
      <!-- Classification strip — kind + venture at a glance. Only renders
           if at least one is set, so older projects don't get an empty
           row. The repo link doubles as a quick-launcher. -->
      {#if project.kind || project.venture || (project.kind === 'software' && project.repo_url)}
        <div class="flex flex-wrap items-center gap-2 -mt-1 text-xs">
          {#if project.kind}
            <span class="px-2 py-0.5 rounded bg-surface1 text-primary uppercase tracking-wider text-[10px] font-medium">{project.kind}</span>
          {/if}
          {#if project.venture}
            <a
              href={`/projects?venture=${encodeURIComponent(project.venture)}`}
              class="px-2 py-0.5 rounded bg-surface1 text-secondary hover:bg-surface2"
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
                    class="w-full rounded-t {b.isThisWeek ? 'bg-primary' : 'bg-surface2'} transition-all"
                    style="height: {pct}%"
                  ></div>
                  <div class="text-[9px] text-dim font-mono leading-none">{b.label}</div>
                </div>
              {/each}
            </div>
          </div>
        {/if}
      </section>

      {#if weekScheduleTotal > 0}
        <!-- This-week schedule strip — Mon-Sun cells showing how
             many of this project's tasks are scheduled per day,
             with each cell clickable into the calendar at that
             day. Hidden when nothing's scheduled this week so a
             quiet project doesn't show empty bars. The strip
             complements the burn-up: burn-up is "what we did
             over the last 8 weeks", schedule is "what's queued
             for the next few days." -->
        <section>
          <div class="flex items-baseline gap-2 mb-2">
            <h3 class="text-xs uppercase tracking-wider text-dim font-medium flex-1">This week's schedule</h3>
            <span class="text-[11px] text-dim font-mono">{weekScheduleTotal} task{weekScheduleTotal === 1 ? '' : 's'}</span>
            <a href="/calendar" class="text-[11px] text-secondary hover:underline">/calendar →</a>
          </div>
          <div class="flex items-end gap-1 h-12">
            {#each weekSchedule as d (d.date)}
              {@const pct = weekScheduleMax === 0 ? 0 : Math.max(2, Math.round((d.count / weekScheduleMax) * 100))}
              <a
                href="/calendar?date={d.date}"
                class="flex-1 flex flex-col items-center justify-end gap-0.5 hover:opacity-80 transition-opacity"
                title="{d.label} {d.date}: {d.count} scheduled"
              >
                <div
                  class="w-full rounded-t {d.isToday ? 'bg-primary' : 'bg-surface2'} transition-all"
                  style="height: {pct}%"
                ></div>
                <div class="text-[9px] {d.isToday ? 'text-primary' : 'text-dim'} font-mono leading-none">{d.label}</div>
              </a>
            {/each}
          </div>
        </section>
      {/if}

      <!-- AI health check — structured 3-section verdict (momentum,
           blockers, next move) grounded in the project's tasks +
           recent activity + last completion + linked goals. The
           model is asked for STRICT JSON so we can render the
           momentum as a coloured pill instead of fishing the prose
           for keywords; if parsing fails we still show the raw
           output so a flaky response is recoverable. The "AI saw …"
           context line keeps the verdict legible — the user knows
           exactly what was fed in. Goes through chatStream so
           Sabbath / consent / redaction / audit all apply. -->
      <section>
        <div class="flex items-baseline gap-2 mb-1.5">
          <h3 class="text-xs uppercase tracking-wider text-dim font-medium flex-1">AI health check</h3>
          {#if aiHealthCtl.aiHealthBusy}
            <button onclick={cancelAIHealth} class="text-[11px] text-warning hover:underline">cancel</button>
          {:else if aiHealthCtl.aiHealth || aiHealthCtl.aiHealthRaw || aiHealthCtl.aiHealthError}
            <button
              onclick={clearAIHealth}
              class="text-[11px] text-dim hover:text-error"
            >clear</button>
          {/if}
          <button
            onclick={() => void runAIHealth()}
            disabled={aiHealthCtl.aiHealthBusy || projectTasks.length === 0}
            class="text-[11px] px-2 py-0.5 rounded bg-surface0 border border-surface1 text-subtext hover:border-primary disabled:opacity-50"
            title="Ask the AI for a momentum / blockers / next-move verdict on this project"
          >{aiHealthCtl.aiHealthBusy ? '✨ analysing…' : (aiHealthCtl.aiHealth || aiHealthCtl.aiHealthRaw) ? '✨ rerun' : '✨ check health'}</button>
        </div>

        {#if aiHealthCtl.aiHealthContextLine && (aiHealthCtl.aiHealthBusy || aiHealthCtl.aiHealth || aiHealthCtl.aiHealthRaw || aiHealthCtl.aiHealthError)}
          <p class="text-[10px] text-dim mb-1.5 font-mono">{aiHealthCtl.aiHealthContextLine}</p>
        {/if}

        {#if aiHealthCtl.aiHealth}
          {@const tone = momentumTone(aiHealthCtl.aiHealth.momentum)}
          <div class="bg-surface0 border border-surface1 rounded px-3 py-3 text-sm text-text space-y-3">
            <!-- Momentum pill + reason -->
            <div class="flex items-baseline gap-2">
              <span
                class="px-2 py-0.5 rounded text-[10px] uppercase tracking-wider font-medium flex-shrink-0"
                style="background: var(--color-{tone}); color: var(--color-base);"
              >{momentumLabel(aiHealthCtl.aiHealth.momentum)}</span>
              <span class="text-text/90 text-xs leading-snug">{aiHealthCtl.aiHealth.momentum_reason}</span>
            </div>

            <!-- Blockers — listed individually so each is scannable
                 instead of buried in a paragraph. -->
            <div>
              <div class="text-[10px] uppercase tracking-wider text-dim mb-1">Blockers</div>
              {#if aiHealthCtl.aiHealth.blockers.length === 0}
                <p class="text-xs text-success">Nothing flagged as stuck.</p>
              {:else}
                <ul class="space-y-1">
                  {#each aiHealthCtl.aiHealth.blockers as b, i (i)}
                    <li class="text-xs text-text/90 flex gap-1.5">
                      <span class="text-error flex-shrink-0">•</span>
                      <span>{b}</span>
                    </li>
                  {/each}
                </ul>
              {/if}
            </div>

            <!-- Single next concrete action — surfaced as a chip the
                 user can copy/paste into the project's next_action
                 field with one click. -->
            <div>
              <div class="flex items-baseline gap-2 mb-1">
                <span class="text-[10px] uppercase tracking-wider text-dim flex-1">Next concrete action</span>
                <button
                  onclick={() => patch({ next_action: aiHealthCtl.aiHealth!.next_action })}
                  class="text-[10px] text-secondary hover:underline"
                  title="copy this into the project's Next action field"
                >use as next action →</button>
              </div>
              <p class="text-sm text-warning font-medium">→ {aiHealthCtl.aiHealth.next_action}</p>
            </div>
          </div>
        {:else if aiHealthCtl.aiHealthError}
          <div class="text-xs text-error border border-error bg-surface0 rounded px-3 py-2">
            <div class="font-medium mb-1">{aiHealthCtl.aiHealthError}</div>
            {#if aiHealthCtl.aiHealthRaw}
              <pre class="text-[10px] text-dim font-mono whitespace-pre-wrap mt-1">{aiHealthCtl.aiHealthRaw}</pre>
            {/if}
          </div>
        {:else if aiHealthCtl.aiHealthBusy}
          <div class="bg-surface0 border border-surface1 rounded px-3 py-2 text-xs text-dim italic">analysing project state…</div>
        {/if}
      </section>

      <!-- Starter pack — one-tap AI generates the four bootstrap
           documents (charter / milestones / risks / kickoff agenda)
           as individual notes the user reviews + saves under
           Projects/<name>/. Streams through chatStream so Sabbath /
           consent / redaction / audit all apply unchanged. -->
      <section>
        <h3 class="text-xs uppercase tracking-wider text-dim font-medium mb-1.5">Starter pack</h3>
        <ProjectStarterPack project={project} />
      </section>

      <!-- Description -->
      <section>
        <div class="flex items-baseline gap-2 mb-1.5">
          <h3 class="text-xs uppercase tracking-wider text-dim font-medium flex-1">Description</h3>
          <!-- AI brief offer — only when description is empty AND
               there's enough signal to ground a brief in (a task
               or a linked goal). With nothing to read, the model
               would just paraphrase the project name. -->
          {#if !project.description && !aiBriefCtl.aiBrief && !aiBriefCtl.aiBriefBusy && (projectTasks.length > 0 || linkedGoals.length > 0)}
            <button
              onclick={() => void runAIBrief()}
              class="text-[11px] px-2 py-0.5 rounded bg-surface0 border border-surface1 text-subtext hover:border-primary"
              title="Ask the AI to draft a one-paragraph brief from the project's tasks + goals"
            >✨ draft brief</button>
          {:else if aiBriefCtl.aiBriefBusy}
            <button onclick={cancelAIBrief} class="text-[11px] text-warning hover:underline">cancel</button>
          {/if}
        </div>

        {#if aiBriefCtl.aiBriefError}
          <div class="text-xs text-error border border-error bg-surface0 rounded px-3 py-2 mb-2">
            {aiBriefCtl.aiBriefError}
            <button onclick={dismissAIBrief} class="ml-2 underline">dismiss</button>
          </div>
        {/if}

        {#if aiBriefCtl.aiBrief || aiBriefCtl.aiBriefBusy}
          <!-- Draft brief in review state. The user has to accept
               (saves to description) or dismiss — never silently
               overwrites the field. The "grounded in N tasks + M
               goals" line tells the user what the model actually
               read so the brief feels auditable, not magic. -->
          <div class="mb-2 border border-surface2 bg-surface1 rounded">
            <div class="px-3 py-2 border-b border-surface2 flex items-baseline gap-2 text-[10px]">
              <span class="text-primary uppercase tracking-wider font-medium flex-1">AI draft · review before saving</span>
              <span class="text-dim font-mono">grounded in {projectTasks.length} task{projectTasks.length === 1 ? '' : 's'}{linkedGoals.length > 0 ? ` + ${linkedGoals.length} goal${linkedGoals.length === 1 ? '' : 's'}` : ''}</span>
            </div>
            <div class="px-3 py-2 text-sm text-text whitespace-pre-wrap break-words min-h-[2rem]">
              {aiBriefCtl.aiBrief || '…'}
            </div>
            {#if !aiBriefCtl.aiBriefBusy && aiBriefCtl.aiBrief}
              <div class="px-3 py-2 border-t border-surface2 flex items-center gap-2">
                <button
                  onclick={() => void applyAIBrief()}
                  disabled={aiBriefCtl.aiBriefSaving}
                  class="text-[11px] px-2 py-0.5 rounded bg-primary text-on-primary hover:opacity-90 disabled:opacity-50"
                >{aiBriefCtl.aiBriefSaving ? 'saving…' : 'save as description'}</button>
                <button
                  onclick={() => void runAIBrief()}
                  disabled={aiBriefCtl.aiBriefSaving}
                  class="text-[11px] px-2 py-0.5 rounded bg-surface0 border border-surface1 text-subtext hover:border-primary"
                >regenerate</button>
                <button
                  onclick={dismissAIBrief}
                  disabled={aiBriefCtl.aiBriefSaving}
                  class="text-[11px] text-dim hover:text-error ml-auto"
                >dismiss</button>
              </div>
            {/if}
          </div>
        {/if}

        {#if editCtl.editingDescription}
          <textarea
            bind:value={editCtl.descBuf}
            onblur={commitDescription}
            onkeydown={(e) => {
              if (e.key === 'Escape') {
                e.preventDefault();
                editCtl.cancelEditDescription();
              }
            }}
            use:focusOnMount
            rows="3"
            class="w-full px-3 py-2 bg-surface0 border border-primary rounded text-sm text-text outline-none"
          ></textarea>
        {:else}
          <button
            onclick={editCtl.startEditDescription}
            class="w-full text-left px-3 py-2 text-sm rounded hover:bg-surface0 {project.description ? 'text-text' : 'text-dim italic'}"
          >{project.description || 'click to add a description…'}</button>
        {/if}
      </section>

      <!-- Vision — per-project narrative. Read view only here; edits
           happen on /vision (the central editor with history + edit
           reasons). The CTA on the empty state creates an empty
           vision doc with key 'project:<slug>' and jumps to /vision
           ?tab=… so the user can write it. -->
      <section>
        <div class="flex items-baseline gap-2 mb-1.5">
          <h3 class="text-xs uppercase tracking-wider text-dim font-medium flex-1">Vision</h3>
          {#if projectVision && (projectVision.content?.trim() ?? '') !== ''}
            <a
              href={`/vision?tab=${encodeURIComponent(projectVisionKey)}`}
              class="text-[11px] text-secondary hover:underline"
              title="Open this project's vision in /vision for editing + history"
            >edit →</a>
          {/if}
        </div>

        {#if projectVisionLoading}
          <div class="h-3 bg-surface1 rounded animate-pulse w-3/4"></div>
        {:else if projectVision && (projectVision.content?.trim() ?? '') !== ''}
          <div class="px-3 py-2 bg-surface0 rounded text-sm text-text project-vision-body">
            <MarkdownRenderer body={projectVision.content ?? ''} />
          </div>
        {:else if projectVision}
          <!-- Doc exists but content is empty — direct the user to fill it -->
          <a
            href={`/vision?tab=${encodeURIComponent(projectVisionKey)}`}
            class="text-xs text-secondary hover:underline"
          >Vision für dieses Projekt schreiben →</a>
        {:else}
          <button
            type="button"
            onclick={createProjectVision}
            disabled={projectVisionCreating}
            class="text-xs text-secondary hover:underline disabled:opacity-50"
          >+ Vision für dieses Projekt anlegen</button>
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
        {#if editCtl.editingNextAction}
          <input
            bind:value={editCtl.nextActionBuf}
            onblur={commitNextAction}
            onkeydown={(e) => {
              if (e.key === 'Enter') commitNextAction();
              else if (e.key === 'Escape') {
                e.preventDefault();
                editCtl.cancelEditNextAction();
              }
            }}
            use:focusOnMount
            class="w-full px-3 py-2 bg-surface0 border border-primary rounded text-sm text-text outline-none"
          />
        {:else}
          <button
            onclick={editCtl.startEditNextAction}
            class="w-full text-left px-3 py-2.5 rounded text-sm border border-warning bg-surface0 text-warning hover:bg-surface1 {!project.next_action ? 'italic opacity-70' : 'font-medium'}"
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

      <!-- Notes linked to this project. Three matching signals
           (frontmatter project: field, [[Name]] wikilink, path under
           folder) are tried in order so an explicit link beats a
           drive-by mention. The two CTAs at the top let the user
           extend the link set: "+ Link existing" pops a search dialog
           that writes the project frontmatter back, "+ New note"
           drops a fresh note pre-filled and jumps to the editor. -->
      <ProjectNotesTab project={project} />

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
