package modules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// fakeModule is a test double that satisfies Module via fixed data.
type fakeModule struct {
	id       string
	name     string
	cat      string
	deps     []string
	cmds     []CommandRef
	keys     []Keybind
	widgets  []WidgetSpec
	settings []SettingsField
}

func (f *fakeModule) ID() string                      { return f.id }
func (f *fakeModule) Name() string                    { return f.name }
func (f *fakeModule) Description() string             { return f.name + " (test)" }
func (f *fakeModule) Category() string                { return f.cat }
func (f *fakeModule) Origin() Origin                  { return OriginBuiltin }
func (f *fakeModule) Commands() []CommandRef          { return f.cmds }
func (f *fakeModule) Keybinds() []Keybind             { return f.keys }
func (f *fakeModule) Widgets() []WidgetSpec           { return f.widgets }
func (f *fakeModule) DependsOn() []string             { return f.deps }
func (f *fakeModule) SettingsSchema() []SettingsField { return f.settings }

func newReg(t *testing.T) *Registry {
	t.Helper()
	return New(t.TempDir())
}

func TestRegister_RejectsDuplicateID(t *testing.T) {
	r := newReg(t)
	if err := r.Register(&fakeModule{id: "alpha"}); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := r.Register(&fakeModule{id: "alpha"}); err == nil {
		t.Fatal("expected duplicate-ID error, got nil")
	}
}

func TestRegister_RejectsEmptyID(t *testing.T) {
	r := newReg(t)
	if err := r.Register(&fakeModule{id: ""}); err == nil {
		t.Fatal("expected empty-ID error, got nil")
	}
}

func TestEnabled_UnknownIDDefaultsTrue(t *testing.T) {
	r := newReg(t)
	if !r.Enabled("never-registered") {
		t.Fatal("unknown ID must default to enabled (migration fallback)")
	}
}

func TestEnabled_KnownIDDefaultsTrue(t *testing.T) {
	r := newReg(t)
	_ = r.Register(&fakeModule{id: "alpha"})
	if !r.Enabled("alpha") {
		t.Fatal("registered module must default to enabled")
	}
}

func TestSetEnabled_RefusesEnableWhenDepDisabled(t *testing.T) {
	r := newReg(t)
	_ = r.Register(&fakeModule{id: "base"})
	_ = r.Register(&fakeModule{id: "dependent", deps: []string{"base"}})
	// Bring everything down in dependency-safe order: dependent first, then base.
	if err := r.SetEnabled("dependent", false); err != nil {
		t.Fatalf("disable dependent: %v", err)
	}
	if err := r.SetEnabled("base", false); err != nil {
		t.Fatalf("disable base: %v", err)
	}
	// Now try to bring dependent back up while its dep is still down — must fail.
	if err := r.SetEnabled("dependent", true); err == nil {
		t.Fatal("expected dep-not-enabled error when enabling dependent with base disabled")
	}
}

func TestSetEnabled_RefusesDisableWithLiveDependent(t *testing.T) {
	r := newReg(t)
	_ = r.Register(&fakeModule{id: "base"})
	_ = r.Register(&fakeModule{id: "dependent", deps: []string{"base"}})
	if err := r.SetEnabled("base", false); err == nil {
		t.Fatal("expected error when disabling base while dependent is enabled")
	}
}

func TestSetEnabled_AllowsDisableAfterDependentDisabled(t *testing.T) {
	r := newReg(t)
	_ = r.Register(&fakeModule{id: "base"})
	_ = r.Register(&fakeModule{id: "dependent", deps: []string{"base"}})
	if err := r.SetEnabled("dependent", false); err != nil {
		t.Fatalf("disable dependent: %v", err)
	}
	if err := r.SetEnabled("base", false); err != nil {
		t.Fatalf("disable base after dependent disabled: %v", err)
	}
	if r.Enabled("base") {
		t.Fatal("base should be disabled now")
	}
}

func TestEnabledModules_PreservesRegistrationOrder(t *testing.T) {
	r := newReg(t)
	for _, id := range []string{"c", "a", "b"} {
		_ = r.Register(&fakeModule{id: id})
	}
	got := r.EnabledModules()
	want := []string{"c", "a", "b"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %d, want %d", len(got), len(want))
	}
	for i, m := range got {
		if m.ID() != want[i] {
			t.Errorf("position %d: got %q, want %q", i, m.ID(), want[i])
		}
	}
}

