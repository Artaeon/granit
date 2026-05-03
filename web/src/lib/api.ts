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

export interface Jot {
  date: string;
  path: string;
  title: string;
  modTime: string;
  size: number;
  frontmatter?: Record<string, unknown>;
  body: string;
  openTasks: number;
}

export interface JotsResponse {
  jots: Jot[];
  nextBefore: string | null;
  hasMore: boolean;
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

export type CalendarEventType =
  | 'daily'
  | 'task_due'
  | 'task_scheduled'
  | 'event'
  | 'ics_event'
  | 'deadline';

export interface CalendarEvent {
  type: CalendarEventType;
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
  /** ICS filename for ics_event types — drives per-source coloring. */
  source?: string;
  /** Deadline importance — set only on type='deadline' (critical/high/normal). */
  importance?: 'critical' | 'high' | 'normal';
}

// Mirrors internal/deadlines.Deadline — top-level "this matters by date X"
// markers stored in .granit/deadlines.json. Linkable to a goal, project,
// and/or any number of tasks (loose foreign keys, no FK enforcement).
export type DeadlineImportance = 'critical' | 'high' | 'normal';
export type DeadlineStatus = 'active' | 'missed' | 'met' | 'cancelled';

export interface Deadline {
  id: string;
  title: string;
  date: string; // YYYY-MM-DD
  description?: string;
  goal_id?: string;
  project?: string;
  task_ids?: string[];
  importance: DeadlineImportance;
  status: DeadlineStatus;
  created_at: string;
  updated_at: string;
}

export type DeadlineCreate = {
  title: string;
  date: string;
  description?: string;
  goal_id?: string;
  project?: string;
  task_ids?: string[];
  importance?: DeadlineImportance;
  status?: DeadlineStatus;
};

export type DeadlinePatch = Partial<DeadlineCreate>;

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
  due_date?: string;
  completed_at?: string;
}

export interface GoalReview {
  date: string;
  note: string;
  progress: number;
}

// Mirrors internal/goals.Goal — the canonical .granit/goals.json schema.
// Status uses the TUI's value space ('completed', not 'done'); the web
// previously rendered a 'done' chip that never matched anything.
export interface Goal {
  id: string;
  title: string;
  description?: string;
  status?: 'active' | 'paused' | 'completed' | 'archived';
  category?: string;
  color?: string;
  tags?: string[];
  target_date?: string;
  created_at?: string;
  updated_at?: string;
  completed_at?: string;
  project?: string;
  milestones?: Milestone[] | null;
  notes?: string;
  review_frequency?: 'weekly' | 'monthly' | 'quarterly' | string;
  last_reviewed?: string;
  review_log?: GoalReview[];
}

export interface CalendarFeed {
  from: string;
  to: string;
  events: CalendarEvent[];
}

// (Deadline canonical type defined above near CalendarEvent.)

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

export interface TimerEntry {
  note_path: string;
  task_text: string;
  task_id?: string;
  start_time: string;
  end_time: string;
  duration: number; // nanoseconds (Go's time.Duration JSON encoding)
  date: string;
}

export interface ActiveTimer {
  notePath: string;
  taskText: string;
  taskId?: string;
  startTime: string;
  elapsedSec?: number;
}

export interface RecurringTask {
  text: string;
  frequency: 'daily' | 'weekly' | 'monthly';
  day_of_week?: number;  // 0-6, used for weekly
  day_of_month?: number; // 1-31, used for monthly
  last_created?: string; // YYYY-MM-DD
  enabled: boolean;
}

// OpenAI model option as exposed by /api/v1/config/openai-models. The
// settings page renders these as dropdown choices with prices in
// the label so the user picks knowingly.
export interface OpenAIModelOption {
  id: string;
  family: string;
  input_per_m: string;
  output_per_m: string;
  note?: string;
  recommended?: boolean;
}

