// Package profiles owns granit's Profile abstraction — the bundle
// of (enabled modules + dashboard layout + templates + default bot
// + keybind overrides) that switches the whole app between
// workflow contexts: Classic, Daily Operator, Researcher, Builder,
// or any user-authored fork.
//
// A Profile is pure data. The package depends only on stdlib +
// internal/atomicio so it stays Lua-implementable later (no
// internal/tui types leak into the manifest). The TUI imports
// profiles and adapts the string-shaped fields (module IDs,
// layout names, widget IDs) to its concrete types at the boundary.
//
// Profile state lives in three layers, resolved in order:
//
//  1. Built-in (compiled into the binary, never on disk)
//  2. ~/.config/granit/profiles/<id>.json (user-global)
//  3. <vault>/.granit/profiles/<id>.json (vault-local)
//
// Same ID across layers: later wins. The vault-local active
// profile lives at <vault>/.granit/active-profile (one-line text,
// just the ID — easy to inspect by hand).
package profiles

// Profile is the manifest a registered profile presents. JSON-shaped
// so a Lua plugin can build one from a table without importing Go.
type Profile struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Description     string            `json:"description,omitempty"`
	BuiltIn         bool              `json:"-"` // computed at register time

	// EnabledModules lists module IDs that this profile keeps on.
	// Empty slice has special meaning: "all modules enabled"
	// (Classic semantics) — distinguishes from a profile that
	// explicitly enables zero modules (use a sentinel like
	// "__none__" if you need that, or just don't author it).
	EnabledModules []string `json:"enabled_modules"`

	// DefaultLayout is one of the tui layout name constants
	// (LayoutDefault, LayoutWriter, etc.). The TUI's profile-apply
	// step honors this only when the user hasn't explicitly
	// overridden cfg.Layout — user choice always wins.
	DefaultLayout string `json:"default_layout,omitempty"`

	// Dashboard specifies the Daily Hub widget grid for this
	// profile. Zero-value (no cells) means "no dashboard" — the
	// profile boots straight to the editor like Classic does
	// today.
	Dashboard DashboardSpec `json:"dashboard,omitempty"`

	// Templates are optional file paths under
	// <vault>/.granit/templates/ used as defaults for the jot,
	// note creation, and project init flows. Inline strings
	// starting with "@inline:" are taken literally instead of
	// loaded from disk.
	Templates TemplatesSpec `json:"templates,omitempty"`

	// DefaultBot is the AI bot ID surfaced as the default
	// assistant for this profile. Empty means "use cfg.Bot".
	DefaultBot string `json:"default_bot,omitempty"`

	// KeybindOverride lets a profile remap chord → command_id.
	// Power users hand-edit the JSON; v1 ships no GUI editor.
	KeybindOverride map[string]string `json:"keybind_overrides,omitempty"`
}

// DashboardSpec is the layout for the Daily Hub. The grid is
// always 12 columns wide (CSS-grid convention so power users can
// reason about it without trial-and-error). Row count is inferred
// from the maximum (Row + RowSpan) across cells. Below ~80 cols of
// terminal width the grid linearizes into a single column.
type DashboardSpec struct {
	Cells []DashboardCell `json:"cells,omitempty"`
}

// DashboardCell places one widget at a grid position. Span
// defaults of 0 are treated as 1 by the renderer so the JSON stays
// terse. Config carries widget-specific knobs (e.g. how many
// recent notes to show, which calendar to display).
type DashboardCell struct {
	WidgetID string         `json:"widget_id"`
	Row      int            `json:"row"`
	Col      int            `json:"col"`
	RowSpan  int            `json:"row_span,omitempty"`
	ColSpan  int            `json:"col_span,omitempty"`
	Config   map[string]any `json:"config,omitempty"`
}

// TemplatesSpec lists optional templates the profile prefers.
// Empty means "use the global cfg defaults."
type TemplatesSpec struct {
	Jot     string `json:"jot,omitempty"`
	Note    string `json:"note,omitempty"`
	Project string `json:"project,omitempty"`
}
