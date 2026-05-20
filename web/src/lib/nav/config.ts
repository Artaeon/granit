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

export const sections: NavSection[] = [
  {
    id: 'daily',
    label: 'Daily',
    items: [
      { href: '/morning', label: 'Morning', icon: 'morning', moduleId: 'morning' },
      { href: '/tasks', label: 'Tasks', icon: 'tasks' },
      { href: '/calendar', label: 'Calendar', icon: 'calendar' },
      { href: '/jots', label: 'Jots', icon: 'jots', moduleId: 'jots' },
      { href: '/habits', label: 'Habits', icon: 'habits', moduleId: 'habit_tracker' },
      { href: '/examen', label: 'Examen', icon: 'examen', moduleId: 'examen' }
    ]
  },
  {
    id: 'plan',
    label: 'Plan',
    items: [
      { href: '/vision', label: 'Vision', icon: 'vision', moduleId: 'vision' },
      { href: '/plans/week', label: 'Weekly plan', icon: 'review' },
      { href: '/review', label: 'Review', icon: 'review', moduleId: 'weekly_review' },
      { href: '/review/maintenance', label: 'Maintenance', icon: 'wrench' },
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
      { href: '/measurements', label: 'Metrics', icon: 'measurements', moduleId: 'measurements' },
      { href: '/prayer', label: 'Prayer', icon: 'prayer', moduleId: 'prayer' },
      { href: '/scripture', label: 'Scripture', icon: 'scripture', moduleId: 'scripture' },
      { href: '/scripture/plans', label: 'Plans', icon: 'plans', moduleId: 'scripture' },
      { href: '/roots', label: 'Roots', icon: 'roots', moduleId: 'roots' }
    ]
  },
  {
    id: 'knowledge',
    label: 'Knowledge',
    items: [
      { href: '/notes', label: 'Notes', icon: 'notes' },
      { href: '/notes/graph', label: 'Graph', icon: 'graph' },
      { href: '/search', label: 'Search', icon: 'search' },
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
      { href: '/agents', label: 'Agents', icon: 'agents', moduleId: 'agents' },
      { href: '/chat', label: 'Chat', icon: 'chat', moduleId: 'chat' }
    ]
  }
];

// Settings stays in the footer rail next to theme + sign-out, not as
// a section item — it's a meta destination.
export const settingsItem: NavItem = { href: '/settings', label: 'Settings', icon: 'settings' };

// Flat nav list — used for: route guard match, mobile back-to-section
// header, modules filter parity. Includes Today + every section item +
// settings so route resolution covers the full surface.
export const nav: NavItem[] = [today, ...sections.flatMap((s) => s.items), settingsItem];

