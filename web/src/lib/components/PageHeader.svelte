<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    title,
    subtitle,
    actions
  }: {
    title: string;
    subtitle?: string;
    actions?: Snippet;
  } = $props();
</script>

<header class="mb-6 flex flex-col sm:flex-row sm:items-baseline sm:justify-between gap-2">
  <div class="min-w-0">
    <h1 class="text-2xl sm:text-3xl font-semibold text-text">{title}</h1>
    {#if subtitle}
      <p class="text-sm text-dim mt-1">{subtitle}</p>
    {/if}
  </div>
  {#if actions}
    <div class="flex items-center gap-2 flex-wrap">{@render actions()}</div>
  {/if}
</header>