func TestEnabledCommands_FiltersDisabledModules(t *testing.T) {
	r := newReg(t)
	_ = r.Register(&fakeModule{id: "a", cmds: []CommandRef{{ID: "a.run", Label: "A Run"}}})
	_ = r.Register(&fakeModule{id: "b", cmds: []CommandRef{{ID: "b.run", Label: "B Run"}}})
	if err := r.SetEnabled("a", false); err != nil {
		t.Fatalf("disable a: %v", err)
	}
	cmds := r.EnabledCommands()
	if len(cmds) != 1 || cmds[0].ID != "b.run" {
		t.Fatalf("expected only b.run, got %+v", cmds)
	}
}

func TestEnabledKeybinds_DetectsConflicts(t *testing.T) {
	r := newReg(t)
	_ = r.Register(&fakeModule{
		id:   "first",
		keys: []Keybind{{Key: "alt+x", CommandID: "first.do"}},
	})
	_ = r.Register(&fakeModule{
		id:   "second",
		keys: []Keybind{{Key: "alt+x", CommandID: "second.do"}},
	})
	binds, conflicts := r.EnabledKeybinds()
	if binds["alt+x"] != "first.do" {
		t.Errorf("first-registered should win: got %q", binds["alt+x"])
	}
	if len(conflicts) != 1 || conflicts[0] != "alt+x" {
		t.Errorf("expected one conflict on alt+x, got %v", conflicts)
	}
}

func TestEnabledKeybinds_SkipsDisabledModules(t *testing.T) {
	r := newReg(t)
	_ = r.Register(&fakeModule{
		id:   "off",
		keys: []Keybind{{Key: "alt+x", CommandID: "off.do"}},
	})
	if err := r.SetEnabled("off", false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	binds, conflicts := r.EnabledKeybinds()
	if len(binds) != 0 {
		t.Errorf("disabled module's keybinds should be hidden, got %v", binds)
	}
	if len(conflicts) != 0 {
		t.Errorf("no conflicts expected, got %v", conflicts)
	}
}

func TestDependents_ListsReverseDeps(t *testing.T) {
	r := newReg(t)
	_ = r.Register(&fakeModule{id: "base"})
	_ = r.Register(&fakeModule{id: "consumer-a", deps: []string{"base"}})
	_ = r.Register(&fakeModule{id: "consumer-b", deps: []string{"base"}})
	_ = r.Register(&fakeModule{id: "unrelated"})
	got := r.Dependents("base")
	want := []string{"consumer-a", "consumer-b"}
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	r := New(dir)
	_ = r.Register(&fakeModule{id: "alpha"})
	_ = r.Register(&fakeModule{id: "beta"})
	if err := r.SetEnabled("alpha", false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if err := r.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	r2 := New(dir)
	_ = r2.Register(&fakeModule{id: "alpha"})
	_ = r2.Register(&fakeModule{id: "beta"})
	if err := r2.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if r2.Enabled("alpha") {
		t.Error("alpha should be disabled after load")
	}
	if !r2.Enabled("beta") {
		t.Error("beta should be enabled after load")
	}
}

func TestLoad_MissingFileIsOK(t *testing.T) {
	r := newReg(t)
	if err := r.Load(); err != nil {
		t.Fatalf("missing file should be silent, got %v", err)
	}
}

func TestSave_CreatesGranitDir(t *testing.T) {
	dir := t.TempDir()
	r := New(dir)
	_ = r.Register(&fakeModule{id: "alpha"})
	_ = r.SetEnabled("alpha", false)
	if err := r.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	want := filepath.Join(dir, ".granit", "modules.json")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected file at %s, got %v", want, err)
	}
	data, err := os.ReadFile(want)
	if err != nil {
		t.Fatal(err)
	}
	var parsed stateFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse saved file: %v", err)
	}
	if parsed.Version != stateVersion {
		t.Errorf("version: got %d want %d", parsed.Version, stateVersion)
	}
	if v, ok := parsed.Enabled["alpha"]; !ok || v {
		t.Errorf("alpha entry: got %v ok=%v want false true", v, ok)
	}
}

