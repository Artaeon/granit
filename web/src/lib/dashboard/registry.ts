import type { Component } from 'svelte';
import type { DashboardWidgetType } from '$lib/api';

// Lazy widget registry. Previously every widget component was
// imported eagerly at module load — 28 widget bundles all parsed
// upfront on the home page even though most users only enable a
// handful. Converting each entry's `component` into a memoised
// loader (() => Promise<Component>) means Vite code-splits each
// widget into its own chunk and the home route only fetches the
// chunks for widgets the user actually has enabled.
//
// Initial load on a typical 8-widget dashboard drops accordingly,
// and a user adding a new widget through the customize panel pays
// the per-widget fetch only at the moment they enable it.
//
// The loader is memoised so re-renders don't refetch — the dynamic
// import resolves to the same module instance, but using
// resolved components directly is the safer/cheaper path than
// re-awaiting each render.

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
  // Today stream — the headline "what's happening now and what's
  // next" widget. Merges today's events + scheduled tasks + due
  // tasks + due deadlines into one chronological feed with past
  // items dimmed, plus a 2-day forward preview. Replaces the need
  // to mentally combine today-tasks + scheduled-today + calendar-
  // week. Sits at the top of the registry so it slots above
  // at-a-glance in fresh dashboards; existing users get it
  // injected just above their at-a-glance via NEW_WIDGETS in
  // routes/+page.svelte.
  { type: 'today-stream', label: 'Today stream', description: 'One chronological feed: events, scheduled tasks, deadlines — plus tomorrow + day-after preview', span: 2, load: lazy(() => import('./widgets/TodayStreamWidget.svelte')) },
  // At-a-glance — single span-2 row of compact daily counts so the
  // user reads "shape of today" before anything else. Pairs with
  // today-stream — they answer "what's the count?" vs "what's the
  // order?".
  { type: 'at-a-glance', label: 'Today at a glance', description: 'Compact stats: tasks due, overdue, deadlines, prayer, habits', span: 2, load: lazy(() => import('./widgets/AtAGlanceWidget.svelte')) },
  // Vision sits at the top of the registry (and gets injected at the
  // top of new dashboards) because it's the layer the user re-reads
  // every morning before drilling into tactics.
  { type: 'vision', label: 'Vision', description: 'Life mission, values, season focus — the layer above goals', span: 2, load: lazy(() => import('./widgets/VisionWidget.svelte')) },
  { type: 'one-thing', label: 'This week\'s one thing', description: 'Surfaces the commitment from your most recent weekly review', span: 2, load: lazy(() => import('./widgets/OneThingWidget.svelte')) },
  { type: 'today-focus', label: 'Today\'s focus', description: 'What you committed to in the morning routine', span: 2, load: lazy(() => import('./widgets/TodayFocusWidget.svelte')) },
  { type: 'now', label: 'Now', description: 'Current time + next event', span: 1, load: lazy(() => import('./widgets/NowWidget.svelte')) },
  { type: 'streaks', label: 'Streaks', description: 'Habit streaks at a glance', span: 1, load: lazy(() => import('./widgets/StreaksWidget.svelte')) },
  { type: 'top-deadlines', label: 'Top deadlines', description: 'Next 3 important deadlines', span: 1, load: lazy(() => import('./widgets/TopDeadlinesWidget.svelte')) },
  // Sits next to Top deadlines so the two "by-when" widgets cluster
  // visually when both are enabled. Pulls from goals.target_date —
  // free-text targets ("Q4 2026") are excluded since the widget is
  // about countdown pressure.
  { type: 'top-goals', label: 'Next goal targets', description: 'Top 3 active goals by target_date proximity', span: 1, load: lazy(() => import('./widgets/TopGoalsWidget.svelte')) },
  // Quick links — surfaces hub favorites on the dashboard. Sits
  // close to top-deadlines / top-goals because it's the same
  // shape: a glance-fast "what do I reach for first" tile.
  { type: 'quick-links', label: 'Quick links', description: 'Top 5 favorites from your Hub — single-click access to the URLs you live in', span: 1, load: lazy(() => import('./widgets/QuickLinksWidget.svelte')) },
  // AI briefing — opt-in via Settings → AI features. Shows a one-
  // click "compose today's briefing" button until generated, then
  // renders the markdown inline with a "save to today" action.
  { type: 'ai-briefing', label: 'AI daily briefing', description: 'One-click summary of today\'s events + urgent tasks + next deadline. Opt-in.', span: 2, load: lazy(() => import('./widgets/AIBriefingWidget.svelte')) },
  // Task velocity — 8-week bar chart of completed-tasks-per-week
  // with a 3-week-avg trend arrow. Sits next to the at-a-glance
  // tile so the user can read "shape of today" alongside "shape
  // of the last two months" without scrolling.
  { type: 'task-velocity', label: 'Task velocity', description: 'Tasks completed per week (last 8 weeks) + trend arrow', span: 1, load: lazy(() => import('./widgets/TaskVelocityWidget.svelte')) },
  // Weekly-review nudge — companion to OneThingWidget. That one
  // surfaces the commitment *from* the most recent review; this
  // one only renders when the most recent review is > 7 days old
  // (or missing), so the dashboard quietly nags toward the next
  // ritual instead of going stale.
  { type: 'weekly-review-nudge', label: 'Weekly review nudge', description: 'CTA when your last weekly review is stale (>7 days) or missing', span: 1, load: lazy(() => import('./widgets/WeeklyReviewNudgeWidget.svelte')) },
  // AI usage — streamlined version of the audit rollup in
  // /settings. Surfaces today's call count, token total, and cost
  // so cost-conscious LLM use stays ambient rather than buried.
  { type: 'ai-usage', label: 'AI usage', description: 'Today\'s AI call count + tokens + cost — streamlined dashboard tile', span: 1, load: lazy(() => import('./widgets/AIUsageWidget.svelte')) },
  { type: 'scripture', label: 'Today\'s verse', description: 'Daily scripture / quote rotation', span: 1, load: lazy(() => import('./widgets/ScriptureWidget.svelte')) },
  // Verse-for-mood — companion to Today's verse. Tap a mood chip
  // ("anxious", "hopeful", …); the widget pulls verses carrying the
  // matching topic from the bundled catalogue. No AI call; uses the
  // /scripture topical index that already lives in scripture.go.
  { type: 'verse-for-mood', label: 'Verse for a mood', description: 'Tap a mood — get a verse on that theme. Topical, no AI required.', span: 1, load: lazy(() => import('./widgets/VerseForMoodWidget.svelte')) },
  { type: 'daily-note', label: 'Daily note', description: 'Link to today\'s daily note', span: 1, load: lazy(() => import('./widgets/DailyNoteWidget.svelte')) },
  // Quick capture spans the full row — the input row is wide and
  // the recent-captures strip below it benefits from breathing room.
  // A column-narrow capture box is the usability hit the user
  // reported when this was span 1.
  { type: 'quick-capture', label: 'Quick capture', description: 'Add a task / jot / note fast — voice + smart parsing + undo', span: 2, load: lazy(() => import('./widgets/QuickCaptureWidget.svelte')) },
  { type: 'today-tasks', label: 'Today\'s tasks', description: 'Overdue + due + open', span: 2, load: lazy(() => import('./widgets/TodayTasksWidget.svelte')) },
  { type: 'scheduled-today', label: 'Scheduled today', description: 'Time-blocked tasks', span: 1, load: lazy(() => import('./widgets/ScheduledTodayWidget.svelte')) },
  { type: 'goals-progress', label: 'Goals progress', description: 'Active goals + milestones', span: 1, load: lazy(() => import('./widgets/GoalsProgressWidget.svelte')) },
  { type: 'recent-notes', label: 'Recent notes', description: 'Latest modified', span: 1, load: lazy(() => import('./widgets/RecentNotesWidget.svelte')) },
  { type: 'projects-active', label: 'Active projects', description: 'From granit projects.json', span: 1, load: lazy(() => import('./widgets/ProjectsWidget.svelte')) },
  // Ventures + Prayer share the "umbrella above tactics" theme — both
  // surface the layer the user is working *for*, not what they're
  // doing right now. Listed adjacent to Projects so the related
  // entities cluster on the dashboard config view.
  { type: 'ventures', label: 'Ventures', description: 'Active ventures with project + goal counts', span: 1, load: lazy(() => import('./widgets/VenturesWidget.svelte')) },
  { type: 'prayer', label: 'Prayer', description: 'Active intentions — work-tied first', span: 1, load: lazy(() => import('./widgets/PrayerWidget.svelte')) },
  { type: 'inbox', label: 'Inbox', description: 'Tasks granit hasn\'t triaged', span: 1, load: lazy(() => import('./widgets/InboxWidget.svelte')) },
  { type: 'calendar-week', label: 'Calendar week', description: 'Next 7 days at a glance', span: 1, load: lazy(() => import('./widgets/CalendarWeekWidget.svelte')) },
  // Three widgets that lived in the bundle but had been dropped
  // from the registry — re-registered so users can opt in via the
  // customize panel. None auto-injected into existing dashboards.
  { type: 'habits', label: 'Habits', description: 'Today\'s habit ticks + per-target progress + at-risk indicator', span: 1, load: lazy(() => import('./widgets/HabitsWidget.svelte')) },
  { type: 'pomodoro', label: 'Pomodoro', description: 'In-page focus timer with audible chime; persists across reloads', span: 1, load: lazy(() => import('./widgets/PomodoroWidget.svelte')) },
  { type: 'install', label: 'Install commands', description: 'Granit\'s install commands for fresh devices', span: 2, load: lazy(() => import('./widgets/InstallWidget.svelte')) },
  // Recent annotations — surfaces the marginalia layer on the
  // home page so re-reads don't have to start by opening the
  // source note. Sister to recent-notes ("what did I edit") with
  // a different question ("what did I think about").
  { type: 'recent-annotations', label: 'Recent margin notes', description: 'Latest annotations across your vault — the marginalia layer at a glance', span: 1, load: lazy(() => import('./widgets/RecentAnnotationsWidget.svelte')) },
  // Sabbath — three-state tile from the synced sabbath schedule.
  // Active: remaining time. Scheduled: countdown to the next start.
  // Off: quiet "set up" link. Reads from the sabbath store; no
  // additional endpoint fetch on dashboard load.
  { type: 'sabbath', label: 'Sabbath', description: 'Countdown to the next sabbath, or remaining time when active', span: 1, load: lazy(() => import('./widgets/SabbathWidget.svelte')) },
  // Roots — four-domain snapshot mirroring the /roots dashboard
  // at glanceable density. Spirit (Bible streak), Mind (books
  // reading), Body (top habit streak), Vocation (top open goal).
  { type: 'roots', label: 'Roots snapshot', description: 'One line per life domain — Spirit, Mind, Body, Vocation', span: 1, load: lazy(() => import('./widgets/RootsWidget.svelte')) },
  // Weekly-plan commitments — pulls tasks whose notePath matches the
  // current ISO week's plan note, groups by venture (the "### Venture"
  // heading they sit under in the plan), shows done/total per group.
  // Bridge between the Sunday planning ritual and the daily view.
  { type: 'weekly-plan', label: 'Weekly plan commitments', description: 'This week\'s committed tasks from /plans/week, grouped by venture', span: 1, load: lazy(() => import('./widgets/WeeklyPlanWidget.svelte')) },
  // Meals — three (or more) daily slots backed by the daily-note
  // `## Meals` section. Goal is glanceable "did I eat enough today?"
  // visibility, NOT calorie tracking. Syncs with the calendar's
  // meal_slot strip.
  { type: 'meals', label: 'Meals', description: 'Today\'s meal slots with checkbox + free-text capture; syncs to the calendar', span: 1, load: lazy(() => import('./widgets/MealsWidget.svelte')) }
];

export function widgetMeta(type: string): WidgetMeta | undefined {
  return widgetRegistry.find((w) => w.type === type);
}
