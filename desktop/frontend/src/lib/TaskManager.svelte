<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<script lang="ts">
  import { createEventDispatcher, onMount, onDestroy } from 'svelte'
  import type { TaskItem } from './types'
  import { getAllTasks, toggleTask as apiToggleTask } from './api'
  const dispatch = createEventDispatcher<{ close: void; openNote: string }>()
  let tasks: TaskItem[] = []
  let loading = true
  let error = ''
  let view: 'today' | 'upcoming' | 'all' | 'completed' | 'by-file' | 'by-priority' = 'today'
  let searchQuery = ''
  let showSyntaxHelp = false

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') dispatch('close')
  }

  onMount(async () => {
    window.addEventListener('keydown', handleKeydown)
    await loadTasks()
  })

  onDestroy(() => {
    window.removeEventListener('keydown', handleKeydown)
  })

  async function loadTasks() {
    loading = true
    error = ''
    try {
      const result = await getAllTasks()
      tasks = Array.isArray(result) ? result : []
    } catch (e) {
      tasks = []
      error = 'Failed to load tasks'
      console.error('TaskManager: loadTasks failed', e)
    } finally {
      loading = false
    }
  }

  // Task metadata — prefer server-provided fields, fall back to text extraction.
  function extractPriority(t: TaskItem): number {
    if (t.priority != null && t.priority > 0) return t.priority
    const text = t.text
    if (text.includes('\u{1F53A}') || text.includes('!!')) return 4
    if (text.includes('\u{23EB}')) return 3
    if (text.includes('\u{1F53C}') || /(?<!\w)!(?!\!)/.test(text)) return 2
    if (text.includes('\u{1F53D}')) return 1
    return 0
  }

  function extractDueDate(t: TaskItem): string | null {
    if (t.dueDate) return t.dueDate
    const m = t.text.match(/📅\s*(\d{4}-\d{2}-\d{2})/)
    if (m) return m[1]
    return null
  }

  function extractTags(t: TaskItem): string[] {
    if (t.tags && t.tags.length > 0) return t.tags.map(tag => '#' + tag)
    const matches = t.text.match(/#[a-zA-Z][\w\-\/]*/g)
    return matches || []
  }

  function cleanText(text: string): string {
    return text
      .replace(/📅\s*\d{4}-\d{2}-\d{2}/g, '')
      .replace(/due:\s*\d{4}-\d{2}-\d{2}/gi, '')
      .replace(/[🔺⏫🔼🔽⏰🔁]/g, '')
      .replace(/~\d+(m|h)/g, '')
      .replace(/snooze:\S+/g, '')
      .replace(/depends:"[^"]*"|depends:\S+/g, '')
      .replace(/goal:G\d{3,}/g, '')
      .replace(/#(daily|weekly|monthly|3x-week)\b/g, '')
      .replace(/\s(daily|weekly|monthly|3x-week)\s/g, ' ')
      .replace(/\s{2,}/g, ' ')
      .trim()
  }

  function fmtEstimate(mins: number): string {
    if (!mins) return ''
    if (mins >= 60) { const h = Math.floor(mins / 60), m = mins % 60; return m ? `${h}h${m}m` : `${h}h` }
    return `${mins}m`
  }

  const todayStr = new Date().toISOString().split('T')[0]
  const inDays = (n: number) => {
    const d = new Date(); d.setDate(d.getDate() + n)
    return d.toISOString().split('T')[0]
  }

  function isOverdue(due: string | null): boolean {
    return due !== null && due < todayStr
  }
  function isToday(due: string | null): boolean {
    return due === todayStr
  }
  function isUpcoming(due: string | null): boolean {
    return due !== null && due > todayStr && due <= inDays(7)
  }

  // Filtering
  $: filtered = tasks
    .filter(t => {
      if (searchQuery.trim()) {
        const q = searchQuery.toLowerCase()
        if (!t.text.toLowerCase().includes(q) && !t.notePath.toLowerCase().includes(q)) return false
      }
      const due = extractDueDate(t)
      switch (view) {
        case 'today': return !t.done && (isToday(due) || isOverdue(due))
        case 'upcoming': return !t.done && (isUpcoming(due) || isToday(due) || isOverdue(due))
        case 'completed': return t.done
        case 'all': case 'by-file': case 'by-priority': return !t.done
        default: return true
      }
    })
    .sort((a, b) => {
      if (view === 'by-file') {
        const c = a.notePath.localeCompare(b.notePath)
        if (c !== 0) return c
        return a.lineNum - b.lineNum
      }
      // Default: done last, then by priority, then by due date
      if (a.done !== b.done) return a.done ? 1 : -1
      if (view === 'by-priority') {
        const pa = extractPriority(a), pb = extractPriority(b)
        if (pa !== pb) return pb - pa
      }
      const da = extractDueDate(a), db = extractDueDate(b)
      if (da && db) {
        if (da !== db) return da.localeCompare(db)
      } else if (da) {
        return -1
      } else if (db) {
        return 1
      }
      return extractPriority(b) - extractPriority(a)
    })

  // Stats
  $: pendingCount = tasks.filter(t => !t.done).length
  $: completedCount = tasks.filter(t => t.done).length
  $: overdueCount = tasks.filter(t => !t.done && isOverdue(extractDueDate(t))).length
  $: todayCount = tasks.filter(t => !t.done && (isToday(extractDueDate(t)) || isOverdue(extractDueDate(t)))).length

  // Groups for by-file view
  $: groups = (() => {
    if (view !== 'by-file') return []
    const map = new Map<string, TaskItem[]>()
    for (const t of filtered) {
      const arr = map.get(t.notePath) || []
      arr.push(t)
      map.set(t.notePath, arr)
    }
    return Array.from(map, ([path, items]) => ({ path, name: noteName(path), tasks: items }))
  })()

  async function toggleTask(task: TaskItem) {
    const prevDone = task.done
    task.done = !task.done
    tasks = [...tasks]
    try {
      await apiToggleTask(task.notePath, task.lineNum)
    } catch (e) {
      console.error('TaskManager: toggleTask failed', e)
      task.done = prevDone
      tasks = [...tasks]
    }
  }

  function noteName(p: string) { return p.replace(/\.md$/, '').split('/').pop() || p }

  function priorityBadge(p: number): { label: string, color: string } {
    switch (p) {
      case 4: return { label: 'Urgent', color: 'var(--ctp-red)' }
      case 3: return { label: 'High', color: 'var(--ctp-peach)' }
      case 2: return { label: 'Medium', color: 'var(--ctp-yellow)' }
      case 1: return { label: 'Low', color: 'var(--ctp-blue)' }
      default: return { label: '', color: '' }
    }
  }

  function dueBadge(due: string | null): { label: string, color: string } {
    if (!due) return { label: '', color: '' }
    if (isOverdue(due)) return { label: 'Overdue', color: 'var(--ctp-red)' }
    if (isToday(due)) return { label: 'Today', color: 'var(--ctp-green)' }
    if (isUpcoming(due)) {
      const d = new Date(due + 'T00:00:00')
      return { label: d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' }), color: 'var(--ctp-yellow)' }
    }
    const d = new Date(due + 'T00:00:00')
    return { label: d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' }), color: 'var(--ctp-overlay0)' }
  }

  const views = [
    { id: 'today' as const, label: 'Today', icon: '◉' },
    { id: 'upcoming' as const, label: 'Upcoming', icon: '→' },
    { id: 'all' as const, label: 'All', icon: '☰' },
    { id: 'completed' as const, label: 'Done', icon: '✓' },
    { id: 'by-file' as const, label: 'By File', icon: '📄' },
    { id: 'by-priority' as const, label: 'By Priority', icon: '⚡' },
  ]
</script>

<div class="fixed inset-0 z-50 flex justify-center pt-[4%]" style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-3xl h-[85vh] bg-ctp-mantle rounded-xl shadow-overlay flex flex-col overflow-hidden"
    style="border: 1px solid color-mix(in srgb, var(--ctp-surface0) 50%, transparent)">

    <!-- Header -->
    <div class="flex items-center justify-between px-5 py-3" style="border-bottom: 1px solid color-mix(in srgb, var(--ctp-surface0) 40%, transparent)">
      <div class="flex items-center gap-3">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-mauve)" stroke-width="1.5" stroke-linecap="round">
          <rect x="2" y="2" width="12" height="12" rx="2" /><path d="M5 8l2 2 4-4" />
        </svg>
        <span class="text-[15px] font-semibold text-ctp-text">Task Manager</span>
      </div>
      <div class="flex items-center gap-4 text-[12px]">
        {#if overdueCount > 0}
          <span class="text-ctp-red font-medium">{overdueCount} overdue</span>
        {/if}
        <span class="text-ctp-overlay1">{pendingCount} pending</span>
        <span class="text-ctp-green">{completedCount} done</span>
        <button class="text-ctp-overlay1 hover:text-ctp-blue transition-colors" on:click={loadTasks}>Refresh</button>
        <kbd class="text-ctp-overlay1 bg-ctp-surface0/50 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1 transition-colors"
          on:click={() => dispatch('close')}>esc</kbd>
      </div>
    </div>

    <!-- View tabs + search -->
    <div class="flex items-center gap-2 px-5 py-2" style="border-bottom: 1px solid color-mix(in srgb, var(--ctp-surface0) 30%, transparent)">
      <div class="flex gap-1 flex-shrink-0">
        {#each views as v}
          <button class="px-2.5 py-1 text-[12px] rounded-md transition-all {view === v.id ? 'bg-ctp-blue text-ctp-crust font-medium' : 'text-ctp-overlay1 hover:text-ctp-text hover-bg-surface'}"
            on:click={() => view = v.id}>
            {v.label}
            {#if v.id === 'today' && todayCount > 0}
              <span class="ml-1 text-[10px] opacity-70">{todayCount}</span>
            {/if}
          </button>
        {/each}
      </div>
      <div class="flex-1">
        <input class="w-full bg-ctp-base/50 rounded-md px-3 py-1.5 text-[13px] text-ctp-text placeholder:text-ctp-overlay0 outline-none"
          style="border: 1px solid color-mix(in srgb, var(--ctp-surface0) 40%, transparent)"
          placeholder="Search tasks..." bind:value={searchQuery} />
      </div>
    </div>

    {#if loading}
      <div class="flex-1 flex items-center justify-center">
        <span class="text-sm text-ctp-overlay1">Loading tasks...</span>
      </div>
    {:else if error}
      <div class="flex-1 flex flex-col items-center justify-center gap-2">
        <p class="text-sm text-ctp-red">{error}</p>
        <button class="text-[12px] text-ctp-blue hover:underline" on:click={loadTasks}>Try again</button>
      </div>
    {:else if filtered.length === 0}
      <div class="flex-1 flex flex-col items-center justify-center gap-2">
        <p class="text-sm text-ctp-overlay1">
          {searchQuery ? `No tasks matching "${searchQuery}"` : view === 'completed' ? 'No completed tasks' : view === 'today' ? 'No tasks for today' : 'No tasks found'}
        </p>
        <p class="text-[12px] text-ctp-overlay0">Add tasks with <code class="text-ctp-blue">- [ ]</code> in your notes</p>
      </div>
    {:else}
      <div class="flex-1 overflow-y-auto py-1">
        {#if view === 'by-file'}
          {#each groups as group}
            <div class="mb-1">
              <button class="w-full px-5 py-1.5 flex items-center gap-2 hover:bg-ctp-surface0/20 transition-colors"
                on:click={() => dispatch('openNote', group.path)}>
                <svg width="11" height="11" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-overlay1)" stroke-width="1.3"><path d="M3 2h7l3 3v9H3z" /><path d="M10 2v3h3" /></svg>
                <span class="text-[13px] font-medium text-ctp-blue">{group.name}</span>
                <span class="text-[11px] text-ctp-overlay0 ml-auto">{group.tasks.length}</span>
              </button>
              {#each group.tasks as task}
                {@const pri = extractPriority(task)}
                {@const due = extractDueDate(task)}
                {@const pb = priorityBadge(pri)}
                {@const db = dueBadge(due)}
                <div class="task-row pl-10">
                  <button class="checkbox" class:done={task.done} on:click={() => toggleTask(task)}>
                    {#if task.done}<svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-crust)" stroke-width="2.5" stroke-linecap="round"><path d="M3 8l3.5 3.5L13 5" /></svg>{/if}
                  </button>
                  <span class="flex-1 text-[13px] text-ctp-text truncate" class:line-through={task.done} class:opacity-40={task.done}>{cleanText(task.text)}</span>
                  {#each extractTags(task) as tag}<span class="tag">{tag}</span>{/each}
                  {#if task.estimatedMinutes}<span class="badge" style="color:var(--ctp-teal);background:color-mix(in srgb, var(--ctp-teal) 12%, transparent)">{fmtEstimate(task.estimatedMinutes)}</span>{/if}
                  {#if task.recurrence}<span class="badge" style="color:var(--ctp-mauve);background:color-mix(in srgb, var(--ctp-mauve) 12%, transparent)">🔁 {task.recurrence}</span>{/if}
                  {#if db.label}<span class="badge" style="color:{db.color};background:color-mix(in srgb, {db.color} 12%, transparent)">{db.label}</span>{/if}
                  {#if pb.label}<span class="badge" style="color:{pb.color};background:color-mix(in srgb, {pb.color} 12%, transparent)">{pb.label}</span>{/if}
                </div>
              {/each}
            </div>
          {/each}
        {:else}
          {#each filtered as task}
            {@const pri = extractPriority(task)}
            {@const due = extractDueDate(task)}
            {@const pb = priorityBadge(pri)}
            {@const db = dueBadge(due)}
            <div class="task-row px-5">
              <button class="checkbox" class:done={task.done} on:click={() => toggleTask(task)}>
                {#if task.done}<svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-crust)" stroke-width="2.5" stroke-linecap="round"><path d="M3 8l3.5 3.5L13 5" /></svg>{/if}
              </button>
              <div class="flex-1 min-w-0">
                <div class="text-[13px] text-ctp-text truncate" class:line-through={task.done} class:opacity-40={task.done}>{cleanText(task.text)}</div>
                <button class="text-[11px] text-ctp-overlay0 hover:text-ctp-blue transition-colors truncate block"
                  on:click={() => dispatch('openNote', task.notePath)}>{noteName(task.notePath)}</button>
              </div>
              {#each extractTags(task) as tag}<span class="tag">{tag}</span>{/each}
              {#if task.estimatedMinutes}<span class="badge" style="color:var(--ctp-teal);background:color-mix(in srgb, var(--ctp-teal) 12%, transparent)">{fmtEstimate(task.estimatedMinutes)}</span>{/if}
              {#if task.recurrence}<span class="badge" style="color:var(--ctp-mauve);background:color-mix(in srgb, var(--ctp-mauve) 12%, transparent)">🔁 {task.recurrence}</span>{/if}
              {#if db.label}<span class="badge" style="color:{db.color};background:color-mix(in srgb, {db.color} 12%, transparent)">{db.label}</span>{/if}
              {#if pb.label}<span class="badge" style="color:{pb.color};background:color-mix(in srgb, {pb.color} 12%, transparent)">{pb.label}</span>{/if}
            </div>
          {/each}
        {/if}
      </div>
    {/if}

    <!-- Syntax help panel -->
    {#if showSyntaxHelp}
      <div class="px-5 py-3 text-[12px] text-ctp-subtext0"
        style="border-top: 1px solid color-mix(in srgb, var(--ctp-surface0) 30%, transparent); background: color-mix(in srgb, var(--ctp-base) 60%, transparent)">
        <div class="grid grid-cols-2 gap-x-6 gap-y-1.5">
          <div class="text-[11px] text-ctp-overlay0 uppercase tracking-wider font-medium col-span-2 mb-0.5">Task Format</div>
          <div><code class="text-ctp-text">- [ ]</code> / <code class="text-ctp-text">- [x]</code> <span class="text-ctp-overlay0 ml-1">Checkbox</span></div>
          <div><code class="text-ctp-text">📅 2026-04-01</code> <span class="text-ctp-overlay0 ml-1">Due date</span></div>

          <div class="text-[11px] text-ctp-overlay0 uppercase tracking-wider font-medium col-span-2 mt-1.5 mb-0.5">Priority</div>
          <div class="col-span-2"><code class="text-ctp-red">🔺</code> highest <code class="text-ctp-peach ml-2">⏫</code> high <code class="text-ctp-yellow ml-2">🔼</code> medium <code class="text-ctp-blue ml-2">🔽</code> low</div>

          <div class="text-[11px] text-ctp-overlay0 uppercase tracking-wider font-medium col-span-2 mt-1.5 mb-0.5">Metadata</div>
          <div><code class="text-ctp-teal">#tag</code> <span class="text-ctp-overlay0 ml-1">Tag (use multiple)</span></div>
          <div><code class="text-ctp-teal">~30m</code> / <code class="text-ctp-teal">~2h</code> <span class="text-ctp-overlay0 ml-1">Time estimate</span></div>
          <div><code class="text-ctp-text">⏰ 09:00-10:30</code> <span class="text-ctp-overlay0 ml-1">Scheduled time block</span></div>
          <div><code class="text-ctp-mauve">🔁 daily</code> <span class="text-ctp-overlay0 ml-1">Recurrence</span></div>
          <div><code class="text-ctp-text">depends:"task name"</code> <span class="text-ctp-overlay0 ml-1">Dependency</span></div>
          <div><code class="text-ctp-text">goal:G001</code> <span class="text-ctp-overlay0 ml-1">Link to goal</span></div>
          <div><code class="text-ctp-text">snooze:2026-04-01T09:00</code> <span class="text-ctp-overlay0 ml-1">Snooze</span></div>

          <div class="text-[11px] text-ctp-overlay0 uppercase tracking-wider font-medium col-span-2 mt-1.5 mb-0.5">Recurrence Options</div>
          <div class="col-span-2"><code class="text-ctp-mauve">🔁 daily</code> &middot; <code class="text-ctp-mauve">🔁 weekly</code> &middot; <code class="text-ctp-mauve">🔁 monthly</code> &middot; <code class="text-ctp-mauve">🔁 3x-week</code> &middot; or use tags: <code class="text-ctp-teal">#daily</code> <code class="text-ctp-teal">#weekly</code></div>

          <div class="text-[11px] text-ctp-overlay0 uppercase tracking-wider font-medium col-span-2 mt-1.5 mb-0.5">Subtasks</div>
          <div class="col-span-2">Indent with 2 spaces: <code class="text-ctp-text">&nbsp;&nbsp;- [ ] Sub-task</code> <span class="text-ctp-overlay0">nested under parent</span></div>

          <div class="col-span-2 mt-2 pt-2" style="border-top: 1px solid color-mix(in srgb, var(--ctp-surface0) 25%, transparent)">
            <span class="text-ctp-overlay0">Example:</span> <code class="text-ctp-text">- [ ] Ship v2.0 📅 2026-04-01 🔺 #release ~2h goal:G001</code>
          </div>
        </div>
      </div>
    {/if}

    <!-- Footer -->
    <div class="px-5 py-2 flex items-center gap-4 text-[11px] text-ctp-overlay0"
      style="border-top: 1px solid color-mix(in srgb, var(--ctp-surface0) 30%, transparent)">
      <span><code class="text-ctp-red">🔺</code><code class="text-ctp-peach">⏫</code><code class="text-ctp-yellow">🔼</code><code class="text-ctp-blue">🔽</code> priority</span>
      <span><code class="text-ctp-subtext0">📅</code> due</span>
      <span><code class="text-ctp-teal">#tag</code></span>
      <span><code class="text-ctp-teal">~30m</code> est.</span>
      <span><code class="text-ctp-mauve">🔁</code> recur</span>
      <button class="ml-auto text-ctp-overlay1 hover:text-ctp-blue transition-colors"
        on:click={() => showSyntaxHelp = !showSyntaxHelp}>{showSyntaxHelp ? 'Hide syntax' : '? Syntax'}</button>
    </div>
  </div>
</div>

<style>
  .task-row { display: flex; align-items: center; gap: 0.625rem; padding-top: 0.375rem; padding-bottom: 0.375rem; transition: background 75ms; }
  .task-row:hover { background: color-mix(in srgb, var(--ctp-surface0) 20%, transparent); }
  .checkbox { flex-shrink: 0; width: 16px; height: 16px; border-radius: 4px; border: 1.5px solid var(--ctp-surface2); display: flex; align-items: center; justify-content: center; transition: all 100ms; }
  .checkbox.done { border-color: var(--ctp-green); background: var(--ctp-green); }
  .checkbox:hover:not(.done) { border-color: var(--ctp-blue); }
  .badge { font-size: 10px; font-weight: 500; padding: 1px 6px; border-radius: 9px; flex-shrink: 0; white-space: nowrap; }
  .tag { font-size: 10px; color: var(--ctp-teal); flex-shrink: 0; }
  .hover-bg-surface:hover { background: color-mix(in srgb, var(--ctp-surface0) 40%, transparent); }
</style>
