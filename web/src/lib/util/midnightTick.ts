// Local-midnight tick helper for dashboard widgets that filter by
// "today". Without this, widgets that capture `todayISO()` at script
// init keep showing yesterday's day until the user reloads, which is
// the most common dashboard "stale tile" complaint at 00:01.
//
// Usage:
//   onMount(() => {
//     const stop = onLocalMidnight(() => { load(); });
//     return () => stop();
//   });
//
// The handler is invoked at the next local 00:00 (plus a 5 s pad to
// absorb microsecond clock-drift edge cases where the timer fires
// just before the day rolls). After firing, the helper re-arms for
// the following midnight automatically, so a long-lived dashboard
// keeps tracking the date roll for as long as the component is
// mounted. Calling the returned function cancels any pending timer.

export function onLocalMidnight(handler: () => void): () => void {
  let timer: ReturnType<typeof setTimeout> | null = null;
  let cancelled = false;

  function arm() {
    if (cancelled) return;
    const now = new Date();
    const next = new Date(now);
    next.setHours(24, 0, 5, 0);
    timer = setTimeout(() => {
      if (cancelled) return;
      try {
        handler();
      } finally {
        arm();
      }
    }, next.getTime() - now.getTime());
  }
  arm();

  return () => {
    cancelled = true;
    if (timer) {
      clearTimeout(timer);
      timer = null;
    }
  };
}
