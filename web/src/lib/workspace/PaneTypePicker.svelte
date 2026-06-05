<!--
  PaneTypePicker — single-button pane-type dropdown for a leaf
  header. Replaces the inline six-pill row PaneSlot used to render so
  each leaf shows the active pane name + a chevron, click to change.

  Stays tiny on purpose. No icons, no descriptions — just the label.
  The PANES catalog supplies the option list so adding a pane type
  is still one line in paneRegistry.ts.
-->
<script lang="ts">
  import { PANES, findPane, type PaneKind } from './paneRegistry';

  type Props = {
    pane: PaneKind;
    onChange: (next: PaneKind) => void;
  };
  let { pane, onChange }: Props = $props();

  let entry = $derived(findPane(pane));
  let open = $state(false);

  function pick(p: PaneKind) {
    open = false;
    if (p !== pane) onChange(p);
  }

  // Close on outside click + Escape so the menu doesn't linger.
  let rootEl: HTMLElement | null = $state(null);
  $effect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (rootEl && !rootEl.contains(e.target as Node)) open = false;
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') open = false;
    };
    window.addEventListener('mousedown', onDown);
    window.addEventListener('keydown', onKey);
    return () => {
      window.removeEventListener('mousedown', onDown);
      window.removeEventListener('keydown', onKey);
    };
  });
</script>

<div bind:this={rootEl} class="relative">
  <button
    type="button"
    onclick={() => (open = !open)}
    aria-haspopup="menu"
    aria-expanded={open}
    title="Change pane type"
    class="inline-flex items-center gap-1 px-2 py-0.5 text-xs font-medium bg-surface0 text-text border border-surface1 hover:border-primary rounded transition-colors"
  >
    <span class="uppercase tracking-wider">{entry?.label ?? pane}</span>
    <svg viewBox="0 0 24 24" class="w-3 h-3 text-dim" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
      <polyline points="6 9 12 15 18 9" />
    </svg>
  </button>
  {#if open}
    <div
      role="menu"
      class="absolute left-0 top-full mt-1 w-40 bg-mantle border border-surface1 rounded-lg shadow-xl py-1 text-sm z-20"
    >
      {#each PANES as p (p.id)}
        <button
          type="button"
          role="menuitem"
          onclick={() => pick(p.id)}
          aria-current={pane === p.id ? 'true' : undefined}
          class="w-full text-left px-3 py-1.5 hover:bg-surface0 transition-colors {pane === p.id ? 'text-primary' : 'text-text'}"
        >{p.label}</button>
      {/each}
    </div>
  {/if}
</div>
