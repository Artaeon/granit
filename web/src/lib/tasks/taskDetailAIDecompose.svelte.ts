// AI Decompose controller for the TaskDetail drawer.
//
// Takes the parent task's title + notes and asks the model for 3-7
// small, concrete sub-tasks. Renders proposals with per-row accept /
// skip / accept-all; accepting calls api.createTask in the parent's
// notePath so the subtask shows up in the same daily/project note.
// Goes through chatStream so audit / sabbath / redaction / cost all
// apply.
//
// Stream lifecycle: Stop (cancel) keeps any partial proposals on
// screen so the user can still salvage them. Close (dismiss) wipes
// the proposals + the raw fallback buffer. This matches the
// Stop / Close convention used by other AI surfaces in the codebase.
//
// PARENTING: `parentLine: task.lineNum` on createTask makes the new
// task an actual INDENTED subtask of the parent. Before this was
// wired through CreateOpts.ParentLine, subtasks landed flat at the
// bottom of the note and looked like sibling tasks — the bug the
// user kept calling out ("subtasks shown as habits"). Now they're
// indented two columns deeper than the parent and the page's
// existing parentMap / collapse chevron pick them up automatically.

import { api, type Task } from '$lib/api';
import { toast } from '$lib/components/toast';
import { rafThrottle } from '$lib/util/streamThrottle';

export type DecomposeSubtask = {
  text: string;
  estimateMinutes?: number;
  rationale?: string;
};

export interface TaskDetailAIDecomposeController {
  readonly busy: boolean;
  readonly error: string;
  readonly raw: string;
  readonly subtasks: DecomposeSubtask[];
  /** Index of the row currently being POSTed (so the row can show
   *  "…" instead of "add" and we can disable sibling rows). -1 = idle. */
  readonly acceptingIdx: number;

  /** Start streaming. No-op if a stream is already in flight. */
  run(): Promise<void>;
  /** Stop the in-flight stream. Keeps the partial proposals on screen
   *  (Stop semantics — user can still salvage what arrived). */
  cancel(): void;
  /** Close the proposals: wipe subtasks + raw + error. */
  dismiss(): void;
  /** Reset for a fresh task target (also aborts any in-flight stream
   *  so a previous task's response can't leak into the next drawer). */
  reset(): void;

  /** Create one proposed subtask in the parent's notePath. Removes
   *  it from the proposals list on success. */
  accept(idx: number): Promise<void>;
  /** Drop a proposal without creating it. */
  skip(idx: number): void;
  /** Walk the proposals list accepting index 0 each iteration — the
   *  array shrinks as each call resolves so this drains it. */
  acceptAll(): Promise<void>;
}

export type TaskDetailAIDecomposeDeps = {
  getTask: () => Task | null;
  onChanged: () => void | Promise<void>;
  /** Called after a proposal is accepted so the parent can refresh
   *  the "existing subtasks" list to include the newly-created child. */
  onAccepted: () => void | Promise<void>;
};

/** Extract the first balanced JSON object from a streamed reply.
 *  Tolerates fenced (```json … ```) and unfenced replies and partial
 *  streams — returns null if the stream hasn't produced a closing
 *  brace yet so the caller skips the parse. */
