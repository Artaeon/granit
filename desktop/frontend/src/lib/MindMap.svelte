<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'
  import { getMindMapData } from './api'
  const dispatch = createEventDispatcher()
  export let notePath: string = ''

  interface MindMapNode {
    id: string
    name: string
    depth: number
    isLink: boolean
    children: MindMapNode[] | null
  }

  interface LayoutNode {
    id: string
    name: string
    depth: number
    x: number
    y: number
  }

  interface LayoutEdge {
    x1: number
    y1: number
    x2: number
    y2: number
    depth: number
  }

  let root: MindMapNode | null = null
  let loading = true
  let layoutNodes: LayoutNode[] = []
  let layoutEdges: LayoutEdge[] = []

  let width = 800
  let height = 600

  // Pan and zoom
  let viewX = 0
  let viewY = 0
  let zoom = 1
  let dragging = false
  let dragStartX = 0
  let dragStartY = 0
  let dragStartViewX = 0
  let dragStartViewY = 0

  let totalNodes = 0

  const depthColors = [
    'var(--ctp-mauve)',
    'var(--ctp-blue)',
    'var(--ctp-sapphire)',
    'var(--ctp-teal)',
    'var(--ctp-green)',
  ]

  function nodeColor(depth: number): string {
    return depthColors[Math.min(depth, depthColors.length - 1)]
  }

  onMount(async () => {
    await loadData()
  })

  async function loadData() {
    loading = true
    try {
      const raw = await getMindMapData(notePath)
      if (raw) {
        root = JSON.parse(raw)
        computeLayout()
      }
    } catch (e) {
      console.error('Failed to load mind map data:', e)
    }
    loading = false
  }

  function computeLayout() {
    layoutNodes = []
    layoutEdges = []
    if (!root) return

    // Radial tree layout
    const centerX = 0
    const centerY = 0
    totalNodes = countNodes(root)

    layoutNodes.push({
      id: root.id,
      name: root.name,
      depth: 0,
      x: centerX,
      y: centerY,
    })

    if (root.children && root.children.length > 0) {
      layoutChildren(root, centerX, centerY, 0, Math.PI * 2, 1)
    }

    // Center the view
    viewX = width / 2
    viewY = height / 2
  }

  function countNodes(node: MindMapNode): number {
    let c = 1
    if (node.children) {
      for (const child of node.children) {
        c += countNodes(child)
      }
    }
    return c
  }

  function countLeaves(node: MindMapNode): number {
    if (!node.children || node.children.length === 0) return 1
    let c = 0
    for (const child of node.children) {
      c += countLeaves(child)
    }
    return c
  }

  function layoutChildren(parent: MindMapNode, px: number, py: number, startAngle: number, sweep: number, depth: number) {
    if (!parent.children || parent.children.length === 0) return

    const radius = 140 + depth * 80
    const totalLeaves = parent.children.reduce((sum, c) => sum + countLeaves(c), 0)

    let currentAngle = startAngle

    for (const child of parent.children) {
      const childLeaves = countLeaves(child)
      const childSweep = (childLeaves / totalLeaves) * sweep
      const angle = currentAngle + childSweep / 2

      const cx = px + Math.cos(angle) * radius
      const cy = py + Math.sin(angle) * radius

      layoutNodes.push({
        id: child.id,
        name: child.name,
        depth: depth,
        x: cx,
        y: cy,
      })

      layoutEdges.push({
        x1: px,
        y1: py,
        x2: cx,
        y2: cy,
        depth: depth,
      })

      if (child.children && child.children.length > 0) {
        layoutChildren(child, cx, cy, currentAngle, childSweep, depth + 1)
      }

      currentAngle += childSweep
    }
  }

  // Bezier curve path for connection lines
  function curvePath(e: LayoutEdge): string {
    const dx = e.x2 - e.x1
    const dy = e.y2 - e.y1
    const cx1 = e.x1 + dx * 0.4
    const cy1 = e.y1
    const cx2 = e.x2 - dx * 0.4
    const cy2 = e.y2
    return `M ${e.x1} ${e.y1} C ${cx1} ${cy1}, ${cx2} ${cy2}, ${e.x2} ${e.y2}`
  }

  function handleWheel(e: WheelEvent) {
    e.preventDefault()
    const delta = e.deltaY > 0 ? 0.9 : 1.1
    const newZoom = Math.max(0.2, Math.min(3, zoom * delta))
    zoom = newZoom
  }

  function handleMouseDown(e: MouseEvent) {
    if (e.button === 0) {
      dragging = true
      dragStartX = e.clientX
      dragStartY = e.clientY
      dragStartViewX = viewX
      dragStartViewY = viewY
    }
  }

  function handleMouseMove(e: MouseEvent) {
    if (dragging) {
      viewX = dragStartViewX + (e.clientX - dragStartX)
      viewY = dragStartViewY + (e.clientY - dragStartY)
    }
  }

  function handleMouseUp() {
    dragging = false
  }

  function handleNodeClick(node: LayoutNode) {
    if (node.depth > 0) {
      dispatch('select', node.id)
    }
  }

  function resetView() {
    viewX = width / 2
    viewY = height / 2
    zoom = 1
  }

  function truncName(name: string, max: number = 18): string {
    if (name.length <= max) return name
    return name.slice(0, max - 2) + '..'
  }

  function nodeRadius(depth: number): number {
    if (depth === 0) return 28
    if (depth === 1) return 20
    return 14
  }

  function fontSize(depth: number): number {
    if (depth === 0) return 12
    if (depth === 1) return 10
    return 9
  }
