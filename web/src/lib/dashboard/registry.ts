import type { Component } from 'svelte';
import type { DashboardWidgetType } from '$lib/api';

// Lazy widget registry. Each entry's `component` is a memoised loader
// (() => Promise<Component>) so Vite code-splits each widget into its
// own chunk and the home route only fetches the chunks for widgets
// the user actually has enabled. The loader is memoised so re-renders
// don't refetch — the dynamic import resolves to the same module
// instance on subsequent calls.
//
// The 2026-05-23 cleanup pulled 16 widgets that read as dashboard
// clutter:
//   - Analytics that belong in Settings/Stats:
//       task-velocity, ai-usage, ai-briefing
//   - Redundant with today-stream (which already merges events,
//     scheduled tasks, deadlines, and tomorrow/day-after preview):
//       at-a-glance, calendar-week, top-deadlines, top-goals
//   - Redundant with today-focus (the morning commitment surface):
//       one-thing, vision
//   - Niche or legacy / better surfaced elsewhere:
//       install, pinned, weekly-review-nudge, recent-annotations,
//       verse-for-mood, pomodoro (already a nav pill), quick-links
//       (lives on /hub)
// The .svelte files for removed widgets stay in the tree (git
// remembers; cheap to revive). Removing them from the registry is
// what hides them from the runtime.

export interface WidgetMeta {
  type: DashboardWidgetType;
  label: string;
  description: string;
  /** number of grid columns (out of 2) the widget should span on lg+ */
  span: 1 | 2;
  /** Memoised dynamic import. Yields the default-exported component
   *  on first call; cached promise on subsequent calls so re-renders
   *  (which call this from {#await}) don't refetch the chunk. */
  load: () => Promise<Component<any>>;
}

function lazy(loader: () => Promise<{ default: Component<any> }>): () => Promise<Component<any>> {
  let cached: Promise<Component<any>> | null = null;
  return () => {
    if (!cached) cached = loader().then((m) => m.default);
    return cached;
  };
}

