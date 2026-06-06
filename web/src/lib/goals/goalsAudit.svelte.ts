// AI "Goal alignment audit" controller.
//
// Reads all active goals + open tasks + recently-completed tasks
// (last 14 days, no goalId) and asks the model: which clusters of
// tasks are NOT advancing any stated goal? Surfaces the gap that
// goal-setters typically can't see for themselves — the busywork
// that fills the day without moving the season.
//
// The audit is honest and non-judgmental. The user may be
// intentionally working off-goal (urgent maintenance, paid work,
// family emergency); the model's job is to name the pattern, not
// to scold. The user can dismiss findings, mark a finding as
// intentional (no action), or jump to /tasks to re-link.

import { api, type Goal, type Task } from '$lib/api';
import { errorMessage } from '$lib/util/errorMessage';
import { isAbortError } from '$lib/util/aiErrors';
import { toast } from '$lib/components/toast';
import type { GoalsDataController } from './goalsData.svelte';
import type { AuditFinding } from './GoalsAIAuditPanel.svelte';

export type AuditScope = {
  orphanOpen: Task[];
  orphanDoneRecent: Task[];
  linkedOpen: number;
  linkedDone14: number;
};

export interface GoalsAuditController {
  readonly auditOpen: boolean;
  readonly auditBusy: boolean;
  readonly auditError: string;
  readonly auditFindings: AuditFinding[];
  readonly auditDismissed: Set<string>;
  /** Tasks the audit looks at — exposed so the panel can show
   *  "AI saw N orphan tasks" without recomputing the slice. */
  readonly auditScope: AuditScope;

  /** Run the audit. No-ops with a toast when there are no active
   *  goals or no orphan tasks to compare. */
  run(): Promise<void>;
  /** Hide a finding from the panel. */
  dismiss(f: AuditFinding): void;
  /** Abort the in-flight stream WITHOUT clearing state (Stop CTA). */
  stop(): void;
  /** Abort + clear all state (Close CTA). */
  close(): void;
}

export interface GoalsAuditDeps {
  dataCtl: GoalsDataController;
}

