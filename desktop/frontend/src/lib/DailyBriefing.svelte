<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'
  const dispatch = createEventDispatcher()
  const api = () => (window as any).go?.main?.GranitApp

  let loading = true
  let error = ''
  let data: any = null

  async function loadBriefing() {
    loading = true
    error = ''
    try {
      data = await api()?.GetDailyBriefing()
    } catch (e) {
      error = String(e)
    }
    loading = false
  }

  function openNote(relPath: string) {
    dispatch('openNote', relPath)
  }

  function timeAgo(isoStr: string): string {
    const now = new Date()
    const then = new Date(isoStr)
    const diff = Math.floor((now.getTime() - then.getTime()) / 60000)
    if (diff < 1) return 'just now'
    if (diff < 60) return `${diff}m ago`
    const hours = Math.floor(diff / 60)
    if (hours < 24) return `${hours}h ago`
    return `${Math.floor(hours / 24)}d ago`
  }

  $: taskPercent = data ? (data.totalTasks > 0 ? Math.round((data.completedTasks / data.totalTasks) * 100) : 0) : 0

  onMount(() => { loadBriefing() })
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center pt-[6%]" style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-xl bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden" style="max-height:85vh">
    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <div class="flex items-center gap-2">
        <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-mauve)" stroke-width="1.5" stroke-linecap="round">
          <circle cx="8" cy="8" r="6" /><path d="M8 4v4l3 2" />
        </svg>
        <span class="text-sm font-semibold text-ctp-mauve">Daily Briefing</span>
      </div>
      <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1 transition-colors"
        on:click={() => dispatch('close')}>esc</kbd>
    </div>

    <!-- Content -->
    <div class="flex-1 overflow-y-auto p-4">
      {#if loading}
        <div class="flex flex-col items-center justify-center py-16 gap-3">
          <div class="w-6 h-6 border-2 border-ctp-mauve border-t-transparent rounded-full animate-spin"></div>
          <span class="text-sm text-ctp-overlay1">Loading briefing...</span>
        </div>
      {:else if error}
        <div class="flex flex-col items-center justify-center py-16 gap-2">
          <span class="text-sm text-ctp-red">{error}</span>
          <button on:click={loadBriefing} class="text-xs text-ctp-blue hover:underline">Retry</button>
        </div>
      {:else if data}
        <!-- Greeting -->
        <div class="mb-5">
          <h2 class="text-lg font-semibold text-ctp-text">{data.greeting}!</h2>
          <p class="text-sm text-ctp-overlay1">{data.dateFormatted}</p>
        </div>

        <!-- Stats cards -->
        <div class="grid grid-cols-3 gap-3 mb-5">
          <div class="bg-ctp-base rounded-lg p-3 border border-ctp-surface0 text-center">
            <div class="text-2xl font-bold text-ctp-blue">{data.modifiedToday}</div>
            <div class="text-[12px] text-ctp-overlay1 uppercase tracking-wide mt-0.5">Modified Today</div>
          </div>
          <div class="bg-ctp-base rounded-lg p-3 border border-ctp-surface0 text-center">
            <div class="text-2xl font-bold text-ctp-green">{data.totalNotes}</div>
            <div class="text-[12px] text-ctp-overlay1 uppercase tracking-wide mt-0.5">Total Notes</div>
          </div>
          <div class="bg-ctp-base rounded-lg p-3 border border-ctp-surface0 text-center">
            <div class="text-2xl font-bold" class:text-ctp-yellow={taskPercent < 100} class:text-ctp-green={taskPercent === 100}>
              {data.completedTasks}/{data.totalTasks}
            </div>
            <div class="text-[12px] text-ctp-overlay1 uppercase tracking-wide mt-0.5">Tasks Done</div>
          </div>
        </div>

        <!-- Task progress bar -->
        {#if data.totalTasks > 0}
          <div class="mb-5">
            <div class="flex items-center justify-between mb-1">
              <span class="text-[13px] text-ctp-overlay1">Task Completion</span>
              <span class="text-[13px] font-medium" class:text-ctp-green={taskPercent === 100} class:text-ctp-yellow={taskPercent < 100}>{taskPercent}%</span>
            </div>
            <div class="w-full h-1.5 bg-ctp-surface0 rounded-full overflow-hidden">
              <div class="h-full rounded-full transition-all duration-500"
                class:bg-ctp-green={taskPercent === 100}
                class:bg-ctp-yellow={taskPercent < 100}
                style="width: {taskPercent}%"></div>
            </div>
          </div>
        {/if}

        <!-- Recent notes -->
        {#if data.recentNotes?.length > 0}
          <div class="mb-5">
            <h3 class="text-xs font-semibold text-ctp-mauve uppercase tracking-wider mb-2">Recent Notes</h3>
            <div class="bg-ctp-base rounded-lg border border-ctp-surface0 divide-y divide-ctp-surface0/50">
              {#each data.recentNotes as note}
                <button on:click={() => openNote(note.relPath)}
                  class="w-full flex items-center justify-between px-3 py-2 hover:bg-ctp-surface0/50 transition-colors text-left">
                  <div class="min-w-0 flex-1">
                    <div class="text-sm text-ctp-text truncate">{note.title}</div>
                    <div class="text-[12px] text-ctp-overlay1 truncate">{note.relPath}</div>
                  </div>
                  <span class="text-[12px] text-ctp-overlay1 ml-2 flex-shrink-0">{timeAgo(note.modTime)}</span>
                </button>
              {/each}
            </div>
          </div>
        {/if}

        <!-- Upcoming events -->
        {#if data.upcomingEvents?.length > 0}
          <div>
            <h3 class="text-xs font-semibold text-ctp-sapphire uppercase tracking-wider mb-2">Upcoming Events</h3>
            <div class="bg-ctp-base rounded-lg border border-ctp-surface0 p-3 space-y-1.5">
              {#each data.upcomingEvents as ev}
                <div class="flex items-center gap-2">
                  <span class="w-1.5 h-1.5 rounded-full bg-ctp-sapphire flex-shrink-0"></span>
                  <span class="text-xs text-ctp-text flex-1">{ev.title}</span>
                  <span class="text-[12px] text-ctp-overlay1">
                    {#if ev.allDay}<span class="text-ctp-overlay1">all day</span>{/if}
                    {#if ev.date !== data.date}
                      <span class="text-ctp-overlay1 ml-1">{ev.date}</span>
                    {/if}
                    {#if ev.location}
                      <span class="text-ctp-overlay1 ml-1">@ {ev.location}</span>
                    {/if}
                  </span>
                </div>
              {/each}
            </div>
          </div>
        {/if}

        <!-- Empty state if nothing -->
        {#if !data.recentNotes?.length && !data.upcomingEvents?.length && data.totalTasks === 0}
          <div class="flex flex-col items-center py-8 gap-2">
            <p class="text-sm text-ctp-overlay1">Your vault is quiet today</p>
            <p class="text-[13px] text-ctp-overlay1">Start writing to see activity here</p>
          </div>
        {/if}
      {/if}
    </div>
  </div>
</div>
