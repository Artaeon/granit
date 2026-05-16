<script lang="ts">
  // /roots — life-stats dashboard, centered on Christ.
  //
  // Four domains of a life under God: Spirit, Mind, Body, Vocation.
  // Each section pulls live data from the modules that own it (bible,
  // virtues, habits, measurements, books, goals, finance, sabbath)
  // and also surfaces hand-tended items the user adds for things
  // that don't have their own module yet (languages spoken, current
  // interests, longings, etc).
  //
  // History — this page started as a contemplative identity diagram
  // ("rooted in Christ", no numbers). The user explicitly overrode
  // that on 2026-05-16 and asked for the stats-dashboard shape.
  // The original contemplative form is preserved in the git history
  // and could be revived as a separate /identity surface later.

  import { onMount } from 'svelte';
  import {
    api,
    type Roots,
    type RootsNode,
    type FinOverview,
    type Goal,
    type Virtue,
    type HabitInfo,
    type BookShelfRow
  } from '$lib/api';
  import { errorMessage } from '$lib/util/errorMessage';
  import { toast } from '$lib/components/toast';

  // ── State for hand-tended record + live data per source ───────
  let roots = $state<Roots | null>(null);
  let bibleStreak = $state<{ current: number; longest: number; todayLogged: boolean } | null>(null);
  let virtues = $state<Virtue[]>([]);
  let habits = $state<HabitInfo[]>([]);
  let measurementSeries = $state<{ name: string; unit: string }[]>([]);
  let latestByMeasurement = $state<Record<string, { value: number; date: string }>>({});
  let books = $state<BookShelfRow[]>([]);
  let goals = $state<Goal[]>([]);
  let activeProjects = $state(0);
  let activeVentures = $state(0);
  let finance = $state<FinOverview | null>(null);
  let sabbathCount = $state(0);
  let prayerCount = $state(0);

  let loading = $state(true);
  let loadError = $state('');
  // Names of sources that failed to load so the UI can show a small
  // "some data missing" pill instead of silently rendering zeros.
  // Promise.allSettled never rejects on its own; without this the
  // user can't tell "no goals yet" from "/api/v1/goals 500'd".
  let partialFailures = $state<string[]>([]);

  // ── Add-item form per ring ────────────────────────────────────
  // Inline composer per section. Keeps the page single-purpose
  // (everything happens here) instead of opening a separate modal.
  let addingFor = $state<number | null>(null);
  let addLabel = $state('');

  onMount(() => { void load(); });

  async function load() {
    loading = true;
    loadError = '';
    try {
      // Fire every source in parallel — the dashboard is read-mostly
      // so latency is dominated by the slowest endpoint. A failure
      // in one source shouldn't blank the whole page; settle catches
      // partial failure and we render whatever did come back.
      const results = await Promise.allSettled([
        api.getRoots(),
        api.bibleStreak(),
        api.listVirtues(),
        api.listHabits(),
        api.listMeasurementSeries(),
        api.listBooks(),
        api.listGoals(),
        api.listProjects(),
        api.listVentures(),
        api.finOverview(),
        api.getSabbathLog(),
        api.listPrayer()
      ]);
      const [r, bs, vs, hs, ms, bks, gs, projs, vens, fin, slog, pry] = results;
      // Names line up with the array order so we can flag failures
      // by index without re-listing each variable.
      const sourceNames = ['roots', 'bible', 'virtues', 'habits', 'measurements', 'books', 'goals', 'projects', 'ventures', 'finance', 'sabbath', 'prayer'];
      partialFailures = results
        .map((res, i) => (res.status === 'rejected' ? sourceNames[i] : null))
        .filter((x): x is string => x !== null);
      if (r.status === 'fulfilled') roots = r.value;
      if (bs.status === 'fulfilled') bibleStreak = bs.value;
      if (vs.status === 'fulfilled') virtues = vs.value.virtues;
      if (hs.status === 'fulfilled') habits = hs.value.habits;
      if (ms.status === 'fulfilled') measurementSeries = ms.value.series;
      if (bks.status === 'fulfilled') books = bks.value.books;
      if (gs.status === 'fulfilled') goals = gs.value.goals;
      if (projs.status === 'fulfilled') activeProjects = projs.value.projects.filter((p) => p.status !== 'archived' && p.status !== 'completed').length;
      if (vens.status === 'fulfilled') activeVentures = vens.value.ventures.length;
      if (fin.status === 'fulfilled') finance = fin.value;
      if (slog.status === 'fulfilled') sabbathCount = slog.value.entries.filter((e) => e.event === 'begin').length;
      if (pry.status === 'fulfilled') prayerCount = pry.value.intentions.filter((i) => i.status === 'praying').length;

      // Latest value per measurement series — one fetch per series
      // (capped at the top 6 to keep the dashboard snappy). We render
      // whatever resolves; missing series just don't show a value.
      if (measurementSeries.length > 0) {
        const top = measurementSeries.slice(0, 6);
        const entrySettled = await Promise.allSettled(
          top.map((s) => api.listMeasurementEntries({ series: s.name, limit: 1 }))
        );
        const acc: Record<string, { value: number; date: string }> = {};
        entrySettled.forEach((res, i) => {
          if (res.status === 'fulfilled' && res.value.entries.length > 0) {
            const e = res.value.entries[0];
            acc[top[i].name] = { value: e.value, date: e.date };
          }
        });
        latestByMeasurement = acc;
      }
    } catch (e) {
      loadError = errorMessage(e);
    } finally {
      loading = false;
    }
  }

  // ── Hand-tended item editing ──────────────────────────────────
  async function addItem(ring: number) {
    const label = addLabel.trim();
    if (!label || !roots) return;
    const newNode: Partial<RootsNode> = { ring, label };
    try {
      const updated = await api.putRoots({
        center: roots.center,
        anchor: roots.anchor,
        nodes: [...(roots.nodes ?? []), newNode] as Partial<RootsNode>[]
      });
      roots = updated;
      addLabel = '';
      addingFor = null;
    } catch (e) {
      toast.error(errorMessage(e));
    }
  }

  async function removeItem(id: string) {
    if (!roots) return;
    const next = (roots.nodes ?? []).filter((n) => n.id !== id);
    try {
      const updated = await api.putRoots({
        center: roots.center,
        anchor: roots.anchor,
        nodes: next
      });
      roots = updated;
    } catch (e) {
      toast.error(errorMessage(e));
    }
  }

  // ── Derived helpers ───────────────────────────────────────────
  let nodesByRing = $derived.by(() => {
    const acc: Record<number, RootsNode[]> = { 1: [], 2: [], 3: [], 4: [] };
    for (const n of roots?.nodes ?? []) {
      if (acc[n.ring]) acc[n.ring].push(n);
    }
    return acc;
  });

  let activeGoals = $derived(goals.filter((g) => g.status !== 'completed' && g.status !== 'archived'));
  let currentlyReading = $derived(books.filter((b) => {
    // BookShelfRow has a `status` field if the user marked one; the
    // shape is loose here — we look for any in-progress signal.
    const s = (b as unknown as { status?: string }).status;
    return s === 'reading' || s === 'in_progress';
  }));
  let booksThisYear = $derived(() => {
    const year = new Date().getFullYear();
    return books.filter((b) => {
      const f = (b as unknown as { finished_at?: string }).finished_at;
      return f && f.startsWith(String(year));
    }).length;
  });

  function eur(cents: number): string {
    if (!Number.isFinite(cents)) return '—';
    const v = cents / 100;
    if (Math.abs(v) >= 1_000_000) return `€${(v / 1_000_000).toFixed(2)}M`;
    if (Math.abs(v) >= 1_000) return `€${(v / 1_000).toFixed(1)}k`;
    return `€${v.toFixed(0)}`;
  }

  // ── Ring styling ──────────────────────────────────────────────
  type RingMeta = { num: number; key: string; title: string; href: string; accent: string };
  const RINGS: RingMeta[] = [
    { num: 1, key: 'spirit',   title: 'Spirit',   href: '/scripture', accent: 'border-yellow text-yellow' },
    { num: 2, key: 'mind',     title: 'Mind',     href: '/books',     accent: 'border-blue text-blue' },
    { num: 3, key: 'body',     title: 'Body',     href: '/habits',    accent: 'border-green text-green' },
    { num: 4, key: 'vocation', title: 'Vocation', href: '/goals',     accent: 'border-mauve text-mauve' }
  ];
