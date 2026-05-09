import { describe, expect, it, beforeEach, afterEach, vi } from 'vitest';
import { createCoalescedReload } from './coalesce';

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe('createCoalescedReload', () => {
  it('does not run on a single trigger before the window expires', () => {
    const work = vi.fn();
    const reload = createCoalescedReload(work, 600);
    reload.trigger();
    vi.advanceTimersByTime(599);
    expect(work).not.toHaveBeenCalled();
  });

  it('runs once at the trailing edge of the window', () => {
    const work = vi.fn();
    const reload = createCoalescedReload(work, 600);
    reload.trigger();
    vi.advanceTimersByTime(600);
    expect(work).toHaveBeenCalledTimes(1);
  });

  it('coalesces a burst of triggers into a single trailing call', () => {
    const work = vi.fn();
    const reload = createCoalescedReload(work, 600);
    // Fire 50 triggers spaced 10ms apart over 500ms.
    for (let i = 0; i < 50; i++) {
      reload.trigger();
      vi.advanceTimersByTime(10);
    }
    // Still inside the original 600ms window.
    expect(work).not.toHaveBeenCalled();
    // Past the window — the trailing call fires once.
    vi.advanceTimersByTime(150);
    expect(work).toHaveBeenCalledTimes(1);
  });

  it('a fresh trigger after work has run arms a new window', () => {
    const work = vi.fn();
    const reload = createCoalescedReload(work, 600);
    reload.trigger();
    vi.advanceTimersByTime(600);
    expect(work).toHaveBeenCalledTimes(1);
    // New trigger AFTER the first work fired.
    reload.trigger();
    vi.advanceTimersByTime(600);
    expect(work).toHaveBeenCalledTimes(2);
  });

  it('cancel() prevents the pending trailing call from firing', () => {
    const work = vi.fn();
    const reload = createCoalescedReload(work, 600);
    reload.trigger();
    vi.advanceTimersByTime(300);
    reload.cancel();
    vi.advanceTimersByTime(1000);
    expect(work).not.toHaveBeenCalled();
  });

  it('flush() runs work immediately and bypasses the window', () => {
    const work = vi.fn();
    const reload = createCoalescedReload(work, 600);
    reload.trigger();
    vi.advanceTimersByTime(100);
    reload.flush();
    expect(work).toHaveBeenCalledTimes(1);
    // Subsequent timer expiration shouldn't double-fire.
    vi.advanceTimersByTime(2000);
    expect(work).toHaveBeenCalledTimes(1);
  });

  it('honours a custom window length', () => {
    const work = vi.fn();
    const reload = createCoalescedReload(work, 100);
    reload.trigger();
    vi.advanceTimersByTime(99);
    expect(work).not.toHaveBeenCalled();
    vi.advanceTimersByTime(2);
    expect(work).toHaveBeenCalledTimes(1);
  });

  it('async work is fire-and-forget — next trigger arms regardless', () => {
    const work = vi.fn(async () => {
      await Promise.resolve();
    });
    const reload = createCoalescedReload(work, 100);
    reload.trigger();
    vi.advanceTimersByTime(100);
    expect(work).toHaveBeenCalledTimes(1);
    reload.trigger();
    vi.advanceTimersByTime(100);
    expect(work).toHaveBeenCalledTimes(2);
  });
});
