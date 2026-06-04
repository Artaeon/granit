<!--
  Thin route shell. The actual notes-list surface — controllers,
  state, template — lives in $lib/notes/NotesListPane.svelte so the
  future workspace shell (Phase 2 of the granit vision) can drop the
  same pane into any split or workspace layout. Standalone users land
  on /notes and get the pane full-viewport; pane embedders pass the
  same component a different parent layout.

  Anything route-specific (URL hydration via /notes?capture=1, the
  Web Share Target intake, SvelteKit `$auth`) stays inside
  NotesListPane for now — until the workspace shell exists we don't
  need a different boundary, and re-hosting the pane in a workspace
  is a separate refactor on its own.
-->
<script lang="ts">
  import NotesListPane from '$lib/notes/NotesListPane.svelte';
</script>

<NotesListPane />
