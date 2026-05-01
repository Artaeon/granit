<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, type Goal } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { inlineMd } from '$lib/util/inlineMd';

  let goals = $state<Goal[]>([]);
  let loading = $state(false);
  let statusFilter = $state<'all' | 'active' | 'paused' | 'done'>('all');

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
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') load();
    });
  });

  let filtered = $derived.by(() => {
    if (statusFilter === 'all') return goals;
    return goals.filter((g) => (g.status ?? 'active') === statusFilter);
  });

  function progress(g: Goal): { done: number; total: number; pct: number } {
    const ms = g.milestones ?? [];
    const total = ms.length;
    if (total === 0) return { done: 0, total: 0, pct: 0 };
    const done = ms.filter((m) => m.done).length;
    return { done, total, pct: Math.round((done / total) * 100) };
  }

  function statusColor(s?: string): { bg: string; text: string; ring: string } {
    switch (s) {
      case 'active':
        return { bg: 'bg-success/15', text: 'text-success', ring: 'ring-success/30' };
      case 'paused':
        return { bg: 'bg-warning/15', text: 'text-warning', ring: 'ring-warning/30' };
      case 'done':
        return { bg: 'bg-info/15', text: 'text-info', ring: 'ring-info/30' };
      default:
        return { bg: 'bg-surface1', text: 'text-subtext', ring: 'ring-surface2' };
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

  let counts = $derived({
    all: goals.length,
    active: goals.filter((g) => (g.status ?? 'active') === 'active').length,
    paused: goals.filter((g) => g.status === 'paused').length,
    done: goals.filter((g) => g.status === 'done').length
  });

  let expanded = $state<Record<string, boolean>>({});
  function toggle(id: string) { expanded = { ...expanded, [id]: !expanded[id] }; }
</script>

<div class="h-full overflow-y-auto">
  <div class="p-4 sm:p-6 lg:p-8 max-w-4xl mx-auto">
    <header class="mb-6 flex flex-col sm:flex-row sm:items-baseline sm:justify-between gap-2">
      <div>
        <h1 class="text-2xl sm:text-3xl font-semibold text-text">Goals</h1>
        <p class="text-sm text-dim mt-1">{goals.length} goals · from <code class="text-xs">.granit/goals.json</code></p>
      </div>
    </header>

    <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-sm mb-6 self-start">
      {#each ['all', 'active', 'paused', 'done'] as s}
        <button
          class="px-3 py-1.5 capitalize {statusFilter === s ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}"
          onclick={() => (statusFilter = s as typeof statusFilter)}
        >
          {s} <span class="text-xs opacity-70">{counts[s as keyof typeof counts]}</span>
        </button>
      {/each}
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
          {@const isOpen = !!expanded[g.id]}
          <article class="bg-surface0 border border-surface1 rounded-lg overflow-hidden hover:border-primary/40 transition-colors">
            <button
              type="button"
              onclick={() => toggle(g.id)}
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
                {#if g.target_date}<span>🎯 {fmtDate(g.target_date)}</span>{/if}
                {#if g.project}<span>📁 {g.project}</span>{/if}
                {#if g.category}<span>· {g.category}</span>{/if}
                {#if p.total > 0}<span>{p.done}/{p.total} milestones</span>{/if}
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
                  <div class="text-[10px] text-dim mt-1">{p.pct}% complete · click to {isOpen ? 'collapse' : 'expand'}</div>
                </div>
              {/if}
            </button>

            {#if isOpen && g.milestones && g.milestones.length > 0}
              <div class="px-4 pb-4 border-t border-surface1 pt-3 bg-mantle/40">
                <ul class="space-y-1.5">
                  {#each g.milestones as m, i (i)}
                    <li class="flex items-start gap-2 text-sm">
                      <span class="w-4 h-4 rounded border flex-shrink-0 flex items-center justify-center mt-0.5
                        {m.done ? 'bg-success border-success' : 'border-surface2'}">
                        {#if m.done}
                          <svg viewBox="0 0 12 12" class="w-3 h-3 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
                        {/if}
                      </span>
                      <span class="{m.done ? 'line-through text-dim' : 'text-text'}">{@html inlineMd(m.text)}</span>
                    </li>
                  {/each}
                </ul>
              </div>
            {/if}
          </article>
        {/each}
      </div>
    {/if}
  </div>
</div>
