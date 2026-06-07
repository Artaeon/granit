<script lang="ts">
  // Slim calendar page header — single ~44px row mirroring the tasks
  // page chrome (TasksPageHeader). Stream R polish.
  //
  // Layout:
  //   LEFT:   "Calendar" title · compact cursor label ("Wed May 28 · Week")
  //   CENTER: icon-only view switcher (Day / 5d / Week / Month / Year / Agenda)
  //   RIGHT:  prev / today / next nav cluster · Find · More ▾ · Capture
  //
  // Plan-mode, density toggles, shortcuts, filters (mobile) all moved
  // into the "More" dropdown to keep the toolbar row from competing
  // with the grid below. Active view = primary tint so a glance at
  // the strip immediately answers "what am I looking at".
  import { focusOnMount } from '$lib/util/focusOnMount';

  type View = 'day' | 'workweek' | 'week' | 'month' | 'year' | 'agenda';
  type HourDensity = 'compact' | 'normal' | 'spacious';
  type MonthDensity = 'comfy' | 'compact';

  let {
    view = $bindable('week'),
    cursor = $bindable(new Date()),
    headline,
    loading = false,
    monthDensity = $bindable('comfy'),
    hourDensity = $bindable('normal'),
    planMode = false,
    onPrev,
    onNext,
    onGotoToday,
    onTogglePlanMode,
    onFindTime,
    onShowShortcuts,
    onOpenFilterDrawer,
    onCapture,
    onPlanDay
  }: {
    view?: View;
    cursor?: Date;
    headline: string;
    loading?: boolean;
    monthDensity?: MonthDensity;
    hourDensity?: HourDensity;
    planMode?: boolean;
    onPrev: () => void;
    onNext: () => void;
    onGotoToday: () => void;
    onTogglePlanMode: () => void;
    onFindTime: () => void;
    onShowShortcuts: () => void;
    onOpenFilterDrawer: () => void;
    onCapture: () => void;
    /** Daily Routine AI — opens the RoutineProposalDrawer and triggers
     *  a streaming proposal for today. Omitted when the page hasn't
     *  wired the drawer yet (e.g. a future surface that doesn't need
     *  the button). */
    onPlanDay?: () => void;
  } = $props();

  let moreOpen = $state(false);
  function toggleMore() { moreOpen = !moreOpen; }
  function closeMore() { moreOpen = false; }

  // Icon-only view switcher. Keys map to the calendar's keyboard
  // shortcuts (d/W/w/m/y/a). Workweek + Year hide below sm: where
  // the strip is tight; both still reachable via the mobile <select>.
  const VIEWS: { key: View; label: string; title: string; icon: string; smOnly?: boolean }[] = [
    {
      key: 'day',
      label: 'Day',
      title: 'Day view (d)',
      // Single column with a header bar.
      icon: 'M5 3h14v18H5z M5 8h14'
    },
    {
      key: 'workweek',
      label: '5d',
      title: 'Workweek — Mon–Fri only (Shift+W)',
      icon: 'M3 5h18 M3 12h18 M3 19h18 M9 5v14 M15 5v14',
      smOnly: true
    },
    {
      key: 'week',
      label: 'Week',
      title: 'Week view (w)',
      // 7 vertical bars representing the week.
      icon: 'M4 5v14 M9 5v14 M14 5v14 M19 5v14 M3 5h18 M3 19h18'
    },
    {
      key: 'month',
      label: 'Month',
      title: 'Month view (m)',
      // 2×2 grid.
      icon: 'M4 4h7v7H4z M13 4h7v7h-7z M4 13h7v7H4z M13 13h7v7h-7z'
    },
    {
      key: 'year',
      label: 'Year',
      title: 'Year view (y)',
      // 3×3 mini-grid.
      icon: 'M4 4h5v5H4z M10 4h5v5h-5z M16 4h4v5h-4z M4 10h5v5H4z M10 10h5v5h-5z M16 10h4v5h-4z M4 16h5v4H4z M10 16h5v4h-5z M16 16h4v4h-4z',
      smOnly: true
    },
    {
      key: 'agenda',
      label: 'Agenda',
      title: 'Agenda — flat 30-day list (a)',
      // 3 horizontal rules with leading dots.
      icon: 'M4 6h16 M4 12h16 M4 18h16'
    }
  ];

  // Compact cursor label for the title strip — "Wed May 28" — paired
  // with the active view name so the user sees both the focus date
  // AND the visible window scope in one glance.
  let compactCursor = $derived(
    cursor.toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' })
  );
  let viewLabel = $derived(
    VIEWS.find((v) => v.key === view)?.label ?? view
  );

  function onMoreKey(e: KeyboardEvent) {
    if (e.key === 'Escape') { closeMore(); }
  }

  // Click-outside guard for the More dropdown.
  function onWindowClick(e: MouseEvent) {
    if (!moreOpen) return;
    const target = e.target as HTMLElement | null;
    if (!target?.closest('[data-more-menu]')) closeMore();
  }
