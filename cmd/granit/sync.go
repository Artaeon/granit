package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// runSync handles "granit sync [vault-path]" — pull, commit all changes, push.
func runSync(args []string) {
	vaultPath := "."
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		vaultPath = args[0]
	} else if envVault := os.Getenv("GRANIT_VAULT"); envVault != "" {
		vaultPath = envVault
	}

	// Validate path
	info, err := os.Stat(vaultPath)
	if err != nil || !info.IsDir() {
		exitError("Not a valid directory: %s", vaultPath)
	}

	quiet := hasFlag("--quiet") || hasFlag("-q")
	dryRun := hasFlag("--dry-run")

	// Auto-initialize git if not a repo
	if !isGitRepo(vaultPath) {
		if !quiet {
			fmt.Print("  Initializing git repository...")
		}
		if !dryRun {
			if _, err := gitCmd(vaultPath, "init"); err != nil {
				exitError("Failed to initialize git: %v", err)
			}
			gitignorePath := filepath.Join(vaultPath, ".gitignore")
			if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
				if err := os.WriteFile(gitignorePath, []byte(".granit/\n.DS_Store\n*.swp\n*.swo\n*~\n"), 0644); err != nil {
					fmt.Fprintf(os.Stderr, "warning: failed to create .gitignore: %v\n", err)
				}
			}
		}
		if !quiet {
			fmt.Println(" done.")
		}
	}

	if !quiet {
		fmt.Printf("Syncing vault: %s\n", vaultPath)
	}

	// Step 1: Pull (rebase)
	if !quiet {
		fmt.Print("  Pulling...")
	}
	if !dryRun {
		pullOut, pullErr := gitCmd(vaultPath, "pull", "--rebase", "--quiet")
		if pullErr != nil {
			// Check for conflict
			if strings.Contains(pullOut, "CONFLICT") || strings.Contains(pullErr.Error(), "CONFLICT") {
				if !quiet {
					fmt.Println(" conflict detected, auto-resolving (accepting newest)...")
				}
				resolveConflictsNewest(vaultPath, quiet)
			} else if strings.Contains(pullErr.Error(), "no tracking information") ||
				strings.Contains(pullErr.Error(), "no such ref") {
				if !quiet {
					fmt.Println(" no remote configured, skipping.")
				}
			} else {
				if !quiet {
					fmt.Printf(" warning: %s\n", strings.TrimSpace(pullOut))
				}
			}
		} else if !quiet {
			fmt.Println(" done.")
		}
	} else if !quiet {
		fmt.Println(" (dry-run)")
	}

	// Step 2: Check for changes
	status, err := gitCmd(vaultPath, "status", "--porcelain")
	if err != nil {
		exitError("Error checking status: %v", err)
	}
	if strings.TrimSpace(status) == "" {
		if !quiet {
			fmt.Println("  Nothing to commit, vault is clean.")
		}
		return
	}

	// Step 3: Stage all
	if !quiet {
		changes := countChanges(status)
		fmt.Printf("  Staging %d change(s)...", changes)
	}
	if !dryRun {
		if _, err := gitCmd(vaultPath, "add", "-A"); err != nil {
			exitError("Error staging changes: %v", err)
		}
	}
	if !quiet {
		fmt.Println(" done.")
	}

	// Step 4: Commit
	msg := fmt.Sprintf("vault: sync %s", time.Now().Format("2006-01-02 15:04:05"))
	if customMsg := getFlagValue("--message"); customMsg != "" {
		msg = customMsg
	} else if customMsg := getFlagValue("-m"); customMsg != "" {
		msg = customMsg
	}
	if !quiet {
		fmt.Print("  Committing...")
	}
	if !dryRun {
		if _, err := gitCmd(vaultPath, "commit", "-m", msg); err != nil {
			exitError("Error committing: %v", err)
		}
	}
	if !quiet {
		fmt.Println(" done.")
	}

	// Step 5: Push
	if !quiet {
		fmt.Print("  Pushing...")
	}
	if !dryRun {
		pushOut, pushErr := gitCmd(vaultPath, "push", "--quiet")
		if pushErr != nil {
			if strings.Contains(pushOut, "no upstream") ||
				strings.Contains(pushErr.Error(), "no upstream") ||
				strings.Contains(pushErr.Error(), "no configured push destination") {
				if !quiet {
					fmt.Println(" no remote configured, skipping push.")
				}
			} else {
				if !quiet {
					fmt.Printf(" warning: push failed (%v). Commit saved locally.\n", pushErr)
				}
			}
		} else if !quiet {
			fmt.Println(" done.")
		}
	} else if !quiet {
		fmt.Println(" (dry-run)")
	}

	if !quiet {
		fmt.Println("  Sync complete.")
	}
}

// isGitRepo checks if the given path is inside a git repository.
func isGitRepo(path string) bool {
	out, err := gitCmd(path, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// gitCmd runs a git command in the given directory and returns combined output.
func gitCmd(dir string, args ...string) (string, error) {
	fullArgs := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", fullArgs...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// resolveConflictsNewest auto-resolves git conflicts by accepting the newest version.
func resolveConflictsNewest(vaultPath string, quiet bool) {
	// Get list of conflicted files
	status, err := gitCmd(vaultPath, "status", "--porcelain")
	if err != nil {
		exitError("Error checking conflict status: %v", err)
	}
	for _, line := range strings.Split(status, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < 3 {
			continue
		}
		// UU = both modified (conflict), AA = both added
		code := line[:2]
		file := strings.TrimSpace(line[3:])
		if code == "UU" || code == "AA" {
			if !quiet {
				fmt.Printf("    Resolving conflict: %s (accepting theirs)\n", file)
			}
			if _, err := gitCmd(vaultPath, "checkout", "--theirs", file); err != nil {
				fmt.Fprintf(os.Stderr, "    Warning: could not resolve %s: %v\n", file, err)
				continue
			}
			if _, err := gitCmd(vaultPath, "add", file); err != nil {
				fmt.Fprintf(os.Stderr, "    Warning: could not stage %s: %v\n", file, err)
			}
		}
	}
	// Continue the rebase
	if _, err := gitCmd(vaultPath, "rebase", "--continue"); err != nil {
		// If rebase continue fails, try to just commit
		_, _ = gitCmd(vaultPath, "-c", "core.editor=true", "rebase", "--continue")
	}
}

// countChanges counts the number of changed files from porcelain output.
func countChanges(status string) int {
	count := 0
	for _, line := range strings.Split(status, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}
