<script lang="ts">
  // Quick-jot composer for /jots. Amplenote-style "fire a thought into
  // today" — the textarea + AI-expand toggle + submit button. When
  // AI-expand is ON, hitting Enter routes the draft through the model
  // first; the streaming preview lives in this component too so the
  // user sees Keep/Discard before any save touches today's daily.
  //
  // Save mechanics (writing under `## Jots`, refetching the date, WS
  // round-trip) stay on the page — this file only renders the chrome
  // and bubbles intents back via callbacks.
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';

  type Props = {
    text: string;
    composerEl?: HTMLTextAreaElement;
    busy: boolean;
    expand: boolean;
    expanding: boolean;
    expandedText: string;
    onSubmit: () => void;
    onToggleExpand: () => void;
    onDiscardExpand: () => void;
    onKeepExpand: () => void;
    onRunExpand: () => void;
    onWikilink: (target: string) => void;
  };

  let {
    text = $bindable(),
    composerEl = $bindable(),
    busy,
    expand,
    expanding,
    expandedText,
    onSubmit,
    onToggleExpand,
    onDiscardExpand,
    onKeepExpand,
    onRunExpand,
    onWikilink
  }: Props = $props();
</script>

<div class="mb-3 bg-surface0 border border-surface1 rounded focus-within:border-primary transition-colors">
  <div class="flex items-start gap-1.5 px-2 py-1.5">
    <textarea
      bind:this={composerEl}
      bind:value={text}
      onkeydown={(e) => {
        // Enter (without shift) submits; Shift+Enter inserts a newline.
        // Cmd/Ctrl+Enter also submits as a power-user convenience.
        if (e.key === 'Enter' && (!e.shiftKey || e.metaKey || e.ctrlKey)) {
          e.preventDefault();
          onSubmit();
        }
      }}
      placeholder={expand ? 'jot a seed thought — AI will expand on Enter' : 'jot a thought — Enter saves, Shift+Enter newline'}
      rows="1"
      disabled={busy || expanding || expandedText.length > 0}
      class="flex-1 bg-transparent text-sm text-text placeholder-dim focus:outline-none resize-y disabled:opacity-50 leading-snug"
    ></textarea>
    <button
      type="button"
      onclick={onToggleExpand}
      aria-pressed={expand}
      class="text-[11px] px-1.5 py-1 rounded {expand ? 'bg-primary text-on-primary' : 'bg-surface1 text-dim hover:bg-surface2 hover:text-text'} font-mono shrink-0"
      title={expand ? 'AI-expand: ON — Enter will expand your draft before saving' : 'AI-expand: OFF — Enter saves verbatim'}
    >AI</button>
    <button
      type="button"
      onclick={onSubmit}
      disabled={busy || expanding || expandedText.length > 0 || !text.trim()}
      class="text-[11px] px-2 py-1 rounded bg-primary text-on-primary font-medium hover:opacity-90 disabled:opacity-40 shrink-0"
    >{busy ? '…' : expand ? 'expand' : 'add'}</button>
  </div>

  {#if expanding || expandedText.length > 0}
    <div class="border-t border-surface1 px-2 py-1.5">
      <div class="flex items-baseline gap-2 mb-1">
        <span class="text-[10px] uppercase tracking-wider text-text font-mono">AI expansion</span>
        {#if expanding}
          <span class="text-[10px] text-dim italic font-mono">streaming…</span>
          <span class="flex-1"></span>
          <button
            type="button"
            onclick={onDiscardExpand}
            class="text-[10px] text-dim hover:text-text font-mono"
          >stop</button>
        {:else}
          <span class="flex-1"></span>
          <button
            type="button"
            onclick={onRunExpand}
            class="text-[10px] text-text hover:underline font-mono"
            title="re-run the AI on the same seed text"
          >try again</button>
          <button
            type="button"
            onclick={onKeepExpand}
            class="text-[10px] px-1.5 py-0.5 rounded bg-primary text-on-primary font-medium hover:opacity-90"
          >keep</button>
          <button
            type="button"
            onclick={onDiscardExpand}
            class="text-[10px] text-dim hover:text-text font-mono"
          >discard</button>
        {/if}
      </div>
      <div class="bg-mantle border border-surface1 rounded p-2 max-h-[20rem] overflow-y-auto">
        {#if expandedText.trim()}
          <MarkdownRenderer body={expandedText} onWikilink={onWikilink} />
        {:else}
          <p class="text-xs text-dim italic">…</p>
        {/if}
      </div>
    </div>
  {/if}
</div>
