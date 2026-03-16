<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import type { Tab } from './types'

  export let tabs: Tab[] = []
  export let activeIndex = -1

  const dispatch = createEventDispatcher()
</script>

<div class="flex items-stretch h-[36px] bg-ctp-mantle border-b border-ctp-surface0/50 overflow-x-auto select-none tab-strip">
  {#each tabs as tab, i}
    {@const isActive = i === activeIndex}
    <button
      class="group relative flex items-center gap-2 px-3.5 min-w-[120px] max-w-[220px] text-[13px] border-r border-ctp-surface0/20
             transition-all duration-100 shrink-0"
      class:bg-ctp-base={isActive}
      class:text-ctp-text={isActive}
      class:tab-active={isActive}
      class:bg-ctp-mantle={!isActive}
      class:text-ctp-subtext0={!isActive}
      class:tab-inactive={!isActive}
      on:click={() => dispatch('select', i)}
      on:auxclick={(e) => { if (e.button === 1) { e.preventDefault(); dispatch('close', i) } }}>

      <!-- Note icon -->
      <svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"
        class="flex-shrink-0" class:opacity-80={!isActive}>
        <path d="M4 2h8v12H4V2zm2 3h4m-4 2.5h3" />
      </svg>

      <!-- Title -->
      <span class="truncate flex-1 text-left font-medium">{tab.title}</span>

      <!-- Dirty dot -->
      {#if tab.dirty}
        <span class="w-[7px] h-[7px] rounded-full bg-ctp-peach flex-shrink-0 animate-pulse"></span>
      {/if}

      <!-- Close button -->
      <button
        class="w-[22px] h-[22px] flex items-center justify-center rounded
               text-ctp-overlay1 transition-all duration-100 flex-shrink-0 text-sm leading-none
               p-0 border-0 bg-transparent hover:bg-ctp-surface1 hover:text-ctp-text"
        class:opacity-0={!isActive}
        class:group-hover:opacity-70={!isActive}
        class:opacity-50={isActive}
        class:hover:opacity-100={true}
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
