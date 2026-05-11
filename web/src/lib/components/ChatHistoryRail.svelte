<script lang="ts">
  import {
    listThreads,
    listPinned,
    searchThreads,
    togglePin,
    type ChatThread,
    type PinnedMessage,
    type ThreadSearchHit
  } from '$lib/chat/history';
  import { findMode } from '$lib/ai/agents';

  // ChatHistoryRail — saved-thread browser + pinned-replies tab. Slides
  // over the chat body on mobile (one-tap thread pick, no chat pushed
  // off-screen); inline strip below the toolbar on desktop (chat below
  // stays visible). Two tabs:
  //   - threads: chronological list of saved chats. Click to load
  //     (parent auto-saves the current thread before swapping).
  //   - pinned: flat list of starred assistant replies across all
  //     threads. Persists even if the parent thread was pruned by LRU.
  //
  // Extracted from AIOverlay.svelte. The rail owns:
  //   - tab + search state (UI ephemera)
  //   - the lists themselves (savedThreads, pinnedItems, historyHits)
  //   - the unpin-from-pinned-tab interaction (calls togglePin and
  //     refreshes locally; reports back to the parent only when the
  //     toggled item belonged to the active thread, so the parent can
  //     refresh its in-thread pinnedIndex highlights)
  // The parent owns:
  //   - the open flag (Esc layering, header toggle button)
  //   - thread loading + deletion (those mutate the parent's messages,
  //     activeThreadId, modeId, etc.; the rail just emits ids)

  interface Props {
    /** Two-way bound to the parent's open flag. */
    open: boolean;
    /** Currently-loaded thread id, used to highlight the active row. */
    activeThreadId: string;
    /** Fired when the user picks a saved thread to load. Parent's
     *  loadSavedThread() handles auto-save + swap. */
    onLoadThread: (id: string) => void;
    /** Fired when the user confirms deletion of a saved thread. */
    onDeleteThread: (id: string) => void;
    /** Fired when the user unpins an item belonging to the currently
     *  active thread, so the parent can refresh its per-message pin
     *  highlights. (Unpinning items from other threads is a no-op for
     *  the parent.) */
    onUnpinForActive: () => void;
  }
  let {
    open = $bindable(),
    activeThreadId,
    onLoadThread,
    onDeleteThread,
    onUnpinForActive
  }: Props = $props();

  let tab = $state<'threads' | 'pinned'>('threads');
  let savedThreads = $state<ChatThread[]>([]);
  let pinnedItems = $state<PinnedMessage[]>([]);
  let search = $state('');
  let hits = $state<ThreadSearchHit[]>([]);

  /** Re-pull the saved-threads + pinned-items lists from localStorage.
   *  Exposed so the parent can prod the rail to refresh after a branch,
   *  pin add, or delete that originated outside this component. */
  export function refresh() {
    savedThreads = listThreads();
    pinnedItems = listPinned();
  }

  // Refresh on open. The parent already calls refresh() on external
  // mutations; this effect handles the "user opened the panel" path so
  // the lists are fresh without requiring the parent to coordinate.
  $effect(() => {
    if (open) refresh();
  });

  // Search index — runs against listThreads() under the hood, so it
  // benefits from refresh() automatically. Empty string clears hits.
  $effect(() => {
    void search;
    if (!search.trim()) {
      hits = [];
      return;
    }
    hits = searchThreads(search);
  });
</script>

