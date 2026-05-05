<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { auth } from '$lib/stores/auth';
  import { api, type Goal } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { inlineMd } from '$lib/util/inlineMd';
  import { toast } from '$lib/components/toast';
  import GoalCreate from '$lib/goals/GoalCreate.svelte';
  import GoalDetail from '$lib/goals/GoalDetail.svelte';
  import VisionContextStrip from '$lib/components/VisionContextStrip.svelte';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import { daysUntilTarget, targetChip, targetBorderColor } from '$lib/goals/util';

  let goals = $state<Goal[]>([]);
  let loading = $state(false);
  // Status values mirror the TUI / internal/goals.Status. The earlier UI
  // rendered a 'done' tab that never matched anything because the TUI
  // writes 'completed'.
  let statusFilter = $state<'all' | 'active' | 'paused' | 'completed' | 'archived'>('all');
  let categoryFilter = $state<string>('');
  let tagFilter = $state<string>('');
  let ventureFilter = $state<string>('');
  let q = $state<string>('');

  let createOpen = $state(false);
  let detailOpen = $state(false);
  let selectedId = $state<string | null>(null);

  async function load() {
    if (!$auth) return;
    loading = true;
    try {
      const list = await api.listGoals();
      goals = list.goals;
    } finally {
      loading = false;
    }
  }
  onMount(() => {
    load();
    const unsub = onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') load();
      // Re-fetch when the TUI (or another web tab) writes goals.json.
      // The server broadcasts state.changed with Path=".granit/goals.json".
      if (ev.type === 'state.changed' && ev.path === '.granit/goals.json') load();
    });
    // Visibility-aware refresh: WS connections are suspended when the
    // tab is backgrounded (especially on mobile Safari), so we'd miss
    // any state.changed event fired in that window. Refetching on
    // visibility flip cheaply guarantees the user never returns to a
    // stale list.
    const onVisible = () => {
      if (document.visibilityState === 'visible') load();
    };
    document.addEventListener('visibilitychange', onVisible);
    window.addEventListener('focus', onVisible);
    return () => {
      unsub();
      document.removeEventListener('visibilitychange', onVisible);
      window.removeEventListener('focus', onVisible);
    };
  });

  // ?focus=<goalId> auto-opens the matching detail drawer. Used by the
  // /tasks page deepLink when a goal-group "open ↗" is clicked. Without
  // this the deepLink used to point at /goals/<id> — a route that
  // didn't exist, so SvelteKit served the SPA shell and the client
  // router fell through silently. The user perceived it as a freeze.
  $effect(() => {
    const focus = $page.url.searchParams.get('focus');
    if (!focus) return;
    if (goals.length === 0) return; // wait until load() resolves
    const g = goals.find((x) => x.id === focus);
    if (g && selectedId !== focus) {
      selectedId = focus;
      detailOpen = true;
    }
  });

  // Selected goal — derived from id so live edits during a refetch find
  // the new copy without reopening the drawer at a stale state.
  let selected = $derived(goals.find((g) => g.id === selectedId) ?? null);

  function openDetail(g: Goal) {
    selectedId = g.id;
    detailOpen = true;
  }

  let filtered = $derived.by(() => {
    let list = goals;
    if (statusFilter !== 'all') list = list.filter((g) => (g.status ?? 'active') === statusFilter);
    if (categoryFilter) list = list.filter((g) => g.category === categoryFilter);
    if (tagFilter) list = list.filter((g) => (g.tags ?? []).includes(tagFilter));
    if (ventureFilter) list = list.filter((g) => (g.venture ?? '') === ventureFilter);
    const term = q.trim().toLowerCase();
    if (term) {
      list = list.filter((g) =>
        g.title.toLowerCase().includes(term) ||
        (g.description ?? '').toLowerCase().includes(term) ||
        (g.notes ?? '').toLowerCase().includes(term) ||
        (g.venture ?? '').toLowerCase().includes(term)
      );
    }
    return list;
  });

  function progress(g: Goal): { done: number; total: number; pct: number } {
    const ms = g.milestones ?? [];
    const total = ms.length;
    if (total === 0) return { done: 0, total: 0, pct: g.status === 'completed' ? 100 : 0 };
    const done = ms.filter((m) => m.done).length;
    return { done, total, pct: Math.round((done / total) * 100) };
  }

  // Status-pill colors. Spec: active=primary, paused=subtext, completed=success, archived=dim.
  function statusColor(s?: string): { bg: string; text: string } {
    switch (s) {
      case 'active':
        return { bg: 'bg-primary/15', text: 'text-primary' };
      case 'paused':
        return { bg: 'bg-surface1', text: 'text-subtext' };
      case 'completed':
        return { bg: 'bg-success/15', text: 'text-success' };
      case 'archived':
        return { bg: 'bg-surface1', text: 'text-dim' };
      default:
        return { bg: 'bg-surface1', text: 'text-subtext' };
    }
  }

  function fmtDate(s: string | undefined): string {
    if (!s) return '';
    const d = new Date(s);
    if (!isNaN(d.getTime())) {
      return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
    }
    return s;
  }

  // Status-aware urgency tone for the goal card's left border.
  // Completed + archived goals stay neutral so a past-target completed
  // goal doesn't shout — its target_date is now historical context,
  // not a call to action. Only `active` and `paused` goals get the
  // urgency treatment so the user's eye lands on living work.
  // The proximity → tone mapping itself lives in $lib/goals/util so
  // /goals and the dashboard widget can't drift.
  function targetTone(g: Goal): string | null {
    if (g.status === 'completed' || g.status === 'archived') return null;
    const days = daysUntilTarget(g.target_date);
    if (days === null) return null;
    if (days < 0) return 'error';
    if (days <= 30) return 'warning';
    if (days <= 90) return 'info';
    return null;
  }

  let counts = $derived({
    all: goals.length,
    active: goals.filter((g) => (g.status ?? 'active') === 'active').length,
    paused: goals.filter((g) => g.status === 'paused').length,
    completed: goals.filter((g) => g.status === 'completed').length,
    archived: goals.filter((g) => g.status === 'archived').length
  });

  // ----- Hero "next target" -----
  // Picks the most-imminent active or paused goal with a parseable
  // target_date and surfaces it as a hero card above the list. Skips
  // goals whose status excludes them from the urgency treatment
  // (completed / archived) and skips free-text target_dates that
  // can't be compared to "today". Falls back to null when the user
  // has no dated goals — the hero card simply doesn't render.
  let goalHero = $derived.by((): { goal: Goal; days: number } | null => {
    let best: Goal | null = null;
    let bestDays = Infinity;
    for (const g of goals) {
      const status = g.status ?? 'active';
      if (status !== 'active' && status !== 'paused') continue;
      const days = daysUntilTarget(g.target_date);
      if (days === null) continue;
      // Earliest target wins; overdue goals (negative days) sort
      // ahead of upcoming ones because they need attention more.
      if (days < bestDays) {
        bestDays = days;
        best = g;
      }
    }
    return best ? { goal: best, days: bestDays } : null;
  });

  // ----- Target-proximity stat strip -----
  // Distribution of dated active+paused goals across urgency
  // buckets, surfaced as a one-line summary below the status tabs.
  // Complements the hero (single most-pressing goal) by showing the
  // shape of the whole pipeline at a glance. Free-text target_dates
  // and undated goals are excluded — they have no place on a
  // proximity axis.
  let targetStats = $derived.by(() => {
    let pastTarget = 0, thisMonth = 0, thisQuarter = 0, later = 0;
    for (const g of goals) {
      const status = g.status ?? 'active';
      if (status !== 'active' && status !== 'paused') continue;
      const days = daysUntilTarget(g.target_date);
      if (days === null) continue;
      if (days < 0) pastTarget++;
      else if (days <= 30) thisMonth++;
      else if (days <= 90) thisQuarter++;
      else later++;
    }
    return { pastTarget, thisMonth, thisQuarter, later };
  });

  // Distinct category + tag chips, sorted by frequency desc so the most
  // common chip surfaces first.
  let categories = $derived.by(() => {
    const m = new Map<string, number>();
    for (const g of goals) {
      const c = (g.category ?? '').trim();
      if (!c) continue;
      m.set(c, (m.get(c) ?? 0) + 1);
    }
    return [...m.entries()].sort((a, b) => b[1] - a[1]).map(([c]) => c);
  });
  let tags = $derived.by(() => {
    const m = new Map<string, number>();
    for (const g of goals) {
      for (const t of g.tags ?? []) m.set(t, (m.get(t) ?? 0) + 1);
    }
    return [...m.entries()].sort((a, b) => b[1] - a[1]).map(([t]) => t);
  });
  let ventures = $derived.by(() => {
    const m = new Map<string, number>();
    for (const g of goals) {
      const v = (g.venture ?? '').trim();
      if (!v) continue;
      m.set(v, (m.get(v) ?? 0) + 1);
    }
    return [...m.entries()].sort((a, b) => b[1] - a[1]).map(([v]) => v);
  });

  async function created(g: Goal) {
    // Optimistic prepend so the new goal renders immediately. The
    // load() below reconciles with the server (auth-stamped CreatedAt,
    // any defaults the server filled in).
    if (!goals.some((x) => x.id === g.id)) {
      goals = [g, ...goals];
    }
    selectedId = g.id;
    detailOpen = true;
    await load();
  }

  async function deleted(_id: string) {
    detailOpen = false;
    selectedId = null;
    await load();
    toast.success('goal deleted');
  }
