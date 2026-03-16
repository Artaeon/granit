<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'
  const dispatch = createEventDispatcher()
  const api = () => (window as any).go?.main?.GranitApp

  export let notePath: string = ''

  interface Question {
    question: string
    choices: string[]
    answer: number
    source: string
  }

  let questions: Question[] = []
  let loading = true
  let state: 'setup' | 'question' | 'feedback' | 'results' = 'setup'

  // Quiz session
  let currentIdx = 0
  let score = 0
  let answers: { selected: number; correct: boolean; timeTaken: number }[] = []
  let startTime = 0
  let questionStart = 0

  // Feedback
  let lastCorrect = false
  let lastAnswer = ''
  let feedbackFlash = ''

  // Question interaction
  let selectedChoice = -1

  onMount(async () => {
    await loadQuestions()
  })

  async function loadQuestions() {
    loading = true
    try {
      if (notePath) {
        questions = (await api()?.GetQuizQuestions(notePath)) || []
      }
    } catch (e) {
      console.error('Failed to load quiz questions:', e)
    }
    loading = false
  }

  function startQuiz() {
    if (questions.length === 0) return
    currentIdx = 0
    score = 0
    answers = []
    startTime = Date.now()
    questionStart = Date.now()
    selectedChoice = -1
    state = 'question'
  }

  function selectAnswer(idx: number) {
    if (state !== 'question') return
    selectedChoice = idx

    const q = questions[currentIdx]
    const correct = idx === q.answer
    const timeTaken = Date.now() - questionStart

    answers.push({ selected: idx, correct, timeTaken })
    if (correct) score++

    lastCorrect = correct
    lastAnswer = q.choices[q.answer] || ''

    feedbackFlash = correct ? 'correct' : 'incorrect'
    state = 'feedback'

    setTimeout(() => { feedbackFlash = '' }, 600)
  }

  function nextQuestion() {
    currentIdx++
    selectedChoice = -1
    questionStart = Date.now()

    if (currentIdx >= questions.length) {
      state = 'results'
    } else {
      state = 'question'
    }
  }

  function retryWrong() {
    const wrong: Question[] = []
    for (let i = 0; i < answers.length; i++) {
      if (!answers[i].correct) {
        wrong.push(questions[i])
      }
    }
    if (wrong.length > 0) {
      questions = wrong
      currentIdx = 0
      score = 0
      answers = []
      startTime = Date.now()
      questionStart = Date.now()
      selectedChoice = -1
      state = 'question'
    }
  }

  function newQuiz() {
    state = 'setup'
    loadQuestions()
  }

  function formatTime(ms: number): string {
    const s = Math.round(ms / 1000)
    const m = Math.floor(s / 60)
    const rem = s % 60
    if (m > 0) return `${m}m ${rem}s`
    return `${rem}s`
  }

  function scorePct(): number {
    if (questions.length === 0) return 0
    return Math.round((score / questions.length) * 100)
  }

  function scoreColor(): string {
    const pct = scorePct()
    if (pct >= 75) return 'text-ctp-green'
    if (pct >= 50) return 'text-ctp-yellow'
    return 'text-ctp-red'
  }

  function handleKeydown(e: KeyboardEvent) {
    if (state === 'question') {
      const q = questions[currentIdx]
      if (e.key === '1' && q.choices.length >= 1) selectAnswer(0)
      else if (e.key === '2' && q.choices.length >= 2) selectAnswer(1)
      else if (e.key === '3' && q.choices.length >= 3) selectAnswer(2)
      else if (e.key === '4' && q.choices.length >= 4) selectAnswer(3)
    } else if (state === 'feedback') {
      if (e.key === ' ' || e.key === 'Enter') {
        e.preventDefault()
        nextQuestion()
      }
    } else if (state === 'results') {
      if (e.key === 'r') retryWrong()
      else if (e.key === 'n') newQuiz()
    }
    if (e.key === 'Escape') {
      dispatch('close')
    }
  }

  const choiceLabels = ['A', 'B', 'C', 'D']
