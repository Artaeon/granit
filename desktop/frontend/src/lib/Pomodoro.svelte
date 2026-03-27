<script lang="ts">
  import { createEventDispatcher, onDestroy } from 'svelte'
  const dispatch = createEventDispatcher()

  type PomodoroState = 'idle' | 'work' | 'shortBreak' | 'longBreak'

  let state: PomodoroState = 'idle'
  let remaining = 25 * 60 // seconds
  let total = 25 * 60
  let sessions = 0
  let timerInterval: ReturnType<typeof setInterval> | null = null

  const WORK = 25 * 60
  const SHORT_BREAK = 5 * 60
  const LONG_BREAK = 15 * 60
  const LONG_BREAK_AFTER = 4

  $: minutes = Math.floor(remaining / 60)
  $: seconds = remaining % 60
  $: progress = total > 0 ? ((total - remaining) / total) * 100 : 0
  $: circumference = 2 * Math.PI * 120
  $: dashOffset = circumference - (progress / 100) * circumference

  $: stateColor = state === 'work' ? 'var(--ctp-peach)' :
                   state === 'shortBreak' ? 'var(--ctp-green)' :
                   state === 'longBreak' ? 'var(--ctp-blue)' :
                   'var(--ctp-overlay0)'

  $: stateLabel = state === 'work' ? 'FOCUS' :
                  state === 'shortBreak' ? 'SHORT BREAK' :
                  state === 'longBreak' ? 'LONG BREAK' :
                  'READY'

  function startTimer() {
    if (timerInterval) return
    if (state === 'idle') {
      state = 'work'
      remaining = WORK
      total = WORK
    }
    timerInterval = setInterval(() => {
      remaining--
      if (remaining <= 0) {
        clearInterval(timerInterval!)
        timerInterval = null
        onPhaseComplete()
      }
    }, 1000)
  }

  function pauseTimer() {
    if (timerInterval) {
      clearInterval(timerInterval)
      timerInterval = null
    }
  }

  function resetTimer() {
    pauseTimer()
    state = 'idle'
    remaining = WORK
    total = WORK
  }

  function onPhaseComplete() {
    if (state === 'work') {
      sessions++
      if (sessions % LONG_BREAK_AFTER === 0) {
        state = 'longBreak'
        remaining = LONG_BREAK
        total = LONG_BREAK
      } else {
        state = 'shortBreak'
        remaining = SHORT_BREAK
        total = SHORT_BREAK
      }
      startTimer()
    } else {
      state = 'idle'
      remaining = WORK
      total = WORK
    }
  }

  function skipPhase() {
    pauseTimer()
    if (state === 'work') {
      sessions++
      if (sessions % LONG_BREAK_AFTER === 0) {
        state = 'longBreak'
        remaining = LONG_BREAK
        total = LONG_BREAK
      } else {
        state = 'shortBreak'
        remaining = SHORT_BREAK
        total = SHORT_BREAK
      }
      startTimer()
    } else {
      state = 'idle'
      remaining = WORK
      total = WORK
    }
  }

  function minimize() {
    dispatch('minimize', { state, remaining, sessions })
    dispatch('close')
  }

  onDestroy(() => {
    if (timerInterval) clearInterval(timerInterval)
  })
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center pt-[6%]" style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-md bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden" style="max-height:85vh">
    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <div class="flex items-center gap-2">
        <span class="text-base">🍅</span>
        <span class="text-sm font-semibold text-ctp-mauve">Pomodoro Timer</span>
      </div>
      <div class="flex items-center gap-2">
        <button on:click={minimize} class="text-[12px] text-ctp-overlay1 hover:text-ctp-text px-1.5 py-0.5 rounded hover:bg-ctp-surface0 transition-colors">minimize</button>
        <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1 transition-colors"
          on:click={() => dispatch('close')}>esc</kbd>
      </div>
    </div>

    <!-- Timer body -->
    <div class="flex-1 flex flex-col items-center justify-center p-6 gap-4">
      <!-- State label -->
      <div class="text-xs font-bold tracking-widest uppercase" style="color: {stateColor}">
        {stateLabel}
      </div>

      <!-- Circular timer -->
      <div class="relative w-64 h-64 flex items-center justify-center">
        <svg class="absolute inset-0 w-full h-full -rotate-90" viewBox="0 0 260 260">
          <!-- Background circle -->
          <circle cx="130" cy="130" r="120" fill="none" stroke="var(--ctp-surface0)" stroke-width="6" />
          <!-- Progress circle -->
          <circle cx="130" cy="130" r="120" fill="none"
            stroke={stateColor}
            stroke-width="6"
            stroke-linecap="round"
            stroke-dasharray={circumference}
            stroke-dashoffset={dashOffset}
            class="transition-all duration-1000 ease-linear" />
        </svg>
        <div class="text-center z-10">
          <div class="text-5xl font-mono font-bold text-ctp-text tabular-nums">
            {String(minutes).padStart(2, '0')}:{String(seconds).padStart(2, '0')}
          </div>
          {#if state !== 'idle'}
            <div class="text-xs text-ctp-overlay1 mt-1">{Math.round(progress)}%</div>
          {/if}
        </div>
      </div>

      <!-- Controls -->
      <div class="flex items-center gap-3 mt-2">
        {#if state === 'idle'}
          <button on:click={startTimer}
            class="px-6 py-2 rounded-lg text-sm font-medium bg-ctp-peach text-ctp-crust hover:opacity-90 transition-opacity">
            Start
          </button>
        {:else if timerInterval}
          <button on:click={pauseTimer}
            class="px-5 py-2 rounded-lg text-sm font-medium bg-ctp-surface1 text-ctp-text hover:bg-ctp-surface2 transition-colors">
            Pause
          </button>
          <button on:click={skipPhase}
            class="px-5 py-2 rounded-lg text-sm font-medium bg-ctp-surface0 text-ctp-overlay1 hover:bg-ctp-surface1 transition-colors">
            Skip
          </button>
        {:else}
          <button on:click={startTimer}
            class="px-5 py-2 rounded-lg text-sm font-medium bg-ctp-green text-ctp-crust hover:opacity-90 transition-opacity">
            Resume
          </button>
          <button on:click={skipPhase}
            class="px-5 py-2 rounded-lg text-sm font-medium bg-ctp-surface0 text-ctp-overlay1 hover:bg-ctp-surface1 transition-colors">
            Skip
          </button>
        {/if}
        <button on:click={resetTimer}
          class="px-5 py-2 rounded-lg text-sm font-medium bg-ctp-surface0 text-ctp-overlay1 hover:bg-ctp-surface1 transition-colors">
          Reset
        </button>
      </div>

      <!-- Session counter -->
      <div class="flex items-center gap-4 mt-4 pt-4 border-t border-ctp-surface0 w-full justify-center">
        <div class="text-center">
          <div class="text-2xl font-bold text-ctp-peach">{sessions}</div>
          <div class="text-[12px] text-ctp-overlay1 uppercase tracking-wide">Sessions</div>
        </div>
        <div class="w-px h-8 bg-ctp-surface0"></div>
        <div class="text-center">
          <div class="text-2xl font-bold text-ctp-text">{sessions * 25}</div>
          <div class="text-[12px] text-ctp-overlay1 uppercase tracking-wide">Minutes</div>
        </div>
        <div class="w-px h-8 bg-ctp-surface0"></div>
        <div class="flex gap-1">
          {#each Array(LONG_BREAK_AFTER) as _, i}
            <div class="w-3 h-3 rounded-full {i < (sessions % LONG_BREAK_AFTER) ? 'bg-ctp-peach' : 'bg-ctp-surface1'}"></div>
          {/each}
        </div>
      </div>
    </div>

    <!-- Footer hint -->
    <div class="px-4 py-2 border-t border-ctp-surface0 text-center">
      <span class="text-[12px] text-ctp-overlay1">
        {state === 'work' ? '25 min focus' : state === 'shortBreak' ? '5 min break' : state === 'longBreak' ? '15 min long break' : 'Press Start to begin'}
        {#if sessions > 0} &middot; Long break after every {LONG_BREAK_AFTER} sessions{/if}
      </span>
    </div>
  </div>
</div>
