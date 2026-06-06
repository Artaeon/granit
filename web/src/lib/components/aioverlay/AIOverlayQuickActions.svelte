<!--
  AIOverlayQuickActions — the two-row action band beneath the header.

  Top row (conditional)  — page-context actions:
    • Run page agent     — opens the host page's ?agent=1 surface
                           (TaskAgent / ProjectManager / GoalAgent /
                            CalendarAgent depending on the route)
    • Context chips      — prompt presets for the current
                           project / goal / calendar scope; click
                           pre-fills the composer with the prompt.

  Bottom row (always present) — global utilities:
    • Briefing           — daily summary
    • Weekly synopsis    — week-roll-up
    • Triage             — inbox triage
    • Deadlines          — detect upcoming deadlines
    • Save / Clear       — only when there's something worth keeping
                           or clearing (a chat thread OR a quick result).

  Pure presentation. The page-aware deriveds + the action services
  live in the parent; this surfaces the buttons + routes clicks
  back via callbacks.
-->
<script lang="ts">
  import {
    projectContextChips,
    CALENDAR_CONTEXT_CHIPS,
    GOAL_CONTEXT_CHIPS
  } from '$lib/chat/contextChips';
  import type { PageAgent } from '$lib/chat/pageContext';

  type Props = {
    pageAgent: PageAgent | null;
    currentProjectName: string;
    currentGoalId: string;
    onCalendarPage: boolean;
    busy: boolean;
    sabbathActive: boolean;
    /** True when there's a chat thread OR a quick-action result on
     *  screen — controls visibility of Save / Clear. */
    hasContent: boolean;
    saving: boolean;
    onLaunchAgent: () => void;
    onPickChip: (prompt: string) => void;
    onBriefing: () => void;
    onSynopsis: () => void;
    onTriage: () => void;
    onDeadlines: () => void;
    onSaveThread: () => void;
    onClear: () => void;
  };

  let {
    pageAgent,
    currentProjectName,
    currentGoalId,
    onCalendarPage,
    busy,
    sabbathActive,
    hasContent,
    saving,
    onLaunchAgent,
    onPickChip,
    onBriefing,
    onSynopsis,
    onTriage,
    onDeadlines,
    onSaveThread,
    onClear
  }: Props = $props();

  // Top row hides itself when there's nothing context-specific to
  // surface (no page agent + no project / goal / calendar context).
  // The global-utility row below always renders.
  let hasContext = $derived(!!pageAgent || !!currentProjectName || onCalendarPage || !!currentGoalId);

  let contextLabel = $derived(
    currentProjectName ? 'Project'
    : onCalendarPage ? 'Calendar'
    : currentGoalId ? 'Goal'
    : 'Context'
  );
</script>

<div class="px-4 py-2.5 border-b border-surface1 flex-shrink-0 space-y-2">
  {#if hasContext}
    <div class="flex items-center gap-2 flex-wrap">
      <span class="text-[10px] font-semibold uppercase tracking-[0.14em] text-dim flex-shrink-0">{contextLabel}</span>
      {#if pageAgent}
        <!-- Run page-scoped Agent — replaces the per-page "Agent"
             toolbar buttons. Navigates with ?agent=1 so the host
             page hydrates and opens its own AgentDialog. -->
        <button
          onclick={onLaunchAgent}
          title="Open the agent for this page"
          class="px-2.5 py-1 min-h-[32px] text-xs bg-primary text-on-primary rounded font-medium hover:opacity-90 inline-flex items-center gap-1.5"
        >
          <span class="text-[10px] font-bold tracking-tight inline-flex items-center justify-center w-4 h-4 rounded-sm bg-mantle text-primary leading-none">{pageAgent.glyph}</span>
          <span>Run agent</span>
        </button>
      {/if}
      {#if currentProjectName}
        {#each projectContextChips(currentProjectName) as c (c.label)}
          <button
            onclick={() => onPickChip(c.prompt)}
            class="px-2.5 py-1 min-h-[32px] text-xs bg-surface0 rounded text-primary hover:bg-surface1 inline-flex items-center transition-colors"
          >{c.label}</button>
        {/each}
      {:else if onCalendarPage}
        {#each CALENDAR_CONTEXT_CHIPS as c (c.label)}
          <button
            onclick={() => onPickChip(c.prompt)}
            class="px-2.5 py-1 min-h-[32px] text-xs bg-surface0 rounded text-primary hover:bg-surface1 inline-flex items-center transition-colors"
          >{c.label}</button>
        {/each}
      {:else if currentGoalId}
        {#each GOAL_CONTEXT_CHIPS as c (c.label)}
          <button
            onclick={() => onPickChip(c.prompt)}
            class="px-2.5 py-1 min-h-[32px] text-xs bg-surface0 rounded text-primary hover:bg-surface1 inline-flex items-center transition-colors"
          >{c.label}</button>
        {/each}
      {/if}
    </div>
  {/if}
  <!-- Global utilities row — always present, separated from
       context chips above. Stays plain so context actions read
       as the headline. -->
  <div class="flex items-center gap-2 flex-wrap">
    <span class="text-[10px] font-semibold uppercase tracking-[0.14em] text-dim flex-shrink-0">Quick</span>
    <button
      onclick={onBriefing}
      disabled={busy || sabbathActive}
      class="px-2.5 py-1 min-h-[32px] text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary hover:text-text disabled:opacity-50 inline-flex items-center transition-colors"
    >Briefing</button>
    <button
      onclick={onSynopsis}
      disabled={busy || sabbathActive}
      class="px-2.5 py-1 min-h-[32px] text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary hover:text-text disabled:opacity-50 inline-flex items-center transition-colors"
    >Weekly synopsis</button>
    <button
      onclick={onTriage}
      disabled={busy || sabbathActive}
      class="px-2.5 py-1 min-h-[32px] text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary hover:text-text disabled:opacity-50 inline-flex items-center transition-colors"
    >Triage</button>
    <button
      onclick={onDeadlines}
      disabled={busy || sabbathActive}
      class="px-2.5 py-1 min-h-[32px] text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary hover:text-text disabled:opacity-50 inline-flex items-center transition-colors"
    >Deadlines</button>
    <span class="flex-1"></span>
    {#if hasContent}
      <button
        onclick={onSaveThread}
        disabled={saving}
        class="px-2 py-1 text-[11px] text-secondary hover:text-subtext hover:underline disabled:opacity-50 inline-flex items-center gap-1"
        title="Save this thread as a markdown note under chat-history/"
      >
        <svg viewBox="0 0 24 24" class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M5 4h11l3 3v13H5z"/>
          <path d="M9 4v5h6V4M8 14h8M8 18h6" stroke-linecap="round"/>
        </svg>
        {saving ? 'saving…' : 'save'}
      </button>
      <button
        onclick={onClear}
        class="px-2 py-1 text-[11px] text-dim hover:text-error transition-colors"
        title="Clear the overlay"
      >clear</button>
    {/if}
  </div>
</div>
