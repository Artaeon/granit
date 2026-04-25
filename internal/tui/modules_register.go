package tui

import (
	"github.com/artaeon/granit/internal/modules"
)

// builtinModule is the in-binary adapter that satisfies modules.Module
// via fixed-data fields. The legacy CommandAction enum is kept off the
// modules.Module interface (so a Lua wrapper can satisfy it verbatim
// without importing tui), and stored here on the adapter instead.
//
// One file per migrated feature lives under internal/tui/modules/ and
// produces a *builtinModule via the helper constructors below.
type builtinModule struct {
	id       string
	name     string
	desc     string
	cat      string
	deps     []string
	cmds     []modules.CommandRef
	keys     []modules.Keybind
	widgets  []modules.WidgetSpec
	settings []modules.SettingsField
	// actions maps CommandRef.ID → CommandAction so the dispatcher can
	// resolve a registry-routed keybind back to the legacy execution
	// path. Phase 1 reuses the existing executeCommand switch; later
	// phases may move dispatch into modules.
	actions map[string]CommandAction
}

func (b *builtinModule) ID() string                              { return b.id }
func (b *builtinModule) Name() string                            { return b.name }
func (b *builtinModule) Description() string                     { return b.desc }
func (b *builtinModule) Category() string                        { return b.cat }
func (b *builtinModule) Origin() modules.Origin                  { return modules.OriginBuiltin }
func (b *builtinModule) Commands() []modules.CommandRef          { return b.cmds }
func (b *builtinModule) Keybinds() []modules.Keybind             { return b.keys }
func (b *builtinModule) Widgets() []modules.WidgetSpec           { return b.widgets }
func (b *builtinModule) DependsOn() []string                     { return b.deps }
func (b *builtinModule) SettingsSchema() []modules.SettingsField { return b.settings }

// builtinRegistration bundles a built-in module with its
// CommandAction map so RegisterBuiltins can build the reverse lookups
// the dispatcher needs.
type builtinRegistration struct {
	mod     *builtinModule
	actions map[string]CommandAction
}

// allBuiltins lists every built-in module to register at startup.
// Each pilot/migration commit appends one entry. Order here is the
// order surfaced in the settings UI.
func allBuiltins() []builtinRegistration {
	return []builtinRegistration{
		pomodoroModule(),
		flashcardsModule(),
	}
}

// RegisterBuiltins registers every compiled-in module with the
// registry and returns the (CommandAction → moduleID) reverse map the
// command palette uses to filter out commands owned by disabled
// modules.
//
// Unmigrated commands have no entry in the returned map and stay
// visible — the registry's default-on-unknown semantics apply at the
// palette layer too.
func RegisterBuiltins(reg *modules.Registry) (cmdActionToModuleID map[CommandAction]string) {
	cmdActionToModuleID = make(map[CommandAction]string)
	for _, b := range allBuiltins() {
		if err := reg.Register(b.mod); err != nil {
			// Duplicate IDs in the built-in list are a programmer
			// error — surface loudly in dev rather than silently
			// shadowing.
			panic("modules: RegisterBuiltins: " + err.Error())
		}
		for _, action := range b.actions {
			cmdActionToModuleID[action] = b.mod.ID()
		}
	}
	return cmdActionToModuleID
}
