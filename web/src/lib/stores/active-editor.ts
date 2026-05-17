// Active editor handle — registry for the currently-mounted CodeMirror
// view, so cross-surface features (AIOverlay's "insert at cursor",
// future "drop into note" actions) can write into the live note
// without each feature needing its own coupling to the editor page.
//
// One editor at a time. The notes editor page calls register(view) on
// mount and register(null) on destroy. Any surface that wants to drop
// text into the doc reads $hasActiveEditor first, then calls
// insertAtCursor(text) — both no-op safely when there's no editor up.
//
// We deliberately don't expose the full EditorView via the store —
// only the narrow "insert text at cursor" surface. Widens it later
// only if a real use case turns up that justifies the coupling.

import { writable, derived } from 'svelte/store';
import type { EditorView } from '@codemirror/view';

const activeView = writable<EditorView | null>(null);

/** True when an editor is currently mounted. Surfaces in components
 *  (e.g. AIOverlay) gate "insert at cursor" buttons on this so the
 *  affordance only appears when it would actually work. */
export const hasActiveEditor = derived(activeView, (v) => v !== null);

/** Register an editor view as the current insert target. Pass null
 *  on destroy. Only the most recent register wins — if a second
 *  editor mounts before the first unmounts (unlikely but possible
 *  on rapid navigation), the second takes over and the first will
 *  be cleared by the next register(null). */
export function registerActiveEditor(view: EditorView | null): void {
  activeView.set(view);
}

/** Insert text at the editor's current cursor position. Returns true
 *  if an editor was available and the insert dispatched, false
 *  otherwise. Callers can use the return value to surface a toast
 *  on miss, though usually the gate is hasActiveEditor before the
 *  button renders so a miss should be impossible.
 *
 *  The inserted text replaces any current selection — same semantics
 *  as paste. Cursor lands after the inserted text and we scroll
 *  it into view so the user immediately sees what landed. */
export function insertAtCursor(text: string): boolean {
  let inserted = false;
  activeView.update((v) => {
    if (!v) return v;
    const sel = v.state.selection.main;
    v.dispatch({
      changes: { from: sel.from, to: sel.to, insert: text },
      selection: { anchor: sel.from + text.length },
      scrollIntoView: true
    });
    v.focus();
    inserted = true;
    return v;
  });
  return inserted;
}
