// Static catalogs surfaced by the command palette.
//
// PAGES: every top-level destination in the app. Mirrors the sidebar
// (see +layout.svelte sections), plus a handful of utility routes
// that aren't in the nav (sabbath, stats) so the switcher can reach
// them. Icon glyphs use NavIcon names rather than emoji — palette
// rows are tight horizontally and a single icon keeps the list
// scannable. The icon's job here is recognition, not pixel-perfect
// parity with the sidebar.
//
// AGENTS: each entry either (a) opens the AI overlay with a seeded
// prompt (briefing/triage/find-time), (b) switches the overlay into a
// specific mode (PM, Goal Manager, Coach, etc.), or (c) navigates to
// the page that owns the agent (Task Agent lives on /tasks). For
// contextual modes (project-manager, goal-manager, calendar-manager)
// we navigate to the matching page first so the prelude can pick up
// entity context; the overlay then opens in that mode. The
// `text: ''` seed keeps the composer empty — the user has chosen the
// posture, not the prompt.
//
// Both lists are plain arrays with no Svelte state, so this lives in
// a pure .ts module. The palette imports them as constants and only
// re-keys the rows (page:/path, agent:slug) when it merges them into
// the unified CmdItem list.

import { goto } from '$app/navigation';
import { openAIOverlay } from '$lib/stores/ai-overlay';
import type { AgentCmd } from './paletteTypes';

export const PAGES: { path: string; label: string; icon: string }[] = [
  { path: '/', label: 'Today', icon: 'today' },
  { path: '/morning', label: 'Morning', icon: 'morning' },
  { path: '/tasks', label: 'Tasks', icon: 'tasks' },
  { path: '/calendar', label: 'Calendar', icon: 'calendar' },
  { path: '/jots', label: 'Jots', icon: 'jots' },
  { path: '/habits', label: 'Habits', icon: 'habits' },
  { path: '/examen', label: 'Examen', icon: 'examen' },
  { path: '/vision', label: 'Vision', icon: 'vision' },
  { path: '/review', label: 'Review', icon: 'review' },
  { path: '/goals', label: 'Goals', icon: 'goals' },
  { path: '/deadlines', label: 'Deadlines', icon: 'deadline' },
  { path: '/projects', label: 'Projects', icon: 'projects' },
  { path: '/ventures', label: 'Ventures', icon: 'ventures' },
  { path: '/finance', label: 'Finance', icon: 'finance' },
  { path: '/shopping', label: 'Shopping', icon: 'shopping' },
  { path: '/hub', label: 'Hub', icon: 'hub' },
  { path: '/people', label: 'People', icon: 'people' },
  { path: '/measurements', label: 'Metrics', icon: 'measurements' },
  { path: '/prayer', label: 'Prayer', icon: 'prayer' },
  { path: '/scripture', label: 'Scripture', icon: 'scripture' },
  { path: '/notes', label: 'Notes', icon: 'notes' },
  { path: '/search', label: 'Search', icon: 'search' },
  { path: '/books', label: 'Books', icon: 'books' },
  { path: '/templates', label: 'Templates', icon: 'templates' },
  { path: '/objects', label: 'Objects', icon: 'objects' },
  { path: '/tags', label: 'Tags', icon: 'tags' },
  { path: '/agents', label: 'Agents', icon: 'agents' },
  { path: '/chat', label: 'Chat', icon: 'chat' },
  { path: '/sabbath', label: 'Sabbath', icon: 'prayer' },
  { path: '/stats', label: 'Stats', icon: 'stats' },
  { path: '/settings', label: 'Settings', icon: 'settings' }
];

export const AGENTS: AgentCmd[] = [
  {
    slug: 'briefing',
    label: 'Run daily briefing',
    detail: 'Top 3 focus items + one thing you might forget',
    icon: 'morning',
    run: () =>
      openAIOverlay({
        text: 'Give me a short morning briefing — top three things I should focus on today and one thing I might be forgetting.',
        send: true
      })
  },
  {
    slug: 'triage',
    label: 'Triage open tasks',
    detail: 'Pick 3 for today, defer or delete the rest',
    icon: 'tasks',
    run: () =>
      openAIOverlay({
        text: 'Help me triage my open tasks — which 3 should I do today, and what should I defer or delete?',
        send: true
      })
  },
  {
    slug: 'find-time',
    label: 'Find time for deep work',
    detail: '60-minute slots in the next 3 days',
    icon: 'calendar',
    run: () =>
      openAIOverlay({
        modeId: 'analyst',
        text: 'Find me 60 minutes for deep work in the next 3 days. List 3 candidate slots.',
        send: false
      })
  },
  {
    slug: 'project-manager',
    label: 'Open Project Manager mode',
    detail: 'PM coach — go to /projects',
    icon: 'projects',
    run: async () => {
      await goto('/projects');
      openAIOverlay({ modeId: 'project-manager', text: '' });
    }
  },
  {
    slug: 'goal-manager',
    label: 'Open Goal Manager mode',
    detail: 'Goal coach — go to /goals',
    icon: 'goals',
    run: async () => {
      await goto('/goals');
      openAIOverlay({ modeId: 'goal-manager', text: '' });
    }
  },
  {
    slug: 'calendar-manager',
    label: 'Open Calendar Manager mode',
    detail: 'Schedule strategist — go to /calendar',
    icon: 'calendar',
    run: async () => {
      await goto('/calendar');
      openAIOverlay({ modeId: 'calendar-manager', text: '' });
    }
  },
  {
    slug: 'task-agent',
    label: 'Open Task Agent on /tasks',
    detail: 'Bulk task ops — press ‘a’ on the tasks page',
    icon: 'tasks',
    hint: 'a',
    run: () => goto('/tasks')
  },
  // Posture-only modes — open the overlay with a different system
  // prompt but no seeded text. The user picks their question.
  {
    slug: 'mode-coach',
    label: 'Switch chat to Coach mode',
    detail: 'Socratic — questions over answers',
    icon: 'chat',
    run: () => openAIOverlay({ modeId: 'coach', text: '' })
  },
  {
    slug: 'mode-research',
    label: 'Switch chat to Research mode',
    detail: 'Grounded answers from your vault',
    icon: 'search',
    run: () => openAIOverlay({ modeId: 'research', text: '' })
  },
  {
    slug: 'mode-writer',
    label: 'Switch chat to Writer mode',
    detail: 'Drafting partner that matches your voice',
    icon: 'jots',
    run: () => openAIOverlay({ modeId: 'writer', text: '' })
  },
  {
    slug: 'mode-analyst',
    label: 'Switch chat to Analyst mode',
    detail: 'Evidence-first — what does the data say',
    icon: 'stats',
    run: () => openAIOverlay({ modeId: 'analyst', text: '' })
  },
  {
    slug: 'mode-architect',
    label: 'Switch chat to Architect mode',
    detail: 'System design with named trade-offs',
    icon: 'objects',
    run: () => openAIOverlay({ modeId: 'architect', text: '' })
  }
];
