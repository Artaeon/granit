<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { api } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import type { AskAIRequest } from '$lib/editor/ask-ai';
  import { rafThrottle } from '$lib/util/streamThrottle';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';

  // AskAIDialog — right-side DRAWER (2026-05-15 redesign).
  //
  // Prior versions were full-screen modals. The user kept reporting
  // the browser freezing on click and disliking the UX. This rewrite:
  //
  // UX
  //   - Slides in from the RIGHT as a 28rem drawer. Editor stays
  //     visible to the left so the user reads their selection in
  //     context, not in a redundant preview block.
  //   - Compact, info-dense header — character + token count inline,
  //     no separate badge row.
  //   - Selection preview is a collapsed <details>; the dominant
  //     pane is the response, where the user's attention belongs.
  //   - Save-as-note is an INLINE form (input + Save/×). The previous
  //     version used the native `prompt()` browser modal which is
  //     visually jarring and blocks the main thread on some browsers
  //     under extensions.
  //   - Footer action row pins Replace as the primary action,
  //     Insert as the secondary; Copy + Save sit on the left.
  //
  // Freeze prevention
  //   - Backdrop is GONE — explicit × / Esc / Cancel to dismiss.
  //     Removes a click-event path that fired the parent's $effect
  //     cascade while still inside the click handler.
  //   - onDismiss() is deferred via `queueMicrotask` so the click
  //     event returns immediately. Any parent reactive churn happens
  //     in the next microtask, not synchronously inside the click.
  //   - Spellcheck disabled on textarea inputs (long instructions
  //     can stall the spellcheck thread).
  //   - Auto-fire from preset uses `tick().then(ask)` (one microtask,
  //     no setTimeout race). If the user dismisses before tick
  //     resolves, the in-flight check short-circuits via dialogState.
  //   - Streaming buffer accumulates in a plain `let buffer = ''`
  //     (non-reactive). Only an rAF-throttled counter (`streamChars`)
  //     updates while streaming. `response` is written ONCE on
  //     onDone — that's when MarkdownRenderer runs.
  //   - Markdown render is gated: it renders the FINAL response one
  //     time, never per-chunk. Raw view toggle for plain text.
  //
  // Public Props interface unchanged so the parent page +
  // EditorAIBar + EditorAIMenu need zero changes.

  interface Props {
    request: AskAIRequest | null;
    sourcePath: string;
    onDismiss: () => void;
  }
  let { request, sourcePath, onDismiss }: Props = $props();

  type DialogState = 'idle' | 'pending' | 'done';
  let dialogState = $state<DialogState>('idle');
  let instruction = $state('');
  let response = $state('');
  let streamChars = $state(0);
  let error = $state('');
  let viewMode = $state<'rendered' | 'raw'>('rendered');
  let abortCtl: AbortController | null = null;
  let inputEl: HTMLTextAreaElement | undefined = $state();

  // Save-as-note state (inline form replaces native prompt()).
  let saveOpen = $state(false);
  let savePath = $state('');
  let saveInputEl: HTMLInputElement | undefined = $state();

  // React to request flips (null → set or set → set) without firing
  // on unrelated reactive churn. `prevRequest` is a plain let so it
  // doesn't itself depend on reactive state.
  let prevRequest: AskAIRequest | null = null;
  $effect(() => {
    if (request === prevRequest) return;
    prevRequest = request;
    if (!request) {
      abortCtl?.abort();
      abortCtl = null;
      return;
    }
    response = '';
    streamChars = 0;
    error = '';
    dialogState = 'idle';
    saveOpen = false;
    instruction = request.presetInstruction?.trim() ?? '';
    if (request.presetInstruction && request.presetInstruction.trim()) {
      // Auto-fire after mount tick — no setTimeout race. If the user
      // dismisses before tick resolves, `request` flips back to null
      // and ask() short-circuits on the !request guard.
      tick().then(() => {
        if (prevRequest !== null) void ask();
      });
    } else {
      tick().then(() => inputEl?.focus());
    }
  });

  const PREVIEW_CHAR_CAP = 600;
  const previewText = $derived.by(() => {
    const t = request?.text ?? '';
    return t.length <= PREVIEW_CHAR_CAP ? t : t.slice(0, PREVIEW_CHAR_CAP) + '…';
  });
  const fullCharCount = $derived(request?.text.length ?? 0);
  const approxTokens = $derived(Math.round(fullCharCount / 4));

  async function ask() {
    if (!request || dialogState === 'pending') return;
    dialogState = 'pending';
    response = '';
    streamChars = 0;
    error = '';
    let buffer = '';
    abortCtl = new AbortController();
    const userMessage = instruction.trim()
      ? `${instruction.trim()}\n\n---\n\n${request.text}`
      : `Help me with this:\n\n${request.text}`;
    const t = rafThrottle(() => {
      streamChars = buffer.length;
    });
    try {
      await api.chatStream(
        [{ role: 'user', content: userMessage }],
        sourcePath || undefined,
        {
          onChunk: (chunk) => {
            buffer += chunk;
            t.onChunk('');
          },
          onDone: () => {
            t.flush();
            response = buffer;
            dialogState = 'done';
            abortCtl = null;
            if (!response.trim()) error = 'AI returned an empty response.';
          },
          onError: (err) => {
            t.flush();
            response = buffer;
            dialogState = 'done';
            abortCtl = null;
            error = err.message;
          }
        },
        abortCtl.signal
      );
    } catch (e) {
      response = buffer;
      dialogState = 'done';
      abortCtl = null;
      error = e instanceof Error ? e.message : String(e);
    }
  }

  function stop() {
    abortCtl?.abort();
    abortCtl = null;
    dialogState = 'done';
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

  function openSave() {
    if (!response) return;
    const stamp = new Date().toISOString().slice(0, 16).replace(/[:T]/g, '-');
    const clean = instruction
      .trim()
      .replace(/[\\/:*?"<>|]/g, '')
      .replace(/\s+/g, ' ')
      .slice(0, 40)
      .trim();
    savePath = clean ? `AI/${clean} ${stamp}.md` : `AI/Response ${stamp}.md`;
    saveOpen = true;
    tick().then(() => saveInputEl?.select());
  }
  async function saveConfirm() {
    let path = savePath.trim();
    if (!path) return;
    if (!path.toLowerCase().endsWith('.md')) path += '.md';
    if (path.startsWith('/') || path.split('/').some((seg) => seg === '..')) {
      toast.error('Path must be vault-relative (no leading slash or `..`).');
      return;
    }
    try {
      await api.createNote({ path, body: response });
      toast.success(`Saved to ${path}`);
      saveOpen = false;
    } catch (err) {
      toast.error('Save failed: ' + (err instanceof Error ? err.message : String(err)));
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
    abortCtl?.abort();
    abortCtl = null;
    response = '';
    error = '';
    dialogState = 'idle';
    saveOpen = false;
    // Defer parent state write so the click handler returns
    // immediately — prevents the perceived freeze where the parent
    // page's $effect cascade runs synchronously inside the click.
    queueMicrotask(() => onDismiss());
  }
  function cancel() {
    request?.cancel();
    close();
  }

  // Global keydown — Esc to dismiss, Cmd/Ctrl-Enter to send. Window-
  // level so the shortcuts work regardless of which element inside
  // the drawer has focus. Re-armed only while the drawer is open.
  $effect(() => {
    if (!request) return;
    function onWindowKey(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        e.preventDefault();
        if (dialogState === 'pending') stop();
        else cancel();
      } else if ((e.metaKey || e.ctrlKey) && e.key === 'Enter' && dialogState !== 'pending') {
        e.preventDefault();
        void ask();
      }
    }
    window.addEventListener('keydown', onWindowKey);
    return () => window.removeEventListener('keydown', onWindowKey);
  });

  onMount(() => () => {
    abortCtl?.abort();
  });
</script>

{#if request}
  <aside
    class="ask-ai-drawer fixed top-0 right-0 z-50 h-dvh w-full sm:w-[28rem] bg-base border-l border-surface1 shadow-2xl flex flex-col"
    role="dialog"
    aria-label="Ask AI"
    tabindex="-1"
  >
    <!-- Header — single compact row. Title · counters · stop · close. -->
    <header class="px-3 h-9 border-b border-surface1 flex items-center gap-2 flex-shrink-0 bg-mantle">
      <span class="text-[11px] uppercase tracking-wider text-primary font-semibold">Ask AI</span>
      {#if fullCharCount > 0}
        <span class="text-[10px] text-dim font-mono tabular-nums">
          {fullCharCount.toLocaleString()}c · ~{approxTokens.toLocaleString()}t
        </span>
      {/if}
      <span class="flex-1"></span>
      {#if dialogState === 'pending'}
        <button
          type="button"
          onclick={stop}
          class="text-[11px] text-error hover:underline px-1"
        >Stop</button>
      {/if}
      <button
        type="button"
        onclick={cancel}
        aria-label="Close"
        class="text-dim hover:text-text leading-none px-1 text-lg"
      >×</button>
    </header>

    <!-- Selection preview — collapsed by default. The editor itself
         shows the selection so this is just for context confirmation. -->
    {#if request.text}
      <details class="border-b border-surface1 flex-shrink-0">
        <summary class="px-3 py-1.5 text-[10px] uppercase tracking-wider text-dim cursor-pointer hover:text-text select-none">
          Selection preview · click to expand
        </summary>
        <pre class="px-3 pb-2 text-[11px] text-subtext whitespace-pre-wrap break-words font-mono max-h-32 overflow-y-auto">{previewText}</pre>
      </details>
    {/if}

    <!-- Instruction row -->
    <div class="px-3 py-2 border-b border-surface1 flex-shrink-0">
      <label
        for="ai-instruction"
        class="block text-[10px] uppercase tracking-wider text-dim mb-1"
      >Instruction</label>
      <textarea
        id="ai-instruction"
        bind:this={inputEl}
        bind:value={instruction}
        rows="3"
        placeholder="What should the AI do? (Cmd-Enter to send)"
        class="w-full px-2 py-1.5 bg-surface0 border border-surface1 text-sm text-text focus:outline-none focus:border-primary resize-none font-sans"
        disabled={dialogState === 'pending'}
        autocomplete="off"
        autocapitalize="sentences"
        spellcheck="false"
      ></textarea>
      <div class="mt-1.5 flex items-center gap-1.5">
        {#if dialogState === 'pending'}
          <span class="text-[11px] text-dim italic flex items-center gap-1.5">
            <span class="ai-spinner" aria-hidden="true"></span>
            <span class="tabular-nums">{streamChars.toLocaleString()} chars · streaming…</span>
          </span>
        {:else}
          <button
            type="button"
            onclick={ask}
            disabled={!request.text}
            class="px-3 py-1 text-sm bg-primary text-on-primary font-medium hover:opacity-90 disabled:opacity-50"
          >Ask AI</button>
          <span class="text-[10px] text-dim font-mono">⌘↵</span>
          <span class="flex-1"></span>
          <button
            type="button"
            onclick={cancel}
            class="text-[11px] text-dim hover:text-text"
          >Cancel</button>
        {/if}
      </div>
    </div>

    <!-- Response area — fills the remaining height. Markdown render
         runs ONCE after stream ends; raw toggle for plain text. -->
    <div class="flex-1 overflow-y-auto">
      {#if error}
        <div class="m-3 px-3 py-2 border border-error bg-surface0 text-xs text-error">
          <div>{error}</div>
          {#if /provider|api key|not configured/i.test(error)}
            <div class="text-dim mt-1">
              Open <a href="/settings" class="text-secondary hover:underline">Settings</a> to configure an AI provider.
            </div>
          {/if}
        </div>
      {/if}

      {#if dialogState === 'done' && response}
        <div class="px-3 py-1.5 border-b border-surface1 flex items-center gap-2 sticky top-0 bg-base z-10">
          <span class="text-[10px] uppercase tracking-wider text-dim">Response</span>
          <span class="text-[10px] text-dim font-mono tabular-nums">{response.length.toLocaleString()}c</span>
          <span class="flex-1"></span>
          <button
            type="button"
            onclick={() => (viewMode = viewMode === 'rendered' ? 'raw' : 'rendered')}
            class="text-[10px] text-dim hover:text-text font-mono"
            title="Toggle rendered markdown vs raw text"
          >{viewMode === 'rendered' ? 'rendered' : 'raw'} ↺</button>
        </div>
        <div class="px-3 py-2">
          {#if viewMode === 'rendered'}
            <div class="text-sm leading-relaxed ai-response-md">
              <MarkdownRenderer body={response} />
            </div>
          {:else}
            <pre class="text-xs text-text whitespace-pre-wrap break-words font-mono">{response}</pre>
          {/if}
        </div>
      {:else if dialogState === 'pending'}
        <div class="px-3 py-6 text-center text-[11px] text-dim italic">
          Response will appear here when the AI finishes.<br />
          Press <kbd class="font-mono text-text">Esc</kbd> or <span class="text-error">Stop</span> to abort.
        </div>
      {/if}
    </div>

    <!-- Save-as-note inline form (replaces window.prompt). Surfaces
         only when the user hits Save in the footer. -->
    {#if saveOpen && response}
      <div class="px-3 py-2 border-t border-surface1 bg-surface0 flex-shrink-0">
        <label for="ai-save-path" class="block text-[10px] uppercase tracking-wider text-dim mb-1">
          Save to vault path
        </label>
        <div class="flex items-center gap-1.5">
          <input
            id="ai-save-path"
            bind:this={saveInputEl}
            bind:value={savePath}
            type="text"
            class="flex-1 min-w-0 px-2 py-1 bg-base border border-surface1 text-xs text-text focus:outline-none focus:border-primary font-mono"
            autocomplete="off"
            spellcheck="false"
            onkeydown={(e) => {
              if (e.key === 'Enter') { e.preventDefault(); void saveConfirm(); }
              if (e.key === 'Escape') { e.preventDefault(); saveOpen = false; }
            }}
          />
          <button
            type="button"
            onclick={saveConfirm}
            class="px-2 py-1 text-xs bg-primary text-on-primary font-medium hover:opacity-90"
          >Save</button>
          <button
            type="button"
            onclick={() => (saveOpen = false)}
            class="px-2 py-1 text-xs text-dim hover:text-text"
            aria-label="Cancel save"
          >×</button>
        </div>
      </div>
    {/if}

    <!-- Footer — action row visible only when there's a response. -->
    {#if dialogState === 'done' && response && !error}
      <footer
        class="px-3 py-2 border-t border-surface1 bg-mantle flex items-center gap-1 flex-shrink-0 flex-wrap"
        style="padding-bottom: max(0.5rem, env(safe-area-inset-bottom));"
      >
        <button
          type="button"
          onclick={copyResponse}
          class="px-2 py-1 text-xs text-subtext hover:text-text hover:bg-surface0"
        >Copy</button>
        <button
          type="button"
          onclick={openSave}
          class="px-2 py-1 text-xs text-subtext hover:text-text hover:bg-surface0"
        >Save as note…</button>
        <span class="flex-1"></span>
        <button
          type="button"
          onclick={insertBelow}
          class="px-2 py-1 text-xs bg-surface0 text-text border border-surface1 hover:border-primary"
        >Insert below</button>
        <button
          type="button"
          onclick={replaceSelection}
          class="px-2.5 py-1 text-xs bg-primary text-on-primary font-medium hover:opacity-90"
        >Replace</button>
      </footer>
    {/if}
  </aside>
{/if}

<style>
  /* Spinner — small, in line with the streaming text. */
  .ai-spinner {
    display: inline-block;
    width: 0.75rem;
    height: 0.75rem;
    border: 2px solid var(--color-surface1);
    border-top-color: var(--color-primary);
    border-radius: 50%;
    animation: ai-spin 0.7s linear infinite;
    vertical-align: middle;
    flex-shrink: 0;
  }
  @keyframes ai-spin {
    to {
      transform: rotate(360deg);
    }
  }

  /* Slide-in animation on mount — keeps the open feeling tactile
     without stealing focus or blocking interaction. Disabled when
     the user prefers reduced motion. */
  .ask-ai-drawer {
    animation: ai-slide-in 160ms cubic-bezier(0.2, 0.8, 0.2, 1);
  }
  @keyframes ai-slide-in {
    from {
      transform: translateX(100%);
    }
    to {
      transform: translateX(0);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .ask-ai-drawer {
      animation: none;
    }
  }

  /* MarkdownRenderer styles — the parent component renders its own
     prose styling, but we tweak a couple of things to make headings
     and lists feel tighter inside the narrow drawer column. */
  .ai-response-md :global(h1) {
    font-size: 1rem;
    font-weight: 600;
    margin-top: 0.75rem;
    margin-bottom: 0.5rem;
  }
  .ai-response-md :global(h2) {
    font-size: 0.95rem;
    font-weight: 600;
    margin-top: 0.625rem;
    margin-bottom: 0.375rem;
  }
  .ai-response-md :global(h3) {
    font-size: 0.875rem;
    font-weight: 600;
    margin-top: 0.5rem;
    margin-bottom: 0.25rem;
  }
  .ai-response-md :global(p) {
    margin-bottom: 0.5rem;
  }
  .ai-response-md :global(ul),
  .ai-response-md :global(ol) {
    padding-left: 1.25rem;
    margin-bottom: 0.5rem;
  }
  .ai-response-md :global(li) {
    margin-bottom: 0.125rem;
  }
  .ai-response-md :global(pre) {
    font-size: 0.75rem;
    padding: 0.5rem;
    background: var(--color-surface0);
    border: 1px solid var(--color-surface1);
    overflow-x: auto;
  }
  .ai-response-md :global(code) {
    font-size: 0.8125rem;
  }
</style>
