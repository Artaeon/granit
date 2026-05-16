// Inline AI — the single CodeMirror extension that backs every AI
// action in the editor. Notion-style: an action streams ghost text at
// the cursor (or alongside a selection it will replace); the user
// accepts with Tab/Cmd-Enter, rejects with Esc, or regenerates with
// Cmd-R while a ghost is active.
//
// Replaces the original continue-writing.ts. The "continue" preset is
// now just one entry in a longer menu — Improve, Fix grammar,
// Summarize, Translate, Expand, Brainstorm, Ask anything — all going
// through the same ghost-render pipeline so the muscle memory stays
// identical across actions.
//
// Three operating modes, picked at call time:
//
//   { kind: 'insert' }
//     Ghost text appears AFTER the cursor. Accept inserts the text at
//     the cursor (no doc was replaced). Used by Continue, Brainstorm,
//     and any free-form prompt at an empty selection.
//
//   { kind: 'replace', from, to }
//     Ghost text appears in place of the [from, to] selection. Accept
//     replaces that range with the streamed text. Used by Improve,
//     Fix grammar, Translate, etc. when run on a selection.
//
//   { kind: 'append', from }
//     Like insert but explicitly anchored at a given position rather
//     than the live cursor. Used when the action should land at the
//     end of the note regardless of where the cursor is.
//
// Why one extension instead of N per-action extensions: every action
// has the same lifecycle (start → stream → accept/reject) and the
// same UI artefact (ghost decoration + maybe a marker around the
// pending range). Splitting per-action would either duplicate the
// state field or fight over a shared one. One field + a discriminator
// is simpler and produces consistent behaviour across actions.

import { EditorView, Decoration, type DecorationSet, WidgetType } from '@codemirror/view';
import { StateField, StateEffect, type Extension } from '@codemirror/state';
import { keymap } from '@codemirror/view';
import { api, type ChatMessage } from '$lib/api';
import { rafThrottle } from '$lib/util/streamThrottle';

// ─── Public types ──────────────────────────────────────────────────

export type InlineAIKind = 'insert' | 'replace' | 'append';

export interface InlineAIRequest {
  kind: InlineAIKind;
  /** Anchor for insert/append. Ignored for replace (uses [from,to]). */
  anchor?: number;
  /** Range to replace. Required for kind='replace'. */
  from?: number;
  to?: number;
  /** Messages to send to /chat/stream. The system prompt steers the
   *  action; the user message carries the input text. */
  messages: ChatMessage[];
  /** Note path to attach as system context — same semantics as the
   *  notePath arg on api.chatStream. Backend appends the note body
   *  as a system message if provided. */
  notePath?: string;
  /** Called when streaming starts (before the first chunk). Lets the
   *  host show a "streaming…" pill or disable buttons. */
  onStart?: () => void;
  /** Called once after the stream finishes (success or error). */
  onSettled?: (info: { ok: boolean; text: string; error?: string }) => void;
}

// ─── State ─────────────────────────────────────────────────────────

interface InlineAIState {
  active: boolean;
  kind: InlineAIKind;
  /** Where the ghost decoration anchors. For insert/append this is
   *  the cursor position; for replace it's the start of the range. */
  anchor: number;
  /** Replace-mode only: end of the range to be replaced on accept. */
  replaceTo: number;
  /** Streamed text so far, displayed in the ghost widget. */
  text: string;
  /** The originating request — preserved so regenerate can re-run the
   *  exact same messages without the host having to remember them. */
  request: InlineAIRequest | null;
}

const EMPTY: InlineAIState = {
  active: false,
  kind: 'insert',
  anchor: 0,
  replaceTo: 0,
  text: '',
  request: null
};

const setInlineAI = StateEffect.define<Partial<InlineAIState> & { active?: boolean }>();
const clearInlineAI = StateEffect.define<null>();

