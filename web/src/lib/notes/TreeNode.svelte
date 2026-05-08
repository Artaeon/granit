<script lang="ts">
  import Self from './TreeNode.svelte';
  import type { TreeNode } from './treeUtils';
  import { pinnedNotes, togglePin } from './pinnedNotes';

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
  let pinned = $derived($pinnedNotes.has(node.path));

  function pinClick(e: MouseEvent) {
    e.preventDefault();
    e.stopPropagation();
    void togglePin(node.path);
  }
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
    class="group flex items-center gap-1.5 py-1 pr-1 text-sm hover:bg-surface0 rounded
      {isCurrent ? 'bg-surface1 text-primary' : 'text-text'}"
    style={pad}
  >
    <span class="text-dim w-3 text-xs">·</span>
    <span class="flex-1 truncate">{node.note?.title ?? node.name.replace(/\.md$/, '')}</span>
    <!-- Pin star: always visible when pinned, fades in on hover when
         not. Stays out of the way for the 90% of notes that aren't
         pinned, never hides the affordance entirely. -->
    <button
      type="button"
      onclick={pinClick}
      title={pinned ? 'Unpin from top of tree' : 'Pin to top of tree'}
      aria-label={pinned ? 'Unpin' : 'Pin'}
      aria-pressed={pinned}
      class="w-5 h-5 flex items-center justify-center rounded text-dim hover:text-warning hover:bg-surface1
        {pinned ? 'text-warning opacity-100' : 'opacity-0 group-hover:opacity-100 focus:opacity-100'}"
    >
      <svg viewBox="0 0 24 24" class="w-3 h-3" fill={pinned ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="2" stroke-linejoin="round">
        <path d="M12 2l2.6 6.5L22 9.3l-5.4 4.7L18 22l-6-3.5L6 22l1.4-8L2 9.3l7.4-.8L12 2z"/>
      </svg>
    </button>
  </a>
{/if}
