<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, type VaultStats, type StatEntry } from '$lib/api';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import { onWsEvent } from '$lib/ws';

  let stats = $state<VaultStats | null>(null);
  let loading = $state(true);

  async function load() {
    if (!$auth) return;
    loading = true;
    try {
      stats = await api.stats();
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

  let chartMax = $derived(Math.max(1, ...(stats?.notesPerMonth ?? []).map((m) => m.value)));

  function formatNum(n: number): string {
    if (n < 1000) return String(n);
    if (n < 10_000) return (n / 1000).toFixed(1) + 'k';
    return Math.round(n / 1000) + 'k';
  }

  function fmtDate(unix: number): string {
    return new Date(unix * 1000).toLocaleDateString(undefined, {
      month: 'short',
      day: 'numeric'
    });
  }
</script>

<div class="h-full overflow-y-auto">
  <div class="p-4 sm:p-6 lg:p-8 max-w-5xl mx-auto">
    <PageHeader title="Vault statistics" subtitle="Live snapshot of what's in the vault" />

    {#if loading && !stats}
      <div class="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-6">
        {#each Array(4) as _}<Skeleton class="h-20 w-full" />{/each}
      </div>
    {:else if stats}
      <!-- Headline numbers -->
      <div class="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-6">
        <div class="bg-surface0 border border-surface1 rounded-lg p-3">
          <div class="text-2xl sm:text-3xl font-semibold text-primary">{formatNum(stats.noteCount)}</div>
          <div class="text-[11px] uppercase tracking-wider text-dim mt-1">Notes</div>
        </div>
        <div class="bg-surface0 border border-surface1 rounded-lg p-3">
          <div class="text-2xl sm:text-3xl font-semibold text-secondary">{formatNum(stats.totalWords)}</div>
          <div class="text-[11px] uppercase tracking-wider text-dim mt-1">Words</div>
        </div>
        <div class="bg-surface0 border border-surface1 rounded-lg p-3">
          <div class="text-2xl sm:text-3xl font-semibold text-info">{formatNum(stats.totalLinks)}</div>
          <div class="text-[11px] uppercase tracking-wider text-dim mt-1">Wikilinks</div>
        </div>
        <div class="bg-surface0 border border-surface1 rounded-lg p-3">
          <div class="text-2xl sm:text-3xl font-semibold text-accent">{formatNum(stats.totalTags)}</div>
          <div class="text-[11px] uppercase tracking-wider text-dim mt-1">Tags</div>
        </div>
      </div>

      <!-- Notes per month bar chart -->
      <section class="bg-surface0 border border-surface1 rounded-lg p-4 sm:p-5 mb-6">
        <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-3">Notes edited per month — last 12</h2>
        <div class="grid grid-cols-12 gap-1 items-end h-32">
          {#each stats.notesPerMonth as m (m.name)}
            {@const h = chartMax > 0 ? Math.round((m.value / chartMax) * 100) : 0}
            <div class="flex flex-col items-center justify-end gap-1 group">
              <div
                class="w-full rounded-t bg-primary/60 group-hover:bg-primary transition-colors min-h-[2px]"
                style="height: {h}%"
                title="{m.name}: {m.value}"
              ></div>
              <span class="text-[9px] text-dim font-mono whitespace-nowrap">{m.name.slice(5)}</span>
            </div>
          {/each}
        </div>
      </section>

      <!-- 4-up grid: top tags, top linked, largest, recent -->
      <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
        <section class="bg-surface0 border border-surface1 rounded-lg p-4">
          <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-3">Top tags</h2>
          {#if stats.topTags.length === 0}
            <div class="text-sm text-dim italic">no tags yet</div>
          {:else}
            <ul class="space-y-1.5">
              {#each stats.topTags as t (t.name)}
                <li class="flex items-center gap-2">
                  <a href="/tags" class="text-sm text-secondary hover:text-primary truncate flex-1">#{t.name}</a>
                  <span class="text-xs text-dim font-mono">{t.value}</span>
                </li>
              {/each}
            </ul>
          {/if}
        </section>

        <section class="bg-surface0 border border-surface1 rounded-lg p-4">
          <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-3">Top linked-to notes</h2>
          {#if stats.topLinkedNotes.length === 0}
            <div class="text-sm text-dim italic">no incoming links yet</div>
          {:else}
            <ul class="space-y-1.5">
              {#each stats.topLinkedNotes as n (n.name)}
                <li class="flex items-center gap-2">
                  <a href="/notes/{encodeURIComponent(n.name)}" class="text-sm text-text hover:text-primary truncate flex-1">{n.name}</a>
                  <span class="text-xs text-dim font-mono">{n.value}</span>
                </li>
              {/each}
            </ul>
          {/if}
        </section>

        <section class="bg-surface0 border border-surface1 rounded-lg p-4">
          <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-3">Largest notes (words)</h2>
          <ul class="space-y-1.5">
            {#each stats.largestNotes as n (n.name)}
              <li class="flex items-center gap-2">
                <a href="/notes/{encodeURIComponent(n.name)}" class="text-sm text-text hover:text-primary truncate flex-1">{n.name}</a>
                <span class="text-xs text-dim font-mono">{formatNum(n.value)}</span>
              </li>
            {/each}
          </ul>
        </section>

        <section class="bg-surface0 border border-surface1 rounded-lg p-4">
          <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-3">Recently edited</h2>
          <ul class="space-y-1.5">
            {#each stats.recentlyEdited as n (n.name)}
              <li class="flex items-center gap-2">
                <a href="/notes/{encodeURIComponent(n.name)}" class="text-sm text-text hover:text-primary truncate flex-1">{n.name}</a>
                <span class="text-xs text-dim font-mono">{fmtDate(n.value)}</span>
              </li>
            {/each}
          </ul>
        </section>
      </div>

      <!-- Health -->
      <section class="bg-surface0 border border-surface1 rounded-lg p-4 mt-6">
        <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-3">Vault health</h2>
        <div class="grid grid-cols-3 gap-3 text-center">
          <div>
            <div class="text-lg font-semibold text-text">{stats.averageWords}</div>
            <div class="text-[11px] text-dim">avg words / note</div>
          </div>
          <div>
            <div class="text-lg font-semibold text-text">{stats.untypedNotes}</div>
            <div class="text-[11px] text-dim">untyped notes</div>
          </div>
          <div>
            <div class="text-lg font-semibold text-text">{stats.orphanNotes}</div>
            <div class="text-[11px] text-dim">orphaned</div>
          </div>
        </div>
      </section>
    {/if}
  </div>
</div>
