<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'
  const dispatch = createEventDispatcher()
  const api = () => (window as any).go?.main?.GranitApp

  interface Prompt {
    category: string
    text: string
  }

  let prompts: Prompt[] = []
  let filteredPrompts: Prompt[] = []
  let currentIndex = 0
  let activeCategory = 'All'
  let loading = true

  const categoryColors: Record<string, string> = {
    'Gratitude': 'var(--ctp-green)',
    'Reflection': 'var(--ctp-blue)',
    'Creativity': 'var(--ctp-mauve)',
    'Goals': 'var(--ctp-yellow)',
    'Mindfulness': 'var(--ctp-teal)',
  }

  $: categories = ['All', ...new Set(prompts.map(p => p.category))]
  $: currentPrompt = filteredPrompts[currentIndex] || null
  $: catColor = currentPrompt ? (categoryColors[currentPrompt.category] || 'var(--ctp-text)') : 'var(--ctp-overlay0)'

  function filterByCategory(cat: string) {
    activeCategory = cat
    if (cat === 'All') {
      filteredPrompts = prompts
    } else {
      filteredPrompts = prompts.filter(p => p.category === cat)
    }
    currentIndex = Math.floor(Math.random() * filteredPrompts.length)
  }

  function nextPrompt() {
    if (filteredPrompts.length <= 1) return
    let next = currentIndex
    while (next === currentIndex) {
      next = Math.floor(Math.random() * filteredPrompts.length)
    }
    currentIndex = next
  }

  function usePrompt() {
    if (!currentPrompt) return
    const title = currentPrompt.text.slice(0, 60).replace(/[?]/g, '').trim()
    const content = `---\ndate: ${new Date().toISOString().slice(0, 10)}\ntype: journal\ntags: [journal, ${currentPrompt.category.toLowerCase()}]\n---\n\n# ${currentPrompt.text}\n\n`
    dispatch('createNote', { title, content })
    dispatch('close')
  }

  async function loadPrompts() {
    loading = true
    try {
      const result = await api()?.GetJournalPrompts()
      if (result && result.length > 0) {
        prompts = result
      }
    } catch {
      // Use fallback prompts
    }

    if (prompts.length === 0) {
      prompts = [
        { category: 'Gratitude', text: "What are 3 things you're grateful for today?" },
        { category: 'Gratitude', text: "Who is someone you appreciate but haven't thanked recently?" },
        { category: 'Reflection', text: 'What was the most meaningful moment of your day?' },
        { category: 'Reflection', text: 'What surprised you today?' },
        { category: 'Creativity', text: 'If you could create anything tomorrow, what would it be?' },
        { category: 'Creativity', text: 'What idea has been nagging at you that you haven\'t explored yet?' },
        { category: 'Goals', text: "What's one step you can take tomorrow toward your biggest goal?" },
        { category: 'Goals', text: 'What does success look like for you right now?' },
        { category: 'Mindfulness', text: 'What emotions did you experience most strongly today?' },
        { category: 'Mindfulness', text: 'Describe the present moment using all five senses.' },
      ]
    }

    filteredPrompts = prompts
    currentIndex = Math.floor(Math.random() * filteredPrompts.length)
    loading = false
  }

  onMount(() => { loadPrompts() })
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center pt-[6%]" style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-lg bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden" style="max-height:85vh">
    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <div class="flex items-center gap-2">
        <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-mauve)" stroke-width="1.5" stroke-linecap="round">
          <path d="M3 2h10a1 1 0 011 1v10a1 1 0 01-1 1H3a1 1 0 01-1-1V3a1 1 0 011-1z" />
          <path d="M5 6h6M5 9h4" />
        </svg>
        <span class="text-sm font-semibold text-ctp-mauve">Journal Prompts</span>
      </div>
      <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1 transition-colors"
        on:click={() => dispatch('close')}>esc</kbd>
    </div>

    {#if loading}
      <div class="flex flex-col items-center justify-center py-16 gap-3">
        <div class="w-6 h-6 border-2 border-ctp-mauve border-t-transparent rounded-full animate-spin"></div>
        <span class="text-sm text-ctp-overlay1">Loading prompts...</span>
      </div>
    {:else}
      <!-- Category tabs -->
      <div class="px-4 py-2 border-b border-ctp-surface0 flex gap-1 flex-wrap">
        {#each categories as cat}
          <button on:click={() => filterByCategory(cat)}
            class="px-2.5 py-1 rounded-full text-[13px] font-medium transition-colors
              {activeCategory === cat
                ? 'text-ctp-crust'
                : 'text-ctp-overlay1 hover:text-ctp-text hover:bg-ctp-surface0'}"
            style={activeCategory === cat ? `background: ${categoryColors[cat] || 'var(--ctp-mauve)'}` : ''}>
            {cat}
          </button>
        {/each}
      </div>

      <!-- Current prompt -->
      <div class="flex-1 flex flex-col items-center justify-center p-8">
        {#if currentPrompt}
          <!-- Category badge -->
          <div class="mb-4">
            <span class="px-2.5 py-1 rounded-full text-[12px] font-medium text-ctp-crust"
              style="background: {catColor}">
              {currentPrompt.category}
            </span>
          </div>

          <!-- Prompt text -->
          <div class="text-center px-4">
            <p class="text-lg font-medium text-ctp-text leading-relaxed">
              "{currentPrompt.text}"
            </p>
          </div>

          <!-- Prompt counter -->
          <div class="mt-4 text-[12px] text-ctp-overlay1">
            {currentIndex + 1} of {filteredPrompts.length}
          </div>
        {:else}
          <p class="text-sm text-ctp-overlay1">No prompts available</p>
        {/if}
      </div>

      <!-- Actions -->
      <div class="px-4 py-4 border-t border-ctp-surface0 flex items-center justify-center gap-3">
        <button on:click={nextPrompt}
          class="flex items-center gap-1.5 px-4 py-2 bg-ctp-surface0 text-ctp-text text-sm rounded-lg hover:bg-ctp-surface1 transition-colors">
          <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
            <path d="M1 8h14M10 3l5 5-5 5" />
          </svg>
          Next Prompt
        </button>
        <button on:click={usePrompt}
          class="flex items-center gap-1.5 px-4 py-2 bg-ctp-mauve text-ctp-crust text-sm font-medium rounded-lg hover:opacity-90 transition-opacity">
          <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
            <path d="M3 2h10a1 1 0 011 1v10a1 1 0 01-1 1H3a1 1 0 01-1-1V3a1 1 0 011-1z" />
            <path d="M8 5v6M5 8h6" />
          </svg>
          Use This Prompt
        </button>
      </div>

      <!-- All prompts list -->
      <div class="border-t border-ctp-surface0 max-h-48 overflow-y-auto">
        {#each filteredPrompts as prompt, i}
          <button on:click={() => currentIndex = i}
            class="w-full text-left px-4 py-2 hover:bg-ctp-surface0/50 transition-colors flex items-start gap-2
              {i === currentIndex ? 'bg-ctp-surface0/30' : ''}">
            <span class="w-1.5 h-1.5 rounded-full flex-shrink-0 mt-1.5" style="background: {categoryColors[prompt.category] || 'var(--ctp-overlay0)'}"></span>
            <span class="text-xs text-ctp-text leading-relaxed">{prompt.text}</span>
          </button>
        {/each}
      </div>
    {/if}
  </div>
</div>
