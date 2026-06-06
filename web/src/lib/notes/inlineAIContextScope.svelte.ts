// Context-scope controller for InlineAIMenu — owns the toggles that
// shape what the AI sees alongside the user's instruction, and the
// lazy fetch / cache for each.
//
// State surface (all reactive, all $state inside the factory):
//
//   scope            — 'note' (default) sends the full note body via
//                      chatStream's notePath; 'section' instead sends
//                      only the current ##/### section the cursor is
//                      in as a focused system prefix. section mode
//                      omits notePath so the backend's auto-inject
//                      doesn't double up.
//   useLinkedNotes   — additive toggle. Fetches up to 6 entries
//                      (backlinks + outgoing wikilinks combined) with
//                      ~320-char body snippets so the model can reason
//                      over actual cross-note content, not just titles.
//   useRecentJots    — additive toggle. Fetches the last 7 daily notes
//                      with ~800-char body snippets. Useful when the
//                      user's question depends on context from the
//                      week's open threads.
//
// Memoization: detectedSection is captured once at construction
// because the cursor position is fixed for the menu's lifetime
// (closed + reopened = fresh event). The linked-notes and recent-
// jots fetches are similarly cached on the controller — the user
// toggling on/off mid-life shouldn't re-hit the server.
//
// effectiveNotePath is a $derived getter so the call site can pass
// it straight into streamInlineAI without re-deriving. We return
// `undefined` (not `''`) for section mode so the field is omitted
// from the request body entirely — chatStream treats `undefined`
// as "no note path", an explicit empty string would still round-
// trip a noisy `"notePath": ""`.

import type { EditorView } from '@codemirror/view';
import { api, type ChatMessage } from '$lib/api';

export type Scope = 'note' | 'section';

export interface DetectedSection {
  heading: string;
  body: string;
}

// Selection-surround budgets — chars before vs. after a selection
// passed into the model as read-only context so single-sentence
// rewrites don't drift in tone or terminology from the surrounding
// paragraphs. Asymmetric on purpose: "before" is what the reader has
// already absorbed by the time they hit the selection, so it carries
// more anchoring signal than "after".
const SELECTION_SURROUND_BEFORE = 600;
const SELECTION_SURROUND_AFTER = 300;

/** Pure helper — pulls trimmed before/after slices around a selection
 *  in the editor's doc. No state, no reactivity. Callers use this
 *  inside their prompt-build step. */
export function readSelectionSurround(
  view: EditorView,
  from: number,
  to: number
): { before: string; after: string } {
  const doc = view.state.doc;
  const beforeStart = Math.max(0, from - SELECTION_SURROUND_BEFORE);
  const afterEnd = Math.min(doc.length, to + SELECTION_SURROUND_AFTER);
  return {
    before: doc.sliceString(beforeStart, from).trimStart(),
    after: doc.sliceString(to, afterEnd).trimEnd()
  };
}

/** Detect the section at a cursor position — a contiguous block from
 *  the nearest heading down to the next heading at the same or higher
 *  level (or EOF). Returns null when the cursor is in pre-heading
 *  text (top of doc / no headings). */
