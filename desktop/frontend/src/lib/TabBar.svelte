<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import type { Tab } from './types'

  export let tabs: Tab[] = []
  export let activeIndex = -1

  const dispatch = createEventDispatcher()
</script>

<div class="flex items-stretch h-[34px] bg-ctp-mantle border-b border-ctp-surface0/50 overflow-x-auto select-none tab-strip">
  {#each tabs as tab, i}
    {@const isActive = i === activeIndex}
    <button
      class="group relative flex items-center gap-1.5 px-3 min-w-[110px] max-w-[200px] text-[11.5px] border-r border-ctp-surface0/20
             transition-all duration-100 shrink-0"
      class:bg-ctp-base={isActive}
      class:text-ctp-text={isActive}
      class:tab-active={isActive}
      class:bg-ctp-mantle={!isActive}
      class:text-ctp-overlay0={!isActive}
      class:tab-inactive={!isActive}
      on:click={() => dispatch('select', i)}
      on:auxclick={(e) => { if (e.button === 1) { e.preventDefault(); dispatch('close', i) } }}>

      <!-- Note icon -->
      <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"
        class="flex-shrink-0" class:opacity-70={!isActive} class:opacity-40={!isActive}>
        <path d="M4 2h8v12H4V2zm2 3h4m-4 2.5h3" />
      </svg>

      <!-- Title -->
      <span class="truncate flex-1 text-left">{tab.title}</span>

      <!-- Dirty dot -->
      {#if tab.dirty}
        <span class="w-[7px] h-[7px] rounded-full bg-ctp-peach flex-shrink-0 animate-pulse"></span>
      {/if}

      <!-- Close button — always visible at low opacity, full on hover -->
      <button
        class="w-[18px] h-[18px] flex items-center justify-center rounded-sm
               text-ctp-overlay0 transition-all duration-75 flex-shrink-0 text-[13px] leading-none
               p-0 border-0 bg-transparent"
        class:opacity-0={!isActive}
        class:group-hover:opacity-60={!isActive}
        class:opacity-40={isActive}
        class:hover:opacity-100={true}
        class:hover:bg-ctp-surface1={true}
        class:hover:text-ctp-text={true}
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
