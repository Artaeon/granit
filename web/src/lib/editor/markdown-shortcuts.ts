// Markdown formatting shortcuts for the CodeMirror editor: Ctrl-B
// wraps the selection in **, Ctrl-I in *, Ctrl-K wraps as a markdown
// link. Cross-platform — `Mod-` resolves to Cmd on macOS and Ctrl on
// everything else, matching the rest of the app's keymaps.
//
// Each command is a no-op when the doc is read-only (CodeMirror
// short-circuits via state.facet(EditorState.readOnly)).

import { EditorView, type KeyBinding } from '@codemirror/view';
import { EditorSelection } from '@codemirror/state';

// Toggle a symmetric inline marker (** for bold, * for italic) around
// each selection. If the selection already starts and ends with the
// marker we strip it (a second Ctrl-B unbolds), otherwise we wrap.
// Empty selections insert the marker pair and place the cursor between
// them — same UX as Obsidian / iA Writer / VS Code's markdown mode.
function toggleWrap(view: EditorView, marker: string): boolean {
  if (view.state.readOnly) return false;
  view.dispatch(
    view.state.changeByRange((range) => {
      const text = view.state.sliceDoc(range.from, range.to);
      // Already wrapped → strip. Use length compare to avoid false
      // positives on short selections like "**" itself.
      if (
        text.length >= marker.length * 2 &&
        text.startsWith(marker) &&
        text.endsWith(marker)
      ) {
        const inner = text.slice(marker.length, text.length - marker.length);
        return {
          changes: { from: range.from, to: range.to, insert: inner },
          range: EditorSelection.range(range.from, range.from + inner.length)
        };
      }
      // Empty selection → insert pair and put the cursor in the middle.
      if (range.from === range.to) {
        const insert = marker + marker;
        return {
          changes: { from: range.from, insert },
          range: EditorSelection.cursor(range.from + marker.length)
        };
      }
      const insert = marker + text + marker;
      return {
        changes: { from: range.from, to: range.to, insert },
        range: EditorSelection.range(range.from + marker.length, range.from + marker.length + text.length)
      };
    })
  );
  return true;
}

// Ctrl-K: turn selection into a markdown link. If the clipboard is
// not pre-staged with a URL (we can't read clipboard from a keybind
// without async permission flows that are awkward in CM), we use the
// pattern [selection]() and place the cursor inside the parens so the
// user pastes the URL there. Empty selection inserts [](text url) and
// puts the cursor on the link text.
function makeLink(view: EditorView): boolean {
  if (view.state.readOnly) return false;
  view.dispatch(
    view.state.changeByRange((range) => {
      const text = view.state.sliceDoc(range.from, range.to);
      if (range.from === range.to) {
        const insert = '[]()';
        return {
          changes: { from: range.from, insert },
          range: EditorSelection.cursor(range.from + 1) // between the brackets
        };
      }
      const insert = `[${text}]()`;
      // Cursor inside the parens — user pastes URL there. The brackets
      // already wrap their selected text.
      return {
        changes: { from: range.from, to: range.to, insert },
        range: EditorSelection.cursor(range.from + insert.length - 1)
      };
    })
  );
  return true;
}

export const markdownShortcuts: readonly KeyBinding[] = [
  { key: 'Mod-b', preventDefault: true, run: (v) => toggleWrap(v, '**') },
  { key: 'Mod-i', preventDefault: true, run: (v) => toggleWrap(v, '*') },
  // Single `_` for italic too — some users prefer underscores. Same
  // toggle behaviour, accepts the alt shortcut.
  { key: 'Mod-Shift-i', preventDefault: true, run: (v) => toggleWrap(v, '_') },
  { key: 'Mod-k', preventDefault: true, run: makeLink }
];

// Smart paste: when the user pastes a URL while text is selected,
// replace the selection with [selection](url). When pasting plain
// text or pasting on an empty selection, fall through to CodeMirror's
// default paste handling.
//
// Detected URLs are HTTP(S) only — bare www. and other shapes pasted
// as plain text shouldn't be silently rewritten because the user
// might intend the literal text. Conservative is right here.
const httpUrl = /^(https?:\/\/[^\s]+)$/;

export const smartPaste = EditorView.domEventHandlers({
  paste(event, view) {
    if (view.state.readOnly) return false;
    const clip = event.clipboardData?.getData('text/plain') ?? '';
    if (!clip) return false;
    const trimmed = clip.trim();
    if (!httpUrl.test(trimmed)) return false;
    const sel = view.state.selection.main;
    if (sel.from === sel.to) return false; // no selection — let default handle
    const selectedText = view.state.sliceDoc(sel.from, sel.to);
    // Don't auto-wrap if the selection itself looks like a URL — the
    // user may be pasting a URL replacement, in which case we want
    // the plain paste behaviour.
    if (httpUrl.test(selectedText.trim())) return false;
    event.preventDefault();
    const insert = `[${selectedText}](${trimmed})`;
    view.dispatch({
      changes: { from: sel.from, to: sel.to, insert },
      selection: EditorSelection.cursor(sel.from + insert.length)
    });
    return true;
  }
});
