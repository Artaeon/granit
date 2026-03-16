<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  export let tags: any[] = []
  export let notesForTag: any[] = []
  const dispatch = createEventDispatcher()

  let selectedTag = ''
  let cursor = 0
  let mode: 'tags' | 'notes' = 'tags'

  function selectTag(tag: string) {
    selectedTag = tag
    cursor = 0
    dispatch('selectTag', tag)
    mode = 'notes'
  }
  function back() { mode = 'tags'; selectedTag = ''; cursor = 0 }
</script>

<div class="fixed inset-0 z-50 flex justify-center pt-[10%]" style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-lg h-[60vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden">
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <div class="flex items-center gap-2">
        {#if mode === 'notes'}
          <button on:click={back} class="text-xs text-ctp-overlay1 hover:text-ctp-text">&larr;</button>
        {/if}
        <span class="text-sm font-semibold text-ctp-text">{mode === 'tags' ? 'Tags' : `#${selectedTag}`}</span>
      </div>
      <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer" on:click={() => dispatch('close')}>esc</kbd>
    </div>
    <div class="flex-1 overflow-y-auto py-1">
      {#if mode === 'tags'}
        {#each tags as tag}
          <button on:click={() => selectTag(tag.name)}
            class="w-full flex items-center justify-between px-4 py-2 hover:bg-ctp-surface0 transition-colors">
            <span class="text-sm text-ctp-blue">#{tag.name}</span>
            <span class="text-xs text-ctp-overlay1">{tag.count} notes</span>
          </button>
        {/each}
        {#if tags.length === 0}
          <p class="text-center text-sm text-ctp-overlay1 py-8">No tags found</p>
        {/if}
      {:else}
        {#each notesForTag as note}
          <button on:click={() => dispatch('openNote', note.relPath)}
            class="w-full flex items-center px-4 py-2 hover:bg-ctp-surface0 text-sm text-ctp-text text-left">
            {note.title}
          </button>
        {/each}
        {#if notesForTag.length === 0}
          <p class="text-center text-sm text-ctp-overlay1 py-8">No notes with this tag</p>
        {/if}
      {/if}
    </div>
  </div>
</div>
