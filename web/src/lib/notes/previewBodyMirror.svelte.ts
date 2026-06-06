// Throttled-mirror controller for the preview pane's body source.
//
// The notes page edits a single `body` $state that CodeMirror writes
// at microtask speed. The MarkdownRenderer pipeline (marked.parse +
// synchronous DOMPurify + postprocess + {@html} swap) is the single
// largest synchronous block on the main thread, and it scales
// linearly with body size — DOMPurify alone is ~80–250 ms on a 100
// KB body, 500–800 ms at 500 KB. Without a throttle, every keystroke
// would re-parse the full document 60–200×/s on fast typing for a
// 600-line note.
//
// This controller exposes `bodyForPreview` — the rAF-throttled (small
// notes) or timer-debounced (large notes) mirror of `body` — and the
// page binds preview surfaces to it instead of the raw `body`. Save
// logic, dirty tracking, and slash-command parsing still read the
// unthrottled `body` because they need the latest text. The rAF
// callback reads body at flush time (not at effect-run time) so it
// always commits the latest text even when 5+ keystrokes fall inside
// one 16ms frame.
//
// Adaptive tiers picked to balance "feels fresh" against "stops
// freezing on pause":
//   • < 32 KB        — rAF (≤16 ms): typing-rate updates feel live.
//   • 32 KB – 128 KB — 400 ms idle pause before reparse.
//   • 128 KB – 512 KB — 800 ms idle pause.
//   • ≥ 512 KB       — 1500 ms idle pause; DOMPurify alone may take
//     ≥500 ms here, so a shorter window queues a backlog the main
//     thread can't drain between keystrokes.

const PREVIEW_TIER_1 = 32 * 1024;
const PREVIEW_TIER_2 = 128 * 1024;
const PREVIEW_TIER_3 = 512 * 1024;

function previewDebounceFor(bytes: number): number {
  if (bytes >= PREVIEW_TIER_3) return 1500;
  if (bytes >= PREVIEW_TIER_2) return 800;
  if (bytes >= PREVIEW_TIER_1) return 400;
  return 0; // signals "use rAF path"
}

export interface PreviewBodyMirror {
  readonly bodyForPreview: string;
  /** Wire into a $effect that tracks `body`. Returns nothing — the
   *  rAF / timer state is owned by the controller. */
  schedule: (body: string) => void;
  /** Tear down pending rAF + timer — call from the page's onDestroy
   *  so a stale frame can't fire after unmount. */
  destroy: () => void;
}

export function createPreviewBodyMirror(): PreviewBodyMirror {
  let bodyForPreview = $state('');
  let raf = 0;
  let timer: ReturnType<typeof setTimeout> | null = null;
  // Captured at schedule-time so the deferred fire reads the latest
  // body, not whatever it was when the rAF was queued. Refs the same
  // string as the page's `body` $state.
  let pending = '';

  function schedule(body: string) {
    pending = body;
    // First-paint fast path: when bodyForPreview is still empty but
    // body has loaded, sync synchronously instead of waiting for the
    // next rAF. Without this, opening a note flashes an empty preview
    // for ~16ms while the throttle's first frame is pending — visible
    // as a brief blank on every load and tab switch. After init,
    // bodyForPreview tracks body via the rAF path, so this branch
    // fires at most once per mount + once per explicit clear-then-
    // type cycle.
    if (bodyForPreview === '' && body !== '') {
      bodyForPreview = body;
      return;
    }
    const debounceMs = previewDebounceFor(body.length);
    if (debounceMs > 0) {
      if (raf) {
        cancelAnimationFrame(raf);
        raf = 0;
      }
      if (timer) clearTimeout(timer);
      timer = setTimeout(() => {
        timer = null;
        bodyForPreview = pending;
      }, debounceMs);
      return;
    }
    if (timer) {
      clearTimeout(timer);
      timer = null;
    }
    if (raf) return;
    raf = requestAnimationFrame(() => {
      raf = 0;
      bodyForPreview = pending;
    });
  }

  function destroy() {
    if (raf) {
      cancelAnimationFrame(raf);
      raf = 0;
    }
    if (timer) {
      clearTimeout(timer);
      timer = null;
    }
  }

  return {
    get bodyForPreview() {
      return bodyForPreview;
    },
    schedule,
    destroy
  };
}
