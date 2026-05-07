<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { api } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import type { AskAIRequest } from '$lib/editor/ask-ai';

  // AskAIDialog — modal that opens when the user fires Mod-Shift-A
  // (or clicks the AI button on the floating toolbar) on a text
  // selection. Sends the selection to /chat as a single-turn
  // user message and shows the response with four action buttons:
  // Copy / Replace / Insert below / Cancel.
  //
  // Optional "instruction" line lets the user steer the AI:
  //   "summarise this in three bullets"
  //   "translate to German"
  //   "make this more concise"
  // When set, it's prepended as a system-style preface before the
  // selection. The system prompt the server adds (granit context)
  // stays — this is a layer on top, not a replacement.

  interface Props {
    request: AskAIRequest | null;
    /** Path of the source note — passed to /chat so the AI has note
     *  context for free, same as the morning routine's AI suggestion. */
    sourcePath: string;
    onDismiss: () => void;
  }

  let { request, sourcePath, onDismiss }: Props = $props();

  let instruction = $state('');
  let response = $state('');
  let pending = $state(false);
  let error = $state('');
  let inputEl: HTMLTextAreaElement | undefined = $state();
  // AbortController for the in-flight streaming call. Stored at the
  // component level so the cancel button + Escape key can both
  // close the upstream connection promptly.
  let abortCtl: AbortController | null = $state(null);

  // Quick-pick instructions — common AI-on-selection tasks the user
  // can fire without typing. Each one is a complete instruction that
  // works across providers; the user can still tweak the field
  // before submitting, or write their own.
  const QUICK_INSTRUCTIONS = [
    { label: 'Summarise', text: 'Summarise the following in 3 concise bullet points.' },
    { label: 'Improve', text: 'Improve the writing of the following — clearer, tighter, same voice. Return only the improved text.' },
    { label: 'Expand', text: 'Expand the following with more detail and context. Keep the same voice.' },
    { label: 'Translate · DE', text: 'Translate the following to German. Return only the translation.' },
    { label: 'Translate · EN', text: 'Translate the following to English. Return only the translation.' },
    { label: 'Bullet list', text: 'Rewrite the following as a markdown bullet list. Return only the list.' },
    // Whole-note actions — useful when the dialog is opened over the
    // entire note body (via the toolbar AI button). Still work on a
    // selection too, just less useful.
    { label: 'Extract tasks', text: 'Extract the actionable items from the following as a markdown task list (`- [ ] task` lines). Each line should be a concrete action. Return ONLY the markdown checklist, no preamble.' },
    { label: 'Suggest tags', text: 'Suggest 5-7 short, lowercase, hyphenated hashtags relevant to the topic of the following. Return ONLY a single line of space-separated tags starting with `#`. Example: `#research #notes-app #productivity`.' },
    { label: 'Outline', text: 'Generate a markdown outline (## headings + bulleted sub-points) of the key ideas in the following. Return only the outline.' }
  ];

  $effect(() => {
    if (request) {
      response = '';
      instruction = '';
      error = '';
      pending = false;
      tick().then(() => inputEl?.focus());
    }
  });

  async function ask() {
    if (!request || pending) return;
    pending = true;
    error = '';
    response = '';
    const userMessage = instruction.trim()
      ? `${instruction.trim()}\n\n---\n\n${request.text}`
      : `Help me with this:\n\n${request.text}`;

    // Streaming path. Tokens land in `response` as they arrive so
    // the user sees the AI typing in real time — much snappier
    // perceived latency than the buffered path. AbortController
    // wires through to the upstream provider so a Stop click
    // closes the connection immediately.
    abortCtl = new AbortController();
    await api.chatStream(
      [{ role: 'user', content: userMessage }],
      sourcePath || undefined,
      {
        onChunk: (chunk) => {
          response += chunk;
        },
        onDone: () => {
          pending = false;
          abortCtl = null;
          if (!response) error = 'AI returned an empty response.';
        },
        onError: (err) => {
          pending = false;
          abortCtl = null;
          error = err.message;
        }
      },
      abortCtl.signal
    );
  }

  function stop() {
    if (abortCtl) {
      abortCtl.abort();
      abortCtl = null;
    }
    pending = false;
  }

  function pickQuick(text: string) {
    instruction = text;
    void ask();
  }

  function copyResponse() {
    if (!response) return;
    try {
      void navigator.clipboard.writeText(response);
      toast.success('Copied');
    } catch {
      toast.error('Copy failed (clipboard blocked?)');
    }
  }

  function replaceSelection() {
    if (!request || !response) return;
    request.replace(response);
    close();
  }

  function insertBelow() {
    if (!request || !response) return;
    request.insertAfter(response);
    close();
  }

  function close() {
    onDismiss();
  }

  function dismiss() {
    // While streaming, ESC / backdrop-click stops the stream rather
    // than sitting locked. Same affordance as the Stop button.
    if (pending) {
      stop();
      return;
    }
    request?.cancel();
    close();
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault();
      dismiss();
    } else if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      e.preventDefault();
      void ask();
    }
  }
</script>

