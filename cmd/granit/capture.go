package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/config"
)

// resolveCaptureVault determines the vault path for capture/clip commands.
// Priority: --vault / -v flag > GRANIT_VAULT env > last opened vault > cwd.
func resolveCaptureVault() string {
	var vaultPath string
	if v := getFlagValue("--vault"); v != "" {
		vaultPath = v
	} else if v := getFlagValue("-v"); v != "" {
		vaultPath = v
	} else if envVault := os.Getenv("GRANIT_VAULT"); envVault != "" {
		vaultPath = envVault
	} else if last := config.LoadVaultList().LastUsed; last != "" {
		vaultPath = last
	} else {
		vaultPath = "."
	}
	// Validate that the resolved path is a directory.
	info, err := os.Stat(vaultPath)
	if err != nil {
		exitError("Vault path does not exist: %s", vaultPath)
	}
	if !info.IsDir() {
		exitError("Vault path is not a directory: %s", vaultPath)
	}
	return vaultPath
}

// resolveTargetFile returns the target filename from flags or default.
func resolveTargetFile() string {
	if f := getFlagValue("--file"); f != "" {
		return f
	}
	if f := getFlagValue("-f"); f != "" {
		return f
	}
	return "inbox.md"
}

// ensureTargetFile creates the target file with frontmatter if it doesn't exist.
func ensureTargetFile(targetPath string) {
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		return
	}
	baseName := strings.TrimSuffix(filepath.Base(targetPath), ".md")
	today := time.Now().Format("2006-01-02")
	header := fmt.Sprintf("---\ntitle: %s\ndate: %s\ntype: inbox\n---\n\n# %s\n", baseName, today, baseName)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		exitError("Error creating directory: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte(header), 0644); err != nil {
		exitError("Error creating file: %v", err)
	}
}

// appendCapture writes a timestamped entry to the target file and prints confirmation.
func appendCapture(vaultPath, targetPath, text string) {
	ensureTargetFile(targetPath)

	// Read existing content.
	existing, err := os.ReadFile(targetPath)
	if err != nil {
		exitError("Error reading file: %v", err)
	}

	// Build new content.
	timestamp := time.Now().Format("15:04")
	entry := fmt.Sprintf("\n- **%s** — %s\n", timestamp, text)
	newContent := append(existing, []byte(entry)...)

	// Atomic write via temp file + rename.
	tmpPath := targetPath + ".tmp"
	if err := os.WriteFile(tmpPath, newContent, 0644); err != nil {
		_ = os.Remove(tmpPath)
		exitError("Error writing to file: %v", err)
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		_ = os.Remove(tmpPath)
		exitError("Error saving file: %v", err)
	}

	rel, _ := filepath.Rel(vaultPath, targetPath)
	if rel == "" {
		rel = targetPath
	}
	fmt.Printf("Captured to %s\n", rel)
}

// runCapture handles "granit capture <text>" — appends text to inbox with timestamp.
func validateTargetInVault(vaultPath, targetPath string) {
	absVault, err := filepath.Abs(vaultPath)
	if err != nil {
		exitError("Error resolving vault path: %v", err)
	}
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		exitError("Error resolving target path: %v", err)
	}
	rel, err := filepath.Rel(absVault, absTarget)
	if err != nil || strings.HasPrefix(rel, "..") {
		exitError("Target file %q is outside the vault directory", targetPath)
	}
}

func runCapture() {
	vaultPath := resolveCaptureVault()
	targetFile := resolveTargetFile()
	targetPath := filepath.Join(vaultPath, targetFile)
	validateTargetInVault(vaultPath, targetPath)

	args := getCapturePositionalArgs()
	if len(args) == 0 {
		exitError("Usage: granit capture \"some text\"\n       granit capture -v ~/notes -f tasks.md \"Buy milk\"")
	}
	text := strings.Join(args, " ")

	appendCapture(vaultPath, targetPath, text)
}

// runClip handles "granit clip" — reads stdin and appends to inbox with timestamp.
func runClip() {
	vaultPath := resolveCaptureVault()
	targetFile := resolveTargetFile()
	targetPath := filepath.Join(vaultPath, targetFile)
	validateTargetInVault(vaultPath, targetPath)

	scanner := bufio.NewScanner(os.Stdin)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		exitError("Error reading stdin: %v", err)
	}
	text := strings.TrimSpace(strings.Join(lines, "\n"))
	if text == "" {
		exitError("Nothing to capture. Pipe text into granit clip:\n       echo \"idea\" | granit clip")
	}

	appendCapture(vaultPath, targetPath, text)
}

// getCapturePositionalArgs returns non-flag arguments after the subcommand,
// skipping both long (--key value) and short (-k value) flags.
func getCapturePositionalArgs() []string {
	var args []string
	for i := 2; i < len(os.Args); i++ {
		arg := os.Args[i]
		if strings.HasPrefix(arg, "--") || strings.HasPrefix(arg, "-") {
			// Skip --key=value
			if strings.Contains(arg, "=") {
				continue
			}
			// Skip the next arg too (it's the flag value)
			i++
			continue
		}
		args = append(args, arg)
	}
	return args
}

// isTerminal checks if stdin is connected to a terminal (not a pipe).
func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return true
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
