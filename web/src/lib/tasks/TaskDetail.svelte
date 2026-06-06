<script lang="ts">
  import { goto } from '$app/navigation';
  import { api, type Task } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { cleanTaskText } from '$lib/util/taskParse';
  import Drawer from '$lib/components/Drawer.svelte';
  import { openAIOverlay } from '$lib/stores/ai-overlay';
  import { rafThrottle } from '$lib/util/streamThrottle';
  import { loadDraft, clearDraft, makeDraftWriter } from '$lib/util/draftAutosave';
  import {
    recurrenceOptions,
    triageStates,
    fmtDate,
    snoozeOffset,
    buildAskAIPrompt
  } from './taskDetailHelpers';
  import { createTaskDetailLinks } from './taskDetailLinks.svelte';
  import { createTaskDetailSubtasks } from './taskDetailSubtasks.svelte';

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

  // Opens the AIOverlay with this task's context pre-loaded. send=false
  // so the user can edit the composer before submitting — the model
  // already has enough context to answer "help me break this down" or
  // "draft a plan" without the user re-stating the task.
  function askAIAboutThisTask(): void {
    if (!task) return;
    openAIOverlay({ text: buildAskAIPrompt(task), send: false });
  }

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

  // Linkable-entity lookup for the Project / Goal / Deadline <select>s.
  // Lazy-loaded on first open per session; see taskDetailLinks.svelte.ts.
  const links = createTaskDetailLinks();
  const projects = $derived(links.projects);
  const goals = $derived(links.goals);
  const deadlines = $derived(links.deadlines);

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
      // rAF throttle — aiDecompRaw + the JSON-parse + filter ran on
      // every token. Stream a 50-subtask decomposition through a
      // fast model and the per-token re-render of the proposals
      // list freezes the dialog.
      const decompT = rafThrottle((full) => {
        aiDecompRaw = full;
        const block = extractDecompJson(full);
        if (block) {
          try {
            const parsed = JSON.parse(block) as { subtasks?: Subtask[] };
            if (Array.isArray(parsed.subtasks)) {
              aiDecompSubtasks = parsed.subtasks
                .filter((s) => s && typeof s.text === 'string' && s.text.trim().length > 0);
            }
          } catch {}
        }
      });
      await api.chatStream(
        [
          { role: 'system', content: system },
          { role: 'user', content: user }
        ],
        task.notePath,
        {
          onChunk: decompT.onChunk,
          onDone: () => { decompT.flush(); },
          onError: (err) => { decompT.flush(); aiDecompError = err.message; }
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
  //
  // PARENTING: `parentLine: task.lineNum` makes the new task an actual
  // INDENTED subtask of the parent. Before this was wired through the
  // store's CreateOpts.ParentLine, subtasks landed flat at the bottom
  // of the note and looked like sibling tasks — which is the bug the
  // user kept calling out ("subtasks shown as habits"). Now they're
  // indented two columns deeper than the parent, and the page's
  // existing parentMap / collapse chevron pick them up automatically.
  async function acceptSubtask(idx: number) {
    if (!task) return;
    const s = aiDecompSubtasks[idx];
    if (!s) return;
    aiDecompAcceptingIdx = idx;
    try {
      let text = s.text.trim();
      if (s.estimateMinutes && s.estimateMinutes > 0) {
        text = `${text} est:${s.estimateMinutes}m`;
      }
      const created = await api.createTask({
        notePath: task.notePath,
        text,
        goalId: task.goalId,
        deadlineId: task.deadlineId,
        parentLine: task.lineNum
      });
      if (task.projectId && created?.id) {
        try {
          await api.patchTask(created.id, { projectId: task.projectId });
        } catch {}
      }
      aiDecompSubtasks = aiDecompSubtasks.filter((_, i) => i !== idx);
      await onChanged?.();
      await loadSubtasks();
      toast.success('Subtask added');
    } catch (e) {
      toast.error('Add failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      aiDecompAcceptingIdx = -1;
    }
  }

  // Subtask list + manual add + toggle / delete behaviour, plus the
  // generation-guarded reload that protects against drawer re-target
  // races. See taskDetailSubtasks.svelte.ts.
  const subtasksCtl = createTaskDetailSubtasks({
    getTask: () => task,
    onChanged: () => onChanged?.()
  });
  const subtasks = $derived(subtasksCtl.subtasks);
  const subtasksLoaded = $derived(subtasksCtl.loaded);
  async function loadSubtasks() { await subtasksCtl.load(); }

  // ── Archive (soft-delete) ─────────────────────────────────────────
  // Markdown line stays intact; sidecar Archived flag flips. List
  // views hide the task by default (?includeArchived=true reveals
  // them). Reversible — Unarchive flips it back. The drawer stays
  // open after archive so the user can confirm + unarchive if they
  // mis-clicked, then they close it themselves.
  async function archiveTask() {
    if (!task || busy) return;
    if (!confirm('Archive this task? It stays in the note file but is hidden from default lists.')) return;
    await patch({ archived: true });
    toast.success('Archived');
  }
  async function unarchiveTask() {
    if (!task || busy) return;
    await patch({ archived: false });
    toast.success('Restored');
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

  // Resync local buffers whenever the modal opens for a different task.
  // Also reset Decompose state — proposals from a previous task
  // shouldn't leak into the next one's drawer.
  // Track which task we last initialised for. Without this, every
  // state write inside the effect re-fires it (Svelte 5 re-runs the
  // effect when `open` or `task` reactive reads change), and the
  // resetters fight with the user's typing in notes / title / etc.
  let lastInitialisedTaskId: string | null = null;
  $effect(() => {
    if (!open || !task) {
      lastInitialisedTaskId = null;
      return;
    }
    if (task.id === lastInitialisedTaskId) return;
    lastInitialisedTaskId = task.id;
    // Prefer a saved draft for notes — that's the most recent intent.
    // Title draft is loaded only when the user actually opens the
    // title editor (titleEditing toggle); otherwise the displayed
    // title comes from the canonical task.text.
    const notesDraft = loadDraft<string | null>(`task.notes.${task.id}`, null);
    notesBuf = (notesDraft !== null && notesDraft !== '') ? notesDraft : (task.notes ?? '');
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
    subtasksCtl.reset();
    void links.load();
    void subtasksCtl.load();
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

  // Draft autosave for the two long-form buffers in this drawer
  // (notes + title). The other buffers — due, scheduled date/time,
  // recurrence — are short controls that commit on change; their
  // loss-on-reload exposure is bounded to "the keystroke you just
  // typed", not paragraphs of work. Keep the drafts per-task-id so
  // switching tasks doesn't cross-contaminate.
  const notesDraftWriter = makeDraftWriter(400);
  const titleDraftWriter = makeDraftWriter(400);
  function notesDraftKey() { return task ? `task.notes.${task.id}` : ''; }
  function titleDraftKey() { return task ? `task.title.${task.id}` : ''; }

  // Notes textarea is always rendered (no edit toggle) so we gate the
  // draft write on "actually differs from server state" instead of
  // an editing flag. Without this gate every task open would write
  // a stale-equal draft, accumulating localStorage entries forever.
  $effect(() => {
    if (!task || !notesDraftKey()) return;
    if (notesBuf === (task.notes ?? '')) return;
    notesDraftWriter.save(notesDraftKey(), notesBuf);
  });
  $effect(() => {
    if (!task || !titleEditing || !titleDraftKey()) return;
    if (titleBuf === cleanTaskText(task.text)) return;
    titleDraftWriter.save(titleDraftKey(), titleBuf);
  });

  async function commitNotes() {
    if (!task) return;
    if (notesBuf === (task.notes ?? '')) {
      // No change to persist — but drop the draft so it doesn't
      // resurrect on next mount with stale-equal content.
      clearDraft(notesDraftKey());
      notesDraftWriter.cancel();
      return;
    }
    await patch({ notes: notesBuf });
    clearDraft(notesDraftKey());
    notesDraftWriter.cancel();
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
    if (!next || next === cleanTaskText(task.text)) {
      titleEditing = false;
      clearDraft(titleDraftKey());
      titleDraftWriter.cancel();
      return;
    }
    titleEditing = false;
    await patch({ text: next });
    clearDraft(titleDraftKey());
    titleDraftWriter.cancel();
  }
  function cancelTitleEdit() {
    if (task) titleBuf = cleanTaskText(task.text);
    titleEditing = false;
    clearDraft(titleDraftKey());
    titleDraftWriter.cancel();
  }
  // When the user opens the title editor, prefer a stale draft over
  // the canonical title text. Called from the onclick that flips
  // titleEditing → true.
  function startTitleEdit() {
    if (!task) return;
    const draft = loadDraft<string | null>(titleDraftKey(), null);
    titleBuf = (draft !== null && draft !== '') ? draft : cleanTaskText(task.text);
    titleEditing = true;
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

  // Snooze quick-actions. Sets snoozedUntil to a YYYY-MM-DDTHH:MM
  // local-time string (built by snoozeOffset) + flips triage to
  // 'snoozed'. Wall-clock semantics so the timing matches the user's
  // intent without TZ arithmetic.
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

</script>

<div class="task-detail-shell">
<Drawer bind:open side="right" responsive width="w-full sm:w-96 md:w-[28rem]">
  {#if task}
    <div class="h-full flex flex-col overflow-hidden">
      <!-- Mobile-only drag handle. Purely cosmetic affordance —
           tells the user this is a bottom-sheet on phones. Real
           drag-to-dismiss is out of scope for this stream; the
           backdrop tap + close button still own dismissal. -->
      <div class="sm:hidden flex justify-center pt-2 pb-1 flex-shrink-0" aria-hidden="true">
        <span class="block w-10 h-1 bg-surface2 rounded-full"></span>
      </div>
      <header class="sticky top-0 z-10 bg-mantle px-3 py-2 border-b border-surface1 flex items-center gap-2 flex-shrink-0">
        <h2 class="text-sm font-semibold text-text flex-1 truncate">Task details</h2>
        {#if busy}
          <span class="text-[10px] text-dim italic" aria-live="polite">saving…</span>
        {/if}
        {#if snoozeActive}
          <span class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-surface0 text-info border border-info" title={`snoozed until ${task?.snoozedUntil}`}>snoozed</span>
        {/if}
        <!-- Ask AI launcher: opens the AIOverlay pre-seeded with the
             task's title + scheduling + notes as context so the user
             can ask "help me decompose this" or "draft a plan" with
             the model already grounded. send=false so the user can
             edit the prompt before submitting. -->
        <button
          onclick={askAIAboutThisTask}
          title="Ask AI about this task"
          aria-label="ask ai about this task"
          class="text-[11px] px-2 py-1 rounded bg-surface0 border border-surface1 text-subtext hover:border-primary hover:text-primary transition-colors"
        >Ask AI</button>
        <button onclick={close} aria-label="close" class="text-dim hover:text-text text-xl leading-none">×</button>
      </header>

      <div class="flex-1 overflow-y-auto p-2 sm:p-3 space-y-3">
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
                onclick={startTitleEdit}
                class="text-base font-medium text-text break-words text-left w-full hover:bg-surface1 rounded px-2 py-1 -mx-2 -my-1 transition-colors {task.done ? 'line-through text-dim' : ''}"
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

        <!-- Subtasks. Shows every direct child (task in the same
             note whose parentLine matches THIS task's lineNum) plus
             a one-line input to add a new subtask manually. The
             input writes through createTask with parentLine set, so
             the new line ends up INDENTED in the markdown — a real
             subtask, not a flat sibling. -->
        <section>
          <div class="flex items-baseline gap-2 mb-1.5">
            <h4 class="text-[11px] uppercase tracking-wider text-dim flex-1">
              Subtasks
              {#if subtasks.length > 0}
                <span class="text-text font-mono tabular-nums ml-1">({subtasks.length})</span>
              {/if}
            </h4>
            {#if subtasksLoaded && subtasks.length > 0}
              <span class="text-[10px] text-dim">
                {subtasks.filter((s) => s.done).length}/{subtasks.length} done
              </span>
            {/if}
          </div>
          {#if subtasks.length > 0}
            <ul class="space-y-0.5 mb-2">
              {#each subtasks as s (s.id)}
                <li class="flex items-center gap-2 text-xs group hover:bg-surface0 rounded px-1 py-0.5 -mx-1">
                  <button
                    type="button"
                    onclick={() => void subtasksCtl.toggleDone(s)}
                    class="w-3.5 h-3.5 rounded border flex items-center justify-center flex-shrink-0
                      {s.done ? 'bg-success border-success' : 'border-surface2 hover:border-primary'}"
                    aria-label={s.done ? 'mark not done' : 'mark done'}
                  >
                    {#if s.done}
                      <svg viewBox="0 0 12 12" class="w-2.5 h-2.5 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
                    {/if}
                  </button>
                  <span class="flex-1 min-w-0 break-words {s.done ? 'line-through text-dim' : 'text-text'}">
                    {cleanTaskText(s.text)}
                  </span>
                  {#if s.priority}
                    <span class="text-[10px] font-mono px-1 rounded bg-surface0 text-dim flex-shrink-0">P{s.priority}</span>
                  {/if}
                  <button
                    type="button"
                    onclick={() => void subtasksCtl.remove(s)}
                    class="text-dim hover:text-error text-base leading-none flex-shrink-0 opacity-0 group-hover:opacity-100 transition-opacity"
                    aria-label="delete subtask"
                    title="delete subtask"
                  >×</button>
                </li>
              {/each}
            </ul>
          {/if}
          <div class="flex items-center gap-1.5">
            <input
              type="text"
              bind:value={subtasksCtl.manualBuf}
              onkeydown={(e) => {
                if (e.key === 'Enter') { e.preventDefault(); void subtasksCtl.addManual(); }
                else if (e.key === 'Escape') { subtasksCtl.manualBuf = ''; }
              }}
              disabled={subtasksCtl.manualBusy}
              placeholder="Add subtask (Enter)"
              autocomplete="off"
              class="flex-1 min-w-0 px-2 py-1 bg-surface0 border border-surface1 rounded text-xs text-text focus:outline-none focus:border-primary disabled:opacity-50"
            />
            <button
              type="button"
              onclick={() => void subtasksCtl.addManual()}
              disabled={!subtasksCtl.manualBuf.trim() || subtasksCtl.manualBusy}
              class="px-2 py-1 text-xs bg-primary text-on-primary rounded font-medium hover:opacity-90 disabled:opacity-50 flex-shrink-0"
              title="Add subtask (Enter)"
            >+</button>
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
                class="text-[11px] px-2 py-0.5 bg-surface1 text-secondary rounded hover:bg-surface2"
                title="AI proposes 3-7 concrete sub-tasks"
              >✨ break it down</button>
            {/if}
          </div>
          {#if aiDecompError}
            <p class="text-xs text-error">{aiDecompError}</p>
          {:else if aiDecompSubtasks.length > 0}
            <div class="bg-surface1 border border-surface2 rounded p-2 space-y-1.5">
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
                      class="px-2 py-0.5 bg-surface0 text-success rounded hover:bg-surface1 disabled:opacity-50 flex-shrink-0"
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
            <div class="flex items-baseline gap-2 mb-2 px-2 py-1.5 bg-surface0 border border-info rounded">
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

        <!-- Archive (soft-delete). The markdown line stays in the
             note file untouched — only the sidecar Archived flag
             flips. Default list views hide archived tasks; the page's
             "Show archived" toggle reveals them. Reversible via the
             Unarchive button that appears in place when archived. -->
        <section class="pt-3 border-t border-surface1">
          <h4 class="text-[11px] uppercase tracking-wider text-dim mb-1.5">Archive</h4>
          {#if task.archived}
            <div class="flex items-center gap-2">
              <span class="flex-1 text-xs text-warning">
                Archived
                {#if task.archivedAt}
                  <span class="text-dim font-mono">· {fmtDate(task.archivedAt)}</span>
                {/if}
              </span>
              <button
                type="button"
                onclick={() => void unarchiveTask()}
                disabled={busy}
                class="px-2 py-1 text-xs bg-surface0 text-success border border-surface1 hover:border-success rounded"
              >Restore</button>
            </div>
          {:else}
            <div class="flex items-center gap-2">
              <span class="flex-1 text-[11px] text-dim italic">
                Hides from lists. Markdown line stays in <span class="font-mono">{task.notePath.split('/').pop()}</span>.
              </span>
              <button
                type="button"
                onclick={() => void archiveTask()}
                disabled={busy}
                class="px-2 py-1 text-xs bg-surface0 text-warning border border-surface1 hover:border-warning rounded"
                title="Soft-delete — hide from lists, keep the markdown line"
              >Archive</button>
            </div>
          {/if}
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
</div>

<style>
  /* Mobile bottom-sheet override. Drawer.svelte hard-codes the
     aside as a right-edge slide-in (inset-y-0 + right-0 +
     translate-x-full). On phones a bottom-sheet feels native —
     overrides re-anchor the same aside to the bottom edge and
     swap the X-axis translate for a Y-axis one. Desktop layout
     stays untouched. The `:global` selector reaches into the
     child Drawer's <aside> through this wrapper.

     Drag-to-dismiss isn't wired here — purely a visual / layout
     pass. The existing close-on-escape + backdrop-tap dismissal
     paths still work because they live on the Drawer component
     itself, untouched. */
  @media (max-width: 639px) {
    .task-detail-shell :global(aside) {
      top: auto !important;
      bottom: 0 !important;
      left: 0 !important;
      right: 0 !important;
      width: 100% !important;
      max-width: 100% !important;
      max-height: 90vh;
      border-left: 0 !important;
      border-top: 1px solid var(--color-surface1);
      border-top-left-radius: 0.75rem;
      border-top-right-radius: 0.75rem;
      transform: translateY(100%);
    }
    .task-detail-shell :global(aside[aria-hidden='false']) {
      transform: translateY(0);
    }
  }
</style>
