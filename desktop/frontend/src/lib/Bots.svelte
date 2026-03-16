<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  export let bots: any[] = []
  const dispatch = createEventDispatcher()

  let step: 'list' | 'input' | 'loading' | 'result' = 'list'
  let selectedBot = -1
  let question = ''
  let result: any = null
  let error = ''

  function selectBot(kind: number) {
    selectedBot = kind
    if (kind === 3) { step = 'input'; question = '' }
    else { step = 'loading'; dispatch('run', { kind, question: '' }) }
  }
  function submitQuestion() { step = 'loading'; dispatch('run', { kind: selectedBot, question }) }
  export function setResult(r: any) { result = r; step = 'result'; error = '' }
  export function setError(e: string) { error = e; step = 'list' }
</script>

<div class="fixed inset-0 z-50 flex justify-center pt-[8%]" style="background:rgba(0,0,0,0.5);backdrop-filter:blur(2px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-xl h-[70vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden">
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <div class="flex items-center gap-2">
        {#if step !== 'list'}
          <button on:click={() => { step = 'list'; error = '' }} class="text-xs text-ctp-overlay1 hover:text-ctp-text">&larr;</button>
        {/if}
        <span class="text-sm font-semibold text-ctp-text">AI Bots</span>
        <span class="text-[10px] px-1.5 py-0.5 rounded bg-ctp-surface0 text-ctp-overlay0">
          {#if step === 'loading'}running...{:else}{bots.length} bots{/if}
        </span>
      </div>
      <kbd class="text-[10px] text-ctp-overlay0 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer" on:click={() => dispatch('close')}>esc</kbd>
    </div>

    {#if error}
      <div class="px-4 py-2 text-xs text-ctp-red bg-ctp-surface0">{error}</div>
    {/if}

    <div class="flex-1 overflow-y-auto py-1">
      {#if step === 'list'}
        {#each bots as bot}
          <button on:click={() => selectBot(bot.kind)}
            class="w-full flex items-center gap-3 px-4 py-3 hover:bg-ctp-surface0 transition-colors text-left">
            <span class="w-8 h-8 rounded-lg bg-ctp-surface0 flex items-center justify-center text-ctp-mauve text-sm">
              {bot.kind === 0 ? '#' : bot.kind === 1 ? '~' : bot.kind === 2 ? 'S' : bot.kind === 3 ? '?' : bot.kind === 4 ? 'W' : bot.kind === 5 ? 'T' : bot.kind === 6 ? 'A' : bot.kind === 7 ? 'M' : 'D'}
            </span>
            <div>
              <div class="text-sm text-ctp-text">{bot.name}</div>
              <div class="text-[10px] text-ctp-overlay0">{bot.desc}</div>
            </div>
          </button>
        {/each}

      {:else if step === 'input'}
        <div class="p-4 space-y-3">
          <p class="text-sm text-ctp-subtext0">Ask a question about your notes:</p>
          <textarea bind:value={question} placeholder="What would you like to know?"
            class="w-full h-32 px-3 py-2 bg-ctp-surface0 text-ctp-text rounded-lg border border-ctp-surface1 focus:border-ctp-blue outline-none text-sm resize-none" autofocus></textarea>
          <button on:click={submitQuestion} disabled={!question.trim()}
            class="px-4 py-2 bg-ctp-blue text-ctp-crust rounded-lg text-sm font-medium hover:opacity-90 disabled:opacity-50">Ask</button>
        </div>

      {:else if step === 'loading'}
        <div class="flex items-center justify-center h-full">
          <div class="text-center space-y-3">
            <div class="text-2xl animate-pulse text-ctp-mauve">...</div>
            <p class="text-sm text-ctp-overlay0">Thinking...</p>
          </div>
        </div>

      {:else if step === 'result' && result}
        <div class="p-4 space-y-3">
          {#if result.tags?.length}
            <div>
              <h4 class="text-xs text-ctp-mauve font-semibold mb-1">Suggested Tags</h4>
              <div class="flex flex-wrap gap-1">
                {#each result.tags as tag}
                  <span class="text-xs bg-ctp-surface0 text-ctp-blue px-2 py-0.5 rounded-full">#{tag}</span>
                {/each}
              </div>
            </div>
          {/if}
          {#if result.links?.length}
            <div>
              <h4 class="text-xs text-ctp-mauve font-semibold mb-1">Suggested Links</h4>
              {#each result.links as link}
                <div class="text-sm text-ctp-blue py-0.5">[[{link}]]</div>
              {/each}
            </div>
          {/if}
          <div>
            <h4 class="text-xs text-ctp-mauve font-semibold mb-1">Response</h4>
            <pre class="text-sm text-ctp-text whitespace-pre-wrap leading-relaxed">{result.response}</pre>
          </div>
        </div>
      {/if}
    </div>
  </div>
</div>