</script>

<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center items-center" style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)"
  on:click|self={() => dispatch('close')}>
  <div class="w-[85vw] h-[80vh] bg-ctp-base rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden">
    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-2 border-b border-ctp-surface0 bg-ctp-mantle">
      <div class="flex items-center gap-3">
        <span class="text-sm font-semibold text-ctp-mauve">Mind Map</span>
        {#if root}
          <span class="text-[12px] text-ctp-overlay1">{root.name}</span>
        {/if}
      </div>
      <div class="flex items-center gap-3 text-[12px] text-ctp-overlay1">
        <span>{totalNodes} nodes</span>
        <span>Zoom: {Math.round(zoom * 100)}%</span>
        <!-- svelte-ignore a11y-click-events-have-key-events -->
        <span class="cursor-pointer hover:text-ctp-text" on:click={resetView}>Reset</span>
        <!-- svelte-ignore a11y-click-events-have-key-events -->
        <kbd class="bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer" on:click={() => dispatch('close')}>esc</kbd>
      </div>
    </div>

    <!-- SVG Canvas -->
    <div class="flex-1 relative" bind:clientWidth={width} bind:clientHeight={height}>
      {#if loading}
        <div class="absolute inset-0 flex items-center justify-center">
          <span class="text-ctp-overlay1 text-sm">Building mind map...</span>
        </div>
      {:else if !root || layoutNodes.length === 0}
        <div class="absolute inset-0 flex items-center justify-center">
          <span class="text-ctp-overlay1 text-sm">No links found for this note</span>
        </div>
      {:else}
        <!-- svelte-ignore a11y-no-static-element-interactions -->
        <svg {width} {height} class="w-full h-full"
          style="cursor:{dragging ? 'grabbing' : 'grab'}"
          on:wheel={handleWheel}
          on:mousedown={handleMouseDown}
          on:mousemove={handleMouseMove}
          on:mouseup={handleMouseUp}
          on:mouseleave={handleMouseUp}>

          <g transform="translate({viewX}, {viewY}) scale({zoom})">
            <!-- Connection lines (curved) -->
            {#each layoutEdges as e}
              <path d={curvePath(e)}
                fill="none"
                stroke={nodeColor(e.depth)}
                stroke-width="1.5"
                opacity="0.35"
                stroke-linecap="round" />
            {/each}

            <!-- Nodes -->
            {#each layoutNodes as n}
              {@const r = nodeRadius(n.depth)}
              <g class="cursor-pointer" on:click={() => handleNodeClick(n)}>
                <!-- Node glow -->
                {#if n.depth === 0}
                  <circle cx={n.x} cy={n.y} r={r + 6} fill={nodeColor(n.depth)} opacity="0.1" />
                {/if}

                <!-- Node circle -->
                <circle cx={n.x} cy={n.y} {r}
                  fill={nodeColor(n.depth)}
                  opacity={n.depth === 0 ? 1 : 0.8}
                  class="hover:opacity-100 transition-opacity" />

                <!-- Node label -->
                <text x={n.x} y={n.y - r - 6}
                  text-anchor="middle"
                  fill="var(--ctp-text)"
                  font-size={fontSize(n.depth)}
                  font-weight={n.depth === 0 ? 'bold' : 'normal'}
                  class="pointer-events-none select-none">
                  {truncName(n.name)}
                </text>
              </g>
            {/each}
          </g>
        </svg>
      {/if}
    </div>

    <!-- Footer -->
    <div class="px-4 py-1.5 border-t border-ctp-surface0 bg-ctp-mantle flex gap-4 text-[12px] text-ctp-overlay1">
      {#each depthColors.slice(0, 3) as color, i}
        <span>
          <span class="inline-block w-2 h-2 rounded-full mr-1" style="background:{color}"></span>
          {i === 0 ? 'Center' : i === 1 ? 'Direct links' : '2nd hop'}
        </span>
      {/each}
      <span class="ml-auto">Scroll to zoom | Drag to pan | Click node to open</span>
    </div>
  </div>
</div>
