<script lang="ts">
  import { createEventDispatcher } from 'svelte'

  export let notePath: string = ''
  export let noteTitle: string = ''

  const dispatch = createEventDispatcher()
  const api = (window as any).go?.main?.GranitApp

  let format: 'html' | 'markdown' = 'html'
  let publishing = false
  let outputPath = ''
  let error = ''

  // Custom metadata fields.
  let customTitle = ''
  let customDesc = ''
  let customTags = ''
  let customDate = new Date().toISOString().slice(0, 10)

  $: customTitle = noteTitle || ''
  $: displayTitle = customTitle || noteTitle || 'Untitled'

  async function publish() {
    if (!api || !notePath) return
    publishing = true
    error = ''
    outputPath = ''
    try {
      const result = await api.PublishToBlog(notePath, format)
      outputPath = result
    } catch (e: any) {
      error = e?.message || 'Publishing failed'
    }
    publishing = false
  }

  function reset() {
    outputPath = ''
    error = ''
  }
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center pt-[10%]"
  style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)"
  on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-md h-fit bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-overlay overflow-hidden">
    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <div class="flex items-center gap-2">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-mauve)" stroke-width="1.5" stroke-linecap="round">
          <path d="M2 3h12M2 7h8M2 11h10M14 9l-2 4-2-4" />
        </svg>
        <span class="text-sm font-semibold text-ctp-text">Blog Publisher</span>
      </div>
      <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1 transition-colors"
        on:click={() => dispatch('close')}>esc</kbd>
    </div>

    {#if outputPath}
      <!-- Success state -->
      <div class="px-4 py-6">
        <div class="flex items-center gap-2 mb-4">
          <svg width="20" height="20" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-green)" stroke-width="2" stroke-linecap="round">
            <path d="M3 8l3 3 7-7" />
          </svg>
          <span class="text-sm font-semibold text-ctp-green">Published successfully!</span>
        </div>
        <div class="bg-ctp-surface0 rounded-lg px-3 py-2 mb-4">
          <div class="text-[12px] text-ctp-overlay1 mb-1">Output path</div>
          <div class="text-[13px] text-ctp-blue break-all">{outputPath}</div>
        </div>
        <button on:click={() => dispatch('close')}
          class="w-full text-[13px] font-medium bg-ctp-surface0 text-ctp-text px-4 py-2 rounded-lg hover:bg-ctp-surface1 transition-colors">
          Close
        </button>
      </div>
    {:else if error}
      <!-- Error state -->
      <div class="px-4 py-6">
        <div class="flex items-center gap-2 mb-4">
          <svg width="20" height="20" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-red)" stroke-width="2" stroke-linecap="round">
            <circle cx="8" cy="8" r="6" /><path d="M8 5v3m0 2.5v.5" />
          </svg>
          <span class="text-sm font-semibold text-ctp-red">Publishing failed</span>
        </div>
        <div class="text-[13px] text-ctp-red/80 bg-ctp-surface0 rounded-lg px-3 py-2 mb-4">{error}</div>
        <button on:click={reset}
          class="w-full text-[13px] font-medium bg-ctp-surface0 text-ctp-text px-4 py-2 rounded-lg hover:bg-ctp-surface1 transition-colors">
          Try Again
        </button>
      </div>
    {:else}
      <!-- Config form -->
      <div class="px-4 py-4 space-y-4">
        <!-- Note info -->
        <div>
          <div class="text-[12px] text-ctp-overlay1 mb-1">Note</div>
          <div class="text-sm text-ctp-blue font-medium truncate">{displayTitle}</div>
          <div class="text-[12px] text-ctp-overlay1 truncate">{notePath}</div>
        </div>

        <!-- Format selector -->
        <div>
          <div class="text-[12px] text-ctp-overlay1 mb-2">Format</div>
          <div class="flex gap-2">
            <button on:click={() => format = 'html'}
              class="flex-1 text-[13px] font-medium px-3 py-2 rounded-lg border transition-colors"
              class:bg-ctp-blue={format === 'html'}
              class:text-ctp-crust={format === 'html'}
              class:border-ctp-blue={format === 'html'}
              class:bg-ctp-surface0={format !== 'html'}
              class:text-ctp-text={format !== 'html'}
              class:border-ctp-surface1={format !== 'html'}
              class:hover:bg-ctp-surface1={format !== 'html'}>
              HTML
            </button>
            <button on:click={() => format = 'markdown'}
              class="flex-1 text-[13px] font-medium px-3 py-2 rounded-lg border transition-colors"
              class:bg-ctp-blue={format === 'markdown'}
              class:text-ctp-crust={format === 'markdown'}
              class:border-ctp-blue={format === 'markdown'}
              class:bg-ctp-surface0={format !== 'markdown'}
              class:text-ctp-text={format !== 'markdown'}
              class:border-ctp-surface1={format !== 'markdown'}
              class:hover:bg-ctp-surface1={format !== 'markdown'}>
              Clean Markdown
            </button>
          </div>
        </div>

        <!-- Metadata -->
        <div>
          <div class="text-[12px] text-ctp-overlay1 mb-2">Metadata</div>
          <div class="space-y-2">
            <input bind:value={customTitle} placeholder="Title"
              class="w-full px-3 py-1.5 text-[13px] bg-ctp-surface0 text-ctp-text rounded-md border border-ctp-surface1 outline-none focus:border-ctp-blue transition-colors" />
            <input bind:value={customDesc} placeholder="Description (optional)"
              class="w-full px-3 py-1.5 text-[13px] bg-ctp-surface0 text-ctp-text rounded-md border border-ctp-surface1 outline-none focus:border-ctp-blue transition-colors" />
            <div class="flex gap-2">
              <input bind:value={customTags} placeholder="Tags (comma-separated)"
                class="flex-1 px-3 py-1.5 text-[13px] bg-ctp-surface0 text-ctp-text rounded-md border border-ctp-surface1 outline-none focus:border-ctp-blue transition-colors" />
              <input type="date" bind:value={customDate}
                class="px-3 py-1.5 text-[13px] bg-ctp-surface0 text-ctp-text rounded-md border border-ctp-surface1 outline-none focus:border-ctp-blue transition-colors" />
            </div>
          </div>
        </div>

        <!-- Format info -->
        <div class="bg-ctp-surface0/50 rounded-lg px-3 py-2">
          <div class="text-[12px] text-ctp-overlay1">
            {#if format === 'html'}
              Exports as blog-ready HTML with embedded styles. Wikilinks are converted to HTML links.
            {:else}
              Exports as clean markdown with proper frontmatter. Wikilinks are converted to plain text.
            {/if}
          </div>
          <div class="text-[12px] text-ctp-overlay1 mt-1">Output: _blog/{format === 'html' ? '*.html' : '*.md'}</div>
        </div>

        <!-- Publish button -->
        <button on:click={publish} disabled={publishing || !notePath}
          class="w-full text-[13px] font-semibold bg-ctp-green text-ctp-crust px-4 py-2.5 rounded-lg hover:opacity-90 transition-opacity disabled:opacity-40">
          {#if publishing}
            Publishing...
          {:else}
            Publish
          {/if}
        </button>
      </div>
    {/if}
  </div>
</div>
