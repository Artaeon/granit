<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, type ObjectType, type ObjectInstance } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';

  let types = $state<ObjectType[]>([]);
  let untyped = $state(0);
  let loading = $state(false);

  let activeType = $state<ObjectType | null>(null);
  let objects = $state<ObjectInstance[]>([]);
  let objLoading = $state(false);

  async function loadTypes() {
    if (!$auth) return;
    loading = true;
    try {
      const r = await api.listTypes();
      types = r.types;
      untyped = r.untyped;
      if (!activeType && types.length > 0) {
        await selectType(types[0]);
      }
    } finally {
      loading = false;
    }
  }

  async function selectType(t: ObjectType) {
    activeType = t;
    objLoading = true;
    try {
      const r = await api.listTypeObjects(t.id);
      objects = r.objects;
    } finally {
      objLoading = false;
    }
  }

  onMount(() => {
    loadTypes();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') {
        loadTypes();
        if (activeType) selectType(activeType);
      }
    });
  });

  // pick a few "summary" properties to show in the gallery (max 3)
  function summaryProps(t: ObjectType): string[] {
    if (!t.properties) return [];
    return t.properties
      .filter((p) => p.kind !== 'checkbox' && p.kind !== 'tag')
      .slice(0, 3)
      .map((p) => p.name);
  }
</script>

<div class="h-full flex">
  <!-- Type list (sidebar) -->
  <aside class="hidden md:flex md:flex-col md:w-64 lg:w-72 border-r border-surface1 bg-mantle/40 flex-shrink-0">
    <div class="px-3 py-3 border-b border-surface1">
      <h2 class="text-sm font-semibold text-text">Types</h2>
      <p class="text-xs text-dim mt-0.5">{types.length} types · {untyped} untyped notes</p>
    </div>
    <ul class="flex-1 overflow-y-auto p-2 space-y-0.5">
      {#each types as t (t.id)}
        <li>
          <button
            onclick={() => selectType(t)}
            class="w-full text-left flex items-center gap-2 px-2 py-1.5 rounded text-sm
              {activeType?.id === t.id ? 'bg-surface1 text-primary' : 'text-subtext hover:bg-surface0'}"
          >
            <span class="w-5 text-center">{t.icon ?? '◇'}</span>
            <span class="flex-1 truncate">{t.name}</span>
            <span class="text-xs text-dim">{t.count}</span>
          </button>
        </li>
      {/each}
    </ul>
  </aside>

  <div class="flex-1 flex flex-col min-w-0">
    <header class="px-3 sm:px-4 py-3 border-b border-surface1 flex items-baseline gap-3 flex-shrink-0">
      <h1 class="text-xl sm:text-2xl font-semibold text-text flex items-baseline gap-2">
        <span>{activeType?.icon ?? '◇'}</span>
        <span>{activeType?.name ?? 'Objects'}</span>
      </h1>
      {#if activeType}
        <span class="text-xs text-dim">{objects.length}</span>
        {#if activeType.folder}
          <span class="text-xs text-dim hidden sm:inline">· folder <code class="text-[10px]">{activeType.folder}/</code></span>
        {/if}
      {/if}
    </header>

    <!-- Mobile type selector -->
    <div class="md:hidden flex overflow-x-auto gap-1 px-3 py-2 border-b border-surface1 flex-shrink-0">
      {#each types as t (t.id)}
        <button
          onclick={() => selectType(t)}
          class="flex-shrink-0 flex items-center gap-1 px-2.5 py-1.5 rounded text-sm
            {activeType?.id === t.id ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext'}"
        >
          <span>{t.icon ?? '◇'}</span><span>{t.name}</span><span class="opacity-70 text-xs">{t.count}</span>
        </button>
      {/each}
    </div>

    <div class="flex-1 overflow-auto p-3 sm:p-4">
      {#if loading && types.length === 0}
        <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-3">
          {#each Array(6) as _}
            <div class="bg-surface0 border border-surface1 rounded-lg p-3 space-y-2">
              <Skeleton class="h-4 w-3/4" />
              <Skeleton class="h-3 w-1/2" />
              <Skeleton class="h-3 w-2/3" />
            </div>
          {/each}
        </div>
      {:else if !activeType}
        <EmptyState icon="◇" title="Pick a type" description="Select a type from the sidebar to see notes that share its schema." />
      {:else if objLoading && objects.length === 0}
        <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-3">
          {#each Array(4) as _}
            <Skeleton class="h-20 w-full" />
          {/each}
        </div>
      {:else if objects.length === 0 && activeType}
        {@const t = activeType}
        <EmptyState
          icon={t.icon ?? '◇'}
          title={`No ${t.name} notes yet`}
          description={`Notes get this type when they have type: ${t.id} in their frontmatter. Create one in the TUI or set the frontmatter manually.`}
        />
      {:else}
        {@const cols = summaryProps(activeType)}
        <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-3">
          {#each objects as o (o.path)}
            <a
              href="/notes/{encodeURIComponent(o.path)}"
              class="bg-surface0 border border-surface1 rounded-lg p-3 hover:border-primary/40 transition-colors block"
            >
              <div class="font-medium text-text truncate">{o.title}</div>
              <div class="text-xs text-dim font-mono mt-0.5 truncate">{o.path}</div>
              {#if cols.length > 0 && o.properties}
                <dl class="mt-2 space-y-0.5">
                  {#each cols as p}
                    {#if o.properties[p]}
                      <div class="flex gap-2 text-xs">
                        <dt class="text-dim flex-shrink-0">{p}</dt>
                        <dd class="text-subtext truncate">{o.properties[p]}</dd>
                      </div>
                    {/if}
                  {/each}
                </dl>
              {/if}
            </a>
          {/each}
        </div>
      {/if}
    </div>
  </div>
</div>
