package tasks

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestResolveInVault_AcceptsValid(t *testing.T) {
	root := "/home/u/vault"
	cases := []struct{ in, want string }{
		{"Tasks.md", "/home/u/vault/Tasks.md"},
		{"Daily/2026-04-25.md", "/home/u/vault/Daily/2026-04-25.md"},
		{"Projects/A/Tasks.md", "/home/u/vault/Projects/A/Tasks.md"},
		{"./inbox.md", "/home/u/vault/inbox.md"},
	}
	for _, c := range cases {
		got, err := resolveInVault(root, c.in)
		if err != nil {
			t.Errorf("resolveInVault(%q): unexpected error %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("resolveInVault(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestResolveInVault_RejectsAbsolute(t *testing.T) {
	if _, err := resolveInVault("/home/u/vault", "/etc/passwd"); !errors.Is(err, ErrEscapesVault) {
		t.Errorf("expected ErrEscapesVault, got %v", err)
	}
}

func TestResolveInVault_RejectsTraversal(t *testing.T) {
	cases := []string{
		"../etc/passwd",
		"Tasks/../../etc/passwd",
		"Daily/../../../sensitive",
		"a/b/c/../../../escape",
		"Tasks/.././../oops",
	}
	for _, in := range cases {
		if _, err := resolveInVault("/home/u/vault", in); !errors.Is(err, ErrEscapesVault) {
			t.Errorf("resolveInVault(%q): expected ErrEscapesVault, got %v", in, err)
		}
	}
}

func TestResolveInVault_RejectsEmpty(t *testing.T) {
	if _, err := resolveInVault("/home/u/vault", ""); !errors.Is(err, ErrEscapesVault) {
		t.Errorf("expected ErrEscapesVault for empty rel, got %v", err)
	}
}

func TestResolveInVault_RejectsNeighbourPrefix(t *testing.T) {
	// /home/u/vault-attacker is a different directory but shares
	// the prefix /home/u/vault. A naive HasPrefix check without
	// the trailing separator would incorrectly allow this.
	root := "/home/u/vault"
	// We don't have a way to construct "vault-attacker/x" from
	// inside the vault since it has no .. in it — but we can
	// verify a path that cleans to the neighbour gets rejected.
	// Construct via abs (which should be caught by IsAbs first).
	if _, err := resolveInVault(root, "/home/u/vault-attacker/x"); !errors.Is(err, ErrEscapesVault) {
		t.Errorf("expected ErrEscapesVault, got %v", err)
	}
}

func TestResolveInVault_AllowsRootItself(t *testing.T) {
	// Edge case — rel "." should resolve to the vault root.
	root := "/home/u/vault"
	got, err := resolveInVault(root, ".")
	if err != nil {
		t.Errorf("unexpected error for '.': %v", err)
	}
	if got != filepath.Clean(root) {
		t.Errorf("got %q, want %q", got, root)
	}
}

func TestResolveInVault_RejectsBackslashTraversalEvenOnUnix(t *testing.T) {
	// Belt-and-suspenders: backslash is a separator on Windows.
	// Reject ".." even when typed with backslashes so a sidecar
	// authored on Windows and replayed on Unix can't bypass.
	if _, err := resolveInVault("/home/u/vault", `..\sensitive`); !errors.Is(err, ErrEscapesVault) {
		t.Errorf("expected ErrEscapesVault for backslash traversal, got %v", err)
	}
}
