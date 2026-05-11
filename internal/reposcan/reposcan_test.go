package reposcan

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Path safety is the load-bearing part of this package — a regression
// that let the handler read arbitrary filesystem paths would be a
// serious leak. The happy-path tests are useful but the rejection
// tests are why this file exists.

func TestScanRepo_RejectsPathTraversal(t *testing.T) {
	allowed := []string{t.TempDir()}
	_, err := ScanRepo("/some/legit/dir/../../etc/passwd", allowed)
	if !errors.Is(err, ErrPathTraversal) {
		t.Errorf("expected ErrPathTraversal, got %v", err)
	}
}

func TestScanRepo_RejectsOutsideAllowedRoots(t *testing.T) {
	// Two distinct tmp dirs. The "allowed" list contains only the
	// first; the scan target is the second. Must refuse.
	allowed := []string{t.TempDir()}
	outside := t.TempDir()
	_, err := ScanRepo(outside, allowed)
	if !errors.Is(err, ErrOutsideAllowed) {
		t.Errorf("expected ErrOutsideAllowed for path outside allowed roots, got %v", err)
	}
}

func TestScanRepo_AcceptsAllowedRoot(t *testing.T) {
	root := t.TempDir()
	// Real subdir under the allowed root.
	sub := filepath.Join(root, "myproject")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, err := ScanRepo(sub, []string{root})
	if err != nil {
		t.Fatalf("expected success for allowed path, got %v", err)
	}
	if ctx.Name != "myproject" {
		t.Errorf("Name = %q, want myproject", ctx.Name)
	}
}

func TestScanRepo_RejectsSymlinkRoot(t *testing.T) {
	// A symlink at the scan root could redirect us anywhere; refuse
	// it even if the target lives inside an allowed root.
	root := t.TempDir()
	target := filepath.Join(root, "real")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "via-link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink not supported on this fs: %v", err)
	}
	_, err := ScanRepo(link, []string{root})
	if !errors.Is(err, ErrSymlinkRoot) {
		t.Errorf("expected ErrSymlinkRoot, got %v", err)
	}
}

func TestScanRepo_RejectsNonDirectory(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "not-a-dir.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ScanRepo(file, []string{root})
	if !errors.Is(err, ErrNotADirectory) {
		t.Errorf("expected ErrNotADirectory for a regular file, got %v", err)
	}
}

