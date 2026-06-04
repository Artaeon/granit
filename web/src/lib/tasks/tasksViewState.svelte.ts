// View / display state for the tasks surface.
//
// Fourth extraction step out of routes/tasks/+page.svelte. Owns the
// visual shape of the page: which view (list/kanban/today/week/…),
// which group-by, which sort, kanban-mode/swimlane, compact-vs-normal
// density, the per-section collapse map, and three transient UI
// toggles (help, filter panel, more-views dropdown).
//
// Also owns the four derivations that turn the filter-pipeline output
// into screen shapes: listGroups (grouped buckets for the list view),
// weekColumns (the week-view 7-column grid), viewCounts (badge counts
// on the view tabs), and taskComparator (sort function used inside
// each bucket). They live here because each is a pure function of
// view config plus the filtered list — same input shape, same place.
//
// The future workspace pane reads from this controller exactly as
// the standalone route does today: getter-property reads, set-pair
// writes, deps bundle for cross-controller state.

import { todayISO, fmtDateISO, type Task, type Project, type Goal, type Deadline } from '$lib/api';
import {
  loadStored,
  loadStoredString,
  saveStored,
  saveStoredString
} from '$lib/util/storage';
import { type View, type Group, type SortBy, OVERFLOW_VIEWS, isStale } from './tasksHelpers';

const VIEW_KEY = 'granit.tasks.view';
const GROUP_KEY = 'granit.tasks.groupBy';
const SORT_KEY = 'granit.tasks.sortBy';
const DENSITY_KEY = 'granit.tasks.density';
const SECTION_COLLAPSE_KEY = 'granit.tasks.collapsedSections';

export type KanbanMode = 'priority' | 'due' | 'triage' | 'config';
export type KanbanSwimlane = 'none' | 'project' | 'tag' | 'priority';

export type DayColumn = {
  date: string;
  label: string;
  sublabel: string;
  isToday: boolean;
  tasks: Task[];
};

export type WeekColumns = {
  unscheduled: Task[];
  overdue: Task[];
  days: DayColumn[];
};

export type ListGroup = {
  key: string;
  label: string;
  tasks: Task[];
  deepLink?: string;
};

export type ViewCounts = {
  inbox: number;
  stale: number;
  quickwins: number;
  review: number;
};

export interface TasksViewStateDeps {
  /** Filter pipeline output. listGroups and weekColumns iterate it
   *  to build their grouped/columnar shape. */
  getFiltered: () => Task[];
  /** Raw loaded tasks. viewCounts walks the whole list (not the
   *  filter pipeline) because the badges read "how many inbox /
   *  stale / quick-wins exist in your vault right now", independent
   *  of which view you're currently looking at. */
  getTasks: () => Task[];
  /** Projects sidecar — listGroups uses it to find folder-based
   *  fallback project membership and to build deep-link URLs for
   *  group rows. */
  getProjects: () => Project[];
  /** Goals sidecar — label lookup for the goal-grouped list view. */
  getGoals: () => Goal[];
  /** Deadlines sidecar — label + sort-by-date for the deadline-
   *  grouped list view. */
  getDeadlines: () => Deadline[];
}

export interface TasksViewStateController {
  // Bindable view config.
  view: View;
  groupBy: Group;
  sortBy: SortBy;
  kanbanMode: KanbanMode;
  kanbanSwimlane: KanbanSwimlane;
  density: 'normal' | 'compact';

  // Bindable UI toggles.
  helpOpen: boolean;
  filterPanelOpen: boolean;
  moreViewsOpen: boolean;

  // Bindable section-collapse map. Read by the SectionList
  // component; written via toggleSection here.
  collapsedSections: Record<string, boolean>;

  // Derived (readonly).
  readonly compactCards: boolean;
  readonly activeOverflowLabel: string;
  readonly taskComparator: (a: Task, b: Task) => number;
  readonly listGroups: ListGroup[];
  readonly weekColumns: WeekColumns;
  readonly viewCounts: ViewCounts;

