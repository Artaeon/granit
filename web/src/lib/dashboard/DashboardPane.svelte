<!--
  DashboardPane — the Today widget grid, rendered as a workspace pane so
  the workspace can be a "canvas / desktop" that shows your widgets
  alongside any other surface you open.

  This is a read-only render of the enabled widgets (the canvas). Widget
  configuration (enable / disable / reorder / layouts / focus mode) still
  lives on the full home dashboard at `/` — the "Customize" link below
  jumps there. Each widget self-loads its own data via the lazy registry
  loader, exactly as on the home route.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type DashboardConfig, type VaultInfo } from '$lib/api';
  import { widgetMeta } from '$lib/dashboard/registry';
  import { sabbath, SABBATH_HIDE_WIDGET_TYPES } from '$lib/stores/sabbath';

  let config = $state<DashboardConfig | null>(null);
  let vault = $state<VaultInfo | null>(null);
  let loaded = $state(false);

  onMount(async () => {
    try {
      const [v, c] = await Promise.all([api.vault(), api.getDashboard()]);
      vault = v;
      config = c;
    } catch {
      // Leave config null — the empty-state hint covers it.
    } finally {
      loaded = true;
    }
  });

  // Enabled widgets that the client actually knows how to render
  // (widgetMeta resolves), minus anything hidden during Sabbath. Mirrors
  // the home route's activeWidgets, without the focus-mode subset.
  let activeWidgets = $derived.by(() => {
    if (!config) return [];
    return config.widgets
      .filter((w) => w.enabled)
      .filter((w) => !($sabbath && SABBATH_HIDE_WIDGET_TYPES.includes(w.type)))
      .map((w) => ({ widget: w, meta: widgetMeta(w.type) }))
      .filter((x): x is { widget: typeof x.widget; meta: NonNullable<typeof x.meta> } => !!x.meta);
  });
</script>

<!-- @container: the grid responds to the PANE width, not the viewport —
     so a narrow pane stacks to one column instead of squishing 3. -->
<div class="@container h-full overflow-y-auto p-3 sm:p-4 bg-base">
  {#if activeWidgets.length > 0}
    <div
      class="grid grid-cols-1 @md:grid-cols-2 @4xl:grid-cols-3 gap-2.5 sm:gap-3 items-start"
      style="grid-auto-flow: dense;"
    >
      {#each activeWidgets as { widget, meta } (widget.id)}
        <div class={meta.span === 2 ? '@md:col-span-2 @4xl:col-span-3 widget-cell' : 'widget-cell'}>
          {#await meta.load()}
            <div class="bg-surface0 border border-surface1 rounded-lg shadow-sm p-3 animate-pulse h-24"></div>
          {:then Widget}
            <Widget vaultPath={vault?.root ?? ''} />
          {:catch err}
            <div class="bg-surface0 border border-error text-error rounded-lg p-3 text-xs">
              Widget {meta.label} failed: {err?.message ?? err}
            </div>
          {/await}
        </div>
      {/each}
    </div>
  {:else if loaded}
    <div class="h-full flex flex-col items-center justify-center text-center gap-2 text-dim">
      <p class="text-sm">No widgets enabled.</p>
      <a href="/" class="text-xs text-primary hover:underline">Customize your dashboard →</a>
    </div>
  {/if}

  {#if activeWidgets.length > 0}
    <div class="mt-4 flex justify-end">
      <a href="/" class="text-[11px] text-dim hover:text-primary transition-colors">⚙ Customize dashboard</a>
    </div>
  {/if}
</div>

<style>
  /* Establish each cell as a CSS container so widgets that use @container
     queries adapt to their cell width (same as the home grid). */
  .widget-cell {
    container-type: inline-size;
  }
</style>
