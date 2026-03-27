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
    <span class="text-[13px] font-medium text-ctp-overlay1">Linked pages</span>
  </div>

  <div class="flex-1 overflow-y-auto">
    <!-- Incoming links -->
    <div class="mb-1">
      <button class="w-full flex items-center gap-1.5 px-4 py-2 text-[13px] text-ctp-overlay1 font-normal transition-colors"
        on:click={() => incomingExpanded = !incomingExpanded}>
        <span class="flex-shrink-0 text-[11px]">{incomingExpanded ? '▾' : '▸'}</span>
        Incoming ({incoming.length})
      </button>
      {#if incomingExpanded}
        <div class="px-2">
          {#each incoming as link}
            <button class="w-full flex flex-col gap-0.5 px-3 py-2.5 text-left border-b border-ctp-surface0/20 hover:bg-ctp-surface0/60 hover:text-ctp-text transition-colors group"
              on:click={() => dispatch('openNote', link.relPath)}>
              <span class="text-[13px] text-ctp-text group-hover:text-ctp-lavender truncate w-full font-medium">{link.title}</span>
              {#if link.context}
                <span class="text-[12px] text-ctp-overlay1 truncate w-full leading-snug">{link.context}</span>
              {/if}
            </button>
          {/each}
          {#if incoming.length === 0}
            <div class="px-3 py-3 text-[12px] text-ctp-overlay1">
              No incoming links yet
            </div>
          {/if}
        </div>
      {/if}
    </div>

    <!-- Outgoing links -->
    <div class="mb-1">
      <button class="w-full flex items-center gap-1.5 px-4 py-2 text-[13px] text-ctp-overlay1 font-normal transition-colors"
        on:click={() => outgoingExpanded = !outgoingExpanded}>
        <span class="flex-shrink-0 text-[11px]">{outgoingExpanded ? '▾' : '▸'}</span>
        Outgoing ({outgoing.length})
      </button>
      {#if outgoingExpanded}
        <div class="px-2">
          {#each outgoing as link}
            <button class="w-full flex items-center px-3 py-2.5 text-left border-b border-ctp-surface0/20 hover:bg-ctp-surface0/60 transition-colors text-[13px] text-ctp-text hover:text-ctp-lavender truncate group"
              on:click={() => dispatch('openNote', link)}>
              {link}
            </button>
          {/each}
          {#if outgoing.length === 0}
            <div class="px-3 py-3 text-[12px] text-ctp-overlay1">
              No outgoing links yet
            </div>
          {/if}
        </div>
      {/if}
    </div>
  </div>
</div>
