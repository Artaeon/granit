<!--
  AIActionBar — the floating button cluster that appears alongside an
  inline-AI ghost. The keyboard chords (Tab/Cmd-Enter/Esc/Cmd-R) still
  work; this surface is the click-discoverable equivalent so users
  who don't know the chords aren't stranded inside a streaming
  ghost.

  Notion-faithful states
    streaming  →  [⏹ Stop] · "AI is writing…"
                  The only action mid-stream is to abort. Stop fires
                  rejectInlineAI which both aborts the request and
                  clears the ghost.
    settled    →  [✓ Keep] [↻ Try again] [✕ Discard]
                  Three primary affordances after the stream finishes.
                  Keep commits the ghost into the doc; Try again
                  re-runs the exact same request; Discard clears.

  Positioning
    The bar is fixed-position in viewport coords. The host page
    feeds in the EditorView so we can call coordsAtPos on the
    relevant doc position whenever the ghost state changes.
      • insert/append modes — anchor below the line containing the
        ghost's start position.
      • replace mode — anchor below the line containing the END of
        the range being replaced, so the bar sits next to what's
        about to change.
    We clamp to the viewport so the bar never overflows on narrow
    screens / late-paragraph edits.
-->
<script lang="ts">
  import type { EditorView } from '@codemirror/view';
  import {
    type InlineAIState,
    acceptInlineAI,
    rejectInlineAI,
    regenerateInlineAI,
    streamInlineAI
  } from '$lib/editor/inline-ai';

  interface Props {
    view: EditorView | undefined;
    state: InlineAIState | null;
  }
  let { view, state }: Props = $props();

  // Position is a $state object that we mutate from an effect. We
  // intentionally don't derive it because the inputs (view + state)
  // are mutable references that Svelte can't track through deeply.
  let pos = $state({ left: 0, top: 0, visible: false });

  // Reposition whenever the ghost activates or moves, AND whenever
  // the editor scrolls (the bar uses `position: fixed` in viewport
  // coords, so any scroll that moves the anchor's screen position
  // must rerun the calculation). Text changes don't move the anchor,
  // but they do trigger a state observer fire — cheap to recompute
  // anyway, so we accept the small re-flow cost over the complexity
  // of memoizing.
  $effect(() => {
    if (!view || !state || !state.active) {
      pos = { left: 0, top: 0, visible: false };
      return;
    }
    const v = view;
    const s = state;
    function reposition() {
      // For insert/append: anchor sits at the ghost's start.
      // For replace: replaceTo is the end of the range being replaced.
      const probe = s.kind === 'replace' ? s.replaceTo : s.anchor;
      const coords = v.coordsAtPos(probe);
      if (!coords) {
        pos = { left: 0, top: 0, visible: false };
        return;
      }
      const vw = window.innerWidth;
      const vh = window.innerHeight;
      const margin = 8;
      // Approximate width — settled mode renders three buttons + a
      // follow-up input, streaming mode renders the writing label +
      // Stop. We clamp against the wider of the two so neither mode
      // ever overflows the viewport on narrow phones.
      const barWidth = s.streaming ? 200 : 420;
      let left = coords.left;
      let top = coords.bottom + 4;
      if (left + barWidth > vw - margin) left = vw - margin - barWidth;
      if (left < margin) left = margin;
      // Flip above the line if we'd overflow the bottom of the viewport.
      if (top + 40 > vh - margin) top = Math.max(margin, coords.top - 36);
      pos = { left, top, visible: true };
    }
    reposition();
    const scrollDOM = v.scrollDOM;
    scrollDOM.addEventListener('scroll', reposition);
    window.addEventListener('resize', reposition);
    // Capture-phase window scroll catches any ancestor that scrolls
    // (sidebars, info panels). Without it the bar would drift when
    // the user scrolls a containing element.
    window.addEventListener('scroll', reposition, true);
    return () => {
      scrollDOM.removeEventListener('scroll', reposition);
      window.removeEventListener('resize', reposition);
      window.removeEventListener('scroll', reposition, true);
    };
  });

  function keep() {
    if (!view) return;
    acceptInlineAI(view);
    view.focus();
  }
  function discard() {
    if (!view) return;
    rejectInlineAI(view);
    view.focus();
  }
  function retry() {
    if (!view) return;
    regenerateInlineAI(view);
    view.focus();
  }

  // Follow-up — Notion-style "refine the result without re-opening
  // the menu". After the stream settles the user can type "make it
  // shorter" / "add an example" / "translate to German" and re-fire
  // the inline-AI request with the previous output as assistant
  // context. The new stream replaces the previous ghost in place.
  let followUp = $state('');

  function submitFollowUp() {
    const instruction = followUp.trim();
    if (!instruction || !view || !state || state.streaming) return;
    const previousOutput = state.text;
    const baseRequest = state.request;
    if (!baseRequest) return;
    // Reuse the request shape — anchor, kind, [from, to], notePath all
    // stay the same. Only the messages change: we replay the previous
    // exchange as assistant context so the model knows what it
    // produced, then layer the user's follow-up on top.
    streamInlineAI(view, {
      ...baseRequest,
      messages: [
        {
          role: 'system',
          content:
            'You previously produced text inside the user\'s note. The user now wants ' +
            'to refine that text. Apply the follow-up instruction and return the UPDATED ' +
            'text only — no preamble, no commentary, no surrounding quotes.'
        },
        { role: 'assistant', content: previousOutput },
        { role: 'user', content: 'Follow-up instruction: ' + instruction }
      ]
    });
    followUp = '';
  }
