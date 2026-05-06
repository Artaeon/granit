// Extract-to-note CodeMirror keybinding.
//
// Power-user flow that mirrors AmpleNote: select text in the current
// note, hit Mod-Shift-X, name the new note, and the selection is
// replaced with a wikilink while the selection's body is moved into
// a fresh note (with a backreference to where it came from).
//
// The keybinding itself doesn't make any HTTP calls — it just hands
// the selection up to the host page through a callback. The page
// handles the dialog UX, the API call, and (after confirmation) calls
// the supplied `apply(title)` to perform the in-document replacement.
// Keeping the keybinding side-effect-free means it composes cleanly
// in tests and lets the host substitute mock dialogs.

import { EditorSelection, type Extension } from '@codemirror/state';
import { keymap, type KeyBinding } from '@codemirror/view';

export interface ExtractRequest {
  /** The selected text — what gets carried into the new note body. */
  text: string;
  /** 1-based line number where the selection began (used to suggest a title). */
  fromLine: number;
  /**
   * Replaces the original selection with `[[title]]`. Caller invokes
   * this AFTER the new note is successfully created, so a failed
   * create doesn't leave the source note pointing at a dead wikilink.
   */
  apply: (title: string) => void;
  /**
   * Cancels the request — restores any UI state, no document edits.
   * Caller invokes this if the user dismisses the dialog.
   */
  cancel: () => void;
}

/**
 * Returns a CodeMirror extension that binds Mod-Shift-X to "extract
 * selection to a new note". The `request` callback receives the
 * selected text plus the apply/cancel handles. If no text is
 * selected the keybind is a no-op (returns false so other handlers
 * can take the chord).
 */
export function extractToNoteKeymap(
  request: (req: ExtractRequest) => void
): Extension {
  const binding: KeyBinding = {
    key: 'Mod-Shift-x',
    preventDefault: true,
    run: (view) => {
      if (view.state.readOnly) return false;
      const sel = view.state.selection.main;
      if (sel.from === sel.to) return false;
      const text = view.state.sliceDoc(sel.from, sel.to);
      const fromLine = view.state.doc.lineAt(sel.from).number;
      // Snapshot the selection range — we'll use it later when the
      // user confirms, after the dialog round-trip. The view's main
      // selection may have moved by then (the user could click into
      // the dialog input and then back).
      const from = sel.from;
      const to = sel.to;
      request({
        text,
        fromLine,
        apply: (title) => {
          const insert = `[[${title}]]`;
          view.dispatch({
            changes: { from, to, insert },
            selection: EditorSelection.cursor(from + insert.length)
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

/**
 * Suggests a note title from the selected text — first non-empty
 * line, trimmed, capped at 60 chars. Strips markdown decorations
 * (`#`, `-`, `*`, leading whitespace) so a heading or bullet item
 * becomes a clean title. Returns the empty string when there's
 * nothing usable; caller should fall back to a generic placeholder.
 */
export function suggestTitle(text: string): string {
  for (const raw of text.split('\n')) {
    let line = raw.trim();
    if (!line) continue;
    // Strip leading markdown markers: heading hashes, bullets,
    // numbered lists, blockquotes.
    line = line
      .replace(/^#+\s*/, '')
      .replace(/^[-*+]\s+/, '')
      .replace(/^\d+\.\s+/, '')
      .replace(/^>\s*/, '')
      .trim();
    if (!line) continue;
    if (line.length > 60) line = line.slice(0, 60).trim();
    return line;
  }
  return '';
}

/**
 * Slug-ifies a title into a vault-safe filename body (no extension).
 * Lowercase, alphanumerics + dashes only, collapses runs, trims
 * leading/trailing dashes. Mirrors the slugify behaviour of /notes
 * so a "+ New note" via either flow lands at the same path shape.
 */
export function slugifyTitle(title: string): string {
  return title
    .toLowerCase()
    .normalize('NFKD')
    // Strip combining diacritical marks (U+0300..U+036F).
    .replace(/[̀-ͯ]/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 80);
}
