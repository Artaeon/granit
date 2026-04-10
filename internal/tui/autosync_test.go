package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ===========================================================================
// AutoSync Tests
// ===========================================================================
//
// These tests shell out to git so they require the git binary to be on
// PATH. The single requireGit helper at the top of the file skips the
// whole suite if git is missing rather than reporting per-test failures.

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH; skipping autosync test")
	}
}

// gitInit creates a brand-new git repo at dir with a local user identity
// and an initial empty commit so subsequent operations have a HEAD to work
// against.
func gitInit(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("init", "--quiet", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	run("commit", "--quiet", "--allow-empty", "-m", "init")
}

// gitRunIn runs git inside dir and returns combined output.
func gitRunIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// writeStringFile writes a string to path with 0o644 perms.
func writeStringFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// readStringFile reads path as a string.
func readStringFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// runMsg invokes a tea.Cmd synchronously and returns its message.
func runMsg(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	return cmd()
}

// ---------------------------------------------------------------------------
// Enabled gating
// ---------------------------------------------------------------------------

func TestAutoSync_DisabledReturnsNil(t *testing.T) {
	a := NewAutoSync(t.TempDir())
	if cmd := a.PullOnOpen(); cmd != nil {
		t.Error("PullOnOpen should return nil when disabled")
	}
	if cmd := a.CommitAndPush(); cmd != nil {
		t.Error("CommitAndPush should return nil when disabled")
	}
}

func TestAutoSync_PullOnOpen_NotAGitRepo(t *testing.T) {
	requireGit(t)
	a := NewAutoSync(t.TempDir())
	a.SetEnabled(true)
	// Vault is not a git repo, so PullOnOpen should return nil rather
	// than running git pull on a directory that has no .git.
	if cmd := a.PullOnOpen(); cmd != nil {
		t.Error("PullOnOpen should return nil for a non-git directory")
	}
}

// ---------------------------------------------------------------------------
// CommitAndPush
// ---------------------------------------------------------------------------

func TestAutoSync_CommitAndPush_NothingToCommit(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	gitInit(t, dir)

	a := NewAutoSync(dir)
	a.SetEnabled(true)
	msg := runMsg(t, a.CommitAndPush())

	res, ok := msg.(autoSyncResultMsg)
	if !ok {
		t.Fatalf("expected autoSyncResultMsg, got %T", msg)
	}
	if res.err != nil {
		t.Errorf("unexpected err: %v", res.err)
	}
	if !strings.Contains(res.output, "nothing to commit") {
		t.Errorf("output = %q, want to contain 'nothing to commit'", res.output)
	}
}

func TestAutoSync_CommitAndPush_PerFileCommits(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	gitInit(t, dir)

	// Track a file with one commit, then make multiple changes: a new
	// untracked file, an update to the tracked one, and another new file.
	tracked := filepath.Join(dir, "tracked.md")
	writeStringFile(t, tracked, "v1")
	gitRunIn(t, dir, "add", "tracked.md")
	gitRunIn(t, dir, "commit", "-m", "seed")

	writeStringFile(t, tracked, "v2")
	writeStringFile(t, filepath.Join(dir, "alpha.md"), "hello")
	writeStringFile(t, filepath.Join(dir, "beta.md"), "world")

	a := NewAutoSync(dir)
	a.SetEnabled(true)
	_ = runMsg(t, a.CommitAndPush())

	// Each changed file should produce its own commit.
	log := gitRunIn(t, dir, "log", "--pretty=%s")
	subjects := strings.Split(strings.TrimSpace(log), "\n")
	wantSubjects := map[string]bool{
		"vault: add alpha.md":      false,
		"vault: add beta.md":       false,
		"vault: update tracked.md": false,
	}
	for _, s := range subjects {
		if _, ok := wantSubjects[s]; ok {
			wantSubjects[s] = true
		}
	}
	for subject, found := range wantSubjects {
		if !found {
			t.Errorf("missing per-file commit %q in log:\n%s", subject, log)
		}
	}
}

func TestAutoSync_CommitAndPush_DeletedFile(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	gitInit(t, dir)

	target := filepath.Join(dir, "doomed.md")
	writeStringFile(t, target, "content")
	gitRunIn(t, dir, "add", "doomed.md")
	gitRunIn(t, dir, "commit", "-m", "add doomed")

	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}

	a := NewAutoSync(dir)
	a.SetEnabled(true)
	_ = runMsg(t, a.CommitAndPush())

	if !strings.Contains(gitRunIn(t, dir, "log", "--pretty=%s"), "vault: remove doomed.md") {
		t.Error("expected per-file remove commit in log")
	}
}

// ---------------------------------------------------------------------------
// CheckStatus
// ---------------------------------------------------------------------------

func TestAutoSync_CheckStatus_NotAGitRepo(t *testing.T) {
	requireGit(t)
	a := NewAutoSync(t.TempDir())
	msg := runMsg(t, a.CheckStatus())
	gs, ok := msg.(gitStatusMsg)
	if !ok {
		t.Fatalf("expected gitStatusMsg, got %T", msg)
	}
	if gs.isGitRepo {
		t.Error("isGitRepo should be false for non-git directory")
	}
}

