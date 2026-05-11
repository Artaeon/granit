import { describe, expect, it } from 'vitest';
import { EditorSelection, EditorState } from '@codemirror/state';
import { EditorView } from '@codemirror/view';

import {
  deriveSelectionState,
  selectionStateExtension,
  type SelectionState
} from './selectionState';

// Spin up a headless EditorView for selection-state assertions. We
// never paint, so jsdom's detached parent is fine.
function mount(initial = '', extensions: ReturnType<typeof selectionStateExtension>[] = []): EditorView {
  const state = EditorState.create({ doc: initial, extensions });
  return new EditorView({ state, parent: document.createElement('div') });
}

describe('deriveSelectionState', () => {
  it('returns empty text and equal from/to when there is no selection', () => {
    const view = mount('hello world');
    const s = deriveSelectionState(view);
    expect(s.from).toBe(0);
    expect(s.to).toBe(0);
    expect(s.text).toBe('');
    view.destroy();
  });

  it('returns the selected slice for a forward range', () => {
    const view = mount('hello world');
    view.dispatch({ selection: EditorSelection.range(0, 5) });
    const s = deriveSelectionState(view);
    expect(s.from).toBe(0);
    expect(s.to).toBe(5);
    expect(s.text).toBe('hello');
    view.destroy();
  });

  it('normalises a backward selection (anchor > head) to ascending from/to', () => {
    // The bar needs from <= to regardless of which direction the
    // user dragged. CodeMirror keeps anchor/head as-given; this
    // helper smooths that wrinkle for downstream code.
    const view = mount('hello world');
    view.dispatch({ selection: EditorSelection.range(11, 6) }); // head before anchor
    const s = deriveSelectionState(view);
    expect(s.from).toBe(6);
    expect(s.to).toBe(11);
    expect(s.text).toBe('world');
    view.destroy();
  });
});

describe('selectionStateExtension', () => {
  it('fires once on mount with the initial selection', () => {
    const events: SelectionState[] = [];
    const view = mount('hello', [selectionStateExtension((s) => events.push(s))]);
    expect(events.length).toBe(1);
    expect(events[0]).toEqual({ from: 0, to: 0, text: '' });
    view.destroy();
  });

  it('fires when the selection range changes', () => {
    const events: SelectionState[] = [];
    const view = mount('hello world', [
      selectionStateExtension((s) => events.push(s))
    ]);
    // Drop the mount-time fire so the rest of the assertion is about
    // selection moves only.
    events.length = 0;
    view.dispatch({ selection: EditorSelection.range(0, 5) });
    expect(events.length).toBe(1);
    expect(events[0].text).toBe('hello');
    // Collapse to a cursor — must also notify (empty text now).
    view.dispatch({ selection: EditorSelection.cursor(3) });
    expect(events.length).toBe(2);
    expect(events[1]).toEqual({ from: 3, to: 3, text: '' });
    view.destroy();
  });

  it('does NOT fire when the user types inside a collapsed selection without moving from/to', () => {
    // Typing at the end advances the cursor → from/to changes → we
    // notify. But re-dispatching the same selection (a no-op) must
    // NOT churn the bar. Verifies the cheap field-compare guard.
    const events: SelectionState[] = [];
    const view = mount('hello', [selectionStateExtension((s) => events.push(s))]);
    events.length = 0;
    // Re-dispatch the SAME selection — listener must stay silent.
    view.dispatch({ selection: EditorSelection.cursor(0) });
    expect(events.length).toBe(0);
    view.destroy();
  });

  it('fires when the text inside the selected range changes (e.g. external doc rewrite)', () => {
    // An external replace-body action (the AI Rewrite flow) changes
    // the doc under a collapsed cursor. The slice covered by from..to
    // is still empty either way, but the from/to may move when the
    // doc shrinks. Either path must notify so the bar's count chip
    // stays accurate.
    const events: SelectionState[] = [];
    const view = mount('hello world', [
      selectionStateExtension((s) => events.push(s))
    ]);
    view.dispatch({ selection: EditorSelection.range(0, 5) });
    events.length = 0;
    // Rewrite the selected range — the new text replaces "hello".
    view.dispatch({ changes: { from: 0, to: 5, insert: 'goodbye' } });
    expect(events.length).toBeGreaterThanOrEqual(1);
    const last = events[events.length - 1];
    // After the replace the selection collapses to a cursor at the
    // insertion point (CodeMirror default), so text is empty but
    // from/to should be inside the new doc bounds.
    expect(last.from).toBeGreaterThanOrEqual(0);
    expect(last.to).toBeLessThanOrEqual(view.state.doc.length);
    view.destroy();
  });
});
