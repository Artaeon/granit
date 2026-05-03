import type { Component } from 'svelte';
import type { DashboardWidgetType } from '$lib/api';
import GreetingWidget from './widgets/GreetingWidget.svelte';
import NowWidget from './widgets/NowWidget.svelte';
import StreaksWidget from './widgets/StreaksWidget.svelte';
import ScriptureWidget from './widgets/ScriptureWidget.svelte';
import DailyNoteWidget from './widgets/DailyNoteWidget.svelte';
import QuickCaptureWidget from './widgets/QuickCaptureWidget.svelte';
import TodayTasksWidget from './widgets/TodayTasksWidget.svelte';
import ScheduledTodayWidget from './widgets/ScheduledTodayWidget.svelte';
import GoalsProgressWidget from './widgets/GoalsProgressWidget.svelte';
import RecentNotesWidget from './widgets/RecentNotesWidget.svelte';
import ProjectsWidget from './widgets/ProjectsWidget.svelte';
import InboxWidget from './widgets/InboxWidget.svelte';
import CalendarWeekWidget from './widgets/CalendarWeekWidget.svelte';
import TodayFocusWidget from './widgets/TodayFocusWidget.svelte';
import TopDeadlinesWidget from './widgets/TopDeadlinesWidget.svelte';
import VisionWidget from './widgets/VisionWidget.svelte';

export interface WidgetMeta {
  type: DashboardWidgetType;
  label: string;
  description: string;
  /** number of grid columns (out of 2) the widget should span on lg+ */
  span: 1 | 2;
  component: Component<any>;
}

export const widgetRegistry: WidgetMeta[] = [
  { type: 'greeting', label: 'Greeting', description: 'Date + welcome', span: 2, component: GreetingWidget },
  // Vision sits at the top of the registry (and gets injected at the
  // top of new dashboards) because it's the layer the user re-reads
  // every morning before drilling into tactics.
  { type: 'vision', label: 'Vision', description: 'Life mission, values, season focus — the layer above goals', span: 2, component: VisionWidget },
  { type: 'today-focus', label: 'Today\'s focus', description: 'What you committed to in the morning routine', span: 2, component: TodayFocusWidget },
  { type: 'now', label: 'Now', description: 'Current time + next event', span: 1, component: NowWidget },
  { type: 'streaks', label: 'Streaks', description: 'Habit streaks at a glance', span: 1, component: StreaksWidget },
  { type: 'top-deadlines', label: 'Top deadlines', description: 'Next 3 important deadlines', span: 1, component: TopDeadlinesWidget },
  { type: 'scripture', label: 'Today\'s verse', description: 'Daily scripture / quote rotation', span: 1, component: ScriptureWidget },
  { type: 'daily-note', label: 'Daily note', description: 'Link to today\'s daily note', span: 1, component: DailyNoteWidget },
  { type: 'quick-capture', label: 'Quick capture', description: 'Add a task fast', span: 1, component: QuickCaptureWidget },
  { type: 'today-tasks', label: 'Today\'s tasks', description: 'Overdue + due + open', span: 2, component: TodayTasksWidget },
  { type: 'scheduled-today', label: 'Scheduled today', description: 'Time-blocked tasks', span: 1, component: ScheduledTodayWidget },
  { type: 'goals-progress', label: 'Goals progress', description: 'Active goals + milestones', span: 1, component: GoalsProgressWidget },
  { type: 'recent-notes', label: 'Recent notes', description: 'Latest modified', span: 1, component: RecentNotesWidget },
  { type: 'projects-active', label: 'Active projects', description: 'From granit projects.json', span: 1, component: ProjectsWidget },
  { type: 'inbox', label: 'Inbox', description: 'Tasks granit hasn\'t triaged', span: 1, component: InboxWidget },
  { type: 'calendar-week', label: 'Calendar week', description: 'Next 7 days at a glance', span: 1, component: CalendarWeekWidget }
];

export function widgetMeta(type: string): WidgetMeta | undefined {
  return widgetRegistry.find((w) => w.type === type);
}