</script>

<svelte:window onclick={onWindowClick} />

<header
  class="flex items-center gap-1.5 px-2 sm:px-3 py-1.5 border-b border-surface1 flex-shrink-0 bg-mantle min-h-[44px]"
>
  <!-- Mobile filter-drawer trigger — desktop hides it because the
       left sidebar is always-visible at md:+ widths. -->
  <button
    onclick={onOpenFilterDrawer}
    aria-label="filters"
    class="md:hidden w-7 h-7 flex items-center justify-center text-subtext hover:bg-surface0 rounded flex-shrink-0"
  >
    <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2">
      <path d="M3 6h18M6 12h12M9 18h6" stroke-linecap="round" />
    </svg>
  </button>

  <!-- LEFT: page title + compact cursor / view stamp. Mirrors the
       Tasks page's "Tasks <count chip>" pattern but with a date
       stamp instead of a count chip — the calendar's "what am I
       looking at" answer is the date window, not a row count. -->
  <h1 class="text-base sm:text-lg font-semibold text-text leading-none flex-shrink-0">Calendar</h1>
  <span class="hidden sm:inline-flex items-center gap-1 text-[11px] font-mono tabular-nums">
    <span class="px-1.5 py-0.5 bg-surface0 border border-surface1 rounded text-subtext truncate max-w-[14rem]">
      <span class="text-text">{compactCursor}</span>
      <span class="text-dim mx-0.5">·</span>
      <span class="text-primary">{viewLabel}</span>
    </span>
  </span>
  {#if loading}
    <span class="hidden md:inline text-[10px] text-dim italic">loading…</span>
  {/if}

  <span class="flex-1"></span>

  <!-- CENTER: icon-only view switcher. Active view = primary fill.
       Workweek + Year collapse on small viewports (still reachable
       via the mobile <select> + keyboard shortcuts). -->
  <div class="hidden sm:flex bg-surface0 border border-surface1 rounded overflow-hidden flex-shrink-0">
    {#each VIEWS as v (v.key)}
      <button
        type="button"
        onclick={() => (view = v.key)}
        title={v.title}
        aria-label={v.label}
        aria-pressed={view === v.key}
        class="w-8 h-7 inline-flex items-center justify-center {v.smOnly ? 'hidden md:inline-flex' : ''} {view === v.key ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1 hover:text-text'}"
      >
        <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d={v.icon} />
        </svg>
      </button>
    {/each}
  </div>

  <!-- Compact view-cycler for mobile — covers every view. -->
  <select
    class="sm:hidden bg-surface0 border border-surface1 rounded px-1.5 py-1 text-xs text-text"
    value={view}
    onchange={(e) => (view = (e.currentTarget as HTMLSelectElement).value as View)}
    aria-label="view"
  >
    {#each VIEWS as v (v.key)}
      <option value={v.key}>{v.label}</option>
    {/each}
  </select>

  <!-- RIGHT: nav cluster. Compact icon trio (prev · today · next),
       same shape Google / Apple Calendar use. -->
  <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden flex-shrink-0">
    <button
      onclick={onPrev}
      aria-label="previous period"
      title="Previous (k / p)"
      class="w-7 h-7 inline-flex items-center justify-center text-subtext hover:bg-surface1 hover:text-text"
    >
      <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M15 6l-6 6 6 6" />
      </svg>
    </button>
    <button
      onclick={onGotoToday}
      aria-label="jump to today"
      title="Today (t)"
      class="w-7 h-7 inline-flex items-center justify-center text-subtext hover:bg-surface1 hover:text-text border-l border-surface1"
    >
      <!-- Calendar-with-dot glyph: today. -->
      <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <rect x="4" y="5" width="16" height="15" rx="1.5" />
        <path d="M4 10h16 M9 3v3 M15 3v3" />
        <circle cx="12" cy="15" r="1.5" fill="currentColor" stroke="none" />
      </svg>
    </button>
    <button
      onclick={onNext}
      aria-label="next period"
      title="Next (j / n)"
      class="w-7 h-7 inline-flex items-center justify-center text-subtext hover:bg-surface1 hover:text-text border-l border-surface1"
    >
      <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M9 6l6 6-6 6" />
      </svg>
    </button>
  </div>

  <!-- Plan my day — Daily Routine AI. Opens the RoutineProposalDrawer
       and streams a proposed rewrite of today's plan + a list of
       event mutations the user can review before applying. -->
  {#if onPlanDay}
    <button
      onclick={onPlanDay}
      class="hidden sm:inline-flex h-7 items-center gap-1 px-2 rounded border bg-surface0 border-surface1 text-subtext hover:border-primary hover:text-text flex-shrink-0 text-xs"
      title="Plan my day with AI"
      aria-label="plan my day"
    >
      <svg
        viewBox="0 0 24 24"
        class="w-3.5 h-3.5"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path d="M12 3v3m0 12v3M3 12h3m12 0h3" />
        <circle cx="12" cy="12" r="4" />
      </svg>
      <span class="hidden md:inline">Plan my day</span>
    </button>
  {/if}

  <!-- Find time — 'f' shortcut. Surfaces free gaps that fit a chosen
       duration. Keeps a prominent slot in the chrome because it's the
       single highest-value scheduling action. -->
  <button
    onclick={onFindTime}
    class="hidden sm:inline-flex w-7 h-7 items-center justify-center rounded border bg-surface0 border-surface1 text-subtext hover:border-primary hover:text-text flex-shrink-0"
    title="Find a free slot (f)"
    aria-label="find free time"
  >
    <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
      <circle cx="12" cy="12" r="9"/>
      <path d="M12 7v5l3 2"/>
    </svg>
  </button>

  <!-- More menu — plan mode, density, jump-to-date, shortcuts.
       Single dropdown so the toolbar stays one row even on tighter
       breakpoints. The button itself lights primary when plan mode
       is on so the user can see the state without opening it. -->
  <div class="relative flex-shrink-0" data-more-menu>
    <button
      type="button"
      onclick={toggleMore}
      aria-haspopup="true"
      aria-expanded={moreOpen}
      title="More options (plan mode, density, jump-to-date, shortcuts)"
      class="w-7 h-7 inline-flex items-center justify-center rounded border {planMode ? 'bg-secondary text-mantle border-secondary' : 'bg-surface0 border-surface1 text-subtext hover:border-primary hover:text-text'}"
    >
      <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round">
        <circle cx="5" cy="12" r="1" />
        <circle cx="12" cy="12" r="1" />
        <circle cx="19" cy="12" r="1" />
      </svg>
    </button>
    {#if moreOpen}
      <div
        role="menu"
        class="absolute right-0 top-full mt-1 z-30 min-w-[14rem] bg-surface0 border border-surface1 rounded shadow-lg py-1 text-xs"
        onkeydown={onMoreKey}
        use:focusOnMount
        tabindex="-1"
      >
        <!-- Plan mode toggle. Same forces-day-view behaviour the
             previous prominent button had. -->
        <button
          type="button"
          role="menuitem"
          onclick={() => { onTogglePlanMode(); closeMore(); }}
          class="w-full text-left px-3 py-1.5 inline-flex items-center justify-between gap-3 {planMode ? 'bg-surface1 text-secondary' : 'text-subtext hover:bg-surface1 hover:text-text'}"
        >
          <span class="inline-flex items-center gap-2">
            <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
              <path d="M3 6h18M3 12h12M3 18h6"/>
            </svg>
            Plan mode
          </span>
          <span class="font-mono text-[10px] {planMode ? 'text-secondary' : 'text-dim'}">{planMode ? 'on' : 'off'}</span>
        </button>

        <!-- Find time — mirrored into the menu so the mobile button
             (hidden in the chrome above sm:) has a path. -->
        <button
          type="button"
          role="menuitem"
          onclick={() => { onFindTime(); closeMore(); }}
          class="sm:hidden w-full text-left px-3 py-1.5 inline-flex items-center gap-2 text-subtext hover:bg-surface1 hover:text-text"
        >
          <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
            <circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>
          </svg>
          Find free time
        </button>

        <!-- Month-density toggle — only relevant in Month view. -->
        {#if view === 'month'}
          <div class="px-3 py-1.5 text-[10px] text-dim uppercase tracking-wider">Month density</div>
          <div class="px-3 pb-1.5 flex gap-1">
            <button
              type="button"
              class="px-2 py-1 text-[11px] rounded border {monthDensity === 'comfy' ? 'bg-primary text-on-primary border-primary' : 'bg-surface0 border-surface1 text-subtext hover:border-primary'}"
              onclick={() => (monthDensity = 'comfy')}
              aria-pressed={monthDensity === 'comfy'}
            >Comfy</button>
            <button
              type="button"
              class="px-2 py-1 text-[11px] rounded border {monthDensity === 'compact' ? 'bg-primary text-on-primary border-primary' : 'bg-surface0 border-surface1 text-subtext hover:border-primary'}"
              onclick={() => (monthDensity = 'compact')}
              aria-pressed={monthDensity === 'compact'}
            >Compact</button>
          </div>
        {/if}

        <!-- Time-grid density — only relevant in Day/Week/Workweek. -->
        {#if view === 'day' || view === 'week' || view === 'workweek'}
          <div class="px-3 py-1.5 text-[10px] text-dim uppercase tracking-wider">Time-grid density</div>
          <div class="px-3 pb-1.5 flex gap-1">
            <button
              type="button"
              class="px-2 py-1 text-[11px] rounded border {hourDensity === 'compact' ? 'bg-primary text-on-primary border-primary' : 'bg-surface0 border-surface1 text-subtext hover:border-primary'}"
              onclick={() => (hourDensity = 'compact')}
              aria-pressed={hourDensity === 'compact'}
              title="32px / hour"
            >Compact</button>
            <button
              type="button"
              class="px-2 py-1 text-[11px] rounded border {hourDensity === 'normal' ? 'bg-primary text-on-primary border-primary' : 'bg-surface0 border-surface1 text-subtext hover:border-primary'}"
              onclick={() => (hourDensity = 'normal')}
              aria-pressed={hourDensity === 'normal'}
              title="48px / hour"
            >Normal</button>
            <button
              type="button"
              class="px-2 py-1 text-[11px] rounded border {hourDensity === 'spacious' ? 'bg-primary text-on-primary border-primary' : 'bg-surface0 border-surface1 text-subtext hover:border-primary'}"
              onclick={() => (hourDensity = 'spacious')}
              aria-pressed={hourDensity === 'spacious'}
              title="72px / hour"
            >Spacious</button>
          </div>
        {/if}

        <!-- Jump-to-date — same native picker the old header had,
             relegated to the menu since it's a low-frequency action
             (most navigation goes through prev/next/today). -->
        <div class="px-3 py-1.5 text-[10px] text-dim uppercase tracking-wider">Jump to date</div>
        <div class="px-3 pb-1.5">
          <input
            type="date"
            aria-label="jump to date"
            onchange={(e) => {
              const v = (e.target as HTMLInputElement).value;
              if (!v) return;
              const [y, mo, d] = v.split('-').map(Number);
              if (y && mo && d) cursor = new Date(y, mo - 1, d);
              (e.target as HTMLInputElement).value = '';
              closeMore();
            }}
            class="w-full px-1.5 py-1 text-xs bg-surface0 border border-surface1 rounded text-text focus:border-primary focus:outline-none"
          />
        </div>

        <!-- Keyboard shortcuts. The chrome hides the bare "?" button
             on small screens, so the menu always carries the entry. -->
        <button
          type="button"
          role="menuitem"
          onclick={() => { onShowShortcuts(); closeMore(); }}
          class="w-full text-left px-3 py-1.5 inline-flex items-center gap-2 text-subtext hover:bg-surface1 hover:text-text border-t border-surface1"
        >
          <span class="w-3.5 h-3.5 inline-flex items-center justify-center font-mono text-[10px] border border-current rounded">?</span>
          Keyboard shortcuts
        </button>
      </div>
    {/if}
  </div>

  <!-- Capture — primary-tinted CTA at the right edge. Mirrors the
       Tasks page's quick-capture button shape and shortcut hint. -->
  <button
    type="button"
    onclick={onCapture}
    aria-label="Create event or task"
    title="New task or event"
    class="px-2 py-1.5 text-xs bg-primary text-on-primary rounded hover:opacity-90 inline-flex items-center gap-1 flex-shrink-0"
  >
    <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
      <path d="M12 5v14M5 12h14"/>
    </svg>
    <span class="hidden md:inline">Capture</span>
  </button>
</header>