export interface AppConfig {
  ai_provider: string;
  openai_model: string;
  openai_key_set: boolean;
  anthropic_model: string;
  anthropic_key_set: boolean;
  ollama_url: string;
  ollama_model: string;
  ai_auto_apply_edits: boolean;
  auto_tag: boolean;
  ghost_writer: boolean;
  background_bots: boolean;
  semantic_search_enabled: boolean;
  daily_notes_folder: string;
  daily_note_template: string;
  daily_recurring_tasks: string[];
  weekly_notes_folder: string;
  weekly_note_template: string;
  auto_daily_note: boolean;
  theme: string;
  auto_dark_mode: boolean;
  dark_theme: string;
  light_theme: string;
  line_numbers: boolean;
  word_wrap: boolean;
  auto_save: boolean;
  editor_tab_size: number;
  editor_insert_tabs: boolean;
  editor_auto_indent: boolean;
  auto_close_brackets: boolean;
  highlight_current_line: boolean;
  task_filter_mode: string;
  task_required_tags: string[];
  task_exclude_folders: string[];
  task_exclude_done: boolean;
  search_content_by_default: boolean;
  max_search_results: number;
  git_auto_sync: boolean;
  pomodoro_goal: number;
  // Kanban (read-only mirrors of config — the web's kanban view honors these).
  // Server always returns the keys (possibly null) so the union type is
  // safe to consume without an existence check.
  kanban_columns: string[] | null;
  kanban_column_tags: Record<string, string> | null;
  kanban_column_wip: Record<string, number> | null;
}

// Patch shape: every field optional, plus opaque-set fields (api keys)
// take a string (empty string clears) rather than a bool.
export type AppConfigPatch = Partial<{
  ai_provider: string;
  openai_key: string; // "" to clear, anything else to set
  openai_model: string;
  anthropic_key: string;
  anthropic_model: string;
  ollama_url: string;
  ollama_model: string;
  ai_auto_apply_edits: boolean;
  auto_tag: boolean;
  ghost_writer: boolean;
  background_bots: boolean;
  semantic_search_enabled: boolean;
  daily_notes_folder: string;
  daily_note_template: string;
  daily_recurring_tasks: string[];
  weekly_notes_folder: string;
  weekly_note_template: string;
  auto_daily_note: boolean;
  theme: string;
  auto_dark_mode: boolean;
  dark_theme: string;
  light_theme: string;
  line_numbers: boolean;
  word_wrap: boolean;
  auto_save: boolean;
  editor_tab_size: number;
  editor_insert_tabs: boolean;
  editor_auto_indent: boolean;
  auto_close_brackets: boolean;
  highlight_current_line: boolean;
  task_filter_mode: string;
  task_required_tags: string[];
  task_exclude_folders: string[];
  task_exclude_done: boolean;
  search_content_by_default: boolean;
  max_search_results: number;
  git_auto_sync: boolean;
  pomodoro_goal: number;
  kanban_columns: string[];
  kanban_column_tags: Record<string, string>;
  kanban_column_wip: Record<string, number>;
}>;

export interface ChatMessage {
  role: 'user' | 'assistant' | 'system';
  content: string;
}

export interface Scripture {
  text: string;
  source?: string;
}

// Bible — bundled World English Bible (PD). Mirrors
// internal/scripture/bible types.
export interface BibleVerse {
  n: number;
  text: string;
}
export interface BibleBookSummary {
  code: string; // USFM, e.g. "JHN"
  name: string;
  testament: 'OT' | 'NT';
  chapters: number;
}
export interface BiblePassage {
  book: string;
  bookCode: string;
  chapter: number;
  startV: number;
  endV: number;
  reference: string; // "Proverbs 3:5-8"
  verses: BibleVerse[];
}
export interface BibleSearchHit {
  book: string;
  bookCode: string;
  chapter: number;
  verse: number;
  text: string;
  reference: string;
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
  // Hard-delete a note. Server emits a note.removed WS event; pages
  // subscribe and refresh. No undo / trash folder yet.
  deleteNote: (path: string) =>
    req<void>(`/notes/${path.split('/').map(encodeURIComponent).join('/')}`, { method: 'DELETE' }),
  // Rename / move a note. Returns { from, to } on success, 409 if
  // the destination already exists.
  renameNote: (from: string, to: string) =>
    req<{ from: string; to: string }>('/notes/rename', {
      method: 'POST',
      body: JSON.stringify({ from, to })
    }),

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
  listJots: (params: { before?: string; limit?: number } = {}) => {
    const qs = new URLSearchParams();
    if (params.before) qs.set('before', params.before);
    if (params.limit !== undefined) qs.set('limit', String(params.limit));
    const suffix = qs.toString() ? `?${qs}` : '';
    return req<JotsResponse>(`/jots${suffix}`);
  },
  dailyContext: () =>
    req<{
      date: string;
      carryover: { id: string; text: string; priority?: number; dueDate?: string; notePath: string }[];
      habits: { text: string; done: boolean }[];
    }>('/daily/context'),

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

