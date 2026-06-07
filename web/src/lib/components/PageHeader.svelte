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

<!-- @container so the header responds to its surface width, not the
     viewport: in a narrow workspace pane the title stacks above the
     actions (and shrinks) instead of cramming them side-by-side. -->
<div class="@container mb-4">
  <header class="flex flex-col @md:flex-row @md:items-baseline @md:justify-between gap-2">
    <div class="min-w-0">
      <h1 class="text-xl @md:text-3xl font-semibold text-text truncate">{title}</h1>
      {#if subtitle}
        <p class="text-sm text-dim mt-1">{subtitle}</p>
      {/if}
    </div>
    {#if actions}
      <div class="flex items-center gap-2 flex-wrap">{@render actions()}</div>
    {/if}
  </header>
</div>
