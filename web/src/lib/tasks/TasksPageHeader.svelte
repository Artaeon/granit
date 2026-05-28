<script lang="ts">
  // Slim page header for /tasks. Stream N — replaces the previous two-row
  // header (PageHeader strip + view-tab toolbar) with a single dense bar:
  //   Title · counts · [view-switcher segmented] · [More views ▾] ·
  //     · [density] · [filter] · [quick capture] · [?]
  // The visual hierarchy is loaded onto the SECTION HEADERS below the
  // chrome (overdue red, today amber, etc), so the chrome itself is
  // intentionally muted — it shouldn't compete with the work surface.
  import { focusOnMount } from '$lib/util/focusOnMount';

  type View = 'list' | 'kanban' | 'today' | 'week' | 'triage' | 'inbox' | 'stale' | 'duplicates' | 'quickwins' | 'review' | 'eisenhower';

  type Props = {
    view: View;
    totalCount: number;
    filteredCount: number;
    activeFilterCount: number;
    density: 'normal' | 'compact';
    // Counts surfaced inline next to specific tabs so the user can see
    // the load on Today / Inbox without opening each view.
    todayLoad: number;       // overdue + due-today
    todayOverdue: number;    // separately tracked so the badge can colour red
    inboxLoad: number;
    moreViewsOpen: boolean;
    activeOverflowLabel: string;
    onSelectView: (v: View) => void;
    onToggleMoreViews: () => void;
    onPickOverflowView: (v: View) => void;
    onMoreViewsKey: (e: KeyboardEvent) => void;
    onToggleDensity: () => void;
    onToggleFilterPanel: () => void;
    onQuickCapture: () => void;
    onToggleHelp: () => void;
  };

  let {
    view,
    totalCount,
    filteredCount,
    activeFilterCount,
    density,
    todayLoad,
    todayOverdue,
    inboxLoad,
    moreViewsOpen,
    activeOverflowLabel,
    onSelectView,
    onToggleMoreViews,
    onPickOverflowView,
    onMoreViewsKey,
    onToggleDensity,
    onToggleFilterPanel,
    onQuickCapture,
    onToggleHelp
  }: Props = $props();

  // Primary view-switcher — icon-only segmented control. Order maps to
  // digit shortcuts 1-4: 1=Today, 2=List, 3=Kanban, 4=Matrix. Week is
  // intentionally NOT in the primary cluster anymore (the original had
  // 5 tabs; Stream N narrows to 4 + overflow) — Week sits in More views.
  const PRIMARY: { key: View; label: string; title: string; icon: string }[] = [
    {
      key: 'today',
      label: 'Today',
      title: 'overdue + due today + scheduled today (1)',
      // Simple sun-on-horizon glyph: today.
      icon: 'M12 3v2 M5.6 5.6l1.4 1.4 M3 12h2 M5.6 18.4l1.4-1.4 M12 19v2 M18.4 18.4l-1.4-1.4 M19 12h2 M18.4 5.6l-1.4 1.4 M12 8a4 4 0 1 1 0 8 4 4 0 0 1 0-8z'
    },
    {
      key: 'list',
      label: 'List',
      title: 'grouped flat list (2)',
      // Three horizontal rules with leading dots — list.
      icon: 'M4 6h16 M4 12h16 M4 18h16'
    },
    {
      key: 'kanban',
      label: 'Kanban',
      title: 'kanban board (3)',
      // 3 vertical columns.
      icon: 'M4 5v14 M12 5v10 M20 5v8'
    },
    {
      key: 'eisenhower',
      label: 'Matrix',
      title: 'urgent × important — 2×2 matrix (4)',
      // 2×2 grid.
      icon: 'M4 4h7v7H4z M13 4h7v7h-7z M4 13h7v7H4z M13 13h7v7h-7z'
    }
  ];

  const OVERFLOW: { key: View; label: string; title: string }[] = [
    { key: 'week',       label: 'Week',       title: '7-day rolling grid' },
    { key: 'triage',     label: 'Triage',     title: 'AI-driven inbox triage proposals' },
    { key: 'inbox',      label: 'Inbox',      title: 'untriaged tasks awaiting categorisation' },
    { key: 'stale',      label: 'Stale',      title: 'not touched in 7+ days — needs a decision' },
    { key: 'duplicates', label: 'Duplicates', title: 'near-duplicate task pairs by text similarity' },
    { key: 'quickwins',  label: 'Quick wins', title: 'high priority + ≤30 min — tackle a few before lunch' },
    { key: 'review',     label: 'Review',     title: 'completed in the last 7 days — celebrate the wins' }
  ];
</script>

