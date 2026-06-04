<script lang="ts">
  /**
   * StatusBar — the persistent thin bar at the bottom of every page.
   *
   * Originally just an "open note" tray; promoted to a general status
   * bar that hosts:
   *   • Left:   workspace switcher pills (shared workspaceStore)
   *   • Middle: open-note chips (last-opened + pinned) — original use
   *   • Right:  connectivity dot + AI-ready dot
   *
   * Layout
   *   • Desktop (≥ md): 28px-tall bar pinned to the viewport bottom.
   *   • Mobile (< md):  same data, anchored above the BottomNav so it
   *     doesn't steal touch targets. iOS safe-area inherited.
   *
   * Visibility
   *   • Always rendered when the user is authed (left + right groups
   *     are always relevant). The middle group self-hides per-chip
   *     when there's nothing to show.
   *
   * Z-index
   *   • z-20 — above editor chrome, below mobile BottomNav (z-30),
   *     AI overlay (z-50), modal scrims (z-40).
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
  import { workspaceStoreSingleton } from '$lib/workspace/workspaceStore.svelte';
  import { isOnline } from '$lib/stores/online';
  import { aiStatus } from '$lib/stores/ai-status';
  import { sabbath } from '$lib/stores/sabbath';
  import { goto } from '$app/navigation';

  const wsStore = workspaceStoreSingleton();

  // Decode the active note path so we don't surface a "jump back" chip
  // for the note the user is already looking at.
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

  let lastChip = $derived.by(() => {
    if (!$lastOpenNote) return null;
    if (activeNotePath === $lastOpenNote.path) return null;
    return $lastOpenNote;
  });

  let visiblePins = $derived.by(() =>
    $pinnedTrayNotes.filter((e) => e.path !== activeNotePath)
  );

  let onWorkspacePage = $derived($page.url.pathname.startsWith('/workspace'));

  // Overflow menu for note chips — single state shared between chips.
  let menuFor = $state<string | null>(null);
  function toggleMenu(path: string) {
    menuFor = menuFor === path ? null : path;
  }
  function closeMenu() {
    menuFor = null;
  }

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

  function switchWorkspace(id: string) {
    wsStore.activeId = id;
    // Jump to /workspace so the switch is visible immediately. If the
    // user is already there, no nav happens.
    if (!onWorkspacePage) void goto('/workspace');
  }

  // Reserve the bar's height so the editor's bottom padding stacks it
  // on top of any other reserves (BottomNav, iOS safe area). h-7 =
  // 28px — kept in sync with the class on the root element below.
  const TRAY_HEIGHT = '1.75rem';
  $effect(() => {
    if (typeof document === 'undefined') return;
    if ($trayEnabled) {
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
  <div
    class="group relative inline-flex items-center min-w-0 h-full
           border-l border-surface1 pl-2 pr-1"
    data-tray-menu
  >
    <a
      href={hrefFor(entry)}
      title={`${opts.kind === 'last' ? 'Last opened' : 'Pinned'} · ${entry.path}`}
      class="inline-flex items-center gap-1.5 min-w-0 max-w-[12rem] md:max-w-[18rem]
             text-[11px] md:text-xs text-subtext hover:text-text transition-colors"
    >
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

{#if $trayEnabled}
  <div
    role="region"
    aria-label="Status bar"
    class="note-tray note-tray-hide-on-kb fixed inset-x-0 z-20 bg-mantle border-t border-surface1
           h-7 md:h-7 flex items-stretch overflow-hidden"
    style="bottom: var(--note-tray-bottom, 0px);"
  >
    <!-- LEFT: Workspace switcher pills. The shared workspaceStore
         singleton means a switch here is reflected in the /workspace
         shell immediately, and vice-versa. We render at most 4 pills
         inline; if the user has more, the rest are reachable via the
         /workspace tray. -->
    <div class="flex items-stretch overflow-x-auto flex-shrink-0 max-w-[60%] md:max-w-[40%]">
      <a
        href="/workspace"
        title="Open workspaces"
        aria-label="Open workspaces"
        class="inline-flex items-center justify-center w-7 h-full text-dim hover:text-primary hover:bg-surface0 transition-colors flex-shrink-0"
        aria-current={onWorkspacePage ? 'page' : undefined}
      >
        <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <rect x="3" y="3" width="8" height="8" rx="1"/>
          <rect x="13" y="3" width="8" height="8" rx="1"/>
          <rect x="3" y="13" width="8" height="8" rx="1"/>
          <rect x="13" y="13" width="8" height="8" rx="1"/>
        </svg>
      </a>
      {#each wsStore.workspaces.slice(0, 4) as w (w.id)}
        {@const active = w.id === wsStore.activeId && onWorkspacePage}
        <button
          type="button"
          onclick={() => switchWorkspace(w.id)}
          title={`Switch to workspace "${w.name}"`}
          class="inline-flex items-center px-2 h-full text-[11px] md:text-xs font-medium border-l border-surface1 transition-colors whitespace-nowrap
            {active ? 'text-primary bg-surface0' : 'text-subtext hover:text-text hover:bg-surface0'}"
        >
          <span class="truncate max-w-[8rem]">{w.name}</span>
        </button>
      {/each}
    </div>

    <!-- MIDDLE: open-note chips. Grows to fill, scrolls horizontally
         if many chips. Self-empty when nothing is open. -->
    <div class="flex items-stretch overflow-x-auto flex-1 min-w-0">
      {#if lastChip}
        {@render chip(lastChip, { kind: 'last' })}
      {/if}
      {#each visiblePins as p (p.path)}
        {@render chip(p, { kind: 'pin' })}
      {/each}
    </div>

    <!-- RIGHT: indicators. Connectivity dot + AI ready dot. Tiny,
         high-contrast, hover-tooltips for detail. -->
    <div class="flex items-center gap-2 px-2 border-l border-surface1 flex-shrink-0">
      <span
        class="inline-flex items-center gap-1 text-[10px] text-dim"
        title={$isOnline ? 'Connected' : 'Offline — changes will sync when back online'}
      >
        <span
          class="w-1.5 h-1.5 rounded-full {$isOnline ? 'bg-success' : 'bg-error'}"
          aria-hidden="true"
        ></span>
        <span class="hidden md:inline">{$isOnline ? 'online' : 'offline'}</span>
      </span>
      {#if $aiStatus}
        <span
          class="inline-flex items-center gap-1 text-[10px] text-dim"
          title={$sabbath
            ? 'AI paused — Sabbath'
            : `AI ready — ${$aiStatus.global_model || $aiStatus.global_provider || 'default'}`}
        >
          <span
            class="w-1.5 h-1.5 rounded-full {$sabbath ? 'bg-warning' : 'bg-success'}"
            aria-hidden="true"
          ></span>
          <span class="hidden md:inline truncate max-w-[8rem]">
            {$sabbath ? 'sabbath' : ($aiStatus.global_model || 'ai')}
          </span>
        </span>
      {/if}
    </div>
  </div>
{/if}

<style>
  .note-tray {
    --note-tray-bottom: calc(3.5rem + env(safe-area-inset-bottom, 0px));
  }
  @media (min-width: 768px) {
    .note-tray {
      --note-tray-bottom: 0px;
    }
  }
  :global(html[data-kb-open]) .note-tray-hide-on-kb {
    transform: translateY(150%);
    pointer-events: none;
  }
  .note-tray-hide-on-kb {
    transition: transform 180ms ease-out;
  }
</style>
