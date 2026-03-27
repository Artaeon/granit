<script lang="ts">
  import { createEventDispatcher, onMount, tick } from 'svelte'
  import { chatWithAI } from './api'

  export let noteTitle: string = ''
  export let noteContent: string = ''

  const dispatch = createEventDispatcher()

  interface ChatMsg {
    role: 'user' | 'assistant' | 'system'
    content: string
    time: string
  }

  let messages: ChatMsg[] = [
    { role: 'system', content: 'Ask me anything about your notes.', time: now() }
  ]
  let input = ''
  let loading = false
  let error = ''
  let messagesEl: HTMLDivElement
  let inputEl: HTMLTextAreaElement

  function now(): string {
    return new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  }

  onMount(async () => {
    await tick()
    inputEl?.focus()
  })

  async function sendMessage() {
    const trimmed = input.trim()
    if (!trimmed || loading) return

    messages = [...messages, { role: 'user', content: trimmed, time: now() }]
    input = ''
    loading = true
    error = ''

    await scrollToBottom()

    try {
      // Build conversation payload (exclude system messages from display).
      const payload = messages
        .filter(m => m.role !== 'system')
        .map(m => ({ role: m.role, content: m.content }))

      const response = await chatWithAI(JSON.stringify(payload))
      if (response) {
        messages = [...messages, { role: 'assistant', content: response, time: now() }]
      } else {
        messages = [...messages, { role: 'assistant', content: 'No response received.', time: now() }]
      }
    } catch (e: any) {
      error = e?.message || String(e)
      messages = [...messages, { role: 'system', content: 'Error: ' + error, time: now() }]
    } finally {
      loading = false
      await scrollToBottom()
    }
  }

  function clearChat() {
    messages = [{ role: 'system', content: 'Chat cleared. Ask me anything about your notes.', time: now() }]
    error = ''
  }

  async function scrollToBottom() {
    await tick()
    if (messagesEl) {
      messagesEl.scrollTop = messagesEl.scrollHeight
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      dispatch('close')
      return
    }
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      sendMessage()
    }
  }

  function renderMarkdown(text: string): string {
    let html = text
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')

    // Code blocks.
    html = html.replace(/```(\w*)\n([\s\S]*?)```/g, (_m, lang, code) => {
      return `<pre class="bg-ctp-crust rounded-lg p-3 my-2 overflow-x-auto text-xs"><code class="language-${lang}">${code.trim()}</code></pre>`
    })
    // Inline code.
    html = html.replace(/`([^`]+)`/g, '<code class="bg-ctp-crust px-1.5 py-0.5 rounded text-ctp-peach text-xs">$1</code>')
    // Bold.
    html = html.replace(/\*\*(.+?)\*\*/g, '<strong class="text-ctp-text font-semibold">$1</strong>')
    // Italic.
    html = html.replace(/\*(.+?)\*/g, '<em>$1</em>')
    // Headings.
    html = html.replace(/^### (.+)$/gm, '<h3 class="text-sm font-semibold text-ctp-mauve mt-2 mb-1">$1</h3>')
    html = html.replace(/^## (.+)$/gm, '<h2 class="text-base font-semibold text-ctp-mauve mt-2 mb-1">$1</h2>')
    html = html.replace(/^# (.+)$/gm, '<h1 class="text-lg font-bold text-ctp-mauve mt-2 mb-1">$1</h1>')
    // Lists.
    html = html.replace(/^- (.+)$/gm, '<li class="ml-4 list-disc">$1</li>')
    html = html.replace(/^\d+\. (.+)$/gm, '<li class="ml-4 list-decimal">$1</li>')
    // Line breaks.
    html = html.replace(/\n/g, '<br>')

    return html
  }
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center pt-[6%]"
  style="background: rgba(17,17,27,0.6); backdrop-filter: blur(8px);"
  on:click|self={() => dispatch('close')}>

  <div class="w-full max-w-2xl h-[80vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden">

    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <div class="flex items-center gap-2">
        <svg width="18" height="18" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-mauve)" stroke-width="1.5" stroke-linecap="round">
          <path d="M2 3h12v8H4l-2 2V3z" />
        </svg>
        <span class="text-sm font-semibold text-ctp-text">AI Chat</span>
        {#if noteTitle}
          <span class="text-[12px] px-1.5 py-0.5 rounded bg-ctp-surface0 text-ctp-overlay1 truncate max-w-[200px]">{noteTitle}</span>
        {/if}
      </div>
      <div class="flex items-center gap-2">
        <button on:click={clearChat}
          class="text-[12px] text-ctp-overlay1 hover:text-ctp-red px-2 py-0.5 rounded bg-ctp-surface0 transition-colors">
          Clear
        </button>
        <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer" on:click={() => dispatch('close')}>esc</kbd>
      </div>
    </div>

    <!-- Messages -->
    <div bind:this={messagesEl} class="flex-1 overflow-y-auto px-4 py-3 space-y-3">
      {#each messages as msg}
        {#if msg.role === 'user'}
          <!-- User message: right-aligned -->
          <div class="flex justify-end">
            <div class="max-w-[75%]">
              <div class="bg-ctp-blue text-ctp-crust rounded-2xl rounded-br-md px-4 py-2.5 text-sm leading-relaxed">
                {msg.content}
              </div>
              <div class="text-[11px] text-ctp-overlay1 text-right mt-1 mr-1">{msg.time}</div>
            </div>
          </div>

        {:else if msg.role === 'assistant'}
          <!-- AI response: left-aligned -->
          <div class="flex justify-start">
            <div class="max-w-[80%]">
              <div class="flex items-center gap-1.5 mb-1">
                <div class="w-5 h-5 rounded-full bg-ctp-mauve/20 flex items-center justify-center">
                  <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-mauve)" stroke-width="2" stroke-linecap="round">
                    <path d="M2 8h12M5 4l-3 4 3 4M11 4l3 4-3 4" />
                  </svg>
                </div>
                <span class="text-[12px] text-ctp-overlay1">Assistant</span>
              </div>
              <div class="bg-ctp-surface0 text-ctp-text rounded-2xl rounded-bl-md px-4 py-2.5 text-sm leading-relaxed prose-sm">
                {@html renderMarkdown(msg.content)}
              </div>
              <div class="text-[11px] text-ctp-overlay1 mt-1 ml-1">{msg.time}</div>
            </div>
          </div>

        {:else if msg.role === 'system'}
          <!-- System message: centered -->
          <div class="flex justify-center">
            <div class="text-[13px] text-ctp-overlay1 italic bg-ctp-surface0/50 px-3 py-1.5 rounded-full max-w-[80%] text-center">
              {msg.content}
            </div>
          </div>
        {/if}
      {/each}

      <!-- Loading indicator -->
      {#if loading}
        <div class="flex justify-start">
          <div class="max-w-[80%]">
            <div class="flex items-center gap-1.5 mb-1">
              <div class="w-5 h-5 rounded-full bg-ctp-mauve/20 flex items-center justify-center">
                <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-mauve)" stroke-width="2" stroke-linecap="round">
                  <path d="M2 8h12M5 4l-3 4 3 4M11 4l3 4-3 4" />
                </svg>
              </div>
              <span class="text-[12px] text-ctp-overlay1">Assistant</span>
            </div>
            <div class="bg-ctp-surface0 text-ctp-text rounded-2xl rounded-bl-md px-4 py-3">
              <div class="flex items-center gap-1.5">
                <span class="w-2 h-2 rounded-full bg-ctp-mauve animate-bounce" style="animation-delay: 0ms"></span>
                <span class="w-2 h-2 rounded-full bg-ctp-mauve animate-bounce" style="animation-delay: 150ms"></span>
                <span class="w-2 h-2 rounded-full bg-ctp-mauve animate-bounce" style="animation-delay: 300ms"></span>
              </div>
            </div>
          </div>
        </div>
      {/if}
    </div>

    <!-- Input area -->
    <div class="border-t border-ctp-surface0 px-4 py-3">
      {#if error}
        <div class="text-[13px] text-ctp-red bg-ctp-surface0 px-3 py-1.5 rounded-lg mb-2">{error}</div>
      {/if}
      <div class="flex items-end gap-2">
        <textarea
          bind:this={inputEl}
          bind:value={input}
          on:keydown={handleKeydown}
          placeholder="Ask about your notes..."
          rows="1"
          class="flex-1 bg-ctp-surface0 text-ctp-text rounded-xl px-4 py-2.5 text-sm outline-none
            border border-ctp-surface1 focus:border-ctp-blue resize-none
            placeholder:text-ctp-surface2 max-h-32"
          disabled={loading}
        ></textarea>
        <button
          on:click={sendMessage}
          disabled={loading || !input.trim()}
          class="w-9 h-9 rounded-xl flex items-center justify-center transition-all
            {loading || !input.trim()
              ? 'bg-ctp-surface0 text-ctp-overlay1 cursor-not-allowed'
              : 'bg-ctp-blue text-ctp-crust hover:opacity-90 cursor-pointer'}"
        >
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
            <path d="M2 8h12M10 4l4 4-4 4" />
          </svg>
        </button>
      </div>
      <div class="flex items-center justify-between mt-2 text-[12px] text-ctp-overlay1">
        <span><kbd class="bg-ctp-surface0 px-1 py-px rounded">Enter</kbd> send &nbsp; <kbd class="bg-ctp-surface0 px-1 py-px rounded">Shift+Enter</kbd> newline</span>
        <span>{messages.filter(m => m.role !== 'system').length} messages</span>
      </div>
    </div>
  </div>
</div>
