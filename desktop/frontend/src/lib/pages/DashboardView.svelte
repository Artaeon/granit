<script lang="ts">
  import { onMount, createEventDispatcher } from 'svelte'
  import { notes, navigateToPage, navigateToJournal, currentView } from '../stores'
  import * as api from '../api'

  const dispatch = createEventDispatcher()

  let stats: any = null
  let tasks: any[] = []
  let recentNotes: any[] = []
  let tags: any[] = []
  let loading = true

  onMount(async () => {
    try {
      const [s, t, tg] = await Promise.allSettled([
        api.getVaultStats(),
        api.getAllTasks(),
        api.getAllTags(),
      ])
      stats = s.status === 'fulfilled' ? s.value : null
      tasks = t.status === 'fulfilled' ? (t.value || []) : []
      tags = tg.status === 'fulfilled' ? (tg.value || []).slice(0, 15) : []
    } catch {}

    recentNotes = $notes
      .slice()
      .sort((a, b) => new Date(b.modTime).getTime() - new Date(a.modTime).getTime())
      .slice(0, 10)
    loading = false
  })

  $: openTasks = tasks.filter((t: any) => !t.done)
  $: doneTasks = tasks.filter((t: any) => t.done)

  function formatRelTime(dateStr: string): string {
    const d = new Date(dateStr)
    const now = new Date()
    const diff = now.getTime() - d.getTime()
    const mins = Math.floor(diff / 60000)
    if (mins < 60) return `${mins}m ago`
    const hours = Math.floor(mins / 60)
    if (hours < 24) return `${hours}h ago`
    const days = Math.floor(hours / 24)
    return `${days}d ago`
  }
</script>

<div class="h-full overflow-y-auto bg-ctp-base">
  <div class="max-w-[900px] mx-auto px-8 py-8">
    <h1 class="text-2xl font-bold text-ctp-text mb-6">Dashboard</h1>

    {#if loading}
      <div class="text-ctp-overlay0 text-sm py-10 text-center">Loading...</div>
    {:else}
      <!-- Stats row -->
      <div class="grid grid-cols-4 gap-4 mb-8">
        <div class="stat-card">
          <div class="stat-value">{stats?.totalNotes ?? $notes.length}</div>
          <div class="stat-label">Notes</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{stats?.totalLinks ?? 0}</div>
          <div class="stat-label">Links</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{openTasks.length}</div>
          <div class="stat-label">Open Tasks</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{tags.length}</div>
          <div class="stat-label">Tags</div>
        </div>
      </div>

      <div class="grid grid-cols-2 gap-6">
        <!-- Open tasks -->
        <div class="panel">
          <h2 class="panel-title">Open Tasks</h2>
          {#if openTasks.length === 0}
            <p class="text-ctp-overlay0 text-sm">No open tasks</p>
          {:else}
            <div class="space-y-1 max-h-[300px] overflow-y-auto">
              {#each openTasks.slice(0, 15) as task}
                <button on:click={() => navigateToPage(task.file)}
                  class="flex items-start gap-2 w-full text-left px-2 py-1 rounded hover:bg-ctp-surface0/50 transition-colors">
                  <span class="w-3 h-3 mt-0.5 rounded-sm border border-ctp-overlay0 flex-shrink-0"></span>
                  <span class="text-[13px] text-ctp-subtext1 line-clamp-1">{task.text}</span>
                </button>
              {/each}
            </div>
          {/if}
        </div>

        <!-- Recently modified -->
        <div class="panel">
          <h2 class="panel-title">Recently Modified</h2>
          <div class="space-y-1 max-h-[300px] overflow-y-auto">
            {#each recentNotes as note}
              <button on:click={() => navigateToPage(note.relPath)}
                class="flex items-center justify-between w-full text-left px-2 py-1 rounded hover:bg-ctp-surface0/50 transition-colors">
                <span class="text-[13px] text-ctp-subtext1 truncate">{note.title}</span>
                <span class="text-[11px] text-ctp-overlay0 flex-shrink-0 ml-2">{formatRelTime(note.modTime)}</span>
              </button>
            {/each}
          </div>
        </div>

        <!-- Tags cloud -->
        <div class="panel">
          <h2 class="panel-title">Top Tags</h2>
          <div class="flex flex-wrap gap-2">
            {#each tags as tag}
              <span class="px-2 py-0.5 rounded-full bg-ctp-surface0/60 text-[12px] text-ctp-subtext0">
                #{tag.tag || tag.name || tag} <span class="text-ctp-overlay0">{tag.count ?? ''}</span>
              </span>
            {/each}
          </div>
        </div>

        <!-- Quick actions -->
        <div class="panel">
          <h2 class="panel-title">Quick Actions</h2>
          <div class="grid grid-cols-2 gap-2">
            <button on:click={() => navigateToJournal()} class="action-btn">Today's Journal</button>
            <button on:click={() => dispatch('command', 'new_note')} class="action-btn">New Note</button>
            <button on:click={() => dispatch('command', 'show_graph')} class="action-btn">Graph View</button>
            <button on:click={() => dispatch('command', 'content_search')} class="action-btn">Search Vault</button>
            <button on:click={() => dispatch('command', 'task_manager')} class="action-btn">All Tasks</button>
            <button on:click={() => dispatch('command', 'show_calendar')} class="action-btn">Calendar</button>
          </div>
        </div>
      </div>
    {/if}
  </div>
</div>

<style>
  .stat-card {
    background: color-mix(in srgb, var(--ctp-surface0) 40%, transparent);
    border: 1px solid color-mix(in srgb, var(--ctp-surface0) 60%, transparent);
    border-radius: 0.5rem;
    padding: 1rem 1.25rem;
    text-align: center;
  }
  .stat-value {
    font-size: 1.75rem;
    font-weight: 700;
    color: var(--ctp-blue);
    line-height: 1;
  }
  .stat-label {
    font-size: 12px;
    color: var(--ctp-overlay0);
    margin-top: 0.375rem;
  }
  .panel {
    background: color-mix(in srgb, var(--ctp-surface0) 25%, transparent);
    border: 1px solid color-mix(in srgb, var(--ctp-surface0) 50%, transparent);
    border-radius: 0.5rem;
    padding: 1rem 1.25rem;
  }
  .panel-title {
    font-size: 14px;
    font-weight: 600;
    color: var(--ctp-subtext1);
    margin-bottom: 0.75rem;
  }
  .action-btn {
    padding: 0.5rem;
    border-radius: 0.375rem;
    font-size: 13px;
    color: var(--ctp-subtext0);
    background: color-mix(in srgb, var(--ctp-surface0) 50%, transparent);
    border: 1px solid color-mix(in srgb, var(--ctp-surface0) 60%, transparent);
    text-align: center;
    transition: all 75ms;
  }
  .action-btn:hover {
    background: color-mix(in srgb, var(--ctp-surface0) 80%, transparent);
    color: var(--ctp-text);
  }
</style>
