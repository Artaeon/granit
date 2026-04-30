package repos

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// initRepo creates a fresh git repo in dir with one initial commit on a
// branch named "main". Skips the test if `git` isn't on PATH so CI
// images without git don't fail spuriously.
func initRepo(t *testing.T, dir string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed — skipping git-dependent test")
	}
	cmds := [][]string{
		{"git", "init", "-q", "-b", "main"},
		{"git", "config", "user.email", "test@example.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "config", "commit.gpgsign", "false"},
	}
	for _, c := range cmds {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("init step %v failed: %v\n%s", c, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"add", "a.txt"},
		{"commit", "-q", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
}

func TestStatusOf_NonRepoReturnsErr(t *testing.T) {
	dir := t.TempDir()
	s, err := StatusOf(dir)
	if err != ErrNotARepo {
		t.Fatalf("expected ErrNotARepo, got %v", err)
	}
	if s.IsRepo {
		t.Fatal("IsRepo should be false for non-repo path")
	}
}

func TestStatusOf_CleanRepo(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir)

	s, err := StatusOf(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !s.IsRepo {
		t.Fatal("IsRepo should be true")
	}
	if s.Branch != "main" {
		t.Errorf("expected branch=main, got %q", s.Branch)
	}
	if s.Dirty != 0 {
		t.Errorf("expected clean working tree, got %d dirty", s.Dirty)
	}
	if s.LastCommit.IsZero() {
		t.Error("expected LastCommit to be set after init commit")
	}
	if !s.IsClean() {
		t.Error("IsClean should be true on a fresh repo with no upstream")
	}
}

func TestStatusOf_DetectsModifiedFile(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := StatusOf(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Dirty != 1 {
		t.Errorf("expected 1 dirty file, got %d", s.Dirty)
	}
	if s.IsClean() {
		t.Error("IsClean should be false with modified file")
	}
}

func TestStatusOf_DetectsUntrackedFile(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := StatusOf(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Dirty != 1 {
		t.Errorf("expected 1 dirty (untracked counted), got %d", s.Dirty)
	}
}

func TestLooksLikeRepo_FalseForPlainDir(t *testing.T) {
	dir := t.TempDir()
	if looksLikeRepo(dir) {
		t.Fatal("plain temp dir should not look like a repo")
	}
}

// TestParseStatus_BranchAndAheadBehind exercises the v2 porcelain
// branch-header parsing without needing a network upstream — we feed
// synthetic input directly so coverage extends past the live-repo
// happy path.
func TestParseStatus_BranchAndAheadBehind(t *testing.T) {
	// Sample output structured the way `git status --porcelain=v2 --branch`
	// emits it for a repo that's 3 ahead, 1 behind, with one modified
	// file and one untracked file.
	input := "# branch.oid abcdef0\n" +
		"# branch.head main\n" +
		"# branch.upstream origin/main\n" +
		"# branch.ab +3 -1\n" +
		"1 .M N... 100644 100644 100644 0000 0000 file.txt\n" +
		"? newfile.txt\n"
	var s Status
	parseStatus(&s, input)
	if s.Branch != "main" {
		t.Errorf("branch: got %q, want main", s.Branch)
	}
	if s.Ahead != 3 {
		t.Errorf("ahead: got %d, want 3", s.Ahead)
	}
	if s.Behind != 1 {
		t.Errorf("behind: got %d, want 1", s.Behind)
	}
	if s.Dirty != 2 {
		t.Errorf("dirty: got %d, want 2 (one modified + one untracked)", s.Dirty)
	}
	if s.IsClean() {
		t.Error("IsClean should be false with ahead/behind/dirty")
	}
}

// TestParseStatus_DetachedHeadEmptyBranch verifies that a "(detached)"
// branch.head marker normalises to an empty Branch field — the badge
// renderer relies on this to switch display modes.
func TestParseStatus_DetachedHeadEmptyBranch(t *testing.T) {
	input := "# branch.head (detached)\n"
	var s Status
	parseStatus(&s, input)
	if s.Branch != "" {
		t.Errorf("expected empty branch for detached HEAD, got %q", s.Branch)
	}
}

func TestStatus_AgeSinceLastCommit(t *testing.T) {
	s := Status{LastCommit: time.Now().Add(-2 * time.Hour)}
	got := s.AgeSinceLastCommit()
	if got < 90*time.Minute || got > 150*time.Minute {
		t.Errorf("expected ~2h, got %v", got)
	}
	zero := Status{}
	if zero.AgeSinceLastCommit() != 0 {
		t.Error("zero LastCommit should report zero age")
	}
}
