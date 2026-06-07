<script lang="ts">
  // Slim page header for /goals. Mirrors TasksPageHeader: single dense
  // 44px bar with title + count + icon-segmented view picker + a
  // "More" dropdown for the AI surfaces (check-in / audit) + primary
  // "+ New goal" button. Replaces the old two-row chrome (PageHeader
  // strip with three pill buttons + a separate view-mode toolbar).
  //
  // The visual hierarchy lives on the goal cards and section banners
  // below — the chrome itself is muted so it doesn't compete with the
  // work surface.
  import { focusOnMount } from '$lib/util/focusOnMount';
  import Button from '$lib/components/Button.svelte';

  type ViewMode = 'cards' | 'list' | 'kanban';

  type Props = {
    view: ViewMode;
    totalCount: number;
    filteredCount: number;
    checkinOpen: boolean;
    checkinBusy: boolean;
    auditOpen: boolean;
    auditBusy: boolean;
    moreOpen: boolean;
    onSelectView: (v: ViewMode) => void;
    onToggleMore: () => void;
    onToggleCheckin: () => void;
    onToggleAudit: () => void;
    onCreate: () => void;
  };

  let {
    view,
    totalCount,
    filteredCount,
    checkinOpen,
    checkinBusy,
    auditOpen,
    auditBusy,
    moreOpen,
    onSelectView,
    onToggleMore,
    onToggleCheckin,
    onToggleAudit,
    onCreate
  }: Props = $props();

  // Primary icon-segmented view picker. Cards / List / Kanban — same
  // three modes the user has always had, now communicated by glyph
  // instead of "Cards | List | Kanban" text buttons.
  const VIEWS: { key: ViewMode; label: string; title: string; icon: string }[] = [
    {
      key: 'cards',
      label: 'Cards',
      title: 'rich card layout',
      // Two stacked rectangles = cards stack.
      icon: 'M3 5h18v6H3z M3 13h18v6H3z'
    },
    {
      key: 'list',
      label: 'List',
      title: 'compact list — denser, one row per goal',
      // Three horizontal lines = list rows.
      icon: 'M4 6h16 M4 12h16 M4 18h16'
    },
    {
      key: 'kanban',
      label: 'Kanban',
      title: 'columns by status',
      // Three vertical columns of varying height.
      icon: 'M4 5v14 M12 5v10 M20 5v8'
    }
  ];
</script>

<div class="flex items-center gap-2 px-3 py-2 border-b border-surface1 flex-shrink-0 bg-mantle">
  <h1 class="text-base sm:text-lg font-semibold text-text leading-none">Goals</h1>

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

  <!-- Icon-segmented view picker (desktop). -->
  <div class="hidden sm:flex bg-surface0 border border-surface1 rounded overflow-hidden">
    {#each VIEWS as v (v.key)}
      <Button
        variant="ghost"
        active={view === v.key}
        onclick={() => onSelectView(v.key)}
        title={v.title}
        aria-label={v.label}
        aria-pressed={view === v.key}
      >
        <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d={v.icon} />
        </svg>
        <span class="hidden md:inline">{v.label}</span>
      </Button>
    {/each}
  </div>

  <!-- Mobile: collapse to a labelled select instead of a row of icons. -->
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

  <!-- More-dropdown for the AI coaching surfaces. The check-in /
       audit buttons used to sit in the header strip permanently —
       they get used weekly, not hourly, so they belong behind a
       fold. Active state (panel open) reflects in the More label
       tint so the user can see something is on without opening the
       menu. -->
  <div class="relative" data-goals-more>
    <Button
      variant="secondary"
      active={checkinOpen || auditOpen}
      aria-haspopup="true"
      aria-expanded={moreOpen}
      onclick={onToggleMore}
      title="AI coaching surfaces"
    >
      <span class="hidden sm:inline">More</span>
      <span class="sm:hidden" aria-hidden="true">⋯</span>
      <span class="text-[9px] opacity-70" aria-hidden="true">▾</span>
    </Button>
    {#if moreOpen}
      <div
        role="menu"
        class="absolute right-0 top-full mt-1 z-30 min-w-[14rem] bg-surface0 border border-surface1 rounded shadow-lg py-1 text-xs"
        use:focusOnMount
        tabindex="-1"
      >
        <button
          type="button"
          role="menuitem"
          onclick={onToggleCheckin}
          disabled={checkinBusy}
          class="w-full text-left px-3 py-1.5 inline-flex items-center justify-between gap-3 {checkinOpen ? 'bg-surface1 text-primary' : 'text-subtext hover:bg-surface1 hover:text-text'} disabled:opacity-50"
          title="Honest one-line verdict + a sharp question for each active goal"
        >
          <span>{checkinOpen ? 'Close weekly check-in' : 'Weekly check-in'}</span>
          <span class="text-[10px] text-dim">AI</span>
        </button>
        <button
          type="button"
          role="menuitem"
          onclick={onToggleAudit}
          disabled={auditBusy}
          class="w-full text-left px-3 py-1.5 inline-flex items-center justify-between gap-3 {auditOpen ? 'bg-surface1 text-warning' : 'text-subtext hover:bg-surface1 hover:text-text'} disabled:opacity-50"
          title="Surface tasks that don't advance any stated goal"
        >
          <span>{auditOpen ? 'Close alignment audit' : 'Alignment audit'}</span>
          <span class="text-[10px] text-dim">AI</span>
        </button>
      </div>
    {/if}
  </div>

  <!-- Primary create button. Always visible — creating a goal is the
       core write action of this page. -->
  <Button
    variant="primary"
    onclick={onCreate}
    aria-label="New goal"
    title="Create a new goal"
  >
    <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
      <path d="M12 5v14M5 12h14"/>
    </svg>
    <span class="hidden md:inline">New goal</span>
  </Button>
</div>
