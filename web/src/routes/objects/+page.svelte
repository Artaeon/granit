<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { auth } from '$lib/stores/auth';
  import { api, type ObjectType, type ObjectInstance } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';

  let types = $state<ObjectType[]>([]);
  let untyped = $state(0);
  let loading = $state(false);

  let activeType = $state<ObjectType | null>(null);
  let objects = $state<ObjectInstance[]>([]);
  let objLoading = $state(false);

  // Inline filter — narrows the visible objects by title or any
  // property value (case-insensitive substring). Cheap because the
  // full set already fits in memory; switching to a debounced
  // server-side query is overkill until a single type holds 5k+ rows.
  let filterText = $state('');

  // Sort key — 'title' is the default Capacities/Notion-style listing;
  // 'modified' lets the user surface what they've recently touched
  // (handy for picking up a half-finished note); 'created' shows
  // newest-first which works for entry-style types like reading log.
  type SortKey = 'title' | 'modified' | 'created';
  let sortKey = $state<SortKey>('title');

  let filtered = $derived.by(() => {
    const q = filterText.trim().toLowerCase();
    let list = objects;
    if (q) {
      list = list.filter((o) => {
        if (o.title.toLowerCase().includes(q)) return true;
        if (o.path.toLowerCase().includes(q)) return true;
        if (o.properties) {
          for (const v of Object.values(o.properties)) {
            if (typeof v === 'string' && v.toLowerCase().includes(q)) return true;
          }
        }
        return false;
      });
    }
    // Stable sort copy — never mutate the source array (it's $state
    // and sorting in place would refire the derive).
    const out = [...list];
    if (sortKey === 'title') {
      out.sort((a, b) => a.title.localeCompare(b.title));
    } else if (sortKey === 'modified') {
      out.sort((a, b) => (b.modifiedTime ?? 0) - (a.modifiedTime ?? 0));
    } else if (sortKey === 'created') {
      out.sort((a, b) => (b.createdTime ?? 0) - (a.createdTime ?? 0));
    }
    return out;
  });

  // Create-new dialog state. Kept inline rather than a separate
  // component — only one entry point and the form is two fields.
  let createOpen = $state(false);
  let createTitle = $state('');
  let createBusy = $state(false);

  function openCreate() {
    if (!activeType) return;
    createTitle = '';
    createOpen = true;
  }

  async function submitCreate() {
    const t = activeType;
    if (!t || !createTitle.trim() || createBusy) return;
    createBusy = true;
    try {
      // Slug: keep alphanum + dashes/underscores, collapse spaces. The
      // server enforces uniqueness; collisions surface as 409 and the
      // user retries with a different title. Folder defaults to vault
      // root if the type doesn't declare one.
      const slug = createTitle.trim().replace(/[^\p{L}\p{N}_-]+/gu, '-').replace(/^-+|-+$/g, '');
      const folder = (t.folder ?? '').replace(/\/+$/, '');
      const path = folder ? `${folder}/${slug}.md` : `${slug}.md`;
      await api.createNote({
        path,
        frontmatter: { type: t.id, title: createTitle.trim() },
        body: `# ${createTitle.trim()}\n\n`
      });
      createOpen = false;
      toast.success(`${t.name} created`);
      goto(`/notes/${encodeURIComponent(path)}`);
    } catch (e) {
      toast.error('failed to create: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      createBusy = false;
    }
  }

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
    <header class="px-3 sm:px-4 py-3 border-b border-surface1 flex flex-wrap items-center gap-3 flex-shrink-0">
      <h1 class="text-xl sm:text-2xl font-semibold text-text flex items-baseline gap-2">
        <span>{activeType?.icon ?? '◇'}</span>
        <span>{activeType?.name ?? 'Objects'}</span>
      </h1>
      {#if activeType}
        <span class="text-xs text-dim">
          {filtered.length}{filterText && filtered.length !== objects.length ? ` of ${objects.length}` : ''}
        </span>
        {#if activeType.folder}
          <span class="text-xs text-dim hidden sm:inline">· folder <code class="text-[10px]">{activeType.folder}/</code></span>
        {/if}
        <span class="flex-1"></span>
        <input
          bind:value={filterText}
          placeholder="filter…"
          class="w-32 sm:w-48 bg-mantle border border-surface1 rounded px-2 py-1 text-xs text-text placeholder-dim focus:outline-none focus:border-primary"
        />
        <select
          bind:value={sortKey}
          class="bg-mantle border border-surface1 rounded px-2 py-1 text-xs text-text focus:outline-none focus:border-primary"
          aria-label="sort"
        >
          <option value="title">A → Z</option>
          <option value="modified">Recent</option>
          <option value="created">Newest</option>
        </select>
        <button
          type="button"
          onclick={openCreate}
          class="text-xs px-2.5 py-1 rounded bg-primary text-on-primary font-medium hover:opacity-90"
        >+ New {activeType.name}</button>
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
      {:else if filtered.length === 0}
        <div class="text-sm text-dim italic">no objects match this filter.</div>
      {:else}
        {@const cols = summaryProps(activeType)}
        <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-3">
          {#each filtered as o (o.path)}
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

<!-- Create-new modal. Single title field; the server fills the rest
     from the type's defaults. Slug-encodes the title for the path. -->
{#if createOpen && activeType}
  <div
    class="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4"
    onclick={() => (createOpen = false)}
    role="dialog"
    tabindex="-1"
    onkeydown={(e) => { if (e.key === 'Escape') createOpen = false; }}
  >
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <div
      onclick={(e) => e.stopPropagation()}
      onkeydown={(e) => e.stopPropagation()}
      class="w-full max-w-sm bg-mantle border border-surface1 rounded-lg shadow-xl"
      role="document"
    >
      <header class="px-4 py-3 border-b border-surface1">
        <h2 class="text-base font-semibold text-text">
          New {activeType.icon ?? '◇'} {activeType.name}
        </h2>
      </header>
      <form onsubmit={(e) => { e.preventDefault(); submitCreate(); }} class="p-4 space-y-3">
        <label class="block">
          <span class="text-xs text-dim">Title</span>
          <input
            bind:value={createTitle}
            placeholder={`${activeType.name} title…`}
            disabled={createBusy}
            class="mt-1 w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary"
          />
        </label>
        <p class="text-[11px] text-dim">
          Saved to <code>{activeType.folder ?? '(vault root)'}/</code> with frontmatter <code>type: {activeType.id}</code>.
        </p>
        <div class="flex gap-2 justify-end pt-1">
          <button
            type="button"
            onclick={() => (createOpen = false)}
            class="text-xs px-3 py-1.5 rounded bg-surface0 text-subtext hover:bg-surface1"
          >Cancel</button>
          <button
            type="submit"
            disabled={createBusy || !createTitle.trim()}
            class="text-xs px-3 py-1.5 rounded bg-primary text-on-primary font-medium hover:opacity-90 disabled:opacity-50"
          >{createBusy ? '…' : 'Create'}</button>
        </div>
      </form>
    </div>
  </div>
{/if}
