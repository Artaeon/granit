<script lang="ts">
  // Note editor header — title, breadcrumbs, tag chips, daily-note
  // arrows, pin toggle, word-count chip, AI-draft badge, streak,
  // view-mode toggle, AI / Research / overflow / save / info-drawer
  // buttons.
  //
  // Extracted from routes/notes/[...path]/+page on 2026-05-28 as
  // part of the god-file decomposition. Markup is byte-identical
  // to the inlined version. The page still owns every piece of
  // state behind it — this component is a presentational shell
  // with a wide prop signature on purpose: cleanly typed inputs +
  // explicit callbacks beat a ball-of-wax that reaches up into
  // the page's closures.
  //
  // overflowTriggerEl is $bindable so the parent can pass the same
  // ref to <NoteOverflowMenu> for click-outside / repositioning.

  import type { Note } from '$lib/api';
  import AIDraftBadge from '$lib/notes/AIDraftBadge.svelte';
  import StreakBadge from '$lib/notes/StreakBadge.svelte';

  type ViewMode = 'edit' | 'preview' | 'split';
  interface Crumb { label: string; href: string }

  interface Props {
    note: Note;
    viewMode: ViewMode;
    isDaily: boolean;
    dailyDate: string | null;
    dailyLabel: string;
    visibleCrumbs: Crumb[];
    allCrumbs: Crumb[];
    crumbsCollapsed: boolean;
    pinned: Set<string>;
    pinBusy: boolean;
    wordCount: number;
    readingMinutes: number;
    previewProgress: number;
    saveStatus: string;
    saving: boolean;
    dirty: boolean;
    saveFailed: boolean;
    saveFlash: boolean;
    overflowOpen: boolean;
    overflowTriggerEl: HTMLButtonElement | undefined;
    onOpenTreeDrawer: () => void;
    onOpenInfoDrawer: () => void;
    onExpandBreadcrumbs: () => void;
    onSetViewMode: (m: ViewMode) => void;
    onTogglePin: () => void;
    onGotoDaily: (dateOrToday: string) => void;
    onShiftDate: (iso: string, days: number) => string;
    onDispatchAI: () => void;
    onOpenResearchMode: () => void;
    onToggleOverflow: () => void;
    onSave: () => void;
  }

  let {
    note,
    viewMode,
    isDaily,
    dailyDate,
    dailyLabel,
    visibleCrumbs,
    allCrumbs,
    crumbsCollapsed,
    pinned,
    pinBusy,
    wordCount,
    readingMinutes,
    previewProgress,
    saveStatus,
    saving,
    dirty,
    saveFailed,
    saveFlash,
    overflowOpen,
    overflowTriggerEl = $bindable(),
    onOpenTreeDrawer,
    onOpenInfoDrawer,
    onExpandBreadcrumbs,
    onSetViewMode,
    onTogglePin,
    onGotoDaily,
    onShiftDate,
    onDispatchAI,
    onOpenResearchMode,
    onToggleOverflow,
    onSave
  }: Props = $props();
</script>

