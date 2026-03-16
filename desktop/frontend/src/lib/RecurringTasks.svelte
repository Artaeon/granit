<script lang="ts">
  import { createEventDispatcher } from 'svelte'

  const dispatch = createEventDispatcher()
  const api = (window as any).go?.main?.GranitApp

  type Task = {
    text: string
    pattern: string
    notePath: string
    line: number
    nextDue: string
  }

  let tasks: Task[] = []
  let loading = false
  let error = ''
  let filter: 'all' | 'overdue' | 'today' | 'week' = 'all'

  async function loadTasks() {
    if (!api) return
    loading = true
    error = ''
    try {
      const result = await api.GetRecurringTasks()
      tasks = result || []
    } catch (e: any) {
      error = e?.message || 'Failed to load recurring tasks'
      tasks = []
    }
    loading = false
  }

  loadTasks()

  function todayStr(): string {
    return new Date().toISOString().slice(0, 10)
  }

  function weekEndStr(): string {
    const d = new Date()
    d.setDate(d.getDate() + 7)
    return d.toISOString().slice(0, 10)
  }

  function getStatus(nextDue: string): 'overdue' | 'today' | 'upcoming' {
    const today = todayStr()
    if (nextDue < today) return 'overdue'
    if (nextDue === today) return 'today'
    return 'upcoming'
  }

  function statusColor(status: string): string {
    switch (status) {
      case 'overdue': return 'text-ctp-red'
      case 'today': return 'text-ctp-yellow'
      case 'upcoming': return 'text-ctp-green'
      default: return 'text-ctp-text'
    }
  }

  function statusBg(status: string): string {
    switch (status) {
      case 'overdue': return 'bg-ctp-red/10'
      case 'today': return 'bg-ctp-yellow/10'
      case 'upcoming': return 'bg-ctp-green/10'
      default: return 'bg-ctp-surface0'
    }
  }

  function statusLabel(status: string): string {
    switch (status) {
      case 'overdue': return 'Overdue'
      case 'today': return 'Due today'
      case 'upcoming': return 'Upcoming'
      default: return ''
    }
  }

  function patternIcon(pattern: string): string {
    if (pattern === 'daily') return 'D'
    if (pattern === 'weekly') return 'W'
    if (pattern === 'monthly') return 'M'
    if (pattern.startsWith('every ')) return pattern.replace('every ', '').charAt(0).toUpperCase()
    return '?'
  }

  $: filteredTasks = tasks.filter(t => {
    if (filter === 'all') return true
    const status = getStatus(t.nextDue)
    if (filter === 'overdue') return status === 'overdue'
    if (filter === 'today') return status === 'today' || status === 'overdue'
    if (filter === 'week') return t.nextDue <= weekEndStr()
    return true
  })

  $: overdueCount = tasks.filter(t => getStatus(t.nextDue) === 'overdue').length
  $: todayCount = tasks.filter(t => getStatus(t.nextDue) === 'today').length

  const filters = [
    { id: 'all' as const, label: 'All' },
    { id: 'overdue' as const, label: 'Overdue' },
    { id: 'today' as const, label: 'Today' },
    { id: 'week' as const, label: 'This Week' },
  ]
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center pt-[8%]"
  style="background:rgba(0,0,0,0.5);backdrop-filter:blur(2px)"
  on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-xl h-[70vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-overlay flex flex-col overflow-hidden">
    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <div class="flex items-center gap-2">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-mauve)" stroke-width="1.5" stroke-linecap="round">
          <rect x="2" y="2" width="12" height="12" rx="2" /><path d="M5 1v2M11 1v2M2 6h12" />
          <path d="M5 9h2v2H5z" fill="var(--ctp-mauve)" stroke="none" />
        </svg>
        <span class="text-sm font-semibold text-ctp-text">Recurring Tasks</span>
        <span class="text-[10px] text-ctp-overlay0 bg-ctp-surface0 px-1.5 py-0.5 rounded">{tasks.length}</span>
        {#if overdueCount > 0}
          <span class="text-[10px] text-ctp-red bg-ctp-red/10 px-1.5 py-0.5 rounded font-medium">{overdueCount} overdue</span>
        {/if}
      </div>
      <div class="flex items-center gap-2">
        <button on:click={loadTasks}
          class="text-[10px] text-ctp-overlay0 hover:text-ctp-text transition-colors"
          title="Refresh">
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
            <path d="M2 8a6 6 0 0 1 10.5-4M14 8a6 6 0 0 1-10.5 4M2 4v4h4M14 12V8h-4" />
          </svg>
        </button>
        <kbd class="text-[10px] text-ctp-overlay0 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1 transition-colors"
          on:click={() => dispatch('close')}>esc</kbd>
      </div>
    </div>

    <!-- Filters -->
    <div class="flex gap-1 px-4 py-2 border-b border-ctp-surface0">
      {#each filters as f}
        <button on:click={() => filter = f.id}
          class="text-[11px] px-3 py-1 rounded-md transition-colors"
          class:bg-ctp-blue={filter === f.id}
          class:text-ctp-crust={filter === f.id}
          class:font-medium={filter === f.id}
          class:text-ctp-overlay0={filter !== f.id}
          class:hover:bg-ctp-surface0={filter !== f.id}>
          {f.label}
          {#if f.id === 'overdue' && overdueCount > 0}
            <span class="ml-0.5">({overdueCount})</span>
          {/if}
          {#if f.id === 'today' && todayCount > 0}
            <span class="ml-0.5">({todayCount})</span>
          {/if}
        </button>
      {/each}
    </div>

    <!-- Content -->
    <div class="flex-1 overflow-y-auto">
      {#if loading}
        <div class="flex flex-col items-center py-16 gap-2">
          <span class="text-ctp-overlay0 text-sm animate-pulse">Scanning for recurring tasks...</span>
        </div>
      {:else if error}
        <div class="flex flex-col items-center py-16 gap-2">
          <span class="text-ctp-red text-sm">{error}</span>
        </div>
      {:else if filteredTasks.length === 0}
        <div class="flex flex-col items-center py-16 gap-2">
          <svg width="24" height="24" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-overlay0)" stroke-width="1.2" stroke-linecap="round" class="opacity-50">
            <rect x="2" y="2" width="12" height="12" rx="2" /><path d="M5 1v2M11 1v2M2 6h12" />
          </svg>
          <span class="text-ctp-overlay0 text-sm">
            {#if filter === 'all'}
              No recurring tasks found
            {:else}
              No {filter} tasks
            {/if}
          </span>
          <span class="text-ctp-surface2 text-[10px]">Add tasks with patterns like "every monday", "daily", or "weekly"</span>
        </div>
      {:else}
        {#each filteredTasks as task}
          {@const status = getStatus(task.nextDue)}
          <button
            on:click={() => dispatch('open', task.notePath)}
            class="w-full px-4 py-3 hover:bg-ctp-surface0/40 transition-colors border-b border-ctp-surface0/30 text-left">
            <div class="flex items-start justify-between gap-2">
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2 mb-1">
                  <!-- Pattern badge -->
                  <span class="text-[10px] font-mono font-semibold w-5 h-5 rounded flex items-center justify-center flex-shrink-0 {statusBg(status)} {statusColor(status)}">
                    {patternIcon(task.pattern)}
                  </span>
                  <span class="text-[12px] text-ctp-text truncate">{task.text}</span>
                </div>
                <div class="flex items-center gap-3 pl-7">
                  <span class="text-[10px] text-ctp-overlay0">{task.pattern}</span>
                  <span class="text-ctp-surface1 text-[10px]">&middot;</span>
                  <span class="text-[10px] text-ctp-overlay0 truncate">{task.notePath}</span>
                  <span class="text-ctp-surface1 text-[10px]">&middot;</span>
                  <span class="text-[10px] text-ctp-overlay0">L{task.line}</span>
                </div>
              </div>
              <div class="flex flex-col items-end gap-0.5 flex-shrink-0">
                <span class="text-[10px] font-medium {statusColor(status)} {statusBg(status)} px-1.5 py-0.5 rounded">
                  {statusLabel(status)}
                </span>
                <span class="text-[10px] text-ctp-surface2">{task.nextDue}</span>
              </div>
            </div>
          </button>
        {/each}
      {/if}
    </div>

    <!-- Footer -->
    <div class="flex items-center gap-3 px-4 py-2 border-t border-ctp-surface0 text-[10px] text-ctp-surface2">
      <span>{filteredTasks.length} tasks</span>
      <span class="text-ctp-surface1">&middot;</span>
      <span>click to open source note</span>
      <span class="text-ctp-surface1">&middot;</span>
      <span>patterns: daily, weekly, monthly, every &lt;day&gt;</span>
    </div>
  </div>
</div>
