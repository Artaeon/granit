<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import type { BacklinkEntry } from './types'

  export let incoming: BacklinkEntry[] = []
  export let outgoing: string[] = []

  const dispatch = createEventDispatcher()

  let incomingExpanded = true
  let outgoingExpanded = true
</script>

<div class="flex flex-col h-full bg-ctp-mantle select-none overflow-hidden">
  <!-- Header -->
  <div class="flex items-center px-4 py-3">
    <span class="text-[12px] font-bold text-ctp-subtext0 uppercase tracking-[0.15em]">Links</span>
  </div>

  <div class="flex-1 overflow-y-auto">
    <!-- Incoming links -->
    <div class="mb-1">
      <button class="w-full flex items-center gap-1.5 px-4 py-2 text-[13px] font-semibold text-ctp-subtext0 hover:bg-ctp-surface0/40 hover:text-ctp-text transition-colors"
        on:click={() => incomingExpanded = !incomingExpanded}>
        <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"
          class="transition-transform duration-100 flex-shrink-0" class:rotate-90={incomingExpanded}>
          <path d="M6 4l4 4-4 4" />
        </svg>
        <svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-blue)" stroke-width="1.5" stroke-linecap="round" class="flex-shrink-0">
          <path d="M10 2L6 8l4 6" /><path d="M14 8H6" />
        </svg>
        Incoming ({incoming.length})
      </button>
      {#if incomingExpanded}
        <div class="px-2">
          {#each incoming as link}
            <button class="w-full flex flex-col gap-0.5 px-3 py-2 text-left rounded-lg hover:bg-ctp-surface0/60 hover:text-ctp-text transition-colors group"
              on:click={() => dispatch('openNote', link.relPath)}>
              <span class="text-[13px] text-ctp-blue group-hover:text-ctp-lavender truncate w-full font-medium">{link.title}</span>
              {#if link.context}
                <span class="text-[12px] text-ctp-overlay1 truncate w-full leading-snug">{link.context}</span>
              {/if}
            </button>
          {/each}
          {#if incoming.length === 0}
            <div class="flex items-center gap-2 px-3 py-3 text-[12px] text-ctp-overlay1">
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" class="opacity-50 flex-shrink-0">
                <circle cx="8" cy="8" r="6" /><path d="M8 5v3M8 10h.01" />
              </svg>
              No incoming links yet
            </div>
          {/if}
        </div>
      {/if}
    </div>

    <!-- Outgoing links -->
    <div class="mb-1">
      <button class="w-full flex items-center gap-1.5 px-4 py-2 text-[13px] font-semibold text-ctp-subtext0 hover:bg-ctp-surface0/40 hover:text-ctp-text transition-colors"
        on:click={() => outgoingExpanded = !outgoingExpanded}>
        <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"
          class="transition-transform duration-100 flex-shrink-0" class:rotate-90={outgoingExpanded}>
          <path d="M6 4l4 4-4 4" />
        </svg>
        <svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-green)" stroke-width="1.5" stroke-linecap="round" class="flex-shrink-0">
          <path d="M6 2l4 6-4 6" /><path d="M2 8h8" />
        </svg>
        Outgoing ({outgoing.length})
      </button>
      {#if outgoingExpanded}
        <div class="px-2">
          {#each outgoing as link}
            <button class="w-full flex items-center gap-2 px-3 py-2 text-left rounded-lg hover:bg-ctp-surface0/60 transition-colors text-[13px] text-ctp-green hover:text-ctp-teal truncate group"
              on:click={() => dispatch('openNote', link)}>
              <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" class="flex-shrink-0 opacity-60">
                <path d="M4 2h8v12H4V2zm2 3h4m-4 2.5h3" />
              </svg>
              {link}
            </button>
          {/each}
          {#if outgoing.length === 0}
            <div class="flex items-center gap-2 px-3 py-3 text-[12px] text-ctp-overlay1">
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" class="opacity-50 flex-shrink-0">
                <circle cx="8" cy="8" r="6" /><path d="M8 5v3M8 10h.01" />
              </svg>
              No outgoing links yet
            </div>
          {/if}
        </div>
      {/if}
    </div>
  </div>
</div>
