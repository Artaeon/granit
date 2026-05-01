<script lang="ts">
  import Self from './TreeNode.svelte';
  import type { TreeNode } from './treeUtils';

  let {
    node,
    depth = 0,
    expanded,
    currentPath,
    onToggle,
    onSelect
  }: {
    node: TreeNode;
    depth?: number;
    expanded: Record<string, boolean>;
    currentPath?: string;
    onToggle: (path: string) => void;
    onSelect?: (path: string) => void;
  } = $props();

  let isOpen = $derived(node.isFolder && (expanded[node.path] ?? false));
  let isCurrent = $derived(!node.isFolder && node.path === currentPath);
  let pad = $derived(`padding-left: ${0.5 + depth * 0.75}rem`);
</script>

{#if node.isFolder}
  <button
    type="button"
    onclick={() => onToggle(node.path)}
    class="w-full text-left flex items-center gap-1.5 py-1 pr-2 text-sm hover:bg-surface0 rounded text-subtext"
    style={pad}
  >
    <span class="text-dim w-3 text-xs">{isOpen ? '▾' : '▸'}</span>
    <span class="text-warning">📁</span>
    <span class="flex-1 truncate">{node.name || '/'}</span>
    {#if node.count !== undefined && node.count > 0}
      <span class="text-[10px] text-dim">{node.count}</span>
    {/if}
  </button>
  {#if isOpen}
    {#each node.children ?? [] as child (child.path)}
      <Self node={child} depth={depth + 1} {expanded} {currentPath} {onToggle} {onSelect} />
    {/each}
  {/if}
{:else}
  <a
    href="/notes/{encodeURIComponent(node.path)}"
    onclick={(e) => { onSelect?.(node.path); }}
    class="flex items-center gap-1.5 py-1 pr-2 text-sm hover:bg-surface0 rounded
      {isCurrent ? 'bg-surface1 text-primary' : 'text-text'}"
    style={pad}
  >
    <span class="text-dim w-3 text-xs">·</span>
    <span class="flex-1 truncate">{node.note?.title ?? node.name.replace(/\.md$/, '')}</span>
  </a>
{/if}
