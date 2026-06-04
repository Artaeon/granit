<!--
  WorkspaceTray — the named-workspace switcher. A pill row of every
  saved workspace plus a "+" affordance for creating new ones. The
  active workspace's chip is primary-tinted; non-active chips are
  surface0 with a hover hint.

  Renaming: double-click a chip to flip it into an inline input. Enter
  commits, Escape reverts. Inline rather than a modal because the
  workspace list is small and a modal would hide the rest of the tray.

  Deleting: × on the active chip when more than one workspace exists.
  Confirm via a single native confirm() — workspace deletion isn't a
  destructive data operation (the panes the workspace points to still
  exist as routes), so a heavy modal would feel disproportionate.
-->
<script lang="ts">
  import type { WorkspaceStoreController } from './workspaceStore.svelte';

  let { store }: { store: WorkspaceStoreController } = $props();

  // Inline-rename state. Tracking the editing-id rather than a
  // boolean lets us only show one input at a time even if the user
  // double-clicks a second chip.
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

  function confirmRemove(id: string, name: string) {
    if (typeof window === 'undefined') return;
    if (window.confirm(`Delete workspace "${name}"?`)) {
      store.remove(id);
    }
  }
</script>

<div
  class="flex items-center gap-1.5 px-2 py-1.5 border-b border-surface1 bg-mantle overflow-x-auto flex-shrink-0 scrollbar-thin"
  role="toolbar"
  aria-label="Workspaces"
>
  <span class="text-dim font-mono uppercase tracking-wider text-[10px]">workspaces</span>
  {#each store.workspaces as w (w.id)}
    {@const active = store.activeId === w.id}
    {#if editingId === w.id}
      <input
        type="text"
        bind:value={editingText}
        onkeydown={onEditKey}
        onblur={commitEdit}
        class="px-2 py-0.5 text-xs font-medium border rounded border-primary bg-surface0 text-text focus:outline-none focus:border-primary"
        size={Math.max(8, editingText.length + 1)}
        autofocus
      />
    {:else}
      <span
        class="inline-flex items-center rounded overflow-hidden border
          {active ? 'border-primary bg-primary text-on-primary' : 'border-surface1 bg-surface0 text-subtext hover:border-primary'}"
      >
        <button
          type="button"
          onclick={() => (store.activeId = w.id)}
          ondblclick={() => startEdit(w.id, w.name)}
          title="Click to activate · double-click to rename"
          class="px-2 py-0.5 text-xs font-medium whitespace-nowrap"
        >{w.name}</button>
        {#if active && store.workspaces.length > 1}
          <button
            type="button"
            onclick={() => confirmRemove(w.id, w.name)}
            title="Delete workspace"
            aria-label="Delete workspace"
            class="px-1.5 py-0.5 text-on-primary/70 hover:text-on-primary border-l border-on-primary/30 leading-none"
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
    class="px-2 py-0.5 text-xs text-dim hover:text-primary border border-dashed border-surface1 hover:border-primary rounded"
  >+ new</button>
</div>

<style>
  .scrollbar-thin::-webkit-scrollbar { height: 0; }
  .scrollbar-thin { scrollbar-width: none; }
</style>
