<!--
  TasksSwipeHint — the small "‹ swipe left to snooze · swipe right for
  done ›" banner shown ONCE to first-time touch-device users above the
  task list. Tells them gestures exist without forcing them to
  discover the behaviour through trial-and-error.

  Self-contained: owns its dismissal flag (localStorage), the touch-
  device probe, the 8-second auto-dismiss, and the visibility gate.
  The parent only decides whether the surrounding view is one where
  the hint applies (list view with at least one card visible) and
  passes that as `applicable`.

  Visibility = touch device AND not dismissed AND applicable. Either
  a tap on the banner or 8 seconds without interaction permanently
  dismisses it via localStorage so it never reappears.
-->
<script lang="ts">
  import { onMount } from 'svelte';

  type Props = {
    /** True when the surrounding view is one where swipe gestures are
     *  available — the parent passes `viewCtl.view === 'list' &&
     *  filterCtl.filtered.length > 0`. */
    applicable: boolean;
  };

  let { applicable }: Props = $props();

  const SWIPE_HINT_KEY = 'granit.tasks.swipe-hint-dismissed';

  let dismissed = $state(
    typeof window !== 'undefined' && window.localStorage.getItem(SWIPE_HINT_KEY) === '1'
  );
  // Only show on touch devices. The matchMedia probe is best-effort —
  // if the API isn't available (very old browser) we err on the side
  // of NOT showing the hint, which is the conservative path.
  let isTouchDevice = $state(false);

  onMount(() => {
    try {
      isTouchDevice = window.matchMedia('(hover: none) and (pointer: coarse)').matches;
    } catch {
      isTouchDevice = false;
    }
    // Auto-dismiss after 8 seconds — the hint is meant as a one-time
    // nudge, not a permanent fixture. The localStorage write inside
    // dismiss() makes the dismissal stick across the next refresh.
    if (!dismissed && isTouchDevice) {
      const handle = setTimeout(() => dismiss(), 8000);
      return () => clearTimeout(handle);
    }
  });

  function dismiss() {
    dismissed = true;
    try { window.localStorage.setItem(SWIPE_HINT_KEY, '1'); } catch {}
  }

  let visible = $derived(isTouchDevice && !dismissed && applicable);
</script>

{#if visible}
  <button
    type="button"
    onclick={dismiss}
    class="w-full max-w-3xl text-center text-[11px] text-dim bg-surface0 border border-surface1 rounded py-2 px-3 flex items-center justify-center gap-2 active:bg-surface1 mb-3"
    aria-label="Dismiss swipe hint"
  >
    <span class="text-warning" aria-hidden="true">‹</span>
    <span>swipe left to snooze</span>
    <span class="text-dim">·</span>
    <span>swipe right for done</span>
    <span class="text-success" aria-hidden="true">›</span>
    <span class="text-dim ml-1">(tap to dismiss)</span>
  </button>
{/if}
