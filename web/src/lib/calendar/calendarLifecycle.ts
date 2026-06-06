// Calendar page lifecycle plumbing — the bits that have to run once
// on mount and tear down on unmount.
//
//   • Initial loads: events + sources + habits + projects + native
//     event entries. Five parallel API calls; each safely tolerates
//     others failing.
//   • WS subscription: refetches the right slices on relevant events.
//     The dispatch table is the only piece of "what touches what"
//     knowledge the page used to carry inline — bundling it here
//     keeps it one place instead of one onMount per slice.
//
// Returns the unsub the parent's onMount expects so the install line
// stays a one-liner:
//
//   onMount(() => installCalendarLifecycle({ dataCtl }));

import { onWsEvent } from '$lib/ws';
import type { CalendarDataController } from './calendarData.svelte';

export interface CalendarLifecycleOptions {
  dataCtl: CalendarDataController;
}

export function installCalendarLifecycle(opts: CalendarLifecycleOptions): () => void {
  const { dataCtl } = opts;

  // Fire the five sidecar loads in parallel. Each handles its own
  // error path and doesn't block siblings.
  void dataCtl.load();
  void dataCtl.loadSources();
  void dataCtl.loadHabits();
  void dataCtl.loadAllProjects();
  void dataCtl.loadNativeEvents();

  return onWsEvent((ev) => {
    if (
      ev.type === 'note.changed' ||
      ev.type === 'note.removed' ||
      ev.type === 'event.changed' ||
      ev.type === 'event.removed' ||
      // task.changed fires from handlers_tasks.go on create / patch /
      // schedule / delete. Without it, dropping a task on the grid or
      // creating one via UnifiedCreate wouldn't repaint until the user
      // reloaded — the file-watcher's note.changed often races the
      // same-process write debounce and skips it.
      ev.type === 'task.changed'
    ) {
      dataCtl.load();
      // Habits live inside daily notes — a note change might mean a
      // habit was ticked. Refetch alongside the event feed.
      dataCtl.loadHabits();
    }
    // Refresh native event entries on any event change so the
    // Calendar Agent's scope reflects current state.
    if (ev.type === 'event.changed' || ev.type === 'event.removed') {
      dataCtl.loadNativeEvents();
    }
    // Deadlines are an overlay on the feed — refetch when the server
    // signals .granit/deadlines.json changed (TUI edit, web edit in
    // another tab, or anything else that calls SaveAll).
    if (ev.type === 'state.changed' && ev.path === '.granit/deadlines.json') {
      dataCtl.load();
    }
    // ICS mutations (create/edit/delete in a subscribed .ics file)
    // broadcast as state.changed with a calendar path — e.g.
    // "calendars/personal.ics" or "merged.ics". Match the path-shape
    // rather than enumerate sources so new calendars added mid-
    // session refresh automatically.
    if (ev.type === 'state.changed' && ev.path && /\.ics$/.test(ev.path)) {
      dataCtl.load();
    }
    // Project metadata changed (rename, colour, status) — refresh the
    // picker so the filter dropdown stays in sync. We don't touch the
    // event feed here; project_id on events is captured at write
    // time, so a project rename doesn't transitively re-key past
    // events (matches the deliberate Task.Project shape).
    if (ev.type === 'project.changed' || ev.type === 'project.removed') {
      dataCtl.loadAllProjects();
    }
  });
}
