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
// The bus is a process-wide singleton (not a `createX()` factory)
// because the workspace shell shares one bus across every leaf — a
// factory would require plumbing the same instance through every
// pane via context, which contradicts the KISS rule.
//
// IMPORTANT: the reactive `$state` lives in a class FIELD and the
// instance is created lazily on first access — never at module-init.
// A bare module-level `let current = $state(null)` hits a TDZ
// ReferenceError ("Cannot access 'X' before initialization") in
// concatenated/minified production bundles when this module's init
// runs before the Svelte 5 runtime finishes binding — the same crash
// that froze every view in commit dd71addc and that recentPanes /
// $isMobile were rewritten to avoid. Constructing the bus on first
// read (which always happens inside a component, well after every
// module's top-level init) keeps the reactivity ChatPane needs while
// closing that window.

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

// Reactive state held in a class field so the `$state` rune is only
// evaluated when the instance is constructed (lazily, below), not at
// module-init time. See the TDZ note in the header.
class ContextBus {
  current = $state<WorkspaceContext | null>(null);

  publish(ctx: WorkspaceContext) {
    this.current = ctx;
  }

  clear() {
    this.current = null;
  }
}

// Lazy singleton — first access constructs the bus. Because every
// caller reads through the accessors below (always from inside a
// component), construction happens after all module-level init, so
// there's no TDZ window and reactive reads still track `current`.
let _bus: ContextBus | null = null;
function bus(): ContextBus {
  return (_bus ??= new ContextBus());
}

export const workspaceContext = {
  /** Current context, or null when nothing is published. */
  get current() {
    return bus().current;
  },
  /** Publish a new context. Replaces any previous value. */
  publish(ctx: WorkspaceContext) {
    bus().publish(ctx);
  },
  /** Clear when the source pane no longer has a focused item.
   *  Panes don't have to call this — the next publish() replaces
   *  the value anyway. */
  clear() {
    bus().clear();
  }
};