  // Goals (granit, full CRUD — schema mirrors internal/goals.Goal)
  listGoals: () => req<{ goals: Goal[]; total: number }>('/goals'),
  createGoal: (g: Partial<Goal>) =>
    req<Goal>('/goals', { method: 'POST', body: JSON.stringify(g) }),
  patchGoal: (id: string, g: Partial<Goal>) =>
    req<Goal>(`/goals/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      body: JSON.stringify(g)
    }),
  deleteGoal: (id: string) =>
    req<void>(`/goals/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  addGoalMilestone: (id: string, m: { text: string; due_date?: string; done?: boolean }) =>
    req<Goal>(`/goals/${encodeURIComponent(id)}/milestones`, {
      method: 'POST',
      body: JSON.stringify(m)
    }),
  patchGoalMilestone: (
    id: string,
    idx: number,
    m: { text?: string; due_date?: string; done?: boolean }
  ) =>
    req<Goal>(`/goals/${encodeURIComponent(id)}/milestones/${idx}`, {
      method: 'PATCH',
      body: JSON.stringify(m)
    }),
  deleteGoalMilestone: (id: string, idx: number) =>
    req<Goal>(`/goals/${encodeURIComponent(id)}/milestones/${idx}`, { method: 'DELETE' }),
  logGoalReview: (id: string, note: string, opts?: { date?: string; progress?: number }) =>
    req<Goal>(`/goals/${encodeURIComponent(id)}/review`, {
      method: 'POST',
      body: JSON.stringify({ note, ...(opts ?? {}) })
    }),

  // Deadlines — top-level dated markers stored in
  // .granit/deadlines.json. listDeadlines is the canonical full-CRUD
  // endpoint; tryListDeadlines is the defensive variant kept for the
  // dashboard widget so a transient 404 / network blip can't tank the
  // home page.
  listDeadlines: () => req<{ deadlines: Deadline[]; total: number }>('/deadlines'),
  getDeadline: (id: string) => req<Deadline>(`/deadlines/${encodeURIComponent(id)}`),
  createDeadline: (d: DeadlineCreate) =>
    req<Deadline>('/deadlines', { method: 'POST', body: JSON.stringify(d) }),
  patchDeadline: (id: string, d: DeadlinePatch) =>
    req<Deadline>(`/deadlines/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      body: JSON.stringify(d)
    }),
  deleteDeadline: (id: string) =>
    req<void>(`/deadlines/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  tryListDeadlines: async (): Promise<Deadline[] | null> => {
    try {
      const r = await req<{ deadlines?: Deadline[]; items?: Deadline[]; total?: number }>('/deadlines');
      const list = r.deadlines ?? r.items ?? null;
      if (!Array.isArray(list)) return null;
      return list.filter((d) => d && typeof d.date === 'string' && d.date.length >= 10);
    } catch {
      return null;
    }
  },

  // Agents
  listAgentPresets: (includePrompt = false) =>
    req<{ presets: AgentPreset[]; total: number }>(
      `/agents/presets${includePrompt ? '?include=prompt' : ''}`
    ),
  listAgentRuns: (limit = 100) =>
    req<{ runs: AgentRun[]; total: number; stats: Record<string, Record<string, number>> }>(
      `/agents/runs?limit=${limit}`
    ),
  runAgent: (
    preset: string,
    goal: string,
    opts?: { maxSteps?: number; budgetMicroCents?: number }
  ) =>
    req<{ runId: string; preset: string }>('/agents/run', {
      method: 'POST',
      body: JSON.stringify({
        preset,
        goal,
        ...(opts?.maxSteps ? { maxSteps: opts.maxSteps } : {}),
        ...(opts?.budgetMicroCents ? { budgetMicroCents: opts.budgetMicroCents } : {})
      })
    }),

  // Plan-day-schedule: synchronous wrapper around the plan-my-day
  // preset that ALSO parses the resulting `## Plan` block in today's
  // daily note and writes scheduledStart/durationMinutes back to each
  // matched task. Returns scheduled + unmatched lists so the UI can
  // toast "Scheduled N of M tasks". Long-running (the LLM call is the
  // critical path); callers should show a busy state.
  runPlanDaySchedule: () =>
    req<{
      runId: string;
      scheduled: { taskId: string; start: string }[];
      unmatched: string[];
    }>('/agents/plan-day-schedule', { method: 'POST' }),

  // Chat
  chat: (messages: ChatMessage[], notePath?: string) =>
    req<{ message: ChatMessage }>('/chat', {
      method: 'POST',
      body: JSON.stringify({ messages, notePath })
    }),

  // Scripture / devotional
  listScriptures: () => req<{ scriptures: Scripture[]; total: number }>('/scripture'),
  todayScripture: () => req<Scripture>('/scripture/today'),
  randomScripture: () => req<Scripture>('/scripture/random'),
  createDevotional: (body: { verse: string; source?: string; reflection?: string }) =>
    req<{ path: string; title: string }>('/devotionals', {
      method: 'POST',
      body: JSON.stringify(body)
    }),

  // Bible — embedded World English Bible (public domain). The reader
  // page hits these to drive the book picker + random-passage button.
  bibleBooks: () =>
    req<{
      books: BibleBookSummary[];
      meta: { name: string; abbreviation: string; license: string };
    }>('/bible/books'),
  bibleChapter: (book: string, chapter: number) =>
    req<{
      book: string;
      bookCode: string;
      testament: 'OT' | 'NT';
      chapter: number;
      verses: BibleVerse[];
      chapters: number;
    }>(`/bible/${encodeURIComponent(book)}/${chapter}`),
  bibleRandom: (opts: { length?: number; book?: string; testament?: 'OT' | 'NT' } = {}) => {
    const params = new URLSearchParams();
    if (opts.length) params.set('length', String(opts.length));
    if (opts.book) params.set('book', opts.book);
    if (opts.testament) params.set('testament', opts.testament);
    const q = params.toString();
    return req<BiblePassage>(`/bible/random${q ? `?${q}` : ''}`);
  },
  bibleSearch: (query: string, limit = 50) =>
    req<{ hits: BibleSearchHit[]; total: number; query: string }>(
      `/bible/search?q=${encodeURIComponent(query)}&limit=${limit}`
    ),

  // Config (web ↔ TUI shared config.json)
  getConfig: () => req<AppConfig>('/config'),
  patchConfig: (patch: Partial<AppConfigPatch>) =>
    req<AppConfig>('/config', { method: 'PATCH', body: JSON.stringify(patch) }),
  listOpenAIModels: () =>
    req<{ models: OpenAIModelOption[] }>('/config/openai-models'),

  // Time tracking
  listTimetracker: () =>
    req<{
      entries: TimerEntry[];
      total: number;
      active: ActiveTimer | null;
      minutesByTaskId: Record<string, number>;
      minutesToday: number;
    }>('/timetracker'),
  clockIn: (body: { notePath?: string; taskText?: string; taskId?: string }) =>
    req<ActiveTimer>('/timetracker/start', { method: 'POST', body: JSON.stringify(body) }),
  clockOut: () => req<{ taskId: string; taskText: string; minutes: number; endTime: string }>('/timetracker/stop', { method: 'POST' }),

  // Recurring tasks (granit's daily/weekly/monthly auto-creator)
  listRecurring: () => req<{ rules: RecurringTask[]; total: number }>('/recurring'),
  putRecurring: (rules: RecurringTask[]) =>
    req<{ rules: RecurringTask[]; total: number }>('/recurring', {
      method: 'PUT',
      body: JSON.stringify({ rules })
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
  // Mark a habit done/undone for ANY date — used by the heatmap to
  // let users retro-fix yesterday's missed log without opening the
  // daily note. Server creates the daily file if it doesn't exist
  // for that date.
  toggleHabit: (name: string, date: string, done: boolean) =>
    req<{ name: string; date: string; done: boolean; path: string }>(
      '/habits/toggle',
      { method: 'POST', body: JSON.stringify({ name, date, done }) }
    ),

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
  | 'streaks'
  | 'scripture'
  | 'today-focus'
  | 'top-deadlines';

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
