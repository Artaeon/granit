<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { api } from '$lib/api';

  // Local 1-hop graph for the current note. Renders as inline SVG
  // with a center node (current) and the immediate neighbors arrayed
  // around it: outgoing wikilinks above, backlinks below. Node size
  // hints at degree (count of neighbors); click any node to navigate.
  //
  // The full vault graph is too dense to navigate visually. The
  // local view answers the only question that's actually useful in
  // a sidebar: "what is this note connected to?". Power users who
  // want the full graph can still build one against the same /links
  // endpoint — this widget covers the 95% case.

  let { path, onNavigate }: { path: string; onNavigate?: (target: string) => void } = $props();

  interface LinksData {
    outgoing: string[];
    backlinks: { path: string; title: string }[];
  }

  let data = $state<LinksData | null>(null);
  let loading = $state(false);

  $effect(() => {
    void path;
    void load();
  });

  async function load() {
    if (!path) return;
    loading = true;
    try {
      data = await api.req<LinksData>(`/links/${encodeURI(path)}`);
    } catch {
      data = { outgoing: [], backlinks: [] };
    } finally {
      loading = false;
    }
  }

  // Layout: center node at the middle, outgoing nodes on the top arc,
  // backlinks on the bottom arc. Simple radial layout — no force
  // simulation needed for 1 hop with usually <12 neighbors. Each
  // node carries position + label + click target.
  type Node = {
    id: string;       // identity for keyed iteration
    title: string;
    target: string;   // click destination (a wikilink for outgoing, a path for backlinks)
    isOutgoing: boolean;
    cx: number;
    cy: number;
    r: number;
  };

  const W = 280;
  const H = 220;
  const CX = W / 2;
  const CY = H / 2;
  const CENTER_R = 14;
  const NODE_R = 8;
  const RADIUS_OUT = 80;
  const RADIUS_BACK = 80;

  let nodes = $derived.by((): Node[] => {
    const out: Node[] = [];
    if (!data) return out;
    const outgoing = data.outgoing.slice(0, 8); // cap so dense notes don't crowd
    const backlinks = data.backlinks.slice(0, 8);
    // Outgoing arrayed across the top half (angles 200° to 340°).
    for (let i = 0; i < outgoing.length; i++) {
      const t = outgoing.length === 1
        ? Math.PI * 1.5
        : Math.PI + (Math.PI / 6) + (i / (outgoing.length - 1)) * (Math.PI * 2 / 3);
      out.push({
        id: 'out:' + outgoing[i],
        title: outgoing[i],
        target: outgoing[i],
        isOutgoing: true,
        cx: CX + RADIUS_OUT * Math.cos(t),
        cy: CY + RADIUS_OUT * Math.sin(t),
        r: NODE_R
      });
    }
    // Backlinks across the bottom half.
    for (let i = 0; i < backlinks.length; i++) {
      const t = backlinks.length === 1
        ? Math.PI / 2
        : (Math.PI / 6) + (i / (backlinks.length - 1)) * (Math.PI * 2 / 3);
      out.push({
        id: 'bk:' + backlinks[i].path,
        title: backlinks[i].title || backlinks[i].path,
        target: backlinks[i].path,
        isOutgoing: false,
        cx: CX + RADIUS_BACK * Math.cos(t),
        cy: CY + RADIUS_BACK * Math.sin(t),
        r: NODE_R
      });
    }
    return out;
  });

  function clickNode(n: Node) {
    if (n.isOutgoing) {
      // Outgoing link is a wikilink target — same handling as the
      // editor's wikilink click.
      onNavigate?.(n.target);
    } else {
      void goto(`/notes/${encodeURIComponent(n.target)}`);
    }
  }

  let centerLabel = $derived(path.split('/').pop()?.replace(/\.md$/, '') ?? 'this note');
</script>

<div class="local-graph">
  {#if loading && !data}
    <div class="text-xs text-dim italic px-2 py-2">loading graph…</div>
  {:else if data && nodes.length === 0}
    <div class="text-xs text-dim italic px-2 py-2">no connections</div>
  {:else if data}
    <svg viewBox="0 0 {W} {H}" class="w-full" role="img" aria-label="Local knowledge graph">
      <!-- Edges drawn first so nodes layer on top. Outgoing edges
           use the secondary color, backlinks use dim — matches the
           text color convention in BacklinksPanel. -->
      {#each nodes as n (n.id)}
        <line
          x1={CX} y1={CY} x2={n.cx} y2={n.cy}
          stroke={n.isOutgoing ? 'var(--color-secondary)' : 'var(--color-dim)'}
          stroke-width="1"
          opacity="0.45"
        />
      {/each}
      <!-- Center node — current note. Larger, primary-tinted. -->
      <circle cx={CX} cy={CY} r={CENTER_R} fill="var(--color-primary)" opacity="0.85"/>
      <text x={CX} y={CY + 4} text-anchor="middle" font-size="10" font-weight="700" fill="var(--color-on-primary)">●</text>
      <!-- Neighbor nodes -->
      {#each nodes as n (n.id)}
        <g class="lg-node" onclick={() => clickNode(n)} role="button" tabindex="0" aria-label={n.title}>
          <circle
            cx={n.cx} cy={n.cy} r={n.r}
            fill={n.isOutgoing ? 'var(--color-secondary)' : 'var(--color-surface2)'}
            stroke={n.isOutgoing ? 'var(--color-secondary)' : 'var(--color-dim)'}
            stroke-width="1.5"
            opacity="0.85"
          />
          <title>{n.isOutgoing ? '→' : '←'} {n.title}</title>
        </g>
      {/each}
      <!-- Center label below the node so long titles wrap into space
           without overlapping the neighbors. -->
      <text x={CX} y={CY + CENTER_R + 12} text-anchor="middle" font-size="10" fill="var(--color-text)" font-weight="600">{centerLabel}</text>
    </svg>
    <!-- Legend + counts. The graph itself is small; the legend doubles
         as a click target for users who want the textual list. -->
    <div class="flex items-center justify-between px-2 pt-1 text-[10px] text-dim">
      <span class="flex items-center gap-1">
        <span class="inline-block w-2 h-2 rounded-full" style="background: var(--color-secondary)"></span>
        {data.outgoing.length} out
      </span>
      <span class="flex items-center gap-1">
        <span class="inline-block w-2 h-2 rounded-full" style="background: var(--color-dim)"></span>
        {data.backlinks.length} in
      </span>
    </div>
  {/if}
</div>

<style>
  .lg-node {
    cursor: pointer;
    transition: opacity 120ms;
  }
  .lg-node:hover circle {
    opacity: 1;
    stroke-width: 2.5;
  }
  .lg-node:focus {
    outline: none;
  }
  .lg-node:focus-visible circle {
    stroke: var(--color-primary);
    stroke-width: 2.5;
  }
</style>
