// Quick-action service for AIOverlay.
//
// The four buttons above the composer (Briefing / Synopsis / Triage /
// Deadlines) each fire a one-shot AI call and render the result in
// the body pane in place of the chat thread. Logic used to live
// inline in AIOverlay as runBriefing/runSynopsis/runTriage/runDeadlines
// + a shared runQuick orchestrator. Pulled here so:
//
//   1. Quick-action dispatch is independently testable.
//   2. The slash-command router (which also fires these) talks to
//      one source of truth instead of grabbing the parent's
//      function references.
//   3. The abort controller is OWNED HERE — not shared with chat
//      send(). Previously the same `abort` handle covered both,
//      which meant starting a quick action while a chat was
//      streaming would silently cancel the chat (or vice versa).
//      The two are now independent: each surface owns its own
//      AbortController, each surface can be cancelled without
//      affecting the other.
//
// State convention: this module OWNS its abort handle (internal
// only — never exposed) but DOES NOT own the parent's busy /
// quickTitle / quickResult / messages slots. Those are read and
// written via the `refs` object. The reason is consistency with the
// rest of AIOverlay — quickResult/quickTitle are rendered in the
// same body pane that toggles between chat and quick-action display,
// so they have to live where the template can reach them.

import { api } from '$lib/api';
import type { ChatMessage } from '$lib/api';
import {
  QUICK_ACTION_TITLES,
  renderTriageProposals,
  renderDeadlineProposals
} from './quickActions';
import { errorMessage } from '$lib/util/errorMessage';

export interface QuickActionRefs {
  /** Mutual-exclusion flag shared with chat send(). A quick action
   *  refuses to start while busy and clears it on completion. */
  busy: boolean;
  /** Heading shown above the result body. */
  quickTitle: string;
  /** Markdown body of the last result (or a status string while
   *  the action is in flight / errored). */
  quickResult: string;
  /** Cleared to [] when a quick action starts — the result panel
   *  replaces the chat thread in the body. */
  messages: ChatMessage[];
}

export interface QuickActionServiceOptions {
  refs: QuickActionRefs;
}

export interface QuickActionService {
  runBriefing(): Promise<void>;
  runSynopsis(): Promise<void>;
  runTriage(): Promise<void>;
  runDeadlines(): Promise<void>;
  /** Cancel an in-flight quick action. Independent of the chat
   *  send()'s abort lifecycle — chat continues unaffected. */
  cancel(): void;
}

export function createQuickActionService(opts: QuickActionServiceOptions): QuickActionService {
  // Owned internally — NEVER exposed. The whole point of this split
  // is that chat send() has its own abort handle that this service
  // can't reach.
  let abort: AbortController | null = null;

  async function runQuick(title: string, fn: (signal: AbortSignal) => Promise<string>) {
    if (opts.refs.busy) return;
    // Cancel any previous quick action that's still in flight. Does
    // NOT touch chat — that's the abort split this PR ships.
    abort?.abort();
    abort = new AbortController();
    opts.refs.busy = true;
    opts.refs.quickTitle = title;
    opts.refs.quickResult = '_running…_';
    // Chat clears when a quick action runs — the body shows ONE of
    // chat / quick-action at a time.
    opts.refs.messages = [];
    try {
      opts.refs.quickResult = await fn(abort.signal);
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        opts.refs.quickResult = '_cancelled_';
      } else {
        const msg = errorMessage(err);
        opts.refs.quickResult = /disabled in AI preferences/i.test(msg)
          ? `_${msg}_  \n\n[Open settings →](/settings)`
          : `_failed:_ ${msg}`;
      }
    } finally {
      opts.refs.busy = false;
      abort = null;
    }
  }

  async function runBriefing() {
    await runQuick(QUICK_ACTION_TITLES.briefing, async (s) => {
      const r = await api.aiDailyBriefing(s);
      return r.markdown;
    });
  }
  async function runSynopsis() {
    await runQuick(QUICK_ACTION_TITLES.synopsis, async (s) => {
      const r = await api.aiWeeklyReview(s);
      return r.markdown;
    });
  }
  async function runTriage() {
    await runQuick(QUICK_ACTION_TITLES.triage, async (s) => {
      const r = await api.aiInboxTriage(s);
      return renderTriageProposals(r.proposals ?? []);
    });
  }
  async function runDeadlines() {
    await runQuick(QUICK_ACTION_TITLES.deadlines, async (s) => {
      const r = await api.aiDeadlineDetect(s);
      return renderDeadlineProposals(r.proposals ?? []);
    });
  }

  function cancel() {
    abort?.abort();
  }

  return {
    runBriefing,
    runSynopsis,
    runTriage,
    runDeadlines,
    cancel
  };
}
