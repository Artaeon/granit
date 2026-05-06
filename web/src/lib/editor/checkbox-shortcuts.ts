// Checkbox-line shortcuts. Two complementary chords for daily-note
// workflows where the user is rapidly capturing tasks while typing
// thoughts:
//
//   Mod-Shift-Enter — insert "- [ ] " at line start (creates a task
//                     on a fresh line, or converts the current
//                     plain line into a task).
//   Mod-Enter       — toggle the checkbox state on the current line
//                     ([ ] ⇄ [x]). No-op when the cursor isn't on a
//                     checkbox line.
//
// Both are no-ops when the doc is read-only. They work with multi-
// cursor: each cursor's line is treated independently.

import { EditorView, type KeyBinding } from '@codemirror/view';
import { EditorSelection } from '@codemirror/state';

const checkboxRe = /^(\s*[-*+]\s+\[)([ xX])(\])/;

function insertChecklistItem(view: EditorView): boolean {
  if (view.state.readOnly) return false;
  view.dispatch(
    view.state.changeByRange((range) => {
      const line = view.state.doc.lineAt(range.head);
      const trimmed = line.text.trimStart();
      // Already a checkbox line → no-op so the chord doesn't double-
      // up. The toggle chord (Mod-Enter) handles those.
      if (checkboxRe.test(trimmed)) {
        return { range, changes: [] };
      }
      // Empty line → insert in place.
      if (trimmed.length === 0) {
        const insert = '- [ ] ';
        return {
          changes: { from: line.from, insert },
          range: EditorSelection.cursor(line.from + insert.length)
        };
      }
      // Non-empty plain line → prepend "- [ ] " preserving any
      // leading indentation.
      const indent = line.text.slice(0, line.text.length - trimmed.length);
      const insert = `${indent}- [ ] `;
      // Replace the indent prefix with `indent + - [ ] ` so the
      // existing text stays intact and the cursor lands at the
      // checkbox text position.
      const cursorOffset = (range.head - line.from) - indent.length;
      return {
        changes: { from: line.from + indent.length, insert: '- [ ] ' },
        range: EditorSelection.cursor(line.from + insert.length + Math.max(0, cursorOffset))
      };
    })
  );
  return true;
}

function toggleCheckbox(view: EditorView): boolean {
  if (view.state.readOnly) return false;
  let didToggle = false;
  view.dispatch(
    view.state.changeByRange((range) => {
      const line = view.state.doc.lineAt(range.head);
      const m = line.text.match(checkboxRe);
      if (!m) return { range, changes: [] };
      didToggle = true;
      const next = m[2] === ' ' ? 'x' : ' ';
      // Position of the box character within the line text.
      const boxOffset = line.text.indexOf('[') + 1;
      const boxAbs = line.from + boxOffset;
      return {
        changes: { from: boxAbs, to: boxAbs + 1, insert: next },
        range
      };
    })
  );
  return didToggle;
}

export const checkboxShortcuts: readonly KeyBinding[] = [
  // Mod-Shift-Enter is the natural chord — Enter alone is the line
  // break, Mod-Shift-Enter is "but make it a checkbox" the same way
  // Mod-b is "but make it bold".
  { key: 'Mod-Shift-Enter', preventDefault: true, run: insertChecklistItem },
  // Mod-Enter toggles the box state on the current line. Returns
  // false when the cursor isn't on a checkbox line so other handlers
  // (or default Enter) can take the chord — important: don't shadow
  // Enter for non-checkbox lines.
  { key: 'Mod-Enter', preventDefault: true, run: toggleCheckbox }
];