{#if request}
  <div
    class="fixed inset-0 z-50 flex items-start justify-center pt-12 px-4 bg-mantle/70 backdrop-blur-sm"
    onclick={dismiss}
    onkeydown={onKey}
    role="presentation"
  >
    <section
      class="w-full max-w-2xl bg-base border border-surface1 rounded-lg shadow-xl max-h-[88vh] flex flex-col"
      onclick={(e) => e.stopPropagation()}
      role="dialog"
      aria-label="Ask AI about selection"
    >
      <header class="px-4 py-3 border-b border-surface1 flex items-baseline gap-2">
        <h2 class="text-sm font-semibold text-text flex-1">Ask AI</h2>
        <span class="text-[11px] text-dim">{request.text.length} char{request.text.length === 1 ? '' : 's'} selected</span>
        <button
          onclick={dismiss}
          aria-label="close"
          class="text-dim hover:text-text text-lg leading-none ml-2"
        >×</button>
      </header>

      <div class="flex-1 overflow-y-auto p-4 space-y-3">
        <!-- Selection preview — shows the user exactly what's being
             sent. Read-only so accidental clicks don't mutate it
             during the round-trip. -->
        <div>
          <span class="block text-[11px] uppercase tracking-wider text-dim mb-1">Your selection</span>
          <pre class="bg-surface0 border border-surface1 rounded px-3 py-2 text-xs text-subtext whitespace-pre-wrap break-words max-h-32 overflow-y-auto font-mono">{request.text}</pre>
        </div>

        <!-- Instruction field. Empty = generic "help me with this".
             Set = explicit steering. Quick-pick chips fire ask()
             immediately so a one-tap "Summarise" workflow exists. -->
        <div>
          <label for="ai-instruction" class="block text-[11px] uppercase tracking-wider text-dim mb-1">Instruction (optional)</label>
          <textarea
            id="ai-instruction"
            bind:this={inputEl}
            bind:value={instruction}
            rows="2"
            placeholder="What should the AI do? (or pick a preset below)"
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
            disabled={pending}
          ></textarea>
          <div class="flex flex-wrap gap-1 mt-1.5">
            {#each QUICK_INSTRUCTIONS as q}
              <button
                type="button"
                onclick={() => pickQuick(q.text)}
                disabled={pending}
                class="px-2 py-0.5 text-[11px] rounded bg-surface0 text-subtext hover:bg-surface1 hover:text-text disabled:opacity-50"
              >{q.label}</button>
            {/each}
          </div>
        </div>

        <!-- Response area. Streams progressively — chunks land in
             `response` as they arrive, so the user sees the AI
             "type" in real time. The pending+empty state shows just
             the spinner; pending+streaming shows the partial
             response with a small streaming indicator above it. -->
        {#if error}
          <div class="text-xs text-error border border-error/30 bg-error/5 rounded px-3 py-2">
            {error}
            {#if /provider|api key|not configured/i.test(error)}
              <div class="text-dim mt-1">
                Open <a href="/settings" class="text-secondary hover:underline">Settings</a> to configure an AI provider.
              </div>
            {/if}
          </div>
        {:else if pending && !response}
          <div class="text-sm text-dim italic flex items-center gap-2">
            <span class="ai-spinner" aria-hidden="true"></span>
            Asking the AI…
          </div>
        {:else if response}
          <div>
            <div class="flex items-baseline justify-between mb-1">
              <span class="text-[11px] uppercase tracking-wider text-dim">Response</span>
              {#if pending}
                <span class="text-[11px] text-secondary flex items-center gap-1.5">
                  <span class="ai-spinner ai-spinner--sm" aria-hidden="true"></span>
                  streaming
                </span>
              {/if}
            </div>
            <div class="bg-surface0 border border-surface1 rounded px-3 py-2 text-sm text-text whitespace-pre-wrap break-words max-h-72 overflow-y-auto">{response}</div>
          </div>
        {/if}
      </div>

      <footer class="px-4 py-3 border-t border-surface1 flex items-center gap-2 flex-wrap">
        {#if pending}
          <button
            type="button"
            onclick={stop}
            class="px-3 py-1.5 text-sm bg-surface0 text-error border border-error/30 rounded hover:bg-error/10"
          >Stop</button>
          <span class="flex-1"></span>
          <span class="text-[11px] text-dim italic">Streaming · cancel anytime</span>
        {:else}
          <button
            type="button"
            onclick={dismiss}
            class="px-3 py-1.5 text-sm text-subtext hover:bg-surface0 rounded"
          >Cancel</button>
          {#if response}
            <button
              type="button"
              onclick={copyResponse}
              class="px-3 py-1.5 text-sm bg-surface0 text-text border border-surface1 rounded hover:border-primary"
            >Copy</button>
            <button
              type="button"
              onclick={insertBelow}
              class="px-3 py-1.5 text-sm bg-surface0 text-text border border-surface1 rounded hover:border-primary"
            >Insert below</button>
            <button
              type="button"
              onclick={replaceSelection}
              class="px-3 py-1.5 text-sm bg-primary text-on-primary rounded font-medium hover:opacity-90"
            >Replace selection</button>
          {:else}
            <span class="flex-1"></span>
            <button
              type="button"
              onclick={ask}
              class="px-3 py-1.5 text-sm bg-primary text-on-primary rounded font-medium hover:opacity-90"
            >Ask AI (⌘↵)</button>
          {/if}
        {/if}
      </footer>
    </section>
  </div>
{/if}

<style>
  .ai-spinner {
    display: inline-block;
    width: 0.875rem;
    height: 0.875rem;
    border: 2px solid var(--color-surface1);
    border-top-color: var(--color-primary);
    border-radius: 50%;
    animation: ai-spin 0.7s linear infinite;
  }
  .ai-spinner--sm {
    width: 0.625rem;
    height: 0.625rem;
    border-width: 1.5px;
    border-top-color: var(--color-secondary);
  }
  @keyframes ai-spin {
    to { transform: rotate(360deg); }
  }
</style>
