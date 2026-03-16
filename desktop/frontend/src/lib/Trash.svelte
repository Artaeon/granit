<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  export let items: any[] = []
  const dispatch = createEventDispatcher()
</script>

<div class="fixed inset-0 z-50 flex justify-center pt-[10%]" style="background:rgba(0,0,0,0.5);backdrop-filter:blur(2px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-lg h-[60vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden">
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <span class="text-sm font-semibold text-ctp-text">Trash ({items.length} items)</span>
      <kbd class="text-[10px] text-ctp-overlay0 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer" on:click={() => dispatch('close')}>esc</kbd>
    </div>
    <div class="flex-1 overflow-y-auto py-1">
      {#each items as item}
        <div class="flex items-center justify-between px-4 py-2 hover:bg-ctp-surface0 group">
          <div class="flex-1 min-w-0">
            <div class="text-sm text-ctp-text truncate">{item.origPath}</div>
            <div class="text-[10px] text-ctp-overlay0">{item.timeAgo}</div>
          </div>
          <div class="flex gap-2 opacity-0 group-hover:opacity-100">
            <button on:click={() => dispatch('restore', item.trashFile)} class="text-xs text-ctp-green hover:underline">Restore</button>
            <button on:click={() => dispatch('purge', item.trashFile)} class="text-xs text-ctp-red hover:underline">Delete</button>
          </div>
        </div>
      {/each}
      {#if items.length === 0}
        <p class="text-center text-sm text-ctp-overlay0 py-8">Trash is empty</p>
      {/if}
    </div>
  </div>
</div>
