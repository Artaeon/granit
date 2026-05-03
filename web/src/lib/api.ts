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
  // Loose foreign keys to top-level entities. goalId is a Gxxx id
  // matching internal/goals.Goal.ID; deadlineId is a 26-char ULID
  // matching internal/deadlines.Deadline.ID. Round-trip through the
  // markdown line via `goal:Gxxx` / `deadline:<ulid>` markers.
  goalId?: string;
  deadlineId?: string;
}

export interface TaskList {
  tasks: Task[];
  total: number;
}

// Plan-my-day preview/apply pair. The dry-run endpoint returns
// PlanProposal[]; the drawer lets the user edit each row's start +
// duration + keep/skip and posts the survivors as PlanApplyProposal[].
//
// PlanLine is the verbatim markdown line the LLM emitted (e.g.
// "- 09:00–09:30 — review PR"); the drawer surfaces it in a tooltip
// so the user can see *why* a slot exists without leaving the panel.
//
// Reason is the matched plan-line text without the time prefix —
// useful when fuzzyMatch picks an unexpected task and the user wants
// to see what the model actually said.
export interface PlanProposal {
  taskId: string;
  taskText: string;
  start: string; // RFC3339
  durationMinutes: number;
  planLine: string;
  reason: string;
}

export interface PlanApplyProposal {
  taskId: string;
  start: string; // RFC3339
  durationMinutes: number;
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

// Saved bible passage with optional note. Persisted to
// .granit/bible-bookmarks.json so the TUI can read the same set.
export interface BibleBookmark {
  id: string;
  bookCode: string;
  book: string;
  chapter: number;
  verseFrom: number;
  verseTo: number;
  reference: string; // server-rendered, e.g. "John 3:16-17"
  text: string;      // snapshot at save time
  note?: string;
  created_at: string;
  updated_at: string;
}

export interface Device {
  id: string;
  label?: string;
  createdAt: string;
  lastUsed: string;
  current: boolean;
}

// Finance — accounts (net worth), subscriptions (recurring drag),
// income streams (active + planned), and money goals. Money is
// integer cents (int64 server-side, number client) to dodge float
// drift. See internal/finance/finance.go for the canonical schema;
// field names here match the JSON tags 1:1.

export type FinAccountKind = 'cash' | 'checking' | 'savings' | 'credit' | 'investment' | 'loan';
export type FinSubCadence = 'monthly' | 'yearly' | 'weekly' | 'quarterly';
export type FinGoalKind = 'savings' | 'payoff' | 'networth';
export type FinIncomeStatus = 'idea' | 'planned' | 'active' | 'paused';
export type FinIncomeKind = 'employment' | 'freelance' | 'business' | 'investment' | 'royalty' | 'other';

export interface FinAccount {
  id: string;
  name: string;
  kind: FinAccountKind | string;
  currency: string;
  balance_cents: number;
  as_of?: string;
  // Optional polish fields — institution name (bank/issuer), color
  // palette key for the row pip, freeform tags. The UI treats any
  // string as a valid color (CSS var lookup with sensible fallback).
  institution?: string;
  color?: string;
  tags?: string[];
  notes?: string;
  archived?: boolean;
  created_at: string;
  updated_at: string;
}

export interface FinSubscription {
  id: string;
  name: string;
  amount_cents: number;
  currency: string;
  cadence: FinSubCadence | string;
  next_renewal: string;
  account_id?: string;
  project?: string; // Project.Name
  tags?: string[];
  category?: string;
  url?: string;
  notes?: string;
  active: boolean;
  created_at: string;
  updated_at: string;
}

export interface FinIncomeStream {
  id: string;
  name: string;
  status: FinIncomeStatus | string;
  kind: FinIncomeKind | string;
  projected_monthly_cents: number;
  actual_monthly_cents: number;
  currency: string;
  // Concrete payout schedule. payout_day_of_month is 1-31 (0/missing
  // = unknown, no projection); payout_cadence defaults to monthly
  // when empty. Drives the cashflow timeline's income lines.
  payout_day_of_month?: number;
  payout_cadence?: FinSubCadence | string;
  account_id?: string;     // where the money lands
  project?: string;        // Project.Name — for ventures + career-tagged jobs
  tags?: string[];
  url?: string;
  started_at?: string;
  notes?: string;
  created_at: string;
  updated_at: string;
}

export interface FinGoal {
  id: string;
  name: string;
  kind: FinGoalKind | string;
  target_cents: number;
  current_cents: number;
  currency: string;
  target_date?: string;
  linked_account_id?: string;
  notes?: string;
  status?: string;
  created_at: string;
  updated_at: string;
}

export interface FinOverview {
  currency: string;
  net_worth_cents: number;
  assets_cents: number;
  liabilities_cents: number;
  income_monthly_actual_cents: number;
  income_monthly_projected_cents: number;
  subscription_monthly_cents: number;
  upcoming_subs_count: number;
  accounts_count: number;
  income_active_count: number;
  income_pipeline_count: number;
  goals_active_count: number;
}

// Prayer intentions — active prayer list with status lifecycle.
export type PrayerStatus = 'praying' | 'answered' | 'archived';
export interface PrayerIntention {
  id: string;
  text: string;
  category?: string;
  status: PrayerStatus | string;
  started_at?: string;
  answered_at?: string;
  answer?: string;
  notes?: string;
  created_at: string;
  updated_at: string;
}

// People — lightweight CRM. The list response also carries derived
// upcoming-birthday + stale-count fields so the page hydrates in
// one round trip.
export interface Person {
  id: string;
  name: string;
  email?: string;
  phone?: string;
  birthday?: string;
  relationship?: string;
  tags?: string[];
  last_contacted_at?: string;
  cadence_days?: number;
  note_path?: string;
  notes?: string;
  archived?: boolean;
  created_at: string;
  updated_at: string;
}

// Measurements — numeric tracking. Two concepts: Series (definition)
// + Entry (logged values).
export interface MeasurementSeries {
  id: string;
  name: string;
  unit: string;
  target?: number;
  direction?: 'up' | 'down' | string;
  notes?: string;
  archived?: boolean;
  created_at: string;
  updated_at: string;
}
export interface MeasurementEntry {
  id: string;
  series_id: string;
  date: string;
  value: number;
  notes?: string;
  created_at: string;
}

export interface CalendarSource {
  id: string;
  source: string; // filename
  path: string;   // absolute
  folder: string; // vault-relative parent
  enabled: boolean;
  /** True iff source is under <vault>/calendars/ — gate edits / new-event UI on this. */
  writable: boolean;
}

// Wire shape for ICS event create/patch responses. Times are RFC3339 for
// timed events, YYYY-MM-DD when allDay is true.
export interface ICSEvent {
  uid: string;
  summary?: string;
  start?: string;
  end?: string;
  allDay?: boolean;
  location?: string;
  description?: string;
  rrule?: string;
}

export type ICSEventCreate = {
  uid?: string;
  summary: string;
  start: string;
  end?: string;
  allDay?: boolean;
  location?: string;
  description?: string;
  rrule?: string;
};

export type ICSEventPatch = Partial<ICSEventCreate>;

// RRULE builder mirroring icswriter.BuildRRULE so the create form can
// produce a rule string client-side. The server re-applies its own
// formatter on patch round-trips, so this just has to be 5545-valid.
export type ICSRecurrenceFreq = '' | 'DAILY' | 'WEEKLY' | 'MONTHLY' | 'YEARLY';

export interface ICSRecurrenceOpts {
  freq: ICSRecurrenceFreq;
  interval?: number;
  count?: number;
  until?: string; // YYYY-MM-DD
  byDay?: string[]; // MO/TU/WE/TH/FR/SA/SU
}

/** Mirrors icswriter.BuildRRULE — same canonical field order so a
 *  client-built rule round-trips byte-identical through the server. */
export function buildRRULE(opts: ICSRecurrenceOpts): string {
  if (!opts.freq) return '';
  const parts: string[] = [`FREQ=${opts.freq}`];
  if (opts.interval && opts.interval > 1) parts.push(`INTERVAL=${opts.interval}`);
  if (opts.count && opts.count > 0) {
    parts.push(`COUNT=${opts.count}`);
  } else if (opts.until) {
    // YYYY-MM-DD → YYYYMMDDT000000Z (server treats UNTIL as UTC).
    const compact = opts.until.replaceAll('-', '') + 'T000000Z';
    parts.push(`UNTIL=${compact}`);
  }
  if (opts.byDay && opts.byDay.length > 0) {
    const days = [...opts.byDay].map((d) => d.toUpperCase()).filter(Boolean).sort();
    parts.push(`BYDAY=${days.join(',')}`);
  }
  return parts.join(';');
}

// Module toggles. Server is the source of truth; .granit/modules.json
// persists changes across both the web and TUI surfaces. Core entries
// arrive on the same response under coreIds — they're surfaces the user
// can never disable (notes, tasks, calendar, settings).
export interface ModuleEntry {
  id: string;
  name: string;
  description: string;
  category: string;
  enabled: boolean;
  dependsOn?: string[];
}

export interface CoreModuleEntry {
  id: string;
  name: string;
}

export interface ModulesResponse {
  modules: ModuleEntry[];
  coreIds: CoreModuleEntry[];
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
  listTasks: (
    params: {
      status?: 'open' | 'done';
      tag?: string;
      due_on?: string;
      due_before?: string;
      note?: string;
      triage?: string;
      priority?: number;
      project?: string;
      goal?: string;
      deadline?: string;
    } = {}
  ) => {
    const qs = new URLSearchParams();
    for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== '') qs.set(k, String(v));
    return req<TaskList>(`/tasks${qs.toString() ? '?' + qs : ''}`);
  },
  patchTask: (
    id: string,
    patch: Partial<Pick<Task, 'done' | 'priority' | 'dueDate' | 'text' | 'scheduledStart' | 'durationMinutes' | 'projectId' | 'snoozedUntil' | 'recurrence' | 'notes' | 'goalId' | 'deadlineId'>> & {
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
    goalId?: string;
    deadlineId?: string;
  }) => req<Task>('/tasks', { method: 'POST', body: JSON.stringify(body) }),
  deleteTask: (id: string) => req<void>(`/tasks/${id}`, { method: 'DELETE' }),

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

