<script lang="ts">
  // Slim page header for /notes. Stream T — replaces the previous three-
  // row chrome (title + create button, search bar, view-tab strip) with a
  // single dense bar mirroring the /tasks pattern:
  //   Title · count · [search input] · [view-switcher segmented] ·
  //     · [More views ▾] · [New]
  // Quick-filter chips (folder/tag clear pills, sort options, save-as-
  // collection) live in a sibling NotesQuickFilters row below this
  // header so the chrome itself stays mute and the work surface owns
  // the visual hierarchy.
  import { focusOnMount } from '$lib/util/focusOnMount';
  import Button from '$lib/components/Button.svelte';

  type View = 'stream' | 'recent' | 'tree' | 'pinned' | 'all' | 'alpha' | 'tags' | 'collections' | 'folders' | 'search';

  type Props = {
    view: View;
    q: string;
    notesCount: number;
    pinnedCount: number;
    searchResultsCount: number;
    moreViewsOpen: boolean;
    activeOverflowLabel: string;
    onSelectView: (v: View) => void;
    onToggleMoreViews: () => void;
    onPickOverflowView: (v: View) => void;
    onMoreViewsKey: (e: KeyboardEvent) => void;
    onQuickCapture: () => void;
    onSearchInput: (value: string) => void;
    onSearchFocus: () => void;
  };

  let {
    view,
    q = $bindable(),
    notesCount,
    pinnedCount,
    searchResultsCount,
    moreViewsOpen,
    activeOverflowLabel,
    onSelectView,
    onToggleMoreViews,
    onPickOverflowView,
    onMoreViewsKey,
    onQuickCapture,
    onSearchInput,
    onSearchFocus
  }: Props = $props();

  // Primary view-switcher — icon-only segmented control. The 5 most-
  // used views: Stream (recency buckets), Tree (hierarchy), Recent (top
  // 30 flat), Pinned, All. Everything else (alpha / tags / collections
  // / folders) sits in the overflow menu so the bar stays narrow on a
  // phone viewport.
  const PRIMARY: { key: View; label: string; title: string; icon: string }[] = [
    {
      key: 'stream',
      label: 'Stream',
      title: 'reverse-chrono buckets — Today / Yesterday / This week / …',
      // Stack of horizontal bars suggesting a time stream.
      icon: 'M3 6h18 M3 12h14 M3 18h10'
    },
    {
      key: 'tree',
      label: 'Tree',
      title: 'classic folder hierarchy',
      // Folder + branch glyph.
      icon: 'M3 7h6l2 2h10v10H3z M7 13h10'
    },
    {
      key: 'recent',
      label: 'Recent',
      title: 'top 30 by modified time — flat list',
      // Clock.
      icon: 'M12 8v4l3 2 M12 3a9 9 0 1 1 0 18 9 9 0 0 1 0-18z'
    },
    {
      key: 'pinned',
      label: 'Pinned',
      title: 'your anchored set',
      // Star.
      icon: 'M12 3l2.6 6.5L22 10.3l-5.4 4.7L18 22l-6-3.5L6 22l1.4-7L2 10.3l7.4-.8z'
    },
    {
      key: 'all',
      label: 'All',
      title: 'flat list with sort options',
      // Three rules — list.
      icon: 'M4 6h16 M4 12h16 M4 18h16'
    }
  ];

  const OVERFLOW: { key: View; label: string; title: string }[] = [
    { key: 'alpha',       label: 'A–Z',          title: 'alphabetical with letter dividers' },
    { key: 'tags',        label: 'Tags',         title: 'grouped by primary tag' },
    { key: 'folders',     label: 'Folders',      title: 'top-level folder cards' },
    { key: 'collections', label: 'Collections',  title: 'saved virtual folders (filter recipes)' }
  ];

  // All views, used by the mobile <select> so every view stays reachable
  // on a small screen without the dropdown.
  const ALL_VIEWS: { key: View; label: string }[] = [
    ...PRIMARY.map((p) => ({ key: p.key, label: p.label })),
    ...OVERFLOW.map((o) => ({ key: o.key, label: o.label }))
  ];
</script>

