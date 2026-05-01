<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Goal } from '$lib/api';
  import { inlineMd } from '$lib/util/inlineMd';

  let goals = $state<Goal[]>([]);
  let loading = $state(false);

  async function load() {
    loading = true;
    try {
      const list = await api.listGoals();
      goals = list.goals.filter((g) => (g.status ?? 'active') === 'active').slice(0, 4);
    } finally {
      loading = false;
    }
  }
  onMount(load);

  function progress(g: Goal): number {
    const ms = g.milestones ?? [];
    if (ms.length === 0) return 0;
    return Math.round((ms.filter((m) => m.done).length / ms.length) * 100);
  }
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-4">
  <div class="flex items-baseline justify-between mb-2">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Goals</h2>
    <a href="/goals" class="text-xs text-secondary hover:underline">all →</a>
  </div>
  {#if loading && goals.length === 0}
    <div class="text-sm text-dim">loading…</div>
  {:else if goals.length === 0}
    <div class="text-sm text-dim italic">no active goals</div>
  {:else}
    <ul class="space-y-3">
      {#each goals as g (g.id)}
        {@const p = progress(g)}
        <li>
          <a href="/goals" class="block hover:opacity-90">
            <div class="text-sm text-text truncate">{@html inlineMd(g.title)}</div>
            {#if g.milestones && g.milestones.length > 0}
              <div class="h-1.5 bg-mantle rounded-full overflow-hidden mt-1">
                <div class="h-full bg-primary" style="width: {p}%"></div>
              </div>
              <div class="text-[10px] text-dim mt-0.5">{p}% · {g.milestones.filter((m) => m.done).length}/{g.milestones.length}</div>
            {:else if g.target_date}
              <div class="text-[10px] text-dim mt-0.5">🎯 {g.target_date}</div>
            {/if}
          </a>
        </li>
      {/each}
    </ul>
  {/if}
</section>
