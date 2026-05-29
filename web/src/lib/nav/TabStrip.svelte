<script lang="ts">
  // Multi-tab Phase 2 strip — sits above the main pane and renders
  // one pill per open tab. Hidden until the user explicitly opens a
  // second tab (so the single-route flow keeps the same vertical
  // budget it had before this feature shipped). Mobile gets nothing
  // for Phase 2 — the strip would compete with the bottom nav for
  // attention and the mobile UX wants narrower scope; Phase 3 will
  // revisit.
  //
  // Title resolution: longest-href nav match wins, otherwise we
  // format the path itself (last segment, %-decoded, slashes →
  // chevrons). The title is computed once at tab creation and
  // refreshed by the layout on navigation — recomputing here on
  // every render would mean Svelte holds a derivation per tab.

  import { goto } from '$app/navigation';
  import { tabsStore, activateTab, closeTab, newTab } from '$lib/stores/tabs';

  // Active tab id lives in the store, derived for highlighting.
  let activeId = $derived($tabsStore.activeTabId);
  let tabs = $derived($tabsStore.tabs);

  // Truncate at 14 chars + ellipsis so the strip stays scannable.
  // Single source so tab pills + the document title stay consistent
  // if the latter ever wants to mirror.
  function truncate(label: string): string {
    if (label.length <= 14) return label;
    return label.slice(0, 13) + '…';
  }

  // Click handler — middle-click on the pill closes the tab (browser
  // convention), normal click activates + navigates, modifier-click
  // is handled separately by the new-tab path in NavSidebar (the
  // strip itself can't open new URLs).
  async function onTabClick(e: MouseEvent, id: string) {
    if (e.button === 1) {
      // Middle-click → close
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
    // "+ new tab" opens a fresh dashboard slot. We use ensureTab
    // semantics via newTab so the user gets a NEW tab even if /
    // is already open in another tab — the affordance reads
    // "give me another workspace", not "switch to dashboard".
    const id = newTab('/', 'Today');
    if (id) goto('/');
  }

</script>

{#if tabs.length > 0}
  <!-- Hidden until at least one tab is open. md:flex so mobile
       keeps its existing single-pane flow (Phase 3 will revisit). -->
  <div
    class="hidden md:flex items-stretch gap-px bg-base border-b border-surface1 px-1 pt-1 overflow-x-auto flex-shrink-0"
    role="tablist"
    aria-label="Open tabs"
  >
    {#each tabs as tab (tab.id)}
      {@const active = tab.id === activeId}
      <div
        class="group relative flex items-center gap-1.5 px-2.5 py-1 text-xs rounded-t border-t border-l border-r min-w-[6rem] max-w-[14rem] cursor-pointer transition-colors {active
          ? 'bg-mantle text-text border-surface1 border-t-primary border-t-2 -mb-px'
          : 'bg-surface0 text-dim border-transparent hover:bg-surface1 hover:text-subtext'}"
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
        <span class="truncate flex-1">{truncate(tab.title)}</span>
        <!-- Close affordance: always visible on active, hover-only
             on inactive so the strip doesn't feel busy at idle. The
             button absorbs its own click so the surrounding tab
             handler doesn't also fire activate. -->
        <button
          type="button"
          aria-label="Close tab {tab.title}"
          title="Close tab"
          class="ml-1 -mr-1 w-4 h-4 flex items-center justify-center rounded transition-opacity hover:bg-surface2 hover:text-text {active ? 'opacity-70' : 'opacity-0 group-hover:opacity-70'}"
          onclick={(e) => {
            e.preventDefault();
            e.stopPropagation();
            closeTabAndNav(tab.id);
          }}
          onmousedown={(e) => e.stopPropagation()}
        >
          <svg viewBox="0 0 12 12" class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round">
            <path d="M3 3l6 6M9 3l-6 6" />
          </svg>
        </button>
      </div>
    {/each}
    <!-- New-tab affordance. Kept slim (icon-only) so the strip's
         visual rhythm reads as "one pill per tab, plus an add". -->
    <button
      type="button"
      onclick={onNewTab}
      title="New tab (Mod+T)"
      aria-label="New tab"
      class="ml-1 px-2 py-1 text-dim hover:text-text hover:bg-surface0 rounded-t text-xs transition-colors flex-shrink-0"
    >
      <svg viewBox="0 0 12 12" class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round">
        <path d="M6 2v8M2 6h8" />
      </svg>
    </button>
  </div>
{/if}
