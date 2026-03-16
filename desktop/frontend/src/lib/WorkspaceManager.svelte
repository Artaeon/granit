<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'
  const dispatch = createEventDispatcher()

  let workspaces: string[] = []
  let loading = true
  let saveMode = false
  let saveName = ''
  let renameMode: string | null = null
  let renameBuf = ''
  let message = ''
  let messageType: 'success' | 'error' = 'success'
  let saveInput: HTMLInputElement | null = null
  let renameInput: HTMLInputElement | null = null

  const api = (window as any).go?.main?.GranitApp

  onMount(loadWorkspaces)

  async function loadWorkspaces() {
    loading = true
    try {
      const result = await api?.ListWorkspaces()
      workspaces = result || []
    } catch (e: any) {
      showMessage('Failed to load workspaces: ' + e.message, 'error')
    }
    loading = false
  }

  async function saveWorkspace() {
    const name = saveName.trim()
    if (!name) return

    // Capture current workspace state from the app
    const state = {
      name,
      savedAt: new Date().toISOString(),
      // The frontend app dispatches the actual state capture
    }

    try {
      // Dispatch event to parent to capture current state
      dispatch('save-workspace', name)
      saveMode = false
      saveName = ''
      showMessage(`Workspace "${name}" saved`, 'success')
      await loadWorkspaces()
    } catch (e: any) {
      showMessage('Save failed: ' + e.message, 'error')
    }
  }

  async function loadWorkspace(name: string) {
    try {
      const data = await api?.LoadWorkspace(name)
      if (data) {
        dispatch('load-workspace', { name, data })
        showMessage(`Workspace "${name}" loaded`, 'success')
      }
    } catch (e: any) {
      showMessage('Load failed: ' + e.message, 'error')
    }
  }

  async function deleteWorkspace(name: string) {
    if (!confirm(`Delete workspace "${name}"?`)) return
    try {
      await api?.DeleteWorkspace(name)
      showMessage(`Workspace "${name}" deleted`, 'success')
      await loadWorkspaces()
    } catch (e: any) {
      showMessage('Delete failed: ' + e.message, 'error')
    }
  }

  async function startRename(name: string) {
    renameMode = name
    renameBuf = name
    // Focus input after render
    setTimeout(() => renameInput?.focus(), 50)
  }

  async function confirmRename(oldName: string) {
    const newName = renameBuf.trim()
    if (!newName || newName === oldName) {
      renameMode = null
      renameBuf = ''
      return
    }
    try {
      await api?.RenameWorkspace(oldName, newName)
      showMessage(`Renamed to "${newName}"`, 'success')
      renameMode = null
      renameBuf = ''
      await loadWorkspaces()
    } catch (e: any) {
      showMessage('Rename failed: ' + e.message, 'error')
    }
  }

  function showMessage(msg: string, type: 'success' | 'error') {
    message = msg
    messageType = type
    setTimeout(() => { message = '' }, 4000)
  }

  function handleSaveKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') saveWorkspace()
    if (e.key === 'Escape') { saveMode = false; saveName = '' }
  }

  function handleRenameKeydown(e: KeyboardEvent, oldName: string) {
    if (e.key === 'Enter') confirmRename(oldName)
    if (e.key === 'Escape') { renameMode = null; renameBuf = '' }
  }

  function enterSaveMode() {
    saveMode = true
    saveName = ''
    setTimeout(() => saveInput?.focus(), 50)
  }
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center pt-[10%]" style="background:rgba(0,0,0,0.5);backdrop-filter:blur(2px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-lg h-[60vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden">
    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <div class="flex items-center gap-2">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-mauve)" stroke-width="1.5" stroke-linecap="round">
          <rect x="2" y="2" width="5" height="5" rx="0.5" />
          <rect x="9" y="2" width="5" height="5" rx="0.5" />
          <rect x="2" y="9" width="5" height="5" rx="0.5" />
          <rect x="9" y="9" width="5" height="5" rx="0.5" />
        </svg>
        <span class="text-sm font-semibold text-ctp-text">Workspaces</span>
        <span class="text-[10px] text-ctp-overlay0 bg-ctp-surface0 px-1.5 py-0.5 rounded-full">
          {workspaces.length}
        </span>
      </div>
      <div class="flex items-center gap-2">
        <button on:click={enterSaveMode}
          class="text-[11px] font-medium bg-ctp-green/90 text-ctp-crust px-3 py-1 rounded-md hover:bg-ctp-green transition-colors">
          Save Current
        </button>
        <kbd class="text-[10px] text-ctp-overlay0 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1 transition-colors"
          on:click={() => dispatch('close')}>esc</kbd>
      </div>
    </div>

    <!-- Save mode input -->
    {#if saveMode}
      <div class="px-4 py-3 border-b border-ctp-surface0 bg-ctp-surface0/20">
        <div class="flex gap-2">
          <input bind:this={saveInput} bind:value={saveName}
            on:keydown={handleSaveKeydown}
            placeholder="Workspace name..."
            class="flex-1 px-3 py-1.5 text-sm bg-ctp-surface0 text-ctp-text rounded-md border border-ctp-surface1 outline-none focus:border-ctp-green transition-colors" />
          <button on:click={saveWorkspace}
            disabled={!saveName.trim()}
            class="text-[11px] font-medium bg-ctp-green text-ctp-crust px-3 py-1.5 rounded-md hover:opacity-90 transition-opacity disabled:opacity-40">
            Save
          </button>
          <button on:click={() => { saveMode = false; saveName = '' }}
            class="text-[11px] text-ctp-overlay0 px-2 py-1.5 rounded-md hover:bg-ctp-surface0 transition-colors">
            Cancel
          </button>
        </div>
      </div>
    {/if}

    <!-- Message bar -->
    {#if message}
      <div class="flex items-center gap-2 px-4 py-2 text-[11px] border-b border-ctp-surface0"
        style="background: color-mix(in srgb, {messageType === 'error' ? 'var(--ctp-red)' : 'var(--ctp-green)'} 8%, transparent);
               color: {messageType === 'error' ? 'var(--ctp-red)' : 'var(--ctp-green)'}">
        <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          {#if messageType === 'error'}<circle cx="8" cy="8" r="6" /><path d="M8 5v3m0 2.5v.5" />{:else}<path d="M3 8l3 3 7-7" />{/if}
        </svg>
        {message}
      </div>
    {/if}

    <!-- Content -->
    <div class="flex-1 overflow-y-auto py-1">
      {#if loading}
        <div class="flex items-center justify-center py-16">
          <span class="text-ctp-overlay0 text-sm">Loading workspaces...</span>
        </div>
      {:else if workspaces.length === 0}
        <div class="flex flex-col items-center py-16 gap-3">
          <svg width="28" height="28" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-overlay0)" stroke-width="1" stroke-linecap="round" class="opacity-40">
            <rect x="2" y="2" width="5" height="5" rx="0.5" />
            <rect x="9" y="2" width="5" height="5" rx="0.5" />
            <rect x="2" y="9" width="5" height="5" rx="0.5" />
            <rect x="9" y="9" width="5" height="5" rx="0.5" />
          </svg>
          <p class="text-ctp-overlay0 text-sm">No workspaces saved yet</p>
          <p class="text-[11px] text-ctp-overlay0">Save your current layout to quickly restore it later.</p>
        </div>
      {:else}
        {#each workspaces as name}
          <div class="flex items-center gap-3 px-4 py-2.5 hover:bg-ctp-surface0/40 transition-colors group">
            <!-- Workspace icon -->
            <div class="w-8 h-8 rounded-lg bg-ctp-surface0 flex items-center justify-center shrink-0 group-hover:bg-ctp-mauve/15 transition-colors">
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-mauve)" stroke-width="1.5" stroke-linecap="round">
                <rect x="2" y="2" width="5" height="5" rx="0.5" />
                <rect x="9" y="2" width="5" height="5" rx="0.5" />
                <rect x="2" y="9" width="5" height="5" rx="0.5" />
              </svg>
            </div>

            <!-- Name (editable if in rename mode) -->
            <div class="flex-1 min-w-0">
              {#if renameMode === name}
                <input bind:this={renameInput} bind:value={renameBuf}
                  on:keydown={(e) => handleRenameKeydown(e, name)}
                  on:blur={() => confirmRename(name)}
                  class="w-full px-2 py-0.5 text-[13px] bg-ctp-surface0 text-ctp-text rounded border border-ctp-blue outline-none" />
              {:else}
                <div class="text-[13px] font-medium text-ctp-text truncate">{name}</div>
              {/if}
            </div>

            <!-- Actions -->
            <div class="flex gap-1 shrink-0 opacity-0 group-hover:opacity-100 transition-opacity">
              <button on:click={() => loadWorkspace(name)}
                class="text-[10px] font-medium px-2 py-0.5 rounded bg-ctp-blue/15 text-ctp-blue hover:bg-ctp-blue/25 transition-colors">
                Load
              </button>
              <button on:click={() => startRename(name)}
                class="text-[10px] font-medium px-2 py-0.5 rounded bg-ctp-surface0 text-ctp-subtext0 hover:bg-ctp-surface1 transition-colors">
                Rename
              </button>
              <button on:click={() => deleteWorkspace(name)}
                class="text-[10px] font-medium px-2 py-0.5 rounded bg-ctp-surface0 text-ctp-red hover:bg-ctp-red/15 transition-colors">
                Delete
              </button>
            </div>
          </div>
        {/each}
      {/if}
    </div>

    <!-- Footer -->
    <div class="px-4 py-2 border-t border-ctp-surface0 text-[10px] text-ctp-overlay0">
      Workspaces save open tabs, active note, and layout state. Stored in .granit/workspaces/.
    </div>
  </div>
</div>
