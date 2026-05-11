<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { api, type Note } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { createCoalescedReload } from '$lib/util/coalesce';
  import { relativeTime } from '$lib/util/relativeTime';
  import { pinnedNotes, ensurePinnedLoaded } from '$lib/notes/pinnedNotes';

  // Pinned notes with folder breadcrumb + relative-time tag.
  // Uses the shared pinnedNotes store so the star toggle in the
  // notes tree updates this widget instantly. We need note titles
  // (the store only carries paths), so we fetch the notes list once
  // and resolve in $derived. Title fallback is the basename when a
  // note is dangling (was renamed/deleted but pin is stale).
  //
  // Renders nothing pinned → a gentle hint to use the ★ button in
  // the notes header. Renders pinned → folder/title rows with a
  // mod-time chip so the user knows which version they're touching.

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

  const reload = createCoalescedReload(loadNotes, 600);
  onMount(() => {
    void loadNotes();
    ensurePinnedLoaded();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') reload.trigger();
    });
  });
  onDestroy(reload.cancel);

  let pinned = $derived.by<{ path: string; title: string; folder: string | null; modTime: string | null }[]>(() => {
    if ($pinnedNotes.size === 0) return [];
    const byPath = new Map<string, Note>();
    for (const n of allNotes) byPath.set(n.path, n);
    const out: { path: string; title: string; folder: string | null; modTime: string | null }[] = [];
    for (const p of $pinnedNotes) {
      const n = byPath.get(p);
      const parts = p.split('/');
      const folder = parts.length > 1 ? parts.slice(0, -1).join('/') : null;
      out.push({
        path: p,
        title: n?.title || p.split('/').pop()!.replace(/\.md$/, ''),
        folder,
        modTime: n?.modTime ?? null
      });
    }
    out.sort((a, b) => a.title.toLowerCase().localeCompare(b.title.toLowerCase()));
    return out;
  });

  // Compact "5m / 2h / 3d" labels with calendar fallback after a
  // week. The widget header is dense; full "ago" suffix would crowd.
  const relTime = (iso: string | null) =>
    relativeTime(iso, { compact: true, dateThresholdDays: 7 });
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-4">
  <div class="flex items-baseline justify-between mb-3">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">★ Pinned</h2>
    {#if pinned.length > 0}
      <span class="text-[11px] text-dim font-mono tabular-nums">{pinned.length}</span>
    {/if}
    <span class="flex-1"></span>
    <a href="/notes" class="text-xs text-secondary hover:underline">browse →</a>
  </div>
  {#if loading && pinned.length === 0 && $pinnedNotes.size > 0}
    <ul class="space-y-2">
      {#each [0, 1, 2] as i (i)}
        <li class="h-4 bg-surface1 rounded animate-pulse {i === 1 ? 'w-3/4' : ''}"></li>
      {/each}
    </ul>
  {:else if pinned.length === 0}
    <p class="text-sm text-dim italic leading-relaxed">
      Pin notes you reach for daily — passwords, accounts, install commands, references.
      Open any note → tap <span class="text-warning">★</span> in the header.
    </p>
  {:else}
    <ul class="space-y-1">
      {#each pinned as p (p.path)}
        <li>
          <a
            href="/notes/{encodeURIComponent(p.path)}"
            class="flex items-baseline gap-2 group py-1 px-2 -mx-2 rounded hover:bg-surface1 transition-colors"
          >
            <span class="text-warning flex-shrink-0">★</span>
            <span class="flex-1 min-w-0">
              <div class="flex items-baseline gap-1.5">
                {#if p.folder}
                  <span class="text-[10px] text-dim/80 flex-shrink-0 truncate max-w-[40%]">{p.folder}/</span>
                {/if}
                <span class="text-sm text-text group-hover:text-primary truncate">{p.title}</span>
              </div>
            </span>
            {#if p.modTime}
              <span class="text-[10px] text-dim font-mono flex-shrink-0">{relTime(p.modTime)}</span>
            {/if}
          </a>
        </li>
      {/each}
    </ul>
  {/if}
</section>
