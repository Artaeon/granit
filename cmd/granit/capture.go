package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func runCapture() {
	vaultPath := "."
	if v := getFlagValue("--vault"); v != "" {
		vaultPath = v
	} else if envVault := os.Getenv("GRANIT_VAULT"); envVault != "" {
		vaultPath = envVault
	}

	toDaily := hasFlag("--daily")
	readStdin := hasFlag("--stdin")
	targetFile := getFlagValue("--to")

	// Determine the target file path
	var targetPath string
	if toDaily {
		today := time.Now().Format("2006-01-02")
		targetPath = filepath.Join(vaultPath, today+".md")
		// Create daily note if it doesn't exist
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			content := fmt.Sprintf("---\ndate: %s\ntype: daily\n---\n\n# %s\n\n## Tasks\n- [ ]\n\n## Notes\n\n", today, today)
			if err := os.WriteFile(targetPath, []byte(content), 0644); err != nil {
				exitError("Error creating daily note: %v", err)
			}
			fmt.Fprintf(os.Stderr, "Created daily note: %s\n", targetPath)
		}
	} else if targetFile != "" {
		targetPath = filepath.Join(vaultPath, targetFile)
	} else {
		targetPath = filepath.Join(vaultPath, "Inbox.md")
	}

	// Collect the text to append
	var text string
	if readStdin || !isTerminal() {
		// Read from stdin
		scanner := bufio.NewScanner(os.Stdin)
		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			exitError("Error reading stdin: %v", err)
		}
		text = strings.Join(lines, "\n")
	} else {
		// Collect from positional arguments
		args := getPositionalArgs(2)
		if len(args) == 0 {
			exitError("Usage: granit capture <text>\n  granit capture --to Tasks.md \"- [ ] Fix bug\"\n  echo \"ideas\" | granit capture --stdin\n  granit capture --daily \"Meeting notes\"")
		}
		text = strings.Join(args, " ")
	}

	if text == "" {
		exitError("Nothing to capture.")
	}

	// Ensure the target file exists; create if not
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		// Create the file with a simple header
		baseName := strings.TrimSuffix(filepath.Base(targetPath), ".md")
		header := fmt.Sprintf("# %s\n\n", baseName)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			exitError("Error creating directory: %v", err)
		}
		if err := os.WriteFile(targetPath, []byte(header), 0644); err != nil {
			exitError("Error creating file: %v", err)
		}
	}

	// Append the text with a newline
	f, err := os.OpenFile(targetPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		exitError("Error opening file: %v", err)
	}
	defer f.Close()

	entry := text + "\n"
	if _, err := f.WriteString(entry); err != nil {
		exitError("Error writing to file: %v", err)
	}

	rel, _ := filepath.Rel(vaultPath, targetPath)
	if rel == "" {
		rel = targetPath
	}
	fmt.Fprintf(os.Stderr, "Captured to %s\n", rel)
}

// isTerminal checks if stdin is connected to a terminal (not a pipe).
func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return true
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
