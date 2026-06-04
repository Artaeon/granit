<!--
  Thin route shell. The actual tasks surface — controllers, state,
  template — lives in $lib/tasks/TasksPane.svelte so the future
  workspace shell (Phase 2 of the granit vision) can drop the same
  pane into any split or workspace layout. Standalone users land on
  /tasks and get the pane full-viewport; pane embedders pass the same
  component a different parent layout.

  Anything route-specific (URL hydration, SvelteKit `$page`/`$auth`)
  stays inside TasksPane for now — until the workspace shell exists
  we don't need a different boundary, and re-hosting the pane in a
  workspace is a separate refactor on its own.
-->
<script lang="ts">
  import TasksPane from '$lib/tasks/TasksPane.svelte';
</script>

<TasksPane />
