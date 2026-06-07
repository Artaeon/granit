// Daily Routine AI — controller. Owns the streaming proposal lifecycle
// (`busy`, `proposal`, `error`, per-op opt-out set) and the apply flow.
// Drives RoutineProposalDrawer.svelte; the page mounts the drawer and
// calls into this controller via its factory deps.
//
// Phase 2 of the workspace-OS arc: the AI proposes the day, the user
// reviews/edits, the apply lands as a partial-safe batch. The
// controller never auto-applies — every mutation goes through user
// confirmation, mirroring the calendar agent's accept/skip posture.

import { api, type RoutineProposal, type RoutineEventOp } from '$lib/api';
import { isAbortError } from '$lib/util/aiErrors';
import { rafThrottle } from '$lib/util/streamThrottle';

export interface RoutineAIControllerDeps {
  /** Called after a successful apply so the page can reload calendar
   *  state. Receives the applied count for an optional summary toast. */
  onApplied?: (applied: number) => void | Promise<void>;
  /** Optional error hook for the page (e.g. surface a toast). The
   *  controller already stores the message in `error` for inline
   *  rendering; this hook is for callers that want a side-channel. */
  onError?: (err: Error) => void;
}

export interface RoutineAIController {
  /** True while the proposal stream is in flight. */
  readonly busy: boolean;
  /** True while the apply request is in flight. */
  readonly applying: boolean;
  /** Latest proposal (the last one the stream emitted). null until
   *  the first parseable JSON object arrives. */
  readonly proposal: RoutineProposal | null;
  /** Inline error message — set on stream / apply failure. Cleared
   *  when a new proposal lands. */
  readonly error: string;
  /** Set of eventOp indices the user has toggled OFF. We track the
   *  rejected set rather than the accepted one so re-emissions of
   *  the proposal don't accidentally re-enable rows the user
   *  already skipped. */
  readonly rejected: Set<number>;
  /** ISO date the proposal targets — set by propose(). */
  readonly date: string;

  /** Convenience: count of ops the user has NOT opted out of, paired
   *  with the total op count. Drives the "Apply selected (N/M)"
   *  footer label. */
  readonly selectedCount: number;
  readonly totalOps: number;

  /** Kick off a streaming proposal for the given date (defaults to
   *  today). Cancels any in-flight stream first. */
  propose(date?: string): Promise<void>;
  /** Cancel an in-flight proposal stream. No-op when idle. */
  cancel(): void;
  /** Apply the current proposal, honouring per-op opt-outs. */
  apply(): Promise<void>;
  /** Discard the current proposal. Clears state without applying. */
  discard(): void;
  /** Flip the rejected flag on op index `idx`. */
  toggleOp(idx: number): void;
}

/** Today's date in YYYY-MM-DD local time. The controller's default
 *  date — uses the browser's local TZ which matches the calendar
 *  pane's day boundary. */
function todayISO(): string {
  const d = new Date();
  const yyyy = d.getFullYear();
  const mm = String(d.getMonth() + 1).padStart(2, '0');
  const dd = String(d.getDate()).padStart(2, '0');
  return `${yyyy}-${mm}-${dd}`;
}

export function createRoutineAICtl(
  deps: RoutineAIControllerDeps = {}
): RoutineAIController {
  let busy = $state(false);
  let applying = $state(false);
  let proposal = $state<RoutineProposal | null>(null);
  let error = $state('');
  let rejected = $state(new Set<number>());
  let date = $state(todayISO());
  let ctrl: AbortController | null = null;

  const totalOps = $derived(proposal?.eventOps?.length ?? 0);
  const selectedCount = $derived(
    Math.max(0, (proposal?.eventOps?.length ?? 0) - rejected.size)
  );

  function reset() {
    proposal = null;
    error = '';
    rejected = new Set();
  }

  async function propose(targetDate?: string): Promise<void> {
    cancel();
    reset();
    if (targetDate) date = targetDate;
    busy = true;
    // Throttle proposal updates to one paint per frame — the AI's
    // chunk cadence is much finer than the screen's refresh rate and
    // re-rendering the diff on every token shape would jank the
    // drawer. Same pattern CalendarAgent uses for raw text.
    const t = rafThrottle((latest) => {
      try {
        proposal = JSON.parse(latest) as RoutineProposal;
      } catch {
        // Half-formed proposal — ignored; the next emission supersedes.
      }
    });
    await new Promise<void>((resolve) => {
      ctrl = api.calendarRoutineProposal(
        {
          onProposal: (p) => {
            // The server already parses each event; round-trip through
            // JSON.stringify so the throttle can dedupe identical
            // payloads cheaply.
            t.onChunk(JSON.stringify(p));
          },
          onDone: () => {
            t.flush();
            busy = false;
            ctrl = null;
            resolve();
          },
          onError: (err) => {
            t.flush();
            busy = false;
            ctrl = null;
            if (isAbortError(err)) {
              resolve();
              return;
            }
            error = err.message;
            deps.onError?.(err);
            resolve();
          }
        },
        date
      );
    });
  }

  function cancel(): void {
    if (ctrl) {
      ctrl.abort();
      ctrl = null;
      busy = false;
    }
  }

  function discard(): void {
    cancel();
    reset();
  }

  function toggleOp(idx: number): void {
    const next = new Set(rejected);
    if (next.has(idx)) next.delete(idx);
    else next.add(idx);
    rejected = next;
  }

  async function apply(): Promise<void> {
    if (!proposal || applying) return;
    applying = true;
    error = '';
    try {
      const ops: RoutineEventOp[] = proposal.eventOps.filter(
        (_, i) => !rejected.has(i)
      );
      const resp = await api.calendarApplyRoutine({
        date,
        dailyPlan: proposal.dailyPlan,
        eventOps: ops
      });
      // Partial-safe: even when some ops failed, the daily plan + the
      // successful ops did land. Surface the failed-row count in the
      // inline error string so the drawer can render a soft warning
      // instead of a hard red banner.
      if (resp.failed.length > 0) {
        error = `${resp.applied} applied · ${resp.failed.length} failed: ${resp.failed
          .map((f) => f.message)
          .slice(0, 3)
          .join('; ')}`;
      }
      await deps.onApplied?.(resp.applied);
      // Keep the proposal visible after a partial-success apply so the
      // user can retry / edit the failed rows. Discard only on a clean
      // apply (no failures).
      if (resp.failed.length === 0) {
        reset();
      }
    } catch (err) {
      const e = err instanceof Error ? err : new Error(String(err));
      error = e.message;
      deps.onError?.(e);
    } finally {
      applying = false;
    }
  }

  return {
    get busy() {
      return busy;
    },
    get applying() {
      return applying;
    },
    get proposal() {
      return proposal;
    },
    get error() {
      return error;
    },
    get rejected() {
      return rejected;
    },
    get date() {
      return date;
    },
    get selectedCount() {
      return selectedCount;
    },
    get totalOps() {
      return totalOps;
    },
    propose,
    cancel,
    apply,
    discard,
    toggleOp
  };
}
