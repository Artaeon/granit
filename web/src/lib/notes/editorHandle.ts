// Shared shape for the CodeMirror Editor `bind:this` handle.
//
// Used by the +page.svelte route (stores the handle for cross-
// surface use: shortcuts, AI bar, lifecycle effects) and by
// NoteEditorPane.svelte (the inner pane that owns the bind:this).
// Hoisted to a .ts file because Svelte components don't have a
// stable `export interface` surface — module scripts work but the
// resolution from a sibling `import type` is fragile.

import type { EditorView } from '@codemirror/view';

export interface EditorHandle {
  scrollToLine: (n: number) => void;
  getScrollTop: () => number;
  setScrollTop: (top: number) => void;
  isCompletionActive: () => boolean;
  dispatchChord: (chord: string) => void;
  getDOM: () => HTMLElement | undefined;
  getView: () => EditorView | undefined;
  openFind: () => void;
  insertAtCursor: (text: string) => void;
  getContent: () => string;
}
