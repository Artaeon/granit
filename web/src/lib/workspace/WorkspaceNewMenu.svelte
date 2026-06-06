<!--
  WorkspaceNewMenu — the popover that opens when the user taps the
  "+" at the end of the WorkspacePills row. Replaces the silent
  blank-create with a tiny picker:

    • Blank             — single Tasks pane (the historical default
                          of just clicking +)
    • <preset name>     — each WORKSPACE_PRESETS entry. The user gets
                          a starter layout in one tap; previously
                          presets were ⌘K-only and mobile users
                          never discovered them.

  All entries close the popover on click. Closes on outside click +
  Escape so it never sticks open after a switch.
-->
<script lang="ts">
  import { WORKSPACE_PRESETS } from './workspacePresets';
  import type { WorkspaceStoreController } from './workspaceStore.svelte';

  type Props = {
    store: WorkspaceStoreController;
    /** Called right after a successful creation so the embedder can
     *  navigate to /workspace (the StatusBar uses this). */
    onCreated?: () => void;
  };

  let { store, onCreated }: Props = $props();

  let open = $state(false);
  let rootEl: HTMLElement | null = $state(null);

  function pickBlank() {
    open = false;
    store.create();
    onCreated?.();
  }

  function pickPreset(id: string) {
    open = false;
    const preset = WORKSPACE_PRESETS.find((p) => p.id === id);
    if (!preset) return;
    store.createWithLayout(preset.name, preset.buildLayout());
    onCreated?.();
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
    title="New workspace — pick a starter layout"
    aria-label="New workspace"
    class="px-2 h-full text-xs text-dim hover:text-primary hover:bg-surface0 border-l border-surface1 flex-shrink-0"
  >+</button>
  {#if open}
    <!-- Pops UP from the StatusBar (which lives at the bottom of the
         viewport) so it doesn't get clipped by the bottom edge. The
         right-0 anchor keeps it from overflowing on narrow viewports
         where the pills row scrolls horizontally. -->
    <div
      role="menu"
      aria-label="New workspace"
      class="absolute right-0 bottom-full mb-1 w-56 bg-mantle border border-surface1 rounded-lg shadow-xl py-1 text-sm z-30"
    >
      <button
        type="button"
        role="menuitem"
        onclick={pickBlank}
        class="w-full text-left px-3 py-1.5 hover:bg-surface0 transition-colors text-text"
      >
        <div class="font-medium">Blank</div>
        <div class="text-[11px] text-dim">Single Tasks pane</div>
      </button>
      <div class="border-t border-surface1 my-1"></div>
      <div class="px-3 pb-1 text-[10px] uppercase tracking-wider text-dim font-mono">Presets</div>
      {#each WORKSPACE_PRESETS as p (p.id)}
        {#if p.id !== 'blank'}
          <button
            type="button"
            role="menuitem"
            onclick={() => pickPreset(p.id)}
            class="w-full text-left px-3 py-1.5 hover:bg-surface0 transition-colors text-text"
          >
            <div class="font-medium">{p.name}</div>
            <div class="text-[11px] text-dim">{p.detail}</div>
          </button>
        {/if}
      {/each}
    </div>
  {/if}
</span>
