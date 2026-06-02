<!--
  Footer rail of the sidebar: sabbath row + utility strip
  (theme cycle, compact toggle, sign-out, ws live/offline pip).

  Extracted from NavSidebar.svelte so the shell stays focused on
  composition (brand / search / pinned / essentials / sections).
  All state is read from the same stores the parent uses; the
  only prop is isCompact + the navigate callback.
-->
<script lang="ts">
  import { sabbath } from '$lib/stores/sabbath';
  import { theme, nextTheme, themeLabel } from '$lib/stores/theme';
  import { toggleSidebarCompact } from '$lib/stores/sidebar-ui';
  import { auth } from '$lib/stores/auth';
  import { wsConnected } from '$lib/ws';
  import { api } from '$lib/api';

  type Props = {
    isCompact: boolean;
    onNavigate?: () => void;
  };

  let { isCompact, onNavigate }: Props = $props();

  function navigate() {
    onNavigate?.();
  }

  async function signOut() {
    try {
      await api.authLogout();
    } catch {
      // network failure is tolerable — clearing local auth state is what
      // makes the next request bounce to the login flow regardless.
    }
    auth.clear();
  }
</script>

<!-- py-2 instead of py-3 — it's a meta surface, not a content one. -->
<div class="border-t border-surface1 {isCompact ? 'px-1.5 py-2 space-y-1' : 'px-2 py-2 space-y-1'}">
  <!-- Sabbath row. Mark 2:27. Split layout in expanded mode: the
       icon+label opens /sabbath (verse, time-remaining, schedule);
       the trailing pill toggles state in place. Compact mode
       collapses to a single icon-toggle. Auto-clears at midnight
       via a read-time check in the store. -->
  {#if isCompact}
    <button
      onclick={() => sabbath.toggle()}
      title={$sabbath ? 'Sabbath mode is on — tap to exit' : 'Enter sabbath mode (hides work modules for today)'}
      class="w-full flex justify-center items-center px-2 py-2 rounded text-sm transition-colors {$sabbath ? 'bg-success text-on-primary hover:opacity-90' : 'text-dim hover:bg-surface0 hover:text-text'}"
    >
      <svg viewBox="0 0 24 24" class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
        {#if $sabbath}
          <path d="M12 2l1.5 4.5L18 8l-4.5 1.5L12 14l-1.5-4.5L6 8l4.5-1.5L12 2zM12 14v8M9 22h6"/>
        {:else}
          <path d="M12 2l2 5h5l-4 3 1.5 5L12 12l-4.5 3L9 10 5 7h5z"/>
        {/if}
      </svg>
    </button>
  {:else}
    <div class="flex items-stretch gap-1 rounded {$sabbath ? 'bg-success text-on-primary' : ''}">
      <a
        href="/sabbath"
        onclick={navigate}
        class="flex-1 flex items-center gap-3 px-3 py-1 rounded-l transition-colors {$sabbath ? 'hover:opacity-90' : 'text-xs text-dim hover:bg-surface0 hover:text-subtext'}"
      >
        <svg viewBox="0 0 24 24" class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
          {#if $sabbath}
            <path d="M12 2l1.5 4.5L18 8l-4.5 1.5L12 14l-1.5-4.5L6 8l4.5-1.5L12 2zM12 14v8M9 22h6"/>
          {:else}
            <path d="M12 2l2 5h5l-4 3 1.5 5L12 12l-4.5 3L9 10 5 7h5z"/>
          {/if}
        </svg>
        <span class="flex-1 text-left">{$sabbath ? 'Sabbath on' : 'Sabbath'}</span>
      </a>
      <button
        onclick={() => sabbath.toggle()}
        title={$sabbath ? 'tap to exit sabbath' : 'enter sabbath now'}
        aria-label={$sabbath ? 'exit sabbath' : 'enter sabbath'}
        class="px-2 py-1 rounded-r transition-colors {$sabbath ? 'hover:opacity-90' : 'text-dim hover:bg-surface0 hover:text-subtext'}"
      >
        <span class="text-sm">{$sabbath ? '×' : '→'}</span>
      </button>
    </div>
  {/if}

  <!-- Utility strip: theme · compact-toggle · sign-out · live pip.
       Single horizontal strip in expanded mode (a meta utility bar,
       not a continuation of the nav). Compact mode stacks them
       vertically since one-per-row fits the narrow rail. -->
  {#if isCompact}
    <button
      onclick={() => theme.set(nextTheme($theme))}
      title={`Theme: ${themeLabel($theme)} — tap to cycle`}
      aria-label="Cycle theme"
      class="w-full flex justify-center items-center px-2 py-1.5 rounded text-dim hover:bg-surface0 hover:text-text transition-colors"
    >
      <svg viewBox="0 0 24 24" class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
        {#if $theme === 'dark'}
          <path d="M21 12.79A9 9 0 1 1 11.21 3a7 7 0 0 0 9.79 9.79z"/>
        {:else if $theme === 'light'}
          <circle cx="12" cy="12" r="4"/>
          <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41"/>
        {:else}
          <circle cx="12" cy="12" r="9"/>
          <path d="M12 3a9 9 0 0 0 0 18z" fill="currentColor"/>
        {/if}
      </svg>
    </button>
    <button
      onclick={toggleSidebarCompact}
      title="Expand sidebar"
      aria-label="Expand sidebar"
      class="hidden md:flex w-full justify-center items-center px-2 py-1.5 rounded text-dim hover:bg-surface0 hover:text-text transition-colors"
    >
      <svg viewBox="0 0 24 24" class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <polyline points="9 18 15 12 9 6" />
      </svg>
    </button>
    <div class="flex justify-center pt-0.5" title={$wsConnected ? 'live' : 'offline'}>
      <span class="w-2 h-2 rounded-full {$wsConnected ? 'bg-success' : 'bg-dim'}"></span>
    </div>
  {:else}
    <div class="flex items-center gap-1 px-1">
      <button
        onclick={() => theme.set(nextTheme($theme))}
        title={`Theme: ${themeLabel($theme)} — tap to cycle`}
        aria-label="Cycle theme"
        class="w-8 h-8 flex items-center justify-center rounded text-dim hover:bg-surface0 hover:text-text transition-colors"
      >
        <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
          {#if $theme === 'dark'}
            <path d="M21 12.79A9 9 0 1 1 11.21 3a7 7 0 0 0 9.79 9.79z"/>
          {:else if $theme === 'light'}
            <circle cx="12" cy="12" r="4"/>
            <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41"/>
          {:else}
            <circle cx="12" cy="12" r="9"/>
            <path d="M12 3a9 9 0 0 0 0 18z" fill="currentColor"/>
          {/if}
        </svg>
      </button>
      <button
        onclick={toggleSidebarCompact}
        title="Collapse to icons"
        aria-label="Collapse sidebar"
        class="hidden md:flex w-8 h-8 items-center justify-center rounded text-dim hover:bg-surface0 hover:text-text transition-colors"
      >
        <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="15 18 9 12 15 6" />
        </svg>
      </button>
      <span class="flex-1"></span>
      <button
        onclick={signOut}
        title="Sign out"
        class="text-[11px] text-dim hover:text-error transition-colors px-2 py-1"
      >sign out</button>
      <span
        class="ml-1 w-2 h-2 rounded-full flex-shrink-0 {$wsConnected ? 'bg-success' : 'bg-dim'}"
        title={$wsConnected ? 'live' : 'offline'}
        aria-label={$wsConnected ? 'live' : 'offline'}
      ></span>
    </div>
  {/if}
</div>
