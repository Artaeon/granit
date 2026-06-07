// API barrel. The HTTP client lives in ./api/client and the ~123 domain
// types in ./api/types; both are re-exported here so the ~240 existing
// `import { api, req, type Foo } from '$lib/api'` call sites keep working
// untouched. The `api` method object stays here for now.
export * from './api/client';
export type * from './api/types';
export { EVENT_STATUSES, buildRRULE } from './api/types';
import { req, reqWithEtag, getToken, ApiError } from './api/client';
import type { Note,NoteList,NoteVersion,NotesGraphNode,NotesGraphEdge,NotesGraph,VaultInfo,Jot,JotsResponse,DayActivityKind,DayActivityItem,DayActivityResponse,Task,TaskList,PlanProposal,PlanApplyProposal,CalendarEventType,CalendarEvent,EventStatus,DeadlineImportance,DeadlineStatus,Deadline,DeadlineCreate,DeadlinePatch,ProjectMilestone,ProjectGoal,ProjectKind,Project,Venture,CalendarEventEntry,EventOverride,Milestone,GoalReview,Goal,CalendarFeed,AgentPreset,AgentRun,TimerEntry,ActiveTimer,RecurringTask,OpenAIModelOption,AppConfig,AppConfigPatch,ChatMessage,Scripture,ScriptureTopic,LectionaryPlan,LectionaryDayReadings,ActiveLectionaryPlan,BibleVerse,BibleBookSummary,BiblePassage,BibleSearchHit,TranslationInfo,PassageCompareTranslation,StrongsEntry,TaggedWord,TaggedVerse,BibleBookmark,Device,FinAccountKind,FinSubCadence,FinGoalKind,FinIncomeStatus,FinIncomeKind,FinAccount,FinSubscription,FinIncomeStream,FinGoal,FinOverview,Vision,VisionHistoryEntry,VisionDoc,VisionsStore,RootsNode,Roots,AIPromptScope,AIPromptEntry,AIPromptLibrary,PlanExtractedItem,PlanExtractionResponse,PlanCommitItem,PlanCommitSkip,PlanCommitResponse,PrayerStatus,PrayerIntention,VirtueStatus,VirtueCheck,HubItem,HubCommand,HubTool,Virtue,ShoppingStatus,ShoppingCadence,ShoppingItem,ShoppingTotals,BookSummary,BookShelfRow,BookChapterMeta,BookTOCEntry,BookDetail,BookProgress,BookHighlight,BookBookmark,BookSidecar,NoteAnnotation,BookDiscoverSource,BookDiscoverResult,BookDiscoverWarning,Person,MeasurementSeries,MeasurementEntry,CalendarSource,ICSEvent,ICSEventCreate,ICSEventPatch,ICSRecurrenceFreq,ICSRecurrenceOpts,ModuleEntry,CoreModuleEntry,ModulesResponse,ProfileEntry,ProfilesResponse } from './api/types';
import type { ObjectTypeProperty,ObjectType,ObjectInstance,HabitDay,HabitInfo,HabitsResponse,MealSlot,MealsResponse,SearchHit,AIFeatureConfig,AIPreferences,AIAuditEntry,AIFeatureStatus,AIStatus,WebSearchConfig,WebSearchConfigPatch,AIMemoryFact,AITriageProposal,AIDeadlineProposal,MaintenanceSuggestion,BacklinkSuggestion,OrphanNote,NotificationPrefs,SabbathSchedulePayload,Email,EmailSignature,NoteTemplate,StatEntry,VaultStats,DashboardWidgetType,DashboardWidget,DashboardLayout,DashboardConfig } from './api/types';

