<script lang="ts">
  import { createEventDispatcher } from 'svelte'

  export let notePath = ''
  export let dirty = false
  export let mode: 'edit' | 'preview' | 'split' = 'split'
  export let wordCount = 0
  export let charCount = 0
  export let cursorLine = 0
  export let cursorCol = 0
  export let aiProvider = ''
  export let themeName = ''
  export let gitBranch = ''

  const dispatch = createEventDispatcher()

  const modeIcons: Record<string, string> = {
    edit: 'M11 1l4 4L5 15H1v-4L11 1z',
    preview: 'M1 8s3-5 7-5 7 5 7 5-3 5-7 5-7-5-7-5z',
    split: 'M2 2h5v12H2zm7 0h5v12H9z',
  }
  const modeLabels: Record<string, string> = { edit: 'Editing', preview: 'Reading', split: 'Split' }
</script>

<div class="flex items-center h-[26px] px-4 bg-ctp-base border-t border-ctp-surface0/15
            text-[11px] text-ctp-overlay0 select-none">

  <!-- Left side -->
  <div class="flex items-center gap-3 min-w-0">
    {#if notePath}
      <span class="truncate max-w-[220px]">{notePath}</span>
    {/if}
    {#if dirty}
      <span class="w-[6px] h-[6px] rounded-full bg-ctp-peach flex-shrink-0 animate-pulse" data-tooltip="Unsaved changes"></span>
    {/if}
  </div>

  <div class="flex-1"></div>

  <!-- Right side -->
  <div class="flex items-center gap-3">
    {#if wordCount > 0}
      <span>{wordCount.toLocaleString()} words</span>
    {/if}

    <!-- Mode toggle -->
    <button on:click={() => dispatch('toggleMode')}
      class="flex items-center gap-1.5 px-2 py-0.5 rounded-md hover:bg-ctp-surface0 hover:text-ctp-text transition-colors"
      data-tooltip="Toggle view (Ctrl+E)">
      <svg width="11" height="11" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
        <path d="{modeIcons[mode]}" />
      </svg>
      <span class="font-medium">{modeLabels[mode]}</span>
    </button>
  </div>
</div>
