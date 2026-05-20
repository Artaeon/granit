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
  import MobileTopBar from '$lib/nav/MobileTopBar.svelte';
  import SabbathRibbon from '$lib/components/SabbathRibbon.svelte';
  import UpdateAvailableBanner from '$lib/components/UpdateAvailableBanner.svelte';
  import MobileAIFab from '$lib/components/MobileAIFab.svelte';
  import QuickCaptureFab from '$lib/components/QuickCaptureFab.svelte';
  import PomodoroPill from '$lib/components/PomodoroPill.svelte';
  import AIOverlay from '$lib/components/AIOverlay.svelte';
  import NoteTray from '$lib/components/NoteTray.svelte';
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
  import { findBinding, matchesKey } from '$lib/keybindings/registry';

  let palette: { show: () => void } | undefined = $state();

  let { children } = $props();

  // Manage the websocket lifecycle alongside auth
  $effect(() => {
    if ($auth) connect();
    else disconnect();
  });

  // Pre-hydration splash hide. Setting data-app-ready on <html>
  // triggers the `[data-app-ready] #app-splash { display: none }`
  // rule in app.html. Done in the root onMount so EVERY child page
  // benefits — even ones that have their own loading states only
  // start showing them after this fires. Wrapped in a defensive
  // try so an SSR or test environment without document doesn't
  // crash.
  onMount(() => {
    try {
      document.documentElement.setAttribute('data-app-ready', '1');
    } catch (_) {
      // No document — ignore.
    }
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
      toast.info(`${match.label} is disabled — enable it in Settings → Features`);
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

  // tray-jump — Mod+Shift+O reopens whatever the note tray remembers.
  // Mirrors the user's "ein system tray" framing: one keystroke
  // returns you to the last note from anywhere in the app. The
  // shortcut intentionally fires even when the user is typing into
  // an input/textarea: it's an app-shell move, not a text edit.
  // Skips when the tray is disabled (settings opt-out), when there's
  // nothing remembered, or when the user is already on that note.
  //
  // The chord is read from $lib/keybindings so a future remap UI
  // touches one file. get() reads each store fresh at keypress time,
  // so the $effect doesn't re-register the listener on every tray
  // tick.
  onMount(() => {
    const binding = findBinding('tray-jump');
    if (!binding) return;
    const onKey = (e: KeyboardEvent) => {
      if (!matchesKey(e, binding.keys)) return;
      if (!get(trayEnabled)) return;
      const entry = get(lastOpenNote);
      if (!entry) return;
      const targetPath = '/notes/' + encodeURIComponent(entry.path);
      const here = get(page).url.pathname;
      if (here === targetPath) return;
      e.preventDefault();
      goto(targetPath);
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  // Browser tab title. Empty home stays as just 'Granit'; deep
  // pages get 'Granit · Pagename' so multiple open tabs are
  // distinguishable. Single source of truth (the same activeNav
  // that drives the mobile header) means new routes pick this up
  // automatically when added to the nav array.
  let tabTitle = $derived.by(() => {
    if (!$activeNav || $activeNav.href === '/') return 'Granit';
    return `Granit · ${$activeNav.label}`;
  });
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
    <MobileTopBar onQuickJump={() => palette?.show()} />

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
    {#if $auth}
      <SabbathRibbon />
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
    <!-- Mobile AI FAB — auth + sabbath-gated by being inside this
         block. Sabbath skip matches the rule applied to
         QuickCaptureFab + PomodoroPill above. -->
    <MobileAIFab />
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
<UpdateAvailableBanner />