const inlineAIField = StateField.define<InlineAIState>({
  create: () => EMPTY,
  update(value, tr) {
    let next = value;
    if (tr.docChanged && next.active) {
      // Re-anchor across edits so the ghost rides along with the doc.
      // If the user typed AT the anchor we treat it as a reject —
      // mirrors the continue-writing UX where typing dismisses ghost.
      const newAnchor = tr.changes.mapPos(next.anchor, 1);
      const newReplaceTo = tr.changes.mapPos(next.replaceTo, -1);
      let userEditedAtAnchor = false;
      tr.changes.iterChanges((_fromA, _toA, fromB, toB) => {
        if (next.kind === 'replace') {
          // For replace mode, any edit OUTSIDE the marked range
          // counts as a reject. Edits inside the range we tolerate
          // (the range moves with the edit).
          if (toB > newReplaceTo || fromB < newAnchor) userEditedAtAnchor = true;
        } else {
          if (toB > newAnchor || fromB === newAnchor) userEditedAtAnchor = true;
        }
      });
      if (userEditedAtAnchor) {
        next = EMPTY;
      } else {
        next = { ...next, anchor: newAnchor, replaceTo: newReplaceTo };
      }
    }
    for (const e of tr.effects) {
      if (e.is(setInlineAI)) {
        next = { ...next, ...e.value, active: e.value.active ?? next.active };
      } else if (e.is(clearInlineAI)) {
        next = EMPTY;
      }
    }
    return next;
  },
  provide: (f) =>
    EditorView.decorations.from(f, (state): DecorationSet => {
      if (!state.active) return Decoration.none;
      const widgets = [
        Decoration.widget({
          widget: new GhostWidget(state.text || '…'),
          side: 1
        }).range(state.anchor)
      ];
      // In replace mode, also paint a subtle background mark over the
      // range that will be overwritten so the user can see what's
      // about to change.
      if (state.kind === 'replace' && state.replaceTo > state.anchor) {
        widgets.push(
          Decoration.mark({ class: 'cm-inline-ai-target' }).range(state.anchor, state.replaceTo)
        );
      }
      return Decoration.set(widgets, true);
    })
});

class GhostWidget extends WidgetType {
  constructor(readonly text: string) {
    super();
  }
  eq(other: GhostWidget) {
    return other.text === this.text;
  }
  toDOM() {
    const span = document.createElement('span');
    span.className = 'cm-ghost-text';
    const parts = this.text.split('\n');
    parts.forEach((part, i) => {
      if (i > 0) span.appendChild(document.createElement('br'));
      span.appendChild(document.createTextNode(part));
    });
    return span;
  }
  ignoreEvent() {
    return true;
  }
}

// ─── Streaming controller ──────────────────────────────────────────
// One in-flight stream at a time. A second call cancels the first so
// you can't end up with two concurrent ghosts vying for the same
// anchor.

let activeAbort: AbortController | null = null;

/** Kick off an inline-AI action. Returns true if the call was
 *  accepted (request was well-formed and a stream was started). */
export function streamInlineAI(view: EditorView, req: InlineAIRequest): boolean {
  // Decide anchor + range.
  let anchor: number;
  let replaceTo: number;
  if (req.kind === 'replace') {
    if (req.from === undefined || req.to === undefined) return false;
    anchor = req.from;
    replaceTo = req.to;
  } else {
    anchor = req.anchor ?? view.state.selection.main.head;
    replaceTo = anchor;
  }

  activeAbort?.abort();
  activeAbort = new AbortController();

  view.dispatch({
    effects: setInlineAI.of({
      active: true,
      kind: req.kind,
      anchor,
      replaceTo,
      text: '',
      request: req
    })
  });

  req.onStart?.();

  // rAF throttle on chunk → at most one StateField update per frame
  // regardless of token rate. Without this, long streams thrash the
  // editor at hundreds of dispatches per second.
  const t = rafThrottle((full) => {
    const display = full.replace(/^\s+/, '');
    view.dispatch({ effects: setInlineAI.of({ text: display }) });
  });

  void api.chatStream(
    req.messages,
    req.notePath,
    {
      onChunk: t.onChunk,
      onDone: () => {
        t.flush();
        activeAbort = null;
        const state = view.state.field(inlineAIField, false);
        req.onSettled?.({ ok: true, text: state?.text ?? '' });
      },
      onError: (err) => {
        t.flush();
        activeAbort = null;
        view.dispatch({ effects: clearInlineAI.of(null) });
        req.onSettled?.({ ok: false, text: '', error: err.message });
        // eslint-disable-next-line no-console
        console.warn('inline-ai:', err.message);
      }
    },
    activeAbort.signal
  );
  return true;
}

/** Commit the pending ghost text into the doc. Insert/append put text
 *  at the anchor; replace overwrites [anchor, replaceTo]. */
