<!--
  DeadlinesPane — life-essential time pressure surfaced as a
  workspace pane. Thin wrapper over the /deadlines route. Future
  work: extract a proper DeadlinesPane (data + view controllers +
  template) if the wrapper's chrome proves heavy in narrow splits.
-->
<script lang="ts">
  import DeadlinesRoute from '../../routes/deadlines/+page.svelte';
</script>

<div class="h-full overflow-auto">
  <DeadlinesRoute />
</div>