export const widgetRegistry: WidgetMeta[] = [
  { type: 'greeting', label: 'Greeting', description: 'Date + welcome', span: 2, load: lazy(() => import('./widgets/GreetingWidget.svelte')) },
  // Tagesordnung — 16 Leitbegriffe as a quiet anchor beneath the
  // greeting. Single source of truth lives in $lib/principles.
  { type: 'tagesordnung', label: 'Tagesordnung', description: 'Sixteen Leitbegriffe — daily anchor for inner order. Quiet, no tracking, no score.', span: 2, load: lazy(() => import('./widgets/TagesordnungWidget.svelte')) },
  // Today stream — the headline "what's happening now and what's
  // next" widget. Merges today's events + scheduled tasks + due
  // tasks + due deadlines into one chronological feed with past
  // items dimmed, plus a 2-day forward preview. Replaces the four
  // tiles (at-a-glance, calendar-week, top-deadlines, top-goals)
  // that were folded into it in the 2026-05-23 cleanup.
  { type: 'today-stream', label: 'Today stream', description: 'One chronological feed: events, scheduled tasks, deadlines — plus tomorrow + day-after preview', span: 2, load: lazy(() => import('./widgets/TodayStreamWidget.svelte')) },
  // Today focus — the AI-suggested #1 thing for the day. Anchors
  // intention right under the stream. Subsumes the older one-thing
  // (weekly commit) and vision (life anchor) widgets.
  { type: 'today-focus', label: 'Today\'s focus', description: 'What you committed to in the morning routine', span: 2, load: lazy(() => import('./widgets/TodayFocusWidget.svelte')) },
  { type: 'now', label: 'Now', description: 'Current time + next event', span: 1, load: lazy(() => import('./widgets/NowWidget.svelte')) },
  { type: 'streaks', label: 'Streaks', description: 'Habit streaks at a glance', span: 1, load: lazy(() => import('./widgets/StreaksWidget.svelte')) },
  { type: 'scripture', label: 'Today\'s verse', description: 'Daily scripture / quote rotation', span: 1, load: lazy(() => import('./widgets/ScriptureWidget.svelte')) },
  { type: 'daily-note', label: 'Daily note', description: 'Link to today\'s daily note', span: 1, load: lazy(() => import('./widgets/DailyNoteWidget.svelte')) },
  // Quick capture spans the full row — the input row is wide and
  // benefits from breathing room. A column-narrow capture box is the
  // usability hit the user reported when this was span 1.
  { type: 'quick-capture', label: 'Quick capture', description: 'Add a task / jot / note fast — voice + smart parsing + undo', span: 2, load: lazy(() => import('./widgets/QuickCaptureWidget.svelte')) },
  { type: 'today-tasks', label: 'Today\'s tasks', description: 'Overdue + due + open', span: 2, load: lazy(() => import('./widgets/TodayTasksWidget.svelte')) },
  { type: 'scheduled-today', label: 'Scheduled today', description: 'Time-blocked tasks', span: 1, load: lazy(() => import('./widgets/ScheduledTodayWidget.svelte')) },
  { type: 'goals-progress', label: 'Goals progress', description: 'Active goals + milestones', span: 1, load: lazy(() => import('./widgets/GoalsProgressWidget.svelte')) },
  { type: 'recent-notes', label: 'Recent notes', description: 'Latest modified', span: 1, load: lazy(() => import('./widgets/RecentNotesWidget.svelte')) },
  { type: 'projects-active', label: 'Active projects', description: 'From granit projects.json', span: 1, load: lazy(() => import('./widgets/ProjectsWidget.svelte')) },
  // Ventures + Prayer share the "umbrella above tactics" theme — both
  // surface the layer the user is working *for*, not what they're
  // doing right now.
  { type: 'ventures', label: 'Ventures', description: 'Active ventures with project + goal counts', span: 1, load: lazy(() => import('./widgets/VenturesWidget.svelte')) },
  { type: 'prayer', label: 'Prayer', description: 'Active intentions — work-tied first', span: 1, load: lazy(() => import('./widgets/PrayerWidget.svelte')) },
  { type: 'inbox', label: 'Inbox', description: 'Tasks granit hasn\'t triaged', span: 1, load: lazy(() => import('./widgets/InboxWidget.svelte')) },
  { type: 'habits', label: 'Habits', description: 'Today\'s habit ticks + per-target progress + at-risk indicator', span: 1, load: lazy(() => import('./widgets/HabitsWidget.svelte')) },
  // Sabbath — three-state tile from the synced sabbath schedule.
  { type: 'sabbath', label: 'Sabbath', description: 'Countdown to the next sabbath, or remaining time when active', span: 1, load: lazy(() => import('./widgets/SabbathWidget.svelte')) },
  // Roots — four-domain snapshot mirroring the /roots dashboard
  // at glanceable density.
  { type: 'roots', label: 'Roots snapshot', description: 'One line per life domain — Spirit, Mind, Body, Vocation', span: 1, load: lazy(() => import('./widgets/RootsWidget.svelte')) },
  // Weekly-plan commitments — pulls tasks whose notePath matches the
  // current ISO week's plan note, groups by venture, shows done/
  // total per group. Bridge between Sunday planning and daily view.
  { type: 'weekly-plan', label: 'Weekly plan commitments', description: 'This week\'s committed tasks from /plans/week, grouped by venture', span: 1, load: lazy(() => import('./widgets/WeeklyPlanWidget.svelte')) },
  // Meals — three (or more) daily slots backed by the daily-note
  // `## Meals` section. Goal is "did I eat enough today?" visibility,
  // NOT calorie tracking.
  { type: 'meals', label: 'Meals', description: 'Today\'s meal slots with checkbox + free-text capture; syncs to the calendar', span: 1, load: lazy(() => import('./widgets/MealsWidget.svelte')) }
];

export function widgetMeta(type: string): WidgetMeta | undefined {
  return widgetRegistry.find((w) => w.type === type);
}
