<script lang="ts">
  // Sabbath ribbon. Visible from every authed page so the state
  // is unmissable; the mode auto-clears at midnight via a read-time
  // check in the sabbath store. Click to exit. Z-index sits below
  // the running-timer pill so they don't clash.

  import { sabbath } from '$lib/stores/sabbath';
</script>

{#if $sabbath}
  <button
    type="button"
    onclick={() => sabbath.disable()}
    class="flex-shrink-0 px-4 py-1.5 bg-success text-on-primary text-xs text-center hover:opacity-90 transition-colors"
    title="Tap to exit sabbath mode"
  >
    Sabbath mode is on — work modules hidden until midnight. Tap to exit.
  </button>
{/if}
