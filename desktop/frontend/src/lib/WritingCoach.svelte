<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  export let content: string = ''
  const dispatch = createEventDispatcher()
  const api = () => (window as any).go?.main?.GranitApp

  interface FeedbackItem {
    category: string
    severity: number
    issue: string
    suggestion: string
  }

  let loading = false
  let error = ''
  let wordCount = 0
  let sentenceCount = 0
  let paragraphCount = 0
  let readabilityScore = 0
  let feedback: FeedbackItem[] = []
  let analyzed = false

  // Local stats (computed immediately)
  $: {
    const words = content.trim().split(/\s+/).filter(Boolean)
    wordCount = words.length

    let sc = 0
    for (const ch of content) {
      if (ch === '.' || ch === '!' || ch === '?') sc++
    }
    sentenceCount = Math.max(sc, 1)

    const paras = content.split(/\n\n+/).filter(p => p.trim())
    paragraphCount = Math.max(paras.length, 1)

    const avgWps = wordCount / sentenceCount
    readabilityScore = avgWps > 25 ? 50 : avgWps > 20 ? 60 : avgWps > 15 ? 70 : 80
  }

  $: avgSentenceLength = sentenceCount > 0 ? Math.round(wordCount / sentenceCount) : 0

  const severityColors: Record<number, string> = {
    1: 'var(--ctp-green)',
    2: 'var(--ctp-yellow)',
    3: 'var(--ctp-red)',
  }

  const severityLabels: Record<number, string> = {
    1: 'minor',
    2: 'moderate',
    3: 'major',
  }

  const categoryIcons: Record<string, string> = {
    'clarity': 'eye',
    'structure': 'layout',
    'style': 'pen',
    'grammar': 'check',
    'tone': 'message',
  }

  function readabilityLabel(score: number): string {
    if (score >= 80) return 'Easy'
    if (score >= 60) return 'Moderate'
    if (score >= 40) return 'Difficult'
    return 'Very Difficult'
  }

  function readabilityColor(score: number): string {
    if (score >= 80) return 'var(--ctp-green)'
    if (score >= 60) return 'var(--ctp-yellow)'
    if (score >= 40) return 'var(--ctp-peach)'
    return 'var(--ctp-red)'
  }

  async function runAnalysis() {
    if (!content.trim()) {
      error = 'No content to analyze'
      return
    }

    loading = true
    error = ''
    feedback = []

    try {
      const result = await api()?.GetWritingFeedback(content)
      if (result) {
        try {
          const parsed = JSON.parse(result)
          if (parsed.wordCount) wordCount = parsed.wordCount
          if (parsed.sentenceCount) sentenceCount = parsed.sentenceCount
          if (parsed.paragraphCount) paragraphCount = parsed.paragraphCount
          if (parsed.readabilityScore) readabilityScore = parsed.readabilityScore
          if (parsed.feedback) feedback = parsed.feedback
        } catch {
          // If not JSON, treat as raw text feedback
          feedback = [{ category: 'style', severity: 1, issue: 'AI Response', suggestion: result }]
        }
      }
      analyzed = true
    } catch (e) {
      error = String(e)
    }

    loading = false
  }
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center pt-[6%]" style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-2xl bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden" style="max-height:85vh">
    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <div class="flex items-center gap-2">
        <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-mauve)" stroke-width="1.5" stroke-linecap="round">
          <path d="M2 12l3-3 2 2 4-4 3 3" /><rect x="1" y="1" width="14" height="14" rx="2" />
        </svg>
        <span class="text-sm font-semibold text-ctp-mauve">Writing Coach</span>
      </div>
      <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1 transition-colors"
        on:click={() => dispatch('close')}>esc</kbd>
    </div>

    <!-- Content -->
    <div class="flex-1 overflow-y-auto p-4 space-y-4">
      <!-- Stats overview -->
      <div class="grid grid-cols-4 gap-3">
        <div class="bg-ctp-base rounded-lg p-3 border border-ctp-surface0 text-center">
          <div class="text-xl font-bold text-ctp-blue">{wordCount}</div>
          <div class="text-[12px] text-ctp-overlay1 uppercase tracking-wide mt-0.5">Words</div>
        </div>
        <div class="bg-ctp-base rounded-lg p-3 border border-ctp-surface0 text-center">
          <div class="text-xl font-bold text-ctp-teal">{sentenceCount}</div>
          <div class="text-[12px] text-ctp-overlay1 uppercase tracking-wide mt-0.5">Sentences</div>
        </div>
        <div class="bg-ctp-base rounded-lg p-3 border border-ctp-surface0 text-center">
          <div class="text-xl font-bold text-ctp-peach">{paragraphCount}</div>
          <div class="text-[12px] text-ctp-overlay1 uppercase tracking-wide mt-0.5">Paragraphs</div>
        </div>
        <div class="bg-ctp-base rounded-lg p-3 border border-ctp-surface0 text-center">
          <div class="text-xl font-bold" style="color: {readabilityColor(readabilityScore)}">{readabilityScore}</div>
          <div class="text-[12px] text-ctp-overlay1 uppercase tracking-wide mt-0.5">Readability</div>
        </div>
      </div>

      <!-- Readability details -->
      <div class="bg-ctp-base rounded-lg p-3 border border-ctp-surface0">
        <div class="flex items-center justify-between mb-2">
          <span class="text-xs text-ctp-overlay1">Readability Score</span>
          <span class="text-xs font-medium" style="color: {readabilityColor(readabilityScore)}">
            {readabilityLabel(readabilityScore)}
          </span>
        </div>
        <div class="w-full h-2 bg-ctp-surface0 rounded-full overflow-hidden">
          <div class="h-full rounded-full transition-all duration-500"
            style="width: {readabilityScore}%; background: {readabilityColor(readabilityScore)}"></div>
        </div>
        <div class="flex items-center justify-between mt-2 text-[12px] text-ctp-overlay1">
          <span>Avg. {avgSentenceLength} words/sentence</span>
          <span>{Math.round(wordCount / Math.max(paragraphCount, 1))} words/paragraph</span>
        </div>
      </div>

      <!-- Analyze button -->
      {#if !analyzed && !loading}
        <div class="flex justify-center py-2">
          <button on:click={runAnalysis}
            class="flex items-center gap-2 px-6 py-2.5 bg-ctp-mauve text-ctp-crust text-sm font-medium rounded-lg hover:opacity-90 transition-opacity">
            <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
              <circle cx="8" cy="8" r="6" /><path d="M8 5v3l2 1" />
            </svg>
            Analyze Writing
          </button>
        </div>
      {/if}

      <!-- Loading -->
      {#if loading}
        <div class="flex flex-col items-center justify-center py-8 gap-3">
          <div class="w-6 h-6 border-2 border-ctp-mauve border-t-transparent rounded-full animate-spin"></div>
          <span class="text-sm text-ctp-overlay1">Analyzing your writing...</span>
          <span class="text-[12px] text-ctp-overlay1">This may take a moment with AI providers</span>
        </div>
      {/if}

      <!-- Error -->
      {#if error}
        <div class="bg-ctp-red/10 border border-ctp-red/30 rounded-lg p-3">
          <span class="text-xs text-ctp-red">{error}</span>
        </div>
      {/if}

      <!-- Feedback results -->
      {#if feedback.length > 0}
        <div>
          <div class="flex items-center justify-between mb-3">
            <h3 class="text-xs font-semibold text-ctp-mauve uppercase tracking-wider">Feedback</h3>
            <div class="flex items-center gap-3 text-[12px]">
              <span style="color: var(--ctp-green)">
                {feedback.filter(f => f.severity === 1).length} minor
              </span>
              <span style="color: var(--ctp-yellow)">
                {feedback.filter(f => f.severity === 2).length} moderate
              </span>
              <span style="color: var(--ctp-red)">
                {feedback.filter(f => f.severity === 3).length} major
              </span>
            </div>
          </div>

          <div class="space-y-2">
            {#each feedback as item}
              <div class="bg-ctp-base rounded-lg border border-ctp-surface0 p-3">
                <div class="flex items-center gap-2 mb-1.5">
                  <!-- Category badge -->
                  <span class="px-2 py-0.5 rounded text-[12px] font-medium bg-ctp-surface0 text-ctp-overlay1 capitalize">
                    {item.category}
                  </span>
                  <!-- Severity -->
                  <span class="text-[12px] font-medium" style="color: {severityColors[item.severity] || 'var(--ctp-overlay0)'}">
                    [{severityLabels[item.severity] || 'unknown'}]
                  </span>
                </div>

                <!-- Issue -->
                <p class="text-xs text-ctp-text mb-1">{item.issue}</p>

                <!-- Suggestion -->
                {#if item.suggestion}
                  <p class="text-[13px] text-ctp-teal italic">{item.suggestion}</p>
                {/if}
              </div>
            {/each}
          </div>
        </div>

        <!-- Re-analyze button -->
        <div class="flex justify-center py-2">
          <button on:click={runAnalysis}
            class="flex items-center gap-1.5 px-4 py-1.5 bg-ctp-surface0 text-ctp-overlay1 text-xs rounded-lg hover:bg-ctp-surface1 transition-colors">
            <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
              <path d="M2 8a6 6 0 0111.3-2.7M14 8a6 6 0 01-11.3 2.7" />
              <path d="M14 2v4h-4M2 14v-4h4" />
            </svg>
            Re-analyze
          </button>
        </div>
      {/if}

      <!-- Empty content warning -->
      {#if wordCount === 0}
        <div class="flex flex-col items-center py-8 gap-2">
          <svg width="24" height="24" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-surface2)" stroke-width="1" stroke-linecap="round" class="opacity-40">
            <path d="M3 2h10a1 1 0 011 1v10a1 1 0 01-1 1H3a1 1 0 01-1-1V3a1 1 0 011-1z" />
            <path d="M5 6h6M5 9h4" />
          </svg>
          <p class="text-sm text-ctp-overlay1">No content to analyze</p>
          <p class="text-[13px] text-ctp-overlay1">Write something in a note first</p>
        </div>
      {/if}
    </div>
  </div>
</div>
