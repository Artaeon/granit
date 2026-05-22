<script lang="ts">
  /**
   * Slim "open note" tray that lives at the bottom of every page so
   * the user can jump back to their last-opened note from anywhere.
   *
   * Layout
   *   • Desktop (≥ md): 28px-tall bar pinned to the viewport bottom
   *     with the note title + a small overflow menu + dismiss.
   *   • Mobile (< md):  same data, but anchored *above* the existing
   *     BottomNav (5-column thumb bar) so it doesn't steal touch
   *     targets. iOS safe-area padding is inherited from BottomNav's
   *     own `pb-safe` so the tray sits flush against its top border.
   *
   * Visibility rules
   *   • Hidden when `trayEnabled` is false (settings opt-out).
   *   • Hidden when there's no `lastOpenNote` AND no pinned entries.
   *   • The lastOpenNote chip is suppressed when the user is already
   *     looking at that note's page — re-showing it would be visual
   *     noise (the editor itself is the affordance).
   *   • Pinned chips render unconditionally (minus whichever is
   *     currently active) so the user can hop between two reference
   *     notes without the tray flickering in/out.
   *
   * Z-index
   *   • z-20 — sits above the editor's chrome (no fixed elements
   *     there) but below the mobile BottomNav (z-30), the AI
   *     overlay (z-50), and modal scrims (z-40). Verified against
   *     BottomNav.svelte and AIOverlay.svelte at component-write
   *     time.
   */

  import { page } from '$app/stores';
  import {
    lastOpenNote,
    pinnedTrayNotes,
    trayEnabled,
    clearOpenNote,
    pinOpenNote,
    unpinOpenNote,
    isTrayPinned,
    type OpenNoteEntry
  } from '$lib/stores/open-note';

  // Decode the active note path from the URL so we can suppress the
  // chip when the user is already on that note. Notes route is
  // /notes/<urlencoded-path>; we strip the prefix + decodeURIComponent
  // the rest. Returns '' when not on a note page.
  let activeNotePath = $derived.by(() => {
    const p = $page.url.pathname;
    if (!p.startsWith('/notes/')) return '';
    const rest = p.slice('/notes/'.length);
    try {
      return decodeURIComponent(rest);
    } catch {
      return rest;
    }
  });

  // The chip the tray surfaces for "jump back". Hidden when the user
  // is on that exact note OR the store is empty.
  let lastChip = $derived.by(() => {
    if (!$lastOpenNote) return null;
    if (activeNotePath === $lastOpenNote.path) return null;
    return $lastOpenNote;
  });

  // Pinned entries other than the one the user is currently viewing.
  // Suppression keeps the strip's visual contract: every chip is a
  // "go somewhere else" affordance.
  let visiblePins = $derived.by(() => {
    return $pinnedTrayNotes.filter((e) => e.path !== activeNotePath);
  });

  // The overall tray renders only when there's at least one chip to
  // show AND the user hasn't opted out in settings.
  let visible = $derived($trayEnabled && (!!lastChip || visiblePins.length > 0));

  // Overflow menu state — a single dropdown shared between the last-
  // open chip and any pinned chip the user opens it on. Tracked by
  // the entry's path so multiple chips don't fight over one open
  // state. `null` = closed.
  let menuFor = $state<string | null>(null);
  function toggleMenu(path: string) {
    menuFor = menuFor === path ? null : path;
  }
  function closeMenu() {
    menuFor = null;
  }

  // Close the menu on outside click / Esc so it doesn't get
  // accidentally left open after the user navigated away.
  $effect(() => {
    if (menuFor === null) return;
    const onDown = (e: MouseEvent) => {
      const t = e.target as HTMLElement | null;
      if (t?.closest('[data-tray-menu]')) return;
      closeMenu();
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') closeMenu();
    };
    window.addEventListener('mousedown', onDown);
    window.addEventListener('keydown', onKey);
    return () => {
      window.removeEventListener('mousedown', onDown);
      window.removeEventListener('keydown', onKey);
    };
  });

  function hrefFor(entry: OpenNoteEntry): string {
    return '/notes/' + encodeURIComponent(entry.path);
  }

  function onPinToggle(entry: OpenNoteEntry) {
    if (isTrayPinned(entry.path)) unpinOpenNote(entry.path);
    else pinOpenNote(entry);
    closeMenu();
  }
  function onClear() {
    clearOpenNote();
    closeMenu();
  }
  function onOpenNewTab(entry: OpenNoteEntry) {
    if (typeof window === 'undefined') return;
    window.open(hrefFor(entry), '_blank', 'noopener');
    closeMenu();
  }

  // Expose the tray's reserved height as a CSS custom property on
  // <html> so the main content's bottom padding can stack it on top
  // of any other reserves (mobile bottom-nav, iOS safe area). The
  // `.main-with-tray` rule in app.css consumes this var. Cleared
  // when the tray hides so the editor reclaims those pixels.
  // h-7 = 1.75rem (28px) — kept in sync with the class on the tray
  // root element below.
  const TRAY_HEIGHT = '1.75rem';
  $effect(() => {
    if (typeof document === 'undefined') return;
    if (visible) {
      document.documentElement.style.setProperty('--note-tray-h', TRAY_HEIGHT);
      return () => {
        document.documentElement.style.removeProperty('--note-tray-h');
      };
    }
    document.documentElement.style.removeProperty('--note-tray-h');
  });
