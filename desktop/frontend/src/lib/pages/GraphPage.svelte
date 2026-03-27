<script lang="ts">
  import { onMount } from 'svelte'
  import { navigateToPage, currentPagePath } from '../stores'
  import { getGraphData } from '../api'

  let data: any = null
  let width = 800
  let height = 600
  let nodes: any[] = []
  let edges: any[] = []
  let nodeMap: Record<string, any> = {}
  let loading = true

  onMount(async () => {
    try {
      data = await getGraphData($currentPagePath || '')
      if (data?.nodes) simulate(data)
    } catch (e) {
      console.error('Failed to load graph:', e)
    }
    loading = false
  })

  function simulate(d: any) {
    nodes = d.nodes.map((n: any) => ({
      ...n,
      x: width / 2 + (Math.random() - 0.5) * width * 0.7,
      y: height / 2 + (Math.random() - 0.5) * height * 0.7,
    }))
    edges = d.edges || []
    nodeMap = {}
    nodes.forEach(n => nodeMap[n.id] = n)

    for (let iter = 0; iter < 80; iter++) {
      for (let i = 0; i < nodes.length; i++) {
        for (let j = i + 1; j < nodes.length; j++) {
          const dx = nodes[j].x - nodes[i].x
          const dy = nodes[j].y - nodes[i].y
          const dist = Math.max(Math.sqrt(dx * dx + dy * dy), 1)
          const f = 800 / (dist * dist)
          nodes[i].x -= dx / dist * f; nodes[i].y -= dy / dist * f
          nodes[j].x += dx / dist * f; nodes[j].y += dy / dist * f
        }
      }
      edges.forEach((e: any) => {
        const s = nodeMap[e.source], t = nodeMap[e.target]
        if (!s || !t) return
        const dx = t.x - s.x, dy = t.y - s.y, dist = Math.sqrt(dx * dx + dy * dy)
        const f = dist * 0.008
        s.x += dx * f; s.y += dy * f; t.x -= dx * f; t.y -= dy * f
      })
      nodes.forEach(n => {
        n.x += (width / 2 - n.x) * 0.01
        n.y += (height / 2 - n.y) * 0.01
      })
    }
    nodes = [...nodes]
  }

  function nodeColor(n: any): string {
    if (n.isCenter) return 'var(--ctp-mauve)'
    if (n.total >= 5) return 'var(--ctp-green)'
    if (n.total === 0) return 'var(--ctp-red)'
    return 'var(--ctp-blue)'
  }

  function nodeRadius(n: any): number {
    return Math.max(4, Math.min(12, 4 + (n.total || 0)))
  }
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="h-full flex flex-col bg-ctp-base">
  <!-- Header -->
  <div class="flex items-center justify-between px-6 h-12 border-b border-ctp-surface0/25 flex-shrink-0">
    <h1 class="text-lg font-bold text-ctp-text">Graph View</h1>
    <div class="flex items-center gap-4 text-[12px] text-ctp-overlay0">
      <span>{data?.stats?.totalNodes || 0} nodes</span>
      <span>{data?.stats?.totalEdges || 0} edges</span>
      <span>{data?.stats?.orphanCount || 0} orphans</span>
    </div>
  </div>

  <!-- Graph -->
  <div class="flex-1 relative" bind:clientWidth={width} bind:clientHeight={height}>
    {#if loading}
      <div class="flex items-center justify-center h-full text-[14px] text-ctp-overlay0">Loading graph...</div>
    {:else}
      <svg {width} {height} class="w-full h-full">
        {#each edges as e}
          {@const s = nodeMap[e.source]}
          {@const t = nodeMap[e.target]}
          {#if s && t}
            <line x1={s.x} y1={s.y} x2={t.x} y2={t.y} stroke="var(--ctp-surface2)" stroke-width="1" opacity="0.4" />
          {/if}
        {/each}
        {#each nodes as n}
          <circle cx={n.x} cy={n.y} r={nodeRadius(n)} fill={nodeColor(n)} opacity="0.9"
            class="cursor-pointer hover:opacity-100"
            on:click={() => navigateToPage(n.id)} />
          <text x={n.x} y={n.y - nodeRadius(n) - 4} text-anchor="middle"
            fill="var(--ctp-subtext0)" font-size="10" class="pointer-events-none select-none">
            {n.name.length > 25 ? n.name.slice(0, 25) + '...' : n.name}
          </text>
        {/each}
      </svg>
    {/if}
  </div>

  <!-- Legend -->
  <div class="px-6 py-2 border-t border-ctp-surface0/25 flex gap-5 text-[12px] text-ctp-overlay0">
    <span><span class="inline-block w-2.5 h-2.5 rounded-full bg-ctp-green mr-1"></span>Hub (5+)</span>
    <span><span class="inline-block w-2.5 h-2.5 rounded-full bg-ctp-blue mr-1"></span>Normal</span>
    <span><span class="inline-block w-2.5 h-2.5 rounded-full bg-ctp-red mr-1"></span>Orphan</span>
    <span><span class="inline-block w-2.5 h-2.5 rounded-full bg-ctp-mauve mr-1"></span>Center</span>
  </div>
</div>
