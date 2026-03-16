<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'

  const dispatch = createEventDispatcher()
  const api = () => (window as any).go?.main?.GranitApp

  interface Snippet {
    trigger: string
    content: string
    description: string
  }

  let snippets: Snippet[] = []
  let userSnippets: Snippet[] = []
  let editIndex = -1
  let editTrigger = ''
  let editContent = ''
  let editDescription = ''
  let showAddForm = false
  let loading = true
  let error = ''
  let successMsg = ''

  onMount(async () => {
    await loadSnippets()
  })

  async function loadSnippets() {
    loading = true
    error = ''
    try {
      const result = await api()?.GetSnippets()
      if (result) {
        snippets = result
      }
    } catch (e: any) {
      error = e?.message || String(e)
    } finally {
      loading = false
    }
  }

  function startAdd() {
    showAddForm = true
    editIndex = -1
    editTrigger = '/'
    editContent = ''
    editDescription = ''
  }

  function startEdit(index: number) {
    const s = userSnippets[index]
    editIndex = index
    showAddForm = true
    editTrigger = s.trigger
    editContent = s.content
    editDescription = s.description
  }

  function cancelEdit() {
    showAddForm = false
    editIndex = -1
    editTrigger = ''
    editContent = ''
    editDescription = ''
  }

  async function saveSnippet() {
    if (!editTrigger.startsWith('/')) {
      error = 'Trigger must start with /'
      return
    }
    if (!editContent.trim()) {
      error = 'Content is required'
      return
    }
    error = ''

    const newSnippet: Snippet = {
      trigger: editTrigger.trim(),
      content: editContent,
      description: editDescription.trim() || editTrigger.trim(),
    }

    if (editIndex >= 0) {
      userSnippets[editIndex] = newSnippet
    } else {
      userSnippets = [...userSnippets, newSnippet]
    }

    await persistSnippets()
    cancelEdit()
    await loadSnippets()
  }

  async function deleteSnippet(index: number) {
    userSnippets = userSnippets.filter((_, i) => i !== index)
    await persistSnippets()
    await loadSnippets()
  }

  async function persistSnippets() {
    try {
      await api()?.SaveSnippets(JSON.stringify(userSnippets))
      successMsg = 'Snippets saved'
      setTimeout(() => { successMsg = '' }, 2000)
    } catch (e: any) {
      error = e?.message || String(e)
    }
  }

  function exportSnippets() {
    const data = JSON.stringify(userSnippets, null, 2)
    const blob = new Blob([data], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'granit-snippets.json'
    a.click()
    URL.revokeObjectURL(url)
  }

  function importSnippets() {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = '.json'
    input.onchange = async (e: any) => {
      const file = e.target?.files?.[0]
      if (!file) return
      try {
        const text = await file.text()
        const imported: Snippet[] = JSON.parse(text)
        if (!Array.isArray(imported)) {
          error = 'Invalid snippet file format'
          return
        }
        userSnippets = [...userSnippets, ...imported]
        await persistSnippets()
        await loadSnippets()
        successMsg = `Imported ${imported.length} snippets`
        setTimeout(() => { successMsg = '' }, 2000)
      } catch (e: any) {
        error = 'Failed to parse import file'
      }
    }
    input.click()
  }

  // Separate built-in from user snippets.
  // Built-in triggers for comparison.
  const builtinTriggers = new Set([
    '/date', '/time', '/datetime', '/todo', '/done', '/h1', '/h2', '/h3',
    '/link', '/code', '/table', '/meeting', '/daily', '/callout', '/divider',
    '/quote', '/img', '/frontmatter'
  ])

  $: builtinList = snippets.filter(s => builtinTriggers.has(s.trigger))
  $: customList = snippets.filter(s => !builtinTriggers.has(s.trigger))
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center pt-[8%]"
  style="background: rgba(17,17,27,0.6); backdrop-filter: blur(8px);"
  on:click|self={() => dispatch('close')}>

  <div class="w-full max-w-xl h-[75vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden">

    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <div class="flex items-center gap-2">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-mauve)" stroke-width="1.5" stroke-linecap="round">
          <path d="M3 3h4l2 10H5L3 3zM9 3h4l-2 10h-4" />
        </svg>
        <span class="text-sm font-semibold text-ctp-text">Snippets</span>
        <span class="text-[12px] px-1.5 py-0.5 rounded bg-ctp-surface0 text-ctp-overlay1">{snippets.length} snippets</span>
      </div>
      <div class="flex items-center gap-2">
        <button on:click={importSnippets}
          class="text-[12px] text-ctp-overlay1 hover:text-ctp-blue px-2 py-0.5 rounded bg-ctp-surface0 transition-colors">
          Import
        </button>
        <button on:click={exportSnippets}
          class="text-[12px] text-ctp-overlay1 hover:text-ctp-blue px-2 py-0.5 rounded bg-ctp-surface0 transition-colors">
          Export
        </button>
        <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer" on:click={() => dispatch('close')}>esc</kbd>
      </div>
    </div>

    {#if error}
      <div class="px-4 py-2 text-xs text-ctp-red bg-ctp-surface0">{error}</div>
    {/if}
    {#if successMsg}
      <div class="px-4 py-2 text-xs text-ctp-green bg-ctp-surface0">{successMsg}</div>
    {/if}

    <!-- Content -->
    <div class="flex-1 overflow-y-auto">
      {#if loading}
        <div class="flex items-center justify-center h-32">
          <div class="text-sm text-ctp-overlay1 animate-pulse">Loading snippets...</div>
        </div>

      {:else}
        <!-- Usage instructions -->
        <div class="px-4 py-3 border-b border-ctp-surface0">
          <div class="text-[13px] text-ctp-overlay1 space-y-1">
            <p>Type a snippet trigger (e.g. <code class="bg-ctp-surface0 px-1 rounded text-ctp-peach">/date</code>) and press <kbd class="bg-ctp-surface0 px-1 py-px rounded">Tab</kbd> to expand it.</p>
            <p>Placeholders: <code class="bg-ctp-surface0 px-1 rounded text-ctp-peach">{'{{date}}'}</code> <code class="bg-ctp-surface0 px-1 rounded text-ctp-peach">{'{{time}}'}</code> <code class="bg-ctp-surface0 px-1 rounded text-ctp-peach">{'{{datetime}}'}</code></p>
          </div>
        </div>

        <!-- Add / Edit form -->
        {#if showAddForm}
          <div class="px-4 py-3 border-b border-ctp-surface0 space-y-3">
            <div class="text-xs font-semibold text-ctp-mauve">{editIndex >= 0 ? 'Edit Snippet' : 'New Snippet'}</div>
            <div class="grid grid-cols-2 gap-2">
              <div>
                <label class="text-[12px] text-ctp-overlay1 block mb-1">Trigger</label>
                <input bind:value={editTrigger} placeholder="/trigger"
                  class="w-full px-2 py-1.5 bg-ctp-surface0 text-ctp-text rounded-lg border border-ctp-surface1 focus:border-ctp-blue outline-none text-xs font-mono" />
              </div>
              <div>
                <label class="text-[12px] text-ctp-overlay1 block mb-1">Description</label>
                <input bind:value={editDescription} placeholder="Brief description"
                  class="w-full px-2 py-1.5 bg-ctp-surface0 text-ctp-text rounded-lg border border-ctp-surface1 focus:border-ctp-blue outline-none text-xs" />
              </div>
            </div>
            <div>
              <label class="text-[12px] text-ctp-overlay1 block mb-1">Content</label>
              <textarea bind:value={editContent} placeholder="Expanded content..."
                rows="4"
                class="w-full px-2 py-1.5 bg-ctp-surface0 text-ctp-text rounded-lg border border-ctp-surface1 focus:border-ctp-blue outline-none text-xs font-mono resize-none"></textarea>
            </div>
            <div class="flex gap-2">
              <button on:click={saveSnippet}
                class="px-3 py-1.5 bg-ctp-blue text-ctp-crust rounded-lg text-xs font-medium hover:opacity-90">
                Save
              </button>
              <button on:click={cancelEdit}
                class="px-3 py-1.5 bg-ctp-surface0 text-ctp-text rounded-lg text-xs hover:bg-ctp-surface1">
                Cancel
              </button>
            </div>
          </div>
        {/if}

        <!-- Custom snippets -->
        {#if customList.length > 0}
          <div class="px-4 pt-3 pb-1">
            <div class="text-[12px] text-ctp-mauve font-semibold uppercase tracking-wider">Custom Snippets</div>
          </div>
          {#each customList as snippet, i}
            <div class="flex items-start gap-3 px-4 py-2 hover:bg-ctp-surface0/50 transition-colors group">
              <code class="text-xs text-ctp-peach bg-ctp-surface0 px-2 py-0.5 rounded font-mono flex-shrink-0 min-w-[80px]">
                {snippet.trigger}
              </code>
              <div class="flex-1 min-w-0">
                <div class="text-xs text-ctp-text">{snippet.description}</div>
                <pre class="text-[12px] text-ctp-overlay1 mt-1 truncate max-w-full">{snippet.content.slice(0, 80)}{snippet.content.length > 80 ? '...' : ''}</pre>
              </div>
              <div class="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity flex-shrink-0">
                <button on:click={() => startEdit(i)}
                  class="text-[12px] text-ctp-blue hover:text-ctp-text px-1.5 py-0.5 rounded bg-ctp-surface0">
                  Edit
                </button>
                <button on:click={() => deleteSnippet(i)}
                  class="text-[12px] text-ctp-red hover:text-ctp-text px-1.5 py-0.5 rounded bg-ctp-surface0">
                  Del
                </button>
              </div>
            </div>
          {/each}
        {/if}

        <!-- Built-in snippets -->
        <div class="px-4 pt-3 pb-1">
          <div class="text-[12px] text-ctp-mauve font-semibold uppercase tracking-wider">Built-in Snippets</div>
        </div>
        {#each builtinList as snippet}
          <div class="flex items-start gap-3 px-4 py-2 hover:bg-ctp-surface0/50 transition-colors">
            <code class="text-xs text-ctp-peach bg-ctp-surface0 px-2 py-0.5 rounded font-mono flex-shrink-0 min-w-[80px]">
              {snippet.trigger}
            </code>
            <div class="flex-1 min-w-0">
              <div class="text-xs text-ctp-text">{snippet.description}</div>
              <pre class="text-[12px] text-ctp-overlay1 mt-1 truncate max-w-full">{snippet.content.slice(0, 80)}{snippet.content.length > 80 ? '...' : ''}</pre>
            </div>
          </div>
        {/each}
      {/if}
    </div>

    <!-- Footer -->
    <div class="flex items-center justify-between px-4 py-2 border-t border-ctp-surface0">
      <button on:click={startAdd}
        class="px-3 py-1.5 bg-ctp-blue text-ctp-crust rounded-lg text-xs font-medium hover:opacity-90 flex items-center gap-1.5">
        <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <path d="M8 3v10M3 8h10" />
        </svg>
        Add Snippet
      </button>
      <div class="text-[12px] text-ctp-overlay1">
        <kbd class="bg-ctp-surface0 px-1 py-px rounded">Esc</kbd> close
      </div>
    </div>
  </div>
</div>
