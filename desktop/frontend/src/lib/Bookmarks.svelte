<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  export let starred: string[] = []
  export let recent: string[] = []
  const dispatch = createEventDispatcher()

  let tab: 'starred' | 'recent' = 'starred'

  function noteName(path: string) { return path.replace(/\.md$/, '').split('/').pop() || path }
  function folderOf(path: string) { const parts = path.split('/'); return parts.length > 1 ? parts.slice(0, -1).join('/') : '' }
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center pt-[10%]" style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-lg h-[60vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-overlay flex flex-col overflow-hidden">
    <!-- Tabs header -->
    <div class="flex items-center justify-between px-4 py-0 border-b border-ctp-surface0">
      <div class="flex gap-0">
        <button on:click={() => tab = 'starred'}
          class="relative px-4 py-3 text-[13px] font-medium flex items-center gap-1.5 transition-colors"
          class:text-ctp-yellow={tab === 'starred'}
          class:text-ctp-overlay1={tab !== 'starred'}>
          <svg width="12" height="12" viewBox="0 0 16 16" fill={tab === 'starred' ? 'var(--ctp-yellow)' : 'none'} stroke="currentColor" stroke-width="1.5">
            <path d="M8 1l2.2 4.5 5 .7-3.6 3.5.9 5L8 12.4 3.5 14.7l.9-5L.8 6.2l5-.7z" />
          </svg>
          Starred
          {#if tab === 'starred'}<div class="absolute bottom-0 left-2 right-2 h-[2px] bg-ctp-yellow rounded-t"></div>{/if}
        </button>
        <button on:click={() => tab = 'recent'}
          class="relative px-4 py-3 text-[13px] font-medium flex items-center gap-1.5 transition-colors"
          class:text-ctp-blue={tab === 'recent'}
          class:text-ctp-overlay1={tab !== 'recent'}>
          <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
            <circle cx="8" cy="8" r="6" /><path d="M8 4v4l3 2" />
          </svg>
          Recent
          {#if tab === 'recent'}<div class="absolute bottom-0 left-2 right-2 h-[2px] bg-ctp-blue rounded-t"></div>{/if}
        </button>
      </div>
      <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1 transition-colors"
        on:click={() => dispatch('close')}>esc</kbd>
    </div>

    <!-- List -->
    <div class="flex-1 overflow-y-auto py-1">
      {#each (tab === 'starred' ? starred : recent) as path}
        <div class="flex items-center justify-between px-4 py-2 hover:bg-ctp-surface0/50 group transition-colors rounded-md mx-1">
          <button on:click={() => dispatch('openNote', path)} class="flex items-center gap-2.5 text-left flex-1 truncate">
            {#if tab === 'starred'}
              <svg width="12" height="12" viewBox="0 0 16 16" fill="var(--ctp-yellow)" stroke="var(--ctp-yellow)" stroke-width="1" class="flex-shrink-0 opacity-60">
                <path d="M8 1l2.2 4.5 5 .7-3.6 3.5.9 5L8 12.4 3.5 14.7l.9-5L.8 6.2l5-.7z" />
              </svg>
            {:else}
              <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-surface2)" stroke-width="1.3" stroke-linecap="round" class="flex-shrink-0">
                <circle cx="8" cy="8" r="5" /><path d="M8 5v3l2.5 1.5" />
              </svg>
            {/if}
            <div class="min-w-0">
              <div class="text-sm text-ctp-text truncate">{noteName(path)}</div>
              {#if folderOf(path)}
                <div class="text-[12px] text-ctp-overlay1 truncate">{folderOf(path)}</div>
              {/if}
            </div>
          </button>
          {#if tab === 'starred'}
            <button on:click={() => dispatch('unstar', path)}
              class="text-[12px] text-ctp-overlay1 hover:text-ctp-red opacity-30 group-hover:opacity-100 transition-opacity px-1.5 py-0.5 rounded hover:bg-ctp-surface0">
              remove
            </button>
          {/if}
        </div>
      {/each}
      {#if (tab === 'starred' ? starred : recent).length === 0}
        <div class="flex flex-col items-center py-12 gap-2">
          {#if tab === 'starred'}
            <svg width="24" height="24" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-surface2)" stroke-width="1" class="opacity-40">
              <path d="M8 1l2.2 4.5 5 .7-3.6 3.5.9 5L8 12.4 3.5 14.7l.9-5L.8 6.2l5-.7z" />
            </svg>
            <p class="text-sm text-ctp-overlay1">No starred notes</p>
            <p class="text-[13px] text-ctp-overlay1">Star notes from the command palette</p>
          {:else}
            <svg width="24" height="24" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-surface2)" stroke-width="1" stroke-linecap="round" class="opacity-40">
              <circle cx="8" cy="8" r="6" /><path d="M8 4v4l3 2" />
            </svg>
            <p class="text-sm text-ctp-overlay1">No recent notes</p>
            <p class="text-[13px] text-ctp-overlay1">Open notes to see them here</p>
          {/if}
        </div>
      {/if}
    </div>
  </div>
</div>
