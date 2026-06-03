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
  import NavFooter from './NavFooter.svelte';
  import ProfileSwitcher from './ProfileSwitcher.svelte';
  import {
    sections,
    today,
    essentials,
    nav,
    type NavItem as NavLink
  } from './config';
  import { sidebarPins } from '$lib/stores/sidebar-pins';
  import {
    collapsedSections,
    toggleSection,
    hiddenSections
  } from '$lib/stores/sidebar-ui';
  import { sabbath, SABBATH_HIDE_MODULES } from '$lib/stores/sabbath';
  import { modulesStore } from '$lib/stores/modules';
  import { openAIOverlay } from '$lib/stores/ai-overlay';
  import { aiStatus } from '$lib/stores/ai-status';
  import { rightPaneStore, toggleRightPane } from '$lib/stores/rightPane';

  type Props = {
    isCompact: boolean;
    onNavigate?: () => void;
    onQuickJump: () => void;
  };

  let { isCompact, onNavigate, onQuickJump }: Props = $props();

  function navigate() {
    onNavigate?.();
  }

  // Pinned rail resolves hrefs from the persisted store against the
  // flat nav so a pin to a route that's been disabled simply drops
  // out instead of throwing. Module + sabbath gating mirrors the
  // section-body filters below.
  //
  // Items that already live in the Essentials tier are filtered out
  // here too — pinning Tasks (an essential) used to render it in
  // both Pinned AND Essentials, which is just visual duplication.
  // Pre-existing pins on essentials silently dedup; the user's
  // intent ("keep this visible") is already satisfied by the
  // higher-tier surface.
  // Set of hrefs that live in a hidden section — pre-computed once
  // per hiddenSections change so pinnedItems can match in O(1).
  let hiddenHrefs = $derived.by(() => {
    const out = new Set<string>();
    for (const s of sections) {
      if (!$hiddenSections[s.id]) continue;
      for (const it of s.items) out.add(it.href);
    }
    return out;
  });

  let pinnedItems = $derived.by(() => {
    void $modulesStore;
    void $sabbath;
    void hiddenHrefs;
    if ($sidebarPins.length === 0) return [] as NavLink[];
    const byHref = new Map(nav.map((n) => [n.href, n]));
    const essentialHrefs = new Set(essentials.map((e) => e.href));
    return $sidebarPins
      .map((h) => byHref.get(h))
      .filter((it): it is NavLink => {
        if (!it) return false;
        if (essentialHrefs.has(it.href)) return false; // already in Essentials
        // Respect Settings → Sidebar Views: if the user has hidden
        // a whole section, a pre-existing pin to one of its routes
        // also disappears from the Pinned rail. The escape hatch for
        // "I don't want AI in my nav at all" was leaking through pins.
        if (hiddenHrefs.has(it.href)) return false;
        if (it.moduleId) {
          if (!modulesStore.isEnabled(it.moduleId)) return false;
          if ($sabbath && SABBATH_HIDE_MODULES.includes(it.moduleId)) return false;
        }
        return true;
      });
  });

  // Essentials — Tier-1 items above the sections. Filters with the
  // same module + sabbath gating as the sections themselves so a
  // disabled module drops its entry from this rail too. Skipped
  // entries are silent (no placeholder); essentials is a curated
  // short-list, not a contract.
  let essentialItems = $derived.by(() => {
    void $modulesStore;
    void $sabbath;
    return essentials.filter((it) => {
      if (it.moduleId) {
        if (!modulesStore.isEnabled(it.moduleId)) return false;
        if ($sabbath && SABBATH_HIDE_MODULES.includes(it.moduleId)) return false;
      }
      return true;
    });
  });

  let visibleSections = $derived.by(() => {
    void $modulesStore;
    void $sabbath;
    void $hiddenSections;
    return sections
      // Drop the whole section if the user hid it via Settings →
      // Sidebar Views. Runs before item filtering so we don't even
      // touch the children when the parent is invisible.
      .filter((s) => !$hiddenSections[s.id])
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
  <!-- Brand area + profile chip + settings shortcut. The earlier
       layout buried profile + settings in the meta footer at the
       bottom of the rail; user feedback (2026-05-25) wanted them
       moved up since active workflow context belongs at the same
       altitude as app identity, and settings is reached often
       enough that a top-row icon beats a footer row. Compact mode
       stacks: logo → profile chip → settings icon. -->
  <!-- Brand row. min-w-0 + overflow-hidden on the parent flex lets
       shrinkable children (ProfileSwitcher chip text) collapse
       below their natural width instead of pushing fixed icons
       past the sidebar's right edge. The "Granit" wordmark dropped
       in expanded mode — the logo + the aria-label carry the brand
       identity and the wordmark was the item with the most cost
       for the least value. See feedback 2026-05-29. -->
  <div class="border-b border-surface1 min-w-0 overflow-hidden {isCompact ? 'px-2 py-2 flex flex-col items-center gap-1.5' : 'px-3 py-2.5 flex items-center gap-1.5'}">
    {#if isCompact}
      <div class="w-8 h-8 rounded bg-surface1 text-primary flex items-center justify-center" aria-label="Granit">
        <Logo class="w-4 h-4" label="" />
      </div>
      <button
        onclick={toggleRightPane}
        title={$rightPaneStore.open ? 'Close right pane (⌘\\)' : 'Open right pane (⌘\\)'}
        aria-label="Toggle right pane"
        aria-pressed={$rightPaneStore.open}
        class="w-7 h-7 flex items-center justify-center rounded transition-colors {$rightPaneStore.open ? 'bg-surface1 text-primary' : 'text-dim hover:text-text hover:bg-surface0'}"
      >
        <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
          <rect x="3" y="4" width="18" height="16" rx="2"/>
          <line x1="15" y1="4" x2="15" y2="20"/>
        </svg>
      </button>
      <ProfileSwitcher isCompact={true} />
      <a
        href="/settings"
        onclick={navigate}
        title="Settings"
        aria-label="Settings"
        class="w-7 h-7 flex items-center justify-center rounded text-dim hover:text-text hover:bg-surface0 transition-colors"
      >
        <NavIcon name="settings" class="w-4 h-4" />
      </a>
    {:else}
      <div class="w-6 h-6 rounded bg-surface1 text-primary flex items-center justify-center flex-shrink-0" aria-label="Granit">
        <Logo class="w-3.5 h-3.5" label="" />
      </div>
      <button
        onclick={toggleRightPane}
        title={$rightPaneStore.open ? 'Close right pane (⌘\\)' : 'Open right pane (⌘\\)'}
        aria-label="Toggle right pane"
        aria-pressed={$rightPaneStore.open}
        class="w-7 h-7 flex items-center justify-center rounded transition-colors flex-shrink-0 {$rightPaneStore.open ? 'bg-surface1 text-primary' : 'text-dim hover:text-text hover:bg-surface0'}"
      >
        <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
          <rect x="3" y="4" width="18" height="16" rx="2"/>
          <line x1="15" y1="4" x2="15" y2="20"/>
        </svg>
      </button>
      <div class="flex-1 min-w-0 flex justify-end">
        <ProfileSwitcher isCompact={false} />
      </div>
      <a
        href="/settings"
        onclick={navigate}
        title="Settings"
        aria-label="Settings"
        class="w-7 h-7 flex items-center justify-center rounded text-dim hover:text-text hover:bg-surface0 transition-colors flex-shrink-0"
      >
        <NavIcon name="settings" class="w-4 h-4" />
      </a>
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

    {#if !isCompact && !$sabbath && $aiStatus}
      <!-- AI model indicator — tiny pill below the Ask AI button
           showing the active provider's model so the user knows
           which backend the next click will pay for. Reads from
           the shared aiStatus store ($lib/stores/ai-status) so
           settings changes propagate without a sidebar refetch.
           Hidden during Sabbath (already gates the AI button) and
           in compact mode (the icon-only rail has no room for a
           model name). Click → /settings/ai for provider switch. -->
      <a
        href="/settings"
        onclick={navigate}
        class="-mt-2 mb-3 px-3 py-1 text-[10px] font-mono text-dim hover:text-subtext transition-colors flex items-center gap-1.5 min-w-0"
        title="Default AI backend — click to change in Settings"
      >
        <span class="w-1.5 h-1.5 rounded-full bg-success flex-shrink-0" aria-hidden="true"></span>
        <span class="truncate">{$aiStatus.global_model || $aiStatus.global_provider || 'AI ready'}</span>
      </a>
    {/if}

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

    <!-- Today + daily-core essentials. No section header (they ARE
         the headline). Position alone signals priority — earlier
         we also bumped the text size, but that made Tasks/Notes
         feel oversized next to the section items below. A small
         visual separator drops below the group to mark the
         transition into the section list. Pin action suppressed:
         they're already top-tier, pinning would just duplicate. -->
    <NavItem
      item={today}
      {isCompact}
      showPinAction={false}
      onNavigate={navigate}
    />
    {#each essentialItems as item (item.href)}
      <NavItem
        {item}
        {isCompact}
        showPinAction={false}
        onNavigate={navigate}
      />
    {/each}
    {#if !isCompact && essentialItems.length > 0}
      <div class="my-2 mx-3 h-px bg-surface1" aria-hidden="true"></div>
    {/if}
    {#if isCompact && essentialItems.length > 0}
      <div class="my-2 flex items-center justify-center gap-1" aria-hidden="true">
        <span class="h-px w-3 bg-surface1"></span>
      </div>
    {/if}

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
        <div class="mt-1">
          <!-- Section header: now a real-button-shaped affordance with
               an always-visible chevron + item count. Previously the
               chevron was opacity-0 group-hover:opacity-100 — invisible
               unless the user happened to mouse over the row, which
               failed the discoverability test ("can you tell sections
               collapse without being told?"). The header is also
               taller (py-1.5) and the label larger (text-[11px]
               font-medium) so layer-2 reads as a distinct tier of
               navigation, not as decoration. -->
          <button
            type="button"
            onclick={() => toggleSection(section.id)}
            aria-expanded={!isCollapsed}
            class="w-full flex items-center gap-1.5 px-2.5 py-1.5 text-[11px] uppercase tracking-wider font-medium text-dim hover:text-subtext hover:bg-surface0 rounded transition-colors"
          >
            <svg
              viewBox="0 0 24 24"
              class="w-3 h-3 flex-shrink-0 transition-transform {isCollapsed ? '-rotate-90' : ''}"
              fill="none"
              stroke="currentColor"
              stroke-width="2.5"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <polyline points="6 9 12 15 18 9" />
            </svg>
            <span class="flex-1 text-left">{section.label}</span>
            <span class="text-[10px] text-dim/70 tabular-nums normal-case tracking-normal" aria-hidden="true">{section.items.length}</span>
          </button>
          {#if !isCollapsed}
            <!-- Layer-2 items indented relative to the header so the
                 nesting reads at a glance, not just from the chevron
                 state. -->
            <div class="space-y-0 ml-2 mt-0.5 border-l border-surface1 pl-1">
              {#each section.items as item}
                <NavItem {item} isCompact={false} onNavigate={navigate} />
              {/each}
            </div>
          {/if}
        </div>
      {/if}
    {/each}
  </nav>

  <NavFooter {isCompact} onNavigate={navigate} />
</div>
