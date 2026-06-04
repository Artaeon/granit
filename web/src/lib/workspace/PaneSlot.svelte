<!--
  PaneSlot — a single tileable cell in the workspace shell. Wraps a
  pane component with a tiny header carrying:

    - the pane-type picker (Tasks / Calendar / Goals)
    - a "close" button when the slot is closable (the workspace shell
      hides this for the last open slot so the user can't end up with
      an empty workspace)

  Stays presentational. Layout, resize, persistence — all owned by
  the parent SplitContainer / workspace route. This component just
  renders one pane and lets the user swap which one.
-->
<script lang="ts">
  import { PANES, findPane, type PaneKind } from './paneRegistry';

  let {
    pane,
    onChange,
    closable = false,
    onClose
  }: {
    pane: PaneKind;
    onChange: (next: PaneKind) => void;
    closable?: boolean;
    onClose?: () => void;
  } = $props();

  let entry = $derived(findPane(pane));
</script>

<div class="flex flex-col h-full min-h-0 border border-surface1 rounded overflow-hidden">
  <header
    class="flex items-center gap-2 px-2 py-1 border-b border-surface1 bg-surface0 text-xs flex-shrink-0"
  >
    <span class="text-dim uppercase tracking-wider text-[10px]">pane</span>
    <!-- Pane-type picker. Single-select pill row so the user can
         see every available pane type at a glance and swap with
         one click — no dropdown, no submenu. -->
    {#each PANES as p (p.id)}
      <button
        type="button"
        onclick={() => onChange(p.id)}
        aria-pressed={pane === p.id}
        class="px-2 py-0.5 rounded text-xs font-medium border transition-colors {pane === p.id
          ? 'bg-primary text-on-primary border-primary'
          : 'bg-surface0 text-subtext border-surface1 hover:border-primary hover:text-text'}"
      >
        {p.label}
      </button>
    {/each}
    {#if closable && onClose}
      <button
        type="button"
        onclick={onClose}
        title="Close this pane"
        aria-label="Close this pane"
        class="ml-auto text-dim hover:text-error text-base leading-none px-1"
      >×</button>
    {/if}
  </header>
  <div class="flex-1 min-h-0 overflow-auto">
    {#if entry}
      {@const PaneComponent = entry.component}
      <PaneComponent />
    {/if}
  </div>
</div>
