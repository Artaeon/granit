<script lang="ts">
  import NotesTree from '$lib/notes/NotesTree.svelte';
</script>

<div class="h-full flex flex-col">
  <header class="px-3 sm:px-4 py-3 border-b border-surface1 flex-shrink-0">
    <h1 class="text-xl sm:text-2xl font-semibold text-text">Notes</h1>
    <p class="text-xs text-dim mt-1">click a note to open. <kbd class="text-[10px] px-1 bg-surface0 border border-surface1 rounded">/</kbd> filter</p>
  </header>
  <div class="flex-1 min-h-0">
    <NotesTree />
  </div>
</div>
