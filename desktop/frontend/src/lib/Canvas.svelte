<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'
  const dispatch = createEventDispatcher()
  const api = () => (window as any).go?.main?.GranitApp

  // ---------- Types ----------
  interface CanvasCard {
    id: string
    title: string
    content: string
    x: number
    y: number
    width: number
    height: number
    color: number
  }

  interface CanvasConnection {
    from: string
    to: string
  }

  interface CanvasData {
    cards: CanvasCard[]
    connections: CanvasConnection[]
  }

  // ---------- State ----------
  let canvases: string[] = []
  let currentCanvas = ''
  let showPicker = true
  let newCanvasName = ''

  let cards: CanvasCard[] = []
  let connections: CanvasConnection[] = []

  // Viewport
  let svgEl: SVGSVGElement
  let containerEl: HTMLDivElement
  let viewWidth = 900
  let viewHeight = 600
  let panX = 0
  let panY = 0
  let zoom = 1

  // Interaction
  let draggingCard: CanvasCard | null = null
  let dragOffsetX = 0
  let dragOffsetY = 0
  let isPanning = false
  let panStartX = 0
  let panStartY = 0
  let panStartPanX = 0
  let panStartPanY = 0

  // Connection drawing
  let connectingFrom: CanvasCard | null = null
  let connectMouseX = 0
  let connectMouseY = 0

  // Card editing
  let editingCard: CanvasCard | null = null
  let editTitle = ''
  let editContent = ''

  // Context menu
  let contextCard: CanvasCard | null = null
  let contextX = 0
  let contextY = 0

  const CARD_COLORS = [
    { name: 'Blue', var: '--ctp-blue', bg: 'rgba(137,180,250,0.15)', border: 'var(--ctp-blue)' },
    { name: 'Green', var: '--ctp-green', bg: 'rgba(166,227,161,0.15)', border: 'var(--ctp-green)' },
    { name: 'Yellow', var: '--ctp-yellow', bg: 'rgba(249,226,175,0.15)', border: 'var(--ctp-yellow)' },
    { name: 'Red', var: '--ctp-red', bg: 'rgba(243,139,168,0.15)', border: 'var(--ctp-red)' },
    { name: 'Purple', var: '--ctp-mauve', bg: 'rgba(203,166,247,0.15)', border: 'var(--ctp-mauve)' },
    { name: 'Peach', var: '--ctp-peach', bg: 'rgba(250,179,135,0.15)', border: 'var(--ctp-peach)' },
  ]

  const CARD_W = 180
  const CARD_H = 100

  // ---------- Lifecycle ----------
  onMount(() => {
    loadCanvasList()
  })

  async function loadCanvasList() {
    try {
      canvases = (await api()?.ListCanvases()) || []
    } catch { canvases = [] }
  }

  async function openCanvas(name: string) {
    currentCanvas = name
    showPicker = false
    try {
      const raw = await api()?.GetCanvas(name)
      const data: CanvasData = raw ? JSON.parse(raw) : { cards: [], connections: [] }
      cards = (data.cards || []).map(c => ({ ...c, id: c.id || genId(), width: c.width || CARD_W, height: c.height || CARD_H }))
      connections = data.connections || []
    } catch {
      cards = []
      connections = []
    }
  }

  async function createCanvas() {
    const name = newCanvasName.trim()
    if (!name) return
    newCanvasName = ''
    await saveCanvasData(name, { cards: [], connections: [] })
    await loadCanvasList()
    openCanvas(name)
  }

  async function deleteCanvas(name: string) {
    try { await api()?.DeleteCanvas(name) } catch {}
    await loadCanvasList()
    if (currentCanvas === name) {
      showPicker = true
      currentCanvas = ''
      cards = []
      connections = []
    }
  }

  function backToPicker() {
    saveCurrentCanvas()
    showPicker = true
  }

  async function saveCurrentCanvas() {
    if (!currentCanvas) return
    await saveCanvasData(currentCanvas, { cards, connections })
  }

  async function saveCanvasData(name: string, data: CanvasData) {
    try { await api()?.SaveCanvas(name, JSON.stringify(data)) } catch {}
  }

  function genId() { return Math.random().toString(36).slice(2, 10) }

  // ---------- Card Operations ----------
  function addCard() {
    const cx = (-panX + viewWidth / 2) / zoom
    const cy = (-panY + viewHeight / 2) / zoom
    const card: CanvasCard = {
      id: genId(),
      title: 'New Card',
      content: '',
      x: cx - CARD_W / 2,
      y: cy - CARD_H / 2,
      width: CARD_W,
      height: CARD_H,
      color: 0,
    }
    cards = [...cards, card]
    editingCard = card
    editTitle = card.title
    editContent = card.content
    saveCurrentCanvas()
  }

  function deleteCard(card: CanvasCard) {
    cards = cards.filter(c => c.id !== card.id)
    connections = connections.filter(cn => cn.from !== card.id && cn.to !== card.id)
    contextCard = null
    saveCurrentCanvas()
  }

  function cycleColor(card: CanvasCard) {
    card.color = (card.color + 1) % CARD_COLORS.length
    cards = [...cards]
    contextCard = null
    saveCurrentCanvas()
  }

  function saveEditingCard() {
    if (!editingCard) return
    editingCard.title = editTitle.trim() || 'Untitled'
    editingCard.content = editContent
    cards = [...cards]
    editingCard = null
    saveCurrentCanvas()
  }

  // ---------- SVG coordinate helpers ----------
  function svgPoint(clientX: number, clientY: number) {
    const rect = svgEl?.getBoundingClientRect()
    if (!rect) return { x: 0, y: 0 }
    return {
      x: (clientX - rect.left - panX) / zoom,
      y: (clientY - rect.top - panY) / zoom,
    }
  }

  // ---------- Mouse handlers ----------
  function onSvgMouseDown(e: MouseEvent) {
    if (e.button === 2) return // right-click handled separately
    if (e.button !== 0) return
    // Only pan if clicking on SVG background (not a card)
    isPanning = true
    panStartX = e.clientX
    panStartY = e.clientY
    panStartPanX = panX
    panStartPanY = panY
  }

  function onSvgMouseMove(e: MouseEvent) {
    if (draggingCard) {
      const pt = svgPoint(e.clientX, e.clientY)
      draggingCard.x = pt.x - dragOffsetX
      draggingCard.y = pt.y - dragOffsetY
      cards = [...cards]
    } else if (isPanning) {
      panX = panStartPanX + (e.clientX - panStartX)
      panY = panStartPanY + (e.clientY - panStartY)
    }
    if (connectingFrom) {
      const pt = svgPoint(e.clientX, e.clientY)
      connectMouseX = pt.x
      connectMouseY = pt.y
    }
  }

  function onSvgMouseUp(e: MouseEvent) {
    if (draggingCard) {
      draggingCard = null
      saveCurrentCanvas()
    }
    isPanning = false
  }

  function onWheel(e: WheelEvent) {
    e.preventDefault()
    const factor = e.deltaY > 0 ? 0.9 : 1.1
    const newZoom = Math.max(0.2, Math.min(3, zoom * factor))
    // Zoom toward cursor position
    const rect = svgEl?.getBoundingClientRect()
    if (rect) {
      const mx = e.clientX - rect.left
      const my = e.clientY - rect.top
      panX = mx - (mx - panX) * (newZoom / zoom)
      panY = my - (my - panY) * (newZoom / zoom)
    }
    zoom = newZoom
  }

  function onCardMouseDown(e: MouseEvent, card: CanvasCard) {
    e.stopPropagation()
    if (connectingFrom) {
      // Complete connection
      if (connectingFrom.id !== card.id) {
        const exists = connections.some(c =>
          (c.from === connectingFrom!.id && c.to === card.id) ||
          (c.from === card.id && c.to === connectingFrom!.id)
        )
        if (!exists) {
          connections = [...connections, { from: connectingFrom.id, to: card.id }]
          saveCurrentCanvas()
        }
      }
      connectingFrom = null
      return
    }
    const pt = svgPoint(e.clientX, e.clientY)
    dragOffsetX = pt.x - card.x
    dragOffsetY = pt.y - card.y
    draggingCard = card
  }

  function onCardContextMenu(e: MouseEvent, card: CanvasCard) {
    e.preventDefault()
    e.stopPropagation()
    contextCard = card
    contextX = e.clientX
    contextY = e.clientY
  }

  function onCardDblClick(e: MouseEvent, card: CanvasCard) {
    e.stopPropagation()
    editingCard = card
    editTitle = card.title
    editContent = card.content
  }

  function startConnect(card: CanvasCard, e: MouseEvent) {
    e.stopPropagation()
    connectingFrom = card
    const pt = svgPoint(e.clientX, e.clientY)
    connectMouseX = pt.x
    connectMouseY = pt.y
  }

  function closeContextMenu() { contextCard = null }

  function cardCenter(card: CanvasCard) {
    return { x: card.x + (card.width || CARD_W) / 2, y: card.y + (card.height || CARD_H) / 2 }
  }

  function findCard(id: string) { return cards.find(c => c.id === id) }
