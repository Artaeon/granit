<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'
  const dispatch = createEventDispatcher()
  const api = () => (window as any).go?.main?.GranitApp

  export let notePath: string = ''

  interface Card {
    front: string
    back: string
    id: string
  }

  interface ProgressEntry {
    interval: number
    easeFactor: number
    reps: number
    lapses: number
    due: string
    lastRating: number
  }

  interface Progress {
    cards: Record<string, ProgressEntry>
    totalReviews: number
    streakDays: number
    lastReview: string
  }

  let cards: Card[] = []
  let progress: Progress = { cards: {}, totalReviews: 0, streakDays: 0, lastReview: '' }
  let loading = true
  let mode: 'deck' | 'review' | 'stats' = 'deck'

  // Review state
  let reviewQueue: Card[] = []
  let reviewIdx = 0
  let showAnswer = false
  let sessionDone = 0
  let sessionTotal = 0
  let sessionCorrect = 0

  // Deck selector
  let deckSource: 'current' | 'all' = 'current'

  $: dueCards = cards.filter(c => {
    const prog = progress.cards[c.id]
    if (!prog) return true
    return !prog.due || new Date(prog.due) <= new Date()
  })

  $: newCards = cards.filter(c => !progress.cards[c.id] || progress.cards[c.id].reps === 0)
  $: masteredCards = cards.filter(c => {
    const p = progress.cards[c.id]
    return p && p.interval >= 21 && p.easeFactor >= 2.5
  })

  onMount(async () => {
    await loadData()
  })

  async function loadData() {
    loading = true
    try {
      const rawProgress = await api()?.GetFlashcardProgress()
      if (rawProgress && rawProgress !== '{}') {
        progress = JSON.parse(rawProgress)
        if (!progress.cards) progress.cards = {}
      }
      if (notePath) {
        cards = (await api()?.GetFlashcards(notePath)) || []
      }
    } catch (e) {
      console.error('Failed to load flashcards:', e)
    }
    loading = false
  }

  function startReview() {
    reviewQueue = [...dueCards]
    reviewIdx = 0
    showAnswer = false
    sessionDone = 0
    sessionCorrect = 0
    sessionTotal = reviewQueue.length
    if (sessionTotal > 0) {
      mode = 'review'
    }
  }

  function flipCard() {
    showAnswer = true
  }

  function rateCard(quality: number) {
    const card = reviewQueue[reviewIdx]
    if (!card) return

    let entry = progress.cards[card.id] || {
      interval: 0,
      easeFactor: 2.5,
      reps: 0,
      lapses: 0,
      due: '',
      lastRating: 0
    }

    // SM-2 algorithm
    const q = Math.max(0, Math.min(5, quality))
    entry.easeFactor += 0.1 - (5 - q) * (0.08 + (5 - q) * 0.02)
    if (entry.easeFactor < 1.3) entry.easeFactor = 1.3

    if (q <= 2) {
      entry.interval = 1
      entry.lapses++
      entry.reps = 0
    } else if (q === 3) {
      entry.reps++
      if (entry.interval < 1) entry.interval = 1
      entry.interval *= 1.2
    } else if (q === 4) {
      entry.reps++
      if (entry.interval < 1) entry.interval = 1
      entry.interval *= entry.easeFactor
      sessionCorrect++
    } else {
      entry.reps++
      if (entry.interval < 1) entry.interval = 1
      entry.interval *= entry.easeFactor * 1.3
      sessionCorrect++
    }

    entry.lastRating = quality
    entry.due = new Date(Date.now() + entry.interval * 24 * 60 * 60 * 1000).toISOString()

    progress.cards[card.id] = entry
    progress.totalReviews++
    progress.lastReview = new Date().toISOString()

    sessionDone++
    reviewIdx++
    showAnswer = false

    if (reviewIdx >= reviewQueue.length) {
      mode = 'deck'
      saveProgress()
    } else {
      saveProgress()
    }
  }

  async function saveProgress() {
    try {
      await api()?.SaveFlashcardProgress(JSON.stringify(progress))
    } catch (e) {
      console.error('Failed to save progress:', e)
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (mode === 'review') {
      if (e.key === ' ' && !showAnswer) {
        e.preventDefault()
        flipCard()
      } else if (showAnswer) {
        if (e.key === '1') rateCard(1)
        else if (e.key === '2') rateCard(2)
        else if (e.key === '3') rateCard(3)
        else if (e.key === '4') rateCard(5)
      }
    }
    if (e.key === 'Escape') {
      if (mode === 'review' || mode === 'stats') {
        mode = 'deck'
      } else {
        dispatch('close')
      }
    }
  }

  function progressPct(): number {
    if (cards.length === 0) return 0
    return Math.round((masteredCards.length / cards.length) * 100)
  }
</script>

<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center items-center" style="background:rgba(0,0,0,0.55);backdrop-filter:blur(3px)"
  on:click|self={() => dispatch('close')} on:keydown={handleKeydown}>
  <div class="w-full max-w-lg bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden" style="max-height:85vh">

    {#if loading}
      <div class="flex items-center justify-center p-12">
        <span class="text-ctp-overlay1 text-sm">Loading flashcards...</span>
      </div>

    {:else if mode === 'deck'}
      <!-- Deck Overview -->
      <div class="flex items-center justify-between px-5 py-3 border-b border-ctp-surface0">
        <span class="text-sm font-semibold text-ctp-mauve">Flashcards</span>
        <div class="flex items-center gap-2">
          <!-- svelte-ignore a11y-click-events-have-key-events -->
          <span class="text-[12px] text-ctp-overlay1 cursor-pointer hover:text-ctp-text" on:click={() => { mode = 'stats' }}>Stats</span>
          <!-- svelte-ignore a11y-click-events-have-key-events -->
          <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer" on:click={() => dispatch('close')}>esc</kbd>
        </div>
      </div>
      <div class="flex-1 overflow-y-auto p-5 space-y-4">
        <!-- Stats cards -->
        <div class="grid grid-cols-2 gap-3">
          <div class="bg-ctp-surface0 rounded-lg p-3">
            <div class="text-[12px] text-ctp-overlay1 uppercase tracking-wider">Total Cards</div>
            <div class="text-xl font-semibold text-ctp-text">{cards.length}</div>
          </div>
          <div class="bg-ctp-surface0 rounded-lg p-3">
            <div class="text-[12px] text-ctp-overlay1 uppercase tracking-wider">Due Today</div>
            <div class="text-xl font-semibold text-ctp-yellow">{dueCards.length}</div>
          </div>
          <div class="bg-ctp-surface0 rounded-lg p-3">
            <div class="text-[12px] text-ctp-overlay1 uppercase tracking-wider">New</div>
            <div class="text-xl font-semibold text-ctp-blue">{newCards.length}</div>
          </div>
          <div class="bg-ctp-surface0 rounded-lg p-3">
            <div class="text-[12px] text-ctp-overlay1 uppercase tracking-wider">Mastered</div>
            <div class="text-xl font-semibold text-ctp-green">{masteredCards.length}</div>
          </div>
        </div>

        <!-- Mastery progress bar -->
        {#if cards.length > 0}
          <div>
            <div class="flex justify-between text-[12px] text-ctp-overlay1 mb-1">
              <span>Mastery Progress</span>
              <span>{progressPct()}%</span>
            </div>
            <div class="w-full h-2.5 bg-ctp-surface0 rounded-full overflow-hidden">
              <div class="h-full bg-ctp-green rounded-full transition-all duration-300"
                style="width:{progressPct()}%"></div>
            </div>
          </div>
        {/if}

        {#if cards.length === 0}
          <div class="text-center py-8">
            <p class="text-ctp-overlay1 text-sm">No flashcards found in this note.</p>
            <p class="text-ctp-overlay1 text-xs mt-2">Add Q:/A: pairs, definition lists (term :: def), or cloze deletions.</p>
          </div>
        {:else}
          <!-- Start Review Button -->
          <!-- svelte-ignore a11y-click-events-have-key-events -->
          <div class="bg-ctp-surface0 hover:bg-ctp-surface1 rounded-lg p-4 cursor-pointer transition-colors text-center"
            on:click={startReview}>
            {#if dueCards.length > 0}
              <div class="text-ctp-mauve font-semibold">Start Review</div>
              <div class="text-[12px] text-ctp-overlay1 mt-1">{dueCards.length} cards due</div>
            {:else}
              <div class="text-ctp-overlay1">No cards due right now</div>
              <div class="text-[12px] text-ctp-overlay1 mt-1">Check back later</div>
            {/if}
          </div>
        {/if}
      </div>

    {:else if mode === 'review'}
      <!-- Review Mode -->
      <div class="flex items-center justify-between px-5 py-3 border-b border-ctp-surface0">
        <span class="text-sm font-semibold text-ctp-mauve">Review</span>
        <span class="text-xs text-ctp-overlay1">{sessionDone + 1} / {sessionTotal}</span>
      </div>

      <!-- Progress bar -->
      <div class="px-5 pt-3">
        <div class="w-full h-1.5 bg-ctp-surface0 rounded-full overflow-hidden">
          <div class="h-full bg-ctp-mauve rounded-full transition-all duration-300"
            style="width:{sessionTotal > 0 ? (sessionDone / sessionTotal * 100) : 0}%"></div>
        </div>
      </div>

      {#if reviewIdx < reviewQueue.length}
        {@const card = reviewQueue[reviewIdx]}
        <div class="flex-1 flex flex-col items-center justify-center p-6">
          <!-- Flashcard -->
          <div class="w-full perspective-1000">
            <div class="relative w-full min-h-[200px] transition-transform duration-500"
              class:rotate-y-180={showAnswer}
              style="transform-style:preserve-3d">
              <!-- Front -->
              <!-- svelte-ignore a11y-click-events-have-key-events -->
              <div class="w-full min-h-[200px] bg-ctp-surface0 rounded-xl border border-ctp-surface1 p-6 flex flex-col items-center justify-center cursor-pointer"
                class:hidden={showAnswer}
                on:click={flipCard}>
                <div class="text-[12px] text-ctp-blue uppercase tracking-wider mb-3">Question</div>
                <div class="text-ctp-text text-center text-lg leading-relaxed">{card.front}</div>
                <div class="text-[12px] text-ctp-overlay1 mt-4">Click or press Space to reveal</div>
              </div>
              <!-- Back -->
              {#if showAnswer}
                <div class="w-full min-h-[200px] bg-ctp-surface0 rounded-xl border border-ctp-green/30 p-6 flex flex-col items-center justify-center">
                  <div class="text-[12px] text-ctp-green uppercase tracking-wider mb-3">Answer</div>
                  <div class="text-ctp-text text-center leading-relaxed whitespace-pre-wrap">{card.back}</div>
                </div>
              {/if}
            </div>
          </div>

          <!-- Rating Buttons -->
          {#if showAnswer}
            <div class="flex gap-2 mt-5 w-full">
              <!-- svelte-ignore a11y-click-events-have-key-events -->
              <button class="flex-1 py-2.5 rounded-lg bg-ctp-red/20 text-ctp-red text-sm font-medium hover:bg-ctp-red/30 transition-colors"
                on:click={() => rateCard(1)}>
                <div>Again</div>
                <div class="text-[12px] opacity-70">1</div>
              </button>
              <!-- svelte-ignore a11y-click-events-have-key-events -->
              <button class="flex-1 py-2.5 rounded-lg bg-ctp-yellow/20 text-ctp-yellow text-sm font-medium hover:bg-ctp-yellow/30 transition-colors"
                on:click={() => rateCard(3)}>
                <div>Hard</div>
                <div class="text-[12px] opacity-70">2</div>
              </button>
              <!-- svelte-ignore a11y-click-events-have-key-events -->
              <button class="flex-1 py-2.5 rounded-lg bg-ctp-blue/20 text-ctp-blue text-sm font-medium hover:bg-ctp-blue/30 transition-colors"
                on:click={() => rateCard(4)}>
                <div>Good</div>
                <div class="text-[12px] opacity-70">3</div>
              </button>
              <!-- svelte-ignore a11y-click-events-have-key-events -->
              <button class="flex-1 py-2.5 rounded-lg bg-ctp-green/20 text-ctp-green text-sm font-medium hover:bg-ctp-green/30 transition-colors"
                on:click={() => rateCard(5)}>
                <div>Easy</div>
                <div class="text-[12px] opacity-70">4</div>
              </button>
            </div>
          {/if}
        </div>
      {:else}
        <!-- Session Complete -->
        <div class="flex-1 flex flex-col items-center justify-center p-8">
          <div class="text-3xl mb-3 text-ctp-green font-bold">Done!</div>
          <div class="text-ctp-text mb-1">{sessionDone} cards reviewed</div>
          <div class="text-ctp-overlay1 text-sm">{sessionCorrect} correct ({sessionTotal > 0 ? Math.round(sessionCorrect / sessionTotal * 100) : 0}%)</div>
          <!-- svelte-ignore a11y-click-events-have-key-events -->
          <button class="mt-6 px-6 py-2 bg-ctp-surface0 hover:bg-ctp-surface1 rounded-lg text-ctp-text text-sm transition-colors"
            on:click={() => { mode = 'deck' }}>Back to Deck</button>
        </div>
      {/if}

    {:else if mode === 'stats'}
      <!-- Stats View -->
      <div class="flex items-center justify-between px-5 py-3 border-b border-ctp-surface0">
        <span class="text-sm font-semibold text-ctp-mauve">Flashcard Stats</span>
        <!-- svelte-ignore a11y-click-events-have-key-events -->
        <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer" on:click={() => { mode = 'deck' }}>back</kbd>
      </div>
      <div class="flex-1 overflow-y-auto p-5 space-y-4">
        <div class="grid grid-cols-2 gap-3">
          <div class="bg-ctp-surface0 rounded-lg p-3">
            <div class="text-[12px] text-ctp-overlay1 uppercase tracking-wider">Total Reviews</div>
            <div class="text-lg font-semibold text-ctp-text">{progress.totalReviews}</div>
          </div>
          <div class="bg-ctp-surface0 rounded-lg p-3">
            <div class="text-[12px] text-ctp-overlay1 uppercase tracking-wider">Streak</div>
            <div class="text-lg font-semibold text-ctp-peach">{progress.streakDays || 0} days</div>
          </div>
        </div>

        <!-- Cards by Difficulty -->
        <div>
          <h3 class="text-xs font-semibold text-ctp-blue uppercase tracking-wider mb-2">Cards by Difficulty</h3>
          {#each [
            { label: 'New', count: newCards.length, color: 'bg-ctp-blue' },
            { label: 'Easy', count: cards.filter(c => { const p = progress.cards[c.id]; return p && p.easeFactor >= 2.5 && p.reps > 0; }).length, color: 'bg-ctp-green' },
            { label: 'Good', count: cards.filter(c => { const p = progress.cards[c.id]; return p && p.easeFactor >= 1.8 && p.easeFactor < 2.5; }).length, color: 'bg-ctp-yellow' },
            { label: 'Hard', count: cards.filter(c => { const p = progress.cards[c.id]; return p && p.easeFactor < 1.8 && p.reps > 0; }).length, color: 'bg-ctp-red' },
          ] as entry}
            <div class="flex items-center gap-2 py-1">
              <span class="text-xs text-ctp-text w-12">{entry.label}</span>
              <div class="flex-1 h-3 bg-ctp-surface0 rounded-full overflow-hidden">
                <div class="h-full {entry.color} rounded-full transition-all"
                  style="width:{cards.length > 0 ? Math.round(entry.count / cards.length * 100) : 0}%"></div>
              </div>
              <span class="text-xs text-ctp-overlay1 w-8 text-right">{entry.count}</span>
            </div>
          {/each}
        </div>

        <!-- Mastery -->
        {#if cards.length > 0}
          <div>
            <h3 class="text-xs font-semibold text-ctp-blue uppercase tracking-wider mb-2">Mastery</h3>
            <div class="flex items-center gap-2">
              <div class="flex-1 h-4 bg-ctp-surface0 rounded-full overflow-hidden flex">
                <div class="h-full bg-ctp-green" style="width:{Math.round(masteredCards.length / cards.length * 100)}%"></div>
                <div class="h-full bg-ctp-mauve" style="width:{Math.round((cards.length - masteredCards.length - newCards.length) / cards.length * 100)}%"></div>
                <div class="h-full bg-ctp-blue" style="width:{Math.round(newCards.length / cards.length * 100)}%"></div>
              </div>
            </div>
            <div class="flex gap-4 mt-2 text-[12px] text-ctp-overlay1">
              <span><span class="inline-block w-2 h-2 rounded-full bg-ctp-green mr-1"></span>Mastered {Math.round(masteredCards.length / cards.length * 100)}%</span>
              <span><span class="inline-block w-2 h-2 rounded-full bg-ctp-mauve mr-1"></span>Learning {Math.round((cards.length - masteredCards.length - newCards.length) / cards.length * 100)}%</span>
              <span><span class="inline-block w-2 h-2 rounded-full bg-ctp-blue mr-1"></span>New {Math.round(newCards.length / cards.length * 100)}%</span>
            </div>
          </div>
        {/if}
      </div>
    {/if}
  </div>
</div>
