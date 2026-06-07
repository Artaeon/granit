<!--
  TasksPresetsBar — saved-filter-preset chip row that sits below the
  quick-add bar. One-click application of a stored filter combo;
  clicking an active user preset shows an inline × to delete. A "+
  save current" dashed chip captures the live filter state under a
  name.

  Starter presets render with a small "starter" label so the user
  knows they're the built-in set and will be replaced the moment
  they save their own. The row always renders — even with zero user
  presets the starter set is shown to surface the feature.
-->
<script lang="ts">
  import Button from '$lib/components/Button.svelte';
  import type { PresetsController } from './tasksPresets.svelte';

  type Props = {
    presetCtl: PresetsController;
  };

  let { presetCtl }: Props = $props();
</script>

<div class="px-3 py-1.5 border-b border-surface1 flex items-center gap-1.5 text-xs flex-shrink-0 flex-wrap">
  <span class="text-dim text-[11px] select-none">Presets</span>
  {#if presetCtl.isShowingStarters}
    <span class="text-[10px] text-dim italic" title="Built-in starter presets — save your own and these go away">starter</span>
  {/if}
  {#each presetCtl.visiblePresets as p (p.name)}
    {@const active = presetCtl.matches(p)}
    {@const isStarter = presetCtl.isShowingStarters}
    <span
      class="inline-flex items-center rounded overflow-hidden border
        {active ? 'border-primary bg-surface1 text-primary' : 'border-surface1 bg-surface0 text-subtext hover:border-primary'}"
    >
      <button
        onclick={() => presetCtl.apply(p)}
        class="px-2 py-0.5"
      >{p.name}</button>
      {#if active && !isStarter}
        <button
          onclick={() => presetCtl.remove(p.name)}
          title="Remove preset"
          class="px-1.5 py-0.5 text-dim hover:text-error border-l border-surface1"
        >×</button>
      {/if}
    </span>
  {/each}
  <Button
    variant="ghost"
    size="sm"
    onclick={() => presetCtl.capture()}
    title="Save the current filters as a named preset"
    class="border border-dashed border-surface1 hover:border-primary text-dim"
  >+ save current</Button>
</div>
