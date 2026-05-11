<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { goto } from '$app/navigation';
  import { api, type Project, type Goal, type SearchHit } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { fuzzyScoreMulti } from '$lib/util/fuzzy';
  import { loadStored, saveStored } from '$lib/util/storage';
  import { openAIOverlay } from '$lib/stores/ai-overlay';
  import NavIcon from './NavIcon.svelte';

  // ── Surface name & history ──────────────────────────────────────────
  // Originally a notes-only quick switcher. As of this iteration the
  // palette is a universal navigator: Pages + Projects + Goals + Notes
  // + Agent commands all in one list. Mod-P opens it in "jump" mode
  // (the fast path — every section indexed, fuzzy filter, ↵ to invoke);
  // Mod-K opens it in the same mode (the two keybinds converge here
  // because the old "actions only" Mod-K never carried its weight).
  // Recent picks bubble to the top within each section, persisted to
  // localStorage so muscle memory survives across reloads.

  type Group = 'Pages' | 'Projects' | 'Goals' | 'Notes' | 'Agents' | 'Content';

  interface CmdItem {
    /** Stable ID — also the localStorage recent key. Pages: 'page:/path',
     *  Projects: 'project:<name>', Goals: 'goal:<id>', Notes: 'note:<path>',
     *  Agents: 'agent:<slug>'. Content (search hits) are excluded from
     *  recents because they're query-driven, not destinations. */
    id: string;
    label: string;
    detail?: string;
    icon: string;
    group: Group;
    /** Keyboard hint rendered on the right (e.g. 'Mod+P', 'a' on /tasks).
     *  Optional — most items don't carry a hotkey. */
    hint?: string;
    /** Pure side-effect. Closes the palette before running so the caller
     *  doesn't have to. */
    run: () => void | Promise<void>;
  }

  let open = $state(false);
  let q = $state('');
  let selected = $state(0);
  let inputEl: HTMLInputElement | undefined = $state();

  // ── Data caches ────────────────────────────────────────────────────
  // Loaded lazily on first open. WS events refresh in the background
  // so subsequent opens see fresh state. Empty arrays render nothing
  // for that section.
  let notes = $state<{ path: string; title: string }[]>([]);
  let projects = $state<Project[]>([]);
  let goals = $state<Goal[]>([]);
  let dataLoaded = $state(false);

  // Live full-text search results — only consulted when the query is
  // ≥3 chars. Surfaces ABOVE everything else (the user typed something
  // specific enough to want body matches), but in its own section so
  // the noise stays contained.
  let searchHits = $state<SearchHit[]>([]);
  let searchToken = 0;
  async function runSearch(query: string) {
    const token = ++searchToken;
    if (query.trim().length < 3) {
      searchHits = [];
      return;
    }
    try {
      const r = await api.search(query, 12);
      if (token !== searchToken) return; // stale response, ignore
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

  async function loadData() {
    // Fire all three in parallel — the slowest will gate dataLoaded.
    // Errors per-source are swallowed so a /goals 500 doesn't kill
    // the whole switcher (the user can still jump to pages + notes).
    const np = api.listNotes({ limit: 30 }).then(
      (r) => { notes = r.notes.map((n) => ({ path: n.path, title: n.title })); },
      () => {}
    );
    const pp = api.listProjects().then(
      (r) => { projects = r.projects; },
      () => {}
    );
    const gp = api.listGoals().then(
      (r) => { goals = r.goals; },
      () => {}
    );
    await Promise.allSettled([np, pp, gp]);
    dataLoaded = true;
  }

  // ── Recents persistence ────────────────────────────────────────────
  // Stored as an ID-array ordered most-recent-first. We cap aggressively
  // (24 entries) so localStorage stays small and ranking stays meaningful
  // — a recent that hasn't been used in months doesn't deserve to crowd
  // out the freshly-touched ones.
  const RECENT_KEY = 'granit.quickswitcher.recent';
  const RECENT_CAP = 24;
  let recents = $state<string[]>(loadStored<string[]>(RECENT_KEY, []));

  function bumpRecent(id: string) {
    const next = [id, ...recents.filter((r) => r !== id)].slice(0, RECENT_CAP);
    recents = next;
    saveStored(RECENT_KEY, next);
  }

  // ── Page index ─────────────────────────────────────────────────────
  // Every top-level destination in the app. The set mirrors what the
  // sidebar exposes (see +layout.svelte sections), plus the few
  // utility routes that aren't in the nav (sabbath, stats) so the
  // switcher can reach them.
  //
  // Icon glyphs use simple Unicode rather than NavIcon SVGs because
  // the switcher row is tight horizontally — a single character keeps
  // the list scannable. The icon's job here is recognition, not
  // pixel-perfect parity with the sidebar.
  const PAGES: { path: string; label: string; icon: string }[] = [
    { path: '/', label: 'Today', icon: 'today' },
    { path: '/morning', label: 'Morning', icon: 'morning' },
    { path: '/tasks', label: 'Tasks', icon: 'tasks' },
    { path: '/calendar', label: 'Calendar', icon: 'calendar' },
    { path: '/jots', label: 'Jots', icon: 'jots' },
    { path: '/habits', label: 'Habits', icon: 'habits' },
    { path: '/examen', label: 'Examen', icon: 'examen' },
    { path: '/vision', label: 'Vision', icon: 'vision' },
    { path: '/review', label: 'Review', icon: 'review' },
    { path: '/goals', label: 'Goals', icon: 'goals' },
    { path: '/deadlines', label: 'Deadlines', icon: 'deadline' },
    { path: '/projects', label: 'Projects', icon: 'projects' },
    { path: '/ventures', label: 'Ventures', icon: 'ventures' },
    { path: '/finance', label: 'Finance', icon: 'finance' },
    { path: '/shopping', label: 'Shopping', icon: 'shopping' },
    { path: '/hub', label: 'Hub', icon: 'hub' },
    { path: '/people', label: 'People', icon: 'people' },
    { path: '/measurements', label: 'Metrics', icon: 'measurements' },
    { path: '/prayer', label: 'Prayer', icon: 'prayer' },
    { path: '/scripture', label: 'Scripture', icon: 'scripture' },
    { path: '/notes', label: 'Notes', icon: 'notes' },
    { path: '/search', label: 'Search', icon: 'search' },
    { path: '/books', label: 'Books', icon: 'books' },
    { path: '/templates', label: 'Templates', icon: 'templates' },
    { path: '/objects', label: 'Objects', icon: 'objects' },
    { path: '/tags', label: 'Tags', icon: 'tags' },
    { path: '/agents', label: 'Agents', icon: 'agents' },
    { path: '/chat', label: 'Chat', icon: 'chat' },
    { path: '/sabbath', label: 'Sabbath', icon: 'prayer' },
    { path: '/stats', label: 'Stats', icon: 'stats' },
    { path: '/settings', label: 'Settings', icon: 'settings' }
  ];

  // ── Agent commands ─────────────────────────────────────────────────
  // Each entry either (a) opens the AI overlay with a seeded prompt
  // (briefing/triage/find-time), (b) switches the overlay into a
  // specific mode (PM, Goal Manager, Coach, etc.), or (c) navigates
  // to the page that owns the agent (Task Agent lives on /tasks).
  //
  // For contextual modes (project-manager, goal-manager,
  // calendar-manager) we navigate to the matching page first so the
  // prelude can pick up entity context; the overlay then opens in that
  // mode. The `text: ''` seed keeps the composer empty — the user has
  // chosen the posture, not the prompt.
  interface AgentCmd {
    slug: string;
    label: string;
    detail: string;
    icon: string;
    hint?: string;
    run: () => void | Promise<void>;
  }

  const AGENTS: AgentCmd[] = [
    {
      slug: 'briefing',
      label: 'Run daily briefing',
      detail: 'Top 3 focus items + one thing you might forget',
      icon: 'morning',
      run: () =>
        openAIOverlay({
          text: 'Give me a short morning briefing — top three things I should focus on today and one thing I might be forgetting.',
          send: true
        })
    },
    {
      slug: 'triage',
      label: 'Triage open tasks',
      detail: 'Pick 3 for today, defer or delete the rest',
      icon: 'tasks',
      run: () =>
        openAIOverlay({
          text: 'Help me triage my open tasks — which 3 should I do today, and what should I defer or delete?',
          send: true
        })
    },
    {
      slug: 'find-time',
      label: 'Find time for deep work',
      detail: '60-minute slots in the next 3 days',
      icon: 'calendar',
      run: () =>
        openAIOverlay({
          modeId: 'analyst',
          text: 'Find me 60 minutes for deep work in the next 3 days. List 3 candidate slots.',
          send: false
        })
    },
    {
      slug: 'project-manager',
      label: 'Open Project Manager mode',
      detail: 'PM coach — go to /projects',
      icon: 'projects',
      run: async () => {
        await goto('/projects');
        openAIOverlay({ modeId: 'project-manager', text: '' });
      }
    },
    {
      slug: 'goal-manager',
      label: 'Open Goal Manager mode',
      detail: 'Goal coach — go to /goals',
      icon: 'goals',
      run: async () => {
        await goto('/goals');
        openAIOverlay({ modeId: 'goal-manager', text: '' });
      }
    },
    {
      slug: 'calendar-manager',
      label: 'Open Calendar Manager mode',
      detail: 'Schedule strategist — go to /calendar',
      icon: 'calendar',
      run: async () => {
        await goto('/calendar');
        openAIOverlay({ modeId: 'calendar-manager', text: '' });
      }
    },
    {
      slug: 'task-agent',
      label: 'Open Task Agent on /tasks',
      detail: 'Bulk task ops — press ‘a’ on the tasks page',
      icon: 'tasks',
      hint: 'a',
      run: () => goto('/tasks')
    },
    // Posture-only modes — open the overlay with a different system
    // prompt but no seeded text. The user picks their question.
    {
      slug: 'mode-coach',
      label: 'Switch chat to Coach mode',
      detail: 'Socratic — questions over answers',
      icon: 'chat',
      run: () => openAIOverlay({ modeId: 'coach', text: '' })
    },
    {
      slug: 'mode-research',
      label: 'Switch chat to Research mode',
      detail: 'Grounded answers from your vault',
      icon: 'search',
      run: () => openAIOverlay({ modeId: 'research', text: '' })
    },
    {
      slug: 'mode-writer',
      label: 'Switch chat to Writer mode',
      detail: 'Drafting partner that matches your voice',
      icon: 'jots',
      run: () => openAIOverlay({ modeId: 'writer', text: '' })
    },
    {
      slug: 'mode-analyst',
      label: 'Switch chat to Analyst mode',
      detail: 'Evidence-first — what does the data say',
      icon: 'stats',
      run: () => openAIOverlay({ modeId: 'analyst', text: '' })
    },
    {
      slug: 'mode-architect',
      label: 'Switch chat to Architect mode',
      detail: 'System design with named trade-offs',
      icon: 'objects',
      run: () => openAIOverlay({ modeId: 'architect', text: '' })
    }
  ];

  // ── Open / close ───────────────────────────────────────────────────
  export function show() {
    open = true;
    q = '';
    selected = 0;
    if (!dataLoaded) loadData();
    tick().then(() => inputEl?.focus());
  }
  function close() {
    open = false;
  }

  // ── Keybinds ───────────────────────────────────────────────────────
  // Mod-K and Mod-P both open the switcher in universal mode. Mod-P
  // historically meant "notes-only quick switcher"; we collapsed the
  // two surfaces because the new switcher is fast enough on the
  // notes-only case (subsequence match on 30 titles is instant) that
  // a dedicated mode-toggle no longer earns its complexity. Power
  // users keep their muscle memory: Mod-P, type a few chars, Enter.
  //
  // Mod-Shift-F still escapes to the dedicated /search page for
  // full-text deep dives; the switcher's Content section is the
  // inline preview, but it caps at 12 hits — go to /search for more.
  onMount(() => {
    const onKey = (e: KeyboardEvent) => {
      const meta = e.metaKey || e.ctrlKey;
      if (meta && !e.shiftKey && (e.key === 'k' || e.key === 'K')) {
        e.preventDefault();
        if (open) close();
        else show();
        return;
      }
      // Mod-P → same surface as Mod-K. Preempts the browser print
      // dialog globally; PrintPreview's own Mod-P handler runs in
      // the capture phase + stopImmediatePropagation so the print
      // overlay still wins when it's the focused surface.
      if (meta && !e.shiftKey && (e.key === 'p' || e.key === 'P')) {
        e.preventDefault();
        if (open) close();
        else show();
        return;
      }
      // Mod-Shift-F → full-text search. Skip when typing into an
      // input so the user can still type 'F' or use the browser's
      // Cmd-Shift-F if they want it.
      if (meta && e.shiftKey && (e.key === 'f' || e.key === 'F')) {
        const el = document.activeElement as HTMLElement | null;
        const tag = el?.tagName?.toLowerCase();
        if (tag === 'input' || tag === 'textarea' || el?.isContentEditable) return;
        e.preventDefault();
        void goto('/search');
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
        invoke(items[selected]);
        return;
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  // Live-refresh notes / projects / goals on WS events. Each event
  // type only refreshes the relevant slice — we don't want a single
  // task change to refetch goals + projects too.
  onMount(() =>
    onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') {
        api.listNotes({ limit: 30 }).then(
          (r) => { notes = r.notes.map((n) => ({ path: n.path, title: n.title })); },
          () => {}
        );
      }
      if (ev.type === 'project.changed' || ev.type === 'project.removed') {
        api.listProjects().then(
          (r) => { projects = r.projects; },
          () => {}
        );
      }
      // Goals live in .granit/goals.json — broadcast as state.changed.
      // We only refetch when the goals file specifically changes so a
      // habits sidecar write doesn't trigger a goals API roundtrip.
      if (ev.type === 'state.changed' && ev.path.endsWith('goals.json')) {
        api.listGoals().then(
          (r) => { goals = r.goals; },
          () => {}
        );
      }
    })
  );

  function invoke(item: CmdItem | undefined) {
    if (!item) return;
    // Bump recent BEFORE running the action so a same-tab navigation
    // doesn't race us to localStorage. Content hits aren't tracked —
    // they're query-driven, not destinations the user will repeat.
    if (item.group !== 'Content') bumpRecent(item.id);
    close();
    void item.run();
  }

  // ── Item building ──────────────────────────────────────────────────
  // Each section produces CmdItems independently, then merges into a
  // single list. Sorting is: group-rank (Content > Pages > Projects >
  // Goals > Notes > Agents) for empty/short queries, fuzzy-score within
  // a group. Recents get a +250 bump within their section so the user's
  // last-touched project floats above one they haven't opened in a year.
  //
  // The recency bonus is additive (not a hard floor) so an exact-match
  // on a fresh item still beats a stale recent — the muscle memory of
  // "I just typed three letters and got the right thing" is preserved
  // even when the recents list is large.

  const RECENT_BUMP = 250;

  function recencyBoost(id: string): number {
    const idx = recents.indexOf(id);
    if (idx < 0) return 0;
    // Linear decay over the recents cap so the most-recent gets the
    // full bump and the oldest in the list gets ~10% of it.
    return RECENT_BUMP * (1 - (idx / RECENT_CAP) * 0.9);
  }

  let items = $derived.by((): CmdItem[] => {
    const needle = q.trim();
    const out: CmdItem[] = [];

    // Pages — always indexed, even before data loads (they're static).
    for (const p of PAGES) {
      const sc = fuzzyScoreMulti(needle, [p.label, p.path]);
      if (sc === null) continue;
      const id = 'page:' + p.path;
      out.push({
        id,
        label: p.label,
        detail: p.path,
        icon: p.icon,
        group: 'Pages',
        run: () => goto(p.path)
      });
      // Stash the score on the item via a parallel map below — we
      // need it for sort but don't want it on the public shape.
      scoreMap.set(id, sc + recencyBoost(id));
    }

    // Projects
    for (const pr of projects) {
      const sc = fuzzyScoreMulti(needle, [pr.name, pr.description ?? '']);
      if (sc === null) continue;
      const id = 'project:' + pr.name;
      out.push({
        id,
        label: pr.name,
        detail: pr.description?.slice(0, 80),
        icon: 'projects',
        group: 'Projects',
        run: () => goto('/projects/' + encodeURIComponent(pr.name))
      });
      scoreMap.set(id, sc + recencyBoost(id));
    }

    // Goals
    for (const g of goals) {
      const sc = fuzzyScoreMulti(needle, [g.title, g.category ?? '']);
      if (sc === null) continue;
      const id = 'goal:' + g.id;
      out.push({
        id,
        label: g.title,
        detail: g.category ?? g.status,
        icon: 'goals',
        group: 'Goals',
        run: () => goto('/goals?focus=' + encodeURIComponent(g.id))
      });
      scoreMap.set(id, sc + recencyBoost(id));
    }

    // Notes (cap 30 from listNotes — already mod-time-desc on the
    // server, so for empty queries the freshest leads).
    for (let i = 0; i < notes.length; i++) {
      const n = notes[i];
      const sc = fuzzyScoreMulti(needle, [n.title, n.path]);
      if (sc === null) continue;
      const id = 'note:' + n.path;
      out.push({
        id,
        label: n.title,
        detail: n.path,
        icon: 'notes',
        group: 'Notes',
        run: () => goto('/notes/' + encodeURIComponent(n.path))
      });
      // Empty-needle: rank by mod-time (server-order). Push the
      // recency-bump on top so a freshly-touched note still leads.
      const empty = !needle;
      scoreMap.set(id, (empty ? 100 - i : sc) + recencyBoost(id));
    }

    // Agent commands
    for (const a of AGENTS) {
      const sc = fuzzyScoreMulti(needle, [a.label, a.detail]);
      if (sc === null) continue;
      const id = 'agent:' + a.slug;
      out.push({
        id,
        label: a.label,
        detail: a.detail,
        icon: a.icon,
        group: 'Agents',
        hint: a.hint,
        run: a.run
      });
      scoreMap.set(id, sc + recencyBoost(id));
    }

    // Content (full-text) — query-driven, no recents bump, scored by
    // result-list order (the API already ranks).
    for (let i = 0; i < searchHits.length; i++) {
      const h = searchHits[i];
      // Skip content hits whose path already appears as a Note title
      // hit — the title row is the better destination.
      const dupId = 'note:' + h.path;
      if (out.some((x) => x.id === dupId)) continue;
      const id = 'content:' + h.path + ':' + h.line;
      out.push({
        id,
        label: h.title,
        detail: h.matchLine,
        icon: 'search',
        group: 'Content',
        run: () => goto('/notes/' + encodeURIComponent(h.path))
      });
      scoreMap.set(id, 700 - i); // sits above mid-tier matches; trumps recents in its own section
    }

    out.sort((a, b) => {
      const ra = groupRank(a.group);
      const rb = groupRank(b.group);
      if (ra !== rb) return ra - rb;
      return (scoreMap.get(b.id) ?? 0) - (scoreMap.get(a.id) ?? 0);
    });
    // Cap to a sensible upper bound. 80 items is more than fits on
    // any screen — the user is going to type, not scroll past 80.
    return out.slice(0, 80);
  });

  // Side-table for derived scores, populated inside the derived. Kept
  // out of the CmdItem shape because consumers (templates) don't need
  // it — sort is the only reader. Map cleared implicitly each derive
  // because we never read entries we didn't write in this pass (and a
  // stale entry from a prior derive is harmless: it'd be overwritten
  // before being read, or never read at all if the item dropped out).
  const scoreMap = new Map<string, number>();

  function groupRank(g: Group): number {
    // Content (full-text body hits) wins when present — the user
    // typed something specific enough to want body matches.
    // Then Pages (fast jumps), Projects, Goals, Notes, Agents.
    if (g === 'Content') return 0;
    if (g === 'Pages') return 1;
    if (g === 'Projects') return 2;
    if (g === 'Goals') return 3;
    if (g === 'Notes') return 4;
    return 5; // Agents
  }

  // Group for visual headers
  let grouped = $derived.by(() => {
    const m: { group: Group; items: CmdItem[] }[] = [];
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
    <div class="px-4 py-3 border-b border-surface1 flex items-center gap-3 flex-shrink-0">
      <svg viewBox="0 0 24 24" class="w-4 h-4 text-dim flex-shrink-0" fill="none" stroke="currentColor" stroke-width="2">
        <circle cx="11" cy="11" r="8" /><path d="m21 21-4.3-4.3" stroke-linecap="round" />
      </svg>
      <input
        bind:this={inputEl}
        bind:value={q}
        placeholder="jump to a page, project, goal, note, or agent…"
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
          <div class="px-4 pt-3 pb-1 text-[10px] uppercase tracking-wider text-dim">{g.group}</div>
          <ul>
            {#each g.items as it, iIdx (it.id)}
              {@const flat = offset(gIdx) + iIdx}
              <li>
                <button
                  data-cmd-idx={flat}
                  onclick={() => invoke(it)}
                  onmouseenter={() => (selected = flat)}
                  class="w-full text-left px-4 py-2 flex items-baseline gap-3 {selected === flat ? 'bg-surface1' : ''}"
                >
                  <span class="w-5 flex items-center justify-center text-dim">
                    <NavIcon name={it.icon} class="w-4 h-4" />
                  </span>
                  <span class="flex-1 min-w-0 truncate text-text">{it.label}</span>
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

    <div class="px-4 py-2 text-[10px] text-dim border-t border-surface1 flex items-center justify-between flex-shrink-0">
      <span>↑↓ navigate · ↵ select · esc close</span>
      <span class="font-mono">{items.length}</span>
    </div>
  </div>
{/if}
