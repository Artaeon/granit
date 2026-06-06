// AI milestone-suggest controller for the goals surface.
//
// Streams a single concrete "what's the next milestone" suggestion
// via the same /chat/stream pipeline every AI feature in Granit
// uses, so the audit / Sabbath / redaction / cost guards apply
// uniformly. One in-flight suggestion at a time, keyed by goal id
// — only one card can be expanded.
//
//   • toggle(g)   — open the panel for g, or close if it's already
//                   open on g (re-rolling the same goal stays a
//                   single click via the visible "Try again" button
//                   which calls suggest() directly).
//   • suggest(g)  — fire a fresh stream. Cancels any prior in-flight
//                   stream first.
//   • adoptAsMilestone(g) — persist the current aiText as a
//                   milestone on g via api.addGoalMilestone, then
//                   close + reload.
//   • close()     — abort + clear state.
//
// State is private to the controller; the page reads through
// getters (open card id, streaming text, busy flag, error string)
// and calls the three methods above.

import { api, type Goal } from '$lib/api';
import { rafThrottle } from '$lib/util/streamThrottle';
import { errorMessage } from '$lib/util/errorMessage';
import { isAbortError } from '$lib/util/aiErrors';
import { toast } from '$lib/components/toast';
import type { GoalsDataController } from './goalsData.svelte';

export interface GoalsAiSuggestController {
  /** Goal id of the currently-open suggest panel, null when closed. */
  readonly aiGoalId: string | null;
  /** Streaming suggestion text. Trimmed + cleaned on stream completion. */
  readonly aiText: string;
  readonly aiBusy: boolean;
  readonly aiError: string;

  /** Open / close on the same goal, or open on a new one. */
  toggle(g: Goal): void;
  /** Re-fire the stream for a goal (the "Try again" CTA path). */
  suggest(g: Goal): Promise<void>;
  /** Persist the current aiText as a milestone on g. */
  adoptAsMilestone(g: Goal): Promise<void>;
  /** Abort the in-flight stream WITHOUT closing the panel. The
   *  onError handler still fires (via AbortError) so the panel
   *  shows an error message; the user can then Try again or
   *  Dismiss. */
  stop(): void;
  /** Abort + clear all panel state (the Dismiss path). */
  close(): void;
}

export interface GoalsAiSuggestDeps {
  dataCtl: GoalsDataController;
}

export function createGoalsAiSuggest(deps: GoalsAiSuggestDeps): GoalsAiSuggestController {
  let aiGoalId = $state<string | null>(null);
  let aiText = $state<string>('');
  let aiBusy = $state(false);
  let aiError = $state<string>('');
  let aiAbort: AbortController | null = null;

  function stop() {
    aiAbort?.abort();
  }

  function close() {
    aiAbort?.abort();
    aiAbort = null;
    aiGoalId = null;
    aiText = '';
    aiError = '';
    aiBusy = false;
  }

  function toggle(g: Goal) {
    if (aiGoalId === g.id) {
      close();
      return;
    }
    void suggest(g);
  }

  async function suggest(g: Goal) {
    // Always (re-)run — the "Try again" button calls this directly
    // for the currently-open goal. The toggle behaviour lives in
    // toggle() so re-rolling stays a single click.
    aiAbort?.abort();
    aiAbort = null;
    aiGoalId = g.id;
    aiBusy = true;
    aiError = '';
    aiText = '';
    aiAbort = new AbortController();

    const ms = g.milestones ?? [];
    const open = ms.filter((m) => !m.done).map((m) => m.text);
    const done = ms.filter((m) => m.done).map((m) => m.text);
    const roll = deps.dataCtl.rollupFor(g);
    // Compose a structured context block — keep it under ~2KB so the
    // prompt cost stays predictable. Only fields with content are
    // emitted, so a sparse goal yields a sparse prompt.
    const ctx = [
      `Goal: ${g.title}`,
      g.description ? `Description: ${g.description}` : '',
      g.target_date ? `Target date: ${g.target_date}` : '',
      g.venture ? `Venture: ${g.venture}` : '',
      g.project ? `Project: ${g.project}` : '',
      g.category ? `Category: ${g.category}` : '',
      open.length > 0 ? `Open milestones:\n${open.map((m) => `- ${m}`).join('\n')}` : '',
      done.length > 0 ? `Completed milestones:\n${done.map((m) => `- ${m}`).join('\n')}` : '',
      roll.open + roll.done > 0
        ? `Linked tasks: ${roll.open} open, ${roll.done} done`
        : ''
    ].filter(Boolean).join('\n\n');

    const userMessage =
      'Propose ONE concrete next milestone for this goal. Rules:\n' +
      '- One line, max ~12 words.\n' +
      '- Action-oriented, starts with a verb (Draft, Ship, Interview, Outline, …).\n' +
      "- Specific enough to know when it's done.\n" +
      '- Must move the goal forward from where it stands now (avoid restating done milestones).\n' +
      '- Output the milestone text only — no preamble, no quotes, no bullet, no period.\n\n' +
      'Goal context:\n\n' + ctx;

    // rAF throttle — aiText is rendered live so the user sees
    // streaming progress instead of a frozen panel.
    const goalT = rafThrottle((full) => { aiText = full; });
    try {
      await api.chatStream(
        [{ role: 'user', content: userMessage }],
        undefined,
        {
          onChunk: goalT.onChunk,
          onDone: () => {
            goalT.flush();
            aiBusy = false;
            aiAbort = null;
            // Trim once at end so the streaming UI shows tokens
            // exactly as they arrive but the final value is clean.
            aiText = aiText.trim().replace(/^["'\-•*]+\s*/, '').replace(/\.\s*$/, '');
            if (!aiText) aiError = 'AI returned an empty suggestion.';
          },
          onError: (err) => {
            goalT.flush();
            aiBusy = false;
            aiAbort = null;
            if (isAbortError(err)) return;
            aiError = err.message;
          }
        },
        aiAbort.signal
      );
    } catch (e) {
      aiBusy = false;
      aiAbort = null;
      aiError = errorMessage(e);
    }
  }

  async function adoptAsMilestone(g: Goal) {
    const text = aiText.trim();
    if (!text) return;
    try {
      await api.addGoalMilestone(g.id, { text });
      toast.success('milestone added');
      close();
      await deps.dataCtl.load();
    } catch (err) {
      toast.error('Add failed: ' + errorMessage(err));
    }
  }

  return {
    get aiGoalId() {
      return aiGoalId;
    },
    get aiText() {
      return aiText;
    },
    get aiBusy() {
      return aiBusy;
    },
    get aiError() {
      return aiError;
    },
    toggle,
    suggest,
    adoptAsMilestone,
    stop,
    close
  };
}
