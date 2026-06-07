<script lang="ts">
  // Desktop top bar. A slim always-present strip above the main pane:
  //   [ ⌘K Jump… ] | <current page / open tabs>            [ + ]
  // The ⌘K launcher makes the command palette the primary way to move
  // around (command-bar-first navigation); the tabs fold in to the same
  // strip so a single thin bar carries jump + context + tabs. Desktop
  // only (md:flex) — mobile keeps its own MobileTopBar + bottom nav.
  //
  // Title resolution for tabs: the title is computed at tab creation and
  // refreshed by the layout on navigation, so we just render tab.title.

  import { goto } from '$app/navigation';
  import { tabsStore, activateTab, closeTab, newTab } from '$lib/stores/tabs';
  import { activeNav } from '$lib/nav/active';

  let { onQuickJump }: { onQuickJump?: () => void } = $props();

  let activeId = $derived($tabsStore.activeTabId);
  let tabs = $derived($tabsStore.tabs);

  // Truncate at 14 chars + ellipsis so the strip stays scannable.
  function truncate(label: string): string {
    if (label.length <= 14) return label;
    return label.slice(0, 13) + '…';
  }

  // Click handler — middle-click closes (browser convention), normal
  // click activates + navigates.
  async function onTabClick(e: MouseEvent, id: string) {
    if (e.button === 1) {
      e.preventDefault();
      await closeTabAndNav(id);
      return;
    }
    if (e.button !== 0) return;
    const url = activateTab(id);
    if (url) goto(url);
  }

  async function closeTabAndNav(id: string) {
    const { nextUrl } = closeTab(id);
    if (nextUrl) await goto(nextUrl);
  }

  function onNewTab() {
    // Fresh Today dashboard slot. Targets /dashboard directly (NOT '/',
    // which redirects and would leave a stale url:'/' tab).
    const id = newTab('/dashboard', 'Today');
    if (id) goto('/dashboard');
  }
</script>

<div
  class="hidden md:flex items-center gap-1.5 bg-mantle border-b border-surface1 px-2 h-9 flex-shrink-0"
  aria-label="Top bar"
>
  <!-- ⌘K command launcher — primary navigation. Reads as a search box so
       click-first users discover it; the kbd hint trains the shortcut. -->
  <button
    type="button"
    onclick={onQuickJump}
    title="Jump to anything (⌘K)"
    aria-label="Jump to anything"
    class="flex items-center gap-2 px-2 py-1 rounded text-xs text-dim hover:bg-surface0 hover:text-text transition-colors flex-shrink-0"
  >
    <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
      <circle cx="11" cy="11" r="7" /><path d="m21 21-4.3-4.3" />
    </svg>
    <span class="hidden lg:inline">Jump to anything…</span>
    <kbd class="text-[10px] font-mono px-1 py-0.5 bg-surface0 border border-surface1 rounded">⌘K</kbd>
  </button>

  <div class="w-px h-4 bg-surface1 flex-shrink-0" aria-hidden="true"></div>

  {#if tabs.length > 1}
    <!-- Open tabs as pills. Active = filled; close on the pill or
         middle-click. Scrolls horizontally when many are open. -->
    <div class="flex items-center gap-1 overflow-x-auto min-w-0" role="tablist" aria-label="Open tabs">
      {#each tabs as tab (tab.id)}
        {@const active = tab.id === activeId}
        <div
          class="group relative flex items-center gap-1.5 pl-2.5 pr-1.5 h-7 rounded text-xs cursor-pointer transition-colors flex-shrink-0 {active
            ? 'bg-surface1 text-text'
            : 'text-dim hover:bg-surface0 hover:text-subtext'}"
          role="tab"
          aria-selected={active}
          tabindex="0"
          onmousedown={(e) => onTabClick(e, tab.id)}
          onkeydown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault();
              const url = activateTab(tab.id);
              if (url) goto(url);
            }
          }}
          title={tab.title + ' — ' + tab.url}
        >
          <span class="truncate max-w-[11rem]">{truncate(tab.title)}</span>
          <button
            type="button"
            aria-label="Close tab {tab.title}"
            title="Close tab"
            class="w-4 h-4 flex items-center justify-center rounded transition-opacity hover:bg-surface2 hover:text-text {active ? 'opacity-70' : 'opacity-0 group-hover:opacity-70'}"
            onclick={(e) => { e.preventDefault(); e.stopPropagation(); closeTabAndNav(tab.id); }}
            onmousedown={(e) => e.stopPropagation()}
          >
            <svg viewBox="0 0 12 12" class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round">
              <path d="M3 3l6 6M9 3l-6 6" />
            </svg>
          </button>
        </div>
      {/each}
    </div>
  {:else}
    <!-- Single tab → just show the current surface as context, no
         closeable pill (avoids the close-the-last-tab respawn). -->
    <span class="text-xs font-medium text-subtext truncate">{$activeNav?.label ?? ''}</span>
  {/if}

  <span class="flex-1"></span>

  <!-- New-tab affordance. -->
  <button
    type="button"
    onclick={onNewTab}
    title="New tab (⌘T)"
    aria-label="New tab"
    class="w-6 h-6 flex items-center justify-center rounded text-dim hover:text-text hover:bg-surface0 transition-colors flex-shrink-0"
  >
    <svg viewBox="0 0 12 12" class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round">
      <path d="M6 2v8M2 6h8" />
    </svg>
  </button>
</div>
