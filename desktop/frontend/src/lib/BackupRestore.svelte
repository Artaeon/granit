<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'
  const dispatch = createEventDispatcher()

  interface BackupInfo {
    name: string
    date: string
    size: number
  }

  let backups: BackupInfo[] = []
  let loading = true
  let creating = false
  let message = ''
  let messageType: 'success' | 'error' = 'success'

  const api = (window as any).go?.main?.GranitApp

  onMount(loadBackups)

  async function loadBackups() {
    loading = true
    try {
      const result = await api?.ListBackups()
      backups = result || []
    } catch (e: any) {
      showMessage('Failed to load backups: ' + e.message, 'error')
    }
    loading = false
  }

  async function createBackup() {
    if (!confirm('Create a backup of the entire vault? This may take a moment for large vaults.')) return
    creating = true
    message = ''
    try {
      const name = await api?.CreateBackup()
      showMessage(`Backup created: ${name}`, 'success')
      await loadBackups()
    } catch (e: any) {
      showMessage('Backup failed: ' + e.message, 'error')
    }
    creating = false
  }

  async function deleteBackup(name: string) {
    if (!confirm(`Delete backup "${name}"? This cannot be undone.`)) return
    try {
      await api?.DeleteBackup(name)
      showMessage('Backup deleted', 'success')
      await loadBackups()
    } catch (e: any) {
      showMessage('Delete failed: ' + e.message, 'error')
    }
  }

  function showMessage(msg: string, type: 'success' | 'error') {
    message = msg
    messageType = type
    setTimeout(() => { message = '' }, 6000)
  }

  function formatSize(bytes: number): string {
    if (bytes < 1024) return bytes + ' B'
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
    if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
    return (bytes / (1024 * 1024 * 1024)).toFixed(2) + ' GB'
  }

  function formatDate(dateStr: string): string {
    try {
      const d = new Date(dateStr)
      return d.toLocaleDateString(undefined, {
        year: 'numeric', month: 'short', day: 'numeric',
        hour: '2-digit', minute: '2-digit'
      })
    } catch {
      return dateStr
    }
  }

  function timeAgo(dateStr: string): string {
    try {
      const d = new Date(dateStr)
      const now = new Date()
      const diffMs = now.getTime() - d.getTime()
      const diffMins = Math.floor(diffMs / 60000)
      if (diffMins < 1) return 'just now'
      if (diffMins < 60) return `${diffMins}m ago`
      const diffHours = Math.floor(diffMins / 60)
      if (diffHours < 24) return `${diffHours}h ago`
      const diffDays = Math.floor(diffHours / 24)
      if (diffDays < 30) return `${diffDays}d ago`
      return `${Math.floor(diffDays / 30)}mo ago`
    } catch {
      return ''
    }
  }
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center pt-[10%]" style="background:rgba(0,0,0,0.5);backdrop-filter:blur(2px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-lg h-[60vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden">
    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <div class="flex items-center gap-2">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-green)" stroke-width="1.5" stroke-linecap="round">
          <path d="M3 8h10M8 3v10" />
          <rect x="2" y="2" width="12" height="12" rx="2" />
        </svg>
        <span class="text-sm font-semibold text-ctp-text">Vault Backup</span>
        {#if backups.length > 0}
          <span class="text-[10px] text-ctp-overlay0 bg-ctp-surface0 px-1.5 py-0.5 rounded-full">
            {backups.length}
          </span>
        {/if}
      </div>
      <div class="flex items-center gap-2">
        <button on:click={createBackup}
          disabled={creating}
          class="text-[11px] font-medium bg-ctp-green/90 text-ctp-crust px-3 py-1 rounded-md hover:bg-ctp-green transition-colors disabled:opacity-50">
          {creating ? 'Creating...' : 'Create Backup'}
        </button>
        <kbd class="text-[10px] text-ctp-overlay0 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1 transition-colors"
          on:click={() => dispatch('close')}>esc</kbd>
      </div>
    </div>

    <!-- Progress indicator -->
    {#if creating}
      <div class="px-4 py-3 border-b border-ctp-surface0 bg-ctp-green/5">
        <div class="flex items-center gap-3">
          <div class="w-4 h-4 border-2 border-ctp-green border-t-transparent rounded-full animate-spin"></div>
          <span class="text-[12px] text-ctp-green">Creating backup... This may take a moment for large vaults.</span>
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
    <div class="flex-1 overflow-y-auto">
      {#if loading}
        <div class="flex items-center justify-center py-16">
          <span class="text-ctp-overlay0 text-sm">Loading backups...</span>
        </div>
      {:else if backups.length === 0}
        <div class="flex flex-col items-center py-16 gap-3">
          <svg width="32" height="32" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-overlay0)" stroke-width="1" stroke-linecap="round" class="opacity-40">
            <rect x="2" y="2" width="12" height="12" rx="2" />
            <path d="M5 6h6M5 8h6M5 10h4" />
          </svg>
          <p class="text-ctp-overlay0 text-sm">No backups yet</p>
          <p class="text-[11px] text-ctp-overlay0">Create a backup to archive your vault as a tar.gz file.</p>
        </div>
      {:else}
        <div class="py-1">
          {#each backups as backup}
            <div class="flex items-center gap-3 px-4 py-2.5 hover:bg-ctp-surface0/40 transition-colors group">
              <!-- Archive icon -->
              <div class="w-8 h-8 rounded-lg bg-ctp-surface0 flex items-center justify-center shrink-0 group-hover:bg-ctp-green/10 transition-colors">
                <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-green)" stroke-width="1.5" stroke-linecap="round">
                  <rect x="2" y="2" width="12" height="12" rx="1" />
                  <path d="M2 5h12M6 8h4" />
                </svg>
              </div>

              <!-- Backup info -->
              <div class="flex-1 min-w-0">
                <div class="text-[12px] font-medium text-ctp-text truncate">{backup.name}</div>
                <div class="flex items-center gap-3 mt-0.5">
                  <span class="text-[10px] text-ctp-overlay0">{formatDate(backup.date)}</span>
                  <span class="text-[10px] text-ctp-overlay0">{timeAgo(backup.date)}</span>
                  <span class="text-[10px] text-ctp-subtext0 font-medium">{formatSize(backup.size)}</span>
                </div>
              </div>

              <!-- Actions -->
              <div class="flex gap-1 shrink-0 opacity-0 group-hover:opacity-100 transition-opacity">
                <button on:click={() => deleteBackup(backup.name)}
                  class="text-[10px] font-medium px-2 py-0.5 rounded bg-ctp-surface0 text-ctp-red hover:bg-ctp-red/15 transition-colors">
                  Delete
                </button>
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>

    <!-- Footer -->
    <div class="px-4 py-2 border-t border-ctp-surface0 text-[10px] text-ctp-overlay0">
      Backups are stored as tar.gz archives in .granit/backups/. Extract manually to restore.
    </div>
  </div>
</div>
