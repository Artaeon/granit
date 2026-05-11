// Package reposcan reads a local git repository and extracts the
// kind of context an AI document-generator can use to write
// charters / architecture overviews / onboarding docs that don't
// hallucinate. Pure file IO + os/exec for git log; no network, no
// writes, no auth — same single-tenant trust model as everything
// else in granit.
//
// What this package is NOT:
//   - A remote-clone tool. We only read paths the user already
//     owns. Cloning would invite an HTTP-side attack surface
//     (credentials, redirects, content trust) that isn't worth it
//     for a personal-knowledge-manager.
//   - A code analyser. We read file headers, package manifests,
//     and commit messages — never AST-walk source or compute
//     coverage. The point is "what is this project about, in
//     prose the model can ground its output in", not static
//     analysis.
//
// Path safety: ScanRepo rejects (1) paths containing ".." segments,
// (2) symlinks at the root, (3) paths whose absolute form falls
// outside the supplied allowedRoots. The handler is expected to
// pass the user's home directory + the vault root as allowedRoots
// so a malicious frontend bug or stale URL can't make the server
// read arbitrary filesystem locations.
package reposcan

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Context is the extracted view the AI consumer reads. Every field
// is bounded so a giant monorepo can't blow the prompt budget; the
// per-field caps are documented inline next to the producer.
type Context struct {
	Path    string `json:"path"`
	Name    string `json:"name"` // basename of the repo path
	IsGit   bool   `json:"isGit"`
	// Manifest is the language-specific package file we recognised
	// (package.json / go.mod / pyproject.toml / Cargo.toml / etc).
	// Empty when none match — the AI doc generator can still work
	// from README alone.
	Manifest        string `json:"manifest,omitempty"`        // file name we found
	ManifestContent string `json:"manifestContent,omitempty"` // capped at ManifestMaxBytes
	// README — the project's narrative. We pick the first matching
	// file under common case-variant names.
	ReadmeName    string `json:"readmeName,omitempty"`
	ReadmeContent string `json:"readmeContent,omitempty"` // capped at ReadmeMaxBytes
	// FileTree is a depth-2 listing of the repo root — directories
	// and top-level files, sorted, capped at FileTreeMaxEntries.
	// Gives the model a structural snapshot without flooding it
	// with src/**/* paths.
	FileTree []string `json:"fileTree,omitempty"`
	// RecentCommits is the last RecentCommitsMax commit subjects
	// from `git log --oneline`. Captures the project's recent
	// motion (refactor wave / new feature push / docs-only week)
	// without including diffs.
	RecentCommits []string `json:"recentCommits,omitempty"`
	// Branch is the currently-checked-out branch name (best-effort
	// — empty when reading `.git/HEAD` fails or the repo is in a
	// detached state).
	Branch string `json:"branch,omitempty"`
}

const (
	ReadmeMaxBytes      = 12_000 // ~3k tokens — comfortable for any chat window
	ManifestMaxBytes    = 6_000
	FileTreeMaxEntries  = 80
	RecentCommitsMax    = 20
	gitLogTimeout       = 5 * time.Second
)

// readmeCandidates / manifestCandidates are ordered by preference —
// the first match wins. README beats readme.txt; package.json
// beats package-lock.json (lock files are noise for prose
// generation).
var (
	readmeCandidates = []string{
		"README.md", "README.MD", "Readme.md", "readme.md",
		"README.rst", "README.txt", "README", "Readme.txt",
	}
	manifestCandidates = []string{
		"package.json",
		"go.mod",
		"pyproject.toml",
		"Cargo.toml",
		"Gemfile",
		"composer.json",
		"build.gradle",
		"build.gradle.kts",
		"pom.xml",
		"mix.exs",
		"deno.json",
		"deno.jsonc",
	}
)

