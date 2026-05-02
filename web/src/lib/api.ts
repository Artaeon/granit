// Minimal API client. Persists the bearer token in localStorage.

const TOKEN_KEY = 'everything.token';

export function getToken(): string | null {
  if (typeof localStorage === 'undefined') return null;
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(tok: string): void {
  localStorage.setItem(TOKEN_KEY, tok);
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY);
}

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

export async function req<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  const tok = getToken();
  if (tok) headers.set('Authorization', `Bearer ${tok}`);
  if (init.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }
  const res = await fetch(`/api/v1${path}`, { ...init, headers });
  if (!res.ok) {
    let msg = res.statusText;
    try {
      const body = await res.json();
      if (body?.error) msg = body.error;
    } catch {}
    throw new ApiError(res.status, msg);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

// ---- types ----

export interface Note {
  path: string;
  title: string;
  modTime: string;
  size: number;
  frontmatter?: Record<string, unknown>;
  body?: string;
  links?: string[];
  tags?: string[];
}

export interface NoteList {
  notes: Note[];
  total: number;
  limit: number;
  offset: number;
}

export interface VaultInfo {
  root: string;
  notes: number;
}

export interface Task {
  id: string;
  notePath: string;
  lineNum: number;
  text: string;
  done: boolean;
  priority: number;
  tags?: string[];
  dueDate?: string;
  scheduledStart?: string;
  durationMinutes?: number;
  estimatedMinutes?: number;
  dependsOn?: string[];
  projectId?: string;
  createdAt?: string;
  completedAt?: string;
  updatedAt?: string;
  snoozedUntil?: string; // YYYY-MM-DDThh:mm
  indent?: number;
  parentLine?: number;
  recurrence?: string; // "daily" | "weekly" | "monthly" | "3x-week" | ""
  notes?: string;      // free-form sidecar metadata
  // Granit-sourced (read-only):
  granitId?: string;
  triage?: 'inbox' | 'triaged' | 'scheduled' | 'done' | 'dropped' | 'snoozed';
  granitOrigin?: string;
}

export interface TaskList {
  tasks: Task[];
  total: number;
}

export type CalendarEventType = 'daily' | 'task_due' | 'task_scheduled' | 'event' | 'ics_event';

export interface CalendarEvent {
  type: 'daily' | 'task_due' | 'task_scheduled' | 'event' | 'ics_event';
  date?: string; // YYYY-MM-DD (all-day events)
  start?: string; // RFC3339
  end?: string; // RFC3339
  title: string;
  notePath?: string;
  taskId?: string;
  eventId?: string;
  done?: boolean;
  priority?: number;
  durationMinutes?: number;
  color?: string;
  location?: string;
}

export interface ProjectMilestone {
  text: string;
  done: boolean;
}

export interface ProjectGoal {
  title: string;
  done: boolean;
  milestones?: ProjectMilestone[];
}

export interface Project {
  name: string;
  description?: string;
  folder?: string;
  tags?: string[];
  status?: string;
  color?: string;
  created_at?: string;
  updated_at?: string;
  notes?: string[];
  task_filter?: string;
  category?: string;
  goals?: ProjectGoal[];
  next_action?: string;
  priority?: number;
  due_date?: string;
  time_spent?: number;
  // Server-decorated:
  progress?: number;
  tasksDone?: number;
  tasksTotal?: number;
}

export interface CalendarEventEntry {
  id: string;
  title: string;
  date: string;
  start_time?: string;
  end_time?: string;
  location?: string;
  color?: string;
  created_at?: string;
}

export interface Milestone {
  text: string;
  done: boolean;
}

export interface Goal {
  id: string;
  title: string;
  description?: string;
  status?: string;
  category?: string;
  tags?: string[];
  target_date?: string;
  created_at?: string;
  updated_at?: string;
  project?: string;
  milestones?: Milestone[] | null;
}

export interface CalendarFeed {
  from: string;
  to: string;
  events: CalendarEvent[];
}

export interface AgentPreset {
  id: string;
  name: string;
  description: string;
  tools: string[];
  includeWrite: boolean;
  systemPrompt?: string;
  source: 'builtin' | 'vault';
}

export interface AgentRun {
  path: string;
  title: string;
  preset: string;
  goal: string;
  status: string;
  started: string;
  steps: number;
  model?: string;
}

export interface Device {
  id: string;
  label?: string;
  createdAt: string;
  lastUsed: string;
  current: boolean;
}

export interface CalendarSource {
  id: string;
  source: string; // filename
  path: string;   // absolute
  folder: string; // vault-relative parent
  enabled: boolean;
}

// ---- endpoints ----

export const api = {
  req,
  health: () => req<{ status: string }>('/health'),
  vault: () => req<VaultInfo>('/vault'),

  // Auth
  authStatus: () => req<{ hasPassword: boolean; sessionCount?: number; setupAt?: string }>('/auth/status'),
  authSetup: (password: string, label?: string) =>
    req<{ token: string }>('/auth/setup', { method: 'POST', body: JSON.stringify({ password, label }) }),
  authLogin: (password: string, label?: string) =>
    req<{ token: string }>('/auth/login', { method: 'POST', body: JSON.stringify({ password, label }) }),
  authLogout: () => req<void>('/auth/logout', { method: 'POST' }),
  authChangePassword: (oldPassword: string, newPassword: string) =>
    req<void>('/auth/change-password', {
      method: 'POST',
      body: JSON.stringify({ old: oldPassword, new: newPassword })
    }),
  authRevokeAll: () => req<void>('/auth/revoke-all', { method: 'POST' }),
  listNotes: (params: { type?: string; tag?: string; folder?: string; q?: string; limit?: number; offset?: number } = {}) => {
    const qs = new URLSearchParams();
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined && v !== '') qs.set(k, String(v));
    }
    const suffix = qs.toString() ? `?${qs}` : '';
    return req<NoteList>(`/notes${suffix}`);
  },
  getNote: (path: string) => req<Note>(`/notes/${encodeURI(path)}`),
  putNote: (path: string, body: { frontmatter?: Record<string, unknown>; body: string }, etag?: string) => {
    const headers: Record<string, string> = {};
    if (etag) headers['If-Match'] = etag;
    return req<Note>(`/notes/${encodeURI(path)}`, {
      method: 'PUT',
      body: JSON.stringify(body),
      headers
    });
  },
  createNote: (body: { path: string; frontmatter?: Record<string, unknown>; body: string }) =>
    req<Note>('/notes', { method: 'POST', body: JSON.stringify(body) }),

  // Tasks
  listTasks: (params: { status?: 'open' | 'done'; tag?: string; due_on?: string; due_before?: string; note?: string; triage?: string } = {}) => {
    const qs = new URLSearchParams();
    for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== '') qs.set(k, String(v));
    return req<TaskList>(`/tasks${qs.toString() ? '?' + qs : ''}`);
  },
  patchTask: (
    id: string,
    patch: Partial<Pick<Task, 'done' | 'priority' | 'dueDate' | 'text' | 'scheduledStart' | 'durationMinutes' | 'projectId' | 'snoozedUntil' | 'recurrence' | 'notes'>> & {
      triage?: 'inbox' | 'triaged' | 'scheduled' | 'done' | 'dropped' | 'snoozed';
      clearSchedule?: boolean;
    }
  ) => req<Task>(`/tasks/${id}`, { method: 'PATCH', body: JSON.stringify(patch) }),
  createTask: (body: {
    notePath: string;
    text: string;
    priority?: number;
    dueDate?: string;
    tags?: string[];
    section?: string;
    scheduledStart?: string;
    durationMinutes?: number;
  }) => req<Task>('/tasks', { method: 'POST', body: JSON.stringify(body) }),

  // Daily
  daily: (date: string = 'today') => req<Note>(`/daily/${date}`),

  // Calendar
  calendar: (from: string, to: string) =>
    req<CalendarFeed>(`/calendar?from=${from}&to=${to}`),
  listCalendarSources: () =>
    req<{ sources: CalendarSource[]; disabled: string[]; total: number }>('/calendar/sources'),
  patchCalendarSources: (disabled: string[]) =>
    req<{ sources: CalendarSource[]; disabled: string[]; total: number }>('/calendar/sources', {
      method: 'PATCH',
      body: JSON.stringify({ disabled })
    }),

  // Projects
  listProjects: () => req<{ projects: Project[]; total: number }>('/projects'),
  getProject: (name: string) => req<Project>(`/projects/${encodeURIComponent(name)}`),
  createProject: (p: Partial<Project>) =>
    req<Project>('/projects', { method: 'POST', body: JSON.stringify(p) }),
  patchProject: (name: string, p: Partial<Project>) =>
    req<Project>(`/projects/${encodeURIComponent(name)}`, {
      method: 'PATCH',
      body: JSON.stringify(p)
    }),
  deleteProject: (name: string) =>
    req<void>(`/projects/${encodeURIComponent(name)}`, { method: 'DELETE' }),

  // Calendar events (events.json)
  listEvents: () => req<{ events: CalendarEventEntry[]; total: number }>('/events'),
  createEvent: (ev: Partial<CalendarEventEntry>) =>
    req<CalendarEventEntry>('/events', { method: 'POST', body: JSON.stringify(ev) }),
  patchEvent: (id: string, ev: Partial<CalendarEventEntry>) =>
    req<CalendarEventEntry>(`/events/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      body: JSON.stringify(ev)
    }),
  deleteEvent: (id: string) =>
    req<void>(`/events/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  // Goals (granit, read-only)
  listGoals: () => req<{ goals: Goal[]; total: number }>('/goals'),

  // Agents
  listAgentPresets: (includePrompt = false) =>
    req<{ presets: AgentPreset[]; total: number }>(
      `/agents/presets${includePrompt ? '?include=prompt' : ''}`
    ),
  listAgentRuns: (limit = 100) =>
    req<{ runs: AgentRun[]; total: number; stats: Record<string, Record<string, number>> }>(
      `/agents/runs?limit=${limit}`
    ),
  runAgent: (preset: string, goal: string) =>
    req<{ runId: string; preset: string }>('/agents/run', {
      method: 'POST',
      body: JSON.stringify({ preset, goal })
    }),

  // Devices (active sessions)
  listDevices: () => req<{ devices: Device[]; total: number }>('/devices'),
  revokeDevice: (id: string) =>
    req<void>(`/devices/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  // Object types (granit registry: built-ins + vault)
  listTypes: () => req<{ types: ObjectType[]; total: number; untyped: number }>('/types'),
  listTypeObjects: (id: string) =>
    req<{ typeId: string; objects: ObjectInstance[]; total: number }>(
      `/types/${encodeURIComponent(id)}/objects`
    ),

  // Tags
  listTags: () => req<{ tags: { tag: string; count: number }[]; total: number }>('/tags'),

  // Pinned notes (granit-compatible — .granit/sidebar-pinned.json)
  listPinned: () => req<{ pinned: { path: string; title: string }[]; total: number }>('/pinned'),
  setPinned: (path: string, pinned: boolean) =>
    req<{ pinned: { path: string; title: string }[]; total: number }>('/pinned', {
      method: 'PATCH',
      body: JSON.stringify({ path, pinned })
    }),

  // Habits (derived from `## Habits` sections in daily notes)
  listHabits: () => req<HabitsResponse>('/habits'),

  // Morning routine — saves a `## Daily Plan` block into today's daily note
  saveMorning: (body: {
    scripture?: { text: string; source: string };
    goal?: string;
    tasks?: string[];
    habits?: string[];
    thoughts?: string;
  }) => req<{ path: string; saved: boolean }>('/morning/save', { method: 'POST', body: JSON.stringify(body) }),

  // Full-text search across vault content (uses granit's TF-IDF SearchIndex)
  search: (q: string, limit = 30) =>
    req<{ results: SearchHit[]; total: number; q: string; ready: boolean }>(
      `/search?q=${encodeURIComponent(q)}&limit=${limit}`
    ),

  // Templates (built-in + vault user templates)
  listTemplates: () => req<{ templates: NoteTemplate[]; total: number }>('/templates'),
  createFromTemplate: (body: { templateName: string; path: string; title?: string }) =>
    req<{ path: string; title: string }>('/notes/from-template', {
      method: 'POST',
      body: JSON.stringify(body)
    }),

  // Vault stats
  stats: () => req<VaultStats>('/stats'),

  // Dashboard config (read/write)
  getDashboard: () => req<DashboardConfig>('/dashboard'),
  putDashboard: (cfg: DashboardConfig) => req<DashboardConfig>('/dashboard', { method: 'PUT', body: JSON.stringify(cfg) })
};

export interface ObjectTypeProperty {
  name: string;
  kind: 'text' | 'number' | 'date' | 'url' | 'tag' | 'checkbox' | 'link' | 'select';
  required?: boolean;
  description?: string;
  options?: string[];
  default?: string;
}

export interface ObjectType {
  id: string;
  name: string;
  icon?: string;
  description?: string;
  folder?: string;
  count: number;
  properties?: ObjectTypeProperty[];
}

export interface ObjectInstance {
  path: string;
  title: string;
  properties?: Record<string, string>;
}

export interface HabitDay {
  date: string;
  done: boolean;
}

export interface HabitInfo {
  name: string;
  days: HabitDay[];
  currentStreak: number;
  longestStreak: number;
  last7Pct: number;
  last30Pct: number;
  doneToday: boolean;
  notePathToday?: string;
  taskIdToday?: string;
}

export interface HabitsResponse {
  habits: HabitInfo[];
  total: number;
  today: string;
  days: number;
}

export interface SearchHit {
  path: string;
  title: string;
  line: number;
  column: number;
  matchLine: string;
  score: number;
}

export interface NoteTemplate {
  name: string;
  content: string;
  isUser?: boolean;
}

export interface StatEntry {
  name: string;
  value: number;
}

export interface VaultStats {
  noteCount: number;
  totalWords: number;
  totalLinks: number;
  totalTags: number;
  untypedNotes: number;
  orphanNotes: number;
  averageWords: number;
  notesPerMonth: StatEntry[];
  topTags: StatEntry[];
  topLinkedNotes: StatEntry[];
  largestNotes: StatEntry[];
  recentlyEdited: StatEntry[];
}

export type DashboardWidgetType =
  | 'greeting'
  | 'pinned'
  | 'daily-note'
  | 'quick-capture'
  | 'today-tasks'
  | 'scheduled-today'
  | 'goals-progress'
  | 'recent-notes'
  | 'projects-active'
  | 'inbox'
  | 'calendar-week'
  | 'install'
  | 'habits'
  | 'pomodoro'
  | 'now'
  | 'streaks';

export interface DashboardWidget {
  id: string;
  type: DashboardWidgetType;
  enabled: boolean;
  config?: Record<string, unknown>;
}

export interface DashboardConfig {
  version: number;
  widgets: DashboardWidget[];
}

// ---- helpers ----

export function todayISO(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

export function fmtDateISO(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}