</script>

{#snippet chip(entry: OpenNoteEntry, opts: { kind: 'last' | 'pin' })}
  {@const pinned = $pinnedTrayNotes.some((e) => e.path === entry.path)}
  {@const menuOpen = menuFor === entry.path}
  <!-- Single chip: clickable body (navigates) + overflow trigger + ×
       dismiss (last-open only — pins are removed via the menu so
       the chip body stays a one-click "jump back"). -->
  <div
    class="group relative inline-flex items-center min-w-0 h-full
           border-l border-surface1 first:border-l-0 pl-2 pr-1"
    data-tray-menu
  >
    <a
      href={hrefFor(entry)}
      title={`${opts.kind === 'last' ? 'Last opened' : 'Pinned'} · ${entry.path}`}
      class="inline-flex items-center gap-1.5 min-w-0 max-w-[12rem] md:max-w-[18rem]
             text-[11px] md:text-xs text-subtext hover:text-text transition-colors"
    >
      <!-- Tiny doc / pin glyph so the user reads the chip-kind at a
           glance without expanding the overflow. Last-open is a
           doc icon; pinned is a filled star to match the sidebar
           pin metaphor elsewhere in the app. -->
      {#if opts.kind === 'pin'}
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

    <!-- Overflow trigger -->
    <button
      type="button"
      onclick={() => toggleMenu(entry.path)}
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

    <!-- Last-open quick dismiss. Pins don't get this — the overflow
         menu's "Unpin" is the explicit, less-accidental remove path
         for pinned entries. -->
    {#if opts.kind === 'last'}
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
      <!-- Overflow menu. Opens upward (bottom-full) since the tray
           itself is anchored to the viewport bottom — a downward
           menu would clip off-screen. mb-1 leaves a small gap so
           the menu visibly detaches from the chip. -->
      <div
        role="menu"
        class="absolute right-0 bottom-full mb-1 w-44 bg-mantle border border-surface1 rounded-lg shadow-xl py-1 text-sm z-10"
      >
        <button
          type="button"
          role="menuitem"
          onclick={() => onPinToggle(entry)}
          class="w-full text-left px-3 py-1.5 hover:bg-surface0 text-text"
        >{pinned ? 'Unpin from tray' : 'Pin to tray'}</button>
        <button
          type="button"
          role="menuitem"
          onclick={() => onOpenNewTab(entry)}
          class="w-full text-left px-3 py-1.5 hover:bg-surface0 text-text"
        >Open in new tab</button>
        {#if opts.kind === 'last'}
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
{/snippet}

{#if visible}
  <!--
    Mobile: the BottomNav (z-30) sits flush at the viewport bottom
    with `pb-safe`. Stacking the tray directly above means the user
    sees [tray strip][bottomnav][safe-area] — touch targets are
    preserved and the OS chrome doesn't clip either bar.

    Desktop: nothing else lives at the bottom, so we sit at bottom:0
    edge-to-edge. The 28px height (h-7) matches the user's "ganz
    schmal" brief — readable but visually unobtrusive.
  -->
  <!-- The tray sits above the bottom-nav on mobile. When the on-
       screen keyboard opens the bottom-nav hides (data-kb-open on
       <html>); the tray follows so a typing user gets the whole
       editor height. Re-appears together with the nav. -->
  <div
    role="region"
    aria-label="Open note tray"
    class="note-tray note-tray-hide-on-kb fixed inset-x-0 z-20 bg-mantle border-t border-surface1
           h-7 md:h-7 flex items-stretch overflow-x-auto"
    style="bottom: var(--note-tray-bottom, 0px);"
  >
    {#if lastChip}
      {@render chip(lastChip, { kind: 'last' })}
    {/if}
    {#each visiblePins as p (p.path)}
      {@render chip(p, { kind: 'pin' })}
    {/each}
  </div>
{/if}

<style>
  /* On mobile the BottomNav owns the bottom 3.5rem + safe-area.
     Lift the tray to sit on top of it without inline-style math at
     the call site. Desktop falls back to 0 (the @media reset). */
  .note-tray {
    --note-tray-bottom: calc(3.5rem + env(safe-area-inset-bottom, 0px));
  }
  @media (min-width: 768px) {
    .note-tray {
      --note-tray-bottom: 0px;
    }
  }
  /* Hide the scrollbar — tray is a single-row horizontal strip; a
     visible scrollbar would steal vertical pixels from a 28px bar. */
  .note-tray::-webkit-scrollbar { display: none; }
  .note-tray { scrollbar-width: none; }
  /* When the on-screen keyboard opens (data-kb-open set by the
     layout's visualViewport listener), the tray slides off-screen
     together with the bottom-nav so a typing user gets the whole
     editor height. */
  :global(html[data-kb-open]) .note-tray-hide-on-kb {
    transform: translateY(150%);
    pointer-events: none;
  }
  .note-tray-hide-on-kb {
    transition: transform 180ms ease-out;
  }
</style>
