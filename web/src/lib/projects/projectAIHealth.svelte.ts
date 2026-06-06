// AI "project health verdict" controller for ProjectDetail.
//
// Bundles the project's state — open/done tasks, recent completions,
// last completion age, linked goals — and asks the model for a
// structured 3-section verdict: momentum (alive/slowing/stalled/
// dead), blockers (what's actually stuck), and the single next
// concrete action.
//
// The model returns JSON so the panel renders the momentum badge as
// a coloured pill instead of fishing prose for keywords; the raw
// stream is also surfaced so a malformed response still shows
// something useful.
//
// Same Stop / Close distinction as the goals AI controllers — stop()
// aborts mid-stream and keeps the partial result + error visible for
// retry; dismiss() resets all state for the next run.

import { api, type Goal, type Project, type Task } from '$lib/api';
import { rafThrottle } from '$lib/util/streamThrottle';

export type HealthMomentum = 'alive' | 'slowing' | 'stalled' | 'dead';

export type HealthVerdict = {
  momentum: HealthMomentum;
  momentum_reason: string;
  blockers: string[];
  next_action: string;
};

export interface ProjectAIHealthController {
  readonly aiHealth: HealthVerdict | null;
  readonly aiHealthRaw: string;
  readonly aiHealthBusy: boolean;
  readonly aiHealthError: string;
  readonly aiHealthContextLine: string;

  run(): Promise<void>;
  /** Abort the in-flight stream WITHOUT clearing — the panel keeps
   *  whatever was streamed so the user can retry. */
  cancel(): void;
  /** Reset all state for a fresh run. */
  dismiss(): void;
}

export interface ProjectAIHealthDeps {
  /** Read the live project record. The controller doesn't store the
   *  project itself — the parent already has it as a prop, and we
   *  want the latest values at run-time. */
  getProject: () => Project;
  /** Open tasks for prompt context. */
  getOpenTasks: () => Task[];
  /** Done tasks for prompt context + the "days since last completion"
   *  signal that drives the momentum verdict. */
  getDoneTasks: () => Task[];
  /** Linked top-level goals for prompt context. */
  getLinkedGoals: () => Goal[];
  /** All tasks (open + done) — used only for the "tasks" plural
   *  noun in the context line. */
  getAllTasks: () => Task[];
}

export function createProjectAIHealth(deps: ProjectAIHealthDeps): ProjectAIHealthController {
  let aiHealth = $state<HealthVerdict | null>(null);
  let aiHealthRaw = $state('');
  let aiHealthBusy = $state(false);
  let aiHealthError = $state('');
  let aiHealthAbort: AbortController | null = null;
  // Context the model actually saw — surfaced above the result so the
  // user understands what the verdict is grounded in. Without this
  // the response feels like a black box; a "saw 12 tasks + 2 goals,
  // last completion 4d ago" line keeps the AI legible.
  let aiHealthContextLine = $state('');

  function daysSinceLastCompletion(): number | null {
    const doneTasks = deps.getDoneTasks();
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

  async function run() {
    if (aiHealthBusy) return;
    aiHealthBusy = true;
    aiHealthError = '';
    aiHealthRaw = '';
    aiHealth = null;
    aiHealthAbort = new AbortController();

    const project = deps.getProject();
    const openTasks = deps.getOpenTasks();
    const doneTasks = deps.getDoneTasks();
    const linkedGoals = deps.getLinkedGoals();
    const allTasks = deps.getAllTasks();

    const sinceLast = daysSinceLastCompletion();
    const dueOpen = openTasks.filter((t) => t.dueDate);
    const overdueOpen = dueOpen.filter((t) => {
      const d = new Date(t.dueDate as string);
      return !Number.isNaN(d.getTime()) && d.getTime() < Date.now();
    });
    aiHealthContextLine =
      `AI saw ${openTasks.length} open + ${doneTasks.length} done task${
        allTasks.length === 1 ? '' : 's'
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
    // declarative, and explicit about the JSON schema — a vague ask
    // gets a vague answer. Outlawing puffery ("synergy", "let's
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

    // rAF throttle so the pre-rendered raw stream doesn't re-render
    // the card per token.
    const healthT = rafThrottle((full) => { aiHealthRaw = full; });
    try {
      await api.chatStream(
        [
          { role: 'system', content: system },
          { role: 'user', content: user }
        ],
        undefined,
        {
          onChunk: healthT.onChunk,
          onDone: () => { healthT.flush(); },
          onError: (err) => { healthT.flush(); aiHealthError = err.message; }
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

  // Stop — abort the stream but KEEP the partial raw + error for
  // retry. Flip busy + null abort synchronously so the "stop"
  // button swaps to "rerun" instantly; without this the UI lags
  // until chatStream's finally settles (which can take a tick
  // when the abort fires mid-await).
  function cancel() {
    aiHealthAbort?.abort();
    aiHealthAbort = null;
    aiHealthBusy = false;
  }

  // Close — abort + wipe. The abort matters because rafThrottle
  // can still flush a queued frame after we clear aiHealthRaw,
  // producing a phantom one-frame rewrite.
  function dismiss() {
    aiHealthAbort?.abort();
    aiHealthAbort = null;
    aiHealthBusy = false;
    aiHealth = null;
    aiHealthRaw = '';
    aiHealthError = '';
    aiHealthContextLine = '';
  }

  return {
    get aiHealth() { return aiHealth; },
    get aiHealthRaw() { return aiHealthRaw; },
    get aiHealthBusy() { return aiHealthBusy; },
    get aiHealthError() { return aiHealthError; },
    get aiHealthContextLine() { return aiHealthContextLine; },
    run,
    cancel,
    dismiss
  };
}
