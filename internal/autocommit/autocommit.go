// Package autocommit provides opt-in git auto-commit on save for the
// web API. When enabled and the vault is a git repository, every
// committed save is rolled into a coalesced commit after a short
// debounce window — so a typing session that triggers 50 autosaves
// produces one tidy commit, not fifty.
//
// The debounce is per-vault (not per-file): every save resets the
// 30-second timer, and when the timer fires we commit all paths
// modified since the last commit in a single git operation. This
// matches what a developer would do manually after a focused work
// session ("save, save, save, commit").
//
// Why opt-in: Granit has no idea whether the user keeps their vault
// in git or not, and committing into a non-git directory would do
// nothing — but committing into a git directory the user uses for
// something else would be hostile. A setting toggle (default off)
// keeps the surprise factor at zero.
package autocommit

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Default debounce. 30 seconds is short enough that a closed laptop
// retains the recent-edits commit, long enough that a flurry of
// autosaves coalesces into one entry. Tunable via SetDebounce for
// tests / power users.
var defaultDebounce = 30 * time.Second

// Manager throttles + executes auto-commits for a single vault.
// Safe to call from multiple goroutines (the API handler is
// concurrent). The pending set is guarded by a mutex; the actual
// git invocation happens in a separate goroutine fired by the
// debounce timer so the calling handler doesn't block on git.
type Manager struct {
	vaultRoot string
	debounce  time.Duration

	mu      sync.Mutex
	enabled bool
	timer   *time.Timer
	pending map[string]struct{}
}

// New constructs a Manager. autocommit is initially disabled —
// callers must SetEnabled(true) explicitly.
func New(vaultRoot string) *Manager {
	return &Manager{
		vaultRoot: vaultRoot,
		debounce:  defaultDebounce,
		pending:   map[string]struct{}{},
	}
}

func (m *Manager) SetEnabled(v bool) {
	m.mu.Lock()
	m.enabled = v
	if !v && m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	m.mu.Unlock()
}

func (m *Manager) IsEnabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.enabled
}

func (m *Manager) SetDebounce(d time.Duration) {
	m.mu.Lock()
	m.debounce = d
	m.mu.Unlock()
}

// IsGitRepo returns true when vaultRoot is inside a git working tree.
// Cheap (one git invocation) — call it once at startup or per-toggle,
// not on every save.
func (m *Manager) IsGitRepo() bool {
	cmd := exec.Command("git", "-C", m.vaultRoot, "rev-parse", "--is-inside-work-tree")
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

// Notify records that a path was saved. If autocommit is enabled,
// schedules (or extends) the debounce timer. Does nothing when
// disabled — the caller doesn't have to check IsEnabled() first.
//
// `relPath` is the vault-relative slash-separated path the API
// handler just wrote. The caller can pass an absolute path; we
// normalise to relative internally.
func (m *Manager) Notify(relPath string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.enabled {
		return
	}
	rel := relPath
	if filepath.IsAbs(rel) {
		if r, err := filepath.Rel(m.vaultRoot, rel); err == nil {
			rel = filepath.ToSlash(r)
		}
	}
	m.pending[rel] = struct{}{}
	if m.timer != nil {
		m.timer.Reset(m.debounce)
		return
	}
	m.timer = time.AfterFunc(m.debounce, m.fire)
}

// CommitNow flushes pending changes immediately, bypassing the
// debounce. Useful from a "commit pending" UI button or from tests.
// Safe to call when there are no pending changes — it returns nil.
func (m *Manager) CommitNow() error {
	m.mu.Lock()
	if len(m.pending) == 0 {
		m.mu.Unlock()
		return nil
	}
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	paths := make([]string, 0, len(m.pending))
	for p := range m.pending {
		paths = append(paths, p)
	}
	m.pending = map[string]struct{}{}
	m.mu.Unlock()
	return m.commit(paths)
}

// fire is the debounce-timer callback. Snapshot the pending set,
// clear it, then run git off-lock so a long git invocation doesn't
// block subsequent Notify calls.
func (m *Manager) fire() {
	m.mu.Lock()
	if !m.enabled || len(m.pending) == 0 {
		m.timer = nil
		m.mu.Unlock()
		return
	}
	paths := make([]string, 0, len(m.pending))
	for p := range m.pending {
		paths = append(paths, p)
	}
	m.pending = map[string]struct{}{}
	m.timer = nil
	m.mu.Unlock()
	if err := m.commit(paths); err != nil {
		// We don't surface this to the user — autocommit is a
		// background safety net, not a primary save path. Log to
		// stderr so an operator running granit attached to a
		// terminal sees it; otherwise silent.
		fmt.Fprintf(stderr(), "autocommit: %v\n", err)
	}
}

// commit stages the given paths and creates a single commit. If
// nothing is staged after the add (e.g. the user reverted the file
// before the timer fired), it's a no-op.
func (m *Manager) commit(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	// `git add -- <path>...` is safer than `git add -A`; we only
	// touch paths granit actually wrote during this window.
	args := append([]string{"-C", m.vaultRoot, "add", "--"}, paths...)
	if out, err := run(args...); err != nil {
		return fmt.Errorf("git add: %s: %w", out, err)
	}
	// `--cached` to check the index — if nothing is staged the
	// commit would fail, and we'd rather skip silently.
	if out, err := run("-C", m.vaultRoot, "diff", "--cached", "--quiet"); err == nil {
		// Exit 0 from `diff --cached --quiet` means no staged
		// changes. Skip.
		return nil
	} else {
		// Exit 1 means there ARE staged changes — that's the path
		// we want. Other exit codes mean an error; surface them.
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return fmt.Errorf("git diff --cached: %s: %w", out, err)
		}
	}
	msg := buildMessage(paths)
	if out, err := run("-C", m.vaultRoot, "commit", "-m", msg); err != nil {
		return fmt.Errorf("git commit: %s: %w", out, err)
	}
	return nil
}

// buildMessage produces a one-line commit summary from the path
// list. For a single file: "Update <path>". For 2-3 files: list
// them. For more: "Update N files". Conventional-commits style
// would be more legible but the user might be on any commit-message
// convention — a neutral imperative line works everywhere.
func buildMessage(paths []string) string {
	switch {
	case len(paths) == 1:
		return "Update " + paths[0]
	case len(paths) <= 3:
		return "Update " + strings.Join(paths, ", ")
	default:
		return fmt.Sprintf("Update %d files", len(paths))
	}
}

// run is a small wrapper around exec.Command that returns combined
// output for diagnostics. Kept as a package var so tests can swap
// it for a fake.
var run = func(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// stderr is a package var so tests can capture log output. Returns
// os.Stderr by default.
var stderr = func() interface {
	Write(p []byte) (n int, err error)
} {
	return osStderr{}
}

type osStderr struct{}

func (osStderr) Write(p []byte) (int, error) {
	// Late binding — keeps "fmt"/"os" out of the var initializer
	// where they'd be eagerly resolved.
	return fmt.Print(string(p))
}