export function createGoalsAudit(deps: GoalsAuditDeps): GoalsAuditController {
  let auditOpen = $state(false);
  let auditBusy = $state(false);
  let auditError = $state('');
  let auditFindings = $state<AuditFinding[]>([]);
  let auditAbort: AbortController | null = null;
  let auditDismissed = $state<Set<string>>(new Set());

  // Tasks the audit looks at — in-flight + recently-done, both
  // unlinked from any goal. Cap at 80 each so the prompt stays
  // bounded; the model sees representative behaviour, not a full
  // dump. Recently-done is limited to the last 14 days because
  // older history isn't actionable for a "this season" check.
  const auditScope = $derived.by<AuditScope>(() => {
    const cutoff = Date.now() - 14 * 24 * 3600 * 1000;
    const orphanOpen = deps.dataCtl.openTasks
      .filter((t) => !t.goalId && (t.text ?? '').trim().length > 0)
      .slice(0, 80);
    const orphanDoneRecent = deps.dataCtl.doneTasks
      .filter((t) => {
        if (t.goalId) return false;
        if (!t.completedAt) return false;
        const d = new Date(t.completedAt).getTime();
        return Number.isFinite(d) && d >= cutoff;
      })
      .slice(0, 80);
    const linkedOpen = deps.dataCtl.openTasks.filter((t) => t.goalId).length;
    const linkedDone14 = deps.dataCtl.doneTasks.filter((t) => {
      if (!t.goalId || !t.completedAt) return false;
      const d = new Date(t.completedAt).getTime();
      return Number.isFinite(d) && d >= cutoff;
    }).length;
    return { orphanOpen, orphanDoneRecent, linkedOpen, linkedDone14 };
  });

  function stop() {
    auditAbort?.abort();
  }

  function close() {
    auditAbort?.abort();
    auditAbort = null;
    auditOpen = false;
    auditBusy = false;
    auditError = '';
    auditFindings = [];
    auditDismissed = new Set();
  }

  async function run() {
    if (auditBusy) return;
    const activeGoals: Goal[] = deps.dataCtl.goals.filter((g) => (g.status ?? 'active') === 'active');
    if (activeGoals.length === 0) {
      toast.error('No active goals to audit against.');
      return;
    }
    const totalOrphan = auditScope.orphanOpen.length + auditScope.orphanDoneRecent.length;
    if (totalOrphan === 0) {
      toast.success('Every recent task is linked to a goal — nothing to audit.');
      return;
    }
    auditAbort?.abort();
    auditAbort = new AbortController();
    auditOpen = true;
    auditBusy = true;
    auditError = '';
    auditFindings = [];
    auditDismissed = new Set();

    const goalLines = activeGoals
      .map((g) => `- ${g.title}${g.target_date ? ` (target ${g.target_date})` : ''}${g.venture ? ` [${g.venture}]` : ''}`)
      .join('\n');
    const orphanOpenLines = auditScope.orphanOpen.map((t) => `- ${t.text}`).join('\n');
    const orphanDoneLines = auditScope.orphanDoneRecent.map((t) => `- ${t.text}`).join('\n');

    const userMessage =
      "You are an honest, non-judgmental auditor of where the user's actual work is going.\n" +
      "Compare the user's ACTIVE GOALS to their TASKS that are NOT linked to any goal. " +
      'Find 2-5 clusters of unlinked tasks that share a theme. For each cluster, surface what is happening and ask whether it was intentional.\n\n' +
      'Rules:\n' +
      '- Be specific. "You worked on support" beats "you worked on miscellaneous things".\n' +
      '- Cluster by theme (e.g. "support / maintenance", "finances / admin", "client work for X", "household").\n' +
      '- Off-goal work is NOT inherently bad — paid work, urgent maintenance, family. Your job is to NAME the pattern, not to scold.\n' +
      '- Include the rough count of tasks in each cluster and 2-3 representative task texts (verbatim).\n' +
      '- The "question" should be honest and useful: "Was this week\'s 12 tasks on X the right call given goal Y is overdue?" — never generic.\n' +
      "- Skip clusters with fewer than 2 tasks. Don't pad to hit a number; 2 sharp findings beat 5 mush ones.\n\n" +
      'Return STRICT JSON ONLY (no markdown fences, no preamble), shape:\n' +
      '[{"cluster": "...", "count": N, "sample": ["...", "..."], "observation": "...", "question": "..."}, ...]\n\n' +
      'ACTIVE GOALS:\n' + (goalLines || '(none)') + '\n\n' +
      `UNLINKED OPEN TASKS (${auditScope.orphanOpen.length}):\n` + (orphanOpenLines || '(none)') + '\n\n' +
      `UNLINKED TASKS COMPLETED IN LAST 14 DAYS (${auditScope.orphanDoneRecent.length}):\n` + (orphanDoneLines || '(none)') + '\n\n' +
      'For context the user already has linked work too: ' +
      `${auditScope.linkedOpen} open tasks tied to goals, ${auditScope.linkedDone14} goal-linked tasks completed in 14d. ` +
      "Don't mention this in your output unless it changes the verdict.";

    let acc = '';
    try {
      await api.chatStream(
        [{ role: 'user', content: userMessage }],
        undefined,
        {
          onChunk: (c) => { acc += c; },
          onDone: () => {
            auditBusy = false;
            auditAbort = null;
            let cleaned = acc.trim();
            if (cleaned.startsWith('```')) {
              cleaned = cleaned.replace(/^```(?:json)?\s*/, '').replace(/```\s*$/, '').trim();
            }
            try {
              const parsed = JSON.parse(cleaned);
              if (!Array.isArray(parsed)) throw new Error('expected array');
              auditFindings = parsed
                .filter((p: unknown) => p && typeof p === 'object')
                .map((p) => p as AuditFinding)
                .filter((p) => typeof p.cluster === 'string' && typeof p.observation === 'string' && typeof p.question === 'string')
                .map((p) => ({
                  cluster: p.cluster,
                  count: typeof p.count === 'number' ? p.count : (Array.isArray(p.sample) ? p.sample.length : 0),
                  sample: Array.isArray(p.sample) ? p.sample.filter((s): s is string => typeof s === 'string').slice(0, 3) : [],
                  observation: p.observation,
                  question: p.question
                }));
              if (auditFindings.length === 0) {
                auditError = 'AI returned no clusters — the work may already be aligned, or the parse failed.';
              }
            } catch (err) {
              auditError = "Couldn't parse audit: " + errorMessage(err);
            }
          },
          onError: (err) => {
            auditBusy = false;
            auditAbort = null;
            if (isAbortError(err)) return;
            auditError = err.message;
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

  function dismiss(f: AuditFinding) {
    auditDismissed = new Set([...auditDismissed, f.cluster]);
  }

  return {
    get auditOpen() {
      return auditOpen;
    },
    get auditBusy() {
      return auditBusy;
    },
    get auditError() {
      return auditError;
    },
    get auditFindings() {
      return auditFindings;
    },
    get auditDismissed() {
      return auditDismissed;
    },
    get auditScope() {
      return auditScope;
    },
    run,
    dismiss,
    stop,
    close
  };
}
