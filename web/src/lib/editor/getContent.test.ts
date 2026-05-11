import { describe, expect, it } from 'vitest';
import { EditorState } from '@codemirror/state';
import { EditorView } from '@codemirror/view';

// The notes editor freeze + data-loss fix hinges on one invariant:
// CodeMirror's view.state.doc is updated *synchronously* inside
// view.dispatch(), with no async/microtask queue in front of it. The
// Editor component's getContent() wraps `view.state.doc.toString()` —
// if that read could ever lag a dispatch, the entire fix collapses
// (the in-flight-edit guards in /notes/[...path]/+page.svelte would
// false-negative the same way the previous stale-mirror code did).
//
// These tests pin the invariant to CodeMirror itself so any future
// upgrade that broke the synchronous-dispatch contract would fail
// here loudly instead of resurfacing as "everything freezes after
// autosave and my note got truncated" on the user's machine.

function mountHeadless(initial = ''): EditorView {
	// jsdom provides document so a detached EditorView works fine
	// without attaching the contentDOM anywhere — we only ever read
	// state.doc, never paint.
	const state = EditorState.create({ doc: initial });
	return new EditorView({ state, parent: document.createElement('div') });
}

describe('Editor getContent() invariant', () => {
	it('view.state.doc reflects the dispatch synchronously', () => {
		// The "bug" the fix prevents: parent reads body, sees stale
		// content, and concludes "no in-flight edits". CodeMirror
		// itself must not exhibit any such lag.
		const view = mountHeadless('hello');
		expect(view.state.doc.toString()).toBe('hello');
		view.dispatch({ changes: { from: 5, insert: ' world' } });
		// Synchronous — no await, no flush — and the doc must already
		// show the new content. If this ever returns 'hello', the fix
		// is no longer load-bearing.
		expect(view.state.doc.toString()).toBe('hello world');
		view.destroy();
	});

	it('rapid sequential dispatches all reflect immediately', () => {
		// User-typing pattern: many small inserts in quick succession.
		// Each must be visible synchronously to subsequent reads so
		// the post-save dirty check + WS-reload guard read the truth
		// at the exact moment they fire.
		const view = mountHeadless('');
		const keystrokes = 'The quick brown fox jumps over the lazy dog.';
		for (let i = 0; i < keystrokes.length; i++) {
			view.dispatch({ changes: { from: view.state.doc.length, insert: keystrokes[i] } });
			// Read immediately after each dispatch — must equal the
			// running accumulator at every step.
			expect(view.state.doc.toString()).toBe(keystrokes.slice(0, i + 1));
		}
		view.destroy();
	});

	it('dispatch followed by an immediate read survives a queued microtask', async () => {
		// Worst-case timing: the user types, the updateListener queues
		// a microtask (internalChange flag reset), and the parent's
		// reactive cascade queues other microtasks in front. The doc
		// read in our guards must return the NEW content even before
		// any of those microtasks run.
		const view = mountHeadless('start');
		// Pin a microtask first — simulates the parent's bind:value
		// write being queued ahead of our guard read.
		let microtaskRan = false;
		queueMicrotask(() => {
			microtaskRan = true;
		});
		// Now dispatch and read in the same synchronous tick.
		view.dispatch({ changes: { from: 5, insert: '+APPEND' } });
		// Guard read — happens BEFORE the microtask queue flushes.
		const synchronousRead = view.state.doc.toString();
		expect(microtaskRan).toBe(false); // proves we're pre-flush
		expect(synchronousRead).toBe('start+APPEND');
		// Drain microtasks and confirm content didn't change.
		await Promise.resolve();
		expect(microtaskRan).toBe(true);
		expect(view.state.doc.toString()).toBe('start+APPEND');
		view.destroy();
	});

	it('toString materialises the full doc on every call (large doc safety)', () => {
		// The fix calls view.state.doc.toString() in the hot path
		// (every save, every WS event, every localStorage draft
		// write). A regression that returned a stale cached string
		// after a dispatch — e.g. some future internal optimisation
		// with a stale length check — would re-introduce the data-
		// loss bug for long notes.
		const view = mountHeadless('a'.repeat(5000));
		expect(view.state.doc.toString().length).toBe(5000);
		view.dispatch({ changes: { from: 5000, insert: 'b'.repeat(3000) } });
		const after = view.state.doc.toString();
		expect(after.length).toBe(8000);
		// Last 3000 chars must be the new content, byte-for-byte.
		expect(after.slice(5000)).toBe('b'.repeat(3000));
		view.destroy();
	});
});
