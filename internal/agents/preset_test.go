package agents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Validate rejects every required-field gap with a clear message
// pointing at the offending preset by ID. Catches typos in
// hand-written vault-local JSON before the runner blows up.
func TestPreset_Validate(t *testing.T) {
	cases := []struct {
		name   string
		in     Preset
		errSub string
	}{
		{"empty id", Preset{Name: "X", Description: "x"}, "ID is required"},
		{"empty name", Preset{ID: "x", Description: "x"}, "Name is required"},
		{"empty desc", Preset{ID: "x", Name: "X"}, "Description is required"},
		{"empty tool name", Preset{ID: "x", Name: "X", Description: "y", Tools: []string{"  "}}, "empty tool name"},
		{"valid", Preset{ID: "x", Name: "X", Description: "y"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.in.Validate()
			if c.errSub == "" {
				if err != nil {
					t.Errorf("expected nil, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.errSub) {
				t.Errorf("expected error containing %q, got %v", c.errSub, err)
			}
		})
	}
}

// NewPresetCatalog seeds with built-ins, silently dropping invalid
// ones (so a programmer error in one built-in doesn't take down
// the runner for everyone).
func TestPresetCatalog_BuiltinsLoaded(t *testing.T) {
	good := Preset{ID: "good", Name: "Good", Description: "yes"}
	bad := Preset{ID: "", Name: "Anonymous", Description: "broken"}
	c := NewPresetCatalog([]Preset{good, bad})
	if c.Len() != 1 {
		t.Errorf("expected 1 (only good loaded), got %d", c.Len())
	}
	if _, ok := c.ByID("good"); !ok {
		t.Error("good preset should be registered")
	}
}

// LoadVaultDir overlays vault-local presets, replacing built-ins
// of the same ID. New IDs are added.
func TestPresetCatalog_VaultLocalOverride(t *testing.T) {
	builtin := Preset{ID: "research-synth", Name: "BuiltIn Synth", Description: "shipped"}
	c := NewPresetCatalog([]Preset{builtin})

	vault := t.TempDir()
	dir := filepath.Join(vault, ".granit", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	override := Preset{ID: "research-synth", Name: "User Synth", Description: "overridden"}
	data, _ := json.Marshal(override)
	if err := os.WriteFile(filepath.Join(dir, "research-synth.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	custom := Preset{ID: "my-custom", Name: "Custom", Description: "new"}
	data, _ = json.Marshal(custom)
	if err := os.WriteFile(filepath.Join(dir, "my-custom.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, errs := c.LoadVaultDir(vault)
	if loaded != 2 || len(errs) != 0 {
		t.Errorf("loaded=%d errs=%v", loaded, errs)
	}
	got, _ := c.ByID("research-synth")
	if got.Name != "User Synth" {
		t.Errorf("override didn't apply: got %q", got.Name)
	}
	if _, ok := c.ByID("my-custom"); !ok {
		t.Error("custom preset should be registered")
	}
	// All() returns alphabetical order: "my-custom" before "research-synth".
	all := c.All()
	if len(all) != 2 || all[0].ID != "my-custom" {
		t.Errorf("All not sorted by ID: %+v", all)
	}
}

// LoadVaultDir on a vault without `.granit/agents/` is a no-op,
// not an error. Critical for fresh vaults where the directory
// hasn't been created yet.
func TestPresetCatalog_NoAgentsDir(t *testing.T) {
	c := NewPresetCatalog(nil)
	vault := t.TempDir()
	loaded, errs := c.LoadVaultDir(vault)
	if loaded != 0 || len(errs) != 0 {
		t.Errorf("expected (0, nil), got (%d, %v)", loaded, errs)
	}
}

// Filename mismatch is flagged per file but doesn't stop loading
// the rest. A vault with one bad file still gets the good ones.
func TestPresetCatalog_FilenameMismatch(t *testing.T) {
	vault := t.TempDir()
	dir := filepath.Join(vault, ".granit", "agents")
	os.MkdirAll(dir, 0o755)
	bad, _ := json.Marshal(Preset{ID: "real-id", Name: "X", Description: "y"})
	os.WriteFile(filepath.Join(dir, "wrong-filename.json"), bad, 0o644)
	good, _ := json.Marshal(Preset{ID: "ok", Name: "OK", Description: "y"})
	os.WriteFile(filepath.Join(dir, "ok.json"), good, 0o644)

	c := NewPresetCatalog(nil)
	loaded, errs := c.LoadVaultDir(vault)
	if loaded != 1 || len(errs) != 1 {
		t.Errorf("loaded=%d errs=%d %v", loaded, len(errs), errs)
	}
	if _, ok := c.ByID("ok"); !ok {
		t.Error("good preset should still load")
	}
}

// SavePreset round-trips through LoadVaultDir cleanly.
func TestSavePreset_RoundTrip(t *testing.T) {
	vault := t.TempDir()
	original := Preset{
		ID: "round-trip", Name: "Round Trip", Description: "test",
		SystemPrompt: "you are...",
		Tools:        []string{"read_note", "search_vault"},
		IncludeWrite: true, MaxSteps: 12,
	}
	if err := SavePreset(vault, original); err != nil {
		t.Fatal(err)
	}
	c := NewPresetCatalog(nil)
	loaded, errs := c.LoadVaultDir(vault)
	if loaded != 1 || len(errs) != 0 {
		t.Fatalf("loaded=%d errs=%v", loaded, errs)
	}
	got, _ := c.ByID("round-trip")
	if got.SystemPrompt != "you are..." || !got.IncludeWrite || got.MaxSteps != 12 ||
		len(got.Tools) != 2 {
		t.Errorf("round-trip lost data: %+v", got)
	}
}

// SavePreset rejects invalid presets before writing — we don't
// want broken JSON files leaking onto disk.
func TestSavePreset_RejectsInvalid(t *testing.T) {
	vault := t.TempDir()
	if err := SavePreset(vault, Preset{ID: "", Name: "x", Description: "y"}); err == nil {
		t.Error("expected SavePreset to reject empty-ID preset")
	}
	dir := filepath.Join(vault, ".granit", "agents")
	if entries, _ := os.ReadDir(dir); len(entries) > 0 {
		t.Errorf("expected empty dir, got %d entries", len(entries))
	}
}

// BuildRegistryForPreset filters the read-tool factories by the
// preset's Tools allow-list. Empty allow-list = all read tools.
func TestBuildRegistryForPreset_AllowList(t *testing.T) {
	rt1 := &stubTool{name: "read_note", kind: KindRead}
	rt2 := &stubTool{name: "search_vault", kind: KindRead}
	rt3 := &stubTool{name: "list_notes", kind: KindRead}
	wt := &stubTool{name: "write_note", kind: KindWrite}

	// Empty allow-list → all read tools.
	preset := Preset{ID: "x", Name: "X", Description: "y"}
	r, err := BuildRegistryForPreset(preset, []Tool{rt1, rt2, rt3}, []Tool{wt})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"read_note", "search_vault", "list_notes"} {
		if _, ok := r.ToolFor(name); !ok {
			t.Errorf("expected %s in registry", name)
		}
	}
	if _, ok := r.ToolFor("write_note"); ok {
		t.Error("write_note should NOT be registered (IncludeWrite=false)")
	}

	// Explicit allow-list of 2 tools → only those.
	preset = Preset{ID: "x", Name: "X", Description: "y",
		Tools: []string{"read_note", "search_vault"}}
	r, _ = BuildRegistryForPreset(preset, []Tool{rt1, rt2, rt3}, []Tool{wt})
	if _, ok := r.ToolFor("list_notes"); ok {
		t.Error("list_notes should NOT be in restricted registry")
	}

	// IncludeWrite=true wires write tools too.
	preset.IncludeWrite = true
	r, _ = BuildRegistryForPreset(preset, []Tool{rt1, rt2, rt3}, []Tool{wt})
	if _, ok := r.ToolFor("write_note"); !ok {
		t.Error("write_note should be registered when IncludeWrite=true")
	}
}

// BuildRegistryForPreset is defensive: a "read" factory list that
// accidentally contains a write tool is silently filtered. Same
// for the inverse. Belt-and-braces against caller mistakes.
func TestBuildRegistryForPreset_KindFiltering(t *testing.T) {
	rt := &stubTool{name: "ro", kind: KindRead}
	misclassified := &stubTool{name: "actually_write", kind: KindWrite}
	preset := Preset{ID: "x", Name: "X", Description: "y"}
	r, _ := BuildRegistryForPreset(preset, []Tool{rt, misclassified}, nil)
	if _, ok := r.ToolFor("actually_write"); ok {
		t.Error("write-kind tool in read factory should be filtered out")
	}
}
