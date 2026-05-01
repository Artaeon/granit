<script lang="ts">
  // Mobile-only top bar with hamburger and current view title. Hidden on md+.
  let { onMenu, title }: { onMenu: () => void; title: string } = $props();
</script>

<header class="md:hidden flex items-center gap-3 px-3 h-12 border-b border-surface1 bg-mantle/90 backdrop-blur sticky top-0 z-30">
  <button
    onclick={onMenu}
    aria-label="menu"
    class="w-10 h-10 -ml-2 flex items-center justify-center text-text hover:bg-surface0 rounded"
  >
    <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
      <path d="M4 6h16M4 12h16M4 18h16" stroke-linecap="round" />
    </svg>
  </button>
  <h1 class="text-base font-medium text-text">{title}</h1>
</header>
