package tui

import (
	"path/filepath"
	"strings"
	"testing"
)

// ===========================================================================
// resolveVaultPath tests
// ===========================================================================

func TestResolveVaultPath_NormalRelative(t *testing.T) {
	vault := t.TempDir()
	got, err := resolveVaultPath(vault, "notes/foo.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(vault, "notes", "foo.md")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveVaultPath_RootItself(t *testing.T) {
	vault := t.TempDir()
	got, err := resolveVaultPath(vault, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != vault {
		t.Errorf("got %q, want %q", got, vault)
	}
}

func TestResolveVaultPath_DotDotEscape(t *testing.T) {
	vault := t.TempDir()
	if _, err := resolveVaultPath(vault, "../../etc/passwd"); err == nil {
		t.Error("expected error for ../../etc/passwd, got nil")
	}
}

func TestResolveVaultPath_DotDotInsideStillOK(t *testing.T) {
	// "notes/../foo.md" cleans to "foo.md" which is still inside the
	// vault and should be allowed.
	vault := t.TempDir()
	got, err := resolveVaultPath(vault, "notes/../foo.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(vault, "foo.md")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveVaultPath_AbsoluteRelArgumentJoinsAsChild(t *testing.T) {
	// filepath.Join treats an absolute second argument as if it were
	// relative on POSIX (it gets concatenated, not substituted), so the
	// result lands inside the vault and is allowed. This test pins that
	// behaviour so a future Join change would be caught.
	vault := t.TempDir()
	got, err := resolveVaultPath(vault, "/etc/passwd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(vault, "etc", "passwd")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveVaultPath_NeighbourPrefix(t *testing.T) {
	// Make sure /tmp/vault does not match /tmp/vaulted/... by accident.
	parent := t.TempDir()
	vault := filepath.Join(parent, "vault")
	neighbour := filepath.Join(parent, "vaulted", "secret.md")
	rel, err := filepath.Rel(vault, neighbour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolveVaultPath(vault, rel); err == nil {
		t.Error("expected error for neighbour-prefix path, got nil")
	}
}

func TestResolveVaultPath_EmptyVaultRoot(t *testing.T) {
	if _, err := resolveVaultPath("", "foo.md"); err == nil {
		t.Error("empty vault root should error, got nil")
	}
}

func TestResolveVaultPath_ErrorIsRecognisable(t *testing.T) {
	vault := t.TempDir()
	_, err := resolveVaultPath(vault, "../escape")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "escape") && err != errPathEscapesVault {
		t.Errorf("error message should mention escape: %v", err)
	}
}
