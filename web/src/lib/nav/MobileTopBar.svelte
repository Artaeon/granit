<script lang="ts">
  // Mobile top bar — back (when in a subpage) · title · search.
  // Settings used to live here too but it's already in the More
  // drawer now, so the redundancy is gone. The bottom nav is the
  // primary nav surface; this top bar is just contextual.
  //
  // Visible only below md. Sticky to the top so the page title
  // and a "back to /<section>" affordance stay reachable while
  // scrolling. The back-arrow appears only on sub-routes whose
  // active nav resolves to a longer href than the current path —
  // it's a "scope up one level" affordance, not a history-back.

  import { page } from '$app/stores';
  import { activeNav } from '$lib/nav/active';

  type Props = {
    onQuickJump: () => void;
  };

  let { onQuickJump }: Props = $props();

  let title = $derived($activeNav?.label ?? 'everything');
  let showBack = $derived(
    !!$activeNav && $activeNav.href !== '/' && $page.url.pathname !== $activeNav.href
  );
</script>

<!-- Sticky header. `pt-safe` adds env(safe-area-inset-top) so on iOS
     PWA standalone the bar shifts below the notch / status bar
     instead of sitting half-under it. Browser tabs without an
     inset see no change. -->
<header
  class="md:hidden flex items-center gap-1 px-3 h-12 border-b border-surface1 bg-mantle sticky top-0 z-30 flex-shrink-0"
  style="padding-top: env(safe-area-inset-top, 0px); height: calc(3rem + env(safe-area-inset-top, 0px));"
>
  {#if showBack && $activeNav}
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
    onclick={onQuickJump}
    aria-label="search"
    class="w-10 h-10 flex items-center justify-center text-subtext hover:text-primary rounded"
  >
    <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
      <circle cx="11" cy="11" r="8" /><path d="m21 21-4.3-4.3" stroke-linecap="round" />
    </svg>
  </button>
</header>
