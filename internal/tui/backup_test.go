package tui

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// CreateBackup
// ---------------------------------------------------------------------------

// setupTestVault creates a temp vault with a couple of notes for backup tests.
func setupTestVault(t *testing.T) string {
	t.Helper()
	vault := t.TempDir()
	if err := os.WriteFile(filepath.Join(vault, "note1.md"), []byte("# First\nhello"), 0644); err != nil {
		t.Fatal(err)
	}
	subdir := filepath.Join(vault, "subfolder")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "note2.md"), []byte("# Second"), 0644); err != nil {
		t.Fatal(err)
	}
	return vault
}

func TestCreateBackup_ProducesReadableZip(t *testing.T) {
	vault := setupTestVault(t)
	if err := CreateBackup(vault); err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}

	backupDir := filepath.Join(vault, ".granit", "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backups dir: %v", err)
	}

	var zipPath string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".zip") {
			zipPath = filepath.Join(backupDir, e.Name())
		}
	}
	if zipPath == "" {
		t.Fatal("no .zip file produced")
	}

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("backup zip is not readable: %v", err)
	}
	defer r.Close()

	// Both notes should be in the archive.
	found := make(map[string]bool)
	for _, f := range r.File {
		found[f.Name] = true
	}
	if !found["note1.md"] {
		t.Error("note1.md missing from backup")
	}
	if !found["subfolder/note2.md"] {
		t.Error("subfolder/note2.md missing from backup")
	}
}

// Regression: CreateBackup must NOT leave a sibling .tmp file behind on
// success. Asserts the atomic-rename happened.
func TestCreateBackup_LeavesNoTmpOnSuccess(t *testing.T) {
	vault := setupTestVault(t)
	if err := CreateBackup(vault); err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}

	backupDir := filepath.Join(vault, ".granit", "backups")
	entries, _ := os.ReadDir(backupDir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("found leftover .tmp file: %s", e.Name())
		}
	}
}

func TestCreateBackup_SkipsExcludedDirs(t *testing.T) {
	vault := setupTestVault(t)
	// Drop a file inside a directory that should be excluded
	gitDir := filepath.Join(vault, ".git")
	_ = os.MkdirAll(gitDir, 0755)
	_ = os.WriteFile(filepath.Join(gitDir, "config"), []byte("ignored"), 0644)

	if err := CreateBackup(vault); err != nil {
		t.Fatal(err)
	}
	backupDir := filepath.Join(vault, ".granit", "backups")
	entries, _ := os.ReadDir(backupDir)
	var zipPath string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".zip") {
			zipPath = filepath.Join(backupDir, e.Name())
		}
	}
	r, _ := zip.OpenReader(zipPath)
	defer r.Close()
	for _, f := range r.File {
		if strings.HasPrefix(f.Name, ".git") {
			t.Errorf("backup unexpectedly included .git entry: %s", f.Name)
		}
	}
}

// ---------------------------------------------------------------------------
// RestoreBackup
// ---------------------------------------------------------------------------

func TestRestoreBackup_RoundTrip(t *testing.T) {
	vault := setupTestVault(t)

	if err := CreateBackup(vault); err != nil {
		t.Fatalf("backup failed: %v", err)
	}
	backupDir := filepath.Join(vault, ".granit", "backups")
	entries, _ := os.ReadDir(backupDir)
	var zipPath string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".zip") {
			zipPath = filepath.Join(backupDir, e.Name())
		}
	}

	// Modify a note, then restore.
	original, _ := os.ReadFile(filepath.Join(vault, "note1.md"))
	_ = os.WriteFile(filepath.Join(vault, "note1.md"), []byte("MODIFIED"), 0644)

	if err := RestoreBackup(vault, zipPath); err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	restored, _ := os.ReadFile(filepath.Join(vault, "note1.md"))
	if string(restored) != string(original) {
		t.Errorf("restore did not bring back original content; got %q, want %q", restored, original)
	}
}

// Regression: RestoreBackup must clean up its per-file .tmp files.
func TestRestoreBackup_LeavesNoTmpOnSuccess(t *testing.T) {
	vault := setupTestVault(t)

	if err := CreateBackup(vault); err != nil {
		t.Fatal(err)
	}
	backupDir := filepath.Join(vault, ".granit", "backups")
	entries, _ := os.ReadDir(backupDir)
	var zipPath string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".zip") {
			zipPath = filepath.Join(backupDir, e.Name())
		}
	}

	if err := RestoreBackup(vault, zipPath); err != nil {
		t.Fatal(err)
	}
	// Walk the vault and check no .tmp files survived (excluding the
	// backups directory itself, which uses its own .tmp on creation).
	_ = filepath.Walk(vault, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if strings.Contains(path, filepath.Join(".granit", "backups")) {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(path, ".tmp") {
			t.Errorf("leftover .tmp after restore: %s", path)
		}
		return nil
	})
}

// Regression: RestoreBackup must reject zip entries that escape the vault
// (path traversal via "../foo" entries in a malicious archive).
func TestRestoreBackup_RejectsZipSlip(t *testing.T) {
	vault := t.TempDir()
	zipPath := filepath.Join(t.TempDir(), "evil.zip")

	// Build a zip whose entry name escapes the vault.
	zf, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)
	w, _ := zw.Create("../escaped.md")
	_, _ = w.Write([]byte("pwn"))
	_ = zw.Close()
	_ = zf.Close()

	if err := RestoreBackup(vault, zipPath); err != nil {
		// Errors are fine — zip-slip rejected.
		t.Logf("restore returned %v (rejected, ok)", err)
	}
	// The escaped file must not exist anywhere outside the vault.
	if _, err := os.Stat(filepath.Join(filepath.Dir(vault), "escaped.md")); err == nil {
		t.Error("zip-slip succeeded: ../escaped.md was written outside vault")
	}
}
