<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'

  export let x = 0
  export let y = 0
  export let items: { label: string; action: string; icon?: string; shortcut?: string; danger?: boolean; separator?: boolean }[] = []

  const dispatch = createEventDispatcher()
  let menuEl: HTMLDivElement

  onMount(() => {
    // Adjust position if menu would go off-screen
    if (menuEl) {
      const rect = menuEl.getBoundingClientRect()
      if (rect.right > window.innerWidth) x = window.innerWidth - rect.width - 8
      if (rect.bottom > window.innerHeight) y = window.innerHeight - rect.height - 8
    }
    // Close on any click outside
    const handler = () => dispatch('close')
    setTimeout(() => window.addEventListener('click', handler), 10)
    return () => window.removeEventListener('click', handler)
  })

  const iconPaths: Record<string, string> = {
    rename: 'M11 1l4 4L5 15H1v-4L11 1z',
    delete: 'M3 4h10l-1 10H4L3 4zM6 2h4M2 4h12',
    move: 'M5 3v10M11 3v10M1 8h14',
    copy: 'M5 3h8v10H5zM3 5v10h8',
    folder: 'M2 5h5l1.5-2H13a1 1 0 0 1 1 1v7a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V5z',
    open: 'M3 2h10v12H3zm2 3h6m-6 3h4',
    newtab: 'M8 2v12M2 8h12',
    star: 'M8 1l2.2 4.5 5 .7-3.6 3.5.9 5L8 12.4 3.5 14.7l.9-5L.8 6.2l5-.7z',
    link: 'M7 3H4a3 3 0 0 0 0 6h3M9 3h3a3 3 0 0 1 0 6H9M5 8h6',
  }
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div bind:this={menuEl} class="fixed z-[70]" style="left:{x}px;top:{y}px" on:click|stopPropagation>
  <div class="bg-ctp-surface0 rounded-lg border border-ctp-surface1 shadow-overlay py-1 min-w-[180px]"
    style="animation: modalSlideIn 120ms cubic-bezier(0.16, 1, 0.3, 1)">
    {#each items as item}
      {#if item.separator}
        <div class="h-px bg-ctp-surface1 my-1 mx-2"></div>
      {:else}
        <button on:click={() => { dispatch('action', item.action); dispatch('close') }}
          class="w-full flex items-center gap-2.5 px-3 py-2 text-[13px] text-left transition-colors rounded-md mx-0.5"
          class:text-ctp-red={item.danger}
          class:hover:bg-ctp-red={item.danger}
          class:hover:bg-opacity-15={item.danger}
          class:text-ctp-text={!item.danger}
          class:hover:bg-ctp-surface1={!item.danger}
          class:hover:text-ctp-text={true}
          style="width: calc(100% - 4px)">
          {#if item.icon && iconPaths[item.icon]}
            <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" class="flex-shrink-0 opacity-80">
              <path d="{iconPaths[item.icon]}" />
            </svg>
          {:else}
            <span class="w-[14px]"></span>
          {/if}
          <span class="flex-1 font-medium">{item.label}</span>
          {#if item.shortcut}
            <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-mantle px-1.5 py-0.5 rounded">{item.shortcut}</kbd>
          {/if}
        </button>
      {/if}
    {/each}
  </div>
</div>
