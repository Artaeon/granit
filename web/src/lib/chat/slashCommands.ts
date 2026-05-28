// Slash-command router for the AI overlay. Split out of
// AIOverlay.svelte because the help text + switch-statement was the
// single longest cohesive chunk in the god file — pure text + a
// dispatcher that hands every effectful action back to the caller
// via injected callbacks, so this module stays free of stores,
// API access, and Svelte runes.
//
// Design:
//   - SLASH_HELP is the long markdown blob rendered as an assistant
//     message when the user types `/help`. Kept here so any future
//     surface that wants to expose the same help can import the
//     same string without dragging the component along.
//   - handleSlashCommand returns true when it recognised the input
//     as a slash command and consumed it (the caller skips chat
//     send). The actual side effects live in the action map the
//     caller injects — the router knows what command + arg the
//     user typed, the host component knows what its closure
//     should do with that information.
//   - Argument parsing is the simplest possible split — the first
//     token is the command, everything after is the arg. Matches
//     the previous inline behaviour byte-for-byte.

import type { ChatMessage } from '$lib/api';
import type { AgentMode } from '$lib/ai/agents';

export const SLASH_HELP = `**Modes** (top-left in this panel)

  - **General** — balanced help across writing, planning, questions
  - **Research** — grounded answers, named sources, no invention (RAG on)
  - **Writer** — drafting partner, matches your voice
  - **Coach** — Socratic, questions over answers (RAG on)
  - **Analyst** — evidence-first, what would falsify the claim (RAG on)
  - **Architect** — trade-offs + recommendations for system design

**Personas** (sharper voices in the same picker)

  - **Lewis** — C.S. Lewis-style writing critic
  - **Aurelius** — Stoic counsel: brief, stern, kind
  - **Socrates** — questions over answers, sharpens half-formed thoughts
  - **Chrysostom** — scripture commentator in the classical tradition
  - **Founder** — operator coach, ship-this-week energy
  - **Magister** — patient tutor for technical concepts, slow and concrete
  - **Examen** — gentle bedtime companion, soft examen-style questions

  Toggle **RAG** to search the vault for relevant notes per question.

**Shortcuts**

  - <kbd>Mod+J</kbd> — toggle this panel
  - <kbd>Mod+1..9</kbd> — switch agent mode/persona by position
  - **🎤 mic** in the input row — voice dictation (browser STT)
  - **save** in the header — write the thread to \`chat-history/\` as a note

**Slash commands**

  - \`/help\` — show this list
  - \`/clear\` — reset the conversation (saves to history first)
  - \`/new\` — start a fresh thread (current is preserved)
  - \`/save\` — save the current thread under \`chat-history/\`
  - \`/briefing\` — daily briefing (today's events + tasks)
  - \`/synopsis\` — weekly synopsis (Wins / Setbacks / Learned / Next)
  - \`/triage\` — inbox triage proposals
  - \`/deadlines\` — detect deadlines in untimed tasks
  - \`/mode <id>\` — switch agent mode (general/research/writer/coach/analyst/architect)
  - \`/persona <id>\` — switch persona (lewis/aurelius/socrates/...)
  - \`/rag\` — toggle RAG retrieval for the next turn
  - \`/forget\` — drop snapshot/note attachment + queued @-mentions

**Reference vault entities**

  Type \`@\` in the composer to pop a picker for tasks, goals,
  projects, deadlines, events, and notes. Pick → the entity's
  fields (id, title, due date, status…) fold into a strict system
  message so the assistant grounds its reply in real data.

**Thread history**

  Every thread auto-saves to local storage (last 30, oldest drop).
  Click the clock icon in the header to browse + search your saved
  chats and pinned replies. Click ☆ on any assistant message to
  pin it across thread eviction. Click the fork glyph to branch
  the thread from that message into a new conversation.

**Where AI lives in granit**

  - **Note editor** — \`Mod-Shift-A\` ask about selection · \`Mod-Shift-/\` ask about section · \`Mod-Alt-Space\` continue writing · link suggester in the right rail
  - **/morning** — "Suggest from tasks" picks today's #1 focus
  - **/tasks** — "Top 3" focus picker · inbox triage · deadline detect
  - **/calendar** — "Plan my week" agent
  - **/goals** — "Suggest milestones" on goal detail
  - **/projects** — AI summary on project detail
  - **/vision** — "Harden vision" critic
  - **/examen** — gentle reflection prompts per section
  - **/people** — "Suggest 3" reach-outs based on cadence + notes
  - **/habits** — pattern insights from last 30 days

  Press <kbd>Mod+J</kbd> to toggle this panel anywhere in granit.`;

