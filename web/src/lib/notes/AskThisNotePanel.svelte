<!--
  AskThisNotePanel — multi-turn Q&A scoped to ONLY the current note.

  Distinct from AskAIDialog (single-turn over a selection) and
  AIOverlay (broader vault chat). This panel lives in the right rail
  alongside Outline / Backlinks / Research, and gives the user a
  conversational surface for "explain section 3", "what's the
  contradiction here?", "rephrase the 4th paragraph" — without
  leaving the page or losing the note's spatial context.

  Conversation is in-memory only. No persistence (multi-turn over a
  note is throwaway by nature; persisting cargo-cults the chat
  metaphor onto a tool that's really just a thinking surface).
  Resets on note change so context doesn't bleed across notes.

  AI gating: chatStream → /chat/stream. Sabbath / redaction / audit
  / cost — all there. Don't bypass.
-->
<script lang="ts">
  import { tick } from 'svelte';
  import { api } from '$lib/api';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';

  interface Turn {
    role: 'user' | 'assistant';
    content: string;
    /** True while this turn's content is still streaming in. */
    streaming?: boolean;
    /** Set when streaming failed mid-flight; lets the UI render an error. */
    error?: string;
  }

  let {
    notePath,
    title,
    body
  }: { notePath: string; title: string; body: string } = $props();

  let turns = $state<Turn[]>([]);
  let input = $state('');
  let pending = $state(false);
  let abort: AbortController | null = null;
  // Re-derived counts for the disclosure line. Cheap — body is a
  // string, this runs once per change.
  let bodyChars = $derived(body.length);
  let bodyTokens = $derived(Math.round(bodyChars / 4));
  let scrollEl: HTMLElement | undefined = $state();

  // Reset the conversation on note change. Otherwise a question about
  // note A would silently appear in note B's panel — confusing and
  // worse, the SYSTEM prompt below would still ground in note B's
  // body, so the AI would answer the wrong question with the wrong
  // context.
  $effect(() => {
    void notePath;
    abort?.abort();
    abort = null;
    turns = [];
    pending = false;
    input = '';
  });

  function clipBody(src: string, max = 14000): string {
    const t = src.trim();
    if (t.length <= max) return t;
    // Keep the head — early sections (intro, framing) are usually
    // the most valuable for context. Note the truncation so the
    // model knows it doesn't have the full doc.
    return t.slice(0, max) + '\n\n…(note truncated to keep the prompt small)';
  }

  function buildSystem(): string {
    const clipped = clipBody(body);
    return [
      "You are a focused study companion answering questions about ONE note from the user's vault. The note is below as your only context. Answer ONLY about this note. If the answer isn't in the note, say so plainly — don't speculate, don't browse, don't pretend. The user already has the note in front of them, so don't quote large blocks back at them; instead point to the section or sentence. Keep replies tight: 2-5 sentences, or a short markdown list for enumerable answers. When the user asks for a rewrite or a translation, return the rewrite/translation as a clean block — no preamble. Match the note's register (formal / casual / technical). If the user asks 'where' something is in the note, name the section or quote a phrase, not a paragraph.",
      '',
      `<note title="${title}" path="${notePath}">`,
      clipped,
      '</note>'
    ].join('\n');
  }

  async function send() {
    const text = input.trim();
    if (!text || pending) return;
    if (!body.trim()) return;
    input = '';
    abort?.abort();
    abort = new AbortController();
    pending = true;

    // Append the user turn. The assistant turn is appended AFTER
    // the system message is built so we don't accidentally include
    // the empty assistant turn in the messages payload.
    const next: Turn[] = [...turns, { role: 'user', content: text }];
    turns = next;
    await tick();
    scrollToBottom();

    // Build the message payload. System always carries the note
    // body, so the panel never relies on cross-message memory of
    // the note — every turn is grounded in the current text.
    const messages = [
      { role: 'system' as const, content: buildSystem() },
      ...turns.map((t) => ({ role: t.role, content: t.content }))
    ];

    // Append a streaming assistant turn placeholder.
    turns = [...turns, { role: 'assistant', content: '', streaming: true }];
    const assistantIdx = turns.length - 1;

    let buf = '';
    try {
      await api.chatStream(messages, notePath || undefined, {
        onChunk: (c) => {
          buf += c;
          turns = turns.map((t, i) => (i === assistantIdx ? { ...t, content: buf } : t));
          scrollToBottom();
        },
        onDone: () => {
          turns = turns.map((t, i) => (i === assistantIdx ? { ...t, streaming: false } : t));
          if (!buf.trim()) {
            turns = turns.map((t, i) => (i === assistantIdx ? { ...t, error: 'AI returned an empty response.' } : t));
          }
        },
        onError: (err) => {
          turns = turns.map((t, i) =>
            i === assistantIdx ? { ...t, streaming: false, error: err.message } : t
          );
        }
      }, abort.signal);
    } finally {
      pending = false;
      abort = null;
    }
  }

  function stop() {
    abort?.abort();
    abort = null;
    pending = false;
    // Mark whichever streaming turn is in flight as no-longer-streaming.
    turns = turns.map((t) => (t.streaming ? { ...t, streaming: false } : t));
  }

  function clearConversation() {
    abort?.abort();
    abort = null;
    pending = false;
    turns = [];
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      void send();
    }
  }

  function scrollToBottom() {
    if (!scrollEl) return;
    requestAnimationFrame(() => {
      if (!scrollEl) return;
      scrollEl.scrollTop = scrollEl.scrollHeight;
    });
  }

  // A few starter prompts — frame the panel for users who don't yet
  // know what to ask. One-tap fills the input + sends.
  const STARTERS: { label: string; prompt: string }[] = [
    { label: 'Summarise', prompt: 'Summarise this note in 4 bullets.' },
    { label: 'Key claim', prompt: "What's the strongest claim this note makes? Quote the line." },
    { label: 'Sections', prompt: 'List the sections of this note with one-line gists.' },
    { label: 'Steel-man', prompt: "Steel-man the position this note takes. Then list its 2 weakest points." },
    { label: 'Questions', prompt: 'What are 5 sharp questions a critical reader would ask of this note?' }
  ];

  function pickStarter(p: string) {
    input = p;
    void send();
  }

  let canAsk = $derived(body.trim().length > 0);
