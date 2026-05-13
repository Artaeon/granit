// Continue Writing — inline AI completion at the cursor.
//
// Trigger: Mod-Alt-Space at the cursor. Sends the last ~2000 chars
// before the cursor as context to /chat/stream and renders streamed
// chunks as ghost-text decoration after the cursor. Tab commits the
// ghost text (replaces the decoration with real text); Esc / typing
// rejects it. Mid-stream cancellation aborts the upstream request.
//
// Chord choice: Mod-Shift-Enter would be natural but is already
// bound to insert-checkbox. Mod-Alt-Space mirrors the Cursor /
// Copilot "expand at cursor" convention and is unclaimed.
//
// The ghost-text approach keeps the user in flow — they don't have
// to switch surfaces (modal / side panel) to accept a continuation.
// One muscle-memory keystroke triggers it; one more accepts.
//
// Why a stand-alone CM extension and not a wrapper around the
// existing Ask-AI dialog: the dialog is the right surface for
// rewrites + multi-step transformations on selected text. The
// continue case is a single stream of unselected text appended at
// cursor; the dialog UX (modal, paste, accept/reject buttons) is
// heavyweight for what is fundamentally a typing-assist gesture.

import { EditorView, Decoration, type DecorationSet, WidgetType } from '@codemirror/view';
import { StateField, StateEffect, type Extension } from '@codemirror/state';
import { keymap } from '@codemirror/view';
import { api, type ChatMessage } from '$lib/api';

// ─── State ─────────────────────────────────────────────────────────

interface GhostState {
  // Insertion anchor — the doc position where ghost text starts.
  // Stays anchored across user edits before this position by
  // adjusting in the field's update step. If the user types AFTER
  // the anchor, we treat it as a reject and clear.
  from: number;
  // The streamed text so far. Replaced wholesale on each chunk
  // (cheaper than rope-style append for the small completion sizes
  // we expect — typically under 500 chars).
  text: string;
  // Active = an in-flight or pending completion. Inactive means the
  // field exists but has no decoration to render.
  active: boolean;
}

const setGhost = StateEffect.define<Partial<GhostState> & { active?: boolean }>();
const clearGhost = StateEffect.define<null>();