</script>

<div class="h-full overflow-y-auto">
  <div class="p-4 sm:p-6 lg:p-8 max-w-4xl mx-auto">
    <VisionContextStrip />
    <PageHeader
      title="Goals"
      subtitle="{goals.length} {goals.length === 1 ? 'goal' : 'goals'} · the things you're committing to in this season"
    >
      {#snippet actions()}
        <button
          onclick={() => (createOpen = true)}
          class="px-3 py-1.5 bg-primary text-on-primary rounded text-sm font-medium hover:opacity-90"
        >+ New goal</button>
      {/snippet}
    </PageHeader>

    <!-- Hero "next target" card — surfaces the most-imminent active
         or paused goal with a parseable target_date so the user lands
         on what's most pressing without scanning the list. Click jumps
         to the goal's detail drawer (same affordance as the cards
         below). Border tint = urgency, mirroring the deadlines hero. -->
    {#if goalHero}
      {@const h = goalHero.goal}
      {@const days = goalHero.days}
      {@const p = progress(h)}
      <button
        type="button"
        onclick={() => openDetail(h)}
        class="w-full text-left block mb-5 p-4 sm:p-5 bg-surface0 border-l-4 rounded-lg hover:border-primary transition-colors"
        style="border-left-color: {targetBorderColor(days)};"
      >
        <div class="flex items-start gap-3">
          <span class="text-2xl flex-shrink-0 mt-0.5" aria-hidden="true">🎯</span>
          <div class="flex-1 min-w-0">
            <div class="text-[11px] uppercase tracking-wider text-dim mb-0.5">Next target</div>
            <div class="text-lg sm:text-xl font-semibold text-text leading-tight break-words">
              {#if days < 0}
                <span class="text-error">{Math.abs(days)} {Math.abs(days) === 1 ? 'day' : 'days'} past target</span> · {h.title}
              {:else if days === 0}
                <span class="text-error">Today</span> · {h.title}
              {:else if days === 1}
                <span class="text-warning">Tomorrow</span> · {h.title}
              {:else}
                <span class="text-primary">{days} days</span>
                <span class="text-dim font-normal">until</span>
                {h.title}
              {/if}
            </div>
            <div class="flex flex-wrap items-center gap-x-3 gap-y-1 mt-1.5 text-xs text-dim">
              <span class="font-mono tabular-nums text-subtext">{fmtDate(h.target_date)}</span>
              {#if h.venture}
                <span class="text-secondary">🏢 {h.venture}</span>
              {/if}
              {#if h.project}
                <span class="text-secondary">📁 {h.project}</span>
              {/if}
              {#if p.total > 0}
                <span class="tabular-nums">{p.done}/{p.total} milestones · {p.pct}%</span>
              {/if}
              <span class="ml-auto text-secondary hover:underline">View →</span>
            </div>
          </div>
        </div>
      </button>
    {/if}

    <!-- Status tabs -->
    <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-sm mb-3 self-start flex-wrap">
      {#each ['all', 'active', 'paused', 'completed', 'archived'] as s}
        <button
          class="px-3 py-1.5 capitalize {statusFilter === s ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
          onclick={() => (statusFilter = s as typeof statusFilter)}
        >
          {s} <span class="text-xs opacity-70">{counts[s as keyof typeof counts]}</span>
        </button>
      {/each}
    </div>

    <!-- Target-proximity stat strip — complements the hero card by
         showing the distribution of dated goals across urgency
         buckets. Hidden when no dated goals exist (the strip would
         just read "0 0 0 0"). -->
    {#if targetStats.pastTarget + targetStats.thisMonth + targetStats.thisQuarter + targetStats.later > 0}
      <div class="flex flex-wrap items-baseline gap-x-4 gap-y-1 mb-4 text-xs">
        {#if targetStats.pastTarget > 0}
          <span class="text-error font-medium tabular-nums">{targetStats.pastTarget} past target</span>
        {/if}
        {#if targetStats.thisMonth > 0}
          <span class="text-warning tabular-nums">{targetStats.thisMonth} this month</span>
        {/if}
        {#if targetStats.thisQuarter > 0}
          <span class="text-info tabular-nums">{targetStats.thisQuarter} this quarter</span>
        {/if}
        {#if targetStats.later > 0}
          <span class="text-dim tabular-nums">{targetStats.later} later</span>
        {/if}
      </div>
    {/if}

    <!-- Search + filter chips -->
    <div class="mb-6 space-y-2">
      <input
        bind:value={q}
        placeholder="search title, description, notes…"
        class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
      />
      {#if categories.length > 0 || tags.length > 0 || ventures.length > 0}
        <div class="flex flex-wrap items-center gap-1.5 text-xs">
          {#if categoryFilter || tagFilter || ventureFilter}
            <button
              onclick={() => { categoryFilter = ''; tagFilter = ''; ventureFilter = ''; }}
              class="px-2 py-0.5 bg-surface1 text-dim rounded hover:text-text"
            >clear filters</button>
          {/if}
          {#each ventures as v}
            <button
              onclick={() => (ventureFilter = ventureFilter === v ? '' : v)}
              class="px-2 py-0.5 rounded {ventureFilter === v ? 'bg-secondary text-on-primary' : 'bg-surface0 text-secondary hover:bg-surface1'}"
              title="filter to this venture"
            >🏢 {v}</button>
          {/each}
          {#each categories as c}
            <button
              onclick={() => (categoryFilter = categoryFilter === c ? '' : c)}
              class="px-2 py-0.5 rounded {categoryFilter === c ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1'}"
            >{c}</button>
          {/each}
          {#each tags as t}
            <button
              onclick={() => (tagFilter = tagFilter === t ? '' : t)}
              class="px-2 py-0.5 rounded {tagFilter === t ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1'}"
            >#{t}</button>
          {/each}
        </div>
      {/if}
    </div>

    {#if loading && goals.length === 0}
      <div class="text-sm text-dim">loading…</div>
    {:else if filtered.length === 0}
      <div class="text-sm text-dim italic">no goals match this filter.</div>
    {:else}
      <div class="space-y-4">
        {#each filtered as g (g.id)}
          {@const p = progress(g)}
          {@const sc = statusColor(g.status)}
          {@const tone = targetTone(g)}
          {@const chip = targetChip(g.target_date)}
          <article
            class="bg-surface0 border border-surface1 rounded-lg overflow-hidden hover:border-primary/40 transition-colors {tone ? 'border-l-4' : ''}"
            style={tone ? `border-left-color: var(--color-${tone});` : ''}
          >
            <button
              type="button"
              onclick={() => openDetail(g)}
              class="w-full text-left p-4 flex flex-col gap-2"
            >
              <div class="flex items-start gap-3">
                <div class="flex-1 min-w-0">
                  <h2 class="text-base sm:text-lg font-semibold text-text break-words">{@html inlineMd(g.title)}</h2>
                  {#if g.description}
                    <p class="text-sm text-subtext mt-1 break-words">{@html inlineMd(g.description)}</p>
                  {/if}
                </div>
                <span class="text-[10px] uppercase tracking-wider px-2 py-0.5 rounded {sc.bg} {sc.text} flex-shrink-0">
                  {g.status ?? 'active'}
                </span>
              </div>

              <div class="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-dim">
                {#if g.target_date}
                  <span class="inline-flex items-baseline gap-1.5">
                    <span>🎯 {fmtDate(g.target_date)}</span>
                    {#if chip && (g.status === 'active' || g.status === 'paused')}
                      <span
                        class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-medium tabular-nums whitespace-nowrap"
                        style="background: color-mix(in srgb, var(--color-{chip.tone}) 14%, transparent); color: var(--color-{chip.tone});"
                      >{chip.label}</span>
                    {/if}
                  </span>
                {/if}
                {#if g.project}<span>📁 {g.project}</span>{/if}
                {#if g.venture}<span class="text-secondary">🏢 {g.venture}</span>{/if}
                {#if g.category}<span>· {g.category}</span>{/if}
                {#if p.total > 0}<span>{p.done}/{p.total} milestones</span>{/if}
                {#if g.review_frequency}<span>↻ {g.review_frequency}</span>{/if}
              </div>

              {#if g.tags && g.tags.length > 0}
                <div class="flex flex-wrap gap-1">
                  {#each g.tags as t}
                    <span class="text-[10px] px-1.5 py-0.5 bg-surface1 text-subtext rounded">#{t}</span>
                  {/each}
                </div>
              {/if}

              {#if p.total > 0}
                <div class="mt-1">
                  <div class="h-1.5 bg-mantle rounded-full overflow-hidden">
                    <div class="h-full bg-primary transition-all" style="width: {p.pct}%"></div>
                  </div>
                  <div class="text-[10px] text-dim mt-1">{p.pct}% complete</div>
                </div>
              {/if}
            </button>
          </article>
        {/each}
      </div>
    {/if}
  </div>
</div>

<GoalCreate bind:open={createOpen} ventures={ventures} onCreated={created} />
<GoalDetail
  bind:open={detailOpen}
  goal={selected}
  onUpdated={load}
  onDeleted={deleted}
/>
