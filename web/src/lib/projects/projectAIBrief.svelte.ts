// AI "project brief" controller for ProjectDetail.
//
// Streams a one-paragraph project brief the user can paste into the
// description field. The model is told to cover three things in
// order — what the project IS, what "done" looks like, who depends
// on it — in plain prose, no bullets, no preamble, no corporate
// sludge. Tasks are the truth signal; the model is steered away
// from making up scope from a fancy project name.
//
// Distinct from projectAIHealth (which renders a structured JSON
// verdict): the brief is plain prose for human curation. The user
// reads the streamed paragraph, then chooses to apply it to the
// description field or dismiss it; this controller never silently
// overwrites.

import { api, type Goal, type Project, type Task } from '$lib/api';
import { rafThrottle } from '$lib/util/streamThrottle';
import { isAbortError } from '$lib/util/aiErrors';

export interface ProjectAIBriefController {
  readonly aiBrief: string;
  readonly aiBriefBusy: boolean;
  readonly aiBriefError: string;
  readonly aiBriefSaving: boolean;

  run(): Promise<void>;
  /** Apply the current brief to the project's description field via
   *  the caller-supplied patch. Returns whether the save succeeded;
   *  on success the brief + error clear so the panel collapses. */
  apply(): Promise<void>;
  /** Abort + clear the brief + error. The dismiss path. */
  dismiss(): void;
  /** Abort the stream WITHOUT clearing (Stop CTA — Same convention as
   *  the other Stop/Close pairs in the codebase). */
  cancel(): void;
}

export interface ProjectAIBriefDeps {
  getProject: () => Project;
  getOpenTasks: () => Task[];
  getDoneTasks: () => Task[];
  getLinkedGoals: () => Goal[];
  /** Caller-supplied patch — usually the parent's `patch({
   *  description })` wrapper that talks to api.patchProject + reloads.
   *  Returns whether the save succeeded; controller clears the brief
   *  on true. */
  applyDescription: (text: string) => Promise<boolean>;
}

export function createProjectAIBrief(deps: ProjectAIBriefDeps): ProjectAIBriefController {
  let aiBrief = $state('');
  let aiBriefBusy = $state(false);
  let aiBriefError = $state('');
  let aiBriefSaving = $state(false);
  let aiBriefAbort: AbortController | null = null;

  async function run() {
    if (aiBriefBusy) return;
    aiBriefBusy = true;
    aiBriefError = '';
    aiBrief = '';
    aiBriefAbort = new AbortController();
    // rAF throttle so the live brief render isn't repainted per token.
    // The apply lambda gates on abort: after dismiss() aborts the
    // stream, a queued rAF frame can still fire and would write
    // back into aiBrief AFTER the wipe — producing a one-frame
    // phantom. Gating on signal.aborted closes that race.
    const briefT = rafThrottle((full) => {
      if (aiBriefAbort?.signal.aborted) return;
      aiBrief = full;
    });

    const project = deps.getProject();
    const openTasks = deps.getOpenTasks();
    const doneTasks = deps.getDoneTasks();
    const linkedGoals = deps.getLinkedGoals();

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

    // Tight system prompt: one paragraph, three things in order, no
    // bullets, no preamble, no invented stakeholders. The "tasks are
    // the truth, not the name" line steers the model away from making
    // up scope from a fancy project name.
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
          onChunk: briefT.onChunk,
          onDone: () => { briefT.flush(); },
          onError: (err) => {
            briefT.flush();
            if (isAbortError(err)) return;
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

  // Stop — abort the stream but KEEP the partial aiBrief for
  // retry. Flip busy + null abort synchronously so the UI swaps
  // to "rerun" instantly; without this the button lags until
  // chatStream's finally settles.
  function cancel() {
    aiBriefAbort?.abort();
    aiBriefAbort = null;
    aiBriefBusy = false;
  }

  async function apply() {
    const text = aiBrief.trim();
    if (!text) return;
    aiBriefSaving = true;
    try {
      const ok = await deps.applyDescription(text);
      if (ok) {
        aiBrief = '';
        aiBriefError = '';
      }
    } finally {
      aiBriefSaving = false;
    }
  }

  // Close — abort + wipe. abort first so a queued rafThrottle
  // frame can't repopulate aiBrief after we clear it.
  function dismiss() {
    aiBriefAbort?.abort();
    aiBriefAbort = null;
    aiBriefBusy = false;
    aiBrief = '';
    aiBriefError = '';
  }

  return {
    get aiBrief() { return aiBrief; },
    get aiBriefBusy() { return aiBriefBusy; },
    get aiBriefError() { return aiBriefError; },
    get aiBriefSaving() { return aiBriefSaving; },
    run,
    apply,
    cancel,
    dismiss
  };
}
