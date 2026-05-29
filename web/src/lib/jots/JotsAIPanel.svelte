<script lang="ts">
  // Multi-mode AI panel for /jots. Three streamed surfaces sharing one
  // chrome: themes (clickable theme chips that become search terms),
  // ask (free-form Q&A over loaded jots), and digest (a structured
  // weekly synthesis the user can save as a note).
  //
  // The page owns the streaming state + abort controller; this
  // component just renders the active mode and routes user intents
  // back via callbacks.
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';

  type AIMode = 'themes' | 'ask' | 'digest';
  type Theme = { label: string; query: string };

  type Props = {
    mode: AIMode;
    busy: boolean;
    error: string;
    themes: Theme[];
    askQuestion: string;
    askAnswer: string;
    askInputEl?: HTMLInputElement;
    digestAnswer: string;
    onStop: () => void;
    onRegenerateThemes: () => void;
    onApplyTheme: (t: Theme) => void;
    onSubmitAsk: () => void;
    onCopy: (text: string) => void;
    onRegenerateDigest: () => void;
    onSaveDigestAsNote: () => void;
    onDismiss: () => void;
    onWikilink: (target: string) => void;
  };

  let {
    mode,
    busy,
    error,
    themes,
    askQuestion = $bindable(),
    askAnswer,
    askInputEl = $bindable(),
    digestAnswer,
    onStop,
    onRegenerateThemes,
    onApplyTheme,
    onSubmitAsk,
    onCopy,
    onRegenerateDigest,
    onSaveDigestAsNote,
    onDismiss,
    onWikilink
  }: Props = $props();
</script>

<div class="mt-1.5 p-2 bg-surface1 border border-surface2 rounded">
  <div class="flex items-baseline gap-2 mb-1">
    <h3 class="text-[10px] uppercase tracking-wider text-text font-medium">
      {#if mode === 'themes'}recurring themes
      {:else if mode === 'ask'}ask jots
      {:else if mode === 'digest'}weekly digest
      {/if}
    </h3>
    <span class="flex-1"></span>
    {#if busy}
      <span class="text-[10px] text-dim italic font-mono">streaming…</span>
      <button
        type="button"
        onclick={onStop}
        class="text-[10px] text-dim hover:text-text font-mono"
        title="stop the current stream"
      >stop</button>
    {:else}
      {#if mode === 'themes' && themes.length > 0}
        <button onclick={onRegenerateThemes} class="text-[10px] text-text hover:underline font-mono">regenerate</button>
      {:else if mode === 'ask' && askAnswer.length > 0}
        <button onclick={onSubmitAsk} class="text-[10px] text-text hover:underline font-mono">re-ask</button>
        <button onclick={() => onCopy(askAnswer)} class="text-[10px] text-dim hover:text-text font-mono">copy</button>
      {:else if mode === 'digest' && digestAnswer.length > 0}
        <button onclick={onRegenerateDigest} class="text-[10px] text-text hover:underline font-mono">regenerate</button>
        <button onclick={() => onCopy(digestAnswer)} class="text-[10px] text-dim hover:text-text font-mono">copy</button>
        <button onclick={onSaveDigestAsNote} class="text-[10px] text-text hover:underline font-mono">save as note</button>
      {/if}
    {/if}
    <button onclick={onDismiss} class="text-[10px] text-dim hover:text-text font-mono">dismiss</button>
  </div>

  {#if error}
    <p class="text-[11px] text-error">{error}</p>
  {/if}

  {#if mode === 'themes'}
    {#if themes.length > 0}
      <div class="flex flex-wrap gap-1.5">
        {#each themes as t (t.label)}
          <button
            type="button"
            onclick={() => onApplyTheme(t)}
            class="text-[11px] px-2 py-0.5 rounded-full bg-mantle border border-surface1 hover:border-primary text-text"
            title={`search: ${t.query}`}
          >{t.label}</button>
        {/each}
      </div>
    {/if}
  {:else if mode === 'ask'}
    <input
      bind:this={askInputEl}
      bind:value={askQuestion}
      onkeydown={(e) => {
        if (e.key === 'Enter') { e.preventDefault(); onSubmitAsk(); }
      }}
      placeholder="e.g. what was I worried about last month?"
      disabled={busy}
      class="w-full bg-mantle border border-surface1 rounded px-2 py-1 text-[12px] text-text placeholder-dim focus:outline-none focus:border-primary mb-1.5 disabled:opacity-50"
    />
    {#if askAnswer.trim()}
      <div class="bg-mantle border border-surface1 rounded p-2 max-h-[24rem] overflow-y-auto">
        <MarkdownRenderer body={askAnswer} onWikilink={onWikilink} />
      </div>
    {/if}
  {:else if mode === 'digest'}
    {#if digestAnswer.trim()}
      <div class="bg-mantle border border-surface1 rounded p-2 max-h-[28rem] overflow-y-auto">
        <MarkdownRenderer body={digestAnswer} onWikilink={onWikilink} />
      </div>
    {/if}
  {/if}
</div>