</script>

<div class="text-sm">
  {#if !canAsk}
    <div class="text-[11px] text-dim italic px-2 py-1.5">
      Empty note — nothing to ask about yet.
    </div>
  {:else}
    {#if turns.length === 0}
      <!-- Cold-start: starter prompts + the input. Once the user
           has a turn, we hide the starters to recover space. -->
      <div class="flex flex-wrap gap-1 mb-2 px-1">
        {#each STARTERS as s}
          <button
            type="button"
            onclick={() => pickStarter(s.prompt)}
            disabled={pending}
            class="text-[10px] px-1.5 py-0.5 rounded bg-surface0 text-subtext hover:bg-surface1 hover:text-text disabled:opacity-50"
            title={s.prompt}
          >{s.label}</button>
        {/each}
      </div>
    {/if}

    {#if turns.length > 0}
      <div
        bind:this={scrollEl}
        class="space-y-2 mb-2 max-h-72 overflow-y-auto pr-1"
      >
        {#each turns as t, i (i)}
          <div class="text-[12px] {t.role === 'user' ? 'text-text' : 'text-subtext'}">
            <div class="text-[9px] uppercase tracking-wider mb-0.5 {t.role === 'user' ? 'text-primary' : 'text-secondary'}">
              {t.role === 'user' ? 'you' : 'assistant'}
            </div>
            {#if t.role === 'user'}
              <p class="px-2 py-1 rounded bg-surface0 whitespace-pre-wrap break-words m-0">{t.content}</p>
            {:else if t.error}
              <p class="px-2 py-1 rounded bg-error/5 border border-error/20 text-error text-[11px] m-0">{t.error}</p>
            {:else if !t.content && t.streaming}
              <span class="px-2 text-dim italic flex items-center gap-1.5">
                <span class="ai-spinner" aria-hidden="true"></span>
                thinking…
              </span>
            {:else}
              <div class="ask-prose px-2">
                <MarkdownRenderer body={t.content} />
              </div>
              {#if t.streaming}
                <span class="px-2 text-[10px] text-secondary italic">streaming…</span>
              {/if}
            {/if}
          </div>
        {/each}
      </div>
    {/if}

    <div class="flex items-end gap-1">
      <textarea
        bind:value={input}
        onkeydown={onKey}
        rows={turns.length === 0 ? 2 : 1}
        placeholder="Ask about this note… (⌘↵ to send)"
        disabled={pending}
        class="flex-1 px-2 py-1 bg-surface0 border border-surface1 rounded text-xs text-text placeholder-dim focus:outline-none focus:border-primary resize-none"
      ></textarea>
      {#if pending}
        <button
          type="button"
          onclick={stop}
          class="px-2 py-1 rounded text-[11px] bg-surface0 text-error border border-error/30 hover:bg-error/10"
        >stop</button>
      {:else}
        <button
          type="button"
          onclick={send}
          disabled={!input.trim()}
          class="px-2 py-1 rounded text-[11px] bg-primary text-on-primary hover:opacity-90 disabled:opacity-40"
        >ask</button>
      {/if}
    </div>

    <div class="flex items-baseline gap-1.5 mt-1 px-1">
      <!-- Disclosure: the model sees this much. Mirrors the
           AskAIDialog's "{N} chars · ~{M} tok" header so the user
           knows the cost shape of every turn. -->
      <span class="text-[9px] text-dim font-mono">
        AI sees {bodyChars.toLocaleString()} chars · ~{bodyTokens.toLocaleString()} tok of this note
      </span>
      <span class="flex-1"></span>
      {#if turns.length > 0}
        <button
          type="button"
          onclick={clearConversation}
          class="text-[9px] text-dim hover:text-error"
          title="Reset the conversation. Note context stays the same; just clears history."
        >reset</button>
      {/if}
    </div>
  {/if}
</div>

<style>
  /* Compact prose inside the panel — the rail is narrow, so a
     normal max-w-prose lays out awkwardly here. Same shape as
     ReferenceNotePanel's reference-prose adjustments. */
  .ask-prose :global(p) { margin: 0.3rem 0; }
  .ask-prose :global(ul),
  .ask-prose :global(ol) { margin: 0.3rem 0; padding-left: 1.1rem; }
  .ask-prose :global(h1) { font-size: 0.95rem; margin-top: 0.4rem; }
  .ask-prose :global(h2) { font-size: 0.9rem; margin-top: 0.4rem; }
  .ask-prose :global(h3) { font-size: 0.85rem; }
  .ask-prose :global(pre) { font-size: 0.7rem; padding: 0.4rem; }
  .ai-spinner {
    display: inline-block;
    width: 0.6rem;
    height: 0.6rem;
    border: 1.5px solid var(--color-surface1);
    border-top-color: var(--color-secondary);
    border-radius: 50%;
    animation: ai-spin 0.7s linear infinite;
  }
  @keyframes ai-spin {
    to { transform: rotate(360deg); }
  }
</style>
