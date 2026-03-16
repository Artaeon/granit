<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  export let content: string = ''
  const dispatch = createEventDispatcher()

  let findText = ''
  let replaceText = ''
  let matchCount = 0
  let currentMatch = 0

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
</script>

<div class="fixed top-12 right-4 z-50 w-80 bg-ctp-mantle rounded-lg border border-ctp-surface0 shadow-xl p-3 space-y-2">
  <div class="flex items-center gap-2">
    <input bind:value={findText} placeholder="Find..." autofocus
      class="flex-1 px-2 py-1 text-xs bg-ctp-surface0 text-ctp-text rounded border border-transparent focus:border-ctp-blue outline-none" />
    <span class="text-[10px] text-ctp-overlay0 w-12 text-right">{matchCount} found</span>
    <button on:click={() => dispatch('close')} class="text-ctp-overlay0 hover:text-ctp-text text-xs">x</button>
  </div>
  <div class="flex items-center gap-2">
    <input bind:value={replaceText} placeholder="Replace..."
      class="flex-1 px-2 py-1 text-xs bg-ctp-surface0 text-ctp-text rounded border border-transparent focus:border-ctp-blue outline-none" />
    <button on:click={replaceOne} class="text-[10px] text-ctp-blue hover:underline">One</button>
    <button on:click={replaceAll} class="text-[10px] text-ctp-blue hover:underline">All</button>
  </div>
</div>
