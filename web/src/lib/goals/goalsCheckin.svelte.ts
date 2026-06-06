// AI "Weekly check-in" controller for the goals surface.
//
// Walks every active + paused goal and asks the model for a sharp
// one-line verdict (on-track / drifting / dead) plus one specific
// question worth sitting with this week. Streams back as a single
// JSON array so the panel renders a checklist of cards and lets the
// user save individual entries to today's jot or the whole batch as
// a single "Weekly check-in" block.
//
// The system prompt borrows from the Coach (Socrates) + Founder
// personas — questions over answers, but operator-grade specifics,
// refusing generic praise. The user message embeds a compact view of
// each goal's milestones, target_date urgency, and linked-task
// velocity (open + 4-week done count) so the model has enough to
// tell drifting from healthy.

import { api, todayISO, type Goal } from '$lib/api';
import { errorMessage } from '$lib/util/errorMessage';
import { isAbortError } from '$lib/util/aiErrors';
import { toast } from '$lib/components/toast';
import { daysUntilTarget } from './util';
import type { GoalsDataController } from './goalsData.svelte';
import type { CheckinEntry } from './GoalsAICheckinPanel.svelte';

export interface GoalsCheckinController {
  readonly checkinOpen: boolean;
  readonly checkinBusy: boolean;
  readonly checkinError: string;
  readonly checkinEntries: CheckinEntry[];
  readonly checkinHidden: Set<string>;
  /** Live-goal slice the AI sees — single source of truth so the
   *  "AI saw" disclosure below the panel matches the prompt exactly. */
  readonly checkinScope: Goal[];

  /** Fire a fresh check-in run. No-op while busy; toasts an error when
   *  the user's goal list has nothing active/paused to check on. */
  run(): Promise<void>;
  /** Persist one entry to today's jot. */
  saveOne(e: CheckinEntry): Promise<void>;
  /** Persist every visible (non-dismissed) entry to today's jot in one
   *  block, then close the panel. */
  saveAll(): Promise<void>;
  /** Hide an entry from the panel without persisting. */
  dismiss(e: CheckinEntry): void;
  /** Bare rollup shape the panel reads (open/done counts only). */
  rollupFor(g: Goal): { open: number; done: number };
  /** Per-goal velocity in the last 4 weeks — exposed because the
   *  checkin panel renders the same number alongside each row's
   *  card so the user sees what the AI saw. */
  recentDoneFor(g: Goal): number;
  /** Abort the in-flight stream WITHOUT clearing state (the Stop
   *  CTA path inside the panel). */
  stop(): void;
  /** Abort + clear all state (the Close CTA path). */
  close(): void;
}

export interface GoalsCheckinDeps {
  dataCtl: GoalsDataController;
}

