<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  export let statusLines: string[] = []
  export let logLines: string[] = []
  export let diffText: string = ''
  export let message: string = ''
  const dispatch = createEventDispatcher()

  let tab: 'status' | 'log' | 'diff' = 'status'
  let commitMsg = ''
  let commitMode = false

  function statusColor(line: string) {
    if (line.startsWith(' M') || line.startsWith('M ')) return 'text-ctp-yellow'
    if (line.startsWith('A ') || line.startsWith('??')) return 'text-ctp-green'
    if (line.startsWith(' D') || line.startsWith('D ')) return 'text-ctp-red'
    return 'text-ctp-text'
  }

  function statusLabel(line: string) {
    if (line.startsWith(' M') || line.startsWith('M ')) return 'Modified'
    if (line.startsWith('A ')) return 'Added'
    if (line.startsWith('??')) return 'Untracked'
    if (line.startsWith(' D') || line.startsWith('D ')) return 'Deleted'
    return ''
  }

  function diffLineClass(line: string) {
    if (line.startsWith('+') && !line.startsWith('+++')) return 'diff-add'
    if (line.startsWith('-') && !line.startsWith('---')) return 'diff-del'
    if (line.startsWith('@@')) return 'diff-hunk'
    return ''
  }

  const tabs = [
    { id: 'status' as const, label: 'Status', icon: 'M2 8h12M5 4h6v8H5z' },
    { id: 'log' as const, label: 'Log', icon: 'M8 2v12M4 6h8M4 10h8' },
    { id: 'diff' as const, label: 'Diff', icon: 'M5 3v10M11 3v10M2 8h12' },
  ]
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center pt-[6%]" style="background:rgba(0,0,0,0.5);backdrop-filter:blur(2px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-2xl h-[80vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-overlay flex flex-col overflow-hidden">
    <!-- Header with tabs -->
    <div class="flex items-center justify-between px-4 py-0 border-b border-ctp-surface0">
      <div class="flex gap-0">
        {#each tabs as t}
          <button on:click={() => { tab = t.id; dispatch('refresh') }}
            class="relative px-4 py-3 text-[12px] font-medium transition-colors"
            class:text-ctp-blue={tab === t.id}
            class:text-ctp-overlay1={tab !== t.id}
            class:hover:text-ctp-subtext0={tab !== t.id}>
            {t.label}
            {#if tab === t.id}
              <div class="absolute bottom-0 left-2 right-2 h-[2px] bg-ctp-blue rounded-t"></div>
            {/if}
          </button>
        {/each}
      </div>
      <div class="flex gap-1.5 items-center">
        <button on:click={() => commitMode = !commitMode}
          class="text-[11px] font-medium bg-ctp-green/90 text-ctp-crust px-3 py-1 rounded-md hover:bg-ctp-green transition-colors">
          Commit
        </button>
        <button on:click={() => dispatch('push')}
          class="text-[11px] font-medium bg-ctp-blue/90 text-ctp-crust px-3 py-1 rounded-md hover:bg-ctp-blue transition-colors">
          Push
        </button>
        <button on:click={() => dispatch('pull')}
          class="text-[11px] font-medium bg-ctp-mauve/90 text-ctp-crust px-3 py-1 rounded-md hover:bg-ctp-mauve transition-colors">
          Pull
        </button>
        <kbd class="text-[10px] text-ctp-overlay0 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1 transition-colors ml-1"
          on:click={() => dispatch('close')}>esc</kbd>
      </div>
    </div>

    <!-- Commit bar -->
    {#if commitMode}
      <div class="px-4 py-3 border-b border-ctp-surface0 flex gap-2 bg-ctp-surface0/30">
        <input bind:value={commitMsg} placeholder="Commit message..."
          on:keydown={(e) => { if (e.key === 'Enter' && commitMsg) { dispatch('commit', commitMsg); commitMsg = ''; commitMode = false } }}
          class="flex-1 px-3 py-1.5 text-sm bg-ctp-surface0 text-ctp-text rounded-md border border-ctp-surface1 outline-none" autofocus />
        <button on:click={() => { if (commitMsg) { dispatch('commit', commitMsg); commitMsg = ''; commitMode = false } }}
          class="text-[11px] font-medium bg-ctp-green text-ctp-crust px-4 py-1.5 rounded-md hover:opacity-90 transition-opacity">
          Commit
        </button>
      </div>
    {/if}

    <!-- Message bar -->
    {#if message}
      {@const isError = message.includes('failed') || message.includes('error') || message.includes('Error')}
      <div class="flex items-center gap-2 px-4 py-2 text-[11px] border-b border-ctp-surface0"
        style="background: color-mix(in srgb, {isError ? 'var(--ctp-red)' : 'var(--ctp-green)'} 8%, transparent);
               color: {isError ? 'var(--ctp-red)' : 'var(--ctp-green)'}">
        <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          {#if isError}<circle cx="8" cy="8" r="6" /><path d="M8 5v3m0 2.5v.5" />{:else}<path d="M3 8l3 3 7-7" />{/if}
        </svg>
        {message}
      </div>
    {/if}

    <!-- Content -->
    <div class="flex-1 overflow-y-auto p-4 font-mono text-[12px] leading-relaxed">
      {#if tab === 'status'}
        {#each statusLines as line}
          <div class="flex items-center gap-3 py-1 px-2 rounded hover:bg-ctp-surface0/40 transition-colors">
            <span class="text-[10px] font-semibold w-16 text-right {statusColor(line)}">{statusLabel(line)}</span>
            <span class="text-ctp-text truncate">{line.slice(3)}</span>
          </div>
        {/each}
        {#if statusLines.length === 0}
          <div class="flex flex-col items-center py-12 gap-2">
            <svg width="24" height="24" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-green)" stroke-width="1.5" stroke-linecap="round" class="opacity-50">
              <path d="M3 8l3 3 7-7" />
            </svg>
            <p class="text-ctp-overlay0 text-sm">Working tree clean</p>
          </div>
        {/if}
      {:else if tab === 'log'}
        {#each logLines as line}
          <div class="py-1 px-2 rounded hover:bg-ctp-surface0/40 transition-colors">
            <span class="text-ctp-yellow font-semibold">{line.slice(0, 7)}</span>
            <span class="text-ctp-text ml-2">{line.slice(8)}</span>
          </div>
        {/each}
        {#if logLines.length === 0}<p class="text-ctp-overlay0 text-center py-8">No commits yet</p>{/if}
      {:else}
        {#if diffText}
          {#each diffText.split('\n') as line}
            <div class="py-0 px-2 {diffLineClass(line)}" style="min-height: 20px">{line}</div>
          {/each}
        {:else}
          <p class="text-ctp-overlay0 text-center py-8">No changes to display</p>
        {/if}
      {/if}
    </div>
  </div>
</div>
