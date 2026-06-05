<!--
  ProjectsPane — projects list surfaced as a workspace pane. Thin
  wrapper over the /projects route. Future work: extract a proper
  ProjectsPane following the established controller pattern if the
  wrapper's chrome proves heavy in narrow splits.
-->
<script lang="ts">
  import ProjectsRoute from '../../routes/projects/+page.svelte';
</script>

<div class="h-full overflow-auto">
  <ProjectsRoute />
</div>