function detectSectionAt(view: EditorView, pos: number): DetectedSection | null {
  const doc = view.state.doc;
  const startLine = doc.lineAt(pos).number;
  let headingLineNum = -1;
  let headingLevel = 0;
  for (let n = startLine; n >= 1; n--) {
    const line = doc.line(n);
    const m = line.text.match(/^(#{1,6})\s+(.+)$/);
    if (m) {
      headingLineNum = n;
      headingLevel = m[1].length;
      break;
    }
  }
  if (headingLineNum === -1) return null;
  const headingLine = doc.line(headingLineNum);
  const headingMatch = headingLine.text.match(/^(#{1,6})\s+(.+)$/);
  if (!headingMatch) return null;
  let endLineNum = doc.lines;
  for (let n = headingLineNum + 1; n <= doc.lines; n++) {
    const line = doc.line(n);
    const m = line.text.match(/^(#{1,6})\s+/);
    if (m && m[1].length <= headingLevel) {
      endLineNum = n - 1;
      break;
    }
  }
  const endLine = doc.line(endLineNum);
  return {
    heading: headingMatch[2].trim(),
    body: doc.sliceString(headingLine.from, endLine.to)
  };
}

// Per-link snippet budget. The handler caps at 400 chars; we re-
// truncate here to a tighter ceiling so the total context doesn't
// explode on densely-linked notes. The cap is on UTF-16 length, not
// tokens — close enough for our scale.
const LINKED_NOTE_SNIPPET_MAX = 320;
const LINKED_NOTES_CAP = 6; // backlinks + outgoing combined

export interface ContextScopeOpts {
  /** The trigger's editor view. Used to detect the section at the
   *  trigger position and as a doc handle for any future fetches
   *  that want to read more context. */
  view: EditorView;
  /** Cursor position at trigger time — fixed for the menu's life. */
  pos: number;
  /** Current note path. Used to fetch linked notes for the note. */
  notePath: string;
}

export interface ContextScopeController {
  scope: Scope;
  useLinkedNotes: boolean;
  useRecentJots: boolean;
  /** Memoized once at construction; null when the cursor is in
   *  pre-heading text. Drives the "§ <heading>" toggle visibility. */
  readonly detectedSection: DetectedSection | null;
  /** $derived — pass straight to streamInlineAI. `undefined` for
   *  section mode so the field is omitted from the request body. */
  readonly effectiveNotePath: string | undefined;
  /** Build the system+context message stack for a request. Includes
   *  the section block (section mode), linked-note snippets, and
   *  recent-jot bodies depending on toggles. Caches each fetch so
   *  the user toggling on/off doesn't re-hit the server. */
  buildContextMessages(systemHead: string): Promise<ChatMessage[]>;
}

export function createContextScopeController(
  opts: ContextScopeOpts
): ContextScopeController {
  let scope = $state<Scope>('note');
  let useLinkedNotes = $state(false);
  let useRecentJots = $state(false);

  const detectedSection = detectSectionAt(opts.view, opts.pos);

  // Lazy fetch caches. Captured as plain `let` (no reactivity) —
  // they're internal bookkeeping; consumers only see the awaited
  // result via buildContextMessages.
  let linkedNotesCache: string | null = null;
  let jotsCache: string | null = null;

  async function fetchLinkedNotes(): Promise<string> {
    if (linkedNotesCache !== null) return linkedNotesCache;
    try {
      // bodies=1 gets us snippet fields per link entry so the AI sees
      // actual content from connected notes, not just titles. Without
      // bodies the prompt is no better than telling the model "these
      // titles exist" — useless for cross-note reasoning.
      const r = await api.req<{
        outgoing: ({ title: string; path?: string; snippet?: string })[];
        backlinks: ({ title: string; path?: string; snippet?: string })[];
      }>(`/links/${encodeURI(opts.notePath)}?bodies=1`);

      // Interleave backlinks first then outgoing — backlinks tend to
      // carry deliberate connections (the other author chose to link
      // here), outgoing are this note's own references. Both useful
      // but backlinks are usually richer signal.
      const all = [
        ...(r.backlinks ?? []).map((b) => ({ ...b, direction: '←' as const })),
        ...(r.outgoing ?? []).map((o) => ({ ...o, direction: '→' as const }))
      ].slice(0, LINKED_NOTES_CAP);

      if (all.length === 0) {
        linkedNotesCache = '';
        return linkedNotesCache;
      }

      const blocks = all.map((entry) => {
        const snippet = (entry.snippet ?? '').slice(0, LINKED_NOTE_SNIPPET_MAX).trim();
        const head = `${entry.direction} [[${entry.title}]]${entry.path ? ' (' + entry.path + ')' : ''}`;
        return snippet ? `${head}\n${snippet}` : head;
      });

      linkedNotesCache =
        'Linked notes in the user\'s vault (← link IN to this note, → linked OUT from this note). ' +
        'Use these as background only — do not edit them, do not quote them verbatim unless asked.\n\n' +
        blocks.join('\n\n---\n\n');
      return linkedNotesCache;
    } catch {
      linkedNotesCache = '';
      return '';
    }
  }

  async function fetchRecentJots(): Promise<string> {
    if (jotsCache !== null) return jotsCache;
    try {
      const r = await api.listJots({ limit: 7 });
      const blocks = r.jots
        .slice(0, 7)
        .map((j) => `### ${j.date}\n${(j.body ?? '').slice(0, 800)}`);
      jotsCache = blocks.length === 0 ? '' : 'Last week of daily notes:\n\n' + blocks.join('\n\n');
      return jotsCache;
    } catch {
      jotsCache = '';
      return '';
    }
  }

  async function buildContextMessages(systemHead: string): Promise<ChatMessage[]> {
    const messages: ChatMessage[] = [{ role: 'system', content: systemHead }];
    // Section scope: include the section text as a focused system
    // prefix so the model anchors on it. The chatStream call site
    // omits notePath when scope === 'section' (see effectiveNotePath
    // below), preventing the backend from double-injecting the full
    // body on top of our targeted section.
    if (scope === 'section' && detectedSection) {
      messages.push({
        role: 'system',
        content:
          'Focus on the section "## ' + detectedSection.heading +
          '" of the user\'s note. Section content:\n\n```\n' +
          detectedSection.body + '\n```'
      });
    }
    if (useLinkedNotes) {
      const b = await fetchLinkedNotes();
      if (b) messages.push({ role: 'system', content: b });
    }
    if (useRecentJots) {
      const j = await fetchRecentJots();
      if (j) messages.push({ role: 'system', content: j });
    }
    return messages;
  }

  let effectiveNotePath = $derived(scope === 'note' ? opts.notePath : undefined);

  return {
    get scope() {
      return scope;
    },
    set scope(v: Scope) {
      scope = v;
    },
    get useLinkedNotes() {
      return useLinkedNotes;
    },
    set useLinkedNotes(v: boolean) {
      useLinkedNotes = v;
    },
    get useRecentJots() {
      return useRecentJots;
    },
    set useRecentJots(v: boolean) {
      useRecentJots = v;
    },
    get detectedSection() {
      return detectedSection;
    },
    get effectiveNotePath() {
      return effectiveNotePath;
    },
    buildContextMessages
  };
}
