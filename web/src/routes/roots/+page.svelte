<script lang="ts">
  // /roots — contemplative radial diagram of who the user is in
  // Christ. Center holds a name + scripture anchor; four concentric
  // rings hold any number of hand-named items each: Identity,
  // Callings, Gifts, Longings.
  //
  // Design intent (and the constraint this page MUST keep):
  //   • No numbers that go up. No streak, no completion %, no
  //     "skill level". This is not a stats tree. If you find
  //     yourself adding a count or a score, re-read the user's
  //     own framing: "not for plain performance but to be rooted."
  //   • Hand-tended only. Items don't auto-populate from goals/
  //     projects/tasks. The discipline of naming what's true is
  //     the point.
  //   • The visualization is the page. Side panel is a tool for
  //     editing the visualization, not a competing surface.
  //
  // Edits debounce-save (no Save button) — contemplative editing
  // shouldn't ask "are you sure" on every change. The server is
  // forgiving: validation surfaces errors inline; the in-memory
  // record is the source of truth until the PUT resolves.

  import { onMount } from 'svelte';
  import { api, type Roots, type RootsNode } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';

  let roots = $state<Roots | null>(null);
  let loading = $state(true);
  let loadError = $state('');
  let saving = $state(false);

  // Selection — which node's detail panel is open, or null for the
  // center editor, or undefined for nothing selected.
  let selectedId = $state<string | null | undefined>(undefined);

  // Local edit buffers — node detail-panel edits flow through these
  // so the SVG re-renders on every keystroke (responsive) without
  // racing the debounced PUT.
  let editLabel = $state('');
  let editDescription = $state('');
  let editScripture = $state('');
  let editRelated = $state(''); // newline-separated paths
  let centerEdit = $state('');
  let anchorEdit = $state('');

  // Ring colors — muted Catppuccin tokens. Picked to be distinct at
  // a glance without competing with the center; outer rings fade
  // slightly so the eye drifts inward (where Christ is).
  const RING_COLORS: Record<number, { stroke: string; node: string }> = {
    1: { stroke: 'stroke-yellow/40', node: 'fill-yellow/80' },
    2: { stroke: 'stroke-blue/40', node: 'fill-blue/80' },
    3: { stroke: 'stroke-green/40', node: 'fill-green/80' },
    4: { stroke: 'stroke-mauve/40', node: 'fill-mauve/80' }
  };

  // Ring radii — picked so labels around the outermost ring don't
  // clip the 800-unit canvas. Center is radius 60; nodes ~10.
  const RING_RADIUS: Record<number, number> = {
    1: 150,
    2: 230,
    3: 310,
    4: 385
  };

  const CANVAS = 900;
  const CX = CANVAS / 2;
  const CY = CANVAS / 2;

  async function load() {
    loading = true;
    loadError = '';
    try {
      roots = await api.getRoots();
      centerEdit = roots.center ?? 'Christ';
      anchorEdit = roots.anchor ?? '';
    } catch (e) {
      loadError = errorMessage(e);
    } finally {
      loading = false;
    }
  }

  onMount(() => { load(); });

  // ── Save debounce ────────────────────────────────────────────
  // Single timer for all edits. 800ms gives the user time to keep
  // typing without thrashing the disk.
  let saveTimer: ReturnType<typeof setTimeout> | null = null;
  function scheduleSave() {
    if (saveTimer) clearTimeout(saveTimer);
    saveTimer = setTimeout(doSave, 800);
  }
  async function doSave() {
    if (!roots) return;
    saving = true;
    try {
      const updated = await api.putRoots({
        center: (roots.center ?? '').trim() || undefined,
        anchor: (roots.anchor ?? '').trim() || undefined,
        nodes: (roots.nodes ?? []).map((n) => ({
          id: n.id,
          ring: n.ring,
          label: n.label,
          description: n.description,
          scripture: n.scripture,
          related_notes: n.related_notes
        }))
      });
      // The server stamps IDs for new nodes; mirror them back so
      // the next edit references the persisted ID.
      if (roots) {
        roots.nodes = updated.nodes ?? [];
        if (selectedId && !roots.nodes.find((n) => n.id === selectedId)) {
          // Selected node was new and just got an ID from the server;
          // pick it back up by matching label+ring (unique enough
          // for the small node count this surface holds).
          const best = roots.nodes.find((n) => n.label === editLabel && n.ring === ringForSelected());
          if (best) selectedId = best.id;
        }
      }
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      saving = false;
    }
  }

  // Helpers --------------------------------------------------------

  function ringForSelected(): number {
    if (!selectedId || !roots) return 1;
    return roots.nodes?.find((n) => n.id === selectedId)?.ring ?? 1;
  }

  function nodesOnRing(ring: number): RootsNode[] {
    return (roots?.nodes ?? []).filter((n) => n.ring === ring);
  }

  // Place node at angle around its ring. Equal spacing; first node
  // starts at the top (angle -90°) and goes clockwise.
  function nodePos(ring: number, idx: number, total: number): { x: number; y: number; angle: number } {
    const angle = -Math.PI / 2 + (idx / Math.max(1, total)) * 2 * Math.PI;
    const r = RING_RADIUS[ring];
    return {
      x: CX + r * Math.cos(angle),
      y: CY + r * Math.sin(angle),
      angle
    };
  }

  // Label position — pushed outward from the node so it doesn't
  // overlap with the node circle. For nodes on the left half we
  // anchor text to the right; right half anchors left; top/bottom
  // are anchored middle.
  function labelPos(p: { x: number; y: number; angle: number }, ring: number): { x: number; y: number; anchor: string } {
    const offset = 16;
    const x = CX + (RING_RADIUS[ring] + offset) * Math.cos(p.angle);
    const y = CY + (RING_RADIUS[ring] + offset) * Math.sin(p.angle);
    const cos = Math.cos(p.angle);
    let anchor = 'middle';
    if (cos > 0.3) anchor = 'start';
    else if (cos < -0.3) anchor = 'end';
    return { x, y, anchor };
  }

  function selectNode(id: string) {
    if (!roots) return;
    const n = roots.nodes?.find((x) => x.id === id);
    if (!n) return;
    selectedId = id;
    editLabel = n.label;
    editDescription = n.description ?? '';
    editScripture = n.scripture ?? '';
    editRelated = (n.related_notes ?? []).join('\n');
  }

  function selectCenter() {
    selectedId = null;
  }

  function deselect() {
    selectedId = undefined;
  }

  function addNode(ring: number) {
    if (!roots) return;
    // Empty ID — server stamps one on PUT.
    const newNode: RootsNode = {
      id: `tmp-${Math.random().toString(36).slice(2, 10)}`,
      ring,
      label: 'New',
      description: '',
      scripture: '',
      related_notes: [],
      created_at: '',
      updated_at: ''
    };
    roots.nodes = [...(roots.nodes ?? []), newNode];
    selectNode(newNode.id);
    scheduleSave();
  }

  function deleteSelected() {
    if (!roots || !selectedId) return;
    roots.nodes = (roots.nodes ?? []).filter((n) => n.id !== selectedId);
    selectedId = undefined;
    scheduleSave();
  }

  // Edit-buffer → record propagation. Each input writes through
  // these handlers so the SVG re-renders live.
  function syncFromBuffers() {
    if (!roots || !selectedId) return;
    const n = roots.nodes?.find((x) => x.id === selectedId);
    if (!n) return;
    n.label = editLabel.trim() || 'Unnamed';
    n.description = editDescription;
    n.scripture = editScripture.trim();
    n.related_notes = editRelated
      .split('\n')
      .map((s) => s.trim())
      .filter(Boolean);
    roots.nodes = [...roots.nodes!]; // trigger reactivity
    scheduleSave();
  }

  function syncCenter() {
    if (!roots) return;
    roots.center = centerEdit;
    roots.anchor = anchorEdit;
    scheduleSave();
  }
