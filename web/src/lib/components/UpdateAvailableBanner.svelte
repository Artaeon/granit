<script lang="ts">
  // "New version available" banner. Sits ABOVE the bottom nav (z-40
  // vs bottom-nav's z-30) and uses md:bottom-3 on desktop so it
  // doesn't collide with the nav rail edge. Kept dismissable because
  // the user might want to finish a thought before we yank them —
  // but the action button reloads on tap so they don't have to hunt
  // for "clear cache".
  //
  // Self-wires to the service worker: when the SW posts
  // {type:'sw-updated'} after clients.claim(), a hidden tab reloads
  // immediately and a visible tab surfaces this banner. Pulled out
  // of +layout so the layout shell stops carrying SW lifecycle.

  import { onMount } from 'svelte';

  let updateAvailable = $state(false);

  onMount(() => {
    if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) return;
    const onMessage = (event: MessageEvent) => {
      if (event?.data?.type !== 'sw-updated') return;
      if (document.visibilityState === 'hidden') {
        location.reload();
      } else {
        updateAvailable = true;
      }
    };
    navigator.serviceWorker.addEventListener('message', onMessage);
    return () => navigator.serviceWorker.removeEventListener('message', onMessage);
  });
</script>

{#if updateAvailable}
  <div
    role="status"
    class="fixed inset-x-3 z-40 bottom-[calc(3.5rem+env(safe-area-inset-bottom,0px)+0.75rem)] md:bottom-3 md:left-auto md:right-3 md:max-w-sm bg-mantle border border-primary rounded-lg shadow-2xl p-3 flex items-center gap-3"
  >
    <div class="flex-1 min-w-0">
      <div class="text-sm font-medium text-text">Update available</div>
      <div class="text-xs text-dim mt-0.5">Reload to pick up the latest build.</div>
    </div>
    <button
      onclick={() => location.reload()}
      class="px-3 py-1.5 text-sm bg-primary text-on-primary rounded font-medium hover:opacity-90 flex-shrink-0"
    >Reload</button>
    <button
      onclick={() => (updateAvailable = false)}
      aria-label="dismiss"
      class="text-dim hover:text-text flex-shrink-0 px-1"
    >×</button>
  </div>
{/if}
