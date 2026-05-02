<script lang="ts">
  import { onMount } from 'svelte';

  // PWA install prompt. Two parallel flows because no single API works
  // everywhere:
  //   - Chromium (Android Chrome, desktop Chrome/Edge): captures the
  //     beforeinstallprompt event and exposes a button that calls
  //     prompt(). Single click → install dialog.
  //   - iOS Safari: never fires beforeinstallprompt. Shows a one-time
  //     "Tap [share] then Add to Home Screen" hint instead, dismissible.
  //
  // Both paths persist a "user dismissed" flag in localStorage so we
  // don't pester. Already-installed sessions (display-mode: standalone)
  // skip both flows entirely.

  // The captured event object — only Chromium populates it. Stored as
  // unknown because BeforeInstallPromptEvent isn't in lib.dom.
  type BIPEvent = Event & {
    prompt: () => Promise<void>;
    userChoice: Promise<{ outcome: 'accepted' | 'dismissed' }>;
  };
  let deferred = $state<BIPEvent | null>(null);

  let isIOS = $state(false);
  let dismissed = $state(false);
  // True once the page is launched as an installed PWA — no prompt
  // needed in that case.
  let installed = $state(false);

  const DISMISS_KEY = 'granit.install.dismissed';

  onMount(() => {
    try {
      dismissed = localStorage.getItem(DISMISS_KEY) === '1';
    } catch {
      dismissed = false;
    }

    // Display-mode standalone (or iOS' navigator.standalone) means we're
    // already installed — never show the prompt to a user who's already
    // done it.
    const standalone = matchMedia('(display-mode: standalone)').matches
      || (typeof navigator !== 'undefined' && (navigator as unknown as { standalone?: boolean }).standalone === true);
    if (standalone) {
      installed = true;
      return;
    }

    // Chromium-style install event. Capture and stash for the button.
    const onBip = (e: Event) => {
      e.preventDefault();
      deferred = e as BIPEvent;
    };
    window.addEventListener('beforeinstallprompt', onBip);

    // After the user accepts, the appinstalled fires — clear the
    // deferred so the button vanishes.
    const onInstalled = () => {
      deferred = null;
      installed = true;
    };
    window.addEventListener('appinstalled', onInstalled);

    // iOS detection: we want Mobile Safari specifically, not Chrome on
    // iOS (which isn't actually Safari but still doesn't expose the
    // install event because Apple). Both share the iOS user-agent and
    // both can install via Safari's share menu, so we treat them the
    // same.
    const ua = navigator.userAgent;
    isIOS = /iPad|iPhone|iPod/.test(ua) && !(window as unknown as { MSStream?: unknown }).MSStream;

    return () => {
      window.removeEventListener('beforeinstallprompt', onBip);
      window.removeEventListener('appinstalled', onInstalled);
    };
  });

  async function install() {
    if (!deferred) return;
    try {
      await deferred.prompt();
      const choice = await deferred.userChoice;
      // Either way, the deferred event can only be used once.
      deferred = null;
      if (choice.outcome === 'dismissed') dismiss();
    } catch {
      deferred = null;
    }
  }

  function dismiss() {
    dismissed = true;
    try { localStorage.setItem(DISMISS_KEY, '1'); } catch {}
  }

  // What the bottom-corner pill should render — Chromium prompt button,
  // iOS hint, or nothing.
  let mode = $derived.by<'chromium' | 'ios' | null>(() => {
    if (installed || dismissed) return null;
    if (deferred) return 'chromium';
    if (isIOS) return 'ios';
    return null;
  });
</script>

{#if mode === 'chromium'}
  <div class="fixed bottom-4 right-4 z-40 max-w-xs bg-mantle border border-surface1 rounded-lg shadow-xl p-3 flex items-start gap-3">
    <div class="w-8 h-8 rounded bg-primary/15 flex items-center justify-center flex-shrink-0">
      <svg viewBox="0 0 24 24" class="w-4 h-4 text-primary" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
        <path d="M12 4v12M6 10l6 6 6-6M4 20h16"/>
      </svg>
    </div>
    <div class="flex-1 min-w-0">
      <p class="text-sm text-text font-medium">Install Granit</p>
      <p class="text-[11px] text-dim mb-2">Faster launch · works offline · feels native.</p>
      <div class="flex gap-2">
        <button
          onclick={install}
          class="px-2.5 py-1 text-xs bg-primary text-mantle rounded font-medium"
        >Install</button>
        <button
          onclick={dismiss}
          class="px-2.5 py-1 text-xs text-dim hover:text-text"
        >Not now</button>
      </div>
    </div>
  </div>
{:else if mode === 'ios'}
  <div class="fixed bottom-4 left-1/2 -translate-x-1/2 z-40 max-w-xs bg-mantle border border-surface1 rounded-lg shadow-xl p-3 flex items-start gap-3">
    <div class="w-8 h-8 rounded bg-primary/15 flex items-center justify-center flex-shrink-0">
      <svg viewBox="0 0 24 24" class="w-4 h-4 text-primary" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
        <path d="M12 4v12M8 8l4-4 4 4M5 14v5a1 1 0 001 1h12a1 1 0 001-1v-5"/>
      </svg>
    </div>
    <div class="flex-1 min-w-0">
      <p class="text-sm text-text font-medium">Add to Home Screen</p>
      <p class="text-[11px] text-dim mb-2">Tap <span class="font-semibold text-text">Share</span> in Safari, then <span class="font-semibold text-text">Add to Home Screen</span>.</p>
      <button
        onclick={dismiss}
        class="px-2.5 py-1 text-xs text-dim hover:text-text -ml-1"
      >Got it</button>
    </div>
  </div>
{/if}
