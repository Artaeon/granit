// Cross-pane context bus — the "AI is aware of its neighbors" piece
// of the granit vision.
//
// A single $state slot tracks the user's currently-focused item
// across every pane. Panes publish on focus / open (e.g. TasksPane
// publishes when a task detail drawer opens, CalendarPane publishes
// when an event is selected, the notes editor publishes when the
// active note changes); the ChatPane reads the bus and surfaces it
// as a "Context: …" chip above the composer + automatically threads
// it into the LLM prelude on send.
//
// Intentionally tiny — one global slot, one publish + clear method.
// Multiple panes don't compete; the most recent publish wins. A
// pane that no longer holds focus can call clear() OR just leave
// its last context standing until something else publishes; the
// chat surface treats both shapes the same.
//
// The bus is a module-level singleton (not a `createX()` factory)
// because the workspace shell shares one bus across every leaf — a
// factory would require plumbing the same instance through every
// pane via context, which contradicts the KISS rule. Module scope
// is the simplest cross-tree singleton in Svelte 5.

import type { PaneKind } from './paneRegistry';

export type WorkspaceContext = {
  /** Which pane produced this context. Used to label the chip and
   *  to route any future "open the source" actions. */
  paneKind: PaneKind;
  /** Stable identifier — task id, event id, note path, etc.
   *  Opaque to consumers; producers + downstream actions agree on
   *  the shape per paneKind. */
  itemId: string;
  /** Short user-facing label — task text, event title, note name. */
  label: string;
  /** Optional short excerpt the chat surface can include in the
   *  prelude — first paragraph of a note, event description,
   *  whatever helps the LLM "see" the context without bloating the
   *  prompt. */
  excerpt?: string;
};

let current = $state<WorkspaceContext | null>(null);

export const workspaceContext = {
  /** Current context, or null when nothing is published. */
  get current() {
    return current;
  },
  /** Publish a new context. Replaces any previous value. */
  publish(ctx: WorkspaceContext) {
    current = ctx;
  },
  /** Clear when the source pane no longer has a focused item.
   *  Panes don't have to call this — the next publish() replaces
   *  the value anyway. */
  clear() {
    current = null;
  }
};
