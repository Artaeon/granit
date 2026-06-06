// Data + rollups controller for the venture detail page.
//
// Third extraction step out of routes/ventures/[name]/+page.svelte.
// Owns the six loaded-state arrays (venture, projects, goals,
// deadlines, intentions, linkedNotes), the loading / notFound flags,
// the parallel load() that hits six endpoints with Promise.allSettled,
// and every derived rollup the page renders against (active*, paused
// projects, aggregate task counts, aggregate progress, next deadline,
// tab descriptor list).
//
// The page still owns the WS subscription, visibilitychange / focus
// listeners, the URL-param effect, and the AI-summary dismiss
// orchestration — those all touch component lifecycle and need to run
// from the component context. This controller exposes load() so those
// callers can trigger a re-fetch.
//
// Aggregation rationale (preserved from the page): client-side
// rollup keeps the four list endpoints cache-friendly (every other
// page hits the same ones) and avoids a server-side decorated
// endpoint that would have to stay in sync with each underlying
// schema. If perf becomes a problem we can collapse to a single
// /ventures/{name} response without touching the page's render logic
// or this controller's public surface.

import {
  api,
  type Venture,
  type Project,
  type Goal,
  type Deadline,
  type PrayerIntention,
  type Note
} from '$lib/api';
import { toast } from '$lib/components/toast';
import { errorMessage } from '$lib/util/errorMessage';
import { daysUntil } from '$lib/deadlines/util';

export type VenturesDetailTab = 'overview' | 'projects' | 'goals' | 'links' | 'notes';

export interface VenturesDetailTabDescriptor {
  id: VenturesDetailTab;
  label: string;
  count?: number;
}

export interface VenturesDetailDataDeps {
  /** Returns the URL-decoded venture name from the route param. Read
   *  each call so the controller picks up navigations to other
   *  ventures without the page having to re-construct it. */
  getName: () => string;
}

export interface VenturesDetailDataController {
  /** Loaded venture record, or null while not yet fetched / not found. */
  venture: Venture | null;
  /** Projects whose `venture` field matches the route name
   *  (case-insensitive). */
  readonly projects: Project[];
  /** Goals filtered by venture. */
  readonly goals: Goal[];
  /** Deadlines filtered by venture (raw — includes met / cancelled). */
  readonly deadlines: Deadline[];
  /** Prayer intentions filtered by venture (raw — includes
   *  resting / answered). */
  readonly intentions: PrayerIntention[];
  /** Notes whose body or frontmatter mentions the venture name —
   *  trimmed to the top 12 server-relevance hits. */
  readonly linkedNotes: Note[];

  /** True while load() is in flight. Drives the skeleton hero. */
  readonly loading: boolean;
  /** True when the venture lookup returned no record. Drives the
   *  "no venture named X" empty state. */
  readonly notFound: boolean;

  // ----- Derived rollups -----
  readonly activeProjects: Project[];
  readonly pausedProjects: Project[];
  readonly activeGoals: Goal[];
  readonly activeDeadlines: Deadline[];
  readonly activeIntentions: PrayerIntention[];
  /** Sum of (tasksTotal − tasksDone) across all venture projects. */
  readonly aggregateTasksOpen: number;
  /** Sum of tasksDone across all venture projects. */
  readonly aggregateTasksDone: number;
  /** Mean of (progress ?? 0) across active projects; 0 when empty. */
  readonly aggregateProgress: number;
  /** Soonest active deadline by daysUntil, or null when none. */
  readonly nextDeadline: Deadline | null;
  /** Tab descriptors for the sub-nav, with counts. The notes tab is
   *  omitted when no notes match — additive only on hit. */
  readonly tabs: VenturesDetailTabDescriptor[];

  /** Fetch venture + linked entities in parallel. Promise.allSettled
   *  so a single failing module (deadlines disabled, prayer disabled)
   *  doesn't take the whole page down. Sets notFound when the
   *  venture itself is missing; toasts on unexpected error. */
  load(): Promise<void>;
}

