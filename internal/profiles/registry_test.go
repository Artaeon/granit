package profiles

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func newReg(t *testing.T) *ProfileRegistry {
	t.Helper()
	return New(t.TempDir())
}

func mkProfile(id, name string) *Profile {
	return &Profile{ID: id, Name: name}
}

func TestRegister_RejectsNilOrEmpty(t *testing.T) {
	r := newReg(t)
	if err := r.Register(nil); err == nil {
		t.Error("expected error for nil profile")
	}
	if err := r.Register(&Profile{}); err == nil {
		t.Error("expected error for empty-ID profile")
	}
}

func TestRegister_OverwriteSameIDPreservesOrder(t *testing.T) {
	r := newReg(t)
	_ = r.Register(mkProfile("a", "First"))
	_ = r.Register(mkProfile("b", "Second"))
	_ = r.Register(mkProfile("a", "First Updated"))

	all := r.All()
	if len(all) != 2 {
		t.Fatalf("got %d profiles, want 2", len(all))
	}
	if all[0].ID != "a" || all[1].ID != "b" {
		t.Errorf("order changed: got %s, %s", all[0].ID, all[1].ID)
	}
	if all[0].Name != "First Updated" {
		t.Errorf("overwrite did not apply: got %q", all[0].Name)
	}
}

func TestRegister_DefensiveCopyIsolatesCallerMutations(t *testing.T) {
	r := newReg(t)
	p := mkProfile("a", "Original")
	_ = r.Register(p)

	// Mutate the caller's pointer.
	p.Name = "Mutated"

	got, _ := r.Get("a")
	if got.Name != "Original" {
		t.Errorf("registry leaked caller mutation: got %q", got.Name)
	}
}

func TestActive_FallsBackToDefaultWhenUnset(t *testing.T) {
	r := newReg(t)
	_ = r.Register(mkProfile(DefaultProfileID, "Classic"))

	if r.ActiveID() != DefaultProfileID {
		t.Errorf("ActiveID = %q, want %q", r.ActiveID(), DefaultProfileID)
	}
	if a := r.Active(); a == nil || a.ID != DefaultProfileID {
		t.Errorf("Active = %+v, want classic", a)
	}
}

func TestActive_ReturnsPlaceholderWhenNothingRegistered(t *testing.T) {
	r := newReg(t)
	a := r.Active()
	if a == nil {
		t.Fatal("Active returned nil")
	}
	if a.ID != DefaultProfileID {
		t.Errorf("placeholder ID = %q, want %q", a.ID, DefaultProfileID)
	}
}

func TestSetActive_RejectsUnknownID(t *testing.T) {
	r := newReg(t)
	_ = r.Register(mkProfile(DefaultProfileID, "Classic"))
	err := r.SetActive("never-registered")
	if !errors.Is(err, ErrUnknownProfile) {
		t.Errorf("expected ErrUnknownProfile, got %v", err)
	}
}

func TestSetActive_PersistsToActivePointer(t *testing.T) {
	vault := t.TempDir()
	r := New(vault)
	_ = r.Register(mkProfile("classic", "Classic"))
	_ = r.Register(mkProfile("operator", "Daily Operator"))

	if err := r.SetActive("operator"); err != nil {
		t.Fatal(err)
	}
	if r.ActiveID() != "operator" {
		t.Errorf("ActiveID = %q after SetActive(operator)", r.ActiveID())
	}

	pointer := filepath.Join(vault, ".granit", "active-profile")
	data, err := os.ReadFile(pointer)
	if err != nil {
		t.Fatalf("active-profile not persisted: %v", err)
	}
	if string(data) != "operator\n" {
		t.Errorf("pointer contents: got %q, want %q", data, "operator\n")
	}
}

func TestLoad_MissingPointerKeepsDefault(t *testing.T) {
	vault := t.TempDir()
	r := New(vault)
	_ = r.Register(mkProfile(DefaultProfileID, "Classic"))
	if err := r.Load(); err != nil {
		t.Fatal(err)
	}
	if r.ActiveID() != DefaultProfileID {
		t.Errorf("expected default after Load with no pointer, got %q", r.ActiveID())
	}
}