func TestScanRepo_ReadmeReadAndCapped(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	readme := "# Granite\n\nA personal knowledge manager.\n\n" + strings.Repeat("padding ", 5000)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, err := ScanRepo(repo, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	if ctx.ReadmeName != "README.md" {
		t.Errorf("ReadmeName = %q, want README.md", ctx.ReadmeName)
	}
	if !strings.HasPrefix(ctx.ReadmeContent, "# Granite") {
		t.Errorf("ReadmeContent doesn't start with the title: %q", ctx.ReadmeContent[:min(80, len(ctx.ReadmeContent))])
	}
	// Truncation: the readme was 5000*8=40000+ bytes; capped to 12_000
	// plus a trailing "(truncated)" marker.
	if !strings.Contains(ctx.ReadmeContent, "(truncated)") {
		t.Errorf("expected '(truncated)' marker on oversized README")
	}
}

func TestScanRepo_PrefersFirstReadmeCandidate(t *testing.T) {
	// When BOTH README.md and README.txt exist, README.md wins
	// (it's first in the candidate list — markdown is the
	// preferred surface). Defensive against a future rearrange of
	// the candidate list that drops the ordering contract.
	root := t.TempDir()
	repo := filepath.Join(root, "r")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("md wins"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.txt"), []byte("txt loses"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, _ := ScanRepo(repo, []string{root})
	if ctx.ReadmeName != "README.md" {
		t.Errorf("expected README.md preferred, got %s", ctx.ReadmeName)
	}
	if ctx.ReadmeContent != "md wins" {
		t.Errorf("wrong content: %q", ctx.ReadmeContent)
	}
}

func TestScanRepo_RecognisesEveryManifestType(t *testing.T) {
	cases := []struct {
		fname   string
		content string
	}{
		{"package.json", `{"name":"x"}`},
		{"go.mod", "module x\n"},
		{"pyproject.toml", "[project]\nname=\"x\"\n"},
		{"Cargo.toml", "[package]\nname=\"x\"\n"},
		{"Gemfile", "source 'https://rubygems.org'\n"},
	}
	for _, c := range cases {
		t.Run(c.fname, func(t *testing.T) {
			root := t.TempDir()
			repo := filepath.Join(root, "r")
			if err := os.MkdirAll(repo, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(repo, c.fname), []byte(c.content), 0o644); err != nil {
				t.Fatal(err)
			}
			ctx, err := ScanRepo(repo, []string{root})
			if err != nil {
				t.Fatal(err)
			}
			if ctx.Manifest != c.fname {
				t.Errorf("expected Manifest=%q, got %q", c.fname, ctx.Manifest)
			}
			if ctx.ManifestContent != c.content {
				t.Errorf("content round-trip failed")
			}
		})
	}
}

func TestScanRepo_FileTreeListing(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "r")
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"README.md", "package.json", "index.ts"} {
		if err := os.WriteFile(filepath.Join(repo, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Hidden files — most are filtered out, .gitignore is kept.
	if err := os.WriteFile(filepath.Join(repo, ".DS_Store"), []byte("noise"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, err := ScanRepo(repo, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	// Dirs come before files; both sorted; trailing slash on dirs.
	got := strings.Join(ctx.FileTree, " ")
	if !strings.Contains(got, "docs/") || !strings.Contains(got, "src/") {
		t.Errorf("missing dirs in tree: %v", ctx.FileTree)
	}
	if !strings.Contains(got, "README.md") || !strings.Contains(got, "package.json") {
		t.Errorf("missing files in tree: %v", ctx.FileTree)
	}
	// Hidden noise filtered.
	if strings.Contains(got, ".DS_Store") {
		t.Errorf(".DS_Store should be filtered: %v", ctx.FileTree)
	}
	// .gitignore explicitly kept as a project signal.
	if !strings.Contains(got, ".gitignore") {
		t.Errorf(".gitignore should be kept as a project signal: %v", ctx.FileTree)
	}
}

func TestScanRepo_IsGitFlagFollowsDotGit(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "r")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	// No .git → IsGit=false.
	ctx, _ := ScanRepo(repo, []string{root})
	if ctx.IsGit {
		t.Errorf("expected IsGit=false without .git/")
	}
	// Create .git as a directory → IsGit=true.
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, _ = ScanRepo(repo, []string{root})
	if !ctx.IsGit {
		t.Errorf("expected IsGit=true with .git/")
	}
}

func TestScanRepo_MissingFilesAreSilent(t *testing.T) {
	// A repo with no README, no manifest, no git — the function
	// must succeed and return what it could find (the name + an
	// empty file list). Missing files are absence-of-signal,
	// not errors.
	root := t.TempDir()
	repo := filepath.Join(root, "bare")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, err := ScanRepo(repo, []string{root})
	if err != nil {
		t.Fatalf("bare directory should not error: %v", err)
	}
	if ctx.Name != "bare" {
		t.Errorf("Name = %q, want bare", ctx.Name)
	}
	if ctx.Manifest != "" || ctx.ReadmeName != "" {
		t.Errorf("expected empty Manifest + ReadmeName on bare repo")
	}
	if ctx.IsGit {
		t.Errorf("expected IsGit=false on bare repo")
	}
}

func TestScanRepo_ExpandsLeadingTilde(t *testing.T) {
	// Bedrock the test against a real per-test HOME so we can plant
	// the repo under HOME and pass "~/<rel>" without depending on
	// the test runner's actual home directory.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	repo := filepath.Join(tmpHome, "myproj")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# X\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// allowedRoots includes the temp HOME so the expanded path
	// passes the safety check.
	ctx, err := ScanRepo("~/myproj", []string{tmpHome})
	if err != nil {
		t.Fatalf("tilde scan failed: %v", err)
	}
	if ctx.Name != "myproj" {
		t.Errorf("Name = %q, want myproj", ctx.Name)
	}
	if ctx.ReadmeName != "README.md" {
		t.Errorf("ReadmeName = %q, want README.md", ctx.ReadmeName)
	}
}

func TestScanRepo_BareTildeExpandsToHome(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	// HOME itself should be a valid scan target — useful when the
	// vault root IS the home dir.
	ctx, err := ScanRepo("~", []string{tmpHome})
	if err != nil {
		t.Fatalf("bare tilde failed: %v", err)
	}
	if ctx.Path != tmpHome {
		t.Errorf("expanded path = %q, want HOME %q", ctx.Path, tmpHome)
	}
}

func TestScanRepo_TildeUserNotExpanded(t *testing.T) {
	// "~root/etc" is left verbatim — we don't try other-user
	// expansion. Filepath.Abs prepends the cwd, which won't be in
	// allowedRoots → 403 (or NotExist if the resulting path is
	// genuinely missing, which is also fine — both block the read).
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	_, err := ScanRepo("~root/etc", []string{tmpHome})
	if err == nil {
		t.Fatal("expected error for ~root/etc, got nil")
	}
	if !errors.Is(err, ErrOutsideAllowed) && !errors.Is(err, ErrPathTraversal) && !os.IsNotExist(err) {
		t.Fatalf("expected outside/traversal/not-found, got %v", err)
	}
}

func TestScanRepo_TildeWithTraversalStillRejected(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	// "~/../etc" expands to "<tmpHome>/../etc" — the traversal check
	// runs AFTER expansion so the ".." segment still trips it.
	_, err := ScanRepo("~/../etc", []string{tmpHome})
	if !errors.Is(err, ErrPathTraversal) {
		t.Fatalf("expected path traversal, got %v", err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
