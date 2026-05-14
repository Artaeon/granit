import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { rafThrottle } from './streamThrottle';

// Contract guard for the rAF throttle helper. The thing the rest of
// the app depends on is:
//   1. apply() is NOT called synchronously inside onChunk — so a
//      300-chunk burst can't trigger 300 reactive state writes.
//   2. apply() IS called once per animation frame with the cumulative
//      buffer (not per-chunk deltas).
//   3. flush() runs apply synchronously with the final buffer + clears
//      any pending frame so the caller can transition pending → done
//      without losing the last chunks.
//   4. flush() on an empty / already-flushed buffer is a no-op.

describe('rafThrottle', () => {
  let rafCallbacks: Array<() => void>;
  let rafSpy: ReturnType<typeof vi.fn>;
  let cafSpy: ReturnType<typeof vi.fn>;
  let prevRAF: typeof globalThis.requestAnimationFrame | undefined;
  let prevCAF: typeof globalThis.cancelAnimationFrame | undefined;

  beforeEach(() => {
    rafCallbacks = [];
    rafSpy = vi.fn((cb: () => void) => {
      rafCallbacks.push(cb);
      return rafCallbacks.length; // 1-based id
    });
    cafSpy = vi.fn();
    prevRAF = globalThis.requestAnimationFrame;
    prevCAF = globalThis.cancelAnimationFrame;
    globalThis.requestAnimationFrame = rafSpy as unknown as typeof requestAnimationFrame;
    globalThis.cancelAnimationFrame = cafSpy as unknown as typeof cancelAnimationFrame;
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

  it('does NOT call apply synchronously on onChunk', () => {
    const apply = vi.fn();
    const t = rafThrottle(apply);
    t.onChunk('a');
    t.onChunk('b');
    t.onChunk('c');
    expect(apply).not.toHaveBeenCalled();
    // Exactly one frame scheduled even with three chunks — the
    // coalescing property.
    expect(rafSpy).toHaveBeenCalledTimes(1);
  });

  it('calls apply once per frame with the cumulative buffer', () => {
    const apply = vi.fn();
    const t = rafThrottle(apply);
    t.onChunk('he');
    t.onChunk('llo');
    fireFrame();
    expect(apply).toHaveBeenCalledTimes(1);
    expect(apply).toHaveBeenLastCalledWith('hello');

    t.onChunk(' world');
    fireFrame();
    expect(apply).toHaveBeenCalledTimes(2);
    expect(apply).toHaveBeenLastCalledWith('hello world');
  });

  it('flush runs apply synchronously with the full buffer', () => {
    const apply = vi.fn();
    const t = rafThrottle(apply);
    t.onChunk('part ');
    t.onChunk('two');
    t.flush();
    expect(apply).toHaveBeenCalledTimes(1);
    expect(apply).toHaveBeenLastCalledWith('part two');
    // The frame would also fire, but the throttle marks itself
    // clean after flush so the queued callback is a no-op when
    // it eventually runs.
    fireFrame();
    expect(apply).toHaveBeenCalledTimes(1);
  });

  it('flush cancels the pending frame to avoid a stale-state paint', () => {
    const apply = vi.fn();
    const t = rafThrottle(apply);
    t.onChunk('x');
    t.flush();
    expect(cafSpy).toHaveBeenCalledTimes(1);
  });

  it('flush is a no-op when there is nothing to apply', () => {
    const apply = vi.fn();
    const t = rafThrottle(apply);
    t.flush();
    t.flush();
    expect(apply).not.toHaveBeenCalled();
  });

  it('value() exposes the current buffer without flushing', () => {
    const apply = vi.fn();
    const t = rafThrottle(apply);
    t.onChunk('abc');
    expect(t.value()).toBe('abc');
    expect(apply).not.toHaveBeenCalled();
  });
});
