<script lang="ts">
  // Sticky toolbar strip on /jots. Holds the date-jump input, search
  // input (data-page-search="1" — the global `/` shortcut binds here),
  // three AI buttons (themes / ask / digest), and the "today" jump.
  //
  // The AI panel + quick-filter chips + search-results dropdown render
  // INSIDE this sticky container so they slide together with the
  // toolbar instead of sticking above an unrelated chrome. Each is a
  // sibling component pulled in by snippet so this file stays
  // declarative.
  import type { Note } from '$lib/api';

  type AIMode = 'none' | 'themes' | 'ask' | 'digest';

  type Props = {
    searchEl?: HTMLInputElement;
    searchText: string;
    searching: boolean;
    searchResults: Note[];
    aiMode: AIMode;
    aiBusy: boolean;
    jotsCount: number;
    onJumpToDate: (e: Event) => void;
    onSearchEnter: () => void;
    onClearSearch: () => void;
    onDetectThemes: () => void;
    onStartAsk: () => void;
    onBuildDigest: () => void;
    onOpenToday: () => void;
    // Slots are rendered through these optional snippets so the
    // panel/filter/results chrome lives with the toolbar visually
    // (sticky) while the rendering logic stays in the dedicated
    // sibling components.
    aiPanel?: import('svelte').Snippet;
    quickFilters?: import('svelte').Snippet;
  };

  let {
    searchEl = $bindable(),
    searchText = $bindable(),
    searching,
    searchResults,
    aiMode,
    aiBusy,
    jotsCount,
    onJumpToDate,
    onSearchEnter,
    onClearSearch,
    onDetectThemes,
    onStartAsk,
    onBuildDigest,
    onOpenToday,
    aiPanel,
    quickFilters
  }: Props = $props();
</script>

<div
  class="sticky top-0 z-20 -mx-3 sm:-mx-5 lg:-mx-6 px-3 sm:px-5 lg:px-6 py-1.5 mb-2 bg-base border-b border-surface1"
>
  <div class="flex flex-wrap items-center gap-1.5">
    <input
      type="date"
      onchange={onJumpToDate}
      title="jump to date"
      class="bg-mantle border border-surface1 rounded px-1.5 py-0.5 text-[11px] text-text focus:outline-none focus:border-primary"
    />
    <div class="flex-1 min-w-[10rem] flex items-center gap-0.5">
      <input
        type="text"
        bind:this={searchEl}
        bind:value={searchText}
        onkeydown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault();
            onSearchEnter();
          }
        }}
        placeholder="search jots…  /"
        data-page-search="1"
        class="flex-1 bg-mantle border border-surface1 rounded px-1.5 py-0.5 text-[11px] text-text placeholder-dim focus:outline-none focus:border-primary"
      />
      {#if searchText}
        <button
          type="button"
          onclick={onClearSearch}
          aria-label="clear search"
          class="text-[11px] text-dim hover:text-text px-1"
        >×</button>
      {/if}
    </div>
    {#if jotsCount >= 1}
      <button
        type="button"
        onclick={onDetectThemes}
        disabled={aiBusy}
        class="text-[11px] px-1.5 py-0.5 rounded {aiMode === 'themes' ? 'bg-primary text-on-primary' : 'bg-surface1 text-text hover:bg-surface2'} border border-surface2 disabled:opacity-50"
        title="surface recurring themes across the loaded jots"
      >themes</button>
      <button
        type="button"
        onclick={onStartAsk}
        disabled={aiBusy}
        class="text-[11px] px-1.5 py-0.5 rounded {aiMode === 'ask' ? 'bg-primary text-on-primary' : 'bg-surface1 text-text hover:bg-surface2'} border border-surface2 disabled:opacity-50"
        title="ask a question about your jots"
      >ask</button>
      <button
        type="button"
        onclick={onBuildDigest}
        disabled={aiBusy}
        class="text-[11px] px-1.5 py-0.5 rounded {aiMode === 'digest' ? 'bg-primary text-on-primary' : 'bg-surface1 text-text hover:bg-surface2'} border border-surface2 disabled:opacity-50"
        title="weekly digest of the last 7 days"
      >digest</button>
    {/if}
    <button
      type="button"
      onclick={onOpenToday}
      title="open today's daily note"
      class="text-[11px] px-1.5 py-0.5 rounded bg-surface0 text-subtext hover:bg-surface1"
    >today</button>
  </div>

  {#if aiPanel}{@render aiPanel()}{/if}
  {#if quickFilters}{@render quickFilters()}{/if}

  {#if searchResults.length > 0}
    <div class="mt-1.5 bg-mantle border border-surface1 rounded p-1.5 max-h-64 overflow-y-auto">
      <div class="text-[10px] uppercase tracking-wider text-dim mb-1.5 px-1">
        {searchResults.length} match{searchResults.length === 1 ? '' : 'es'}
      </div>
      <ul class="space-y-0.5">
        {#each searchResults as n (n.path)}
          <li>
            <a
              href="/notes/{encodeURIComponent(n.path)}"
              class="block px-2 py-1 rounded text-sm text-text hover:bg-surface0"
            >
              <span class="font-medium">{n.title}</span>
              <span class="text-xs text-dim ml-2">{n.path}</span>
            </a>
          </li>
        {/each}
      </ul>
    </div>
  {:else if searchText && !searching}
    <div class="mt-2 text-xs text-dim italic px-1">no matches — press Enter to search</div>
  {/if}
</div>
