<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  export let settings: any[] = []
  const dispatch = createEventDispatcher()

  let search = ''
  let editingKey = ''
  let editValue = ''

  $: categories = [...new Set(settings.map(s => s.category))]
  $: filtered = search
    ? settings.filter(s => s.label.toLowerCase().includes(search.toLowerCase()) || s.category.toLowerCase().includes(search.toLowerCase()))
    : settings

  function grouped(cat: string) { return filtered.filter(s => s.category === cat) }

  function toggle(s: any) { dispatch('update', { key: s.key, value: !s.value }) }
  function cycle(s: any) {
    const idx = (s.options.indexOf(s.value) + 1) % s.options.length
    dispatch('update', { key: s.key, value: s.options[idx] })
  }
  function startEdit(s: any) { editingKey = s.key; editValue = String(s.value ?? '') }
  function commitEdit(s: any) {
    dispatch('update', { key: s.key, value: s.type === 'int' ? parseInt(editValue) || 0 : editValue })
    editingKey = ''
  }

  const categoryIcons: Record<string, string> = {
    'Appearance': '🎨', 'Editor': '✏️', 'AI': '🤖', 'Files': '📁', 'Advanced': '⚙️',
  }
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center pt-[6%]" style="background:rgba(0,0,0,0.5);backdrop-filter:blur(2px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-2xl h-[80vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-overlay flex flex-col overflow-hidden">
    <!-- Header -->
    <div class="flex items-center justify-between px-5 py-3 border-b border-ctp-surface0">
      <div class="flex items-center gap-2">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-overlay1)" stroke-width="1.5" stroke-linecap="round">
          <circle cx="8" cy="8" r="3" /><path d="M8 1v2m0 10v2M1 8h2m10 0h2M2.9 2.9l1.4 1.4m7.4 7.4l1.4 1.4M13.1 2.9l-1.4 1.4M4.3 11.7l-1.4 1.4" />
        </svg>
        <span class="text-sm font-semibold text-ctp-text">Settings</span>
      </div>
      <div class="flex items-center gap-2">
        <div class="search-input-wrapper flex items-center gap-1.5 px-2.5 py-1 bg-ctp-surface0/60 rounded-lg border border-ctp-surface0/70 transition-all">
          <svg width="11" height="11" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-overlay0)" stroke-width="1.5" stroke-linecap="round">
            <circle cx="7" cy="7" r="4" /><path d="M10 10l3.5 3.5" />
          </svg>
          <input bind:value={search} placeholder="Filter..."
            class="bg-transparent text-xs text-ctp-text outline-none w-28 placeholder:text-ctp-surface2 border-none" />
        </div>
        <kbd class="text-[10px] text-ctp-overlay0 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1 transition-colors"
          on:click={() => dispatch('close')}>esc</kbd>
      </div>
    </div>

    <!-- Settings list -->
    <div class="flex-1 overflow-y-auto p-5 space-y-5">
      {#each categories as cat}
        {@const items = grouped(cat)}
        {#if items.length > 0}
          <div>
            <div class="flex items-center gap-2 mb-3 pb-2 border-b border-ctp-surface0/40">
              <span class="text-xs">{categoryIcons[cat] || '⚙️'}</span>
              <h3 class="text-[11px] font-bold text-ctp-overlay1 uppercase tracking-[0.12em]">{cat}</h3>
            </div>
            {#each items as s}
              <div class="flex items-center justify-between py-2.5 px-3 rounded-lg hover:bg-ctp-surface0/40 transition-colors group">
                <div class="flex flex-col gap-0.5">
                  <span class="text-[13px] text-ctp-text">{s.label}</span>
                </div>
                {#if s.type === 'bool'}
                  <button on:click={() => toggle(s)}
                    class="toggle-switch flex-shrink-0" class:active={s.value}>
                  </button>
                {:else if s.type === 'select'}
                  <button on:click={() => cycle(s)}
                    class="text-[11px] text-ctp-blue bg-ctp-surface0 px-3 py-1 rounded-md
                           hover:bg-ctp-surface1 transition-colors font-medium border border-ctp-surface0 hover:border-ctp-surface1">
                    {s.value || '(none)'}
                  </button>
                {:else if editingKey === s.key}
                  <input bind:value={editValue} on:blur={() => commitEdit(s)} on:keydown={(e) => e.key === 'Enter' && commitEdit(s)}
                    class="text-[11px] bg-ctp-surface0 text-ctp-text px-3 py-1 rounded-md border border-ctp-blue outline-none w-44" autofocus />
                {:else}
                  <button on:click={() => startEdit(s)}
                    class="text-[11px] text-ctp-subtext0 bg-ctp-surface0 px-3 py-1 rounded-md
                           hover:bg-ctp-surface1 transition-colors truncate max-w-[200px] border border-ctp-surface0 hover:border-ctp-surface1">
                    {s.value || '(click to set)'}
                  </button>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
      {/each}
    </div>

    <!-- Footer -->
    <div class="px-5 py-2.5 border-t border-ctp-surface0 text-[10px] text-ctp-overlay0 flex items-center gap-3">
      <span>Click values to edit</span>
      <span class="text-ctp-surface1">&middot;</span>
      <span>Changes save automatically</span>
    </div>
  </div>
</div>
