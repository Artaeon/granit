<script lang="ts">
  // Sidebar shell. Renders the brand, quick-jump button, Ask-AI
  // button, pinned + recent rails, Today, sections, and the footer
  // rail (settings, theme, sabbath, compact, sign out). Lives in
  // $lib/nav/ rather than $lib/components/ so the navigation
  // primitives (config, active, NavItem, NavSidebar) sit together.
  //
  // The earlier AI sub-rail (Mode pill + 3 quick-action chips) was
  // removed during the "professional" cleanup pass — the chips were
  // discovery scaffolding for the AI overlay that's now reachable
  // via the prominent Ask-AI button + ⌘J. Removing them buys ~50px
  // of rail above the section list, which the actual nav uses.
  //
  // Drives its own state from the sidebar-ui store (collapsed
  // sections, compact toggle) and the pinned / recent stores.
  // Sabbath, modules, and theme are read directly from their
  // stores. Parent passes:
  //   isCompact     — width mode (false in the mobile drawer)
  //   onNavigate    — called when a row click should also close
  //                   the mobile drawer; no-op on desktop
  //   onQuickJump   — opens the command palette (the palette
  //                   instance is bound in the layout)

  import Logo from '$lib/components/Logo.svelte';
  import NavIcon from '$lib/components/NavIcon.svelte';
  import NavItem from './NavItem.svelte';
  import {
    sections,
    today,
    settingsItem,
    nav,
    type NavItem as NavLink
  } from './config';
  import { sidebarPins } from '$lib/stores/sidebar-pins';
  import { sidebarRecent, MAX_RECENT } from '$lib/stores/sidebar-recent';
  import {
    collapsedSections,
    toggleSection,
    sidebarCompact,
    toggleSidebarCompact
  } from '$lib/stores/sidebar-ui';
  import { sabbath, SABBATH_HIDE_MODULES } from '$lib/stores/sabbath';
  import { modulesStore } from '$lib/stores/modules';
  import { theme, nextTheme, themeLabel } from '$lib/stores/theme';
  import { auth } from '$lib/stores/auth';
  import { wsConnected } from '$lib/ws';
  import { openAIOverlay } from '$lib/stores/ai-overlay';
  import { api } from '$lib/api';

  type Props = {
    isCompact: boolean;
    onNavigate?: () => void;
    onQuickJump: () => void;
  };

  let { isCompact, onNavigate, onQuickJump }: Props = $props();

  function navigate() {
    onNavigate?.();
  }

  // Pinned / recent rails resolve hrefs from the persisted stores
  // against the flat nav so a pin to a route that's been disabled
  // simply drops out instead of throwing. Module + sabbath gating
  // mirrors the section-body filters below.
  let pinnedItems = $derived.by(() => {
    void $modulesStore;
    void $sabbath;
    if ($sidebarPins.length === 0) return [] as NavLink[];
    const byHref = new Map(nav.map((n) => [n.href, n]));
    return $sidebarPins
      .map((h) => byHref.get(h))
      .filter((it): it is NavLink => {
        if (!it) return false;
        if (it.moduleId) {
          if (!modulesStore.isEnabled(it.moduleId)) return false;
          if ($sabbath && SABBATH_HIDE_MODULES.includes(it.moduleId)) return false;
        }
        return true;
      });
  });

  let recentItems = $derived.by(() => {
    void $modulesStore;
    void $sabbath;
    if ($sidebarRecent.length === 0) return [] as NavLink[];
    const pinned = new Set($sidebarPins);
    const byHref = new Map(nav.map((n) => [n.href, n]));
    return $sidebarRecent
      .map((h) => byHref.get(h))
      .filter((it): it is NavLink => {
        if (!it) return false;
        if (pinned.has(it.href)) return false;
        if (it.href === '/') return false; // Today is rendered separately
        if (it.moduleId) {
          if (!modulesStore.isEnabled(it.moduleId)) return false;
          if ($sabbath && SABBATH_HIDE_MODULES.includes(it.moduleId)) return false;
        }
        return true;
      })
      .slice(0, MAX_RECENT);
  });

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
</script>

