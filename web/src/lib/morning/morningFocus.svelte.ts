// Today's win + #1-focus state for the morning ritual.
//
// Fifth extraction step out of routes/morning/+page.svelte. Owns the
// three free-text bindings the user types into (winSentence, goal,
// linkedGoalId) plus the AI "suggest one focused outcome" round-trip
// that lands in the goal field.
//
// The suggester is a single-shot api.chat call (no streaming) — same
// behaviour the inline page had. The result is trim+unquoted; the
// "use this" action copies it into `goal` and clears the suggestion
// pill.
//
// Goal-link chips: tapping a chip toggles the link; the first toggle
// also seeds the focus field with the goal's title when empty, so
// the user doesn't have to retype it. Tapping the same chip again
// (or "clear") drops the link without touching the goal text.

import type { ChatMessage, Goal } from '$lib/api';
import { toast } from '$lib/components/toast';
import { errorMessage } from '$lib/util/errorMessage';
import { classifyAiError } from '$lib/util/aiErrors';

export interface MorningFocusDeps {
  /** ISO date — interpolated into the prompt so the model can pick
   *  date-sensitive language ("today" vs the calendar date). */
  todayISO: string;
  /** Reactive accessor for the urgency-sorted tasks. Read at
   *  suggest() time so the prompt sees the fresh ordering after a
   *  re-load. */
  getSortedTasks: () => Array<{
    text: string;
    priority: number;
    dueDate?: string;
  }>;
  /** Single-shot chat — mirror of api.chat. Injected for testing. */
  chat: (
    messages: ChatMessage[]
  ) => Promise<{ message: { content: string } }>;
}

export interface MorningFocusController {
  /** The "what would make today a win?" sentence. */
  winSentence: string;
  /** The "today's #1 focus" sentence. AI suggestions land here on
   *  "use this". */
  goal: string;
  /** Optional goal-id link. When set, the saved daily note appends
   *  " — contributes to: <title>". */
  linkedGoalId: string;
  /** Most recent AI suggestion. Empty when no pending suggestion. */
  readonly suggestion: string;
  /** True while the chat round-trip is in flight. */
  readonly suggesting: boolean;

  suggestFocus(): Promise<void>;
  /** Move the suggestion into the goal field and clear the pill. */
  acceptSuggestion(): void;
  /** Drop the pending suggestion without using it. */
  dismissSuggestion(): void;
  /** Toggle a goal-link chip. First tap also seeds an empty goal
   *  with the goal title; subsequent untoggle leaves goal alone. */
  pickGoalLink(g: Goal): void;
  /** Drop the link without touching the goal text. Used by the
   *  "clear" affordance next to the chips. */
  clearGoalLink(): void;

  /** Restore from snapshot — preserves the three text fields. */
  restore(snap: {
    winSentence?: string;
    goal?: string;
    linkedGoalId?: string;
  }): void;
}

export function createMorningFocus(deps: MorningFocusDeps): MorningFocusController {
  let winSentence = $state('');
  let goal = $state('');
  let linkedGoalId = $state<string>('');
  let suggestion = $state('');
  let suggesting = $state(false);

  function focusContext(): string {
    return deps
      .getSortedTasks()
      .slice(0, 12)
      .map((t) => {
        const p = t.priority > 0 ? `P${t.priority} ` : '';
        const due = t.dueDate ? ` (due ${t.dueDate})` : '';
        return `- ${p}${t.text}${due}`;
      })
      .join('\n');
  }

  async function suggestFocus() {
    suggesting = true;
    suggestion = '';
    try {
      const ctx = focusContext();
      const userMsg = ctx
        ? `It's ${deps.todayISO}. My open tasks (top by priority + due date):\n\n${ctx}\n\nIf I only got ONE thing done today, what should it be? Reply with a single, action-oriented sentence (8–14 words). No preamble, no list, no quotes — just the sentence. Pick something concrete from the tasks above or, if nothing fits, propose a focused outcome.`
        : `It's ${deps.todayISO}. I haven't logged any open tasks. Suggest one focused outcome for today as a single action-oriented sentence (8–14 words). No preamble, no quotes.`;
      const r = await deps.chat([{ role: 'user', content: userMsg }]);
      suggestion = r.message.content
        .trim()
        .replace(/^["'`]+|["'`]+$/g, '')
        .trim();
    } catch (e) {
      const raw = errorMessage(e);
      const hint = classifyAiError(raw);
      toast.error(hint.headline, { action: hint.cta, details: hint.raw });
    } finally {
      suggesting = false;
    }
  }

  function acceptSuggestion() {
    if (suggestion) goal = suggestion;
    suggestion = '';
  }
  function dismissSuggestion() {
    suggestion = '';
  }

  function pickGoalLink(g: Goal) {
    if (linkedGoalId === g.id) {
      linkedGoalId = '';
    } else {
      linkedGoalId = g.id;
      if (!goal.trim()) goal = g.title;
    }
  }
  function clearGoalLink() {
    linkedGoalId = '';
  }

  function restore(snap: {
    winSentence?: string;
    goal?: string;
    linkedGoalId?: string;
  }) {
    winSentence = snap.winSentence ?? '';
    goal = snap.goal ?? '';
    linkedGoalId = snap.linkedGoalId ?? '';
  }

  return {
    get winSentence() {
      return winSentence;
    },
    set winSentence(v) {
      winSentence = v;
    },
    get goal() {
      return goal;
    },
    set goal(v) {
      goal = v;
    },
    get linkedGoalId() {
      return linkedGoalId;
    },
    set linkedGoalId(v) {
      linkedGoalId = v;
    },
    get suggestion() {
      return suggestion;
    },
    get suggesting() {
      return suggesting;
    },
    suggestFocus,
    acceptSuggestion,
    dismissSuggestion,
    pickGoalLink,
    clearGoalLink,
    restore
  };
}
