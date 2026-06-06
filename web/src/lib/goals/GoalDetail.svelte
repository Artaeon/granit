<script lang="ts">
  import { api, fmtDateISO, type Goal, type Milestone } from '$lib/api';
  import { openAIOverlay } from '$lib/stores/ai-overlay';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import Drawer from '$lib/components/Drawer.svelte';
  import { inlineMd } from '$lib/util/inlineMd';
  import EntityDeadlines from '$lib/deadlines/EntityDeadlines.svelte';
  import { focusOnMount } from '$lib/util/focusOnMount';
  import { loadDraft, clearDraft, makeDraftWriter } from '$lib/util/draftAutosave';
  import {
    createGoalDetailAIMilestones,
    type MilestoneProposal
  } from './goalDetailAIMilestones.svelte';
  import {
    createGoalDetailAITasks,
    type TaskProposal
  } from './goalDetailAITasks.svelte';
  import { createGoalDetailVerses } from './goalDetailVerses.svelte';
  import { createGoalDetailTasksBurnup } from './goalDetailTasksBurnup.svelte';

  // Detail-and-edit drawer for a single goal. Mirrors ProjectDetail's
  // approach: every field commits via PATCH on blur / explicit toggle so
  // the user never sees a "save" dance for individual properties.
  // Milestones live inside the same drawer (add/edit/toggle/delete).
  let {
    open = $bindable(false),
    goal,
    onUpdated,
    onDeleted,
    onOpenDashboard
  }: {
    open?: boolean;
    goal: Goal | null;
    onUpdated: () => void | Promise<void>;
    onDeleted: (id: string) => void | Promise<void>;
    /** Optional — when supplied, the header shows a Dashboard button
     *  that delegates to the parent. The parent /goals page owns the
     *  dashboard URL state (?focus=X&dashboard=1) and renders the
     *  GoalDashboardPanel overlay, so this component stays unaware
     *  of how the dashboard mounts. Mirrors ProjectDetail. */
    onOpenDashboard?: () => void;
  } = $props();

  let saving = $state(false);
  let titleBuf = $state('');
  let editingTitle = $state(false);
  let descBuf = $state('');
  let editingDesc = $state(false);
  let notesBuf = $state('');
  let editingNotes = $state(false);

  // Milestones — local input buffers for the "add" form and per-row edits.
  let newMilestoneText = $state('');
  let newMilestoneDue = $state('');
  let editingMilestoneIdx = $state<number | null>(null);
  let editingMilestoneText = $state('');
  let editingMilestoneDue = $state('');

  // Reviews — buffer for "Log review".
  let reviewBuf = $state('');

  // Topical scripture verses for the goal — topic resolution + fetch
  // + carousel cursor live in goalDetailVerses. The parent reads
  // versesCtl.X (verses / verseCursor / verseTopic / verseLoading /
  // currentVerse) and calls versesCtl.next().
  const versesCtl = createGoalDetailVerses({ getGoal: () => goal });
  const verses = $derived(versesCtl.verses);
  const verseCursor = $derived(versesCtl.verseCursor);
  const verseTopic = $derived(versesCtl.verseTopic);
  const verseLoading = $derived(versesCtl.verseLoading);
  const nextVerse = versesCtl.next;

  // ── Linked tasks + burn-up ───────────────────────────────────────
  // Tasks carry a free goalId reference; we fetch all and filter
  // client-side. Same pattern ProjectDetail uses for project tasks.
  // Burn-up bucketed by ISO week so a "W19" tally on the goal lines
  // up with the dashboard TaskVelocityWidget and the project pages.
  // Linked tasks + 8-week burnup live in goalDetailTasksBurnup. Same
  // ISO-week scheme as the dashboard so a "W19" tally matches across
  // surfaces. Parent reads tasksCtl.X.
  const tasksCtl = createGoalDetailTasksBurnup({ getGoal: () => goal });
  const loadGoalTasks = tasksCtl.loadGoalTasks;
  // Last goal id we initialised buffers for. Parent swaps the goal
  // prop on list-click without unmounting, so we need to close any
  // open inline editors — titleBuf/descBuf/notesBuf still hold the
  // OLD goal's text otherwise and the draft-save $effects would
  // write that text under the NEW goal's draft key.
  let lastGoalId = '';
  $effect(() => {
    void goal?.id;
    if (goal) loadGoalTasks();
    if (goal && lastGoalId && lastGoalId !== goal.id) {
      editingTitle = false;
      editingDesc = false;
      editingNotes = false;
      titleDraftWriter.cancel();
      descDraftWriter.cancel();
      notesDraftWriter.cancel();
    }
    if (goal) lastGoalId = goal.id;
  });

  // Local $derived aliases for the template — writes go through
  // tasksCtl.loadGoalTasks().
  const goalTasks = $derived(tasksCtl.goalTasks);
  const burnup = $derived(tasksCtl.burnup);
  const burnupMax = $derived(tasksCtl.burnupMax);
  const burnupTotal = $derived(tasksCtl.burnupTotal);
  const openTaskCount = $derived(tasksCtl.openTaskCount);
  const doneTaskCount = $derived(tasksCtl.doneTaskCount);

  // ── AI-suggested milestones ──────────────────────────────────────
  // Fires /chat with the goal's context (title, description,
  // target_date, existing milestones) and asks for 3-5 milestone
  // suggestions in strict JSON. Renders proposals as accept/skip
  // chips inline in the Milestones section. Goes through the
  // chat audit gate so each suggestion is logged with token
  // counts in settings → AI features.
  // AI milestone suggester — state + stream + accept/skip live in
  // goalDetailAIMilestones. The parent reads aiMilestoneCtl.X.
  const aiMilestoneCtl = createGoalDetailAIMilestones({
    getGoal: () => goal,
    onUpdated
  });
  const suggestMilestones = aiMilestoneCtl.suggest;
  const cancelMilestoneAI = aiMilestoneCtl.cancel;
  const acceptMilestone = aiMilestoneCtl.accept;
  const skipMilestone = aiMilestoneCtl.skip;

  // ── AI-proposed THIS-WEEK tasks ─────────────────────────────────
  // Bridges the strategy/execution gap by asking the model for 3-5
  // concrete tasks the user could create THIS WEEK to advance the
  // goal. Each proposal is editable before accept — the user can
  // tweak the wording or due date inline. Accept calls api.createTask
  // with goalId linked so the task surfaces in the goal's roll-up
  // (the same path the existing manual flow uses on /tasks).
  //
  // Distinct from the milestone suggester above: milestones are the
  // structural breakdown of the goal ("Ship v1 to 5 beta users"),
  // tasks are the concrete next-7-days work ("Email 3 candidates
  // for beta", "Draft onboarding email", …). A goal with rich
  // milestones still needs this — the user knows the milestone
  // ladder but freezes on what to do Monday morning.
  // AI this-week-tasks suggester — state + stream + accept/skip/all
  // live in goalDetailAITasks. The parent reads aiTaskCtl.X.
  const aiTaskCtl = createGoalDetailAITasks({
    getGoal: () => goal,
    getOpenTaskCount: () => openTaskCount,
    getDoneTaskCount: () => doneTaskCount,
    reloadGoalTasks: () => loadGoalTasks(),
    onUpdated
  });
  const suggestTasks = aiTaskCtl.suggest;
  const cancelTaskAI = aiTaskCtl.cancel;
  const acceptTaskProposal = aiTaskCtl.accept;
  const skipTaskProposal = aiTaskCtl.skip;
  const acceptAllTaskProposals = aiTaskCtl.acceptAll;

  let reviewOpen = $state(false);

  const statusOptions: Goal['status'][] = ['active', 'paused', 'completed', 'archived'];
  const colorOptions = ['blue', 'green', 'mauve', 'peach', 'red', 'yellow', 'pink', 'lavender', 'teal', 'sapphire'];
  const categoryOptions = ['career', 'health', 'learning', 'relationships', 'finance', 'creative', 'spiritual', 'other'];

  async function patch(p: Partial<Goal>): Promise<boolean> {
    if (!goal) return false;
    saving = true;
    try {
      await api.patchGoal(goal.id, p);
      await onUpdated();
      return true;
    } catch (e) {
      toast.error('save failed: ' + (errorMessage(e)));
      return false;
    } finally {
      saving = false;
    }
  }

  // Draft autosave for the three inline-editable fields. Same pattern
  // as ProjectDetail — buffer-to-localStorage on change, restore on
  // re-entry to edit, clear on successful commit. Keyed per-goal so
  // switching goals in the drawer doesn't cross-contaminate. Notes
  // field is the longest-form of the three and the highest loss risk.
  const titleDraftWriter = makeDraftWriter(400);
  const descDraftWriter = makeDraftWriter(400);
  const notesDraftWriter = makeDraftWriter(400);
  function titleDraftKey() { return goal ? `goal.title.${goal.id}` : ''; }
  function descDraftKey() { return goal ? `goal.description.${goal.id}` : ''; }
  function notesDraftKey() { return goal ? `goal.notes.${goal.id}` : ''; }

  $effect(() => {
    if (editingTitle && titleDraftKey()) titleDraftWriter.save(titleDraftKey(), titleBuf);
  });
  $effect(() => {
    if (editingDesc && descDraftKey()) descDraftWriter.save(descDraftKey(), descBuf);
  });
  $effect(() => {
    if (editingNotes && notesDraftKey()) notesDraftWriter.save(notesDraftKey(), notesBuf);
  });

  async function commitTitle() {
    editingTitle = false;
    if (goal && titleBuf.trim() && titleBuf !== goal.title) await patch({ title: titleBuf.trim() });
    clearDraft(titleDraftKey());
    titleDraftWriter.cancel();
  }
  async function commitDesc() {
    editingDesc = false;
    if (goal && descBuf !== (goal.description ?? '')) await patch({ description: descBuf });
    clearDraft(descDraftKey());
    descDraftWriter.cancel();
  }
  async function commitNotes() {
    editingNotes = false;
    if (goal && notesBuf !== (goal.notes ?? '')) await patch({ notes: notesBuf });
    clearDraft(notesDraftKey());
    notesDraftWriter.cancel();
  }

  async function setStatus(s: Goal['status']) { await patch({ status: s }); }

  // Open the AI overlay pre-seeded with this goal's context: title +
  // status + target date + milestone progress + linked tasks. The
  // model is grounded enough to answer "draft my next milestone",
  // "what's blocking me on this?", "summarise my progress" without
  // the user having to restate the goal each time.
  function askAIAboutThisGoal(): void {
    if (!goal) return;
    const g = goal;
    const lines = [`I'm working on this goal:`, '', `- ${g.title}`];
    if (g.status) lines.push(`- status: ${g.status}`);
    if (g.target_date) lines.push(`- target date: ${g.target_date}`);
    if (g.category) lines.push(`- category: ${g.category}`);
    if (g.description && g.description.trim() !== '') {
      lines.push(`- description: ${g.description.trim()}`);
    }
    if (g.milestones && g.milestones.length > 0) {
      const done = g.milestones.filter((m) => m.done).length;
      lines.push(`- milestones: ${done}/${g.milestones.length} done`);
      const next = g.milestones.find((m) => !m.done);
      if (next) lines.push(`- next milestone: ${next.text}`);
    }
    const openTasks = goalTasks.filter((t) => !t.done);
    if (openTasks.length > 0) {
      lines.push(`- ${openTasks.length} open task${openTasks.length === 1 ? '' : 's'} linked`);
    }
    lines.push('', `What would help me move it forward?`);
    openAIOverlay({ text: lines.join('\n'), send: false });
  }
  async function setTargetDate(v: string) { await patch({ target_date: v }); }
  async function setCategory(v: string) { await patch({ category: v || undefined }); }
  async function setColor(v: string) { await patch({ color: v }); }
  async function setReviewFrequency(v: string) { await patch({ review_frequency: v || undefined }); }
  async function setProject(v: string) { await patch({ project: v.trim() || undefined }); }
  async function setVenture(v: string) { await patch({ venture: v.trim() || undefined }); }
  async function setTags(raw: string) {
    const tags = raw.split(',').map((t) => t.trim()).filter(Boolean);
    await patch({ tags });
  }

  async function addMilestone() {
    if (!goal || !newMilestoneText.trim()) return;
    saving = true;
    try {
      await api.addGoalMilestone(goal.id, {
        text: newMilestoneText.trim(),
        due_date: newMilestoneDue || undefined
      });
      newMilestoneText = '';
      newMilestoneDue = '';
      await onUpdated();
    } catch (e) {
      toast.error('add milestone failed: ' + (errorMessage(e)));
    } finally {
      saving = false;
    }
  }

  async function toggleMilestone(idx: number, m: Milestone) {
    if (!goal) return;
    try {
      await api.patchGoalMilestone(goal.id, idx, { done: !m.done });
      await onUpdated();
    } catch (e) {
      toast.error('toggle failed: ' + (errorMessage(e)));
    }
  }

  function startEditMilestone(idx: number, m: Milestone) {
    editingMilestoneIdx = idx;
    editingMilestoneText = m.text;
    editingMilestoneDue = m.due_date ?? '';
  }

  async function commitEditMilestone() {
    if (!goal || editingMilestoneIdx === null) return;
    const idx = editingMilestoneIdx;
    editingMilestoneIdx = null;
    try {
      await api.patchGoalMilestone(goal.id, idx, {
        text: editingMilestoneText,
        due_date: editingMilestoneDue
      });
      await onUpdated();
    } catch (e) {
      toast.error('edit failed: ' + (errorMessage(e)));
    }
  }

  async function removeMilestone(idx: number) {
    if (!goal) return;
    if (!confirm('Remove this milestone?')) return;
    try {
      await api.deleteGoalMilestone(goal.id, idx);
      await onUpdated();
    } catch (e) {
      toast.error('delete milestone failed: ' + (errorMessage(e)));
    }
  }

  async function logReview() {
    if (!goal || !reviewBuf.trim()) return;
    saving = true;
    try {
      await api.logGoalReview(goal.id, reviewBuf.trim());
      reviewBuf = '';
      reviewOpen = false;
      await onUpdated();
      toast.success('review logged');
    } catch (e) {
      toast.error('log review failed: ' + (errorMessage(e)));
    } finally {
      saving = false;
    }
  }

  async function deleteGoal() {
    if (!goal) return;
    if (!confirm(`Delete goal "${goal.title}"? This is irreversible.`)) return;
    try {
      await api.deleteGoal(goal.id);
      open = false;
      await onDeleted(goal.id);
    } catch (e) {
      toast.error('delete failed: ' + (errorMessage(e)));
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

  function statusTone(s?: string): string {
    if (s === 'active') return 'primary';
    if (s === 'paused') return 'subtext';
    if (s === 'completed') return 'success';
    if (s === 'archived') return 'dim';
    return 'subtext';
  }

  let progressPct = $derived.by(() => {
    if (!goal) return 0;
    const ms = goal.milestones ?? [];
    if (ms.length === 0) return goal.status === 'completed' ? 100 : 0;
    return Math.round((ms.filter((m) => m.done).length / ms.length) * 100);
  });
</script>

<Drawer bind:open side="right" responsive width="w-full sm:w-[32rem] md:w-[40rem]">
  {#if goal}
    <div class="flex flex-col h-full">
      <header class="px-3 py-2 border-b border-surface1 flex items-center gap-2 flex-shrink-0">
        <span class="w-3 h-3 rounded-full flex-shrink-0" style="background: {colorVar(goal.color)}"></span>
        {#if editingTitle}
          <input
            bind:value={titleBuf}
            onblur={commitTitle}
            onkeydown={(e) => { if (e.key === 'Enter') commitTitle(); else if (e.key === 'Escape') editingTitle = false; }}
            use:focusOnMount
            class="text-base font-semibold flex-1 px-1 -mx-1 bg-surface0 border border-primary rounded text-text outline-none"
          />
        {:else}
          <button
            onclick={() => {
              const draft = loadDraft<string | null>(titleDraftKey(), null);
              titleBuf = (draft && draft !== '') ? draft : goal.title;
              editingTitle = true;
            }}
            class="text-base font-semibold text-text flex-1 text-left truncate hover:text-primary"
            title="click to rename"
          >{goal.title}</button>
        {/if}
        <select
          value={goal.status ?? 'active'}
          onchange={(e) => setStatus((e.target as HTMLSelectElement).value as Goal['status'])}
          class="text-xs px-2 py-1 bg-surface0 border border-surface1 rounded hover:border-primary"
          style="color: var(--color-{statusTone(goal.status)})"
        >
          {#each statusOptions as s}<option value={s}>{s}</option>{/each}
        </select>
        <!-- Dashboard — opens the full-screen GoalDashboardPanel
             overlay above the goals layout. Lives behind a thin
             delegate so the detail drawer stays unaware of how the
             dashboard mounts; the parent /goals page owns the
             ?focus=X&dashboard=1 URL state and renders the overlay. -->
        <!-- Ask AI about this goal — opens the AIOverlay pre-seeded
             with title + status + target date + milestone progress
             + linked tasks. Model is grounded so the user can ask
             "draft my next milestone" or "summarise my progress"
             without re-stating context. send=false so the user can
             edit the prompt before submitting. -->
        <button
          onclick={askAIAboutThisGoal}
          title="ask AI about this goal"
          aria-label="ask ai about this goal"
          class="px-2.5 py-1.5 min-h-[36px] text-xs rounded border border-surface1 bg-surface0 text-subtext hover:border-primary hover:text-primary inline-flex items-center gap-1"
        >
          <span aria-hidden="true">✨</span>
          <span class="hidden sm:inline">Ask AI</span>
        </button>
        {#if onOpenDashboard}
          <button
            onclick={onOpenDashboard}
            title="open goal dashboard — visual operating picture"
            aria-label="open goal dashboard"
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
        <a
          href={`/prayer?goal=${encodeURIComponent(goal.id)}&text=${encodeURIComponent('For: ' + goal.title)}`}
          title="add a prayer intention for this goal"
          aria-label="pray for this goal"
          class="w-9 h-9 flex items-center justify-center text-dim hover:text-primary rounded text-base"
        >🙏</a>
        <button
          onclick={deleteGoal}
          aria-label="delete"
          title="delete goal"
          class="w-9 h-9 flex items-center justify-center text-dim hover:text-error rounded"
        >
          <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
            <path d="M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/>
          </svg>
        </button>
        <button onclick={() => (open = false)} aria-label="close" class="text-dim hover:text-text px-2">×</button>
      </header>

      <div class="flex-1 overflow-y-auto p-2 sm:p-4 space-y-4">
        {#if goal.project || goal.venture}
          <!-- Cross-link chips. The goal carries free-text references
               to a project name and / or venture; the chips make
               those clickable so the user can hop to the project
               page (`/projects?p=<name>`) or venture page directly,
               without having to scroll down to the form fields and
               copy-paste the name. Bidirectional surface: the
               project page already lists "linked goals", this is
               the goal-side mirror. -->
          <div class="flex items-center gap-1.5 flex-wrap text-[11px]">
            {#if goal.project}
              <a
                href="/projects?p={encodeURIComponent(goal.project)}"
                class="px-2 py-0.5 rounded-full bg-surface1 text-secondary border border-surface2 hover:border-surface2 hover:bg-surface2 transition-colors"
                title="Open the linked project"
              >📁 {goal.project}</a>
            {/if}
            {#if goal.venture}
              <a
                href="/ventures/{encodeURIComponent(goal.venture)}"
                class="px-2 py-0.5 rounded-full bg-surface1 text-primary border border-surface2 hover:border-primary hover:bg-surface2 transition-colors"
                title="Open the linked venture"
              >🏢 {goal.venture}</a>
            {/if}
          </div>
        {/if}

        <!-- Progress -->
        <section>
          <div class="flex items-baseline justify-between mb-1.5">
            <h3 class="text-xs uppercase tracking-wider text-dim font-medium">Progress</h3>
            <span class="text-xs text-subtext font-mono">{progressPct}%</span>
          </div>
          <div class="h-2 rounded-full bg-surface0 overflow-hidden">
            <div class="h-full transition-all" style="width: {progressPct}%; background: {colorVar(goal.color)}"></div>
          </div>

          {#if goalTasks.length > 0}
            <!-- Linked-task counts + 8-week burn-up. Only goals
                 whose tasks carry an explicit goalId reference show
                 up here — the milestone-based progress bar above
                 covers the milestone path. The two views complement:
                 milestones tell you "how much of the plan is done",
                 burn-up tells you "are we still moving". -->
            <div class="mt-3 flex items-baseline gap-2 text-[11px] text-dim">
              <span class="font-mono">{openTaskCount} open · {doneTaskCount} done</span>
              <span class="flex-1"></span>
              {#if burnupTotal > 0}
                <span class="font-mono">{burnupTotal} done in 8w</span>
              {/if}
            </div>
            {#if burnupTotal > 0}
              <div class="mt-1.5">
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
          {/if}
        </section>

        <!-- Description -->
        <section>
          <h3 class="text-xs uppercase tracking-wider text-dim font-medium mb-1.5">Description</h3>
          {#if editingDesc}
            <textarea
              bind:value={descBuf}
              onblur={commitDesc}
              onkeydown={(e) => { if (e.key === 'Escape') editingDesc = false; }}
              use:focusOnMount
              rows="3"
              class="w-full px-3 py-2 bg-surface0 border border-primary rounded text-sm text-text outline-none"
            ></textarea>
          {:else}
            <button
              onclick={() => {
                const draft = loadDraft<string | null>(descDraftKey(), null);
                descBuf = (draft && draft !== '') ? draft : (goal.description ?? '');
                editingDesc = true;
              }}
              class="w-full text-left px-3 py-2 text-sm rounded hover:bg-surface0 {goal.description ? 'text-text' : 'text-dim italic'}"
            >{#if goal.description}{@html inlineMd(goal.description)}{:else}click to add a description…{/if}</button>
          {/if}
        </section>

        <!-- Milestones -->
        <section>
          <div class="flex items-baseline gap-2 mb-2">
            <h3 class="text-xs uppercase tracking-wider text-dim font-medium flex-1">Milestones</h3>
            {#if aiMilestoneCtl.busy}
              <button
                onclick={cancelMilestoneAI}
                class="text-[11px] text-warning hover:underline"
              >cancel</button>
            {:else}
              <button
                onclick={() => void suggestMilestones()}
                class="text-[11px] px-2 py-0.5 rounded bg-surface0 border border-surface1 text-subtext hover:border-primary"
                title="Ask the AI to suggest 3-5 milestones for this goal"
              >✨ suggest</button>
            {/if}
          </div>

          {#if aiMilestoneCtl.error}
            <div class="mb-2 px-3 py-2 text-xs text-error border border-error bg-surface0 rounded">
              {aiMilestoneCtl.error}
            </div>
          {/if}
          {#if aiMilestoneCtl.proposals.length > 0}
            <!-- AI proposals panel. Each row is accept / skip — accept
                 calls api.addGoalMilestone with the proposed text + due
                 date. Skip just dismisses. Same shape as the inbox-
                 triage proposals on /tasks so the muscle memory carries
                 over. -->
            <div class="mb-3 px-3 py-2 bg-surface1 border border-surface2 rounded">
              <div class="text-[10px] uppercase tracking-wider text-secondary font-semibold mb-1.5">AI suggestions ({aiMilestoneCtl.proposals.length})</div>
              <ul class="space-y-1.5">
                {#each aiMilestoneCtl.proposals as p (p.text)}
                  <li class="flex items-start gap-2 text-xs">
                    <div class="flex-1 min-w-0">
                      <div class="text-text">{p.text}</div>
                      {#if p.due_date}
                        <div class="text-dim text-[10px] font-mono mt-0.5">due {p.due_date}</div>
                      {/if}
                    </div>
                    <button
                      onclick={() => void acceptMilestone(p)}
                      class="px-2 py-0.5 bg-surface0 text-success rounded hover:bg-surface1"
                    >accept</button>
                    <button
                      onclick={() => skipMilestone(p)}
                      class="px-2 py-0.5 text-dim hover:text-text"
                    >skip</button>
                  </li>
                {/each}
              </ul>
            </div>
          {/if}

          <ul class="space-y-1.5 mb-3">
            {#each goal.milestones ?? [] as m, i (i)}
              <li class="flex items-start gap-2 text-sm group">
                <button
                  onclick={() => toggleMilestone(i, m)}
                  aria-label={m.done ? 'mark incomplete' : 'mark complete'}
                  class="w-4 h-4 rounded border flex-shrink-0 flex items-center justify-center mt-1 {m.done ? 'bg-success border-success' : 'border-surface2 hover:border-primary'}"
                >
                  {#if m.done}
                    <svg viewBox="0 0 12 12" class="w-3 h-3 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
                  {/if}
                </button>
                <div class="flex-1 min-w-0">
                  {#if editingMilestoneIdx === i}
                    <div class="space-y-1">
                      <input
                        bind:value={editingMilestoneText}
                        class="w-full px-2 py-1 bg-surface0 border border-primary rounded text-sm text-text outline-none"
                      />
                      <input
                        bind:value={editingMilestoneDue}
                        type="date"
                        class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-xs text-text"
                      />
                      <div class="flex gap-2">
                        <button
                          onclick={commitEditMilestone}
                          class="px-2 py-0.5 text-xs bg-primary text-on-primary rounded"
                        >save</button>
                        <button
                          onclick={() => (editingMilestoneIdx = null)}
                          class="px-2 py-0.5 text-xs bg-surface1 text-subtext rounded"
                        >cancel</button>
                      </div>
                    </div>
                  {:else}
                    <button
                      onclick={() => startEditMilestone(i, m)}
                      class="block w-full text-left {m.done ? 'line-through text-dim' : 'text-text'} hover:text-primary"
                    >{m.text}</button>
                    <div class="flex flex-wrap items-center gap-x-3 text-[11px] text-dim">
                      {#if m.due_date}<span>due {m.due_date}</span>{/if}
                      {#if m.completed_at}<span>done {m.completed_at.slice(0, 10)}</span>{/if}
                    </div>
                  {/if}
                </div>
                {#if editingMilestoneIdx !== i}
                  <button
                    onclick={() => removeMilestone(i)}
                    aria-label="remove milestone"
                    class="opacity-0 group-hover:opacity-100 text-dim hover:text-error text-xs transition-opacity"
                  >×</button>
                {/if}
              </li>
            {/each}
          </ul>
          <div class="flex gap-2">
            <input
              bind:value={newMilestoneText}
              onkeydown={(e) => { if (e.key === 'Enter') addMilestone(); }}
              placeholder="new milestone…"
              class="flex-1 px-2 py-1.5 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
            />
            <input
              bind:value={newMilestoneDue}
              type="date"
              class="px-2 py-1.5 bg-surface0 border border-surface1 rounded text-sm text-text"
            />
            <button
              onclick={addMilestone}
              disabled={!newMilestoneText.trim() || saving}
              class="px-3 py-1.5 bg-primary text-on-primary rounded text-sm disabled:opacity-50"
            >+ add</button>
          </div>
        </section>

        <!-- Deadlines linked to this goal — same component the project
             panel uses, so the visual language matches. -->
        {#if goal}
          <EntityDeadlines scope={{ kind: 'goal', id: goal.id, title: goal.title }} />
        {/if}

        <!-- AI "this week's tasks" pipeline. Distinct from the
             milestone suggester above — milestones are the structural
             ladder, these are the concrete next-7-days work that
             closes a milestone. Each proposal is editable in place
             (text + due date) before accept; accepted ones are
             created via api.createTask with goalId linked so they
             show up in the open / done counts and the burn-up. -->
        <section>
          <div class="flex items-baseline gap-2 mb-2">
            <h3 class="text-xs uppercase tracking-wider text-dim font-medium flex-1">This week's tasks</h3>
            {#if aiTaskCtl.busy}
              <button
                onclick={cancelTaskAI}
                class="text-[11px] text-warning hover:underline"
              >cancel</button>
            {:else}
              <button
                onclick={() => void suggestTasks()}
                class="text-[11px] px-2 py-0.5 rounded bg-surface0 border border-surface1 text-subtext hover:border-primary"
                title="Ask the AI for 3-5 concrete tasks to advance this goal this week"
              >✨ suggest tasks</button>
            {/if}
          </div>

          {#if aiTaskCtl.error}
            <div class="mb-2 px-3 py-2 text-xs text-error border border-error bg-surface0 rounded">
              {aiTaskCtl.error}
            </div>
          {/if}

          {#if aiTaskCtl.busy && aiTaskCtl.proposals.length === 0}
            <div class="text-xs text-dim italic px-3 py-2 flex items-center gap-2">
              <span class="inline-block w-1.5 h-3 bg-primary/60 animate-pulse rounded-sm"></span>
              proposing 3-5 tasks for this week…
            </div>
          {/if}

          {#if aiTaskCtl.proposals.length > 0}
            <div class="px-3 py-2 bg-surface1 border border-surface2 rounded">
              <div class="flex items-baseline gap-2 mb-2">
                <span class="text-[10px] uppercase tracking-wider text-primary font-semibold">AI proposals ({aiTaskCtl.proposals.length})</span>
                <span class="flex-1"></span>
                <button
                  onclick={() => void acceptAllTaskProposals()}
                  class="text-[11px] text-primary hover:underline"
                  title="Create all proposals as tasks linked to this goal"
                >+ accept all</button>
              </div>
              <ul class="space-y-1.5">
                {#each aiTaskCtl.proposals as p (p.text)}
                  <li class="flex items-start gap-2 text-xs">
                    <div class="flex-1 min-w-0 space-y-1">
                      {#if p.edit}
                        <input
                          bind:value={p.text}
                          class="w-full px-2 py-1 bg-surface0 border border-primary rounded text-xs text-text outline-none"
                        />
                        <input
                          type="date"
                          value={p.dueDate ?? ''}
                          onchange={(e) => (p.dueDate = (e.target as HTMLInputElement).value || undefined)}
                          class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-xs text-text"
                        />
                      {:else}
                        <div class="text-text break-words">{p.text}</div>
                        {#if p.dueDate}
                          <div class="text-dim text-[10px] font-mono">due {p.dueDate}</div>
                        {/if}
                      {/if}
                    </div>
                    {#if p.edit}
                      <button
                        onclick={() => (p.edit = false)}
                        class="px-2 py-0.5 bg-surface1 text-subtext rounded hover:bg-surface2"
                      >done</button>
                    {:else}
                      <button
                        onclick={() => (p.edit = true)}
                        class="px-2 py-0.5 text-dim hover:text-text"
                        title="edit before accepting"
                      >edit</button>
                    {/if}
                    <button
                      onclick={() => void acceptTaskProposal(p)}
                      class="px-2 py-0.5 bg-surface0 text-success rounded hover:bg-surface1"
                      title="Create as a task linked to this goal"
                    >accept</button>
                    <button
                      onclick={() => skipTaskProposal(p)}
                      class="px-2 py-0.5 text-dim hover:text-text"
                    >skip</button>
                  </li>
                {/each}
              </ul>
            </div>
          {/if}
        </section>

        <!-- Reviews -->
        <section>
          <div class="flex items-baseline justify-between mb-2">
            <h3 class="text-xs uppercase tracking-wider text-dim font-medium">Review log</h3>
            <button
              onclick={() => (reviewOpen = !reviewOpen)}
              class="text-xs text-secondary hover:underline"
            >{reviewOpen ? 'cancel' : '+ log review'}</button>
          </div>
          {#if reviewOpen}
            <div class="mb-3 space-y-2">
              <textarea
                bind:value={reviewBuf}
                rows="3"
                placeholder="how is this goal going? what's blocked? what's next?"
                class="w-full px-3 py-2 bg-surface0 border border-primary rounded text-sm text-text outline-none"
              ></textarea>
              <button
                onclick={logReview}
                disabled={!reviewBuf.trim() || saving}
                class="px-3 py-1.5 bg-primary text-on-primary rounded text-sm disabled:opacity-50"
              >{saving ? 'saving…' : 'log review'}</button>
            </div>
          {/if}
          {#if (goal.review_log ?? []).length === 0}
            <p class="text-xs text-dim italic">no reviews logged yet.</p>
          {:else}
            <ul class="space-y-2">
              {#each [...(goal.review_log ?? [])].reverse() as r}
                <li class="px-3 py-2 bg-surface0 rounded text-sm">
                  <div class="flex items-baseline justify-between mb-1">
                    <span class="text-xs text-subtext font-mono">{r.date}</span>
                    <span class="text-[11px] text-dim">{r.progress}%</span>
                  </div>
                  <p class="text-text whitespace-pre-wrap">{r.note}</p>
                </li>
              {/each}
            </ul>
          {/if}
        </section>

        <!-- Verse for this goal — silently hidden when no topic resolves
             (no category mapping, no matching tag). Single line of dense
             chrome; the "next" walk-through reuses the same per-topic
             cursor pattern VerseForMoodWidget uses on the dashboard. -->
        {#if verses.length > 0}
          {@const v = verses[verseCursor]}
          <section>
            <div class="flex items-baseline justify-between mb-1.5">
              <h3 class="text-xs uppercase tracking-wider text-dim font-medium">
                Verse for this goal{#if verseTopic}<span class="opacity-70"> · {verseTopic}</span>{/if}
              </h3>
              {#if verses.length > 1}
                <div class="flex items-center gap-2 text-[11px]">
                  <span class="text-dim">{verseCursor + 1}/{verses.length}</span>
                  <button onclick={nextVerse} class="text-secondary hover:underline">next ↻</button>
                </div>
              {/if}
            </div>
            <blockquote class="text-sm text-text leading-relaxed font-serif italic px-3 py-2 border-l-2 border-secondary/40 bg-surface0/50 rounded-r">
              "{v.text}"
              {#if v.source}
                <cite class="block mt-1 text-xs text-subtext not-italic">— {v.source}</cite>
              {/if}
            </blockquote>
          </section>
        {:else if verseLoading}
          <section>
            <p class="text-xs text-dim italic">Loading verse…</p>
          </section>
        {/if}

        <!-- Notes -->
        <section>
          <h3 class="text-xs uppercase tracking-wider text-dim font-medium mb-1.5">Notes</h3>
          {#if editingNotes}
            <textarea
              bind:value={notesBuf}
              onblur={commitNotes}
              onkeydown={(e) => { if (e.key === 'Escape') editingNotes = false; }}
              use:focusOnMount
              rows="4"
              class="w-full px-3 py-2 bg-surface0 border border-primary rounded text-sm text-text outline-none"
            ></textarea>
          {:else}
            <button
              onclick={() => {
                const draft = loadDraft<string | null>(notesDraftKey(), null);
                notesBuf = (draft && draft !== '') ? draft : (goal.notes ?? '');
                editingNotes = true;
              }}
              class="w-full text-left px-3 py-2 text-sm rounded hover:bg-surface0 whitespace-pre-wrap {goal.notes ? 'text-text' : 'text-dim italic'}"
            >{goal.notes || 'click to add notes…'}</button>
          {/if}
        </section>

        <!-- Metadata grid -->
        <section class="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-3 pt-4 border-t border-surface1">
          <div>
            <label for="g-target" class="text-[11px] uppercase tracking-wider text-dim block mb-1">Target date</label>
            <input
              id="g-target"
              type="date"
              value={goal.target_date ?? ''}
              onchange={(e) => setTargetDate((e.target as HTMLInputElement).value)}
              class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text"
            />
          </div>
          <div>
            <label for="g-cat" class="text-[11px] uppercase tracking-wider text-dim block mb-1">Category</label>
            <select
              id="g-cat"
              value={goal.category ?? ''}
              onchange={(e) => setCategory((e.target as HTMLSelectElement).value)}
              class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text"
            >
              <option value="">—</option>
              {#each categoryOptions as c}<option value={c}>{c}</option>{/each}
            </select>
          </div>
          <div>
            <label for="g-rev" class="text-[11px] uppercase tracking-wider text-dim block mb-1">Review frequency</label>
            <select
              id="g-rev"
              value={goal.review_frequency ?? ''}
              onchange={(e) => setReviewFrequency((e.target as HTMLSelectElement).value)}
              class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text"
            >
              <option value="">— none —</option>
              <option value="weekly">weekly</option>
              <option value="monthly">monthly</option>
              <option value="quarterly">quarterly</option>
            </select>
          </div>
          <div>
            <label for="g-proj" class="text-[11px] uppercase tracking-wider text-dim block mb-1">Project</label>
            <input
              id="g-proj"
              value={goal.project ?? ''}
              onblur={(e) => setProject((e.target as HTMLInputElement).value)}
              placeholder="link to a project name"
              class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text"
            />
          </div>
          <div>
            <label for="g-venture" class="text-[11px] uppercase tracking-wider text-dim block mb-1">Venture / Company</label>
            <input
              id="g-venture"
              value={goal.venture ?? ''}
              onblur={(e) => setVenture((e.target as HTMLInputElement).value)}
              placeholder="e.g. Stoicera"
              class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text"
            />
          </div>
          <div class="sm:col-span-2">
            <label for="g-tags" class="text-[11px] uppercase tracking-wider text-dim block mb-1">Tags</label>
            <input
              id="g-tags"
              value={(goal.tags ?? []).join(', ')}
              onblur={(e) => setTags((e.target as HTMLInputElement).value)}
              placeholder="comma, separated"
              class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text"
            />
          </div>
          <div class="sm:col-span-2">
            <span class="text-[11px] uppercase tracking-wider text-dim block mb-1">Color</span>
            <div class="flex gap-1.5 flex-wrap">
              {#each colorOptions as c}
                <button
                  onclick={() => setColor(c)}
                  aria-label="color {c}"
                  class="w-6 h-6 rounded-full border-2 {goal.color === c ? 'border-text' : 'border-surface1'}"
                  style="background: {colorVar(c)}"
                ></button>
              {/each}
            </div>
          </div>
        </section>

        <footer class="text-[11px] text-dim pt-2 border-t border-surface1 flex justify-between">
          <span>created {(goal.created_at ?? '').slice(0, 10) || '—'}</span>
          <span>updated {(goal.updated_at ?? '').slice(0, 10) || '—'}</span>
        </footer>
      </div>
    </div>
  {/if}
</Drawer>
