// Package modules defines the Module abstraction and Registry that let
// granit features (built-in or Lua-defined) declare their commands,
// keybinds, widgets, and dependencies in one canonical place.
//
// The Registry is the single source of truth for "what is enabled right
// now." The command palette, keybind dispatcher, and dashboard widget
// system all consult it to decide what surfaces to the user.
//
// This package has zero dependencies on internal/tui so it can be
// consumed from a Lua bridge, the settings UI, or external tools
// without dragging in the full TUI runtime.
package modules

// Origin records where a module came from. Built-in modules are
// compiled into the granit binary; Lua modules are loaded from disk.
type Origin string

const (
	OriginBuiltin Origin = "builtin"
	OriginLua     Origin = "lua"
)

// CommandRef is the data-shaped declaration of a command a module
// contributes to the palette. The string-shaped ID is what gets stored
// in keymaps and frecency tables; the adapter layer in internal/tui
// resolves IDs back to the legacy CommandAction enum at registration
// time.
//
// Keeping CommandRef free of Go-specific types (no func, no enum) is
// what lets Lua modules declare commands via a JSON manifest.
type CommandRef struct {
	ID       string // stable, e.g. "pomodoro.start"
	Label    string // display
	Desc     string // palette description
	Shortcut string // display-only hint, e.g. "Alt+P"
	Category string // palette grouping (matches tui CatXxx strings)
	IconName string // resolved to an icon char by the tui adapter
}

// Keybind binds a key chord to a command ID owned by the module.
// The When field is reserved for future contextual binds (e.g.
// "editor.focused"); Phase 1 ignores it.
type Keybind struct {
	Key       string
	CommandID string
	When      string
}

// WidgetSpec declares a dashboard widget the module can populate.
// Phase 1 stores the spec only — widget rendering ships in the
// Profiles + Daily Hub phase.
type WidgetSpec struct {
	ID    string
	Title string
}

// SettingsField describes one configurable setting a module exposes.
// The settings UI uses this to render a typed editor.
type SettingsField struct {
	Key   string
	Label string
	Type  string // "bool" | "int" | "string" | "enum"
	Enum  []string
}

// Module is the contract every feature unit satisfies. Built-in
// adapters (internal/tui/modules/*.go) and the Phase-3 Lua wrapper
// both implement this interface.
//
// All methods are intentionally side-effect free — calling Commands()
// twice must return equal slices. The registry caches results.
type Module interface {
	ID() string
	Name() string
	Description() string
	Category() string
	Origin() Origin

	Commands() []CommandRef
	Keybinds() []Keybind
	Widgets() []WidgetSpec

	DependsOn() []string
	SettingsSchema() []SettingsField
}
