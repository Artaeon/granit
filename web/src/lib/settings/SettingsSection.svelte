<script lang="ts">
  // SettingsSection — heading + optional status pill + optional
  // "advanced ▾" collapse. Used by every tab. The advanced snippet
  // collapses behind a <details> so each tab fits one screen by
  // default with no information loss.
  import type { Snippet } from 'svelte';

  let {
    title,
    status,
    advancedLabel,
    children,
    advanced
  }: {
    title: string;
    status?: string;
    advancedLabel?: string;
    children: Snippet;
    advanced?: Snippet;
  } = $props();
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-3 mb-2.5">
  <header class="flex items-baseline justify-between mb-1">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">{title}</h2>
    {#if status}
      <span class="text-[10px] uppercase tracking-wider text-dim">{status}</span>
    {/if}
  </header>
  <div class="divide-surface1">
    {@render children()}
  </div>
  {#if advanced}
    <details class="mt-2 group">
      <summary class="text-[11px] text-dim hover:text-text cursor-pointer select-none list-none flex items-center gap-1">
        <span class="inline-block transition-transform group-open:rotate-90 text-[10px]">▸</span>
        {advancedLabel || 'Show advanced'}
      </summary>
      <div class="mt-2 pt-2 border-t border-surface1">
        {@render advanced()}
      </div>
    </details>
  {/if}
</section>