// Effectful slots the slash router hands control back to. The host
// component owns the actual implementations — that's where the
// reactive state lives — but the router decides which slot fires
// for which command and how the argument is shaped.
export interface SlashCommandHandlers {
  /** Append the verbatim user message + a synthesized assistant
   *  reply (used by /help, /memory). Lets us put help text and
   *  memory dumps in the persisted thread so the user can scroll
   *  back to them. */
  appendAssistantReply: (userText: string, assistantContent: string) => void;
  clearChat: () => void;
  startNewThread: () => void;
  saveThreadAsNote: () => void | Promise<void>;
  runBriefing: () => void | Promise<void>;
  runSynopsis: () => void | Promise<void>;
  runTriage: () => void | Promise<void>;
  runDeadlines: () => void | Promise<void>;
  /** /remember <fact> — adds a fact and refreshes memory. */
  rememberFact: (fact: string) => void | Promise<void>;
  /** /memory — render the current facts as an assistant message. */
  showMemory: (userText: string) => void | Promise<void>;
  /** /forget-fact <id-prefix> — delete by id-prefix. */
  forgetFact: (idPrefix: string) => void | Promise<void>;
  /** /mode <id> | /persona <id> — switch + announce. Fires the
   *  toast itself; the host's selectMode already handles announce()
   *  but the previous inline flow surfaced an additional success
   *  toast that this slot still owns. */
  selectModeAndToast: (mode: AgentMode) => void;
  /** Toast-only feedback for unknown mode/persona ids. */
  unknownModeOrPersona: (kind: 'mode' | 'persona', arg: string) => void;
  /** /rag — toggle + announce. Caller flips the rag flag itself. */
  toggleRag: () => void;
  /** /forget + /detach — drop snapshot/note/@mention queue. */
  detachContext: () => void;
  /** Usage-error toast for missing args. */
  usageError: (msg: string) => void;
  /** Move focus back into the composer (used after toggle-style
   *  commands so the user keeps typing). */
  refocusComposer: () => void;
}

/**
 * Routes a leading-slash composer string. Returns true when the
 * input was recognised + the matching handler invoked, false when
 * the prefix didn't match any command (caller falls through to
 * normal chat send so a pasted code block starting with "/" doesn't
 * get hijacked).
 *
 * Mirrors the previous inline switch byte-for-byte — same commands,
 * same arg parsing, same toast / refocus order.
 */
export function handleSlashCommand(
  raw: string,
  modes: ReadonlyArray<AgentMode>,
  h: SlashCommandHandlers
): boolean {
  const trimmed = raw.trim();
  const parts = trimmed.split(/\s+/);
  const cmd = parts[0].toLowerCase();
  const arg = parts.slice(1).join(' ').trim();
  switch (cmd) {
    case '/help':
      h.appendAssistantReply(raw, SLASH_HELP);
      return true;
    case '/clear':
      h.clearChat();
      return true;
    case '/new':
      h.startNewThread();
      return true;
    case '/save':
      void h.saveThreadAsNote();
      return true;
    case '/briefing':
      void h.runBriefing();
      return true;
    case '/synopsis':
      void h.runSynopsis();
      return true;
    case '/triage':
      void h.runTriage();
      return true;
    case '/deadlines':
      void h.runDeadlines();
      return true;
    case '/remember':
      if (!arg) {
        h.usageError('usage: /remember <fact about yourself>');
        return true;
      }
      void h.rememberFact(arg);
      return true;
    case '/memory':
      void h.showMemory(raw);
      return true;
    case '/forget-fact':
      if (!arg) {
        h.usageError('usage: /forget-fact <id-prefix>');
        return true;
      }
      void h.forgetFact(arg);
      return true;
    case '/mode':
    case '/persona': {
      if (!arg) {
        h.usageError(`usage: ${cmd} <id>`);
        return true;
      }
      const wanted = arg.toLowerCase();
      const target = modes.find(
        (m) => m.id.toLowerCase() === wanted || m.label.toLowerCase() === wanted
      );
      if (!target) {
        h.unknownModeOrPersona(cmd === '/mode' ? 'mode' : 'persona', arg);
        return true;
      }
      h.selectModeAndToast(target);
      return true;
    }
    case '/rag':
      h.toggleRag();
      h.refocusComposer();
      return true;
    case '/forget':
    case '/detach':
      h.detachContext();
      h.refocusComposer();
      return true;
    default:
      return false;
  }
}

// Helper builder for /memory's assistant content. Kept here so the
// renderer + the slash router agree on the format; the AIOverlay
// closure assembles it from its (already-loaded) memory facts.
export function formatMemoryAsAssistantContent(
  facts: ReadonlyArray<{ id: string; content: string; tags?: string[] }>
): string {
  if (facts.length === 0) {
    return '_(no long-term memory recorded yet. Use `/remember <fact>` to add one.)_';
  }
  const lines = facts
    .map(
      (f, i) =>
        `${i + 1}. ${f.content}${
          f.tags && f.tags.length > 0 ? ` _(${f.tags.join(', ')})_` : ''
        } · \`${f.id.slice(0, 6)}\``
    )
    .join('\n');
  return `**Long-term memory (${facts.length} fact${facts.length === 1 ? '' : 's'}):**\n\n${lines}\n\n_Use \`/forget-fact <id-prefix>\` to remove one._`;
}

// Re-export so the AIOverlay component doesn't need a second import
// from $lib/api for the type signature on appendAssistantReply.
export type { ChatMessage };