</script>

{#if state && state.active && pos.visible}
  <div
    class="fixed z-40 flex items-center gap-1 bg-base border border-surface2 rounded shadow-lg p-1 text-[11px] font-mono"
    style="left: {pos.left}px; top: {pos.top}px;"
    role="toolbar"
    aria-label="AI result actions"
  >
    {#if state.streaming}
      <span class="px-1.5 py-0.5 text-dim">AI writing…</span>
      <button
        type="button"
        onclick={discard}
        class="px-1.5 py-0.5 rounded bg-surface0 hover:bg-surface1 text-text"
        title="abort streaming"
      >Stop</button>
    {:else if state.error}
      <!-- Stream failed. We kept the ghost active so the user can
           Retry or Discard without losing the original request. Any
           partial text the model emitted before the error is still
           there (rendered as ghost above), so a Keep would commit
           the partial — useful when long generations rate-limit
           mid-stream. -->
      <span class="px-1.5 py-0.5 text-error" title={state.error}>error: {state.error.slice(0, 60)}{state.error.length > 60 ? '…' : ''}</span>
      <button
        type="button"
        onclick={retry}
        class="px-1.5 py-0.5 rounded bg-primary text-on-primary font-medium hover:opacity-90"
        title="re-run the same request"
      >Retry</button>
      {#if state.text.length > 0}
        <button
          type="button"
          onclick={keep}
          class="px-1.5 py-0.5 rounded bg-surface0 hover:bg-surface1 text-text"
          title="keep what made it through before the error"
        >Keep partial</button>
      {/if}
      <button
        type="button"
        onclick={discard}
        class="px-1.5 py-0.5 rounded bg-surface0 hover:bg-surface1 text-dim hover:text-text"
        title="throw it away"
      >Discard</button>
    {:else}
      <button
        type="button"
        onclick={keep}
        class="px-1.5 py-0.5 rounded bg-primary text-on-primary font-medium hover:opacity-90"
        title="commit the AI result into the note (Tab)"
      >Keep</button>
      <button
        type="button"
        onclick={retry}
        class="px-1.5 py-0.5 rounded bg-surface0 hover:bg-surface1 text-text"
        title="re-run the same request (⌘R)"
      >Try again</button>
      <button
        type="button"
        onclick={discard}
        class="px-1.5 py-0.5 rounded bg-surface0 hover:bg-surface1 text-dim hover:text-text"
        title="throw away the result (Esc)"
      >Discard</button>
      <!-- Follow-up — type a refinement instruction and Enter re-fires
           the same inline-AI request with the previous output as
           assistant context. Notion-style "tell AI what to change". -->
      <input
        type="text"
        bind:value={followUp}
        onkeydown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault();
            submitFollowUp();
          } else if (e.key === 'Escape') {
            // Esc inside the follow-up input clears it without
            // discarding the ghost — the user can keep iterating.
            // A second Esc with empty input falls through to the
            // editor's Escape handler which calls rejectInlineAI.
            if (followUp.length > 0) {
              e.preventDefault();
              e.stopPropagation();
              followUp = '';
            }
          }
        }}
        placeholder="refine: shorter, add example, translate…"
        class="flex-1 min-w-[8rem] bg-surface0 border border-surface1 rounded px-1.5 py-0.5 text-text placeholder-dim focus:outline-none focus:border-primary"
      />
    {/if}
  </div>
{/if}
