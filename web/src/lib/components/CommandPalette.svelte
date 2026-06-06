<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { workspaceCommands } from '$lib/workspace/workspaceCommands';
  import NavIcon from './NavIcon.svelte';
  import type { CmdItem } from './commandPalette/paletteTypes';
  import { createPaletteRecents } from './commandPalette/paletteRecents.svelte';
  import { PAGES, AGENTS } from './commandPalette/paletteCatalog';
  import { createPaletteData } from './commandPalette/paletteData.svelte';
  import { buildItems, groupItems } from './commandPalette/paletteItems';
  import { installPaletteKeyboard } from './commandPalette/installPaletteKeyboard';

  // ── Surface name & history ──────────────────────────────────────────
  // Originally a notes-only quick switcher. As of this iteration the
  // palette is a universal navigator: Pages + Projects + Goals + Notes
  // + Agent commands all in one list. Mod-P opens it in "jump" mode
  // (the fast path — every section indexed, fuzzy filter, ↵ to invoke);
  // Mod-K opens it in the same mode (the two keybinds converge here
  // because the old "actions only" Mod-K never carried its weight).
  // Recent picks bubble to the top within each section, persisted to
  // localStorage so muscle memory survives across reloads.

  let open = $state(false);
  let q = $state('');
  let selected = $state(0);
  let inputEl: HTMLInputElement | undefined = $state();

  // ── Data caches ────────────────────────────────────────────────────
  // Controller-owned — see paletteData.svelte for the seven indexed
  // slices, the lazy load() pipeline, the WS refresh wiring, and the
  // debounced full-text search. Loaded lazily on first open(); WS
  // events refresh in the background so subsequent opens see fresh
  // state.
  const data = createPaletteData();
  // Mirror state into local consts so the template + items derivation
  // read unchanged.
  const notes = $derived(data.notes);
  const projects = $derived(data.projects);
  const goals = $derived(data.goals);
  const tasks = $derived(data.tasks);
  const habits = $derived(data.habits);
  const deadlines = $derived(data.deadlines);
  const events = $derived(data.events);
  const searchHits = $derived(data.searchHits);
  const dataLoaded = $derived(data.dataLoaded);

  // Debounce the full-text search to 180ms after the last keystroke
  // so we don't pummel /api/search on every character. The
  // controller's runSearch enforces the ≥3-char gate + the
  // stale-response guard.
  let searchDebounce: ReturnType<typeof setTimeout> | undefined;
  $effect(() => {
    const q2 = q;
    if (searchDebounce) clearTimeout(searchDebounce);
    searchDebounce = setTimeout(() => data.runSearch(q2), 180);
  });

  // ── Recents persistence ────────────────────────────────────────────
  // Controller-owned — see paletteRecents.svelte for the cap, decay
  // curve, and persistence shape. The items derivation reads
  // recents.recencyBoost(id) per row; invoke() calls recents.bump(id)
  // before navigating so a same-tab redirect can't lose the write.
  const recents = createPaletteRecents();

  // PAGES / AGENTS catalogs — see ./commandPalette/paletteCatalog
  // for the full lists + the rationale per agent posture.

  // ── Open / close ───────────────────────────────────────────────────
  export function show() {
    open = true;
    q = '';
    selected = 0;
    if (!dataLoaded) void data.load();
    tick().then(() => inputEl?.focus());
  }
  function close() {
    open = false;
  }

  // ── Keybinds ───────────────────────────────────────────────────────
  // Global Mod-K / Mod-P toggle, Mod-Shift-F escape to /search, and
  // the in-palette arrow / Tab / Enter / Esc handlers all live in
  // installPaletteKeyboard. See its header for the priority rules.
  onMount(() =>
    installPaletteKeyboard({
      isOpen: () => open,
      getSelected: () => selected,
      setSelected: (n) => { selected = n; },
      getItems: () => items,
      getGrouped: () => grouped,
      open: show,
      close,
      invoke,
      scrollSelectedIntoView
    })
  );

  // Live-refresh every indexed slice on WS events — controller-owned
  // (see paletteData.installRefresh for the per-event-type slice
  // routing). Cleanup runs on unmount.
  onMount(() => data.installRefresh());

  function invoke(item: CmdItem | undefined) {
    if (!item) return;
    // Bump recent BEFORE running the action so a same-tab navigation
    // doesn't race us to localStorage. Content hits aren't tracked —
    // they're query-driven, not destinations the user will repeat.
    if (item.group !== 'Content') recents.bump(item.id);
    close();
    void item.run();
  }

  // ── Item building ──────────────────────────────────────────────────
  // Each section produces CmdItems independently in the pure builder
  // (see paletteItems.buildItems) and they merge into a single list.
  // Sorting is: group-rank (Content > Pages > Tasks > Events >
  // Deadlines > Projects > Goals > Notes > Habits > Agents/Workspace)
  // and fuzzy-score within a group. Recents get an additive bump
  // within their section so the user's last-touched project floats
  // above one they haven't opened in a year — but exact-matches on
  // fresh items still beat stale recents.
  //
  // The $derived.by wrapper keeps the data slice + workspaceCommands()
  // reads inside Svelte's tracking so the list rebuilds whenever any
  // input mutates.
  let items = $derived.by<CmdItem[]>(() =>
    buildItems({
      query: q,
      workspaceCmds: workspaceCommands(),
      pages: PAGES,
      agents: AGENTS,
      projects,
      goals,
      notes,
      tasks,
      events,
      deadlines,
      habits,
      searchHits,
      recencyBoost: (id) => recents.recencyBoost(id),
      isRecent: (id) => recents.includes(id)
    })
  );

  // Group for visual headers.
  let grouped = $derived(groupItems(items));

  // Reset selection when query changes
  $effect(() => {
    void q;
    selected = 0;
  });

  function scrollSelectedIntoView() {
    const el = document.querySelector(`[data-cmd-idx="${selected}"]`);
    el?.scrollIntoView({ block: 'nearest' });
  }
