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

  const nav = [
    { href: '/', label: 'Today', icon: 'today' },
    { href: '/morning', label: 'Morning', icon: 'morning' },
    { href: '/tasks', label: 'Tasks', icon: 'tasks' },
    { href: '/calendar', label: 'Calendar', icon: 'calendar' },
    { href: '/habits', label: 'Habits', icon: 'habits' },
    { href: '/goals', label: 'Goals', icon: 'goals' },
    { href: '/projects', label: 'Projects', icon: 'projects' },
    { href: '/agents', label: 'Agents', icon: 'agents' },
    { href: '/chat', label: 'Chat', icon: 'chat' },
    { href: '/scripture', label: 'Scripture', icon: 'scripture' },
    { href: '/objects', label: 'Objects', icon: 'objects' },
    { href: '/tags', label: 'Tags', icon: 'tags' },
    { href: '/jots', label: 'Jots', icon: 'jots' },
    { href: '/notes', label: 'Notes', icon: 'notes' },
    { href: '/settings', label: 'Settings', icon: 'settings' }
  ];

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

  function NavLinks() {}
</script>

{#snippet navContent()}
  <div class="flex flex-col h-full">
    <div class="px-4 py-3 border-b border-surface1">
      <div class="text-xs uppercase tracking-wider text-dim">everything</div>
      <div class="text-sm text-subtext mt-0.5">your vault, anywhere</div>
    </div>
    <nav class="flex-1 px-2 py-3 space-y-0.5">
      <button
        onclick={() => { palette?.show(); drawerOpen = false; }}
        class="w-full flex items-center gap-3 px-3 py-2.5 rounded text-sm text-subtext hover:bg-surface0 mb-1"
      >
        <NavIcon name="search" class="w-5 h-5 flex-shrink-0" />
        <span class="flex-1 text-left">Quick jump</span>
        <kbd class="text-[10px] text-dim font-mono px-1.5 py-0.5 bg-surface0 border border-surface1 rounded">⌘K</kbd>
      </button>
      {#each nav as item}
        {@const active = $page.url.pathname === item.href || (item.href !== '/' && $page.url.pathname.startsWith(item.href))}
        <a
          href={item.href}
          onclick={() => (drawerOpen = false)}
          class="flex items-center gap-3 px-3 py-2.5 rounded text-sm
            {active ? 'bg-surface1 text-primary' : 'text-subtext hover:bg-surface0'}"
        >
          <NavIcon name={item.icon} class="w-5 h-5 flex-shrink-0" />
          <span>{item.label}</span>
        </a>
      {/each}
    </nav>
    <div class="px-3 py-3 border-t border-surface1 space-y-2">
      <button
        onclick={() => theme.set(nextTheme($theme))}
        class="w-full flex items-center gap-3 px-3 py-2 rounded text-sm text-subtext hover:bg-surface0 transition-colors"
      >
        <span class="w-5 text-center text-base">{themeIcon($theme)}</span>
        <span class="flex-1 text-left">Theme: {themeLabel($theme)}</span>
        <span class="text-[10px] text-dim">tap to cycle</span>
      </button>
      <div class="flex items-center justify-between px-3">
        <button
          onclick={async () => { try { await api.authLogout(); } catch {} auth.clear(); }}
          class="text-xs text-dim hover:text-error"
        >
          sign out
        </button>
        <div class="flex items-center gap-1.5">
          <span class="w-2 h-2 rounded-full {$wsConnected ? 'bg-success' : 'bg-dim'}" title={$wsConnected ? 'live' : 'offline'}></span>
          <span class="text-[10px] text-dim font-mono">v0.0.1</span>
        </div>
      </div>
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

    <!-- Desktop sidebar -->
    <aside class="hidden md:flex md:w-56 lg:w-60 bg-mantle border-r border-surface1 flex-shrink-0">
      {@render navContent()}
    </aside>

    <!-- Mobile "More" drawer (full nav) — opened from bottom-nav More button -->
    <Drawer bind:open={drawerOpen} side="left">
      {@render navContent()}
    </Drawer>
  {/if}

  <main class="flex-1 min-h-0 min-w-0 overflow-hidden">
    {@render children()}
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
