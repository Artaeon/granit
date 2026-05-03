<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { api, type ChatMessage } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { classifyAiError } from '$lib/util/aiErrors';
  import PageHeader from '$lib/components/PageHeader.svelte';

  // Multi-turn chat with the configured LLM. History lives in localStorage
  // (one current conversation; the user "saves" via "save as note" to keep
  // a long-term record). Server is stateless — we ship the whole history
  // every turn.

  const STORAGE_KEY = 'granit.chat.current';
  type Stored = { messages: ChatMessage[]; updatedAt: number };

  let messages = $state<ChatMessage[]>([]);
  let input = $state('');
  let busy = $state(false);
  let scrollEl: HTMLDivElement | undefined = $state();
  let inputEl: HTMLTextAreaElement | undefined = $state();

  // Restore the in-progress conversation on first paint so a refresh
  // doesn't lose context. Save after every message exchange.
  onMount(() => {
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      if (raw) {
        const parsed = JSON.parse(raw) as Stored;
        if (Array.isArray(parsed.messages)) messages = parsed.messages;
      }
    } catch {}
    inputEl?.focus();
  });

  $effect(() => {
    void messages;
    try { localStorage.setItem(STORAGE_KEY, JSON.stringify({ messages, updatedAt: Date.now() } satisfies Stored)); } catch {}
  });

  // Auto-scroll to bottom on new messages.
  $effect(() => {
    void messages.length;
    void busy;
    tick().then(() => {
      if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
    });
  });

  async function send(e?: Event) {
    e?.preventDefault();
    const text = input.trim();
    if (!text || busy) return;
    busy = true;
    const userMsg: ChatMessage = { role: 'user', content: text };
    messages = [...messages, userMsg];
    input = '';
    try {
      const r = await api.chat(messages);
      messages = [...messages, r.message];
    } catch (err) {
      // Surface the server error inline so the user can see what went
      // wrong (e.g. "no API key set"). Drop the user's message back into
      // the input so they can retry without re-typing. The classifier
      // turns raw "ollama: 404 …" noise into a one-line headline plus
      // an Open-Settings CTA; raw is still available behind "details".
      const msg = err instanceof Error ? err.message : String(err);
      console.error('[chat] send failed:', msg);
      const hint = classifyAiError(msg);
      toast.error(hint.headline, { action: hint.cta, details: hint.raw });
      messages = messages.slice(0, -1);
      input = text;
    } finally {
      busy = false;
      tick().then(() => inputEl?.focus());
    }
  }

  function reset() {
    if (messages.length > 0 && !confirm('Clear the current conversation?')) return;
    messages = [];
    input = '';
  }

  // Save the conversation as a markdown note so the user has a permanent
  // record. Uses /api/v1/notes — the same path the editor uses.
  async function saveAsNote() {
    if (messages.length === 0) return;
    const stamp = new Date().toISOString().slice(0, 16).replace('T', ' ');
    const title = `Chat ${stamp}`;
    const path = `Chats/${stamp.replace(/[: ]/g, '-')}.md`;
    const body =
      `---\ntype: chat\ndate: ${new Date().toISOString().slice(0, 10)}\n---\n\n# ${title}\n\n` +
      messages
        .map((m) => `## ${m.role === 'user' ? 'You' : m.role === 'assistant' ? 'Assistant' : 'System'}\n\n${m.content}`)
        .join('\n\n');
    try {
      await api.createNote({ path, body });
      toast.success(`saved to ${path}`);
    } catch (e) {
      toast.error('save failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  function onKey(e: KeyboardEvent) {
    // Enter sends; Shift+Enter inserts newline. Mirrors the convention
    // every chat UI uses since Slack made it standard.
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      send();
    }
  }
</script>

<div class="h-full flex flex-col overflow-hidden">
  <header class="px-4 py-3 border-b border-surface1 flex-shrink-0 flex items-center gap-2 max-w-3xl w-full mx-auto">
    <PageHeader title="Chat" subtitle="Talk to your AI — same provider as the agents" />
    <span class="flex-1"></span>
    {#if messages.length > 0}
      <button onclick={saveAsNote} class="text-xs px-3 py-1.5 bg-surface0 border border-surface1 rounded text-subtext hover:border-primary">
        Save as note
      </button>
      <button onclick={reset} class="text-xs px-3 py-1.5 text-dim hover:text-error">
        Clear
      </button>
    {/if}
  </header>

  <div bind:this={scrollEl} class="flex-1 overflow-y-auto">
    <div class="max-w-3xl mx-auto px-4 py-6">
      {#if messages.length === 0}
        <div class="text-center py-12">
          <p class="text-sm text-dim">Say hi. The model has no vault context unless you mention notes by name or paste excerpts.</p>
          <p class="text-[11px] text-dim/70 italic mt-2">
            Conversations are stored locally — nothing leaves the server (except your messages going to the AI provider).
          </p>
        </div>
      {/if}

      <ol class="space-y-4">
        {#each messages as m, i (i)}
          <li class="flex gap-3 {m.role === 'user' ? 'justify-end' : ''}">
            {#if m.role !== 'user'}
              <div class="w-8 h-8 rounded-full bg-primary/15 text-primary flex items-center justify-center flex-shrink-0 text-xs font-mono mt-1">AI</div>
            {/if}
            <div
              class="max-w-[85%] sm:max-w-[75%] px-4 py-2.5 rounded-lg whitespace-pre-wrap break-words text-sm leading-relaxed
                {m.role === 'user' ? 'bg-primary text-on-primary' : m.role === 'system' ? 'bg-surface0 text-dim border border-surface1 italic' : 'bg-surface0 text-text border border-surface1'}"
            >{m.content}</div>
            {#if m.role === 'user'}
              <div class="w-8 h-8 rounded-full bg-surface1 text-subtext flex items-center justify-center flex-shrink-0 text-xs font-mono mt-1">You</div>
            {/if}
          </li>
        {/each}
        {#if busy}
          <li class="flex gap-3">
            <div class="w-8 h-8 rounded-full bg-primary/15 text-primary flex items-center justify-center flex-shrink-0 text-xs font-mono mt-1">AI</div>
            <div class="px-4 py-2.5 rounded-lg bg-surface0 border border-surface1">
              <div class="flex gap-1 items-center text-dim">
                <span class="w-1.5 h-1.5 rounded-full bg-current animate-bounce"></span>
                <span class="w-1.5 h-1.5 rounded-full bg-current animate-bounce" style="animation-delay: 0.15s"></span>
                <span class="w-1.5 h-1.5 rounded-full bg-current animate-bounce" style="animation-delay: 0.3s"></span>
              </div>
            </div>
          </li>
        {/if}
      </ol>
    </div>
  </div>

  <div class="border-t border-surface1 bg-mantle/50 flex-shrink-0">
    <form onsubmit={send} class="max-w-3xl mx-auto p-3 flex gap-2 items-end">
      <textarea
        bind:this={inputEl}
        bind:value={input}
        onkeydown={onKey}
        placeholder="Send a message…   (Enter to send, Shift+Enter for newline)"
        rows="1"
        class="flex-1 min-w-0 px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary resize-none max-h-40"
      ></textarea>
      <button
        type="submit"
        disabled={busy || !input.trim()}
        class="px-4 py-2 bg-primary text-on-primary rounded text-sm font-medium disabled:opacity-50"
      >Send</button>
    </form>
  </div>
</div>
