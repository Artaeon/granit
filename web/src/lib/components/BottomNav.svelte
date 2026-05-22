<script lang="ts">
  import { page } from '$app/stores';
  import NavIcon from './NavIcon.svelte';

  interface Tab {
    href: string;
    label: string;
    icon: string;
  }

  let { onMore }: { onMore: () => void } = $props();

  // Four primary tabs + a More drawer button = five thumb columns,
  // the Apple/Material density target. The four primary tabs are
  // the four most-frequently-used surfaces — daily home, action
  // list, knowledge surface, scheduled time. Settings is a meta
  // destination and now lives in the More drawer (one tap deeper)
  // alongside everything else.
  const tabs: Tab[] = [
    { href: '/', label: 'Today', icon: 'today' },
    { href: '/tasks', label: 'Tasks', icon: 'tasks' },
    { href: '/notes', label: 'Notes', icon: 'notes' },
    { href: '/calendar', label: 'Calendar', icon: 'calendar' }
  ];

  function isActive(href: string): boolean {
    if (href === '/') return $page.url.pathname === '/';
    return $page.url.pathname === href || $page.url.pathname.startsWith(href + '/');
  }

  // "More" is highlighted when we're on a route not represented by the tabs.
  let moreActive = $derived.by(() => {
    const path = $page.url.pathname;
    if (tabs.some((t) => isActive(t.href))) return false;
    return true;
  });
</script>

<!-- Five-column thumb bar. Each cell renders as a flat, edge-to-
     edge box (no rounded pill behind the icon, no rounded button
     shape) to match the rest of the app's not-rounded design.
     Active state is a thin top accent rail + filled background +
     primary-tinted text — reads as a tab without the floating-pill
     look. Vertical divider lines between cells make the five
     columns scannable as discrete targets, matching the dense
     power-UI grammar of the rest of the app. -->
<!-- Mobile bottom-nav. `bottom-nav-hide-on-kb` is intentional: when
     the layout's visualViewport listener sets data-kb-open on <html>,
     the bar disappears so the on-screen keyboard doesn't sit on top
     of it and the user gets the whole screen for typing. Re-appears
     when the keyboard closes. -->
<nav
  aria-label="primary"
  class="bottom-nav-hide-on-kb md:hidden fixed bottom-0 inset-x-0 z-30 bg-mantle border-t border-surface1 pb-safe"
>
  <div class="flex items-stretch justify-around h-14">
    {#each tabs as t, i (t.href)}
      {@const active = isActive(t.href)}
      <a
        href={t.href}
        aria-current={active ? 'page' : undefined}
        aria-label={t.label}
        class="relative flex flex-col items-center justify-center flex-1 gap-0.5 transition-colors
          {i > 0 ? 'border-l border-surface1' : ''}
          {active ? 'text-primary bg-surface0' : 'text-dim active:bg-surface0 active:text-text'}"
      >
        {#if active}
          <!-- Top accent rail — 2px primary line flush against the
               top edge of the cell. Anchors the active state without
               the floating-pill effect. -->
          <span aria-hidden="true" class="absolute top-0 inset-x-0 h-0.5 bg-primary"></span>
        {/if}
        <NavIcon name={t.icon} class="w-5 h-5" />
        <span class="text-[10px] font-medium tracking-wide uppercase leading-none">{t.label}</span>
      </a>
    {/each}
    <button
      onclick={onMore}
      aria-label="more"
      aria-current={moreActive ? 'page' : undefined}
      class="relative flex flex-col items-center justify-center flex-1 gap-0.5 transition-colors border-l border-surface1
        {moreActive ? 'text-primary bg-surface0' : 'text-dim active:bg-surface0 active:text-text'}"
    >
      {#if moreActive}
        <span aria-hidden="true" class="absolute top-0 inset-x-0 h-0.5 bg-primary"></span>
      {/if}
      <NavIcon name="more" class="w-5 h-5" />
      <span class="text-[10px] font-medium tracking-wide uppercase leading-none">More</span>
    </button>
  </div>
</nav>

<style>
  /* Hide the mobile bottom-nav while the on-screen keyboard is open.
     +layout.svelte's visualViewport listener flips data-kb-open on
     <html> when the keyboard pushes content out of the visible
     viewport. Without this rule, the nav sits under the keyboard
     wasting the bottom strip of layout and visually competes with
     whatever floating action the OS surfaces above its keyboard. */
  :global(html[data-kb-open]) .bottom-nav-hide-on-kb {
    transform: translateY(110%);
    pointer-events: none;
  }
  .bottom-nav-hide-on-kb {
    transition: transform 180ms ease-out;
  }
</style>
