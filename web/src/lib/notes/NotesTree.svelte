<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, type Note } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { buildTree, filterTree, ancestorFolders, type TreeNode } from './treeUtils';
  import TreeNodeView from './TreeNode.svelte';
  import { pinnedNotes, unpinPath, ensurePinnedLoaded } from './pinnedNotes';

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
    ensurePinnedLoaded();
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

  // Pinned notes — surfaced in their own section above the tree.
  // The pinned set is a localStorage of paths; we resolve them
  // against `notes` so the rendering stays in sync if a note got
  // renamed or deleted (and the user can prune dangling pins).
  let pinnedRows = $derived.by(() => {
    if ($pinnedNotes.size === 0) return [];
    const byPath = new Map<string, Note>();
    for (const n of notes) byPath.set(n.path, n);
    const rows: { path: string; note: Note | null }[] = [];
    for (const p of $pinnedNotes) rows.push({ path: p, note: byPath.get(p) ?? null });
    rows.sort((a, b) => {
      const ta = a.note?.title ?? a.path;
      const tb = b.note?.title ?? b.path;
      return ta.toLowerCase().localeCompare(tb.toLowerCase());
    });
    return rows;
  });

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
      {#if pinnedRows.length > 0 && !q.trim()}
        <!-- Pinned section. Hidden while filtering so search results
             aren't competing with the pin list above. The dangling-
             pin row (note no longer exists) renders dim with an
             unpin × so the user can prune; otherwise it would just
             ghost-link to a 404. -->
        <div class="text-[10px] uppercase tracking-wider text-dim px-3 pt-1 pb-1 flex items-baseline gap-1.5">
          <svg viewBox="0 0 24 24" class="w-2.5 h-2.5" fill="currentColor" stroke="none"><path d="M12 2l2.6 6.5L22 9.3l-5.4 4.7L18 22l-6-3.5L6 22l1.4-8L2 9.3l7.4-.8L12 2z"/></svg>
          Pinned
        </div>
        {#each pinnedRows as row (row.path)}
          {#if row.note}
            <a
              href="/notes/{encodeURIComponent(row.path)}"
              onclick={() => onSelect?.(row.path)}
              class="group flex items-center gap-1.5 py-1 px-2 text-sm hover:bg-surface0 rounded
                {currentPath === row.path ? 'bg-surface1 text-primary' : 'text-text'}"
            >
              <span class="text-warning text-xs">★</span>
              <span class="flex-1 truncate">{row.note.title || row.note.path.replace(/\.md$/, '')}</span>
            </a>
          {:else}
            <div class="flex items-center gap-1.5 py-1 px-2 text-sm text-dim italic">
              <span class="text-dim">⌀</span>
              <span class="flex-1 truncate" title={row.path}>{row.path}</span>
              <button
                type="button"
                onclick={() => unpinPath(row.path)}
                title="Remove dangling pin"
                aria-label="Remove dangling pin"
                class="text-dim hover:text-error"
              >×</button>
            </div>
          {/if}
        {/each}
        <div class="border-b border-surface1 mx-2 my-2"></div>
      {/if}
      {#each filtered.children ?? [] as child (child.path)}
        <TreeNodeView node={child} {expanded} {currentPath} onToggle={toggle} {onSelect} />
      {/each}
    {/if}
  </div>
</div>