// Shared PUT-note shape used by both putNoteWithEtag (etag-aware) and
// putNote (etag-discarding convenience) in the api object below. Lives
// here (not in ./api/types) because it's runtime — it rides reqWithEtag
// — and sidesteps the "can't refer to `api` from inside the object
// literal" awkwardness while keeping the wire format in one place.
function putNoteRaw(path: string, body: { frontmatter?: Record<string, unknown>; body: string }, etag?: string) {
  const headers: Record<string, string> = {};
  if (etag) headers['If-Match'] = etag;
  return reqWithEtag<Note>(`/notes/${encodeURI(path)}`, {
    method: 'PUT',
    body: JSON.stringify(body),
    headers
  });
}

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
  // Same fetch but also returns the ETag for optimistic-concurrency
  // round-trips on subsequent putNote calls. The editor uses this on
  // load() so save() can pass `If-Match` and the backend can detect
  // a foreign edit between load and save (concurrent tab, TUI, sync).
  getNoteWithEtag: (path: string) => reqWithEtag<Note>(`/notes/${encodeURI(path)}`),
  // Concept-graph fetch — backs the force-directed view at /notes/graph.
  // Filters compose: a tag+folder pair narrows to notes that match
  // BOTH (plus their direct neighbours, server-side). `limit` caps
  // node count and the server keeps the highest-degree nodes when
  // clipping so the visible structure is the anchored part of the
  // web, not 300 random orphans.
  notesGraph: (params: { tag?: string; folder?: string; limit?: number } = {}) => {
    const qs = new URLSearchParams();
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined && v !== '') qs.set(k, String(v));
    }
    const suffix = qs.toString() ? `?${qs}` : '';
    return req<NotesGraph>(`/notes/graph${suffix}`);
  },
  // PUT that exposes the response ETag — used by the notes editor's
  // autosave loop so the next save's If-Match stays anchored to the
  // latest server state. `etag` here is the OPTIONAL If-Match the
  // client sends; the returned `etag` is the post-save token.
  putNoteWithEtag: putNoteRaw,
  // Convenience wrapper for callers that don't track concurrency
  // tokens (dashboard quick-capture, calendar daily edit, project
  // notes tab, etc.). Discards the returned ETag.
  putNote: async (path: string, body: { frontmatter?: Record<string, unknown>; body: string }, etag?: string) => {
    const { data } = await putNoteRaw(path, body, etag);
    return data;
  },
  createNote: (body: { path: string; frontmatter?: Record<string, unknown>; body: string }) =>
    req<Note>('/notes', { method: 'POST', body: JSON.stringify(body) }),

  // Note history — snapshots are written on every save (the backend
  // does this automatically). The chi router needed two non-history
  // prefixes (/history-version/* and /history-restore/*) because
  // wildcards must be terminal, hence the slightly awkward path
  // shape. See internal/history/history.go for the storage layout.
  listHistory: (path: string) =>
    req<{ path: string; versions: NoteVersion[] }>(`/history/${encodeURI(path)}`),
  getHistoryVersion: (path: string, timestamp: string) =>
    req<{ path: string; timestamp: string; body: string }>(
      `/history-version/${encodeURI(path)}?ts=${encodeURIComponent(timestamp)}`
    ),
  restoreHistoryVersion: (path: string, timestamp: string) =>
    req<Note>(`/history-restore/${encodeURI(path)}`, {
      method: 'POST',
      body: JSON.stringify({ timestamp })
    }),
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
    /** Assigns the new task to a project on creation. Same semantics as
     *  the PATCH handler — written to the sidecar after the markdown
     *  line lands, so the task surfaces under its project immediately. */
    projectId?: string;
    /** Recurrence hashtag tag — "daily" / "weekly" / "monthly" / "3x-week".
     *  Bundled into the create so a follow-up PATCH isn't needed; without
     *  this the new task would flicker (appear, broadcast, reload, then
     *  patch reorders it as recurrence lands). Anything outside the
     *  whitelist is silently dropped by the server. */
    recurrence?: string;
    /** When set, the new task is inserted as a subtask of the task on
     *  this 1-indexed line in `notePath`. Resulting markdown is
     *  indented one level deeper than the parent (2 spaces). */
    parentLine?: number;
  }) => req<Task>('/tasks', { method: 'POST', body: JSON.stringify(body) }),
  deleteTask: (id: string) => req<void>(`/tasks/${id}`, { method: 'DELETE' }),
  // Duplicate-pair finder — deterministic scan, no AI cost. Returns
  // open task pairs whose normalised-token Jaccard similarity is
  // above `threshold` (default 0.6, overridable via query). Cap of
  // 25 pairs server-side; client decides what to render. UI calls
  // patchTask({ done: true, triage: 'dropped' }) on the loser to
  // merge a pair down to one task.
  taskDuplicates: (opts?: { threshold?: number }) => {
    const qs = opts?.threshold !== undefined ? `?threshold=${opts.threshold}` : '';
    return req<{
      pairs: { a: Task; b: Task; similarity: number }[];
      threshold: number;
      scanned: number;
    }>(`/tasks/duplicates${qs}`);
  },

  // Daily
  daily: (date: string = 'today') => req<Note>(`/daily/${date}`),
  listJots: (params: { before?: string; limit?: number } = {}) => {
    const qs = new URLSearchParams();
    if (params.before) qs.set('before', params.before);
    if (params.limit !== undefined) qs.set('limit', String(params.limit));
    const suffix = qs.toString() ? `?${qs}` : '';
    return req<JotsResponse>(`/jots${suffix}`);
  },
  // Day activity — every item (notes, tasks, events, habits, prayer,
  // hub links, jots) anchored on a single calendar day. Powers the
  // collapsed "What happened that day" details block on each Jot
  // header + the live `## Day overview` section on daily notes.
  dayActivity: (date: string, limit?: number) => {
    const qs = new URLSearchParams({ date });
    if (limit !== undefined) qs.set('limit', String(limit));
    return req<DayActivityResponse>(`/day-activity?${qs.toString()}`);
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
  // Semantic verse search — accepts a free-text query like "verses
  // about waiting on God" and returns the catalogue verses for the
  // 1-3 topics the AI thinks match. Closed-set under the hood: the
  // model picks from existing topic ids only, so verse refs never
  // hallucinate. Empty arrays come back when the catalogue carries
  // no topic metadata (user-edited scriptures.md replaced the
  // defaults) — callers can show a fall-back hint in that case.
  scriptureSemanticSearch: (body: { query: string; limit?: number }) =>
    req<{ topics: string[]; scriptures: Scripture[]; query: string }>(
      '/scripture/semantic-search',
      { method: 'POST', body: JSON.stringify(body) }
    ),
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

  // Translation-aware reads. /bible/translations returns every bundled
  // translation (WEB by default, plus ASV / KJV / BBE if those have been
  // fetched into internal/scripture/bible/). /bible/passage-compare
  // returns the same passage rendered in N translations side by side —
  // unknown ids are skipped silently rather than erroring out the whole
  // request, so a stale client doesn't break when a translation is
  // removed.
  bibleTranslations: () =>
    req<{ translations: TranslationInfo[]; total: number }>('/bible/translations'),
  biblePassageCompare: (params: {
    book: string;
    chapter: number;
    verseFrom?: number;
    verseTo?: number;
    translations: string[];
  }) => {
    const qs = new URLSearchParams();
    qs.set('book', params.book);
    qs.set('chapter', String(params.chapter));
    if (params.verseFrom) qs.set('verseFrom', String(params.verseFrom));
    if (params.verseTo) qs.set('verseTo', String(params.verseTo));
    if (params.translations.length) qs.set('translations', params.translations.join(','));
    return req<{
      translations: PassageCompareTranslation[];
      book: string;
      chapter: number;
      verseFrom: number;
      verseTo: number;
    }>(`/bible/passage-compare?${qs.toString()}`);
  },

  // Strong's lexicon + tagged-bible word study. Both data sources are
  // optional (fetched via scripts/fetch-strongs.sh, not bundled in
  // source) so callers should hit strongsStatus() first and degrade
  // gracefully when either is absent.
  strongsStatus: () =>
    req<{ lexicon: boolean; tagged: boolean }>('/bible/strongs/status'),
  strongsEntry: (code: string) =>
    req<StrongsEntry>('/bible/strongs/' + encodeURIComponent(code)),
  taggedChapter: (book: string, chapter: number) =>
    req<{ book: string; chapter: number; verses: TaggedVerse[] }>(
      `/bible/tagged?book=${encodeURIComponent(book)}&chapter=${chapter}`
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

  // Lectionary — bundled Bible reading plans (M'Cheyne 1-year,
  // chronological NT, 90-day NT). The list endpoint returns plan
  // summaries WITHOUT readings (cheap); the detail endpoint returns
  // the full plan including every day. /plans/active joins user state
  // with today's readings for each active plan so the page renders in
  // a single round trip.
  lectionaryPlans: () =>
    req<{ plans: LectionaryPlan[]; total: number }>('/scripture/plans'),
  lectionaryPlan: (id: string) =>
    req<LectionaryPlan & { readings: LectionaryDayReadings[] }>(
      `/scripture/plans/${encodeURIComponent(id)}`
    ),
  lectionaryPlanDay: (id: string, day: number) =>
    req<LectionaryDayReadings>(
      `/scripture/plans/${encodeURIComponent(id)}/day/${day}`
    ),
  lectionaryActivePlans: () =>
    req<{ active: ActiveLectionaryPlan[]; total: number }>('/scripture/plans/active'),
  lectionaryStartPlan: (id: string) =>
    req<{ active: ActiveLectionaryPlan[]; total: number }>(
      `/scripture/plans/${encodeURIComponent(id)}/start`,
      { method: 'POST' }
    ),
  lectionaryStopPlan: (id: string) =>
    req<{ active: ActiveLectionaryPlan[]; total: number }>(
      `/scripture/plans/${encodeURIComponent(id)}/start`,
      { method: 'DELETE' }
    ),
  lectionaryScheduleToday: (id: string) =>
    req<{ task: Task; day: number; passages: string[] }>(
      `/scripture/plans/${encodeURIComponent(id)}/schedule-today`,
      { method: 'POST' }
    ),

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
  // Weekly plan — freeform brain-dump → AI-extracted proposal of
  // tasks/milestones → user accepts subset → commit creates the
  // canonical plan note at Plans/<weekISO>.md with tasks under
  // "### <Venture>" sections.
  extractPlan: (body: { plan_text: string; week_iso?: string }) =>
    req<PlanExtractionResponse>('/plans/extract', {
      method: 'POST', body: JSON.stringify(body)
    }),
  commitPlan: (body: { plan_text: string; week_iso?: string; items: PlanCommitItem[] }) =>
    req<PlanCommitResponse>('/plans/commit', {
      method: 'POST', body: JSON.stringify(body)
    }),

  // AI prompt library — user-curated saved prompts, surfaced as a
  // "Library" category in the inline AI menu + a "save prompt" entry
  // on user chat messages. Single record per vault.
  getAIPrompts: () => req<AIPromptLibrary>('/ai/prompts'),
  putAIPrompts: (lib: { entries: Partial<AIPromptEntry>[] }) =>
    req<AIPromptLibrary>('/ai/prompts', { method: 'PUT', body: JSON.stringify(lib) }),

  getRoots: () => req<Roots>('/roots'),
  putRoots: (r: { center?: string; anchor?: string; nodes?: Partial<RootsNode>[] }) =>
    req<Roots>('/roots', { method: 'PUT', body: JSON.stringify(r) }),
  putVision: (v: Partial<Vision>) =>
    req<Vision>('/vision', { method: 'PUT', body: JSON.stringify(v) }),

  // Visions (plural) — multi-document catalogue with per-doc edit
  // history + reasons. The PUT endpoint requires `reason` in the
  // body and 400s without it; that's the whole point of the feature
  // (intentional, reviewable vision edits).
  listVisions: () => req<VisionsStore>('/visions'),
  getVisionDoc: (key: string) => req<VisionDoc>(`/visions/${encodeURIComponent(key)}`),
  putVisionDoc: (key: string, body: { content: string; reason: string }) =>
    req<VisionDoc>(`/visions/${encodeURIComponent(key)}`, {
      method: 'PUT',
      body: JSON.stringify(body)
    }),
  createVisionDoc: (body: { key: string; label: string; content?: string; reason?: string }) =>
    req<VisionDoc>('/visions', { method: 'POST', body: JSON.stringify(body) }),
  pinVisionDoc: (key: string) =>
    req<VisionsStore>(`/visions/${encodeURIComponent(key)}/pin`, { method: 'POST' }),

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

  // Meals — daily-note `## Meals` section. listMeals merges parsed
  // rows with the user's default slots; patchMeal upserts a single
  // slot (done flag + optional free-text "what I ate"). Date defaults
  // to today on both endpoints when omitted.
  listMeals: (date?: string) => {
    const q = date ? `?date=${encodeURIComponent(date)}` : '';
    return req<MealsResponse>(`/meals${q}`);
  },
  patchMeal: (slot: {
    time: string;
    name?: string;
    date?: string;
    done?: boolean;
    text?: string;
  }) =>
    req<MealsResponse>('/meals', {
      method: 'PATCH',
      body: JSON.stringify(slot)
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
  // Habit stacking — anchor `name` to `after` ("after I do <after>,
  // I do <name>"). Empty `after` clears the anchor. Server rejects
  // self-references. Writes the .granit/habits-stacks.json sidecar.
  setHabitStack: (name: string, after: string) =>
    req<{ name: string; after: string }>(
      `/habits/${encodeURIComponent(name)}/stack`,
      { method: 'PUT', body: JSON.stringify({ after }) }
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
  // Web research settings — per-vault provider choice + Brave key.
  // The Brave key is write-only on the wire; reads only carry a
  // `brave_key_set` flag so a settings refresh doesn't leak the
  // secret into the network tab.
  getWebSearchConfig: () => req<WebSearchConfig>('/ai/web-search'),
  patchWebSearchConfig: (p: WebSearchConfigPatch) =>
    req<WebSearchConfig>('/ai/web-search', {
      method: 'PATCH',
      body: JSON.stringify(p)
    }),
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

  // Vault maintenance — weekly digest (streamed) + orphan rescue
  // + per-suggestion apply. The weekly-digest endpoint serves SSE
  // with three event kinds: `suggestion` (data is one MaintenanceSuggestion
  // JSON object), `done` (data is {count}), `error` (data is {message}).
  // Returns an AbortController so the caller can cancel mid-stream
  // — mirrors the chatStream contract.
  maintenanceWeeklyDigest: (
    handlers: {
      onSuggestion: (s: MaintenanceSuggestion) => void;
      onDone?: () => void;
      onError?: (err: Error) => void;
    },
    options?: { lookbackDays?: number; maxSuggestions?: number }
  ): AbortController => {
    const ctrl = new AbortController();
    (async () => {
      const headers = new Headers({ 'Content-Type': 'application/json' });
      const tok = getToken();
      if (tok) headers.set('Authorization', `Bearer ${tok}`);
      let res: Response;
      try {
        res = await fetch('/api/v1/maintenance/weekly-digest', {
          method: 'POST',
          headers,
          body: JSON.stringify(options ?? {}),
          signal: ctrl.signal
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
      // SSE parser — same hand-rolled shape as chatStream. Each event
      // is a `field: value` block terminated by a blank line. We only
      // care about `event` (suggestion/done/error) + `data` (JSON).
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
          if (event === 'error') {
            const parsed = JSON.parse(data) as { message?: string };
            handlers.onError?.(new Error(parsed.message ?? 'stream error'));
          } else if (event === 'done') {
            handlers.onDone?.();
          } else if (event === 'suggestion') {
            const parsed = JSON.parse(data) as MaintenanceSuggestion;
            handlers.onSuggestion(parsed);
          }
        } catch {
          // Malformed event — skip rather than abort the whole stream.
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
        if (dataLines.length > 0) flush();
      } catch (e) {
        if (e instanceof DOMException && e.name === 'AbortError') return;
        handlers.onError?.(e instanceof Error ? e : new Error(String(e)));
      }
    })();
    return ctrl;
  },

  maintenanceOrphans: (opts?: { suggest?: boolean }) => {
    const qs = opts?.suggest ? '?suggest=1' : '';
    return req<{ orphans: OrphanNote[] }>(`/maintenance/orphans${qs}`);
  },

  maintenanceApply: (s: MaintenanceSuggestion) =>
    req<{ ok: boolean; next?: string; target?: string }>(
      '/maintenance/apply-suggestion',
      { method: 'POST', body: JSON.stringify(s) }
    ),

  // Notification preferences — per-category toggles, quiet
  // hours, defaults. Stored at .granit/notifications.json.
  getNotificationPrefs: () =>
    req<{ prefs: NotificationPrefs; warning?: string }>('/notifications/prefs'),
  putNotificationPrefs: (prefs: NotificationPrefs) =>
    req<{ prefs: NotificationPrefs }>('/notifications/prefs', {
      method: 'PUT',
      body: JSON.stringify(prefs)
    }),

  // Sabbath — schedule synced server-side so the rule set on one
  // device shows up on all devices. Daily on/off (active_on) is
  // mirrored too for server-side gating, but the UI activation
  // decision stays per-device in localStorage.
  getSabbath: () =>
    req<{
      active_on: string;
      skip_on: string;
      active_now: boolean;
      active_today: boolean;
      remaining_minutes: number;
      schedule: SabbathSchedulePayload;
    }>('/sabbath'),
  putSabbath: (body: { active_on?: string; skip_on?: string; schedule?: SabbathSchedulePayload }) =>
    req<{
      active_on: string;
      skip_on: string;
      active_now: boolean;
      active_today: boolean;
      remaining_minutes: number;
      schedule: SabbathSchedulePayload;
    }>('/sabbath', {
      method: 'PUT',
      body: JSON.stringify(body)
    }),
  getSabbathLog: () =>
    req<{ entries: { at: string; event: 'begin' | 'end' }[] }>('/sabbath/log'),

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
    req<ModulesResponse>('/modules', { method: 'PUT', body: JSON.stringify({ enabled: patch }) }),

  // Profiles — workflow-context bundles. Activating one persists the
  // active-profile pointer server-side AND applies the profile's
  // EnabledModules to the modules registry as a side effect.
  listProfiles: () => req<ProfilesResponse>('/profiles'),
  activateProfile: (id: string) =>
    req<ProfilesResponse>(`/profiles/${encodeURIComponent(id)}/activate`, { method: 'POST' })
};


// ---- helpers ----

// Re-exported from lib/util/date.ts (the canonical home). Keeping the
// names available from $lib/api so existing imports keep working
// without a touch-everything refactor.
export { fmtDateISO, todayISO } from '$lib/util/date';
