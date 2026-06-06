// Cursor + scroll-progress observation for the notes editor.
//
// Wraps two small chunks of reactive state the Editor reports up:
//
//   cursorLine / cursorCol  — 1-indexed (matches what every editor
//                             status bar shows). Drives the
//                             status-bar cursor readout + the
//                             outline panel's active-line bias.
//   cursorSelLen            — selection length in characters; > 0
//                             means the user has a selection and the
//                             status bar surfaces a "{N} selected"
//                             badge.
//   readProgress            — 0..1 scroll position. When the doc
//                             fits in viewport the denominator is 0
//                             and we clamp to 0; once the user
//                             scrolls down a long doc we tint a 2px
//                             line at the top of the editor pane to
//                             surface 'how far through am I'.
//                             Cheap, no polling — the Editor's
//                             onScroll callback is already
//                             rAF-throttled at the source.
//
// Exposes ready-made onCursor + onScroll callbacks the Editor wires
// to via `onCursor={cb.onCursor}` / `onScroll={cb.onScroll}` — no
// route-side closures allocating per render.

export interface EditorCallbacks {
  readonly cursorLine: number;
  readonly cursorCol: number;
  readonly cursorSelLen: number;
  readonly readProgress: number;
  /** Wire as `onCursor` on the Editor component. */
  onCursor: (c: { line: number; col: number; selLen: number }) => void;
  /** Wire as `onScroll` on the Editor component. */
  onScroll: (s: { top: number; height: number; viewport: number }) => void;
}

export function createEditorCallbacks(): EditorCallbacks {
  let cursorLine = $state(1);
  let cursorCol = $state(1);
  let cursorSelLen = $state(0);
  let readProgress = $state(0);

  function onCursor(c: { line: number; col: number; selLen: number }): void {
    cursorLine = c.line;
    cursorCol = c.col;
    cursorSelLen = c.selLen;
  }

  function onScroll(s: { top: number; height: number; viewport: number }): void {
    const denom = Math.max(1, s.height - s.viewport);
    readProgress = Math.max(0, Math.min(1, s.top / denom));
  }

  return {
    get cursorLine() { return cursorLine; },
    get cursorCol() { return cursorCol; },
    get cursorSelLen() { return cursorSelLen; },
    get readProgress() { return readProgress; },
    onCursor,
    onScroll
  };
}
