package objects

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// NewRegistry pre-loads built-ins so a fresh vault has working
// galleries from the very first launch.
func TestNewRegistry_HasBuiltins(t *testing.T) {
	r := NewRegistry()
	if r.Len() < 5 {
		t.Errorf("expected at least 5 built-in types, got %d", r.Len())
	}
	for _, id := range []string{"person", "book", "project", "meeting", "idea"} {
		if _, ok := r.ByID(id); !ok {
			t.Errorf("expected built-in type %q to be registered", id)
		}
	}
}

// All returns types in deterministic alphabetical order so the type
// list in the Object Browser doesn't shuffle between renders.
func TestRegistry_AllIsSorted(t *testing.T) {
	r := NewRegistry()
	all := r.All()
	for i := 1; i < len(all); i++ {
		if all[i-1].ID > all[i].ID {
			t.Errorf("All() is not sorted: %q before %q", all[i-1].ID, all[i].ID)
		}
	}
}

// Vault-local override REPLACES a built-in by ID. We disable a
// built-in's optional fields and add a new one to confirm the merge
// semantics (replacement, not deep merge).
func TestLoadVaultDir_OverrideReplacesBuiltin(t *testing.T) {
	vault := t.TempDir()
	dir := filepath.Join(vault, ".granit", "types")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	custom := Type{
		ID: "person", Name: "Custom Person",
		Properties: []Property{
			{Name: "slack", Kind: KindText, Required: true},
		},
	}
	data, _ := json.Marshal(custom)
	if err := os.WriteFile(filepath.Join(dir, "person.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry()
	loaded, errs := r.LoadVaultDir(vault)
	if len(errs) != 0 {
		t.Fatalf("unexpected load errors: %v", errs)
	}
	if loaded != 1 {
		t.Errorf("loaded count: got %d, want 1", loaded)
	}
	got, _ := r.ByID("person")
	if got.Name != "Custom Person" {
		t.Errorf("Name not overridden: got %q", got.Name)
	}
	if len(got.Properties) != 1 || got.Properties[0].Name != "slack" {
		t.Errorf("Properties not overridden — expected [slack], got %+v", got.Properties)
	}
}

// LoadVaultDir on a vault without `.granit/types/` is a no-op, NOT an
// error. New vaults shouldn't error just because they have no
// overrides — that's the common case.
func TestLoadVaultDir_NoTypesDir(t *testing.T) {
	vault := t.TempDir()
	r := NewRegistry()
	loaded, errs := r.LoadVaultDir(vault)
	if loaded != 0 || len(errs) != 0 {
		t.Errorf("expected (0, nil), got (%d, %v)", loaded, errs)
	}
}

// Filename mismatch flags an error per file but does not stop loading
// the rest. A vault with one bad file should still see the good ones
// loaded.
func TestLoadVaultDir_FilenameMismatch(t *testing.T) {
	vault := t.TempDir()
	dir := filepath.Join(vault, ".granit", "types")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Bad: filename "foo.json" but id="bar"
	bad, _ := json.Marshal(Type{ID: "bar", Name: "Bar", Properties: []Property{{Name: "x"}}})
	os.WriteFile(filepath.Join(dir, "foo.json"), bad, 0o644)
	// Good: filename matches id
	good, _ := json.Marshal(Type{ID: "person", Name: "Override", Properties: []Property{{Name: "name"}}})
	os.WriteFile(filepath.Join(dir, "person.json"), good, 0o644)

	r := NewRegistry()
	loaded, errs := r.LoadVaultDir(vault)
	if loaded != 1 {
		t.Errorf("loaded: got %d, want 1", loaded)
	}
	if len(errs) != 1 {
		t.Errorf("errs: got %d, want 1", len(errs))
	} else if !strings.Contains(errs[0].Error(), "does not match") {
		t.Errorf("error text: %v", errs[0])
	}
	// Good override took effect.
	p, _ := r.ByID("person")
	if p.Name != "Override" {
		t.Errorf("good override didn't apply: got %q", p.Name)
	}
}

// Invalid type JSON is rejected per-file with a clear error; other
// files still load.
func TestLoadVaultDir_InvalidJSON(t *testing.T) {
	vault := t.TempDir()
	dir := filepath.Join(vault, ".granit", "types")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{not json"), 0o644)
	r := NewRegistry()
	_, errs := r.LoadVaultDir(vault)
	if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d", len(errs))
	}
}

// SaveType writes a JSON file at the conventional path and round-trips
// through LoadVaultDir cleanly.
func TestSaveType_RoundTrip(t *testing.T) {
	vault := t.TempDir()
	original := Type{
		ID: "snippet", Name: "Snippet", Icon: "✂️",
		Properties: []Property{
			{Name: "lang", Kind: KindText, Required: true},
			{Name: "tags", Kind: KindTag},
		},
	}
	if err := SaveType(vault, original); err != nil {
		t.Fatalf("SaveType: %v", err)
	}
	r := NewRegistry()
	loaded, errs := r.LoadVaultDir(vault)
	if loaded != 1 || len(errs) != 0 {
		t.Fatalf("LoadVaultDir: loaded=%d errs=%v", loaded, errs)
	}
	got, ok := r.ByID("snippet")
	if !ok {
		t.Fatal("snippet not loaded")
	}
	if got.Name != "Snippet" || len(got.Properties) != 2 {
		t.Errorf("round-trip lost data: %+v", got)
	}
}

// SaveType rejects an invalid type before writing — we don't want
// half-broken JSON files leaking onto disk.
func TestSaveType_RejectsInvalid(t *testing.T) {
	vault := t.TempDir()
	if err := SaveType(vault, Type{ID: "", Name: "x"}); err == nil {
		t.Error("expected SaveType to reject empty-ID type")
	}
	// Verify nothing was written.
	dir := filepath.Join(vault, ".granit", "types")
	if entries, _ := os.ReadDir(dir); len(entries) > 0 {
		t.Errorf("expected empty types dir, got %d entries", len(entries))
	}
}

// Set rejects invalid types and DOES NOT install them, so the registry
// stays in a known-good state. Pairs with SaveType_RejectsInvalid for
// the in-memory path.
func TestRegistry_SetRejectsInvalid(t *testing.T) {
	r := NewRegistry()
	before := r.Len()
	if err := r.Set(Type{ID: "", Name: "X"}); err == nil {
		t.Error("expected Set to reject empty-ID type")
	}
	if r.Len() != before {
		t.Errorf("invalid Set should not change registry; before=%d after=%d", before, r.Len())
	}
}
