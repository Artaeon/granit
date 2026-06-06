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
  import {
    loadUserPresets,
    saveUserPreset,
    removeUserPreset,
    cloneWithNewIds,
    type UserPreset
  } from './userPresets';
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
  // User presets are loaded on every open so a save inside one
  // session reflects without a refresh. The list stays in component
  // state so removals are reactive.
  let userPresets = $state<UserPreset[]>([]);
  // Inline rename input for "Save current layout as preset". When
  // open, the bottom row swaps from a button to an input + check.
  // The window.prompt() it replaces was jarring and broke flow on
  // mobile (system dialog covered the menu).
  let savingPreset = $state(false);
  let savingName = $state('');
  let savingInputEl: HTMLInputElement | null = $state(null);

  function refreshUserPresets() {
    userPresets = loadUserPresets();
  }

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

  function pickUserPreset(id: string) {
    open = false;
    const preset = userPresets.find((p) => p.id === id);
    if (!preset) return;
    // Clone the saved layout with fresh IDs so re-applying the same
    // preset twice doesn't leak duplicate node IDs into the new
    // workspace.
    store.createWithLayout(preset.name, cloneWithNewIds(preset.layout));
    onCreated?.();
  }

  function startSaveCurrent() {
    savingPreset = true;
    savingName = store.active.name;
    // Focus the input on the next tick so the autofocus attribute
    // takes effect after Svelte mounts the new node.
    queueMicrotask(() => savingInputEl?.focus());
  }
  function commitSaveCurrent() {
    const name = savingName.trim();
    if (!name) {
      cancelSaveCurrent();
      return;
    }
    saveUserPreset(name, store.active.layout);
    savingPreset = false;
    savingName = '';
    refreshUserPresets();
  }
  function cancelSaveCurrent() {
    savingPreset = false;
    savingName = '';
  }

  function removePreset(e: MouseEvent, id: string) {
    e.stopPropagation();
    if (typeof window === 'undefined') return;
    const target = userPresets.find((p) => p.id === id);
    if (!target) return;
    if (!window.confirm(`Delete preset "${target.name}"?`)) return;
    removeUserPreset(id);
    refreshUserPresets();
  }

  $effect(() => {
    if (!open) return;
    // Refresh user presets each time the menu opens so saves from
    // earlier in the session reflect without a page refresh.
    refreshUserPresets();
    // Reset the inline-save row so it doesn't stay open across an
    // outside-click + reopen cycle.
    savingPreset = false;
    savingName = '';
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
      {#if userPresets.length > 0}
        <div class="border-t border-surface1 my-1"></div>
        <div class="px-3 pb-1 text-[10px] uppercase tracking-wider text-dim font-mono">Your presets</div>
        {#each userPresets as p (p.id)}
          <div class="group flex items-center hover:bg-surface0 transition-colors">
            <button
              type="button"
              role="menuitem"
              onclick={() => pickUserPreset(p.id)}
              class="flex-1 text-left px-3 py-1.5 text-text"
            >
              <div class="font-medium truncate">{p.name}</div>
              <div class="text-[11px] text-dim">custom layout</div>
            </button>
            <button
              type="button"
              onclick={(e) => removePreset(e, p.id)}
              title="Delete this preset"
              aria-label={`Delete preset ${p.name}`}
              class="px-2 py-1 text-dim hover:text-error opacity-0 group-hover:opacity-100 transition-opacity"
            >×</button>
          </div>
        {/each}
      {/if}
      <div class="border-t border-surface1 my-1"></div>
      {#if savingPreset}
        <!-- Inline rename row — replaces the window.prompt() that
             used to fire on save. Enter commits, Escape cancels,
             blur commits (so tapping outside the input still saves).
             Pre-filled with the active workspace's name so the user
             only has to confirm or tweak. -->
        <div class="flex items-center gap-1 px-2 py-1.5">
          <input
            bind:this={savingInputEl}
            bind:value={savingName}
            onkeydown={(e) => {
              if (e.key === 'Enter') { e.preventDefault(); commitSaveCurrent(); }
              else if (e.key === 'Escape') { e.preventDefault(); cancelSaveCurrent(); }
            }}
            onblur={commitSaveCurrent}
            type="text"
            aria-label="Preset name"
            placeholder="Preset name"
            class="flex-1 min-w-0 px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
          />
          <button
            type="button"
            onclick={cancelSaveCurrent}
            aria-label="Cancel"
            title="Cancel"
            class="px-1.5 py-1 text-dim hover:text-text"
          >×</button>
        </div>
      {:else}
        <button
          type="button"
          onclick={startSaveCurrent}
          class="w-full text-left px-3 py-1.5 text-[11px] text-dim hover:text-primary hover:bg-surface0 transition-colors"
          title="Save the current workspace layout as a reusable preset"
        >+ Save current layout as preset</button>
      {/if}
    </div>
  {/if}
</span>
