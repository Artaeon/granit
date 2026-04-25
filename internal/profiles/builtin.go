package profiles

// Built-in widget IDs. Concentrated here so builtin profile
// definitions reference symbolic constants, not bare strings —
// makes a typo loud at compile time and lets the widget registry
// (commit 4) cross-reference. Lua-defined widgets use their own
// IDs; the Daily Hub controller resolves both via the same
// registry.
const (
	WidgetTodayJot         = "today.jot"
	WidgetTodayCalendar    = "today.calendar"
	WidgetTodayTasks       = "today.tasks"
	WidgetTodayOverdue     = "today.overdue"
	WidgetTriageCount      = "triage.count"
	WidgetGoalProgress     = "goal.progress"
	WidgetHabitStreak      = "habit.streak"
	WidgetRecentNotes      = "recent.notes"
	WidgetScripture        = "dashboard.scripture"
	WidgetBusinessPulse    = "dashboard.businesspulse"
)

// LayoutDefault et al. mirror the layout name constants in tui to
// avoid a circular import. Keep these in sync — there's a contract
// test in builtin_test.go that imports tui to verify, so a drift
// caught early rather than at runtime.
const (
	layoutDefault    = "default"
	layoutDashboard  = "dashboard"
	layoutResearch   = "research"
	layoutCockpit    = "cockpit"
)

// BuiltinProfiles returns the 4 profiles compiled into the
// binary. Order matters — Classic first so it surfaces at the
// top of any picker UI (it's the most common choice and the
// migration default for existing vaults).
//
// EnabledModules semantics:
//
//   - Empty slice: all modules stay enabled (Classic). The
//     profile-apply step skips SetEnabled calls entirely.
//   - Non-empty slice: every registered module is set on/off
//     based on membership in this list. Unknown module IDs in
//     the list are recorded but no-op until that module
//     registers (Lua plugin loads, future built-in ships, etc.).
//
// Many of the module IDs referenced by Daily Operator,
// Researcher, and Builder don't exist as registered modules yet
// — they ship as Module Registry entries in later phases. The
// profile manifests are forward-looking on purpose: the moment
// daily_jot becomes a module, profiles that list it pick it up
// automatically without a re-edit.
func BuiltinProfiles() []*Profile {
	return []*Profile{
		classicProfile(),
		dailyOperatorProfile(),
		researcherProfile(),
		builderProfile(),
	}
}

// classicProfile mirrors the pre-relaunch experience: every module
// enabled, default layout, dashboard widgets that match what the
// existing dashboard.go overlay shows today (today's tasks,
// overdue, scripture, business pulse) plus recent notes for a bit
// more reach. Existing vaults migrate into this profile on first
// boot of the relaunch and see no surprises.
func classicProfile() *Profile {
	return &Profile{
		ID:             DefaultProfileID,
		Name:           "Classic",
		Description:    "The pre-relaunch granit. Every module enabled, the existing dashboard, the same keybinds. Migration-safe default.",
		EnabledModules: nil, // empty = "leave all modules alone"
		DefaultLayout:  layoutDefault,
		Dashboard: DashboardSpec{
			Cells: []DashboardCell{
				// Row 0 — scripture is verse-wide; recent notes hugs the right edge.
				{WidgetID: WidgetScripture, Row: 0, Col: 0, ColSpan: 8},
				{WidgetID: WidgetRecentNotes, Row: 0, Col: 8, ColSpan: 4},
				// Row 1 — today/overdue task split.
				{WidgetID: WidgetTodayTasks, Row: 1, Col: 0, ColSpan: 6},
				{WidgetID: WidgetTodayOverdue, Row: 1, Col: 6, ColSpan: 6},
				// Row 2 — business pulse is a trend chart, wants full width.
				{WidgetID: WidgetBusinessPulse, Row: 2, Col: 0, ColSpan: 12},
			},
		},
	}
}

