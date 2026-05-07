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

<nav
  aria-label="primary"
  class="md:hidden fixed bottom-0 inset-x-0 z-30 bg-mantle/95 backdrop-blur border-t border-surface1 pb-safe"
>
  <div class="flex items-stretch justify-around h-14">
    {#each tabs as t (t.href)}
      {@const active = isActive(t.href)}
      <a
        href={t.href}
        aria-current={active ? 'page' : undefined}
        aria-label={t.label}
        class="flex flex-col items-center justify-center flex-1 gap-0.5 transition-colors
          {active ? 'text-primary' : 'text-dim active:text-text'}"
      >
        <!-- Active state: a small pill behind the icon, not a
             full-row fill. Reads as the active tab without
             visually crowding the rest of the row. -->
        <span
          class="w-10 h-6 rounded-full flex items-center justify-center transition-colors
            {active ? 'bg-primary/15' : ''}"
        >
          <NavIcon name={t.icon} class="w-5 h-5" />
        </span>
        <span class="text-[10px] font-medium">{t.label}</span>
      </a>
    {/each}
    <button
      onclick={onMore}
      aria-label="more"
      aria-current={moreActive ? 'page' : undefined}
      class="flex flex-col items-center justify-center flex-1 gap-0.5 transition-colors
        {moreActive ? 'text-primary' : 'text-dim active:text-text'}"
    >
      <span
        class="w-10 h-6 rounded-full flex items-center justify-center transition-colors
          {moreActive ? 'bg-primary/15' : ''}"
      >
        <NavIcon name="more" class="w-5 h-5" />
      </span>
      <span class="text-[10px] font-medium">More</span>
    </button>
  </div>
</nav>
