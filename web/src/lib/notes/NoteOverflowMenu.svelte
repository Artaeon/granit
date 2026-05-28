<script lang="ts">
  // Note editor overflow menu (⋯) — single home for secondary
  // actions (find, history, PDF, slideshow, audio, reading mode,
  // focus mode, flashcards, keyboard shortcuts). Owns its own
  // open / positioning state + bindable trigger ref so callers can
  // toggle it without managing repositioning logic.
  //
  // Extracted from routes/notes/[...path]/+page on 2026-05-28.
  // Behaviour is byte-identical to the inlined version: same
  // viewport-aware fixed-coordinate positioning pattern as
  // EditorAIMenu, same menu-item markup, same disabled flags.
  //
  // Inputs that change menu state come in as bindable: open
  // (toggled by both the trigger button outside this component and
  // an internal Escape / click-outside handler) and triggerEl (the
  // button the menu anchors to). Behaviour changes (open find,
  // toggle audio, …) come in as callbacks so the parent stays the
  // single source of truth for editor / overlay state.

  interface Props {
    open: boolean;
    triggerEl: HTMLButtonElement | undefined;
    audioOpen: boolean;
    readingMode: boolean;
    focusMode: boolean;
    schedulingFlashcards: boolean;
    onOpenFind: () => void;
    onOpenHistory: () => void;
    onOpenPrint: () => void;
    onOpenPresentation: () => void;
    onToggleAudio: () => void;
    onToggleReadingMode: () => void;
    onToggleFocusMode: () => void;
    onScheduleFlashcards: () => void;
    onOpenHelp: () => void;
  }

  let {
    open = $bindable(),
    triggerEl,
    audioOpen,
    readingMode,
    focusMode,
    schedulingFlashcards,
    onOpenFind,
    onOpenHistory,
    onOpenPrint,
    onOpenPresentation,
    onToggleAudio,
    onToggleReadingMode,
    onToggleFocusMode,
    onScheduleFlashcards,
    onOpenHelp
  }: Props = $props();

  let menuEl: HTMLDivElement | undefined = $state();
  let menuTop = $state(0);
  let menuLeft = $state(0);
  let menuWidth = $state(240);

  function reposition() {
    if (!triggerEl) return;
    const rect = triggerEl.getBoundingClientRect();
    const vw = window.innerWidth;
    const margin = 8;
    menuWidth = Math.min(240, vw - margin * 2);
    let left = rect.right - menuWidth;
    if (left < margin) left = margin;
    if (left + menuWidth > vw - margin) left = vw - margin - menuWidth;
    menuLeft = left;
    menuTop = rect.bottom + 4;
  }

  $effect(() => {
    if (!open) return;
    reposition();
    function onDocClick(e: MouseEvent) {
      if (!menuEl || !triggerEl) return;
      if (e.target instanceof Node && menuEl.contains(e.target)) return;
      if (e.target instanceof Node && triggerEl.contains(e.target)) return;
      open = false;
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') open = false;
    }
    function onResize() { reposition(); }
    document.addEventListener('mousedown', onDocClick);
    document.addEventListener('keydown', onKey);
    window.addEventListener('resize', onResize);
    window.addEventListener('scroll', onResize, true);
    return () => {
      document.removeEventListener('mousedown', onDocClick);
      document.removeEventListener('keydown', onKey);
      window.removeEventListener('resize', onResize);
      window.removeEventListener('scroll', onResize, true);
    };
  });

  // Wrapper helpers — close the menu then dispatch the parent action.
  // Inlined per-button to keep the markup the way it was; this keeps
  // the menu close cohesive with each button's click.
  function pick(fn: () => void) {
    return () => {
      open = false;
      fn();
    };
  }
</script>

