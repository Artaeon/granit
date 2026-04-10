package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withFakeHome redirects $HOME (and $XDG_CONFIG_HOME if present) to a temp
// directory so that ConfigDir / vaultsPath / SaveVaultList write into the
// test's TempDir rather than the real user config.
func withFakeHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	return dir
}

// ---------------------------------------------------------------------------
// LoadVaultList — empty / missing file
// ---------------------------------------------------------------------------

func TestLoadVaultList_MissingFile(t *testing.T) {
	withFakeHome(t)
	vl := LoadVaultList()
	if len(vl.Vaults) != 0 {
		t.Errorf("expected empty vault list, got %d entries", len(vl.Vaults))
	}
	if vl.LastUsed != "" {
		t.Errorf("expected empty LastUsed, got %q", vl.LastUsed)
	}
}

// Regression: malformed JSON in vaults.json must not crash; load returns empty.
func TestLoadVaultList_MalformedJSON(t *testing.T) {
	withFakeHome(t)
	_ = os.MkdirAll(ConfigDir(), 0755)
	_ = os.WriteFile(filepath.Join(ConfigDir(), "vaults.json"), []byte("not json"), 0644)

	vl := LoadVaultList()
	if len(vl.Vaults) != 0 {
		t.Errorf("expected empty vault list on malformed JSON, got %d", len(vl.Vaults))
	}
}

// ---------------------------------------------------------------------------
// SaveVaultList round-trip
// ---------------------------------------------------------------------------

func TestSaveVaultList_RoundTrip(t *testing.T) {
	withFakeHome(t)

	original := VaultList{
		Vaults: []VaultEntry{
			{Path: "/home/me/notes", Name: "notes", LastOpen: "2026-04-10"},
			{Path: "/home/me/work", Name: "work", LastOpen: "2026-04-09"},
		},
		LastUsed: "/home/me/notes",
	}
	SaveVaultList(original)

	loaded := LoadVaultList()
	if len(loaded.Vaults) != 2 {
		t.Fatalf("expected 2 vaults, got %d", len(loaded.Vaults))
	}
	if loaded.LastUsed != "/home/me/notes" {
		t.Errorf("LastUsed not preserved, got %q", loaded.LastUsed)
	}
	if loaded.Vaults[0].Name != "notes" {
		t.Errorf("first vault name wrong: %q", loaded.Vaults[0].Name)
	}
}

// Regression: SaveVaultList must use atomic writes (commit 6aa198a) so a
// crash mid-save cannot truncate vaults.json and lose every known vault.
func TestSaveVaultList_AtomicNoTmp(t *testing.T) {
	withFakeHome(t)

	SaveVaultList(VaultList{
		Vaults:   []VaultEntry{{Path: "/x", Name: "x"}},
		LastUsed: "/x",
	})

	tmp := filepath.Join(ConfigDir(), "vaults.json.tmp")
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf("expected no .tmp file after successful save, stat err = %v", err)
	}
}

// ---------------------------------------------------------------------------
// AddVault / RemoveVault behavior
// ---------------------------------------------------------------------------

func TestAddVault_NewEntry(t *testing.T) {
	vl := VaultList{}
	dir := t.TempDir()

	vl.AddVault(dir)

	if len(vl.Vaults) != 1 {
		t.Fatalf("expected 1 vault, got %d", len(vl.Vaults))
	}
	abs, _ := filepath.Abs(dir)
	if vl.Vaults[0].Path != abs {
		t.Errorf("expected absolute path %q, got %q", abs, vl.Vaults[0].Path)
	}
	if vl.LastUsed != abs {
		t.Errorf("LastUsed should be set, got %q", vl.LastUsed)
	}
	if vl.Vaults[0].Name != filepath.Base(abs) {
		t.Errorf("expected name from basename, got %q", vl.Vaults[0].Name)
	}
}

func TestAddVault_UpdatesExisting(t *testing.T) {
	dir := t.TempDir()
	vl := VaultList{}
	vl.AddVault(dir)
	originalCount := len(vl.Vaults)

	// Add the same vault again — should update LastOpen, not duplicate.
	vl.AddVault(dir)
	if len(vl.Vaults) != originalCount {
		t.Errorf("AddVault should not duplicate existing entry, got %d", len(vl.Vaults))
	}
}

func TestRemoveVault_ClearsLastUsed(t *testing.T) {
	dir := t.TempDir()
	abs, _ := filepath.Abs(dir)
	vl := VaultList{}
	vl.AddVault(dir)

	vl.RemoveVault(abs)

	if len(vl.Vaults) != 0 {
		t.Errorf("expected vault removed, %d remain", len(vl.Vaults))
	}
	if vl.LastUsed != "" {
		t.Errorf("expected LastUsed cleared, got %q", vl.LastUsed)
	}
}

func TestRemoveVault_PreservesOtherLastUsed(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	abs1, _ := filepath.Abs(dir1)
	abs2, _ := filepath.Abs(dir2)
	vl := VaultList{}
	vl.AddVault(dir1)
	vl.AddVault(dir2)
	// LastUsed is now dir2 since it was added second.
	vl.RemoveVault(abs1) // remove non-active vault

	if vl.LastUsed != abs2 {
		t.Errorf("removing non-active vault should not change LastUsed, got %q want %q", vl.LastUsed, abs2)
	}
}

// ---------------------------------------------------------------------------
// vaultsPath
// ---------------------------------------------------------------------------

func TestVaultsPath_UnderConfigDir(t *testing.T) {
	withFakeHome(t)
	got := vaultsPath()
	if !strings.HasSuffix(got, "vaults.json") {
		t.Errorf("expected path to end in vaults.json, got %q", got)
	}
	if !strings.Contains(got, "granit") {
		t.Errorf("expected path under granit config, got %q", got)
	}
}
