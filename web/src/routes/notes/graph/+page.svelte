<script lang="ts">
  // Concept-graph view — force-directed network of notes (nodes) and
  // wikilinks (edges). "More like the mind" surface: every other notes
  // view imposes hierarchy (alpha / folders / tags); this one shows
  // the actual web the user has built.
  //
  // The simulation is hand-rolled: ~80 lines of Verlet-ish integration
  // with Coulomb repulsion + spring attraction + a weak centring
  // pull. No third-party graph dep — the web bundle stays trim and
  // the constants live right next to the view so they're easy to
  // tune from one file.
  import { onMount, untrack } from 'svelte';
  import { goto } from '$app/navigation';
  import { auth } from '$lib/stores/auth';
  import { api, type NotesGraph } from '$lib/api';

  // --- data --------------------------------------------------------------

  let graph = $state<NotesGraph | null>(null);
  let loading = $state(true);
  let loadError = $state<string | null>(null);
  let computing = $state(false);

  // Filter state. Empty string = unfiltered. The "universe" each
  // dropdown picks from is derived from whatever graph we last
  // fetched, so a tag-restricted graph doesn't leak unfiltered
  // options back into the menus.
  let tagFilter = $state('');
  let folderFilter = $state('');

  // --- viewport (pan + zoom) ---------------------------------------------

  // CSS transform on the inner <g> wrapper — translate first, scale
  // second, so dragging stays at screen-pixel granularity regardless
  // of zoom. We rebuild a `matrix(...)` string each frame.
  let panX = $state(0);
  let panY = $state(0);
  let zoom = $state(1);
  let svgEl: SVGSVGElement | undefined = $state();
  // Drag tracking. We don't use a $state for these because the values
  // change at pointermove cadence — mutation inside listeners during
  // a frame is fine, no UI binding needs to react.
  let dragging = false;
  let dragStartX = 0;
  let dragStartY = 0;
  let dragOriginPanX = 0;
  let dragOriginPanY = 0;

  // Multi-touch tracking for pinch-zoom on mobile. Wheel events don't
  // fire on touch devices, so without pinch the graph is panned-only
  // on phones — useless on a 5000-node vault where the user needs to
  // zoom into a cluster. We track the active pointers in a Map keyed
  // by pointerId; when exactly two are down we drop the single-pointer
  // pan into pinch mode and recompute zoom from their distance ratio.
  // Single-pointer pan resumes the moment one finger lifts.
  const pointers = new Map<number, { x: number; y: number }>();
  let pinching = false;
  let pinchStartDist = 0;
  let pinchStartZoom = 1;
  // The midpoint of the two fingers (in screen pixels relative to the
  // SVG's bounding box) at the moment pinch starts. We anchor the zoom
  // around this point so the spot under the user's fingers stays put
  // as they spread / squeeze.
  let pinchAnchorX = 0;
  let pinchAnchorY = 0;
  // panX/panY at pinch start, for anchor math during pinch.
  let pinchStartPanX = 0;
  let pinchStartPanY = 0;

  // Viewport dims. SVG is set to fill its container; we observe its
  // ResizeObserver to get current width/height so the centring force
  // and the initial node placement work in real pixels.
  let viewW = $state(800);
  let viewH = $state(600);

  // --- simulation state -------------------------------------------------

  // Per-node mutable position + velocity. Kept off the $state graph
  // because we mutate it 60x/sec — Svelte reactivity overhead per
  // assignment would dominate. Instead we keep the arrays in plain
  // refs and force a single re-render at the end of each rAF tick
  // via a $state counter.
  interface SimNode {
    id: string;
    title: string;
    path: string;
    degree: number;
    tags: string[];
    x: number;
    y: number;
    vx: number;
    vy: number;
    r: number;
  }
  interface SimEdge {
    a: SimNode;
    b: SimNode;
  }
  // $state.raw: the array root is reactive (reassigning triggers a
  // re-render) but the inner objects are NOT proxied. 300 nodes × N²
  // inner-loop velocity writes would be a meaningful slice of the
  // frame budget if every assignment went through Svelte's reactive
  // proxy, and there's no benefit — we never read individual node
  // properties in templates outside of the {#each n.x ...} read
  // path, which happens during the post-frame re-render anyway.
  // One slice() per frame at the end of the tick flips the array
  // identity so the keyed each block sees fresh data while keeping
  // DOM nodes (no teardown — keyed by stable id).
  let simNodes = $state.raw<SimNode[]>([]);
  let simEdges = $state.raw<SimEdge[]>([]);
  let raf: number | null = null;

  // Force-law constants. Tuned by eye for a vault in the ~50–300 node
  // range; the user can adjust if their vault drifts very dense.
  //   REPEL_K     — Coulomb strength. Larger → notes push apart more.
  //   REST_LEN    — natural edge length in pixels.
  //   SPRING_K    — spring stiffness for linked pairs.
  //   CENTRE_K    — pull every node weakly toward the canvas centre.
  //   DAMPING     — per-frame velocity multiplier (0 = freeze, 1 = none).
  //   MAX_TICKS   — hard ceiling so the loop ALWAYS terminates.
  //   KE_THRESH   — kinetic energy below which we call it converged.
  //   MAX_SPEED   — clamp so a degenerate force can't shoot a node off-screen.
  const REPEL_K = 4500;
  const REST_LEN = 60;
  const SPRING_K = 0.04;
  const CENTRE_K = 0.012;
  const DAMPING = 0.9;
  const MAX_TICKS = 600;
  const KE_THRESH = 0.02;
  const MAX_SPEED = 30;
  let tickCount = 0;

  function buildSimulation(g: NotesGraph) {
    const cx = viewW / 2;
    const cy = viewH / 2;
    // Seed positions on a circle scaled to viewport — keeps the cloud
    // visible on first frame instead of starting at a single point
    // (which would explode under repulsion before settling).
    const radius = Math.min(viewW, viewH) * 0.35;
    const byId = new Map<string, SimNode>();
    simNodes = g.nodes.map((n, i) => {
      const angle = (i / Math.max(1, g.nodes.length)) * Math.PI * 2;
      const node: SimNode = {
        id: n.id,
        title: n.title || n.path,
        path: n.path,
        degree: n.degree,
        tags: n.tags ?? [],
        x: cx + Math.cos(angle) * radius,
        y: cy + Math.sin(angle) * radius,
        vx: 0,
        vy: 0,
        r: Math.min(12, 3 + n.degree / 3) // ~3-12 px, sized by degree
      };
      byId.set(n.id, node);
      return node;
    });
    const edges: SimEdge[] = [];
    for (const e of g.edges) {
      const a = byId.get(e.source);
      const b = byId.get(e.target);
      if (a && b) edges.push({ a, b });
    }
    simEdges = edges;
    tickCount = 0;
  }

  // One simulation step. Returns kinetic energy so the driver can
  // decide whether we've converged.
  function tick(): number {
    const cx = viewW / 2;
    const cy = viewH / 2;
    // Snapshot the proxied $state arrays into plain locals — 300x300
    // inner-loop accesses through Svelte's reactive proxy would burn
    // a meaningful slice of the frame budget. The underlying node
    // objects are NOT proxied (only the array root is), so mutating
    // a.x / a.vx inside the loop is direct, no overhead.
    const nodes = simNodes;
    const edges = simEdges;
    const n = nodes.length;
    // Repulsion — O(N²). Fine for N ≤ 300; the server caps it there.
    // A quadtree would be the next step if we ever raise that limit.
    for (let i = 0; i < n; i++) {
      const a = nodes[i];
      for (let j = i + 1; j < n; j++) {
        const b = nodes[j];
        const dx = b.x - a.x;
        const dy = b.y - a.y;
        // Soften the singularity at d ≈ 0 — otherwise two coincident
        // seeds blow the simulation up before the first frame paints.
        const d2 = dx * dx + dy * dy + 0.01;
        const d = Math.sqrt(d2);
        const f = REPEL_K / d2;
        const fx = (dx / d) * f;
        const fy = (dy / d) * f;
        a.vx -= fx;
        a.vy -= fy;
        b.vx += fx;
        b.vy += fy;
      }
      // Centring pull — keeps the cloud on-screen without anchoring
      // any single node, so the user can still pan freely.
      a.vx += (cx - a.x) * CENTRE_K;
      a.vy += (cy - a.y) * CENTRE_K;
    }
    // Attraction via Hooke's law on each edge.
    for (const e of edges) {
      const dx = e.b.x - e.a.x;
      const dy = e.b.y - e.a.y;
      const d = Math.sqrt(dx * dx + dy * dy) + 0.01;
      const f = (d - REST_LEN) * SPRING_K;
      const fx = (dx / d) * f;
      const fy = (dy / d) * f;
      e.a.vx += fx;
      e.a.vy += fy;
      e.b.vx -= fx;
      e.b.vy -= fy;
    }
    // Integrate + dampen. Speed clamp guards against a force spike
    // (rare but possible during the first few frames when nodes start
    // close together) sending a node off into geometric oblivion.
    let ke = 0;
    for (const a of nodes) {
      a.vx *= DAMPING;
      a.vy *= DAMPING;
      const sp = Math.sqrt(a.vx * a.vx + a.vy * a.vy);
      if (sp > MAX_SPEED) {
        a.vx = (a.vx / sp) * MAX_SPEED;
        a.vy = (a.vy / sp) * MAX_SPEED;
      }
      a.x += a.vx;
      a.y += a.vy;
      ke += a.vx * a.vx + a.vy * a.vy;
    }
    return ke / Math.max(1, n);
  }

  function startSimulation() {
    if (raf !== null) cancelAnimationFrame(raf);
    computing = true;
    const loop = () => {
      const ke = tick();
      tickCount++;
      // Force a reactive re-render by flipping both arrays' identity.
      // The values inside are the same Sim{Node,Edge} instances we
      // just mutated; Svelte 5 only re-runs the template blocks whose
      // tracked $state root has reassigned, so an in-place mutation
      // alone would leave the SVG frozen on iteration zero. Edges
      // need the same treatment because the <line> block reads
      // `simEdges` (not `simNodes`) — fine-grained reactivity means
      // re-rendering the node block doesn't cascade to the edge block.
      simNodes = simNodes.slice();
      simEdges = simEdges.slice();
      if (tickCount >= MAX_TICKS || (tickCount > 30 && ke < KE_THRESH)) {
        // Converged (or hit the safety ceiling). Stop drawing — the
        // graph stays where the user can pan / zoom freely without
        // the layout drifting underneath them.
        raf = null;
        computing = false;
        return;
      }
      raf = requestAnimationFrame(loop);
    };
    raf = requestAnimationFrame(loop);
  }

  // --- data loader -------------------------------------------------------

  async function load() {
    if (!$auth) return;
    loading = true;
    loadError = null;
    try {
      const params: { tag?: string; folder?: string; limit?: number } = { limit: 300 };
      if (tagFilter) params.tag = tagFilter;
      if (folderFilter) params.folder = folderFilter;
      const g = await api.notesGraph(params);
      graph = g;
      buildSimulation(g);
      startSimulation();
    } catch (e) {
      loadError = e instanceof Error ? e.message : String(e);
      graph = { nodes: [], edges: [] };
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    load();
    return () => {
      if (raf !== null) cancelAnimationFrame(raf);
    };
  });

  // ResizeObserver setup runs whenever the SVG element appears in the
  // DOM — could be delayed if the first render lands in the loading
  // state (svgEl is still undefined there). Re-binding inside an
  // effect means we pick it up the moment the {:else} branch swaps
  // the SVG into place.
  $effect(() => {
    const el = svgEl;
    if (!el) return;
    const measure = () => {
      const r = el.getBoundingClientRect();
      viewW = Math.max(200, r.width);
      viewH = Math.max(200, r.height);
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  });

  // Re-fetch whenever a filter changes. Untrack avoids the obvious
  // infinite loop: load() flips graph, which doesn't touch the
  // filters, but we still want the effect bounded to filter values.
  let firstFilterRun = true;
  $effect(() => {
    void tagFilter;
    void folderFilter;
    if (firstFilterRun) {
      firstFilterRun = false;
      return;
    }
    untrack(() => load());
  });

  // --- derived: filter universes ----------------------------------------

  // Tag universe = every unique tag mentioned by any node in the
  // currently-loaded graph. Sorted alphabetically; the user picks
  // from these in the chip-strip dropdown.
  let allTags = $derived.by(() => {
    if (!graph) return [] as string[];
    const set = new Set<string>();
    for (const n of graph.nodes) {
      for (const t of n.tags ?? []) set.add(t);
    }
    return Array.from(set).sort();
  });

  // Folder universe = every top-level folder path present in the
  // node set (notes at vault root are folded into an unlabeled '/'
  // bucket which we just don't surface — the user clears the filter
  // to see them).
  let allFolders = $derived.by(() => {
    if (!graph) return [] as string[];
    const set = new Set<string>();
    for (const n of graph.nodes) {
      const idx = n.path.indexOf('/');
      if (idx > 0) set.add(n.path.slice(0, idx));
    }
    return Array.from(set).sort();
  });

  // --- pan + zoom handlers ----------------------------------------------

  // Helper: pinch math. Distance between the two active pointers in
  // screen pixels (used to derive the scale factor). Returns 0 when
  // we don't have exactly two pointers down — caller branches.
  function pointerPairDistance(): number {
    if (pointers.size !== 2) return 0;
    const [a, b] = Array.from(pointers.values());
    const dx = a.x - b.x;
    const dy = a.y - b.y;
    return Math.hypot(dx, dy);
  }
  function pointerPairMidpoint(): { x: number; y: number } {
    if (pointers.size !== 2) return { x: 0, y: 0 };
    const [a, b] = Array.from(pointers.values());
    return { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 };
  }

  function onPointerDown(ev: PointerEvent) {
    // Touch / pen come through as button === 0 — keep them; only skip
    // secondary mouse buttons (right-click, middle-click).
    if (ev.pointerType === 'mouse' && ev.button !== 0) return;
    pointers.set(ev.pointerId, { x: ev.clientX, y: ev.clientY });
    svgEl?.setPointerCapture(ev.pointerId);

    if (pointers.size >= 2) {
      // Two fingers: enter pinch mode. Drop any pan-in-progress so the
      // graph doesn't lurch when the second finger lands.
      dragging = false;
      pinching = true;
      pinchStartDist = pointerPairDistance();
      pinchStartZoom = zoom;
      pinchStartPanX = panX;
      pinchStartPanY = panY;
      const mid = pointerPairMidpoint();
      const rect = svgEl?.getBoundingClientRect();
      pinchAnchorX = mid.x - (rect?.left ?? 0);
      pinchAnchorY = mid.y - (rect?.top ?? 0);
      return;
    }

    // Single pointer: pan as before.
    dragging = true;
    dragStartX = ev.clientX;
    dragStartY = ev.clientY;
    dragOriginPanX = panX;
    dragOriginPanY = panY;
  }
  function onPointerMove(ev: PointerEvent) {
    const tracked = pointers.get(ev.pointerId);
    if (tracked) {
      tracked.x = ev.clientX;
      tracked.y = ev.clientY;
    }

    if (pinching) {
      // Recompute zoom from distance ratio and anchor the scale around
      // the pinch midpoint (screen-local pixels). Same anchor-math as
      // the wheel handler — the canvas point under the anchor stays
      // under the anchor as the scale changes.
      const dist = pointerPairDistance();
      if (dist <= 0 || pinchStartDist <= 0) return;
      const newZoom = Math.max(0.2, Math.min(5, pinchStartZoom * (dist / pinchStartDist)));
      const factor = newZoom / pinchStartZoom;
      panX = pinchAnchorX - (pinchAnchorX - pinchStartPanX) * factor;
      panY = pinchAnchorY - (pinchAnchorY - pinchStartPanY) * factor;
      zoom = newZoom;
      return;
    }

    if (!dragging) return;
    panX = dragOriginPanX + (ev.clientX - dragStartX);
    panY = dragOriginPanY + (ev.clientY - dragStartY);
  }
  function onPointerUp(ev: PointerEvent) {
    pointers.delete(ev.pointerId);
    svgEl?.releasePointerCapture(ev.pointerId);
    if (pointers.size < 2 && pinching) {
      // Lifting one finger ends the pinch. If a single finger remains
      // we DON'T resume pan (would feel like an accidental shove
      // right when the user finishes their zoom). They tap-and-drag
      // again to pan.
      pinching = false;
      dragging = false;
    }
    if (pointers.size === 0) {
      dragging = false;
    }
  }
  function onWheel(ev: WheelEvent) {
    ev.preventDefault();
    // Zoom around the cursor: shift the pan such that the canvas
    // point under the pointer stays under the pointer after scale
    // changes. Without this the graph would zoom toward (0,0)
    // which is jarring when the user is focused on a particular
    // cluster off-centre.
    if (!svgEl) return;
    const rect = svgEl.getBoundingClientRect();
    const px = ev.clientX - rect.left;
    const py = ev.clientY - rect.top;
    const k = ev.deltaY < 0 ? 1.1 : 1 / 1.1;
    const newZoom = Math.max(0.2, Math.min(5, zoom * k));
    const factor = newZoom / zoom;
    panX = px - (px - panX) * factor;
    panY = py - (py - panY) * factor;
    zoom = newZoom;
  }

  // --- node interaction -------------------------------------------------

  let hoverNode = $state<SimNode | null>(null);
  function nodeClick(n: SimNode) {
    if (dragging) return; // ignore the spurious click that ends a drag
    goto(`/notes/${encodeURIComponent(n.path)}`);
  }

</script>

<!-- Tab title is set by the layout's `tabTitle` derived (Granit ·
     Graph) so we don't need a per-page <svelte:head>. -->

