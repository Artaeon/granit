// AI status summary controller for the venture detail page.
//
// Second extraction step out of routes/ventures/[name]/+page.svelte.
// Owns the streaming AI summary panel: the busy/text/error tri-state,
// the in-flight AbortController, the venture snapshot builder, and
// the start (`summarize`) / cancel-partial (`cancel`) / wipe
// (`dismiss`) trio.
//
// Follows the project-wide AI streamer convention: cancel() abandons
// the in-flight request but keeps the partial transcript, while
// dismiss() also clears the panel. The page only needs the dismiss
// behavior today; cancel() is here for the share-pass (a future
// "Stop" button next to "Regenerate"), in line with the rest of the
// codebase's AI controllers.
//
// The snapshot trims free-text fields and caps each list so the
// prompt stays compact — the model sees the shape, not the entire
// history. Lists are read through a getter the page passes in, so
// the controller always sees the current rollups (active deadlines
// in particular are derived in the page; we don't recompute here).

import {
  api,
  type Venture,
  type Project,
  type Goal,
  type Deadline,
  type PrayerIntention
} from '$lib/api';
import { daysUntil } from '$lib/deadlines/util';
import { isAbortError } from '$lib/util/aiErrors';

export interface VenturesDetailAISummaryDeps {
  /** Returns the venture currently displayed, or null while loading
   *  / not-found. summarize() no-ops on null. */
  getVenture: () => Venture | null;
  /** All projects under this venture (already filtered by the page). */
  getProjects: () => Project[];
  /** All goals under this venture. */
  getGoals: () => Goal[];
  /** Active (non-met / non-cancelled) deadlines — the panel uses the
   *  active subset because met deadlines don't inform the next-focus
   *  recommendation the model writes. */
  getActiveDeadlines: () => Deadline[];
  /** Active (status === 'praying') intentions, same rationale. */
  getActiveIntentions: () => PrayerIntention[];
}

export interface VenturesDetailAISummaryController {
  /** True while a stream is in flight. Drives the trigger button label
   *  (`thinking…` → `regenerate`). */
  readonly busy: boolean;
  /** Accumulated streamed prose. Cleared by dismiss(); preserved by
   *  cancel(). Empty string when no summary has been requested yet. */
  readonly text: string;
  /** Last error message from the stream, or '' when none. The error
   *  state is mutually exclusive with text — the page renders one or
   *  the other. */
  readonly error: string;

  /** Kick off a new summary. Aborts any in-flight request so a fast
   *  "regenerate" tap doesn't race with the previous stream. No-ops
   *  if the venture is unloaded or a stream is already running. */
  summarize(): Promise<void>;
  /** Abort the in-flight request but keep the partial transcript on
   *  screen. Surfaced for the future Stop button. */
  cancel(): void;
  /** Abort and wipe — used by the panel's × close button. */
  dismiss(): void;
}

export function createVenturesDetailAISummary(
  deps: VenturesDetailAISummaryDeps
): VenturesDetailAISummaryController {
  let busy = $state(false);
  let text = $state('');
  let error = $state('');
  let abort: AbortController | null = null;

  // Build a compact JSON snapshot of everything the model needs to write
  // a concise narrative. We omit free-text fields that could explode
  // the prompt (project notes, goal review_log) and cap each list to
  // a sensible number — the model gets the shape, not the entire
  // history.
  function buildSnapshot(): string {
    const venture = deps.getVenture();
    if (!venture) return '{}';
    const projects = deps.getProjects();
    const goals = deps.getGoals();
    const activeDeadlines = deps.getActiveDeadlines();
    const activeIntentions = deps.getActiveIntentions();
    return JSON.stringify(
      {
        venture: {
          name: venture.name,
          mission: venture.mission || undefined,
          description: venture.description || undefined,
          status: venture.status ?? 'active',
          tags: venture.tags ?? undefined
        },
        projects: projects.slice(0, 20).map((p) => ({
          name: p.name,
          status: p.status ?? 'active',
          kind: p.kind || undefined,
          description: p.description ? p.description.slice(0, 200) : undefined,
          progress: p.progress ?? undefined,
          tasksOpen: (p.tasksTotal ?? 0) - (p.tasksDone ?? 0),
          tasksDone: p.tasksDone ?? 0,
          dueDate: p.due_date || undefined,
          nextAction: p.next_action || undefined
        })),
        goals: goals.slice(0, 20).map((g) => ({
          title: g.title,
          status: g.status ?? 'active',
          targetDate: g.target_date || undefined,
          milestonesDone: (g.milestones ?? []).filter((m) => m.done).length,
          milestonesTotal: (g.milestones ?? []).length
        })),
        deadlines: activeDeadlines.slice(0, 10).map((d) => ({
          title: d.title,
          date: d.date,
          daysUntil: daysUntil(d.date),
          importance: d.importance || undefined
        })),
        prayerIntentions: activeIntentions.slice(0, 10).map((p) => ({
          text: p.text.slice(0, 160),
          startedAt: p.started_at || undefined
        }))
      },
      null,
      2
    );
  }

  async function summarize() {
    const venture = deps.getVenture();
    if (!venture || busy) return;
    abort?.abort();
    abort = new AbortController();
    busy = true;
    error = '';
    text = '';
    const snap = buildSnapshot();
    const system =
      'You are a concise venture analyst. The user will give you a JSON snapshot of one venture (mission, projects with progress, goals with milestones, upcoming deadlines, prayer intentions). Write a brief plain-prose status summary in 4-6 sentences. Lead with momentum (where things are moving), name 1-2 specific risks or near-term deadlines, and end with a single suggested next focus. Be specific — reference real names and numbers. No markdown, no bullets, no headers. Plain paragraphs only.';
    const user = `Venture snapshot:\n\n\`\`\`json\n${snap}\n\`\`\`\n\nWrite the status summary now.`;
    let buf = '';
    try {
      await api.chatStream(
        [
          { role: 'system', content: system },
          { role: 'user', content: user }
        ],
        undefined,
        {
          onChunk: (c) => {
            // Gate on abort — dismiss() wipes text; without this, an
            // in-flight onChunk fires once more after the abort and
            // re-populates text from the buffer.
            if (abort?.signal.aborted) return;
            buf += c;
            text = buf;
          },
          onDone: () => {},
          onError: (err) => {
            if (isAbortError(err)) return;
            error = err.message;
          }
        },
        abort.signal
      );
    } finally {
      busy = false;
      abort = null;
    }
  }

  function cancel() {
    abort?.abort();
    abort = null;
    busy = false;
  }

  function dismiss() {
    abort?.abort();
    abort = null;
    busy = false;
    text = '';
    error = '';
  }

  return {
    get busy() {
      return busy;
    },
    get text() {
      return text;
    },
    get error() {
      return error;
    },
    summarize,
    cancel,
    dismiss
  };
}
