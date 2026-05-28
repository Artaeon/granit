<script lang="ts">
  // RightPaneDashboard — vertical scroll of a richer widget set than
  // the Phase 1 'widgets' option. Renders six widgets in a fixed
  // order: TodayTasks → TodayFocus → Habits → Streaks → GoalsProgress
  // → DailyNote. Each widget renders at its dashboard-default size;
  // they flow vertically inside the pane's scroll container.
  //
  // The slim 'widgets' option (3 widgets) stays as a focused
  // alternative for users who want a tight strip. 'dashboard' is the
  // expanded version for "give me everything I'd see on the main
  // dashboard, in a column".

  import TodayTasksWidget from '$lib/dashboard/widgets/TodayTasksWidget.svelte';
  import TodayFocusWidget from '$lib/dashboard/widgets/TodayFocusWidget.svelte';
  import HabitsWidget from '$lib/dashboard/widgets/HabitsWidget.svelte';
  import StreaksWidget from '$lib/dashboard/widgets/StreaksWidget.svelte';
  import GoalsProgressWidget from '$lib/dashboard/widgets/GoalsProgressWidget.svelte';
  import DailyNoteWidget from '$lib/dashboard/widgets/DailyNoteWidget.svelte';
</script>

<div class="flex flex-col h-full overflow-y-auto p-2 space-y-2">
  <TodayTasksWidget />
  <TodayFocusWidget />
  <HabitsWidget />
  <StreaksWidget />
  <GoalsProgressWidget />
  <DailyNoteWidget />
</div>
