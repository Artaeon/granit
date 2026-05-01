<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, type Note } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';

  let tags = $state<{ tag: string; count: number }[]>([]);
  let loading = $state(false);

  let activeTag = $state<string | null>(null);
  let notes = $state<Note[]>([]);
  let notesLoading = $state(false);

  async function loadTags() {
    if (!$auth) return;
    loading = true;
    try {
      const r = await api.listTags();
      tags = r.tags;
    } finally {
      loading = false;
    }
  }

  async function selectTag(t: string) {
    activeTag = t;
    notesLoading = true;
    try {
      const r = await api.listNotes({ tag: t, limit: 500 });
      notes = r.notes;
    } finally {
      notesLoading = false;
    }
  }

  onMount(() => {
    loadTags();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') {
        loadTags();
        if (activeTag) selectTag(activeTag);
      }
    });
  });

  // Tag size scaling for the cloud — log-scale buckets
  function tagSize(count: number, max: number): string {
    if (max <= 0) return 'text-sm';
    const ratio = Math.log(1 + count) / Math.log(1 + max);
    if (ratio > 0.85) return 'text-2xl';
    if (ratio > 0.6) return 'text-xl';
    if (ratio > 0.4) return 'text-base';
    if (ratio > 0.2) return 'text-sm';
    return 'text-xs';
  }
  let max = $derived(tags[0]?.count ?? 0);
</script>

<div class="h-full overflow-y-auto">
  <div class="p-4 sm:p-6 lg:p-8 max-w-5xl mx-auto">
    <header class="mb-6">
      <h1 class="text-2xl sm:text-3xl font-semibold text-text">Tags</h1>
      <p class="text-sm text-dim mt-1">{tags.length} tag{tags.length === 1 ? '' : 's'} across the vault</p>
    </header>

    {#if loading && tags.length === 0}
      <div class="flex flex-wrap gap-x-3 gap-y-2">
        {#each Array(20) as _, i}
          <Skeleton class="h-{['5','6','7','5','6','7','5','6'][i % 8]} w-{['16','20','24','12','18','14','22','16'][i % 8]}" />
        {/each}
      </div>
    {:else if tags.length === 0}
      <EmptyState
        icon="#"
        title="No tags yet"
        description="Add `tags: [foo, bar]` to a note's frontmatter, or use #inline tags in the body. They show up here automatically."
      />
    {:else}
      <section class="mb-8">
        <h2 class="text-xs uppercase tracking-wider text-dim mb-3 font-medium">All tags</h2>
        <div class="flex flex-wrap items-baseline gap-x-3 gap-y-1.5">
          {#each tags as t (t.tag)}
            <button
              onclick={() => selectTag(t.tag)}
              class="{tagSize(t.count, max)} {activeTag === t.tag ? 'text-primary font-semibold' : 'text-secondary hover:text-primary'} transition-colors"
              title="{t.count} note{t.count !== 1 ? 's' : ''}"
            >
              #{t.tag}<span class="text-dim text-xs ml-0.5">{t.count}</span>
            </button>
          {/each}
        </div>
      </section>

      {#if activeTag}
        <section>
          <h2 class="text-xs uppercase tracking-wider text-dim mb-3 font-medium">
            #{activeTag} · {notes.length} note{notes.length === 1 ? '' : 's'}
          </h2>
          {#if notesLoading && notes.length === 0}
            <div class="text-sm text-dim">loading…</div>
          {:else}
            <ul class="space-y-1">
              {#each notes as n (n.path)}
                <li class="py-1.5 border-b border-surface0">
                  <a href="/notes/{encodeURIComponent(n.path)}" class="block group">
                    <div class="text-text group-hover:text-primary">{n.title}</div>
                    <div class="text-xs text-dim font-mono mt-0.5 truncate">{n.path}</div>
                  </a>
                </li>
              {/each}
            </ul>
          {/if}
        </section>
      {/if}
    {/if}
  </div>
</div>
