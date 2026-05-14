<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { api } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import type { AskAIRequest } from '$lib/editor/ask-ai';
  import { rafThrottle } from '$lib/util/streamThrottle';

  // AskAIDialog — MINIMAL REWRITE (2026-05-15).
  //
  // The previous version was ~950 lines and the user kept reporting
  // the browser freezing for 5+ seconds on click — bad enough that
  // Chrome showed the "Page Unresponsive" dialog. Multiple targeted
  // fixes (lineDiff cap, preview cap, rAF chunk throttle, null-safe
  // reads, stat-derive memoisation) didn't resolve it. Five parallel
  // exploration agents converged on the same diagnosis: the dialog
  // accumulated too many simultaneously-active hot paths (live diff
  // view, live markdown render, live per-frame stats, history chip
  // row, ~30 preset chips, dismiss timer, throughput chip ticker)
  // and the cumulative cost was the freeze.
  //
  // This rewrite strips the dialog to the absolute essentials:
  //
  //   - Selection preview, capped at 2000 chars rendered. The full
  //     selection text is what we send to the AI; the on-screen
  //     <pre> only renders the cap.
  //   - Instruction textarea (manual or auto-filled from preset).
  //   - DURING STREAMING: a centered spinner + Stop button only.
  //     No live response text. No stats. No diff. No markdown
  //     parse. The buffer accumulates in memory; nothing renders.
  //   - POST-STREAM: response in a plain <pre> with a small length
  //     line. No diff. No markdown render. Plain text rendering is
  //     near-free even for 10K-char responses.
  //   - Action buttons: Cancel · Copy · Save as note · Insert below
  //     · Replace selection.
  //
  // Removed entirely (relative to the prior version):
  //   - Diff view (lineDiff is O(N*M); even with size caps it was
  //     a hot path during streaming for medium-sized inputs).
  //   - Live markdown render of streaming response.
  //   - Per-chunk stats $derived chain (chars / words / lines /
  //     tokens / throughput) — the timer alone caused dozens of
  //     re-renders per second.
  //   - Preset chip grid (~13 chips × 4 groups). The editor's More
  //     menu is the canonical preset entry; this dialog stays
  //     focused on the "show me the response" surface.
  //   - Recent-prompt history row.
  //   - Auto-fire countdown indicator.
  //   - Big-input warning chip.
  //   - Provider / model metadata strip.
  //
  // Auto-fire (from presetInstruction) is preserved so existing
  // bar/menu wiring keeps working with no caller changes.
  //
  // If a user wants the rich features back later, they live in git
  // history (commits up to ~e5e2e9fc) — re-introduce one at a time
  // with profiling each step so the freeze doesn't recur.

  interface Props {
    request: AskAIRequest | null;
    /** Path of the source note — passed to /chat so the AI has note
     *  context for free. Same shape as the previous component. */
    sourcePath: string;
    onDismiss: () => void;
  }
  let { request, sourcePath, onDismiss }: Props = $props();

  // ── State machine ────────────────────────────────────────────────
  // Simple finite states: 'idle' (waiting for user to send) →
  // 'pending' (streaming) → 'done' (response ready). Errors set
  // 'done' with `error` populated. Reset to 'idle' on close.
  type DialogState = 'idle' | 'pending' | 'done';
  let dialogState = $state<DialogState>('idle');
  let instruction = $state('');
  let response = $state('');
  let error = $state('');
  let abortCtl: AbortController | null = $state(null);
  let inputEl: HTMLTextAreaElement | undefined = $state();
  let autoFireTimer: ReturnType<typeof setTimeout> | null = null;
  const AUTO_FIRE_DELAY_MS = 400;

  // Reset the dialog state when `request` flips from null → set
  // (a new AI feature was clicked). Bounded set of state writes:
  // everything resets to a known idle baseline. The preset is
  // applied here, and an auto-fire is scheduled if one is present.
  $effect(() => {
    if (!request) return;
    // Always reset before applying the new request — the previous
    // session's state can't leak into this one.
    response = '';
    error = '';
    dialogState = 'idle';
    abortCtl?.abort();
    abortCtl = null;
    if (autoFireTimer !== null) {
      clearTimeout(autoFireTimer);
      autoFireTimer = null;
    }
    if (request.presetInstruction && request.presetInstruction.trim()) {
      instruction = request.presetInstruction.trim();
      // Focus the textarea first, then auto-fire after a short
      // window so the user can cancel by typing. Same UX cadence
      // as the prior version, much smaller surface area.
      tick().then(() => {
        inputEl?.focus();
        autoFireTimer = setTimeout(() => {
          autoFireTimer = null;
          void ask();
        }, AUTO_FIRE_DELAY_MS);
      });
    } else {
      // No preset: leave the textarea empty (no localStorage
      // round-trip in the minimal version — caller-driven prefill
      // is the only way text shows up).
      instruction = '';
      tick().then(() => inputEl?.focus());
    }
  });

  // Preview cap. The full `request.text` is what we send to the AI;
  // the on-screen <pre> only ever renders this slice. 2000 chars is
  // generous (a paragraph + change) and keeps the browser's layout
  // engine well below the 100KB-text-node freeze threshold.
  const PREVIEW_CHAR_CAP = 2000;
  const previewText = $derived(
    !request?.text
      ? ''
      : request.text.length <= PREVIEW_CHAR_CAP
        ? request.text
        : request.text.slice(0, PREVIEW_CHAR_CAP)
  );
  const previewTruncated = $derived((request?.text.length ?? 0) > PREVIEW_CHAR_CAP);
  const fullCharCount = $derived(request?.text.length ?? 0);

  // ── Streaming ────────────────────────────────────────────────────
  // Two hard constraints behind this implementation:
  //   1. We do NOT render `response` while pending. Chunks accumulate
  //      into the buffer; the response state only updates ONCE, on
  //      onDone. The during-stream UI is a centered spinner + Stop.
  //   2. The rafThrottle helper still wraps the per-chunk apply,
  //      but apply is essentially a no-op (writes to a non-reactive
  //      `pendingBuffer`). State writes only happen on onDone /
  //      onError, so reactivity churn during streaming is zero.
  async function ask() {
    if (!request || dialogState === 'pending') return;
    if (autoFireTimer !== null) {
      clearTimeout(autoFireTimer);
      autoFireTimer = null;
    }
    dialogState = 'pending';
    response = '';
    error = '';
    let buffer = '';
    abortCtl = new AbortController();
    const userMessage = instruction.trim()
      ? `${instruction.trim()}\n\n---\n\n${request.text}`
      : `Help me with this:\n\n${request.text}`;
    // rAF coalescer kept for symmetry / future use, but the apply
    // function deliberately doesn't write reactive state. Buffer
    // accumulation happens in onChunk; state writes happen on
    // onDone. Zero reactive work during streaming.
    const t = rafThrottle(() => {
      // No-op: buffer-only write is done in onChunk below.
    });
    try {
      await api.chatStream(
        [{ role: 'user', content: userMessage }],
        sourcePath || undefined,
        {
          onChunk: (chunk) => {
            buffer += chunk;
            // Drain rAF queue so the throttle's internal frame
            // counter resets cleanly; even though apply is a no-op,
            // this keeps the cancel semantics correct.
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
            response = buffer; // surface whatever arrived before the failure
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

  async function saveAsNote() {
    if (!response) return;
    const stamp = new Date().toISOString().slice(0, 16).replace(/[:T]/g, '-');
    const clean = instruction
      .trim()
      .replace(/[\\/:*?"<>|]/g, '')
      .replace(/\s+/g, ' ')
      .slice(0, 40)
      .trim();
    const guess = clean ? `AI/${clean} ${stamp}.md` : `AI/Response ${stamp}.md`;
    const raw = prompt('Save AI response as a new note.\n\nVault-relative path (must end in .md):', guess);
    if (!raw) return;
    let path = raw.trim();
    if (!path) return;
    if (!path.toLowerCase().endsWith('.md')) path += '.md';
    if (path.startsWith('/') || path.split('/').some((seg) => seg === '..')) {
      toast.error('Invalid path — must be vault-relative.');
      return;
    }
    try {
      await api.createNote({ path, body: response });
      toast.success(`Saved to ${path}`);
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
    if (autoFireTimer !== null) {
      clearTimeout(autoFireTimer);
      autoFireTimer = null;
    }
    abortCtl?.abort();
    abortCtl = null;
    // Reset to idle baseline so a reopen starts fresh.
    response = '';
    error = '';
    dialogState = 'idle';
    onDismiss();
  }
  function dismiss() {
    if (dialogState === 'pending') {
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

  // Belt-and-braces cleanup on unmount — kill any in-flight stream
  // + clear the auto-fire timer so a quickly-closed dialog can't
  // leak a setTimeout or a pending fetch.
  onMount(() => {
    return () => {
      if (autoFireTimer !== null) clearTimeout(autoFireTimer);
      abortCtl?.abort();
    };
  });
</script>

{#if request}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="fixed inset-0 z-50 flex items-end sm:items-start justify-center sm:pt-12 sm:px-4 bg-black/60"
    onclick={dismiss}
    onkeydown={onKey}
    role="presentation"
  >
    <section
      class="w-full sm:max-w-2xl bg-base border border-surface1 shadow-xl max-h-[92dvh] sm:max-h-[88vh] flex flex-col"
      onclick={(e) => e.stopPropagation()}
      role="dialog"
      aria-label="Ask AI about selection"
    >
      <!-- Mobile drag handle hint (visual only — tap-outside / X to dismiss). -->
      <div class="sm:hidden flex justify-center pt-2 pb-1">
        <span class="block w-10 h-1 bg-surface2"></span>
      </div>

      <header class="px-3 py-2 border-b border-surface1 flex items-baseline gap-2">
        <h2 class="text-sm font-semibold text-text flex-1">Ask AI</h2>
        <span class="text-[11px] text-dim font-mono tabular-nums">
          {fullCharCount.toLocaleString()} chars
        </span>
        <button
          onclick={dismiss}
          aria-label="close"
          class="text-dim hover:text-text text-lg leading-none ml-2"
        >×</button>
      </header>

      <div class="flex-1 overflow-y-auto p-2 sm:p-3 space-y-2">
        <!-- Selection preview. Render at most PREVIEW_CHAR_CAP chars
             so a 100KB note body can't lock the browser's layout
             engine. The AI receives the full text in ask(). -->
        <div>
          <span class="block text-[11px] uppercase tracking-wider text-dim mb-1 flex items-baseline gap-2">
            <span>Your selection</span>
            {#if previewTruncated}
              <span
                class="text-[10px] text-warning normal-case tracking-normal"
                title="The full text is sent to the AI; only this preview is truncated for speed."
              >preview truncated · {(fullCharCount - PREVIEW_CHAR_CAP).toLocaleString()} more chars sent</span>
            {/if}
          </span>
          <pre
            class="bg-surface0 border border-surface1 px-3 py-2 text-xs text-subtext whitespace-pre-wrap break-words max-h-32 overflow-y-auto font-mono"
          >{previewText}{#if previewTruncated}…{/if}</pre>
        </div>

        <!-- Instruction. Empty means "Help me with this:". -->
        <div>
          <label
            for="ai-instruction"
            class="block text-[11px] uppercase tracking-wider text-dim mb-1"
          >Instruction (optional)</label>
          <textarea
            id="ai-instruction"
            bind:this={inputEl}
            bind:value={instruction}
            rows="2"
            placeholder="What should the AI do?"
            class="w-full px-3 py-2 bg-surface0 border border-surface1 text-sm text-text focus:outline-none focus:border-primary"
            disabled={dialogState === 'pending'}
            enterkeyhint="send"
            autocomplete="off"
            autocapitalize="sentences"
            spellcheck="true"
          ></textarea>
        </div>

        <!-- Response panel. Three discrete states — no in-betweens.
             - idle: nothing shown beyond the input.
             - pending: spinner + "streaming..." text + Stop. No live
                        text. The response buffer accumulates off-DOM.
             - done: full response in a plain <pre>, no markdown
                     parse. Error overlay on top when present. -->
        {#if dialogState === 'pending'}
          <div class="bg-surface0 border border-surface1 px-3 py-4 text-sm text-dim italic flex items-center gap-2">
            <span class="ai-spinner" aria-hidden="true"></span>
            Streaming — response will appear when the AI finishes. Press Stop to abort.
          </div>
        {:else if dialogState === 'done'}
          {#if error}
            <div class="bg-surface0 border border-error px-3 py-2 text-xs text-error">
              {error}
              {#if /provider|api key|not configured/i.test(error)}
                <div class="text-dim mt-1">
                  Open <a href="/settings" class="text-secondary hover:underline">Settings</a> to configure an AI provider.
                </div>
              {/if}
            </div>
          {/if}
          {#if response}
            <div>
              <div class="flex items-baseline gap-2 mb-1">
                <span class="text-[11px] uppercase tracking-wider text-dim">Response</span>
                <span class="text-[10px] text-dim font-mono tabular-nums">
                  {response.length.toLocaleString()} chars
                </span>
              </div>
              <pre
                class="bg-surface0 border border-surface1 px-3 py-2 text-sm text-text whitespace-pre-wrap break-words max-h-72 overflow-y-auto"
              >{response}</pre>
            </div>
          {/if}
        {/if}
      </div>

      <footer
        class="px-4 py-3 border-t border-surface1 flex items-center gap-2 flex-wrap"
        style="padding-bottom: max(0.75rem, env(safe-area-inset-bottom));"
      >
        {#if dialogState === 'pending'}
          <button
            type="button"
            onclick={stop}
            class="px-3 py-2 sm:py-1.5 min-h-[44px] sm:min-h-0 text-sm bg-surface0 text-error border border-error hover:bg-surface1"
          >Stop</button>
          <span class="flex-1"></span>
          <span class="text-[11px] text-dim italic">Streaming · cancel anytime</span>
        {:else}
          <button
            type="button"
            onclick={dismiss}
            class="px-3 py-2 sm:py-1.5 min-h-[44px] sm:min-h-0 text-sm text-subtext hover:bg-surface0"
          >Cancel</button>
          {#if response}
            <button
              type="button"
              onclick={copyResponse}
              class="px-3 py-2 sm:py-1.5 min-h-[44px] sm:min-h-0 text-sm bg-surface0 text-text border border-surface1 hover:border-primary"
            >Copy</button>
            <button
              type="button"
              onclick={saveAsNote}
              class="px-3 py-2 sm:py-1.5 min-h-[44px] sm:min-h-0 text-sm bg-surface0 text-text border border-surface1 hover:border-primary"
            >Save as note</button>
            <button
              type="button"
              onclick={insertBelow}
              class="px-3 py-2 sm:py-1.5 min-h-[44px] sm:min-h-0 text-sm bg-surface0 text-text border border-surface1 hover:border-primary"
            >Insert below</button>
            <button
              type="button"
              onclick={replaceSelection}
              class="px-3 py-2 sm:py-1.5 min-h-[44px] sm:min-h-0 text-sm bg-primary text-on-primary font-medium hover:opacity-90"
            >Replace</button>
          {:else}
            <span class="flex-1"></span>
            <button
              type="button"
              onclick={ask}
              class="px-3 py-2 sm:py-1.5 min-h-[44px] sm:min-h-0 text-sm bg-primary text-on-primary font-medium hover:opacity-90"
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
  @keyframes ai-spin {
    to { transform: rotate(360deg); }
  }
</style>