export function createVenturesDetailData(
  deps: VenturesDetailDataDeps
): VenturesDetailDataController {
  let venture = $state<Venture | null>(null);
  let projects = $state<Project[]>([]);
  let goals = $state<Goal[]>([]);
  let deadlines = $state<Deadline[]>([]);
  let intentions = $state<PrayerIntention[]>([]);
  let linkedNotes = $state<Note[]>([]);
  let loading = $state(false);
  let notFound = $state(false);

  async function load() {
    const name = deps.getName();
    if (!name) return;
    loading = true;
    notFound = false;
    try {
      // Fetch venture + the four linked-entity lists in parallel.
      // Each list endpoint we already use elsewhere, so the browser
      // typically has the response in cache. Promise.allSettled so
      // a single failing module (deadlines disabled, prayer disabled)
      // doesn't take the whole page down.
      const [vRes, pRes, gRes, dRes, iRes, nRes] = await Promise.allSettled([
        api.getVenture(name),
        api.listProjects(),
        api.listGoals(),
        api.tryListDeadlines(),
        api.listPrayer(),
        // Notes search by venture name — the server's full-text index
        // surfaces notes that mention the name in body or frontmatter.
        // Best-effort; if the search endpoint is unavailable we just
        // hide the Notes tab content.
        api.listNotes({ q: name, limit: 30 })
      ]);

      if (vRes.status !== 'fulfilled') {
        notFound = true;
        return;
      }
      venture = vRes.value;

      // Filter each list by venture (case-insensitive — the server
      // does the same on Find, so a project tagged with lowercase
      // venture name still rolls up to the canonical record).
      const lowerName = name.toLowerCase();
      const allProjects = pRes.status === 'fulfilled' ? pRes.value.projects : [];
      const allGoals = gRes.status === 'fulfilled' ? gRes.value.goals : [];
      const allDeadlines = dRes.status === 'fulfilled' ? (dRes.value ?? []) : [];
      const allIntentions = iRes.status === 'fulfilled' ? iRes.value.intentions : [];
      const allNotes = nRes.status === 'fulfilled' ? nRes.value.notes : [];

      projects = allProjects.filter((p) => (p.venture ?? '').toLowerCase() === lowerName);
      goals = allGoals.filter((g) => (g.venture ?? '').toLowerCase() === lowerName);
      deadlines = allDeadlines.filter((d) => (d.venture ?? '').toLowerCase() === lowerName);
      intentions = allIntentions.filter((p) => (p.venture ?? '').toLowerCase() === lowerName);
      // Notes don't have a structured venture field — full-text match
      // is the best signal we have, and we trust the server's relevance
      // ordering. We trim to the top 12 so the tab stays scannable.
      linkedNotes = allNotes.slice(0, 12);
    } catch (e) {
      toast.error('failed to load venture: ' + errorMessage(e));
    } finally {
      loading = false;
    }
  }

  // ----- Derived rollups -----

  let activeProjects = $derived(projects.filter((p) => (p.status ?? 'active') === 'active'));
  let pausedProjects = $derived(projects.filter((p) => p.status === 'paused'));
  let activeGoals = $derived(goals.filter((g) => (g.status ?? 'active') === 'active'));
  let activeDeadlines = $derived(
    deadlines.filter((d) => d.status !== 'met' && d.status !== 'cancelled')
  );
  let activeIntentions = $derived(intentions.filter((p) => p.status === 'praying'));

  // Aggregate task counts across the venture's projects — a single
  // "23 open · 7 done" line at the top of the page is the fastest
  // read on overall momentum.
  let aggregateTasksOpen = $derived(
    projects.reduce((acc, p) => acc + ((p.tasksTotal ?? 0) - (p.tasksDone ?? 0)), 0)
  );
  let aggregateTasksDone = $derived(projects.reduce((acc, p) => acc + (p.tasksDone ?? 0), 0));

  // Average progress across active projects (each project's progress
  // is already derived server-side from its goals + tasks).
  let aggregateProgress = $derived.by(() => {
    if (activeProjects.length === 0) return 0;
    const sum = activeProjects.reduce((acc, p) => acc + (p.progress ?? 0), 0);
    return sum / activeProjects.length;
  });

  // Next deadline — the one with the smallest non-negative daysUntil
  // among active deadlines. Used in the hero metric tile.
  let nextDeadline = $derived.by<Deadline | null>(() => {
    if (activeDeadlines.length === 0) return null;
    return [...activeDeadlines].sort((a, b) => daysUntil(a.date) - daysUntil(b.date))[0];
  });

  // Tab button helpers — counts on each tab so the user can see what's
  // behind each one without flipping. Tabs hide entirely when the
  // venture has nothing in that bucket and there's nothing actionable
  // there (overview/projects/goals always show; links/notes hide on
  // empty since they're additive).
  let tabs = $derived.by<VenturesDetailTabDescriptor[]>(() => {
    const list: VenturesDetailTabDescriptor[] = [
      { id: 'overview', label: 'Overview' },
      { id: 'projects', label: 'Projects', count: projects.length },
      { id: 'goals', label: 'Goals', count: goals.length },
      {
        id: 'links',
        label: 'Deadlines & prayer',
        count: activeDeadlines.length + activeIntentions.length
      }
    ];
    if (linkedNotes.length > 0) {
      list.push({ id: 'notes', label: 'Notes', count: linkedNotes.length });
    }
    return list;
  });

  return {
    get venture() {
      return venture;
    },
    set venture(v) {
      venture = v;
    },
    get projects() {
      return projects;
    },
    get goals() {
      return goals;
    },
    get deadlines() {
      return deadlines;
    },
    get intentions() {
      return intentions;
    },
    get linkedNotes() {
      return linkedNotes;
    },
    get loading() {
      return loading;
    },
    get notFound() {
      return notFound;
    },
    get activeProjects() {
      return activeProjects;
    },
    get pausedProjects() {
      return pausedProjects;
    },
    get activeGoals() {
      return activeGoals;
    },
    get activeDeadlines() {
      return activeDeadlines;
    },
    get activeIntentions() {
      return activeIntentions;
    },
    get aggregateTasksOpen() {
      return aggregateTasksOpen;
    },
    get aggregateTasksDone() {
      return aggregateTasksDone;
    },
    get aggregateProgress() {
      return aggregateProgress;
    },
    get nextDeadline() {
      return nextDeadline;
    },
    get tabs() {
      return tabs;
    },
    load
  };
}
