<!--
  AddPaneMenu — tiny popover used by the mobile PaneSlot's "+ pane"
  button. Mobile shows one leaf at a time, so when the user adds a
  pane they should see the menu of pane types and PICK what they
  actually want rather than getting a silent default.

  Pure presentation. The parent (PaneSlot) decides what splitting
  the new pane means (always horizontal on mobile, since axis is
  invisible there); this component just routes the user's pick back
  via onPick.
-->
<script lang="ts">
  import { PANES, type PaneKind } from './paneRegistry';
  import NavIcon from '$lib/components/NavIcon.svelte';

  type Props = {
    /** Don't show the pane kind that's already in the parent slot —
     *  that's just a no-op visually. */
    excludePane: PaneKind;
    onPick: (pane: PaneKind) => void;
  };

  let { excludePane, onPick }: Props = $props();

  let open = $state(false);
  let rootEl: HTMLElement | null = $state(null);

  function pick(p: PaneKind) {
    open = false;
    onPick(p);
  }

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

<span bind:this={rootEl} class="relative inline-block">
  <button
    type="button"
    onclick={() => (open = !open)}
    aria-haspopup="menu"
    aria-expanded={open}
    title="Add a pane"
    aria-label="Add a pane"
    class="tap-target px-2 py-1 text-dim hover:text-primary border border-surface1 hover:border-primary rounded text-[11px] leading-none font-medium inline-flex items-center gap-1"
  >
    <span aria-hidden="true">+</span>
    <span>pane</span>
  </button>
  {#if open}
    <!-- Right-anchored so the menu stays on-screen even when the
         button sits at the right edge of a narrow slot header. -->
    <div
      role="menu"
      aria-label="Add a pane"
      class="absolute right-0 top-full mt-1 w-44 bg-mantle border border-surface1 rounded-lg shadow-xl py-1 text-sm z-20"
    >
      {#each PANES as p (p.id)}
        {#if p.id !== excludePane}
          <button
            type="button"
            role="menuitem"
            onclick={() => pick(p.id)}
            class="w-full inline-flex items-center gap-2 text-left px-3 py-1.5 hover:bg-surface0 transition-colors text-text"
          >
            <NavIcon name={p.icon} class="w-4 h-4 flex-shrink-0 text-dim" />
            <span>{p.label}</span>
          </button>
        {/if}
      {/each}
    </div>
  {/if}
</span>
