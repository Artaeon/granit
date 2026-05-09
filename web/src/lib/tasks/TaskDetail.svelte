<script lang="ts">
  import { goto } from '$app/navigation';
  import { api, type Task, type Project, type Goal, type Deadline } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { cleanTaskText } from '$lib/util/taskParse';
  import Drawer from '$lib/components/Drawer.svelte';

  // TaskDetail is the side-drawer that pops open when the user clicks
  // a task card. Editable fields not already inline-editable on the card:
  // recurrence, free-form notes, dependency text. Read-only summary of
  // metadata (created/completed/updated) for context.

  let {
    open = $bindable(false),
    task,
    onChanged
  }: {
    open?: boolean;
    task: Task | null;
    onChanged?: () => void | Promise<void>;
  } = $props();

  let notesBuf = $state('');
  let recurrenceBuf = $state('');
  let busy = $state(false);
  // Editable scheduling. The drawer was previously read-only here;
  // users had to bounce to the calendar grid to set a due date.
  let dueBuf = $state('');
  let schedDateBuf = $state(''); // YYYY-MM-DD
  let schedTimeBuf = $state(''); // HH:MM
  // Inline title editing — click the title to edit, Enter / blur to
  // commit, Esc to cancel.
  let titleEditing = $state(false);
  let titleBuf = $state('');
  // Tags + estimate edits would round-trip through the task line's
  // #tag / est:Nm markers (parseTaskInput handles them on read), but
  // the patchTask API doesn't currently expose direct fields for
  // either. Keeping these read-only on the drawer for now — quick-add
  // bar is the canonical surface for setting them at create time.

  // Linkable-entity lists. Lazy-loaded the first time the drawer opens
  // so the list pages don't pay the lookup cost on every card render.
  let projects = $state<Project[]>([]);
  let goals = $state<Goal[]>([]);
  let deadlines = $state<Deadline[]>([]);
  let linksLoaded = $state(false);

  // ── AI Decompose ────────────────────────────────────────────────
  // Takes the task title + notes and asks the model for 3-7 small,
  // concrete sub-tasks. Renders proposals with per-row accept/skip;
  // accepting calls api.createTask in the parent's notePath so the
  // subtask shows up in the same daily/project note. Goes through
  // chatStream so audit/sabbath/redaction/cost all apply.
  type Subtask = {
    text: string;
    estimateMinutes?: number;
    rationale?: string;
  };
  let aiDecompBusy = $state(false);
  let aiDecompError = $state('');
  let aiDecompRaw = $state('');
  let aiDecompSubtasks = $state<Subtask[]>([]);
  let aiDecompAbort: AbortController | null = null;
  let aiDecompAcceptingIdx = $state<number>(-1);

  function extractDecompJson(s: string): string | null {
    if (!s) return null;
    const fence = s.match(/```(?:json)?\s*([\s\S]*?)```/);
    const candidate = fence ? fence[1] : s;
    const start = candidate.indexOf('{');
    if (start < 0) return null;
    let depth = 0;
    for (let i = start; i < candidate.length; i++) {
      const c = candidate[i];
      if (c === '{') depth++;
      else if (c === '}') {
        depth--;
        if (depth === 0) return candidate.slice(start, i + 1);
      }
    }
    return null;
  }

  async function runAIDecompose() {
    if (!task || aiDecompBusy) return;
    aiDecompBusy = true;
    aiDecompError = '';
    aiDecompRaw = '';
    aiDecompSubtasks = [];
    aiDecompAbort = new AbortController();
    // Fetch existing siblings (tasks in same notePath) so the prompt
    // can be told "don't propose duplicates of these". Best-effort —
    // failure just means the dedup hint is missing, not that we crash.
    let existingSiblings: string[] = [];
    try {
      const r = await api.listTasks({ status: 'open' });
      existingSiblings = r.tasks
        .filter((t) => t.notePath === task!.notePath && t.id !== task!.id)
        .map((t) => t.text)
        .slice(0, 30);
    } catch {}
    const system =
      'You are a focused task decomposer. The user has one task; your job is to break it into 3-7 small, ' +
      'concrete sub-tasks they can DO, not vague "research X" stubs. ' +
      'Hard rules: ' +
      '(1) Each subtask is a single concrete action, ideally finishable in under 60 minutes. ' +
      '(2) Use ACTIVE verbs ("draft the intro paragraph", "email Sarah for the spec PDF") — never "look into", "research", "consider", "explore". ' +
      '(3) Order them by execution sequence. The first subtask should be something the user can start in the next 15 minutes. ' +
      '(4) Do NOT propose subtasks that duplicate the supplied existing-siblings list — those already exist. ' +
      '(5) Estimate each in minutes (15, 30, 45, 60, 90, 120). ' +
      '(6) Keep the rationale to ONE short clause under 12 words, only if it adds non-obvious context. Most subtasks need no rationale. ' +
      '(7) Output STRICT JSON ONLY, no fences, no preamble. Schema: ' +
      '{"subtasks":[{"text":"<concrete action>","estimateMinutes":30,"rationale":"<optional, short>"}]}. ' +
      '(8) If the task is too small to decompose meaningfully, return {"subtasks":[]}.';
    const user =
      `Parent task: ${task.text}\n` +
      (task.notes ? `\nParent notes:\n${task.notes}\n` : '') +
      (existingSiblings.length > 0
        ? `\nExisting siblings in the same note (do NOT duplicate):\n${existingSiblings.map((s) => '- ' + s).join('\n')}\n`
        : '') +
      '\nReturn the strict JSON now.';
    try {
      await api.chatStream(
        [
          { role: 'system', content: system },
          { role: 'user', content: user }
        ],
        task.notePath,
        {
          onChunk: (c) => {
            aiDecompRaw += c;
            const block = extractDecompJson(aiDecompRaw);
            if (block) {
              try {
                const parsed = JSON.parse(block) as { subtasks?: Subtask[] };
                if (Array.isArray(parsed.subtasks)) {
                  aiDecompSubtasks = parsed.subtasks
                    .filter((s) => s && typeof s.text === 'string' && s.text.trim().length > 0);
                }
              } catch {}
            }
          },
          onError: (err) => { aiDecompError = err.message; }
        },
        aiDecompAbort.signal
      );
    } finally {
      aiDecompBusy = false;
      aiDecompAbort = null;
    }
  }
  function cancelAIDecompose() { aiDecompAbort?.abort(); }
  function dismissAIDecompose() {
    aiDecompRaw = '';
    aiDecompError = '';
    aiDecompSubtasks = [];
  }

  // Accept a subtask: create a task in the parent's notePath with the
  // proposed text. Estimate goes via the est:Nm marker the parser
  // already understands so the existing taskParse round-trips work.
  async function acceptSubtask(idx: number) {
    if (!task) return;
    const s = aiDecompSubtasks[idx];
    if (!s) return;
    aiDecompAcceptingIdx = idx;
    try {
      // Parser convention: `est:30m` in the task line gets stripped
      // by cleanTaskText for display but persists as estimatedMinutes
      // sidecar metadata. Embedding it directly in `text` keeps us
      // independent of API field names that may not exist.
      let text = s.text.trim();
      if (s.estimateMinutes && s.estimateMinutes > 0) {
        text = `${text} est:${s.estimateMinutes}m`;
      }
      // Inherit parent's goal + deadline directly. createTask's body
      // doesn't accept projectId — server derives project membership
      // from the parent's notePath / sidecar metadata, which the
      // subtask inherits for free by living in the same note. If the
      // parent has an explicit projectId override we patch after.
      const created = await api.createTask({
        notePath: task.notePath,
        text,
        goalId: task.goalId,
        deadlineId: task.deadlineId
      });
      if (task.projectId && created?.id) {
        try {
          await api.patchTask(created.id, { projectId: task.projectId });
        } catch {}
      }
      aiDecompSubtasks = aiDecompSubtasks.filter((_, i) => i !== idx);
      await onChanged?.();
      toast.success('Subtask added');
    } catch (e) {
      toast.error('Add failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      aiDecompAcceptingIdx = -1;
    }
  }
  function skipSubtask(idx: number) {
    aiDecompSubtasks = aiDecompSubtasks.filter((_, i) => i !== idx);
  }
  async function acceptAllSubtasks() {
    while (aiDecompSubtasks.length > 0) {
      // Always accept index 0 — the array shrinks as each call resolves.
      await acceptSubtask(0);
    }
  }

  async function loadLinks() {
    if (linksLoaded) return;
    linksLoaded = true;
    // Settle these in parallel — three independent reads. Failures
    // degrade silently to an empty list rather than blocking the
    // drawer; the dropdown will just show "(none)".
    const [pp, gg, dd] = await Promise.allSettled([
      api.listProjects(),
      api.listGoals(),
      api.listDeadlines()
    ]);
    if (pp.status === 'fulfilled') projects = pp.value.projects;
    if (gg.status === 'fulfilled') goals = gg.value.goals;
    if (dd.status === 'fulfilled') deadlines = dd.value.deadlines;
  }

  // Resync local buffers whenever the modal opens for a different task.
  // Also reset Decompose state — proposals from a previous task
  // shouldn't leak into the next one's drawer.
  $effect(() => {
    if (open && task) {
      notesBuf = task.notes ?? '';
      recurrenceBuf = task.recurrence ?? '';
      dueBuf = task.dueDate ?? '';
      schedDateBuf = task.scheduledStart ? task.scheduledStart.slice(0, 10) : '';
      schedTimeBuf = task.scheduledStart ? task.scheduledStart.slice(11, 16) : '';
      titleEditing = false;
      titleBuf = cleanTaskText(task.text);
      aiDecompAbort?.abort();
      aiDecompRaw = '';
      aiDecompError = '';
      aiDecompSubtasks = [];
      aiDecompBusy = false;
      void loadLinks();
    }
  });

  async function patch(patch: Parameters<typeof api.patchTask>[1]) {
    if (!task) return;
    busy = true;
    try {
      await api.patchTask(task.id, patch);
      await onChanged?.();
    } catch (e) {
      toast.error('save failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      busy = false;
    }
  }

  async function commitNotes() {
    if (!task) return;
    if (notesBuf === (task.notes ?? '')) return;
    await patch({ notes: notesBuf });
  }

  async function setRecurrence(r: string) {
    recurrenceBuf = r;
    await patch({ recurrence: r });
  }

  async function setPriority(p: number) { await patch({ priority: p }); }
  async function toggleDone() { if (task) await patch({ done: !task.done }); }
  async function setTriage(state: NonNullable<Task['triage']>) { await patch({ triage: state }); }
  async function setProject(name: string) { await patch({ projectId: name }); }
  async function setGoal(id: string) { await patch({ goalId: id }); }
  async function setDeadline(id: string) { await patch({ deadlineId: id }); }

  // Title edit. cleanTaskText strips inline markers (!1 / due:.. / #tag)
  // for display; we round-trip the user's edit as the new task text and
  // let the parser re-extract markers from the new line on the next
  // read. Empty title is a no-op.
  async function commitTitle() {
    if (!task) { titleEditing = false; return; }
    const next = titleBuf.trim();
    if (!next || next === cleanTaskText(task.text)) { titleEditing = false; return; }
    titleEditing = false;
    await patch({ text: next });
  }
  function cancelTitleEdit() {
    if (task) titleBuf = cleanTaskText(task.text);
    titleEditing = false;
  }

  // Date / time edits. The backend accepts dueDate as 'YYYY-MM-DD' or
  // empty to clear. scheduledStart is 'YYYY-MM-DDTHH:MM' (local, no
  // zone — the backend stores it as wall-clock, see commit 05183fc).
  async function commitDue() {
    if (!task) return;
    const next = dueBuf.trim();
    if (next === (task.dueDate ?? '')) return;
    await patch({ dueDate: next });
  }
  async function commitScheduled() {
    if (!task) return;
    const ds = schedDateBuf.trim();
    const ts = schedTimeBuf.trim();
    let next = '';
    if (ds && ts) next = `${ds}T${ts}`;
    else if (ds) next = `${ds}T09:00`; // sensible default if user picked date but no time
    if (next === (task.scheduledStart ?? '')) return;
    await patch({ scheduledStart: next });
  }
  async function clearScheduled() {
    if (!task) return;
    schedDateBuf = '';
    schedTimeBuf = '';
    await patch({ scheduledStart: '' });
  }

  // Snooze quick-actions. Sets snoozedUntil to the given local-time
  // YYYY-MM-DDTHH:MM string + flips triage to 'snoozed'. Uses local
  // wall-clock so the timing matches the user's intent without TZ
  // arithmetic.
  function snoozeOffset(days: number, hour = 9): string {
    const d = new Date();
    d.setDate(d.getDate() + days);
    d.setHours(hour, 0, 0, 0);
    const y = d.getFullYear();
    const m = String(d.getMonth() + 1).padStart(2, '0');
    const dd = String(d.getDate()).padStart(2, '0');
    const hh = String(d.getHours()).padStart(2, '0');
    const mi = String(d.getMinutes()).padStart(2, '0');
    return `${y}-${m}-${dd}T${hh}:${mi}`;
  }
  async function snoozeUntil(days: number) {
    await patch({ snoozedUntil: snoozeOffset(days), triage: 'snoozed' });
  }
  async function unsnooze() {
    await patch({ snoozedUntil: '', triage: 'triaged' });
  }
  let snoozeActive = $derived.by(() => {
    if (!task?.snoozedUntil) return false;
    const sn = new Date(task.snoozedUntil);
    return Number.isFinite(sn.getTime()) && sn.getTime() > Date.now();
  });

  function close() { open = false; }
  function openNote() {
    if (!task) return;
    goto(`/notes/${encodeURIComponent(task.notePath)}`);
    close();
  }

  function fmtDate(s?: string): string {
    if (!s) return '—';
    const d = new Date(s);
    return d.toLocaleString();
  }

  const recurrenceOptions: { value: string; label: string }[] = [
    { value: '', label: 'none' },
    { value: 'daily', label: 'daily' },
    { value: 'weekly', label: 'weekly' },
    { value: 'monthly', label: 'monthly' },
    { value: '3x-week', label: '3× / week' }
  ];
  const triageStates: NonNullable<Task['triage']>[] = ['inbox', 'triaged', 'scheduled', 'done', 'dropped', 'snoozed'];
</script>

<Drawer bind:open side="right" responsive width="w-full sm:w-96 md:w-[28rem]">
  {#if task}
    <div class="h-full flex flex-col overflow-hidden">
      <header class="px-4 py-3 border-b border-surface1 flex items-center gap-2 flex-shrink-0">
        <h2 class="text-sm font-semibold text-text flex-1 truncate">Task details</h2>
        {#if busy}
          <span class="text-[10px] text-dim italic" aria-live="polite">saving…</span>
        {/if}
        {#if snoozeActive}
          <span class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-info/15 text-info border border-info/25" title={`snoozed until ${task?.snoozedUntil}`}>snoozed</span>
        {/if}
        <button onclick={close} aria-label="close" class="text-dim hover:text-text text-xl leading-none">×</button>
      </header>

      <div class="flex-1 overflow-y-auto p-4 space-y-4">
        <!-- Title + done toggle -->
        <section class="flex items-start gap-2">
          <button
            onclick={toggleDone}
            disabled={busy}
            class="w-5 h-5 mt-0.5 rounded border flex items-center justify-center flex-shrink-0
              {task.done ? 'bg-success border-success' : 'border-surface2 hover:border-primary'}"
            aria-label={task.done ? 'mark not done' : 'mark done'}
          >
            {#if task.done}
              <svg viewBox="0 0 12 12" class="w-3 h-3 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
            {/if}
          </button>
          <div class="flex-1 min-w-0">
            {#if titleEditing}
              <input
                bind:value={titleBuf}
                onblur={commitTitle}
                onkeydown={(e) => {
                  if (e.key === 'Enter') { e.preventDefault(); void commitTitle(); }
                  else if (e.key === 'Escape') { e.preventDefault(); cancelTitleEdit(); }
                }}
                disabled={busy}
                aria-label="task title"
                class="w-full px-2 py-1 -mx-2 -my-1 bg-mantle border border-primary rounded text-base font-medium text-text focus:outline-none"
              />
            {:else}
              <button
                type="button"
                onclick={() => { titleEditing = true; }}
                class="text-base font-medium text-text break-words text-left w-full hover:bg-surface1/40 rounded px-2 py-1 -mx-2 -my-1 transition-colors {task.done ? 'line-through text-dim' : ''}"
                title="click to rename"
              >{cleanTaskText(task.text)}</button>
            {/if}
            <a href="/notes/{encodeURIComponent(task.notePath)}" onclick={openNote} class="text-xs text-secondary hover:underline font-mono">
              {task.notePath}
            </a>
          </div>
        </section>

        <!-- Priority pills -->
        <section>
          <h4 class="text-[11px] uppercase tracking-wider text-dim mb-1.5">Priority</h4>
          <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-xs">
            {#each [{ p: 0, label: 'none' }, { p: 1, label: 'P1' }, { p: 2, label: 'P2' }, { p: 3, label: 'P3' }] as o}
              <button
                onclick={() => setPriority(o.p)}
                disabled={busy}
                class="flex-1 px-3 py-1.5 {task.priority === o.p ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
              >{o.label}</button>
            {/each}
          </div>
        </section>

        <!-- AI Decompose. Streams 3-7 concrete sub-tasks; user
             accepts each individually and the API call creates them
             in the parent's notePath, inheriting project/goal/deadline.
             Hidden behind a button (not always-on) so the drawer
             stays calm when you just want to set a priority. -->
        <section>
          <div class="flex items-baseline gap-2 mb-1.5">
            <h4 class="text-[11px] uppercase tracking-wider text-dim flex-1">Decompose</h4>
            {#if aiDecompBusy}
              <button
                onclick={cancelAIDecompose}
                class="text-[11px] text-warning hover:underline"
              >cancel</button>
            {:else if aiDecompSubtasks.length > 0 || aiDecompError || aiDecompRaw}
              <button
                onclick={() => void runAIDecompose()}
                class="text-[11px] text-secondary hover:underline"
              >↻ regenerate</button>
              <button
                onclick={dismissAIDecompose}
                class="text-[11px] text-dim hover:text-error"
              >dismiss</button>
            {:else}
              <button
                onclick={() => void runAIDecompose()}
                class="text-[11px] px-2 py-0.5 bg-secondary/15 text-secondary rounded hover:bg-secondary/25"
                title="AI proposes 3-7 concrete sub-tasks"
              >✨ break it down</button>
            {/if}
          </div>
          {#if aiDecompError}
            <p class="text-xs text-error">{aiDecompError}</p>
          {:else if aiDecompSubtasks.length > 0}
            <div class="bg-secondary/5 border border-secondary/30 rounded p-2 space-y-1.5">
              <div class="flex items-center mb-0.5">
                <span class="text-[10px] uppercase tracking-wider text-secondary font-semibold flex-1">{aiDecompSubtasks.length} proposed</span>
                {#if aiDecompSubtasks.length > 1}
                  <button
                    onclick={() => void acceptAllSubtasks()}
                    disabled={aiDecompAcceptingIdx >= 0}
                    class="text-[10px] text-success hover:underline disabled:opacity-50"
                    title="Add every proposed subtask"
                  >accept all</button>
                {/if}
              </div>
              <ul class="space-y-1.5">
                {#each aiDecompSubtasks as s, idx (s.text + idx)}
                  <li class="flex items-start gap-2 text-xs">
                    <div class="flex-1 min-w-0">
                      <div class="text-text">
                        {s.text}
                        {#if s.estimateMinutes}
                          <span class="text-dim ml-1 font-mono tabular-nums">{s.estimateMinutes}m</span>
                        {/if}
                      </div>
                      {#if s.rationale}
                        <div class="text-dim italic mt-0.5">{s.rationale}</div>
                      {/if}
                    </div>
                    <button
                      onclick={() => void acceptSubtask(idx)}
                      disabled={aiDecompAcceptingIdx >= 0}
                      class="px-2 py-0.5 bg-success/15 text-success rounded hover:bg-success/25 disabled:opacity-50 flex-shrink-0"
                    >{aiDecompAcceptingIdx === idx ? '…' : 'add'}</button>
                    <button
                      onclick={() => skipSubtask(idx)}
                      disabled={aiDecompAcceptingIdx >= 0}
                      class="px-2 py-0.5 text-dim hover:text-text flex-shrink-0"
                    >skip</button>
                  </li>
                {/each}
              </ul>
              <p class="text-[10px] text-dim italic mt-1.5">Adds to <span class="font-mono">{task.notePath.split('/').pop()}</span> · inherits project / goal / deadline.</p>
            </div>
          {:else if aiDecompBusy}
            <p class="text-xs text-dim italic">thinking…</p>
          {:else if aiDecompRaw}
            <!-- JSON parse failed AND we got prose. Surface the raw
                 reply rather than silently swallowing it so the user
                 can still salvage value. -->
            <div class="bg-surface0 rounded p-2 text-xs text-subtext whitespace-pre-wrap">{aiDecompRaw}</div>
          {/if}
        </section>

        <!-- Schedule. Two inputs: due date (a calendar deadline,
             optional time) and scheduled-start (the actual block on
             the calendar grid). Some tasks have both — "due Friday,
             scheduled to start Wednesday morning". -->
        <section>
          <h4 class="text-[11px] uppercase tracking-wider text-dim mb-1.5">Schedule</h4>
          <div class="space-y-2 text-xs">
            <label class="flex items-center gap-2">
              <span class="text-dim w-20 flex-shrink-0">Due</span>
              <input
                type="date"
                bind:value={dueBuf}
                onchange={commitDue}
                disabled={busy}
                class="flex-1 min-w-0 bg-surface0 border border-surface1 rounded px-2 py-1 text-text"
              />
              {#if dueBuf}
                <button onclick={() => { dueBuf = ''; void commitDue(); }} class="text-dim hover:text-error" title="clear due date" aria-label="clear due date">×</button>
              {/if}
            </label>
            <label class="flex items-center gap-2">
              <span class="text-dim w-20 flex-shrink-0">Start</span>
              <input
                type="date"
                bind:value={schedDateBuf}
                onchange={commitScheduled}
                disabled={busy}
                class="flex-1 min-w-0 bg-surface0 border border-surface1 rounded px-2 py-1 text-text"
              />
              <input
                type="time"
                bind:value={schedTimeBuf}
                onchange={commitScheduled}
                disabled={busy || !schedDateBuf}
                class="w-24 bg-surface0 border border-surface1 rounded px-2 py-1 text-text disabled:opacity-50"
              />
              {#if schedDateBuf || schedTimeBuf}
                <button onclick={() => void clearScheduled()} class="text-dim hover:text-error" title="clear scheduled start" aria-label="clear scheduled start">×</button>
              {/if}
            </label>
            {#if task.estimatedMinutes}
              <div class="flex items-baseline gap-2 text-[11px] text-dim">
                <span class="w-20 flex-shrink-0">Estimate</span>
                <span class="text-text font-mono tabular-nums">{task.estimatedMinutes}m</span>
                <span class="italic">— set via <code>est:30m</code> in the quick-add bar</span>
              </div>
            {/if}
          </div>
        </section>

        <!-- Snooze. Common-cadence quick-actions on the row, custom
             ISO input falling out below. Setting any of these flips
             triage to 'snoozed' so the task hides from default views
             until the timestamp passes. -->
        <section>
          <h4 class="text-[11px] uppercase tracking-wider text-dim mb-1.5">Snooze</h4>
          {#if snoozeActive && task.snoozedUntil}
            <div class="flex items-baseline gap-2 mb-2 px-2 py-1.5 bg-info/10 border border-info/20 rounded">
              <span class="text-xs text-info flex-1">until {fmtDate(task.snoozedUntil)}</span>
              <button onclick={unsnooze} disabled={busy} class="text-[11px] text-warning hover:underline">unsnooze</button>
            </div>
          {/if}
          <div class="flex flex-wrap gap-1 text-xs">
            <button onclick={() => void snoozeUntil(1)} disabled={busy} class="px-2.5 py-1 rounded bg-surface0 border border-surface1 text-subtext hover:border-info hover:text-info">tomorrow</button>
            <button onclick={() => void snoozeUntil(2)} disabled={busy} class="px-2.5 py-1 rounded bg-surface0 border border-surface1 text-subtext hover:border-info hover:text-info">in 2d</button>
            <button onclick={() => void snoozeUntil(7)} disabled={busy} class="px-2.5 py-1 rounded bg-surface0 border border-surface1 text-subtext hover:border-info hover:text-info">next week</button>
            <button onclick={() => void snoozeUntil(14)} disabled={busy} class="px-2.5 py-1 rounded bg-surface0 border border-surface1 text-subtext hover:border-info hover:text-info">in 2 weeks</button>
            <button onclick={() => void snoozeUntil(30)} disabled={busy} class="px-2.5 py-1 rounded bg-surface0 border border-surface1 text-subtext hover:border-info hover:text-info">next month</button>
          </div>
        </section>

        <!-- Tags (read-only summary). Tags round-trip as #tag markers
             in the task line; the quick-add bar is the place to set
             them at create time. -->
        {#if task.tags && task.tags.length > 0}
          <section>
            <h4 class="text-[11px] uppercase tracking-wider text-dim mb-1.5">Tags</h4>
            <div class="flex flex-wrap gap-1">
              {#each task.tags as t (t)}
                <span class="text-[11px] px-1.5 py-0.5 rounded bg-surface1 text-accent">#{t}</span>
              {/each}
            </div>
          </section>
        {/if}

        <!-- Triage row -->
        <section>
          <h4 class="text-[11px] uppercase tracking-wider text-dim mb-1.5">Triage</h4>
          <div class="grid grid-cols-3 gap-1 text-xs">
            {#each triageStates as st}
              <button
                onclick={() => setTriage(st)}
                disabled={busy}
                class="px-2 py-1 rounded {(task.triage || 'inbox') === st ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1'}"
              >{st}</button>
            {/each}
          </div>
        </section>

        <!-- Recurrence -->
        <section>
          <h4 class="text-[11px] uppercase tracking-wider text-dim mb-1.5">Recurrence</h4>
          <div class="flex flex-wrap gap-1 text-xs">
            {#each recurrenceOptions as o}
              <button
                onclick={() => setRecurrence(o.value)}
                disabled={busy}
                class="px-2.5 py-1 rounded {recurrenceBuf === o.value ? 'bg-info text-mantle' : 'bg-surface0 text-subtext hover:bg-surface1'}"
              >{o.label}</button>
            {/each}
          </div>
          <p class="text-[10px] text-dim mt-1">Writes a <code>#daily</code>/<code>#weekly</code>/etc. tag onto the task line.</p>
        </section>

        <!-- Project / Goal / Deadline links. Single-select per type;
             saving via patchTask round-trips through the markdown line
             (goal:Gxxx + deadline:<ulid> markers; projectId is sidecar
             metadata). Selecting "(none)" clears the link. -->
        <section>
          <h4 class="text-[11px] uppercase tracking-wider text-dim mb-1.5">Links</h4>
          <div class="space-y-2 text-xs">
            <label class="flex items-center gap-2">
              <span class="text-dim w-20 flex-shrink-0">Project</span>
              <select
                value={task.projectId ?? ''}
                onchange={(e) => setProject((e.currentTarget as HTMLSelectElement).value)}
                disabled={busy}
                class="flex-1 min-w-0 bg-surface0 border border-surface1 rounded px-2 py-1 text-text"
              >
                <option value="">(none)</option>
                {#each projects as p (p.name)}
                  <option value={p.name}>{p.name}</option>
                {/each}
              </select>
            </label>
            <label class="flex items-center gap-2">
              <span class="text-dim w-20 flex-shrink-0">Goal</span>
              <select
                value={task.goalId ?? ''}
                onchange={(e) => setGoal((e.currentTarget as HTMLSelectElement).value)}
                disabled={busy}
                class="flex-1 min-w-0 bg-surface0 border border-surface1 rounded px-2 py-1 text-text"
              >
                <option value="">(none)</option>
                {#each goals as g (g.id)}
                  <option value={g.id}>{g.id} — {g.title}</option>
                {/each}
              </select>
            </label>
            <label class="flex items-center gap-2">
              <span class="text-dim w-20 flex-shrink-0">Deadline</span>
              <select
                value={task.deadlineId ?? ''}
                onchange={(e) => setDeadline((e.currentTarget as HTMLSelectElement).value)}
                disabled={busy}
                class="flex-1 min-w-0 bg-surface0 border border-surface1 rounded px-2 py-1 text-text"
              >
                <option value="">(none)</option>
                {#each deadlines as d (d.id)}
                  <option value={d.id}>{d.date} — {d.title}</option>
                {/each}
              </select>
            </label>
          </div>
        </section>

        <!-- Free-form notes -->
        <section>
          <h4 class="text-[11px] uppercase tracking-wider text-dim mb-1.5">Notes</h4>
          <textarea
            bind:value={notesBuf}
            onblur={commitNotes}
            placeholder="any details, links, context…"
            rows="4"
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
          ></textarea>
          <p class="text-[10px] text-dim mt-1">Stored in the task sidecar — not in the markdown.</p>
        </section>

        <!-- Read-only metadata -->
        <section class="pt-4 border-t border-surface1">
          <h4 class="text-[11px] uppercase tracking-wider text-dim mb-2">Metadata</h4>
          <dl class="text-xs space-y-1">
            <div class="flex gap-2"><dt class="text-dim w-24">ID</dt><dd class="text-text font-mono">{task.id}</dd></div>
            {#if task.createdAt}
              <div class="flex gap-2"><dt class="text-dim w-24">Created</dt><dd class="text-text">{fmtDate(task.createdAt)}</dd></div>
            {/if}
            {#if task.completedAt}
              <div class="flex gap-2"><dt class="text-dim w-24">Completed</dt><dd class="text-text">{fmtDate(task.completedAt)}</dd></div>
            {/if}
            {#if task.updatedAt}
              <div class="flex gap-2"><dt class="text-dim w-24">Updated</dt><dd class="text-text">{fmtDate(task.updatedAt)}</dd></div>
            {/if}
            {#if task.dependsOn && task.dependsOn.length}
              <div class="flex gap-2"><dt class="text-dim w-24">Depends on</dt><dd class="text-text">{task.dependsOn.join(', ')}</dd></div>
            {/if}
          </dl>
        </section>
      </div>
    </div>
  {/if}
</Drawer>
