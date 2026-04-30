// Package repos exposes a small, dependency-free helper for inspecting
// the git status of a local repository: branch name, dirty file count,
// ahead/behind versus upstream, and last-commit timestamp.
//
// Used by granit's project tracking — a typed-project note can declare
// `repo: /path/to/repo` and the Project Hub strip shows live status,
// the Repo Tracker tab lists every repo under a configured root, and
// saved views surface "Dirty Repos" / "Stale Repos" on the dashboard.
//
// Why shell out to `git` instead of go-git or libgit2?
//   - Zero new dependencies. `git` is already on every dev machine.
//   - Status flags (clean, ahead/behind, branch) are exactly what
//     `git` prints — no impedance mismatch with the user's mental
//     model from the command line.
//   - The cost of a single `git status --porcelain=v2 --branch` is
//     a few ms on a typical repo. We bound it with a 3s timeout to
//     avoid pathological cases (huge repo, slow disk, network FS).
package repos

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Status captures the live git state of a single repository. Returned
// fields:
//
//   - IsRepo   = false when the path doesn't contain a .git directory.
//                Caller can decide to surface "(no git)" rather than
//                error out.
//   - Branch   = current branch name (HEAD ref short name) or empty
//                when detached.
//   - Dirty    = count of files with any uncommitted change (modified,
//                added, deleted, untracked-tracked, renamed).
//                Untracked files NOT counted unless tracked-as-modified —
//                matches the user's mental model of "needs commit."
//   - Ahead    = commits the local branch has that upstream doesn't.
//   - Behind   = commits upstream has that local doesn't.
//   - LastCommit = mtime of the latest commit on the current branch
//                  (zero value when no commits / detached / no upstream).
type Status struct {
	IsRepo     bool
	Branch     string
	Dirty      int
	Ahead      int
	Behind     int
	LastCommit time.Time
}

// IsClean reports whether the working tree has zero uncommitted files
// AND is in sync with upstream. Convenience for status badges.
func (s Status) IsClean() bool {
	return s.Dirty == 0 && s.Ahead == 0 && s.Behind == 0
}

// AgeSinceLastCommit returns how long since the latest commit. Zero
// when LastCommit is unset (caller should ignore the duration in
// that case).
func (s Status) AgeSinceLastCommit() time.Duration {
	if s.LastCommit.IsZero() {
		return 0
	}
	return time.Since(s.LastCommit)
}

// gitCommandTimeout caps every `git` invocation so a wedged repo (NFS
// mount, lock file, huge worktree) can't stall the TUI render thread.
// 3s is generous — local repos return in <50ms, network filesystems
// in <500ms typically.
const gitCommandTimeout = 3 * time.Second

// ErrNotARepo is returned when the path doesn't look like a git
// repository. Callers should typically convert this into a
// `Status{IsRepo: false}` rather than propagate.
var ErrNotARepo = errors.New("repos: not a git repository")

// StatusOf returns the Status for the repository at `path`. The path
// can be the repo root or any directory inside the worktree — git
// itself walks up to find .git.
//
// Returns (Status{IsRepo: false}, ErrNotARepo) when the path doesn't
// resolve to a repository, so callers can use a single conditional
// (`if !s.IsRepo`) without inspecting the error.
//
// Other errors propagate (timeouts, permission failures); the
// returned Status is zero-value with IsRepo set to true so callers
// can still display "(error)" without losing the repo's identity.
func StatusOf(path string) (Status, error) {
	if !looksLikeRepo(path) {
		return Status{IsRepo: false}, ErrNotARepo
	}
	s := Status{IsRepo: true}

	// Read branch + ahead/behind + dirty count via `git status --porcelain=v2 --branch`.
	// porcelain=v2 is the stable scriptable format with explicit
	// branch headers and per-file change records. We parse line by
	// line — no regex, no allocations beyond the line slice.
	out, err := runGit(path, "status", "--porcelain=v2", "--branch")
	if err != nil {
		return s, err
	}
	parseStatus(&s, out)

	// Read last-commit mtime as Unix seconds via `git log -1 --format=%ct`.
	// Cheap (one commit object). Empty output means no commits.
	out, err = runGit(path, "log", "-1", "--format=%ct")
	if err == nil {
		txt := strings.TrimSpace(out)
		if txt != "" {
			if secs, perr := strconv.ParseInt(txt, 10, 64); perr == nil {
				s.LastCommit = time.Unix(secs, 0)
			}
		}
	}

	return s, nil
}

// looksLikeRepo cheaply determines if `path` plausibly contains a git
// repository — `.git` exists as a directory or as a file (worktree
// pointer). Avoids invoking `git` at all for non-repo paths so a
// scan over a `Projects/` folder doesn't fork hundreds of processes
// for plain directories.
func looksLikeRepo(path string) bool {
	gitPath := filepath.Join(path, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return false
	}
	// Both regular .git directories AND worktree files (text file
	// pointing at the real .git) are valid.
	return info.IsDir() || info.Mode().IsRegular()
}

// runGit invokes `git -C <path> <args...>` with the package timeout.
// Returns combined stdout (stderr is captured for error context but
// not returned to keep parsing predictable).
func runGit(path string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitCommandTimeout)
	defer cancel()
	all := append([]string{"-C", path}, args...)
	cmd := exec.CommandContext(ctx, "git", all...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// parseStatus extracts Branch, Ahead, Behind, Dirty from the v2
// porcelain output. Lines come in three forms relevant to us:
//
//   # branch.head <branchname>
//   # branch.ab +<ahead> -<behind>
//   1 <XY>... | 2 <XY>... | u <XY>... | ? ...
//
// Anything we don't recognise is silently skipped — git can add new
// header types and we don't want to break on them.
func parseStatus(s *Status, out string) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "# branch.head "):
			s.Branch = strings.TrimSpace(strings.TrimPrefix(line, "# branch.head "))
			if s.Branch == "(detached)" {
				s.Branch = ""
			}
		case strings.HasPrefix(line, "# branch.ab "):
			// Format: "# branch.ab +N -M"
			rest := strings.TrimPrefix(line, "# branch.ab ")
			fields := strings.Fields(rest)
			for _, f := range fields {
				if strings.HasPrefix(f, "+") {
					if n, err := strconv.Atoi(strings.TrimPrefix(f, "+")); err == nil {
						s.Ahead = n
					}
				} else if strings.HasPrefix(f, "-") {
					if n, err := strconv.Atoi(strings.TrimPrefix(f, "-")); err == nil {
						s.Behind = n
					}
				}
			}
		case strings.HasPrefix(line, "# "):
			// Other header (branch.oid, branch.upstream) — ignore.
		default:
			// Per-file change record: 1, 2, u, ? all count as a
			// single dirty entry. Exception: "?" lines are
			// untracked-untracked-yet — counted because the user
			// generally cares about "is there anything new" too.
			s.Dirty++
		}
	}
}
