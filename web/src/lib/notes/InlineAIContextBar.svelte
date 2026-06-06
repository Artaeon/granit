<!--
  InlineAIContextBar — bottom toggle row of the InlineAIMenu.

  Three concerns:
    1. note vs. section scope — exclusive. The "§ <heading>" button
       only appears when the cursor actually lives inside a heading
       section (parent passes `detectedSection` or null).
    2. additive cross-context toggles — "+ linked notes" and
       "+ 7d jots", layered on top of either scope.
    3. send-to-chat handoff — the escape hatch into the Cmd+J overlay.
       The parent handles the seed-and-open dance via `onSendToChat`.

  Right-aligned hint hides on touch where there are no chords to
  read; the parent reports whether history.length > 0 so the label
  reads "history/pick" vs. just "pick".

  Style follows the rest of the codebase: state stays in the parent
  controller, mutations come back through `setX` callbacks rather
  than via `$bindable` — keeps the controller's getter/setter
  surface as the single source of truth.
-->
<script lang="ts">
  import type { Scope, DetectedSection } from './inlineAIContextScope.svelte';

  interface Props {
    scope: Scope;
    useLinkedNotes: boolean;
    useRecentJots: boolean;
    /** null when the cursor is in pre-heading text. The "§" button
     *  is hidden in that case. */
    detectedSection: DetectedSection | null;
    /** Disable interactive controls while an AI stream is in flight. */
    busy: boolean;
    /** Whether the menu has any history items — used to swap the
     *  keyboard hint label between "history/pick" and "pick". */
    hasHistory: boolean;
    setScope: (s: Scope) => void;
    setUseLinkedNotes: (on: boolean) => void;
    setUseRecentJots: (on: boolean) => void;
    /** Invoked by the chat hand-off button. The parent seeds the
     *  Cmd+J overlay with the note path / selection / typed prompt
     *  and closes the menu. */
    onSendToChat: () => void;
  }
  let {
    scope,
    useLinkedNotes,
    useRecentJots,
    detectedSection,
    busy,
    hasHistory,
    setScope,
    setUseLinkedNotes,
    setUseRecentJots,
    onSendToChat
  }: Props = $props();
</script>

<!-- Context bar — wraps on narrow screens so the toggles don't
     overflow the menu width; keyboard hint hides on touch since
     there are no chords to read. -->
<div class="flex items-center flex-wrap gap-x-1.5 gap-y-1 px-2 py-1.5 border-t border-surface1 text-[10px] font-mono">
  <span class="text-dim">scope:</span>
  <!-- Note vs. section — exclusive toggle. The note button is
       always available; the section button only when the cursor
       actually lives inside a heading section. -->
  <button
    type="button"
    onclick={() => setScope('note')}
    class="px-1 py-0.5 rounded {scope === 'note' ? 'bg-primary text-on-primary' : 'bg-surface0 text-dim hover:bg-surface1 hover:text-text'}"
    title="send the entire note body to AI"
  >note</button>
  {#if detectedSection}
    <button
      type="button"
      onclick={() => setScope('section')}
      class="px-1 py-0.5 rounded {scope === 'section' ? 'bg-primary text-on-primary' : 'bg-surface0 text-dim hover:bg-surface1 hover:text-text'}"
      title="send only the current section: {detectedSection.heading}"
    >§ {detectedSection.heading.length > 14 ? detectedSection.heading.slice(0, 14) + '…' : detectedSection.heading}</button>
  {/if}
  <span class="text-dim opacity-40 mx-0.5">|</span>
  <button
    type="button"
    onclick={() => setUseLinkedNotes(!useLinkedNotes)}
    class="px-1 py-0.5 rounded {useLinkedNotes ? 'bg-primary text-on-primary' : 'bg-surface0 text-dim hover:bg-surface1 hover:text-text'}"
    title="include short body snippets from up to 6 linked notes (both backlinks and outgoing wikilinks) — the AI then reasons over actual content, not just titles"
  >+ linked notes</button>
  <button
    type="button"
    onclick={() => setUseRecentJots(!useRecentJots)}
    class="px-1 py-0.5 rounded {useRecentJots ? 'bg-primary text-on-primary' : 'bg-surface0 text-dim hover:bg-surface1 hover:text-text'}"
    title="include the last 7 days of daily notes"
  >+ 7d jots</button>
  <!-- Hand-off to the global chat sidebar. Seeded with the note
       path + selection + the prompt the user was typing; nothing
       gets inserted into the doc by this path. -->
  <button
    type="button"
    onclick={onSendToChat}
    disabled={busy}
    class="px-1 py-0.5 rounded bg-surface0 text-dim hover:bg-surface1 hover:text-text"
    title="open the Cmd+J chat sidebar pre-filled with this note + your prompt"
  >↗ chat</button>
  <span class="ml-auto text-dim opacity-60 hidden sm:inline">
    ↑↓ {hasHistory ? 'history/pick' : 'pick'} · ⏎ run · Esc
  </span>
</div>
