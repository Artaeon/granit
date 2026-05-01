<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Project } from '$lib/api';
  import { onWsEvent } from '$lib/ws';

  let projects = $state<Project[]>([]);

  async function load() {
    try {
      const list = await api.listProjects();
      projects = list.projects
        .filter((p) => (p.status ?? 'active') === 'active')
        .sort((a, b) => (b.priority ?? 0) - (a.priority ?? 0))
        .slice(0, 5);
    } catch {
      projects = [];
    }
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type.startsWith('project.') || ev.type === 'note.changed') load();
    });
  });

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

<section class="bg-surface0 border border-surface1 rounded-lg p-4">
  <div class="flex items-baseline justify-between mb-3">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Active projects</h2>
    <a href="/projects" class="text-xs text-secondary hover:underline">all →</a>
  </div>
  {#if projects.length === 0}
    <div class="text-sm text-dim italic">no active projects</div>
  {:else}
    <ul class="space-y-3">
      {#each projects as p (p.name)}
        {@const pct = Math.round((p.progress ?? 0) * 100)}
        <li>
          <a href="/projects?p={encodeURIComponent(p.name)}" class="block group">
            <div class="flex items-baseline gap-2 mb-1">
              <span class="w-2 h-2 rounded-full flex-shrink-0" style="background: {colorVar(p.color)}"></span>
              <span class="text-sm text-text flex-1 truncate group-hover:text-primary">{p.name}</span>
              <span class="text-[10px] text-dim font-mono">{pct}%</span>
            </div>
            {#if p.next_action}
              <div class="text-xs text-warning truncate ml-4 mb-1">→ {p.next_action}</div>
            {/if}
            <div class="ml-4 h-1 rounded-full bg-base overflow-hidden">
              <div class="h-full" style="width: {pct}%; background: {colorVar(p.color)}"></div>
            </div>
          </a>
        </li>
      {/each}
    </ul>
  {/if}
</section>