func TestMirrorLegacy_DoesNotOverrideExisting(t *testing.T) {
	r := newReg(t)
	_ = r.Register(&fakeModule{id: "alpha"})
	_ = r.Register(&fakeModule{id: "beta"})
	// Explicitly disable alpha through the registry first.
	if err := r.SetEnabled("alpha", false); err != nil {
		t.Fatalf("disable alpha: %v", err)
	}
	// Legacy says alpha=true, beta=false. Existing alpha entry must win;
	// beta should pick up the legacy value.
	r.MirrorLegacy(map[string]bool{"alpha": true, "beta": false})
	if r.Enabled("alpha") {
		t.Error("explicit disable on alpha must survive MirrorLegacy")
	}
	if r.Enabled("beta") {
		t.Error("beta should now be disabled via legacy mirror")
	}
}

func TestMirrorLegacy_DisablesUnregisteredID(t *testing.T) {
	// Migration scenario: the user has cfg.CorePlugins["foo"] = false,
	// but the "foo" module hasn't been migrated yet, so it isn't
	// registered. Enabled("foo") must still respect the user's choice
	// — otherwise gates that switch from CorePluginEnabled to
	// registry.Enabled silently re-enable disabled features.
	r := newReg(t)
	r.MirrorLegacy(map[string]bool{"foo": false, "bar": true})
	if r.Enabled("foo") {
		t.Error("legacy disable on unregistered module must be honored")
	}
	if !r.Enabled("bar") {
		t.Error("legacy enable on unregistered module must be honored")
	}
	if !r.Enabled("never-mentioned") {
		t.Error("truly unknown ID must still default to enabled")
	}
}

func TestMirrorLegacy_NilMapIsSafe(t *testing.T) {
	r := newReg(t)
	_ = r.Register(&fakeModule{id: "alpha"})
	r.MirrorLegacy(nil) // must not panic
	if !r.Enabled("alpha") {
		t.Error("alpha should still be enabled")
	}
}

func TestSetEnabledBatch_HandlesDepOrder(t *testing.T) {
	r := newReg(t)
	_ = r.Register(&fakeModule{id: "base"})
	_ = r.Register(&fakeModule{id: "dep1", deps: []string{"base"}})
	_ = r.Register(&fakeModule{id: "dep2", deps: []string{"base"}})

	// Disable base + dependents in one batch — would fail
	// per-call (base can't be disabled while deps are enabled).
	err := r.SetEnabledBatch(map[string]bool{
		"base": false, "dep1": false, "dep2": false,
	})
	if err != nil {
		t.Fatalf("batch disable: %v", err)
	}
	if r.Enabled("base") || r.Enabled("dep1") || r.Enabled("dep2") {
		t.Errorf("expected all disabled, got base=%v dep1=%v dep2=%v",
			r.Enabled("base"), r.Enabled("dep1"), r.Enabled("dep2"))
	}

	// Enable in opposite direction in one batch — would fail
	// per-call if dep1 went first.
	err = r.SetEnabledBatch(map[string]bool{
		"dep1": true, "base": true,
	})
	if err != nil {
		t.Fatalf("batch enable: %v", err)
	}
	if !r.Enabled("base") || !r.Enabled("dep1") {
		t.Errorf("batch enable failed")
	}
	// dep2 wasn't in the batch and should remain disabled.
	if r.Enabled("dep2") {
		t.Error("dep2 not in batch, should still be disabled")
	}
}

func TestSetEnabledBatch_DetectsImpossible(t *testing.T) {
	r := newReg(t)
	_ = r.Register(&fakeModule{id: "base"})
	_ = r.Register(&fakeModule{id: "dep", deps: []string{"base"}})
	// Disable base while keeping dep enabled — impossible.
	err := r.SetEnabledBatch(map[string]bool{
		"base": false,
		"dep":  true,
	})
	if err == nil {
		t.Error("expected error on internally-inconsistent batch")
	}
}

func TestSetEnabledBatch_EmptyIsNoOp(t *testing.T) {
	r := newReg(t)
	_ = r.Register(&fakeModule{id: "x"})
	if err := r.SetEnabledBatch(nil); err != nil {
		t.Errorf("nil should be no-op, got %v", err)
	}
	if !r.Enabled("x") {
		t.Error("x flipped on empty batch")
	}
}

func TestPath_ReportsLocation(t *testing.T) {
	dir := t.TempDir()
	r := New(dir)
	want := filepath.Join(dir, ".granit", "modules.json")
	if got := r.Path(); got != want {
		t.Errorf("got %q want %q", got, want)
	}
}
