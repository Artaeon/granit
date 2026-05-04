<script lang="ts">
  import '../app.css';
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api } from '$lib/api';
  import { page } from '$app/stores';
  import Drawer from '$lib/components/Drawer.svelte';
  import CommandPalette from '$lib/components/CommandPalette.svelte';
  import BottomNav from '$lib/components/BottomNav.svelte';
  import Toaster from '$lib/components/Toaster.svelte';
  import OfflineBanner from '$lib/components/OfflineBanner.svelte';
  import InstallPrompt from '$lib/components/InstallPrompt.svelte';
  import RunningTimer from '$lib/components/RunningTimer.svelte';
  import NavIcon from '$lib/components/NavIcon.svelte';
  import { connect, disconnect, wsConnected } from '$lib/ws';
  import { theme, nextTheme, themeIcon, themeLabel } from '$lib/stores/theme';
  import { modulesStore } from '$lib/stores/modules';
  import { sabbath, SABBATH_HIDE_MODULES } from '$lib/stores/sabbath';
  import { goto } from '$app/navigation';
  import { toast } from '$lib/components/toast';

  let palette: { show: () => void } | undefined = $state();

  let { children } = $props();

  // Manage the websocket lifecycle alongside auth
  $effect(() => {
    if ($auth) connect();
    else disconnect();
  });

  // Auto-reload when the service worker activates a new build. The SW
  // posts {type:'sw-updated'} after clients.claim(); we honor it only
  // when the page is hidden or the user has nothing in flight, so a
  // mid-edit refresh can't drop unsaved work. The drafts module also
  // protects against this — it persists every keystroke to
  // localStorage — but skipping the reload while visible keeps the UX
  // polite.
  onMount(() => {
    if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) return;
    const onMessage = (event: MessageEvent) => {
      if (event?.data?.type !== 'sw-updated') return;
      if (document.visibilityState === 'hidden') {
        location.reload();
      } else {
        // Reload on next focus loss, so the visible session isn't
        // interrupted but a tab-switch picks up the new build.
        const onceHidden = () => {
          if (document.visibilityState === 'hidden') {
            document.removeEventListener('visibilitychange', onceHidden);
            location.reload();
          }
        };
        document.addEventListener('visibilitychange', onceHidden);
      }
    };
    navigator.serviceWorker.addEventListener('message', onMessage);
    return () => navigator.serviceWorker.removeEventListener('message', onMessage);
  });

  // moduleId gates the entry against the modules store. Entries without
  // a moduleId stay visible unconditionally.
  type NavItem = { href: string; label: string; icon: string; moduleId?: string };

  // Grouped nav. Section ID is the persistence key for collapsed state
  // and the header label. The Today entry sits above all groups (no
  // header) because it's the always-on home — sections start where
  // organisation begins to help.
  const today: NavItem = { href: '/', label: 'Today', icon: 'today' };

  type NavSection = { id: string; label: string; items: NavItem[] };
  const sections: NavSection[] = [
    {
      id: 'daily',
      label: 'Daily',
      items: [
        { href: '/morning', label: 'Morning', icon: 'morning', moduleId: 'morning' },
        { href: '/tasks', label: 'Tasks', icon: 'tasks' },
        { href: '/calendar', label: 'Calendar', icon: 'calendar' },
        { href: '/jots', label: 'Jots', icon: 'jots', moduleId: 'jots' },
        { href: '/habits', label: 'Habits', icon: 'habits', moduleId: 'habit_tracker' }
      ]
    },
    {
      id: 'plan',
      label: 'Plan',
      items: [
        { href: '/vision', label: 'Vision', icon: 'vision', moduleId: 'vision' },
        { href: '/review', label: 'Review', icon: 'review', moduleId: 'weekly_review' },
        { href: '/goals', label: 'Goals', icon: 'goals', moduleId: 'goals' },
        { href: '/deadlines', label: 'Deadlines', icon: 'deadline', moduleId: 'deadlines' },
        { href: '/projects', label: 'Projects', icon: 'projects', moduleId: 'projects' }
      ]
    },
    {
      id: 'life',
      label: 'Life',
      items: [
        { href: '/finance', label: 'Finance', icon: 'finance', moduleId: 'finance' },
        { href: '/people', label: 'People', icon: 'people', moduleId: 'people' },
        { href: '/measurements', label: 'Metrics', icon: 'measurements', moduleId: 'measurements' },
        { href: '/scripture', label: 'Scripture', icon: 'scripture', moduleId: 'scripture' }
      ]
    },
    {
      id: 'knowledge',
      label: 'Knowledge',
      items: [
        { href: '/notes', label: 'Notes', icon: 'notes' },
        { href: '/objects', label: 'Objects', icon: 'objects', moduleId: 'objects' },
        { href: '/tags', label: 'Tags', icon: 'tags' }
      ]
    },
    {
      id: 'ai',
      label: 'AI',
      items: [
        { href: '/agents', label: 'Agents', icon: 'agents', moduleId: 'agents' },
        { href: '/chat', label: 'Chat', icon: 'chat', moduleId: 'chat' }
      ]
    }
  ];

  // Settings stays in the footer rail next to theme + sign-out, not as
  // a section item — it's a meta destination.
  const settingsItem: NavItem = { href: '/settings', label: 'Settings', icon: 'settings' };

  // Flat nav list — used for: route guard match, mobile back-to-section
  // header, modules filter parity. Includes Today + every section item +
  // settings so route resolution covers the full surface.
  const nav: NavItem[] = [today, ...sections.flatMap((s) => s.items), settingsItem];

  // Per-section visible items (after module filter + sabbath overlay).
  // Sections with no visible items collapse out of the rendered list
  // entirely so the user doesn't see an empty header. Sabbath mode
  // hides work modules on top of the user's persistent module config
  // — it's a temporal overlay, not a config edit.
  let visibleSections = $derived.by(() => {
    void $modulesStore;
    void $sabbath;
    return sections
      .map((s) => ({
        ...s,
        items: s.items.filter((item) => {
          if (item.moduleId && !modulesStore.isEnabled(item.moduleId)) return false;
          if ($sabbath && item.moduleId && SABBATH_HIDE_MODULES.includes(item.moduleId)) return false;
          return true;
        })
      }))
      .filter((s) => s.items.length > 0);
  });

  // ── Sidebar UX state ──────────────────────────────────────────────
  // Collapsed sections + compact mode are both per-device localStorage.
  // collapsedSections is a record of section.id → true to keep the
  // wire format tiny (only collapsed sections are stored).
  const COLLAPSED_KEY = 'granit.sidebar.collapsed';
  const COMPACT_KEY = 'granit.sidebar.compact';
  function loadCollapsed(): Record<string, boolean> {
    if (typeof localStorage === 'undefined') return {};
    try {
      return JSON.parse(localStorage.getItem(COLLAPSED_KEY) ?? '{}') as Record<string, boolean>;
    } catch {
      return {};
    }
  }
  let collapsedSections = $state<Record<string, boolean>>(loadCollapsed());
  function toggleSection(id: string) {
    const next = { ...collapsedSections };
    if (next[id]) delete next[id];
    else next[id] = true;
    collapsedSections = next;
    try { localStorage.setItem(COLLAPSED_KEY, JSON.stringify(next)); } catch {}
  }

  let compact = $state<boolean>(
    typeof localStorage !== 'undefined' && localStorage.getItem(COMPACT_KEY) === '1'
  );
  function toggleCompact() {
    compact = !compact;
    try { localStorage.setItem(COMPACT_KEY, compact ? '1' : '0'); } catch {}
  }

  // Route guard: if the user lands on a path whose module is disabled
  // (deep link, bookmark, stale tab), bounce to home. We use a tiny
  // delay-via-effect rather than a load function because layout.ts is
  // SSR-only — by the time any code runs the SPA is already mounted.
  $effect(() => {
    void $modulesStore; // re-run when modules state arrives/changes
    const path = $page.url.pathname;
    const match = nav.find(
      (n) => n.href !== '/' && (path === n.href || path.startsWith(n.href + '/'))
    );
    if (match?.moduleId && !modulesStore.isEnabled(match.moduleId)) {
      toast.info(`${match.label} is disabled — enable it in Settings → Modules`);
      goto('/', { replaceState: true });
    }
  });

  let drawerOpen = $state(false);

  // Close drawer on route change
  $effect(() => {
    void $page.url.pathname;
    drawerOpen = false;
  });

  let activeNav = $derived.by(() => {
    return nav.find((n) => n.href === $page.url.pathname || (n.href !== '/' && $page.url.pathname.startsWith(n.href)));
  });
  let title = $derived(activeNav?.label ?? 'everything');
  let showBackToSection = $derived(
    !!activeNav && activeNav.href !== '/' && $page.url.pathname !== activeNav.href
  );

  // Browser tab title. Empty home stays as just 'Granit'; deep
  // pages get 'Granit · Pagename' so multiple open tabs are
  // distinguishable. Single source of truth (the same activeNav
  // that drives the mobile header) means new routes pick this up
  // automatically when added to the nav array.
  let tabTitle = $derived.by(() => {
    if (!activeNav || activeNav.href === '/') return 'Granit';
    return `Granit · ${activeNav.label}`;
  });

  function NavLinks() {}
