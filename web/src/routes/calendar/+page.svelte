<!--
  Thin route shell. The actual calendar surface — controllers, state,
  template — lives in $lib/calendar/CalendarPane.svelte so the future
  workspace shell (Phase 2 of the granit vision) can drop the same
  pane into any split or workspace layout. Standalone users land on
  /calendar and get the pane full-viewport; pane embedders pass the
  same component a different parent layout.

  Anything route-specific (URL hydration of ?plan=1 and ?project=,
  SvelteKit `$page`/`$auth`) stays inside CalendarPane for now — until
  the workspace shell exists we don't need a different boundary, and
  re-hosting the pane in a workspace is a separate refactor on its own.
-->
<script lang="ts">
  import CalendarPane from '$lib/calendar/CalendarPane.svelte';
</script>

<CalendarPane />
