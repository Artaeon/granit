<script lang="ts">
  import '../app.css';
  import { onMount, untrack } from 'svelte';
  import { get } from 'svelte/store';
  import { auth } from '$lib/stores/auth';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import Drawer from '$lib/components/Drawer.svelte';
  import CommandPalette from '$lib/components/CommandPalette.svelte';
  import BottomNav from '$lib/components/BottomNav.svelte';
  import Toaster from '$lib/components/Toaster.svelte';
  import OfflineBanner from '$lib/components/OfflineBanner.svelte';
  import InstallPrompt from '$lib/components/InstallPrompt.svelte';
  import RunningTimer from '$lib/components/RunningTimer.svelte';
  import NavSidebar from '$lib/nav/NavSidebar.svelte';
  import QuickCaptureFab from '$lib/components/QuickCaptureFab.svelte';
  import PomodoroPill from '$lib/components/PomodoroPill.svelte';
  import AIOverlay from '$lib/components/AIOverlay.svelte';
  import NoteTray from '$lib/components/NoteTray.svelte';
  import { openAIOverlay } from '$lib/stores/ai-overlay';
  import { recordVisit } from '$lib/stores/sidebar-recent';
  import { lastOpenNote, trayEnabled } from '$lib/stores/open-note';
  import { nav, sections } from '$lib/nav/config';
  import { activeNav } from '$lib/nav/active';
  import { connect, disconnect } from '$lib/ws';
  import { modulesStore } from '$lib/stores/modules';
  import { sabbath, SABBATH_HIDE_MODULES } from '$lib/stores/sabbath';
  import { startNavBadges } from '$lib/stores/nav-badges';
  import { expandSectionTransient, sidebarCompact } from '$lib/stores/sidebar-ui';
  import { toast } from '$lib/components/toast';

  let palette: { show: () => void } | undefined = $state();

  let { children } = $props();

  // Manage the websocket lifecycle alongside auth
  $effect(() => {
    if ($auth) connect();
    else disconnect();
  });

  // Auto-reload when the service worker activates a new build. The SW
  // posts {type:'sw-updated'} after clients.claim(). Previously we
  // waited for the next tab-hide so a mid-edit refresh wouldn't drop
  // unsaved work — but mobile users often keep one tab open all day,
  // and silently waiting meant the user thought we'd shipped nothing
  // (cache-stuck on yesterday's bundle for hours after a deploy).
  // New behaviour: hidden tab → reload immediately; visible tab →
  // surface a toast with a "Reload" action so the user gets agency
  // without being yanked mid-keystroke. The note drafts module
  // already preserves every keystroke to localStorage so even a
  // reload-during-edit can't lose data.
  let updateAvailable = $state(false);
  onMount(() => {
    // Pre-hydration splash hide. Setting data-app-ready on <html>
    // triggers the `[data-app-ready] #app-splash { display: none }`
    // rule in app.html. Done in the root onMount so EVERY child page
    // benefits — even ones that have their own loading states only
    // start showing them after this fires. Wrapped in a defensive
    // try so an SSR or test environment without document doesn't
    // crash.
    try {
      document.documentElement.setAttribute('data-app-ready', '1');
    } catch (_) {
      // No document — ignore.
    }
    if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) return;
    const onMessage = (event: MessageEvent) => {
      if (event?.data?.type !== 'sw-updated') return;
      if (document.visibilityState === 'hidden') {
        location.reload();
      } else {
        updateAvailable = true;
      }
    };
    navigator.serviceWorker.addEventListener('message', onMessage);
    return () => navigator.serviceWorker.removeEventListener('message', onMessage);
  });

  // Sidebar live badges (overdue tasks, today's events) are driven by
  // $lib/stores/nav-badges. startNavBadges() wires the auth + WS
  // lifecycle once and returns a cleanup for onMount tear-down.
  onMount(() => startNavBadges());

  // Record the current route into the recent-visits store whenever
  // navigation lands somewhere. Pulled out of $effect into untrack
  // because the store update would otherwise re-trigger the effect
  // via its own subscription if anything in the effect read it.
  $effect(() => {
    const path = $page.url.pathname;
    untrack(() => recordVisit(path));
  });

  // Auto-expand the section containing the active route. Without
  // this the user can land on /goals (collapsed-by-default Plan
  // section) and the sidebar misleads them about where they are.
  // The expand is transient — closing the section again and going
  // elsewhere restores the user's persisted preference.
  $effect(() => {
    const path = $page.url.pathname;
    if (path === '/') return;
    untrack(() => {
      for (const s of sections) {
        const inSection = s.items.some((it) => path === it.href || path.startsWith(it.href + '/'));
        if (inSection) expandSectionTransient(s.id);
      }
    });
  });

  // Route guard: if the user lands on a path whose module is disabled
  // (deep link, bookmark, stale tab), bounce to home. We use a tiny
  // delay-via-effect rather than a load function because layout.ts is
  // SSR-only — by the time any code runs the SPA is already mounted.
  // Also bounces sabbath-hidden modules — typing /tasks during
  // Sabbath should redirect to home with a soft message rather than
  // letting the user back-door past the discipline.
  $effect(() => {
    void $modulesStore; // re-run when modules state arrives/changes
    void $sabbath;
    const path = $page.url.pathname;
    const match = nav.find(
      (n) => n.href !== '/' && (path === n.href || path.startsWith(n.href + '/'))
    );
    if (match?.moduleId && !modulesStore.isEnabled(match.moduleId)) {
      toast.info(`${match.label} is disabled — enable it in Settings → Modules`);
      goto('/', { replaceState: true });
      return;
    }
    if ($sabbath && match?.moduleId && SABBATH_HIDE_MODULES.includes(match.moduleId)) {
      toast.info(`${match.label} is hidden during Sabbath — exit Sabbath mode to access.`);
      goto('/', { replaceState: true });
    }
  });

  let drawerOpen = $state(false);

  // Close drawer on route change
  $effect(() => {
    void $page.url.pathname;
    drawerOpen = false;
  });

  // Mod-Shift-O — jump to whatever the open-note tray remembers.
  // Mirrors the user's "ein system tray" framing: one keystroke
  // returns you to the last note from anywhere in the app. The
  // shortcut intentionally fires even when the user is typing into
  // an input/textarea, mirroring Mod+J (Ask AI) and Mod+K (palette):
  // these are global app-shell shortcuts, not text editing ones.
  // Skips when the tray is disabled (settings opt-out), when there's
  // nothing remembered, or when the user is already on that note.
  //
  // get() is intentional: we don't want this $effect to re-subscribe
  // (and re-register the listener) every time the tray store ticks.
  // The listener is registered once and reads fresh values at
  // keypress time.
  onMount(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey) || !e.shiftKey || e.altKey) return;
      if (e.key.toLowerCase() !== 'o') return;
      if (!get(trayEnabled)) return;
      const entry = get(lastOpenNote);
      if (!entry) return;
      const targetPath = '/notes/' + encodeURIComponent(entry.path);
      // Already there → suppress (the chip would be hidden anyway).
      const here = get(page).url.pathname;
      if (here === targetPath) return;
      e.preventDefault();
      goto(targetPath);
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  let title = $derived($activeNav?.label ?? 'everything');
  let showBackToSection = $derived(
    !!$activeNav && $activeNav.href !== '/' && $page.url.pathname !== $activeNav.href
  );

  // Browser tab title. Empty home stays as just 'Granit'; deep
  // pages get 'Granit · Pagename' so multiple open tabs are
  // distinguishable. Single source of truth (the same activeNav
  // that drives the mobile header) means new routes pick this up
  // automatically when added to the nav array.
  let tabTitle = $derived.by(() => {
    if (!$activeNav || $activeNav.href === '/') return 'Granit';
    return `Granit · ${$activeNav.label}`;
  });

  function NavLinks() {}
