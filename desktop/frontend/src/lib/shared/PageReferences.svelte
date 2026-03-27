<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'
  import { getBacklinkContext } from '../api'
  import type { BacklinkEntry } from '../types'

  export let relPath: string

  const dispatch = createEventDispatcher()

  let backlinks: BacklinkEntry[] = []
  let loading = true
  let expanded = true

  $: if (relPath) loadBacklinks(relPath)

  async function loadBacklinks(path: string) {
    loading = true
    try {
      backlinks = await getBacklinkContext(path) || []
    } catch {
      backlinks = []
    }
    loading = false
  }
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="page-references">
  <button on:click={() => expanded = !expanded}
    class="flex items-center gap-2 text-[13px] font-semibold text-ctp-subtext0 hover:text-ctp-text transition-colors mb-2">
    <svg width="8" height="8" viewBox="0 0 16 16" fill="currentColor" class="transition-transform" class:rotate-90={expanded}>
      <path d="M6 4l4 4-4 4z" />
    </svg>
    {backlinks.length} Linked Reference{backlinks.length !== 1 ? 's' : ''}
  </button>

  {#if expanded}
    {#if loading}
      <p class="text-[12px] text-ctp-overlay0 pl-4">Loading...</p>
    {:else if backlinks.length === 0}
      <p class="text-[12px] text-ctp-overlay0 pl-4">No references found</p>
    {:else}
      <div class="space-y-2">
        {#each backlinks as bl}
          <div class="pl-4 py-1.5 rounded-md hover:bg-ctp-surface0/30 transition-colors cursor-pointer"
            on:click={() => dispatch('navigate', bl.relPath)}>
            <div class="text-[13px] text-ctp-blue font-medium">{bl.title}</div>
            {#if bl.context}
              <div class="text-[12px] text-ctp-overlay1 mt-0.5 line-clamp-2">{bl.context}</div>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  {/if}
</div>

<style>
  .line-clamp-2 {
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
</style>