{#if open}
  <!-- Rendered with `position: fixed` and viewport-clamped coordinates
       so it escapes any ancestor overflow (editor / drawer ancestors)
       and never lands off-screen on narrow phones. -->
  <div
    bind:this={menuEl}
    role="menu"
    aria-label="More actions"
    class="fixed z-50 bg-mantle border border-surface1 rounded-md shadow-xl py-1 text-sm"
    style="top: {menuTop}px; left: {menuLeft}px; width: {menuWidth}px;"
  >
    <button
      type="button"
      role="menuitem"
      onclick={pick(onOpenFind)}
      class="w-full px-3 py-2 flex items-center gap-2.5 text-text hover:bg-surface0 text-left min-h-[2.25rem]"
    >
      <svg viewBox="0 0 24 24" class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8">
        <circle cx="11" cy="11" r="7"/>
        <path d="M21 21l-4.5-4.5" stroke-linecap="round"/>
      </svg>
      <span>Find / replace</span>
    </button>
    <button
      type="button"
      role="menuitem"
      onclick={pick(onOpenHistory)}
      class="w-full px-3 py-2 flex items-center gap-2.5 text-text hover:bg-surface0 text-left min-h-[2.25rem]"
    >
      <svg viewBox="0 0 24 24" class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8">
        <circle cx="12" cy="12" r="9"/>
        <path d="M12 7v5l3 2" stroke-linecap="round"/>
        <path d="M3 12a9 9 0 0114-7.5l1 1" stroke-linecap="round"/>
        <path d="M3 4v4h4" stroke-linecap="round"/>
      </svg>
      <span>Version history</span>
    </button>
    <button
      type="button"
      role="menuitem"
      onclick={pick(onOpenPrint)}
      class="w-full px-3 py-2 flex items-center gap-2.5 text-text hover:bg-surface0 text-left min-h-[2.25rem]"
    >
      <svg viewBox="0 0 24 24" class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8">
        <path d="M6 9V4h12v5"/>
        <rect x="6" y="14" width="12" height="6" rx="1"/>
        <path d="M6 17H4a2 2 0 01-2-2v-3a2 2 0 012-2h16a2 2 0 012 2v3a2 2 0 01-2 2h-2"/>
      </svg>
      <span>Export PDF</span>
    </button>
    <button
      type="button"
      role="menuitem"
      onclick={pick(onOpenPresentation)}
      class="w-full px-3 py-2 flex items-center gap-2.5 text-text hover:bg-surface0 text-left min-h-[2.25rem]"
    >
      <svg viewBox="0 0 24 24" class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8">
        <rect x="3" y="4" width="18" height="13" rx="1.5"/>
        <path d="M8 21h8M12 17v4" stroke-linecap="round"/>
      </svg>
      <span>Slideshow</span>
    </button>
    <button
      type="button"
      role="menuitemcheckbox"
      aria-checked={audioOpen}
      onclick={pick(onToggleAudio)}
      class="w-full px-3 py-2 flex items-center gap-2.5 hover:bg-surface0 text-left min-h-[2.25rem]
        {audioOpen ? 'text-secondary' : 'text-text'}"
    >
      <svg viewBox="0 0 24 24" class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8">
        <path d="M11 5L6 9H2v6h4l5 4V5z" stroke-linejoin="round"/>
        <path d="M15.5 8.5a5 5 0 010 7" stroke-linecap="round"/>
        <path d="M19 5a9 9 0 010 14" stroke-linecap="round"/>
      </svg>
      <span>{audioOpen ? 'Close audio' : 'Read aloud'}</span>
    </button>
    <button
      type="button"
      role="menuitemcheckbox"
      aria-checked={readingMode}
      onclick={pick(onToggleReadingMode)}
      class="w-full px-3 py-2 flex items-center gap-2.5 hover:bg-surface0 text-left min-h-[2.25rem]
        {readingMode ? 'text-primary' : 'text-text'}"
    >
      <svg viewBox="0 0 24 24" class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8">
        <path d="M2 5h7a3 3 0 013 3v11a2 2 0 00-2-2H2V5z"/>
        <path d="M22 5h-7a3 3 0 00-3 3v11a2 2 0 012-2h8V5z"/>
      </svg>
      <span>{readingMode ? 'Exit reading mode' : 'Reading mode'}</span>
    </button>
    <button
      type="button"
      role="menuitemcheckbox"
      aria-checked={focusMode}
      onclick={pick(onToggleFocusMode)}
      class="w-full px-3 py-2 flex items-center gap-2.5 hover:bg-surface0 text-left min-h-[2.25rem]
        {focusMode ? 'text-primary' : 'text-text'}"
    >
      <svg viewBox="0 0 24 24" class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8">
        {#if focusMode}
          <path d="M9 4v4H5M15 4v4h4M9 20v-4H5M15 20v-4h4" stroke-linecap="round" stroke-linejoin="round"/>
        {:else}
          <path d="M4 9V5h4M20 9V5h-4M4 15v4h4M20 15v4h-4" stroke-linecap="round" stroke-linejoin="round"/>
        {/if}
      </svg>
      <span>{focusMode ? 'Exit focus mode' : 'Focus mode'}</span>
    </button>
    <div class="border-t border-surface1 my-1"></div>
    <!-- Schedule flashcards — parses Q:/A: pairs in the body and
         drops a 1/3/7/14/30-day review series on the calendar per
         card. Same shape as the scripture memory-verse drill so
         the calendar treats both surfaces identically. -->
    <button
      type="button"
      role="menuitem"
      onclick={onScheduleFlashcards}
      disabled={schedulingFlashcards}
      class="w-full px-3 py-2 flex items-center gap-2.5 text-text hover:bg-surface0 text-left min-h-[2.25rem] disabled:opacity-60"
    >
      <svg viewBox="0 0 24 24" class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
        <rect x="4" y="6" width="14" height="10" rx="1.5"/>
        <rect x="6" y="4" width="14" height="10" rx="1.5"/>
        <path d="M10 10h4"/>
      </svg>
      <span>{schedulingFlashcards ? 'Scheduling…' : 'Schedule flashcard reviews'}</span>
    </button>
    <button
      type="button"
      role="menuitem"
      onclick={pick(onOpenHelp)}
      class="w-full px-3 py-2 flex items-center gap-2.5 text-text hover:bg-surface0 text-left min-h-[2.25rem]"
    >
      <span class="w-4 h-4 flex-shrink-0 flex items-center justify-center font-mono text-sm">?</span>
      <span>Keyboard shortcuts</span>
    </button>
  </div>
{/if}
