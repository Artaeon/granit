<!--
  TodayPane — the daily-glance surface as a workspace pane. Thin
  wrapper over RightPaneToday so the data-load logic stays in one
  place. The slim chrome the right-pane variant uses looks fine in
  a wider workspace pane too — just more whitespace around the daily
  note preview + top tasks.
-->
<script lang="ts">
  import RightPaneToday from '$lib/nav/rightpane/RightPaneToday.svelte';
</script>

<div class="h-full overflow-auto">
  <RightPaneToday />
</div>
