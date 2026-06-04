<!--
  Thin route shell. The actual goals surface — controllers, state,
  template — lives in $lib/goals/GoalsPane.svelte so the future
  workspace shell (Phase 2 of the granit vision) can drop the same
  pane into any split or workspace layout. Standalone users land on
  /goals and get the pane full-viewport; pane embedders pass the
  same component a different parent layout.
-->
<script lang="ts">
  import GoalsPane from '$lib/goals/GoalsPane.svelte';
</script>

<GoalsPane />
