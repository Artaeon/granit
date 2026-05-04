<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Venture } from '$lib/api';
  import { onWsEvent } from '$lib/ws';

  // VenturesWidget — compact rollup of the user's active ventures with
  // their project + goal counts. The dashboard's home for "what
  // umbrellas am I working under?". Click any chip to drill into the
  // venture's project list (?venture=…) so the widget doubles as
  // navigation. Hidden when there are no active ventures so we don't
  // show an empty card to users who haven't adopted the feature yet.

  let ventures = $state<Venture[]>([]);
  let loaded = $state(false);

  async function load() {
    try {
      const r = await api.listVentures();
      ventures = r.ventures;
    } catch {
      ventures = [];
    } finally {
      loaded = true;
    }
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/ventures.json') load();
      // Project / goal counts on each card depend on those sidecars
      // too, so refresh when they move (cheap: same single endpoint).
      if (ev.type.startsWith('project.')) load();
      if (ev.type === 'state.changed' && ev.path === '.granit/goals.json') load();
    });
  });

  // Filter to active ventures, sorted alphabetically — alphabetic
  // order is stable across loads (the server doesn't sort) so a
  // re-render doesn't shuffle the cards under the user's mouse.
  let active = $derived(
    ventures
      .filter((v) => (v.status ?? 'active') === 'active')
      .sort((a, b) => a.name.localeCompare(b.name))
  );

  function colorVar(c?: string): string {
    const map: Record<string, string> = {
      red: 'error', yellow: 'warning', orange: 'accent', green: 'success',
      blue: 'secondary', purple: 'primary', cyan: 'info', mauve: 'primary',
      peach: 'accent', teal: 'info', sapphire: 'secondary', pink: 'accent',
      lavender: 'primary', flamingo: 'error'
    };
    return `var(--color-${map[c ?? ''] ?? 'secondary'})`;
  }
</script>

{#if loaded && active.length > 0}
  <section class="bg-surface0 border border-surface1 rounded-lg p-4">
    <div class="flex items-baseline justify-between mb-3">
      <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Ventures</h2>
      <a href="/ventures" class="text-xs text-secondary hover:underline">all →</a>
    </div>
    <ul class="space-y-1.5">
      {#each active as v (v.name)}
        {@const projectsCount = v.project_count ?? 0}
        {@const goalsCount = v.goal_count ?? 0}
        <li>
          <a
            href={`/projects?venture=${encodeURIComponent(v.name)}`}
            class="flex items-center gap-2.5 px-2.5 py-1.5 rounded hover:bg-mantle/40 group"
          >
            <span class="w-2 h-2 rounded-full flex-shrink-0" style="background: {colorVar(v.color)}"></span>
            <span class="text-sm text-text flex-1 min-w-0 truncate group-hover:text-primary">{v.name}</span>
            <span class="text-[11px] text-dim flex-shrink-0 tabular-nums">
              {projectsCount}p · {goalsCount}g
            </span>
          </a>
        </li>
      {/each}
    </ul>
  </section>
{/if}
