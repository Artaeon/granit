<!--
  StatusIndicators — the right-hand cluster of the StatusBar:
  connectivity dot + AI ready dot. Reads its own stores, so the
  parent just renders <StatusIndicators /> without prop plumbing.

  Sabbath dims the AI dot to warning; offline dims the connectivity
  dot to error. Both expose hover tooltips with the underlying state.
-->
<script lang="ts">
  import { isOnline } from '$lib/stores/online';
  import { aiStatus } from '$lib/stores/ai-status';
  import { sabbath } from '$lib/stores/sabbath';
</script>

<div class="flex items-center gap-2 px-2 border-l border-surface1 flex-shrink-0">
  <span
    class="inline-flex items-center gap-1 text-[10px] text-dim"
    title={$isOnline ? 'Connected' : 'Offline — changes will sync when back online'}
  >
    <span
      class="w-1.5 h-1.5 rounded-full {$isOnline ? 'bg-success' : 'bg-error'}"
      aria-hidden="true"
    ></span>
    <span class="hidden md:inline">{$isOnline ? 'online' : 'offline'}</span>
  </span>
  {#if $aiStatus}
    <span
      class="inline-flex items-center gap-1 text-[10px] text-dim"
      title={$sabbath
        ? 'AI paused — Sabbath'
        : `AI ready — ${$aiStatus.global_model || $aiStatus.global_provider || 'default'}`}
    >
      <span
        class="w-1.5 h-1.5 rounded-full {$sabbath ? 'bg-warning' : 'bg-success'}"
        aria-hidden="true"
      ></span>
      <span class="hidden md:inline truncate max-w-[8rem]">
        {$sabbath ? 'sabbath' : ($aiStatus.global_model || 'ai')}
      </span>
    </span>
  {/if}
</div>
