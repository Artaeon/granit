<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { api, type Note } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { createCoalescedReload } from '$lib/util/coalesce';

  // Recent notes with a one-line excerpt + tag chips, ordered by
  // mod-time. The original widget showed title + date only; that
  // works but doesn't help the user remember WHICH version of a
  // recurring title (Reading-2026-05-09 vs Reading-2026-05-08) is
  // the one they touched. Excerpt + tags solve recall without
  // crowding the row.

  let notes = $state<Note[]>([]);
  let loading = $state(false);

  // Coalesce WS-driven reloads — note.changed fires repeatedly while
  // the user types in the editor; without coalescing the widget
  // refetches on every keystroke and re-paints the rail. The shared
  // helper uses a trailing-edge throttle (the original code here was
  // a debounce, which would never fire under sustained typing).
  async function load() {
    loading = true;
    try {
      const list = await api.listNotes({ limit: 6 });
      notes = list.notes;
    } finally {
      loading = false;
    }
  }
  const reload = createCoalescedReload(() => load(), 600);

  onMount(() => {
    void load();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') reload.trigger();
    });
  });
  onDestroy(reload.cancel);

  // Relative date — feels warmer than "Apr 12" when the user is
  // looking at "what did I touch yesterday".
  function fmtRelative(iso: string): string {
    const then = new Date(iso).getTime();
    const now = Date.now();
    const min = Math.round((now - then) / 60_000);
    if (min < 1) return 'just now';
    if (min < 60) return `${min}m ago`;
    const h = Math.round(min / 60);
    if (h < 24) return `${h}h ago`;
    const d = Math.round(h / 24);
    if (d < 7) return `${d}d ago`;
    return new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
  }

  // Excerpt — first non-empty line of the body that isn't a heading,
  // frontmatter, or callout marker. Listing notes don't carry the
  // body, so the excerpt only fills in if/when the listing endpoint
  // includes one in the future. We deliberately don't getNote each
  // listed note (would be N+1 round-trips per dashboard load).
  function excerpt(n: Note): string {
    const src = ((n.body ?? '') as string).trim();
    if (!src) return '';
    const lines = src.split(/\r?\n/);
    for (const ln of lines) {
      const t = ln.trim();
      if (!t) continue;
      if (t.startsWith('#')) continue;
      if (t.startsWith('---')) continue;
      if (t.startsWith('::')) continue; // callout-style markers
      return t.length > 90 ? t.slice(0, 90) + '…' : t;
    }
    return '';
  }

  // Folder breadcrumb — just the top folder, dimmer than the title.
  // Helps disambiguate notes with the same title across folders.
  function topFolder(path: string): string | null {
    const parts = path.split('/');
    if (parts.length < 2) return null;
    return parts[0];
  }
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-4">
  <div class="flex items-baseline justify-between mb-2">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Recent</h2>
    <a href="/notes" class="text-xs text-secondary hover:underline">all →</a>
  </div>
  {#if loading && notes.length === 0}
    <ul class="space-y-2">
      {#each [0, 1, 2, 3] as i (i)}
        <li class="flex items-baseline gap-2">
          <span class="flex-1 h-3 bg-surface1 rounded animate-pulse"></span>
          <span class="w-12 h-3 bg-surface1 rounded animate-pulse"></span>
        </li>
      {/each}
    </ul>
  {:else if notes.length === 0}
    <p class="text-sm text-dim italic">No notes yet.</p>
  {:else}
    <ul class="space-y-2">
      {#each notes as n (n.path)}
        {@const ex = excerpt(n)}
        {@const folder = topFolder(n.path)}
        <li>
          <a
            href="/notes/{encodeURIComponent(n.path)}"
            class="block py-1 px-2 -mx-2 rounded hover:bg-surface1/60 transition-colors group"
          >
            <div class="flex items-baseline gap-2">
              {#if folder}
                <span class="text-[10px] text-dim/80 flex-shrink-0">{folder}/</span>
              {/if}
              <span class="text-sm text-text font-medium truncate flex-1 group-hover:text-primary">{n.title}</span>
              <span class="text-[10px] text-dim flex-shrink-0">{fmtRelative(n.modTime)}</span>
            </div>
            {#if ex}
              <p class="text-[11px] text-dim mt-0.5 truncate">{ex}</p>
            {/if}
            {#if n.tags && n.tags.length > 0}
              <div class="flex flex-wrap gap-1 mt-1">
                {#each n.tags.slice(0, 3) as t (t)}
                  <span class="text-[10px] px-1.5 py-0 rounded bg-surface1 text-accent">#{t}</span>
                {/each}
                {#if n.tags.length > 3}
                  <span class="text-[10px] text-dim">+{n.tags.length - 3}</span>
                {/if}
              </div>
            {/if}
          </a>
        </li>
      {/each}
    </ul>
  {/if}
</section>
