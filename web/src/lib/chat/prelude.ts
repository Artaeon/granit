// Build the prelude (system messages) prepended to every chat turn
// in the AI overlay. Extracted out of AIOverlay.svelte because the
// orchestration was the second-largest cohesive chunk in the god
// file — 120+ LOC of "first-turn vs every-turn" rules plus four
// contextual loaders. Pure-ish: takes a snapshot of the relevant
// flags + loader closures, returns the assembled ChatMessage[].
// The caller owns API access + reactive state; this module owns
// the policy (what goes in, when).
//
// First-turn rule recap (token-cost discipline):
//   - mode posture          → EVERY turn (cheap; one paragraph)
//   - capabilities system   → FIRST turn (model internalises)
//   - long-term memory      → FIRST turn (carried in conversation)
//   - mentioned entities    → ONLY the turn they were attached
//   - project / goal /
//     calendar context      → FIRST turn (token cost)
//   - vault snapshot        → FIRST turn (token cost)
//   - RAG hits              → EVERY turn the toggle is on (different
//                              query → different hits)

import type { ChatMessage, AIMemoryFact } from '$lib/api';
import type { AgentMode } from '$lib/ai/agents';
import type { MentionRef } from '$lib/components/MentionPicker.svelte';
import type { RagHit } from '$lib/chat/rag';
import { loadProjectContext, renderProjectContext } from '$lib/ai/projectManagerContext';
import { loadGoalContext, renderGoalContext } from '$lib/ai/goalManagerContext';
import { loadCalendarContext, renderCalendarContext } from '$lib/ai/calendarManagerContext';
import { retrieveForRag } from '$lib/chat/rag';

// Loader signature types — the underlying modules pass inline shapes
// to their factories; we re-name them here so PreludeInputs reads
// cleanly without nested object types. Keep in sync if the source
// loaders ever grow new dependencies.
export type ProjectContextLoaders = Parameters<typeof loadProjectContext>[1];
export type GoalContextLoaders = Parameters<typeof loadGoalContext>[1];
export type CalendarContextLoaders = Parameters<typeof loadCalendarContext>[0];

// Capabilities prompt — describes the structured-output channels
// (follow-ups + granit-action chips) the renderer parses out of the
// assistant reply. First turn only since the model internalises it
// after one example; re-injecting on every turn would burn ~400
// tokens per turn for no marginal lift.
export const AGENT_CAPABILITIES_SYSTEM = `Granit gives you two structured-output channels that turn parts of your reply into one-tap UI in the user's chat overlay. Use them when they would help the user act on your answer — not speculatively.

FOLLOW-UPS — at the very end of your reply (no other text after), append a single <followups>...</followups> block with up to 3 short prompts the user might naturally want next, one per line. Skip entirely when nothing useful comes to mind.

<followups>
Want me to break this into subtasks?
Should I draft the email reply?
</followups>

VAULT ACTIONS — when you propose creating something in the user's vault, emit a fenced "granit-action" JSON block:

\`\`\`granit-action
{"type":"task","text":"Call Anna about the contract","dueDate":"2026-05-12","priority":2}
\`\`\`

\`\`\`granit-action
{"type":"event","title":"Lunch with Sarah","start":"2026-05-12T13:00:00","end":"2026-05-12T14:00:00","location":"Centro"}
\`\`\`

\`\`\`granit-action
{"type":"note","title":"Reading list","body":"- Meditations\\n- The Brothers Karamazov\\n","folder":"Lists"}
\`\`\`

\`\`\`granit-action
{"type":"remember","content":"User's wife is named Anna","tags":["family"]}
\`\`\`

Fields: task.text required; dueDate/priority/notePath optional. event.title+start required; end/location optional. note.title+body required; folder optional. remember.content required; tags optional. priority is 1 (low) to 3 (high). Dates use YYYY-MM-DD; datetimes use floating ISO (no Z). Emit zero or many; the user picks which to commit.`;

export interface PreludeInputs {
  /** Active agent mode — posture string is appended every turn. */
  mode: AgentMode;
  /** Long-term memory facts — first-turn only. */
  aiMemoryFacts: ReadonlyArray<AIMemoryFact>;
  /** Currently-queued @-mentions — only the turn they're attached. */
  mentionedRefs: ReadonlyArray<MentionRef>;
  /** Page-aware context — empty string means "not in scope". */
  currentNotePath: string;
  currentProjectName: string;
  currentGoalId: string;
  onCalendarPage: boolean;
  /** First-turn flags + loaders for the contextual preludes. */
  attachSnapshot: boolean;
  snapshotData: unknown;
  rag: boolean;
  /** True for the FIRST turn of a thread; gates one-shot context. */
  isFirstTurn: boolean;
  /** The user's typed query — used as the RAG retrieval target. */
  query: string;
  /** Loader bundles. Each one is closure-injected so this module
   *  doesn't pull api.* directly. */
  projectLoaders: ProjectContextLoaders;
  goalLoaders: GoalContextLoaders;
  calendarLoaders: CalendarContextLoaders;
  todayISO: string;
}

export interface PreludeResult {
  /** System messages to prepend before the user turn. */
  messages: ChatMessage[];
  /** RAG hits used for this turn (empty when rag=false or empty).
   *  Callers thread this back into the per-turn sources map. */
  ragHits: RagHit[];
}

