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

// One historical snapshot of a note's body. Surfaced by
// api.listHistory; the body itself is fetched lazily via
// api.getHistoryVersion when the user selects an entry to preview.
export interface NoteVersion {
  /** ISO-8601 UTC timestamp of when the prior content was snapped. */
  timestamp: string;
  /** Byte length of the snapshotted content (for the size badge). */
  size: number;
  /** First 16 hex chars of SHA-256(content) — short fingerprint and
   *  dedup hint. */
  hash: string;
}

// Concept-graph payload — whole-vault wikilink network shaped for the
// force-directed view at /notes/graph. `degree` is the GLOBAL degree
// (computed before the limit-clip on the server) so a node's visual
// size matches how connected it really is, even when half its
// neighbours fell off the visible set.
export interface NotesGraphNode {
  id: string;
  title: string;
  path: string;
  degree: number;
  tags?: string[];
}

export interface NotesGraphEdge {
  source: string;
  target: string;
}

export interface NotesGraph {
  nodes: NotesGraphNode[];
  edges: NotesGraphEdge[];
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

// Day-activity — every item created/completed/touched on a single
// calendar day. The server (internal/dayactivity) is the source of
// truth; this matches the JSON shape it emits. Kind is open-ended
// so a future server-side addition (e.g. 'measurement') round-trips
// without a typescript regen.
export type DayActivityKind =
  | 'note_created'
  | 'task_created'
  | 'task_completed'
  | 'event'
  | 'habit'
  | 'prayer'
  | 'jot'
  | 'hub_item'
  | string;

export interface DayActivityItem {
  kind: DayActivityKind;
  at: string;
  title: string;
  detail?: string;
  path?: string;
  target_id?: string;
}

export interface DayActivityResponse {
  date: string;
  items: DayActivityItem[];
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
  | 'deadline'
  | 'meal_slot'
  | 'goal_target';

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
  /** Content-pipeline fields. Only meaningful when kind==='content';
   *  silently ignored for every other kind. Status drives the chip
   *  letter (D/R/S/P) on calendar cards and the kanban grouping in
   *  the pipeline overlay. Channels[0] drives the week-view swim
   *  lane assignment. Tags is general-purpose cross-event grouping.
   *
   *  Status is typed `string` (not `EventStatus`) because the wire
   *  carries any string the backend stores — including ''  to clear
   *  on PATCH — and the UI treats unknown values as 'no status'.
   *  EVENT_STATUSES is the picker vocabulary; the type is loose so
   *  PATCH payloads with empty-string clear don't need a cast. */
  status?: string;
  channels?: string[];
  tags?: string[];
}

/** Canonical content-pipeline stages. Stored as plain strings so the
 *  backend round-trips unknown values; this union is the UI-side
 *  expectation. The picker offers these in order; the renderer
 *  treats anything outside the union as 'no status set' (no chip). */
export type EventStatus =
  | 'idea'
  | 'drafting'
  | 'review'
  | 'scheduled'
  | 'published'
  | 'archived';

export const EVENT_STATUSES: readonly EventStatus[] = [
  'idea',
  'drafting',
  'review',
  'scheduled',
  'published',
  'archived'
] as const;

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
  /** Content-pipeline fields — only meaningful when kind==='content'.
   *  Status drives the pipeline-overlay column; channels[0] drives
   *  the week-view swim lane; tags is general-purpose grouping.
   *  Status typed `string` for the same wire-flexibility reason as
   *  CalendarEvent.status — see EVENT_STATUSES for the picker
   *  vocabulary. */
  status?: string;
  channels?: string[];
  tags?: string[];
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

// Lectionary — bundled multi-day reading plans. The list endpoint
// returns just the summary fields (no `readings`); the detail endpoint
// fills in the full day-by-day schedule.
export interface LectionaryPlan {
  id: string;
  name: string;
  description: string;
  lengthDays: number;
  readings?: LectionaryDayReadings[];
}
export interface LectionaryDayReadings {
  day: number;       // 1-indexed
  passages: string[]; // e.g. ["Gen 1", "Matt 1", "Ezra 1", "Acts 1"]
}
// Active plan view: state (planId, startedAt) merged with the snapshot
// of today's readings + which day of the plan the user is on. Empty
// `todayPassages` means the plan has run past its end (finished=true)
// or the plan id is stale.
export interface ActiveLectionaryPlan {
  planId: string;
  planName: string;
  lengthDays: number;
  startedAt: string;       // RFC3339
  dayOfPlan: number;       // 1-indexed; may exceed lengthDays past the end
  finished: boolean;
  todayPassages: string[]; // empty when finished
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

// Per-translation metadata for the bible translation picker. Returned
// by /bible/translations. WEB is always present; ASV / KJV / BBE only
// appear when the corresponding JSON has been dropped into
// internal/scripture/bible/ via scripts/fetch-bible-translations.sh.
export interface TranslationInfo {
  id: string;           // lowercase, e.g. "web", "asv"
  name: string;         // "World English Bible"
  abbreviation: string; // "WEB"
  license: string;      // "Public Domain"
  year?: number;        // 1901 (ASV), 1611 (KJV), etc.
}

// One column of a passage-compare response. Verses are clamped to the
// requested range and (when populated) the reference is server-rendered
// against this translation's book naming.
export interface PassageCompareTranslation {
  id: string;
  name: string;
  abbreviation: string;
  reference: string;
  verses: BibleVerse[];
}

// Strong's lexicon entry — Greek (G####) or Hebrew (H####) word study
// data. Mirrors internal/scripture/bible.StrongsEntry. All fields are
// optional in upstream data; treat empty strings as "missing".
export interface StrongsEntry {
  lemma?: string;       // original-language form, e.g. "ἀγάπη"
  translit?: string;    // transliteration, e.g. "agápē"
  strongs_def?: string; // Strong's own definition
  kjv_def?: string;     // gloss of how the KJV renders the word
  derivation?: string;  // etymology / root note
}

// One word in the tagged bible. `strongs` may be empty for untagged
// glue words ("the", punctuation, etc.) depending on the upstream
// dataset's granularity — the UI just renders those as plain text.
export interface TaggedWord {
  text: string;
  strongs?: string;
}

// One verse from the tagged bible. `n` is the 1-indexed verse number
// matching the equivalent BibleVerse.n.
export interface TaggedVerse {
  n: number;
  words: TaggedWord[];
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

// Visions (plural) — multi-document catalogue. Each Doc is one
// named vision narrative (Hauptvision / Kurzversion / Mission /
// Stoicera / Körper / Glaube — plus user-defined keys) with its
// own edit history. Distinct from Vision (singular) above, which
// still owns the values list + season focus sidecar.
export interface VisionHistoryEntry {
  when: string;          // ISO timestamp of the edit
  reason: string;        // why the user changed it
  content: string;       // snapshot of the previous content
}
export interface VisionDoc {
  key: string;
  label: string;
  content?: string;
  pinned?: boolean;      // surfaced on the home page Kurzvision widget
  updated_at: string;
  history?: VisionHistoryEntry[];
}
export interface VisionsStore {
  version: number;
  docs: VisionDoc[];
}

// Roots — the "rooted in Christ" diagram. Single record per vault.
// Ring numbers: 1=Identity, 2=Callings, 3=Gifts, 4=Longings. The
// server sends ring_labels so the client never has to hard-code
// them — single source of truth lives in internal/roots.
export interface RootsNode {
  id: string;
  ring: number;
  label: string;
  description?: string;
  scripture?: string;
  related_notes?: string[];
  created_at: string;
  updated_at: string;
}

export interface Roots {
  center?: string;
  anchor?: string;
  nodes?: RootsNode[];
  updated_at: string;
  ring_labels: Record<number, string>;
}

// User-curated AI prompts. Different from auto-recents
// (RecentPrompt in lib/ai/recentPrompts.ts) which capture what the
// user typed without intent, and different from built-in presets
// which ship with the app: library entries are deliberately saved
// for reuse.
//
// scope filters where each entry surfaces in the inline AI menu:
//   selection → only with text selected (rewrite-shaped prompts)
//   cursor    → only at an empty cursor (generate-shaped prompts)
//   either    → both
export type AIPromptScope = 'selection' | 'cursor' | 'either';

export interface AIPromptEntry {
  id: string;
  label: string;
  prompt: string;
  scope: AIPromptScope;
  created_at: string;
}

export interface AIPromptLibrary {
  entries: AIPromptEntry[];
  updated_at?: string;
}

// Weekly plan extraction — what the AI returns from /plans/extract.
// match_type categorises how the AI resolved this item's parent:
//   "exact"    — venture/project name matched a vault entity verbatim
//   "fuzzy"    — close but not exact (see match_confidence 0-100)
//   "new"      — proposes a task with no existing parent (review carefully)
//   "personal" — non-venture/personal item (sermon prep, etc.)
export interface PlanExtractedItem {
  kind: 'task' | 'milestone';
  label: string;
  venture_name?: string;
  project_name?: string;
  goal_id?: string;
  due_date?: string;
  source_line?: string;
  match_type?: 'exact' | 'fuzzy' | 'new' | 'personal';
  match_confidence?: number;
  rationale?: string;
}

export interface PlanExtractionResponse {
  items: PlanExtractedItem[];
  unmatched?: string[];
  warning?: string;
  raw?: string;
}

// What the client sends back to /plans/commit after the user has
// reviewed and accepted (possibly edited) the proposal. Same shape
// as PlanExtractedItem minus the AI-only fields (match_type,
// match_confidence, rationale) since the user has already
// committed to the routing by accepting.
export interface PlanCommitItem {
  kind: 'task' | 'milestone';
  label: string;
  venture_name?: string;
  project_name?: string;
  goal_id?: string;
  due_date?: string;
  source_line?: string;
}

export interface PlanCommitSkip {
  label: string;
  reason: string;
}

export interface PlanCommitResponse {
  plan_path: string;
  created_task_ids: string[];
  created_milestones_count: number;
  skipped?: PlanCommitSkip[];
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

// Profile — workflow-context bundle (modules + dashboard + layout +
// keybinds). Switchable from the nav profile-chip; activating one
// PUTs the new active pointer AND applies the profile's enabled-
// modules set as a side-effect.
export interface ProfileEntry {
  id: string;
  name: string;
  description?: string;
  builtIn: boolean;
  enabledModules?: string[]; // empty = "all modules" (Classic semantics)
  defaultLayout?: string;
  hasDashboard?: boolean;
}
export interface ProfilesResponse {
  profiles: ProfileEntry[];
  activeId: string;
}


// ---- additional domain types ----
// Moved verbatim from api.ts (they were defined after the `api` object);
// kept here so every domain type lives in one module.

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
  // Habit stacking — when set, the name of another habit this one
  // is anchored to. "After I do <stackAfter>, I do this." Behavioural-
  // science anchoring; the UI surfaces it as a chain.
  stackAfter?: string;
}

export interface HabitsResponse {
  habits: HabitInfo[];
  total: number;
  today: string;
  days: number;
}

// Meals — one slot per planned meal in the day. Source of truth is the
// daily note's `## Meals` section; the API merges with the user's
// defaults so the client always sees the full row list.
export interface MealSlot {
  time: string; // HH:MM
  name: string;
  done: boolean;
  text?: string;
}

export interface MealsResponse {
  date: string;
  slots: MealSlot[];
  done: number;
  total: number;
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

/** Per-vault web-research config, mirroring internal/websearch.Config
 *  with one safety swap: the server never echoes the Brave key back
 *  to the client, so we receive a `brave_key_set` boolean instead. To
 *  paste a new key the user PATCHes { brave_key: 'sk-…' }; to clear,
 *  PATCH { brave_key: '' }. */
export interface WebSearchConfig {
  provider: string;
  brave_key_set: boolean;
  max_results: number;
}
export interface WebSearchConfigPatch {
  provider?: string;
  /** Empty string clears the stored key; non-empty replaces it. */
  brave_key?: string;
  max_results?: number;
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

// Vault-maintenance suggestion union. The server emits these as
// NDJSON over SSE for the weekly-digest endpoint, and accepts the
// same shape POSTed at the apply endpoint. Discriminated by `kind`
// — TypeScript narrows each branch when a switch on `kind` is used,
// which is exactly how the maintenance page handles them.
export type MaintenanceSuggestion =
  | { kind: 'merge'; notes: string[]; reason?: string }
  | {
      kind: 'retitle';
      note: string;
      currentTitle?: string;
      suggestedTitle: string;
      reason?: string;
    }
  | {
      kind: 'missing-tags';
      note: string;
      suggestedTags: string[];
      reason?: string;
    }
  | {
      kind: 'add-backlink';
      fromNotePath: string;
      toNotePath: string;
      anchorText?: string;
    };

export interface BacklinkSuggestion {
  from: string;
  excerpt?: string;
}

export interface OrphanNote {
  path: string;
  title: string;
  modTime: string;
  suggestedBacklinks?: BacklinkSuggestion[];
}

export interface NotificationPrefs {
  calendar: { enabled: boolean };
  tasks: { enabled: boolean; due_today_time: string };
  deadlines: { enabled: boolean; days_before: number[]; at_time: string };
  quiet_hours: { enabled: boolean; start: string; end: string };
  default_event_reminder: number;
}

// Wire-format schedule shape — matches internal/sabbath.Schedule
// JSON tags exactly. The local SabbathSchedule alias in the store
// uses camelCase for ergonomic TS; this is the over-the-wire shape.
export interface SabbathSchedulePayload {
  enabled: boolean;
  day_of_week: number;     // 0=Sunday … 6=Saturday
  start_hour: number;      // 0-23
  start_minute: number;    // 0-59
  duration_minutes: number; // 1440 = 24h
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

// Widget types the client knows about. Removed types (16 of them — see
// the 2026-05-23 cleanup commit) were dropped from this union AND from
// the runtime registry. An existing saved config with one of the
// removed types still parses (the dashboardWidget Type field is a
// plain string on the server) but the client filters them out via
// widgetMeta() returning undefined, and the load-time migration in
// +page.svelte's load() actively strips them from the saved config
// so the file stays clean over time.
export type DashboardWidgetType =
  | 'greeting'
  | 'daily-note'
  | 'quick-capture'
  | 'today-tasks'
  | 'scheduled-today'
  | 'goals-progress'
  | 'recent-notes'
  | 'projects-active'
  | 'inbox'
  | 'habits'
  | 'now'
  | 'streaks'
  | 'scripture'
  | 'today-focus'
  | 'ventures'
  | 'prayer'
  | 'today-stream'
  | 'sabbath'
  | 'roots'
  | 'weekly-plan'
  | 'meals'
  | 'tagesordnung'
  | 'kurzvision';

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
