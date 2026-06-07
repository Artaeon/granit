<script lang="ts">
  import Button from '$lib/components/Button.svelte';
  import Chip from '$lib/components/Chip.svelte';
  // Optional quick-filter chip row sitting under the slim NotesPageHeader.
  // Renders conditionally per view:
  //   - 'all' with an active folder or tag filter → clear pill(s) +
  //                sort segmented control
  //   - 'search' with a query → "Save as collection…" button
  // For other views we render nothing (the parent only inserts this
  // component when there's something to show).
  type SortKey = 'modified' | 'created' | 'name' | 'size';

  type Props = {
    view: 'stream' | 'recent' | 'tree' | 'pinned' | 'all' | 'alpha' | 'tags' | 'collections' | 'folders' | 'search';
    folderFilter: string;
    tagFilter: string;
    sortKey: SortKey;
    searchActive: boolean;
    onClearFolder: () => void;
    onClearTag: () => void;
    onPickSort: (s: SortKey) => void;
    onSaveCollection: () => void;
  };

  let {
    view,
    folderFilter,
    tagFilter,
    sortKey,
    searchActive,
    onClearFolder,
    onClearTag,
    onPickSort,
    onSaveCollection
  }: Props = $props();

  const SORTS: { id: SortKey; label: string }[] = [
    { id: 'modified', label: 'modified' },
    { id: 'created',  label: 'created' },
    { id: 'name',     label: 'name' },
    { id: 'size',     label: 'size' }
  ];

  // Only render the row when there's something to show. We compute the
  // visibility flag once so the consuming page can ALWAYS render
  // <NotesQuickFilters/> unconditionally and the component decides
  // itself whether to occupy vertical space. 'all' always renders the
  // sort segmented; 'search' only when a query is active (to expose
  // Save-as-collection).
  let visible = $derived(view === 'all' || (view === 'search' && searchActive));
</script>

{#if visible}
  <div class="flex items-center gap-2 px-3 py-1.5 border-b border-surface1 flex-shrink-0 bg-mantle/60 overflow-x-auto">
    {#if view === 'all'}
      {#if folderFilter}
        <Chip tone="warning" onclick={onClearFolder} title="Clear folder filter" class="flex-shrink-0">
          <svg viewBox="0 0 24 24" class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
            <path d="M3 7h6l2 2h10v10H3z"/>
          </svg>
          <span class="font-medium">{folderFilter === '__root__' ? '/' : folderFilter}</span>
          <span aria-hidden="true">×</span>
        </Chip>
      {/if}
      {#if tagFilter}
        <Chip tone="neutral" onclick={onClearTag} title="Clear tag filter" class="flex-shrink-0">
          <span class="font-medium">#{tagFilter}</span>
          <span aria-hidden="true">×</span>
        </Chip>
      {/if}
      <!-- Sort segmented control — pinned to the right on sm+ so it
           doesn't crowd the active-filter pills when they're long. -->
      <span class="ml-auto inline-flex items-center gap-0.5 text-[11px] text-dim flex-shrink-0">
        <span class="hidden sm:inline mr-1">sort</span>
        <span class="inline-flex bg-surface0 border border-surface1 rounded overflow-hidden">
          {#each SORTS as s (s.id)}
            <Button variant="ghost" size="sm" active={sortKey === s.id} onclick={() => onPickSort(s.id)}>{s.label}</Button>
          {/each}
        </span>
      </span>
    {:else if view === 'search' && searchActive}
      <Button variant="secondary" size="sm" onclick={onSaveCollection} title="Save current search as a collection" class="ml-auto flex-shrink-0">
        <svg viewBox="0 0 24 24" class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <path d="M19 21l-7-5-7 5V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2z"/>
        </svg>
        Save as collection
      </Button>
    {/if}
  </div>
{/if}
