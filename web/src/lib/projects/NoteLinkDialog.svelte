<script lang="ts">
  import { api, type Note } from '$lib/api';
  import { onMount, untrack } from 'svelte';

  // Modal search dialog used by the "Notes" section to link an
  // existing note to a project. Opens centred (not a side drawer)
  // because the user's mental model here is "pick from a list",
  // closer to a command palette than a property panel.
  //
  // Server-side full-text search is the seed; client-side dedup
  // strips notes already linked to the project (passed as
  // excludePaths). Debounced by 200ms — note search is cheap on the
  // server but the keystroke storm on a slow connection still pays
  // dividends from rate-limiting.

  let {
    open = $bindable(false),
    excludePaths = [],
    onPick
  }: {
    open?: boolean;
    /** Note paths to hide from results — typically notes already
     *  linked to the project, so the user can't accidentally
     *  re-link a note already in the list. */
    excludePaths?: string[];
    onPick: (note: Note) => void | Promise<void>;
  } = $props();

  let q = $state('');
  let results = $state<Note[]>([]);
  let loading = $state(false);
  let cursor = $state(0);
  let inputEl = $state<HTMLInputElement | null>(null);

  // Reset state every time the dialog (re)opens so a stale query
  // from a prior project doesn't carry over. Using untrack on the
  // results read so we only re-run on `open` flip, not on every
  // keystroke that changes results.
  $effect(() => {
    if (open) {
      untrack(() => {
        q = '';
        results = [];
        cursor = 0;
      });
      // Focus the input once the dialog has mounted; rAF defers
      // until after the open transition starts so iOS doesn't ignore
      // the focus during the transform.
      requestAnimationFrame(() => inputEl?.focus());
    }
  });

  // Debounced server search. Empty query lists the most recent notes
  // (no q param) so the user has something to pick from immediately.
  let searchAbort: AbortController | null = null;
  let searchTimer: ReturnType<typeof setTimeout> | null = null;
  $effect(() => {
    void q;
    if (!open) return;
    if (searchTimer) clearTimeout(searchTimer);
    searchTimer = setTimeout(() => void runSearch(), 200);
  });

  async function runSearch() {
    searchAbort?.abort();
    searchAbort = new AbortController();
    loading = true;
    try {
      const term = q.trim();
      const res = await api.listNotes({
        q: term || undefined,
        limit: 30
      });
      // Only commit if this is still the latest query — abort logic
      // doesn't help across the listNotes call (no signal support),
      // so we cross-check the in-flight q against the live one.
      if (term !== q.trim() && term !== '' && q.trim() !== '') return;
      const exclude = new Set(excludePaths);
      results = res.notes.filter((n) => !exclude.has(n.path));
      cursor = 0;
    } catch (e) {
      console.error('note search failed', e);
      results = [];
    } finally {
      loading = false;
    }
  }

  function noteTitle(n: Note): string {
    if (n.title && n.title.trim() !== '') return n.title;
    const base = n.path.split('/').pop() ?? n.path;
    return base.replace(/\.md$/i, '');
  }

  function close() {
    open = false;
  }

  async function pick(n: Note) {
    await onPick(n);
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      close();
      return;
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      cursor = Math.min(cursor + 1, Math.max(0, results.length - 1));
      return;
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault();
      cursor = Math.max(cursor - 1, 0);
      return;
    }
    if (e.key === 'Enter' && results[cursor]) {
      e.preventDefault();
      void pick(results[cursor]);
      return;
    }
  }

  onMount(() => {
    // Initial open might fire before the effect runs (server-side
    // bind), so kick off the search defensively.
    if (open) void runSearch();
  });
</script>

{#if open}
  <!-- Backdrop. Clicking dismisses; the dialog body stops propagation. -->
  <button
    onclick={close}
    aria-label="close note picker"
    class="fixed inset-0 z-40 bg-black/50"
  ></button>

  <div
    class="fixed inset-0 z-50 flex items-start justify-center p-4 sm:p-8 pointer-events-none"
    role="dialog"
    aria-modal="true"
    aria-label="link a note to this project"
  >
    <div class="bg-base border border-surface1 rounded-lg shadow-2xl w-full max-w-2xl max-h-[85dvh] flex flex-col pointer-events-auto overflow-hidden">
      <header class="px-4 py-3 border-b border-surface1 flex items-center gap-2 flex-shrink-0">
        <span class="text-sm font-medium text-text flex-1">Link an existing note</span>
        <button
          onclick={close}
          aria-label="close"
          class="w-8 h-8 flex items-center justify-center text-dim hover:text-text rounded"
        >×</button>
      </header>

      <div class="px-4 py-3 border-b border-surface1 flex-shrink-0">
        <input
          bind:this={inputEl}
          bind:value={q}
          onkeydown={onKey}
          placeholder="search vault for a note…"
          class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
        />
        <p class="text-[11px] text-dim mt-1">↑↓ to navigate · ↵ to link · esc to close</p>
      </div>

      <div class="flex-1 overflow-y-auto">
        {#if loading && results.length === 0}
          <div class="p-4 text-sm text-dim italic">searching…</div>
        {:else if results.length === 0}
          <div class="p-4 text-sm text-dim italic">
            {q.trim() ? 'No matches.' : 'Type to search, or pick from the most recent notes.'}
          </div>
        {:else}
          <ul>
            {#each results as n, i (n.path)}
              <li>
                <button
                  onclick={() => void pick(n)}
                  onmouseenter={() => (cursor = i)}
                  class="w-full text-left px-4 py-2.5 border-b border-surface1/50 transition-colors {i === cursor ? 'bg-surface1' : 'hover:bg-surface0'}"
                >
                  <div class="flex items-baseline gap-2">
                    <span class="text-sm text-text font-medium truncate flex-1">{noteTitle(n)}</span>
                    {#if n.modTime}
                      <span class="text-[10px] text-dim font-mono flex-shrink-0">{n.modTime.slice(0, 10)}</span>
                    {/if}
                  </div>
                  <p class="text-[11px] text-dim font-mono truncate mt-0.5">{n.path}</p>
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      </div>
    </div>
  </div>
{/if}
