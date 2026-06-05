// TasksPane URL ↔ state sync.
//
// Filters, view, and group are URL-shareable: opening a P1-filtered
// kanban in a new tab lands you on exactly what the sender saw. The
// sync runs both ways — read URL → controllers on hydrate, write
// controllers → URL on every change after hydrate.
//
// History footprint: replaceState (via SvelteKit's goto with
// replaceState: true) so a keystroke in the search box doesn't pile
// up history entries.
//
// The factory returns { hydrate, sync }. Parent calls hydrate() in
// onMount before subscribing dependent effects; sync() runs from a
// $effect that touches every filter / view slot.

import { goto } from '$app/navigation';
import type { Page } from '@sveltejs/kit';
import type { TasksFilterStateController } from './tasksFilterState.svelte';
import type { TasksViewStateController } from './tasksViewState.svelte';
import type { View, Group, SmartFilter } from './tasksHelpers';

const VIEWS: View[] = [
  'list', 'kanban', 'today', 'week', 'triage',
  'inbox', 'stale', 'duplicates', 'quickwins', 'review', 'eisenhower'
];
const GROUPS: Group[] = ['due', 'priority', 'note', 'project', 'tag', 'goal', 'deadline'];
const SMART_FILTERS: SmartFilter[] = [
  'overdue', 'today', 'tomorrow', 'thisWeek', 'noDue',
  'noPriority', 'highPriority', 'hasSubtasks', 'hasEstimate', 'noEstimate'
];

export type TasksUrlSyncRefs = {
  filterCtl: TasksFilterStateController;
  viewCtl: TasksViewStateController;
  /** Read the current SvelteKit page so URL writes target the
   *  correct pathname. */
  getPage: () => Page;
  /** Called once if ?agent=1 was present on hydrate — opens the
   *  Task Agent dialog. The URL param is cleared as part of
   *  hydration so a refresh doesn't re-pop the dialog. */
  onAgentParam: () => void;
};

export type TasksUrlSyncController = {
  /** Read URL params → controllers. Idempotent; calling twice
   *  re-applies whatever's currently in the URL. Sets the internal
   *  hydrated flag so subsequent sync() calls actually write. */
  hydrate(): void;
  /** Write controllers → URL. No-op until hydrate() has run, so a
   *  $effect can call it eagerly without overwriting the URL with
   *  defaults before hydration. */
  sync(): void;
};

export function createTasksUrlSync(refs: TasksUrlSyncRefs): TasksUrlSyncController {
  let hydrated = false;

  function hydrate() {
    if (typeof window === 'undefined') return;
    const { filterCtl, viewCtl } = refs;
    const sp = new URL(window.location.href).searchParams;
    const get = (k: string) => sp.get(k) ?? '';

    if (sp.has('status')) {
      const s = get('status');
      if (s === 'open' || s === 'done' || s === 'all') filterCtl.status = s;
    }
    if (sp.has('q')) filterCtl.q = get('q');
    if (sp.has('tag')) {
      // Comma-separated. Filter empties so a stale URL with
      // accidental ",, " doesn't ghost in an empty-string tag.
      filterCtl.tagFilters = get('tag').split(',').map((s) => s.trim()).filter(Boolean);
    }
    if (sp.has('project')) filterCtl.projectFilter = get('project');
    if (sp.has('priority')) {
      const n = Number(get('priority'));
      filterCtl.priorityFilter = n >= 1 && n <= 3 ? n : '';
    }
    if (sp.has('goal')) filterCtl.goalFilter = get('goal');
    if (sp.has('deadline')) filterCtl.deadlineFilter = get('deadline');
    if (sp.has('view')) {
      const v = get('view') as View;
      if (VIEWS.includes(v)) viewCtl.view = v;
    }
    if (sp.has('group')) {
      const g = get('group') as Group;
      if (GROUPS.includes(g)) viewCtl.groupBy = g;
    }
    if (sp.has('smart')) {
      const v = get('smart') as SmartFilter;
      if (SMART_FILTERS.includes(v)) filterCtl.smartFilter = v;
    }
    // ?agent=1 launches the Task Agent. Consumed once: clear the
    // param on hydrate so a refresh doesn't re-pop the dialog.
    if (sp.get('agent') === '1') {
      refs.onAgentParam();
      const next = new URLSearchParams(sp);
      next.delete('agent');
      const qs = next.toString();
      const pathname = refs.getPage().url.pathname;
      void goto(qs ? `${pathname}?${qs}` : pathname, {
        replaceState: true,
        noScroll: true,
        keepFocus: true
      });
    }
    hydrated = true;
  }

  function sync() {
    if (!hydrated) return;
    if (typeof window === 'undefined') return;
    const { filterCtl, viewCtl } = refs;
    const sp = new URLSearchParams();
    if (filterCtl.status !== 'open') sp.set('status', filterCtl.status);
    if (filterCtl.q) sp.set('q', filterCtl.q);
    if (filterCtl.tagFilters.length > 0) sp.set('tag', filterCtl.tagFilters.join(','));
    if (filterCtl.projectFilter) sp.set('project', filterCtl.projectFilter);
    if (filterCtl.priorityFilter !== '') sp.set('priority', String(filterCtl.priorityFilter));
    if (filterCtl.goalFilter) sp.set('goal', filterCtl.goalFilter);
    if (filterCtl.deadlineFilter) sp.set('deadline', filterCtl.deadlineFilter);
    if (viewCtl.view !== 'list') sp.set('view', viewCtl.view);
    if (viewCtl.groupBy !== 'due') sp.set('group', viewCtl.groupBy);
    if (filterCtl.smartFilter) sp.set('smart', filterCtl.smartFilter);
    const qs = sp.toString();
    const pathname = refs.getPage().url.pathname;
    const next = qs ? `${pathname}?${qs}` : pathname;
    // replaceState — we don't want every keystroke in the search
    // box adding to browser history.
    void goto(next, { replaceState: true, noScroll: true, keepFocus: true });
  }

  return { hydrate, sync };
}