<div class="flex items-center gap-2 px-3 py-2 border-b border-surface1 flex-shrink-0 bg-mantle">
  <h1 class="text-base sm:text-lg font-semibold text-text leading-none">Notes</h1>
  <!-- Count chip — total + pinned. Compact, monospaced so the digits
       don't shift width as notes are added/removed. -->
  <span class="inline-flex items-center gap-1 text-[11px] font-mono tabular-nums">
    <span class="px-1.5 py-0.5 bg-surface0 border border-surface1 rounded text-dim">
      <span class="text-text font-semibold">{notesCount}</span>
    </span>
    {#if pinnedCount > 0}
      <span class="text-dim" title="pinned">★{pinnedCount}</span>
    {/if}
  </span>

  <!-- Search — flex-1 so it consumes the remaining horizontal space.
       data-page-search="1" hooks the global `/` shortcut. The
       view='search' flip is the parent's responsibility (passed in via
       onSearchInput / onSearchFocus). -->
  <div class="relative flex-1 min-w-0 max-w-md">
    <svg
      viewBox="0 0 24 24"
      class="absolute left-2 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-dim pointer-events-none"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      aria-hidden="true"
    >
      <circle cx="11" cy="11" r="7"></circle>
      <path d="m21 21-4.3-4.3"></path>
    </svg>
    <input
      bind:value={q}
      onfocus={onSearchFocus}
      oninput={(e) => onSearchInput((e.currentTarget as HTMLInputElement).value)}
      placeholder="Search notes…"
      data-page-search="1"
      class="w-full pl-7 pr-2 py-1.5 bg-surface0 border border-surface1 rounded text-xs text-text placeholder-dim focus:outline-none focus:border-primary"
    />
  </div>

  <!-- Primary view-switcher. Icon-segmented control; the label is
       hidden under md so the bar stays narrow. Active = primary
       background; the search-result tab badge shows when a query is
       active and the active view is `search`. -->
  <div class="hidden sm:flex bg-surface0 border border-surface1 rounded overflow-hidden">
    {#each PRIMARY as p (p.key)}
      <Button
        variant="ghost"
        active={view === p.key}
        onclick={() => onSelectView(p.key)}
        title={p.title}
        aria-label={p.label}
        aria-pressed={view === p.key}
      >
        <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d={p.icon} />
        </svg>
        <span class="hidden md:inline">{p.label}</span>
        {#if p.key === 'pinned' && pinnedCount > 0 && view !== 'pinned'}
          <span class="text-[10px] tabular-nums font-mono text-warning">{pinnedCount}</span>
        {/if}
      </Button>
    {/each}
  </div>

  <!-- Mobile-only view picker. Includes EVERY view (primary + overflow
       + search if a query is active) so the user has a single control
       to switch view on a small screen. -->
  <select
    class="sm:hidden bg-surface0 border border-surface1 rounded px-2 py-1 text-xs text-text"
    value={view}
    onchange={(e) => onSelectView((e.currentTarget as HTMLSelectElement).value as View)}
    aria-label="view"
  >
    {#each ALL_VIEWS as v (v.key)}
      <option value={v.key}>{v.label}</option>
    {/each}
    {#if q.trim()}
      <option value="search">Search ({searchResultsCount})</option>
    {/if}
  </select>

  <!-- More-views overflow dropdown. data-more-views marker hooks the
       parent's click-outside effect. Shows '· {label}' when an
       overflow view is currently active so the user still has the
       visual breadcrumb. -->
  <div class="relative hidden sm:block" data-more-views>
    <Button
      variant="secondary"
      active={!!activeOverflowLabel}
      aria-haspopup="true"
      aria-expanded={moreViewsOpen}
      onclick={onToggleMoreViews}
      title="More views"
    >
      {activeOverflowLabel ? `· ${activeOverflowLabel}` : 'More'}
      <span class="text-[9px] opacity-70" aria-hidden="true">▾</span>
    </Button>
    {#if moreViewsOpen}
      <div
        role="menu"
        class="absolute right-0 top-full mt-1 z-30 min-w-[11rem] bg-surface0 border border-surface1 rounded shadow-lg py-1 text-xs"
        onkeydown={onMoreViewsKey}
        use:focusOnMount
        tabindex="-1"
      >
        {#each OVERFLOW as ov (ov.key)}
          <button
            type="button"
            role="menuitem"
            class="w-full text-left px-3 py-1.5 inline-flex items-center justify-between gap-3 {view === ov.key ? 'bg-surface1 text-primary' : 'text-subtext hover:bg-surface1 hover:text-text'}"
            onclick={() => onPickOverflowView(ov.key)}
            title={ov.title}
          >
            <span>{ov.label}</span>
          </button>
        {/each}
      </div>
    {/if}
  </div>

  <!-- Quick-capture entry. Mirrors tasks' Capture button so muscle
       memory transfers between pages. -->
  <Button
    variant="primary"
    onclick={onQuickCapture}
    aria-label="Quick capture"
    title="Quick capture (⌘N)"
  >
    <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round">
      <path d="M12 5v14M5 12h14"/>
    </svg>
    <span class="hidden md:inline">New</span>
  </Button>
</div>
