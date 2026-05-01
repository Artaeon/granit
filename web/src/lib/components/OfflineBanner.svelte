<script lang="ts">
  import { fly } from 'svelte/transition';
  import { isOnline, wasOffline } from '$lib/stores/online';
</script>

{#if !$isOnline}
  <div
    in:fly={{ y: -10, duration: 180 }}
    class="md:hidden fixed top-12 inset-x-0 z-30 px-3 py-1.5 bg-warning/15 border-b border-warning/30 text-warning text-xs flex items-center gap-2 backdrop-blur"
  >
    <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
      <path d="M2 2l20 20M8.5 16.5a5 5 0 0 1 7 0M5 12.55a11 11 0 0 1 14.08-1.4M1.42 9a16 16 0 0 1 4.5-3.07M9 19.5h.01" />
    </svg>
    <span>Offline — viewing cached data. Edits queue for retry.</span>
  </div>
  <div
    in:fly={{ x: 10, duration: 180 }}
    class="hidden md:flex fixed top-3 right-3 z-30 px-3 py-1.5 rounded-lg border bg-warning/15 border-warning/30 text-warning text-xs items-center gap-2 backdrop-blur"
  >
    <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
      <path d="M2 2l20 20M8.5 16.5a5 5 0 0 1 7 0M5 12.55a11 11 0 0 1 14.08-1.4M1.42 9a16 16 0 0 1 4.5-3.07" />
    </svg>
    <span>Offline</span>
  </div>
{:else if $wasOffline}
  <div
    in:fly={{ x: 10, duration: 180 }}
    class="fixed top-3 right-3 z-30 px-3 py-1.5 rounded-lg border bg-success/15 border-success/30 text-success text-xs flex items-center gap-2 backdrop-blur"
  >
    <span>✓ Back online</span>
  </div>
{/if}
