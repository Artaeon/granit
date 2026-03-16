<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'
  const dispatch = createEventDispatcher()
  const api = () => (window as any).go?.main?.GranitApp

  // ---------- Types ----------
  interface TaskItem {
    text: string
    done: boolean
    notePath: string
    lineNum: number
  }

  // ---------- State ----------
  let tasks: TaskItem[] = []
  let loading = true
  let filter: 'all' | 'pending' | 'completed' = 'all'
  let sortBy: 'file' | 'priority' = 'file'
  let searchQuery = ''

  // ---------- Lifecycle ----------
  onMount(async () => {
    await loadTasks()
    loading = false
  })

  async function loadTasks() {
    try {
      tasks = (await api()?.GetAllTasks()) || []
    } catch {
      tasks = []
    }
  }

  // ---------- Filtering & Sorting ----------
  $: filtered = tasks
    .filter(t => {
      if (filter === 'pending') return !t.done
      if (filter === 'completed') return t.done
      return true
    })
    .filter(t => {
      if (!searchQuery.trim()) return true
      const q = searchQuery.toLowerCase()
      return t.text.toLowerCase().includes(q) || t.notePath.toLowerCase().includes(q)
    })
    .sort((a, b) => {
      if (sortBy === 'file') {
        const cmp = a.notePath.localeCompare(b.notePath)
        if (cmp !== 0) return cmp
        return a.lineNum - b.lineNum
      }
      // priority: extract priority markers
      const pa = extractPriority(a.text)
      const pb = extractPriority(b.text)
      if (pa !== pb) return pb - pa // higher priority first
      return a.notePath.localeCompare(b.notePath)
    })

  $: totalCount = tasks.length
  $: pendingCount = tasks.filter(t => !t.done).length
  $: completedCount = tasks.filter(t => t.done).length

  function extractPriority(text: string): number {
    if (text.includes('\u{1F53A}')) return 4 // highest
    if (text.includes('\u{23EB}')) return 3  // high
    if (text.includes('\u{1F53C}')) return 2 // medium
    if (text.includes('\u{1F53D}')) return 1 // low
    // Also check text-based markers
    if (text.includes('!!')) return 3
    if (text.includes('!')) return 2
    return 0
  }

  function priorityLabel(text: string): string {
    const p = extractPriority(text)
    switch (p) {
      case 4: return 'Highest'
      case 3: return 'High'
      case 2: return 'Medium'
      case 1: return 'Low'
      default: return ''
    }
  }

  function priorityColor(text: string): string {
    const p = extractPriority(text)
    switch (p) {
      case 4: return 'var(--ctp-red)'
      case 3: return 'var(--ctp-peach)'
      case 2: return 'var(--ctp-yellow)'
      case 1: return 'var(--ctp-blue)'
      default: return ''
    }
  }

  // ---------- Actions ----------
  async function toggleTask(task: TaskItem) {
    try {
      await api()?.ToggleTask(task.notePath, task.lineNum)
      task.done = !task.done
      tasks = [...tasks]
    } catch {}
  }

  function openNote(task: TaskItem) {
    dispatch('openNote', task.notePath)
  }

  function noteName(path: string) { return path.replace(/\.md$/, '').split('/').pop() || path }
  function folderOf(path: string) { const parts = path.split('/'); return parts.length > 1 ? parts.slice(0, -1).join('/') : '' }

  // ---------- Groups ----------
  interface TaskGroup {
    path: string
    name: string
    tasks: TaskItem[]
  }

  $: groups = (() => {
    if (sortBy !== 'file') return []
    const map = new Map<string, TaskItem[]>()
    for (const t of filtered) {
      const arr = map.get(t.notePath) || []
      arr.push(t)
      map.set(t.notePath, arr)
    }
    const result: TaskGroup[] = []
    for (const [path, items] of map) {
      result.push({ path, name: noteName(path), tasks: items })
    }
    return result
  })()

  const filterOptions = [
    { id: 'all' as const, label: 'All' },
    { id: 'pending' as const, label: 'Pending' },
    { id: 'completed' as const, label: 'Completed' },
  ]

  const sortOptions = [
    { id: 'file' as const, label: 'By File' },
    { id: 'priority' as const, label: 'By Priority' },
  ]
</script>

