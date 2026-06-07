<script lang="ts">
  import '../app.css';
  import { onMount, untrack, tick } from 'svelte';
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
  import RightPane from '$lib/nav/RightPane.svelte';
  import TabStrip from '$lib/nav/TabStrip.svelte';
  import SabbathRibbon from '$lib/components/SabbathRibbon.svelte';
  import UpdateAvailableBanner from '$lib/components/UpdateAvailableBanner.svelte';
  import MobileAIFab from '$lib/components/MobileAIFab.svelte';
  import QuickCaptureFab from '$lib/components/QuickCaptureFab.svelte';
  import PomodoroPill from '$lib/components/PomodoroPill.svelte';
  import AIOverlay from '$lib/components/AIOverlay.svelte';
  import NoteTray from '$lib/components/NoteTray.svelte';
  import ShortcutsOverlay from '$lib/components/ShortcutsOverlay.svelte';
  import { lastOpenNote, trayEnabled } from '$lib/stores/open-note';
  import { nav, sections } from '$lib/nav/config';
  import { activeNav } from '$lib/nav/active';
  import { connect, disconnect } from '$lib/ws';
  import { modulesStore } from '$lib/stores/modules';
  import { sabbath, SABBATH_HIDE_MODULES } from '$lib/stores/sabbath';
  import { startNavBadges } from '$lib/stores/nav-badges';
  import {
    expandSectionTransient,
    clearTransientExpands,
    sidebarCompact
  } from '$lib/stores/sidebar-ui';
  import {
    rightPaneStore,
    toggleRightPane,
    setRightPaneContent,
    type RightPaneContent
  } from '$lib/stores/rightPane';
  import {
    tabsStore,
    setActiveTabUrl,
    newTab,
    closeTab,
    cycleTab,
    activateNth,
    setActiveScroll,
    clearTabs
  } from '$lib/stores/tabs';
  import { titleForUrl } from '$lib/nav/tabTitle';
  import { aiOverlayOpen } from '$lib/stores/ai-overlay';
  import { toast } from '$lib/components/toast';
  import { findBinding, matchesKey } from '$lib/keybindings/registry';
  import { isMobile, isMobileNow } from '$lib/util/breakpoint';
  import { workspaceStoreSingleton } from '$lib/workspace/workspaceStore.svelte';
  import { routeToPaneKind } from '$lib/workspace/paneRegistry';
  import { leaves } from '$lib/workspace/splitTree';
  import { isTypingTarget } from '$lib/util/isTypingTarget';
  import { todayISO } from '$lib/util/date';

  let palette: { show: () => void } | undefined = $state();
  let shortcutsOpen = $state(false);

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

  // Auto-expand the section containing the active route. Without
  // this the user can land on /goals (collapsed-by-default Plan
  // section) and the sidebar misleads them about where they are.
  // The expand lives in a separate transient store layered over the
  // persisted collapse state; navigating away clears it so the
  // user's persisted preference takes over again.
  $effect(() => {
    const path = $page.url.pathname;
    untrack(() => {
      clearTransientExpands();
      if (path === '/') return;
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
  //
  // Wait for $modulesStore.loaded before deciding. modulesStore.isEnabled
  // defaults to `true` for unknown ids while the API request is in
  // flight, so without this guard a deep link to /finance on a slow
  // network would render briefly, then the module status would arrive,
  // and if the module IS enabled the guard would still re-evaluate —
  // but the cold path can produce a spurious redirect when the user
  // navigates DURING the load. Cheap, correct, no race.
  $effect(() => {
    if (!$modulesStore.loaded) return;
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

  // shortcuts-help — `?` opens the cheat-sheet from anywhere in the
  // app, BUT not while the user is typing in an input/textarea
  // (otherwise `?` in a text field would never reach the document).
  // No Mod modifier — it's a single-key shortcut, app-shell scope.
  onMount(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== '?') return;
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      const target = e.target as HTMLElement | null;
      const tag = target?.tagName;
      if (tag === 'INPUT' || tag === 'TEXTAREA' || target?.isContentEditable) return;
      e.preventDefault();
      shortcutsOpen = true;
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  // focus-page-search — `/` from anywhere outside a text input focuses
  // the current page's primary search/filter input (the one tagged
  // with data-page-search="1"). Pages without such an input silently
  // no-op so the keystroke isn't "stolen" with nothing visible to show
  // for it. Some routes (deadlines, jots) still wire their own `/`
  // handler that targets the same input — both fire, both focus the
  // same element, harmless.
  //
  // go-to-today — `g` then `d` within 350ms jumps to today's Daily
  // note. Two-key chord from the vim / Gmail tradition. State lives in
  // closure-local vars (NOT $state) since this listener doesn't render
  // anything. A timer clears the pending `g` so an idle `g` doesn't
  // hijack a later `d` press. Any non-`d` key during the window also
  // cancels — so `g` then `e` doesn't accidentally feel jumpy.
  onMount(() => {
    let gPending = false;
    let gTimer: ReturnType<typeof setTimeout> | null = null;
    const clearG = () => {
      gPending = false;
      if (gTimer) {
        clearTimeout(gTimer);
        gTimer = null;
      }
    };

    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey) {
        clearG();
        return;
      }
      if (isTypingTarget(e.target)) {
        clearG();
        return;
      }

      // `/` — focus page search
      if (e.key === '/') {
        clearG();
        const el = document.querySelector(
          '[data-page-search="1"]'
        ) as HTMLInputElement | null;
        if (el) {
          e.preventDefault();
          el.focus();
          el.select?.();
        }
        return;
      }

      // `g d` — go to today's daily note
      if (gPending && e.key === 'd') {
        clearG();
        e.preventDefault();
        goto('/notes/' + encodeURIComponent(`Daily/${todayISO()}.md`));
        return;
      }
      // `g w` — promote the current route into the focused leaf of
      // the active workspace. Replaces rather than splitting so a
      // repeated press doesn't stutter into a chain of duplicate
      // panes. The store tracks the focused leaf per-workspace and
      // auto-coerces to a real id on layout change. No-op when the
      // route has no pane counterpart.
      if (gPending && e.key === 'w') {
        clearG();
        const kind = routeToPaneKind(get(page).url.pathname);
        if (!kind) return;
        e.preventDefault();
        const store = workspaceStoreSingleton();
        const targetId = store.focusedLeafId || leaves(store.active.layout)[0]?.id;
        if (targetId) store.setPane(targetId, kind);
        goto('/workspace');
        return;
      }
      if (e.key === 'g') {
        gPending = true;
        if (gTimer) clearTimeout(gTimer);
        gTimer = setTimeout(() => {
          gPending = false;
          gTimer = null;
        }, 350);
        return;
      }
      // Any other key inside the window cancels the pending `g`.
      if (gPending) clearG();
    };

    window.addEventListener('keydown', onKey);
    return () => {
      window.removeEventListener('keydown', onKey);
      if (gTimer) clearTimeout(gTimer);
    };
  });

  // Global mobile-keyboard awareness. Two detection paths fold into a
  // single data-kb-open signal on <html> that CSS / child components
  // can read:
  //
  //   1. visualViewport delta — innerHeight - vv.height > 120 means
  //      the keyboard is up. This is the only signal on older iOS
  //      (<16.4) and Chrome Android (<108) where the layout viewport
  //      doesn't shrink when the keyboard opens.
  //
  //   2. focusin / focusout on editable elements — input / textarea /
  //      contentEditable. This is the only signal on browsers that
  //      DO honour `interactive-widget=resizes-content` in the
  //      viewport meta (iOS 16.4+, Chrome 108+), because there
  //      innerHeight and vv.height shrink TOGETHER, leaving the
  //      delta at zero. Without the focus path, data-kb-open would
  //      never fire on modern phones and bottom-nav / editor
  //      toolbar logic that depends on it would break.
  //
  // Either path setting kb-open is enough; we OR them. --kb-h still
  // gets the viewport delta (zero on modern browsers, useful on
  // legacy) so floating UI that wants a precise lift value can use it.
  onMount(() => {
    if (typeof window === 'undefined') return;
    const html = document.documentElement;
    const vv = window.visualViewport;

    let viewportKbOpen = false;
    let focusKbOpen = false;

    function commit() {
      if (viewportKbOpen || focusKbOpen) html.setAttribute('data-kb-open', '1');
      else html.removeAttribute('data-kb-open');
    }

    function isEditable(el: EventTarget | null): boolean {
      const node = el as HTMLElement | null;
      if (!node || node.nodeType !== 1) return false;
      const tag = node.tagName;
      if (tag === 'INPUT') {
        const t = (node as HTMLInputElement).type;
        // checkbox / radio / button etc. don't bring up the soft
        // keyboard, so they shouldn't flip data-kb-open.
        return ['text', 'search', 'email', 'tel', 'url', 'password', 'number', 'date', 'time', 'datetime-local', 'month', 'week'].includes(t);
      }
      if (tag === 'TEXTAREA') return true;
      return node.isContentEditable === true;
    }

    function updateViewport() {
      const obscured = Math.max(0, window.innerHeight - (vv?.height ?? window.innerHeight));
      html.style.setProperty('--kb-h', `${obscured}px`);
      // 120px threshold separates "keyboard is up" from "URL bar
      // collapsed during scroll" (which shrinks VV by 40-80px on iOS).
      viewportKbOpen = obscured > 120;
      commit();
    }

    // Only mobile breakpoint cares about the soft keyboard. Desktop
    // input focus shouldn't masquerade as "keyboard up" — there's no
    // OS keyboard to be displaced. isMobileNow() is a sync read on
    // every event so a user crossing the breakpoint mid-focus gets
    // the right behaviour at the next event boundary.
    function onFocusIn(ev: FocusEvent) {
      if (!isMobileNow()) return;
      if (isEditable(ev.target)) {
        focusKbOpen = true;
        commit();
      }
    }
    function onFocusOut() {
      if (!isMobileNow()) return;
      // Delay one tick: focus often shifts editable→editable without
      // an intervening blur (e.g. picker → input). Re-check on the
      // next microtask so a focus-bounce doesn't toggle the keyboard
      // signal off-then-on, which would cause the bottom-nav /
      // toolbar to flicker.
      queueMicrotask(() => {
        focusKbOpen = isEditable(document.activeElement);
        commit();
      });
    }

    // Subscribe to the breakpoint store so a crossing to desktop
    // (iPad rotation, touch-laptop resize) clears any stale
    // focusKbOpen flag set while the device was in mobile mode.
    // Without this, rotating an iPad with a text input still
    // focused would leave data-kb-open=1 on desktop and bottom-nav-
    // hide-on-kb would persist incorrectly.
    const unsubMobile = isMobile.subscribe((m) => {
      if (!m && focusKbOpen) {
        focusKbOpen = false;
        commit();
      }
    });

    if (vv) {
      vv.addEventListener('resize', updateViewport);
      vv.addEventListener('scroll', updateViewport);
      updateViewport();
    }
    document.addEventListener('focusin', onFocusIn);
    document.addEventListener('focusout', onFocusOut);

    return () => {
      if (vv) {
        vv.removeEventListener('resize', updateViewport);
        vv.removeEventListener('scroll', updateViewport);
      }
      document.removeEventListener('focusin', onFocusIn);
      document.removeEventListener('focusout', onFocusOut);
      unsubMobile();
    };
  });

  // Right-pane global shortcuts. Mod+\ toggles the companion pane;
  // Mod+Shift+1..5 jump to a content option. Both fire from
  // anywhere — including text inputs — because they're app-shell
  // moves, not text edits. Bindings are read from the registry so
  // the cheat sheet stays in sync. The numeric leaf-key compare in
  // matchesKey handles 'Mod+Shift+1'..'Mod+Shift+5' via the single-
  // char path (event.key for digit keys is the digit itself).
  onMount(() => {
    const toggleBinding = findBinding('right-pane-toggle');
    const contentBindings: Array<{ id: string; content: RightPaneContent }> = [
      { id: 'right-pane-calendar', content: 'calendar' },
      { id: 'right-pane-notes', content: 'notes' },
      { id: 'right-pane-ai', content: 'ai' },
      { id: 'right-pane-vision', content: 'vision' },
      { id: 'right-pane-widgets', content: 'widgets' },
      { id: 'right-pane-tasks', content: 'tasks' },
      { id: 'right-pane-today', content: 'today' },
      { id: 'right-pane-goals', content: 'goals' },
      { id: 'right-pane-habits', content: 'habits' },
      { id: 'right-pane-dashboard', content: 'dashboard' }
    ];
    const contentChords = contentBindings
      .map((b) => ({ ...b, binding: findBinding(b.id) }))
      .filter((b): b is typeof b & { binding: NonNullable<ReturnType<typeof findBinding>> } => !!b.binding);

    const onKey = (e: KeyboardEvent) => {
      if (toggleBinding && matchesKey(e, toggleBinding.keys)) {
        e.preventDefault();
        toggleRightPane();
        return;
      }
      for (const c of contentChords) {
        if (matchesKey(e, c.binding.keys)) {
          e.preventDefault();
          setRightPaneContent(c.content);
          return;
        }
      }
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

  // ── Multi-tab Phase 2 wiring ──────────────────────────────────────
  // mainScrollEl is the per-route scrollable container — we bind to
  // it so we can capture the LEAVING tab's scrollTop before a switch
  // and restore the ENTERING tab's scrollTop after the new route
  // renders. The strip itself reads $tabsStore directly; the layout
  // only owns the navigation interception + scroll bookkeeping.
  let mainScrollEl: HTMLElement | undefined = $state();

  // Track the pathname+search the layout last wrote to the store so
  // we don't bounce the active tab title when the page store fires a
  // re-derivation for the same URL (e.g. hashchange on the same path).
  let lastWrittenUrl = $state<string | null>(null);

  // Sync the active tab with the current navigation. Treats sidebar
  // clicks as "address-bar typing" — they update the active tab's
  // URL in place rather than creating a new tab. New tabs are
  // explicitly opened via the TabStrip + or Mod+T.
  $effect(() => {
    const url = $page.url.pathname + $page.url.search;
    const label = $activeNav?.label ?? titleForUrl(url);
    if (url === lastWrittenUrl) return;
    lastWrittenUrl = url;
    setActiveTabUrl(url, label);
  });

  // Scroll persistence on tab switch. We observe activeTabId
  // transitions: before the new tab activates we capture the current
  // mainScrollEl.scrollTop into the LEAVING tab (already done by the
  // strip's click handler? no — the store update happens AFTER nav
  // goto in our setup, so the active id flips first). Instead we
  // listen for activeTabId changes here and, on the NEXT frame after
  // the new route has rendered, restore that tab's persisted scroll.
  // The capture path runs continuously on scroll via the listener
  // below so the LEAVING value is always fresh.
  let lastActiveId = $state<string | null>(null);
  $effect(() => {
    const id = $tabsStore.activeTabId;
    if (id === lastActiveId) return;
    lastActiveId = id;
    if (!id) return;
    const tab = $tabsStore.tabs.find((t) => t.id === id);
    if (!tab) return;
    // Wait for the new route to render before scrolling. tick() +
    // rAF together cover both Svelte's microtask and the browser's
    // paint, which fixes the "scroll resets to 0 then jumps" jank
    // that happens when restoring on the same frame as the route
    // swap.
    tick().then(() => {
      requestAnimationFrame(() => {
        if (!mainScrollEl) return;
        mainScrollEl.scrollTop = tab.scrollTop;
      });
    });
  });

  // Capture scroll position continuously into the active tab. Cheap
  // — just a writable update keyed by id. We attach to mainScrollEl
  // once it's bound; the effect re-runs if mainScrollEl ever rebinds
  // (e.g. layout remount).
  $effect(() => {
    const el = mainScrollEl;
    if (!el) return;
    let raf = 0;
    const onScroll = () => {
      // Coalesce to next frame so a fast scroll doesn't write
      // dozens of times per second. Single rAF is enough — the
      // captured value is approximate by design.
      if (raf) return;
      raf = requestAnimationFrame(() => {
        raf = 0;
        setActiveScroll(el.scrollTop);
      });
    };
    el.addEventListener('scroll', onScroll, { passive: true });
    return () => {
      el.removeEventListener('scroll', onScroll);
      if (raf) cancelAnimationFrame(raf);
    };
  });

  // Tab keyboard shortcuts. Listener kept in a single onMount so
  // teardown is one removeEventListener. The Mod+1..9 path defers
  // to the AI overlay — when the overlay is open it owns those
  // chords for mode quick-switch, which is the documented behaviour
  // (see AIOverlay.svelte's effect at L1767). Mod+Tab cycles tabs;
  // we don't try to handle Mod+Tab without any tabs open (the
  // browser would normally hand it to the OS, but Chromium/Firefox
  // never deliver Mod+Tab to web apps anyway — we keep the binding
  // in the registry for documentation).
  onMount(() => {
    function isMac(): boolean {
      return typeof navigator !== 'undefined' &&
        /Mac|iPhone|iPad/i.test(navigator.platform || navigator.userAgent);
    }
    const onKey = (e: KeyboardEvent) => {
      const mod = isMac() ? e.metaKey : e.ctrlKey;
      if (!mod) return;
      // Mod+T → new tab on current route
      if (!e.shiftKey && !e.altKey && e.key.toLowerCase() === 't') {
        e.preventDefault();
        const url = $page.url.pathname + $page.url.search;
        const label = $activeNav?.label ?? titleForUrl(url);
        newTab(url, label);
        // newTab activates the new tab; navigate to refresh route
        // state (same URL — SvelteKit no-ops but our $effect picks
        // up the activeTabId change and restores scroll = 0).
        goto(url);
        return;
      }
      // Mod+W → close active tab
      if (!e.shiftKey && !e.altKey && e.key.toLowerCase() === 'w') {
        e.preventDefault();
        const activeId = $tabsStore.activeTabId;
        if (!activeId) return;
        const { nextUrl } = closeTab(activeId);
        if ($tabsStore.tabs.length === 0) {
          clearTabs();
          goto('/');
        } else if (nextUrl) {
          goto(nextUrl);
        }
        return;
      }
      // Mod+Tab / Mod+Shift+Tab → cycle. preventDefault keeps the
      // browser from doing its own tab cycle (which would be a
      // no-op for a single-page app, but cleaner not to flash).
      if (e.key === 'Tab') {
        if ($tabsStore.tabs.length === 0) return;
        e.preventDefault();
        const url = cycleTab(e.shiftKey ? -1 : 1);
        if (url) goto(url);
        return;
      }
      // Mod+1..9 → activate Nth tab. Defer to the AI overlay when
      // it's open (the overlay's own listener uses preventDefault
      // but we belt-and-braces gate here too so the ordering
      // doesn't matter).
      if (!e.shiftKey && !e.altKey && e.key >= '1' && e.key <= '9') {
        if (get(aiOverlayOpen)) return;
        const n = parseInt(e.key, 10);
        const url = activateNth(n);
        if (url) {
          e.preventDefault();
          goto(url);
        }
        return;
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
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
      class="hidden md:flex bg-mantle border-r border-surface1 flex-shrink-0 transition-[width] duration-150 {$sidebarCompact ? 'md:w-12' : 'md:w-48 lg:w-52'}"
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
    class="flex-1 min-h-0 min-w-0 overflow-hidden flex flex-col main-with-tray"
    style="padding-right: var(--ai-pinned-w, 0px);"
  >
    {#if $auth}
      <SabbathRibbon />
      <!-- Multi-tab Phase 2 strip. Self-hides when no tabs are open
           (initial / single-route state pays no vertical cost) and
           hides itself on mobile (md:flex) until Phase 3. The
           scrollable content area below owns the per-route scroll
           position which we persist into the active tab. -->
      <TabStrip />
    {/if}
    <!-- Inner content area. overflow-hidden is intentional — each
         route owns its own scroll container (h-full overflow-y-auto)
         so the layout doesn't double-scroll. mainScrollEl binding
         is kept for scroll persistence on routes that DON'T own
         their scroll (Phase 3 will deepen per-page integration). -->
    <div bind:this={mainScrollEl} class="flex-1 min-h-0 overflow-hidden">
      {@render children()}
    </div>
  </main>

  {#if $auth && $rightPaneStore.open}
    <!-- Right pane companion column. Phase 1.5 added a mobile bottom-
         sheet variant — the component now branches internally on
         viewport (mediaQuery md+) and renders either a flex-sibling
         column (desktop) or a fixed bottom sheet (mobile). Auth-gated
         since none of its content (events, notes, vision) is
         reachable pre-login. -->
    <RightPane />
  {/if}

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

<!-- Global shortcuts cheat sheet. Opens via `?` from anywhere; the
     overlay self-derives its contents from the KEYBINDINGS registry
     so there's no drift between the chord that actually fires and
     the row the user sees. Auth-agnostic — the cheat sheet is
     useful even on the login screen ("how do I see what I can do
     once I'm in?"). -->
<ShortcutsOverlay bind:open={shortcutsOpen} onClose={() => (shortcutsOpen = false)} />