</script>

<svelte:head>
  <title>Roots · granit</title>
</svelte:head>

<div class="h-full overflow-y-auto bg-mantle">
  <div class="max-w-7xl mx-auto px-4 py-6">
    <header class="mb-4">
      <h1 class="text-2xl font-semibold text-text">Roots</h1>
      <p class="text-sm text-dim mt-1 max-w-prose">
        Identity, callings, gifts, longings — named in Christ. Not a stats tree.
        Add what's true, leave what isn't.
      </p>
    </header>

    {#if loading}
      <p class="text-sm text-dim italic">loading…</p>
    {:else if loadError}
      <p class="text-sm text-error">{loadError}</p>
    {:else if roots}
      <div class="grid grid-cols-1 lg:grid-cols-[1fr_22rem] gap-4">
        <!-- Radial canvas. -->
        <div class="bg-base border border-surface1 rounded-lg p-2 sm:p-4">
          <svg
            viewBox="0 0 {CANVAS} {CANVAS}"
            class="w-full h-auto"
            role="img"
            aria-label="Roots diagram"
          >
            <!-- Rings (concentric strokes). -->
            {#each [4, 3, 2, 1] as ring (ring)}
              <circle
                cx={CX}
                cy={CY}
                r={RING_RADIUS[ring]}
                class="fill-none {RING_COLORS[ring].stroke}"
                stroke-width="1"
                stroke-dasharray={ring === 4 ? '2 6' : ''}
              />
            {/each}

            <!-- Ring labels on the right axis, faint, vertical air. -->
            {#each [1, 2, 3, 4] as ring (ring)}
              <text
                x={CX + RING_RADIUS[ring] - 4}
                y={CY + 4}
                text-anchor="end"
                class="text-[11px] fill-dim font-mono uppercase tracking-wider opacity-60 pointer-events-none"
              >{roots.ring_labels[ring]}</text>
            {/each}

            <!-- Center. -->
            <g
              role="button"
              tabindex="0"
              onclick={selectCenter}
              onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); selectCenter(); } }}
              class="cursor-pointer"
            >
              <circle
                cx={CX}
                cy={CY}
                r="55"
                class="fill-base stroke-text"
                stroke-width={selectedId === null ? '2.5' : '1.5'}
              />
              <text
                x={CX}
                y={CY - 4}
                text-anchor="middle"
                class="text-[22px] fill-text font-serif"
              >{roots.center || 'Christ'}</text>
              {#if roots.anchor}
                <text
                  x={CX}
                  y={CY + 16}
                  text-anchor="middle"
                  class="text-[10px] fill-dim font-mono"
                >{roots.anchor}</text>
              {/if}
            </g>

            <!-- Nodes per ring. -->
            {#each [1, 2, 3, 4] as ring (ring)}
              {@const ringNodes = nodesOnRing(ring)}
              {#each ringNodes as node, idx (node.id)}
                {@const p = nodePos(ring, idx, ringNodes.length)}
                {@const lp = labelPos(p, ring)}
                <g
                  role="button"
                  tabindex="0"
                  onclick={() => selectNode(node.id)}
                  onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); selectNode(node.id); } }}
                  class="cursor-pointer"
                >
                  <circle
                    cx={p.x}
                    cy={p.y}
                    r={selectedId === node.id ? 11 : 8}
                    class="{RING_COLORS[ring].node} stroke-base"
                    stroke-width="2"
                  />
                  <text
                    x={lp.x}
                    y={lp.y}
                    text-anchor={lp.anchor}
                    dominant-baseline="middle"
                    class="text-[13px] fill-text font-medium"
                  >{node.label}</text>
                  {#if node.scripture}
                    <text
                      x={lp.x}
                      y={lp.y + 14}
                      text-anchor={lp.anchor}
                      dominant-baseline="middle"
                      class="text-[10px] fill-dim font-mono"
                    >{node.scripture}</text>
                  {/if}
                </g>
              {/each}
            {/each}
          </svg>

          <!-- Add buttons — one per ring, color-coded to match. -->
          <div class="mt-4 grid grid-cols-2 sm:grid-cols-4 gap-2">
            {#each [1, 2, 3, 4] as ring (ring)}
              <button
                type="button"
                onclick={() => addNode(ring)}
                class="text-xs px-2.5 py-1.5 rounded border border-surface1 bg-surface0 hover:bg-surface1 text-subtext"
                title="add to {roots.ring_labels[ring]}"
              >+ {roots.ring_labels[ring]}</button>
            {/each}
          </div>

          {#if saving}
            <p class="mt-2 text-[11px] text-dim italic">saving…</p>
          {/if}
        </div>

        <!-- Side panel — center editor / node editor / hint. -->
        <aside class="bg-base border border-surface1 rounded-lg p-4 lg:sticky lg:top-4 self-start">
          {#if selectedId === null}
            <!-- Center editor. -->
            <header class="flex items-baseline gap-2 mb-3">
              <h2 class="text-sm font-semibold text-text">Center</h2>
              <span class="text-xs text-dim">the name everything else is around</span>
            </header>
            <label class="block mb-3">
              <span class="text-xs uppercase tracking-wider text-dim">Name</span>
              <input
                type="text"
                bind:value={centerEdit}
                oninput={syncCenter}
                placeholder="Christ"
                class="mt-1 w-full px-2.5 py-1.5 bg-surface0 border border-surface1 rounded text-text focus:outline-none focus:border-primary"
              />
            </label>
            <label class="block mb-3">
              <span class="text-xs uppercase tracking-wider text-dim">Anchor (scripture)</span>
              <input
                type="text"
                bind:value={anchorEdit}
                oninput={syncCenter}
                placeholder="Col 1:17"
                class="mt-1 w-full px-2.5 py-1.5 bg-surface0 border border-surface1 rounded text-text focus:outline-none focus:border-primary font-mono"
              />
            </label>
            <button
              type="button"
              onclick={deselect}
              class="text-xs text-dim hover:text-text underline"
            >close</button>
          {:else if selectedId !== undefined}
            <!-- Node editor. -->
            {@const sel = roots.nodes?.find((n) => n.id === selectedId)}
            {#if sel}
              <header class="flex items-baseline gap-2 mb-3">
                <h2 class="text-sm font-semibold text-text">{roots.ring_labels[sel.ring]}</h2>
                <button
                  type="button"
                  onclick={deselect}
                  class="ml-auto text-xs text-dim hover:text-text"
                >close</button>
              </header>
              <label class="block mb-3">
                <span class="text-xs uppercase tracking-wider text-dim">Name</span>
                <input
                  type="text"
                  bind:value={editLabel}
                  oninput={syncFromBuffers}
                  class="mt-1 w-full px-2.5 py-1.5 bg-surface0 border border-surface1 rounded text-text focus:outline-none focus:border-primary"
                />
              </label>
              <label class="block mb-3">
                <span class="text-xs uppercase tracking-wider text-dim">What is true here</span>
                <textarea
                  bind:value={editDescription}
                  oninput={syncFromBuffers}
                  rows="3"
                  placeholder="A sentence the user could read aloud, slowly, without flinching."
                  class="mt-1 w-full px-2.5 py-1.5 bg-surface0 border border-surface1 rounded text-text focus:outline-none focus:border-primary resize-y leading-relaxed"
                ></textarea>
              </label>
              <label class="block mb-3">
                <span class="text-xs uppercase tracking-wider text-dim">Scripture</span>
                <input
                  type="text"
                  bind:value={editScripture}
                  oninput={syncFromBuffers}
                  placeholder="Eph 1:4"
                  class="mt-1 w-full px-2.5 py-1.5 bg-surface0 border border-surface1 rounded text-text focus:outline-none focus:border-primary font-mono"
                />
              </label>
              <label class="block mb-3">
                <span class="text-xs uppercase tracking-wider text-dim">Related notes</span>
                <textarea
                  bind:value={editRelated}
                  oninput={syncFromBuffers}
                  rows="2"
                  placeholder="One vault path per line — e.g. People/John.md"
                  class="mt-1 w-full px-2.5 py-1.5 bg-surface0 border border-surface1 rounded text-text focus:outline-none focus:border-primary resize-y font-mono text-xs"
                ></textarea>
              </label>
              <button
                type="button"
                onclick={deleteSelected}
                class="text-xs text-error hover:underline"
              >remove this</button>
            {/if}
          {:else}
            <!-- Empty selection. -->
            <h2 class="text-sm font-semibold text-text mb-2">Begin where you are.</h2>
            <p class="text-xs text-subtext leading-relaxed">
              Click the center to name what — or Whom — everything is rooted in.
              Click any item in a ring to edit it. Use the buttons below the diagram
              to add a new item to Identity, Callings, Gifts, or Longings.
            </p>
            <p class="text-xs text-dim mt-4 leading-relaxed">
              This page does not score, rate, or measure. It is a place to read your
              own rootedness slowly and let it correct you. Add only what is true.
            </p>
          {/if}
        </aside>
      </div>
    {/if}
  </div>
</div>
