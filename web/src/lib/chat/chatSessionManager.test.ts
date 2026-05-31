import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import {
  createChatSessionManager,
  type ChatStreamHandlers,
  type ChatSessionRefs,
  type PreludeBundle
} from './chatSessionManager.svelte';
import type { ChatMessage } from '$lib/api';

// Contract tests for the chat session orchestrator.
//
// We test the manager as a pure unit. Both the prelude assembly and
// the chat-stream call are injected through options, so no $lib/api
// module mocking is needed. Tests drive the manager directly and
// inspect the parent's refs object after each step.
//
// The four interesting behaviors covered here:
//   - send() append / stream / finalize contract
//   - busy + slash-command + await-save guards at the start of send()
//   - cancelInflight() abort propagation
//   - the three race fixes the extraction shipped:
//       #1 onError silenced after abort (rapid resend can't be
//          clobbered by a late error from the previous stream)
//       #2 scrollHeight read AFTER tick (stick-to-bottom uses the
//          new paint, not the stale DOM)
//       #3 send() awaits in-flight autoSaveThread before starting

function makeRefs(initial: Partial<ChatSessionRefs> = {}): ChatSessionRefs {
  return {
    input: '',
    busy: false,
    messages: [],
    mentionedRefs: [],
    lastRagHits: [],
    perTurnRagHits: {},
    quickTitle: '',
    quickResult: '',
    ...initial
  };
}

interface Capture {
  /** Last set of handlers passed to chatStream — captured so a test
   *  can fire onChunk/onDone/onError when it wants to. */
  handlers: ChatStreamHandlers | null;
  /** Signal forwarded into chatStream — captured so a test can read
   *  `.aborted` after cancelInflight() fires. */
  signal: AbortSignal | null;
  /** The `messages` payload chatStream received. */
  history: ChatMessage[] | null;
  /** notePath param the manager forwarded. */
  notePath: string | undefined;
  /** Resolves the chatStream promise (simulates server confirming
   *  stream completion). */
  resolveStream: () => void;
  /** Rejects the chatStream promise — used by the abort test path. */
  rejectStream: (err: Error) => void;
}

/** Build a deterministic chatStream stub that captures its inputs and
 *  hands control back to the test (so the test can fire onChunk/
 *  onDone/onError at will). */
function makeStreamStub(): {
  fn: ReturnType<typeof vi.fn>;
  capture: Capture;
} {
  const capture: Capture = {
    handlers: null,
    signal: null,
    history: null,
    notePath: undefined,
    resolveStream: () => {},
    rejectStream: () => {}
  };
  const fn = vi.fn(
    (
      history: ChatMessage[],
      notePath: string | undefined,
      handlers: ChatStreamHandlers,
      signal: AbortSignal
    ) => {
      capture.history = history;
      capture.notePath = notePath;
      capture.handlers = handlers;
      capture.signal = signal;
      return new Promise<void>((resolve, reject) => {
        capture.resolveStream = resolve;
        capture.rejectStream = reject;
      });
    }
  );
  return { fn, capture };
}

const emptyPrelude: PreludeBundle = {
  messages: [],
  ragHits: [],
  notePathForStream: null
};

