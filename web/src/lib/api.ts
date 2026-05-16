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
  // Soft-delete flag. true = archived (hidden from default lists,
  // markdown line intact). Set/cleared via PATCH /tasks/:id { archived: bool }.
  archived?: boolean;
  archivedAt?: string;
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
  /** False when this ICS event lives in a read-only location (vault
   *  root or <vault>/Calendars/). The UI hides edit/drag affordances
   *  so users don't waste a click on a server-side 403. Only emitted
   *  for type='ics_event'; native events are editable through their
   *  own endpoints and don't need this flag. */
  editable?: boolean;
  /** Deadline importance — set only on type='deadline' (critical/high/normal). */
  importance?: 'critical' | 'high' | 'normal';
  /** Source recurrence rule when this occurrence was expanded from a
   *  recurring event. Repeated on every occurrence so the chip can
   *  show a ↻ indicator and the edit modal can target the series. */
  rrule?: string;
  /** Optional event type — drives the glyph prefix + default tint on
   *  the calendar chip. Empty / undefined for generic events. */
  kind?: string;
  /** Optional project name this event/task is linked to. For events
   *  it comes from granitmeta.Event.ProjectID; for tasks it's the
   *  task's project_id (sidecar) or Project (markdown-extracted)
   *  field, surfaced uniformly so the calendar's project-filter
   *  folds events + tasks together. Empty for unlinked rows. */
  project_id?: string;
  /** Set on a recurring-event occurrence when a per-instance
   *  override is applied. Carries the canonical key into the
   *  series' Event.Overrides map so the detail UI can offer a
   *  'reset this occurrence' action. Empty for plain occurrences
   *  and non-recurring events. */
  override_key?: string;
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
  // Free-text venture name — links the deadline to a Venture record.
  // Stored as a string for the same backwards-compat reasons as
  // Project.venture / Goal.venture (renaming the venture won't
  // transitively repoint).
  venture?: string;
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
  venture?: string;
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

// Canonical project kinds. Free-form on the server (the field is just a
// string) so the UI can introduce new kinds without a server migration,
// but the values below are the ones the create form / detail panel
// surfaces today.
export type ProjectKind =
  | 'software'
  | 'content'
  | 'research'
  | 'business'
  | 'personal'
  | 'creative'
  | 'client'
  | 'other';

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
  // Kind drives which extra fields the UI surfaces (e.g. repo_url
  // is hidden unless kind === 'software'). Persisted regardless so a
  // reclassification doesn't drop data.
  kind?: ProjectKind | string;
  // Free-text venture/company name — groups projects under a parent.
  venture?: string;
  // Source-control URL (mainly for software projects).
  repo_url?: string;
  // Server-decorated:
  progress?: number;
  tasksDone?: number;
  tasksTotal?: number;
}

// Mirrors internal/ventures.Venture — the umbrella entity above
// projects + goals. Project.venture and Goal.venture stay as free-text
// strings; this record adds the optional enrichment (description,
// mission, color, ...) when the user explicitly creates one.
export interface Venture {
  name: string;
  description?: string;
  mission?: string;
  color?: string;
  status?: 'active' | 'paused' | 'archived' | string;
  url?: string;
  tags?: string[];
  created_at?: string;
  updated_at?: string;
  // Server-decorated:
  project_count?: number;
  goal_count?: number;
}

export interface CalendarEventEntry {
  id: string;
  title: string;
  date: string;
  start_time?: string;
  end_time?: string;
  location?: string;
  color?: string;
  /** Minutes before event start to fire a Web Push reminder. 0 / undefined = no reminder. */
  remind_minutes_before?: number;
  /** RFC3339 timestamp of the most recent reminder fired (set by server). */
  last_reminder_fired?: string;
  created_at?: string;
  /** RFC 5545 recurrence rule. Empty for one-off events.
   *  Common shapes the picker emits: "FREQ=DAILY", "FREQ=WEEKLY",
   *  "FREQ=WEEKLY;BYDAY=MO,WE,FR", "FREQ=MONTHLY", "FREQ=YEARLY".
   *  Optional UNTIL=YYYYMMDDT235959Z suffix. The backend uses the
   *  same expander as ICS so semantics match across sources. */
  rrule?: string;
  /** Optional project link — free-text project name (matches
   *  Project.name). The calendar surfaces a small chip + colour
   *  overlay for linked events, and the per-project filter on the
   *  calendar page folds these in alongside scheduled tasks. Empty
   *  for unlinked events. */
  project_id?: string;
  /** Optional event type — meeting / focus / personal / travel /
   *  break / blocker. Drives the chip glyph + default tint on the
   *  calendar grid. Empty / undefined for generic events. */
  kind?: string;
  /** Per-occurrence overrides for a recurring event. Keyed by the
   *  occurrence's UTC anchor (YYYY-MM-DDTHH:MM:SS for timed,
   *  YYYY-MM-DD for all-day) — the same shape as ex_dates. The
   *  expander applies the override fields to the matching occurrence
   *  on render so a single instance can move/rename without rewriting
   *  the series base. */
  overrides?: Record<string, EventOverride>;
}

export interface EventOverride {
  /** YYYY-MM-DD — when set, the occurrence shifts to this calendar
   *  day. Time-of-day is preserved unless start_time / end_time also
   *  change it. */
  date?: string;
  /** HH:MM 24-hour. When start_time alone is set, duration is
   *  preserved (drag-move). When both are set, both wall-clock times
   *  are explicit (drag-resize). */
  start_time?: string;
  end_time?: string;
  title?: string;
  location?: string;
  color?: string;
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
  // Free-text venture/company name — mirrors Project.venture so a
  // venture roll-up can pull both projects and goals.
  venture?: string;
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
  // Optional theme tags ("love", "hope", "fear", "work", …). Populated
  // for the bundled defaults; absent for user-edited scriptures.md
  // entries (the parser doesn't read tags from the markdown source —
  // a deliberate "topics live in the binary, content lives in the
  // vault" split so the user's overrides stay simple to author).
  topics?: string[];
}

