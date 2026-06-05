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
  import WorkspacePills from '$lib/workspace/WorkspacePills.svelte';
  import NoteChip from './NoteChip.svelte';
  import StatusIndicators from './StatusIndicators.svelte';
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

  function onPillSwitch(_id: string) {
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

{#if $trayEnabled}
  <div
    role="region"
    aria-label="Status bar"
    class="note-tray note-tray-hide-on-kb fixed inset-x-0 z-20 bg-mantle border-t border-surface1
           h-7 md:h-7 flex items-stretch overflow-hidden"
    style="bottom: var(--note-tray-bottom, 0px);"
  >
    <!-- LEFT: Workspace switcher. WorkspacePills is the single
         switcher used across the app — full create/rename/delete
         live here, so /workspace no longer needs its own tray.
         Active pill only highlights on the /workspace route so the
         StatusBar reads as "go to" elsewhere and as "is at" there. -->
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
      <WorkspacePills store={wsStore} highlightActive={onWorkspacePage} onSwitch={onPillSwitch} />
    </div>

    <!-- MIDDLE: open-note chips. Grows to fill, scrolls horizontally
         if many chips. Self-empty when nothing is open. -->
    <div class="flex items-stretch overflow-x-auto flex-1 min-w-0">
      {#if lastChip}
        <NoteChip
          entry={lastChip}
          kind="last"
          pinned={$pinnedTrayNotes.some((e) => e.path === lastChip.path)}
          href={hrefFor(lastChip)}
          menuOpen={menuFor === lastChip.path}
          onToggleMenu={() => toggleMenu(lastChip.path)}
          onPinToggle={() => onPinToggle(lastChip)}
          onClear={onClear}
          onOpenNewTab={() => onOpenNewTab(lastChip)}
        />
      {/if}
      {#each visiblePins as p (p.path)}
        <NoteChip
          entry={p}
          kind="pin"
          pinned={true}
          href={hrefFor(p)}
          menuOpen={menuFor === p.path}
          onToggleMenu={() => toggleMenu(p.path)}
          onPinToggle={() => onPinToggle(p)}
          onClear={onClear}
          onOpenNewTab={() => onOpenNewTab(p)}
        />
      {/each}
    </div>

    <!-- RIGHT: connectivity + AI indicators. Self-contained — reads
         its own stores. -->
    <StatusIndicators />
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
