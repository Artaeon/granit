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
    regenerateInlineAI
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
      const barWidth = 280; // approximate; clamps below for narrow phones
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
    {/if}
  </div>
{/if}
