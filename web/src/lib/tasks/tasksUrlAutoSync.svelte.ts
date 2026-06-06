// $effect-driven auto-sync companion to tasksUrlSync.
//
// tasksUrlSync.ts stays pure data-sync (read URL -> controllers,
// write controllers -> URL). This installer wraps sync() in the
// $effect that triggers it on every filter / view change, with the
// untracked call so the sync's own goto/$page reads don't become deps
// of the effect itself (otherwise every URL write would re-fire the
// effect that wrote it).

import { untrack } from 'svelte';
import type { TasksFilterStateController } from './tasksFilterState.svelte';
import type { TasksViewStateController } from './tasksViewState.svelte';
import type { TasksUrlSyncController } from './tasksUrlSync';

export function installTasksUrlAutoSync(args: {
  filterCtl: TasksFilterStateController;
  viewCtl: TasksViewStateController;
  urlSync: TasksUrlSyncController;
}): void {
  const { filterCtl, viewCtl, urlSync } = args;
  $effect(() => {
    // Explicit `void` list — source-of-truth for what should retrigger
    // the URL write. Adding a new URL-shareable filter dimension means
    // adding both a sync() write (in tasksUrlSync) and a void here.
    void filterCtl.status;
    void filterCtl.q;
    void filterCtl.tagFilters;
    void filterCtl.projectFilter;
    void filterCtl.priorityFilter;
    void filterCtl.goalFilter;
    void filterCtl.deadlineFilter;
    void viewCtl.view;
    void viewCtl.groupBy;
    void filterCtl.smartFilter;
    untrack(() => urlSync.sync());
  });
}