<div class="fixed inset-0 z-50 flex justify-center pt-[5%]" style="background:rgba(0,0,0,0.5);backdrop-filter:blur(2px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-2xl h-[82vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-overlay flex flex-col overflow-hidden">

    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-2.5 border-b border-ctp-surface0">
      <div class="flex items-center gap-2">
        <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-mauve)" stroke-width="1.5" stroke-linecap="round">
          <rect x="2" y="2" width="12" height="12" rx="2" /><path d="M5 8l2 2 4-4" />
        </svg>
        <span class="text-sm font-semibold text-ctp-text">Task Manager</span>
      </div>
      <div class="flex items-center gap-3">
        <span class="text-[10px] text-ctp-overlay0">{pendingCount} pending</span>
        <span class="text-[10px] text-ctp-green">{completedCount} done</span>
        <span class="text-[10px] text-ctp-overlay0">{totalCount} total</span>
        <button class="text-[10px] text-ctp-overlay0 hover:text-ctp-blue transition-colors"
          on:click={loadTasks}>Refresh</button>
        <kbd class="text-[10px] text-ctp-overlay0 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1 transition-colors"
          on:click={() => dispatch('close')}>esc</kbd>
      </div>
    </div>

    <!-- Toolbar: search, filter, sort -->
    <div class="flex items-center gap-2 px-4 py-2 border-b border-ctp-surface0">
      <!-- Search -->
      <div class="flex-1 relative">
        <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-overlay0)" stroke-width="1.5" class="absolute left-2.5 top-1/2 -translate-y-1/2">
          <circle cx="7" cy="7" r="4.5" /><path d="M10.5 10.5L14 14" />
        </svg>
        <input class="w-full bg-ctp-base border border-ctp-surface0 rounded-lg pl-8 pr-3 py-1.5 text-[12px] text-ctp-text placeholder-ctp-overlay0 focus:outline-none focus:border-ctp-blue"
          placeholder="Search tasks..."
          bind:value={searchQuery} />
      </div>

      <!-- Filter buttons -->
      <div class="flex bg-ctp-base rounded-lg border border-ctp-surface0 overflow-hidden">
        {#each filterOptions as opt}
          <button class="px-2.5 py-1 text-[11px] transition-colors"
            class:bg-ctp-blue={filter === opt.id}
            class:text-ctp-base={filter === opt.id}
            class:text-ctp-overlay0={filter !== opt.id}
            class:hover:text-ctp-text={filter !== opt.id}
            on:click={() => filter = opt.id}>
            {opt.label}
          </button>
        {/each}
      </div>

      <!-- Sort buttons -->
      <div class="flex bg-ctp-base rounded-lg border border-ctp-surface0 overflow-hidden">
        {#each sortOptions as opt}
          <button class="px-2.5 py-1 text-[11px] transition-colors"
            class:bg-ctp-surface1={sortBy === opt.id}
            class:text-ctp-text={sortBy === opt.id}
            class:text-ctp-overlay0={sortBy !== opt.id}
            class:hover:text-ctp-text={sortBy !== opt.id}
            on:click={() => sortBy = opt.id}>
            {opt.label}
          </button>
        {/each}
      </div>
    </div>

    {#if loading}
      <div class="flex-1 flex items-center justify-center">
        <span class="text-sm text-ctp-overlay0">Loading tasks...</span>
      </div>
    {:else if filtered.length === 0}
      <div class="flex-1 flex flex-col items-center justify-center gap-2">
        <svg width="28" height="28" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-surface2)" stroke-width="1" class="opacity-40">
          <rect x="2" y="2" width="12" height="12" rx="2" /><path d="M5 8l2 2 4-4" />
        </svg>
        <p class="text-sm text-ctp-overlay0">
          {#if searchQuery}
            No tasks matching "{searchQuery}"
          {:else if filter === 'pending'}
            No pending tasks
          {:else if filter === 'completed'}
            No completed tasks
          {:else}
            No tasks found in vault
          {/if}
        </p>
        <p class="text-[11px] text-ctp-surface2">Tasks are lines starting with - [ ] or - [x] in your notes</p>
      </div>
    {:else}
      <!-- Task List -->
      <div class="flex-1 overflow-y-auto py-1">
        {#if sortBy === 'file'}
          <!-- Grouped by file -->
          {#each groups as group}
            <div class="mb-1">
              <!-- File header -->
              <button class="w-full px-4 py-1.5 flex items-center gap-2 hover:bg-ctp-surface0/30 transition-colors"
                on:click={() => dispatch('openNote', group.path)}>
                <svg width="11" height="11" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-overlay0)" stroke-width="1.3">
                  <path d="M3 2h7l3 3v9H3z" /><path d="M10 2v3h3" />
                </svg>
                <span class="text-[11px] font-medium text-ctp-blue">{group.name}</span>
                {#if folderOf(group.path)}
                  <span class="text-[10px] text-ctp-surface2">{folderOf(group.path)}</span>
                {/if}
                <span class="text-[10px] text-ctp-overlay0 ml-auto">{group.tasks.length}</span>
              </button>

              <!-- Tasks in this file -->
              {#each group.tasks as task}
                <div class="flex items-center gap-2.5 px-4 py-1.5 pl-8 hover:bg-ctp-surface0/30 group transition-colors">
                  <!-- Checkbox -->
                  <button class="flex-shrink-0 w-4 h-4 rounded border transition-colors flex items-center justify-center"
                    class:border-ctp-green={task.done}
                    class:bg-ctp-green={task.done}
                    class:border-ctp-surface2={!task.done}
                    on:click={() => toggleTask(task)}>
                    {#if task.done}
                      <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-base)" stroke-width="2.5" stroke-linecap="round"><path d="M3 8l3.5 3.5L13 5" /></svg>
                    {/if}
                  </button>

                  <!-- Task text -->
                  <span class="flex-1 text-[12px] text-ctp-text truncate"
                    class:line-through={task.done}
                    class:opacity-50={task.done}>
                    {task.text}
                  </span>

                  <!-- Priority badge -->
                  {#if priorityLabel(task.text)}
                    <span class="text-[9px] font-medium px-1.5 py-0.5 rounded-full"
                      style="color:{priorityColor(task.text)};background:color-mix(in srgb, {priorityColor(task.text)} 15%, transparent)">
                      {priorityLabel(task.text)}
                    </span>
                  {/if}

                  <!-- Line number -->
                  <span class="text-[9px] text-ctp-surface2 opacity-0 group-hover:opacity-100 transition-opacity">
                    L{task.lineNum}
                  </span>
                </div>
              {/each}
            </div>
          {/each}
        {:else}
          <!-- Flat list sorted by priority -->
          {#each filtered as task}
            <div class="flex items-center gap-2.5 px-4 py-2 hover:bg-ctp-surface0/30 group transition-colors">
              <!-- Checkbox -->
              <button class="flex-shrink-0 w-4 h-4 rounded border transition-colors flex items-center justify-center"
                class:border-ctp-green={task.done}
                class:bg-ctp-green={task.done}
                class:border-ctp-surface2={!task.done}
                on:click={() => toggleTask(task)}>
                {#if task.done}
                  <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-base)" stroke-width="2.5" stroke-linecap="round"><path d="M3 8l3.5 3.5L13 5" /></svg>
                {/if}
              </button>

              <!-- Task text -->
              <div class="flex-1 min-w-0">
                <div class="text-[12px] text-ctp-text truncate"
                  class:line-through={task.done}
                  class:opacity-50={task.done}>
                  {task.text}
                </div>
                <button class="text-[10px] text-ctp-overlay0 hover:text-ctp-blue truncate block transition-colors"
                  on:click={() => openNote(task)}>
                  {noteName(task.notePath)}
                  {#if folderOf(task.notePath)}
                    <span class="text-ctp-surface2">/ {folderOf(task.notePath)}</span>
                  {/if}
                </button>
              </div>

              <!-- Priority badge -->
              {#if priorityLabel(task.text)}
                <span class="text-[9px] font-medium px-1.5 py-0.5 rounded-full flex-shrink-0"
                  style="color:{priorityColor(task.text)};background:color-mix(in srgb, {priorityColor(task.text)} 15%, transparent)">
                  {priorityLabel(task.text)}
                </span>
              {/if}

              <!-- Line number -->
              <span class="text-[9px] text-ctp-surface2 opacity-0 group-hover:opacity-100 transition-opacity flex-shrink-0">
                L{task.lineNum}
              </span>
            </div>
          {/each}
        {/if}
      </div>
    {/if}

    <!-- Footer -->
    <div class="px-4 py-1.5 border-t border-ctp-surface0 flex items-center gap-4 text-[10px] text-ctp-overlay0">
      <span>Click checkbox to toggle</span>
      <span>Click file name to open note</span>
      <span>Tasks: <code class="text-ctp-blue">- [ ]</code> and <code class="text-ctp-green">- [x]</code> in notes</span>
    </div>
  </div>
</div>