</script>

<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center items-center" style="background:rgba(0,0,0,0.55);backdrop-filter:blur(3px)"
  on:click|self={() => dispatch('close')} on:keydown={handleKeydown}>
  <div class="w-full max-w-lg bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden" style="max-height:85vh">

    {#if loading}
      <div class="flex items-center justify-center p-12">
        <span class="text-ctp-overlay0 text-sm">Generating quiz questions...</span>
      </div>

    {:else if state === 'setup'}
      <!-- Setup -->
      <div class="flex items-center justify-between px-5 py-3 border-b border-ctp-surface0">
        <span class="text-sm font-semibold text-ctp-mauve">Quiz Mode</span>
        <!-- svelte-ignore a11y-click-events-have-key-events -->
        <kbd class="text-[10px] text-ctp-overlay0 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer" on:click={() => dispatch('close')}>esc</kbd>
      </div>
      <div class="flex-1 overflow-y-auto p-5 space-y-4">
        <div class="text-ctp-text text-sm">Test your knowledge from your notes!</div>

        <div class="bg-ctp-surface0 rounded-lg p-4">
          <div class="text-[10px] text-ctp-overlay0 uppercase tracking-wider mb-2">Questions Available</div>
          <div class="text-2xl font-semibold text-ctp-peach">{questions.length}</div>
        </div>

        {#if questions.length === 0}
          <div class="text-center py-4">
            <p class="text-ctp-overlay0 text-sm">No quiz questions could be generated from this note.</p>
            <p class="text-ctp-overlay0 text-xs mt-2">Add headings, lists, bold terms, or definition pairs to generate questions.</p>
          </div>
        {:else}
          <!-- svelte-ignore a11y-click-events-have-key-events -->
          <button class="w-full py-3 rounded-lg bg-ctp-mauve/20 text-ctp-mauve font-semibold hover:bg-ctp-mauve/30 transition-colors"
            on:click={startQuiz}>
            Start Quiz ({questions.length} questions)
          </button>
        {/if}
      </div>

    {:else if state === 'question'}
      <!-- Question -->
      {@const q = questions[currentIdx]}
      <div class="flex items-center justify-between px-5 py-3 border-b border-ctp-surface0">
        <span class="text-sm font-semibold text-ctp-mauve">Question {currentIdx + 1} / {questions.length}</span>
        <span class="text-xs text-ctp-overlay0">Score: {score}</span>
      </div>

      <!-- Progress bar -->
      <div class="px-5 pt-3">
        <div class="w-full h-1.5 bg-ctp-surface0 rounded-full overflow-hidden">
          <div class="h-full bg-ctp-mauve rounded-full transition-all duration-300"
            style="width:{((currentIdx + 1) / questions.length * 100)}%"></div>
        </div>
      </div>

      <div class="flex-1 p-5 space-y-4">
        <!-- Question text -->
        <div class="bg-ctp-surface0 rounded-lg p-4">
          <div class="text-ctp-text leading-relaxed">{q.question}</div>
        </div>

        <!-- Choices -->
        <div class="space-y-2">
          {#each q.choices as choice, i}
            <!-- svelte-ignore a11y-click-events-have-key-events -->
            <div class="flex items-center gap-3 p-3 rounded-lg bg-ctp-base border border-ctp-surface0 cursor-pointer hover:border-ctp-mauve/50 transition-colors"
              class:border-ctp-mauve={selectedChoice === i}
              on:click={() => selectAnswer(i)}>
              <span class="w-7 h-7 rounded-full bg-ctp-surface0 flex items-center justify-center text-xs font-semibold text-ctp-overlay0 shrink-0">
                {choiceLabels[i] || i + 1}
              </span>
              <span class="text-sm text-ctp-text">{choice}</span>
            </div>
          {/each}
        </div>

        <div class="text-[10px] text-ctp-overlay0 text-center">Press 1-{q.choices.length} to select</div>
      </div>

    {:else if state === 'feedback'}
      <!-- Feedback -->
      <div class="flex items-center justify-between px-5 py-3 border-b border-ctp-surface0">
        <span class="text-sm font-semibold text-ctp-mauve">Question {currentIdx + 1} / {questions.length}</span>
        <span class="text-xs text-ctp-overlay0">Score: {score}</span>
      </div>

      <div class="flex-1 flex flex-col items-center justify-center p-8"
        style="{feedbackFlash === 'correct' ? 'background: color-mix(in srgb, var(--ctp-green) 8%, transparent)' : feedbackFlash === 'incorrect' ? 'background: color-mix(in srgb, var(--ctp-red) 8%, transparent)' : ''}">
        {#if lastCorrect}
          <div class="text-4xl mb-3 text-ctp-green font-bold">Correct!</div>
        {:else}
          <div class="text-4xl mb-3 text-ctp-red font-bold">Incorrect</div>
          <div class="mt-2 text-sm">
            <span class="text-ctp-yellow font-semibold">Correct answer: </span>
            <span class="text-ctp-text">{lastAnswer}</span>
          </div>
        {/if}

        <div class="mt-6 text-[10px] text-ctp-overlay0">Press Space to continue</div>
        <!-- svelte-ignore a11y-click-events-have-key-events -->
        <button class="mt-3 px-6 py-2 bg-ctp-surface0 hover:bg-ctp-surface1 rounded-lg text-ctp-text text-sm transition-colors"
          on:click={nextQuestion}>Continue</button>
      </div>

    {:else if state === 'results'}
      <!-- Results -->
      <div class="flex items-center justify-between px-5 py-3 border-b border-ctp-surface0">
        <span class="text-sm font-semibold text-ctp-mauve">Quiz Results</span>
        <!-- svelte-ignore a11y-click-events-have-key-events -->
        <kbd class="text-[10px] text-ctp-overlay0 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer" on:click={() => dispatch('close')}>esc</kbd>
      </div>

      <div class="flex-1 overflow-y-auto p-5 space-y-4">
        <!-- Score summary -->
        <div class="text-center py-4">
          <div class="text-4xl font-bold {scoreColor()}">{scorePct()}%</div>
          <div class="text-ctp-text mt-1">{score} / {questions.length} correct</div>
          <div class="text-ctp-overlay0 text-sm mt-1">Time: {formatTime(Date.now() - startTime)}</div>
        </div>

        <!-- Question-by-question breakdown -->
        <div>
          <h3 class="text-xs font-semibold text-ctp-blue uppercase tracking-wider mb-2">Answers</h3>
          <div class="space-y-1">
            {#each answers as a, i}
              <div class="flex items-center gap-2 py-1.5 px-2 rounded text-sm">
                {#if a.correct}
                  <span class="text-ctp-green text-xs shrink-0">&#10004;</span>
                {:else}
                  <span class="text-ctp-red text-xs shrink-0">&#10008;</span>
                {/if}
                <span class="text-ctp-overlay0 truncate flex-1">{questions[i]?.question || ''}</span>
              </div>
            {/each}
          </div>
        </div>

        <!-- Action buttons -->
        <div class="flex gap-2">
          {#if answers.some(a => !a.correct)}
            <!-- svelte-ignore a11y-click-events-have-key-events -->
            <button class="flex-1 py-2.5 rounded-lg bg-ctp-yellow/20 text-ctp-yellow text-sm font-medium hover:bg-ctp-yellow/30 transition-colors"
              on:click={retryWrong}>
              Retry Wrong (r)
            </button>
          {/if}
          <!-- svelte-ignore a11y-click-events-have-key-events -->
          <button class="flex-1 py-2.5 rounded-lg bg-ctp-blue/20 text-ctp-blue text-sm font-medium hover:bg-ctp-blue/30 transition-colors"
            on:click={newQuiz}>
            New Quiz (n)
          </button>
        </div>
      </div>
    {/if}
  </div>
</div>
