package repos

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestOpenFolder_EmptyPathErrors(t *testing.T) {
	if err := OpenFolder(""); err == nil {
		t.Fatal("expected error on empty path")
	}
}

func TestOpenFolder_NonExistentPathErrors(t *testing.T) {
	if err := OpenFolder("/definitely/does/not/exist/12345"); err == nil {
		t.Fatal("expected error on non-existent path")
	}
}

func TestOpenCommand_SelectsBinary(t *testing.T) {
	dir, err := os.MkdirTemp("", "openfolder-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	cmd, err := openCommand(dir)
	if runtime.GOOS == "linux" || runtime.GOOS == "freebsd" {
		// May error if xdg-open isn't installed in CI — that's the
		// expected behaviour and the error message guides the user.
		// In a normal dev environment xdg-utils is present, so we
		// only assert the binary name when the lookup succeeded.
		if err == nil {
			if !strings.Contains(cmd.Path, "xdg-open") {
				t.Errorf("expected xdg-open, got %q", cmd.Path)
			}
		}
		return
	}
	if err != nil {
		t.Fatalf("unexpected err on %s: %v", runtime.GOOS, err)
	}
	want := "open"
	if runtime.GOOS == "windows" {
		want = "explorer"
	}
	if !strings.Contains(cmd.Path, want) {
		t.Errorf("on %s expected %q, got %q", runtime.GOOS, want, cmd.Path)
	}
}