  // Methods.
  /** Pick a view from the More-views dropdown — also closes it. */
  pickOverflowView(v: View): void;
  /** Escape on the More-views menu dismisses it. */
  onMoreViewsKey(e: KeyboardEvent): void;
  /** Set the view from any caller (URL hydration, preset apply,
   *  tab click). Closes the More-views dropdown. */
  selectView(v: View): void;
  /** Walk the VIEW_CYCLE order — `[` / `]` keyboard chords. */
  cycleView(direction: 1 | -1): void;
  /** Flip per-section collapse state; reads the same per-section
   *  default the SectionList uses so the first toggle flips against
   *  the effective state. */
  toggleSection(key: string): void;
}

import { VIEW_CYCLE } from './tasksHelpers';

export function createTasksViewState(deps: TasksViewStateDeps): TasksViewStateController {
  let view = $state<View>(loadStoredString(VIEW_KEY, 'list') as View);
  let groupBy = $state<Group>(loadStoredString(GROUP_KEY, 'due') as Group);
  let sortBy = $state<SortBy>(loadStoredString(SORT_KEY, 'auto') as SortBy);
  let kanbanMode = $state<KanbanMode>('priority');
  let kanbanSwimlane = $state<KanbanSwimlane>('none');
  let density = $state<'normal' | 'compact'>(
    loadStoredString(DENSITY_KEY, 'normal') === 'compact' ? 'compact' : 'normal'
  );
  let helpOpen = $state(false);
  let filterPanelOpen = $state(false);
  let moreViewsOpen = $state(false);
  let collapsedSections = $state<Record<string, boolean>>(
    loadStored<Record<string, boolean>>(SECTION_COLLAPSE_KEY, {})
  );

  // Persistence effects — same per-key shape the page had before;
  // co-located with the state they persist.
  $effect(() => saveStoredString(VIEW_KEY, view));
  $effect(() => saveStoredString(GROUP_KEY, groupBy));
  $effect(() => saveStoredString(SORT_KEY, sortBy));
  $effect(() => saveStoredString(DENSITY_KEY, density));
  $effect(() => saveStored(SECTION_COLLAPSE_KEY, collapsedSections));

  // Click-outside dismiss for the overflow menu. Window-level
  // listener installed only while the menu is open so the rest of
  // the page doesn't pay for it. The menu + button live inside an
  // element marked with data-more-views; any click outside that
  // subtree closes the menu.
  $effect(() => {
    if (!moreViewsOpen) return;
    function onDocClick(e: MouseEvent) {
      const target = e.target as HTMLElement | null;
      if (target && target.closest('[data-more-views]')) return;
      moreViewsOpen = false;
    }
    window.addEventListener('mousedown', onDocClick);
    return () => window.removeEventListener('mousedown', onDocClick);
  });

  let compactCards = $derived(density === 'compact');

  let activeOverflowLabel = $derived(
    OVERFLOW_VIEWS.find((v) => v.key === view)?.label ?? ''
  );

  // Per-bucket task comparator. Routed through a derived so every
  // group-by branch can sort buckets through one place — pick a
  // different sortBy and the entire list reshapes consistently.
  // 'auto' preserves the historical due-then-priority shape so
  // existing users aren't surprised on first load.
  let taskComparator = $derived.by<(a: Task, b: Task) => number>(() => {
    const dueOf = (t: Task) => t.dueDate ?? (t.scheduledStart?.slice(0, 10) ?? '');
    const prioOf = (t: Task) => t.priority || 99;
    const ageOf = (t: Task) => t.createdAt ?? '';
    const estOf = (t: Task) => t.estimatedMinutes ?? 0;
    const textOf = (t: Task) => t.text.toLowerCase();
    switch (sortBy) {
      case 'priority':
        return (a, x) => {
          const d = prioOf(a) - prioOf(x);
          if (d !== 0) return d;
          const ad = dueOf(a),
            xd = dueOf(x);
          return ad === xd ? 0 : ad < xd ? -1 : 1;
        };
      case 'due':
        return (a, x) => {
          const ad = dueOf(a),
            xd = dueOf(x);
          if (!ad && xd) return 1;
          if (ad && !xd) return -1;
          if (ad !== xd) return ad < xd ? -1 : 1;
          return prioOf(a) - prioOf(x);
        };
      case 'age':
        return (a, x) => {
          const aa = ageOf(a),
            xa = ageOf(x);
          if (aa !== xa) return aa < xa ? -1 : 1;
          return prioOf(a) - prioOf(x);
        };
      case 'alpha':
        return (a, x) => textOf(a).localeCompare(textOf(x));
      case 'estimate':
        return (a, x) => {
          const d = estOf(a) - estOf(x);
          if (d !== 0) return d;
          return prioOf(a) - prioOf(x);
        };
      default:
        return (a, x) => {
          const ad = dueOf(a),
            xd = dueOf(x);
          if (ad !== xd) return ad < xd ? -1 : 1;
          return prioOf(a) - prioOf(x);
        };
    }
  });

  // Per-smart-filter counts so the view tabs can show badges.
  // Derived from the unfiltered open task list because the badge
  // reflects "is this view worth visiting right now", not "after
  // filters".
  let viewCounts = $derived.by<ViewCounts>(() => {
    const tasks = deps.getTasks();
    const sevenDaysAgo = Date.now() - 7 * 24 * 60 * 60 * 1000;
    let inbox = 0,
      stale = 0,
      quickwins = 0,
      review = 0;
    for (const t of tasks) {
      if (!t.done) {
        if ((t.triage || 'inbox') === 'inbox') inbox++;
        if (isStale(t)) stale++;
        if (
          t.priority >= 1 &&
          t.priority <= 2 &&
          t.estimatedMinutes &&
          t.estimatedMinutes <= 30
        )
          quickwins++;
      } else if (t.completedAt && new Date(t.completedAt).getTime() > sevenDaysAgo) {
        review++;
      }
    }
    return { inbox, stale, quickwins, review };
  });

  let listGroups = $derived.by<ListGroup[]>(() => {
    const filtered = deps.getFiltered();
    const projects = deps.getProjects();
    const goals = deps.getGoals();
    const deadlines = deps.getDeadlines();
    if (groupBy === 'due') {
      const now = new Date();
      const today = fmtDateISO(now);
      const tmw = new Date(now);
      tmw.setDate(tmw.getDate() + 1);
      const tomorrow = fmtDateISO(tmw);
      const wk = new Date(now);
      wk.setDate(wk.getDate() + 7);
      const weekEnd = fmtDateISO(wk);
      const b: Record<string, Task[]> = {
        overdue: [],
        today: [],
        tomorrow: [],
        this_week: [],
        later: [],
        no_date: [],
        done: []
      };
      for (const t of filtered) {
        if (t.done) {
          b.done.push(t);
          continue;
        }
        if (!t.dueDate && !t.scheduledStart) {
          b.no_date.push(t);
          continue;
        }
        const d = t.dueDate ?? (t.scheduledStart ? t.scheduledStart.slice(0, 10) : '');
        if (d < today) b.overdue.push(t);
        else if (d === today) b.today.push(t);
        else if (d === tomorrow) b.tomorrow.push(t);
        else if (d < weekEnd) b.this_week.push(t);
        else b.later.push(t);
      }
      // Per-bucket ordering: 'auto' uses the legacy "date asc, then
      // priority asc" rule; an explicit sortBy choice applies the
      // selected criterion to EVERY bucket so the user gets the same
      // shape regardless of which group they look at.
      Object.values(b).forEach((arr) => arr.sort(taskComparator));
      return [
        { key: 'overdue', label: 'Overdue', tasks: b.overdue },
        { key: 'today', label: 'Today', tasks: b.today },
        { key: 'tomorrow', label: 'Tomorrow', tasks: b.tomorrow },
        { key: 'this_week', label: 'This week', tasks: b.this_week },
        { key: 'later', label: 'Later', tasks: b.later },
        { key: 'no_date', label: 'No date', tasks: b.no_date },
        { key: 'done', label: 'Done', tasks: b.done }
      ].filter((g) => g.tasks.length > 0);
    }
    if (groupBy === 'priority') {
      const b: Record<string, Task[]> = { '1': [], '2': [], '3': [], '0': [] };
      for (const t of filtered) b[String(t.priority)].push(t);
      return [
        { key: '1', label: 'P1 high', tasks: b['1'] },
        { key: '2', label: 'P2 med', tasks: b['2'] },
        { key: '3', label: 'P3 low', tasks: b['3'] },
        { key: '0', label: 'no priority', tasks: b['0'] }
      ].filter((g) => g.tasks.length > 0);
    }
    if (groupBy === 'tag') {
      const b: Record<string, Task[]> = {};
      for (const t of filtered) {
        const tags = t.tags && t.tags.length ? t.tags : ['(untagged)'];
        for (const tag of tags) (b[tag] ??= []).push(t);
      }
      return Object.entries(b)
        .map(([k, v]) => ({
          key: k,
          label: '#' + k.replace('(untagged)', 'untagged'),
          tasks: v
        }))
        .sort((a, b) => b.tasks.length - a.tasks.length);
    }
    if (groupBy === 'project') {
      const b: Record<string, Task[]> = {};
      for (const t of filtered) {
        // Prefer explicit projectId; fall back to membership
        // inferred from matching project's folder; else the
        // top-level folder.
        let key = t.projectId || '';
        if (!key) {
          const matched = projects.find(
            (p) => p.folder && t.notePath.startsWith(p.folder + '/')
          );
          key = matched?.name ?? (t.notePath.split('/')[0] || '(no project)');
        }
        (b[key] ??= []).push(t);
      }
      return Object.entries(b)
        .map(([k, v]) => ({
          key: k,
          label: k,
          tasks: v,
          deepLink: projects.find((p) => p.name === k)
            ? `/projects/${encodeURIComponent(k)}`
            : undefined
        }))
        .sort((a, b) => a.label.localeCompare(b.label));
    }
    if (groupBy === 'goal') {
      const b: Record<string, Task[]> = {};
      for (const t of filtered) {
        const key = t.goalId || '(no goal)';
        (b[key] ??= []).push(t);
      }
      return Object.entries(b)
        .map(([k, v]) => {
          const g = goals.find((x) => x.id === k);
          return {
            key: k,
            label: g ? `🎯 ${g.title} (${g.id})` : k,
            tasks: v,
            // /goals/[id] doesn't exist as a route — the SPA shell
            // matched but the client router fell through, looking
            // like a freeze on click. Use the same-page focus param
            // the /goals page already understands.
            deepLink: g ? `/goals?focus=${encodeURIComponent(g.id)}` : undefined
          };
        })
        .sort((a, b) => {
          // Pin (no goal) to the bottom so the named buckets are
          // surfaced first.
          if (a.key === '(no goal)') return 1;
          if (b.key === '(no goal)') return -1;
          return a.label.localeCompare(b.label);
        });
    }
    if (groupBy === 'deadline') {
      const b: Record<string, Task[]> = {};
      for (const t of filtered) {
        const key = t.deadlineId || '(no deadline)';
        (b[key] ??= []).push(t);
      }
      return Object.entries(b)
        .map(([k, v]) => {
          const d = deadlines.find((x) => x.id === k);
          return {
            key: k,
            label: d ? `⏰ ${d.title} · ${d.date}` : k,
            tasks: v,
            deepLink: d ? `/deadlines?focus=${encodeURIComponent(d.id)}` : undefined
          };
        })
        .sort((a, b) => {
          if (a.key === '(no deadline)') return 1;
          if (b.key === '(no deadline)') return -1;
          // Sort by deadline date ascending — soonest first.
          const da = deadlines.find((x) => x.id === a.key)?.date ?? '';
          const db = deadlines.find((x) => x.id === b.key)?.date ?? '';
          return da.localeCompare(db);
        });
    }
    // groupBy === 'note' — group by source notePath. Default catch-
    // all so any future group shapes still produce a sensible result.
    const b: Record<string, Task[]> = {};
    for (const t of filtered) (b[t.notePath] ??= []).push(t);
    return Object.entries(b)
      .map(([k, v]) => ({ key: k, label: k, tasks: v }))
      .sort((a, b) => a.label.localeCompare(b.label));
  });

  let weekColumns = $derived.by<WeekColumns>(() => {
    const filtered = deps.getFiltered();
    const today = todayISO();
    const todayD = new Date(today + 'T00:00:00');
    const days: DayColumn[] = [];
    const byDate = new Map<string, Task[]>();
    const unscheduled: Task[] = [];
    const overdue: Task[] = [];
    for (let i = 0; i < 7; i++) {
      const d = new Date(todayD);
      d.setDate(d.getDate() + i);
      const iso = fmtDateISO(d);
      days.push({
        date: iso,
        label:
          i === 0
            ? 'Today'
            : i === 1
              ? 'Tomorrow'
              : d.toLocaleDateString(undefined, { weekday: 'short' }),
        sublabel: d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' }),
        isToday: i === 0,
        tasks: []
      });
      byDate.set(iso, days[i].tasks);
    }
    for (const t of filtered) {
      if (t.done) continue;
      const due = t.dueDate ?? '';
      const sched = t.scheduledStart ? t.scheduledStart.slice(0, 10) : '';
      const anchor = due || sched;
      if (!anchor) {
        unscheduled.push(t);
        continue;
      }
      if (anchor < today) {
        overdue.push(t);
        continue;
      }
      const bucket = byDate.get(anchor);
      if (bucket) bucket.push(t);
      // Tasks beyond +6 days fall off the grid; the user can switch
      // to list view with a future-week filter for those.
    }
    // Sort each column by scheduled time (if any), then priority.
    const cmp = (a: Task, b: Task) => {
      const at = a.scheduledStart ? a.scheduledStart.slice(11) : '99:99';
      const bt = b.scheduledStart ? b.scheduledStart.slice(11) : '99:99';
      if (at !== bt) return at.localeCompare(bt);
      return (a.priority || 9) - (b.priority || 9);
    };
    for (const col of days) col.tasks.sort(cmp);
    unscheduled.sort((a, b) => (a.priority || 9) - (b.priority || 9));
    overdue.sort((a, b) => (a.dueDate ?? '').localeCompare(b.dueDate ?? ''));
    return { unscheduled, overdue, days };
  });

  function pickOverflowView(v: View) {
    view = v;
    moreViewsOpen = false;
  }

  function onMoreViewsKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      moreViewsOpen = false;
      e.stopPropagation();
    }
  }

  function selectView(v: View) {
    view = v;
    moreViewsOpen = false;
  }

  function cycleView(direction: 1 | -1) {
    const i = VIEW_CYCLE.indexOf(view);
    const base = i >= 0 ? i : 0;
    const next = (base + direction + VIEW_CYCLE.length) % VIEW_CYCLE.length;
    view = VIEW_CYCLE[next];
  }

  function toggleSection(key: string) {
    // Read the same per-section default the SectionList uses so the
    // toggle flips against the effective state (a 'later' section
    // that looks collapsed because of the default but has no
    // explicit entry should record 'false' on first toggle, not
    // 'true').
    const defaultCollapsed = key === 'later' || key === 'no_date' || key === 'done';
    const current = collapsedSections[key];
    const effective = current === undefined ? defaultCollapsed : current;
    collapsedSections = { ...collapsedSections, [key]: !effective };
  }

  return {
    get view() {
      return view;
    },
    set view(v) {
      view = v;
    },
    get groupBy() {
      return groupBy;
    },
    set groupBy(v) {
      groupBy = v;
    },
    get sortBy() {
      return sortBy;
    },
    set sortBy(v) {
      sortBy = v;
    },
    get kanbanMode() {
      return kanbanMode;
    },
    set kanbanMode(v) {
      kanbanMode = v;
    },
    get kanbanSwimlane() {
      return kanbanSwimlane;
    },
    set kanbanSwimlane(v) {
      kanbanSwimlane = v;
    },
    get density() {
      return density;
    },
    set density(v) {
      density = v;
    },
    get helpOpen() {
      return helpOpen;
    },
    set helpOpen(v) {
      helpOpen = v;
    },
    get filterPanelOpen() {
      return filterPanelOpen;
    },
    set filterPanelOpen(v) {
      filterPanelOpen = v;
    },
    get moreViewsOpen() {
      return moreViewsOpen;
    },
    set moreViewsOpen(v) {
      moreViewsOpen = v;
    },
    get collapsedSections() {
      return collapsedSections;
    },
    set collapsedSections(v) {
      collapsedSections = v;
    },
    get compactCards() {
      return compactCards;
    },
    get activeOverflowLabel() {
      return activeOverflowLabel;
    },
    get taskComparator() {
      return taskComparator;
    },
    get listGroups() {
      return listGroups;
    },
    get weekColumns() {
      return weekColumns;
    },
    get viewCounts() {
      return viewCounts;
    },
    pickOverflowView,
    onMoreViewsKey,
    selectView,
    cycleView,
    toggleSection
  };
}
