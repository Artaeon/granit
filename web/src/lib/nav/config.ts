// Sidebar navigation config. Pulled out of +layout.svelte so the
// layout shell stays focused on composition and the nav surface has
// a single source of truth that route guards, mobile headers, and
// the command palette can all consume.
//
// moduleId gates an entry against the modules store. Entries without
// a moduleId stay visible unconditionally. The flat `nav` array is
// what callers should use for route resolution + module filtering;
// `sections` is the rendering shape.

export type NavItem = {
  href: string;
  label: string;
  icon: string;
  moduleId?: string;
};

export type NavSection = {
  id: string;
  label: string;
  items: NavItem[];
};

// Today sits above all groups (no header) because it's the always-on
// home — sections start where organisation begins to help.
export const today: NavItem = { href: '/', label: 'Today', icon: 'today' };

// Workspace sits next to Today as the second always-visible tier-0
// item. It's the granit "VSCode-for-life" surface — a named, tiled
// pane layout the user composes themselves. Promoted out of the
// section list so the user discovers it without scanning groups.
export const workspace: NavItem = { href: '/workspace', label: 'Workspaces', icon: 'workspace' };

// Tier-1 essentials — rendered above the sections with heavier visual
// weight (bigger text, primary tint, more padding). The user's daily-
// core workflow lives here; anything they touch every day deserves
// the most prominent rail. Items are still NavItems with the same
// route resolution — the difference is purely how NavSidebar renders
// them. Today is in the flat `today` constant (rendered separately
// even from this tier) so the home is the absolute first thing the
// eye lands on.
export const essentials: NavItem[] = [
  { href: '/tasks', label: 'Tasks', icon: 'tasks' },
  { href: '/calendar', label: 'Calendar', icon: 'calendar' },
  { href: '/notes', label: 'Notes', icon: 'notes' },
  { href: '/chat', label: 'Chat', icon: 'chat', moduleId: 'chat' },
  { href: '/habits', label: 'Habits', icon: 'habits', moduleId: 'habit_tracker' },
  { href: '/jots', label: 'Jots', icon: 'jots', moduleId: 'jots' },
  { href: '/morning', label: 'Morning', icon: 'morning', moduleId: 'morning' }
];

// Niche routes pulled out of the sidebar to reduce cognitive load —
// they remain fully functional via direct URL / command palette / AI
// agent navigation, but don't appear in the rail by default. Per user
// feedback: "examen, maintenance, weekly plan, review nicht so wichtig".
// Keep this list explicit so a future "show advanced" toggle could
// promote them back in one place.
const HIDDEN_NAV: NavItem[] = [
  { href: '/examen', label: 'Examen', icon: 'examen', moduleId: 'examen' },
  { href: '/plans/week', label: 'Weekly plan', icon: 'review' },
  { href: '/review', label: 'Review', icon: 'review', moduleId: 'weekly_review' },
  { href: '/review/maintenance', label: 'Maintenance', icon: 'wrench' }
];

// Section order is work-first: the planning + life pillars sit
// above reference/spiritual surfaces so a glance down the rail
// hits the user's most-actioned groups before the reference
// material. Habits used to be its own one-item "Daily" section
// — it's now folded into essentials since one item never
// justified its own header.
export const sections: NavSection[] = [
  {
    id: 'plan',
    label: 'Plan',
    items: [
      { href: '/vision', label: 'Vision', icon: 'vision', moduleId: 'vision' },
      { href: '/goals', label: 'Goals', icon: 'goals', moduleId: 'goals' },
      { href: '/deadlines', label: 'Deadlines', icon: 'deadline', moduleId: 'deadlines' },
      { href: '/projects', label: 'Projects', icon: 'projects', moduleId: 'projects' },
      { href: '/ventures', label: 'Ventures', icon: 'ventures', moduleId: 'ventures' }
    ]
  },
  {
    id: 'life',
    label: 'Life',
    items: [
      { href: '/finance', label: 'Finance', icon: 'finance', moduleId: 'finance' },
      { href: '/shopping', label: 'Shopping', icon: 'shopping', moduleId: 'shopping' },
      { href: '/hub', label: 'Hub', icon: 'hub', moduleId: 'hub' },
      { href: '/people', label: 'People', icon: 'people', moduleId: 'people' },
      { href: '/measurements', label: 'Metrics', icon: 'measurements', moduleId: 'measurements' }
    ]
  },
  {
    id: 'spiritual',
    label: 'Spiritual',
    items: [
      { href: '/scripture', label: 'Scripture', icon: 'scripture', moduleId: 'scripture' },
      { href: '/scripture/plans', label: 'Plans', icon: 'plans', moduleId: 'scripture' },
      { href: '/prayer', label: 'Prayer', icon: 'prayer', moduleId: 'prayer' },
      { href: '/roots', label: 'Roots', icon: 'roots', moduleId: 'roots' }
    ]
  },
  {
    id: 'knowledge',
    label: 'Knowledge',
    items: [
      { href: '/notes/graph', label: 'Graph', icon: 'graph' },
      { href: '/search', label: 'Search', icon: 'search' },
      { href: '/stats', label: 'Stats', icon: 'stats' },
      { href: '/books', label: 'Books', icon: 'books', moduleId: 'books' },
      { href: '/templates', label: 'Templates', icon: 'templates' },
      { href: '/objects', label: 'Objects', icon: 'objects', moduleId: 'objects' },
      { href: '/tags', label: 'Tags', icon: 'tags' }
    ]
  },
  {
    id: 'ai',
    label: 'AI',
    items: [
      { href: '/agents', label: 'Agents', icon: 'agents', moduleId: 'agents' }
    ]
  }
];

// Settings stays in the footer rail next to theme + sign-out, not as
// a section item — it's a meta destination.
export const settingsItem: NavItem = { href: '/settings', label: 'Settings', icon: 'settings' };

// Flat nav list — used for: route guard match, mobile back-to-section
// header, modules filter parity. Includes Today + essentials + every
// section item + the hidden-from-sidebar set + settings so route
// resolution still covers the full surface even when a route is
// invisible in the rail.
export const nav: NavItem[] = [
  today,
  workspace,
  ...essentials,
  ...sections.flatMap((s) => s.items),
  ...HIDDEN_NAV,
  settingsItem
];
