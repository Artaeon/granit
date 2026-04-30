package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artaeon/granit/internal/objects"
)

// rtInitRepo creates a fresh git repo with one commit. Skips when git
// isn't on PATH.
func rtInitRepo(t *testing.T, dir string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "t@x"},
		{"config", "user.name", "T"},
		{"config", "commit.gpgsign", "false"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"add", "-A"},
		{"commit", "-q", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func TestRepoTracker_ScanFindsRepos(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "demo")
	if err := os.Mkdir(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rtInitRepo(t, repoDir)
	// Plain dir without .git — must NOT be picked up.
	if err := os.Mkdir(filepath.Join(root, "plain"), 0o755); err != nil {
		t.Fatal(err)
	}

	r := NewRepoTracker()
	r.SetSize(100, 30)
	r.Open(root, objects.NewRegistry(), objects.NewIndex())

	if len(r.rows) != 1 {
		t.Fatalf("expected 1 repo found, got %d", len(r.rows))
	}
	if r.rows[0].Name != "demo" {
		t.Errorf("wrong row name: %q", r.rows[0].Name)
	}
	if !r.rows[0].Status.IsRepo {
		t.Error("status should mark this as a repo")
	}
}

func TestRepoTracker_EnterImportsRepo(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "alpha")
	_ = os.Mkdir(repoDir, 0o755)
	rtInitRepo(t, repoDir)

	r := NewRepoTracker()
	r.SetSize(100, 30)
	r.Open(root, objects.NewRegistry(), objects.NewIndex())

	r, _ = r.Update(tea.KeyMsg{Type: tea.KeyEnter})
	path, content, ok := r.GetImportRequest()
	if !ok {
		t.Fatal("expected import request after Enter")
	}
	if !strings.Contains(path, "alpha") {
		t.Errorf("path missing repo name: %q", path)
	}
	if !strings.Contains(content, "type: project") {
		t.Errorf("content missing type: project; got:\n%s", content)
	}
	if !strings.Contains(content, "repo: ") || !strings.Contains(content, "alpha") {
		t.Errorf("content missing repo path: %q", content)
	}
}

func TestRepoTracker_AlreadyImportedJumps(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "beta")
	_ = os.Mkdir(repoDir, 0o755)
	rtInitRepo(t, repoDir)

	// Build an index that already has a project note pointing at this repo.
	reg := objects.NewRegistry()
	bld := objects.NewBuilder(reg)
	bld.Add("Projects/beta.md", "beta",
		map[string]string{"type": "project", "repo": repoDir})
	idx := bld.Finalize()

	r := NewRepoTracker()
	r.SetSize(100, 30)
	r.Open(root, reg, idx)

	if !r.rows[0].Imported {
		t.Fatalf("expected row to be marked Imported (path=%q ImportedPath=%q)",
			r.rows[0].Path, r.rows[0].ImportedPath)
	}

	r, _ = r.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if _, _, ok := r.GetImportRequest(); ok {
		t.Error("Enter on imported row should NOT create another note")
	}
	jumpPath, ok := r.GetJumpRequest()
	if !ok || jumpPath != "Projects/beta.md" {
		t.Errorf("expected jump to existing note, got (%q, %v)", jumpPath, ok)
	}
}

func TestRepoTracker_OpenWithBadRootShowsHint(t *testing.T) {
	r := NewRepoTracker()
	r.SetSize(100, 30)
	r.Open("", objects.NewRegistry(), objects.NewIndex())
	if r.statusMsg == "" {
		t.Error("expected a hint message for empty scan root")
	}
	if len(r.rows) != 0 {
		t.Errorf("expected no rows, got %d", len(r.rows))
	}
}

func TestExpandHome_Tilde(t *testing.T) {
	got, err := expandHome("~/foo")
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, "foo")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExpandHome_AbsolutePassthrough(t *testing.T) {
	got, _ := expandHome("/abs/path")
	if got != "/abs/path" {
		t.Errorf("absolute should pass through, got %q", got)
	}
}
