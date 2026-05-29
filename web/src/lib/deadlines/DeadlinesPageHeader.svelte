<script lang="ts">
  // Slim page header for /deadlines. Stream BB. Mirrors
  // TasksPageHeader / GoalsPageHeader: a single dense bar replacing
  // the previous PageHeader strip + separate view-mode toolbar.
  //
  // Layout (left → right):
  //   Title · count chip · [view picker (List / Timeline / Calendar)] ·
  //   [group-by select — list view only] · [search box] · [help ?] ·
  //   [+ New deadline]
  //
  // The work surface (cards + sectioned list) carries the visual
  // hierarchy (importance dots, urgency tinted section headers); the
  // chrome itself is intentionally muted.
  import { focusOnMount } from '$lib/util/focusOnMount';

  type ViewMode = 'list' | 'timeline' | 'calendar';
  type GroupBy = 'urgency' | 'status' | 'month';

  type Props = {
    view: ViewMode;
    groupBy: GroupBy;
    totalCount: number;
    filteredCount: number;
    q: string;
    searchEl?: HTMLInputElement | null;
    shortcutsOpen: boolean;
    onSelectView: (v: ViewMode) => void;
    onSelectGroup: (g: GroupBy) => void;
    onSearchChange: (v: string) => void;
    onSearchKey: (e: KeyboardEvent) => void;
    onToggleShortcuts: () => void;
    onCloseShortcuts: () => void;
    onCreate: () => void;
  };

  let {
    view,
    groupBy,
    totalCount,
    filteredCount,
    q,
    searchEl = $bindable(null),
    shortcutsOpen,
    onSelectView,
    onSelectGroup,
    onSearchChange,
    onSearchKey,
    onToggleShortcuts,
    onCloseShortcuts,
    onCreate
  }: Props = $props();

  // Primary icon-segmented view picker — List / Timeline / Calendar.
  // Glyphs match the inline emoji the old toolbar used (☰ ┊ ▦) but
  // rendered as SVG paths so they read consistently against the
  // segmented background tint.
  const VIEWS: { key: ViewMode; label: string; title: string; icon: string }[] = [
    {
      key: 'list',
      label: 'List',
      title: 'grouped list (v to cycle)',
      // Three horizontal rules — flat list.
      icon: 'M4 6h16 M4 12h16 M4 18h16'
    },
    {
      key: 'timeline',
      label: 'Timeline',
      title: 'vertical rail of deadlines (v to cycle)',
      // Vertical line with two dots — timeline rail.
      icon: 'M12 4v16 M9 8h6 M9 14h6'
    },
    {
      key: 'calendar',
      label: 'Calendar',
      title: 'month-grid heat view (v to cycle)',
      // 2x2 grid as a calendar glyph.
      icon: 'M4 4h7v7H4z M13 4h7v7h-7z M4 13h7v7H4z M13 13h7v7h-7z'
    }
  ];

  const GROUPS: { key: GroupBy; label: string; title: string }[] = [
    { key: 'urgency', label: 'Urgency', title: 'Overdue → This week → … (g to cycle)' },
    { key: 'status', label: 'Status', title: 'Active / Missed / Met / Cancelled (g to cycle)' },
    { key: 'month', label: 'Month', title: 'Calendar-month buckets (g to cycle)' }
  ];
</script>