// dailyOperatorProfile is the planning-loop front door. Capture
// (jot), see today (calendar + tasks), keep triage on the radar
// (inbox count), surface streak/goal context to nudge focus.
func dailyOperatorProfile() *Profile {
	return &Profile{
		ID:          "daily_operator",
		Name:        "Daily Operator",
		Description: "Capture-Triage-Schedule-Execute loop. The default for daily planning ritual.",
		EnabledModules: []string{
			"pomodoro",
			"task_manager",
			"habit_tracker",
			"recurring_tasks",
			"daily_jot",
			"plan_my_day",
			"focus_session",
			"time_tracker",
			"calendar",
		},
		DefaultLayout: layoutCockpit,
		Dashboard: DashboardSpec{
			Cells: []DashboardCell{
				// Row 0 — jot is the front door (focus lands here on open).
				{WidgetID: WidgetTodayJot, Row: 0, Col: 0, ColSpan: 8},
				{WidgetID: WidgetTodayCalendar, Row: 0, Col: 8, ColSpan: 4},
				// Row 1 — today's plan + triage queue + habit ping.
				{WidgetID: WidgetTodayTasks, Row: 1, Col: 0, ColSpan: 6},
				{WidgetID: WidgetTriageCount, Row: 1, Col: 6, ColSpan: 2},
				{WidgetID: WidgetHabitStreak, Row: 1, Col: 8, ColSpan: 4},
				// Row 2 — goal context, full width so bars are readable.
				{WidgetID: WidgetGoalProgress, Row: 2, Col: 0, ColSpan: 12},
			},
		},
	}
}

// researcherProfile foregrounds knowledge work. Less task chrome,
// more substrate: jot, recent notes, scripture for a daily
// centering moment, triage count to keep capture flowing without
// derailing the deep-work focus.
func researcherProfile() *Profile {
	return &Profile{
		ID:          "researcher",
		Name:        "Researcher",
		Description: "Notes, graph, AI tooling foregrounded. For deep work and synthesis sessions.",
		EnabledModules: []string{
			// Universal personal tools — always reachable via
			// palette / direct shortcut regardless of profile.
			// Without these in the list, switching to Researcher
			// silently disabled habits / calendar / tasks etc.
			// and the user couldn't find them via Ctrl+X.
			"task_manager",
			"habit_tracker",
			"calendar",
			"daily_jot",
			"pomodoro",
			"focus_session",
			"recurring_tasks",
			// Profile focus: research / synthesis tooling.
			"knowledge_graph",
			"smart_connections",
			"semantic_search",
			"ai_templates",
			"research_agent",
			"ai_chat",
			"link_assist",
			"mind_map",
			"dataview",
		},
		DefaultLayout: layoutResearch,
		Dashboard: DashboardSpec{
			Cells: []DashboardCell{
				// Row 0 — capture front and center; recent notes for re-entry.
				{WidgetID: WidgetTodayJot, Row: 0, Col: 0, ColSpan: 8},
				{WidgetID: WidgetRecentNotes, Row: 0, Col: 8, ColSpan: 4},
				// Row 1 — daily centering + light task awareness.
				{WidgetID: WidgetScripture, Row: 1, Col: 0, ColSpan: 8},
				{WidgetID: WidgetTriageCount, Row: 1, Col: 8, ColSpan: 4},
			},
		},
	}
}

// builderProfile is for shipping work: projects, kanban, goals,
// standup. Less daily-ritual chrome, more execution surface.
func builderProfile() *Profile {
	return &Profile{
		ID:          "builder",
		Name:        "Builder",
		Description: "Projects, kanban, goals, standup. For PMs and engineers shipping deliverables.",
		EnabledModules: []string{
			// Universal personal tools — always reachable via
			// palette / direct shortcut regardless of profile.
			"task_manager",
			"habit_tracker",
			"calendar",
			"daily_jot",
			"pomodoro",
			"focus_session",
			"recurring_tasks",
			// Profile focus: project / shipping tooling.
			"kanban",
			"project_mode",
			"goals_mode",
			"standup_generator",
			"blog_publisher",
			"ai_project_planner",
		},
		DefaultLayout: layoutDashboard,
		Dashboard: DashboardSpec{
			Cells: []DashboardCell{
				// Row 0 — capture + today's queue.
				{WidgetID: WidgetTodayJot, Row: 0, Col: 0, ColSpan: 8},
				{WidgetID: WidgetTodayTasks, Row: 0, Col: 8, ColSpan: 4},
				// Row 1 — what's the goal + what changed recently.
				{WidgetID: WidgetGoalProgress, Row: 1, Col: 0, ColSpan: 8},
				{WidgetID: WidgetRecentNotes, Row: 1, Col: 8, ColSpan: 4},
			},
		},
	}
}

// RegisterBuiltins puts the 4 profiles into the registry and
// flags them as built-in. Returns the first registration error,
// if any (programmer error — should never happen at runtime).
func RegisterBuiltins(r *ProfileRegistry) error {
	for _, p := range BuiltinProfiles() {
		if err := r.Register(p); err != nil {
			return err
		}
		r.MarkBuiltIn(p.ID)
	}
	return nil
}