const ghostField = StateField.define<GhostState>({
  create: () => ({ from: 0, text: '', active: false }),
  update(value, tr) {
    let next = value;
    // Adjust anchor if edits happened before/at the anchor.
    if (tr.docChanged && next.active) {
      const newFrom = tr.changes.mapPos(next.from, 1);
      // If a user edit landed AT the anchor (or past it), we treat
      // it as the user rejecting / overriding — clear the ghost.
      let userEditedAtAnchor = false;
      tr.changes.iterChanges((fromA, toA, fromB, toB) => {
        if (toB > newFrom || fromB === newFrom) userEditedAtAnchor = true;
      });
      if (userEditedAtAnchor) {
        next = { from: 0, text: '', active: false };
      } else {
        next = { ...next, from: newFrom };
      }
    }
    for (const e of tr.effects) {
      if (e.is(setGhost)) {
        next = { ...next, ...e.value, active: e.value.active ?? next.active };
      } else if (e.is(clearGhost)) {
        next = { from: 0, text: '', active: false };
      }
    }
    return next;
  },
  provide: (f) =>
    EditorView.decorations.from(f, (state) => {
      if (!state.active || state.text.length === 0) return Decoration.none;
      return Decoration.set([
        Decoration.widget({ widget: new GhostWidget(state.text), side: 1 }).range(state.from)
      ]);
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
    // Multi-line ghost text: split at `\n` and insert <br> so the
    // continuation visibly extends across paragraphs without
    // losing CodeMirror's line layout.
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
//
// One in-flight completion at a time. Aborting an old completion
// before starting a new one prevents stale chunks from landing on
// top of a new ghost.

let activeAbort: AbortController | null = null;

function buildContextMessages(view: EditorView): ChatMessage[] {
  const cur = view.state.selection.main.head;
  // Take up to 2000 chars before the cursor as the "what came
  // before" context. CM's sliceDoc handles efficient substring
  // even on big docs.
  const start = Math.max(0, cur - 2000);
  const before = view.state.sliceDoc(start, cur);
  // 400 chars after the cursor — gives the model some sense of
  // where the prose is heading so it doesn't write past existing
  // material.
  const after = view.state.sliceDoc(cur, Math.min(view.state.doc.length, cur + 400));

  const system =
    'You are a writing assistant continuing the user\'s prose at the cursor. ' +
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
  if (cur.from !== cur.to) return false; // need empty selection

  // Already streaming? Cancel and start a new one.
  activeAbort?.abort();
  activeAbort = new AbortController();

  view.dispatch({
    effects: setGhost.of({ from: cur.head, text: '', active: true })
  });

  const messages = buildContextMessages(view);
  let acc = '';
  // Throttle the ghost-widget re-paint to once per animation frame.
  // Pre-fix this dispatched a fresh setGhost on every streamed
  // chunk — for a typical 100-300 chunk LLM stream that's hundreds
  // of CodeMirror transactions in quick succession, each rebuilding
  // the ghost decoration widget DOM. The user reported the app
  // freezing during "continue writing" (and the user-perceptible
  // symptom of other AI features that stream into the editor was
  // the same root cause). rAF coalesces multiple chunks into one
  // visible frame: visible typing rate is unchanged, CPU usage
  // drops by an order of magnitude.
  let pendingFrame = 0;
  let dirty = false;
  const paint = () => {
    pendingFrame = 0;
    if (!dirty) return;
    dirty = false;
    const display = acc.replace(/^\s+/, '');
    view.dispatch({ effects: setGhost.of({ text: display, active: true }) });
  };
  void api.chatStream(
    messages,
    undefined,
    {
      onChunk: (chunk) => {
        acc += chunk;
        dirty = true;
        if (pendingFrame === 0) {
          pendingFrame = requestAnimationFrame(paint);
        }
      },
      onDone: () => {
        // Flush the trailing buffer immediately so the user sees the
        // FULL continuation the moment the stream ends, not on the
        // next frame after.
        if (pendingFrame !== 0) {
          cancelAnimationFrame(pendingFrame);
          pendingFrame = 0;
        }
        if (dirty) {
          dirty = false;
          const display = acc.replace(/^\s+/, '');
          view.dispatch({ effects: setGhost.of({ text: display, active: true }) });
        }
        activeAbort = null;
      },
      onError: (err) => {
        if (pendingFrame !== 0) {
          cancelAnimationFrame(pendingFrame);
          pendingFrame = 0;
        }
        activeAbort = null;
        view.dispatch({ effects: clearGhost.of(null) });
        // Surface as console + optional toast hook? We keep it
        // console-only here — the link-suggester / overlay surfaces
        // already toast user-facing errors. Continue-writing fires
        // silently and a no-result is the natural failure mode.
        // eslint-disable-next-line no-console
        console.warn('continue-writing:', err.message);
      }
    },
    activeAbort.signal
  );
  return true;
}

export function acceptContinuation(view: EditorView): boolean {
  const g = view.state.field(ghostField, false);
  if (!g || !g.active || g.text.length === 0) return false;
  view.dispatch({
    changes: { from: g.from, to: g.from, insert: g.text },
    selection: { anchor: g.from + g.text.length },
    effects: clearGhost.of(null),
    scrollIntoView: true
  });
  return true;
}

export function rejectContinuation(view: EditorView): boolean {
  const g = view.state.field(ghostField, false);
  if (!g || !g.active) return false;
  activeAbort?.abort();
  activeAbort = null;
  view.dispatch({ effects: clearGhost.of(null) });
  return true;
}

/** Read whether a ghost is currently rendered. Lets the host show a
 *  status pill while the user has a pending continuation. Cheap to
 *  poll from a $derived because StateField reads are O(1). */
export function isContinuationActive(view: EditorView | undefined): boolean {
  if (!view) return false;
  const g = view.state.field(ghostField, false);
  return !!g && g.active;
}

// ─── Public extension ──────────────────────────────────────────────

export function continueWritingExtension(): Extension {
  return [
    ghostField,
    keymap.of([
      {
        // Mod-Alt-Space — kick off a continuation. Three-key chord
        // makes it deliberate; doesn't shadow any existing binding.
        key: 'Mod-Alt-Space',
        run: (view) => continueWriting(view)
      },
      {
        // Tab — accept ghost text if active. Falls through if not so
        // CM's native Tab (indent / autocomplete pick) still works.
        key: 'Tab',
        run: (view) => acceptContinuation(view)
      },
      {
        // Escape — reject in-flight or pending ghost.
        key: 'Escape',
        run: (view) => rejectContinuation(view)
      }
    ])
  ];
}