</script>

<div class="fixed inset-0 z-50 flex justify-center pt-[3%]" style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)" on:click|self={() => dispatch('close')}>
  <div class="w-[90vw] max-w-6xl h-[88vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-overlay flex flex-col overflow-hidden">

    {#if showPicker}
      <!-- Canvas Picker -->
      <div class="flex items-center justify-between px-4 py-2.5 border-b border-ctp-surface0">
        <div class="flex items-center gap-2">
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-mauve)" stroke-width="1.5" stroke-linecap="round">
            <rect x="1" y="1" width="14" height="14" rx="2" /><line x1="5" y1="1" x2="5" y2="15" /><line x1="11" y1="1" x2="11" y2="15" /><line x1="1" y1="5" x2="15" y2="5" /><line x1="1" y1="11" x2="15" y2="11" />
          </svg>
          <span class="text-sm font-semibold text-ctp-text">Canvases</span>
        </div>
        <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1 transition-colors"
          on:click={() => dispatch('close')}>esc</kbd>
      </div>

      <div class="flex-1 overflow-y-auto p-4">
        <!-- Create new -->
        <div class="flex gap-2 mb-4">
          <input
            class="flex-1 bg-ctp-base border border-ctp-surface0 rounded-lg px-3 py-1.5 text-sm text-ctp-text placeholder-ctp-overlay0 focus:outline-none focus:border-ctp-blue"
            placeholder="New canvas name..."
            bind:value={newCanvasName}
            on:keydown={e => e.key === 'Enter' && createCanvas()}
          />
          <button
            class="px-3 py-1.5 bg-ctp-blue text-ctp-base text-[13px] font-medium rounded-lg hover:opacity-90 transition-opacity"
            on:click={createCanvas}>
            Create
          </button>
        </div>

        {#if canvases.length === 0}
          <div class="flex flex-col items-center py-16 gap-2">
            <svg width="32" height="32" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-surface2)" stroke-width="1" class="opacity-40">
              <rect x="1" y="1" width="14" height="14" rx="2" /><line x1="5" y1="1" x2="5" y2="15" /><line x1="11" y1="1" x2="11" y2="15" />
            </svg>
            <p class="text-sm text-ctp-overlay1">No canvases yet</p>
            <p class="text-[13px] text-ctp-overlay1">Create one to get started</p>
          </div>
        {:else}
          <div class="grid grid-cols-2 sm:grid-cols-3 gap-3">
            {#each canvases as name}
              <div class="group bg-ctp-base border border-ctp-surface0 rounded-lg p-3 hover:border-ctp-blue transition-colors cursor-pointer"
                on:click={() => openCanvas(name)}>
                <div class="flex items-center justify-between mb-1">
                  <span class="text-sm text-ctp-text font-medium truncate">{name}</span>
                  <button class="text-[12px] text-ctp-overlay1 hover:text-ctp-red opacity-0 group-hover:opacity-100 transition-opacity"
                    on:click|stopPropagation={() => deleteCanvas(name)}>
                    <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M4 4l8 8M12 4l-8 8" /></svg>
                  </button>
                </div>
                <div class="text-[12px] text-ctp-overlay1">Click to open</div>
              </div>
            {/each}
          </div>
        {/if}
      </div>

    {:else}
      <!-- Canvas Editor -->
      <div class="flex items-center justify-between px-4 py-2 border-b border-ctp-surface0">
        <div class="flex items-center gap-3">
          <button class="text-[13px] text-ctp-overlay1 hover:text-ctp-blue transition-colors flex items-center gap-1"
            on:click={backToPicker}>
            <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2"><path d="M10 2L4 8l6 6" /></svg>
            Back
          </button>
          <span class="text-sm font-semibold text-ctp-text">{currentCanvas}</span>
          <span class="text-[12px] text-ctp-overlay1">{cards.length} cards</span>
          <span class="text-[12px] text-ctp-overlay1">{connections.length} connections</span>
        </div>
        <div class="flex items-center gap-2">
          <button class="px-2 py-1 bg-ctp-blue text-ctp-base text-[13px] rounded hover:opacity-90 transition-opacity"
            on:click={addCard}>+ Card</button>
          <span class="text-[12px] text-ctp-overlay1">{Math.round(zoom * 100)}%</span>
          <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1"
            on:click={() => dispatch('close')}>esc</kbd>
        </div>
      </div>

      <!-- SVG Canvas Area -->
      <div class="flex-1 relative overflow-hidden bg-ctp-base" bind:this={containerEl}
        bind:clientWidth={viewWidth} bind:clientHeight={viewHeight}
        on:click={closeContextMenu}>
        <svg bind:this={svgEl}
          width={viewWidth} height={viewHeight}
          class="w-full h-full cursor-grab"
          class:cursor-grabbing={isPanning}
          on:mousedown={onSvgMouseDown}
          on:mousemove={onSvgMouseMove}
          on:mouseup={onSvgMouseUp}
          on:mouseleave={onSvgMouseUp}
          on:wheel={onWheel}>

          <!-- Dot grid background -->
          <defs>
            <pattern id="dotgrid" width={20 * zoom} height={20 * zoom} patternUnits="userSpaceOnUse"
              x={panX % (20 * zoom)} y={panY % (20 * zoom)}>
              <circle cx={1} cy={1} r="1" fill="var(--ctp-surface1)" opacity="0.4" />
            </pattern>
          </defs>
          <rect width="100%" height="100%" fill="url(#dotgrid)" />

          <g transform="translate({panX},{panY}) scale({zoom})">
            <!-- Connection lines -->
            {#each connections as conn}
              {@const fromCard = findCard(conn.from)}
              {@const toCard = findCard(conn.to)}
              {#if fromCard && toCard}
                {@const fc = cardCenter(fromCard)}
                {@const tc = cardCenter(toCard)}
                <line x1={fc.x} y1={fc.y} x2={tc.x} y2={tc.y}
                  stroke="var(--ctp-overlay1)" stroke-width={1.5 / zoom} opacity="0.5" />
                <!-- Arrow head -->
                {@const angle = Math.atan2(tc.y - fc.y, tc.x - fc.x)}
                {@const arrowSize = 8}
                <polygon
                  points="{tc.x},{tc.y} {tc.x - arrowSize * Math.cos(angle - 0.3)},{tc.y - arrowSize * Math.sin(angle - 0.3)} {tc.x - arrowSize * Math.cos(angle + 0.3)},{tc.y - arrowSize * Math.sin(angle + 0.3)}"
                  fill="var(--ctp-overlay0)" opacity="0.5" />
              {/if}
            {/each}

            <!-- Connecting line preview -->
            {#if connectingFrom}
              {@const fc = cardCenter(connectingFrom)}
              <line x1={fc.x} y1={fc.y} x2={connectMouseX} y2={connectMouseY}
                stroke="var(--ctp-blue)" stroke-width={1.5 / zoom} stroke-dasharray={`${4/zoom} ${4/zoom}`} opacity="0.7" />
            {/if}

            <!-- Cards -->
            {#each cards as card}
              {@const col = CARD_COLORS[card.color % CARD_COLORS.length]}
              <g on:mousedown={e => onCardMouseDown(e, card)}
                on:dblclick={e => onCardDblClick(e, card)}
                on:contextmenu={e => onCardContextMenu(e, card)}>
                <!-- Card shadow -->
                <rect x={card.x + 2} y={card.y + 2} width={card.width || CARD_W} height={card.height || CARD_H}
                  rx="6" fill="rgba(0,0,0,0.2)" />
                <!-- Card background -->
                <rect x={card.x} y={card.y} width={card.width || CARD_W} height={card.height || CARD_H}
                  rx="6" fill={col.bg} stroke={col.border} stroke-width={1.5 / zoom} />
                <!-- Color strip -->
                <rect x={card.x} y={card.y} width="4" height={card.height || CARD_H}
                  rx="2" fill={col.border} />
                <!-- Title -->
                <text x={card.x + 12} y={card.y + 20} fill="var(--ctp-text)"
                  font-size="12" font-weight="600" class="pointer-events-none select-none">
                  {card.title.length > 22 ? card.title.slice(0, 22) + '...' : card.title}
                </text>
                <!-- Content preview -->
                {#if card.content}
                  <text x={card.x + 12} y={card.y + 38} fill="var(--ctp-subtext0)"
                    font-size="10" class="pointer-events-none select-none">
                    {card.content.length > 28 ? card.content.slice(0, 28) + '...' : card.content}
                  </text>
                {/if}
                <!-- Connect handle (right edge) -->
                <circle cx={card.x + (card.width || CARD_W)} cy={card.y + (card.height || CARD_H) / 2}
                  r={5} fill="var(--ctp-surface0)" stroke={col.border} stroke-width="1.5"
                  class="cursor-crosshair opacity-0 hover:opacity-100 transition-opacity"
                  on:mousedown={e => startConnect(card, e)} />
              </g>
            {/each}
          </g>
        </svg>

        <!-- Context menu -->
        {#if contextCard}
          <div class="fixed bg-ctp-base border border-ctp-surface0 rounded-lg shadow-lg py-1 z-[60] min-w-[140px]"
            style="left:{contextX}px;top:{contextY}px">
            <button class="w-full px-3 py-1.5 text-[13px] text-left text-ctp-text hover:bg-ctp-surface0 flex items-center gap-2"
              on:click={() => { if (contextCard) { editingCard = contextCard; editTitle = contextCard.title; editContent = contextCard.content; contextCard = null } }}>
              <svg width="11" height="11" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M11 2l3 3L5 14H2v-3z" /></svg>
              Edit
            </button>
            <button class="w-full px-3 py-1.5 text-[13px] text-left text-ctp-text hover:bg-ctp-surface0 flex items-center gap-2"
              on:click={() => { if (contextCard) cycleColor(contextCard) }}>
              <svg width="11" height="11" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="8" cy="8" r="6" /></svg>
              Color
            </button>
            <button class="w-full px-3 py-1.5 text-[13px] text-left text-ctp-text hover:bg-ctp-surface0 flex items-center gap-2"
              on:click={() => { if (contextCard) startConnect(contextCard, new MouseEvent('mousedown')) }}>
              <svg width="11" height="11" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M4 12L12 4" /><circle cx="12" cy="4" r="2" /></svg>
              Connect
            </button>
            <div class="border-t border-ctp-surface0 my-1"></div>
            <button class="w-full px-3 py-1.5 text-[13px] text-left text-ctp-red hover:bg-ctp-surface0 flex items-center gap-2"
              on:click={() => { if (contextCard) deleteCard(contextCard) }}>
              <svg width="11" height="11" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M4 4l8 8M12 4l-8 8" /></svg>
              Delete
            </button>
          </div>
        {/if}

        <!-- Edit card modal -->
        {#if editingCard}
          <div class="absolute inset-0 flex items-center justify-center z-[60]" style="background:rgba(0,0,0,0.3)"
            on:click|self={() => saveEditingCard()}>
            <div class="w-80 bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-overlay p-4">
              <div class="text-[13px] font-semibold text-ctp-text mb-3">Edit Card</div>
              <input class="w-full bg-ctp-base border border-ctp-surface0 rounded-lg px-3 py-1.5 text-sm text-ctp-text mb-2 focus:outline-none focus:border-ctp-blue"
                placeholder="Title" bind:value={editTitle}
                on:keydown={e => e.key === 'Enter' && !e.shiftKey && saveEditingCard()} />
              <textarea class="w-full bg-ctp-base border border-ctp-surface0 rounded-lg px-3 py-1.5 text-sm text-ctp-text mb-3 resize-none focus:outline-none focus:border-ctp-blue"
                rows="3" placeholder="Content (optional)" bind:value={editContent}></textarea>
              <!-- Color picker -->
              <div class="flex gap-1.5 mb-3">
                {#each CARD_COLORS as col, i}
                  <button class="w-5 h-5 rounded-full border-2 transition-transform"
                    style="background:{col.border};border-color:{editingCard && editingCard.color === i ? 'var(--ctp-text)' : 'transparent'}"
                    on:click={() => { if (editingCard) { editingCard.color = i; cards = [...cards] } }}>
                  </button>
                {/each}
              </div>
              <div class="flex justify-end gap-2">
                <button class="px-3 py-1 text-[13px] text-ctp-overlay1 hover:text-ctp-text transition-colors"
                  on:click={() => { editingCard = null }}>Cancel</button>
                <button class="px-3 py-1 bg-ctp-blue text-ctp-base text-[13px] rounded-lg hover:opacity-90"
                  on:click={saveEditingCard}>Save</button>
              </div>
            </div>
          </div>
        {/if}
      </div>

      <!-- Footer -->
      <div class="px-4 py-1.5 border-t border-ctp-surface0 flex items-center gap-4 text-[12px] text-ctp-overlay1">
        <span>Drag to move cards</span>
        <span>Double-click to edit</span>
        <span>Right-click for menu</span>
        <span>Scroll to zoom</span>
        <span>Drag background to pan</span>
        <span>Click edge dot to connect</span>
      </div>
    {/if}
  </div>
</div>
