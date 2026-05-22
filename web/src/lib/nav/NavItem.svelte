<script lang="ts">
  // A single sidebar nav entry. Renders the icon + label, optional
  // badge (overdue tasks, today's events), and pin toggle. Pulled
  // out of +layout.svelte so the shell stays focused on composition
  // and the per-item render gets a tested seam.
  //
  // Compact mode collapses the row to icon-only with a corner badge
  // overlay; expanded mode shows label + count chip + hover-revealed
  // pin star. The pin action is suppressed for Today and Settings
  // (the always-on home + meta destinations).

  import NavIcon from '$lib/components/NavIcon.svelte';
  import { overdueTaskCount, todayEventCount } from '$lib/stores/nav-badges';
  import { sidebarPins, togglePin } from '$lib/stores/sidebar-pins';
  import { activeNav } from '$lib/nav/active';
  import type { NavItem } from '$lib/nav/config';

  type Props = {
    item: NavItem;
    isCompact: boolean;
    showPinAction?: boolean;
    /** Visual tier. 'essential' renders bigger + bolder for the
     *  Tier-1 rail above the sections (Today + daily-core). 'default'
     *  is the normal section-item style. */
    tier?: 'essential' | 'default';
    onNavigate?: () => void;
  };

  let {
    item,
    isCompact,
    showPinAction = true,
    tier = 'default',
    onNavigate
  }: Props = $props();

  let active = $derived($activeNav?.href === item.href);
  let badge = $derived(
    item.href === '/tasks' && $overdueTaskCount > 0
      ? { count: $overdueTaskCount, tone: 'error' as const, label: `${$overdueTaskCount} overdue` }
      : item.href === '/calendar' && $todayEventCount > 0
        ? { count: $todayEventCount, tone: 'subtle' as const, label: `${$todayEventCount} today` }
        : null
  );
  let pinned = $derived($sidebarPins.includes(item.href));
  let canPin = $derived(
    showPinAction && item.href !== '/' && item.href !== '/settings' && !isCompact
  );
</script>

<a
  href={item.href}
  onclick={() => onNavigate?.()}
  title={isCompact ? (badge ? `${item.label} — ${badge.label}` : item.label) : undefined}
  aria-label={badge ? `${item.label}, ${badge.label}` : item.label}
  class="group relative flex items-center {isCompact ? 'justify-center px-2 py-2' : 'gap-3 px-3'} rounded transition-colors
    {tier === 'essential' && !isCompact ? 'py-2 text-[15px]' : 'py-1.5 text-sm'}
    {active
      ? tier === 'essential'
        ? 'text-primary bg-surface1 font-semibold'
        : 'text-primary bg-surface1 font-medium'
      : tier === 'essential'
        ? 'text-text font-medium hover:bg-surface0 focus-visible:bg-surface0 focus-visible:outline-none'
        : 'text-subtext hover:bg-surface0 hover:text-text focus-visible:bg-surface0 focus-visible:text-text focus-visible:outline-none'}"
>
  <!-- Active rail: a 3px accent strip on the left edge replaces
       the heavier full-row fill, so scanning down the sidebar
       lands on the active item without the eye getting pulled.
       3px reads cleanly on every density / DPR — 2px disappeared
       under blur on some displays. -->
  {#if active}
    <span class="absolute left-0 top-1.5 bottom-1.5 w-[3px] rounded-full bg-primary" aria-hidden="true"></span>
  {/if}
  <span class="relative flex-shrink-0">
    <!-- Essential-tier icons render a hair bigger so the row's
         vertical rhythm matches its heavier text size. Default
         tier sticks with 20px (w-5) which has been the rail's
         baseline forever. -->
    <NavIcon name={item.icon} class="{tier === 'essential' && !isCompact ? 'w-[22px] h-[22px]' : 'w-5 h-5'} flex-shrink-0" />
    {#if isCompact && badge}
      <!-- Compact-mode badge sits as a corner overlay on the icon
           so the rail can still surface alerts without labels. The
           error tone shows the digit; the subtle tone collapses to
           a dot since the count is informational, not urgent. -->
      {#if badge.tone === 'error'}
        <span
          class="absolute -top-1.5 -right-1.5 min-w-[16px] h-4 px-1 rounded-full bg-error text-on-primary text-[9px] font-bold leading-4 text-center"
          aria-hidden="true"
        >{badge.count > 9 ? '9+' : badge.count}</span>
      {:else}
        <span class="absolute -top-0.5 -right-0.5 w-1.5 h-1.5 rounded-full bg-primary" aria-hidden="true"></span>
      {/if}
    {/if}
  </span>
  {#if !isCompact}
    <span class="truncate flex-1">{item.label}</span>
    {#if badge}
      {#if badge.tone === 'error'}
        <span
          class="ml-auto inline-flex items-center justify-center min-w-[18px] h-[18px] px-1.5 rounded-full bg-error text-on-primary text-[10px] font-semibold leading-none"
          aria-hidden="true"
        >{badge.count > 99 ? '99+' : badge.count}</span>
      {:else}
        <span
          class="ml-auto inline-flex items-center justify-center min-w-[18px] h-[18px] px-1.5 rounded-full bg-surface1 text-subtext text-[10px] font-medium leading-none"
          aria-hidden="true"
        >{badge.count > 99 ? '99+' : badge.count}</span>
      {/if}
    {/if}
    {#if canPin}
      <!-- Pin toggle. Always visible when pinned (so the user can
           unpin without hovering); revealed on hover otherwise. The
           button uses stopPropagation so clicking the star doesn't
           also navigate. Reads as a star to match the universal pin
           metaphor. -->
      <button
        type="button"
        onclick={(e) => { e.preventDefault(); e.stopPropagation(); togglePin(item.href); }}
        title={pinned ? 'unpin from sidebar top' : 'pin to sidebar top'}
        aria-label={pinned ? `Unpin ${item.label}` : `Pin ${item.label}`}
        class="ml-1 p-0.5 rounded transition-opacity {pinned ? 'text-warning opacity-100' : 'text-dim opacity-0 group-hover:opacity-70 hover:!opacity-100 hover:text-warning'}"
      >
        <svg viewBox="0 0 16 16" class="w-3.5 h-3.5" fill={pinned ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="1.4" stroke-linejoin="round">
          <path d="M8 1.5l1.85 4.05L14 6.2l-3.1 2.85L11.7 13 8 10.85 4.3 13l.8-3.95L2 6.2l4.15-.65z"/>
        </svg>
      </button>
    {/if}
  {/if}
</a>
