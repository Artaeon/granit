// Inline-AI trigger — the two ways a user opens the AI menu.
//
//   1. Mod-/ (Cmd-/ on Mac, Ctrl-/ elsewhere) — opens the menu at
//      the cursor regardless of what's typed there. Works with or
//      without a selection. Power-user shortcut. Pairs naturally
//      with the "/ai" slash trigger ("the chord for the / surface").
//
//      Originally Mod-K, but that's already taken by the global
//      CommandPalette (universal search) and markdown-shortcuts
//      (make-link), and three handlers vying for the same chord
//      made the trigger unreliable.
//
//   2. Typing "/ai" at the start of a line — opens the menu inline
//      so a user who lives in the keyboard but doesn't know the
//      chord can discover the feature by trial. The "/ai" trigger
//      string is consumed (deleted from the doc) when the menu
//      picks any action, so the eventual insertion replaces the
//      trigger rather than appearing after it.
//
// Both paths land in the same callback the host page passes in. The
// host owns the popover positioning + mount; this extension just
// reports "user wants the menu, here's the anchor + current state".
//
// Anchor coords are screen-relative so the host can place a fixed-
// positioned popover that survives the editor's internal scroll
// container. Coords are derived from CodeMirror's coordsAtPos —
// returns null if the position isn't currently laid out (e.g. far
// off-screen), in which case we fall back to the editor scroll
// rect's top-left so the menu still appears somewhere sensible.

import { EditorView, keymap, ViewPlugin, type ViewUpdate } from '@codemirror/view';
import { type Extension } from '@codemirror/state';

export interface InlineAITriggerEvent {
  /** Viewport-relative anchor coordinates for the popover. */
  x: number;
  y: number;
  /** Doc position the trigger fired at — host can use this to know
   *  where the eventual streamInlineAI insert/replace should land. */
  pos: number;
  /** When typed-trigger fires, this is the [from, to] range of the
   *  "/ai" text in the doc; the host should remove it before
   *  inserting AI output so the trigger string doesn't end up in
   *  the saved note. Undefined for chord-triggered opens. */
  triggerRange?: { from: number; to: number };
  /** Selection at trigger time — empty range when no selection. The
   *  menu uses this to switch between "operate at cursor" and
   *  "rewrite this selection" modes. */
  selection: { from: number; to: number; text: string };
  /** The EditorView that fired the trigger — passed straight to
   *  streamInlineAI when the menu eventually runs. */
  view: EditorView;
}

function makeEvent(
  view: EditorView,
  pos: number,
  triggerRange: { from: number; to: number } | undefined
): InlineAITriggerEvent {
  const coords = view.coordsAtPos(pos);
  const rect = view.scrollDOM.getBoundingClientRect();
  // coordsAtPos can return null for positions outside the rendered
  // line range; fall back to the editor's scroll-rect origin so the
  // popover at least appears on-screen.
  const x = coords ? coords.left : rect.left + 16;
  const y = coords ? coords.bottom + 4 : rect.top + 16;
  const sel = view.state.selection.main;
  return {
    x,
    y,
    pos,
    triggerRange,
    selection: {
      from: sel.from,
      to: sel.to,
      text: sel.from === sel.to ? '' : view.state.sliceDoc(sel.from, sel.to)
    },
    view
  };
}

// "/ai" inline trigger. Only fires when the user has just typed the
// third character of "/ai" at the start of a line (after whitespace
// only) — avoids hijacking the chord inside paths like "config/ai/foo"
// or "https://...". The regex check intentionally allows the prefix
// to land anywhere on a line of pure whitespace; "  /ai" works the
// same as a flush "/ai". The trigger string is reported as
// triggerRange so the host can consume it when an action runs.
function makeTypedTriggerPlugin(onTrigger: (e: InlineAITriggerEvent) => void) {
  return ViewPlugin.fromClass(
    class {
      constructor(public view: EditorView) {}
      update(u: ViewUpdate) {
        if (!u.docChanged) return;
        // Only react to insertions, not deletions. We fire on the
        // transition from "/a" → "/ai" so the trigger is exactly the
        // moment the user finishes typing the chord. Multiple changes
        // in one transaction (e.g. paste) we ignore — pasting "/ai"
        // shouldn't pop a menu.
        let inserted: { from: number; to: number; text: string } | null = null;
        let n = 0;
        u.changes.iterChanges((_fA, _tA, fB, tB, ins) => {
          n += 1;
          inserted = { from: fB, to: tB, text: ins.toString() };
        });
        if (n !== 1 || !inserted) return;
        // Re-type to satisfy TS narrow loss across iter callback.
        const ch: { from: number; to: number; text: string } = inserted;
        // Final character must be "i" or "I", and the doc state must
        // now have "/ai" ending at the inserted range.
        if (ch.text.length === 0) return;
        const endChar = u.state.sliceDoc(ch.to - 1, ch.to);
        if (endChar !== 'i' && endChar !== 'I') return;
        const lineAt = u.state.doc.lineAt(ch.to);
        const lineHead = u.state.sliceDoc(lineAt.from, ch.to);
        const match = lineHead.match(/(^|\s)(\/ai)$/i);
        if (!match) return;
        const triggerFrom = ch.to - match[2].length;
        // Defer the callback by one microtask so any other plugins
        // listening to this transaction finish first. The host will
        // typically open a popover, which triggers DOM measurement —
        // letting CM settle before measuring avoids a layout thrash.
        queueMicrotask(() => {
          onTrigger(makeEvent(this.view, triggerFrom, { from: triggerFrom, to: ch.to }));
        });
      }
    }
  );
}

export function inlineAITriggerExtension(
  onTrigger: (e: InlineAITriggerEvent) => void
): Extension {
  return [
    makeTypedTriggerPlugin(onTrigger),
    keymap.of([
      {
        // Mod-/ opens the inline-AI menu at the cursor or over the
        // current selection. Mirrors the discoverability path (the
        // user types `/ai` to open the same menu — the chord is
        // "the shortcut for the / surface").
        //
        // Originally bound to Mod-K, but K is already taken by both
        // the global CommandPalette (universal search) AND
        // markdown-shortcuts.ts (make-link). Three handlers fighting
        // for Mod-K made the trigger unreliable. Mod-/ is otherwise
        // unbound in this editor (we don't enable CodeMirror's
        // toggle-comment), so the menu opens cleanly.
        key: 'Mod-/',
        preventDefault: true,
        run: (view) => {
          const pos = view.state.selection.main.head;
          queueMicrotask(() => onTrigger(makeEvent(view, pos, undefined)));
          return true;
        }
      }
    ])
  ];
}