<div class="flex items-center gap-2 px-3 py-2 border-b border-surface1 flex-shrink-0 bg-mantle">
  <h1 class="text-base sm:text-lg font-semibold text-text leading-none">Tasks</h1>
  <!-- Count chip — total + filtered. The filtered chip only shows when
       the filter actually narrows the list, so the user doesn't have to
       read N/N when nothing is filtering. -->
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

  <!-- Primary view-switcher. Icon-only segmented control on desktop;
       compact label-only on the smallest viewports (the icons collapse
       to text). Active view = primary background with on-primary text. -->
  <div class="hidden sm:flex bg-surface0 border border-surface1 rounded overflow-hidden">
    {#each PRIMARY as p (p.key)}
      <button
        type="button"
        onclick={() => onSelectView(p.key)}
        title={p.title}
        aria-label={p.label}
        aria-pressed={view === p.key}
        class="px-2 py-1.5 inline-flex items-center gap-1 text-xs {view === p.key ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
      >
        <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d={p.icon} />
        </svg>
        <span class="hidden md:inline">{p.label}</span>
        {#if p.key === 'today' && todayLoad > 0 && view !== 'today'}
          <span
            class="text-[10px] tabular-nums font-mono {todayOverdue > 0 ? 'text-error' : 'text-warning'}"
            title="{todayOverdue} overdue + {todayLoad - todayOverdue} due today"
          >{todayLoad}</span>
        {/if}
      </button>
    {/each}
  </div>

  <!-- Compact label-cycler for mobile — keeps the same 4 primary views
       accessible in a tiny pill. -->
  <select
    class="sm:hidden bg-surface0 border border-surface1 rounded px-2 py-1 text-xs text-text"
    value={view}
    onchange={(e) => onSelectView((e.currentTarget as HTMLSelectElement).value as View)}
    aria-label="view"
  >
    {#each PRIMARY as p (p.key)}
      <option value={p.key}>{p.label}</option>
    {/each}
    {#each OVERFLOW as o (o.key)}
      <option value={o.key}>{o.label}</option>
    {/each}
  </select>

  <!-- More-views overflow dropdown. Same data-more-views marker the
       page's click-outside effect looks for. -->
  <div class="relative hidden sm:block" data-more-views>
    <button
      type="button"
      class="px-2 py-1.5 inline-flex items-center gap-1 bg-surface0 border border-surface1 rounded text-xs {activeOverflowLabel ? 'text-primary' : 'text-subtext'} hover:bg-surface1"
      aria-haspopup="true"
      aria-expanded={moreViewsOpen}
      onclick={onToggleMoreViews}
      title="More views"
    >
      {activeOverflowLabel ? `· ${activeOverflowLabel}` : 'More'}
      {#if !activeOverflowLabel && inboxLoad > 0}
        <span class="text-[10px] tabular-nums text-secondary font-mono">{inboxLoad}</span>
      {/if}
      <span class="text-[9px] opacity-70" aria-hidden="true">▾</span>
    </button>
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
            {#if ov.key === 'inbox' && inboxLoad > 0}
              <span class="text-[10px] tabular-nums font-mono text-secondary">{inboxLoad}</span>
            {/if}
          </button>
        {/each}
      </div>
    {/if}
  </div>

  <!-- Density toggle. Persisted at the route level (DENSITY_KEY); icon
       glyph signals current state — three close lines = compact. -->
  <button
    type="button"
    onclick={onToggleDensity}
    aria-pressed={density === 'compact'}
    title={density === 'compact' ? 'Compact — click for comfortable' : 'Comfortable — click for compact'}
    class="px-2 py-1.5 text-xs font-mono leading-none border border-surface1 rounded {density === 'compact' ? 'bg-primary text-on-primary' : 'bg-surface0 text-dim hover:bg-surface1 hover:text-text'}"
  >{density === 'compact' ? '≡' : '≣'}</button>

  <!-- Filter button. Counts active filters in a small badge so the user
       always knows how many dimensions are narrowing the list before
       opening the slide-out panel. -->
  <button
    type="button"
    onclick={onToggleFilterPanel}
    aria-label="filters"
    title="Open the filter panel (priority, project, tag, goal, deadline, search)"
    class="relative px-2 py-1.5 inline-flex items-center gap-1 text-xs border rounded {activeFilterCount > 0 ? 'border-primary bg-surface1 text-primary' : 'border-surface1 bg-surface0 text-subtext hover:bg-surface1 hover:text-text'}"
  >
    <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M4 5h16l-6 8v6l-4-2v-4z" />
    </svg>
    <span class="hidden md:inline">Filter</span>
    {#if activeFilterCount > 0}
      <span class="px-1 py-0 bg-primary text-on-primary text-[9px] font-mono rounded leading-tight">{activeFilterCount}</span>
    {/if}
  </button>

  <!-- Quick capture — kicks the global QuickCaptureFab. -->
  <button
    type="button"
    onclick={onQuickCapture}
    aria-label="Quick capture"
    title="Quick capture (Cmd-Shift-N)"
    class="px-2 py-1.5 text-xs bg-primary text-on-primary rounded hover:opacity-90 inline-flex items-center gap-1"
  >
    <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
      <path d="M12 5v14M5 12h14"/>
    </svg>
    <span class="hidden md:inline">Capture</span>
  </button>

  <button
    type="button"
    onclick={onToggleHelp}
    aria-label="keyboard shortcuts"
    title="keyboard shortcuts (?)"
    class="hidden sm:inline-flex w-7 h-7 items-center justify-center text-dim hover:text-text border border-surface1 rounded text-sm"
  >?</button>
</div>