  // Local writable .ics calendars under <vault>/calendars/. Remote
  // subscriptions stay read-only — the create-event form gates the
  // picker on the source's writable flag.
  createCalendar: (body: { name: string; display_name?: string }) =>
    req<CalendarSource>('/calendars', { method: 'POST', body: JSON.stringify(body) }),
  createICSEvent: (source: string, ev: ICSEventCreate) =>
    req<ICSEvent>(`/calendars/${encodeURIComponent(source)}/events`, {
      method: 'POST',
      body: JSON.stringify(ev)
    }),
  patchICSEvent: (source: string, uid: string, patch: ICSEventPatch) =>
    req<ICSEvent>(`/calendars/${encodeURIComponent(source)}/events/${encodeURIComponent(uid)}`, {
      method: 'PATCH',
      body: JSON.stringify(patch)
    }),
  deleteICSEvent: (source: string, uid: string) =>
    req<void>(`/calendars/${encodeURIComponent(source)}/events/${encodeURIComponent(uid)}`, {
      method: 'DELETE'
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
  //
  // Pass {dryRun: true} to PREVIEW the proposals without writing — the
  // server returns the same `proposals` shape but skips Schedule(). The
  // PlanMyDayDrawer component uses this to let the user edit times +
  // toggle keep/skip per row before applying.
  runPlanDaySchedule: (opts?: { dryRun?: boolean }) =>
    req<{
      runId: string;
      scheduled: { taskId: string; start: string }[];
      unmatched: string[];
      proposals: PlanProposal[];
      dryRun: boolean;
    }>('/agents/plan-day-schedule', {
      method: 'POST',
      body: JSON.stringify({ dry_run: !!opts?.dryRun })
    }),

  // Plan-day-apply: commit a (possibly user-edited) subset of proposals
  // returned by a prior dry-run. No LLM call — pure sidecar write, fast.
  // The drawer uses this on the user's "Apply" click so the edited
  // schedule lands on the calendar atomically.
  applyPlanDaySchedule: (proposals: PlanApplyProposal[]) =>
    req<{
      scheduled: { taskId: string; start: string }[];
      errors: string[];
    }>('/agents/plan-day-apply', {
      method: 'POST',
      body: JSON.stringify({ proposals })
    }),

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

  // Bible bookmarks — saved passages, optional note.
  listBibleBookmarks: () =>
    req<{ bookmarks: BibleBookmark[]; total: number }>('/bible/bookmarks'),
  createBibleBookmark: (
    body: Pick<BibleBookmark, 'bookCode' | 'book' | 'chapter' | 'verseFrom' | 'verseTo' | 'text'> & {
      note?: string;
    }
  ) =>
    req<BibleBookmark>('/bible/bookmarks', {
      method: 'POST',
      body: JSON.stringify(body)
    }),
  patchBibleBookmark: (id: string, patch: { note?: string }) =>
    req<BibleBookmark>(`/bible/bookmarks/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      body: JSON.stringify(patch)
    }),
  deleteBibleBookmark: (id: string) =>
    req<void>(`/bible/bookmarks/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  // Finance — accounts / subscriptions / income streams / goals.
  // The overview endpoint is a single composite read for the
  // dashboard summary, so the page hydrates in one round trip.
  finOverview: () => req<FinOverview>('/finance/overview'),

  finListAccounts: () => req<{ accounts: FinAccount[]; total: number }>('/finance/accounts'),
  finCreateAccount: (b: Partial<FinAccount>) =>
    req<FinAccount>('/finance/accounts', { method: 'POST', body: JSON.stringify(b) }),
  finPatchAccount: (id: string, p: Partial<FinAccount>) =>
    req<FinAccount>(`/finance/accounts/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(p) }),
  finDeleteAccount: (id: string) =>
    req<void>(`/finance/accounts/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  finListSubscriptions: () => req<{ subscriptions: FinSubscription[]; total: number }>('/finance/subscriptions'),
  finCreateSubscription: (b: Partial<FinSubscription>) =>
    req<FinSubscription>('/finance/subscriptions', { method: 'POST', body: JSON.stringify(b) }),
  finPatchSubscription: (id: string, p: Partial<FinSubscription>) =>
    req<FinSubscription>(`/finance/subscriptions/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(p) }),
  finDeleteSubscription: (id: string) =>
    req<void>(`/finance/subscriptions/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  finListIncome: () => req<{ streams: FinIncomeStream[]; total: number }>('/finance/income'),
  finCreateIncome: (b: Partial<FinIncomeStream>) =>
    req<FinIncomeStream>('/finance/income', { method: 'POST', body: JSON.stringify(b) }),
  finPatchIncome: (id: string, p: Partial<FinIncomeStream>) =>
    req<FinIncomeStream>(`/finance/income/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(p) }),
  finDeleteIncome: (id: string) =>
    req<void>(`/finance/income/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  finListGoals: () => req<{ goals: FinGoal[]; total: number }>('/finance/goals'),
  finCreateGoal: (b: Partial<FinGoal>) =>
    req<FinGoal>('/finance/goals', { method: 'POST', body: JSON.stringify(b) }),
  finPatchGoal: (id: string, p: Partial<FinGoal>) =>
    req<FinGoal>(`/finance/goals/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(p) }),
  finDeleteGoal: (id: string) =>
    req<void>(`/finance/goals/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  // Prayer intentions
  listPrayer: () => req<{ intentions: PrayerIntention[]; total: number }>('/prayer/intentions'),
  createPrayer: (b: Partial<PrayerIntention>) =>
    req<PrayerIntention>('/prayer/intentions', { method: 'POST', body: JSON.stringify(b) }),
  patchPrayer: (id: string, p: Partial<PrayerIntention>) =>
    req<PrayerIntention>(`/prayer/intentions/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(p) }),
  deletePrayer: (id: string) =>
    req<void>(`/prayer/intentions/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  // People — list response includes derived fields the page uses
  // for the header pill + upcoming-birthdays section.
  listPeople: (opts: { birthdaysWithin?: number } = {}) => {
    const params = new URLSearchParams();
    if (opts.birthdaysWithin) params.set('birthdays_within', String(opts.birthdaysWithin));
    const q = params.toString();
    return req<{
      people: Person[];
      total: number;
      stale_count: number;
      upcoming_birthdays: Person[];
    }>(`/people${q ? `?${q}` : ''}`);
  },
  createPerson: (b: Partial<Person>) =>
    req<Person>('/people', { method: 'POST', body: JSON.stringify(b) }),
  patchPerson: (id: string, p: Partial<Person>) =>
    req<Person>(`/people/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(p) }),
  pingPerson: (id: string) =>
    req<Person>(`/people/${encodeURIComponent(id)}/ping`, { method: 'POST' }),
  deletePerson: (id: string) =>
    req<void>(`/people/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  // Measurements
  listMeasurementSeries: () =>
    req<{
      series: MeasurementSeries[];
      total: number;
      latest: Record<string, MeasurementEntry>;
      entry_count: number;
    }>('/measurements/series'),
  createMeasurementSeries: (b: Partial<MeasurementSeries>) =>
    req<MeasurementSeries>('/measurements/series', { method: 'POST', body: JSON.stringify(b) }),
  patchMeasurementSeries: (id: string, p: Partial<MeasurementSeries>) =>
    req<MeasurementSeries>(`/measurements/series/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(p) }),
  deleteMeasurementSeries: (id: string) =>
    req<void>(`/measurements/series/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  listMeasurementEntries: (opts: { series?: string; limit?: number } = {}) => {
    const params = new URLSearchParams();
    if (opts.series) params.set('series', opts.series);
    if (opts.limit) params.set('limit', String(opts.limit));
    const q = params.toString();
    return req<{ entries: MeasurementEntry[]; total: number }>(`/measurements/entries${q ? `?${q}` : ''}`);
  },
  createMeasurementEntry: (b: Partial<MeasurementEntry>) =>
    req<MeasurementEntry>('/measurements/entries', { method: 'POST', body: JSON.stringify(b) }),
  deleteMeasurementEntry: (id: string) =>
    req<void>(`/measurements/entries/${encodeURIComponent(id)}`, { method: 'DELETE' }),

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
  putDashboard: (cfg: DashboardConfig) => req<DashboardConfig>('/dashboard', { method: 'PUT', body: JSON.stringify(cfg) }),

  // Module toggles. listModules returns both the toggleable modules
  // and the always-on core IDs (notes/tasks/calendar/settings) so the
  // settings UI can render a unified list.
  listModules: () => req<ModulesResponse>('/modules'),
  setModules: (patch: Record<string, boolean>) =>
    req<ModulesResponse>('/modules', { method: 'PUT', body: JSON.stringify({ enabled: patch }) })
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
  // Epoch millis from the source file's mtime. Server sets both to
  // the same value today since Linux ctime is unreliable as a real
  // creation-time signal — see the handler comment in
  // internal/serveapi/handlers_types.go.
  modifiedTime?: number;
  createdTime?: number;
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

// Re-exported from lib/util/date.ts (the canonical home). Keeping the
// names available from $lib/api so existing imports keep working
// without a touch-everything refactor.
export { fmtDateISO, todayISO } from '$lib/util/date';
