<script lang="ts">
  // RightPaneWidgets — vertical strip of dashboard widgets in the
  // companion column. Phase 1 renders three widgets at their default
  // size and lets them flow inside the pane's overflow-y-auto. The
  // widgets are full-width within the pane and stack vertically with
  // a small gap so the user gets the same surfaces they see on the
  // dashboard without leaving their current main-pane route.
  //
  // None of the widgets accept a `size` prop — they're all built for
  // the dashboard's full column width. They cope reasonably with a
  // 360-ish-px container because their internal layout is already
  // flex-based; the truncate / overflow handling on rows keeps them
  // legible at this width.

  import TodayTasksWidget from '$lib/dashboard/widgets/TodayTasksWidget.svelte';
  import HabitsWidget from '$lib/dashboard/widgets/HabitsWidget.svelte';
  import TodayFocusWidget from '$lib/dashboard/widgets/TodayFocusWidget.svelte';
</script>

<div class="flex flex-col h-full overflow-y-auto p-2 space-y-2">
  <TodayTasksWidget />
  <HabitsWidget />
  <TodayFocusWidget />
</div>
