<script lang="ts">
  import { api, type Goal, type Milestone, type Task } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import Drawer from '$lib/components/Drawer.svelte';
  import { inlineMd } from '$lib/util/inlineMd';
  import EntityDeadlines from '$lib/deadlines/EntityDeadlines.svelte';

  // Detail-and-edit drawer for a single goal. Mirrors ProjectDetail's
  // approach: every field commits via PATCH on blur / explicit toggle so
  // the user never sees a "save" dance for individual properties.
  // Milestones live inside the same drawer (add/edit/toggle/delete).
  let {
    open = $bindable(false),
    goal,
    onUpdated,
    onDeleted
  }: {
    open?: boolean;
    goal: Goal | null;
    onUpdated: () => void | Promise<void>;
    onDeleted: (id: string) => void | Promise<void>;
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

  // ── Linked tasks + burn-up ───────────────────────────────────────
  // Tasks carry a free goalId reference; we fetch all and filter
  // client-side. Same pattern ProjectDetail uses for project tasks.
  // Burn-up bucketed by ISO week so a "W19" tally on the goal lines
  // up with the dashboard TaskVelocityWidget and the project pages.
  let goalTasks = $state<Task[]>([]);
  async function loadGoalTasks() {
    if (!goal) return;
    try {
      const r = await api.listTasks({});
      goalTasks = r.tasks.filter((t) => t.goalId === goal!.id);
    } catch {
      goalTasks = [];
    }
  }
  $effect(() => {
    void goal?.id;
    if (goal) loadGoalTasks();
  });

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
    for (const t of goalTasks) {
      if (!t.done || !t.completedAt) continue;
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
  const openTaskCount = $derived(goalTasks.filter((t) => !t.done).length);
  const doneTaskCount = $derived(goalTasks.filter((t) => t.done).length);

  // ── AI-suggested milestones ──────────────────────────────────────
  // Fires /chat with the goal's context (title, description,
  // target_date, existing milestones) and asks for 3-5 milestone
  // suggestions in strict JSON. Renders proposals as accept/skip
  // chips inline in the Milestones section. Goes through the
  // chat audit gate so each suggestion is logged with token
  // counts in settings → AI features.
  interface MilestoneProposal { text: string; due_date?: string }
  let aiMilestoneBusy = $state(false);
  let aiMilestoneProposals = $state<MilestoneProposal[]>([]);
  let aiMilestoneError = $state('');
  let aiMilestoneAbort: AbortController | null = null;

  async function suggestMilestones() {
    if (!goal || aiMilestoneBusy) return;
    aiMilestoneBusy = true;
    aiMilestoneError = '';
    aiMilestoneProposals = [];
    aiMilestoneAbort = new AbortController();
    const existing = (goal.milestones ?? []).map((m) => `- ${m.text}${m.due_date ? ` (due ${m.due_date})` : ''}`).join('\n');
    const ctx = [
      `Goal: ${goal.title}`,
      goal.description ? `Description: ${goal.description}` : '',
      goal.target_date ? `Target date: ${goal.target_date}` : '',
      existing ? `Existing milestones:\n${existing}` : 'No milestones yet.'
    ].filter(Boolean).join('\n\n');
    const userMessage =
      'Suggest 3-5 concrete milestones to break this goal down into trackable, finishable steps. ' +
      'Each milestone should be specific enough that the user knows when it\'s done. ' +
      'Distribute due dates evenly between today and the target date when one is given; otherwise omit due_date.\n\n' +
      'Return STRICT JSON ONLY (no markdown fences, no preamble), shape:\n' +
      '[{"text": "...", "due_date": "YYYY-MM-DD"}, ...]\n\n' +
      'Goal context:\n\n' + ctx;
    let acc = '';
    try {
      await api.chatStream(
        [{ role: 'user', content: userMessage }],
        undefined,
        {
          onChunk: (c) => { acc += c; },
          onError: (err) => { aiMilestoneError = err.message; }
        },
        aiMilestoneAbort.signal
      );
      // Strip optional code fences and parse.
      let cleaned = acc.trim();
      if (cleaned.startsWith('```')) {
        cleaned = cleaned.replace(/^```(?:json)?\s*/, '').replace(/```\s*$/, '').trim();
      }
      const parsed = JSON.parse(cleaned);
      if (!Array.isArray(parsed)) throw new Error('expected array');
      aiMilestoneProposals = parsed
        .filter((p: unknown) => p && typeof p === 'object' && typeof (p as MilestoneProposal).text === 'string')
        .map((p) => ({
          text: (p as MilestoneProposal).text.trim(),
          due_date: typeof (p as MilestoneProposal).due_date === 'string' ? (p as MilestoneProposal).due_date : undefined
        }))
        .slice(0, 5);
    } catch (err) {
      if (!aiMilestoneError) {
        const msg = err instanceof Error ? err.message : String(err);
        aiMilestoneError = `Couldn't parse suggestions: ${msg}`;
      }
    } finally {
      aiMilestoneBusy = false;
      aiMilestoneAbort = null;
    }
  }
  function cancelMilestoneAI() { aiMilestoneAbort?.abort(); }
  async function acceptMilestone(p: MilestoneProposal) {
    if (!goal) return;
    try {
      await api.addGoalMilestone(goal.id, { text: p.text, due_date: p.due_date });
      aiMilestoneProposals = aiMilestoneProposals.filter((x) => x !== p);
      await onUpdated();
    } catch (e) {
      toast.error('add failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }
  function skipMilestone(p: MilestoneProposal) {
    aiMilestoneProposals = aiMilestoneProposals.filter((x) => x !== p);
  }

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
  interface TaskProposal {
    text: string;
    dueDate?: string;
    edit: boolean;        // local UI state — proposal is in edit mode
  }
  let aiTaskBusy = $state(false);
  let aiTaskProposals = $state<TaskProposal[]>([]);
  let aiTaskError = $state('');
  let aiTaskAbort: AbortController | null = null;

  function todayPlusDays(n: number): string {
    const d = new Date();
    d.setDate(d.getDate() + n);
    return d.toISOString().slice(0, 10);
  }

  async function suggestTasks() {
    if (!goal || aiTaskBusy) return;
    aiTaskBusy = true;
    aiTaskError = '';
    aiTaskProposals = [];
    aiTaskAbort = new AbortController();

    const ms = goal.milestones ?? [];
    const openMs = ms.filter((m) => !m.done).slice(0, 8).map((m) => m.text);
    const doneMs = ms.filter((m) => m.done).slice(-4).map((m) => m.text);
    const today = todayPlusDays(0);
    const friday = (() => {
      // Coming Friday — falls back to today+7 if it's already weekend.
      const d = new Date();
      const dow = d.getDay(); // 0 Sun … 5 Fri
      const delta = dow <= 5 ? 5 - dow : 7 - dow + 5;
      d.setDate(d.getDate() + (delta === 0 ? 5 : delta));
      return d.toISOString().slice(0, 10);
    })();

    const ctx = [
      `Goal: ${goal.title}`,
      goal.description ? `Description: ${goal.description}` : '',
      goal.target_date ? `Target date: ${goal.target_date}` : '',
      goal.venture ? `Venture: ${goal.venture}` : '',
      goal.project ? `Project: ${goal.project}` : '',
      openMs.length > 0 ? `Open milestones:\n${openMs.map((m) => `- ${m}`).join('\n')}` : '',
      doneMs.length > 0 ? `Recent done milestones:\n${doneMs.map((m) => `- ${m}`).join('\n')}` : '',
      `Linked tasks: ${openTaskCount} open, ${doneTaskCount} done`
    ].filter(Boolean).join('\n\n');

    const userMessage =
      'You are a founder-coach. The user has a goal and wants 3-5 concrete TASKS they could DO THIS WEEK to advance it.\n\n' +
      'Rules for each task:\n' +
      '- Action-oriented, starts with a verb (Email, Draft, Call, Outline, Ship, Interview, Sketch, …).\n' +
      '- Doable in one sitting (≤ 2h). NOT a milestone, NOT a vague intention.\n' +
      '- Specific enough that the user knows when it\'s done.\n' +
      '- Distinct from open milestones above — these are the WORK that closes a milestone, not a restatement of one.\n' +
      '- One line, ≤ 14 words. No quotes, no period, no bullet.\n' +
      '- Set due_date to a specific weekday this week (today is ' + today + ', this Friday is ' + friday + '). Distribute due dates so they don\'t all land on the same day. Format YYYY-MM-DD.\n\n' +
      'Return STRICT JSON ONLY (no markdown fences, no preamble), shape:\n' +
      '[{"text": "...", "due_date": "YYYY-MM-DD"}, ...]\n\n' +
      'Goal context:\n\n' + ctx;

    let acc = '';
    try {
      await api.chatStream(
        [{ role: 'user', content: userMessage }],
        undefined,
        {
          onChunk: (c) => { acc += c; },
          onError: (err) => { aiTaskError = err.message; }
        },
        aiTaskAbort.signal
      );
      let cleaned = acc.trim();
      if (cleaned.startsWith('```')) {
        cleaned = cleaned.replace(/^```(?:json)?\s*/, '').replace(/```\s*$/, '').trim();
      }
      const parsed = JSON.parse(cleaned);
      if (!Array.isArray(parsed)) throw new Error('expected array');
      aiTaskProposals = parsed
        .filter((p: unknown) => p && typeof p === 'object' && typeof (p as { text?: unknown }).text === 'string')
        .map((p) => {
          const obj = p as { text: string; due_date?: unknown };
          return {
            text: obj.text.trim(),
            dueDate: typeof obj.due_date === 'string' && obj.due_date ? obj.due_date : undefined,
            edit: false
          } satisfies TaskProposal;
        })
        .slice(0, 5);
      if (aiTaskProposals.length === 0 && !aiTaskError) {
        aiTaskError = 'AI returned no task proposals.';
      }
    } catch (err) {
      if (!aiTaskError) {
        const msg = err instanceof Error ? err.message : String(err);
        aiTaskError = `Couldn't parse tasks: ${msg}`;
      }
    } finally {
      aiTaskBusy = false;
      aiTaskAbort = null;
    }
  }

  function cancelTaskAI() { aiTaskAbort?.abort(); }

  async function acceptTaskProposal(p: TaskProposal) {
    if (!goal || !p.text.trim()) return;
    try {
      // Created in today's daily note, same convention /tasks
      // quick-add uses. The goalId link is what makes it show up
      // in this goal's roll-up + burn-up next time we load.
      const daily = await api.daily('today');
      await api.createTask({
        notePath: daily.path,
        text: p.text.trim(),
        dueDate: p.dueDate || undefined,
        goalId: goal.id
      });
      aiTaskProposals = aiTaskProposals.filter((x) => x !== p);
      await loadGoalTasks();
      await onUpdated();
      toast.success('task added');
    } catch (e) {
      toast.error('add task failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }
  function skipTaskProposal(p: TaskProposal) {
    aiTaskProposals = aiTaskProposals.filter((x) => x !== p);
  }
  async function acceptAllTaskProposals() {
    const list = [...aiTaskProposals];
    for (const p of list) await acceptTaskProposal(p);
  }

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
      toast.error('save failed: ' + (e instanceof Error ? e.message : String(e)));
      return false;
    } finally {
      saving = false;
    }
  }

  async function commitTitle() {
    editingTitle = false;
    if (goal && titleBuf.trim() && titleBuf !== goal.title) await patch({ title: titleBuf.trim() });
  }
  async function commitDesc() {
    editingDesc = false;
    if (goal && descBuf !== (goal.description ?? '')) await patch({ description: descBuf });
  }
  async function commitNotes() {
    editingNotes = false;
    if (goal && notesBuf !== (goal.notes ?? '')) await patch({ notes: notesBuf });
  }

  async function setStatus(s: Goal['status']) { await patch({ status: s }); }
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
      toast.error('add milestone failed: ' + (e instanceof Error ? e.message : String(e)));
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
      toast.error('toggle failed: ' + (e instanceof Error ? e.message : String(e)));
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
      toast.error('edit failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function removeMilestone(idx: number) {
    if (!goal) return;
    if (!confirm('Remove this milestone?')) return;
    try {
      await api.deleteGoalMilestone(goal.id, idx);
      await onUpdated();
    } catch (e) {
      toast.error('delete milestone failed: ' + (e instanceof Error ? e.message : String(e)));
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
      toast.error('log review failed: ' + (e instanceof Error ? e.message : String(e)));
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
      <header class="px-4 py-3 border-b border-surface1 flex items-center gap-2 flex-shrink-0">
        <span class="w-3 h-3 rounded-full flex-shrink-0" style="background: {colorVar(goal.color)}"></span>
        {#if editingTitle}
          <input
            bind:value={titleBuf}
            onblur={commitTitle}
            onkeydown={(e) => { if (e.key === 'Enter') commitTitle(); else if (e.key === 'Escape') editingTitle = false; }}
            autofocus
            class="text-base font-semibold flex-1 px-1 -mx-1 bg-surface0 border border-primary rounded text-text outline-none"
          />
        {:else}
          <button
            onclick={() => { titleBuf = goal.title; editingTitle = true; }}
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

      <div class="flex-1 overflow-y-auto p-4 sm:p-6 space-y-6">
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
                class="px-2 py-0.5 rounded-full bg-secondary/10 text-secondary border border-secondary/20 hover:border-secondary/40 hover:bg-secondary/15 transition-colors"
                title="Open the linked project"
              >📁 {goal.project}</a>
            {/if}
            {#if goal.venture}
              <a
                href="/ventures/{encodeURIComponent(goal.venture)}"
                class="px-2 py-0.5 rounded-full bg-primary/10 text-primary border border-primary/20 hover:border-primary/40 hover:bg-primary/15 transition-colors"
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
                        class="w-full rounded-t {b.isThisWeek ? 'bg-primary' : 'bg-secondary/40'} transition-all"
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
              autofocus
              rows="3"
              class="w-full px-3 py-2 bg-surface0 border border-primary rounded text-sm text-text outline-none"
            ></textarea>
          {:else}
            <button
              onclick={() => { descBuf = goal.description ?? ''; editingDesc = true; }}
              class="w-full text-left px-3 py-2 text-sm rounded hover:bg-surface0 {goal.description ? 'text-text' : 'text-dim italic'}"
            >{#if goal.description}{@html inlineMd(goal.description)}{:else}click to add a description…{/if}</button>
          {/if}
        </section>

        <!-- Milestones -->
        <section>
          <div class="flex items-baseline gap-2 mb-2">
            <h3 class="text-xs uppercase tracking-wider text-dim font-medium flex-1">Milestones</h3>
            {#if aiMilestoneBusy}
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

          {#if aiMilestoneError}
            <div class="mb-2 px-3 py-2 text-xs text-error border border-error/30 bg-error/5 rounded">
              {aiMilestoneError}
            </div>
          {/if}
          {#if aiMilestoneProposals.length > 0}
            <!-- AI proposals panel. Each row is accept / skip — accept
                 calls api.addGoalMilestone with the proposed text + due
                 date. Skip just dismisses. Same shape as the inbox-
                 triage proposals on /tasks so the muscle memory carries
                 over. -->
            <div class="mb-3 px-3 py-2 bg-secondary/5 border border-secondary/30 rounded">
              <div class="text-[10px] uppercase tracking-wider text-secondary font-semibold mb-1.5">AI suggestions ({aiMilestoneProposals.length})</div>
              <ul class="space-y-1.5">
                {#each aiMilestoneProposals as p (p.text)}
                  <li class="flex items-start gap-2 text-xs">
                    <div class="flex-1 min-w-0">
                      <div class="text-text">{p.text}</div>
                      {#if p.due_date}
                        <div class="text-dim text-[10px] font-mono mt-0.5">due {p.due_date}</div>
                      {/if}
                    </div>
                    <button
                      onclick={() => void acceptMilestone(p)}
                      class="px-2 py-0.5 bg-success/15 text-success rounded hover:bg-success/25"
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
            {#if aiTaskBusy}
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

          {#if aiTaskError}
            <div class="mb-2 px-3 py-2 text-xs text-error border border-error/30 bg-error/5 rounded">
              {aiTaskError}
            </div>
          {/if}

          {#if aiTaskBusy && aiTaskProposals.length === 0}
            <div class="text-xs text-dim italic px-3 py-2 flex items-center gap-2">
              <span class="inline-block w-1.5 h-3 bg-primary/60 animate-pulse rounded-sm"></span>
              proposing 3-5 tasks for this week…
            </div>
          {/if}

          {#if aiTaskProposals.length > 0}
            <div class="px-3 py-2 bg-primary/5 border border-primary/30 rounded">
              <div class="flex items-baseline gap-2 mb-2">
                <span class="text-[10px] uppercase tracking-wider text-primary font-semibold">AI proposals ({aiTaskProposals.length})</span>
                <span class="flex-1"></span>
                <button
                  onclick={() => void acceptAllTaskProposals()}
                  class="text-[11px] text-primary hover:underline"
                  title="Create all proposals as tasks linked to this goal"
                >+ accept all</button>
              </div>
              <ul class="space-y-1.5">
                {#each aiTaskProposals as p (p.text)}
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
                      class="px-2 py-0.5 bg-success/15 text-success rounded hover:bg-success/25"
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

        <!-- Notes -->
        <section>
          <h3 class="text-xs uppercase tracking-wider text-dim font-medium mb-1.5">Notes</h3>
          {#if editingNotes}
            <textarea
              bind:value={notesBuf}
              onblur={commitNotes}
              onkeydown={(e) => { if (e.key === 'Escape') editingNotes = false; }}
              autofocus
              rows="4"
              class="w-full px-3 py-2 bg-surface0 border border-primary rounded text-sm text-text outline-none"
            ></textarea>
          {:else}
            <button
              onclick={() => { notesBuf = goal.notes ?? ''; editingNotes = true; }}
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
