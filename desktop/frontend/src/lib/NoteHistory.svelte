<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'
  export let notePath: string = ''
  const dispatch = createEventDispatcher()

  interface NoteVersion {
    hash: string
    date: string
    message: string
    author: string
  }

  let versions: NoteVersion[] = []
  let loading = true
  let error = ''

  // View modes: 'timeline' | 'diff' | 'snapshot'
  let viewMode: 'timeline' | 'diff' | 'snapshot' = 'timeline'
  let selectedVersion: NoteVersion | null = null
  let snapshotContent = ''
  let diffContent = ''
  let diffDisplay: 'unified' | 'sidebyside' = 'unified'
  let loadingContent = false
  let message = ''
  let messageType: 'success' | 'error' = 'success'

  const api = (window as any).go?.main?.GranitApp

  onMount(loadHistory)

  async function loadHistory() {
    if (!notePath) return
    loading = true
    error = ''
    try {
      const result = await api?.GetNoteHistory(notePath)
      versions = result || []
      if (versions.length === 0) {
        error = 'No git history found for this note'
      }
    } catch (e: any) {
      error = e.message || 'Failed to load history'
    }
    loading = false
  }

  async function viewSnapshot(version: NoteVersion) {
    selectedVersion = version
    loadingContent = true
    try {
      snapshotContent = await api?.GetNoteAtVersion(notePath, version.hash) || ''
      viewMode = 'snapshot'
    } catch (e: any) {
      showMessage('Failed to load version: ' + e.message, 'error')
    }
    loadingContent = false
  }

  async function viewDiff(version: NoteVersion) {
    selectedVersion = version
    loadingContent = true
    try {
      diffContent = await api?.GetNoteDiff(notePath, version.hash) || ''
      viewMode = 'diff'
    } catch (e: any) {
      showMessage('Failed to load diff: ' + e.message, 'error')
    }
    loadingContent = false
  }

  async function restoreVersion(version: NoteVersion) {
    if (!confirm(`Restore note to version ${version.hash.slice(0, 7)} from ${version.date}?`)) return
    try {
      await api?.RestoreNoteVersion(notePath, version.hash)
      showMessage('Note restored to version ' + version.hash.slice(0, 7), 'success')
      dispatch('restored')
    } catch (e: any) {
      showMessage('Restore failed: ' + e.message, 'error')
    }
  }

  function showMessage(msg: string, type: 'success' | 'error') {
    message = msg
    messageType = type
    setTimeout(() => { message = '' }, 5000)
  }

  function diffLineClass(line: string): string {
    if (line.startsWith('+') && !line.startsWith('+++')) return 'text-ctp-green bg-ctp-green/5'
    if (line.startsWith('-') && !line.startsWith('---')) return 'text-ctp-red bg-ctp-red/5'
    if (line.startsWith('@@')) return 'text-ctp-blue bg-ctp-blue/5'
    if (line.startsWith('diff ')) return 'text-ctp-peach font-semibold'
    if (line.startsWith('---') || line.startsWith('+++')) return 'text-ctp-mauve font-semibold'
    return 'text-ctp-text'
  }

  function shortHash(hash: string): string {
    return hash.length > 7 ? hash.slice(0, 7) : hash
  }

  $: noteBase = notePath.split('/').pop()?.replace('.md', '') || notePath
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center pt-[4%]" style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-3xl h-[85vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden">
    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <div class="flex items-center gap-2">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-mauve)" stroke-width="1.5" stroke-linecap="round">
          <circle cx="8" cy="8" r="6" />
          <path d="M8 4v4l2 2" />
        </svg>
        {#if viewMode !== 'timeline'}
          <button on:click={() => { viewMode = 'timeline'; selectedVersion = null }}
            class="text-[13px] text-ctp-overlay1 hover:text-ctp-text transition-colors">
            History
          </button>
          <span class="text-ctp-overlay1 text-[13px]">/</span>
          <span class="text-sm font-semibold text-ctp-text ml-1">
            {viewMode === 'diff' ? 'Diff' : 'Snapshot'}: {selectedVersion ? shortHash(selectedVersion.hash) : ''}
          </span>
        {:else}
          <span class="text-sm font-semibold text-ctp-text">History: {noteBase}</span>
          {#if versions.length > 0}
            <span class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded-full">
              {versions.length} version{versions.length === 1 ? '' : 's'}
            </span>
          {/if}
        {/if}
      </div>
      <div class="flex items-center gap-2">
        {#if viewMode === 'diff'}
          <button on:click={() => diffDisplay = diffDisplay === 'unified' ? 'sidebyside' : 'unified'}
            class="text-[12px] font-medium bg-ctp-surface0 text-ctp-subtext0 px-2 py-0.5 rounded hover:bg-ctp-surface1 transition-colors">
            {diffDisplay === 'unified' ? 'Side-by-side' : 'Unified'}
          </button>
        {/if}
        <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1 transition-colors"
          on:click={() => dispatch('close')}>esc</kbd>
      </div>
    </div>

    <!-- Message bar -->
    {#if message}
      <div class="flex items-center gap-2 px-4 py-2 text-[13px] border-b border-ctp-surface0"
        style="background: color-mix(in srgb, {messageType === 'error' ? 'var(--ctp-red)' : 'var(--ctp-green)'} 8%, transparent);
               color: {messageType === 'error' ? 'var(--ctp-red)' : 'var(--ctp-green)'}">
        <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          {#if messageType === 'error'}<circle cx="8" cy="8" r="6" /><path d="M8 5v3m0 2.5v.5" />{:else}<path d="M3 8l3 3 7-7" />{/if}
        </svg>
        {message}
      </div>
    {/if}

    <!-- Content -->
    <div class="flex-1 overflow-y-auto">
      {#if loading || loadingContent}
        <div class="flex items-center justify-center py-16">
          <span class="text-ctp-overlay1 text-sm">Loading...</span>
        </div>
      {:else if error && viewMode === 'timeline'}
        <div class="flex flex-col items-center py-16 gap-3">
          <svg width="24" height="24" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-overlay1)" stroke-width="1" stroke-linecap="round" class="opacity-40">
            <circle cx="8" cy="8" r="6" />
            <path d="M8 4v4l2 2" />
          </svg>
          <p class="text-ctp-overlay1 text-sm">{error}</p>
          <p class="text-[13px] text-ctp-overlay1">Initialize a git repo in your vault to track note history.</p>
        </div>
      {:else if viewMode === 'timeline'}
        <!-- Timeline view -->
        <div class="py-2">
          {#each versions as version, i}
            <div class="flex gap-3 px-4 py-0 group">
              <!-- Timeline connector -->
              <div class="flex flex-col items-center pt-3">
                <div class="w-2.5 h-2.5 rounded-full border-2 shrink-0
                  {i === 0 ? 'bg-ctp-mauve border-ctp-mauve' : 'bg-ctp-surface0 border-ctp-surface1 group-hover:border-ctp-mauve'} transition-colors"></div>
                {#if i < versions.length - 1}
                  <div class="w-px flex-1 bg-ctp-surface1 min-h-[24px]"></div>
                {/if}
              </div>

              <!-- Commit info -->
              <div class="flex-1 pb-3 min-w-0">
                <div class="flex items-start justify-between gap-2">
                  <div class="min-w-0 flex-1">
                    <div class="flex items-center gap-2">
                      <span class="text-[13px] font-mono text-ctp-yellow">{shortHash(version.hash)}</span>
                      <span class="text-[12px] text-ctp-overlay1">{version.date}</span>
                    </div>
                    <p class="text-[13px] text-ctp-text mt-0.5 truncate">{version.message}</p>
                    <p class="text-[12px] text-ctp-overlay1 mt-0.5">{version.author}</p>
                  </div>

                  <!-- Action buttons -->
                  <div class="flex gap-1 shrink-0 opacity-0 group-hover:opacity-100 transition-opacity pt-0.5">
                    <button on:click={() => viewDiff(version)}
                      class="text-[12px] font-medium px-2 py-0.5 rounded bg-ctp-surface0 text-ctp-subtext0 hover:bg-ctp-surface1 hover:text-ctp-blue transition-colors">
                      Diff
                    </button>
                    <button on:click={() => viewSnapshot(version)}
                      class="text-[12px] font-medium px-2 py-0.5 rounded bg-ctp-surface0 text-ctp-subtext0 hover:bg-ctp-surface1 hover:text-ctp-mauve transition-colors">
                      View
                    </button>
                    <button on:click={() => restoreVersion(version)}
                      class="text-[12px] font-medium px-2 py-0.5 rounded bg-ctp-surface0 text-ctp-subtext0 hover:bg-ctp-surface1 hover:text-ctp-green transition-colors">
                      Restore
                    </button>
                  </div>
                </div>
              </div>
            </div>
          {/each}
        </div>
      {:else if viewMode === 'diff'}
        <!-- Diff view -->
        <div class="p-4 font-mono text-[13px] leading-relaxed">
          {#if selectedVersion}
            <div class="mb-3 text-[12px] text-ctp-overlay1">
              Comparing <span class="text-ctp-yellow font-semibold">{shortHash(selectedVersion.hash)}</span>
              ({selectedVersion.date}) with current version
            </div>
          {/if}
          {#each diffContent.split('\n') as line}
            <div class="py-0 px-2 rounded {diffLineClass(line)}" style="min-height: 18px">
              {line || '\u00A0'}
            </div>
          {/each}
          {#if diffContent.trim() === '(no differences)'}
            <p class="text-ctp-overlay1 text-center py-8 font-sans text-sm">No differences from current version</p>
          {/if}
        </div>
      {:else if viewMode === 'snapshot'}
        <!-- Snapshot view -->
        <div class="p-4">
          {#if selectedVersion}
            <div class="flex items-center justify-between mb-3">
              <span class="text-[12px] text-ctp-overlay1">
                Content at <span class="text-ctp-yellow font-mono font-semibold">{shortHash(selectedVersion.hash)}</span>
                ({selectedVersion.date})
              </span>
              <button on:click={() => restoreVersion(selectedVersion)}
                class="text-[12px] font-medium px-2.5 py-1 rounded bg-ctp-green/15 text-ctp-green hover:bg-ctp-green/25 transition-colors">
                Restore this version
              </button>
            </div>
          {/if}
          <pre class="text-[13px] leading-relaxed text-ctp-text bg-ctp-base rounded-lg p-4 border border-ctp-surface0 overflow-x-auto whitespace-pre-wrap">{snapshotContent}</pre>
        </div>
      {/if}
    </div>

    <!-- Footer -->
    <div class="px-4 py-2 border-t border-ctp-surface0 text-[12px] text-ctp-overlay1">
      {#if viewMode === 'timeline'}
        Hover a commit to view diff, snapshot, or restore.
      {:else if viewMode === 'diff'}
        Color key: <span class="text-ctp-green">+added</span> <span class="text-ctp-red">-removed</span> <span class="text-ctp-blue">@@hunk</span>
      {:else}
        Viewing historical content. Click "Restore" to replace current version.
      {/if}
    </div>
  </div>
</div>