export function createGoalsCheckin(deps: GoalsCheckinDeps): GoalsCheckinController {
  let checkinOpen = $state(false);
  let checkinBusy = $state(false);
  let checkinError = $state('');
  let checkinEntries = $state<CheckinEntry[]>([]);
  let checkinAbort: AbortController | null = null;
  // Set of goal ids the user has already saved or dismissed in this
  // session — keeps the buttons honest if the user clicks "save" then
  // changes their mind ("dismiss" hides the row).
  let checkinHidden = $state<Set<string>>(new Set());

  const checkinScope = $derived(
    deps.dataCtl.goals.filter((g) => {
      const s = g.status ?? 'active';
      return s === 'active' || s === 'paused';
    })
  );

  function stop() {
    checkinAbort?.abort();
  }

  function close() {
    checkinAbort?.abort();
    checkinAbort = null;
    checkinOpen = false;
    checkinBusy = false;
    checkinError = '';
    checkinEntries = [];
    checkinHidden = new Set();
  }

  // Per-goal velocity in the last 4 weeks — used in the prompt so the
  // model can spot "0 done in 4w" drift without us telling it what to
  // think. Same goalId-on-task indexing the page already uses for the
  // rollup chips, but bucketed by completedAt.
  function recentDoneFor(g: Goal): number {
    const cutoff = Date.now() - 28 * 24 * 3600 * 1000;
    let n = 0;
    for (const t of deps.dataCtl.doneTasks) {
      if (t.goalId !== g.id || !t.completedAt) continue;
      const d = new Date(t.completedAt).getTime();
      if (!Number.isFinite(d)) continue;
      if (d >= cutoff) n++;
    }
    return n;
  }

  async function run() {
    if (checkinBusy) return;
    if (checkinScope.length === 0) {
      toast.error('No active or paused goals to check in on.');
      return;
    }
    checkinAbort?.abort();
    checkinAbort = new AbortController();
    checkinOpen = true;
    checkinBusy = true;
    checkinError = '';
    checkinEntries = [];
    checkinHidden = new Set();

    // Compact per-goal block. Cap milestone lists at ~6 each so a goal
    // with a long history doesn't dominate the prompt budget.
    const blocks: string[] = [];
    for (const g of checkinScope) {
      const ms = g.milestones ?? [];
      const open = ms.filter((m) => !m.done).slice(0, 6).map((m) => m.text);
      const done = ms.filter((m) => m.done).slice(-6).map((m) => m.text);
      const days = daysUntilTarget(g.target_date);
      const roll = deps.dataCtl.rollupFor(g);
      const recent = recentDoneFor(g);
      const lines: string[] = [
        `[id: ${g.id}] ${g.title}`,
        `status: ${g.status ?? 'active'}`
      ];
      if (g.target_date) {
        const urgency =
          days === null ? 'no-date'
          : days < 0 ? `${Math.abs(days)}d past target`
          : days <= 7 ? `${days}d left`
          : days <= 30 ? `${days}d left (this month)`
          : days <= 90 ? `${days}d left (this quarter)`
          : `${days}d left`;
        lines.push(`target: ${g.target_date} — ${urgency}`);
      }
      if (g.venture) lines.push(`venture: ${g.venture}`);
      if (g.project) lines.push(`project: ${g.project}`);
      lines.push(`milestones: ${done.length} done, ${open.length} open`);
      if (open.length > 0) lines.push(`open milestones: ${open.map((m) => `"${m}"`).join('; ')}`);
      if (done.length > 0) lines.push(`recent done: ${done.map((m) => `"${m}"`).join('; ')}`);
      lines.push(`linked tasks: ${roll.open} open, ${roll.done} done lifetime, ${recent} done in last 4w`);
      blocks.push(lines.join('\n'));
    }

    const userMessage =
      'You are a Socratic coach with operator-grade specificity.\n' +
      'Refuse generic praise. The user can already pat themselves on the back; your job is to add what they would not naturally see.\n\n' +
      'For EACH goal below, return a one-sentence honest verdict and ONE specific question worth sitting with this week.\n\n' +
      'Verdict rules:\n' +
      '- "on-track" — momentum is real, recent done > 0 OR milestones are closing in cadence with the target_date\n' +
      '- "drifting" — talked about, not moved on; few/no done milestones or tasks in last 4w; target_date pressure rising\n' +
      '- "dead" — past target with no progress, OR open milestones haven\'t been touched and the user has clearly shifted\n' +
      'Be willing to call drifting "drifting". The user wants honesty over kindness.\n\n' +
      'Question rules:\n' +
      '- Specific to THIS goal, not a generic "what is your why".\n' +
      '- Pry at an assumption, a contradiction, or a missing definition the user is probably carrying.\n' +
      '- Sometimes uncomfortable. Always kind.\n' +
      '- One question. No multi-part. No "What if you tried…" — questions, not advice.\n\n' +
      'Return STRICT JSON ONLY (no markdown fences, no preamble), shape:\n' +
      '[{"id": "<echo the goal id>", "title": "<echo the goal title>", "verdict": "on-track|drifting|dead", "question": "..."}, ...]\n' +
      'Order matches input order. Do NOT skip any goal.\n\n' +
      'Goals:\n\n' + blocks.join('\n\n---\n\n');

    let acc = '';
    try {
      await api.chatStream(
        [{ role: 'user', content: userMessage }],
        undefined,
        {
          onChunk: (c) => { acc += c; },
          onDone: () => {
            checkinBusy = false;
            checkinAbort = null;
            // Strip optional code fences and parse.
            let cleaned = acc.trim();
            if (cleaned.startsWith('```')) {
              cleaned = cleaned.replace(/^```(?:json)?\s*/, '').replace(/```\s*$/, '').trim();
            }
            try {
              const parsed = JSON.parse(cleaned);
              if (!Array.isArray(parsed)) throw new Error('expected array');
              checkinEntries = parsed
                .filter((p: unknown) => p && typeof p === 'object')
                .map((p) => p as CheckinEntry)
                .filter((p) => typeof p.question === 'string' && typeof p.verdict === 'string');
              if (checkinEntries.length === 0) {
                checkinError = 'AI returned no entries.';
              }
            } catch (err) {
              checkinError = "Couldn't parse check-in: " + errorMessage(err);
            }
          },
          onError: (err) => {
            checkinBusy = false;
            checkinAbort = null;
            if (isAbortError(err)) return;
            checkinError = err.message;
          }
        },
        checkinAbort.signal
      );
    } catch (e) {
      checkinBusy = false;
      checkinAbort = null;
      checkinError = errorMessage(e);
    }
  }

  // Compose the markdown block that will be appended to today's jot
  // (or saved alongside, depending on which button the user clicked).
  // Visible only entries (not dismissed) make it in — that's the
  // user's curation step before persistence.
  function checkinAsMarkdown(entries: CheckinEntry[]): string {
    const today = todayISO();
    const lines: string[] = [`\n\n## Weekly goal check-in — ${today}\n`];
    for (const e of entries) {
      const verdictTag = e.verdict === 'on-track' ? 'ON-TRACK'
        : e.verdict === 'drifting' ? 'DRIFTING'
        : e.verdict === 'dead' ? 'DEAD'
        : e.verdict.toUpperCase();
      lines.push(`### ${e.title} — _${verdictTag}_`);
      lines.push(`> ${e.question}`);
      lines.push('');
    }
    return lines.join('\n');
  }

  async function saveOne(e: CheckinEntry) {
    try {
      const note = await api.daily('today');
      const block = checkinAsMarkdown([e]);
      const next = (note.body ?? '') + block;
      await api.putNote(note.path, { frontmatter: note.frontmatter, body: next }, undefined);
      checkinHidden = new Set([...checkinHidden, e.id]);
      toast.success("saved to today's jot");
    } catch (err) {
      toast.error('save failed: ' + errorMessage(err));
    }
  }

  async function saveAll() {
    const visible = checkinEntries.filter((e) => !checkinHidden.has(e.id));
    if (visible.length === 0) return;
    try {
      const note = await api.daily('today');
      const block = checkinAsMarkdown(visible);
      const next = (note.body ?? '') + block;
      await api.putNote(note.path, { frontmatter: note.frontmatter, body: next }, undefined);
      toast.success(`saved ${visible.length} entr${visible.length === 1 ? 'y' : 'ies'} to today's jot`);
      close();
    } catch (err) {
      toast.error('save failed: ' + errorMessage(err));
    }
  }

  function dismiss(e: CheckinEntry) {
    checkinHidden = new Set([...checkinHidden, e.id]);
  }

  // Bare-rollup view for the AI checkin panel — drops the project
  // field because the panel only reads open/done counts.
  function rollupFor(g: Goal): { open: number; done: number } {
    const r = deps.dataCtl.rollupFor(g);
    return { open: r.open, done: r.done };
  }

  return {
    get checkinOpen() {
      return checkinOpen;
    },
    get checkinBusy() {
      return checkinBusy;
    },
    get checkinError() {
      return checkinError;
    },
    get checkinEntries() {
      return checkinEntries;
    },
    get checkinHidden() {
      return checkinHidden;
    },
    get checkinScope() {
      return checkinScope;
    },
    run,
    saveOne,
    saveAll,
    dismiss,
    rollupFor,
    recentDoneFor,
    stop,
    close
  };
}