var (
	ErrPathTraversal  = errors.New("reposcan: path contains '..' segment")
	ErrOutsideAllowed = errors.New("reposcan: path falls outside allowed roots")
	ErrNotADirectory  = errors.New("reposcan: path is not a directory")
	ErrSymlinkRoot    = errors.New("reposcan: path is a symlink (refused)")
)

// ScanRepo reads the given local path and returns the context. The
// path must:
//   - resolve to an absolute path WITHIN one of allowedRoots
//   - not contain ".." segments before resolution
//   - point to an existing directory (not a symlink to one)
//
// Leading "~" / "~/" is expanded to the user's home directory so
// the UI placeholder ("~/Projects/granit") works as a real path.
// The traversal check runs on the ORIGINAL input (pre-expansion)
// because filepath.Join inside expandTilde would Clean() the path
// and silently absorb a ".." segment.
//
// A repo without `.git` is still scanned (IsGit=false, no commits/
// branch); the AI can still use the README + manifest as grounding.
// Missing files are silent — the consumer reads "what's present"
// rather than handling "couldn't open file X".
func ScanRepo(path string, allowedRoots []string) (*Context, error) {
	if strings.Contains(path, "..") {
		return nil, ErrPathTraversal
	}
	path = expandTilde(path)
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if !isInsideAny(abs, allowedRoots) {
		return nil, ErrOutsideAllowed
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrSymlinkRoot
	}
	if !info.IsDir() {
		return nil, ErrNotADirectory
	}
	ctx := &Context{
		Path:  abs,
		Name:  filepath.Base(abs),
		IsGit: fileExists(filepath.Join(abs, ".git")),
	}
	ctx.ReadmeName, ctx.ReadmeContent = readFirstMatch(abs, readmeCandidates, ReadmeMaxBytes)
	ctx.Manifest, ctx.ManifestContent = readFirstMatch(abs, manifestCandidates, ManifestMaxBytes)
	ctx.FileTree = listTopLevel(abs)
	if ctx.IsGit {
		ctx.RecentCommits = recentCommitSubjects(abs)
		ctx.Branch = currentBranch(abs)
	}
	return ctx, nil
}

func isInsideAny(abs string, roots []string) bool {
	for _, root := range roots {
		rootAbs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		// Filepath.Rel returns "../…" when abs isn't under root,
		// or a path starting with the relative segments when it is.
		// We accept abs == rootAbs (the user scans their home root)
		// and any strict descendant.
		rel, err := filepath.Rel(rootAbs, abs)
		if err != nil {
			continue
		}
		if rel == "." || (!strings.HasPrefix(rel, "..") && !strings.HasPrefix(rel, string(filepath.Separator))) {
			return true
		}
	}
	return false
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// expandTilde handles the "~" and "~/foo" shorthand. Shells do this
// transparently; HTTP clients don't, so a user pasting
// "~/Projects/granit" into the UI would otherwise hit a 404. We
// only expand the LEADING tilde — "~user" (other-user expansion)
// and embedded tildes are left alone (no surprises, matches POSIX
// shell behaviour with HOME unset). When HOME isn't available the
// input is returned unchanged; the downstream allowedRoots check
// will then surface a clear "outside allowed roots" error.
func expandTilde(p string) string {
	if p == "" || p[0] != '~' {
		return p
	}
	if p != "~" && !strings.HasPrefix(p, "~/") {
		// "~user/…" — not supported; leave verbatim.
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == "~" {
		return home
	}
	return filepath.Join(home, p[2:])
}

// readFirstMatch tries every candidate name in order under dir.
// Returns the FIRST one that exists, capped at maxBytes. Empty
// strings on no match — caller treats absence as "not present"
// rather than an error.
func readFirstMatch(dir string, candidates []string, maxBytes int64) (name, content string) {
	for _, c := range candidates {
		p := filepath.Join(dir, c)
		info, err := os.Stat(p)
		if err != nil || info.IsDir() {
			continue
		}
		data, err := readBounded(p, maxBytes)
		if err != nil {
			continue
		}
		return c, data
	}
	return "", ""
}

// readBounded reads up to maxBytes from a file. If the file is
// larger, we truncate AT a byte boundary and append a "(truncated)"
// note so the AI doesn't keep summarising as if it had the full
// content.
func readBounded(path string, maxBytes int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	buf := make([]byte, maxBytes+1) // +1 so we can detect "more than max"
	n, err := f.Read(buf)
	if err != nil && err.Error() != "EOF" {
		// io.EOF on a fully-read file is fine; surface any other.
		if !errors.Is(err, fs.ErrInvalid) {
			// Tolerate read failures — return what we got.
		}
	}
	if int64(n) > maxBytes {
		return string(buf[:maxBytes]) + "\n\n…(truncated)", nil
	}
	return string(buf[:n]), nil
}

// listTopLevel returns the depth-1 listing of dir, dirs first then
// files, sorted alphabetically within each group. Hidden entries
// (leading dot) are skipped EXCEPT for .gitignore and a few common
// dotfiles that the AI legitimately uses as project signals.
var keepDotfiles = map[string]struct{}{
	".gitignore":    {},
	".dockerignore": {},
	".env.example":  {},
	".editorconfig": {},
	".github":       {},
}

func listTopLevel(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var dirs, files []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			if _, keep := keepDotfiles[name]; !keep {
				continue
			}
		}
		if e.IsDir() {
			dirs = append(dirs, name+"/")
		} else {
			files = append(files, name)
		}
	}
	sort.Strings(dirs)
	sort.Strings(files)
	all := append(dirs, files...)
	if len(all) > FileTreeMaxEntries {
		all = append(all[:FileTreeMaxEntries], fmt.Sprintf("…and %d more", len(all)-FileTreeMaxEntries))
	}
	return all
}

