<script lang="ts">
  import { onMount } from 'svelte';

  // SelectionToolbar — Notion / Medium-style floating toolbar that
  // appears above the user's text selection in the CodeMirror editor.
  //
  // The toolbar fires command events back to the host page; the host
  // dispatches them via the editor's already-bound keymap shortcuts.
  // This keeps the toolbar a pure UI surface and means new keymap
  // chords automatically work as buttons too.
  //
  // Positioning: we listen for selectionchange on the document AND
  // selection updates on the editor's DOM, find the bounding rect
  // of the current text selection, and pin the toolbar above it. On
  // empty selections the toolbar hides.

  interface Action {
    label: string;
    /** Keyboard chord to dispatch when the user clicks. */
    chord: string;
    /** Plain-language hint shown in the tooltip. */
    title: string;
  }

  // The action set mirrors the editor's most-used keybindings so the
  // toolbar UI surfaces what's already there — discoverable at the
  // same time as the cheat sheet (?), but for the user who just
  // selected text and wants to do something to it.
  const ACTIONS: Action[] = [
    { label: 'B', chord: 'mod+b', title: 'Bold' },
    { label: 'I', chord: 'mod+i', title: 'Italic' },
    { label: '◐', chord: 'mod+shift+h', title: 'Highlight (==text==)' },
    { label: '<>', chord: 'mod+`', title: 'Inline code' },
    { label: '🔗', chord: 'mod+k', title: 'Link' },
    { label: 'H1', chord: 'mod+alt+1', title: 'Heading 1' },
    { label: 'H2', chord: 'mod+alt+2', title: 'Heading 2' },
    { label: 'H3', chord: 'mod+alt+3', title: 'Heading 3' },
    { label: '•', chord: 'mod+shift+8', title: 'Bullet list' },
    { label: '"', chord: 'mod+shift+9', title: 'Blockquote' },
    { label: '↗', chord: 'mod+shift+x', title: 'Extract to new note' },
    { label: '✨', chord: 'mod+shift+a', title: 'Ask AI about selection' },
    // Section AI — same Ask-AI dialog but operates on the heading-
    // bounded section the cursor is in. Surfaced here too so the
    // shortcut is discoverable without memorising the keymap; the
    // chord doesn't require a selection but having it next to the
    // selection-AI button reads as 'wider scope, same dialog'.
    { label: '§✨', chord: 'mod+shift+/', title: 'Ask AI about the current section' }
  ];

  interface Props {
    container: HTMLElement | undefined;
    /**
     * Dispatch a chord into the editor. The host page hands this
     * down — typically a thin wrapper that synthesizes a KeyboardEvent
     * and dispatches it on the editor view's DOM so the existing
     * keymap handles it. Single source of truth (the keymap) keeps
     * toolbar buttons and keyboard shortcuts behaviourally identical.
     */
    onCommand: (chord: string) => void;
  }

  let { container, onCommand }: Props = $props();

  let visible = $state(false);
  let top = $state(0);
  let left = $state(0);
  // Toolbar's measured width — used to centre it over the selection.
  // Re-measured every time it shows so font / DPI changes don't
  // leave the toolbar drifting off-centre.
  let toolbarEl: HTMLDivElement | undefined = $state();
  let toolbarWidth = $state(0);

  function update() {
    if (!container) {
      visible = false;
      return;
    }
    const sel = window.getSelection();
    if (!sel || sel.isCollapsed || sel.rangeCount === 0) {
      visible = false;
      return;
    }
    // Make sure the selection is INSIDE the editor; otherwise some
    // other input on the page would trigger the toolbar.
    const range = sel.getRangeAt(0);
    if (!container.contains(range.commonAncestorContainer)) {
      visible = false;
      return;
    }
    const rect = range.getBoundingClientRect();
    if (rect.width === 0 && rect.height === 0) {
      visible = false;
      return;
    }
    // Position above the selection, centre-aligned. 8px gap so the
    // toolbar's bottom edge doesn't touch the highlight.
    const tbWidth = toolbarWidth || 360;
    top = rect.top + window.scrollY - 44;
    left = Math.max(
      8,
      Math.min(window.innerWidth - tbWidth - 8, rect.left + window.scrollX + rect.width / 2 - tbWidth / 2)
    );
    visible = true;
  }

  onMount(() => {
    const onSelChange = () => requestAnimationFrame(update);
    document.addEventListener('selectionchange', onSelChange);
    window.addEventListener('scroll', onSelChange, true);
    window.addEventListener('resize', onSelChange);
    return () => {
      document.removeEventListener('selectionchange', onSelChange);
      window.removeEventListener('scroll', onSelChange, true);
      window.removeEventListener('resize', onSelChange);
    };
  });

  // Re-measure the toolbar after it mounts so the centring math has
  // a real width to work with on the second update.
  $effect(() => {
    if (visible && toolbarEl) {
      toolbarWidth = toolbarEl.offsetWidth;
    }
  });

  function fire(chord: string) {
    onCommand(chord);
    // Keep the toolbar visible after a click so the user can chain
    // (e.g., bold + italic). The selectionchange event will hide it
    // once they click outside.
  }
</script>

{#if visible}
  <div
    bind:this={toolbarEl}
    class="sel-toolbar"
    style="top: {top}px; left: {left}px;"
    role="toolbar"
    aria-label="Selection actions"
    onmousedown={(e) => e.preventDefault()}
  >
    {#each ACTIONS as a}
      <button
        type="button"
        title={a.title}
        onclick={() => fire(a.chord)}
        class="sel-btn"
      >{a.label}</button>
    {/each}
  </div>
{/if}

<style>
  .sel-toolbar {
    position: absolute;
    z-index: 40;
    display: inline-flex;
    align-items: stretch;
    gap: 0.125rem;
    padding: 0.25rem;
    background: var(--color-mantle);
    border: 1px solid var(--color-surface1);
    border-radius: 0.375rem;
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.25);
    user-select: none;
  }
  .sel-btn {
    min-width: 2rem;
    padding: 0.25rem 0.5rem;
    background: transparent;
    color: var(--color-subtext);
    border: none;
    border-radius: 0.25rem;
    font-size: 0.8125rem;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    cursor: pointer;
  }
  .sel-btn:hover {
    background: var(--color-surface0);
    color: var(--color-text);
  }
  /* Hide on print + on mobile (where the OS context menu already
     covers selection actions and the floating toolbar fights for
     the same screen real estate). */
  @media print {
    .sel-toolbar { display: none; }
  }
  @media (max-width: 640px) {
    .sel-toolbar { display: none; }
  }
</style>
