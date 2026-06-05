<!--
  HabitsPane — daily-life habit tracking surfaced as a workspace pane.
  Thin wrapper over the /habits route's component so the data-load +
  view-mode logic lives in one place. Future work: extract a proper
  HabitsPane out of the route (data controller + view controller +
  template), following the pattern used for Tasks/Calendar/Goals.
  Until that lands, the wrapper keeps Habits reachable in workspaces
  without duplicating the 1.1k-LOC route body.
-->
<script lang="ts">
  import HabitsRoute from '../../routes/habits/+page.svelte';
</script>

<div class="h-full overflow-auto">
  <HabitsRoute />
</div>
