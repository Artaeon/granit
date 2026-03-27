<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  export let items: any[] = []
  const dispatch = createEventDispatcher()

  const colors = ['', 'var(--ctp-mauve)', 'var(--ctp-blue)', 'var(--ctp-sapphire)', 'var(--ctp-teal)', 'var(--ctp-green)', 'var(--ctp-yellow)']
</script>

<div class="fixed inset-0 z-50 flex justify-center pt-[10%]" style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-md h-[60vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden">
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <span class="text-sm font-semibold text-ctp-text">Outline</span>
      <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer" on:click={() => dispatch('close')}>esc</kbd>
    </div>
    <div class="flex-1 overflow-y-auto py-2">
      {#each items as item}
        <button on:click={() => dispatch('jump', item.line)}
          class="w-full flex items-center gap-2 px-4 py-1.5 hover:bg-ctp-surface0 text-left transition-colors"
          style="padding-left: {16 + (item.level - 1) * 16}px">
          <span class="text-[12px] text-ctp-overlay1 w-8 text-right font-mono">L{item.line + 1}</span>
          <span class="text-sm truncate" style="color: {colors[item.level] || 'var(--ctp-text)'}; font-weight: {item.level <= 2 ? '600' : '400'}">
            {item.level <= 2 ? '●' : '○'} {item.text}
          </span>
        </button>
      {/each}
      {#if items.length === 0}
        <p class="text-center text-sm text-ctp-overlay1 py-8">No headings found</p>
      {/if}
    </div>
    <div class="px-4 py-2 border-t border-ctp-surface0 text-[12px] text-ctp-overlay1">Click heading to jump</div>
  </div>
</div>