</script>

<svelte:head>
  <title>{tabTitle}</title>
</svelte:head>

<!--
  h-dvh (dynamic viewport height) instead of h-screen (100vh) so the
  shell actually shrinks when the on-screen keyboard opens on mobile.
  100vh on iOS Safari stays at the full screen height regardless of
  the keyboard — that's the whole reason child surfaces (chat, ai
  overlay) need their own visualViewport hacks. h-dvh fixes this at
  the root, and falls back to 100vh on browsers that don't support
  dvh (auto via Tailwind's fallback chain).
-->
<div class="h-dvh flex flex-col md:flex-row overflow-hidden">
  {#if $auth}
    <!-- Mobile top bar — back (when in subpage) · title · search.
         Settings used to live here too but it's already in the More
         drawer now, so the redundancy is gone. The bottom nav is the
         primary nav surface; this top bar is just contextual. -->
    <header class="md:hidden flex items-center gap-1 px-3 h-12 border-b border-surface1 bg-mantle sticky top-0 z-30 flex-shrink-0">
      {#if showBackToSection && $activeNav}
        <a
          href={$activeNav.href}
          aria-label="back to {$activeNav.label}"
          class="w-10 h-10 -ml-2 flex items-center justify-center text-subtext hover:text-primary rounded"
        >
          <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M15 18l-6-6 6-6" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
        </a>
      {/if}
      <h1 class="text-base font-medium text-text flex-1 truncate">{title}</h1>
      <button
        onclick={() => palette?.show()}
        aria-label="search"
        class="w-10 h-10 flex items-center justify-center text-subtext hover:text-primary rounded"
      >
        <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="11" cy="11" r="8" /><path d="m21 21-4.3-4.3" stroke-linecap="round" />
        </svg>
      </button>
    </header>

    <!-- Desktop sidebar — expand/compact width is driven by the
         compact toggle in the footer rail. Both states animate via
         the transition class so the resize feels intentional. -->
    <aside
      class="hidden md:flex bg-mantle border-r border-surface1 flex-shrink-0 transition-[width] duration-150 {$sidebarCompact ? 'md:w-14' : 'md:w-56 lg:w-60'}"
    >
      <NavSidebar
        isCompact={$sidebarCompact}
        onNavigate={() => (drawerOpen = false)}
        onQuickJump={() => palette?.show()}
      />
    </aside>

    <!-- Mobile "More" drawer always renders the full (non-compact)
         nav — a temporary panel doesn't benefit from icon-only mode. -->
    <Drawer bind:open={drawerOpen} side="left">
      <NavSidebar
        isCompact={false}
        onNavigate={() => (drawerOpen = false)}
        onQuickJump={() => palette?.show()}
      />
    </Drawer>
  {/if}

  <!-- NoteTray sets --note-tray-h on <html> when visible. The
       tray-reserve class stacks that var on top of the existing
       pb-bottomnav (mobile) / pb-0 (desktop) base so the bottom
       28px of editable content isn't clipped behind the tray. -->
  <main
    class="flex-1 min-h-0 min-w-0 overflow-hidden flex flex-col pb-bottomnav md:pb-0 main-with-tray"
    style="padding-right: var(--ai-pinned-w, 0px);"
  >
    <!-- Sabbath ribbon. Visible from every authed page so the state
         is unmissable; the mode auto-clears at midnight. Click to
         exit. Z-index sits below the running-timer pill so they
         don't clash. -->
    {#if $auth && $sabbath}
      <button
        type="button"
        onclick={() => sabbath.disable()}
        class="flex-shrink-0 px-4 py-1.5 bg-success text-on-primary text-xs text-center hover:opacity-90 transition-colors"
        title="Tap to exit sabbath mode"
      >
        Sabbath mode is on — work modules hidden until midnight. Tap to exit.
      </button>
    {/if}
    <div class="flex-1 min-h-0 overflow-hidden">
      {@render children()}
    </div>
  </main>

  {#if $auth}
    <BottomNav onMore={() => (drawerOpen = true)} />
  {/if}
</div>

{#if $auth}
  <CommandPalette bind:this={palette} />
  <OfflineBanner />
  <!-- Floating top-right pill that's only visible while a clock-in
       is running. Position keeps it out of the way of the editor and
       the offline banner; on mobile the component itself hides
       below the sm breakpoint to avoid crowding the bottom nav. -->
  <div class="fixed top-3 right-3 z-30">
    <RunningTimer />
  </div>
  <!-- One-time PWA install hint. The component is self-gating: it only
       renders when the browser has fired beforeinstallprompt (Chromium)
       or when we detect iOS Safari, and only when the user hasn't
       already installed or dismissed it. Auth-gated because pre-login
       is too early to be useful. -->
  <InstallPrompt />
  <!-- Global quick-capture FAB. Single keystroke (Ctrl+Shift+N) opens
       a small task-capture modal from anywhere in the app. Auth-gated
       since pre-login captures don't have a daily note to write to.
       Hidden during Sabbath — capturing a task is the work the day
       is supposed to be free of. -->
  {#if !$sabbath}
    <QuickCaptureFab />
    <!-- Pomodoro pill — focus-session timer that survives navigation
         + tab close (state in localStorage). Hidden during Sabbath
         (the day's not for focus sessions). The pill itself self-
         hides when idle + no recent finish, so the bottom-right
         stays clean for users who don't use it. -->
    <PomodoroPill />
    <!-- Mobile AI FAB — Mod+J doesn't exist on a phone keyboard,
         and the sidebar "Ask AI" button is hidden behind a drawer
         on mobile. This sparkle button sits above the bottom nav
         (with iOS safe-area padding) so the AI overlay is one tap
         away. Desktop hides this since the sidebar pill covers it.
         Sabbath skip matches the rule applied to QuickCaptureFab. -->
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
  {/if}
  <!-- Global AI overlay. Listens for Mod+J on its own — no
       external trigger needed. Auth-gated since pre-login the
       configured-LLM lookup would 401. -->
  <AIOverlay />

  <!-- Persistent "open note" tray. Slim bottom bar (desktop) /
       chip above the bottom nav (mobile) that surfaces the last
       opened note + any tray-pinned notes so the user can jump
       back from anywhere. The component self-gates on its own
       settings toggle + visibility rules (hidden when on the
       active note, hidden when nothing stored). Auth-gated since
       a pre-login user has no vault to remember from. -->
  <NoteTray />
{/if}
<Toaster />

<!-- "New version available" banner. Sits ABOVE the bottom nav (z-40 vs
     bottom-nav's z-30) and uses sm:bottom-3 on desktop so it doesn't
     collide with the nav rail edge. Kept dismissable because the user
     might want to finish a thought before we yank them — but the action
     button reloads on tap so they don't have to hunt for "clear cache". -->
{#if updateAvailable}
  <div
    role="status"
    class="fixed inset-x-3 z-40 bottom-[calc(3.5rem+env(safe-area-inset-bottom,0px)+0.75rem)] md:bottom-3 md:left-auto md:right-3 md:max-w-sm bg-mantle border border-primary rounded-lg shadow-2xl p-3 flex items-center gap-3"
  >
    <div class="flex-1 min-w-0">
      <div class="text-sm font-medium text-text">Update available</div>
      <div class="text-xs text-dim mt-0.5">Reload to pick up the latest build.</div>
    </div>
    <button
      onclick={() => location.reload()}
      class="px-3 py-1.5 text-sm bg-primary text-on-primary rounded font-medium hover:opacity-90 flex-shrink-0"
    >Reload</button>
    <button
      onclick={() => (updateAvailable = false)}
      aria-label="dismiss"
      class="text-dim hover:text-text flex-shrink-0 px-1"
    >×</button>
  </div>
{/if}