{#if open}
  <!-- History panel layout split between mobile + desktop: on phones,
       history is a full-screen slide-over that COVERS the chat (one
       tap to a thread, no half-screen-of-chat-pushed-down nonsense);
       on desktop, it stays inline as a top strip beneath the toolbar
       so the chat below is still visible. The panel is positioned
       `absolute inset-0` on mobile within the dialog — z-30 sits above
       the chat body but below the header (z-50 on the resize handle)
       so the user can still see what thread they came from. -->
  <div
    class="ai-history-panel border-surface1 bg-mantle flex flex-col
           absolute inset-0 z-30 border-t
           md:static md:bg-mantle md:border-b md:border-t-0 md:max-h-[40dvh]"
  >
    <div class="flex items-center gap-1 px-3 pt-3 pb-1 text-[11px] flex-shrink-0">
      <button
        type="button"
        onclick={() => (tab = 'threads')}
        class="tap-target px-2.5 py-1.5 rounded transition-colors {tab === 'threads' ? 'bg-surface1 text-text font-medium' : 'text-dim hover:text-text hover:bg-surface0'}"
      >Threads <span class="opacity-60">({savedThreads.length})</span></button>
      <button
        type="button"
        onclick={() => (tab = 'pinned')}
        class="tap-target px-2.5 py-1.5 rounded transition-colors {tab === 'pinned' ? 'bg-surface1 text-text font-medium' : 'text-dim hover:text-text hover:bg-surface0'}"
      >Pinned <span class="opacity-60">({pinnedItems.length})</span></button>
      <span class="flex-1"></span>
      <button
        type="button"
        onclick={() => (open = false)}
        class="tap-target inline-flex items-center justify-center text-dim hover:text-text hover:bg-surface0 active:bg-surface1 rounded px-2 py-1 text-base leading-none transition-colors"
        aria-label="Close history"
      >×</button>
    </div>
    <div class="flex-1 min-h-0 overflow-y-auto">
    {#if tab === 'threads'}
      <div class="px-3 pt-2 pb-1">
        <input
          type="text"
          bind:value={search}
          placeholder="Search threads…"
          class="w-full bg-surface0 border border-surface1 rounded px-2 py-1 text-xs text-text placeholder-dim focus:outline-none focus:border-primary"
        />
      </div>
      <ul class="px-2 pb-2">
        {#if search.trim()}
          {#if hits.length === 0}
            <li class="px-2 py-3 text-center text-[11px] text-dim italic">No matches.</li>
          {:else}
            {#each hits as hit (hit.thread.id)}
              <li>
                <button
                  type="button"
                  onclick={() => onLoadThread(hit.thread.id)}
                  class="w-full text-left px-2 py-1.5 rounded hover:bg-surface0 group {hit.thread.id === activeThreadId ? 'bg-surface0' : ''}"
                >
                  <div class="flex items-baseline gap-2">
                    <span class="text-xs text-text font-medium truncate flex-1">{hit.thread.title}</span>
                    <span class="text-[9px] text-dim flex-shrink-0">{findMode(hit.thread.modeId).glyph}</span>
                  </div>
                  <div class="text-[10px] text-dim leading-snug truncate mt-0.5">{hit.excerpt}</div>
                </button>
              </li>
            {/each}
          {/if}
        {:else if savedThreads.length === 0}
          <li class="px-2 py-3 text-center text-[11px] text-dim italic">No saved threads yet. Send a message to start one.</li>
        {:else}
          {#each savedThreads as t (t.id)}
            <li class="group flex items-stretch gap-1">
              <button
                type="button"
                onclick={() => onLoadThread(t.id)}
                class="flex-1 min-w-0 text-left px-2 py-1.5 rounded hover:bg-surface0 {t.id === activeThreadId ? 'bg-surface0' : ''}"
              >
                <div class="flex items-baseline gap-2">
                  <span class="text-xs text-text font-medium truncate flex-1">{t.title}</span>
                  <span class="text-[9px] text-dim flex-shrink-0" title={findMode(t.modeId).label}>{findMode(t.modeId).glyph}</span>
                </div>
                <div class="text-[10px] text-dim mt-0.5 flex items-center gap-2">
                  <span>{new Date(t.updatedAt).toLocaleDateString()} {new Date(t.updatedAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
                  <span>· {t.messages.filter((m) => m.role !== 'system').length} msgs</span>
                </div>
              </button>
              <button
                type="button"
                onclick={() => { if (confirm('Delete this thread?')) onDeleteThread(t.id); }}
                class="px-1 text-dim hover:text-error opacity-0 group-hover:opacity-100 transition-opacity"
                aria-label="Delete thread"
                title="Delete thread"
              >
                <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13" stroke-linecap="round" stroke-linejoin="round"/>
                </svg>
              </button>
            </li>
          {/each}
        {/if}
      </ul>
    {:else}
      <ul class="px-2 pt-2 pb-2">
        {#if pinnedItems.length === 0}
          <li class="px-2 py-3 text-center text-[11px] text-dim italic">No pinned replies yet. Click ☆ on any assistant message to keep it.</li>
        {:else}
          {#each pinnedItems as p (p.threadId + ':' + p.messageIndex + ':' + p.pinnedAt)}
            <li class="group px-2 py-1.5 rounded hover:bg-surface0">
              <div class="flex items-baseline gap-2 mb-1">
                <span class="text-[10px] text-dim flex-1 truncate">{p.threadTitle}</span>
                <span class="text-[9px] text-dim flex-shrink-0" title={findMode(p.modeId).label}>{findMode(p.modeId).glyph}</span>
                <button
                  type="button"
                  onclick={() => {
                    togglePin({ threadId: p.threadId, threadTitle: p.threadTitle, modeId: p.modeId, messageIndex: p.messageIndex, content: p.content });
                    refresh();
                    if (p.threadId === activeThreadId) onUnpinForActive();
                  }}
                  class="text-warning hover:text-error opacity-60 group-hover:opacity-100 transition-opacity"
                  title="Unpin"
                  aria-label="Unpin"
                >
                  <svg viewBox="0 0 24 24" class="w-3 h-3" fill="currentColor"><polygon points="12 2 15 9 22 9 17 14 19 22 12 17 5 22 7 14 2 9 9 9"/></svg>
                </button>
              </div>
              <div class="text-[11px] text-subtext leading-snug line-clamp-3">{p.content}</div>
            </li>
          {/each}
        {/if}
      </ul>
    {/if}
    </div>
  </div>
{/if}