<div class="flex flex-col h-full">
  <!-- Brand area collapses to icon-only in compact mode. The
       earlier "your vault, anywhere" tagline was visual noise on a
       surface the user looks at every minute; the wordmark + logo
       carry the brand without it. -->
  <div class="border-b border-surface1 {isCompact ? 'px-2 py-2.5 flex justify-center' : 'px-3 py-2.5'}">
    {#if isCompact}
      <div class="w-8 h-8 rounded bg-surface1 text-primary flex items-center justify-center" aria-label="Granit">
        <Logo class="w-4 h-4" label="" />
      </div>
    {:else}
      <div class="flex items-center gap-2">
        <div class="w-6 h-6 rounded bg-surface1 text-primary flex items-center justify-center flex-shrink-0">
          <Logo class="w-3.5 h-3.5" label="" />
        </div>
        <div class="text-sm font-semibold text-text">Granit</div>
      </div>
    {/if}
  </div>

  <nav class="flex-1 overflow-y-auto {isCompact ? 'px-1.5 py-2' : 'px-2 py-2'} space-y-0.5">
    <!-- Quick jump — visually subdued so the Ask-AI button below
         is the dominant primary affordance. ⌘K is the keyboard
         pattern; this row exists for click-discovery. -->
    <button
      onclick={() => { onQuickJump(); navigate(); }}
      title={isCompact ? 'Quick jump (⌘K)' : undefined}
      class="w-full flex items-center {isCompact ? 'justify-center px-2 py-1.5' : 'gap-3 px-3 py-1.5'} rounded text-xs text-dim hover:bg-surface0 hover:text-subtext transition-colors"
    >
      <NavIcon name="search" class="w-4 h-4 flex-shrink-0" />
      {#if !isCompact}
        <span class="flex-1 text-left">Quick jump</span>
        <kbd class="text-[10px] font-mono px-1.5 py-0.5 bg-surface0 border border-surface1 rounded">⌘K</kbd>
      {/if}
    </button>

    <!-- Ask AI — opens the global AI overlay. Subtle gradient
         border + sparkle icon distinguish it from regular nav so
         the user notices an "intelligence" surface without it
         dominating the rail. Mod+J also works from anywhere; this
         button is for discovery + click-first users.

         When Sabbath mode is on, AI calls are server-side gated; we
         dim the sparkle and surface a paused dot so the user
         understands the click will be silenced before they make it. -->
    <button
      onclick={() => { openAIOverlay(); navigate(); }}
      title={isCompact ? ($sabbath ? 'AI paused — Sabbath' : 'Ask AI (⌘J)') : undefined}
      class="w-full flex items-center {isCompact ? 'justify-center px-2 py-2' : 'gap-3 px-3 py-2'} rounded text-sm mt-1 mb-3 transition-colors {$sabbath ? 'bg-surface0 text-dim' : 'bg-primary text-on-primary hover:opacity-90 font-medium'}"
    >
      <span class="relative flex-shrink-0">
        <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
          <path d="M12 3v3M12 18v3M5.6 5.6l2.1 2.1M16.3 16.3l2.1 2.1M3 12h3M18 12h3M5.6 18.4l2.1-2.1M16.3 7.7l2.1-2.1"/>
          <circle cx="12" cy="12" r="3.5" fill="currentColor"/>
        </svg>
        {#if $sabbath}
          <span class="absolute -top-0.5 -right-0.5 w-2 h-2 rounded-full bg-warning" aria-hidden="true"></span>
        {/if}
      </span>
      {#if !isCompact}
        {#if $sabbath}
          <span class="flex-1 text-left">AI paused</span>
          <span class="text-[10px] text-warning font-medium">Sabbath</span>
        {:else}
          <span class="flex-1 text-left">Ask AI</span>
          <kbd class="text-[10px] font-mono px-1.5 py-0.5 rounded border border-on-primary text-on-primary opacity-70">⌘J</kbd>
        {/if}
      {/if}
    </button>

    <!-- Pinned items — user-curated rail above Today. Hidden when
         empty so first-time users don't see a phantom group. The
         pin star inside each NavItem is the only entry point;
         there's no separate manage screen because the action
         model is "see it in nav, hover to pin/unpin". In compact
         mode the items render without their group header (parity
         with the section dividers below). -->
    {#if pinnedItems.length > 0}
      {#if isCompact}
        {#each pinnedItems as item (item.href)}
          <NavItem {item} isCompact={true} onNavigate={navigate} />
        {/each}
        <div class="my-1.5 flex items-center justify-center" aria-hidden="true">
          <span class="w-1 h-1 rounded-full bg-surface1"></span>
        </div>
      {:else}
        <!-- Pinned + Recent + Sections share a single header style:
             tiny caps + lowercase, no extra border, no inline icon.
             Earlier the star/clock icons added noise without adding
             information that the label alone wasn't already giving. -->
        <div class="pb-1">
          <div class="px-3 pt-2 pb-0.5 text-[10px] uppercase tracking-wider text-dim">Pinned</div>
          <div class="space-y-0">
            {#each pinnedItems as item (item.href)}
              <NavItem {item} isCompact={false} onNavigate={navigate} />
            {/each}
          </div>
        </div>
      {/if}
    {/if}

    <!-- Recent items — surfaces what the user just touched so a
         re-entry into the app lands them back on the rhythm they
         had. Sits between Pinned (curated) and Today (home) so the
         top-of-rail mental model is: things I anchored, things I
         was just on, then home. Hidden when empty so first-time
         users don't see a phantom group. In compact mode renders
         without its header (parity with Pinned). -->
    {#if recentItems.length > 0}
      {#if isCompact}
        {#each recentItems as item (item.href)}
          <NavItem {item} isCompact={true} onNavigate={navigate} />
        {/each}
        <div class="my-1.5 flex items-center justify-center" aria-hidden="true">
          <span class="w-1 h-1 rounded-full bg-surface1"></span>
        </div>
      {:else}
        <div class="pb-1">
          <div class="px-3 pt-2 pb-0.5 text-[10px] uppercase tracking-wider text-dim">Recent</div>
          <div class="space-y-0">
            {#each recentItems as item (item.href)}
              <NavItem {item} isCompact={false} onNavigate={navigate} />
            {/each}
          </div>
        </div>
      {/if}
    {/if}

    <!-- Today sits above all groups, no header, since it's home. -->
    <NavItem item={today} {isCompact} showPinAction={false} onNavigate={navigate} />

    <!-- Sections. In compact mode the section header collapses to a
         thin separator line so the visual rhythm of grouping is
         preserved without the labels. -->
    {#each visibleSections as section}
      {@const isCollapsed = !!$collapsedSections[section.id] && !isCompact}
      {#if isCompact}
        <!-- Compact section divider: a short centered rule + a tiny
             pip so the visual rhythm of grouping survives icon-only
             mode without forcing the user to remember which icon
             belongs to which section. Title surfaces the section
             label on hover for orientation. -->
        <div
          class="my-2.5 flex items-center justify-center gap-1"
          aria-hidden="true"
          title={section.label}
        >
          <span class="h-px w-2 bg-surface1"></span>
          <span class="w-1 h-1 rounded-full bg-surface1"></span>
          <span class="h-px w-2 bg-surface1"></span>
        </div>
        {#each section.items as item}
          <NavItem {item} isCompact={true} onNavigate={navigate} />
        {/each}
      {:else}
        <div>
          <button
            type="button"
            onclick={() => toggleSection(section.id)}
            aria-expanded={!isCollapsed}
            class="w-full flex items-center gap-1 px-3 pt-2 pb-0.5 text-[10px] uppercase tracking-wider text-dim hover:text-subtext transition-colors"
          >
            <span class="flex-1 text-left">{section.label}</span>
            <svg viewBox="0 0 24 24" class="w-3 h-3 opacity-0 group-hover:opacity-100 transition-transform {isCollapsed ? '-rotate-90' : ''}" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="6 9 12 15 18 9" />
            </svg>
          </button>
          {#if !isCollapsed}
            <div class="space-y-0">
              {#each section.items as item}
                <NavItem {item} isCompact={false} onNavigate={navigate} />
              {/each}
            </div>
          {/if}
        </div>
      {/if}
    {/each}
  </nav>

  <!-- Footer rail. Settings, theme, compact toggle, sign out. -->
  <div class="border-t border-surface1 {isCompact ? 'px-1.5 py-2 space-y-1' : 'px-2 py-3 space-y-1'}">
    <NavItem item={settingsItem} {isCompact} onNavigate={navigate} />

    <button
      onclick={() => theme.set(nextTheme($theme))}
      title={isCompact ? `Theme: ${themeLabel($theme)} — tap to cycle` : undefined}
      class="w-full flex items-center {isCompact ? 'justify-center px-2 py-2' : 'gap-3 px-3 py-1.5'} rounded text-sm text-subtext hover:bg-surface0 hover:text-text transition-colors"
    >
      <svg viewBox="0 0 24 24" class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
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
      {#if !isCompact}
        <span class="flex-1 text-left">Theme: {themeLabel($theme)}</span>
        <span class="text-[10px] text-dim">cycle</span>
      {/if}
    </button>

    <!-- Sabbath row. Mark 2:27: "the sabbath was made for man." A
         split layout: the icon+label opens the /sabbath landing
         (verse, time-remaining, schedule); the "→" pill toggles
         sabbath state in place. Two distinct intents, one row.
         Compact mode collapses both into a single icon-button
         that just toggles, since hover-tooltips do most of the
         explaining and a side-by-side button row doesn't fit.
         Auto-clears at midnight via a read-time check in the
         store, so a forgotten 'on' state recovers the next
         morning by itself. -->
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
          class="flex-1 flex items-center gap-3 px-3 py-2 rounded-l transition-colors {$sabbath ? 'hover:opacity-90' : 'text-dim hover:bg-surface0 hover:text-text'}"
        >
          <svg viewBox="0 0 24 24" class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
            {#if $sabbath}
              <path d="M12 2l1.5 4.5L18 8l-4.5 1.5L12 14l-1.5-4.5L6 8l4.5-1.5L12 2zM12 14v8M9 22h6"/>
            {:else}
              <path d="M12 2l2 5h5l-4 3 1.5 5L12 12l-4.5 3L9 10 5 7h5z"/>
            {/if}
          </svg>
          <span class="flex-1 text-left">{$sabbath ? 'Sabbath on' : 'Sabbath'}</span>
          <span class="text-[10px] {$sabbath ? 'opacity-80' : 'text-dim'}">open</span>
        </a>
        <button
          onclick={() => sabbath.toggle()}
          title={$sabbath ? 'tap to exit sabbath' : 'enter sabbath now'}
          aria-label={$sabbath ? 'exit sabbath' : 'enter sabbath'}
          class="px-2.5 py-2 rounded-r transition-colors {$sabbath ? 'hover:opacity-90' : 'text-dim hover:bg-surface0 hover:text-text'}"
        >
          <span class="text-base">{$sabbath ? '×' : '→'}</span>
        </button>
      </div>
    {/if}

    <!-- Desktop-only compact toggle. Hidden on mobile because the
         drawer is already an icon-poor experience and a compact
         toggle in a temporary panel doesn't save anything. -->
    <button
      onclick={toggleSidebarCompact}
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
