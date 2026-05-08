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
import { keymap, type EditorView, type KeyBinding } from '@codemirror/view';

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
  // Shared "fire ask-AI for this range" helper — used by both the
  // selection chord (Mod-Shift-A) and the section chord (Mod-Shift-S)
  // so the apply path is identical regardless of how the range was
  // produced.
  const fireForRange = (view: EditorView, from: number, to: number) => {
    const text = view.state.sliceDoc(from, to);
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
  };

  const selectionBinding: KeyBinding = {
    key: 'Mod-Shift-a',
    preventDefault: true,
    run: (view) => {
      if (view.state.readOnly) return false;
      const sel = view.state.selection.main;
      if (sel.from === sel.to) return false;
      // Capture the range now — when the user accepts the response
      // (after a few seconds of API round-trip + reading), the view's
      // main selection may have moved. Replays into the original
      // location feel right because the user's mental model is "I
      // asked the AI about THIS spot, put it back THERE".
      fireForRange(view, sel.from, sel.to);
      return true;
    }
  };

  // Mod-Shift-/ — operate on the section the cursor is inside.
  // A "section" runs from a heading line up to (but not including) the
  // next heading of the same or higher level. Cursor not inside any
  // heading? Use the whole doc up to the first heading. This makes
  // "summarise THIS section" a one-keystroke gesture without first
  // dragging a selection.
  const sectionBinding: KeyBinding = {
    key: 'Mod-Shift-/',
    preventDefault: true,
    run: (view) => {
      if (view.state.readOnly) return false;
      const range = currentSectionRange(view);
      if (!range) return false;
      fireForRange(view, range.from, range.to);
      return true;
    }
  };
  return keymap.of([selectionBinding, sectionBinding]);
}

interface Range {
  from: number;
  to: number;
}

// currentSectionRange — returns [from, to) for the markdown section
// containing the cursor. A section is delimited by ATX headings:
// it starts at the first heading at-or-before the cursor and ends
// just before the next heading whose level is <= the start heading's
// level. If no heading precedes the cursor, the section is the prelude
// (top of doc → first heading). Returns null only when the doc is
// empty.
function currentSectionRange(view: EditorView): Range | null {
  const doc = view.state.doc;
  if (doc.length === 0) return null;
  const cursorLine = doc.lineAt(view.state.selection.main.head).number;

  type HeadingLine = { line: number; level: number };
  const headings: HeadingLine[] = [];
  for (let i = 1; i <= doc.lines; i++) {
    const text = doc.line(i).text;
    const m = /^(#{1,6})\s+/.exec(text);
    if (m) headings.push({ line: i, level: m[1].length });
  }

  // Find the heading at-or-before the cursor. If none, the section is
  // [doc start, first heading).
  let startIdx = -1;
  for (let i = headings.length - 1; i >= 0; i--) {
    if (headings[i].line <= cursorLine) {
      startIdx = i;
      break;
    }
  }
  let fromLine: number;
  let level: number;
  if (startIdx < 0) {
    fromLine = 1;
    level = 0; // forces the loop below to terminate at the first heading
  } else {
    fromLine = headings[startIdx].line;
    level = headings[startIdx].level;
  }
  // End: first heading after fromLine whose level <= our level. If
  // we're in the prelude (level=0), any heading ends it.
  let toLineExclusive = doc.lines + 1;
  for (let i = startIdx + 1; i < headings.length; i++) {
    if (headings[i].level <= level || level === 0) {
      toLineExclusive = headings[i].line;
      break;
    }
  }
  const fromPos = doc.line(fromLine).from;
  const toPos = toLineExclusive > doc.lines ? doc.length : doc.line(toLineExclusive).from;
  if (fromPos === toPos) return null;
  return { from: fromPos, to: toPos };
}