export function extractDecompJson(s: string): string | null {
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

const DECOMP_SYSTEM_PROMPT =
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

export function createTaskDetailAIDecompose(deps: TaskDetailAIDecomposeDeps): TaskDetailAIDecomposeController {
  let busy = $state(false);
  let error = $state('');
  let raw = $state('');
  let subtasks = $state<DecomposeSubtask[]>([]);
  let acceptingIdx = $state<number>(-1);
  let abort: AbortController | null = null;

  async function run() {
    const task = deps.getTask();
    if (!task || busy) return;
    busy = true;
    error = '';
    raw = '';
    subtasks = [];
    abort = new AbortController();
    // Best-effort dedup hint — fetch open tasks in the same note so the
    // prompt can be told "don't propose duplicates of these". Failure
    // just means the hint is missing, not that we crash.
    let existingSiblings: string[] = [];
    try {
      const r = await api.listTasks({ status: 'open' });
      existingSiblings = r.tasks
        .filter((t) => t.notePath === task.notePath && t.id !== task.id)
        .map((t) => t.text)
        .slice(0, 30);
    } catch {}
    const user =
      `Parent task: ${task.text}\n` +
      (task.notes ? `\nParent notes:\n${task.notes}\n` : '') +
      (existingSiblings.length > 0
        ? `\nExisting siblings in the same note (do NOT duplicate):\n${existingSiblings.map((s) => '- ' + s).join('\n')}\n`
        : '') +
      '\nReturn the strict JSON now.';
    try {
      // rAF throttle — raw + the JSON-parse + filter previously ran
      // on every token. Streaming a 50-subtask decomposition through
      // a fast model would freeze the dialog because the proposals
      // list re-rendered per token.
      const decompT = rafThrottle((full) => {
        raw = full;
        const block = extractDecompJson(full);
        if (block) {
          try {
            const parsed = JSON.parse(block) as { subtasks?: DecomposeSubtask[] };
            if (Array.isArray(parsed.subtasks)) {
              subtasks = parsed.subtasks.filter(
                (s) => s && typeof s.text === 'string' && s.text.trim().length > 0
              );
            }
          } catch {}
        }
      });
      await api.chatStream(
        [
          { role: 'system', content: DECOMP_SYSTEM_PROMPT },
          { role: 'user', content: user }
        ],
        task.notePath,
        {
          onChunk: decompT.onChunk,
          onDone: () => { decompT.flush(); },
          onError: (err) => { decompT.flush(); error = err.message; }
        },
        abort.signal
      );
    } finally {
      busy = false;
      abort = null;
    }
  }

  // Stop — abort the in-flight stream but KEEP the partial output +
  // error so the user can retry without losing context. Flip busy +
  // null abort synchronously so the "cancel" button swaps to
  // "regenerate" instantly; without these the UI stays stuck on
  // cancel until chatStream's `finally` block settles (which can
  // lag if the abort is mid-await).
  function cancel() {
    abort?.abort();
    abort = null;
    busy = false;
  }

  // Close — abort + wipe. Without the abort, an in-flight stream's
  // rafThrottle would keep writing into raw/subtasks after we
  // cleared them, leaving a half-replaced phantom.
  function dismiss() {
    abort?.abort();
    abort = null;
    busy = false;
    raw = '';
    error = '';
    subtasks = [];
  }

  function reset() {
    abort?.abort();
    abort = null;
    busy = false;
    raw = '';
    error = '';
    subtasks = [];
    acceptingIdx = -1;
  }

  async function accept(idx: number) {
    const task = deps.getTask();
    if (!task) return;
    const s = subtasks[idx];
    if (!s) return;
    acceptingIdx = idx;
    try {
      // Estimate goes through the est:Nm marker the parser already
      // understands so existing taskParse round-trips keep working.
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
      // projectId isn't a CreateOpts field — patch it after creation
      // so the new child inherits the parent's project sidecar.
      if (task.projectId && created?.id) {
        try {
          await api.patchTask(created.id, { projectId: task.projectId });
        } catch {}
      }
      subtasks = subtasks.filter((_, i) => i !== idx);
      await deps.onChanged();
      await deps.onAccepted();
      toast.success('Subtask added');
    } catch (e) {
      toast.error('Add failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      acceptingIdx = -1;
    }
  }

  function skip(idx: number) {
    subtasks = subtasks.filter((_, i) => i !== idx);
  }

  async function acceptAll() {
    // Always accept index 0 — the array shrinks as each call resolves.
    // Bail-on-failure guard: if accept() catches its error WITHOUT
    // shrinking the array (network failure, server reject), this
    // loop would spin forever and freeze the drawer. Snapshot the
    // length before each iteration and break if it didn't decrease.
    while (subtasks.length > 0) {
      const before = subtasks.length;
      await accept(0);
      if (subtasks.length >= before) break;
    }
  }

  return {
    get busy() { return busy; },
    get error() { return error; },
    get raw() { return raw; },
    get subtasks() { return subtasks; },
    get acceptingIdx() { return acceptingIdx; },
    run,
    cancel,
    dismiss,
    reset,
    accept,
    skip,
    acceptAll
  };
}