<div class="flex h-full flex-col">
  <!-- Filter strip. Dense / thin / power-UI shape — two dropdowns
       and a node count, no decoration beyond the chip pill. -->
  <div class="flex items-center gap-2 border-b border-surface1 bg-surface0 px-3 py-1.5 text-sm">
    <span class="text-dim">Filter</span>
    <select
      bind:value={tagFilter}
      class="rounded border border-surface1 bg-base px-2 py-0.5 text-sm"
      aria-label="Filter by tag"
    >
      <option value="">All tags</option>
      {#each allTags as t (t)}
        <option value={t}>#{t}</option>
      {/each}
    </select>
    <select
      bind:value={folderFilter}
      class="rounded border border-surface1 bg-base px-2 py-0.5 text-sm"
      aria-label="Filter by folder"
    >
      <option value="">All folders</option>
      {#each allFolders as f (f)}
        <option value={f}>{f}/</option>
      {/each}
    </select>
    {#if tagFilter || folderFilter}
      <button
        type="button"
        class="rounded border border-surface1 px-2 py-0.5 text-xs text-dim hover:bg-surface1"
        onclick={() => { tagFilter = ''; folderFilter = ''; }}
      >
        clear
      </button>
    {/if}
    <span class="ml-auto text-xs text-dim">
      {#if graph}
        {graph.nodes.length} notes · {graph.edges.length} links
      {/if}
      {#if computing}
        <span class="ml-2">computing layout…</span>
      {/if}
    </span>
  </div>

  <!-- Canvas. flex-1 fills the remaining viewport. Empty-state and
       loading-state both render INSIDE the same container so the
       layout doesn't jump as the data arrives. -->
  <div class="relative flex-1 overflow-hidden bg-base">
    {#if loading && !graph}
      <div class="absolute inset-0 flex items-center justify-center text-sm text-dim">
        computing layout…
      </div>
    {:else if loadError}
      <div class="absolute inset-0 flex items-center justify-center text-sm text-error">
        failed to load graph: {loadError}
      </div>
    {:else if graph && graph.nodes.length === 0}
      <div class="absolute inset-0 flex flex-col items-center justify-center gap-2 text-sm text-dim">
        <p>No notes in this view.</p>
        <p>
          Capture some on <a href="/notes" class="underline">/notes</a>
          and link them with <code>[[wikilinks]]</code> — they will appear here as a web.
        </p>
      </div>
    {:else}
      <svg
        bind:this={svgEl}
        class="absolute inset-0 h-full w-full cursor-grab graph-canvas"
        class:cursor-grabbing={dragging}
        onpointerdown={onPointerDown}
        onpointermove={onPointerMove}
        onpointerup={onPointerUp}
        onpointercancel={onPointerUp}
        onwheel={onWheel}
        role="presentation"
      >
        <g transform="translate({panX} {panY}) scale({zoom})">
          {#each simEdges as e (e.a.id + '|' + e.b.id)}
            <line
              x1={e.a.x}
              y1={e.a.y}
              x2={e.b.x}
              y2={e.b.y}
              stroke="currentColor"
              stroke-opacity="0.2"
              stroke-width={1 / zoom}
            />
          {/each}
          {#each simNodes as n (n.id)}
            <g
              transform="translate({n.x} {n.y})"
              onpointerenter={() => (hoverNode = n)}
              onpointerleave={() => { if (hoverNode === n) hoverNode = null; }}
              onclick={() => nodeClick(n)}
              role="button"
              tabindex="-1"
              class="cursor-pointer"
            >
              <circle
                r={n.r}
                fill="currentColor"
                fill-opacity={hoverNode === n ? 0.9 : 0.7}
              />
              {#if zoom > 1.4 || hoverNode === n}
                <text
                  x={n.r + 3}
                  y={3}
                  font-size={11 / zoom}
                  fill="currentColor"
                  pointer-events="none"
                >
                  {n.title}
                </text>
              {/if}
            </g>
          {/each}
        </g>
      </svg>
      {#if hoverNode}
        <div
          class="pointer-events-none absolute left-3 bottom-3 max-w-xs rounded border border-surface1 bg-surface0 px-2 py-1 text-xs shadow"
        >
          <div class="font-medium">{hoverNode.title}</div>
          <div class="text-dim">{hoverNode.path}</div>
          <div class="text-dim">{hoverNode.degree} link{hoverNode.degree === 1 ? '' : 's'}</div>
        </div>
      {/if}
    {/if}
  </div>
</div>

<style>
  /* Cursor variants — `cursor-grabbing` isn't in the default Tailwind
     util set in this project, so define both inline. Keeps the
     "interactive viewport" affordance obvious without pulling in a
     plugin. */
  :global(.cursor-grab) { cursor: grab; }
  :global(.cursor-grabbing) { cursor: grabbing; }

  /* Two-finger pinch-zoom + single-finger pan need full pointer-event
     control; without touch-action:none the browser intercepts the
     gesture for native page-scroll / page-zoom and the graph never
     sees the second pointer at all. Scoped to the canvas so the rest
     of the page (filter dropdowns) still uses normal touch behaviour. */
  .graph-canvas {
    touch-action: none;
  }
</style>
