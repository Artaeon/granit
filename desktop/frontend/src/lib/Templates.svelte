<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  export let templates: any[] = []
  const dispatch = createEventDispatcher()
  let cursor = 0
  let noteName = ''
  let step: 'pick' | 'name' = 'pick'
  let selectedIdx = -1
  let inputEl: HTMLInputElement

  function pick(idx: number) { selectedIdx = idx; step = 'name'; noteName = ''; setTimeout(() => inputEl?.focus(), 50) }
  function create() { if (noteName.trim()) dispatch('create', { idx: selectedIdx, name: noteName.trim() }) }
</script>

<div class="fixed inset-0 z-50 flex justify-center pt-[10%]" style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-md h-[60vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden">
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <span class="text-sm font-semibold text-ctp-text">{step === 'pick' ? 'New from Template' : 'Note Name'}</span>
      <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer" on:click={() => dispatch('close')}>esc</kbd>
    </div>
    {#if step === 'pick'}
      <div class="flex-1 overflow-y-auto py-1">
        {#each templates as t, i}
          <button on:click={() => pick(i)} class="w-full flex items-center gap-3 px-4 py-2.5 hover:bg-ctp-surface0 transition-colors text-left">
            <span class="text-ctp-mauve text-sm">{t.isUser ? '*' : '#'}</span>
            <span class="text-sm text-ctp-text">{t.name}</span>
          </button>
        {/each}
      </div>
    {:else}
      <div class="flex-1 flex flex-col items-center justify-center gap-4 px-8">
        <p class="text-sm text-ctp-subtext0">Template: <span class="text-ctp-mauve">{templates[selectedIdx]?.name}</span></p>
        <input bind:this={inputEl} bind:value={noteName} placeholder="Enter note name..."
          on:keydown={(e) => { if (e.key === 'Enter') create(); if (e.key === 'Escape') { step = 'pick' } }}
          class="w-full px-3 py-2 bg-ctp-surface0 text-ctp-text rounded-lg border border-ctp-surface1 focus:border-ctp-blue outline-none text-sm" />
        <button on:click={create} class="px-6 py-2 bg-ctp-blue text-ctp-crust rounded-lg text-sm font-medium hover:opacity-90">Create</button>
      </div>
    {/if}
  </div>
</div>
