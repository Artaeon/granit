package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// autoSyncResultMsg carries the result of a background git auto-sync operation.
type autoSyncResultMsg struct {
	action string // "pull", "commit", "push"
	output string
	err    error
}

// AutoSync manages automatic git commit and push on save, and pull on open.
type AutoSync struct {
	enabled   bool
	vaultPath string
}

// NewAutoSync creates a new AutoSync instance for the given vault.
func NewAutoSync(vaultPath string) AutoSync {
	return AutoSync{vaultPath: vaultPath}
}

// SetEnabled enables or disables auto-sync.
func (a *AutoSync) SetEnabled(enabled bool) {
	a.enabled = enabled
}

// IsEnabled returns whether auto-sync is active.
func (a *AutoSync) IsEnabled() bool {
	return a.enabled
}

// isGitRepo checks if the vault path is inside a git repository.
func (a *AutoSync) isGitRepo() bool {
	cmd := exec.Command("git", "-C", a.vaultPath, "rev-parse", "--is-inside-work-tree")
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}


// PullOnOpen runs git pull in the background when the vault is opened.
// Returns a tea.Cmd that performs the pull asynchronously.
// On conflict, auto-resolves by accepting the newest (remote) version.
func (a *AutoSync) PullOnOpen() tea.Cmd {
	if !a.enabled || !a.isGitRepo() {
		return nil
	}
	vaultPath := a.vaultPath
	return func() tea.Msg {
		gitRun := func(args ...string) (string, error) {
			fullArgs := append([]string{"-C", vaultPath}, args...)
			cmd := exec.Command("git", fullArgs...)
			out, err := cmd.CombinedOutput()
			return string(out), err
		}

		out, err := gitRun("pull", "--rebase", "--quiet")
		if err == nil {
			return autoSyncResultMsg{action: "pull", output: strings.TrimSpace(out)}
		}

		// Check if the error is a rebase conflict
		if !strings.Contains(out, "CONFLICT") && !strings.Contains(out, "conflict") {
			return autoSyncResultMsg{action: "pull", output: out, err: err}
		}

		// Auto-resolve conflicts by accepting the newest (theirs) version
		resolved := 0
		status, statusErr := gitRun("status", "--porcelain")
		if statusErr == nil {
			for _, line := range strings.Split(status, "\n") {
				line = strings.TrimSpace(line)
				if len(line) < 3 {
					continue
				}
				code := line[:2]
				file := strings.TrimSpace(line[3:])
				if code == "UU" || code == "AA" {
					if _, resolveErr := gitRun("checkout", "--theirs", file); resolveErr == nil {
						_, _ = gitRun("add", file)
						resolved++
					}
				}
			}
		}

		// Continue the rebase
		if resolved > 0 {
			_, _ = gitRun("rebase", "--continue")
			msg := fmt.Sprintf("auto-resolved %d conflict(s) (accepted newest)", resolved)
			return autoSyncResultMsg{action: "pull", output: msg}
		}

		// If we couldn't resolve, abort the rebase to leave vault usable
		_, _ = gitRun("rebase", "--abort")
		return autoSyncResultMsg{action: "pull", output: "conflict detected, rebase aborted", err: err}
	}
}

// CommitAndPush stages all changes, commits with a timestamped message,
// and pushes to the remote. Returns a tea.Cmd that runs asynchronously.
func (a *AutoSync) CommitAndPush() tea.Cmd {
	if !a.enabled {
		return nil
	}
	if !a.isGitRepo() {
		// Auto-init git for the vault
		cmd := exec.Command("git", "-C", a.vaultPath, "init")
		if err := cmd.Run(); err != nil {
			return nil
		}
		// Create .gitignore if missing
		gitignorePath := a.vaultPath + "/.gitignore"
		if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
			_ = os.WriteFile(gitignorePath, []byte(".granit/\n.DS_Store\n*.swp\n*.swo\n*~\n"), 0644)
		}
	}
	vaultPath := a.vaultPath
	return func() tea.Msg {
		gitIn := func(args ...string) (string, error) {
			fullArgs := append([]string{"-C", vaultPath}, args...)
			cmd := exec.Command("git", fullArgs...)
			out, err := cmd.CombinedOutput()
			return string(out), err
		}

		// Check for changes first
		status, err := gitIn("status", "--porcelain")
		if err != nil {
			return autoSyncResultMsg{action: "commit", err: err}
		}
		if strings.TrimSpace(status) == "" {
			// Nothing to commit
			return autoSyncResultMsg{action: "commit", output: "nothing to commit"}
		}

		// Stage all changes
		if _, err := gitIn("add", "-A"); err != nil {
			return autoSyncResultMsg{action: "commit", err: fmt.Errorf("git add: %w", err)}
		}

		// Commit with timestamp
		msg := fmt.Sprintf("vault: auto-save %s", time.Now().Format("2006-01-02 15:04:05"))
		if _, err := gitIn("commit", "-m", msg); err != nil {
			return autoSyncResultMsg{action: "commit", err: fmt.Errorf("git commit: %w", err)}
		}

		// Try to push (non-fatal if it fails — no remote configured, etc.)
		out, pushErr := gitIn("push", "--quiet")
		if pushErr != nil {
			// Push failed but commit succeeded
			return autoSyncResultMsg{action: "push", output: "committed (push failed: " + strings.TrimSpace(out) + ")"}
		}

		return autoSyncResultMsg{action: "push", output: "synced"}
	}
}
