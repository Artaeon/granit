package serveapi

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Syncer keeps a server-hosted vault checkout in sync with its git remote.
//
//   - On a tick (interval, default 60s): git pull --rebase --autostash.
//   - If working tree is dirty: git add -A, commit, push.
//
// This means a TUI commit pushed locally lands on the web within `interval`
// seconds, and any web-side write commits + pushes back so the loop closes.
//
// Conflict policy: --rebase --autostash + "newest wins" if a manual conflict
// occurs (delegated to git's defaults). For a single-tenant vault edited by
// one user across two devices, conflicts are rare.
type Syncer struct {
	vaultRoot string
	interval  time.Duration
	log       *slog.Logger

	mu       sync.Mutex
	lastPull time.Time
	lastPush time.Time
	lastErr  error
	pulls    int
	pushes   int
}

func NewSyncer(vaultRoot string, interval time.Duration, log *slog.Logger) *Syncer {
	if log == nil {
		log = slog.Default()
	}
	if interval < 10*time.Second {
		interval = 10 * time.Second
	}
	return &Syncer{vaultRoot: vaultRoot, interval: interval, log: log}
}

func (s *Syncer) Run(ctx context.Context) {
	if !isGitRepo(s.vaultRoot) {
		s.log.Warn("granit web --sync: vault is not a git repo, sync disabled", "root", s.vaultRoot)
		return
	}
	s.log.Info("git auto-sync running", "interval", s.interval)
	// Run once immediately on startup so the server starts with the latest.
	s.syncOnce()
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.syncOnce()
		}
	}
}

type SyncStatus struct {
	Enabled  bool      `json:"enabled"`
	Interval string    `json:"interval"`
	LastPull time.Time `json:"lastPull,omitempty"`
	LastPush time.Time `json:"lastPush,omitempty"`
	Pulls    int       `json:"pulls"`
	Pushes   int       `json:"pushes"`
	LastErr  string    `json:"lastErr,omitempty"`
}

func (s *Syncer) Status() SyncStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := SyncStatus{
		Enabled:  true,
		Interval: s.interval.String(),
		LastPull: s.lastPull,
		LastPush: s.lastPush,
		Pulls:    s.pulls,
		Pushes:   s.pushes,
	}
	if s.lastErr != nil {
		out.LastErr = s.lastErr.Error()
	}
	return out
}

func (s *Syncer) syncOnce() {
	if _, err := s.git("pull", "--rebase", "--autostash", "--quiet"); err != nil {
		s.recordErr(fmt.Errorf("pull: %w", err))
		return
	}
	s.mark(true, false)

	out, err := s.git("status", "--porcelain")
	if err != nil {
		s.recordErr(fmt.Errorf("status: %w", err))
		return
	}
	if strings.TrimSpace(out) == "" {
		s.recordErr(nil)
		return // working tree clean
	}

	if _, err := s.git("add", "-A"); err != nil {
		s.recordErr(fmt.Errorf("add: %w", err))
		return
	}
	msg := fmt.Sprintf("granit web auto-sync %s", time.Now().Format("2006-01-02 15:04:05"))
	if _, err := s.git("commit", "-m", msg); err != nil {
		s.recordErr(fmt.Errorf("commit: %w", err))
		return
	}
	if _, err := s.git("push", "--quiet"); err != nil {
		s.recordErr(fmt.Errorf("push: %w", err))
		return
	}
	s.mark(false, true)
	s.recordErr(nil)
	s.log.Info("git auto-sync committed", "msg", msg)
}

func (s *Syncer) git(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = s.vaultRoot
	// Inherit env so SSH agents / GIT_* settings work.
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (s *Syncer) mark(pull, push bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if pull {
		s.lastPull = time.Now()
		s.pulls++
	}
	if push {
		s.lastPush = time.Now()
		s.pushes++
	}
}

func (s *Syncer) recordErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastErr = err
	if err != nil {
		s.log.Warn("granit web sync error", "err", err)
	}
}

func isGitRepo(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, ".git"))
	if err != nil {
		return false
	}
	return st.IsDir() || !st.IsDir() // .git can be a file (worktree) too
}