// Topic count pair returned by /api/v1/scripture and
// /api/v1/scripture/topics. Sorted by count desc, name asc.
export interface ScriptureTopic {
  topic: string;
  count: number;
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

// Vision — life mission + core values + 90-day season focus.
// Single record per vault. The server decorates the on-disk shape
// with derived season_day / season_total ("day 12 of 90") so the
// UI doesn't redo the date math on every render.
export interface Vision {
  mission?: string;
  values?: string[];
  season_focus?: string;
  season_started_at?: string;
  notes?: string;
  updated_at: string;
  season_day?: number;
  season_total?: number;
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
  // Linkage to other granit entities — at most one of project/goal/
  // venture/person is meaningful for any given intention, but
  // multiples are allowed. PassageRef is free-text scripture (e.g.
  // "Phil 4:6-7") that the prayer page renders as a clickable jump
  // into the bible reader.
  project?: string;
  goal?: string;
  venture?: string;
  person?: string;
  note_path?: string;
  passage_ref?: string;
  created_at: string;
  updated_at: string;
}

// Virtues — character formation tracker. Mirrors internal/virtues
// — a Virtue carries a name, anchor (free-form scripture or
// definition), optional season, and a history of weekly Checks
// that capture a 1–5 self-evaluation plus reflection note.
export type VirtueStatus = 'active' | 'paused' | 'archived';
export interface VirtueCheck {
  week_start: string; // YYYY-MM-DD, Monday of the scored week
  score: number; // 1–5
  note?: string;
  logged_at: string; // RFC3339
}
// HubItem — entry on the personal launch pad (/hub). Title is the
// only required field; everything else is optional so the same
// shape covers a pure link, a tool entry, or a non-critical
// credential record without forcing the user to pick a "kind".
export interface HubItem {
  id: string;
  title: string;
  url?: string;
  category?: string;
  icon?: string;
  notes?: string;
  username?: string;
  password?: string;
  favorite?: boolean;
  position?: number;          // manual order within a category (drag-to-reorder)
  last_visited_at?: string;   // RFC3339 — set by /visit endpoint on click
  created_at?: string;
  updated_at?: string;
}

// HubCommand — one row of a tool's setup-command list. Label is
// the human reading ("install via brew"); Command is the shell
// line that lands in the clipboard on copy. Notes is optional
// context (pre-conditions, expected output, etc).
export interface HubCommand {
  label: string;
  command: string;
  notes?: string;
}

// HubTool — a curated catalogue card. The "tools" half of the hub:
// program name + an ordered list of setup commands the user copies
// into a terminal. Separate from HubItem (links) — different shape,
// different storage file (.granit/hub-tools.json).
export interface HubTool {
  id: string;
  name: string;
  description?: string;
  icon?: string;
  color?: string;
  tags?: string[];
  commands: HubCommand[];
  sort_order?: number;
  created_at?: string;
  updated_at?: string;
}
export interface Virtue {
  id: string;
  name: string;
  description?: string;
  anchor?: string; // free-form: scripture ref / definition / quote
  status: VirtueStatus | string;
  season?: string;
  color?: string;
  /** Habit names whose practice cultivates this virtue. */
  linked_habits?: string[];
  created_at: string;
  updated_at: string;
  checks?: VirtueCheck[];
}

// Shopping list. Mirrors internal/shopping — single-record model
// with a `standard` flag for recurring needs (bread, olive oil,
// basic clothing). Re-planning a bought standard flips its
// status back to 'planned' in place; no template-duplication.
export type ShoppingStatus = 'planned' | 'bought' | 'skipped';
// Cadence for recurring standards. "" means catalogue-only (no
// monthly projection contribution). The /finance overview reads
// the cadence-driven monthly estimate alongside subscriptions to
// surface "what does my baseline month cost".
export type ShoppingCadence = '' | 'weekly' | 'biweekly' | 'monthly' | 'quarterly' | 'yearly';
export interface ShoppingItem {
  id: string;
  name: string;
  description?: string;
  url?: string;
  price?: number;
  quantity?: number;
  category?: string;
  status: ShoppingStatus | string;
  standard?: boolean;
  cadence?: ShoppingCadence | string;
  notes?: string;
  bought_at?: string;
  created_at: string;
  updated_at: string;
}
export interface ShoppingTotals {
  planned_count: number;
  planned_sum: number;
  bought_month_count: number;
  bought_month_sum: number;
  recurring_monthly_estimate: number;
  recurring_standards_count?: number;
}

// Books — the EPUB reader. Library lives at <vault>/Books/; per-
// book sidecar (progress + highlights + bookmarks) at
// .granit/books/<id>.json. Mirrors internal/books.
export interface BookSummary {
  id: string;
  title: string;
  authors?: string[];
  hasCover: boolean;
  path: string;
  bytes: number;
}
export interface BookShelfRow extends BookSummary {
  lastReadAt?: string;
  furthestChapter: number;
  progressPct: number;
  totalChapters: number;
}
export interface BookChapterMeta {
  index: number;
  label: string;
  linear: boolean;
}
export interface BookTOCEntry {
  Title: string;
  SpineIdx: number;
  Children?: BookTOCEntry[];
}
export interface BookDetail extends BookSummary {
  chapters: BookChapterMeta[];
  toc?: BookTOCEntry[];
}
export interface BookProgress {
  chapterIdx: number;
  scrollFraction: number;
  lastReadAt?: string;
  furthestChapter: number;
}
export interface BookHighlight {
  id: string;
  chapterIdx: number;
  text: string;
  prefix?: string;
  suffix?: string;
  color: string;
  note?: string;
  createdAt: string;
  updatedAt?: string;
}
export interface BookBookmark {
  id: string;
  chapterIdx: number;
  scrollFraction?: number;
  label: string;
  createdAt: string;
}
export interface BookSidecar {
  bookId: string;
  progress: BookProgress;
  highlights?: BookHighlight[];
  bookmarks?: BookBookmark[];
}

// Margin annotations on notes — user-authored marginalia tied to
// a specific line, displayed as a side column in the editor /
// preview. Mirrors internal/annotations.
export interface NoteAnnotation {
  id: string;
  notePath: string;
  lineNum: number;
  anchorText: string;
  text: string;
  color?: string;
  createdAt: string;
  updatedAt?: string;
}

// Discover — one row from the search proxy. The shape is shared
// across sources so the UI renders a uniform card grid.
//
// The 'standardebooks' source is kept in the type union for API
// compatibility with older clients but Standard Ebooks paywalled
// every catalogue OPDS feed in 2026 (Patrons Circle Basic auth).
// The /discover page hides it from the source filter; the backend
// surfaces a per-source warning if a stale client still requests it.
export type BookDiscoverSource = 'gutenberg' | 'standardebooks';
export interface BookDiscoverResult {
  source: BookDiscoverSource;
  externalId: string;
  title: string;
  authors?: string[];
  // Latest death year across all authors. Lets the UI show
  // "(d. 1817)" so users in life+70 jurisdictions can self-check
  // whether a US-public-domain title is also free in their country.
  authorDeathYear?: number;
  language?: string;
  subjects?: string[];
  publishedYear?: number;
  downloadUrl: string;
  coverUrl?: string;
  externalUrl?: string;
  license?: string;
  description?: string;
}

// Per-source warning surfaced when one source fails but at least
// one other source returned successfully. Lets the UI render
// "Standard Ebooks unavailable" inline instead of treating a
// partial outage as a hard search failure.
export interface BookDiscoverWarning {
  source: BookDiscoverSource;
  message: string;
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
  /** Granit's X-GRANIT-KIND extension — empty string clears the
   *  property; unknown values display as generic. */
  kind?: string;
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
  // Per-vault print defaults at .granit/print-config.json. Header /
  // footer / mode used by the note-print preview overlay; backed by
  // the server so they survive across browsers and devices.
  getPrintConfig: () => req<{ header: string; footer: string; mode: string }>('/print-config'),
  putPrintConfig: (cfg: { header: string; footer: string; mode: string }) =>
    req<{ header: string; footer: string; mode: string }>('/print-config', {
      method: 'PUT',
      body: JSON.stringify(cfg)
    }),

