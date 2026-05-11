// Selection-state plumbing for the editor's AI bar.
//
// The bar lives outside CodeMirror and re-renders when the user
// expands / shrinks the selection. Polling state on every render
// would mean the bar has no way to know "the selection changed" —
// CodeMirror owns the truth and only notifies via its own ViewPlugin
// lifecycle. So we wire a tiny ViewPlugin whose only job is to fire
// a callback whenever the main selection range moves, with the new
// from/to + the slice of the doc inside it.
//
// Kept in $lib/notes (next to the bar) rather than $lib/editor (next
// to CodeMirror primitives) because nothing else in the editor needs
// this — it's a notes-feature extension point, not a core editor
// concern. The pure deriveSelectionState() helper is exported so the
// bar can re-derive on demand (e.g. when it first mounts, after the
// view is already alive) without waiting for the next selection-set
// event.

import { ViewPlugin, type EditorView, type ViewUpdate } from '@codemirror/view';
import type { Extension } from '@codemirror/state';

export interface SelectionState {
  /** Document offset of the selection anchor (the lower of from/to). */
  from: number;
  /** Document offset of the selection head. Equal to `from` when no
   *  text is selected (cursor only). */
  to: number;
  /** The selected substring. Empty string when no text is selected. */
  text: string;
}

/** Pure read of the main selection from a live EditorView. Safe to
 *  call at any time the view is mounted. */
export function deriveSelectionState(view: EditorView): SelectionState {
  const main = view.state.selection.main;
  const from = Math.min(main.from, main.to);
  const to = Math.max(main.from, main.to);
  return {
    from,
    to,
    text: from === to ? '' : view.state.doc.sliceString(from, to)
  };
}

/** ViewPlugin that fires `onChange` whenever the main selection or
 *  the document changes (typing can shift the selection without a
 *  selectionSet event firing — e.g. delete-back collapses range to
 *  cursor). Coalesces back-to-back updates so the host only re-renders
 *  once per CodeMirror update batch. */
export function selectionStateExtension(
  onChange: (s: SelectionState) => void
): Extension {
  return ViewPlugin.fromClass(
    class {
      // Cache the last snapshot so we avoid notifying the host when
      // the user is just typing inside a collapsed selection (very
      // common: most keystrokes don't change from/to). Cheap field
      // compare beats round-tripping through the bar's reactive
      // recompute on every keystroke.
      private last: SelectionState;
      constructor(view: EditorView) {
        this.last = deriveSelectionState(view);
        // Fire once on mount so the bar paints its initial state
        // without waiting for the first user gesture.
        onChange(this.last);
      }
      update(u: ViewUpdate) {
        // selectionSet catches click / arrow-key moves; docChanged
        // catches typing (which can shift the cursor without a
        // selectionSet event). OR-ing both covers the cases where
        // the cursor moved relative to the doc — including when a
        // selection collapses or expands as a side effect of an
        // edit.
        if (!u.selectionSet && !u.docChanged) return;
        const next = deriveSelectionState(u.view);
        if (
          next.from === this.last.from &&
          next.to === this.last.to &&
          next.text === this.last.text
        ) {
          return;
        }
        this.last = next;
        onChange(next);
      }
    }
  );
}