export function acceptInlineAI(view: EditorView): boolean {
  const s = view.state.field(inlineAIField, false);
  if (!s || !s.active || s.text.length === 0) return false;
  const changes = s.kind === 'replace'
    ? { from: s.anchor, to: s.replaceTo, insert: s.text }
    : { from: s.anchor, to: s.anchor, insert: s.text };
  view.dispatch({
    changes,
    selection: { anchor: s.anchor + s.text.length },
    effects: clearInlineAI.of(null),
    scrollIntoView: true
  });
  return true;
}

/** Abort any in-flight stream and clear the ghost. */
export function rejectInlineAI(view: EditorView): boolean {
  const s = view.state.field(inlineAIField, false);
  if (!s || !s.active) return false;
  activeAbort?.abort();
  activeAbort = null;
  view.dispatch({ effects: clearInlineAI.of(null) });
  return true;
}

/** Re-run the exact request that produced the current ghost. Lets
 *  the user "try again" without retyping the prompt. No-op if no
 *  ghost is active. */
export function regenerateInlineAI(view: EditorView): boolean {
  const s = view.state.field(inlineAIField, false);
  if (!s || !s.active || !s.request) return false;
  return streamInlineAI(view, s.request);
}

/** Whether a ghost is currently rendered. Cheap O(1) field read so
 *  hosts can poll from a $derived to drive status pills. */
export function isInlineAIActive(view: EditorView | undefined): boolean {
  if (!view) return false;
  const s = view.state.field(inlineAIField, false);
  return !!s && s.active;
}

/** Snapshot of the current ghost state. Hosts use this to render a
 *  "streaming N chars" pill or expose a regenerate button. */
export function getInlineAIState(view: EditorView | undefined): InlineAIState | null {
  if (!view) return null;
  const s = view.state.field(inlineAIField, false);
  return s ?? null;
}

// ─── Continue-writing preset (legacy chord) ────────────────────────
// Mod-Alt-Space at an empty selection streams a free-form continuation
// of what came before the cursor. Same behavior as the old
// continue-writing extension; kept so muscle memory is preserved.

function buildContinueMessages(view: EditorView): ChatMessage[] {
  const cur = view.state.selection.main.head;
  const start = Math.max(0, cur - 2000);
  const before = view.state.sliceDoc(start, cur);
  const after = view.state.sliceDoc(cur, Math.min(view.state.doc.length, cur + 400));

  const system =
    "You are a writing assistant continuing the user's prose at the cursor. " +
    'Match their voice, register, and structure. Continue naturally — write 1-3 sentences ' +
    '(or 1-2 short paragraphs at most) that flow from the preceding text. ' +
    'Do NOT repeat what came before. Do NOT introduce headers or lists unless the surrounding text already uses them. ' +
    'Return ONLY the continuation text — no preamble, no quotes, no commentary. ' +
    'If the cursor is mid-sentence, complete the sentence first.';

  const user =
    'Text BEFORE cursor:\n```\n' + before + '\n```\n\n' +
    (after.trim().length > 0
      ? 'Text AFTER cursor (do not overwrite, just be aware):\n```\n' + after + '\n```\n\n'
      : '') +
    'Continue from the cursor:';

  return [
    { role: 'system', content: system },
    { role: 'user', content: user }
  ];
}

export function continueWriting(view: EditorView): boolean {
  const cur = view.state.selection.main;
  if (cur.from !== cur.to) return false;
  return streamInlineAI(view, {
    kind: 'insert',
    anchor: cur.head,
    messages: buildContinueMessages(view)
  });
}

// ─── Public extension ──────────────────────────────────────────────

export function inlineAIExtension(): Extension {
  return [
    inlineAIField,
    keymap.of([
      { key: 'Mod-Alt-Space', run: (view) => continueWriting(view) },
      { key: 'Tab', run: (view) => acceptInlineAI(view) },
      { key: 'Mod-Enter', run: (view) => acceptInlineAI(view) },
      { key: 'Escape', run: (view) => rejectInlineAI(view) },
      // Regenerate the current ghost. Mod-r is normally browser refresh
      // but inside a CodeMirror editor we can preventDefault it; the
      // chord only fires when a ghost is active (otherwise we return
      // false and the browser's refresh wins).
      { key: 'Mod-r', preventDefault: true, run: (view) => regenerateInlineAI(view) }
    ])
  ];
}
