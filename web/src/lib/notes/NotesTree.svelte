<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, type Note } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { buildTree, filterTree, ancestorFolders, type TreeNode } from './treeUtils';
  import TreeNodeView from './TreeNode.svelte';

  let {
    currentPath,
    onSelect,
    autoLoad = true
  }: {
    currentPath?: string;
    onSelect?: (path: string) => void;
    autoLoad?: boolean;
  } = $props();

  let notes = $state<Note[]>([]);
  let loading = $state(false);
  let q = $state('');
  let expanded = $state<Record<string, boolean>>({});

  async function load() {
    if (!$auth) return;
    loading = true;
    try {
      const list = await api.listNotes({ limit: 5000 });
      notes = list.notes;
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    if (autoLoad) load();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') load();
    });
  });

  // Auto-expand ancestors of current path. We read `expanded` inside
  // untrack() so the assignment below doesn't re-trigger this same effect
  // (spreading creates a new object reference, which would otherwise loop).
  $effect(() => {
    if (!currentPath) return;
    const ancestors = ancestorFolders(currentPath);
    untrack(() => {
      let changed = false;
      const next = { ...expanded };
      for (const a of ancestors) {
        if (!next[a]) { next[a] = true; changed = true; }
      }
      if (changed) expanded = next;
    });
  });

  let tree = $derived.by(() => buildTree(notes));
  let filtered = $derived.by(() => (q.trim() ? filterTree(tree, q.trim()) : tree));

  // While searching, auto-expand all matching folders. Same untrack pattern.
  $effect(() => {
    if (!q.trim() || !filtered) return;
    untrack(() => {
      const next: Record<string, boolean> = { ...expanded };
      let changed = false;
      const visit = (n: TreeNode) => {
        if (n.isFolder) {
          if (!next[n.path]) { next[n.path] = true; changed = true; }
          n.children?.forEach(visit);
        }
      };
      visit(filtered);
      if (changed) expanded = next;
    });
  });

  function toggle(path: string) {
    expanded = { ...expanded, [path]: !expanded[path] };
  }

  function expandAll() {
    const next: Record<string, boolean> = {};
    const visit = (n: TreeNode) => {
      if (n.isFolder) {
        next[n.path] = true;
        n.children?.forEach(visit);
      }
    };
    visit(tree);
    expanded = next;
  }
  function collapseAll() {
    expanded = {};
  }

  export function reload() { return load(); }
</script>

<div class="flex flex-col h-full min-h-0">
  <div class="px-2 py-2 flex-shrink-0 space-y-2">
    <input
      bind:value={q}
      placeholder="filter notes…"
      class="w-full px-2 py-1.5 bg-surface0 border border-surface1 rounded text-base sm:text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
    />
    <div class="flex items-center justify-between text-xs">
      <span class="text-dim">{notes.length} notes</span>
      <span class="space-x-2">
        <button onclick={expandAll} class="text-dim hover:text-text">expand</button>
        <button onclick={collapseAll} class="text-dim hover:text-text">collapse</button>
      </span>
    </div>
  </div>

  <div class="flex-1 min-h-0 overflow-y-auto px-1 pb-3">
    {#if loading && notes.length === 0}
      <div class="px-3 py-2 text-sm text-dim">loading…</div>
    {:else if filtered === null}
      <div class="px-3 py-2 text-sm text-dim italic">no matches</div>
    {:else}
      {#each filtered.children ?? [] as child (child.path)}
        <TreeNodeView node={child} {expanded} {currentPath} onToggle={toggle} {onSelect} />
      {/each}
    {/if}
  </div>
</div>