// recentCommitSubjects calls `git log --pretty=format:%s -n N` with
// a short timeout. Returns nil on timeout / non-git / git errors —
// missing commit history is a soft degradation, not a hard error.
func recentCommitSubjects(repoDir string) []string {
	// Bounded process: 5s wall-clock + the small output cap protect
	// against a misconfigured repo with a huge commit message.
	// LookPath returns ("", err) when git isn't installed — soft
	// degrade rather than block the scan (the README + manifest
	// are still useful context without commit history).
	if _, err := exec.LookPath("git"); err != nil {
		return nil
	}
	cmd := exec.Command("git", "-C", repoDir, "log", "--pretty=format:%s", "-n", fmt.Sprintf("%d", RecentCommitsMax))
	// Use a deadline-bounded run via a goroutine so a hung git
	// process (broken repo, NFS hang) doesn't block the request.
	type result struct {
		out []byte
		err error
	}
	ch := make(chan result, 1)
	go func() {
		out, err := cmd.Output()
		ch <- result{out, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			return nil
		}
		var subjects []string
		sc := bufio.NewScanner(strings.NewReader(string(r.out)))
		// One subject per line; cap each at 200 chars to defend
		// against the rare git history with a paragraph-long
		// first line of a commit.
		for sc.Scan() {
			line := sc.Text()
			if len(line) > 200 {
				line = line[:200] + "…"
			}
			subjects = append(subjects, line)
		}
		return subjects
	case <-time.After(gitLogTimeout):
		_ = cmd.Process.Kill()
		return nil
	}
}

// currentBranch reads .git/HEAD and parses the ref name. Empty on
// detached HEAD or a missing file — both are valid states.
func currentBranch(repoDir string) string {
	data, err := os.ReadFile(filepath.Join(repoDir, ".git", "HEAD"))
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(data))
	if !strings.HasPrefix(s, "ref: ") {
		return "" // detached HEAD points at a raw SHA
	}
	ref := strings.TrimPrefix(s, "ref: ")
	// "refs/heads/main" → "main".
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		return ref[i+1:]
	}
	return ref
}
