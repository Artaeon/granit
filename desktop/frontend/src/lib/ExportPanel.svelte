<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  export let notePath: string = ''
  export let message: string = ''
  const dispatch = createEventDispatcher()

  const formats = [
    { id: 'html', name: 'HTML', desc: 'Convert to HTML file' },
    { id: 'text', name: 'Plain Text', desc: 'Strip formatting to .txt' },
    { id: 'pdf', name: 'PDF', desc: 'Export via pandoc (if available)' },
    { id: 'all', name: 'Export All as HTML', desc: 'Export entire vault with index' },
  ]
</script>

<div class="fixed inset-0 z-50 flex justify-center pt-[15%]" style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-sm h-fit bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl overflow-hidden">
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <span class="text-sm font-semibold text-ctp-text">Export</span>
      <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer" on:click={() => dispatch('close')}>esc</kbd>
    </div>
    {#if message}
      <div class="px-4 py-2 text-xs {message.includes('error') || message.includes('Error') ? 'text-ctp-red' : 'text-ctp-green'} bg-ctp-surface0">{message}</div>
    {/if}
    <div class="py-1">
      {#each formats as fmt}
        <button on:click={() => dispatch('export', fmt.id)}
          class="w-full flex items-center gap-3 px-4 py-3 hover:bg-ctp-surface0 transition-colors text-left"
          disabled={fmt.id !== 'all' && !notePath}>
          <div>
            <div class="text-sm text-ctp-text">{fmt.name}</div>
            <div class="text-[12px] text-ctp-overlay1">{fmt.desc}</div>
          </div>
        </button>
      {/each}
    </div>
  </div>
</div>