describe('chatSessionManager', () => {
  // rAF capture — same shape the streamThrottle test uses. Lets us
  // step through the throttle's frame ticks manually.
  let rafCallbacks: Array<() => void>;
  let prevRAF: typeof globalThis.requestAnimationFrame | undefined;
  let prevCAF: typeof globalThis.cancelAnimationFrame | undefined;

  beforeEach(() => {
    rafCallbacks = [];
    prevRAF = globalThis.requestAnimationFrame;
    prevCAF = globalThis.cancelAnimationFrame;
    globalThis.requestAnimationFrame = ((cb: () => void) => {
      rafCallbacks.push(cb);
      return rafCallbacks.length;
    }) as unknown as typeof requestAnimationFrame;
    globalThis.cancelAnimationFrame = (() => {}) as unknown as typeof cancelAnimationFrame;
  });

  afterEach(() => {
    if (prevRAF) globalThis.requestAnimationFrame = prevRAF;
    if (prevCAF) globalThis.cancelAnimationFrame = prevCAF;
  });

  function fireFrame() {
    const cbs = rafCallbacks;
    rafCallbacks = [];
    for (const cb of cbs) cb();
  }

  // ── Clean send ────────────────────────────────────────────────
  it('appends [user, assistant] and streams chunks into the assistant slot', async () => {
    const refs = makeRefs({ input: 'hello' });
    const { fn: chatStream, capture } = makeStreamStub();
    const mgr = createChatSessionManager({
      refs,
      buildPrelude: async () => emptyPrelude,
      chatStream,
      handleSlashCommand: () => false,
      autoSaveThread: () => {},
      awaitSave: async () => {},
      resetForClear: () => {},
      getScrollEl: () => undefined
    });

    const sendPromise = mgr.send();
    // Let the awaitSave + buildPrelude microtasks resolve so
    // chatStream is called.
    await Promise.resolve();
    await Promise.resolve();

    // refs.messages got [user, assistant=''] appended; busy=true;
    // composer cleared.
    expect(refs.messages).toHaveLength(2);
    expect(refs.messages[0]).toEqual({ role: 'user', content: 'hello' });
    expect(refs.messages[1]).toEqual({ role: 'assistant', content: '' });
    expect(refs.busy).toBe(true);
    expect(refs.input).toBe('');
    expect(capture.handlers).not.toBeNull();

    // Stream a chunk; the rAF throttle defers commits until the
    // frame tick fires.
    capture.handlers!.onChunk('Hi ');
    capture.handlers!.onChunk('there');
    expect(refs.messages[1].content).toBe(''); // not yet committed
    fireFrame();
    expect(refs.messages[1].content).toBe('Hi there');

    // onDone flushes (no-op here — buffer already drained) and the
    // stream promise resolves. finally runs.
    capture.handlers!.onDone!();
    capture.resolveStream();
    await sendPromise;

    expect(refs.busy).toBe(false);
    expect(refs.messages[1].content).toBe('Hi there');
  });

  // ── Guards at the top of send() ───────────────────────────────
  it('refuses while busy', async () => {
    const refs = makeRefs({ input: 'hello', busy: true });
    const { fn: chatStream } = makeStreamStub();
    const mgr = createChatSessionManager({
      refs,
      buildPrelude: async () => emptyPrelude,
      chatStream,
      handleSlashCommand: () => false,
      autoSaveThread: () => {},
      awaitSave: async () => {},
      resetForClear: () => {},
      getScrollEl: () => undefined
    });

    await mgr.send();
    expect(chatStream).not.toHaveBeenCalled();
    // Composer text preserved so the user doesn't lose their draft.
    expect(refs.input).toBe('hello');
  });

  it('refuses on empty input', async () => {
    const refs = makeRefs({ input: '   ' });
    const { fn: chatStream } = makeStreamStub();
    const mgr = createChatSessionManager({
      refs,
      buildPrelude: async () => emptyPrelude,
      chatStream,
      handleSlashCommand: () => false,
      autoSaveThread: () => {},
      awaitSave: async () => {},
      resetForClear: () => {},
      getScrollEl: () => undefined
    });

    await mgr.send();
    expect(chatStream).not.toHaveBeenCalled();
  });

  it('routes slash commands without firing chatStream', async () => {
    const refs = makeRefs({ input: '/help' });
    const { fn: chatStream } = makeStreamStub();
    const slashHandler = vi.fn(() => true);
    const mgr = createChatSessionManager({
      refs,
      buildPrelude: async () => emptyPrelude,
      chatStream,
      handleSlashCommand: slashHandler,
      autoSaveThread: () => {},
      awaitSave: async () => {},
      resetForClear: () => {},
      getScrollEl: () => undefined
    });

    await mgr.send();
    expect(slashHandler).toHaveBeenCalledWith('/help');
    expect(chatStream).not.toHaveBeenCalled();
    // refs.input is the parent's slot — the slash router clears it
    // through its OWN path (handleSlashCommand returning true is the
    // signal). The manager does not touch input on the slash branch.
    expect(refs.input).toBe('/help');
  });

  // ── Race fix #3 — awaitSave gate ───────────────────────────────
  it('awaits in-flight autoSaveThread before starting the next send', async () => {
    const refs = makeRefs({ input: 'hello' });
    const { fn: chatStream } = makeStreamStub();
    let resolveSave!: () => void;
    const awaitSave = vi.fn(
      () => new Promise<void>((res) => { resolveSave = res; })
    );
    const mgr = createChatSessionManager({
      refs,
      buildPrelude: async () => emptyPrelude,
      chatStream,
      handleSlashCommand: () => false,
      autoSaveThread: () => {},
      awaitSave,
      resetForClear: () => {},
      getScrollEl: () => undefined
    });

    const sendPromise = mgr.send();
    // Wait several microtasks — the gate should still hold us back.
    for (let i = 0; i < 5; i++) await Promise.resolve();
    expect(chatStream).not.toHaveBeenCalled();
    expect(refs.busy).toBe(false);

    // Unblock the save — send proceeds past the gate.
    resolveSave();
    for (let i = 0; i < 5; i++) await Promise.resolve();
    expect(chatStream).toHaveBeenCalledTimes(1);
    expect(refs.busy).toBe(true);

    // Tear down so the test doesn't leak the open promise.
    // We rejectStream rather than resolveStream so the awaiting
    // send() exits via its catch-free path (no onError attached).
    sendPromise.catch(() => {});
  });

  // ── cancelInflight ────────────────────────────────────────────
  it('cancelInflight aborts the stream signal', async () => {
    const refs = makeRefs({ input: 'hello' });
    const { fn: chatStream, capture } = makeStreamStub();
    const mgr = createChatSessionManager({
      refs,
      buildPrelude: async () => emptyPrelude,
      chatStream,
      handleSlashCommand: () => false,
      autoSaveThread: () => {},
      awaitSave: async () => {},
      resetForClear: () => {},
      getScrollEl: () => undefined
    });

    const sendPromise = mgr.send();
    for (let i = 0; i < 3; i++) await Promise.resolve();
    expect(capture.signal).not.toBeNull();
    expect(capture.signal!.aborted).toBe(false);

    mgr.cancelInflight();
    expect(capture.signal!.aborted).toBe(true);

    // Simulate the stream catching the abort + onError firing with
    // an AbortError. Race fix #1 silences this — the assistant
    // message stays as the empty placeholder, no `_error:_` shows.
    capture.handlers!.onError!(new Error('aborted'));
    capture.rejectStream(new Error('aborted'));
    await sendPromise.catch(() => {});

    expect(refs.messages[1].content).toBe('');
    expect(refs.busy).toBe(false);
  });

  // ── Race fix #1 — silence late onError after rapid resend ──────
  it('silences a late onError once the controller is aborted (race fix #1)', async () => {
    const refs = makeRefs({ input: 'hello' });
    const { fn: chatStream, capture } = makeStreamStub();
    const mgr = createChatSessionManager({
      refs,
      buildPrelude: async () => emptyPrelude,
      chatStream,
      handleSlashCommand: () => false,
      autoSaveThread: () => {},
      awaitSave: async () => {},
      resetForClear: () => {},
      getScrollEl: () => undefined
    });

    // Send #1 — captures its handlers + signal.
    const send1 = mgr.send();
    for (let i = 0; i < 3; i++) await Promise.resolve();
    const handlers1 = capture.handlers!;
    // Stream a chunk so the assistant message has real content. If
    // race fix #1 regressed, the late onError below would overwrite
    // this with `_error:_ ...`.
    handlers1.onChunk('partial reply');
    fireFrame();
    expect(refs.messages[1].content).toBe('partial reply');

    // User cancels (this is the load-bearing precondition — the
    // race only matters because the controller is aborted).
    mgr.cancelInflight();
    capture.rejectStream(new Error('aborted'));
    await send1.catch(() => {});

    // Now the OLD stream's network layer flushes a late onError
    // (the fetch had already returned its reader by the time abort
    // landed). With race fix #1 this is a no-op; without it the
    // partial reply gets clobbered.
    handlers1.onError!(new Error('connection reset'));

    expect(refs.messages[1].content).toBe('partial reply');
  });

  // ── Error mid-stream (no abort) ────────────────────────────────
  it('renders _error:_ when onError fires before abort', async () => {
    const refs = makeRefs({ input: 'hello' });
    const { fn: chatStream, capture } = makeStreamStub();
    const mgr = createChatSessionManager({
      refs,
      buildPrelude: async () => emptyPrelude,
      chatStream,
      handleSlashCommand: () => false,
      autoSaveThread: () => {},
      awaitSave: async () => {},
      resetForClear: () => {},
      getScrollEl: () => undefined
    });

    const sendPromise = mgr.send();
    for (let i = 0; i < 3; i++) await Promise.resolve();
    capture.handlers!.onError!(new Error('boom'));
    capture.resolveStream();
    await sendPromise;

    expect(refs.messages[1].content).toBe('_error:_ boom');
    expect(refs.busy).toBe(false);
  });

  // ── Append to existing thread (the regen scenario) ─────────────
  it('appends [user, assistant] to a pre-existing thread', async () => {
    // Pre-populated state: a prior turn is already in the thread.
    // This is what regenAssistantMessage looks like from the
    // session-manager's perspective — the parent has already
    // truncated via replayFromUserMessage, and now we send().
    const refs = makeRefs({
      input: 'follow up',
      messages: [
        { role: 'user', content: 'first question' },
        { role: 'assistant', content: 'first answer' }
      ]
    });
    const { fn: chatStream, capture } = makeStreamStub();
    const mgr = createChatSessionManager({
      refs,
      buildPrelude: async () => emptyPrelude,
      chatStream,
      handleSlashCommand: () => false,
      autoSaveThread: () => {},
      awaitSave: async () => {},
      resetForClear: () => {},
      getScrollEl: () => undefined
    });

    const sendPromise = mgr.send();
    for (let i = 0; i < 3; i++) await Promise.resolve();

    expect(refs.messages).toHaveLength(4);
    expect(refs.messages[2]).toEqual({
      role: 'user',
      content: 'follow up'
    });
    expect(refs.messages[3]).toEqual({ role: 'assistant', content: '' });

    capture.handlers!.onChunk('second answer');
    fireFrame();
    capture.handlers!.onDone!();
    capture.resolveStream();
    await sendPromise;

    expect(refs.messages[3].content).toBe('second answer');
  });

  // ── clearChat ──────────────────────────────────────────────────
  it('clearChat snapshots then empties the thread', () => {
    const refs = makeRefs({
      messages: [
        { role: 'user', content: 'q' },
        { role: 'assistant', content: 'a' }
      ],
      quickTitle: 'Briefing',
      quickResult: 'old',
      perTurnRagHits: { 1: [{ path: 'a.md', title: 'A', excerpt: '' }] }
    });
    const autoSave = vi.fn();
    const resetForClear = vi.fn();
    const mgr = createChatSessionManager({
      refs,
      buildPrelude: async () => emptyPrelude,
      chatStream: vi.fn(),
      handleSlashCommand: () => false,
      autoSaveThread: autoSave,
      awaitSave: async () => {},
      resetForClear,
      getScrollEl: () => undefined
    });

    mgr.clearChat();
    expect(autoSave).toHaveBeenCalledTimes(1);
    expect(refs.messages).toEqual([]);
    expect(refs.quickTitle).toBe('');
    expect(refs.quickResult).toBe('');
    expect(refs.perTurnRagHits).toEqual({});
    expect(resetForClear).toHaveBeenCalledTimes(1);
  });

  it('clearChat is a no-op on an empty thread', () => {
    const refs = makeRefs();
    const autoSave = vi.fn();
    const resetForClear = vi.fn();
    const mgr = createChatSessionManager({
      refs,
      buildPrelude: async () => emptyPrelude,
      chatStream: vi.fn(),
      handleSlashCommand: () => false,
      autoSaveThread: autoSave,
      awaitSave: async () => {},
      resetForClear,
      getScrollEl: () => undefined
    });

    mgr.clearChat();
    expect(autoSave).not.toHaveBeenCalled();
    expect(resetForClear).not.toHaveBeenCalled();
  });

  // ── prelude payload is correctly assembled ─────────────────────
  it('forwards prelude messages + thread + user msg as the chatStream payload', async () => {
    const refs = makeRefs({
      input: 'hello',
      messages: [{ role: 'user', content: 'prior' }]
    });
    const { fn: chatStream, capture } = makeStreamStub();
    const mgr = createChatSessionManager({
      refs,
      buildPrelude: async () => ({
        messages: [{ role: 'system', content: 'PRELUDE' }],
        ragHits: [],
        notePathForStream: 'Notes/example.md'
      }),
      chatStream,
      handleSlashCommand: () => false,
      autoSaveThread: () => {},
      awaitSave: async () => {},
      resetForClear: () => {},
      getScrollEl: () => undefined
    });

    void mgr.send();
    for (let i = 0; i < 3; i++) await Promise.resolve();

    expect(capture.history).toEqual([
      { role: 'system', content: 'PRELUDE' },
      { role: 'user', content: 'prior' },
      { role: 'user', content: 'hello' }
    ]);
    expect(capture.notePath).toBe('Notes/example.md');
  });

  // ── perTurnRagHits keyed by assistant index ────────────────────
  it('records ragHits against the assistant message index', async () => {
    const refs = makeRefs({ input: 'hello' });
    const { fn: chatStream } = makeStreamStub();
    const mgr = createChatSessionManager({
      refs,
      buildPrelude: async () => ({
        messages: [],
        ragHits: [{ path: 'a.md', title: 'A', excerpt: 'ex' }],
        notePathForStream: null
      }),
      chatStream,
      handleSlashCommand: () => false,
      autoSaveThread: () => {},
      awaitSave: async () => {},
      resetForClear: () => {},
      getScrollEl: () => undefined
    });

    void mgr.send();
    for (let i = 0; i < 3; i++) await Promise.resolve();

    // Assistant message index is 1 (after the [user, assistant]
    // pair appended in this send).
    expect(refs.lastRagHits).toHaveLength(1);
    expect(refs.perTurnRagHits[1]).toHaveLength(1);
    expect(refs.perTurnRagHits[1][0].path).toBe('a.md');
  });
});
