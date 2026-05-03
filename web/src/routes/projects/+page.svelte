<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { api, type Project } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import ProjectDetail from '$lib/projects/ProjectDetail.svelte';
  import ProjectCreate from '$lib/projects/ProjectCreate.svelte';

  let projects = $state<Project[]>([]);
  let loading = $state(false);
  let q = $state('');
  let statusFilter = $state<'all' | 'active' | 'paused' | 'completed' | 'archived'>('active');
  let createOpen = $state(false);

  async function load() {
    loading = true;
    try {
      const r = await api.listProjects();
      projects = r.projects;
    } catch (e) {
      toast.error('failed to load projects: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type.startsWith('project.')) load();
      if (ev.type === 'note.changed' || ev.type === 'note.removed') load();
    });
  });

  // Selected project name from query param. Two-pane layout: list left, detail right.
  let selectedName = $derived($page.url.searchParams.get('p') ?? '');
  let selected = $derived(projects.find((p) => p.name === selectedName) ?? null);

  function selectProject(name: string) {
    const params = new URLSearchParams($page.url.searchParams);
    if (name) params.set('p', name);
    else params.delete('p');
    goto(`/projects?${params.toString()}`, { replaceState: true, keepFocus: true });
  }

  let filtered = $derived.by(() => {
    let list = projects;
    if (statusFilter !== 'all') list = list.filter((p) => (p.status ?? 'active') === statusFilter);
    const term = q.trim().toLowerCase();
    if (term) {
      list = list.filter((p) =>
        p.name.toLowerCase().includes(term) ||
        (p.description ?? '').toLowerCase().includes(term) ||
        (p.tags ?? []).some((t) => t.toLowerCase().includes(term))
      );
    }
    // Sort: active first → priority desc → name
    return [...list].sort((a, b) => {
      const sa = a.status ?? 'active';
      const sb = b.status ?? 'active';
      if (sa !== sb) {
        const order = { active: 0, paused: 1, completed: 2, archived: 3 } as Record<string, number>;
        return (order[sa] ?? 9) - (order[sb] ?? 9);
      }
      const pa = a.priority ?? 0;
      const pb = b.priority ?? 0;
      if (pa !== pb) return pb - pa;
      return a.name.localeCompare(b.name);
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

  function statusTone(status: string): string {
    if (status === 'active') return 'success';
    if (status === 'paused') return 'warning';
    if (status === 'completed') return 'info';
    if (status === 'archived') return 'subtext';
    return 'subtext';
  }

  async function created(p: Project) {
    createOpen = false;
    await load();
    selectProject(p.name);
  }

  async function deleted(name: string) {
    selectProject('');
    await load();
    toast.success(`project "${name}" deleted`);
  }
</script>

<div class="h-full flex">
  <!-- List -->
  <aside class="w-full md:w-72 lg:w-80 xl:w-96 flex-shrink-0 border-r border-surface1 bg-mantle/40 flex flex-col {selectedName ? 'hidden md:flex' : ''}">
    <header class="px-3 py-2.5 border-b border-surface1 flex items-center gap-2 flex-shrink-0">
      <h2 class="text-sm font-medium text-text flex-1">Projects</h2>
      <button
        onclick={() => (createOpen = true)}
        class="px-2.5 py-1 text-xs bg-primary text-on-primary rounded hover:opacity-90"
      >+ new</button>
    </header>
    <div class="px-3 py-2 space-y-2 flex-shrink-0">
      <input
        bind:value={q}
        placeholder="filter…"
        class="w-full px-2 py-1.5 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
      />
      <div class="flex gap-1 text-xs">
        {#each ['active', 'paused', 'completed', 'archived', 'all'] as s}
          <button
            class="flex-1 px-1 py-0.5 rounded {statusFilter === s ? 'bg-surface1 text-text' : 'text-dim hover:text-text'}"
            onclick={() => (statusFilter = s as typeof statusFilter)}
          >{s}</button>
        {/each}
      </div>
    </div>
    <div class="flex-1 overflow-y-auto">
      {#if loading && projects.length === 0}
        <div class="p-4 text-sm text-dim">loading…</div>
      {:else if filtered.length === 0}
        <div class="p-4 text-sm text-dim italic">no projects</div>
      {:else}
        <ul class="divide-y divide-surface1">
          {#each filtered as p (p.name)}
            {@const active = p.name === selectedName}
            {@const progress = p.progress ?? 0}
            <li>
              <button
                onclick={() => selectProject(p.name)}
                class="w-full text-left px-3 py-2.5 hover:bg-surface0 {active ? 'bg-surface1' : ''}"
              >
                <div class="flex items-baseline gap-2 mb-1">
                  <span class="w-2 h-2 rounded-full flex-shrink-0" style="background: {colorVar(p.color)}"></span>
                  <span class="text-sm font-medium text-text flex-1 truncate">{p.name}</span>
                  <span
                    class="text-[10px] uppercase tracking-wider flex-shrink-0"
                    style="color: var(--color-{statusTone(p.status ?? 'active')})"
                  >{p.status ?? 'active'}</span>
                </div>
                {#if p.description}
                  <p class="text-xs text-subtext line-clamp-2 mb-1.5">{p.description}</p>
                {/if}
                <div class="flex items-center gap-2 text-[10px]">
                  <div class="flex-1 h-1 rounded-full bg-surface0 overflow-hidden">
                    <div
                      class="h-full"
                      style="width: {Math.round(progress * 100)}%; background: {colorVar(p.color)}"
                    ></div>
                  </div>
                  <span class="text-dim font-mono w-10 text-right">{Math.round(progress * 100)}%</span>
                  {#if p.tasksTotal != null && p.tasksTotal > 0}
                    <span class="text-dim">{p.tasksDone}/{p.tasksTotal}</span>
                  {/if}
                </div>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  </aside>

  <!-- Detail -->
  <main class="flex-1 min-w-0 {selectedName ? 'block' : 'hidden md:block'}">
    {#if selected}
      <ProjectDetail
        project={selected}
        onClose={() => selectProject('')}
        onUpdated={load}
        onDeleted={deleted}
      />
    {:else}
      <div class="h-full flex items-center justify-center text-dim text-sm">
        Select a project from the list, or create a new one.
      </div>
    {/if}
  </main>
</div>

<ProjectCreate bind:open={createOpen} onCreated={created} />