</script>

<svelte:head>
  <title>{tabTitle}</title>
</svelte:head>

{#snippet navItem(item: NavItem, isCompact: boolean)}
  {@const active = $page.url.pathname === item.href || (item.href !== '/' && $page.url.pathname.startsWith(item.href))}
  <a
    href={item.href}
    onclick={() => (drawerOpen = false)}
    title={isCompact ? item.label : undefined}
    aria-label={item.label}
    class="group relative flex items-center {isCompact ? 'justify-center px-2 py-2' : 'gap-3 px-3 py-2'} rounded text-sm transition-colors
      {active ? 'text-primary bg-surface1/60' : 'text-subtext hover:bg-surface0 hover:text-text'}"
  >
    <!-- Active rail: a 2px accent strip on the left edge replaces
         the heavier full-row fill, so scanning down the sidebar
         lands on the active item without the eye getting pulled. -->
    {#if active}
      <span class="absolute left-0 top-1.5 bottom-1.5 w-[2px] rounded-full bg-primary" aria-hidden="true"></span>
    {/if}
    <NavIcon name={item.icon} class="w-5 h-5 flex-shrink-0" />
    {#if !isCompact}
      <span class="truncate">{item.label}</span>
    {/if}
  </a>
{/snippet}

{#snippet navContent(isCompact: boolean)}
  <div class="flex flex-col h-full">
    <!-- Brand area collapses to icon-only in compact mode so the rail
         doesn't blow up to full width on narrow desktops. The 'e'
         monogram + accent dot reads as a logo without needing the
         full text. -->
    <div class="border-b border-surface1 {isCompact ? 'px-2 py-3 flex justify-center' : 'px-4 py-3'}">
      {#if isCompact}
        <div class="w-9 h-9 rounded bg-primary/15 text-primary flex items-center justify-center font-semibold">e</div>
      {:else}
        <div class="text-xs uppercase tracking-wider text-dim">everything</div>
        <div class="text-sm text-subtext mt-0.5">your vault, anywhere</div>
      {/if}
    </div>

    <nav class="flex-1 overflow-y-auto {isCompact ? 'px-1.5 py-3' : 'px-2 py-3'} space-y-1">
      <!-- Quick jump — compact form drops the kbd hint + label,
           keeps the icon as the click target. -->
      <button
        onclick={() => { palette?.show(); drawerOpen = false; }}
        title={isCompact ? 'Quick jump (⌘K)' : undefined}
        class="w-full flex items-center {isCompact ? 'justify-center px-2 py-2' : 'gap-3 px-3 py-2'} rounded text-sm text-subtext hover:bg-surface0 hover:text-text mb-2 transition-colors"
      >
        <NavIcon name="search" class="w-5 h-5 flex-shrink-0" />
        {#if !isCompact}
          <span class="flex-1 text-left">Quick jump</span>
          <kbd class="text-[10px] text-dim font-mono px-1.5 py-0.5 bg-surface0 border border-surface1 rounded">⌘K</kbd>
        {/if}
      </button>

      <!-- Today sits above all groups, no header, since it's home. -->
      {@render navItem(today, isCompact)}

      <!-- Sections. In compact mode the section header collapses to a
           thin separator line so the visual rhythm of grouping is
           preserved without the labels. -->
      {#each visibleSections as section}
        {@const isCollapsed = !!collapsedSections[section.id] && !isCompact}
        {#if isCompact}
          <div class="my-2 border-t border-surface1/60" aria-hidden="true"></div>
          {#each section.items as item}
            {@render navItem(item, true)}
          {/each}
        {:else}
          <div class="pt-2">
            <button
              type="button"
              onclick={() => toggleSection(section.id)}
              aria-expanded={!isCollapsed}
              class="w-full flex items-center gap-1.5 px-3 py-1 text-[10px] uppercase tracking-wider text-dim hover:text-subtext transition-colors"
            >
              <span class="flex-1 text-left">{section.label}</span>
              <svg viewBox="0 0 24 24" class="w-3 h-3 transition-transform {isCollapsed ? '-rotate-90' : ''}" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="6 9 12 15 18 9" />
              </svg>
            </button>
            {#if !isCollapsed}
              <div class="space-y-0.5 mt-0.5">
                {#each section.items as item}
                  {@render navItem(item, false)}
                {/each}
              </div>
            {/if}
          </div>
        {/if}
      {/each}
    </nav>

    <!-- Footer rail. Settings, theme, compact toggle, sign out. -->
    <div class="border-t border-surface1 {isCompact ? 'px-1.5 py-2 space-y-1' : 'px-2 py-3 space-y-1'}">
      {@render navItem(settingsItem, isCompact)}

      <button
        onclick={() => theme.set(nextTheme($theme))}
        title={isCompact ? `Theme: ${themeLabel($theme)} — tap to cycle` : undefined}
        class="w-full flex items-center {isCompact ? 'justify-center px-2 py-2' : 'gap-3 px-3 py-2'} rounded text-sm text-subtext hover:bg-surface0 hover:text-text transition-colors"
      >
        <span class="w-5 text-center text-base flex-shrink-0">{themeIcon($theme)}</span>
        {#if !isCompact}
          <span class="flex-1 text-left">Theme: {themeLabel($theme)}</span>
          <span class="text-[10px] text-dim">cycle</span>
        {/if}
      </button>

      <!-- Sabbath toggle. Hides work modules for the day (Mark 2:27).
           Auto-clears at midnight via a read-time check in the store,
           so a forgotten 'on' state recovers the next morning by
           itself. The active state pulses a small dot in compact
           mode + shifts the button to the success palette so the
           user sees at a glance whether sabbath is on. -->
      <button
        onclick={() => sabbath.toggle()}
        title={$sabbath ? 'Sabbath mode is on — tap to exit' : 'Enter sabbath mode (hides work modules for today)'}
        class="w-full flex items-center {isCompact ? 'justify-center px-2 py-2' : 'gap-3 px-3 py-2'} rounded text-sm transition-colors {$sabbath ? 'bg-success/15 text-success hover:bg-success/25' : 'text-dim hover:bg-surface0 hover:text-text'}"
      >
        <span class="w-5 text-center text-base flex-shrink-0">{$sabbath ? '🕊️' : '✦'}</span>
        {#if !isCompact}
          <span class="flex-1 text-left">{$sabbath ? 'Sabbath on' : 'Sabbath'}</span>
          <span class="text-[10px] text-dim">{$sabbath ? 'tap to exit' : 'rest day'}</span>
        {/if}
      </button>

      <!-- Desktop-only compact toggle. Hidden on mobile because the
           drawer is already an icon-poor experience and a compact
           toggle in a temporary panel doesn't save anything. -->
      <button
        onclick={toggleCompact}
        title={isCompact ? 'Expand sidebar' : 'Collapse to icons'}
        class="hidden md:flex w-full items-center {isCompact ? 'justify-center px-2 py-2' : 'gap-3 px-3 py-2'} rounded text-sm text-dim hover:bg-surface0 hover:text-text transition-colors"
      >
        <svg viewBox="0 0 24 24" class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          {#if isCompact}
            <polyline points="9 18 15 12 9 6" />
          {:else}
            <polyline points="15 18 9 12 15 6" />
          {/if}
        </svg>
        {#if !isCompact}<span class="flex-1 text-left">Collapse</span>{/if}
      </button>

      {#if !isCompact}
        <div class="flex items-center justify-between px-3 pt-1">
          <button
            onclick={async () => { try { await api.authLogout(); } catch {} auth.clear(); }}
            class="text-xs text-dim hover:text-error transition-colors"
          >
            sign out
          </button>
          <div class="flex items-center gap-1.5" title={$wsConnected ? 'live' : 'offline'}>
            <span class="w-2 h-2 rounded-full {$wsConnected ? 'bg-success' : 'bg-dim'}"></span>
            <span class="text-[10px] text-dim font-mono">v0.0.1</span>
          </div>
        </div>
      {:else}
        <!-- Compact connection pip lives at the very bottom, on its
             own line, so the rail still surfaces live/offline state. -->
        <div class="flex justify-center pt-1" title={$wsConnected ? 'live' : 'offline'}>
          <span class="w-2 h-2 rounded-full {$wsConnected ? 'bg-success' : 'bg-dim'}"></span>
        </div>
      {/if}
    </div>
  </div>
{/snippet}

<div class="h-screen flex flex-col md:flex-row overflow-hidden">
  {#if $auth}
    <!-- Mobile top bar — minimal: title + back (when in subpage) + search -->
    <header class="md:hidden flex items-center gap-2 px-3 h-12 border-b border-surface1 bg-mantle/90 backdrop-blur sticky top-0 z-30 flex-shrink-0">
      {#if showBackToSection && activeNav}
        <a
          href={activeNav.href}
          aria-label="back to {activeNav.label}"
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
      <a
        href="/settings"
        aria-label="settings"
        class="w-10 h-10 flex items-center justify-center text-subtext hover:text-primary rounded"
      >
        <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
      </a>
    </header>

    <!-- Desktop sidebar — expand/compact width is driven by the
         compact toggle in the footer rail. Both states animate via
         the transition class so the resize feels intentional. -->
    <aside
      class="hidden md:flex bg-mantle border-r border-surface1 flex-shrink-0 transition-[width] duration-150 {compact ? 'md:w-14' : 'md:w-56 lg:w-60'}"
    >
      {@render navContent(compact)}
    </aside>

    <!-- Mobile "More" drawer always renders the full (non-compact)
         nav — a temporary panel doesn't benefit from icon-only mode. -->
    <Drawer bind:open={drawerOpen} side="left">
      {@render navContent(false)}
    </Drawer>
  {/if}

  <main class="flex-1 min-h-0 min-w-0 overflow-hidden flex flex-col pb-bottomnav md:pb-0">
    <!-- Sabbath ribbon. Visible from every authed page so the state
         is unmissable; the mode auto-clears at midnight. Click to
         exit. Z-index sits below the running-timer pill so they
         don't clash. -->
    {#if $auth && $sabbath}
      <button
        type="button"
        onclick={() => sabbath.disable()}
        class="flex-shrink-0 px-4 py-1.5 bg-success/10 border-b border-success/30 text-xs text-success text-center hover:bg-success/15 transition-colors"
        title="Tap to exit sabbath mode"
      >
        🕊️ Sabbath mode is on — work modules hidden until midnight. Tap to exit.
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
{/if}
<Toaster />
