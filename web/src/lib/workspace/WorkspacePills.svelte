<!--
  WorkspacePills — the single workspace switcher used by the
  StatusBar and (previously) the /workspace route's standalone
  tray. One component, one source of truth: switch, rename,
  delete, create.

  Behaviour mirrors the old WorkspaceTray's affordances:
    • click   → switch active workspace
    • dblclk  → inline rename (Enter commits, Esc reverts)
    • × on the active pill (when count > 1) → confirm delete
    • + button at the end → create a new workspace
  All driven by the same WorkspaceStoreController — the StatusBar
  passes the singleton instance so cross-surface state stays
  coherent.

  Stays tiny on purpose. No icons beyond ×/+. The owner decides
  outer chrome (the StatusBar wraps it in the 28px bar; future
  embedders pass their own padding container).
-->
<script lang="ts">
  import type { WorkspaceStoreController } from './workspaceStore.svelte';

  type Props = {
    store: WorkspaceStoreController;
    /** Whether the active workspace should render as primary-tinted.
     *  StatusBar only highlights when the user is on /workspace
     *  (otherwise the pill is just a "go to this workspace" link);
     *  a standalone tray would always highlight. Defaults to true. */
    highlightActive?: boolean;
    /** Called after a successful switch so the StatusBar can
     *  navigate to /workspace. Optional — embedders that already
     *  live inside /workspace pass nothing. */
    onSwitch?: (id: string) => void;
  };

  let { store, highlightActive = true, onSwitch }: Props = $props();

  let editingId = $state<string | null>(null);
  let editingText = $state('');

  function startEdit(id: string, current: string) {
    editingId = id;
    editingText = current;
  }
  function commitEdit() {
    if (editingId) store.rename(editingId, editingText);
    editingId = null;
    editingText = '';
  }
  function cancelEdit() {
    editingId = null;
    editingText = '';
  }
  function onEditKey(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault();
      commitEdit();
    } else if (e.key === 'Escape') {
      e.preventDefault();
      cancelEdit();
    }
  }

  function switchTo(id: string) {
    store.activeId = id;
    onSwitch?.(id);
  }

  function confirmRemove(id: string, name: string) {
    if (typeof window === 'undefined') return;
    if (window.confirm(`Delete workspace "${name}"?`)) {
      store.remove(id);
    }
  }
</script>

{#each store.workspaces as w (w.id)}
  {@const active = highlightActive && store.activeId === w.id}
  {#if editingId === w.id}
    <input
      type="text"
      bind:value={editingText}
      onkeydown={onEditKey}
      onblur={commitEdit}
      class="px-2 h-full text-xs font-medium border border-primary bg-surface0 text-text focus:outline-none flex-shrink-0"
      size={Math.max(8, editingText.length + 1)}
      autofocus
    />
  {:else}
    <span
      class="inline-flex items-stretch border-l border-surface1 transition-colors whitespace-nowrap
        {active ? 'bg-primary text-on-primary' : 'text-subtext hover:text-text hover:bg-surface0'}"
    >
      <button
        type="button"
        onclick={() => switchTo(w.id)}
        ondblclick={() => startEdit(w.id, w.name)}
        title={`Switch to "${w.name}" · double-click to rename`}
        class="px-2 h-full text-[11px] md:text-xs font-medium"
      >
        <span class="truncate max-w-[8rem] inline-block align-middle">{w.name}</span>
      </button>
      {#if active && store.workspaces.length > 1}
        <button
          type="button"
          onclick={() => confirmRemove(w.id, w.name)}
          title="Delete workspace"
          aria-label="Delete workspace"
          class="px-1.5 h-full text-on-primary/70 hover:text-on-primary border-l border-on-primary/30 leading-none text-xs"
        >×</button>
      {/if}
    </span>
  {/if}
{/each}
<button
  type="button"
  onclick={() => store.create()}
  title="New workspace"
  aria-label="New workspace"
  class="px-2 h-full text-xs text-dim hover:text-primary hover:bg-surface0 border-l border-surface1 flex-shrink-0"
>+</button>
