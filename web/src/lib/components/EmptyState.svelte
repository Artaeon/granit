<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    icon,
    title,
    description,
    action
  }: {
    icon?: string;
    title: string;
    description?: string;
    action?: Snippet;
  } = $props();
</script>

<div class="text-center py-10 px-4">
  {#if icon}
    <div class="text-4xl mb-3 opacity-60">{icon}</div>
  {/if}
  <div class="text-base font-medium text-text">{title}</div>
  {#if description}
    <p class="text-sm text-dim mt-2 max-w-sm mx-auto leading-relaxed">{description}</p>
  {/if}
  {#if action}
    <div class="mt-4 flex justify-center">{@render action()}</div>
  {/if}
</div>
