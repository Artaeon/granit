<!--
  BibleBookmarksMode — the saved-passages panel that used to live as
  the {:else if mode === 'bookmarks'} branch inside
  web/src/routes/scripture/+page.svelte. Extracted because the
  scripture page is ~2100 lines and bookmarks is a fully
  self-contained read+delete+annotate surface backed by its own JSON
  file (.granit/bible-bookmarks.json).

  Owns:
    - bookmarks list state + lazy load
    - delete / save-note actions
    - WS subscription for the bookmark file (auto-reload when the TUI
      or another tab adds/edits a bookmark)
    - the panel markup

  Does NOT own:
    - the create path (bookmarkPassage / bookmarkVerse stay in the
      parent because they're triggered from the bible-reader buttons,
      not from this panel). Server emits state.changed on .granit/
      bible-bookmarks.json after each create; the WS listener here
      picks it up and reloads automatically.
    - opening a bookmark in the reader. The parent owns the bible
      reader's mode + chapter state; we route through onOpenBookmark.

  Props
    active           true when the parent's mode === 'bookmarks'. The
                     component lazy-loads on first true and reloads on
                     WS events thereafter.
    onOpenBookmark   parent callback that switches the scripture page
                     into bible mode at the bookmark's chapter and
                     scrolls to its first verse.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type BibleBookmark } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { onWsEvent } from '$lib/ws';

  let {
    active,
    onOpenBookmark
  }: {
    active: boolean;
    onOpenBookmark: (b: BibleBookmark) => void | Promise<void>;
  } = $props();

  let bookmarks = $state<BibleBookmark[]>([]);
  let loaded = $state(false);

  async function load() {
    try {
      const r = await api.listBibleBookmarks();
      bookmarks = r.bookmarks;
      loaded = true;
    } catch (e) {
      toast.error('failed to load bookmarks: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // Lazy: only fetch when the tab first becomes active. Reloads
  // afterwards happen through the WS listener below.
  $effect(() => {
    if (active && !loaded) {
      void load();
    }
  });

  // WS subscription scoped to the bookmark file only. Listens
  // regardless of `active` so a bookmark added from the bible-reader
  // (which calls api.createBibleBookmark) refreshes the panel even
  // though the user isn't looking at it yet.
  onMount(() =>
    onWsEvent((ev) => {
      if (ev.type !== 'state.changed') return;
      if (ev.path === '.granit/bible-bookmarks.json' && loaded) {
        void load();
      }
    })
  );

  async function remove(b: BibleBookmark) {
    if (!confirm(`Remove bookmark "${b.reference}"?`)) return;
    try {
      await api.deleteBibleBookmark(b.id);
      toast.success('bookmark removed');
      await load();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // Note-edit commits on blur. Suppress the network round-trip when
  // the value didn't actually change — avoids spurious WS broadcasts
  // and 'note saved' toasts on every tab-out.
  async function saveNote(b: BibleBookmark, note: string) {
    if (note === (b.note ?? '')) return;
    try {
      await api.patchBibleBookmark(b.id, { note });
      toast.success('note saved');
      await load();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }
</script>

{#if !loaded}
  <div class="text-sm text-dim">loading bookmarks…</div>
{:else if bookmarks.length === 0}
  <div class="bg-surface0 border border-surface1 rounded-lg p-6 text-center">
    <p class="text-sm text-dim">
      No bookmarks yet. Open a passage in the Bible tab and click <span class="text-primary">★ Bookmark</span> to save it.
    </p>
  </div>
{:else}
  <ul class="space-y-3">
    {#each bookmarks as b (b.id)}
      <li class="bg-surface0 border border-surface1 rounded-lg p-3">
        <div class="flex items-baseline gap-2 mb-2">
          <button
            type="button"
            onclick={() => onOpenBookmark(b)}
            class="text-sm text-primary font-mono hover:underline"
          >{b.reference}</button>
          <span class="flex-1"></span>
          <button
            type="button"
            onclick={() => remove(b)}
            class="text-xs text-dim hover:text-error"
            aria-label="Remove bookmark"
          >remove</button>
        </div>
        <p class="text-sm text-text font-serif italic leading-relaxed">"{b.text}"</p>
        <textarea
          value={b.note ?? ''}
          placeholder="Add a personal note…"
          onblur={(e) => saveNote(b, (e.currentTarget as HTMLTextAreaElement).value)}
          class="w-full mt-3 px-2 py-1.5 bg-mantle border border-surface1 rounded text-xs text-text placeholder-dim focus:outline-none focus:border-primary resize-y"
          rows="2"
        ></textarea>
      </li>
    {/each}
  </ul>
  <p class="text-[11px] text-dim italic mt-3">
    Synced via <code>.granit/bible-bookmarks.json</code> — same file the granit TUI reads.
  </p>
{/if}
