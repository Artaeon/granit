// Selection → ask AI keybinding. Mirrors the extract-to-note flow:
// the keymap is side-effect-free, it just hands the selection up
// through a callback. The host page shows the modal, fires the
// /chat call, and on accept invokes the supplied apply() to splice
// the AI response into the document.
//
// Three apply modes the host can offer the user:
//   - replace: substitute the AI response for the original selection
//   - insertAfter: keep the original, append the response below
//   - copy: do nothing to the doc; user just copied the response
//
// Keeping the keybinding pure means tests can substitute mock
// dialogs and the chord composes cleanly with the existing keymap.

import { EditorSelection, type Extension } from '@codemirror/state';
import { keymap, type KeyBinding } from '@codemirror/view';

export interface AskAIRequest {
  /** The selected text — the prompt body sent to /chat. */
  text: string;
  /**
   * Replace the original selection with `replacement`. Used by the
   * "Replace selection" action in the dialog.
   */
  replace: (replacement: string) => void;
  /**
   * Insert `addition` immediately after the selection on its own
   * paragraph. Used by "Insert below" — keeps the original prompt
   * + the AI response side-by-side so the user can compare.
   */
  insertAfter: (addition: string) => void;
  /** Cancel — restore focus, no document edits. */
  cancel: () => void;
}

export function askAIKeymap(
  request: (req: AskAIRequest) => void
): Extension {
  const binding: KeyBinding = {
    key: 'Mod-Shift-a',
    preventDefault: true,
    run: (view) => {
      if (view.state.readOnly) return false;
      const sel = view.state.selection.main;
      if (sel.from === sel.to) return false;
      const text = view.state.sliceDoc(sel.from, sel.to);
      // Capture the range now — when the user accepts the response
      // (after a few seconds of API round-trip + reading), the view's
      // main selection may have moved. Replays into the original
      // location feel right because the user's mental model is "I
      // asked the AI about THIS spot, put it back THERE".
      const from = sel.from;
      const to = sel.to;
      request({
        text,
        replace: (replacement) => {
          view.dispatch({
            changes: { from, to, insert: replacement },
            selection: EditorSelection.cursor(from + replacement.length)
          });
          view.focus();
        },
        insertAfter: (addition) => {
          // Place a hard line break + the addition right after the
          // selection. If the cursor's not already at a line boundary
          // we add a leading newline so the addition starts fresh.
          const line = view.state.doc.lineAt(to);
          const atLineEnd = to === line.to;
          const prefix = atLineEnd ? '\n\n' : '\n\n';
          const insert = prefix + addition + (atLineEnd ? '' : '\n');
          view.dispatch({
            changes: { from: to, to: to, insert },
            selection: EditorSelection.cursor(to + insert.length)
          });
          view.focus();
        },
        cancel: () => {
          view.focus();
        }
      });
      return true;
    }
  };
  return keymap.of([binding]);
}