  // Hub — personal launch pad at .granit/hub.json. Quick-access
  // links, tools, and optional non-critical credentials. Real
  // secrets stay in a password manager; the hub is for the daily
  // "URL of staging dashboard" / "API key for service X" tier.
  listHubItems: () => req<{ items: HubItem[]; total: number }>('/hub/items'),
  createHubItem: (item: Partial<HubItem>) =>
    req<HubItem>('/hub/items', { method: 'POST', body: JSON.stringify(item) }),
  patchHubItem: (id: string, patch: Partial<HubItem>) =>
    req<HubItem>(`/hub/items/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      body: JSON.stringify(patch)
    }),
  deleteHubItem: (id: string) =>
    req<void>(`/hub/items/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  // Drag-to-reorder: send the new ordering of a category's items.
  // The server rewrites Position 1..N on each ID in order; items
  // not in the array keep their existing position.
  reorderHubItems: (ids: string[]) =>
    req<void>('/hub/reorder', { method: 'POST', body: JSON.stringify({ ids }) }),
  // Stamp last_visited_at on a hub item. Fire-and-forget on the
  // client side: a failed visit-stamp shouldn't block the user
  // from reaching their destination.
  visitHubItem: (id: string) =>
    req<void>(`/hub/items/${encodeURIComponent(id)}/visit`, { method: 'POST' }),

  // Hub tools — curated setup-command catalogue at
  // .granit/hub-tools.json. Same module as the link launcher; a
  // separate file (and a separate WS event hub.tools.changed) so
  // a tab on the tools section can refresh independently.
  listHubTools: () => req<{ tools: HubTool[]; total: number }>('/hub/tools'),
  createHubTool: (tool: Partial<HubTool>) =>
    req<HubTool>('/hub/tools', { method: 'POST', body: JSON.stringify(tool) }),
  patchHubTool: (id: string, patch: Partial<HubTool>) =>
    req<HubTool>(`/hub/tools/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      body: JSON.stringify(patch)
    }),
  deleteHubTool: (id: string) =>
    req<void>(`/hub/tools/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  // Drag-to-reorder for tool cards. Server rewrites sort_order
  // 1..N on each ID in the array order; tools not in the array
  // keep their existing position.
  reorderHubTools: (ids: string[]) =>
    req<void>('/hub/tools/reorder', { method: 'POST', body: JSON.stringify({ ids }) }),
  // Append the curated starter set (git, Node+pnpm, Docker, shell
  // snippets) to the user's catalogue. Idempotent against duplicates
  // by name (case-insensitive) so a double-click doesn't end up
  // with two "git" cards. Returns the count of new tools added.
  seedHubTools: () =>
    req<{ added: number; total: number }>('/hub/tools/seed', { method: 'POST' }),
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
      // includeArchived=true returns archived tasks alongside active.
      // archived=true returns ONLY archived (for the Archive drawer).
      // Default omits both, so archived tasks are hidden from every
      // existing list view automatically.
      includeArchived?: boolean;
      archived?: boolean;
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
      archived?: boolean;
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
    /** When set, the new task is inserted as a subtask of the task on
     *  this 1-indexed line in `notePath`. Resulting markdown is
     *  indented one level deeper than the parent (2 spaces). */
    parentLine?: number;
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

  // Consecutive-day daily-note streak — drives the status-bar
  // streak badge. Cheap to call; backend just scans the vault
  // snapshot for YYYY-MM-DD.md filenames.
  dailyStreak: () =>
    req<{
      current: number;
      longest: number;
      lastDate?: string;
      todayLogged: boolean;
    }>('/daily/streak'),
  // Bible reading streak — parallel surface to dailyStreak. Same
  // shape so the StreakBadge component can render either without
  // a per-source branch. recordBibleRead is idempotent on date so
  // the bible page can fire-and-forget on every passage open.
  bibleStreak: () =>
    req<{
      current: number;
      longest: number;
      lastDate?: string;
      todayLogged: boolean;
    }>('/bible/streak'),
  recordBibleRead: (date?: string) =>
    req<{ added: boolean; date: string }>('/bible/read', {
      method: 'POST',
      body: JSON.stringify(date ? { date } : {})
    }),

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
  // Append an EXDATE entry to a recurring ICS series so the named
  // occurrence is excluded from the rendered calendar. Pairs with
  // createICSEvent to implement "edit only this occurrence" — the
  // UI calls skip + create-standalone sequentially to detach a
  // single instance from a series. `date` is RFC3339 for timed
  // events or YYYY-MM-DD for all-day.
  skipICSOccurrence: (source: string, uid: string, date: string) =>
    req<ICSEvent>(`/calendars/${encodeURIComponent(source)}/events/${encodeURIComponent(uid)}/skip`, {
      method: 'POST',
      body: JSON.stringify({ date })
    }),

  // Projects
  listProjects: () => req<{ projects: Project[]; total: number }>('/projects'),
  getProject: (name: string) => req<Project>(`/projects/${encodeURIComponent(name)}`),
  // Read-only local-repo scanner. Backend enforces path lives
  // under the user's home or vault root + refuses traversal /
  // symlinks. Frontend uses the returned README + manifest +
  // commits as grounding context for AI starter-pack generation.
  scanRepo: (path: string) =>
    req<{
      path: string;
      name: string;
      isGit: boolean;
      manifest?: string;
      manifestContent?: string;
      readmeName?: string;
      readmeContent?: string;
      fileTree?: string[];
      recentCommits?: string[];
      branch?: string;
    }>('/reposcan', {
      method: 'POST',
      body: JSON.stringify({ path })
    }),
  createProject: (p: Partial<Project>) =>
    req<Project>('/projects', { method: 'POST', body: JSON.stringify(p) }),
  patchProject: (name: string, p: Partial<Project>) =>
    req<Project>(`/projects/${encodeURIComponent(name)}`, {
      method: 'PATCH',
      body: JSON.stringify(p)
    }),
  deleteProject: (name: string) =>
    req<void>(`/projects/${encodeURIComponent(name)}`, { method: 'DELETE' }),

  // Ventures (umbrella above projects/goals)
  listVentures: () => req<{ ventures: Venture[]; total: number }>('/ventures'),
  getVenture: (name: string) => req<Venture>(`/ventures/${encodeURIComponent(name)}`),
  createVenture: (v: Partial<Venture>) =>
    req<Venture>('/ventures', { method: 'POST', body: JSON.stringify(v) }),
  patchVenture: (name: string, v: Partial<Venture>) =>
    req<Venture>(`/ventures/${encodeURIComponent(name)}`, {
      method: 'PATCH',
      body: JSON.stringify(v)
    }),
  deleteVenture: (name: string) =>
    req<void>(`/ventures/${encodeURIComponent(name)}`, { method: 'DELETE' }),

  // Editor snippets — slash-command templates the CodeMirror
  // autocomplete extension surfaces when the user types '/'. Read-only;
  // the source of truth lives in internal/snippets shared with the TUI.
  listSnippets: () =>
    req<{ snippets: { trigger: string; description: string; content: string }[]; total: number }>(
      '/snippets'
    ),

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
  /** Add an EXDATE to a recurring event so a single occurrence is
   *  cancelled without disrupting the rest of the series. Date is
   *  YYYY-MM-DD for all-day events, YYYY-MM-DDTHH:MM:SS for timed.
   *  No-op when the date is already in the list. */
  skipEventOccurrence: (id: string, date: string) =>
    req<CalendarEventEntry>(`/events/${encodeURIComponent(id)}/skip`, {
      method: 'POST',
      body: JSON.stringify({ date })
    }),
  /** Set or clear a per-occurrence override on a recurring event.
   *  `key` identifies the occurrence (UTC YYYY-MM-DDTHH:MM:SS for
   *  timed, YYYY-MM-DD for all-day — same shape as the EXDATE entries
   *  the skip endpoint writes). Pass an empty `override` object to
   *  clear an existing entry. The series base is untouched — only
   *  the matching occurrence renders with the overridden fields.
   *  Backend refuses overrides on non-recurring events. */
  overrideEventOccurrence: (
    id: string,
    key: string,
    override: EventOverride
  ) =>
    req<CalendarEventEntry>(`/events/${encodeURIComponent(id)}/override`, {
      method: 'POST',
      body: JSON.stringify({ key, override })
    }),

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

  // Chat — streaming variant. Yields tokens as they arrive via the
  // onChunk callback so consumers can render progressively. Uses
  // the SSE shape served by /chat/stream (data: {chunk}, event:
  // done, event: error). AbortSignal honoured both client-side
  // (closes the fetch) and server-side (the upstream LLM context
  // is cancelled when r.Context is done).
  //
  // Returns when the stream completes — onDone fires once on
  // success, onError once on failure. Aborts surface as an
  // AbortError that's intentionally NOT routed to onError so the
  // caller can detect "user cancelled" vs "real failure" cleanly.
  chatStream: async (
    messages: ChatMessage[],
    notePath: string | undefined,
    handlers: {
      onChunk: (chunk: string) => void;
      onDone?: () => void;
      onError?: (err: Error) => void;
    },
    signal?: AbortSignal
  ): Promise<void> => {
    const headers = new Headers({ 'Content-Type': 'application/json' });
    const tok = getToken();
    if (tok) headers.set('Authorization', `Bearer ${tok}`);
    let res: Response;
    try {
      res = await fetch('/api/v1/chat/stream', {
        method: 'POST',
        headers,
        body: JSON.stringify({ messages, notePath }),
        signal
      });
    } catch (e) {
      if (e instanceof DOMException && e.name === 'AbortError') return;
      handlers.onError?.(e instanceof Error ? e : new Error(String(e)));
      return;
    }
    if (!res.ok) {
      let msg = res.statusText;
      try {
        const body = await res.json();
        if (body?.error) msg = body.error;
      } catch {}
      handlers.onError?.(new ApiError(res.status, msg));
      return;
    }
    if (!res.body) {
      handlers.onError?.(new Error('No response body'));
      return;
    }
    // Parse the SSE stream by hand — each event is a sequence of
    // `field: value` lines terminated by `\n\n`. We only care about
    // `event` (default = "message" / our chunk events) and `data`
    // (JSON payload).
    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buf = '';
    let event = '';
    let dataLines: string[] = [];

    const flush = () => {
      if (dataLines.length === 0) return;
      const data = dataLines.join('\n');
      dataLines = [];
      try {
        const parsed = JSON.parse(data) as { chunk?: string; message?: string };
        if (event === 'error') {
          handlers.onError?.(new Error(parsed.message ?? 'stream error'));
        } else if (event === 'done') {
          handlers.onDone?.();
        } else if (parsed.chunk !== undefined) {
          handlers.onChunk(parsed.chunk);
        }
      } catch {
        // Malformed event — skip rather than aborting the whole stream.
      }
      event = '';
    };

    try {
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buf += decoder.decode(value, { stream: true });
        let idx: number;
        while ((idx = buf.indexOf('\n')) >= 0) {
          const line = buf.slice(0, idx).replace(/\r$/, '');
          buf = buf.slice(idx + 1);
          if (line === '') {
            flush();
            continue;
          }
          if (line.startsWith('event: ')) event = line.slice(7).trim();
          else if (line.startsWith('data: ')) dataLines.push(line.slice(6));
        }
      }
      // Final flush for an event that arrived without a trailing
      // blank line — rare in practice but happens on abrupt close.
      if (dataLines.length > 0) flush();
    } catch (e) {
      if (e instanceof DOMException && e.name === 'AbortError') return;
      handlers.onError?.(e instanceof Error ? e : new Error(String(e)));
    }
  },

  // Scripture / devotional. Optional `topic` filters to verses tagged
  // with that theme (case-insensitive). The response always carries the
  // full topic list so a single round trip can render both the chip
  // strip and the filtered verses.
  listScriptures: (topic?: string) => {
    const qs = topic ? `?topic=${encodeURIComponent(topic)}` : '';
    return req<{
      scriptures: Scripture[];
      total: number;
      topics: ScriptureTopic[];
      topic: string;
    }>(`/scripture${qs}`);
  },
  scriptureTopics: () =>
    req<{ topics: ScriptureTopic[]; total: number }>('/scripture/topics'),
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

  // Vision — single-record GET / PUT. PUT is a full upsert, not
  // a patch-merge: the form is a flat five-field shape and a full
  // body is the least-surprising contract.
  getVision: () => req<Vision>('/vision'),
  putVision: (v: Partial<Vision>) =>
    req<Vision>('/vision', { method: 'PUT', body: JSON.stringify(v) }),

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
  // Delete a habit everywhere — strips matching checkbox lines from
  // every daily note's `## Habits` section. Destructive: streak data
  // for this habit is gone too. Returns the number of files touched.
  deleteHabit: (name: string) =>
    req<{ name: string; filesTouched: number }>(
      `/habits/${encodeURIComponent(name)}`,
      { method: 'DELETE' }
    ),
  // Rename a habit everywhere — rewrites the visible text on matching
  // checkbox lines, preserves checkbox state + per-line markers
  // (priority, due:, #tags). Streak history follows the new name.
  renameHabit: (name: string, newName: string) =>
    req<{ name: string; newName: string; filesTouched: number }>(
      `/habits/${encodeURIComponent(name)}`,
      { method: 'PATCH', body: JSON.stringify({ new_name: newName }) }
    ),

  // Morning routine — saves a `## Daily Plan` block into today's daily note
  saveMorning: (body: {
    scripture?: { text: string; source: string };
    goal?: string;
    tasks?: string[];
    habits?: string[];
    thoughts?: string;
  }) => req<{ path: string; saved: boolean }>('/morning/save', { method: 'POST', body: JSON.stringify(body) }),

  // Daily examen — Ignatian evening reflection saved as a `## Examen`
  // section in the daily note. Empty `date` defaults to today;
  // explicit YYYY-MM-DD lets the user backfill an examen for the
  // wrong-end-of-the-day case (typed it the next morning).
  saveExamen: (body: {
    date?: string;
    saw_god?: string;
    missed?: string;
    gratitude?: string;
    tomorrow?: string;
  }) => req<{ path: string; saved: boolean }>('/examen', { method: 'POST', body: JSON.stringify(body) }),

  // Shopping list — single-record items with a `standard` flag for
  // recurring needs. /totals serves the /finance overview rollup.
  listShopping: () => req<{ items: ShoppingItem[]; total: number }>('/shopping'),
  shoppingTotals: () => req<ShoppingTotals>('/shopping/totals'),
  getShoppingItem: (id: string) => req<ShoppingItem>(`/shopping/${encodeURIComponent(id)}`),
  createShoppingItem: (it: Partial<ShoppingItem>) =>
    req<ShoppingItem>('/shopping', { method: 'POST', body: JSON.stringify(it) }),
  patchShoppingItem: (id: string, it: Partial<ShoppingItem>) =>
    req<ShoppingItem>(`/shopping/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      body: JSON.stringify(it)
    }),
  deleteShoppingItem: (id: string) =>
    req<void>(`/shopping/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  // Books — EPUB reader. Library at <vault>/Books/, sidecar at
  // .granit/books/<id>.json. Cover + asset endpoints are binary;
  // we expose helper URLs the components can use directly with
  // <img src=...> after fetching to a blob (auth requires Bearer
  // header which <img> can't send).
  listBooks: () => req<{ books: BookShelfRow[]; total: number }>('/books'),
  getBook: (id: string) => req<BookDetail>(`/books/${encodeURIComponent(id)}`),
  getBookChapter: (id: string, idx: number) =>
    req<{ index: number; html: string }>(
      `/books/${encodeURIComponent(id)}/chapter/${idx}`
    ),
  getBookSidecar: (id: string) =>
    req<BookSidecar>(`/books/${encodeURIComponent(id)}/sidecar`),
  putBookProgress: (id: string, p: Partial<BookProgress>) =>
    req<{ ok: true }>(`/books/${encodeURIComponent(id)}/progress`, {
      method: 'PUT',
      body: JSON.stringify(p)
    }),
  createBookHighlight: (id: string, h: Partial<BookHighlight>) =>
    req<BookHighlight>(`/books/${encodeURIComponent(id)}/highlights`, {
      method: 'POST',
      body: JSON.stringify(h)
    }),
  patchBookHighlight: (id: string, hid: string, body: { note?: string; color?: string }) =>
    req<BookHighlight>(
      `/books/${encodeURIComponent(id)}/highlights/${encodeURIComponent(hid)}`,
      { method: 'PATCH', body: JSON.stringify(body) }
    ),
  deleteBookHighlight: (id: string, hid: string) =>
    req<void>(
      `/books/${encodeURIComponent(id)}/highlights/${encodeURIComponent(hid)}`,
      { method: 'DELETE' }
    ),
  createBookBookmark: (id: string, b: Partial<BookBookmark>) =>
    req<BookBookmark>(`/books/${encodeURIComponent(id)}/bookmarks`, {
      method: 'POST',
      body: JSON.stringify(b)
    }),
  deleteBookBookmark: (id: string, bid: string) =>
    req<void>(
      `/books/${encodeURIComponent(id)}/bookmarks/${encodeURIComponent(bid)}`,
      { method: 'DELETE' }
    ),
  // Cover image fetch — returns a Blob URL the caller is responsible
  // for revoking when the component unmounts. Returns null on 404
  // (book has no cover) so the UI can render a typographic fallback.
  bookCoverBlobURL: async (id: string): Promise<string | null> => {
    const tok = getToken();
    const headers = new Headers();
    if (tok) headers.set('Authorization', `Bearer ${tok}`);
    const res = await fetch(`/api/v1/books/${encodeURIComponent(id)}/cover`, { headers });
    if (res.status === 404) return null;
    if (!res.ok) throw new ApiError(res.status, res.statusText);
    const blob = await res.blob();
    return URL.createObjectURL(blob);
  },
  // AI margin-annotation suggester. Proposes 3-5 annotations the
  // user reviews + accepts via the regular create endpoint. Off by
  // default — opt-in via Settings → AI features.
  aiAnnotateNote: (notePath: string, signal?: AbortSignal) =>
    req<{
      annotations: { lineNum: number; anchorText: string; text: string; color: string }[];
      raw?: string;
      warning?: string;
    }>('/ai/annotate-note', {
      method: 'POST',
      body: JSON.stringify({ notePath }),
      signal
    }),

  // Margin annotations on notes. The list endpoint accepts an
  // optional notePath query — empty returns the full store for a
  // future cross-vault "all annotations" surface; the editor
  // always passes the active path.
  listAnnotations: (notePath?: string) => {
    const q = notePath ? `?notePath=${encodeURIComponent(notePath)}` : '';
    return req<{ annotations: NoteAnnotation[]; total: number }>(`/annotations${q}`);
  },
  createAnnotation: (a: Partial<NoteAnnotation>) =>
    req<NoteAnnotation>('/annotations', { method: 'POST', body: JSON.stringify(a) }),
  patchAnnotation: (id: string, patch: { text?: string; color?: string; lineNum?: number; anchorText?: string }) =>
    req<NoteAnnotation>(`/annotations/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      body: JSON.stringify(patch)
    }),
  deleteAnnotation: (id: string) =>
    req<void>(`/annotations/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  // Discover — search legal e-book sources (Project Gutenberg +
  // Standard Ebooks) and import a chosen result into the vault.
  // The import endpoint streams the EPUB through the backend so:
  //   1. CORS / auth on the external host is irrelevant
  //   2. The server can sniff the response and reject non-EPUB
  //      replies before they hit the user's vault
  discoverBooks: (q: string, opts: { sources?: BookDiscoverSource[]; limit?: number } = {}) => {
    const params = new URLSearchParams({ q });
    if (opts.sources && opts.sources.length > 0) params.set('source', opts.sources.join(','));
    if (opts.limit) params.set('limit', String(opts.limit));
    return req<{
      results: BookDiscoverResult[];
      total: number;
      q: string;
      warnings?: BookDiscoverWarning[];
    }>(`/books/discover/search?${params}`);
  },
  importBook: (body: { source: BookDiscoverSource; downloadUrl: string; title?: string }) =>
    req<BookSummary>('/books/discover/import', {
      method: 'POST',
      body: JSON.stringify(body)
    }),

  // Asset fetch — for chapter HTML's rewritten `src=".../asset/..."`
  // refs to resolve through auth. We fetch each asset to a blob and
  // patch the rendered DOM, since <img> can't carry the bearer.
  bookAssetBlobURL: async (id: string, relPath: string): Promise<string | null> => {
    const tok = getToken();
    const headers = new Headers();
    if (tok) headers.set('Authorization', `Bearer ${tok}`);
    const url = `/api/v1/books/${encodeURIComponent(id)}/asset?path=${encodeURIComponent(relPath)}`;
    const res = await fetch(url, { headers });
    if (!res.ok) return null;
    const blob = await res.blob();
    return URL.createObjectURL(blob);
  },

  // Virtues — character formation tracker. Checks live on a separate
  // POST endpoint to avoid two clients clobbering each other's
  // history via a PATCH array.
  listVirtues: () => req<{ virtues: Virtue[]; total: number }>('/virtues'),
  getVirtue: (id: string) => req<Virtue>(`/virtues/${encodeURIComponent(id)}`),
  createVirtue: (v: Partial<Virtue>) =>
    req<Virtue>('/virtues', { method: 'POST', body: JSON.stringify(v) }),
  patchVirtue: (id: string, v: Partial<Virtue>) =>
    req<Virtue>(`/virtues/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      body: JSON.stringify(v)
    }),
  deleteVirtue: (id: string) =>
    req<void>(`/virtues/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  logVirtueCheck: (id: string, body: { week_start?: string; score: number; note?: string }) =>
    req<Virtue>(`/virtues/${encodeURIComponent(id)}/checks`, {
      method: 'POST',
      body: JSON.stringify(body)
    }),

  // Full-text search across vault content (uses granit's TF-IDF SearchIndex)
  search: (q: string, limit = 30) =>
    req<{ results: SearchHit[]; total: number; q: string; ready: boolean }>(
      `/search?q=${encodeURIComponent(q)}&limit=${limit}`
    ),

  // AI: foundation (prefs, audit log, snapshot view) + Tier 1
  // features (daily briefing, weekly review, inbox triage).
  // Every feature is opt-in via /ai/prefs; the audit log records
  // every outbound request (sizes only — no prompt bodies stored).
  getAIPrefs: () => req<{ prefs: AIPreferences; warning?: string }>('/ai/prefs'),
  putAIPrefs: (p: AIPreferences) =>
    req<{ prefs: AIPreferences }>('/ai/prefs', { method: 'PUT', body: JSON.stringify(p) }),
  getAIAudit: () => req<{ entries: AIAuditEntry[] }>('/ai/audit'),
  clearAIAudit: () => req<void>('/ai/audit', { method: 'DELETE' }),
  getAIStatus: () => req<AIStatus>('/ai/status'),
  getAISnapshot: () => req<{ snapshot: unknown }>('/ai/snapshot'),
  // Long-term AI memory — facts about the user the chat overlay
  // injects into every thread's system prelude. User-controlled:
  // add via slash command or proposed-action chip, edit/delete via
  // settings panel.
  listAIMemory: () =>
    req<{ facts: AIMemoryFact[]; total: number }>('/ai/memory'),
  addAIMemory: (content: string, tags?: string[]) =>
    req<AIMemoryFact>('/ai/memory', {
      method: 'POST',
      body: JSON.stringify({ content, tags })
    }),
  patchAIMemory: (id: string, content?: string, tags?: string[]) =>
    req<AIMemoryFact>(`/ai/memory/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      body: JSON.stringify({ content, tags })
    }),
  deleteAIMemory: (id: string) =>
    req<void>(`/ai/memory/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  aiDailyBriefing: (signal?: AbortSignal) =>
    req<{ markdown: string }>('/ai/daily-briefing', { method: 'POST', body: '{}', signal }),
  aiWeeklyReview: (signal?: AbortSignal) =>
    req<{ markdown: string }>('/ai/weekly-review', { method: 'POST', body: '{}', signal }),
  aiInboxTriage: (signal?: AbortSignal) =>
    req<{ proposals: AITriageProposal[]; raw?: string; warning?: string }>(
      '/ai/inbox-triage',
      { method: 'POST', body: '{}', signal }
    ),
  aiDeadlineDetect: (signal?: AbortSignal) =>
    req<{ proposals: AIDeadlineProposal[]; raw?: string; warning?: string }>(
      '/ai/deadline-detect',
      { method: 'POST', body: '{}', signal }
    ),
  aiSuggestLinks: (
    body: { note_path: string; content: string; existing_tags?: string[] },
    signal?: AbortSignal
  ) =>
    req<{
      tags: { name: string; rationale?: string }[];
      links: {
        type: 'note' | 'project' | 'goal' | 'venture';
        ref: string;
        title?: string;
        rationale?: string;
      }[];
      raw?: string;
      warning?: string;
    }>('/ai/suggest-links', { method: 'POST', body: JSON.stringify(body), signal }),

  // Notification preferences — per-category toggles, quiet
  // hours, defaults. Stored at .granit/notifications.json.
  getNotificationPrefs: () =>
    req<{ prefs: NotificationPrefs; warning?: string }>('/notifications/prefs'),
  putNotificationPrefs: (prefs: NotificationPrefs) =>
    req<{ prefs: NotificationPrefs }>('/notifications/prefs', {
      method: 'PUT',
      body: JSON.stringify(prefs)
    }),

  // Email tracker — CRM-grade record of correspondence. Storage:
  // <vault>/.granit/emails.json. Fields mirror internal/email.Email.
  listEmails: () => req<{ emails: Email[]; total: number }>('/emails'),
  getEmail: (id: string) => req<Email>(`/emails/${encodeURIComponent(id)}`),
  createEmail: (e: Partial<Email>) =>
    req<Email>('/emails', { method: 'POST', body: JSON.stringify(e) }),
  patchEmail: (id: string, e: Partial<Email>) =>
    req<Email>(`/emails/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      body: JSON.stringify(e)
    }),
  deleteEmail: (id: string) =>
    req<void>(`/emails/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  // Email signatures — HTML signature library at
  // <vault>/.granit/email-signatures.json. Render on the page
  // inside an iframe sandbox so user-authored HTML can't fire
  // scripts on the host.
  listEmailSignatures: () =>
    req<{ signatures: EmailSignature[]; total: number }>('/email-signatures'),
  getEmailSignature: (id: string) =>
    req<EmailSignature>(`/email-signatures/${encodeURIComponent(id)}`),
  createEmailSignature: (s: Partial<EmailSignature>) =>
    req<EmailSignature>('/email-signatures', { method: 'POST', body: JSON.stringify(s) }),
  patchEmailSignature: (id: string, s: Partial<EmailSignature>) =>
    req<EmailSignature>(`/email-signatures/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      body: JSON.stringify(s)
    }),
  deleteEmailSignature: (id: string) =>
    req<void>(`/email-signatures/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  // Autocommit settings — debounced git-commit-on-save, opt-in.
  getAutocommit: () => req<{ enabled: boolean; isGitRepo: boolean }>('/autocommit'),
  putAutocommit: (enabled: boolean) =>
    req<{ enabled: boolean; isGitRepo: boolean }>('/autocommit', {
      method: 'PUT',
      body: JSON.stringify({ enabled })
    }),

  // AI chapter generation — fired from the "missing wikilink →
  // generate with AI" affordance. Body shape mirrors handlers_ai_chapter.go.
  generateChapter: (body: {
    parentPath?: string;
    chapterTitle: string;
    outline?: string;
    save?: boolean;
    targetPath?: string;
  }) =>
    req<{ content: string; path?: string }>('/ai/generate-chapter', {
      method: 'POST',
      body: JSON.stringify(body)
    }),

  // Stoicera intranet integration. Exposes a read-only API surface
  // for the stoicera-intranet app (intranet.stoicera.cyou) to sync
  // projects/tasks/goals belonging to a specific venture. Off by
  // default; the user enables + names the venture in Settings.
  getStoiceraSettings: () =>
    req<{ enabled: boolean; venture_name: string; token_masked: string; has_token: boolean }>(
      '/stoicera-integration/settings'
    ),
  patchStoiceraSettings: (body: { enabled?: boolean; venture_name?: string; regenerate?: boolean }) =>
    req<{ enabled: boolean; venture_name: string; token_masked: string; has_token: boolean }>(
      '/stoicera-integration/settings',
      { method: 'PATCH', body: JSON.stringify(body) }
    ),
  getStoiceraToken: () => req<{ token: string }>('/stoicera-integration/token'),

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
  // Layout presets. Save snapshots the current widgets under a name;
  // activate switches the live arrangement to a saved preset; delete
  // drops one. All four return the full updated DashboardConfig so
  // the client can swap state in one trip.
  listDashboardLayouts: () =>
    req<{ layouts: DashboardLayout[]; active: string }>('/dashboard/layouts'),
  saveDashboardLayout: (name: string) =>
    req<DashboardConfig>('/dashboard/layouts', {
      method: 'POST',
      body: JSON.stringify({ name })
    }),
  deleteDashboardLayout: (name: string) =>
    req<DashboardConfig>(`/dashboard/layouts/${encodeURIComponent(name)}`, { method: 'DELETE' }),
  activateDashboardLayout: (name: string) =>
    req<DashboardConfig>(`/dashboard/layouts/${encodeURIComponent(name)}/activate`, {
      method: 'POST'
    }),

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

export interface AIFeatureConfig {
  enabled: boolean;
  provider?: string;
}
export interface AIPreferences {
  features: Record<string, AIFeatureConfig>;
  redaction_enabled: boolean;
  disabled_redaction?: string[];
  default_provider?: string;
}
export interface AIAuditEntry {
  timestamp: string;
  feature: string;
  provider?: string;
  model?: string;
  prompt_size_bytes: number;
  prompt_hash?: string;
  redactions?: { name: string; count: number }[];
  response_size_bytes?: number;
  prompt_tokens?: number;
  completion_tokens?: number;
  /** Cost in micro-cents (1/1_000_000 of a cent). 0 = unset (no pricing
   *  data for this model, or non-billing provider like Ollama). */
  cost_micro_cents?: number;
  error?: string;
}
export interface AIFeatureStatus {
  enabled: boolean;
  provider: string;
  model: string;
  source: 'feature' | 'default' | 'global' | string;
}
export interface AIStatus {
  sabbath_active: boolean;
  global_provider: string;
  global_model: string;
  redaction: boolean;
  default_provider?: string;
  features: Record<string, AIFeatureStatus>;
}

/** Long-term AI memory fact. The chat overlay folds the list into
 *  every thread's system prelude so the model knows the user's
 *  cross-thread context ("user's wife is Anna", "user is vegetarian").
 *  User-controlled — added via slash command or proposed-action chip;
 *  removable from the settings panel. */
export interface AIMemoryFact {
  id: string;
  content: string;
  tags?: string[];
  createdAt: string;
  updatedAt?: string;
}
export interface AITriageProposal {
  id: string;
  priority: number;
  schedule: 'today' | 'tomorrow' | 'this_week' | 'next_week' | 'no_date' | string;
  rationale: string;
}
export interface AIDeadlineProposal {
  id: string;
  due_date: string;
  rationale: string;
}

export interface NotificationPrefs {
  calendar: { enabled: boolean };
  tasks: { enabled: boolean; due_today_time: string };
  deadlines: { enabled: boolean; days_before: number[]; at_time: string };
  quiet_hours: { enabled: boolean; start: string; end: string };
  default_event_reminder: number;
}

export interface Email {
  id: string;
  direction: 'in' | 'out';
  subject: string;
  from: string;
  to?: string[];
  cc?: string[];
  body?: string;
  received_at?: string;
  sent_at?: string;
  status: 'inbox' | 'read' | 'replied' | 'archived';
  tags?: string[];
  follow_up_date?: string;
  person_id?: string;
  project?: string;
  created_at: string;
  updated_at: string;
}

export interface EmailSignature {
  id: string;
  name: string;
  html: string;
  /** Optional plain-text fallback for clients that strip HTML. */
  plain_text?: string;
  /** Free-text grouping — "Work", "Venture: Stoicera", etc. */
  category?: string;
  /** At most one per vault; the "use this unless I pick another" pick. */
  is_default?: boolean;
  created_at?: string;
  updated_at?: string;
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
  | 'top-deadlines'
  | 'vision'
  | 'one-thing'
  | 'ventures'
  | 'prayer'
  | 'at-a-glance'
  | 'top-goals'
  | 'quick-links'
  | 'ai-briefing'
  | 'task-velocity'
  | 'weekly-review-nudge'
  | 'ai-usage'
  | 'today-stream'
  | 'recent-annotations';

export interface DashboardWidget {
  id: string;
  type: DashboardWidgetType;
  enabled: boolean;
  config?: Record<string, unknown>;
}

// One named preset. Saved alongside the active arrangement so the user
// can switch between focus / morning / shutdown layouts without
// re-checking each widget. Backend stores them in the same
// .granit/everything-dashboard.json file under `layouts`.
export interface DashboardLayout {
  name: string;
  widgets: DashboardWidget[];
}

export interface DashboardConfig {
  version: number;
  widgets: DashboardWidget[];
  /** Name of the currently-applied layout, or "" when on an ad-hoc one. */
  active?: string;
  /** Saved layout presets — empty for users who haven't created any. */
  layouts?: DashboardLayout[];
}

// ---- helpers ----

// Re-exported from lib/util/date.ts (the canonical home). Keeping the
// names available from $lib/api so existing imports keep working
// without a touch-everything refactor.
export { fmtDateISO, todayISO } from '$lib/util/date';
