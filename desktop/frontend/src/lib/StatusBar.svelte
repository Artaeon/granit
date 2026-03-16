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

<div class="flex items-center h-[28px] px-4 bg-ctp-crust border-t border-ctp-surface0/50
            text-[10.5px] text-ctp-overlay0 select-none gap-2">

  <!-- Note path + dirty -->
  {#if notePath}
    <span class="truncate max-w-[200px]">{notePath}</span>
    {#if dirty}
      <span class="w-[6px] h-[6px] rounded-full bg-ctp-peach flex-shrink-0 animate-pulse" data-tooltip="Unsaved changes"></span>
    {/if}
    <span class="text-ctp-surface1">&middot;</span>
  {/if}

  <!-- Cursor position -->
  {#if cursorLine > 0}
    <span class="font-mono text-ctp-overlay0">Ln {cursorLine}, Col {cursorCol}</span>
    <span class="text-ctp-surface1">&middot;</span>
  {/if}

  <div class="flex-1"></div>

  <!-- Char count -->
  {#if charCount > 0}
    <span>{charCount.toLocaleString()} chars</span>
    <span class="text-ctp-surface1">&middot;</span>
  {/if}

  <!-- Word count -->
  {#if wordCount > 0}
    <span>{wordCount.toLocaleString()} words</span>
    <span class="text-ctp-surface1">&middot;</span>
  {/if}

  <!-- AI provider badge -->
  {#if aiProvider && aiProvider !== 'local'}
    {@const color = aiProvider === 'ollama' ? 'var(--ctp-green)' : 'var(--ctp-blue)'}
    <span class="flex items-center gap-1 px-1.5 py-0.5 rounded-md text-[9px] font-medium"
      style="color: {color}; background: color-mix(in srgb, {color} 10%, transparent);">
      <span class="w-[5px] h-[5px] rounded-full" style="background: {color}"></span>
      {aiProvider}
    </span>
    <span class="text-ctp-surface1">&middot;</span>
  {/if}

  <!-- Theme -->
  {#if themeName}
    <span class="truncate max-w-[100px] text-ctp-surface2">{themeName}</span>
    <span class="text-ctp-surface1">&middot;</span>
  {/if}

  <!-- Git branch -->
  {#if gitBranch}
    <span class="flex items-center gap-1 text-ctp-mauve">
      <svg width="11" height="11" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
        <circle cx="5" cy="4" r="2" /><circle cx="5" cy="12" r="2" /><circle cx="12" cy="6" r="2" />
        <path d="M5 6v4M7.8 5.2L10 6" />
      </svg>
      <span class="truncate max-w-[80px]">{gitBranch}</span>
    </span>
    <span class="text-ctp-surface1">&middot;</span>
  {/if}

  <!-- Mode toggle -->
  <button on:click={() => dispatch('toggleMode')}
    class="flex items-center gap-1.5 px-2 py-0.5 rounded-md hover:bg-ctp-surface0 transition-colors"
    data-tooltip="Toggle view (Ctrl+E)">
    <svg width="11" height="11" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
      <path d="{modeIcons[mode]}" />
    </svg>
    <span class="font-medium">{modeLabels[mode]}</span>
  </button>

  <span class="text-ctp-surface1">&middot;</span>
  <span class="text-ctp-surface2 hover:text-ctp-overlay0 transition-colors cursor-default"
    data-tooltip="Ctrl+X for command palette">Ctrl+X</span>
</div>