</script>

<svelte:head>
  <title>Roots · granit</title>
</svelte:head>

<div class="h-full overflow-y-auto bg-mantle">
  <div class="max-w-6xl mx-auto px-4 py-6">
    <header class="mb-6 flex items-baseline gap-3">
      <div class="flex-1">
        <h1 class="text-2xl font-semibold text-text">Roots</h1>
        {#if roots?.center}
          <p class="text-sm text-dim mt-1">
            Centered in <span class="text-text font-medium">{roots.center}</span>
            {#if roots.anchor}<span class="text-dim/80"> · {roots.anchor}</span>{/if}
          </p>
        {:else}
          <p class="text-sm text-dim mt-1">Spirit · Mind · Body · Vocation</p>
        {/if}
      </div>
      {#if partialFailures.length > 0}
        <span
          class="text-[10px] font-mono px-2 py-1 rounded bg-warning/10 text-warning border border-warning/30"
          title="Failed to load: {partialFailures.join(', ')}"
        >{partialFailures.length} source{partialFailures.length === 1 ? '' : 's'} unavailable</span>
      {/if}
    </header>

    {#if loading}
      <p class="text-sm text-dim italic">loading life data…</p>
    {:else if loadError}
      <p class="text-sm text-error">{loadError}</p>
    {:else if partialFailures.length >= 8}
      <!-- All-or-most-sources-down case. The "N unavailable" pill in
           the header signals counts but doesn't explain the cause.
           When the failure ratio is this high the user probably has
           an auth or connectivity problem rather than 8 independent
           module bugs, so call that out plainly. -->
      <div class="bg-base border border-surface1 rounded-lg p-4 mb-6">
        <p class="text-sm text-warning">{partialFailures.length} of 12 life-data sources couldn't be reached.</p>
        <p class="text-xs text-dim mt-1">Usually this means the granit server is unreachable or your session expired. Check the server log, or sign out and back in.</p>
      </div>
    {/if}
    {#if !loading && !loadError}
      <!-- KPI row — one headline metric per ring. Big numbers,
           click-through to the source module. -->
      <div class="grid grid-cols-2 lg:grid-cols-4 gap-3 mb-6">
        <a href="/scripture" class="block bg-base border border-surface1 rounded-lg p-4 hover:border-yellow transition-colors">
          <p class="text-[10px] uppercase tracking-wider text-dim">Spirit · Bible streak</p>
          <p class="text-2xl text-text font-light mt-1">
            {bibleStreak?.current ?? 0}
            <span class="text-sm text-dim">day{bibleStreak?.current === 1 ? '' : 's'}</span>
          </p>
          <p class="text-[11px] text-dim mt-0.5">longest {bibleStreak?.longest ?? 0} · {bibleStreak?.todayLogged ? 'today logged' : 'today open'}</p>
        </a>
        <a href="/books" class="block bg-base border border-surface1 rounded-lg p-4 hover:border-blue transition-colors">
          <p class="text-[10px] uppercase tracking-wider text-dim">Mind · Books</p>
          <p class="text-2xl text-text font-light mt-1">
            {books.length}
            <span class="text-sm text-dim">on shelf</span>
          </p>
          <p class="text-[11px] text-dim mt-0.5">{currentlyReading.length} reading now</p>
        </a>
        <a href="/habits" class="block bg-base border border-surface1 rounded-lg p-4 hover:border-green transition-colors">
          <p class="text-[10px] uppercase tracking-wider text-dim">Body · Habits</p>
          <p class="text-2xl text-text font-light mt-1">
            {habits.filter((h) => h.doneToday).length}<span class="text-dim">/{habits.length}</span>
          </p>
          <p class="text-[11px] text-dim mt-0.5">done today</p>
        </a>
        <a href="/goals" class="block bg-base border border-surface1 rounded-lg p-4 hover:border-mauve transition-colors">
          <p class="text-[10px] uppercase tracking-wider text-dim">Vocation · Active goals</p>
          <p class="text-2xl text-text font-light mt-1">
            {activeGoals.length}
            <span class="text-sm text-dim">open</span>
          </p>
          <p class="text-[11px] text-dim mt-0.5">{activeProjects} projects · {activeVentures} ventures</p>
        </a>
      </div>

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <!-- ── SPIRIT ─────────────────────────────────────────── -->
        <section class="bg-base border border-surface1 rounded-lg p-4 border-l-4 {RINGS[0].accent}">
          <header class="flex items-baseline gap-2 mb-3">
            <h2 class="text-base font-semibold text-text">Spirit</h2>
            <span class="text-xs text-dim">faith · virtues · prayer · sabbath</span>
          </header>

          <ul class="space-y-1.5 text-sm">
            <li class="flex items-baseline justify-between">
              <a href="/scripture" class="text-subtext hover:text-text">Bible reading</a>
              <span class="text-dim font-mono text-xs">{bibleStreak?.current ?? 0}d streak · {bibleStreak?.longest ?? 0} best</span>
            </li>
            <li class="flex items-baseline justify-between">
              <a href="/prayer" class="text-subtext hover:text-text">Prayer intentions</a>
              <span class="text-dim font-mono text-xs">{prayerCount} active</span>
            </li>
            <li class="flex items-baseline justify-between">
              <a href="/sabbath" class="text-subtext hover:text-text">Sabbaths observed</a>
              <span class="text-dim font-mono text-xs">{sabbathCount}</span>
            </li>
            <li class="flex items-baseline justify-between">
              <a href="/examen" class="text-subtext hover:text-text">Examen</a>
              <span class="text-dim font-mono text-xs">→ daily</span>
            </li>
          </ul>

          {#if virtues.length > 0}
            <div class="mt-3 pt-3 border-t border-surface1">
              <p class="text-[10px] uppercase tracking-wider text-dim mb-1.5">Virtues cultivating</p>
              <ul class="flex flex-wrap gap-1.5 text-xs">
                {#each virtues.slice(0, 8) as v (v.id)}
                  <li class="px-2 py-0.5 bg-surface0 border border-surface1 rounded text-text">{v.name}</li>
                {/each}
              </ul>
            </div>
          {/if}

          <!-- Hand-tended Spirit items -->
          <div class="mt-3 pt-3 border-t border-surface1">
            <p class="text-[10px] uppercase tracking-wider text-dim mb-1.5">Rooted in</p>
            {#if nodesByRing[1].length === 0 && addingFor !== 1}
              <button
                type="button"
                onclick={() => { addingFor = 1; addLabel = ''; }}
                class="text-xs text-dim hover:text-text"
              >+ add (e.g. "child of God", "baptised", a scripture identity)</button>
            {:else}
              <ul class="flex flex-wrap gap-1.5 text-xs">
                {#each nodesByRing[1] as n (n.id)}
                  <li class="group flex items-center gap-1 px-2 py-0.5 bg-surface0 border border-surface1 rounded text-text">
                    <span>{n.label}</span>
                    <button
                      type="button"
                      onclick={() => removeItem(n.id)}
                      class="opacity-0 group-hover:opacity-100 text-dim hover:text-error text-[10px]"
                      title="remove"
                    >×</button>
                  </li>
                {/each}
                {#if addingFor !== 1}
                  <li>
                    <button
                      type="button"
                      onclick={() => { addingFor = 1; addLabel = ''; }}
                      class="text-xs text-dim hover:text-text px-2"
                    >+</button>
                  </li>
                {/if}
              </ul>
            {/if}
            {#if addingFor === 1}
              <form
                onsubmit={(e) => { e.preventDefault(); addItem(1); }}
                class="mt-2 flex items-center gap-1"
              >
                <input
                  type="text"
                  bind:value={addLabel}
                  placeholder="what is true here"
                  class="flex-1 px-2 py-1 text-xs bg-surface0 border border-surface1 rounded text-text focus:outline-none focus:border-yellow"
                  autofocus
                />
                <button type="submit" class="text-xs px-2 py-1 bg-surface1 hover:bg-surface2 text-text rounded">add</button>
                <button type="button" onclick={() => { addingFor = null; }} class="text-xs text-dim hover:text-text px-1">cancel</button>
              </form>
            {/if}
          </div>
        </section>

        <!-- ── MIND ───────────────────────────────────────────── -->
        <section class="bg-base border border-surface1 rounded-lg p-4 border-l-4 {RINGS[1].accent}">
          <header class="flex items-baseline gap-2 mb-3">
            <h2 class="text-base font-semibold text-text">Mind</h2>
            <span class="text-xs text-dim">knowledge · languages · learning</span>
          </header>

          <ul class="space-y-1.5 text-sm">
            <li class="flex items-baseline justify-between">
              <a href="/books" class="text-subtext hover:text-text">Books on shelf</a>
              <span class="text-dim font-mono text-xs">{books.length} · {currentlyReading.length} reading</span>
            </li>
            <li class="flex items-baseline justify-between">
              <a href="/notes" class="text-subtext hover:text-text">Notes</a>
              <span class="text-dim font-mono text-xs">→ vault</span>
            </li>
          </ul>

          {#if currentlyReading.length > 0}
            <div class="mt-3 pt-3 border-t border-surface1">
              <p class="text-[10px] uppercase tracking-wider text-dim mb-1.5">Reading now</p>
              <ul class="space-y-0.5 text-xs">
                {#each currentlyReading.slice(0, 4) as b (b.id)}
                  <li class="flex items-baseline gap-2">
                    <a href="/books/{encodeURIComponent(b.id)}" class="text-text hover:underline truncate">{b.title}</a>
                    {#if b.authors && b.authors.length > 0}<span class="text-dim text-[11px] truncate">{b.authors.join(', ')}</span>{/if}
                  </li>
                {/each}
              </ul>
            </div>
          {/if}

          <!-- Hand-tended Mind items (languages, current learnings, ...) -->
          <div class="mt-3 pt-3 border-t border-surface1">
            <p class="text-[10px] uppercase tracking-wider text-dim mb-1.5">Languages · learnings</p>
            {#if nodesByRing[2].length === 0 && addingFor !== 2}
              <button
                type="button"
                onclick={() => { addingFor = 2; addLabel = ''; }}
                class="text-xs text-dim hover:text-text"
              >+ add (e.g. "German", "Greek B1", "Stoicism")</button>
            {:else}
              <ul class="flex flex-wrap gap-1.5 text-xs">
                {#each nodesByRing[2] as n (n.id)}
                  <li class="group flex items-center gap-1 px-2 py-0.5 bg-surface0 border border-surface1 rounded text-text">
                    <span>{n.label}</span>
                    <button type="button" onclick={() => removeItem(n.id)} class="opacity-0 group-hover:opacity-100 text-dim hover:text-error text-[10px]" title="remove">×</button>
                  </li>
                {/each}
                {#if addingFor !== 2}
                  <li>
                    <button type="button" onclick={() => { addingFor = 2; addLabel = ''; }} class="text-xs text-dim hover:text-text px-2">+</button>
                  </li>
                {/if}
              </ul>
            {/if}
            {#if addingFor === 2}
              <form onsubmit={(e) => { e.preventDefault(); addItem(2); }} class="mt-2 flex items-center gap-1">
                <input type="text" bind:value={addLabel} placeholder="language or topic" class="flex-1 px-2 py-1 text-xs bg-surface0 border border-surface1 rounded text-text focus:outline-none focus:border-blue" autofocus />
                <button type="submit" class="text-xs px-2 py-1 bg-surface1 hover:bg-surface2 text-text rounded">add</button>
                <button type="button" onclick={() => { addingFor = null; }} class="text-xs text-dim hover:text-text px-1">cancel</button>
              </form>
            {/if}
          </div>
        </section>

        <!-- ── BODY ───────────────────────────────────────────── -->
        <section class="bg-base border border-surface1 rounded-lg p-4 border-l-4 {RINGS[2].accent}">
          <header class="flex items-baseline gap-2 mb-3">
            <h2 class="text-base font-semibold text-text">Body</h2>
            <span class="text-xs text-dim">health · habits · measurements</span>
          </header>

          {#if habits.length > 0}
            <div class="mb-3">
              <p class="text-[10px] uppercase tracking-wider text-dim mb-1.5">Habit streaks</p>
              <ul class="space-y-0.5 text-xs">
                {#each habits.slice().sort((a, b) => b.currentStreak - a.currentStreak).slice(0, 5) as h (h.name)}
                  <li class="flex items-baseline justify-between">
                    <a href="/habits" class="text-text hover:underline truncate">{h.name}</a>
                    <span class="text-dim font-mono text-[11px]">{h.currentStreak}d · {h.last30Pct}% mo</span>
                  </li>
                {/each}
              </ul>
            </div>
          {/if}

          {#if measurementSeries.length > 0}
            <div class="pt-3 border-t border-surface1">
              <p class="text-[10px] uppercase tracking-wider text-dim mb-1.5">Latest measurements</p>
              <ul class="space-y-0.5 text-xs">
                {#each measurementSeries.slice(0, 6) as s (s.name)}
                  {@const latest = latestByMeasurement[s.name]}
                  <li class="flex items-baseline justify-between">
                    <a href="/measurements" class="text-text hover:underline truncate">{s.name}</a>
                    {#if latest}
                      <span class="text-dim font-mono text-[11px]">{latest.value}{s.unit ? ' ' + s.unit : ''} · {latest.date}</span>
                    {:else}
                      <span class="text-dim font-mono text-[11px]">—</span>
                    {/if}
                  </li>
                {/each}
              </ul>
            </div>
          {/if}

          {#if habits.length === 0 && measurementSeries.length === 0}
            <p class="text-xs text-dim italic">No habits or measurements yet — set some up in /habits or /measurements.</p>
          {/if}

          <div class="mt-3 pt-3 border-t border-surface1">
            <p class="text-[10px] uppercase tracking-wider text-dim mb-1.5">Cared for as a gift</p>
            {#if nodesByRing[3].length === 0 && addingFor !== 3}
              <button
                type="button"
                onclick={() => { addingFor = 3; addLabel = ''; }}
                class="text-xs text-dim hover:text-text"
              >+ add (e.g. "running", "yoga", "fast Mondays")</button>
            {:else}
              <ul class="flex flex-wrap gap-1.5 text-xs">
                {#each nodesByRing[3] as n (n.id)}
                  <li class="group flex items-center gap-1 px-2 py-0.5 bg-surface0 border border-surface1 rounded text-text">
                    <span>{n.label}</span>
                    <button type="button" onclick={() => removeItem(n.id)} class="opacity-0 group-hover:opacity-100 text-dim hover:text-error text-[10px]" title="remove">×</button>
                  </li>
                {/each}
                {#if addingFor !== 3}
                  <li>
                    <button type="button" onclick={() => { addingFor = 3; addLabel = ''; }} class="text-xs text-dim hover:text-text px-2">+</button>
                  </li>
                {/if}
              </ul>
            {/if}
            {#if addingFor === 3}
              <form onsubmit={(e) => { e.preventDefault(); addItem(3); }} class="mt-2 flex items-center gap-1">
                <input type="text" bind:value={addLabel} placeholder="practice" class="flex-1 px-2 py-1 text-xs bg-surface0 border border-surface1 rounded text-text focus:outline-none focus:border-green" autofocus />
                <button type="submit" class="text-xs px-2 py-1 bg-surface1 hover:bg-surface2 text-text rounded">add</button>
                <button type="button" onclick={() => { addingFor = null; }} class="text-xs text-dim hover:text-text px-1">cancel</button>
              </form>
            {/if}
          </div>
        </section>

        <!-- ── VOCATION ───────────────────────────────────────── -->
        <section class="bg-base border border-surface1 rounded-lg p-4 border-l-4 {RINGS[3].accent}">
          <header class="flex items-baseline gap-2 mb-3">
            <h2 class="text-base font-semibold text-text">Vocation</h2>
            <span class="text-xs text-dim">work · wealth · stewardship</span>
          </header>

          {#if finance}
            <div class="mb-3">
              <p class="text-[10px] uppercase tracking-wider text-dim mb-1.5">Finance</p>
              <ul class="space-y-0.5 text-xs">
                <li class="flex items-baseline justify-between">
                  <a href="/finance" class="text-text hover:underline">Net worth</a>
                  <span class="text-dim font-mono text-[11px]">{eur(finance.net_worth_cents)}</span>
                </li>
                <li class="flex items-baseline justify-between">
                  <a href="/finance" class="text-text hover:underline">Monthly income (actual)</a>
                  <span class="text-dim font-mono text-[11px]">{eur(finance.income_monthly_actual_cents)}</span>
                </li>
                <li class="flex items-baseline justify-between">
                  <a href="/finance" class="text-text hover:underline">Subscriptions/mo</a>
                  <span class="text-dim font-mono text-[11px]">{eur(finance.subscription_monthly_cents)}</span>
                </li>
              </ul>
            </div>
          {/if}

          {#if activeGoals.length > 0}
            <div class="pt-3 border-t border-surface1">
              <p class="text-[10px] uppercase tracking-wider text-dim mb-1.5">Open goals</p>
              <ul class="space-y-0.5 text-xs">
                {#each activeGoals.slice(0, 5) as g (g.id)}
                  <li class="flex items-baseline justify-between gap-2">
                    <a href="/goals" class="text-text hover:underline truncate">{g.title}</a>
                    {#if g.target_date}<span class="text-dim font-mono text-[11px]">→ {g.target_date}</span>{/if}
                  </li>
                {/each}
              </ul>
            </div>
          {/if}

          <div class="mt-3 pt-3 border-t border-surface1">
            <p class="text-[10px] uppercase tracking-wider text-dim mb-1.5">Callings · longings</p>
            {#if nodesByRing[4].length === 0 && addingFor !== 4}
              <button
                type="button"
                onclick={() => { addingFor = 4; addLabel = ''; }}
                class="text-xs text-dim hover:text-text"
              >+ add (e.g. "husband", "writer", "build a school")</button>
            {:else}
              <ul class="flex flex-wrap gap-1.5 text-xs">
                {#each nodesByRing[4] as n (n.id)}
                  <li class="group flex items-center gap-1 px-2 py-0.5 bg-surface0 border border-surface1 rounded text-text">
                    <span>{n.label}</span>
                    <button type="button" onclick={() => removeItem(n.id)} class="opacity-0 group-hover:opacity-100 text-dim hover:text-error text-[10px]" title="remove">×</button>
                  </li>
                {/each}
                {#if addingFor !== 4}
                  <li>
                    <button type="button" onclick={() => { addingFor = 4; addLabel = ''; }} class="text-xs text-dim hover:text-text px-2">+</button>
                  </li>
                {/if}
              </ul>
            {/if}
            {#if addingFor === 4}
              <form onsubmit={(e) => { e.preventDefault(); addItem(4); }} class="mt-2 flex items-center gap-1">
                <input type="text" bind:value={addLabel} placeholder="role, work, longing" class="flex-1 px-2 py-1 text-xs bg-surface0 border border-surface1 rounded text-text focus:outline-none focus:border-mauve" autofocus />
                <button type="submit" class="text-xs px-2 py-1 bg-surface1 hover:bg-surface2 text-text rounded">add</button>
                <button type="button" onclick={() => { addingFor = null; }} class="text-xs text-dim hover:text-text px-1">cancel</button>
              </form>
            {/if}
          </div>
        </section>
      </div>
    {/if}
  </div>
</div>