</script>

{#if open}
  <button
    onclick={close}
    aria-label="close"
    class="fixed inset-0 z-[60] bg-black/60 cursor-default"
  ></button>
  <div
    role="dialog"
    aria-modal="true"
    class="fixed left-1/2 top-[12vh] -translate-x-1/2 z-[61] w-[92vw] max-w-xl bg-mantle border border-surface1 rounded-xl shadow-2xl overflow-hidden flex flex-col max-h-[80vh]"
  >
    <div class="px-3 py-2 border-b border-surface1 flex items-center gap-3 flex-shrink-0">
      <svg viewBox="0 0 24 24" class="w-4 h-4 text-dim flex-shrink-0" fill="none" stroke="currentColor" stroke-width="2">
        <circle cx="11" cy="11" r="8" /><path d="m21 21-4.3-4.3" stroke-linecap="round" />
      </svg>
      <input
        bind:this={inputEl}
        bind:value={q}
        placeholder="jump to anything — task, event, deadline, page, project, goal, note, habit, agent…"
        class="flex-1 bg-transparent text-base sm:text-sm text-text placeholder-dim focus:outline-none"
      />
      <span class="text-[10px] text-dim font-mono px-1.5 py-0.5 bg-surface0 border border-surface1 rounded">esc</span>
    </div>

    <div class="flex-1 min-h-0 overflow-y-auto">
      {#if items.length === 0}
        <div class="px-4 py-6 text-sm text-dim">
          {dataLoaded ? 'no matches' : 'loading…'}
        </div>
      {:else}
        {@const offset = (gIdx: number) => grouped.slice(0, gIdx).reduce((s, g) => s + g.items.length, 0)}
        {#each grouped as g, gIdx (g.group)}
          <div class="px-4 pt-2 pb-0.5 text-[10px] uppercase tracking-wider text-dim flex items-center gap-1.5">
            <span>{g.group}</span>
            <!-- Hit count — at-a-glance density signal. The user
                 reads "PAGES 32 · CONTENT 8" and knows whether to
                 keep typing or Tab into a denser bucket. -->
            <span class="text-dim/70 font-mono normal-case">({g.items.length})</span>
          </div>
          <ul>
            {#each g.items as it, iIdx (it.id)}
              {@const flat = offset(gIdx) + iIdx}
              <li>
                <button
                  data-cmd-idx={flat}
                  onclick={() => invoke(it)}
                  onmouseenter={() => (selected = flat)}
                  class="w-full text-left px-4 py-1.5 flex items-baseline gap-2.5 {selected === flat ? 'bg-surface1' : ''}"
                >
                  <span class="w-5 h-5 flex items-center justify-center text-dim flex-shrink-0">
                    <NavIcon name={it.icon} class="w-4 h-4" />
                  </span>
                  <span class="flex-1 min-w-0 truncate text-text text-sm">{it.label}</span>
                  {#if it.hint}
                    <kbd class="text-[10px] text-dim font-mono px-1.5 py-0.5 bg-surface0 border border-surface1 rounded flex-shrink-0">{it.hint}</kbd>
                  {/if}
                  {#if it.detail}
                    <span class="hidden sm:inline text-xs text-dim font-mono truncate max-w-[40%]">{it.detail}</span>
                  {/if}
                </button>
              </li>
            {/each}
          </ul>
        {/each}
      {/if}
    </div>

    <!-- Slim cheat-sheet footer — keyboard hints for the four
         in-palette gestures. Power users learn them once;
         beginners pick up Tab + ? on accident. font-mono so the
         glyph row reads as keys, not prose. -->
    <div class="px-3 py-1.5 text-[10px] text-dim font-mono border-t border-surface1 flex items-center justify-between flex-shrink-0">
      <span>↑↓ navigate · ⇥ group · ↵ open · esc close · ? shortcuts</span>
      <span>{items.length}</span>
    </div>
  </div>
{/if}
