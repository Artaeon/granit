<script lang="ts">
  // RightPaneNotes — slim recent-notes list (latest 15). The /notes
  // endpoint already returns the listing ordered by modTime descending
  // (RecentNotesWidget consumes the same shape), so the right-pane
  // can stay dumb and trust the server order.
  //
  // Each row is: title | folder-prefix | updated-relative. Reuses
  // relativeTime so the format matches the dashboard's recent rail.
  // WS-driven reloads coalesced for the same reason — editor typing
  // fires note.changed per-keystroke.

  import { onMount, onDestroy } from 'svelte';
  import { api, type Note } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { createCoalescedReload } from '$lib/util/coalesce';
  import { relativeTime } from '$lib/util/relativeTime';

  let notes = $state<Note[]>([]);
  let loading = $state(true);
  let error = $state(false);

  async function load() {
    try {
      const list = await api.listNotes({ limit: 15 });
      notes = list.notes;
      error = false;
    } catch {
      error = true;
    } finally {
      loading = false;
    }
  }
  const reload = createCoalescedReload(load, 600);

  onMount(() => {
    void load();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') reload.trigger();
    });
  });
  onDestroy(reload.cancel);

  const fmtRelative = (iso: string) => relativeTime(iso, { dateThresholdDays: 7 });

  function topFolder(path: string): string | null {
    const parts = path.split('/');
    if (parts.length < 2) return null;
    return parts[0];
  }
</script>

<div class="flex flex-col h-full text-sm">
  {#if loading}
    <div class="p-3 space-y-2">
      {#each [0, 1, 2, 3, 4] as i (i)}
        <div class="h-3 w-full bg-surface1 rounded animate-pulse"></div>
      {/each}
    </div>
  {:else if error}
    <p class="p-3 text-dim italic text-xs">Couldn't load notes.</p>
  {:else if notes.length === 0}
    <p class="p-3 text-dim italic text-xs">No notes yet.</p>
  {:else}
    <ul class="flex-1 overflow-y-auto px-1 py-1 space-y-0">
      {#each notes as n (n.path)}
        {@const folder = topFolder(n.path)}
        <li>
          <a
            href="/notes/{encodeURIComponent(n.path)}"
            class="block px-2 py-1.5 rounded hover:bg-surface0 transition-colors group"
          >
            <div class="flex items-baseline gap-2 min-w-0">
              <span class="text-xs text-text truncate flex-1 group-hover:text-primary" title={n.title}>{n.title}</span>
              <span class="text-[10px] text-dim flex-shrink-0 tabular-nums">{fmtRelative(n.modTime)}</span>
            </div>
            {#if folder}
              <div class="text-[10px] text-dim/80 truncate">{folder}/</div>
            {/if}
          </a>
        </li>
      {/each}
    </ul>

    <footer class="border-t border-surface1 px-3 py-1.5 flex-shrink-0">
      <a href="/notes" class="text-xs text-secondary hover:underline">Open Notes →</a>
    </footer>
  {/if}
</div>
