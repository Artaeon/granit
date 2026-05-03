package modules

// Boot constructs a Registry rooted at the given vault path, registers
// the baseline web-surface module declarations, and calls Load so any
// persisted enable-state from .granit/modules.json is honored.
//
// The TUI command path and serveapi.Server both call Boot so they
// target the same on-disk state file. The TUI then layers its own
// builtin module declarations on top via tui.RegisterBuiltins; web
// only consults the registry for enable-state, so registering the
// baseline declarations here is enough for the web surface to gate
// nav, route guards, and dashboard widgets.
//
// Each declaration is a name+description+category-only stub. The real
// behavior for these IDs lives in the web routes / TUI overlays
// already; the registry is purely the on/off switch. When the TUI
// later grows real module adapters for these IDs (deferred), they can
// Register over the stubs by ID (Register would error on duplicates,
// so the TUI path needs to skip baseline IDs the TUI owns natively).
func Boot(vaultRoot string) (*Registry, error) {
	r := New(vaultRoot)
	for _, decl := range baselineDeclarations() {
		if err := r.Register(decl); err != nil {
			return nil, err
		}
	}
	if err := r.Load(); err != nil {
		return nil, err
	}
	return r, nil
}

// baselineModule is the data-only Module satisfier used for stubs
// boot registers before the TUI adds its richer adapters. Keeping the
// shape minimal (no commands, no keybinds, no widgets) is intentional:
// these declarations exist purely to give the registry a known ID +
// human-readable name + category for the settings UI to render.
type baselineModule struct {
	id   string
	name string
	desc string
	cat  string
}

func (b *baselineModule) ID() string                      { return b.id }
func (b *baselineModule) Name() string                    { return b.name }
func (b *baselineModule) Description() string             { return b.desc }
func (b *baselineModule) Category() string                { return b.cat }
func (b *baselineModule) Origin() Origin                  { return OriginBuiltin }
func (b *baselineModule) Commands() []CommandRef          { return nil }
func (b *baselineModule) Keybinds() []Keybind             { return nil }
func (b *baselineModule) Widgets() []WidgetSpec           { return nil }
func (b *baselineModule) DependsOn() []string             { return nil }
func (b *baselineModule) SettingsSchema() []SettingsField { return nil }

// baselineDeclarations is the canonical web-surface module list.
// Snake_case IDs match the existing TUI registry convention (e.g.
// habit_tracker). The four core IDs in CoreIDs are NOT in this list —
// they're surfaced separately via /api/v1/modules so the UI can render
// them as always-on.
func baselineDeclarations() []Module {
	return []Module{
		&baselineModule{id: "projects", name: "Projects", desc: "Project tracking and progress", cat: "Planning"},
		&baselineModule{id: "agents", name: "AI Agents", desc: "Agent runs, presets, and run history", cat: "AI"},
		&baselineModule{id: "objects", name: "Objects", desc: "Typed objects and the type registry", cat: "Knowledge"},
		&baselineModule{id: "morning", name: "Morning Routine", desc: "Daily morning checkin with tasks, habits, and reflection", cat: "Daily"},
		&baselineModule{id: "jots", name: "Jots", desc: "Daily-note feed and quick entries", cat: "Daily"},
		&baselineModule{id: "scripture", name: "Scripture", desc: "Verse of the day and devotional notes", cat: "Knowledge"},
		&baselineModule{id: "deadlines", name: "Deadlines", desc: "Top-level dated commitments", cat: "Planning"},
		&baselineModule{id: "goals", name: "Goals", desc: "Long-term goals with milestones and reviews", cat: "Planning"},
		&baselineModule{id: "chat", name: "AI Chat", desc: "Multi-turn AI chat", cat: "AI"},
		// habit_tracker matches the ID the TUI already registers. The
		// web exposes it via the friendly path /habits but the canonical
		// ID stays habit_tracker.
		&baselineModule{id: "habit_tracker", name: "Habit Tracker", desc: "Daily habits, streaks, and progress", cat: "Daily"},
		&baselineModule{id: "finance", name: "Finance", desc: "Accounts, subscriptions, income streams, money goals", cat: "Life"},
		&baselineModule{id: "people", name: "People", desc: "Lightweight relationship tracker — last contact, birthdays, cadence reminders", cat: "Life"},
		&baselineModule{id: "prayer", name: "Prayer", desc: "Active prayer intentions with status lifecycle. Lives in /scripture#intentions.", cat: "Spiritual"},
		&baselineModule{id: "measurements", name: "Measurements", desc: "Numeric tracking — weight, sleep, exercise reps, mood, anything", cat: "Life"},
	}
}

// CoreIDs lists the module IDs the user cannot disable. Surfaced by
// the /api/v1/modules handler so the settings UI can render them with
// a lock icon. Not registered as Modules — a core ID isn't a toggleable
// declaration, it's a constant of the product.
var CoreIDs = []string{"notes", "tasks", "calendar", "settings"}

// CoreNames is the matching display table for CoreIDs.
var CoreNames = map[string]string{
	"notes":    "Notes",
	"tasks":    "Tasks",
	"calendar": "Calendar",
	"settings": "Settings",
}
