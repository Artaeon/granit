// rAF coalescer for AI chatStream handlers.
//
// Problem: every onChunk callback typically writes to one or more
// reactive state slots, which fires a fresh render of the host
// component. For fast models that emit 100+ tokens/sec, this means
// 100+ re-renders/sec. Combined with derived chains (chars / words /
// lines / preview / list-rebuild) the main thread chokes and the
// app feels frozen even though no single operation is slow.
//
// Solution: buffer chunks between requestAnimationFrame ticks.
// `acc` grows on every call; we only flush to the state writer
// once per frame. Visible streaming is unchanged to the human eye
// (>60 Hz is invisible) but CPU drops by an order of magnitude.
//
// Usage:
//   const t = rafThrottle((full) => {
//     response = full;            // OR rebuild derived state from `full`
//   });
//   onChunk: t.onChunk,
//   onDone: () => { t.flush(); ...done logic... },
//   onError: () => { t.flush(); ...error logic... },
//
// flush() is synchronous so caller can be sure the final state
// reflects every chunk before transitioning out of `pending`.

export interface ChunkThrottle {
  /** Append a streamed chunk; schedules a flush for the next frame. */
  onChunk: (chunk: string) => void;
  /** Force a synchronous flush of any pending buffer. Idempotent. */
  flush: () => void;
  /** Read the current accumulated buffer (does NOT flush). Useful
   *  inside onDone for `JSON.parse(t.value())` style consumers. */
  value: () => string;
}

export function rafThrottle(apply: (accumulated: string) => void): ChunkThrottle {
  let acc = '';
  let dirty = false;
  let frame = 0;
  // SSR safety: outside the browser we still need a callable shape so
  // unit tests / server prerender don't blow up. Fall back to
  // microtask scheduling so behavior is "flush eventually" instead
  // of "never".
  const raf: (cb: () => void) => number =
    typeof requestAnimationFrame !== 'undefined'
      ? (cb) => requestAnimationFrame(cb)
      : (cb) => {
          queueMicrotask(cb);
          return 1;
        };
  const caf: (id: number) => void =
    typeof cancelAnimationFrame !== 'undefined' ? cancelAnimationFrame : () => {};

  const paint = () => {
    frame = 0;
    if (!dirty) return;
    dirty = false;
    apply(acc);
  };

  return {
    onChunk(chunk: string) {
      acc += chunk;
      dirty = true;
      if (frame === 0) frame = raf(paint);
    },
    flush() {
      if (frame !== 0) {
        caf(frame);
        frame = 0;
      }
      if (dirty) {
        dirty = false;
        apply(acc);
      }
    },
    value() {
      return acc;
    }
  };
}
