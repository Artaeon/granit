<!--
  NoteChip — the per-note chip rendered inside the StatusBar's
  middle section. Kind 'last' shows the file icon + a × dismiss
  button; kind 'pin' shows a pin star and skips the dismiss. Both
  expose a "⋯" menu for pin/unpin + open-in-new-tab + clear.

  Pure presentational — all state (menu open / pinned set / clear)
  lives in the StatusBar; this component just renders one chip.
-->
<script lang="ts">
  import type { OpenNoteEntry } from '$lib/stores/open-note';

  type Props = {
    entry: OpenNoteEntry;
    kind: 'last' | 'pin';
    pinned: boolean;
    href: string;
    menuOpen: boolean;
    onToggleMenu: () => void;
    onPinToggle: () => void;
    onClear: () => void;
    onOpenNewTab: () => void;
  };

  let {
    entry,
    kind,
    pinned,
    href,
    menuOpen,
    onToggleMenu,
    onPinToggle,
    onClear,
    onOpenNewTab
  }: Props = $props();
</script>

<div
  class="group relative inline-flex items-center min-w-0 h-full border-l border-surface1 pl-2 pr-1"
  data-tray-menu
>
  <a
    {href}
    title={`${kind === 'last' ? 'Last opened' : 'Pinned'} · ${entry.path}`}
    class="inline-flex items-center gap-1.5 min-w-0 max-w-[12rem] md:max-w-[18rem] text-[11px] md:text-xs text-subtext hover:text-text transition-colors"
  >
    {#if kind === 'pin'}
      <svg viewBox="0 0 16 16" class="w-3 h-3 flex-shrink-0 text-warning" fill="currentColor" aria-hidden="true">
        <path d="M8 1.5l1.85 4.05L14 6.2l-3.1 2.85L11.7 13 8 10.85 4.3 13l.8-3.95L2 6.2l4.15-.65z"/>
      </svg>
    {:else}
      <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <path d="M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"/>
        <polyline points="14 3 14 9 20 9"/>
      </svg>
    {/if}
    <span class="truncate">{entry.title || entry.path}</span>
  </a>

  <button
    type="button"
    onclick={onToggleMenu}
    aria-haspopup="menu"
    aria-expanded={menuOpen}
    aria-label="Tray actions"
    title="More"
    class="ml-1 w-5 h-5 inline-flex items-center justify-center rounded text-dim hover:text-text hover:bg-surface1 transition-colors"
  >
    <svg viewBox="0 0 24 24" class="w-3 h-3" fill="currentColor" aria-hidden="true">
      <circle cx="5" cy="12" r="1.5"/>
      <circle cx="12" cy="12" r="1.5"/>
      <circle cx="19" cy="12" r="1.5"/>
    </svg>
  </button>

  {#if kind === 'last'}
    <button
      type="button"
      onclick={onClear}
      aria-label="Dismiss from tray"
      title="Dismiss"
      class="ml-0.5 w-5 h-5 inline-flex items-center justify-center rounded text-dim hover:text-error hover:bg-surface1 transition-colors"
    >
      <svg viewBox="0 0 24 24" class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
        <path d="M6 6l12 12M6 18L18 6"/>
      </svg>
    </button>
  {/if}

  {#if menuOpen}
    <div
      role="menu"
      class="absolute right-0 bottom-full mb-1 w-44 bg-mantle border border-surface1 rounded-lg shadow-xl py-1 text-sm z-10"
    >
      <button
        type="button"
        role="menuitem"
        onclick={onPinToggle}
        class="w-full text-left px-3 py-1.5 hover:bg-surface0 text-text"
      >{pinned ? 'Unpin from tray' : 'Pin to tray'}</button>
      <button
        type="button"
        role="menuitem"
        onclick={onOpenNewTab}
        class="w-full text-left px-3 py-1.5 hover:bg-surface0 text-text"
      >Open in new tab</button>
      {#if kind === 'last'}
        <button
          type="button"
          role="menuitem"
          onclick={onClear}
          class="w-full text-left px-3 py-1.5 hover:bg-surface0 text-error"
        >Clear from tray</button>
      {/if}
    </div>
  {/if}
</div>
