<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import { showRightSidebar, rightSidebarPage } from '../stores'
  import PageReferences from '../shared/PageReferences.svelte'

  const dispatch = createEventDispatcher()
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="flex flex-col h-full bg-ctp-mantle border-l border-ctp-surface0/30 w-[320px]">
  <div class="flex items-center justify-between px-4 h-10 border-b border-ctp-surface0/30 flex-shrink-0">
    <span class="text-[13px] font-medium text-ctp-subtext1">Linked References</span>
    <button on:click={() => showRightSidebar.set(false)}
      class="w-6 h-6 flex items-center justify-center rounded text-ctp-overlay0 hover:text-ctp-text hover:bg-ctp-surface0/50 transition-colors">
      <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
        <path d="M4 4l8 8M12 4l-8 8" />
      </svg>
    </button>
  </div>
  <div class="flex-1 overflow-y-auto">
    {#if $rightSidebarPage}
      <PageReferences relPath={$rightSidebarPage} on:navigate />
    {:else}
      <div class="flex items-center justify-center h-full text-[13px] text-ctp-overlay0">
        Open a page to see references
      </div>
    {/if}
  </div>
</div>
