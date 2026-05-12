<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { api } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import type { AskAIRequest } from '$lib/editor/ask-ai';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';
  import { lineDiff } from '$lib/util/lineDiff';
  import { loadStoredString, saveStoredString } from '$lib/util/storage';

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
  // View toggle for the response panel — 'preview' renders as
  // markdown (the default), 'diff' shows a line-by-line LCS diff
  // of the original selection against the AI response. Diff is the
  // killer view for Improve / Fix grammar / Shorten where you want
  // to see exactly what changed before hitting Replace.
  let viewMode = $state<'preview' | 'diff'>('preview');
  let inputEl: HTMLTextAreaElement | undefined = $state();
  // AbortController for the in-flight streaming call. Stored at the
  // component level so the cancel button + Escape key can both
  // close the upstream connection promptly.
  let abortCtl: AbortController | null = $state(null);

  // Quick-pick instructions, grouped by intent. Each is a complete
  // instruction that works across providers; the user can still
  // tweak the field before submitting. Grouping makes the chip wall
  // scannable instead of looking like 13 random buttons in a row.
  const PRESET_GROUPS: { label: string; items: { label: string; text: string }[] }[] = [
    {
      label: 'Transform',
      items: [
        { label: 'Improve', text: 'Improve the writing of the following — clearer, tighter, same voice. Return only the improved text, no preamble.' },
        { label: 'Fix grammar', text: 'Fix grammar and typos in the following. Preserve voice, structure, and meaning. Return only the corrected text.' },
        { label: 'Shorten', text: 'Shorten the following while preserving the key points. Return only the shorter version.' },
        { label: 'Expand', text: 'Expand the following with more detail and context. Keep the same voice. Return only the expanded text.' },
        { label: 'Make formal', text: 'Rewrite the following in a more formal, professional register. Return only the rewritten text.' },
        { label: 'Make casual', text: 'Rewrite the following in a more casual, conversational register. Return only the rewritten text.' }
      ]
    },
    {
      label: 'Extract',
      items: [
        { label: 'Summarise', text: 'Summarise the following in 3 concise bullet points.' },
        { label: 'Outline', text: 'Generate a markdown outline (## headings + bulleted sub-points) of the key ideas in the following. Return only the outline.' },
        { label: 'Tasks', text: 'Extract the actionable items from the following as a markdown task list (`- [ ] task` lines). Each line should be a concrete action. Return ONLY the markdown checklist, no preamble.' },
        { label: 'Tags', text: 'Suggest 5-7 short, lowercase, hyphenated hashtags relevant to the topic of the following. Return ONLY a single line of space-separated tags starting with `#`. Example: `#research #notes-app #productivity`.' },
        { label: 'Continue', text: 'Continue writing from where the following text leaves off, in the same voice and style. Return only the continuation, no preamble.' }
      ]
    },
    {
      label: 'Translate',
      items: [
        { label: 'EN', text: 'Translate the following to English. Return only the translation.' },
        { label: 'DE', text: 'Translate the following to German. Return only the translation.' },
        { label: 'Bullet list', text: 'Rewrite the following as a markdown bullet list. Return only the list.' }
      ]
    },
    {
      // Reflect — the dialog's "thinking" surface as opposed to its
      // "rewriting" surface. Output here is meant to be READ next to
      // the original (replace mode rarely makes sense), so each
      // prompt frames the response as commentary rather than a
      // drop-in rewrite. Insert-Below is the natural action.
      label: 'Reflect',
      items: [
        { label: 'Explain', text: 'Explain the following clearly. Assume the reader knows the broad surrounding context but not this specific topic. Return a short markdown explanation — definitions, intuition, one concrete example. No preamble.' },
        { label: 'Steel-man', text: 'Steel-man the argument in the following. Begin with "**Strongest version:**" and write the most charitable, most rigorous version of the argument. Then on a new line "**Weak points:**" with 2-3 honest weaknesses. Return only this markdown block.' },
        { label: 'Counter', text: 'Argue the opposite of the position in the following. Make the strongest possible case for the contrary view in 2-4 sentences. Return only the counter-argument prose, no preamble.' },
        { label: 'Connect', text: 'What ideas, concepts, or fields does the following connect to? Return a markdown bullet list of 5 connections, each with one line of why. Return ONLY the list.' },
        { label: 'Question', text: 'Generate 5 sharp, non-leading questions that would deepen understanding of the following. Return ONLY a numbered markdown list, one question per line, no preamble.' }
      ]
    }
  ];

  // Last-used instruction persists across sessions so a re-open of
  // the dialog over a different selection lets the user re-run the
  // same prompt with one click. localStorage-backed; size-bounded so
  // a giant prompt doesn't blow the quota.
  const LAST_INSTRUCTION_KEY = 'granit.ai.lastInstruction';
  function loadLastInstruction(): string {
    return loadStoredString(LAST_INSTRUCTION_KEY, '');
  }
  function saveLastInstruction(s: string): void {
    if (s.length > 0 && s.length < 500) saveStoredString(LAST_INSTRUCTION_KEY, s);
  }

  // Preset-path delay before auto-fire. The user picked the preset
  // on the bar so they EXPECT a request to fire — but flashing the
  // dialog open and immediately streaming gives no window for "wait,
  // I meant a different selection" / "let me tweak the instruction".
  // 400ms is long enough to read what's about to be sent + reach
  // for the input, short enough that the user doesn't feel like
  // they're waiting. Any keystroke in the instruction box cancels
  // the timer (the user is editing; that IS their cancel).
  let autoFireTimer: ReturnType<typeof setTimeout> | null = $state(null);
  let autoFireCountdown = $state(false);
  const AUTO_FIRE_DELAY_MS = 400;

  function cancelAutoFire() {
    if (autoFireTimer !== null) {
      clearTimeout(autoFireTimer);
      autoFireTimer = null;
    }
    autoFireCountdown = false;
  }

  $effect(() => {
    if (request) {
      response = '';
      // Cancel any leftover countdown from a previous open before
      // we (maybe) start a new one — protects against rapid
      // preset → close → different-preset opens stacking timers.
      cancelAutoFire();
      // When the bar (or any host) passed a presetInstruction, honour
      // it AND auto-fire AFTER a short cancellable delay. The user
      // already picked the action on the bar; the delay gives them
      // a beat to edit if they want.
      if (request.presetInstruction && request.presetInstruction.trim()) {
        instruction = request.presetInstruction.trim();
        viewMode = request.presetView ?? (isRewriteInstruction(instruction) ? 'diff' : 'preview');
        error = '';
        pending = false;
        autoFireCountdown = true;
        tick().then(() => {
          inputEl?.focus();
          autoFireTimer = setTimeout(() => {
            autoFireTimer = null;
            autoFireCountdown = false;
            void ask();
          }, AUTO_FIRE_DELAY_MS);
        });
      } else {
        // Pre-fill with last-used instruction so a "summarise this"
        // workflow becomes one click on subsequent selections. The
        // user can still clear or change before sending.
        instruction = loadLastInstruction();
        error = '';
        pending = false;
        viewMode = 'preview';
        tick().then(() => inputEl?.focus());
      }
    }
  });

  // Diff helpers live in $lib/util/lineDiff so the version-history
  // panel can share them. Same LCS algorithm, same DiffLine shape.
  const diff = $derived(
    request && response ? lineDiff(request.text, response) : []
  );
  const diffStatsView = $derived.by(() => {
    let added = 0;
    let removed = 0;
    for (const l of diff) {
      if (l.type === 'add') added++;
      else if (l.type === 'del') removed++;
    }
    return { added, removed };
  });

  async function ask() {
    if (!request || pending) return;
    pending = true;
    error = '';
    response = '';
    saveLastInstruction(instruction.trim());
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

  // Heuristic for "this is a rewrite, not a generation." When the
  // user picks Improve / Fix grammar / Shorten / Expand / Make
  // formal / Make casual the most useful default view is the diff
  // — the response is a transformation of the input. Generations
  // (Summarise, Outline, Tasks, Tags, Continue, Translate, Bullet
  // list) produce new content where a diff is misleading. The
  // function is conservative: any rewrite verb in the instruction
  // flips to diff; anything else stays preview.
  function isRewriteInstruction(s: string): boolean {
    const lower = s.toLowerCase();
    return (
      lower.includes('improve the writing') ||
      lower.includes('fix grammar') ||
      lower.includes('shorten the following') ||
      lower.includes('expand the following') ||
      lower.includes('rewrite the following in a more formal') ||
      lower.includes('rewrite the following in a more casual')
    );
  }

  function pickQuick(text: string) {
    // Cancel any pending preset-auto-fire so picking a different
    // chip mid-countdown doesn't race the queued instruction.
    cancelAutoFire();
    instruction = text;
    // Auto-switch to diff for rewrite-style presets so the user
    // sees what changed at a glance. The toggle stays available
    // — user can flip back to preview anytime.
    viewMode = isRewriteInstruction(text) ? 'diff' : 'preview';
    void ask();
  }

  function regenerate() {
    if (!request || pending || !instruction.trim()) return;
    void ask();
  }

  // Size + rough token estimate for the dialog header. ~4
  // chars/token is the industry rule of thumb across English
  // text and most LLM tokenisers; close enough for a "should
  // I worry about cost?" gut check. bigInput flips a warning
  // chip when the input is large enough to matter (>16k chars
  // ≈ 4k tokens, a meaningful chunk of a context window).
  const charCount = $derived(request?.text.length ?? 0);
  const tokenEstimate = $derived(Math.round(charCount / 4));
  const bigInput = $derived(charCount > 16000);

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
    // Clear any pending preset auto-fire so a closed-then-reopened
    // dialog doesn't accidentally fire the previous request.
    cancelAutoFire();
    onDismiss();
  }

  function dismiss() {
    // While streaming, ESC / backdrop-click stops the stream rather
    // than sitting locked. Same affordance as the Stop button.
    if (pending) {
      stop();
      return;
    }
    cancelAutoFire();
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
  <!-- Mobile slides up from the bottom (items-end + rounded-t),
       desktop centers (items-start + pt-12 + rounded-lg). dvh
       (dynamic viewport) accounts for iOS Safari's address bar
       so a long preset list doesn't get clipped when the bar is
       visible. The presets row is dense — on a phone it fits two
       rows of chips before the response panel begins. -->
  <div
    class="fixed inset-0 z-50 flex items-end sm:items-start justify-center sm:pt-12 sm:px-4 bg-black/60"
    onclick={dismiss}
    onkeydown={onKey}
    role="presentation"
  >
    <section
      class="w-full sm:max-w-2xl bg-base border border-surface1 rounded-t-xl sm:rounded-lg shadow-xl max-h-[92dvh] sm:max-h-[88vh] flex flex-col"
      onclick={(e) => e.stopPropagation()}
      role="dialog"
      aria-label="Ask AI about selection"
    >
      <!-- Drag-handle visual hint on mobile, the iOS/Android sheet
           convention. Doesn't actually drag — tap-outside or X to
           dismiss — but signals "this is a sheet" without a
           native widget. -->
      <div class="sm:hidden flex justify-center pt-2 pb-1">
        <span class="block w-10 h-1 rounded-full bg-surface2"></span>
      </div>
      <header class="px-4 py-3 border-b border-surface1 flex items-baseline gap-2">
        <h2 class="text-sm font-semibold text-text flex-1">Ask AI</h2>
        <span class="text-[11px] text-dim font-mono">
          {charCount.toLocaleString()} chars · ~{tokenEstimate.toLocaleString()} tok
        </span>
        {#if bigInput}
          <span
            class="text-[10px] px-1.5 py-0.5 rounded bg-surface0 text-warning border border-warning"
            title="Large input — costs more tokens. Consider selecting a portion instead of the whole note."
          >big</span>
        {/if}
        <button
          onclick={dismiss}
          aria-label="close"
          class="text-dim hover:text-text text-lg leading-none ml-2"
        >×</button>
      </header>

      <div class="flex-1 overflow-y-auto p-2 sm:p-3 space-y-2">
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
          <div class="relative">
            <textarea
              id="ai-instruction"
              bind:this={inputEl}
              bind:value={instruction}
              oninput={cancelAutoFire}
              onkeydown={cancelAutoFire}
              rows="2"
              placeholder="What should the AI do? (or pick a preset below)"
              class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
              disabled={pending}
            ></textarea>
            {#if autoFireCountdown}
              <!-- Tiny countdown pill — surfaces that a preset is
                   queued to auto-fire so the user isn't surprised
                   when it does, and tells them how to back out
                   ("type to edit"). The pill clears the moment
                   cancelAutoFire runs (any key in the textarea). -->
              <div class="absolute right-2 top-2 inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-surface1 text-[10px] text-primary pointer-events-none">
                <span class="inline-block w-1.5 h-1.5 rounded-full bg-primary animate-pulse"></span>
                <span>preset queued · type to edit</span>
              </div>
            {/if}
          </div>
          <!-- Grouped presets — Transform / Extract / Translate.
               Each group is a labeled row of chips so the wall of
               13 buttons reads as scannable categories. Group label
               on the left, chips flow on the right. -->
          <div class="space-y-1.5 mt-2">
            {#each PRESET_GROUPS as group}
              <div class="flex items-center gap-2 flex-wrap">
                <span class="text-[10px] uppercase tracking-wider text-dim w-16 flex-shrink-0">{group.label}</span>
                {#each group.items as q}
                  <button
                    type="button"
                    onclick={() => pickQuick(q.text)}
                    disabled={pending}
                    class="px-2 py-0.5 text-[11px] rounded bg-surface0 text-subtext hover:bg-surface1 hover:text-text disabled:opacity-50 transition-colors"
                  >{q.label}</button>
                {/each}
              </div>
            {/each}
          </div>
        </div>

        <!-- Response area. Streams progressively — chunks land in
             `response` as they arrive, so the user sees the AI
             "type" in real time. The pending+empty state shows just
             the spinner; pending+streaming shows the partial
             response with a small streaming indicator above it. -->
        {#if error}
          <div class="text-xs text-error border border-error bg-surface0 rounded px-3 py-2">
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
            <div class="flex items-baseline gap-2 mb-1">
              <span class="text-[11px] uppercase tracking-wider text-dim">Response</span>
              <!-- Preview / Diff toggle. Diff is unbeatable for
                   "did the AI actually change what I wanted" — the
                   stats badge shows +N/-M lines so the user knows
                   at a glance how invasive the rewrite was. -->
              {#if !pending}
                <div class="inline-flex bg-surface0 border border-surface1 rounded overflow-hidden text-[10px]">
                  <button
                    type="button"
                    onclick={() => (viewMode = 'preview')}
                    class="px-2 py-0.5 {viewMode === 'preview' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
                  >Preview</button>
                  <button
                    type="button"
                    onclick={() => (viewMode = 'diff')}
                    class="px-2 py-0.5 {viewMode === 'diff' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
                  >Diff</button>
                </div>
                {#if viewMode === 'diff'}
                  <span class="text-[10px] font-mono">
                    <span class="text-success">+{diffStatsView.added}</span>
                    <span class="text-error ml-1">−{diffStatsView.removed}</span>
                  </span>
                {/if}
              {/if}
              <span class="flex-1"></span>
              {#if pending}
                <span class="text-[11px] text-secondary flex items-center gap-1.5">
                  <span class="ai-spinner ai-spinner--sm" aria-hidden="true"></span>
                  streaming
                </span>
              {:else if instruction.trim()}
                <!-- Regenerate — re-fires the same instruction with
                     a fresh stream. Saves the close/reopen dance
                     when the first response wasn't what the user
                     wanted. Hidden during streaming and when the
                     instruction is blank (nothing to regenerate). -->
                <button
                  type="button"
                  onclick={regenerate}
                  class="text-[11px] text-secondary hover:underline"
                  title="Re-run the same instruction"
                >↻ regenerate</button>
              {/if}
            </div>
            {#if viewMode === 'diff' && !pending}
              <!-- Diff view: line-by-line LCS over the original
                   selection vs the AI response. Adds in green,
                   removes in red, unchanged in dim text — same
                   visual grammar as `git diff` so any developer
                   reads it instantly. Whitespace pre-wrap so
                   long lines wrap rather than horizontally
                   scrolling on mobile. -->
              <div class="bg-surface0 border border-surface1 rounded text-xs font-mono max-h-72 overflow-y-auto">
                {#each diff as l, i (i)}
                  {#if l.type === 'eq'}
                    <div class="px-3 py-0.5 text-dim whitespace-pre-wrap break-words"><span class="opacity-60">  </span>{l.text || ' '}</div>
                  {:else if l.type === 'add'}
                    <div class="px-3 py-0.5 bg-surface0 text-success whitespace-pre-wrap break-words"><span class="opacity-80">+ </span>{l.text || ' '}</div>
                  {:else}
                    <div class="px-3 py-0.5 bg-surface0 text-error whitespace-pre-wrap break-words"><span class="opacity-80">- </span>{l.text || ' '}</div>
                  {/if}
                {/each}
              </div>
            {:else}
              <!-- Markdown-rendered response. AI replies are typically
                   markdown — bullets, headers, code blocks — and the
                   previous plain-text rendering ate all the structure.
                   The renderer's `prose` styles match the editor's
                   reading view so what the user sees here is what
                   they'll get on Replace / Insert. -->
              <div class="bg-surface0 border border-surface1 rounded px-3 py-2 text-sm text-text break-words max-h-72 overflow-y-auto">
                <div class="prose prose-sm max-w-none">
                  <MarkdownRenderer body={response} />
                </div>
              </div>
            {/if}
          </div>
        {/if}
      </div>

      <footer
        class="px-4 py-3 border-t border-surface1 flex items-center gap-2 flex-wrap"
        style="padding-bottom: max(0.75rem, env(safe-area-inset-bottom));"
      >
        {#if pending}
          <button
            type="button"
            onclick={stop}
            class="px-3 py-2 sm:py-1.5 min-h-[44px] sm:min-h-0 text-sm bg-surface0 text-error border border-error rounded hover:bg-surface1"
          >Stop</button>
          <span class="flex-1"></span>
          <span class="text-[11px] text-dim italic">Streaming · cancel anytime</span>
        {:else}
          <button
            type="button"
            onclick={dismiss}
            class="px-3 py-2 sm:py-1.5 min-h-[44px] sm:min-h-0 text-sm text-subtext hover:bg-surface0 rounded"
          >Cancel</button>
          {#if response}
            <button
              type="button"
              onclick={copyResponse}
              class="px-3 py-2 sm:py-1.5 min-h-[44px] sm:min-h-0 text-sm bg-surface0 text-text border border-surface1 rounded hover:border-primary"
            >Copy</button>
            <button
              type="button"
              onclick={insertBelow}
              class="px-3 py-2 sm:py-1.5 min-h-[44px] sm:min-h-0 text-sm bg-surface0 text-text border border-surface1 rounded hover:border-primary"
            >Insert below</button>
            <button
              type="button"
              onclick={replaceSelection}
              class="px-3 py-2 sm:py-1.5 min-h-[44px] sm:min-h-0 text-sm bg-primary text-on-primary rounded font-medium hover:opacity-90"
            >Replace selection</button>
          {:else}
            <span class="flex-1"></span>
            <button
              type="button"
              onclick={ask}
              class="px-3 py-2 sm:py-1.5 min-h-[44px] sm:min-h-0 text-sm bg-primary text-on-primary rounded font-medium hover:opacity-90"
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
