<script lang="ts">
  // Mobile AI FAB — Mod+J doesn't exist on a phone keyboard, and
  // the sidebar "Ask AI" button is hidden behind a drawer on mobile.
  // This sparkle button sits above the bottom nav (with iOS
  // safe-area padding) so the AI overlay is one tap away. Desktop
  // hides this since the sidebar pill covers it.

  import { openAIOverlay } from '$lib/stores/ai-overlay';
</script>

<button
  type="button"
  onclick={() => openAIOverlay()}
  aria-label="Ask AI"
  title="Ask AI"
  class="md:hidden fixed right-5 z-30 w-12 h-12 flex items-center justify-center rounded-full bg-primary text-on-primary shadow-lg hover:opacity-90 transition-all active:scale-95"
  style="bottom: calc(3.5rem + env(safe-area-inset-bottom, 0px) + 0.75rem);"
>
  <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
    <path d="M12 3v3M12 18v3M5.6 5.6l2.1 2.1M16.3 16.3l2.1 2.1M3 12h3M18 12h3M5.6 18.4l2.1-2.1M16.3 7.7l2.1-2.1"/>
    <circle cx="12" cy="12" r="3.5" fill="currentColor" fill-opacity="0.25"/>
  </svg>
</button>
