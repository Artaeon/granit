<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import type { Tab } from './types'

  export let tabs: Tab[] = []
  export let activeIndex = -1

  const dispatch = createEventDispatcher()
</script>

<div class="flex items-stretch h-[36px] bg-ctp-base border-b border-ctp-surface0/20 overflow-x-auto select-none tab-strip">
  {#each tabs as tab, i}
    {@const isActive = i === activeIndex}
    <button
      class="group relative flex items-center gap-2 px-3.5 min-w-[120px] max-w-[220px] text-[13px]
             transition-all duration-100 shrink-0
             {isActive ? 'text-ctp-text font-medium' : 'text-ctp-overlay1 font-normal hover:bg-ctp-surface0/30'}"
      style="{isActive ? 'background: color-mix(in srgb, var(--ctp-surface0) 70%, transparent)' : ''}"
      on:click={() => dispatch('select', i)}
      on:auxclick={(e) => { if (e.button === 1) { e.preventDefault(); dispatch('close', i) } }}>

      <!-- Title -->
      <span class="truncate flex-1 text-left">{tab.title}</span>

      <!-- Dirty dot -->
      {#if tab.dirty}
        <span class="w-[7px] h-[7px] rounded-full bg-ctp-peach flex-shrink-0 animate-pulse"></span>
      {/if}

      <!-- Close button -->
      <button
        class="w-[22px] h-[22px] flex items-center justify-center rounded
               text-ctp-overlay1 transition-all duration-100 flex-shrink-0 text-sm leading-none
               p-0 border-0 bg-transparent hover:bg-ctp-surface1 hover:text-ctp-text
               opacity-0 group-hover:opacity-50 hover:!opacity-100"
        on:click|stopPropagation={() => dispatch('close', i)}
        tabindex="-1">
        &times;
      </button>
    </button>
  {/each}

  <!-- New tab hint area -->
  <div class="flex-1 min-w-[40px]"></div>
</div>

<style>
  .tab-strip::-webkit-scrollbar { display: none; }
  .tab-strip { scrollbar-width: none; }
</style>