<div class="flex items-center gap-2 px-3 py-2 border-b border-surface1 flex-shrink-0 bg-mantle">
  <h1 class="text-base sm:text-lg font-semibold text-text leading-none">Deadlines</h1>

  <!-- Count chip — total + filtered. The filtered chip only renders
       when the active filter actually narrows the list. -->
  <span class="inline-flex items-center gap-1 text-[11px] font-mono tabular-nums">
    <span class="px-1.5 py-0.5 bg-surface0 border border-surface1 rounded text-dim">
      <span class="text-text font-semibold">{totalCount}</span>
    </span>
    {#if filteredCount !== totalCount}
      <span class="text-dim">/</span>
      <span class="px-1.5 py-0.5 bg-surface0 border border-primary/40 rounded">
        <span class="text-primary font-semibold">{filteredCount}</span>
      </span>
    {/if}
  </span>

  <span class="flex-1"></span>

  <!-- View picker — icon-segmented on desktop. Mobile collapses to
       a labelled select so the icons don't crowd the header. -->
  <div class="hidden sm:flex bg-surface0 border border-surface1 rounded overflow-hidden">
    {#each VIEWS as v (v.key)}
      <button
        type="button"
        onclick={() => onSelectView(v.key)}
        title={v.title}
        aria-label={v.label}
        aria-pressed={view === v.key}
        class="px-2 py-1.5 inline-flex items-center gap-1 text-xs {view === v.key ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
      >
        <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d={v.icon} />
        </svg>
        <span class="hidden md:inline">{v.label}</span>
      </button>
    {/each}
  </div>

  <select
    class="sm:hidden bg-surface0 border border-surface1 rounded px-2 py-1 text-xs text-text"
    value={view}
    onchange={(e) => onSelectView((e.currentTarget as HTMLSelectElement).value as ViewMode)}
    aria-label="view"
  >
    {#each VIEWS as v (v.key)}
      <option value={v.key}>{v.label}</option>
    {/each}
  </select>

  <!-- Group-by — only meaningful in list view. Compact select
       replaces the old segmented control to save horizontal space in
       the slim header. -->
  {#if view === 'list'}
    <select
      class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text"
      value={groupBy}
      onchange={(e) => onSelectGroup((e.currentTarget as HTMLSelectElement).value as GroupBy)}
      aria-label="group by"
      title="Group list by… (g to cycle)"
    >
      {#each GROUPS as g (g.key)}
        <option value={g.key}>by {g.label.toLowerCase()}</option>
      {/each}
    </select>
  {/if}

  <!-- Search — title / description / project / venture substring. -->
  <div class="relative hidden md:block">
    <input
      bind:this={searchEl}
      value={q}
      oninput={(e) => onSearchChange((e.currentTarget as HTMLInputElement).value)}
      onkeydown={onSearchKey}
      type="text"
      placeholder="Search… (/)"
      data-page-search="1"
      class="w-44 lg:w-56 pl-7 pr-6 py-1.5 bg-surface0 border border-surface1 rounded text-xs text-text focus:outline-none focus:border-primary"
    />
    <span class="absolute left-2 top-1/2 -translate-y-1/2 text-dim text-xs pointer-events-none" aria-hidden="true">⌕</span>
    {#if q}
      <button
        type="button"
        onclick={() => onSearchChange('')}
        class="absolute right-1.5 top-1/2 -translate-y-1/2 text-dim hover:text-text text-xs"
        aria-label="clear search"
      >×</button>
    {/if}
  </div>

  <!-- Help chip — opens a popover listing the page-scoped keybinds.
       The outside-click capture sits in a fixed overlay below the
       popover so click-anywhere-else dismisses it. -->
  <div class="relative">
    <button
      type="button"
      onclick={onToggleShortcuts}
      aria-expanded={shortcutsOpen}
      aria-label="Keyboard shortcuts"
      title="Keyboard shortcuts (?)"
      class="hidden sm:inline-flex w-7 h-7 items-center justify-center rounded border border-surface1 text-dim hover:text-text hover:border-surface2 text-xs"
    >?</button>
    {#if shortcutsOpen}
      <button
        type="button"
        aria-label="close shortcuts"
        onclick={onCloseShortcuts}
        class="fixed inset-0 z-30 cursor-default"
      ></button>
      <div
        role="dialog"
        aria-label="Keyboard shortcuts"
        use:focusOnMount
        tabindex="-1"
        class="absolute right-0 mt-1 z-40 w-64 bg-surface0 border border-surface1 rounded-lg shadow-lg p-3 text-xs"
      >
        <div class="text-[10px] uppercase tracking-wider text-dim font-medium mb-2">Keyboard shortcuts</div>
        <ul class="space-y-1.5">
          {#each [
            { k: 'n', label: 'New deadline' },
            { k: '/', label: 'Focus search' },
            { k: '1 / 2 / 3', label: 'Filter Critical / High / Normal' },
            { k: 'v', label: 'Cycle view (list / timeline / calendar)' },
            { k: 'g', label: 'Cycle group-by (urgency / status / month)' },
            { k: 'Esc', label: 'Clear filters / close' },
            { k: '?', label: 'Toggle this help' }
          ] as row}
            <li class="flex items-baseline gap-2">
              <kbd class="font-mono text-[10px] bg-surface1 px-1.5 py-0.5 rounded text-text whitespace-nowrap">{row.k}</kbd>
              <span class="text-subtext">{row.label}</span>
            </li>
          {/each}
        </ul>
      </div>
    {/if}
  </div>

  <!-- Primary create button. -->
  <button
    type="button"
    onclick={onCreate}
    aria-label="New deadline"
    title="New deadline (n)"
    class="px-2 py-1.5 text-xs bg-primary text-on-primary rounded hover:opacity-90 inline-flex items-center gap-1"
  >
    <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
      <path d="M12 5v14M5 12h14"/>
    </svg>
    <span class="hidden md:inline">New deadline</span>
  </button>
</div>
