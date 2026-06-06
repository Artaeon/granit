<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { goto } from '$app/navigation';
  import {
    api,
    fmtDateISO,
    todayISO,
    type Project,
    type Goal,
    type SearchHit,
    type Task,
    type HabitInfo,
    type Deadline,
    type CalendarEvent
  } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { fuzzyScoreMulti } from '$lib/util/fuzzy';
  import { workspaceCommands } from '$lib/workspace/workspaceCommands';
  import NavIcon from './NavIcon.svelte';
  import type { Group, CmdItem } from './commandPalette/paletteTypes';
  import { createPaletteRecents } from './commandPalette/paletteRecents.svelte';
  import { PAGES, AGENTS } from './commandPalette/paletteCatalog';

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
  // Loaded lazily on first open. WS events refresh in the background
  // so subsequent opens see fresh state. Empty arrays render nothing
  // for that section.
  let notes = $state<{ path: string; title: string }[]>([]);
  let projects = $state<Project[]>([]);
  let goals = $state<Goal[]>([]);
  // Individual entity rows (Spotlight-style): open tasks, all habits,
  // active deadlines, upcoming events. Indexed alongside Pages so the
  // palette finds them with the same fuzzy filter — no separate UI.
  let tasks = $state<Task[]>([]);
  let habits = $state<HabitInfo[]>([]);
  let deadlines = $state<Deadline[]>([]);
  let events = $state<CalendarEvent[]>([]);
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
    // Fire all sources in parallel — the slowest gates dataLoaded.
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
    // Open tasks — fuzzy filter searches the text + project name. We
    // pull only open ones (closed tasks aren't actionable navigation
    // targets); the /tasks page is the canonical home if the user
    // wants archived/done. Cap defensively at 200 in case a heavy
    // user has hundreds open — the palette ranks by score so
    // a needle still finds the right row, but we don't ship a list
    // of 500 to fuzzyScore on every keystroke.
    const tp = api.listTasks({ status: 'open' }).then(
      (r) => { tasks = r.tasks.slice(0, 200); },
      () => {}
    );
    // All habits — usually <20, no cap needed.
    const hp = api.listHabits().then(
      (r) => { habits = r.habits; },
      () => {}
    );
    // Active deadlines only (met/cancelled clutter the list).
    const dp = api.tryListDeadlines().then(
      (r) => {
        const all = r ?? [];
        deadlines = all.filter((d) => d.status === 'active');
      },
      () => {}
    );
    // Calendar events — next 14 days. Covers "go to my meeting"
    // jumps. Past events aren't useful as nav targets; further-out
    // events go through the calendar page directly.
    const ep = fetchEventsWindow();
    await Promise.allSettled([np, pp, gp, tp, hp, dp, ep]);
    dataLoaded = true;
  }

  // Single window: today → today + 14 days. Shared by the initial
  // loadData() call and the WS-driven event.changed refetch so the
  // date math + cutoff stay in lockstep — previously the same loop
  // existed twice and a change to one side could drift.
  function fetchEventsWindow(): Promise<void> {
    const today = todayISO();
    const cutoffDate = new Date();
    cutoffDate.setDate(cutoffDate.getDate() + 14);
    return api.calendar(today, fmtDateISO(cutoffDate)).then(
      (r) => { events = r.events; },
      () => {}
    );
  }

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
      // Tab / Shift-Tab — jump to the first item of the next /
      // previous group. Power gesture for hopping past a long
      // Pages list into Tasks or Content without arrow-spamming.
      // Without modifiers so it never collides with browser tab-
      // navigation (the palette swallows focus while open).
      if (e.key === 'Tab') {
        e.preventDefault();
        if (grouped.length === 0) return;
        // Find the current group index from `selected`.
        let acc = 0;
        let curGroup = 0;
        for (let i = 0; i < grouped.length; i++) {
          const end = acc + grouped[i].items.length;
          if (selected < end) { curGroup = i; break; }
          acc = end;
        }
        const dir = e.shiftKey ? -1 : 1;
        const nextGroup = (curGroup + dir + grouped.length) % grouped.length;
        // Flat index of the first item in `nextGroup`.
        let offset = 0;
        for (let i = 0; i < nextGroup; i++) offset += grouped[i].items.length;
        selected = offset;
        scrollSelectedIntoView();
        return;
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  // Live-refresh every indexed slice on WS events. Each event type
  // only refreshes the relevant slice so a task change doesn't drag a
  // goals + projects + notes round-trip with it.
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
      // Tasks — any task mutation invalidates the open-tasks list.
      if (ev.type === 'task.changed') {
        api.listTasks({ status: 'open' }).then(
          (r) => { tasks = r.tasks.slice(0, 200); },
          () => {}
        );
      }
      // Habits sidecars live under .granit/habits/. A check-off or
      // habit-add fires state.changed with that prefix.
      if (ev.type === 'state.changed' && ev.path?.startsWith('.granit/habits/')) {
        api.listHabits().then(
          (r) => { habits = r.habits; },
          () => {}
        );
      }
      // Deadlines live in .granit/deadlines.json.
      if (ev.type === 'state.changed' && ev.path?.endsWith('deadlines.json')) {
        api.tryListDeadlines().then(
          (r) => {
            const all = r ?? [];
            deadlines = all.filter((d) => d.status === 'active');
          },
          () => {}
        );
      }
      // Calendar events — refetch the 14-day window on event mutations.
      // Skipping note.changed here on purpose: note writes fire on
      // every editor autosave keystroke, and piling a calendar refetch
      // onto every one would burn bandwidth for the rare case where
      // a note edit changes a scheduled-task event.
      if (ev.type === 'event.changed' || ev.type === 'event.removed') {
        void fetchEventsWindow();
      }
    })
  );

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
  // Each section produces CmdItems independently, then merges into a
  // single list. Sorting is: group-rank (Content > Pages > Projects >
  // Goals > Notes > Agents) for empty/short queries, fuzzy-score within
  // a group. Recents get an additive bump within their section (see
  // recents.recencyBoost) so the user's last-touched project floats
  // above one they haven't opened in a year — but exact-matches on
  // fresh items still beat stale recents.

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
      scoreMap.set(id, sc + recents.recencyBoost(id));
    }

    // Workspace commands — split / close / swap focused pane. Each
    // command's run is a thunk; reading workspaceCommands() inside
    // this $derived.by tracks the workspace store reactively so the
    // command list refreshes when focus / pane kind change.
    for (const wc of workspaceCommands()) {
      const sc = fuzzyScoreMulti(needle, [wc.label, wc.detail]);
      if (sc === null) continue;
      out.push({
        id: wc.id,
        label: wc.label,
        detail: wc.detail,
        icon: wc.icon,
        group: 'Workspace',
        run: wc.run
      });
      scoreMap.set(wc.id, sc + recents.recencyBoost(wc.id));
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
      scoreMap.set(id, sc + recents.recencyBoost(id));
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
      scoreMap.set(id, sc + recents.recencyBoost(id));
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
      scoreMap.set(id, (empty ? 100 - i : sc) + recents.recencyBoost(id));
    }

    // Tasks — open ones, indexed by text + project. Empty needle:
    // hide everything except recents so an empty palette doesn't dump
    // 100 task rows in the user's face (the /tasks page is for that).
    // With a needle: show every fuzzy match.
    for (const t of tasks) {
      const sc = fuzzyScoreMulti(needle, [t.text, t.projectId ?? '']);
      if (sc === null) continue;
      const id = 'task:' + t.id;
      const isRecent = recents.includes(id);
      if (!needle && !isRecent) continue;
      // Detail line: project (if any) + due date hint so the user
      // can pick the right task when the text alone is ambiguous.
      const bits: string[] = [];
      if (t.projectId) bits.push(t.projectId);
      if (t.dueDate) bits.push('due ' + t.dueDate);
      else if (t.scheduledStart) bits.push('at ' + t.scheduledStart.slice(11, 16));
      out.push({
        id,
        label: t.text,
        detail: bits.join(' · '),
        icon: 'tasks',
        group: 'Tasks',
        run: () => goto('/tasks?focus=' + encodeURIComponent(t.id))
      });
      scoreMap.set(id, sc + recents.recencyBoost(id));
    }

    // Calendar events — next 14 days. Detail line carries the
    // start time + a one-glance type glyph so two events with the
    // same title (recurring stand-up) read distinguishably.
    for (const ev of events) {
      const sc = fuzzyScoreMulti(needle, [ev.title, ev.location ?? '']);
      if (sc === null) continue;
      // Each event already carries either start (RFC3339) or date
      // (YYYY-MM-DD all-day). Compose a stable id from the strongest
      // available identifier.
      const stableId = ev.eventId || ev.taskId || `${ev.title}@${ev.start || ev.date}`;
      const id = 'event:' + stableId;
      const dateStr = (ev.start || ev.date || '').slice(0, 10);
      const timeStr = ev.start ? ev.start.slice(11, 16) : 'all-day';
      out.push({
        id,
        label: ev.title,
        detail: `${dateStr} · ${timeStr}${ev.location ? ' · ' + ev.location : ''}`,
        icon: 'calendar',
        group: 'Events',
        // /calendar doesn't yet read a `?date=` query param, so we
        // jump to the calendar page and let the user land on the
        // event from the visible day. Future v2: add date routing.
        run: () => goto('/calendar')
      });
      scoreMap.set(id, sc + recents.recencyBoost(id));
    }

    // Deadlines — active only (filtered at load time). Date proximity
    // matters more than fuzzy score for sort, so we lean on the date
    // string as a tiebreaker via score adjustment.
    for (const d of deadlines) {
      const sc = fuzzyScoreMulti(needle, [d.title, d.project ?? '', d.venture ?? '']);
      if (sc === null) continue;
      const id = 'deadline:' + d.id;
      const bits: string[] = [d.date];
      if (d.importance && d.importance !== 'normal') bits.push(d.importance);
      if (d.project) bits.push(d.project);
      out.push({
        id,
        label: d.title,
        detail: bits.join(' · '),
        icon: 'deadline',
        group: 'Deadlines',
        // /deadlines doesn't yet read ?focus — navigate to the page
        // and let the user scan. The detail line already shows the
        // distinguishing fields (date + importance + project).
        run: () => goto('/deadlines')
      });
      scoreMap.set(id, sc + recents.recencyBoost(id));
    }

    // Habits — usually a small set (<20). All show on empty needle so
    // the user can jump to the habits page with one keystroke.
    for (const h of habits) {
      const sc = fuzzyScoreMulti(needle, [h.name]);
      if (sc === null) continue;
      const id = 'habit:' + h.name;
      out.push({
        id,
        label: h.name,
        detail: h.doneToday ? 'done today' : `${h.currentStreak}d streak`,
        icon: 'habits',
        group: 'Habits',
        run: () => goto('/habits')
      });
      scoreMap.set(id, sc + recents.recencyBoost(id));
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
      scoreMap.set(id, sc + recents.recencyBoost(id));
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
    // Ordering encodes "what does the user most often want when they
    // type something into this palette". Content body hits win
    // because they're the most specific (the user typed enough to
    // get a body match). Pages next — keystroke-to-jump is the
    // headline use. Tasks above other entities because action items
    // are the highest-frequency navigation target. Then Events /
    // Deadlines (time-pressure surfaces), Projects + Goals
    // (structural anchors), Notes (the long tail), Habits +
    // Agents (lowest because their pages are reachable by keyboard
    // already; the palette rows are reach-from-anywhere fallbacks).
    if (g === 'Content') return 0;
    if (g === 'Pages') return 1;
    if (g === 'Tasks') return 2;
    if (g === 'Events') return 3;
    if (g === 'Deadlines') return 4;
    if (g === 'Projects') return 5;
    if (g === 'Goals') return 6;
    if (g === 'Notes') return 7;
    if (g === 'Habits') return 8;
    return 9; // Agents
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