func TestAutoSync_CheckStatus_CleanRepo(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	gitInit(t, dir)

	a := NewAutoSync(dir)
	msg := runMsg(t, a.CheckStatus())
	gs := msg.(gitStatusMsg)
	if !gs.isGitRepo {
		t.Error("isGitRepo should be true after git init")
	}
	if !gs.isSynced {
		t.Error("isSynced should be true on a clean repo")
	}
	if gs.changed != 0 {
		t.Errorf("changed = %d, want 0", gs.changed)
	}
}

func TestAutoSync_CheckStatus_DirtyRepo(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	gitInit(t, dir)
	writeStringFile(t, filepath.Join(dir, "a.md"), "hi")
	writeStringFile(t, filepath.Join(dir, "b.md"), "hi")

	a := NewAutoSync(dir)
	msg := runMsg(t, a.CheckStatus())
	gs := msg.(gitStatusMsg)
	if gs.isSynced {
		t.Error("isSynced should be false with untracked files")
	}
	if gs.changed != 2 {
		t.Errorf("changed = %d, want 2", gs.changed)
	}
}

// ---------------------------------------------------------------------------
// PullOnOpen — full conflict-resolution path
// ---------------------------------------------------------------------------
//
// We build a tiny two-repo setup: a bare "remote" that two clones share,
// then make divergent commits on the same line in the two clones to force
// a conflict on rebase pull. The auto-resolver should accept "theirs"
// (the newest remote version) and continue the rebase.

func TestAutoSync_PullOnOpen_NoChanges(t *testing.T) {
	requireGit(t)
	_, local := setupTwoRepoPair(t)

	a := NewAutoSync(local)
	a.SetEnabled(true)
	msg := runMsg(t, a.PullOnOpen())
	res := msg.(autoSyncResultMsg)
	if res.err != nil {
		t.Errorf("unexpected err: %v", res.err)
	}
}

func TestAutoSync_PullOnOpen_AutoResolvesConflict(t *testing.T) {
	requireGit(t)
	remote, local := setupTwoRepoPair(t)

	// Make a "newer" change on the remote side via a second clone, so the
	// remote ref advances with content that conflicts with our local edit.
	other := t.TempDir()
	gitRunIn(t, other, "clone", "--quiet", remote, ".")
	gitRunIn(t, other, "config", "user.email", "other@example.com")
	gitRunIn(t, other, "config", "user.name", "Other")
	writeStringFile(t, filepath.Join(other, "shared.md"), "REMOTE\n")
	gitRunIn(t, other, "add", "shared.md")
	gitRunIn(t, other, "commit", "-m", "remote change")
	gitRunIn(t, other, "push", "--quiet", "origin", "main")

	// Make a conflicting local change.
	writeStringFile(t, filepath.Join(local, "shared.md"), "LOCAL\n")
	gitRunIn(t, local, "add", "shared.md")
	gitRunIn(t, local, "commit", "-m", "local change")

	a := NewAutoSync(local)
	a.SetEnabled(true)
	msg := runMsg(t, a.PullOnOpen())
	res := msg.(autoSyncResultMsg)

	if res.err != nil {
		t.Fatalf("expected auto-resolve to succeed, got err: %v\noutput: %s", res.err, res.output)
	}
	if !strings.Contains(res.output, "auto-resolved") {
		t.Errorf("output = %q, want to mention auto-resolved", res.output)
	}

	// During rebase pull, --theirs refers to the upstream side, so the
	// resolver should have kept REMOTE\n.
	if got := readStringFile(t, filepath.Join(local, "shared.md")); got != "REMOTE\n" {
		t.Errorf("shared.md = %q, want %q", got, "REMOTE\n")
	}

	// And the rebase should be cleaned up — no .git/rebase-merge directory.
	rebaseDir := filepath.Join(local, ".git", "rebase-merge")
	if _, err := os.Stat(rebaseDir); err == nil {
		t.Error(".git/rebase-merge still exists; rebase did not finish")
	}
}

// setupTwoRepoPair builds a bare remote and one clone configured with a
// local user identity. The clone has a single seed commit on main pushed
// to the remote.
func setupTwoRepoPair(t *testing.T) (remote, local string) {
	t.Helper()
	remote = t.TempDir()
	gitRunIn(t, remote, "init", "--bare", "--quiet", "-b", "main")

	local = t.TempDir()
	gitRunIn(t, local, "clone", "--quiet", remote, ".")
	gitRunIn(t, local, "config", "user.email", "test@example.com")
	gitRunIn(t, local, "config", "user.name", "Test")
	writeStringFile(t, filepath.Join(local, "shared.md"), "seed\n")
	gitRunIn(t, local, "add", "shared.md")
	gitRunIn(t, local, "commit", "-m", "seed")
	gitRunIn(t, local, "push", "--quiet", "origin", "main")
	return remote, local
}
