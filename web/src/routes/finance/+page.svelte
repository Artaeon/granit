<!--
  Thin route shell. The actual finance surface — controllers, state,
  template — lives in $lib/finance/FinancePane.svelte so the future
  workspace shell (Phase 2 of the granit vision) can drop the same
  pane into any split or workspace layout. Standalone users land on
  /finance and get the pane full-viewport; pane embedders pass the
  same component a different parent layout.
-->
<script lang="ts">
  import FinancePane from '$lib/finance/FinancePane.svelte';
</script>

<FinancePane />