/**
 * Build the prelude for one chat turn. The caller has already
 * decided whether this is a first turn (messages.length === 0
 * at call time) and supplies the input snapshot — we never read
 * reactive state here so the rules stay testable.
 *
 * Order matters: posture first, then capabilities + memory, then
 * mentioned-entities, then page-scope context, then snapshot,
 * then RAG. The model reads top-to-bottom; we keep "who you are"
 * before "what you're looking at" before "your tools" before
 * "the user's actual question."
 */
export async function buildPrelude(inputs: PreludeInputs): Promise<PreludeResult> {
  const prelude: ChatMessage[] = [];

  // Mode posture — every turn (cheap; one paragraph). Keeps the
  // mode active even after history is long.
  prelude.push({ role: 'system', content: inputs.mode.system });

  // Agent capabilities — first turn only.
  if (inputs.isFirstTurn) {
    prelude.push({ role: 'system', content: AGENT_CAPABILITIES_SYSTEM });
  }

  // Long-term memory — first turn only. Skipped when the store is
  // empty so a fresh vault doesn't pay for an empty prelude.
  if (inputs.isFirstTurn && inputs.aiMemoryFacts.length > 0) {
    const lines = inputs.aiMemoryFacts.map((f) => `- ${f.content}`);
    prelude.push({
      role: 'system',
      content:
        "These are persistent facts the user has told Granit to remember about themselves. Use them when relevant — don't re-ask for context they've already given.\n\n" +
        lines.join('\n')
    });
  }

  // @-mentioned entity context. Strict, structured system message —
  // gives the model real fields rather than relying on the user's
  // prose to convey them. Injected only on the turn the mentions
  // are attached, then cleared by the caller so a follow-up doesn't
  // spam the same context.
  if (inputs.mentionedRefs.length > 0) {
    const lines = inputs.mentionedRefs.map((r) => `- ${r.contextLine}`);
    prelude.push({
      role: 'system',
      content:
        'The user has explicitly referenced these vault entities in their message. Use these fields when answering — do not invent ids or dates.\n\n' +
        lines.join('\n')
    });
  }

  // Project-scoped context. Skipped after first turn so a long
  // thread doesn't burn tokens re-asserting the context.
  if (inputs.isFirstTurn && inputs.currentProjectName) {
    try {
      const bundle = await loadProjectContext(inputs.currentProjectName, inputs.projectLoaders);
      prelude.push({
        role: 'system',
        content:
          "The user is currently looking at this project in Granit. Use it as the default subject of their messages — they don't need to re-state which project they mean.\n\n" +
          renderProjectContext(bundle)
      });
    } catch {
      // Project fetch failure — skip the injection silently.
    }
  }

  // Calendar context — date-window flavour rather than per-entity.
  // Fires when the user opens chat from /calendar and we're not
  // already loading project/goal scope.
  if (
    inputs.isFirstTurn &&
    inputs.onCalendarPage &&
    !inputs.currentProjectName &&
    !inputs.currentGoalId
  ) {
    try {
      const bundle = await loadCalendarContext(inputs.calendarLoaders, { todayISO: inputs.todayISO });
      prelude.push({
        role: 'system',
        content:
          "The user is currently looking at their calendar in Granit. Use this date-window context as the default subject of their messages.\n\n" +
          renderCalendarContext(bundle)
      });
    } catch {
      // Listing failure — skip silently.
    }
  }

  // Goal-scoped context — mirror of the project flow. Project wins
  // for the rare case of an overlapping URL.
  if (inputs.isFirstTurn && inputs.currentGoalId && !inputs.currentProjectName) {
    try {
      const bundle = await loadGoalContext(inputs.currentGoalId, inputs.goalLoaders);
      prelude.push({
        role: 'system',
        content:
          "The user is currently focused on this goal in Granit. Use it as the default subject of their messages — they don't need to re-state which goal they mean.\n\n" +
          renderGoalContext(bundle)
      });
    } catch {
      // Goal not found / fetch failure — skip silently.
    }
  }

  // Vault snapshot — non-note routes, first turn only.
  if (
    inputs.isFirstTurn &&
    inputs.attachSnapshot &&
    inputs.snapshotData &&
    !inputs.currentNotePath
  ) {
    prelude.push({
      role: 'system',
      content:
        "Here's a snapshot of the user's vault — today's events, " +
        'open tasks, recent notes, active goals, and deadlines. ' +
        'Refer to it when relevant; do not invent content beyond it.\n\n' +
        '```json\n' + JSON.stringify(inputs.snapshotData, null, 2) + '\n```'
    });
  }

  // RAG — runs on every turn the toggle is on, so a follow-up
  // question about a different topic retrieves different notes.
  // Composing with attachNote (note in system) is supported: pairing
  // both gives "explain this concept using my other notes too."
  let ragHits: RagHit[] = [];
  if (inputs.rag) {
    try {
      const hits = await retrieveForRag(inputs.query, inputs.currentNotePath);
      if (hits.length > 0) {
        ragHits = hits;
        const formatted = hits
          .map((h, i) => `### Note ${i + 1}: ${h.title}\nPath: \`${h.path}\`\n\n${h.excerpt}`)
          .join('\n\n---\n\n');
        prelude.push({
          role: 'system',
          content: `RAG retrieved ${hits.length} note(s) from the user's vault that match this query. Quote from these when relevant; cite the note title in your reply. Do NOT invent content beyond what's here. If they don't actually answer the question, say so plainly.\n\n${formatted}`
        });
      }
    } catch {
      // Retrieval failure shouldn't block the chat — fall through.
    }
  }

  return { messages: prelude, ragHits };
}
