<script lang="ts">
  // Calendar page header chrome: filter-drawer trigger (mobile),
  // today / prev / next, headline, date-jump input, find-time, plan-
  // mode toggle, month-density toggle, hour-density toggle, view
  // switcher, shortcuts trigger. Extracted verbatim from
  // +page.svelte so the host page only owns load(), filter, drag
  // dispatch logic — not the toolbar markup.
  //
  // Behaviour is identical to the pre-extraction inline version.
  // The view + cursor + density values stay owned by the parent
  // (they drive other layout decisions like which renderer to mount
  // and are persisted to localStorage) — this component reads them
  // through props and writes back via $bindable.

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
    onOpenFilterDrawer
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
  } = $props();
</script>

<header class="flex items-center gap-1 px-2 sm:px-3 py-1.5 border-b border-surface1 flex-shrink-0 flex-wrap">
  <button
    onclick={onOpenFilterDrawer}
    aria-label="filters"
    class="md:hidden w-8 h-8 flex items-center justify-center text-subtext hover:bg-surface0 rounded"
  >
    <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
      <path d="M3 6h18M6 12h12M9 18h6" stroke-linecap="round" />
    </svg>
  </button>
  <button onclick={onGotoToday} class="px-2 py-1 text-xs sm:text-sm bg-surface0 border border-surface1 rounded hover:border-primary">today</button>
  <button onclick={onPrev} aria-label="prev" class="w-7 h-7 flex items-center justify-center text-sm bg-surface0 border border-surface1 rounded hover:border-primary">‹</button>
  <button onclick={onNext} aria-label="next" class="w-7 h-7 flex items-center justify-center text-sm bg-surface0 border border-surface1 rounded hover:border-primary">›</button>
  <h2 class="text-sm sm:text-base text-text font-medium truncate">{headline}</h2>
  <!-- Jump to a specific date. Hidden on the smallest screens
       where the header is already crowded; the prev/next +
       "today" buttons cover the common case. The input is
       always reset back to empty after a navigation so re-
       selecting the same date triggers the jump again. -->
  <input
    type="date"
    aria-label="jump to date"
    title="Jump to date"
    onchange={(e) => {
      const v = (e.target as HTMLInputElement).value;
      if (!v) return;
      const [y, mo, d] = v.split('-').map(Number);
      if (y && mo && d) cursor = new Date(y, mo - 1, d);
      (e.target as HTMLInputElement).value = '';
    }}
    class="hidden sm:block px-1.5 py-1 text-xs bg-surface0 border border-surface1 rounded text-dim hover:border-primary focus:border-primary focus:outline-none"
  />
  <span class="flex-1"></span>
  {#if loading}<span class="hidden sm:inline text-xs text-dim">loading…</span>{/if}
  <!-- Plan mode pill — distinct fill (secondary accent) when active
       so the user can see at a glance that scheduling-by-drag is
       live. Forces day view when toggled on (the side-rail layout
       collapses week-views below useful width). -->
  <!-- Find time — surfaces the first N free slots that fit a
       chosen duration. Composes with the active filters since it
       consumes the same filtered events list. 'f' shortcut. -->
  <button
    onclick={onFindTime}
    class="px-2.5 py-1.5 text-xs sm:text-sm rounded border bg-surface0 border-surface1 text-subtext hover:border-primary flex items-center gap-1"
    title="Find a free slot (f)"
    aria-label="find free time"
  >
    <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
      <circle cx="12" cy="12" r="9"/>
      <path d="M12 7v5l3 2"/>
    </svg>
    <span class="hidden sm:inline">Find</span>
  </button>
  <button
    onclick={onTogglePlanMode}
    class="px-2.5 py-1.5 text-xs sm:text-sm rounded border flex items-center gap-1 transition-colors
      {planMode
        ? 'bg-secondary text-mantle border-secondary hover:opacity-90'
        : 'bg-surface0 border-surface1 text-subtext hover:border-secondary'}"
    title={planMode ? 'Exit Plan mode' : 'Enter Plan mode (drag tasks onto the grid)'}
  >
    <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
      <path d="M3 6h18M3 12h12M3 18h6"/>
    </svg>
    Plan
  </button>
  <!-- Calendar Agent (Plan my week, Find free time, Day insight,
       Dashboard) lives in the chat sidebar — Run agent on
       /calendar. Header stays minimal: nav + view switcher. -->
  <!-- Month-only density toggle. Compact = 6 chips/cell with tiny
       text — useful on busy months / smaller screens; Comfy = 3
       chips/cell with bigger text — better at-a-glance reading on
       lighter months. Persisted per-device. -->
  {#if view === 'month'}
    <!-- Density toggle was desktop-only (hidden md:flex). Mobile
         users on a busy month benefit from compact even more than
         desktop ones — show it on phones too. -->
    <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-[11px]" title="Month grid density">
      <button
        class="px-2 py-1.5 {monthDensity === 'comfy' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
        onclick={() => (monthDensity = 'comfy')}
        aria-pressed={monthDensity === 'comfy'}
      >Comfy</button>
      <button
        class="px-2 py-1.5 {monthDensity === 'compact' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
        onclick={() => (monthDensity = 'compact')}
        aria-pressed={monthDensity === 'compact'}
      >Compact</button>
    </div>
  {/if}
  <!-- Time-grid density — compact / normal / spacious, applies to
       Day / Week / Workweek views. Maps to per-hour pixel height
       in HourGrid (32/48/72). On a typical laptop, compact fits
       ~14 hours without scroll; spacious is two-clicks of vertical
       space for meeting-heavy days where the user wants room to
       read every title on a 15-min slot. -->
  {#if view === 'day' || view === 'week' || view === 'workweek'}
    <div class="hidden sm:flex bg-surface0 border border-surface1 rounded overflow-hidden text-[11px]" title="Time-grid density (per-hour height)">
      <button
        class="px-2 py-1.5 {hourDensity === 'compact' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
        onclick={() => (hourDensity = 'compact')}
        aria-pressed={hourDensity === 'compact'}
        title="32px / hour — sees the most at once, tight"
      >Compact</button>
      <button
        class="px-2 py-1.5 {hourDensity === 'normal' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
        onclick={() => (hourDensity = 'normal')}
        aria-pressed={hourDensity === 'normal'}
        title="48px / hour — historical default"
      >Normal</button>
      <button
        class="px-2 py-1.5 {hourDensity === 'spacious' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
        onclick={() => (hourDensity = 'spacious')}
        aria-pressed={hourDensity === 'spacious'}
        title="72px / hour — room to read every event on a 15-min slot"
      >Spacious</button>
    </div>
  {/if}
  <!-- View switcher — Apple Calendar set: Day / Week / Month / Year.
       3-day and Agenda were retired alongside the AI toolbar
       cleanup; Agenda's content lives in /tasks + the chat
       sidebar's "what's coming up" prompts. -->
  <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-xs sm:text-sm">
    <button
      class="px-2 sm:px-3 py-1.5 {view === 'day' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
      onclick={() => (view = 'day')}
    >Day</button>
    <button
      class="px-2 sm:px-3 py-1.5 {view === 'workweek' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'} hidden sm:inline-block"
      onclick={() => (view = 'workweek')}
      title="Mon–Fri only (Shift+W)"
    >5d</button>
    <button
      class="px-2 sm:px-3 py-1.5 {view === 'week' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
      onclick={() => (view = 'week')}
    >Week</button>
    <button
      class="px-2 sm:px-3 py-1.5 {view === 'month' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
      onclick={() => (view = 'month')}
    >Month</button>
    <button
      class="px-2 sm:px-3 py-1.5 {view === 'year' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'} hidden sm:inline-block"
      onclick={() => (view = 'year')}
    >Year</button>
    <button
      class="px-2 sm:px-3 py-1.5 {view === 'agenda' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
      onclick={() => (view = 'agenda')}
      title="Flat 30-day list (great on mobile)"
    >Agenda</button>
  </div>
  <button
    onclick={onShowShortcuts}
    aria-label="keyboard shortcuts"
    title="Keyboard shortcuts (?)"
    class="hidden md:flex w-8 h-8 items-center justify-center text-dim hover:text-text hover:bg-surface0 rounded text-xs font-mono"
  >?</button>
</header>
