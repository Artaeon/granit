<script lang="ts">
  // Modes + Personas dropdown for the AIOverlay header. Split out of
  // AIOverlay.svelte because it was a ~130-LOC popover block with its
  // own click-outside scrim, three list sections (generic / contextual
  // / personas), and per-section "is this in scope?" hinting. Logic
  // here stays pure-presentational: read the catalogue from
  // $lib/ai/agents, render the current selection, fire onSelect.
  //
  // The host AIOverlay owns the visibility flag (so Esc-from-anywhere
  // can dismiss it via the same Esc handler that closes the mention /
  // slash pickers) and the `selectMode` callback (so persisting the
  // mode + announcing it + clearing autoMode stays in one place). All
  // we get is the modeId + the scope-detection flags + the close
  // signal.

  import { GENERIC_MODES, CONTEXTUAL_MODES, PERSONAS } from '$lib/ai/agents';

  let {
    modeId,
    currentProjectName,
    currentGoalId,
    onCalendarPage,
    onSelect,
    onDismiss
  }: {
    modeId: string;
    currentProjectName: string;
    currentGoalId: string;
    onCalendarPage: boolean;
    onSelect: (id: string) => void;
    onDismiss: () => void;
  } = $props();
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div
  role="presentation"
  class="fixed inset-0 z-40"
  onclick={onDismiss}
></div>
<div
  role="listbox"
  class="absolute left-0 top-full mt-1 w-[min(18rem,calc(100vw-1rem))] bg-mantle border border-surface1 rounded-lg shadow-xl z-50 py-1 max-h-[70dvh] overflow-y-auto"
>
  <!-- Generic modes group. The "modes" header is implicit (the picker
       opens with them; no need to label what the user is already
       looking at). The "personas" header below makes the second group
       obvious. -->
  <div class="px-3 pt-2 pb-1 text-[10px] font-semibold uppercase tracking-[0.14em] text-dim">Modes</div>
  {#each GENERIC_MODES as m (m.id)}
    <button
      type="button"
      role="option"
      aria-selected={m.id === modeId}
      onclick={() => { onSelect(m.id); onDismiss(); }}
      class="w-full flex items-center gap-2.5 px-3 py-2 hover:bg-surface0 text-left transition-colors {m.id === modeId ? 'bg-surface1' : ''}"
    >
      <span class="text-[11px] font-semibold tracking-tight leading-none flex-shrink-0 inline-flex items-center justify-center w-7 h-7 rounded-md bg-surface1 text-subtext">{m.glyph}</span>
      <div class="flex-1 min-w-0">
        <div class="text-[13px] font-medium text-text leading-tight">{m.label}</div>
        <div class="text-[11px] text-dim leading-snug mt-0.5">{m.tagline}</div>
      </div>
      {#if m.id === modeId}
        <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 text-primary flex-shrink-0" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="20 6 9 17 4 12"/>
        </svg>
      {/if}
    </button>
  {/each}
  {#if CONTEXTUAL_MODES.length > 0}
    <!-- Contextual modes — page-aware. They auto-switch when the user
         is on a matching URL (project / goal / calendar) and revert
         when the user leaves. Visually distinguished with a primary-
         tinted glyph background so the user reads them as "tied to a
         page", not generic postures. Out-of-scope modes are dimmed +
         carry a "needs <X>" hint so the user knows the prelude won't
         carry context — they can still pick (sometimes useful for the
         system-prompt posture alone), it just won't be the full PM /
         Goal / Calendar manager experience. -->
    <div class="border-t border-surface1 mt-1"></div>
    <div class="px-3 pt-2 pb-1 text-[10px] font-semibold uppercase tracking-[0.14em] text-primary">Contextual</div>
    {#each CONTEXTUAL_MODES as m (m.id)}
      {@const inScope =
        (m.id === 'project-manager' && !!currentProjectName) ||
        (m.id === 'goal-manager' && !!currentGoalId) ||
        (m.id === 'calendar-manager' && onCalendarPage)}
      {@const scopeHint =
        m.id === 'project-manager'
          ? 'open a project'
          : m.id === 'goal-manager'
          ? 'focus a goal'
          : 'open the calendar'}
      <button
        type="button"
        role="option"
        aria-selected={m.id === modeId}
        onclick={() => { onSelect(m.id); onDismiss(); }}
        class="w-full flex items-center gap-2.5 px-3 py-2 hover:bg-surface0 text-left transition-colors {m.id === modeId ? 'bg-surface1' : ''} {inScope ? '' : 'text-dim'}"
        title={inScope
          ? m.tagline
          : `Pick-able from any page, but the prelude won't carry context until you ${scopeHint}.`}
      >
        <span class="text-[11px] font-semibold tracking-tight leading-none flex-shrink-0 inline-flex items-center justify-center w-7 h-7 rounded-md bg-primary text-on-primary">{m.glyph}</span>
        <div class="flex-1 min-w-0">
          <div class="text-[13px] font-medium text-text leading-tight inline-flex items-center gap-1.5">
            {m.label}
            {#if !inScope}
              <span class="text-[9px] uppercase tracking-wider text-dim font-normal bg-surface1 px-1 py-0.5 rounded">needs {scopeHint}</span>
            {/if}
          </div>
          <div class="text-[11px] text-dim leading-snug mt-0.5">{m.tagline}</div>
        </div>
        {#if m.id === modeId}
          <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 text-primary flex-shrink-0" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="20 6 9 17 4 12"/>
          </svg>
        {/if}
      </button>
    {/each}
  {/if}
  {#if PERSONAS.length > 0}
    <!-- Personas group — sharper, named voices. Visually distinguished
         by a divider, a section header, an accent-coloured glyph
         background, and an italic tagline so the user reads "this is a
         character, not a generic posture" at a glance. -->
    <div class="border-t border-surface1 mt-1"></div>
    <div class="px-3 pt-2 pb-1 text-[10px] font-semibold uppercase tracking-[0.14em] text-secondary">Personas</div>
    {#each PERSONAS as m (m.id)}
      <button
        type="button"
        role="option"
        aria-selected={m.id === modeId}
        onclick={() => { onSelect(m.id); onDismiss(); }}
        class="w-full flex items-center gap-2.5 px-3 py-2 hover:bg-surface0 text-left transition-colors {m.id === modeId ? 'bg-surface1' : ''}"
      >
        <span class="text-[11px] font-semibold tracking-tight leading-none flex-shrink-0 inline-flex items-center justify-center w-7 h-7 rounded-md bg-secondary text-on-primary">{m.glyph}</span>
        <div class="flex-1 min-w-0">
          <div class="text-[13px] font-medium text-text leading-tight">{m.label}</div>
          <div class="text-[11px] text-dim leading-snug italic mt-0.5">{m.tagline}</div>
        </div>
        {#if m.id === modeId}
          <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 text-primary flex-shrink-0" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="20 6 9 17 4 12"/>
          </svg>
        {/if}
      </button>
    {/each}
  {/if}
</div>
