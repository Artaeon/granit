<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { goto } from '$app/navigation';
  import { api, type Note, type SearchHit } from '$lib/api';
  import { onWsEvent } from '$lib/ws';

  interface CmdItem {
    id: string;
    label: string;
    detail?: string;
    icon?: string;
    group: string;
    score: number;
    run: () => void | Promise<void>;
  }

  let open = $state(false);
  let q = $state('');
  let selected = $state(0);
  let inputEl: HTMLInputElement | undefined = $state();

  // Mode controls which groups participate in the result list.
  //   'all'   — everything: Pages, Actions, Notes, Content. Triggered
  //             by Mod-K. The full command palette UX.
  //   'notes' — Notes + Content only. Triggered by Mod-P, the
  //             Amplenote-style "quick switcher" — a focused note
  //             picker that overrides browser print so the user can
  //             jump to any note by title without scanning the tree.
  type PaletteMode = 'all' | 'notes';
  let mode = $state<PaletteMode>('all');

  // Cached vault titles for the "Notes" group. Refresh on WS events.
  let notes = $state<{ path: string; title: string }[]>([]);
  let notesLoaded = $state(false);

  async function loadNotes() {
    try {
      const r = await api.listNotes({ limit: 5000 });
      notes = r.notes.map((n) => ({ path: n.path, title: n.title }));
      notesLoaded = true;
    } catch {}
  }

  // Live full-text search results — debounced, only when query is meaningful
  let searchHits = $state<SearchHit[]>([]);
  let searchToken = 0;
  async function runSearch(query: string) {
    const token = ++searchToken;
    if (query.trim().length < 3) {
      searchHits = [];
      return;
    }
    try {
      const r = await api.search(query, 20);
      if (token !== searchToken) return; // stale
      searchHits = r.results;
    } catch {
      if (token === searchToken) searchHits = [];
    }
  }
  let searchDebounce: ReturnType<typeof setTimeout> | undefined;
  $effect(() => {
    const q2 = q;
    if (searchDebounce) clearTimeout(searchDebounce);
    searchDebounce = setTimeout(() => runSearch(q2), 180);
  });

  // Static actions/pages (always available)
  const pages: CmdItem[] = [
    { id: 'p-today', label: 'Today (dashboard)', group: 'Pages', icon: '◐', score: 0, run: () => goto('/') },
    { id: 'p-morning', label: 'Morning routine', group: 'Pages', icon: '☀', score: 0, run: () => goto('/morning') },
    { id: 'p-tasks', label: 'Tasks', group: 'Pages', icon: '✓', score: 0, run: () => goto('/tasks') },
    { id: 'p-calendar', label: 'Calendar', group: 'Pages', icon: '▦', score: 0, run: () => goto('/calendar') },
    { id: 'p-habits', label: 'Habits', group: 'Pages', icon: '◈', score: 0, run: () => goto('/habits') },
    { id: 'p-goals', label: 'Goals', group: 'Pages', icon: '◎', score: 0, run: () => goto('/goals') },
    { id: 'p-objects', label: 'Objects (typed notes)', group: 'Pages', icon: '◇', score: 0, run: () => goto('/objects') },
    { id: 'p-tags', label: 'Tags', group: 'Pages', icon: '#', score: 0, run: () => goto('/tags') },
    { id: 'p-notes', label: 'Notes (vault tree)', group: 'Pages', icon: '✎', score: 0, run: () => goto('/notes') }
  ];

  const actions: CmdItem[] = [
    {
      id: 'a-today-daily',
      label: "Open today's daily note",
      group: 'Actions',
      icon: '📅',
      score: 0,
      run: async () => {
        try {
          const n = await api.daily('today');
          goto(`/notes/${encodeURIComponent(n.path)}`);
        } catch {}
      }
    },
    {
      id: 'a-yesterday',
      label: "Open yesterday's daily note",
      group: 'Actions',
      icon: '📆',
      score: 0,
      run: async () => {
        try {
          const n = await api.daily('yesterday');
          goto(`/notes/${encodeURIComponent(n.path)}`);
        } catch {}
      }
    },
    {
      id: 'a-customize',
      label: 'Customize dashboard',
      group: 'Actions',
      icon: '⚙',
      score: 0,
      run: () => goto('/?edit=1')
    }
  ];

  // Open / close. Default mode mirrors the Mod-K behaviour ("all").
  // Callers (or the Mod-P keymap below) pass 'notes' to scope the
  // palette to the quick-switcher experience.
  export function show(asMode: PaletteMode = 'all') {
    open = true;
    mode = asMode;
    q = '';
    selected = 0;
    if (!notesLoaded) loadNotes();
    tick().then(() => inputEl?.focus());
  }
  function close() {
    open = false;
  }

  onMount(() => {
    const onKey = (e: KeyboardEvent) => {
      const meta = e.metaKey || e.ctrlKey;
      if (meta && e.key === 'k') {
        e.preventDefault();
        if (open) close();
        else show('all');
        return;
      }
      // Mod-P → quick switcher (notes-only). Overrides the browser
      // print dialog globally; PrintPreview's own Mod-P handler runs
      // in the capture phase + stopImmediatePropagation so it still
      // wins inside the print overlay (otherwise the user would lose
      // their way to Save-as-PDF). e.shiftKey check excludes
      // Mod-Shift-P which is reserved for future "fast print".
      if (meta && !e.shiftKey && (e.key === 'p' || e.key === 'P')) {
        e.preventDefault();
        if (open && mode === 'notes') close();
        else show('notes');
        return;
      }
      if (!open) return;
      if (e.key === 'Escape') {
        e.preventDefault();
        close();
        return;
      }
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        selected = Math.min(items.length - 1, selected + 1);
        scrollSelectedIntoView();
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        selected = Math.max(0, selected - 1);
        scrollSelectedIntoView();
        return;
      }
      if (e.key === 'Enter') {
        e.preventDefault();
        items[selected]?.run();
        close();
        return;
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  // Live-refresh notes cache
  onMount(() =>
    onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') loadNotes();
    })
  );

  // Fuzzy-ish scoring: exact > prefix > substring; shorter wins on tie.
  function score(needle: string, hay: string): number {
    if (!needle) return 1;
    const n = needle.toLowerCase();
    const h = hay.toLowerCase();
    if (h === n) return 1000;
    if (h.startsWith(n)) return 600 - h.length;
    const idx = h.indexOf(n);
    if (idx >= 0) return 200 - idx - h.length;
    return -1;
  }

  let items = $derived.by((): CmdItem[] => {
    const needle = q.trim();
    const all: CmdItem[] = [];

    // Notes-mode short-circuits Pages + Actions so the user gets a
    // focused jumper. Empty queries still surface notes (sorted by
    // server modTime so most-recent leads — that's the muscle-memory
    // win for power users: Mod-P, Enter, you're back where you were).
    if (mode === 'all') {
      for (const p of pages) {
        const sc = score(needle, p.label);
        if (sc >= 0) all.push({ ...p, score: sc });
      }
      for (const a of actions) {
        const sc = score(needle, a.label);
        if (sc >= 0) all.push({ ...a, score: sc });
      }
    }
    if (notesLoaded) {
      for (let i = 0; i < notes.length; i++) {
        const n = notes[i];
        const sc = Math.max(score(needle, n.title), score(needle, n.path));
        // In notes-mode an empty query still surfaces every note —
        // ranked by recency (notes is already modTime-desc per the
        // server). Tiny negative-index nudge so the first entry wins
        // when scores tie at the empty-needle = 1 baseline.
        const finalScore = needle ? sc - 50 : 100 - i;
        if (sc >= 0) {
          all.push({
            id: 'n-' + n.path,
            label: n.title,
            detail: n.path,
            group: 'Notes',
            icon: '✎',
            score: finalScore,
            run: () => goto('/notes/' + encodeURIComponent(n.path))
          });
        }
      }
    }
    // Full-text content matches — skip ones whose path already appears as a title hit
    const seenPaths = new Set(all.filter((x) => x.group === 'Notes').map((x) => (x.id.startsWith('n-') ? x.id.slice(2) : '')));
    for (const h of searchHits) {
      if (seenPaths.has(h.path)) continue;
      all.push({
        id: 'c-' + h.path + ':' + h.line,
        label: h.title,
        detail: h.matchLine,
        group: 'Content',
        icon: '⌕',
        score: -100, // always below notes/pages, but ranked among themselves by API order
        run: () => goto('/notes/' + encodeURIComponent(h.path))
      });
    }
    all.sort((a, b) => {
      // Pages first, then Actions, then Notes (title), then Content — within group by score desc
      const groupRank = (g: string) =>
        g === 'Pages' ? 0 : g === 'Actions' ? 1 : g === 'Notes' ? 2 : 3;
      if (groupRank(a.group) !== groupRank(b.group)) {
        return groupRank(a.group) - groupRank(b.group);
      }
      return b.score - a.score;
    });
    return all.slice(0, 80);
  });

  // Group for visual headers
  let grouped = $derived.by(() => {
    const m: { group: string; items: CmdItem[] }[] = [];
    for (const it of items) {
      const last = m[m.length - 1];
      if (last && last.group === it.group) last.items.push(it);
      else m.push({ group: it.group, items: [it] });
    }
    return m;
  });

  // Reset selection when query changes
  $effect(() => {
    void q;
    selected = 0;
  });

  function scrollSelectedIntoView() {
    const el = document.querySelector(`[data-cmd-idx="${selected}"]`);
    el?.scrollIntoView({ block: 'nearest' });
  }

  // global index map (for keyboard nav across groups)
  let flatIndex = $derived.by(() => {
    const out: number[] = [];
    let i = 0;
    for (const g of grouped) for (const _ of g.items) out.push(i++);
    return out;
  });
</script>

{#if open}
  <button
    onclick={close}
    aria-label="close"
    class="fixed inset-0 z-[60] bg-black/60 backdrop-blur-sm cursor-default"
  ></button>
  <div
    role="dialog"
    aria-modal="true"
    class="fixed left-1/2 top-[12vh] -translate-x-1/2 z-[61] w-[92vw] max-w-xl bg-mantle border border-surface1 rounded-xl shadow-2xl overflow-hidden"
  >
    <div class="px-4 py-3 border-b border-surface1 flex items-center gap-3">
      <svg viewBox="0 0 24 24" class="w-4 h-4 text-dim flex-shrink-0" fill="none" stroke="currentColor" stroke-width="2">
        <circle cx="11" cy="11" r="8" /><path d="m21 21-4.3-4.3" stroke-linecap="round" />
      </svg>
      <input
        bind:this={inputEl}
        bind:value={q}
        placeholder={mode === 'notes' ? 'jump to a note…' : 'jump to a page, note, or action…'}
        class="flex-1 bg-transparent text-base sm:text-sm text-text placeholder-dim focus:outline-none"
      />
      {#if mode === 'notes'}
        <span class="text-[10px] text-secondary font-mono px-1.5 py-0.5 bg-secondary/10 border border-secondary/30 rounded">notes</span>
      {/if}
      <span class="text-[10px] text-dim font-mono px-1.5 py-0.5 bg-surface0 border border-surface1 rounded">esc</span>
    </div>

    <div class="max-h-[60vh] overflow-y-auto">
      {#if items.length === 0}
        <div class="px-4 py-6 text-sm text-dim">no matches</div>
      {:else}
        {@const offset = (gIdx: number) => grouped.slice(0, gIdx).reduce((s, g) => s + g.items.length, 0)}
        {#each grouped as g, gIdx (g.group)}
          <div class="px-4 pt-3 pb-1 text-[10px] uppercase tracking-wider text-dim">{g.group}</div>
          <ul>
            {#each g.items as it, iIdx (it.id)}
              {@const flat = offset(gIdx) + iIdx}
              <li>
                <button
                  data-cmd-idx={flat}
                  onclick={() => { it.run(); close(); }}
                  onmouseenter={() => (selected = flat)}
                  class="w-full text-left px-4 py-2 flex items-baseline gap-3 {selected === flat ? 'bg-surface1' : ''}"
                >
                  <span class="w-5 text-center text-base">{it.icon ?? '·'}</span>
                  <span class="flex-1 min-w-0 truncate text-text">{it.label}</span>
                  {#if it.detail}
                    <span class="text-xs text-dim font-mono truncate">{it.detail}</span>
                  {/if}
                </button>
              </li>
            {/each}
          </ul>
        {/each}
      {/if}
    </div>

    <div class="px-4 py-2 text-[10px] text-dim border-t border-surface1 flex items-center justify-between">
      <span>↑↓ navigate · ↵ select · esc close</span>
      <span class="font-mono">{items.length}</span>
    </div>
  </div>
{/if}
