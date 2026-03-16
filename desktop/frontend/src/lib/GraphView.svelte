<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'
  export let data: any = null
  const dispatch = createEventDispatcher()

  let svgEl: SVGSVGElement
  let width = 800
  let height = 600
  let nodes: any[] = []
  let edges: any[] = []

  let nodeMap: Record<string, any> = {}

  $: if (data && data.nodes) simulate(data)

  function simulate(d: any) {
    nodes = d.nodes.map((n: any, i: number) => ({
      ...n, x: width/2 + (Math.random()-0.5)*width*0.7, y: height/2 + (Math.random()-0.5)*height*0.7
    }))
    edges = d.edges || []
    nodeMap = {}
    nodes.forEach(n => nodeMap[n.id] = n)

    for (let iter = 0; iter < 80; iter++) {
      for (let i = 0; i < nodes.length; i++) {
        for (let j = i+1; j < nodes.length; j++) {
          const dx = nodes[j].x - nodes[i].x
          const dy = nodes[j].y - nodes[i].y
          const dist = Math.max(Math.sqrt(dx*dx+dy*dy), 1)
          const f = 800 / (dist*dist)
          nodes[i].x -= dx/dist*f; nodes[i].y -= dy/dist*f
          nodes[j].x += dx/dist*f; nodes[j].y += dy/dist*f
        }
      }
      edges.forEach((e: any) => {
        const s = nodeMap[e.source], t = nodeMap[e.target]
        if (!s || !t) return
        const dx = t.x-s.x, dy = t.y-s.y, dist = Math.sqrt(dx*dx+dy*dy)
        const f = dist * 0.008
        s.x += dx*f; s.y += dy*f; t.x -= dx*f; t.y -= dy*f
      })
      nodes.forEach(n => { n.x += (width/2-n.x)*0.01; n.y += (height/2-n.y)*0.01 })
    }
    nodes = [...nodes]
  }

  function nodeColor(n: any) {
    if (n.isCenter) return 'var(--ctp-mauve)'
    if (n.total >= 5) return 'var(--ctp-green)'
    if (n.total === 0) return 'var(--ctp-red)'
    return 'var(--ctp-blue)'
  }
  function nodeRadius(n: any) { return Math.max(4, Math.min(12, 4 + n.total)) }
</script>

<div class="fixed inset-0 z-50 flex justify-center items-center" style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)" on:click|self={() => dispatch('close')}>
  <div class="w-[85vw] h-[80vh] bg-ctp-base rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden">
    <div class="flex items-center justify-between px-4 py-2 border-b border-ctp-surface0 bg-ctp-mantle">
      <span class="text-sm font-semibold text-ctp-text">Note Graph</span>
      <div class="flex items-center gap-3 text-[12px] text-ctp-overlay1">
        <span>{data?.stats?.totalNodes || 0} nodes</span>
        <span>{data?.stats?.totalEdges || 0} edges</span>
        <span>{data?.stats?.orphanCount || 0} orphans</span>
        <kbd class="bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer" on:click={() => dispatch('close')}>esc</kbd>
      </div>
    </div>
    <div class="flex-1 relative" bind:clientWidth={width} bind:clientHeight={height}>
      <svg bind:this={svgEl} {width} {height} class="w-full h-full">
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
            on:click={() => dispatch('select', n.id)} />
          <text x={n.x} y={n.y - nodeRadius(n) - 4} text-anchor="middle"
            fill="var(--ctp-subtext0)" font-size="9" class="pointer-events-none select-none">
            {n.name.length > 20 ? n.name.slice(0, 20) + '...' : n.name}
          </text>
        {/each}
      </svg>
    </div>
    <div class="px-4 py-1.5 border-t border-ctp-surface0 bg-ctp-mantle flex gap-4 text-[12px] text-ctp-overlay1">
      <span><span class="inline-block w-2 h-2 rounded-full bg-ctp-green mr-1"></span>Hub (5+)</span>
      <span><span class="inline-block w-2 h-2 rounded-full bg-ctp-blue mr-1"></span>Normal</span>
      <span><span class="inline-block w-2 h-2 rounded-full bg-ctp-red mr-1"></span>Orphan</span>
      <span><span class="inline-block w-2 h-2 rounded-full bg-ctp-mauve mr-1"></span>Center</span>
    </div>
  </div>
</div>
