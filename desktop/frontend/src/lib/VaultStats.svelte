<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  export let stats: any = {}
  const dispatch = createEventDispatcher()

  function bar(value: number, max: number) {
    if (max === 0) return 0
    return Math.round((value / max) * 100)
  }
  function fmt(n: number) { return n >= 1000 ? (n/1000).toFixed(1) + 'k' : String(n) }
</script>

<div class="fixed inset-0 z-50 flex justify-center pt-[8%]" style="background:rgba(0,0,0,0.5);backdrop-filter:blur(2px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-xl h-[75vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden">
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <span class="text-sm font-semibold text-ctp-text">Vault Statistics</span>
      <kbd class="text-[10px] text-ctp-overlay0 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer" on:click={() => dispatch('close')}>esc</kbd>
    </div>
    <div class="flex-1 overflow-y-auto p-4 space-y-5">
      <!-- Overview -->
      <div class="grid grid-cols-2 gap-3">
        {#each [
          ['Notes', stats.totalNotes],
          ['Words', stats.totalWords],
          ['Links', stats.totalLinks],
          ['Backlinks', stats.totalBacklinks],
          ['Tags', stats.uniqueTagCount],
          ['Orphans', stats.orphanNotes],
        ] as [label, value]}
          <div class="bg-ctp-surface0 rounded-lg p-3">
            <div class="text-[10px] text-ctp-overlay0 uppercase tracking-wider">{label}</div>
            <div class="text-lg font-semibold text-ctp-text">{fmt(value || 0)}</div>
          </div>
        {/each}
        <div class="bg-ctp-surface0 rounded-lg p-3 col-span-2">
          <div class="text-[10px] text-ctp-overlay0 uppercase tracking-wider">Avg Links / Note</div>
          <div class="text-lg font-semibold text-ctp-text">{(stats.avgLinks || 0).toFixed(1)}</div>
        </div>
      </div>

      <!-- Most Connected -->
      {#if stats.topLinked?.length}
        <div>
          <h3 class="text-xs font-semibold text-ctp-mauve uppercase tracking-wider mb-2">Most Connected</h3>
          {#each stats.topLinked as entry}
            <div class="flex items-center gap-2 py-1">
              <span class="text-xs text-ctp-text w-32 truncate">{entry.name}</span>
              <div class="flex-1 h-3 bg-ctp-surface0 rounded-full overflow-hidden">
                <div class="h-full bg-ctp-blue rounded-full transition-all" style="width:{bar(entry.value, stats.topLinked[0]?.value)}%"></div>
              </div>
              <span class="text-xs text-ctp-overlay0 w-8 text-right">{entry.value}</span>
            </div>
          {/each}
        </div>
      {/if}

      <!-- Largest Notes -->
      {#if stats.largestNotes?.length}
        <div>
          <h3 class="text-xs font-semibold text-ctp-mauve uppercase tracking-wider mb-2">Largest Notes</h3>
          {#each stats.largestNotes as entry}
            <div class="flex items-center gap-2 py-1">
              <span class="text-xs text-ctp-text w-32 truncate">{entry.name}</span>
              <div class="flex-1 h-3 bg-ctp-surface0 rounded-full overflow-hidden">
                <div class="h-full bg-ctp-green rounded-full" style="width:{bar(entry.value, stats.largestNotes[0]?.value)}%"></div>
              </div>
              <span class="text-xs text-ctp-overlay0 w-12 text-right">{fmt(entry.value)}w</span>
            </div>
          {/each}
        </div>
      {/if}

      <!-- Top Tags -->
      {#if stats.topTags?.length}
        <div>
          <h3 class="text-xs font-semibold text-ctp-mauve uppercase tracking-wider mb-2">Top Tags</h3>
          <div class="flex flex-wrap gap-2">
            {#each stats.topTags as tag}
              <span class="text-xs bg-ctp-surface0 text-ctp-blue px-2 py-1 rounded-full">#{tag.name} <span class="text-ctp-overlay0">{tag.value}</span></span>
            {/each}
          </div>
        </div>
      {/if}
    </div>
  </div>
</div>