<header class="flex items-center gap-1.5 sm:gap-2 px-2 sm:px-3 py-2 border-b border-surface1 flex-shrink-0 bg-mantle sticky top-0 z-20">
  <!-- Hidden on mobile: the layout's top-bar already shows a back
       arrow to /notes for any subpath, so a second one here pushes
       the view-mode toggle (and save button) off the right edge on
       narrow phones. -->
  <a
    href="/notes"
    aria-label="back to notes"
    class="hidden md:flex w-9 h-9 items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0"
  >
    <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
      <path d="M15 18l-6-6 6-6" stroke-linecap="round" stroke-linejoin="round" />
    </svg>
  </a>
  <button
    onclick={onOpenTreeDrawer}
    aria-label="vault tree"
    title="vault tree"
    class="lg:hidden w-9 h-9 flex items-center justify-center text-subtext hover:bg-surface0 rounded flex-shrink-0"
  >
    <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
      <path d="M3 6h18M3 12h18M3 18h18" stroke-linecap="round" />
    </svg>
  </button>
  {#if isDaily && dailyDate}
    <button
      onclick={() => onGotoDaily(onShiftDate(dailyDate, -1))}
      aria-label="previous day"
      title="previous day"
      class="w-9 h-9 flex items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0"
    >‹</button>
  {/if}
  <div class="min-w-0 flex-1">
    <!-- Single-line ellipsis with the full title surfaced via
         the native tooltip + an explicit aria-label so the
         hidden tail is still discoverable on hover (desktop)
         and accessible to screen readers. The h1 itself only
         shows up to one row's worth so the buttons on the
         right never get pushed off the viewport. -->
    <h1
      class="text-base sm:text-lg font-semibold text-text truncate"
      title={note.title}
      aria-label={note.title}
    >
      {note.title}
      {#if dailyLabel}
        <span class="ml-2 text-xs font-normal text-dim uppercase tracking-wider">{dailyLabel}</span>
      {/if}
    </h1>
    <!-- Folder breadcrumbs — each segment is a clickable filter
         link back into the notes index. Deep paths collapse to
         first/…/last with the ellipsis acting as an expand
         toggle so the bar stays one-line even on
         work/projects/2026/q1/notes/foo.md. Tag chips render
         beside the trail when present. -->
    <div class="text-[11px] text-dim flex items-center gap-1 min-w-0 flex-nowrap overflow-hidden">
      <a href="/notes" class="hover:text-primary flex-shrink-0">vault</a>
      {#each visibleCrumbs as c, i}
        <span class="text-dim/60 flex-shrink-0">/</span>
        {#if crumbsCollapsed && i === 2}
          <button
            type="button"
            onclick={onExpandBreadcrumbs}
            class="px-1 rounded hover:bg-surface0 hover:text-text flex-shrink-0 font-mono"
            title="Show full path ({allCrumbs.length} folders)"
            aria-label="Expand collapsed folders"
          >…</button>
          <span class="text-dim/60 flex-shrink-0">/</span>
        {/if}
        <a
          href={c.href}
          class="hover:text-primary truncate font-mono {i === visibleCrumbs.length - 1 ? '' : 'flex-shrink'}"
          title={c.label}
        >{c.label}</a>
      {/each}
      {#if (note.frontmatter as Record<string, unknown>)?.tags && Array.isArray((note.frontmatter as Record<string, unknown>).tags)}
        <span class="ml-2 hidden sm:flex items-center gap-1 flex-wrap min-w-0">
          {#each ((note.frontmatter as Record<string, unknown>).tags as string[]).slice(0, 6) as t}
            <a
              href="/notes?tag={encodeURIComponent(t)}"
              class="px-1.5 py-0.5 rounded text-[10px] hover:bg-surface1 flex-shrink-0"
              style="background: color-mix(in srgb, var(--color-secondary) 14%, transparent); color: var(--color-secondary);"
            >#{t}</a>
          {/each}
        </span>
      {/if}
    </div>
  </div>
  {#if isDaily && dailyDate}
    <button
      onclick={() => onGotoDaily(onShiftDate(dailyDate, 1))}
      aria-label="next day"
      title="next day"
      class="w-9 h-9 flex items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0"
    >›</button>
    <button
      onclick={() => onGotoDaily('today')}
      aria-label="today"
      title="jump to today"
      class="px-3 py-1.5 text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary hover:text-primary hidden md:inline-flex flex-shrink-0"
    >today</button>
  {/if}
  <button
    onclick={onTogglePin}
    disabled={pinBusy}
    aria-label={pinned.has(note.path) ? 'unpin' : 'pin'}
    title={pinned.has(note.path) ? 'unpin from dashboard' : 'pin to dashboard'}
    class="w-9 h-9 flex items-center justify-center rounded text-lg disabled:opacity-50
      {pinned.has(note.path) ? 'text-warning' : 'text-dim hover:text-warning'}"
  >
    {pinned.has(note.path) ? '★' : '☆'}
  </button>
  <span class="text-xs text-dim hidden lg:inline">
    {wordCount} words{#if wordCount >= 50} · {readingMinutes} min read{#if viewMode === 'preview' && previewProgress > 0.05 && previewProgress < 0.95} · {Math.max(1, Math.ceil(readingMinutes * (1 - previewProgress)))} left{/if}{/if}
  </span>
  <!-- AI-draft back-link chip — surfaces for notes saved
       through the sidebar chat's "save as note" flow. Reads
       frontmatter.type === 'ai-draft' + optional project /
       goal / calendar_window to render a back-link to the
       source context. Self-hides when the note isn't a
       draft, so the row stays clean for hand-written notes. -->
  <AIDraftBadge
    frontmatter={note.frontmatter as Record<string, unknown> | undefined}
  />
  <!-- Daily-note streak badge — surfaces consecutive-day count
       when the user has any history. Auto-hides when there's no
       history to brag about. Wrapped in a hidden-on-phones span
       so the streak chip doesn't squeeze the title row on narrow
       viewports; the badge still renders on the dashboard and
       at sm+. -->
  <span class="hidden lg:inline-flex">
    <StreakBadge />
  </span>
  <!-- view-mode toggle: 3-button strip from md+ (when there's room
       for icon + tooltip), 2-button toggle below md so the header
       keeps its save button on-screen on phones. -->
  <div class="hidden md:flex bg-surface0 border border-surface1 rounded overflow-hidden text-xs">
    {#each [{m: 'edit', l: 'edit', i: '✎'}, {m: 'split', l: 'split', i: '⊟'}, {m: 'preview', l: 'preview', i: '👁'}] as v}
      <button
        onclick={() => onSetViewMode(v.m as ViewMode)}
        class="px-2.5 py-1.5 {viewMode === v.m ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
        title={v.l}
      >
        <span class="text-[11px]">{v.i}</span>
      </button>
    {/each}
  </div>
  <!-- mobile: 2-mode toggle (edit/preview only). Sub-md so it
       only appears when the 3-button strip above is hidden — both
       at the same breakpoint to avoid a dead window where neither
       toggle renders. -->
  <button
    onclick={() => onSetViewMode(viewMode === 'preview' ? 'edit' : 'preview')}
    aria-label={viewMode === 'preview' ? 'edit source' : 'show preview'}
    class="md:hidden w-9 h-9 flex items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0 text-base"
  >
    {viewMode === 'preview' ? '✎' : '👁'}
  </button>
  <!-- 2026-05-25 toolbar redesign: secondary actions (find,
       history, PDF, slideshow, audio, reading mode, focus
       mode, help) ALL live in the overflow menu now — at
       every breakpoint, not just below lg. The header was
       carrying ~17 buttons on desktop, which pushed the
       title row into a hard-to-scan strip. Primary tier
       (view-mode, AI, Research, overflow) stays in the
       rail; the rest collapses behind ⋯ even on wide
       monitors. Faster scan, cleaner rhythm, single home
       for "everything else". -->

  <!-- AI affordance: open the InlineAIMenu at the editor's
       cursor. Cmd-/ from inside the editor does the same thing
       with a keystroke; this button is for click-first users
       and as a discoverable entry point in the toolbar.
       (Previously bound to Mod-k, but that chord is now
       claimed by the global CommandPalette + markdown-link —
       dispatching it here would either open search or wrap
       the selection as a link.) -->
  <button
    type="button"
    onclick={onDispatchAI}
    title="AI — Cmd-/ or type /ai in the editor"
    class="w-9 h-9 flex items-center justify-center text-subtext hover:text-text hover:bg-surface0 rounded flex-shrink-0 text-[10px] font-mono uppercase tracking-wider"
  >AI</button>
  <!-- Research Mode — pins the AI overlay as a side rail
       seeded with this note's title + tags + leading excerpt,
       framed as exploration. Stays open while the user
       wanders backlinks / annotations / other notes so the
       AI is a running thinking partner rather than a one-
       shot Q&A. Hidden below lg because the toolbar is
       already busy on tablet+phone. -->
  <button
    type="button"
    onclick={onOpenResearchMode}
    title="Research Mode — pin AI side-rail with this note as context"
    aria-label="open research mode"
    class="hidden lg:flex w-9 h-9 items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0 text-base"
  >
    <span aria-hidden="true">🔬</span>
  </button>
  <!-- Overflow trigger — single home for all secondary actions
       (find, history, PDF, slideshow, audio, reading mode,
       focus mode, keyboard shortcuts, flashcards). Visible at
       every breakpoint so the toolbar has one consistent
       "more actions" affordance and the wide-viewport row
       doesn't carry 17 buttons. -->
  <button
    bind:this={overflowTriggerEl}
    onclick={onToggleOverflow}
    aria-label="More actions"
    aria-haspopup="menu"
    aria-expanded={overflowOpen}
    title="More actions"
    class="w-9 h-9 flex items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0 text-lg leading-none"
  >⋯</button>
  <!-- Save button. Stays primary — it's the always-on signal of
       whether the buffer is dirty, saving, or saved cleanly. -->
  <button
    onclick={onSave}
    disabled={(!dirty && !saveFailed) || saving}
    title={saveStatus}
    class="px-3 sm:px-4 py-2.5 sm:py-2 min-h-[40px] sm:min-h-0 rounded text-sm font-medium disabled:opacity-60 transition-shadow flex-shrink-0
      {saveFailed ? 'bg-error text-mantle' : dirty || saving ? 'bg-primary text-on-primary' : 'bg-surface1 text-subtext'}
      {saveFlash ? 'save-flash' : ''}"
  >
    {saveStatus}
  </button>
  <button
    onclick={onOpenInfoDrawer}
    aria-label="outline & backlinks"
    class="xl:hidden w-9 h-9 flex items-center justify-center text-subtext hover:bg-surface0 rounded"
  >
    <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
      <path d="M4 6h16M4 10h10M4 14h16M4 18h10" stroke-linecap="round" />
    </svg>
  </button>
</header>

<style>
  /* Save-button success flash — 1.2s outline pulse that fires when
     the parent flips saveFlash=true after a successful autosave.
     Has to live in the component that renders the .save-flash
     element (the save button) because Svelte's scoped CSS hashes
     selectors per-component; a copy in the page's <style> would
     not reach into this component's DOM. Token values are
     identical to the original page-scoped rule. */
  @keyframes save-flash {
    0%   { box-shadow: 0 0 0 0 rgb(var(--color-success-rgb, 34 197 94) / 0.55); }
    60%  { box-shadow: 0 0 0 6px rgb(var(--color-success-rgb, 34 197 94) / 0); }
    100% { box-shadow: 0 0 0 0 rgb(var(--color-success-rgb, 34 197 94) / 0); }
  }
  .save-flash {
    animation: save-flash 1.2s ease-out 1;
  }
</style>

