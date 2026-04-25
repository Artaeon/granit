package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsNewVault_BrandNewIsTrue(t *testing.T) {
	dir := t.TempDir()
	if !isNewVault(dir) {
		t.Error("a vault with no .granit/ should be detected as new")
	}
}

func TestIsNewVault_EmptyVaultRootReturnsFalse(t *testing.T) {
	// Defensive: an empty vault root means "no vault loaded yet."
	// Don't trigger the picker in that case.
	if isNewVault("") {
		t.Error("empty vault root should not trigger picker")
	}
}

func TestIsNewVault_AnyMarkerFileMakesItOld(t *testing.T) {
	cases := []string{
		"active-profile",
		"modules.json",
		"tasks-meta.json",
	}
	for _, marker := range cases {
		t.Run(marker, func(t *testing.T) {
			dir := t.TempDir()
			granitDir := filepath.Join(dir, ".granit")
			if err := os.MkdirAll(granitDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(granitDir, marker), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
			if isNewVault(dir) {
				t.Errorf("vault with %s should NOT be detected as new", marker)
			}
		})
	}
}

func TestIsNewVault_GranitDirWithoutMarkersStillNew(t *testing.T) {
	// .granit/ exists but contains only unrelated files (e.g.
	// .granit/themes/ from a partial install). Still new from
	// the profile system's perspective — the user hasn't been
	// through Phase 3 yet.
	dir := t.TempDir()
	granitDir := filepath.Join(dir, ".granit")
	_ = os.MkdirAll(granitDir, 0o700)
	_ = os.WriteFile(filepath.Join(granitDir, "themes.json"), []byte("{}"), 0o600)
	if !isNewVault(dir) {
		t.Error("vault with .granit/ but no profile markers should still be new")
	}
}
