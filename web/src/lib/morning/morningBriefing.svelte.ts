// AI morning brief streamer for the ritual page.
//
// Fourth extraction step out of routes/morning/+page.svelte. Owns the
// 60-110 word "shape of the day" streamed brief: text, busy flag,
// error string, AbortController, plus the snapshot of the previous
// brief that gets shown dimmed while a regenerate is in flight.
//
// Three actions, each with the established semantics:
//   - run()      — start (or regenerate) the brief
//   - cancel()   — abort the in-flight call, keep the previous text
//   - dismiss()  — hide the brief panel for the rest of the day
//                  (persisted per-day in localStorage)
//
// AbortError handling: the onError callback filters DOMException
// AbortError so cancelling mid-stream doesn't surface a spurious
// "operation aborted" headline — same pattern as the other AI
// streamers (projectAIHealth, habitsAI). The cancel path resets
// busy + restores the previous brief before the onError can fire.
//
// Inputs (events / tasks / goals / deadlines) come from the data
// controller via getter deps so the prompt sees the freshest data
// each run() call without this controller holding a stale snapshot.

import type {
  ChatMessage,
  CalendarEvent,
  Task,
  Goal,
  Deadline
} from '$lib/api';
import { rafThrottle } from '$lib/util/streamThrottle';
import { classifyAiError, isAbortError } from '$lib/util/aiErrors';
import { errorMessage } from '$lib/util/errorMessage';
import {
  BRIEFING_SYSTEM_PROMPT,
  buildBriefingUserPrompt
} from './briefingPrompt';

type ChatStreamHandlers = {
  onChunk: (chunk: string) => void;
  onDone?: () => void;
  onError?: (err: Error) => void;
};

export interface MorningBriefingDeps {
  /** ISO date the brief is being generated for. Goes into the
   *  prompt + the per-day dismiss key. */
  todayISO: string;

  // Reactive getters — read at run() time so the prompt sees the
  // freshest data the data controller has loaded.
  getEvents: () => CalendarEvent[];
  getTasks: () => Task[];
  getGoals: () => Goal[];
  getDeadlines: () => { d: Deadline; days: number }[];

  /** chatStream binding — injected so unit tests can stub it without
   *  monkey-patching the api singleton. Mirror of api.chatStream. */
  chatStream: (
    messages: ChatMessage[],
    notePath: string | undefined,
    handlers: ChatStreamHandlers,
    signal?: AbortSignal
  ) => Promise<void>;
}

export interface MorningBriefingController {
  /** Currently rendered brief body. Streamed in via rafThrottle. */
  readonly text: string;
  /** Snapshot of the previous brief during a regenerate — the page
   *  renders it dimmed under the new one until the first token
   *  arrives. */
  readonly prev: string;
  readonly busy: boolean;
  readonly error: string;
  /** Per-day dismiss flag. Persisted to localStorage so reopening
   *  the page later today keeps the brief hidden. */
  dismissed: boolean;

  /** Hydrate the dismissed flag from localStorage. Called after the
   *  data load completes so it lines up with the rest of the
   *  per-day chrome. */
  hydrateDismissed(): void;

  run(): Promise<void>;
  cancel(): void;
  /** Hide the brief for the rest of the day. Best-effort persistence
   *  — private-mode / quota errors are swallowed; the flag still
   *  sticks in memory until reload. */
  dismiss(): void;
}

export function createMorningBriefing(
  deps: MorningBriefingDeps
): MorningBriefingController {
  let text = $state('');
  let prev = $state('');
  let busy = $state(false);
  let error = $state('');
  let dismissed = $state(false);
  let abort: AbortController | null = null;

  const dismissKey = `granit.morning.briefDismissed.${deps.todayISO}`;

  function hydrateDismissed() {
    dismissed =
      typeof localStorage !== 'undefined' &&
      localStorage.getItem(dismissKey) === '1';
  }

  async function run() {
    if (busy) return;
    error = '';
    // Snapshot whatever's currently on screen — the UI will keep it
    // visible (dimmed) until the first streamed token lands. This is
    // the regenerate-keep-context affordance.
    prev = text;
    text = '';
    busy = true;
    abort?.abort();
    abort = new AbortController();
    const user = buildBriefingUserPrompt({
      todayISO: deps.todayISO,
      events: deps.getEvents(),
      tasks: deps.getTasks(),
      goals: deps.getGoals(),
      deadlines: deps.getDeadlines()
    });
    // rAF throttle so a fast model doesn't repaint the rendered brief
    // per token — same shape as the other AI dialogs.
    const t = rafThrottle((full) => {
      text = full;
    });
    try {
      await deps.chatStream(
        [
          { role: 'system', content: BRIEFING_SYSTEM_PROMPT },
          { role: 'user', content: user }
        ],
        undefined,
        {
          onChunk: t.onChunk,
          onDone: () => {
            t.flush();
            busy = false;
            abort = null;
            prev = '';
            if (!text.trim()) error = 'AI returned an empty brief.';
          },
          onError: (err) => {
            t.flush();
            // Cancelling the request mid-stream surfaces here too —
            // skip the error chrome so the cancel path's "keep
            // previous text" affordance isn't undone by a stale
            // "operation aborted" headline. busy + abort were
            // already reset by cancel() in that case.
            if (isAbortError(err)) {
              return;
            }
            busy = false;
            abort = null;
            // Restore the previous brief on failure so the user
            // isn't left with an empty panel + just an error.
            if (prev && !text.trim()) {
              text = prev;
            }
            prev = '';
            const hint = classifyAiError(err.message);
            error = hint.headline;
          }
        },
        abort.signal
      );
    } catch (e) {
      if (isAbortError(e)) return;
      busy = false;
      abort = null;
      if (prev && !text.trim()) {
        text = prev;
      }
      prev = '';
      error = errorMessage(e);
    }
  }

  function cancel() {
    abort?.abort();
    abort = null;
    busy = false;
    // Cancel restores the previous brief — the user usually cancels
    // because they decided the old one was fine after all.
    if (prev && !text.trim()) {
      text = prev;
    }
    prev = '';
  }

  function dismiss() {
    dismissed = true;
    try {
      localStorage.setItem(dismissKey, '1');
    } catch {
      // private mode / quota — ignore; brief stays dismissed in
      // memory until reload, which is the right fallback.
    }
  }

  return {
    get text() {
      return text;
    },
    get prev() {
      return prev;
    },
    get busy() {
      return busy;
    },
    get error() {
      return error;
    },
    get dismissed() {
      return dismissed;
    },
    set dismissed(v) {
      dismissed = v;
    },
    hydrateDismissed,
    run,
    cancel,
    dismiss
  };
}
