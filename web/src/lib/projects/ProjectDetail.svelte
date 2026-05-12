<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Goal, type Project, type ProjectGoal, type Task } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import GoalEditor from './GoalEditor.svelte';
  import ProjectNotesTab from './ProjectNotesTab.svelte';
  import ProjectStarterPack from './ProjectStarterPack.svelte';
  import TaskRow from '$lib/components/TaskRow.svelte';
  import EntityDeadlines from '$lib/deadlines/EntityDeadlines.svelte';
  import { openAIOverlay } from '$lib/stores/ai-overlay';

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

  // Open the AI overlay pre-seeded with this project's context.
  // The seed includes name + status + description + open-task count
  // + linked goals so the model can answer "what's blocking this?",
  // "what should I work on next?", "draft a status update" without
  // the user having to re-state the project's basics.
  function askAIAboutThisProject(): void {
    const lines = [`I'm working on this project:`, '', `- ${project.name}`];
    if (project.status) lines.push(`- status: ${project.status}`);
    if (project.description && project.description.trim() !== '') {
      lines.push(`- description: ${project.description.trim()}`);
    }
    if (project.next_action && project.next_action.trim() !== '') {
      lines.push(`- next action: ${project.next_action.trim()}`);
    }
    const openTasks = projectTasks.filter((t) => !t.done);
    if (openTasks.length > 0) {
      lines.push(`- ${openTasks.length} open task${openTasks.length === 1 ? '' : 's'}`);
    }
    if (linkedGoals.length > 0) {
      const titles = linkedGoals.map((g) => g.title).join('; ');
      lines.push(`- linked goals: ${titles}`);
    }
    lines.push('', `What would help me move this forward?`);
    openAIOverlay({ text: lines.join('\n'), send: false });
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

  // ── This-week schedule strip ─────────────────────────────────────
  // 7-cell mini-calendar (Mon-Sun) showing how many of this
  // project's tasks are scheduled on each day of the current
  // week. Density bar height keys off the busiest day so a
  // light week and a heavy week both render readably. Cells are
  // clickable links into the calendar at that day, so a user
  // can hop straight from "this project has 3 tasks Wednesday"
  // to the day view.
  function startOfThisWeekMonday(): Date {
    const d = new Date();
    const dow = (d.getDay() + 6) % 7; // 0 = Monday
    d.setDate(d.getDate() - dow);
    d.setHours(0, 0, 0, 0);
    return d;
  }
  function ymd(d: Date): string {
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  }
  const weekSchedule = $derived.by(() => {
    const start = startOfThisWeekMonday();
    const today = ymd(new Date());
    const days: { date: string; label: string; count: number; isToday: boolean }[] = [];
    const labels = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
    for (let i = 0; i < 7; i++) {
      const d = new Date(start);
      d.setDate(d.getDate() + i);
      days.push({
        date: ymd(d),
        label: labels[i],
        count: 0,
        isToday: ymd(d) === today
      });
    }
    for (const t of projectTasks) {
      if (t.done || !t.scheduledStart) continue;
      const day = t.scheduledStart.slice(0, 10);
      const cell = days.find((x) => x.date === day);
      if (cell) cell.count++;
    }
    return days;
  });
  const weekScheduleMax = $derived(weekSchedule.reduce((m, d) => Math.max(m, d.count), 0));
  const weekScheduleTotal = $derived(weekSchedule.reduce((s, d) => s + d.count, 0));

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
  type HealthMomentum = 'alive' | 'slowing' | 'stalled' | 'dead';
  type HealthVerdict = {
    momentum: HealthMomentum;
    momentum_reason: string;
    blockers: string[];
    next_action: string;
  };

  let aiHealth = $state<HealthVerdict | null>(null);
  let aiHealthRaw = $state('');
  let aiHealthBusy = $state(false);
  let aiHealthError = $state('');
  let aiHealthAbort: AbortController | null = null;
  // Context the model actually saw — surfaced above the result so
  // the user understands what the verdict is grounded in. Without
  // this the response feels like a black box; a "saw 12 tasks +
  // 2 goals, last completion 4d ago" line keeps the AI legible.
  let aiHealthContextLine = $state('');

  function daysSinceLastCompletion(): number | null {
    let mostRecent: Date | null = null;
    for (const t of doneTasks) {
      if (!t.completedAt) continue;
      const d = new Date(t.completedAt);
      if (Number.isNaN(d.getTime())) continue;
      if (!mostRecent || d > mostRecent) mostRecent = d;
    }
    if (!mostRecent) return null;
    return Math.floor((Date.now() - mostRecent.getTime()) / 86400000);
  }

  async function runAIHealth() {
    if (aiHealthBusy) return;
    aiHealthBusy = true;
    aiHealthError = '';
    aiHealthRaw = '';
    aiHealth = null;
    aiHealthAbort = new AbortController();

    const sinceLast = daysSinceLastCompletion();
    const dueOpen = openTasks.filter((t) => t.dueDate);
    const overdueOpen = dueOpen.filter((t) => {
      const d = new Date(t.dueDate as string);
      return !Number.isNaN(d.getTime()) && d.getTime() < Date.now();
    });
    aiHealthContextLine =
      `AI saw ${openTasks.length} open + ${doneTasks.length} done task${
        projectTasks.length === 1 ? '' : 's'
      }` +
      (linkedGoals.length > 0 ? ` · ${linkedGoals.length} goal${linkedGoals.length === 1 ? '' : 's'}` : '') +
      (sinceLast === null ? ' · no completions yet' : ` · last completion ${sinceLast}d ago`) +
      (overdueOpen.length > 0 ? ` · ${overdueOpen.length} overdue` : '');

    // Compact, token-stingy context. Recent completions go newest-
    // first so the model anchors on momentum signal rather than
    // ancient history. Cap at 12 of each kind — beyond that the
    // model just paraphrases noise.
    const ctx = [
      `Project: ${project.name}`,
      project.status ? `Status: ${project.status}` : '',
      project.description ? `Description: ${project.description}` : '(no description)',
      project.next_action ? `Stated next action: ${project.next_action}` : '',
      project.due_date ? `Due: ${project.due_date}` : '',
      project.created_at ? `Created: ${project.created_at}` : '',
      sinceLast === null ? 'No completions on record yet.' : `Last completion: ${sinceLast} day(s) ago.`,
      `Tasks: ${openTasks.length} open / ${doneTasks.length} done` +
        (overdueOpen.length > 0 ? ` (${overdueOpen.length} overdue)` : ''),
      openTasks.length > 0
        ? `Open tasks (top ${Math.min(12, openTasks.length)}):\n${openTasks
            .slice(0, 12)
            .map((t) => `- ${t.text}${t.dueDate ? ` (due ${t.dueDate})` : ''}${t.scheduledStart ? ` [scheduled]` : ''}`)
            .join('\n')}`
        : '',
      doneTasks.length > 0
        ? `Recent completions (newest first):\n${[...doneTasks]
            .sort((a, b) => (b.completedAt ?? '').localeCompare(a.completedAt ?? ''))
            .slice(0, 8)
            .map((t) => `- ${t.text}${t.completedAt ? ` (${t.completedAt.slice(0, 10)})` : ''}`)
            .join('\n')}`
        : '',
      linkedGoals.length > 0
        ? `Linked goals:\n${linkedGoals.map((g) => `- ${g.title}${g.status ? ` [${g.status}]` : ''}`).join('\n')}`
        : ''
    ]
      .filter(Boolean)
      .join('\n\n');

    // The system prompt is the load-bearing part. Keep it sharp,
    // declarative, and explicit about the JSON schema — a vague
    // ask gets a vague answer. Outlawing puffery ("synergy", "let's
    // align", "leverage") tightens the voice considerably.
    const system =
      'You are a senior project manager who reads project state and renders an honest verdict in seconds. ' +
      'Output STRICT JSON only — no preamble, no code fence, no commentary. Schema:\n' +
      '{\n' +
      '  "momentum": "alive" | "slowing" | "stalled" | "dead",\n' +
      '  "momentum_reason": string  // one sentence, evidence-based, name specific signals\n' +
      '  "blockers": string[]       // 0-3 concrete blockers; [] if nothing is stuck\n' +
      '  "next_action": string      // ONE concrete action the user could do today, ≤14 words\n' +
      '}\n\n' +
      'Rules:\n' +
      '- "alive": work shipped in the last 7 days AND no overdue stack.\n' +
      '- "slowing": last completion 8-21 days ago, or open list growing without closes.\n' +
      '- "stalled": last completion 22+ days ago, or status=paused with overdue tasks.\n' +
      '- "dead": no completions ever or last completion 60+ days ago and status=active.\n' +
      '- Blockers must be SPECIFIC. Bad: "needs prioritization". Good: "3 tasks all blocked on client review".\n' +
      '- The next_action must be a verb-led concrete step, not a category. Bad: "review tasks". Good: "draft the onboarding email and send to Sara".\n' +
      '- No corporate sludge: no "synergy", "leverage", "let\'s align", "circle back", "actionable insights".\n' +
      '- If the project has zero tasks, momentum is "dead" and next_action is "write down what done looks like, or archive this".';

    const user = `Project context:\n\n${ctx}\n\nReturn the JSON verdict.`;

    try {
      await api.chatStream(
        [
          { role: 'system', content: system },
          { role: 'user', content: user }
        ],
        undefined,
        {
          onChunk: (c) => {
            aiHealthRaw += c;
          },
          onError: (err) => {
            aiHealthError = err.message;
          }
        },
        aiHealthAbort.signal
      );
      // Parse on completion. Streaming-parse JSON would render
      // garbage half-objects to the user; far cleaner to wait for
      // the whole payload then parse once.
      const trimmed = aiHealthRaw.trim();
      if (trimmed) {
        try {
          // Strip ``` fences if the model ignored the no-fence rule.
          const cleaned = trimmed.replace(/^```(?:json)?\s*/i, '').replace(/\s*```$/i, '');
          const parsed = JSON.parse(cleaned) as HealthVerdict;
          if (
            parsed &&
            typeof parsed.momentum === 'string' &&
            typeof parsed.next_action === 'string' &&
            Array.isArray(parsed.blockers)
          ) {
            aiHealth = parsed;
          } else {
            aiHealthError = 'AI returned unexpected shape — see raw output below.';
          }
        } catch {
          aiHealthError = 'AI did not return valid JSON — see raw output below.';
        }
      }
    } finally {
      aiHealthBusy = false;
      aiHealthAbort = null;
    }
  }
  function cancelAIHealth() {
    aiHealthAbort?.abort();
  }
  function clearAIHealth() {
    aiHealth = null;
    aiHealthRaw = '';
    aiHealthError = '';
    aiHealthContextLine = '';
  }

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
  let aiBrief = $state('');
  let aiBriefBusy = $state(false);
  let aiBriefError = $state('');
  let aiBriefAbort: AbortController | null = null;
  let aiBriefSaving = $state(false);

  async function runAIBrief() {
    if (aiBriefBusy) return;
    aiBriefBusy = true;
    aiBriefError = '';
    aiBrief = '';
    aiBriefAbort = new AbortController();

    const ctx = [
      `Project name: ${project.name}`,
      project.kind ? `Kind: ${project.kind}` : '',
      project.venture ? `Venture: ${project.venture}` : '',
      project.tags && project.tags.length > 0 ? `Tags: ${project.tags.join(', ')}` : '',
      project.next_action ? `Stated next action: ${project.next_action}` : '',
      openTasks.length > 0
        ? `Open tasks (suggest scope):\n${openTasks
            .slice(0, 15)
            .map((t) => `- ${t.text}`)
            .join('\n')}`
        : '',
      doneTasks.length > 0
        ? `Already shipped:\n${[...doneTasks]
            .sort((a, b) => (b.completedAt ?? '').localeCompare(a.completedAt ?? ''))
            .slice(0, 8)
            .map((t) => `- ${t.text}`)
            .join('\n')}`
        : '',
      linkedGoals.length > 0
        ? `Linked goals:\n${linkedGoals.map((g) => `- ${g.title}`).join('\n')}`
        : ''
    ]
      .filter(Boolean)
      .join('\n\n');

    // Tight system prompt: one paragraph, three things in order,
    // no bullets, no preamble, no invented stakeholders. The "tasks
    // are the truth, not the name" line steers the model away from
    // making up scope from a fancy project name.
    const system =
      'You write tight project briefs the user can paste into a description field. ' +
      'Output ONE paragraph, 2-4 sentences, plain prose. No headings, no bullets, no preamble like "This project is about" or "The goal of". ' +
      'Cover three things in this order: (1) what this project IS in concrete terms, (2) what "done" looks like, (3) who or what depends on it (or "nothing yet" if unclear). ' +
      'Infer from the task list — the tasks are the truth, not the name. ' +
      'Do not invent stakeholders, deadlines, or technologies that are not in the context. ' +
      'No corporate sludge: no "synergy", "leverage", "robust", "best-in-class", "stakeholders aligning", "drive value". ' +
      'Under 70 words.';
    const user = `Write a brief for this project.\n\n${ctx}`;

    try {
      await api.chatStream(
        [
          { role: 'system', content: system },
          { role: 'user', content: user }
        ],
        undefined,
        {
          onChunk: (c) => {
            aiBrief += c;
          },
          onError: (err) => {
            aiBriefError = err.message;
          }
        },
        aiBriefAbort.signal
      );
    } finally {
      aiBriefBusy = false;
      aiBriefAbort = null;
    }
  }
  function cancelAIBrief() {
    aiBriefAbort?.abort();
  }
  async function applyAIBrief() {
    const text = aiBrief.trim();
    if (!text) return;
    aiBriefSaving = true;
    try {
      const ok = await patch({ description: text });
      if (ok) {
        aiBrief = '';
        aiBriefError = '';
      }
    } finally {
      aiBriefSaving = false;
    }
  }
  function dismissAIBrief() {
    aiBriefAbort?.abort();
    aiBrief = '';
    aiBriefError = '';
  }
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
          {#if aiHealthBusy}
            <button onclick={cancelAIHealth} class="text-[11px] text-warning hover:underline">cancel</button>
          {:else if aiHealth || aiHealthRaw || aiHealthError}
            <button
              onclick={clearAIHealth}
              class="text-[11px] text-dim hover:text-error"
            >clear</button>
          {/if}
          <button
            onclick={() => void runAIHealth()}
            disabled={aiHealthBusy || projectTasks.length === 0}
            class="text-[11px] px-2 py-0.5 rounded bg-surface0 border border-surface1 text-subtext hover:border-primary disabled:opacity-50"
            title="Ask the AI for a momentum / blockers / next-move verdict on this project"
          >{aiHealthBusy ? '✨ analysing…' : (aiHealth || aiHealthRaw) ? '✨ rerun' : '✨ check health'}</button>
        </div>

        {#if aiHealthContextLine && (aiHealthBusy || aiHealth || aiHealthRaw || aiHealthError)}
          <p class="text-[10px] text-dim mb-1.5 font-mono">{aiHealthContextLine}</p>
        {/if}

        {#if aiHealth}
          {@const tone = momentumTone(aiHealth.momentum)}
          <div class="bg-surface0 border border-surface1 rounded px-3 py-3 text-sm text-text space-y-3">
            <!-- Momentum pill + reason -->
            <div class="flex items-baseline gap-2">
              <span
                class="px-2 py-0.5 rounded text-[10px] uppercase tracking-wider font-medium flex-shrink-0"
                style="background: var(--color-{tone}); color: var(--color-base);"
              >{momentumLabel(aiHealth.momentum)}</span>
              <span class="text-text/90 text-xs leading-snug">{aiHealth.momentum_reason}</span>
            </div>

            <!-- Blockers — listed individually so each is scannable
                 instead of buried in a paragraph. -->
            <div>
              <div class="text-[10px] uppercase tracking-wider text-dim mb-1">Blockers</div>
              {#if aiHealth.blockers.length === 0}
                <p class="text-xs text-success">Nothing flagged as stuck.</p>
              {:else}
                <ul class="space-y-1">
                  {#each aiHealth.blockers as b, i (i)}
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
                  onclick={() => patch({ next_action: aiHealth!.next_action })}
                  class="text-[10px] text-secondary hover:underline"
                  title="copy this into the project's Next action field"
                >use as next action →</button>
              </div>
              <p class="text-sm text-warning font-medium">→ {aiHealth.next_action}</p>
            </div>
          </div>
        {:else if aiHealthError}
          <div class="text-xs text-error border border-error bg-surface0 rounded px-3 py-2">
            <div class="font-medium mb-1">{aiHealthError}</div>
            {#if aiHealthRaw}
              <pre class="text-[10px] text-dim font-mono whitespace-pre-wrap mt-1">{aiHealthRaw}</pre>
            {/if}
          </div>
        {:else if aiHealthBusy}
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
          {#if !project.description && !aiBrief && !aiBriefBusy && (projectTasks.length > 0 || linkedGoals.length > 0)}
            <button
              onclick={() => void runAIBrief()}
              class="text-[11px] px-2 py-0.5 rounded bg-surface0 border border-surface1 text-subtext hover:border-primary"
              title="Ask the AI to draft a one-paragraph brief from the project's tasks + goals"
            >✨ draft brief</button>
          {:else if aiBriefBusy}
            <button onclick={cancelAIBrief} class="text-[11px] text-warning hover:underline">cancel</button>
          {/if}
        </div>

        {#if aiBriefError}
          <div class="text-xs text-error border border-error bg-surface0 rounded px-3 py-2 mb-2">
            {aiBriefError}
            <button onclick={dismissAIBrief} class="ml-2 underline">dismiss</button>
          </div>
        {/if}

        {#if aiBrief || aiBriefBusy}
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
              {aiBrief || '…'}
            </div>
            {#if !aiBriefBusy && aiBrief}
              <div class="px-3 py-2 border-t border-surface2 flex items-center gap-2">
                <button
                  onclick={() => void applyAIBrief()}
                  disabled={aiBriefSaving}
                  class="text-[11px] px-2 py-0.5 rounded bg-primary text-on-primary hover:opacity-90 disabled:opacity-50"
                >{aiBriefSaving ? 'saving…' : 'save as description'}</button>
                <button
                  onclick={() => void runAIBrief()}
                  disabled={aiBriefSaving}
                  class="text-[11px] px-2 py-0.5 rounded bg-surface0 border border-surface1 text-subtext hover:border-primary"
                >regenerate</button>
                <button
                  onclick={dismissAIBrief}
                  disabled={aiBriefSaving}
                  class="text-[11px] text-dim hover:text-error ml-auto"
                >dismiss</button>
              </div>
            {/if}
          </div>
        {/if}

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
