<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Note } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import { pinnedNotes, ensurePinnedLoaded } from '$lib/notes/pinnedNotes';

  // Use the shared pinnedNotes store so a star toggle in the notes
  // tree updates this widget instantly. We still need note titles
  // (the store only carries paths), so we fetch the notes list once
  // and resolve in $derived. Title fallback is the basename when a
  // note is dangling (was renamed/deleted but pin is stale).

  let allNotes = $state<Note[]>([]);
  let loading = $state(false);

  async function loadNotes() {
    loading = true;
    try {
      const r = await api.listNotes({ limit: 5000 });
      allNotes = r.notes;
    } finally {
      loading = false;
    }
  }

  let pinned = $derived.by<{ path: string; title: string }[]>(() => {
    if ($pinnedNotes.size === 0) return [];
    const byPath = new Map<string, Note>();
    for (const n of allNotes) byPath.set(n.path, n);
    const out: { path: string; title: string }[] = [];
    for (const p of $pinnedNotes) {
      const n = byPath.get(p);
      out.push({ path: p, title: n?.title || p.replace(/\.md$/, '') });
    }
    out.sort((a, b) => a.title.toLowerCase().localeCompare(b.title.toLowerCase()));
    return out;
  });

  onMount(() => {
    loadNotes();
    ensurePinnedLoaded();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') loadNotes();
    });
  });
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-4">
  <div class="flex items-baseline justify-between mb-3">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">★ Pinned</h2>
    <a href="/notes" class="text-xs text-secondary hover:underline">browse →</a>
  </div>
  {#if loading && pinned.length === 0}
    <div class="space-y-2">
      <Skeleton class="h-4 w-3/4" />
      <Skeleton class="h-4 w-2/3" />
      <Skeleton class="h-4 w-1/2" />
    </div>
  {:else if pinned.length === 0}
    <div class="text-sm text-dim italic leading-relaxed">
      pin notes you need fast access to from any device — passwords, nextcloud info, account tokens, install commands.
      open any note → tap ★ in the header.
    </div>
  {:else}
    <ul class="space-y-1.5">
      {#each pinned as p (p.path)}
        <li>
          <a href="/notes/{encodeURIComponent(p.path)}" class="flex items-baseline gap-2 group">
            <span class="text-warning">★</span>
            <span class="flex-1 text-text group-hover:text-primary truncate">{p.title}</span>
          </a>
        </li>
      {/each}
    </ul>
  {/if}
</section>
