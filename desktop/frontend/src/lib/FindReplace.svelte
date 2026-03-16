<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'
  export let content: string = ''
  const dispatch = createEventDispatcher()

  let findText = ''
  let replaceText = ''
  let matchCount = 0
  let currentMatch = 0
  let findInput: HTMLInputElement

  onMount(() => findInput?.focus())

  $: {
    if (findText && content) {
      const regex = new RegExp(escapeRegex(findText), 'gi')
      matchCount = (content.match(regex) || []).length
    } else {
      matchCount = 0
    }
    currentMatch = Math.min(currentMatch, matchCount)
  }

  function escapeRegex(s: string) { return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') }

  function replaceOne() {
    if (!findText) return
    const idx = content.toLowerCase().indexOf(findText.toLowerCase())
    if (idx >= 0) {
      const newContent = content.substring(0, idx) + replaceText + content.substring(idx + findText.length)
      dispatch('replace', newContent)
    }
  }

  function replaceAll() {
    if (!findText) return
    const regex = new RegExp(escapeRegex(findText), 'gi')
    dispatch('replace', content.replace(regex, replaceText))
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') dispatch('close')
  }
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50" on:click|self={() => dispatch('close')}
  style="background: rgba(17,17,27,0.3); backdrop-filter: blur(4px);">
  <div class="fixed top-14 right-4 z-50 w-[340px] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-overlay p-4 space-y-3"
    style="animation: modalSlideIn 200ms cubic-bezier(0.16, 1, 0.3, 1);">
    <!-- Header -->
    <div class="flex items-center justify-between">
      <span class="text-sm font-semibold text-ctp-text">Find & Replace</span>
      <button on:click={() => dispatch('close')}
        class="w-6 h-6 flex items-center justify-center rounded-md text-ctp-overlay1 hover:bg-ctp-surface0 hover:text-ctp-text transition-colors">
        <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <path d="M4 4l8 8M12 4l-8 8" />
        </svg>
      </button>
    </div>

    <!-- Find row -->
    <div class="flex items-center gap-2">
      <input bind:this={findInput} bind:value={findText} on:keydown={handleKeydown} placeholder="Find..."
        class="flex-1 px-2.5 py-1.5 text-sm bg-ctp-base text-ctp-text rounded-lg border border-ctp-surface0 focus:border-ctp-blue outline-none" />
      <span class="text-[12px] text-ctp-subtext0 w-14 text-right tabular-nums font-mono"
        class:text-ctp-peach={matchCount > 0}>{matchCount} found</span>
    </div>

    <!-- Replace row -->
    <div class="flex items-center gap-2">
      <input bind:value={replaceText} on:keydown={handleKeydown} placeholder="Replace..."
        class="flex-1 px-2.5 py-1.5 text-sm bg-ctp-base text-ctp-text rounded-lg border border-ctp-surface0 focus:border-ctp-blue outline-none" />
      <button on:click={replaceOne} disabled={!matchCount}
        class="px-2.5 py-1.5 text-[12px] font-medium text-ctp-blue bg-ctp-surface0 rounded-lg hover:bg-ctp-surface1 transition-colors">
        One
      </button>
      <button on:click={replaceAll} disabled={!matchCount}
        class="px-2.5 py-1.5 text-[12px] font-medium text-ctp-blue bg-ctp-surface0 rounded-lg hover:bg-ctp-surface1 transition-colors">
        All
      </button>
    </div>

    <!-- Footer hint -->
    <div class="text-[11px] text-ctp-overlay1 flex items-center gap-2">
      <kbd class="bg-ctp-surface0 px-1.5 py-0.5 rounded text-[11px]">Esc</kbd> close
    </div>
  </div>
</div>
