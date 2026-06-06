// AI streaming state for the finance surface.
//
// Third extraction step out of routes/finance/+page.svelte. Owns the
// two streaming AI panels: the overview "snapshot" (3-paragraph read
// of where you stand) and the subscriptions "audit" (cancellation
// candidates ranked by annual saving). Both share the same shape —
// text + busy + error + AbortController, run/cancel/dismiss — so
// folding them into one controller is cheaper than two near-identical
// ones.
//
// External state (overview + subs + streams + goals) is reached via
// the deps bundle so the controller doesn't have to import the data
// controller directly. Same with the pure prompt builders / system
// prompts — injected so this file stays free of $lib/finance circular
// imports.
//
// The page still owns the chrome rendering. This controller is pure
// state machinery + chatStream wiring.

import type {
  ChatMessage,
  FinOverview,
  FinSubscription,
  FinIncomeStream,
  FinGoal
} from '$lib/api';
import { rafThrottle } from '$lib/util/streamThrottle';
import { classifyAiError, isAbortError } from '$lib/util/aiErrors';
import { errorMessage } from '$lib/util/errorMessage';

type ChatStreamHandlers = {
  onChunk: (chunk: string) => void;
  onDone?: () => void;
  onError?: (err: Error) => void;
};

export interface FinanceAIDeps {
  /** Reactive snapshot getters — read at runSnapshot() time so the
   *  prompt sees the freshest data. */
  getOverview: () => FinOverview | null;
  getSubs: () => FinSubscription[];
  getStreams: () => FinIncomeStream[];
  getGoals: () => FinGoal[];

  /** System prompts + pure builders — injected so this file doesn't
   *  reach into aiPrompts.ts itself, keeping the dep graph one-way. */
  snapshotSystemPrompt: string;
  subAuditSystemPrompt: string;
  buildSnapshotPrompt: (args: {
    overview: FinOverview;
    subscriptions: FinSubscription[];
    streams: FinIncomeStream[];
    goals: FinGoal[];
  }) => string;
  buildSubAuditPrompt: (args: {
    subscriptions: FinSubscription[];
    monthlyIncomeCents: number;
    currency: string;
  }) => string;

  /** chatStream binding — injected so unit tests can stub it without
   *  monkey-patching the api singleton. Mirror of api.chatStream. */
  chatStream: (
    messages: ChatMessage[],
    notePath: string | undefined,
    handlers: ChatStreamHandlers,
    signal?: AbortSignal
  ) => Promise<void>;
}

export interface FinanceAIController {
  // Snapshot state.
  readonly snapshotText: string;
  readonly snapshotBusy: boolean;
  readonly snapshotError: string;
  runSnapshot(): Promise<void>;
  cancelSnapshot(): void;
  dismissSnapshot(): void;

  // Sub-audit state.
  readonly auditText: string;
  readonly auditBusy: boolean;
  readonly auditError: string;
  runSubAudit(): Promise<void>;
  cancelSubAudit(): void;
  dismissSubAudit(): void;
}

export function createFinanceAI(deps: FinanceAIDeps): FinanceAIController {
  // ── snapshot ────────────────────────────────────────────────────
  let snapshotText = $state('');
  let snapshotBusy = $state(false);
  let snapshotError = $state('');
  let snapshotAbort: AbortController | null = null;

  // ── sub-audit ───────────────────────────────────────────────────
  let auditText = $state('');
  let auditBusy = $state(false);
  let auditError = $state('');
  let auditAbort: AbortController | null = null;

  async function runSnapshot() {
    const overview = deps.getOverview();
    if (!overview || snapshotBusy) return;
    snapshotError = '';
    snapshotText = '';
    snapshotBusy = true;
    snapshotAbort?.abort();
    snapshotAbort = new AbortController();
    const user = deps.buildSnapshotPrompt({
      overview,
      subscriptions: deps.getSubs(),
      streams: deps.getStreams(),
      goals: deps.getGoals()
    });
    const t = rafThrottle((full) => {
      snapshotText = full;
    });
    try {
      await deps.chatStream(
        [
          { role: 'system', content: deps.snapshotSystemPrompt },
          { role: 'user', content: user }
        ],
        undefined,
        {
          onChunk: t.onChunk,
          onDone: () => {
            t.flush();
            snapshotBusy = false;
            snapshotAbort = null;
            if (!snapshotText.trim()) snapshotError = 'AI returned an empty snapshot.';
          },
          onError: (err) => {
            t.flush();
            snapshotBusy = false;
            snapshotAbort = null;
            if (isAbortError(err)) return;
            snapshotError = classifyAiError(err.message).headline;
          }
        },
        snapshotAbort.signal
      );
    } catch (e) {
      snapshotBusy = false;
      snapshotAbort = null;
      snapshotError = errorMessage(e);
    }
  }
  function cancelSnapshot() {
    snapshotAbort?.abort();
    snapshotAbort = null;
    snapshotBusy = false;
  }
  function dismissSnapshot() {
    snapshotText = '';
    snapshotError = '';
  }

  async function runSubAudit() {
    if (auditBusy) return;
    const overview = deps.getOverview();
    auditError = '';
    auditText = '';
    auditBusy = true;
    auditAbort?.abort();
    auditAbort = new AbortController();
    const user = deps.buildSubAuditPrompt({
      subscriptions: deps.getSubs(),
      monthlyIncomeCents: overview?.income_monthly_actual_cents ?? 0,
      currency: overview?.currency ?? 'EUR'
    });
    const t = rafThrottle((full) => {
      auditText = full;
    });
    try {
      await deps.chatStream(
        [
          { role: 'system', content: deps.subAuditSystemPrompt },
          { role: 'user', content: user }
        ],
        undefined,
        {
          onChunk: t.onChunk,
          onDone: () => {
            t.flush();
            auditBusy = false;
            auditAbort = null;
            if (!auditText.trim()) auditError = 'AI returned an empty audit.';
          },
          onError: (err) => {
            t.flush();
            auditBusy = false;
            auditAbort = null;
            if (isAbortError(err)) return;
            auditError = classifyAiError(err.message).headline;
          }
        },
        auditAbort.signal
      );
    } catch (e) {
      auditBusy = false;
      auditAbort = null;
      auditError = errorMessage(e);
    }
  }
  function cancelSubAudit() {
    auditAbort?.abort();
    auditAbort = null;
    auditBusy = false;
  }
  function dismissSubAudit() {
    auditText = '';
    auditError = '';
  }

  return {
    get snapshotText() { return snapshotText; },
    get snapshotBusy() { return snapshotBusy; },
    get snapshotError() { return snapshotError; },
    runSnapshot,
    cancelSnapshot,
    dismissSnapshot,

    get auditText() { return auditText; },
    get auditBusy() { return auditBusy; },
    get auditError() { return auditError; },
    runSubAudit,
    cancelSubAudit,
    dismissSubAudit
  };
}