func TestLoad_ReadsActivePointer(t *testing.T) {
	vault := t.TempDir()
	r := New(vault)
	_ = r.Register(mkProfile("classic", "Classic"))
	_ = r.Register(mkProfile("operator", "Daily Operator"))

	pointer := filepath.Join(vault, ".granit", "active-profile")
	if err := os.MkdirAll(filepath.Dir(pointer), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pointer, []byte("operator\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := r.Load(); err != nil {
		t.Fatal(err)
	}
	if r.ActiveID() != "operator" {
		t.Errorf("ActiveID = %q after Load, want operator", r.ActiveID())
	}
}

func TestLoad_UnknownActiveIDFallsBackToDefault(t *testing.T) {
	vault := t.TempDir()
	r := New(vault)
	_ = r.Register(mkProfile("classic", "Classic"))

	pointer := filepath.Join(vault, ".granit", "active-profile")
	_ = os.MkdirAll(filepath.Dir(pointer), 0o700)
	_ = os.WriteFile(pointer, []byte("nonexistent\n"), 0o600)

	if err := r.Load(); err != nil {
		t.Fatal(err)
	}
	if r.ActiveID() != DefaultProfileID {
		t.Errorf("ActiveID = %q for unknown pointer, want default", r.ActiveID())
	}
}

func TestLoad_VaultLocalProfileOverridesBuiltin(t *testing.T) {
	vault := t.TempDir()
	r := New(vault)
	// Register a built-in.
	_ = r.Register(mkProfile("classic", "Built-in Classic"))
	r.MarkBuiltIn("classic")

	// Place a vault-local override.
	override := Profile{ID: "classic", Name: "Vault Override"}
	dir := filepath.Join(vault, ".granit", "profiles")
	_ = os.MkdirAll(dir, 0o700)
	data, _ := json.Marshal(override)
	_ = os.WriteFile(filepath.Join(dir, "classic.json"), data, 0o600)

	if err := r.Load(); err != nil {
		t.Fatal(err)
	}
	got, _ := r.Get("classic")
	if got.Name != "Vault Override" {
		t.Errorf("override didn't apply: got %q", got.Name)
	}
	if got.BuiltIn {
		t.Error("BuiltIn flag should clear when override loads")
	}
}

func TestLoad_MalformedJSONIsSkipped(t *testing.T) {
	vault := t.TempDir()
	r := New(vault)
	_ = r.Register(mkProfile("classic", "Classic"))

	dir := filepath.Join(vault, ".granit", "profiles")
	_ = os.MkdirAll(dir, 0o700)
	_ = os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{not valid"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "noid.json"), []byte(`{"name":"oops"}`), 0o600)

	if err := r.Load(); err != nil {
		t.Fatalf("Load should not error on malformed file, got %v", err)
	}
	if len(r.All()) != 1 {
		t.Errorf("malformed files should be skipped, got %d profiles", len(r.All()))
	}
}

func TestLoad_NonexistentDirIsOK(t *testing.T) {
	r := New(t.TempDir())
	_ = r.Register(mkProfile("classic", "Classic"))
	if err := r.Load(); err != nil {
		t.Errorf("missing profiles dir should be silent, got %v", err)
	}
}

func TestConcurrentRegisterAndActive(t *testing.T) {
	r := newReg(t)
	_ = r.Register(mkProfile("classic", "Classic"))

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Reader: spam Active() while writers register
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = r.Active()
					_ = r.All()
					_ = r.ActiveID()
				}
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = r.Register(mkProfile("p"+string(rune('a'+i%26)), "Profile"))
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = r.SetActive("classic")
		}
	}()

	// Let it churn briefly, then stop readers.
	for i := 0; i < 10; i++ {
		_ = r.All()
	}
	close(stop)
	wg.Wait()
}

func TestActivePath_EmptyWhenNoVault(t *testing.T) {
	r := New("")
	if r.ActivePath() != "" {
		t.Errorf("ActivePath should be empty for vault-less registry, got %q", r.ActivePath())
	}
}

func TestSetActive_NoVaultSucceedsInMemoryOnly(t *testing.T) {
	r := New("")
	_ = r.Register(mkProfile("classic", "Classic"))
	if err := r.SetActive("classic"); err != nil {
		t.Errorf("SetActive should work without a vault, got %v", err)
	}
	if r.ActiveID() != "classic" {
		t.Errorf("ActiveID = %q, want classic", r.ActiveID())
	}
}
